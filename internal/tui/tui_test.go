package tui

import (
	stdcolor "image/color"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/vitruves/ghostty-config/internal/collection"
	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/extensions"
	"github.com/vitruves/ghostty-config/internal/fontdl"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// harness builds an editor over a throwaway config tree with a small theme
// library: two bundled themes and the curated collection.
type harness struct {
	t     *testing.T
	m     *Model
	dir   string
	paths ghostty.Paths
}

func newHarness(t *testing.T, config string) *harness {
	t.Helper()
	return newHarnessWith(t, config, Options{NoReload: true})
}

func newHarnessWith(t *testing.T, config string, opts Options) *harness {
	t.Helper()
	dir := t.TempDir()
	builtin := filepath.Join(dir, "resources", "themes")
	os.MkdirAll(builtin, 0o755)
	os.WriteFile(filepath.Join(builtin, "Catppuccin Mocha"), []byte("palette = 0=#45475a\npalette = 1=#f38ba8\npalette = 2=#a6e3a1\npalette = 3=#f9e2af\npalette = 4=#89b4fa\npalette = 5=#f5c2e7\npalette = 6=#94e2d5\npalette = 7=#a6adc8\npalette = 8=#585b70\npalette = 9=#f37799\npalette = 10=#89d88b\npalette = 11=#ebd391\npalette = 12=#74a8fc\npalette = 13=#f2aede\npalette = 14=#6bd7ca\npalette = 15=#bac2de\nbackground = #1e1e2e\nforeground = #cdd6f4\ncursor-color = #f5e0dc\ncursor-text = #1e1e2e\nselection-background = #585b70\nselection-foreground = #cdd6f4\n"), 0o644)
	os.WriteFile(filepath.Join(builtin, "Dracula"), []byte("palette = 0=#21222c\npalette = 1=#ff5555\npalette = 2=#50fa7b\npalette = 3=#f1fa8c\npalette = 4=#bd93f9\npalette = 5=#ff79c6\npalette = 6=#8be9fd\npalette = 7=#f8f8f2\npalette = 8=#6272a4\npalette = 9=#ff6e6e\npalette = 10=#69ff94\npalette = 11=#ffffa5\npalette = 12=#d6acff\npalette = 13=#ff92df\npalette = 14=#a4ffff\npalette = 15=#ffffff\nbackground = #282a36\nforeground = #f8f8f2\n"), 0o644)
	xdg := filepath.Join(dir, "xdg")
	os.MkdirAll(xdg, 0o755)
	if config != "" {
		os.WriteFile(filepath.Join(xdg, "config"), []byte(config), 0o644)
	}
	paths := ghostty.Paths{
		ConfigDir:         xdg,
		Roots:             []string{filepath.Join(xdg, "config"), filepath.Join(xdg, "config.ghostty")},
		ThemesDir:         filepath.Join(xdg, "themes"),
		ResourceThemeDirs: []string{builtin},
		StateDir:          filepath.Join(dir, "state"),
	}
	if _, _, err := collection.Install(paths.ThemesDir); err != nil {
		t.Fatal(err)
	}
	tree, err := ghostty.Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	lib := ghostty.LoadLibrary(paths)
	state := ghostty.LoadState(paths)
	state.Welcomed = true
	// Most tests were written for the clear interface, which follows the
	// theme being shown and paints no ground; the others have their own tests.
	state.Interface = "clear"
	writeDebounce = time.Millisecond
	// Tests have no terminal to detect: draw in full colour, rounded.
	t.Setenv("GHOSTTY_CONFIG_GLYPHS", "round")
	m := New(paths, tree, lib, state, opts)
	m.live = nil
	// Tests start from the menu; opening on the themes has its own test.
	m.setPromptText("")
	m.lastText = ""
	// A blinking cursor returns a timer on every keystroke, which the
	// harness would sit through.
	m.prompt.SetVirtualCursor(false)
	// A fixed font catalog so tests never scan the machine.
	m.families = []ghostty.Family{
		{Name: "Hack", Styles: []string{"Regular", "Bold"}, Mono: true},
		{Name: "Menlo", Styles: []string{"Regular", "Bold", "Italic"}, Mono: true},
		{Name: "Helvetica", Styles: []string{"Regular"}, Mono: false},
	}
	m.fontsLoaded = true
	h := &harness{t: t, m: m, dir: dir, paths: paths}
	h.send(tea.WindowSizeMsg{Width: 130, Height: 38})
	return h
}

func (h *harness) send(msg tea.Msg) {
	_, cmd := h.m.Update(msg)
	for i := 0; cmd != nil && i < 8; i++ {
		out := cmd()
		cmd = nil
		switch out := out.(type) {
		case tea.BatchMsg:
			for _, c := range out {
				if c != nil {
					if inner := c(); inner != nil {
						if _, isQuit := inner.(tea.QuitMsg); isQuit {
							return
						}
						_, cmd = h.m.Update(inner)
					}
				}
			}
		case nil:
		default:
			if _, isQuit := out.(tea.QuitMsg); isQuit {
				return
			}
			_, cmd = h.m.Update(out)
		}
	}
}

// keyPress turns a key name ("up", "ctrl+x", "shift+tab", "a") into the
// message the terminal would send.
func keyPress(k string) tea.KeyPressMsg {
	var mod tea.KeyMod
	for {
		switch {
		case strings.HasPrefix(k, "ctrl+") && len(k) > 5:
			mod |= tea.ModCtrl
			k = k[5:]
			continue
		case strings.HasPrefix(k, "shift+") && len(k) > 6:
			mod |= tea.ModShift
			k = k[6:]
			continue
		case strings.HasPrefix(k, "alt+") && len(k) > 4:
			mod |= tea.ModAlt
			k = k[4:]
			continue
		}
		break
	}
	named := map[string]rune{
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
		"tab": tea.KeyTab, "enter": tea.KeyEnter, "esc": tea.KeyEscape, "backspace": tea.KeyBackspace,
		"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "home": tea.KeyHome, "end": tea.KeyEnd,
		"f1": tea.KeyF1, "f2": tea.KeyF2, "delete": tea.KeyDelete,
	}
	if code, ok := named[k]; ok {
		return tea.KeyPressMsg{Code: code, Mod: mod}
	}
	if k == "space" {
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	r := []rune(k)[0]
	if mod&tea.ModCtrl != 0 {
		return tea.KeyPressMsg{Code: r, Mod: mod}
	}
	return tea.KeyPressMsg{Code: r, Text: k, Mod: mod}
}

func (h *harness) key(keys ...string) {
	for _, k := range keys {
		h.send(keyPress(k))
	}
}

// typeText types a string one rune at a time, spaces included.
func (h *harness) typeText(s string) {
	for _, r := range s {
		if r == ' ' {
			h.key("space")
		} else {
			h.key(string(r))
		}
	}
}

func (h *harness) view() string { return ansi.Strip(h.m.render()) }

func (h *harness) dump(name string) {
	dir := os.Getenv("GHOSTTY_CONFIG_DUMP")
	if dir == "" {
		return
	}
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, name+".txt"), []byte(h.view()), 0o644)
	os.WriteFile(filepath.Join(dir, name+".ansi"), []byte(h.m.render()), 0o644)
	// What the terminal's own colours are, for the cells that paint none.
	ui := h.m.panelChrome()
	os.WriteFile(filepath.Join(dir, name+".term"), []byte(ui.bg+" "+ui.fg), 0o644)
}

