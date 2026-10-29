package tui

import (
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// stubKeys makes the shortcut editor read Ghostty 1.3's actions and macOS
// defaults from the copy built in, and spell shortcuts the Mac's way,
// whatever machine the tests run on.
func stubKeys(t *testing.T) {
	t.Helper()
	load, mac := loadKeyCatalog, keysMac
	loadKeyCatalog = func(string) ghostty.KeyCatalog { return ghostty.BuiltinKeyCatalog(true) }
	keysMac = true
	t.Cleanup(func() { loadKeyCatalog, keysMac = load, mac })
}

// openKeys opens the list with filter typed after the command word.
func (h *harness) openKeys(filter string) {
	h.t.Helper()
	h.m.setPromptText("Keys " + filter)
	if cmd := h.m.afterMove(); cmd != nil {
		h.runCmd(cmd)
	}
	if !h.m.kb.loaded {
		h.t.Fatal("the actions were not read")
	}
}

func (h *harness) row(action string) option {
	h.t.Helper()
	for i, o := range h.m.results {
		if o.key != nil && o.value == action {
			h.m.sel.cursor = i
			return o
		}
	}
	h.t.Fatalf("no row for %s", action)
	return option{}
}

func press(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: mod}
}

func TestKeysListsActionsInGroupsWithTheirShortcuts(t *testing.T) {
	stubKeys(t)
	h := newHarness(t, "keybind = ctrl+shift+n=new_window\nkeybind = super+k=unbind\n")
	h.openKeys("")

	var groups []string
	for _, o := range h.m.results {
		if o.header {
			groups = append(groups, o.label)
		}
	}
	if strings.Join(groups[:5], ",") != "Tabs,Splits,Windows,Quick terminal,Clipboard" {
		t.Fatalf("groups: %v", groups)
	}
	if o := h.row("new_tab"); o.detail != "⌘T" || o.current {
		t.Fatalf("new tab is Ghostty's ⌘T: %+v", o)
	}
	if o := h.row("new_window"); !o.current || !strings.Contains(o.detail, "⌃⇧N") {
		t.Fatalf("new window carries the user's shortcut: %+v", o)
	}
	if o := h.row("goto_tab:1"); o.detail != "⌘1" {
		t.Fatalf("tab 1's two default spellings are one shortcut: %q", o.detail)
	}
	if o := h.row("clear_screen"); o.detail != "" {
		t.Fatalf("clear_screen was unbound: %q", o.detail)
	}
	h.row("toggle_quick_terminal")
	h.row("goto_tab:9")
	h.row("new_split:left")
	n := 0
	for _, o := range h.m.results {
		if o.key != nil && strings.HasPrefix(o.value, "goto_tab:") {
			n++
		}
	}
	if n != 9 {
		t.Fatalf("goto_tab is offered for tabs 1 to 9, got %d", n)
	}
	for _, o := range h.m.results {
		if o.value == "text" || o.value == "set_font_size" || o.value == "close_tab" || o.value == "toggle_tab_overview" {
			t.Fatalf("%s should not be listed here", o.value)
		}
	}
	h.row("new_tab")
	h.m.centre = true
	h.dump("keys-cells-list")
	if !strings.Contains(h.view(), "⌘T") {
		t.Fatal("the text interface should show the shortcuts")
	}
	if got := h.m.currentValueOf(h.m.cmd); got != "1 of yours" {
		t.Fatalf("the menu says %q", got)
	}
}

func TestKeysSearchByNameDescriptionAndShortcut(t *testing.T) {
	stubKeys(t)
	h := newHarness(t, "")
	h.openKeys("toggle_quick_terminal")
	if o := h.m.selected(); o == nil || o.value != "toggle_quick_terminal" {
		t.Fatalf("keys toggle_quick_terminal should land on it: %+v", o)
	}
	h.openKeys("cmd+shift+]")
	if o := h.m.selected(); o == nil || o.value != "next_tab" {
		t.Fatalf("a typed shortcut finds its action: %+v", o)
	}
	h.openKeys("scrollback")
	found := false
	for _, o := range h.m.results {
		found = found || o.value == "write_scrollback_file:open"
	}
	if !found {
		t.Fatal("a word of the description finds the action")
	}
}

