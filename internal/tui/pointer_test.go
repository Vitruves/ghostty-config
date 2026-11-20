package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// indexOfCommand is the line of the menu a command is on.
func (h *harness) indexOfCommand(name string) int {
	for i, o := range h.m.results {
		if o.cmd != nil && o.cmd.name == name {
			return i
		}
	}
	h.t.Fatalf("no command %q in the menu", name)
	return -1
}

func (h *harness) motion(x, y int, held bool) {
	b := tea.MouseNone
	if held {
		b = tea.MouseLeft
	}
	h.send(tea.MouseMotionMsg{X: x, Y: y + h.m.top, Button: b})
}

// A slider is pulled: the value follows the pointer on screen and is written
// once, when the button is let go.
func TestPointerDragsSlider(t *testing.T) {
	h := newPixHarness(t, "midnight")
	h.m.jumpToGroup("Window")
	at := h.indexOfCommand("opacity")
	r, ok := h.find(hitValue, at, "slider")
	if !ok {
		t.Fatal("the opacity line has no slider to click")
	}
	before := h.config()
	h.click(r.x, r.y)
	if h.m.drag == nil {
		t.Fatal("a press on the slider did not take hold of it")
	}
	h.motion(r.x+r.w/2, r.y, true)
	h.shot("40-slider-held")
	if h.config() != before {
		t.Fatalf("the config was written while the slider was still held:\n%s", h.config())
	}
	mid := h.m.drag.value
	h.motion(r.x+r.w-1, r.y, true)
	if h.m.drag.value == mid {
		t.Fatalf("the held slider did not follow the pointer: still %s", mid)
	}
	h.send(tea.MouseReleaseMsg{X: r.x + r.w - 1, Y: r.y + h.m.top, Button: tea.MouseLeft})
	if h.m.drag != nil {
		t.Fatal("the slider is still held after the button was let go")
	}
	// The maximum is Ghostty's default: the line goes.
	if strings.Contains(h.config(), "\nbackground-opacity = 0.85") || h.m.results[at].cmd.set.get(h.m) != "1.00" {
		t.Fatalf("letting go at the end of the track did not write the maximum:\n%s", h.config())
	}

	// Opened, the setting has the whole scale to pull.
	h.m.setPromptText("Opacity ")
	wide, ok := h.find(hitValue, -1, "slider")
	if !ok {
		t.Fatal("an open number has no slider")
	}
	h.click(wide.x+wide.w/2, wide.y)
	h.shot("42-wide-slider")
	h.send(tea.MouseReleaseMsg{X: wide.x + wide.w/2, Y: wide.y + h.m.top, Button: tea.MouseLeft})
	if !strings.Contains(h.config(), "\nbackground-opacity = 0.6") || h.m.cmd == nil {
		t.Fatalf("the open setting's slider did not write the middle of the scale, or closed it:\n%s", h.config())
	}
	h.m.setPromptText("")
	h.m.jumpToGroup("Window")
	if r, ok = h.find(hitValue, h.indexOfCommand("opacity"), "slider"); !ok {
		t.Fatal("back on the menu, the opacity line has no slider")
	}

	// A release the terminal never sent: the next move with no button held
	// writes what the slider was on.
	h.click(r.x, r.y)
	h.motion(r.x, r.y+1, false)
	if h.m.drag != nil || !strings.Contains(h.config(), "background-opacity = 0.3") {
		t.Fatalf("a move with no button did not end the pull at the minimum:\n%s", h.config())
	}
}

// The mark of a changed setting puts the default back, the button of a
// setting with many values opens them, and Changed walks the changed settings.
func TestPointerResetsAndSteps(t *testing.T) {
	h := newPixHarness(t, "midnight")
	h.m.jumpToGroup("Window")
	at := h.indexOfCommand("paddingcolor")
	h.motion(0, 0, false)
	h.m.sel.cursor = at
	r, ok := h.find(hitValue, at, resetValue)
	if !ok {
		t.Fatal("a changed setting under the highlight offers no way back to its default")
	}
	h.click(r.x, r.y)
	if strings.Contains(h.config(), "\nwindow-padding-color") {
		t.Fatalf("the arrow did not put the padding colour back to its default:\n%s", h.config())
	}
	if _, ok := h.find(hitValue, at, resetValue); ok {
		t.Fatal("a setting at its default still offers to be put back")
	}

	// More than four values: a button with the value, that opens their
	// list as Enter does.
	at = h.indexOfCommand("blur")
	h.m.sel.cursor = at
	was := h.m.results[at].cmd.set.get(h.m)
	r, ok = h.find(hitValue, at, openValue)
	if !ok {
		t.Fatal("a setting with many values has no button to open them")
	}
	h.click(r.x+r.w/2, r.y)
	if h.m.screen() != screenOptions || h.m.cmd == nil || h.m.cmd.set == nil || h.m.cmd.set.cmd != "blur" {
		t.Fatal("the button did not open the list of the blur's values")
	}
	if got := h.m.cmd.set.get(h.m); got != was {
		t.Fatalf("opening the list changed the blur from %s to %s", was, got)
	}
	h.m.setPromptText("")
	h.m.jumpToGroup("Window")

	r, ok = h.find(hitButton, -1, "next-changed")
	if !ok {
		t.Fatal("Changed is not there to click")
	}
	seen := map[int]bool{}
	for i := 0; i < 3; i++ {
		h.click(r.x, r.y)
		o := h.m.selected()
		if o == nil || o.cmd == nil || !h.m.changed(o.cmd.set) {
			t.Fatalf("Changed went to a line that is not changed: %d", h.m.sel.cursor)
		}
		seen[h.m.sel.cursor] = true
	}
	if len(seen) < 2 {
		t.Fatal("Changed stayed on one setting")
	}
}

// The pointer marks what it is over without moving the highlight, and the
// keys take the mark away.
func TestPointerHover(t *testing.T) {
	for iface, shot := range map[string]string{"midnight": "41-hover", "paper": "41-hover-light"} {
		pointerHover(t, iface, shot)
	}
}

func pointerHover(t *testing.T, iface, shot string) {
	h := newPixHarness(t, iface)
	h.m.jumpToGroup("Input")
	at := h.indexOfCommand("blink")
	r, ok := h.find(hitValue, at, "false")
	if !ok {
		t.Fatal("no pill for false on the blink line")
	}
	cur := h.m.sel.cursor
	h.motion(r.x+r.w/2, r.y, false)
	if h.m.sel.cursor != cur {
		t.Fatal("passing over a line moved the highlight")
	}
	h.shot(shot)
	h.key("down")
	if h.m.hoverX >= 0 {
		t.Fatal("the pointer's mark stayed after a key")
	}
}
