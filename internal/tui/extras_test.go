package tui

import (
	"image"
	stdcolor "image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/fontdl"
	"github.com/vitruves/ghostty-config/internal/fontinfo"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// command finds a command of the registry by name.
func (h *harness) command(name string) *command {
	h.t.Helper()
	for _, c := range h.m.commands() {
		if c.name == name {
			return c
		}
	}
	h.t.Fatalf("no command %s", name)
	return nil
}

// set runs a setting with a value, as Enter on that value would.
func (h *harness) set(name, value string) {
	h.t.Helper()
	h.command(name).run(h.m, "", &option{value: value})
}

func (h *harness) value(name string) string {
	h.t.Helper()
	return h.command(name).set.get(h.m)
}

// landscape writes a small picture to path: a sky, a sun and hills, which is
// enough to see where an image lands in the window picture.
func landscape(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fy := float64(y) / float64(h)
			c := stdcolor.NRGBA{uint8(250 - 120*fy), uint8(170 - 60*fy), uint8(120 + 100*fy), 255}
			if math.Hypot(float64(x)-float64(w)*0.68, float64(y)-float64(h)*0.42) < float64(h)*0.14 {
				c = stdcolor.NRGBA{255, 226, 150, 255}
			}
			if float64(y) > float64(h)*(0.7+0.08*math.Sin(float64(x)/float64(w)*7)) {
				c = stdcolor.NRGBA{uint8(40 + 30*fy), uint8(70 + 20*fy), 80, 255}
			}
			img.Set(x, y, c)
		}
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if strings.HasSuffix(path, ".jpg") {
		err = jpeg.Encode(f, img, nil)
	} else {
		err = png.Encode(f, img)
	}
	if err != nil {
		t.Fatal(err)
	}
}

// Each flag of bell-features and shell-integration-features is a switch of
// its own, and switching one rewrites only its word of the list.
func TestFlagSettingsKeepTheWordsTheyDoNotKnow(t *testing.T) {
	h := newHarness(t, "theme = Dracula\nbell-features = future-thing,no-title\nshell-integration-features = no-cursor\n")
	if h.value("belltitle") != "off" || h.value("bellbounce") != "on" || h.value("bellborder") != "off" {
		t.Fatalf("flags read wrong: title %s attention %s border %s", h.value("belltitle"), h.value("bellbounce"), h.value("bellborder"))
	}
	h.set("bellborder", "on")
	if got := activeLines(h.config(), "bell-features"); len(got) != 1 || got[0] != "future-thing,no-title,border" {
		t.Fatalf("border on: %v", got)
	}
	h.set("belltitle", "on")
	if got := activeLines(h.config(), "bell-features"); len(got) != 1 || got[0] != "future-thing,border" {
		t.Fatalf("title back to its default drops its word: %v", got)
	}
	h.set("shellcursor", "on")
	h.set("sshenv", "on")
	if got := activeLines(h.config(), "shell-integration-features"); len(got) != 1 || got[0] != "ssh-env" {
		t.Fatalf("shell features: %v", got)
	}
	if !strings.Contains(h.config(), "#shell-integration-features = no-cursor") {
		t.Fatalf("the line rewritten keeps its place:\n%s", h.config())
	}

	// The sound file and its volume only matter with the custom sound on.
	if h.m.listed(h.command("bellfile")) || h.m.listed(h.command("bellvolume")) {
		t.Fatal("the sound file is listed with the custom sound off")
	}
	h.set("bellaudio", "on")
	if !h.m.listed(h.command("bellfile")) || !h.m.listed(h.command("bellvolume")) {
		t.Fatal("the sound file should be listed once the custom sound is on")
	}
	if got := activeLines(h.config(), "bell-features"); got[0] != "future-thing,border,audio" {
		t.Fatalf("audio on: %v", got)
	}
}

