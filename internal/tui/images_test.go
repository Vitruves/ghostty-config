package tui

import (
	stdcolor "image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/vitruves/ghostty-config/internal/color"
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

// TestPaletteFrames draws the screens in the default interface, in a dark
// terminal and in a light one, for a look with tools/shot.sh.
func TestPaletteFrames(t *testing.T) {
	for i, term := range []struct{ name, theme string }{{"dark", "sumi-ink"}, {"light", "neue-grafik"}} {
		h, _ := imageHarness(t, "theme = "+term.theme+"\n", 124, 38)
		h.m.state.Interface = ""
		// The two runs dump their pictures into one directory: keep their
		// numbers apart.
		h.m.imgs.next += uint32(i) * 0x4000
		// The terminal runs the theme its config names, and says so.
		applied, _ := h.m.lib.Get(term.theme)
		bg, _ := color.ParseHex(applied.Background())
		fg, _ := color.ParseHex(applied.Foreground())
		h.send(tea.BackgroundColorMsg{Color: stdcolor.RGBA{R: uint8(bg.R), G: uint8(bg.G), B: uint8(bg.B), A: 0xff}})
		h.send(tea.ForegroundColorMsg{Color: stdcolor.RGBA{R: uint8(fg.R), G: uint8(fg.G), B: uint8(fg.B), A: 0xff}})

		h.m.setPromptText("Theme ")
		h.key("right", "down")
		h.dump(term.name + "-1-wall")
		h.m.setPromptText("Theme catp")
		h.dump(term.name + "-2-search")
		h.m.setPromptText("")
		h.m.group = 2
		h.m.recompute()
		h.key("down", "down", "down")
		h.dump(term.name + "-3-window")
		h.m.setPromptText("Preset glass")
		h.key("enter", "esc")
		h.m.setPromptText("Titlebar ")
		h.key("down")
		h.dump(term.name + "-4-titlebar")
		h.m.setPromptText("Font ")
		h.dump(term.name + "-5-font")
		h.m.setPromptText("Shader ")
		h.key("down", "down")
		h.dump(term.name + "-9-shader")
		h.m.setPromptText("Edit blue")
		h.key("enter")
		h.dump(term.name + "-6-edit")
		h.key("right", "esc", "esc", "esc")
		h.dump(term.name + "-7-confirm")
		h.key("esc")
		h.send(keyPress("f1"))
		h.dump(term.name + "-8-help")
	}
	// The same wall where the terminal draws no pictures.
	h := newHarness(t, "theme = sumi-ink\n")
	h.m.state.Interface = ""
	h.send(tea.WindowSizeMsg{Width: 124, Height: 38})
	h.m.setPromptText("Theme ")
	h.dump("cells-1-wall")
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	h.dump("cells-2-narrow")
}

// A picture's number is carried as a 24-bit colour: the numbering wraps
// before it outgrows one, and passes over the numbers still held.
func TestPictureNumbersWrapWithinTheColour(t *testing.T) {
	s := newImageStore(nil)
	s.next = firstID
	held := s.ensure("held", 1, 1, func() []byte { return nil })
	s.next = lastID - 1
	seen := map[uint32]bool{held: true}
	for i := 0; i < 4; i++ {
		id := s.ensure(itoa(i), 1, 1, func() []byte { return nil })
		if id == 0 || id > lastID {
			t.Fatalf("picture %d got number %#x, outside the 24-bit colour", i, id)
		}
		if seen[id] {
			t.Fatalf("number %#x was given twice", id)
		}
		seen[id] = true
	}
	if !seen[firstID+1] {
		t.Fatalf("the numbering did not wrap to the start: %v", seen)
	}
}

// The store frees pictures by the room they take in the terminal, not only
// by their number, so that the terminal never reaches its own bound and
// frees rows still on screen; a picture of the frame being drawn stays.
func TestPicturesAreFreedByTheRoomTheyTake(t *testing.T) {
	var sent strings.Builder
	s := newImageStore(&sent)
	const w, h = 6000, 44 // one row of a full-screen 6K window: about 1 MB
	band := make([]byte, w*h*4)
	frame := func(first, n int) {
		for i := first; i < first+n; i++ {
			s.ensureRGBA(strconv.Itoa(i), band, w, h, 300, 1, nil)
		}
		s.flush()
	}
	for f := 0; f < 8; f++ {
		frame(f*80, 80)
		if s.bytes > maxImageBytes+80*len(band) {
			t.Fatalf("frame %d: the store holds %d MB", f, s.bytes>>20)
		}
	}
	if len(s.ids) >= 8*80 || !strings.Contains(sent.String(), "a=d,d=I") {
		t.Fatal("nothing was freed")
	}
	// The last frame is whole.
	for i := 7 * 80; i < 8*80; i++ {
		if _, ok := s.ids[strconv.Itoa(i)]; !ok {
			t.Fatalf("picture %d of the frame on screen was freed", i)
		}
	}
	total := 0
	for k := range s.ids {
		total += s.size[k]
	}
	if total != s.bytes {
		t.Fatalf("sizes add up to %d, the store counts %d", total, s.bytes)
	}
}
