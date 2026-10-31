package tui

import (
	"bytes"
	"io"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The benchmarks time what a person feels: the first frame, a key that
// moves the highlight, a letter typed into the search. They draw with the
// picture interface, the default, on a wall of every theme the Ghostty on
// this machine ships when there is one, as people see it.

// newBenchHarness is the picture interface on a cols by rows terminal.
func newBenchHarness(b *testing.B, cols, rows int) *harness {
	b.Helper()
	h := newHarnessWith(b, pixConfig, Options{NoReload: true, Images: true, ImgOut: io.Discard, Pixel: true, PixCell: [2]int{17, 38}})
	if lib := ghostty.LoadLibrary(ghostty.DefaultPaths("", "")); len(lib.Themes) > len(h.m.lib.Themes) {
		h.m.lib = lib
	}
	h.m.state.Interface = "midnight"
	h.send(tea.WindowSizeMsg{Width: cols, Height: rows})
	h.m.imgReady = true
	h.m.setPromptText("Theme ")
	h.pxFrame()
	return h
}

// pxFrame draws a frame and sends the pictures it queued, as View does: the
// queue left to grow would be timed and kept as well.
func (h *harness) pxFrame() {
	h.m.render()
	h.m.imgs.flush()
}

// BenchmarkFirstFrame draws every row of the themes wall from nothing, as
// at startup or after the cell size changes.
func BenchmarkFirstFrame(b *testing.B) {
	for _, size := range []struct {
		name       string
		cols, rows int
	}{{"120x34", 120, 34}, {"240x68", 240, 68}} {
		b.Run(size.name, func(b *testing.B) {
			h := newBenchHarness(b, size.cols, size.rows)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h.m.imgs.dropAll()
				h.m.px.forget()
				h.pxFrame()
			}
		})
	}
}

// BenchmarkSameFrame draws a frame that shows nothing new: the cost of a
// key that changes nothing on screen.
func BenchmarkSameFrame(b *testing.B) {
	for _, screen := range []string{"Theme ", "Window"} {
		b.Run(screen, func(b *testing.B) {
			h := newBenchHarness(b, 120, 34)
			if screen == "Window" {
				h.m.setPromptText("")
				h.m.jumpToGroup("Window")
			}
			h.pxFrame()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h.pxFrame()
			}
		})
	}
}

// BenchmarkMove moves the highlight one card along the wall and back, or
// one setting down the Window section and back, a frame drawn after each.
func BenchmarkMove(b *testing.B) {
	for _, screen := range []string{"Theme ", "Window"} {
		b.Run(screen, func(b *testing.B) {
			h := newBenchHarness(b, 120, 34)
			there, back := "right", "left"
			if screen == "Window" {
				h.m.setPromptText("")
				h.m.jumpToGroup("Window")
				there, back = "down", "up"
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h.key(there)
				h.pxFrame()
				h.key(back)
				h.pxFrame()
			}
		})
	}
}

// BenchmarkTypeSearch types a letter into the themes' search and takes it
// back, a frame drawn after each.
func BenchmarkTypeSearch(b *testing.B) {
	h := newBenchHarness(b, 120, 34)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.key("s")
		h.pxFrame()
		h.key("backspace")
		h.pxFrame()
	}
}

// BenchmarkCellWall moves the highlight along the themes wall and back in
// the text interface, for a terminal that shows no pictures.
func BenchmarkCellWall(b *testing.B) {
	h := newHarnessWith(b, pixConfig, Options{NoReload: true})
	if lib := ghostty.LoadLibrary(ghostty.DefaultPaths("", "")); len(lib.Themes) > len(h.m.lib.Themes) {
		h.m.lib = lib
	}
	h.send(tea.WindowSizeMsg{Width: 120, Height: 34})
	h.m.setPromptText("Theme ")
	h.m.render()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.key("right")
		h.m.render()
		h.key("left")
		h.m.render()
	}
}

// BenchmarkNew builds the model, as the editor does before its first frame.
func BenchmarkNew(b *testing.B) {
	h := newBenchHarness(b, 120, 34)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		New(h.paths, h.m.tree, h.m.lib, h.m.state, h.m.opts)
	}
}

// TestTheWallIsCopiedBack moves the highlight along the wall: the cards it
// does not touch are copied from the last frame, and the frame is the one
// drawing every card would make.
func TestTheWallIsCopiedBack(t *testing.T) {
	for _, iface := range []string{"midnight", "paper"} {
		h := newPixHarness(t, iface)
		h.m.imgReady = true
		h.m.setPromptText("Theme ")
		h.m.state.Favorites = append(h.m.state.Favorites, h.m.lib.Themes[3].Name)
		h.m.render()
		mm := h.m.px.cards
		for _, k := range []string{"right", "right", "down", "left"} {
			misses := mm.Misses
			h.key(k)
			h.m.render()
			if n := mm.Misses - misses; n > 2 {
				t.Errorf("%s: %s drew %d cards again, want the two it left and landed on", iface, k, n)
			}
			if mm.Refused > 0 {
				t.Fatalf("%s: a card was drawn past its bounds", iface)
			}
			got := h.m.px.canvas.Img.Pix
			mm.Reset()
			c := h.m.pixCanvas()
			h.m.drawScreen(c)
			if !bytes.Equal(got, c.Img.Pix) {
				t.Fatalf("%s: after %s the frame differs from one drawn afresh", iface, k)
			}
			// Put the frame back, as the memo had it.
			h.m.render()
		}
	}
}
