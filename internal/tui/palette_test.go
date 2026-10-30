package tui

import (
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

var pixelOpts = Options{NoReload: true, Images: true, ImgOut: io.Discard, Pixel: true, PixCell: [2]int{17, 38}}

// newPaletteHarness opens the editor in Palette on the picture interface's
// config, with Ghostty's actions read from the copy built in.
func newPaletteHarness(t *testing.T, opts Options) *harness {
	t.Helper()
	stubKeys(t)
	was := harnessProfile
	harnessProfile = ghostty.ProfilePalette
	defer func() { harnessProfile = was }()
	h := newHarnessWith(t, pixConfig, opts)
	if _, ok := h.m.profile().(*palette); !ok {
		t.Fatalf("the profile in effect is %T", h.m.profile())
	}
	return h
}

func (h *harness) pal() *palette { return h.m.profile().(*palette) }

// search types a query from an empty search line.
func (h *harness) search(q string) {
	h.t.Helper()
	h.key("ctrl+u")
	h.typeText(q)
}

// palSelect highlights the row of a command, a theme or a value by its id.
func (h *harness) palSelect(id string) {
	h.t.Helper()
	p := h.pal()
	p.refresh(h.m)
	for i, r := range p.rows {
		if r.selectable() && r.id() == id {
			p.sel = i
			h.send(tea.KeyPressMsg{Code: tea.KeyF24}) // settles the highlight, as a key would
			return
		}
	}
	var ids []string
	for _, r := range p.rows {
		ids = append(ids, r.id())
	}
	h.t.Fatalf("no row %q in %v", id, ids)
}

// palIDs are the rows of the list, by kind and id.
func (h *harness) palIDs() map[palKind][]string {
	p := h.pal()
	p.refresh(h.m)
	out := map[palKind][]string{}
	for _, r := range p.rows {
		if r.selectable() {
			out[r.kind] = append(out[r.kind], r.id())
		}
	}
	return out
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// One line finds every kind of thing, each the way its own list finds it.
func TestPaletteSearchesEveryKind(t *testing.T) {
	h := newPaletteHarness(t, Options{NoReload: true})
	for _, c := range []struct {
		query string
		kind  palKind
		id    string
	}{
		{"padding", palSetting, "c:paddingx"},
		{"padding colour", palSetting, "c:paddingcolor"},
		{"dracula", palTheme, "t:Dracula"},
		{"menlo", palFont, "v:Menlo"},
		{"new tab", palShortcut, "k:new_tab"},
		{"reload", palCommand, "c:reload"},
		{"backup", palCommand, "c:backup"},
		{"font size", palSetting, "c:size"},
	} {
		h.search(c.query)
		if got := h.palIDs()[c.kind]; !has(got, c.id) {
			t.Errorf("%q did not find %s among %v", c.query, c.id, got)
		}
	}
	// Words narrow: a dark theme is found by its kind and a word of its note.
	h.search("dark")
	for _, id := range h.palIDs()[palTheme] {
		if th, _ := h.m.lib.Get(strings.TrimPrefix(id, "t:")); th != nil && !th.IsDark() {
			t.Fatalf("dark found the light theme %s", id)
		}
	}
	// The All filter shows a few of each kind and says how many there are.
	h.search("a")
	if n := len(h.palIDs()[palTheme]); n > 6 {
		t.Fatalf("All shows %d themes, want at most 6", n)
	}
	if !strings.Contains(h.pal().countLine(), "themes") {
		t.Fatalf("the count says %q", h.pal().countLine())
	}
	h.search("zzqqxx")
	if len(h.palIDs()) != 0 || !strings.Contains(h.view(), "Nothing matches") {
		t.Fatalf("a query that finds nothing should say so:\n%s", h.view())
	}
}

// Before anything is typed the list offers the theme in effect, what is
// changed, the settings people reach for and the commands that keep files.
func TestPaletteHome(t *testing.T) {
	h := newPaletteHarness(t, Options{NoReload: true})
	ids := h.palIDs()
	if len(ids[palTheme]) == 0 || ids[palTheme][0] != "t:sumi-ink" {
		t.Fatalf("the home list does not start on the applied theme: %v", ids[palTheme])
	}
	if !has(ids[palSetting], "c:paddingx") || !has(ids[palSetting], "c:opacity") {
		t.Fatalf("the changed settings are not offered: %v", ids[palSetting])
	}
	if !has(ids[palCommand], "c:undo") {
		t.Fatalf("the commands are not offered: %v", ids[palCommand])
	}
	view := h.view()
	for _, want := range []string{"CHANGED", "OFTEN USED", "COMMANDS", "ghostty-config · Palette"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the home list lacks %q:\n%s", want, view)
		}
	}
}

// Tab and Shift+Tab walk the filters; a click picks one.
func TestPaletteFilters(t *testing.T) {
	h := newPaletteHarness(t, Options{NoReload: true})
	h.key("tab")
	if h.pal().filter != 1 {
		t.Fatalf("Tab went to %s", palFilters[h.pal().filter])
	}
	ids := h.palIDs()
	if len(ids) != 1 || len(ids[palTheme]) < len(h.m.lib.Themes) {
		t.Fatalf("the Themes filter lists %v kinds and %d themes of %d", len(ids), len(ids[palTheme]), len(h.m.lib.Themes))
	}
	h.key("shift+tab", "shift+tab")
	if palFilters[h.pal().filter] != "Changed" {
		t.Fatalf("Shift+Tab went to %s", palFilters[h.pal().filter])
	}
	for _, id := range h.palIDs()[palSetting] {
		if c := h.m.commandNamed(strings.TrimPrefix(id, "c:")); !h.m.palChanged(c) {
			t.Fatalf("Changed lists %s, which is not changed", id)
		}
	}
	if !has(h.palIDs()[palSetting], "c:paddingcolor") {
		t.Fatalf("Changed lacks the padding colour: %v", h.palIDs())
	}
	h.search("pad")
	if got := h.palIDs()[palSetting]; has(got, "c:paddingy") || !has(got, "c:paddingx") {
		t.Fatalf("Changed with a query lists %v", got)
	}
	r, ok := h.find(hitRail, 3, "")
	if !ok {
		t.Fatal("the Settings filter is not clickable")
	}
	h.click(r.x, r.y)
	if palFilters[h.pal().filter] != "Settings" {
		t.Fatalf("a click went to %s", palFilters[h.pal().filter])
	}
	if ids := h.palIDs(); len(ids) != 1 || len(ids[palSetting]) == 0 {
		t.Fatalf("the Settings filter lists %v", ids)
	}
}

// ← and → write a setting the way Atelier's menu does, Ctrl+Z takes the
// step back and Delete puts the default back.
func TestPaletteChangesASetting(t *testing.T) {
	h := newPaletteHarness(t, Options{NoReload: true})
	h.search("padding colour")
	h.palSelect("c:paddingcolor")
	h.key("right")
	if !strings.Contains(h.config(), "window-padding-color = extend-always") {
		t.Fatalf("→ did not step the padding colour:\n%s", h.config())
	}
	if !strings.Contains(h.m.status, "written") {
		t.Fatalf("the status line says %q", h.m.status)
	}
	h.key("ctrl+z")
	if !strings.Contains(h.config(), "window-padding-color = extend\n") {
		t.Fatalf("Ctrl+Z did not undo the step:\n%s", h.config())
	}
	h.key("delete")
	if strings.Contains(h.config(), "\nwindow-padding-color") {
		t.Fatalf("Delete did not put the default back:\n%s", h.config())
	}
	if h.pal().recent[0] != "paddingcolor" {
		t.Fatalf("the setting is not remembered as changed this session: %v", h.pal().recent)
	}
	// A switch is switched by Enter.
	h.search("balance")
	h.palSelect("c:balance")
	before := h.m.settingNow("balance")
	h.key("enter")
	if h.m.settingNow("balance") == before {
		t.Fatalf("Enter did not switch %s", "balance")
	}
	// The highlighted setting says what the value does and what is written.
	h.search("padding colour")
	h.palSelect("c:paddingcolor")
	h.key("right")
	if v := h.view(); !strings.Contains(v, "+ window-padding-color = extend") {
		t.Fatalf("the opened setting does not say what it writes:\n%s", v)
	}
}

// A setting with many values opens their list on Enter; choosing one writes
// it and goes back to the results.
func TestPaletteOpensALongList(t *testing.T) {
	h := newPaletteHarness(t, Options{NoReload: true})
	h.search("font size")
	h.palSelect("c:size")
	h.key("enter")
	p := h.pal()
	if p.scope == nil || p.scope.name != "size" {
		t.Fatal("Enter on the font size did not open its values")
	}
	if r := p.current(); r == nil || !r.opt.current {
		t.Fatal("the values do not open on the one in effect")
	}
	h.typeText("16")
	h.key("enter")
	if !strings.Contains(h.config(), "font-size = 16") || p.scope != nil {
		t.Fatalf("choosing 16 did not write it and go back (scope %v):\n%s", p.scope, h.config())
	}
	if p.query != "font size" {
		t.Fatalf("back on the results the query is %q", p.query)
	}
	// Esc leaves the values without writing.
	h.palSelect("c:size")
	h.key("enter", "down", "esc")
	if p.scope != nil || !strings.Contains(h.config(), "font-size = 16") {
		t.Fatalf("Esc in the values wrote or stayed:\n%s", h.config())
	}
}

// Moving onto a theme only shows it; Enter applies it; Ctrl+F stars it.
func TestPaletteThemePreviewAndApply(t *testing.T) {
	h := newPaletteHarness(t, Options{NoReload: true})
	before := h.config()
	h.search("dracula")
	if h.m.cur == nil || h.m.cur.Name != "Dracula" {
		t.Fatalf("the theme found is not shown: %v", h.m.cur)
	}
	if h.config() != before {
		t.Fatalf("looking at a theme wrote the config:\n%s", h.config())
	}
	h.key("ctrl+f")
	if !h.m.state.IsFavorite("Dracula") {
		t.Fatal("Ctrl+F did not star the theme")
	}
	h.key("esc")
	if h.m.cur.Name != "sumi-ink" {
		t.Fatalf("clearing the search left %s shown", h.m.cur.Name)
	}
	h.search("dracula")
	h.key("enter")
	if !strings.Contains(h.config(), "theme = Dracula") {
		t.Fatalf("Enter did not apply the theme:\n%s", h.config())
	}
}

// Esc clears what is typed, then the filter, then leaves.
func TestPaletteEscape(t *testing.T) {
	h := newPaletteHarness(t, Options{NoReload: true})
	h.typeText("pad")
	h.key("tab")
	h.key("esc")
	if p := h.pal(); p.query != "" || p.filter != 1 || h.m.quitting {
		t.Fatalf("the first Esc: query %q, filter %d, quitting %v", p.query, p.filter, h.m.quitting)
	}
	h.key("esc")
	if p := h.pal(); p.filter != 0 || h.m.quitting {
		t.Fatalf("the second Esc: filter %d, quitting %v", p.filter, h.m.quitting)
	}
	h.key("esc")
	if !h.m.quitting {
		t.Fatal("Esc on the bare list did not leave")
	}
}

// F3 reads Ghostty's page on the highlighted setting, or the shortcut's
// action.
func TestPaletteExplainTarget(t *testing.T) {
	h := newPaletteHarness(t, Options{NoReload: true})
	h.search("padding colour")
	h.palSelect("c:paddingcolor")
	if name, action := h.pal().explainTarget(h.m); name != "window-padding-color" || action {
		t.Fatalf("F3 on the padding colour reads %q (%v)", name, action)
	}
	h.search("new tab")
	h.palSelect("k:new_tab")
	if name, action := h.pal().explainTarget(h.m); name != "new_tab" || !action {
		t.Fatalf("F3 on a shortcut reads %q (%v)", name, action)
	}
	h.key("enter")
	if h.m.overlay != overlayCapture || h.m.kb.action != "new_tab" {
		t.Fatal("Enter on a shortcut did not ask for one")
	}
}

// A click on a pill of the opened setting writes it; the wheel moves.
func TestPaletteMouse(t *testing.T) {
	for _, opts := range []Options{{NoReload: true}, pixelOpts} {
		h := newPaletteHarness(t, opts)
		h.send(tea.WindowSizeMsg{Width: 120, Height: 34})
		h.send(imgReadyMsg{h.m.imgSeq})
		h.search("padding")
		h.palSelect("c:paddingcolor")
		r, ok := h.find(hitValue, -1, "background")
		if !ok {
			t.Fatalf("pixel %v: the pill of background is not clickable", opts.Pixel)
		}
		h.click(r.x, r.y)
		if strings.Contains(h.config(), "\nwindow-padding-color = extend") {
			t.Fatalf("pixel %v: a click on background did not write it:\n%s", opts.Pixel, h.config())
		}
		at := h.pal().sel
		h.send(tea.MouseWheelMsg{X: 40, Y: 12, Button: tea.MouseWheelDown})
		if h.pal().sel == at {
			t.Fatalf("pixel %v: the wheel did not move", opts.Pixel)
		}
		row, ok := h.find(hitResult, at, "")
		if !ok {
			t.Fatalf("pixel %v: the row is not clickable", opts.Pixel)
		}
		h.click(row.x+2, row.y)
		if h.pal().sel != at {
			t.Fatalf("pixel %v: a click did not highlight the row", opts.Pixel)
		}
	}
}

// The text interface draws every screen within the window, light and dark,
// at the usual size and at 80×24.
func TestPaletteCells(t *testing.T) {
	for _, iface := range []string{"midnight", "paper"} {
		h := newPaletteHarness(t, Options{NoReload: true})
		h.m.state.Interface = iface
		for _, size := range [][2]int{{120, 34}, {80, 24}} {
			h.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			for _, q := range []string{"", "dark", "padding colour", "copy", "reload"} {
				h.search(q)
				if q == "padding colour" {
					h.palSelect("c:paddingcolor")
				}
				frame := h.m.render()
				lines := strings.Split(frame, "\n")
				if len(lines) != size[1] {
					t.Fatalf("%s %v %q: %d rows", iface, size, q, len(lines))
				}
				for i, l := range lines {
					if w := lipgloss.Width(l); w != size[0] {
						t.Fatalf("%s %v %q: row %d is %d wide", iface, size, q, i, w)
					}
				}
				view := h.view()
				if !strings.Contains(view, "Palette") {
					t.Fatalf("%s %v %q: the footer is missing:\n%s", iface, size, q, view)
				}
				h.dump("palette-" + iface + "-" + itoa(size[0]) + "-" + strings.ReplaceAll(q, " ", "_"))
			}
		}
	}
}

// palettePixelScreens draws Palette's screens for TestPixelScreens: the list
// before anything is typed, a search for themes, an opened setting, the
// light scheme, and 80×24.
func palettePixelScreens(t *testing.T) {
	for _, iface := range []string{"midnight", "paper"} {
		h := newPaletteHarness(t, pixelOpts)
		h.m.state.Interface = iface
		for _, size := range [][2]int{{120, 34}, {80, 24}} {
			h.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			suffix := ""
			if size[0] == 80 {
				suffix = "-80x24"
			}
			if iface == "paper" {
				suffix += "-light"
			}
			h.search("")
			h.shot("30-palette-empty" + suffix)
			h.search("warm dark")
			h.shot("31-palette-themes" + suffix)
			h.search("padding")
			h.palSelect("c:paddingcolor")
			h.shot("32-palette-setting" + suffix)
			h.search("copy")
			h.shot("33-palette-mixed" + suffix)
			h.search("font size")
			h.palSelect("c:size")
			h.key("enter")
			h.shot("34-palette-values" + suffix)
			h.key("esc")
		}
	}
}