func (h *harness) config() string {
	data, _ := os.ReadFile(filepath.Join(h.dir, "xdg", "config"))
	return string(data)
}

// click presses at a cell of the editor, which takes the whole screen.
func (h *harness) click(x, y int) {
	h.send(tea.MouseClickMsg{X: x, Y: y + h.m.top, Button: tea.MouseLeft})
}

func (h *harness) find(kind hitKind, index int, name string) (region, bool) {
	h.m.render()
	for _, r := range h.m.regions {
		if r.kind == kind && (index < 0 || r.index == index) && (name == "" || r.name == name) {
			return r, true
		}
	}
	return region{}, false
}

func TestPaletteReachesEveryCommandAtTheUsersWindowSize(t *testing.T) {
	h := newHarness(t, "font-size = 14\ntheme = Dracula\n")
	// The window the user actually runs: 105 columns by 28 rows.
	h.send(tea.WindowSizeMsg{Width: 105, Height: 28})
	v := h.view()
	for _, want := range []string{" Themes ", " Fonts ", " Window ", " Tool ", "Theme", "Favs", "Dracula", "10 commands", "applied Dracula"} {
		if !strings.Contains(v, want) {
			t.Fatalf("palette lacks %q:\n%s", want, v)
		}
	}
	// The highlighted command is described in full beside the list, wrapped
	// rather than cut.
	if !strings.Contains(v, "Theme <name>") || !strings.Contains(v, "Browse themes; moving only looks") || !strings.Contains(v, "highlighted one.") {
		t.Fatalf("the highlighted command should be described in full:\n%s", v)
	}
	// Nothing is framed: no box is drawn round the list, the explanation or
	// the screen.
	if strings.ContainsAny(v, "╭╮╰╯│┌┐└┘") {
		t.Fatalf("the screen draws no frames:\n%s", v)
	}
	lines := strings.Split(v, "\n")
	if len(lines) != 28 {
		t.Fatalf("view should fill the terminal: %d lines", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 105 {
			t.Fatalf("line %d is %d cells wide, want 105: %q", i, w, l)
		}
	}
	h.dump("01-themes-group")

	// Every group fits whole, one at a time, with what each setting is now.
	seen := map[string]bool{}
	for i := 0; i < len(groups); i++ {
		for _, o := range h.m.results {
			seen[o.value] = true
			if !strings.Contains(h.view(), " "+o.label) {
				t.Fatalf("group %s: %q is not on screen:\n%s", groups[h.m.group], o.label, h.view())
			}
		}
		h.dump("02-group-" + strings.ToLower(groups[h.m.group]))
		h.key("tab")
	}
	for _, c := range h.m.commands() {
		if !seen[c.name] {
			t.Errorf("command %s is reachable from no group", c.name)
		}
	}
	if h.m.group != 0 {
		t.Fatalf("four Tabs should come back to Themes, on %d", h.m.group)
	}

	h.key("right")
	if o := h.m.selected(); o == nil || o.value != "font" || groups[h.m.group] != "Fonts" {
		t.Fatalf("right should show the Fonts group, on %v", o)
	}
	h.key("down", "down")
	if o := h.m.selected(); o == nil || o.value != "size" || o.detail != "14 pt" {
		t.Fatalf("down twice should reach size with its value, on %+v", o)
	}
	h.key("enter")
	if h.m.cmd == nil || h.m.cmd.name != "size" || h.m.prompt.Value() != "Size " {
		t.Fatalf("Enter should open the command, prompt %q", h.m.prompt.Value())
	}
	h.key("esc")

	// Typing searches every group.
	h.typeText("pad")
	if h.m.cmd != nil {
		t.Fatal("pad is ambiguous and should keep the menu")
	}
	v = h.view()
	if !strings.Contains(v, "Paddingx") || !strings.Contains(v, "Paddingcolor") || !strings.Contains(v, "all sections") || strings.Contains(v, "Autoreload") {
		t.Fatalf("typing should search the commands:\n%s", v)
	}
	h.dump("03-search")
	h.key("esc")

	// F2 steps aside to show the terminal alone: one line is left, and the
	// alternate screen is given up while it lasts.
	h.send(keyPress("f2"))
	if strings.Contains(h.view(), "Themes") || !strings.Contains(h.view(), "brings ghostty-config back") || h.m.View().AltScreen {
		t.Fatalf("F2 should leave only the terminal:\n%s", h.view())
	}
	h.key("x")
	if !strings.Contains(h.view(), "Themes") || !h.m.View().AltScreen {
		t.Fatal("any key should bring the editor back")
	}
}

func TestOpensOnTheThemesWithTheirColours(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	fresh := New(h.paths, h.m.tree, h.m.lib, h.m.state, Options{NoReload: true})
	fresh.live = nil
	h.m = fresh
	h.send(tea.WindowSizeMsg{Width: 105, Height: 28})
	if fresh.cmd == nil || fresh.cmd.name != "theme" || fresh.prompt.Value() != "Theme " {
		t.Fatalf("the editor should open on the theme list, prompt %q", fresh.prompt.Value())
	}
	if o := fresh.selected(); o == nil || o.value != "Dracula" {
		t.Fatalf("the applied theme should be highlighted, on %+v", o)
	}
	v := h.view()
	// The wall: its groups, the cards with their specimen and their sixteen
	// colours, and the applied theme marked.
	for _, want := range []string{"Bundled", "Dracula", "Aa", "▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄", "applied", "This is the configured theme."} {
		if !strings.Contains(v, want) {
			t.Fatalf("opening view lacks %q:\n%s", want, v)
		}
	}
	for i, l := range strings.Split(v, "\n") {
		if w := ansi.StringWidth(l); w != 105 {
			t.Fatalf("line %d is %d cells wide, want 105: %q", i, w, l)
		}
	}
	h.dump("00-opening")
}

func TestThemeCommandPreviewsThenApplies(t *testing.T) {
	h := newHarness(t, "# mine\nbackground = #ff0000\ntheme = Dracula\nfont-size = 12\n")
	before := h.config()
	h.typeText("theme catp")
	if h.m.cmd == nil || h.m.cmd.name != "theme" {
		t.Fatalf("prompt should resolve to theme, got %v", h.m.cmd)
	}
	if h.m.cur.Name != "Catppuccin Mocha" {
		t.Fatalf("typing should preview the best match, showing %s", h.m.cur.Name)
	}
	if h.config() != before {
		t.Fatal("previewing must not write the config")
	}
	if !strings.Contains(h.view(), "showing Catppuccin Mocha") {
		t.Fatalf("header should say what is shown:\n%s", h.view())
	}
	h.send(tea.WindowSizeMsg{Width: 105, Height: 28})
	h.dump("04-theme-results")
	h.key("enter")
	cfg := h.config()
	if !strings.Contains(cfg, "theme = Catppuccin Mocha\n") {
		t.Fatalf("theme not applied:\n%s", cfg)
	}
	if !strings.Contains(cfg, "#background = #ff0000  # disabled by ghostty-config") {
		t.Fatalf("override not disabled:\n%s", cfg)
	}
	if !strings.Contains(cfg, "# mine\n") || !strings.Contains(cfg, "font-size = 12\n") {
		t.Fatalf("other lines must survive:\n%s", cfg)
	}
	if h.m.prompt.Value() != "Theme " {
		t.Fatalf("after Enter the prompt keeps the command, got %q", h.m.prompt.Value())
	}
	if o := h.m.selected(); o == nil || o.value != "Catppuccin Mocha" {
		t.Fatalf("the list should stay on the theme just applied, on %+v", o)
	}
}

func TestSlotEditorAndSave(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.typeText("edit backg")
	h.key("enter")
	if !h.m.editing || h.m.editKey != "background" {
		t.Fatalf("Enter on a slot should open the editor, editing=%v key=%q", h.m.editing, h.m.editKey)
	}
	h.send(tea.WindowSizeMsg{Width: 105, Height: 28})
	h.dump("05-edit")
	original := h.m.cur.Colors["background"]
	h.key("right", "right")
	if h.m.cur.Colors["background"] == original || !h.m.dirty {
		t.Fatal("arrows should brighten the slot and mark the theme dirty")
	}
	h.typeText("123456")
	if h.m.cur.Colors["background"] != "#123456" {
		t.Fatalf("typing six hex digits should set the slot, got %s", h.m.cur.Colors["background"])
	}
	h.key("down")
	if h.m.editKey != "foreground" {
		t.Fatalf("down should move to the next slot, got %s", h.m.editKey)
	}
	h.key("esc")
	if h.m.editing {
		t.Fatal("Esc should leave the editor")
	}
	h.key("esc")
	h.typeText("save")
	h.key("enter")
	if h.m.prompt.Value() != "Save Dracula-custom" {
		t.Fatalf("saving a bundled theme should propose a name, got %q", h.m.prompt.Value())
	}
	h.key("enter")
	if _, err := os.Stat(filepath.Join(h.paths.ThemesDir, "Dracula-custom")); err != nil {
		t.Fatalf("fork not written: %v", err)
	}
	if !strings.Contains(h.config(), "theme = Dracula-custom\n") || h.m.dirty {
		t.Fatalf("fork should be applied and clean:\n%s", h.config())
	}
	// Direct form: edit <slot> #hex.
	h.typeText("edit red #ff0000")
	h.key("enter")
	if h.m.cur.Colors["palette.1"] != "#ff0000" || h.m.editing {
		t.Fatalf("edit red #ff0000 should set the slot outright, got %s editing=%v", h.m.cur.Colors["palette.1"], h.m.editing)
	}
}

func TestNewRandomAndUndo(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.typeText("new light tri")
	h.key("enter")
	if h.m.cur == nil || h.m.cur.Source != ghostty.SourceDraft || h.m.cur.IsDark() {
		t.Fatalf("new light triadic should load a light draft, got %+v", h.m.cur)
	}
	h.dump("05-new")
	h.key("esc")
	h.typeText("undo")
	h.key("enter")
	if h.m.cur.Name != "Dracula" {
		t.Fatalf("undo should drop the draft, showing %s", h.m.cur.Name)
	}
	h.typeText("random")
	h.key("enter")
	if h.m.cur.Source != ghostty.SourceDraft {
		t.Fatal("random should load a draft")
	}
}

func TestFontCommands(t *testing.T) {
	h := newHarness(t, "theme = Dracula\nfont-family = Menlo\nfont-size = 14\n")
	h.typeText("font ")
	if !strings.Contains(h.view(), "Hack") || !strings.Contains(h.view(), "monospace") {
		t.Fatalf("font should list families:\n%s", h.view())
	}
	h.send(tea.WindowSizeMsg{Width: 105, Height: 28})
	h.dump("06-font")
	h.typeText("hack")
	h.key("enter")
	if !strings.Contains(h.config(), "font-family = Hack\n") {
		t.Fatalf("font hack should write the family:\n%s", h.config())
	}
	h.key("esc")
	h.typeText("style bold")
	h.key("enter")
	h.key("esc")
	h.typeText("size 15")
	h.key("enter")
	cfg := h.config()
	for _, want := range []string{"font-family = Hack\n", "font-style = Bold\n", "font-size = 15\n"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config lacks %q:\n%s", want, cfg)
		}
	}
}