// The settings a platform lacks are not offered there.
func TestQuickTerminalAndLaunchSettingsFollowThePlatform(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	has := map[string]bool{}
	for _, c := range h.m.commands() {
		has[c.name] = true
	}
	mac := runtime.GOOS == "darwin"
	for name, want := range map[string]bool{
		"quickposition": true, "quicksize": true, "quickhide": true, "columns": true, "rows": true, "shell": true, "bellsound": true,
		"quickscreen": mac, "quickanimation": mac, "quickspaces": mac, "windowx": mac, "windowy": mac,
		"quickkeyboard": !mac, "quicklayer": !mac,
	} {
		if has[name] != want {
			t.Errorf("%s offered %v on %s, want %v", name, has[name], runtime.GOOS, want)
		}
	}
	// The quick terminal names the binding that opens it, which the keys
	// command sets.
	if s := h.command("quickposition").set; !strings.Contains(s.hint, "keys toggle_quick_terminal") {
		t.Fatalf("the quick terminal should say how to give it a shortcut: %q", s.hint)
	}
	wantHide := map[bool]string{true: "on", false: "off"}[mac]
	if h.value("quickhide") != wantHide {
		t.Errorf("autohide defaults to %s here, reads %s", wantHide, h.value("quickhide"))
	}
}

// A size at launch needs both columns and rows: setting one fills in the
// other, default takes the line away.
func TestLaunchSizeFillsInItsPartner(t *testing.T) {
	h := newHarness(t, "theme = Dracula\nquick-terminal-size = 30%,600px\n")
	if h.value("columns") != "default" {
		t.Fatalf("columns read %s", h.value("columns"))
	}
	h.set("columns", "120")
	if !strings.Contains(h.config(), "window-width = 120\n") || !strings.Contains(h.config(), "window-height = 24\n") {
		t.Fatalf("columns alone should bring rows with them:\n%s", h.config())
	}
	h.set("rows", "40")
	h.set("columns", "default")
	if strings.Contains(h.config(), "\nwindow-width") || !strings.Contains(h.config(), "window-height = 40\n") {
		t.Fatalf("default takes the columns away and leaves the rows:\n%s", h.config())
	}
	// Typed values snap to the scale and stay inside it.
	h.command("rows").run(h.m, "1", nil)
	if !strings.Contains(h.config(), "window-height = 4\n") {
		t.Fatalf("rows below the minimum should be the minimum:\n%s", h.config())
	}

	// The quick terminal's second size, across, is kept.
	if h.value("quicksize") != "30%" {
		t.Fatalf("quick size read %s", h.value("quicksize"))
	}
	h.set("quicksize", "50%")
	if got := activeLines(h.config(), "quick-terminal-size"); len(got) != 1 || got[0] != "50%,600px" {
		t.Fatalf("quick size: %v", got)
	}
	h.set("quicksize", "default")
	if len(activeLines(h.config(), "quick-terminal-size")) != 0 {
		t.Fatalf("default takes the quick size away:\n%s", h.config())
	}
}

