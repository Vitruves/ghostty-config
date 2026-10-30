package tui

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/vitruves/ghostty-config/internal/ghostty"
	"github.com/vitruves/ghostty-config/internal/pix"
)

const pixConfig = "theme = sumi-ink\nbackground-opacity = 0.85\nbackground-blur = macos-glass-regular\nmacos-titlebar-style = tabs\nwindow-padding-x = 14\nwindow-padding-color = extend\nwindow-save-state = always\n"

func newPixHarness(t *testing.T, iface string) *harness {
	t.Helper()
	h := newHarnessWith(t, pixConfig, Options{NoReload: true, Images: true, ImgOut: io.Discard, Pixel: true, PixCell: [2]int{17, 38}})
	h.m.state.Interface = iface
	h.send(tea.WindowSizeMsg{Width: 120, Height: 34})
	return h
}

// shot draws the screen as the terminal would show it and, when
// GHOSTTY_CONFIG_PXSHOTS names a directory, writes it there as a PNG.
func (h *harness) shot(name string) {
	h.t.Helper()
	m := h.m
	if !m.pixelOn() {
		h.t.Fatal("the picture interface is off")
	}
	// The real path, rows of placeholders included.
	if rows := strings.Split(m.render(), "\n"); len(rows) != m.height {
		h.t.Fatalf("%s: %d rows for a %d-row terminal", name, len(rows), m.height)
	}
	dir := os.Getenv("GHOSTTY_CONFIG_PXSHOTS")
	if dir == "" {
		return
	}
	m.regions = m.regions[:0]
	c := m.pixCanvas()
	m.drawScreen(c)
	var b bytes.Buffer
	_ = png.Encode(&b, c.Img)
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, name+".png"), b.Bytes(), 0o644)
}

func (h *harness) highlightTheme(name string) {
	for i, o := range h.m.results {
		if o.theme != nil && o.theme.Name == name {
			h.m.sel.cursor = i
			h.m.galAlign = true
			h.m.afterMove()
			return
		}
	}
	h.t.Fatalf("no theme %q on the wall", name)
}

func (h *harness) highlightCommand(name string) {
	for i, o := range h.m.results {
		if o.cmd != nil && o.cmd.name == name {
			h.m.sel.cursor = i
			return
		}
	}
	h.t.Fatalf("no command %q in the menu", name)
}