func TestWindowCommands(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.typeText("paddingx 20")
	h.key("enter")
	h.key("esc")
	h.typeText("opacity 0.9")
	h.key("enter")
	h.key("esc")
	h.typeText("ligatures off")
	h.key("enter")
	cfg := h.config()
	for _, want := range []string{"window-padding-x = 20\n", "background-opacity = 0.90\n", "font-feature = -calt, -liga, -dlig\n"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config lacks %q:\n%s", want, cfg)
		}
	}
	if runtime.GOOS == "darwin" {
		h.key("esc")
		h.typeText("titlebar ")
		h.send(tea.WindowSizeMsg{Width: 105, Height: 28})
		h.dump("07-titlebar")
		h.typeText("tabs")
		h.key("enter")
		if !strings.Contains(h.config(), "macos-titlebar-style = tabs\n") {
			t.Fatalf("titlebar tabs not written:\n%s", h.config())
		}
	}
}

func TestMouseSelectsRunsAndJumpsToCommands(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.typeText("theme ")
	r, ok := h.find(hitResult, -1, "")
	if !ok {
		t.Fatal("no result rows registered")
	}
	// Pick a row other than the one already highlighted.
	target := r
	for _, rr := range h.m.regions {
		if rr.kind == hitResult && rr.index != h.m.sel.cursor {
			target = rr
			break
		}
	}
	h.click(target.x+2, target.y)
	if h.m.sel.cursor != target.index {
		t.Fatalf("click should highlight row %d, got %d", target.index, h.m.sel.cursor)
	}
	name := h.m.cur.Name
	if strings.Contains(h.config(), "theme = "+name+"\n") {
		t.Fatal("a single click must not apply")
	}
	h.click(target.x+2, target.y)
	if !strings.Contains(h.config(), "theme = "+name+"\n") {
		t.Fatalf("double-click should apply %s:\n%s", name, h.config())
	}
	// A group chip leads back to the menu on that group.
	group, ok := h.find(hitGroup, -1, "Window")
	if !ok {
		t.Fatal("no Window chip")
	}
	h.click(group.x+1, group.y)
	if h.m.prompt.Value() != "" || groups[h.m.group] != "Window" || h.m.selected() == nil || h.m.selected().cmd == nil || h.m.selected().cmd.group != "Window" {
		t.Fatalf("clicking Window should show that group, prompt %q group %d", h.m.prompt.Value(), h.m.group)
	}
	before := h.m.sel.cursor
	h.send(tea.MouseWheelMsg{X: group.x, Y: group.y + 4, Button: tea.MouseWheelDown})
	if h.m.sel.cursor != before+1 {
		t.Fatalf("wheel should move the highlight: %d → %d", before, h.m.sel.cursor)
	}
	// And a double click on a row opens its command.
	cell, ok := h.find(hitResult, 0, "")
	if !ok {
		t.Fatal("no row registered")
	}
	h.click(cell.x+2, cell.y)
	h.click(cell.x+2, cell.y)
	if h.m.cmd == nil || h.m.cmd.group != "Window" {
		t.Fatalf("double-clicking a command should open it, got %v", h.m.cmd)
	}
}