// The background image is picked from the files likely places hold, or
// typed; only a real PNG or JPEG can be chosen, and the settings that shape
// it appear with it.
func TestBackgroundImageIsChosenFromFilesOrTyped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	landscape(t, filepath.Join(home, "Pictures", "dunes.png"), 64, 40)
	landscape(t, filepath.Join(home, "Downloads", "lake.jpg"), 64, 40)
	os.WriteFile(filepath.Join(home, "Pictures", "fake.png"), []byte("not a picture"), 0o644)
	h := newHarness(t, "theme = Dracula\n")

	if h.m.listed(h.command("bgfit")) {
		t.Fatal("the fit of no image is listed")
	}
	h.typeText("bgimage ")
	var labels []string
	for _, o := range h.m.results {
		labels = append(labels, o.label)
	}
	if got := strings.Join(labels, " "); got != "none dunes.png lake.jpg" {
		t.Fatalf("the list should hold none and the two real pictures, not the fake: %q", got)
	}
	h.typeText("~/Pictures/nope.png")
	if o := h.m.results[len(h.m.results)-1]; !o.header || o.detail != "no such file" {
		t.Fatalf("a missing file is said so and cannot be chosen: %+v", h.m.results)
	}
	h.key("enter")
	if strings.Contains(h.config(), "background-image") {
		t.Fatalf("Enter on a missing file wrote it:\n%s", h.config())
	}
	h.m.setPromptText("Bgimage ~/Downloads/lake.jpg")
	h.key("enter")
	want := filepath.Join(home, "Downloads", "lake.jpg")
	if got := activeLines(h.config(), "background-image"); len(got) != 1 || got[0] != want {
		t.Fatalf("the typed path should be written in full: %v", got)
	}
	if !h.m.listed(h.command("bgfit")) || h.m.currentValueOf(h.command("bgimage")) != "lake.jpg" {
		t.Fatal("with an image, its settings are listed and the menu names the file")
	}
	h.set("bgfit", "cover")
	h.set("bgimage", "none")
	if len(activeLines(h.config(), "background-image")) != 0 {
		t.Fatalf("none takes the image away:\n%s", h.config())
	}
}

// fakeFont gives the first family of the harness a file, and what reading
// it found, so the font's own settings appear without a real font.
func fakeFont(h *harness, name string, info *fontinfo.Info) {
	file := ghostty.FontFile{Path: filepath.Join(h.dir, name)}
	h.m.families[0].Files = map[string]ghostty.FontFile{"Regular": file}
	h.m.fontFamily = h.m.families[0].Name
	h.m.fx.infos = map[string]fontInfo{fileKey(file): {info: info}}
	h.send(tea.WindowSizeMsg{Width: h.m.width, Height: h.m.height})
}

// The features and axes of the font in use are settings of their own, which
// write font-feature and font-variation lines beside the ligatures.
func TestFontFeaturesAndAxesOfTheFontInUse(t *testing.T) {
	h := newHarness(t, "theme = Dracula\nfont-family = Hack\n")
	fakeFont(h, "Hack-Regular.ttf", &fontinfo.Info{
		Features: []fontinfo.Feature{{Tag: "calt"}, {Tag: "kern"}, {Tag: "ss01", Name: "Round dots"}, {Tag: "zero"}},
		Axes:     []fontinfo.Axis{{Tag: "wght", Name: "Weight", Min: 100, Default: 400, Max: 800}},
	})
	has := map[string]bool{}
	for _, c := range h.m.commands() {
		has[c.name] = true
	}
	if !has["ss01"] || !has["zero"] || !has["weight"] || has["calt"] || has["kern"] {
		t.Fatalf("the optional features and the axis should be offered, the ligatures and kerning not: %v", has)
	}
	if sub, _ := subOf(h.command("ss01")); sub != "Features" {
		t.Fatalf("ss01 is in %s", sub)
	}
	if h.command("ss01").set.label != "Round dots" {
		t.Fatal("a stylistic set is called by the font's own name for it")
	}
	h.set("ligatures", "off")
	h.set("ss01", "on")
	h.set("zero", "on")
	if got := activeLines(h.config(), "font-feature"); strings.Join(got, " | ") != "-calt, -liga, -dlig | ss01 | zero" {
		t.Fatalf("features beside the ligatures: %v", got)
	}
	h.set("ligatures", "on")
	h.set("ss01", "off")
	if got := activeLines(h.config(), "font-feature"); strings.Join(got, " | ") != "zero" {
		t.Fatalf("ligatures back on, ss01 off: %v", got)
	}
	if h.value("weight") != "default" {
		t.Fatalf("an axis nobody set is the font's default, reads %s", h.value("weight"))
	}
	h.set("weight", "350")
	if got := activeLines(h.config(), "font-variation"); len(got) != 1 || got[0] != "wght=350" {
		t.Fatalf("weight: %v", got)
	}
	h.set("weight", "default")
	if len(activeLines(h.config(), "font-variation")) != 0 {
		t.Fatalf("default takes the axis away:\n%s", h.config())
	}

	// Another font: its features replace these.
	fakeFont(h, "Other-Regular.ttf", &fontinfo.Info{})
	for _, c := range h.m.commands() {
		if c.name == "ss01" || c.name == "weight" {
			t.Fatalf("%s outlived the font that had it", c.name)
		}
	}
}