func TestPixelScreens(t *testing.T) {
	h := newPixHarness(t, "midnight")

	h.m.setPromptText("Theme ")
	h.highlightTheme("matcha-whisk")
	h.shot("01-themes")

	h.m.jumpToGroup("Window")
	h.highlightCommand("paddingcolor")
	h.shot("02-window")

	h.key("right")
	if !strings.Contains(h.config(), "window-padding-color = extend-always") {
		t.Fatalf("→ on the padding colour did not step it:\n%s", h.config())
	}
	h.key("delete")
	if strings.Contains(h.config(), "\nwindow-padding-color") {
		t.Fatalf("Delete did not put the padding colour back to its default:\n%s", h.config())
	}

	h.m.jumpToGroup("Fonts")
	h.shot("03-fonts")
	h.m.jumpToGroup("Input")
	h.shot("04-input")
	h.m.jumpToGroup("Session")
	h.shot("05-session")
	h.m.jumpToGroup("Extensions")
	h.shot("06-extensions")
	h.m.jumpToGroup("Tool")
	h.shot("07-tool")

	// The specimen is set in the family's own file; the embedded faces stand
	// in for one on disk.
	dir := filepath.Join("..", "kimg", "fonts")
	h.m.families[0].Files = map[string]ghostty.FontFile{
		"Regular": {Path: filepath.Join(dir, "sans-regular.ttf")},
		"Bold":    {Path: filepath.Join(dir, "sans-bold.ttf")},
	}
	h.m.setPromptText("Font ")
	if load := h.m.wantSpecimen(); load == nil {
		t.Fatal("the font list did not ask for the highlighted family's files")
	} else {
		h.send(load())
	}
	if h.m.specimenFont(h.m.families[0].Files["Regular"]) == nil {
		t.Fatal("the highlighted family's file was not read")
	}
	h.shot("08-font-list")
	before := h.config()
	h.key("down")
	if h.config() != before {
		t.Fatalf("moving through the families wrote the configuration:\n%s", h.config())
	}
	h.key("enter")
	if !strings.Contains(h.config(), "font-family = ") {
		t.Fatalf("Enter on a family did not write it:\n%s", h.config())
	}
	h.m.setPromptText("Opacity ")
	h.shot("09-opacity-values")

	h.m.setPromptText("")
	h.m.jumpToGroup("Window")
	h.m.overlay = overlayHelp
	h.shot("10-help")
	h.m.overlay = overlayNone
	h.m.openConfirm("Delete the theme file matcha-whisk from your themes directory? A backup is not kept.", nil)
	h.shot("11-confirm")
	h.m.overlay = overlayWelcome
	h.shot("12-welcome")
	h.m.overlay = overlayNone

	// The chooser, for a new user and for one who used the tool before
	// layouts were offered, at the size of a usual window and at 80×24, in
	// a dark and a light scheme.
	for _, iface := range []string{"midnight", "paper"} {
		for _, returning := range []bool{false, true} {
			ch := newChooserHarness(t, returning, Options{NoReload: true, Images: true, ImgOut: io.Discard, Pixel: true, PixCell: [2]int{17, 38}, Version: "v1.5.0"})
			ch.m.state.Interface = iface
			name := "12b-chooser-new"
			if returning {
				name = "12c-chooser-returning"
			}
			for _, size := range [][2]int{{120, 34}, {80, 24}} {
				ch.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				suffix := ""
				if size[0] == 80 {
					suffix = "-80x24"
				}
				if iface == "paper" {
					suffix += "-light"
				}
				ch.shot(name + suffix)
			}
			if returning && iface == "midnight" {
				ch.key("right")
				ch.shot(name + "-palette")
			}
		}
	}

	h.m.setPromptText("Edit ")
	h.key("enter")
	h.shot("13-editor")

	light := newPixHarness(t, "paper")
	light.m.setPromptText("Theme ")
	light.highlightTheme("sakura-fade")
	light.shot("14-themes-light")
	light.m.jumpToGroup("Window")
	light.highlightCommand("opacity")
	light.shot("15-window-light")

	// What keeps the files safe: the session's history, what it changed, the
	// backups, and Ghostty's own check of the config.
	s := newPixHarness(t, "midnight")
	s.do("size 15", true)
	s.do("paddingx 20", true)
	s.m.setPromptText("")
	s.m.jumpToGroup("Tool")
	s.highlightCommand("undo")
	s.shot("16-tool-history")
	s.do("changes", false)
	s.shot("17-changes")
	s.do("restore", false)
	s.shot("18-restore")
	s.key("enter")
	s.shot("19-restore-confirm")
	s.key("esc")
	// The answer is set here: no program is run for a picture.
	s.m.paths.Binary = filepath.Join(s.dir, "no-ghostty")
	s.m.problems = s.m.tree.Place(ghostty.ParseValidate("font-size: invalid value \"15\"\nmacos-titlebar-style: invalid value \"tabs\"\n"))
	s.m.checked, s.m.checkTook, s.m.checkOpened = true, 11*time.Millisecond, true
	s.m.setPromptText("Check ")
	s.shot("20-check")
	s.m.problems = nil
	s.m.setPromptText("Check ")
	s.shot("21-check-clean")
	s.m.setPromptText("")
	s.m.problems = s.m.tree.Place(ghostty.ParseValidate("font-size: invalid value \"15\"\n"))
	s.m.tellProblems(nil)
	s.shot("22-problem-status")

	light = newPixHarness(t, "paper")
	light.do("size 15", true)
	light.do("restore", false)
	light.shot("23-restore-light")

	palettePixelScreens(t)
	guidedShots(t)
}

// TestReadmeShots draws the pictures of the README when
// GHOSTTY_CONFIG_README names a directory: `make readme-shots`.
func TestReadmeShots(t *testing.T) {
	dir := os.Getenv("GHOSTTY_CONFIG_README")
	if dir == "" {
		t.Skip("GHOSTTY_CONFIG_README is not set")
	}
	t.Setenv("GHOSTTY_CONFIG_PXSHOTS", dir)
	harnessXDG = filepath.Join(".config", "ghostty")
	defer func() { harnessXDG = "xdg" }()
	shoot := func(iface string, name string, set func(h *harness)) {
		h := newPixHarness(t, iface)
		t.Setenv("HOME", h.dir)
		set(h)
		h.shot(name)
	}
	shoot("midnight", "themes", func(h *harness) {
		h.m.setPromptText("Theme ")
		h.highlightTheme("matcha-whisk")
	})
	shoot("paper", "window", func(h *harness) {
		h.m.setPromptText("")
		h.m.jumpToGroup("Window")
		h.highlightCommand("opacity")
	})
	shoot("midnight", "input", func(h *harness) {
		h.m.setPromptText("")
		h.m.jumpToGroup("Input")
		h.highlightCommand("cursor")
	})
	shoot("paper", "themes-light", func(h *harness) {
		h.m.setPromptText("Theme ")
		h.highlightTheme("sakura-fade")
	})
}

