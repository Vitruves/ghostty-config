package tui

import (
	"hash/maphash"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// frame is a hash of the picture the screen draws now.
func (h *harness) frame() uint64 {
	h.t.Helper()
	h.m.render()
	c := h.m.pixCanvas()
	h.m.drawScreen(c)
	return maphash.Bytes(maphash.Seed(h.m.px.seed), c.Img.Pix)
}

// Every preference of the look changes what is drawn, at the size of a
// usual window and at 80×24, and the default of each is the look the tool
// always had.
func TestEveryLookChangesThePicture(t *testing.T) {
	for _, size := range [][2]int{{120, 34}, {80, 24}} {
		for _, p := range []*uiPref{prefAccent, prefContrast, prefSize, prefCards} {
			for _, v := range p.values {
				h := newPixHarness(t, "midnight")
				h.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				h.m.setPromptText("Theme ")
				h.highlightTheme("matcha-whisk")
				before := h.frame()
				*p.field(h.m.state) = v
				after := h.frame()
				if (v == p.def) != (before == after) {
					t.Errorf("%dx%d: %s %s should change the picture unless it is the default", size[0], size[1], p.name, v)
				}
				name := p.name + "-" + v
				if size[0] == 80 {
					name += "-80x24"
				}
				h.shot("look-" + name)
			}
		}
	}
}

// Smaller cards make more of them in a row; a larger interface fewer.
func TestCardsAndSizeChangeTheWall(t *testing.T) {
	h := newPixHarness(t, "midnight")
	h.m.setPromptText("Theme ")
	cols := func() int { h.m.render(); return h.m.px.cols }
	normal := cols()
	h.m.state.Cards = "small"
	small := cols()
	h.m.state.Cards = "large"
	large := cols()
	if !(small > normal && normal > large) {
		t.Fatalf("cards per row: small %d, normal %d, large %d", small, normal, large)
	}
	h.m.state.Cards = ""
	h.m.state.UISize = "large"
	if n := cols(); n >= normal {
		t.Fatalf("a large interface should fit fewer cards in a row: %d, normal %d", n, normal)
	}
	h.m.state.UISize = "small"
	if n := cols(); n <= normal {
		t.Fatalf("a small interface should fit more cards in a row: %d, normal %d", n, normal)
	}
}

// Walking the accents shows each one; Esc puts back the one in effect, and
// only Enter keeps one.
func TestAnAccentLookedAtIsNotSaved(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.typeText("accent ")
	h.key("down")
	if h.m.state.Accent == "" {
		t.Fatal("moving the highlight should show another accent")
	}
	h.key("esc")
	if h.m.state.Accent != "" {
		t.Fatalf("Esc should put the accent back, is %q", h.m.state.Accent)
	}
	h.typeText("accent ")
	h.key("down")
	chosen := h.m.state.Accent
	h.key("enter", "esc", "esc")
	if got := ghostty.LoadState(h.paths).Accent; got != chosen || got == "" {
		t.Fatalf("Enter should keep the accent %q, saved %q", chosen, got)
	}
}

// The accent is the scheme's own until another is chosen, then that one,
// readable on the scheme's ground, light or dark; high contrast lifts the
// quiet text to 7:1.
func TestAccentAndContrastTuneTheScheme(t *testing.T) {
	for _, iface := range []string{"midnight", "paper", "auto"} {
		h := newHarness(t, "theme = Dracula\n")
		h.m.state.Interface = iface
		was := h.m.panelChrome()
		if was != h.m.schemeChrome() {
			t.Fatalf("%s: the default accent and contrast should leave the scheme alone", iface)
		}
		for _, a := range prefAccent.values[1:] {
			h.m.state.Accent = a
			c := h.m.panelChrome()
			if c.accent == was.accent || c.bg != was.bg || c.fg != was.fg {
				t.Fatalf("%s: accent %s gave %s on %s (was %s)", iface, a, c.accent, c.bg, was.accent)
			}
			if r := color.Contrast(c.accent, c.bg); r < 4.5 {
				t.Fatalf("%s: accent %s reads at %.1f:1", iface, a, r)
			}
		}
		h.m.state.Accent = ""
		h.m.state.Contrast = "high"
		if c := h.m.panelChrome(); !c.high || color.Contrast(c.muted, c.bg) < 7 {
			t.Fatalf("%s: high contrast left the quiet text at %.1f:1", iface, color.Contrast(c.muted, c.bg))
		}
	}
}

// The text interface draws a fixed grid of cells: the sizes are left out of
// its menu, the colours kept.
func TestTheSizesAreOnlyOfferedToPictures(t *testing.T) {
	menu := func(h *harness) string {
		h.m.setPromptText("")
		h.m.jumpToGroup("Tool")
		var names []string
		for _, o := range h.m.results {
			if o.cmd != nil {
				names = append(names, o.cmd.name)
			}
		}
		return " " + strings.Join(names, " ") + " "
	}
	cells := menu(newHarness(t, "theme = Dracula\n"))
	if !strings.Contains(cells, " accent ") || !strings.Contains(cells, " uicontrast ") {
		t.Fatalf("the colours should be in the text menu: %s", cells)
	}
	if strings.Contains(cells, " uisize ") || strings.Contains(cells, " cards ") {
		t.Fatalf("the sizes should not be in the text menu: %s", cells)
	}
	pics := menu(newPixHarness(t, "midnight"))
	if !strings.Contains(pics, " uisize ") || !strings.Contains(pics, " cards ") {
		t.Fatalf("the sizes should be in the picture menu: %s", pics)
	}
}

// The part of the Tool section that holds the look, and its lists, at both
// sizes and on a light scheme.
func TestInterfaceLookScreens(t *testing.T) {
	for _, iface := range []string{"midnight", "paper"} {
		for _, size := range [][2]int{{120, 34}, {80, 24}} {
			h := newPixHarness(t, iface)
			h.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			name := iface + "-" + itoa(size[0]) + "x" + itoa(size[1])
			h.m.jumpToGroup("Tool")
			h.highlightCommand("accent")
			h.shot("look-tool-" + name)
			h.m.setPromptText("Accent ")
			h.key("down", "down", "down")
			h.shot("look-accent-list-" + name)
			h.key("esc")
			h.m.state.Accent, h.m.state.Contrast, h.m.state.UISize = "teal", "high", "large"
			h.m.jumpToGroup("Window")
			h.highlightCommand("opacity")
			h.shot("look-teal-high-large-" + name)
		}
	}
}

// The colours of the look in the text interface: `tools/shot.sh <dir>
// TestCellLookFrames` renders them.
func TestCellLookFrames(t *testing.T) {
	for _, size := range [][2]int{{120, 34}, {80, 24}} {
		for _, iface := range []string{"midnight", "paper"} {
			h := newHarness(t, "theme = Dracula\n")
			h.m.state.Interface = iface
			h.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			name := "cells-" + iface + "-" + itoa(size[0]) + "x" + itoa(size[1])
			h.m.setPromptText("Accent ")
			h.key("down", "down", "down", "down")
			if h.m.state.Accent != "teal" {
				t.Fatalf("the fourth accent down should be teal, is %q", h.m.state.Accent)
			}
			h.dump(name + "-accent-list")
			h.key("enter")
			h.m.state.Contrast = "high"
			h.m.setPromptText("")
			h.m.jumpToGroup("Window")
			h.key("down", "down")
			h.dump(name + "-teal-high")
		}
	}
}