// The icons map to Symbols Nerd Font Mono when it is installed; when it is
// not, the Enter that turns them on downloads it and maps them in one go.
func TestNerdFontIconsDownloadTheirFamilyFirst(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	var fetched []string
	real := installFont
	installFont = func(d fontdl.FontDownload, _ func(string)) error {
		fetched = append(fetched, d.Archive)
		return nil
	}
	defer func() { installFont = real }()

	h.typeText("icons ")
	for h.m.selected() == nil || h.m.selected().value != "on" {
		h.key("up")
	}
	if !strings.Contains(h.view(), "Enter downloads it") {
		t.Fatalf("the panel should say that Enter downloads the family:\n%s", h.view())
	}
	// Enter starts the download, with no question in between; the command
	// is run by hand so the test stops before the machine's fonts would be
	// listed again.
	_, cmd := h.m.Update(keyPress("enter"))
	if h.m.overlay != overlayBusy || cmd == nil || strings.Contains(h.config(), "font-codepoint-map") {
		t.Fatalf("on should start the download at once, overlay %v:\n%s", h.m.overlay, h.config())
	}
	if !strings.Contains(h.m.busyText, "Symbols Nerd Font") {
		t.Fatalf("the progress should name what is downloaded: %q", h.m.busyText)
	}
	done := cmd().(installDoneMsg)
	h.m.Update(done)
	h.m.fontsLoaded, h.m.fontLoading = true, false
	if len(fetched) != 1 || fetched[0] != "NerdFontsSymbolsOnly" {
		t.Fatalf("fetched %v", fetched)
	}
	if got := activeLines(h.config(), "font-codepoint-map"); len(got) != 1 || !strings.HasSuffix(got[0], "=Symbols Nerd Font Mono") || !strings.HasPrefix(got[0], "U+23FB-U+23FE,") {
		t.Fatalf("the icons should be mapped once it is installed: %v", got)
	}
	h.set("icons", "off")
	if len(activeLines(h.config(), "font-codepoint-map")) != 0 {
		t.Fatalf("off takes the map away:\n%s", h.config())
	}

	// Installed already: on maps at once.
	h.m.families = append(h.m.families, ghostty.Family{Name: "Symbols Nerd Font Mono", Styles: []string{"Regular"}, Mono: true})
	h.set("icons", "on")
	if len(activeLines(h.config(), "font-codepoint-map")) != 1 || len(fetched) != 1 {
		t.Fatalf("an installed family is mapped without a download:\n%s", h.config())
	}
}