func TestLookingNeverChangesTheConfig(t *testing.T) {
	const config = "theme = Dracula\n"
	h := newHarness(t, config)

	// Opening the theme list, from the menu or by typing, lands on the
	// theme in effect: it must not preview the first theme of the list.
	h.key("enter") // the menu's first row is Theme
	if h.m.cmd == nil || h.m.cmd.name != "theme" {
		t.Fatalf("Enter on the first row should open Theme, got %v", h.m.cmd)
	}
	if h.m.cur.Name != "Dracula" || h.m.selected() == nil || h.m.selected().value != "Dracula" {
		t.Fatalf("opening the list must stay on the applied theme, showing %s on %+v", h.m.cur.Name, h.m.selected())
	}
	h.key("esc")
	h.typeText("theme ")
	if h.m.cur.Name != "Dracula" || h.m.selected().value != "Dracula" {
		t.Fatalf("typing the command must stay on the applied theme, showing %s", h.m.cur.Name)
	}

	// Browsing then leaving writes nothing.
	h.key("up", "left", "up")
	if h.m.cur.Name == "Dracula" {
		t.Fatal("moving should preview another theme")
	}
	h.key("esc")
	if h.m.prompt.Value() != "" || h.m.quitting {
		t.Fatal("first Esc goes back to the menu only")
	}
	h.key("esc")
	if !h.m.quitting {
		t.Fatal("second Esc leaves")
	}
	if h.config() != config {
		t.Fatalf("leaving after looking must not touch the config:\n%s", h.config())
	}
	if !strings.Contains(h.m.ExitNote(), "Dracula is still applied") {
		t.Fatalf("the exit note should say nothing changed, got %q", h.m.ExitNote())
	}
}

func TestLeavingWithEditsAsks(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.typeText("edit backg")
	h.key("enter", "right", "esc", "esc", "esc")
	if h.m.overlay != overlayConfirm || h.m.quitting {
		t.Fatalf("leaving with unsaved edits should ask, overlay %v", h.m.overlay)
	}
	h.dump("12-confirm")
	h.key("esc")
	if h.m.overlay != overlayNone || h.m.quitting {
		t.Fatal("Esc should stay in the editor")
	}
	h.key("esc")
	h.key("n")
	if !h.m.quitting || h.config() != "theme = Dracula\n" {
		t.Fatalf("n should leave without saving:\n%s", h.config())
	}

	h = newHarness(t, "theme = Dracula\n")
	h.typeText("edit backg")
	h.key("enter", "right", "esc", "esc", "esc", "y")
	if !h.m.quitting || !strings.Contains(h.config(), "theme = Dracula-custom\n") {
		t.Fatalf("y should save and apply before leaving:\n%s", h.config())
	}
}