// The renderer clears the whole screen after a resize, and the terminal
// drops every picture with it: after a resize, every row must be sent again,
// even one whose pixels did not change.
func TestPicturesAreSentAgainAfterAResize(t *testing.T) {
	var out bytes.Buffer
	h := newHarnessWith(t, pixConfig, Options{NoReload: true, Images: true, ImgOut: &out, Pixel: true, PixCell: [2]int{17, 38}})
	h.m.View()
	first := strings.Count(out.String(), "a=T")
	if first == 0 {
		t.Fatal("the first frame sent no picture")
	}
	out.Reset()
	h.m.View()
	if strings.Contains(out.String(), "a=T") {
		t.Fatal("an unchanged frame sent its pictures again")
	}
	h.send(tea.WindowSizeMsg{Width: 130, Height: 38})
	h.m.View()
	if n := strings.Count(out.String(), "a=T"); n != first {
		t.Fatalf("after a resize %d pictures were sent, want all %d of the frame", n, first)
	}
}

// The controls of a setting answer to the mouse: a pill picks its value, a
// switch flips, a point along a slider sets the value under it.
func TestSettingControlsAreClickable(t *testing.T) {
	h := newPixHarness(t, "midnight")
	h.m.jumpToGroup("Window")

	pill, ok := h.find(hitValue, -1, "dark")
	if !ok {
		t.Fatal("no pill for a dark window theme")
	}
	h.click(pill.x+pill.w/2, pill.y)
	if !strings.Contains(h.config(), "window-theme = dark") {
		t.Fatalf("clicking the pill did not set it:\n%s", h.config())
	}

	toggle, ok := h.find(hitValue, -1, "on")
	if !ok {
		t.Fatal("no switch to turn on")
	}
	h.click(toggle.x+toggle.w/2, toggle.y)
	if o := h.m.results[toggle.index]; o.cmd.set.get(h.m) != "on" {
		t.Fatalf("clicking the switch of %s left it %s", o.cmd.set.key, o.cmd.set.get(h.m))
	}

	for _, r := range h.m.regions {
		if r.kind == hitValue && r.name == "slider" && h.m.results[r.index].cmd.name == "paddingx" {
			h.click(r.x, r.y)
			if !strings.Contains(h.config(), "window-padding-x = 0") {
				t.Fatalf("clicking the start of the slider did not set 0:\n%s", h.config())
			}
			h.click(r.x+r.w-1, r.y)
			if !strings.Contains(h.config(), "window-padding-x = 60") {
				t.Fatalf("clicking the end of the slider did not set 60:\n%s", h.config())
			}
			return
		}
	}
	t.Fatal("no slider for the left and right padding")
}

// A newer release opens a dialog: Next time closes it, Install replaces the
// binary through the installer and says so.
func TestUpdateDialog(t *testing.T) {
	h := newPixHarness(t, "midnight")
	h.m.opts.Version = "1.0.0"
	var asked string
	old := installRelease
	installRelease = func(_ context.Context, v string) (string, error) {
		asked = v
		return "/usr/local/bin/ghostty-config", nil
	}
	defer func() { installRelease = old }()

	h.send(updateMsg{latest: "1.0.0"})
	if h.m.overlay == overlayUpdate {
		t.Fatal("the same version offered itself as an update")
	}
	h.send(updateMsg{latest: "1.2.0"})
	if h.m.overlay != overlayUpdate {
		t.Fatal("a newer release opened no dialog")
	}
	h.shot("16-update")
	if _, ok := h.find(hitButton, -1, "update-install"); !ok {
		t.Fatal("the dialog has no Install button")
	}
	h.key("esc")
	if h.m.overlay != overlayNone || asked != "" {
		t.Fatalf("Next time left overlay %d, installed %q", h.m.overlay, asked)
	}

	h.send(updateMsg{latest: "1.2.0"})
	h.key("enter")
	if asked != "1.2.0" {
		t.Fatalf("Install asked for %q, want 1.2.0", asked)
	}
	if h.m.overlay != overlayMessage || !strings.Contains(h.m.messageText, "Updated to 1.2.0") {
		t.Fatalf("after Install: overlay %d, %q", h.m.overlay, h.m.messageText)
	}
	if !strings.Contains(h.m.ExitNote(), "1.2.0") {
		t.Fatalf("the exit note does not say the new version: %q", h.m.ExitNote())
	}

	// Held back while the welcome is open, shown when it closes.
	w := newPixHarness(t, "midnight")
	w.m.opts.Version = "1.0.0"
	w.m.overlay = overlayWelcome
	w.send(updateMsg{latest: "1.2.0"})
	if w.m.overlay != overlayWelcome {
		t.Fatal("the update dialog covered the welcome")
	}
	w.key("enter")
	if w.m.overlay != overlayUpdate {
		t.Fatal("closing the welcome did not show the update")
	}
}