// The pictures of the new settings, for looking at: GHOSTTY_CONFIG_PXSHOTS.
func TestPixelExtraScreens(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	landscape(t, filepath.Join(home, "Pictures", "dunes.png"), 960, 600)
	landscape(t, filepath.Join(home, "Pictures", "Wallpapers", "harbour.jpg"), 800, 800)
	landscape(t, filepath.Join(home, "Downloads", "lake.jpg"), 1200, 500)

	h := newPixHarness(t, "midnight")
	h.m.setPromptText("")
	h.m.jumpToGroup("Window")
	h.highlightCommand("bgimage")
	h.shot("20-window-image-none")

	// Browsing the files: the window picture shows the highlighted one once
	// it is read, and moving never waits for it.
	h.m.setPromptText("Bgimage ")
	h.key("down")
	if load := h.m.wantThumbs(); load != nil {
		h.send(load())
	}
	if o := h.m.selected(); o == nil || h.m.thumbOf(o.value) == nil {
		t.Fatalf("the highlighted image was not read: %+v", h.m.selected())
	}
	if spec := h.m.windowSpec("bgimage", h.m.selected()); spec.Backdrop.Img == nil {
		t.Fatal("the window picture does not carry the highlighted image")
	}
	h.shot("21-image-files")
	h.key("enter")
	if !strings.Contains(h.config(), "background-image = ") {
		t.Fatalf("Enter did not write the image:\n%s", h.config())
	}
	h.m.setPromptText("Bgfit ")
	h.key("down")
	h.shot("22-image-fit-cover")
	h.m.setPromptText("")
	h.m.jumpToGroup("Window")
	h.set("bgopacity", "0.35")
	h.set("bgposition", "top-right")
	h.highlightCommand("bgopacity")
	h.shot("23-window-image-set")
	h.highlightCommand("columns")
	h.shot("24-window-size")

	h.m.jumpToGroup("Session")
	h.highlightCommand("quickposition")
	h.shot("25-session-quick")
	h.highlightCommand("bellborder")
	h.shot("26-session-bell")
	h.highlightCommand("sshenv")
	h.shot("27-session-shell")
	h.m.setPromptText("Shell ")
	h.shot("28-shell-values")

	h.m.setPromptText("")
	fakeFont(h, "Hack-Regular.ttf", &fontinfo.Info{
		Features: []fontinfo.Feature{{Tag: "calt"}, {Tag: "cv01"}, {Tag: "cv02"}, {Tag: "frac"}, {Tag: "ss01", Name: "Broken equals ligatures"},
			{Tag: "ss02", Name: "Broken compare and equal ligatures"}, {Tag: "ss03"}, {Tag: "zero"}},
		Axes: []fontinfo.Axis{{Tag: "wght", Name: "Weight", Min: 100, Default: 400, Max: 800}},
	})
	h.set("zero", "on")
	h.set("weight", "350")
	h.m.jumpToGroup("Fonts")
	h.highlightCommand("ss01")
	h.shot("29-fonts-features")
	h.highlightCommand("weight")
	h.shot("30-fonts-axis")
	h.highlightCommand("icons")
	h.shot("31-fonts-icons")
	h.m.setPromptText("Weight ")
	h.shot("32-weight-values")

	light := newPixHarness(t, "paper")
	light.m.setPromptText("")
	light.set("bgimage", filepath.Join(home, "Downloads", "lake.jpg"))
	light.set("bgfit", "cover")
	light.m.jumpToGroup("Window")
	if load := light.m.wantThumbs(); load != nil {
		light.send(load())
	}
	light.highlightCommand("bgfit")
	light.shot("33-window-image-light")
}

// The text interface shows the same settings: the value in the list, what it
// does beside it.
func TestCellsExtraScreens(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	landscape(t, filepath.Join(home, "Pictures", "dunes.png"), 64, 40)
	h := newHarness(t, "theme = Dracula\nbell-features = border\n")
	h.m.jumpToGroup("Session")
	h.highlightCommand("bellborder")
	v := h.view()
	for _, want := range []string{"Bellborder", "A border round the terminal that rang"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the bell lacks %q:\n%s", want, v)
		}
	}
	h.dump("40-cells-session")
	h.m.setPromptText("Bgimage ")
	h.key("down")
	v = h.view()
	for _, want := range []string{"dunes.png", "~/Pictures", "PNG"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the image list lacks %q:\n%s", want, v)
		}
	}
	h.dump("41-cells-images")
	h.m.setPromptText("Quickposition ")
	if v := h.view(); !strings.Contains(v, "toggle_quick_terminal") {
		t.Fatalf("the quick terminal should name its shortcut:\n%s", v)
	}
	h.dump("42-cells-quick")
}
