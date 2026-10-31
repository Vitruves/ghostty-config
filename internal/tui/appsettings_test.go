package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

func f4() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyF4} }

// saved is the state file as written.
func (h *harness) saved() string {
	data, _ := os.ReadFile(filepath.Join(h.paths.StateDir, "state.json"))
	return string(data)
}

// toRow highlights the row of a preference in the open dialog.
func (h *harness) toRow(p *uiPref) {
	h.t.Helper()
	rows, _ := h.m.settingsRows()
	for i, r := range rows {
		if r == p {
			h.m.ps.at = i
			return
		}
	}
	h.t.Fatalf("no row %s in F4", p.name)
}

// F4 opens the editor's own settings in every layout and in both
// interfaces; F4 again, Esc and a click off the card close them.
func TestF4OpensAndClosesEverywhere(t *testing.T) {
	for _, prof := range []string{ghostty.ProfileAtelier, ghostty.ProfilePalette, ghostty.ProfileGuided} {
		for _, pixel := range []bool{false, true} {
			was := harnessProfile
			harnessProfile = prof
			var h *harness
			if pixel {
				h = newPixHarness(t, "midnight")
			} else {
				h = newHarness(t, "")
			}
			harnessProfile = was
			name := prof
			if pixel {
				name += " pixel"
			}
			h.send(f4())
			if h.m.overlay != overlaySettings {
				t.Fatalf("%s: F4 should open the settings, overlay %d", name, h.m.overlay)
			}
			if pixel {
				h.shot("f4-" + prof)
			} else if v := h.view(); !strings.Contains(v, "ghostty-config settings") || !strings.Contains(v, "Simple glyphs") {
				t.Fatalf("%s: the settings should show:\n%s", name, v)
			}
			h.send(f4())
			if h.m.overlay != overlayNone {
				t.Fatalf("%s: F4 again should close them", name)
			}
			h.send(f4())
			h.key("esc")
			if h.m.overlay != overlayNone {
				t.Fatalf("%s: Esc should close them", name)
			}
			h.send(f4())
			h.render()
			h.click(0, 0)
			if h.m.overlay != overlayNone {
				t.Fatalf("%s: a click off the card should close them", name)
			}
			// From the help too, which only reads.
			h.m.overlay = overlayHelp
			h.send(f4())
			if h.m.overlay != overlaySettings {
				t.Fatalf("%s: F4 should open over the help", name)
			}
		}
	}
}

func (h *harness) render() { h.m.render() }

// The arrows show a value at once; Esc puts back what was not kept, and the
// state file never holds it.
func TestF4ChangeThenEscPutsBack(t *testing.T) {
	h := newHarness(t, "")
	h.send(f4())
	h.toRow(prefAccent)
	h.key("right")
	if h.m.state.Accent != "mono" {
		t.Fatalf("→ on the accent should show the next one: %q", h.m.state.Accent)
	}
	if !strings.Contains(h.view(), "‹ ") {
		t.Error("the highlighted row should show its arrows")
	}
	// Another row changed meanwhile, then the dialog closed.
	h.toRow(prefContrast)
	h.key("right")
	if h.m.state.Contrast != "high" {
		t.Fatal("→ on the contrast should show high")
	}
	h.key("esc")
	if h.m.state.Accent != "" || h.m.state.Contrast != "" {
		t.Fatalf("Esc should put back both: accent %q, contrast %q", h.m.state.Accent, h.m.state.Contrast)
	}
	if s := h.saved(); strings.Contains(s, "mono") || strings.Contains(s, "high") {
		t.Fatalf("nothing looked at should be saved:\n%s", s)
	}
	// A long row lists its values: walking shows each, Esc in the list puts
	// back the one before it.
	h.send(f4())
	h.toRow(prefInterface)
	h.key("enter")
	if !h.m.ps.list {
		t.Fatal("Enter on the scheme should list its values")
	}
	h.key("up", "up")
	if h.m.state.Interface == "clear" {
		t.Fatal("walking the schemes should show each")
	}
	h.key("esc")
	if h.m.state.Interface != "clear" || h.m.ps.list || h.m.overlay != overlaySettings {
		t.Fatalf("Esc in the list should put the scheme back and go back to the rows: %q", h.m.state.Interface)
	}
	// Simple glyphs come into effect at once, and go with Esc.
	h.toRow(prefGlyphs)
	h.key("right")
	if h.m.caps.roundCaps {
		t.Fatal("simple glyphs should drop the round ends at once")
	}
	h.send(f4())
	if !h.m.caps.roundCaps || h.m.state.Plain {
		t.Fatal("F4 should put the round ends back")
	}
}

