package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/vitruves/ghostty-config/internal/collection"
	"github.com/vitruves/ghostty-config/internal/color"
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
	writeDebounce = time.Millisecond
	// Tests have no terminal to detect: draw in full colour, rounded.
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Setenv("GHOSTTY_CONFIG_GLYPHS", "round")
	m := New(paths, tree, lib, state, Options{NoReload: true})
	m.live = nil
	// Tests start from the menu; opening on the themes has its own test.
	m.setPromptText("")
	m.lastText = ""
	// A blinking cursor returns a timer on every keystroke, which the
	// harness would sit through.
	m.prompt.Cursor.SetMode(cursor.CursorStatic)
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

func (h *harness) key(keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEscape}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		h.send(msg)
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

func (h *harness) view() string { return ansi.Strip(h.m.View()) }

func (h *harness) dump(name string) {
	dir := os.Getenv("GHOSTTY_CONFIG_DUMP")
	if dir == "" {
		return
	}
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, name+".txt"), []byte(h.view()), 0o644)
}

func (h *harness) config() string {
	data, _ := os.ReadFile(filepath.Join(h.dir, "xdg", "config"))
	return string(data)
}

// click presses at a cell of the editor; the terminal reports it counted
// from the top of the screen, and the editor sits in the bottom rows.
func (h *harness) click(x, y int) {
	h.send(tea.MouseMsg{X: x, Y: y + h.m.top, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
}

func (h *harness) find(kind hitKind, index int, name string) (region, bool) {
	h.m.View()
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
	// The highlighted command is described in full, never cut.
	if !strings.Contains(v, "Theme <name>") || !strings.Contains(v, "moving previews it in this window.") {
		t.Fatalf("the highlighted command should be described in full:\n%s", v)
	}
	// Around the palette is the user's own terminal: nothing is drawn there.
	if strings.Contains(v, "user@host") {
		t.Fatalf("no terminal sample is drawn around the palette:\n%s", v)
	}
	lines := strings.Split(v, "\n")
	if len(lines) != InlineHeight(28) {
		t.Fatalf("view should fill the bottom rows it takes: %d lines", len(lines))
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
	if !strings.Contains(v, "Paddingx") || !strings.Contains(v, "Paddingcolor") || !strings.Contains(v, "searching every group") || strings.Contains(v, "Autoreload") {
		t.Fatalf("typing should search the commands:\n%s", v)
	}
	h.dump("03-search")
	h.key("esc")

	// F2 hides the palette to look at the terminal alone.
	h.send(tea.KeyMsg{Type: tea.KeyF2})
	if strings.Contains(h.view(), "╭") || !strings.Contains(h.view(), "brings the palette back") {
		t.Fatalf("F2 should leave only the terminal:\n%s", h.view())
	}
	h.key("x")
	if !strings.Contains(h.view(), "╭") {
		t.Fatal("any key should bring the palette back")
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
	for _, want := range []string{"╭", "╮", "╰", "╯", "normal", "bright", "#282a36", "█"} {
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
	h.send(tea.MouseMsg{X: group.x, Y: group.y + 4, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
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
	h.key("down", "down", "down")
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
	// By default nothing paints a background: the palette sits on the
	// terminal's own, so the frame around it can be round.
	painted := regexp.MustCompile(`[\[;]48;`)
	if painted.MatchString(strings.Split(h.m.View(), "\n")[1]) || h.m.panelChrome().paint() != "" || !strings.Contains(h.view(), "╭") || !strings.Contains(h.view(), "╯") {
		t.Fatalf("the clear palette paints no background and has a rounded frame:\n%s", h.view())
	}
	if len(strings.Split(h.m.View(), "\n")) != InlineHeight(28) || h.m.top != 28-InlineHeight(28) {
		t.Fatal("the editor draws in the bottom rows only")
	}
	h.send(tea.KeyMsg{Type: tea.KeyF2})
	h.key("x")
	h.key("esc")
	h.typeText("interface paper")
	h.key("enter")
	h.key("esc")
	h.typeText("theme ")
	if !strings.Contains(h.m.View(), paper) {
		t.Fatal("interface paper draws a light palette")
	}
	if !color.IsDark(h.m.cur.Background()) || color.IsDark(h.m.panelChrome().bg) {
		t.Fatal("a light palette over a dark theme is the point of paper")
	}
	ui := h.m.panelChrome()
	if color.Contrast(ui.border, ui.bg) < 2.5 || color.Contrast(ui.border, h.m.cur.Background()) < 2.5 {
		t.Fatalf("the border %s should stand out from the card and from the terminal", ui.border)
	}
	// Every ramp starts in the same column, whatever the row says after it.
	column := -1
	rows := 0
	for _, l := range strings.Split(h.view(), "\n") {
		if !strings.Contains(l, " dark ") && !strings.Contains(l, " light ") {
			continue
		}
		i := strings.Index(l, "████████████████")
		if i < 0 {
			continue
		}
		at := ansi.StringWidth(l[:i])
		if column >= 0 && at != column {
			t.Fatalf("ramps are not aligned: column %d and %d\n%s", column, at, h.view())
		}
		column = at
		rows++
	}
	if rows < 8 {
		t.Fatalf("expected a long list of themes, saw %d rows:\n%s", rows, h.view())
	}
	h.dump("13-theme-list")
	h.key("esc")
	h.typeText("interface theme")
	h.key("enter")
	if strings.Contains(h.m.View(), paper) || h.m.state.Interface != "theme" {
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
	h.send(tea.KeyMsg{Type: tea.KeyF1})
	if !strings.Contains(h.view(), "Keys and commands") || !strings.Contains(h.view(), "Theme <name>") {
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
	raw := h.m.View()
	if strings.ContainsAny(raw, "\ue0b6\ue0b4"+cornerTL+cornerTR+cornerBL+cornerBR+bandTop+bandBot) {
		t.Fatal("glyphs the terminal may not have must not be drawn")
	}
	for i, l := range strings.Split(ansi.Strip(raw), "\n") {
		if w := ansi.StringWidth(l); w != 105 {
			t.Fatalf("line %d is %d cells wide, want 105: %q", i, w, l)
		}
	}
	// Every cell is painted: after a reset the colours are set again before
	// anything is drawn, so nothing ever shows the terminal's own background.
	const reset = "\x1b[0m"
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