// The cell size comes from the terminal, never from the window's pixel size:
// a resize asks for it, and only a new answer changes it.
func TestCellSizeComesFromTheTerminal(t *testing.T) {
	var out bytes.Buffer
	h := newHarnessWith(t, pixConfig, Options{NoReload: true, Images: true, ImgOut: &out, Pixel: true, PixCell: [2]int{17, 38}})
	_, cmd := h.m.Update(tea.WindowSizeMsg{Width: 118, Height: 30})
	if cmd == nil {
		t.Fatal("a resize asked nothing of the terminal")
	}
	if h.m.px.cw != 17 || h.m.px.ch != 38 {
		t.Fatalf("a resize changed the cell to %dx%d", h.m.px.cw, h.m.px.ch)
	}
	h.send(uv.CellSizeEvent{Width: 19, Height: 42})
	if h.m.px.cw != 19 || h.m.px.ch != 42 {
		t.Fatalf("the reported cell was not taken: %dx%d", h.m.px.cw, h.m.px.ch)
	}
	out.Reset()
	h.m.View()
	if !strings.Contains(out.String(), "s="+itoa(118*19)+",v=42") {
		t.Fatal("the rows were not drawn again at the new cell size")
	}
}

// Leaving frees every picture by its number, while the screen that holds
// them is up, and two runs never number their pictures alike.
func TestPicturesAreFreedOnTheWayOut(t *testing.T) {
	var out bytes.Buffer
	h := newHarnessWith(t, pixConfig, Options{NoReload: true, Images: true, ImgOut: &out, Pixel: true, PixCell: [2]int{17, 38}})
	h.m.View()
	var ids []uint32
	for _, id := range h.m.imgs.ids {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		t.Fatal("no picture was sent")
	}
	cmd := h.m.dropPictures()
	if cmd == nil {
		t.Fatal("nothing frees the pictures")
	}
	raw, ok := cmd().(tea.RawMsg)
	if !ok {
		t.Fatalf("freeing the pictures is %T", cmd())
	}
	for _, id := range ids {
		if !strings.Contains(fmt.Sprint(raw.Msg), "d=I,i="+itoa(int(id))+",") {
			t.Fatalf("picture %d is not freed by number", id)
		}
	}
	other := newImageStore(nil)
	if other.next == newImageStore(nil).next && other.next == h.m.imgs.next {
		t.Fatal("runs number their pictures alike")
	}
}

// A dialog dims the screen behind it, and what is dimmed answers to no
// click: the footer's Enter, under the dialog, must not confirm it.
func TestTheDimmedFooterIsNotAButton(t *testing.T) {
	h := newPixHarness(t, "midnight")
	h.m.jumpToGroup("Window")
	enter, ok := h.find(hitButton, -1, "enter")
	if !ok {
		t.Fatal("the footer has no Enter button")
	}
	confirmed := false
	h.m.openConfirm("Delete the theme file matcha-whisk from your themes directory? A backup is not kept.", func(*Model) tea.Cmd {
		confirmed = true
		return nil
	})
	if _, ok := h.find(hitButton, -1, "enter"); ok {
		t.Fatal("the footer's Enter is still a button under the dialog")
	}
	h.click(enter.x+enter.w/2, enter.y)
	if confirmed || h.m.overlay != overlayConfirm {
		t.Fatalf("a click on the dimmed footer confirmed the dialog (overlay %d)", h.m.overlay)
	}
	yes, ok := h.find(hitButton, -1, "confirm-yes")
	if !ok {
		t.Fatal("the dialog has no Yes button")
	}
	h.click(yes.x+yes.w/2, yes.y)
	if !confirmed {
		t.Fatal("Yes did not confirm")
	}
}