func TestInterfaceHasItsOwnColoursAndRampsLineUp(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.send(tea.WindowSizeMsg{Width: 105, Height: 28})
	h.typeText("theme ")
	// The background of the paper interface, as the terminal is told it.
	paper := regexp.MustCompile(`48;2;\d+;\d+;\d+`).FindString(on("#000000", "#f6f3ec").Render("x"))
	if paper == "" {
		t.Fatal("could not work out how the interface background is encoded")
	}
	// The clear interface paints no background behind its text: the screen
	// sits on the terminal's own, and nothing is framed.
	painted := regexp.MustCompile(`[\[;]48;`)
	rows := strings.Split(h.m.render(), "\n")
	if painted.MatchString(rows[0]) || painted.MatchString(rows[len(rows)-1]) || h.m.panelChrome().paint() != "" || strings.ContainsAny(h.view(), "╭╮╰╯│") {
		t.Fatalf("the clear interface paints no background and draws no frame:\n%s", h.view())
	}
	if len(rows) != 28 || h.m.top != 0 {
		t.Fatal("the editor takes the whole terminal")
	}
	h.send(keyPress("f2"))
	h.key("x")
	h.key("esc")
	h.typeText("interface paper")
	h.key("enter")
	h.key("esc")
	h.typeText("theme ")
	if !strings.Contains(h.m.render(), paper) {
		t.Fatal("interface paper draws a light palette")
	}
	if !color.IsDark(h.m.cur.Background()) || color.IsDark(h.m.panelChrome().bg) {
		t.Fatal("a light palette over a dark theme is the point of paper")
	}
	ui := h.m.panelChrome()
	if color.Contrast(ui.muted, ui.bg) < 4.5 || color.Contrast(ui.accent, ui.bg) < 4.5 {
		t.Fatalf("quiet text %s and the accent %s must stay readable on %s", ui.muted, ui.accent, ui.bg)
	}
	// The wall is a grid: every card of a row starts where the one above it
	// does, and a row holds as many cards as the window is wide.
	h.key("home")
	h.m.render()
	columns := map[int]bool{}
	cards := 0
	for _, r := range h.m.regions {
		if r.kind == hitResult && r.w == tileW {
			columns[r.x] = true
			cards++
		}
	}
	if want := h.m.layout().cols; cards < 8 || len(columns) != want {
		t.Fatalf("expected a wall of cards in %d columns, saw %d cards in %d:\n%s", want, cards, len(columns), h.view())
	}
	h.dump("13-theme-list")
	h.key("esc")
	h.typeText("interface theme")
	h.key("enter")
	if strings.Contains(h.m.render(), paper) || h.m.state.Interface != "theme" {
		t.Fatal("interface theme should follow the theme being shown")
	}
	h.key("esc")
	h.typeText("interface graphite")
	h.key("enter")
	h.key("esc")
	h.dump("14-graphite")
}

func TestHelpWelcomeAndNarrow(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.send(keyPress("f1"))
	if !strings.Contains(h.view(), "Keys and commands") || !strings.Contains(h.view(), "Theme Favs") {
		t.Fatalf("help should list keys and commands:\n%s", h.view())
	}
	h.dump("08-help")
	h.key("x")
	h.m.overlay = overlayWelcome
	if !strings.Contains(h.view(), "g h o s t t y") {
		t.Fatal("welcome missing")
	}
	h.dump("09-welcome")
	h.key("enter")
	for _, w := range []int{50, 80, 100} {
		h.send(tea.WindowSizeMsg{Width: w, Height: 24})
		for i, l := range strings.Split(h.view(), "\n") {
			if ansi.StringWidth(l) != w {
				t.Fatalf("width %d: line %d is %d cells: %q", w, i, ansi.StringWidth(l), l)
			}
		}
	}
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	h.dump("10-narrow")
}

func TestRunsInAnyTerminal(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	// A terminal that is not Ghostty and cannot draw the rounded ends.
	h.m.caps = termCaps{}
	h.m.live = nil
	h.send(tea.WindowSizeMsg{Width: 105, Height: 28})
	h.typeText("theme ")
	raw := h.m.render()
	if strings.ContainsAny(raw, "\ue0b6\ue0b4") {
		t.Fatal("glyphs the terminal may not have must not be drawn")
	}
	for i, l := range strings.Split(ansi.Strip(raw), "\n") {
		if w := ansi.StringWidth(l); w != 105 {
			t.Fatalf("line %d is %d cells wide, want 105: %q", i, w, l)
		}
	}
	// Every cell is painted: after a reset the colours are set again before
	// anything is drawn, so nothing ever shows the terminal's own background.
	const reset = "\x1b[m"
	if !strings.Contains(raw, reset) || strings.Contains(raw, "\x1b[0m") {
		t.Fatal("the test looks for the reset the styles write, and it is not the one in the frame")
	}
	for i, l := range strings.Split(raw, "\n") {
		rest := l
		for {
			j := strings.Index(rest, reset)
			if j < 0 {
				break
			}
			rest = rest[j+len(reset):]
			if rest != "" && !strings.HasPrefix(rest, "\x1b[") {
				t.Fatalf("line %d draws %q in the terminal's own colours", i, rest[:min(len(rest), 12)])
			}
		}
	}
	// The reload keystroke goes to the frontmost window, so it is never
	// sent from a terminal that is not Ghostty.
	h.m.opts.NoReload = false
	if cmd := h.m.reload(); cmd != nil {
		t.Fatal("reload must not be attempted outside Ghostty")
	}
	h.dump("11-plain-terminal")
}

// The tool opens on "Theme "; the first Tab must leave it for the next group
// rather than complete a theme name nobody asked for.
func TestFirstTabChangesGroup(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.key("tab")
	if groups[h.m.group] != "Fonts" || h.m.cmd != nil || h.m.prompt.Value() != "" {
		t.Fatalf("first Tab should show Fonts on the menu, on %s cmd=%v prompt=%q", groups[h.m.group], h.m.cmd, h.m.prompt.Value())
	}
	h.key("shift+tab", "shift+tab")
	if groups[h.m.group] != "Tool" {
		t.Fatalf("Shift+Tab twice should wrap to Tool, on %s", groups[h.m.group])
	}
	if got := h.config(); got != "theme = Dracula\n" {
		t.Fatalf("Tab must not write anything:\n%s", got)
	}
}

func TestPresetWritesItsSettings(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.key("esc")
	h.typeText("preset focus")
	h.key("enter")
	cfg := h.config()
	for _, want := range []string{"window-padding-x = 28", "window-padding-y = 22", "cursor-style = bar", "scrollbar = never", "adjust-cell-height = 10%"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("preset focus should write %q:\n%s", want, cfg)
		}
	}
	h.key("esc")
	h.typeText("preset default")
	h.key("enter")
	if cfg := h.config(); strings.Contains(cfg, "\nwindow-padding-x = 28") || strings.Contains(cfg, "\nscrollbar = never") {
		t.Fatalf("preset default should undo it:\n%s", cfg)
	}
}