func TestCaptureTurnsKeysIntoTriggers(t *testing.T) {
	cases := []struct {
		key  tea.Key
		want string
	}{
		{tea.Key{Code: 't', Mod: tea.ModSuper | tea.ModShift}, "super+shift+t"},
		{tea.Key{Code: 'a', Mod: tea.ModCtrl}, "ctrl+a"},
		{tea.Key{Code: tea.KeyLeft, Mod: tea.ModAlt}, "alt+arrow_left"},
		{tea.Key{Code: tea.KeyEnter, Mod: tea.ModShift}, "shift+enter"},
		{tea.Key{Code: tea.KeyF5}, "f5"},
		{tea.Key{Code: ',', Mod: tea.ModSuper}, "super+,"},
		{tea.Key{Code: tea.KeyPgDown, Mod: tea.ModSuper}, "super+page_down"},
		{tea.Key{Code: 'T', Mod: tea.ModCtrl}, "ctrl+t"},
	}
	for _, c := range cases {
		ch, ok := captureChord(c.key)
		if !ok || ch.String() != c.want {
			t.Errorf("%v: got %q %v, want %q", c.key, ch.String(), ok, c.want)
		}
	}
	for _, k := range []tea.Key{{Code: 'a', Text: "a"}, {Code: 'a', Text: "A", Mod: tea.ModShift}, {Code: tea.KeyUp}, {Code: tea.KeySpace, Text: " "}} {
		if ch, ok := captureChord(k); ok {
			t.Errorf("%v is typing, not a shortcut: %q", k, ch.String())
		}
	}
}

func TestCaptureWritesTheShortcut(t *testing.T) {
	stubKeys(t)
	h := newHarness(t, "font-size = 13\nkeybind = super+j=scroll_to_bottom\n\ntheme = Dracula\n")
	h.openKeys("")
	h.row("toggle_quick_terminal")
	h.key("enter")
	if h.m.overlay != overlayCapture {
		t.Fatal("Enter should ask for a shortcut")
	}
	h.send(press(tea.KeyF5, 0))
	h.dump("keys-cells-capture")
	h.key("backspace", "backspace")
	if !strings.Contains(h.view(), "New shortcut for Quick terminal") {
		t.Fatalf("the text interface should show the question:\n%s", h.view())
	}
	if !h.m.kb.global {
		t.Fatal("the quick terminal's shortcut is global unless told otherwise")
	}
	h.send(press('`', tea.ModSuper))
	if h.m.kb.text != "super+`" {
		t.Fatalf("captured %q", h.m.kb.text)
	}
	h.key("enter")
	if h.m.overlay != overlayNone {
		t.Fatalf("Enter should write and close, error: %s", h.m.kb.err)
	}
	want := "keybind = super+j=scroll_to_bottom\nkeybind = global:super+`=toggle_quick_terminal\n\ntheme"
	if !strings.Contains(h.config(), want) {
		t.Fatalf("the binding goes next to the user's other one:\n%s", h.config())
	}
	if !strings.Contains(h.m.status, "Accessibility") {
		t.Fatalf("a global shortcut says what it needs: %q", h.m.status)
	}

	// Ctrl+G takes the global off again.
	h.row("toggle_quick_terminal")
	h.key("ctrl+g")
	if !strings.Contains(h.config(), "keybind = super+`=toggle_quick_terminal\n") {
		t.Fatalf("global off:\n%s", h.config())
	}
}

func TestCaptureTypesSequencesAndRefusesNonsense(t *testing.T) {
	stubKeys(t)
	h := newHarness(t, "")
	h.openKeys("")
	h.row("new_window")
	h.key("enter")
	h.send(press('a', tea.ModCtrl))
	h.typeText(">n")
	if h.m.kb.text != "ctrl+a>n" {
		t.Fatalf("typed %q", h.m.kb.text)
	}
	h.key("backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace")
	h.typeText("ctrl+bogus")
	h.key("enter")
	if h.m.overlay != overlayCapture || !strings.Contains(h.m.kb.err, "bogus") {
		t.Fatalf("a key Ghostty does not know is refused: %q", h.m.kb.err)
	}
	h.key("esc")
	if h.m.overlay != overlayNone || strings.Contains(h.config(), "keybind") {
		t.Fatalf("Esc writes nothing:\n%s", h.config())
	}

	h.key("enter")
	h.send(press('a', tea.ModCtrl))
	h.typeText(">n")
	h.key("enter")
	if !strings.Contains(h.config(), "keybind = ctrl+a>n=new_window") {
		t.Fatalf("the sequence is written:\n%s", h.config())
	}
}