// The rows go through temporary files only when they are written to the
// terminal itself: written anywhere else nothing would read the files, and
// nothing would delete them.
func TestPicturesGoInlineWhenNotWrittenToATerminal(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "")
	var out bytes.Buffer
	if filesReachTerminal(&out) || filesReachTerminal(io.Discard) {
		t.Fatal("pictures written to a buffer would go through files")
	}
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if filesReachTerminal(f) {
		t.Fatal("pictures written to a plain file would go through files")
	}
	h := newHarnessWith(t, pixConfig, Options{NoReload: true, Images: true, ImgOut: &out, Pixel: true, PixCell: [2]int{17, 38}})
	h.m.View()
	if h.m.px.seq != 0 || strings.Contains(out.String(), ",t=t;") {
		t.Fatal("a frame written to a buffer went through temporary files")
	}
	if !strings.Contains(out.String(), ",o=z,") {
		t.Fatal("the rows were not sent inline")
	}
}

// Files of ours older than a few minutes were never read and never will be:
// a new run sweeps them away, and leaves the rest alone.
func TestStaleTemporaryFilesAreSwept(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, age time.Duration) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(-age)
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
		return p
	}
	stale := write(tempPrefix+"1-1", time.Hour)
	fresh := write(tempPrefix+"1-2", time.Second)
	other := write("tty-graphics-protocol-someone-else", time.Hour)
	sweepTempFiles(dir, staleTempFile)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("a stale file was left")
	}
	for _, p := range []string{fresh, other} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s was swept: %v", filepath.Base(p), err)
		}
	}
}

// On the way out a run removes the files it wrote that the terminal has
// not read.
func TestLeftoverTemporaryFilesAreRemovedOnTheWayOut(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	h := newPixHarness(t, "midnight")
	h.m.px.viaFile = true
	h.m.imgs.reset()
	h.m.render()
	if h.m.px.seq == 0 {
		t.Fatal("no row went through a file")
	}
	left, _ := filepath.Glob(filepath.Join(os.TempDir(), tempPrefix+"*"))
	if len(left) == 0 {
		t.Fatal("no file was written")
	}
	h.m.dropPictures()
	if left, _ := filepath.Glob(filepath.Join(os.TempDir(), tempPrefix+"*")); len(left) != 0 {
		t.Fatalf("%d files were left behind", len(left))
	}
}

// The panel beside the list scrolls, as in the text interface: what it
// shows moves with the scroll, which stops at the last line, and a line
// that does not fit whole is left for the next scroll.
func TestThePanelScrolls(t *testing.T) {
	h := newPixHarness(t, "midnight")
	m := h.m
	p := newPxPal(m.panelChrome())
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("line %02d of the explanation", i))
	}
	const lh = 19
	draw := func(scroll int, hgt float32) *pix.Canvas {
		m.previewScrl = scroll
		c := pix.New(400, 400, 1)
		c.Clear(p.bg)
		m.pxBridge(c, p, lines, 0, 0, 400, hgt)
		return c
	}
	top := draw(0, 10*lh)
	if m.previewScrl != 0 {
		t.Fatalf("the scroll moved to %d by itself", m.previewScrl)
	}
	down := draw(3, 10*lh)
	if bytes.Equal(top.Img.Pix, down.Img.Pix) {
		t.Fatal("scrolling the panel changed nothing drawn")
	}
	draw(1000, 10*lh)
	if m.previewScrl != len(lines)-10 {
		t.Fatalf("the scroll went to %d, want it to stop at %d", m.previewScrl, len(lines)-10)
	}
	// Two lines and a half: the half is not drawn.
	half := draw(0, 2.5*lh)
	bg := half.Img.RGBAAt(0, 0)
	for y := 2 * lh; y < 2*lh+lh/2; y++ {
		for x := 0; x < 400; x++ {
			if half.Img.RGBAAt(x, y) != bg {
				t.Fatalf("the third line was drawn cut in half (pixel %d,%d)", x, y)
			}
		}
	}
}