func TestDeleteOneThemeThenTheCollection(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	path := filepath.Join(h.paths.ThemesDir, "swiss-grid")
	if _, err := os.Stat(path); err != nil {
		t.Fatal("collection should be installed")
	}
	h.typeText("delete swiss-grid")
	h.key("enter")
	if h.m.overlay != overlayConfirm || !strings.Contains(h.m.confirmText, "swiss-grid") {
		t.Fatalf("delete should ask first, overlay %v %q", h.m.overlay, h.m.confirmText)
	}
	h.key("n")
	if _, err := os.Stat(path); err != nil {
		t.Fatal("declining must keep the file")
	}
	h.key("esc")
	h.typeText("delete swiss-grid")
	h.key("enter", "y")
	if _, err := os.Stat(path); err == nil {
		t.Fatal("confirming should delete the file")
	}
	h.key("esc")
	h.typeText("delete collection")
	h.key("enter", "y")
	entries, _ := os.ReadDir(h.paths.ThemesDir)
	if len(entries) != 0 {
		t.Fatalf("collection should be gone, %d files left", len(entries))
	}
	if _, err := os.Stat(filepath.Join(h.dir, "resources", "themes", "Dracula")); err != nil {
		t.Fatal("bundled themes must never be touched")
	}
}

func TestCtrlXDeletesTheHighlightedTheme(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	path := filepath.Join(h.paths.ThemesDir, "swiss-grid")
	h.typeText("theme swiss-grid")
	if v := h.view(); !strings.Contains(v, "^X") || !strings.Contains(v, "Ctrl+X") {
		t.Fatalf("the delete key should be shown for a deletable theme:\n%s", v)
	}
	h.key("ctrl+x")
	if h.m.overlay != overlayConfirm {
		t.Fatalf("Ctrl+X should ask, overlay %v", h.m.overlay)
	}
	h.key("y")
	if _, err := os.Stat(path); err == nil {
		t.Fatal("file should be gone")
	}
	h.typeText("")
	h.key("esc")
	h.typeText("theme Dracula")
	h.key("ctrl+x")
	if h.m.overlay == overlayConfirm {
		t.Fatal("a bundled theme must not offer deletion")
	}
}

// The midnight interface has colours of its own: browsing another theme,
// light or dark, must not change them.
func TestMidnightIgnoresTheTheme(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.m.state.Interface = "midnight"
	before := h.m.panelChrome()
	light, ok := h.m.lib.Get("swiss-grid")
	if !ok {
		t.Fatal("swiss-grid should be installed")
	}
	h.m.showTheme(light)
	after := h.m.panelChrome()
	if before != after {
		t.Fatalf("the palette followed the theme: %+v -> %+v", before, after)
	}
	h.m.recompute()
	for i, l := range strings.Split(h.view(), "\n") {
		if w := ansi.StringWidth(l); w != h.m.width {
			t.Fatalf("line %d is %d cells wide, want %d", i, w, h.m.width)
		}
	}
}

// The default interface has no colours of its own. It takes the terminal's,
// so the same screen reads in a light terminal and in a dark one, paints no
// ground of its own, and changes when the terminal does.
func TestAutoFollowsTheTerminalLightOrDark(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.m.state.Interface = ""
	if h.m.interfaceName() != "auto" {
		t.Fatalf("the default interface should be auto, is %s", h.m.interfaceName())
	}
	// Until the terminal answers, the configured theme stands in for it.
	if ui := h.m.panelChrome(); ui.bg != "#282a36" || !ui.clear {
		t.Fatalf("before the terminal answers, the ground is the applied theme's, unpainted: %+v", ui)
	}
	check := func(what string) {
		t.Helper()
		ui := h.m.panelChrome()
		for name, hex := range map[string]string{"text": ui.fg, "quiet text": ui.muted, "accent": ui.accent, "ok": ui.ok, "warning": ui.warn, "danger": ui.danger} {
			if r := color.Contrast(hex, ui.bg); r < 4.5 {
				t.Errorf("%s: %s %s reads at %.1f:1 on %s", what, name, hex, r, ui.bg)
			}
		}
		if r := color.Contrast(ui.selFg, ui.selBg); r < 4.5 {
			t.Errorf("%s: the highlighted row reads at %.1f:1", what, r)
		}
		if regexp.MustCompile(`[\[;]48;`).MatchString(strings.Split(h.m.render(), "\n")[2]) {
			t.Errorf("%s: an empty row paints a background", what)
		}
	}
	h.send(tea.BackgroundColorMsg{Color: stdcolor.RGBA{R: 0xf4, G: 0xf5, B: 0xf7, A: 0xff}})
	h.send(tea.ForegroundColorMsg{Color: stdcolor.RGBA{R: 0x1d, G: 0x21, B: 0x25, A: 0xff}})
	if ui := h.m.panelChrome(); ui.bg != "#f4f5f7" || ui.fg != "#1d2125" || color.IsDark(ui.bg) {
		t.Fatalf("a light terminal should give a light interface: %+v", ui)
	}
	check("light terminal")
	h.typeText("theme ")
	check("light terminal, wall of themes")
	h.send(tea.BackgroundColorMsg{Color: stdcolor.RGBA{R: 0x0e, G: 0x0f, B: 0x11, A: 0xff}})
	h.send(tea.ForegroundColorMsg{Color: stdcolor.RGBA{R: 0xd8, G: 0xda, B: 0xdf, A: 0xff}})
	if !color.IsDark(h.m.panelChrome().bg) {
		t.Fatal("a dark terminal should give a dark interface")
	}
	check("dark terminal")
	// A terminal that says its scheme changed is asked for its colours again.
	if _, cmd := h.m.Update(uv.LightColorSchemeEvent{}); cmd == nil {
		t.Fatal("a change of scheme should ask the terminal for its colours")
	}
}