func TestATakenShortcutAsksAndMovesIt(t *testing.T) {
	stubKeys(t)
	h := newHarness(t, "")
	h.openKeys("")
	h.row("new_window")
	h.key("enter")
	// Ghostty acts on ⌘T itself, so it is typed.
	h.typeText("super+t")
	h.key("enter")
	if h.m.overlay != overlayConfirm || !strings.Contains(h.m.confirmText, "New tab") {
		t.Fatalf("the conflict should be asked about: %q", h.m.confirmText)
	}
	h.key("n")
	if strings.Contains(h.config(), "keybind") {
		t.Fatal("no writes nothing")
	}
	h.row("new_window")
	h.key("enter")
	h.typeText("super+t")
	h.key("enter", "y")
	h.openKeys("")
	if o := h.row("new_tab"); o.detail != "" {
		t.Fatalf("new tab lost ⌘T: %q", o.detail)
	}
	if o := h.row("new_window"); !strings.Contains(o.detail, "⌘T") {
		t.Fatalf("new window has ⌘T: %q", o.detail)
	}

	// Delete puts new window back to Ghostty's own, and new tab gets ⌘T back.
	h.row("new_window")
	h.key("delete")
	if !strings.Contains(h.config(), "#keybind = super+t=new_window  # disabled by ghostty-config") {
		t.Fatalf("Delete switches the line off:\n%s", h.config())
	}
	if o := h.row("new_tab"); o.detail != "⌘T" {
		t.Fatalf("new tab has ⌘T again: %q", o.detail)
	}
}

func TestUnbindAndTypedBinding(t *testing.T) {
	stubKeys(t)
	h := newHarness(t, "")
	h.openKeys("")
	h.row("goto_tab:2")
	h.key("ctrl+x")
	cfg := h.config()
	if !strings.Contains(cfg, "keybind = super+digit_2=unbind\nkeybind = super+2=unbind\n") {
		t.Fatalf("both spellings of ⌘2 are unbound:\n%s", cfg)
	}
	if o := h.row("goto_tab:2"); o.detail != "" {
		t.Fatalf("tab 2 has no shortcut left: %q", o.detail)
	}
	h.key("delete")
	if o := h.row("goto_tab:2"); o.detail != "⌘2" {
		t.Fatalf("Delete gives Ghostty's back: %q", o.detail)
	}

	h.openKeys("super+shift+k=text:hello")
	h.key("enter")
	if !strings.Contains(h.config(), "keybind = super+shift+k=text:hello") {
		t.Fatalf("a typed binding is written as it is:\n%s", h.config())
	}
}

// TestPixelKeyScreens draws the shortcut editor in the picture interface;
// GHOSTTY_CONFIG_PXSHOTS=<dir> writes the pictures.
func TestPixelKeyScreens(t *testing.T) {
	stubKeys(t)
	config := pixConfig + "keybind = global:super+backquote=toggle_quick_terminal\nkeybind = ctrl+shift+n=new_window\nkeybind = super+k=unbind\nkeybind = ctrl+a>n=new_tab\n"
	for _, iface := range []string{"midnight", "paper"} {
		h := newHarnessWith(t, config, Options{NoReload: true, Images: true, ImgOut: io.Discard, Pixel: true, PixCell: [2]int{17, 38}})
		h.m.state.Interface = iface
		h.send(tea.WindowSizeMsg{Width: 120, Height: 34})
		h.openKeys("")
		h.row("new_tab")
		h.shot("keys-" + iface + "-1-list")
		if iface == "paper" {
			continue
		}
		h.row("toggle_quick_terminal")
		h.m.centre = true
		h.shot("keys-" + iface + "-2-quick-terminal")
		h.row("clear_screen")
		h.m.centre = true
		h.shot("keys-" + iface + "-3-unbound")

		h.row("toggle_quick_terminal")
		h.key("enter")
		h.shot("keys-" + iface + "-4-capture-empty")
		h.send(press('`', tea.ModSuper|tea.ModAlt))
		h.shot("keys-" + iface + "-5-capture")
		h.typeText(">x")
		h.key("enter")
		h.shot("keys-" + iface + "-6-capture-error")
		h.key("esc")

		h.row("new_window")
		h.key("enter")
		h.typeText("super+t")
		h.key("enter")
		h.shot("keys-" + iface + "-7-conflict")
		h.key("n")

		h.openKeys("tab")
		h.shot("keys-" + iface + "-8-search")
	}
}

// runCmd runs a command the way Bubble Tea would, batches included.
func (h *harness) runCmd(cmd tea.Cmd) {
	out := cmd()
	if batch, ok := out.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				h.runCmd(c)
			}
		}
		return
	}
	if out != nil {
		h.send(out)
	}
}