// Enter keeps the highlighted row in state.json, and only it: another row
// still being looked at is not saved with it.
func TestF4EnterKeepsInTheStateFile(t *testing.T) {
	h := newHarness(t, "")
	h.send(f4())
	h.toRow(prefAccent)
	h.key("right")
	h.toRow(prefContrast)
	h.key("right", "enter")
	s := h.saved()
	if !strings.Contains(s, `"contrast": "high"`) {
		t.Fatalf("Enter should keep the contrast:\n%s", s)
	}
	if strings.Contains(s, `"accent"`) {
		t.Fatalf("the accent was only looked at:\n%s", s)
	}
	if h.m.state.Accent != "mono" {
		t.Fatal("the accent should still be shown")
	}
	h.key("esc")
	if h.m.state.Contrast != "high" || h.m.state.Accent != "" {
		t.Fatalf("Esc keeps what Enter kept and puts back the rest: contrast %q, accent %q", h.m.state.Contrast, h.m.state.Accent)
	}
	// The drawing is listed, and says it is for the next launch.
	h.send(f4())
	h.toRow(prefDrawing)
	if !strings.Contains(h.view(), "applies at next launch") {
		t.Errorf("the drawing should say when it applies:\n%s", h.view())
	}
	h.key("enter", "down", "down", "enter")
	if s := h.saved(); !strings.Contains(s, `"drawing": "cells"`) {
		t.Fatalf("the list should keep the drawing:\n%s", s)
	}
	// Reload and the update check, off.
	h.toRow(prefAutoReload)
	h.key("right", "enter")
	h.toRow(prefUpdates)
	h.key("right", "enter")
	s = h.saved()
	if !strings.Contains(s, `"auto_reload": false`) || !strings.Contains(s, `"update_check": false`) {
		t.Fatalf("Enter should keep reload and the update check off:\n%s", s)
	}
	if h.m.state.AutoReloadEnabled() {
		t.Fatal("reload should be off at once")
	}
	if l := ghostty.LoadState(h.paths); l.Drawing != "cells" || l.UpdateCheckEnabled() || l.Contrast != "high" {
		t.Fatalf("the state read back: %+v", l)
	}
}

// The layout is switched by Enter, through the profiles' seam, never by the
// arrows alone.
func TestF4SwitchesTheLayout(t *testing.T) {
	h := newHarness(t, "")
	h.send(f4())
	h.toRow(prefLayout)
	h.key("right")
	if h.m.state.ActiveProfile() != ghostty.ProfileAtelier {
		t.Fatal("the arrows alone should not switch the layout")
	}
	if !strings.Contains(h.view(), "Palette") {
		t.Errorf("the row should show the layout the arrows came to:\n%s", h.view())
	}
	h.key("enter")
	if h.m.state.ActiveProfile() != ghostty.ProfilePalette {
		t.Fatalf("Enter should switch to Palette: %q", h.m.state.ActiveProfile())
	}
	if !strings.Contains(h.saved(), `"profile": "palette"`) {
		t.Fatal("the layout should be saved")
	}
	// Esc after a switch puts nothing back.
	h.key("esc")
	if h.m.state.ActiveProfile() != ghostty.ProfilePalette {
		t.Fatal("the layout switched to stays")
	}
}

// A flag given wins: its row shows it and the arrows leave it.
func TestF4ShowsWhatAFlagSet(t *testing.T) {
	h := newHarnessWith(t, "", Options{NoReload: true, Locked: map[string]Lock{"drawing": {Flag: "-cells", Value: "cells"}}})
	h.send(f4())
	h.toRow(prefDrawing)
	h.key("right", "enter")
	if h.m.state.Drawing != "" {
		t.Fatalf("a row a flag set should not change: %q", h.m.state.Drawing)
	}
	if v := h.view(); !strings.Contains(v, "set by -cells") {
		t.Errorf("the row should name the flag:\n%s", v)
	}
}

// The mouse: a click on a value steps it or lists it, a click on a listed
// value keeps it.
func TestF4WithTheMouse(t *testing.T) {
	h := newHarness(t, "")
	h.send(f4())
	rows, _ := h.m.settingsRows()
	contrast := indexOf(func() []string {
		var n []string
		for _, r := range rows {
			n = append(n, r.name)
		}
		return n
	}(), "uicontrast")
	r, ok := h.find(hitButton, contrast, "ps-value")
	if !ok {
		t.Fatal("the contrast's value should answer a click")
	}
	h.click(r.x, r.y)
	if h.m.ps.at != contrast || h.m.state.Contrast != "high" {
		t.Fatalf("a click on the value should step it: row %d, %q", h.m.ps.at, h.m.state.Contrast)
	}
	h.toRow(prefAccent)
	accent := h.m.ps.at
	r, _ = h.find(hitButton, accent, "ps-value")
	h.click(r.x, r.y)
	if !h.m.ps.list {
		t.Fatal("a click on a long row's value should list it")
	}
	r, ok = h.find(hitButton, 4, "ps-pick")
	if !ok {
		t.Fatal("the listed values should answer a click")
	}
	h.click(r.x, r.y)
	if !strings.Contains(h.saved(), `"accent": "`+prefAccent.values[4]+`"`) {
		t.Fatalf("a click on a listed value should keep it:\n%s", h.saved())
	}
}

// The dialog fits 80×24 in both interfaces, its last line included.
func TestF4FitsIn80x24(t *testing.T) {
	h := newHarness(t, "")
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	h.send(f4())
	v := h.view()
	for _, want := range []string{"ghostty-config settings", "LOOK", "BEHAVIOUR", "Layout", "Simple glyphs", "Esc close"} {
		if !strings.Contains(v, want) {
			t.Errorf("80×24 should show %q:\n%s", want, v)
		}
	}
	if n := len(strings.Split(h.m.render(), "\n")); n != 24 {
		t.Fatalf("%d rows", n)
	}
	h.dump("f4-80x24-rows-cells")
	h.toRow(prefDrawing)
	h.key("enter")
	if v := h.view(); !strings.Contains(v, "text only") || !strings.Contains(v, "Esc back") {
		t.Errorf("the list should fit 80×24:\n%s", v)
	}
	h.dump("f4-80x24-list-cells")

	p := newPixHarness(t, "midnight")
	p.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	p.send(f4())
	p.shot("f4-80x24")
	rows, _ := p.m.settingsRows()
	last, ok := p.find(hitButton, len(rows)-1, "ps-row")
	card, _ := p.find(hitButton, 0, "ps-card")
	if !ok || last.y >= 24 || card.y < 0 || card.y+card.h > 24 {
		t.Fatalf("the picture dialog should fit 80×24: last row %+v, card %+v", last, card)
	}
}