// The wall is walked in two dimensions, group after group, and a click on a
// group in the left column goes to it.
func TestWallIsWalkedInTwoDimensions(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.typeText("theme ")
	cols := h.m.layout().cols
	if cols < 3 {
		t.Fatalf("a window 130 wide should hold several cards per row, holds %d", cols)
	}
	h.key("home")
	first := h.m.sel.cursor
	if h.m.results[first].header || first != 1 {
		t.Fatalf("home should land on the first card, under its title: %d", first)
	}
	h.key("right", "right")
	if h.m.sel.cursor != first+2 {
		t.Fatalf("right twice should move two cards on, at %d", h.m.sel.cursor)
	}
	h.key("down")
	if h.m.sel.cursor != first+2+cols {
		t.Fatalf("down should land on the card below, at %d want %d", h.m.sel.cursor, first+2+cols)
	}
	h.key("up", "left", "left")
	if h.m.sel.cursor != first {
		t.Fatalf("up and left twice should come back, at %d", h.m.sel.cursor)
	}
	// Shift+Down goes to the next group, and the group is marked beside the wall.
	before := themeFamily(h.m.selected().theme)
	h.key("shift+down")
	after := themeFamily(h.m.selected().theme)
	if after == before || !h.m.results[h.m.sel.cursor-1].header {
		t.Fatalf("shift+down should land on the first card of the next group: %s then %s", before, after)
	}
	rail, ok := h.find(hitRail, -1, "Wabi-sabi")
	if !ok {
		t.Fatalf("the groups should be listed beside the wall:\n%s", h.view())
	}
	h.click(rail.x+1, rail.y)
	if o := h.m.selected(); o == nil || themeFamily(o.theme) != "Wabi-sabi" {
		t.Fatalf("a click on a group should go to it, on %+v", o)
	}
	if v := h.view(); !strings.Contains(v, "Wabi-sabi  10 themes, dark to light") {
		t.Fatalf("the title of the group should be on screen:\n%s", v)
	}
	// A family name typed after the command lists that family alone.
	h.key("esc")
	h.typeText("theme gestalt")
	n := 0
	for _, o := range h.m.results {
		if o.theme != nil {
			n++
			if themeFamily(o.theme) != "Gestalt" {
				t.Fatalf("theme gestalt should list that family only, has %s", o.theme.Name)
			}
		}
	}
	if n != 10 {
		t.Fatalf("Gestalt has ten themes, listed %d", n)
	}
	// Looking, all the while, wrote nothing.
	if got := h.config(); got != "theme = Dracula\n" {
		t.Fatalf("walking the wall must not write anything:\n%s", got)
	}
}

// Installing a font is done where fonts are chosen: the families that can be
// downloaded are listed under the installed ones, and Enter on one fetches it.
func TestFontsAreInstalledWhereTheyAreChosen(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	var fetched []string
	real := installFont
	installFont = func(d fontdl.FontDownload, _ func(string)) error {
		fetched = append(fetched, d.Name)
		return nil
	}
	defer func() { installFont = real }()

	for _, c := range h.m.commands() {
		if c.name == "download" {
			t.Fatal("installing a font is part of the font list, not a command of its own")
		}
	}
	h.typeText("font ")
	v := h.view()
	for _, want := range []string{here(), "monospace first", "Not installed yet", "Enter installs one", "JetBrains Mono", "download"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the font list lacks %q:\n%s", want, v)
		}
	}
	for _, o := range h.m.results {
		if o.label == "Hack" && strings.HasPrefix(o.value, downloadPrefix) {
			t.Fatal("a family that is installed must not be offered for download")
		}
	}
	h.typeText("jetbrains")
	o := h.m.selected()
	if o == nil || o.label != "JetBrains Mono" || !strings.HasPrefix(o.value, downloadPrefix) {
		t.Fatalf("typing a family that is not installed should land on its download, on %+v", o)
	}
	if !strings.Contains(h.view(), "downloads and installs it") || !strings.Contains(h.view(), "nerd-fonts") {
		t.Fatalf("the panel should say what Enter does and where the font comes from:\n%s", h.view())
	}
	if h.config() != "theme = Dracula\n" {
		t.Fatalf("resting on a download writes nothing:\n%s", h.config())
	}
	// Enter fetches it behind the spinner. The command is run by hand so the
	// test stops before the machine's fonts would be listed again.
	_, cmd := h.m.Update(keyPress("enter"))
	if h.m.overlay != overlayBusy || cmd == nil {
		t.Fatalf("Enter on a download should start it, overlay %v", h.m.overlay)
	}
	done, ok := cmd().(installDoneMsg)
	if !ok || len(fetched) != 1 || fetched[0] != "JetBrains Mono" || len(done.installed) != 1 {
		t.Fatalf("exactly that family should have been fetched: %v %+v", fetched, done)
	}
}

// A setting is changed where it is listed: ← and → step it through its values
// and each step is written. On a line that holds no scale they walk the
// sections, as Tab does everywhere.
func TestArrowsStepASettingWhereItIsListed(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.key("tab", "tab")
	if groups[h.m.group] != "Window" {
		t.Fatalf("two Tabs should reach Window, on %s", groups[h.m.group])
	}
	for i := 0; i < 20 && (h.m.selected() == nil || h.m.selected().value != "paddingx"); i++ {
		h.key("down")
	}
	if o := h.m.selected(); o == nil || o.value != "paddingx" || o.detail != "2" {
		t.Fatalf("Paddingx should be listed with its value, on %+v", o)
	}
	if !strings.Contains(h.view(), "←→ change") {
		t.Fatalf("the keys should say the setting can be changed in place:\n%s", h.view())
	}
	h.key("right")
	if o := h.m.selected(); !strings.Contains(h.config(), "window-padding-x = 4\n") || o == nil || o.value != "paddingx" || o.detail != "4" || groups[h.m.group] != "Window" {
		t.Fatalf("right should step the setting and stay on it, on %+v:\n%s", o, h.config())
	}
	h.key("left", "left")
	if !strings.Contains(h.config(), "window-padding-x = 0\n") {
		t.Fatalf("left twice should step it down to 0:\n%s", h.config())
	}
	before := h.config()
	h.key("left")
	if h.config() != before || groups[h.m.group] != "Window" {
		t.Fatal("at the end of the scale the arrow does nothing, and does not leave the section")
	}
	h.key("home", "right")
	if groups[h.m.group] != "Input" {
		t.Fatalf("on a line with no scale the arrows walk the sections, on %s", groups[h.m.group])
	}
}

