package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func imageHarness(t *testing.T, config string, w, hgt int) (*harness, *strings.Builder) {
	t.Helper()
	var sent strings.Builder
	h := newHarnessWith(t, config, Options{NoReload: true, Images: true, CellW: 20, CellH: 42, ImgOut: &sent})
	if dir := os.Getenv("GHOSTTY_CONFIG_DUMP"); dir != "" {
		h.m.imgs.dumpDir = filepath.Join(dir, "img")
	}
	h.send(tea.WindowSizeMsg{Width: w, Height: hgt})
	return h, &sent
}

// With pictures on, the highlighted theme is shown as a card made of
// placeholder cells beside its details, and the picture is sent once.
func TestThemeDetailsShowAPictureWhenTheTerminalCan(t *testing.T) {
	h, sent := imageHarness(t, "theme = Dracula\n", 130, 38)
	h.m.setPromptText("Theme ")
	frame := h.m.render()
	if !strings.Contains(frame, "\U0010EEEE") {
		t.Fatal("the highlighted theme should be drawn as a picture")
	}
	for i, l := range strings.Split(frame, "\n") {
		if w := ansi.StringWidth(l); w != 130 {
			t.Fatalf("line %d is %d cells wide, want 130", i, w)
		}
	}
	h.m.imgs.flush()
	if !strings.Contains(sent.String(), "a=T,f=100,U=1,") {
		t.Fatal("the picture should have been sent to the terminal")
	}
	sent.Reset()
	h.m.render()
	h.m.imgs.flush()
	if sent.Len() != 0 {
		t.Fatalf("an unchanged frame resent %d bytes", sent.Len())
	}
	h.dump("pic-theme")
	// Without pictures the same frame has none.
	plain := newHarness(t, "theme = Dracula\n")
	plain.m.setPromptText("Theme ")
	if strings.Contains(plain.m.render(), "\U0010EEEE") {
		t.Fatal("no placeholders without pictures")
	}
	// Other commands carry no picture.
	h.m.setPromptText("Window")
	h.key("tab")
	if strings.Contains(h.m.render(), "\U0010EEEE") {
		t.Fatal("only themes are pictures")
	}
}

// TestPaletteFrames draws the palette in its default interface, for a look
// with tools/shot.sh.
func TestPaletteFrames(t *testing.T) {
	h, _ := imageHarness(t, "theme = dusk-synth\n", 130, 38)
	h.m.state.Interface = ""
	h.m.setPromptText("Theme ")
	h.key("down", "down")
	h.dump("pal-theme")
	h.m.setPromptText("")
	h.m.group = 2
	h.m.recompute()
	h.dump("pal-window")
	h.m.setPromptText("Preset glass")
	h.dump("pal-preset")
	h.m.setPromptText("Edit blue")
	h.key("enter")
	h.dump("pal-edit")
}