// The Extensions section fetches what Ghostty can be extended with, shaders
// and packs of themes, and says exactly what it writes. Fonts are not there:
// they are installed where they are chosen.
func TestExtensionsInstallShadersAndThemePacks(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	var asked []string
	real := extensions.Get
	extensions.Get = func(url string) ([]byte, error) {
		asked = append(asked, url)
		if strings.HasSuffix(url, ".glsl") {
			return []byte("void mainImage(out vec4 colour, in vec2 at) {}\n"), nil
		}
		return []byte("background = #1e1e2e\nforeground = #cdd6f4\n"), nil
	}
	defer func() { extensions.Get = real }()

	var names []string
	for _, c := range h.m.commands() {
		if c.group == "Extensions" {
			names = append(names, c.name)
		}
	}
	if strings.Join(names, " ") != "shader pack" {
		t.Fatalf("Extensions should hold shaders and theme packs, and nothing about fonts: %v", names)
	}
	if !strings.Contains(h.view(), " Extensions ") {
		t.Fatalf("the section should be among the others at the top:\n%s", h.view())
	}

	h.typeText("shader ")
	v := h.view()
	for _, want := range []string{"none", "Cursor effects", "Screen effects", "cursor_smear", "bloom", "download"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the list of shaders lacks %q:\n%s", want, v)
		}
	}
	h.typeText("bloom")
	if v := h.view(); !strings.Contains(v, "0xhckr/ghostty-shaders") || !strings.Contains(v, "none stated") {
		t.Fatalf("a shader whose repository states no licence should say so:\n%s", v)
	}
	h.key("esc")
	h.typeText("shader cursor_bl")
	if o := h.m.selected(); o == nil || o.value != "cursor_blaze" {
		t.Fatalf("typing should find the shader, on %+v", o)
	}
	if v := h.view(); !strings.Contains(v, "KroneCorylus/ghostty-shader-playground") || !strings.Contains(v, "MIT") || !strings.Contains(v, "custom-shader = ") {
		t.Fatalf("the panel should say where it comes from and what is written:\n%s", v)
	}
	if len(asked) != 0 || h.config() != "theme = Dracula\n" {
		t.Fatal("looking at a shader fetches nothing and writes nothing")
	}
	h.key("enter")
	file := filepath.Join(h.paths.ConfigDir, "shaders", "cursor_blaze.glsl")
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("the shader should be installed beside the config: %v", err)
	}
	if len(asked) != 1 || asked[0] != "https://raw.githubusercontent.com/KroneCorylus/ghostty-shader-playground/main/public/shaders/cursor_blaze.glsl" {
		t.Fatalf("exactly that file should have been fetched: %v", asked)
	}
	if !strings.Contains(h.config(), "custom-shader = "+file+"\n") {
		t.Fatalf("the config should name the shader:\n%s", h.config())
	}
	// Nothing reloads Ghostty here, so nothing is on screen to ask about.
	if h.m.overlay != overlayNone {
		t.Fatalf("no question without a reload, overlay %v", h.m.overlay)
	}
	h.key("esc")
	h.typeText("shader ")
	if o := h.m.selected(); o == nil || o.value != "cursor_blaze" || !o.current || o.detail != "on" {
		t.Fatalf("the list should open on the shader that is on, on %+v", o)
	}
	h.key("esc")
	h.typeText("shader none")
	h.key("enter")
	if strings.Contains(h.config(), "\ncustom-shader = ") {
		t.Fatalf("none should switch the shader off:\n%s", h.config())
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("switching a shader off keeps its file")
	}

	// A pack of themes lands in the themes directory and on the wall.
	h.key("esc")
	h.typeText("pack catp")
	h.key("enter")
	for _, name := range []string{"catppuccin-latte", "catppuccin-frappe", "catppuccin-macchiato", "catppuccin-mocha"} {
		if _, err := os.Stat(filepath.Join(h.paths.ThemesDir, name)); err != nil {
			t.Fatalf("the pack should have written %s: %v", name, err)
		}
		if _, ok := h.m.lib.Get(name); !ok {
			t.Fatalf("%s should be in the library at once", name)
		}
	}
	if o := h.m.results[0]; o.label != "catppuccin" || o.detail != "installed" {
		t.Fatalf("the pack should be listed as installed, is %+v", o)
	}
}

// Where Ghostty reloads by itself a shader is on screen as soon as it is
// switched on, and a shader that fails can leave the window unreadable: the
// question whether to keep it puts the config back when it goes unanswered.
func TestAShaderIsPutBackUnlessItIsKept(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	realGet, realReload, realDelay := extensions.Get, reloadGhostty, keepDelay
	extensions.Get = func(string) ([]byte, error) { return []byte("void mainImage(out vec4 colour, in vec2 at) {}\n"), nil }
	reloads := 0
	reloadGhostty = func(string) error { reloads++; return nil }
	keepDelay = time.Millisecond
	defer func() { extensions.Get, reloadGhostty, keepDelay = realGet, realReload, realDelay }()
	h.m.opts.NoReload = false
	h.m.caps.inGhostty = true

	// Unanswered: the harness runs the timer out, and the line is gone again.
	h.typeText("shader bloom")
	h.key("enter")
	if strings.Contains(h.config(), "\ncustom-shader = ") || h.m.overlay != overlayNone || reloads < 2 {
		t.Fatalf("an unanswered question should put the config back and reload (%d reloads), overlay %v:\n%s", reloads, h.m.overlay, h.config())
	}
	if !strings.Contains(h.m.status, "was not kept") {
		t.Fatalf("the status should say it was put back, says %q", h.m.status)
	}

	// Answered: the timer of this question is left unrun, as if it were
	// still counting, and y keeps the shader.
	h.key("esc")
	h.typeText("shader bloom")
	h.m.Update(keyPress("enter"))
	if h.m.overlay != overlayConfirm || !strings.Contains(h.m.confirmText, "Keep it?") {
		t.Fatalf("switching a shader on should ask whether to keep it, overlay %v", h.m.overlay)
	}
	h.key("y")
	if !strings.Contains(h.config(), "\ncustom-shader = ") || !strings.Contains(h.m.status, "kept") {
		t.Fatalf("y should keep the shader:\n%s", h.config())
	}
	// And Esc on the question puts it back, as no does.
	h.key("esc")
	h.typeText("shader crt")
	h.m.Update(keyPress("enter"))
	h.key("esc")
	if !strings.Contains(h.config(), "bloom.glsl") || strings.Contains(h.config(), "crt.glsl") {
		t.Fatalf("Esc should put back the shader that was on before:\n%s", h.config())
	}
}
