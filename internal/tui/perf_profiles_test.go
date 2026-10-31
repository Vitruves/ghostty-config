package tui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The benchmarks of Palette and Guided, and of Atelier drawn in cells: a key
// as a person presses it, then the frame it leads to, on every theme the
// Ghostty on this machine ships when there is one.

// newProfileBench opens the editor in a profile, in pictures or in cells.
func newProfileBench(b *testing.B, profile string, pixel bool) *harness {
	b.Helper()
	return newProfileHarness(b, profile, pixel)
}

func newProfileHarness(b testing.TB, profile string, pixel bool) *harness {
	b.Helper()
	load, mac := loadKeyCatalog, keysMac
	loadKeyCatalog = func(string) ghostty.KeyCatalog { return ghostty.BuiltinKeyCatalog(true) }
	keysMac = true
	b.Cleanup(func() { loadKeyCatalog, keysMac = load, mac })
	was := harnessProfile
	harnessProfile = profile
	defer func() { harnessProfile = was }()
	opts := Options{NoReload: true}
	if pixel {
		opts = Options{NoReload: true, Images: true, ImgOut: io.Discard, Pixel: true, PixCell: [2]int{17, 38}}
	}
	h := newHarnessWith(b, pixConfig, opts)
	if lib := ghostty.LoadLibrary(ghostty.DefaultPaths("", "")); len(lib.Themes) > len(h.m.lib.Themes) {
		h.m.lib = lib
		h.m.cmds = nil
	}
	h.m.state.Interface = "midnight"
	h.send(tea.WindowSizeMsg{Width: 120, Height: 34})
	h.send(imgReadyMsg{h.m.imgSeq})
	h.m.imgReady = pixel
	h.m.render()
	return h
}

var renderers = []struct {
	name  string
	pixel bool
}{{"pixel", true}, {"cells", false}}

// BenchmarkPaletteType types "cursor" a letter at a time from an empty
// search line, a frame drawn after each, then clears it.
func BenchmarkPaletteType(b *testing.B) {
	for _, r := range renderers {
		b.Run(r.name, func(b *testing.B) {
			h := newProfileBench(b, ghostty.ProfilePalette, r.pixel)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, c := range "cursor" {
					h.key(string(c))
					h.m.render()
				}
				h.key("ctrl+u")
				h.m.render()
			}
		})
	}
}

// BenchmarkPaletteMove moves down the mixed results of a query and back.
func BenchmarkPaletteMove(b *testing.B) {
	for _, r := range renderers {
		b.Run(r.name, func(b *testing.B) {
			h := newProfileBench(b, ghostty.ProfilePalette, r.pixel)
			h.typeText("so")
			h.m.render()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for j := 0; j < 4; j++ {
					h.key("down")
					h.m.render()
				}
				for j := 0; j < 4; j++ {
					h.key("up")
					h.m.render()
				}
			}
		})
	}
}

// BenchmarkPaletteFilter walks the filters with Tab on a query, all the
// way round, and on nothing typed, where each kind is listed whole.
func BenchmarkPaletteFilter(b *testing.B) {
	for _, r := range renderers {
		b.Run(r.name, func(b *testing.B) {
			h := newProfileBench(b, ghostty.ProfilePalette, r.pixel)
			h.typeText("so")
			h.m.render()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for range palFilters {
					h.key("tab")
					h.m.render()
				}
			}
		})
		b.Run("empty/"+r.name, func(b *testing.B) {
			h := newProfileBench(b, ghostty.ProfilePalette, r.pixel)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for range palFilters {
					h.key("tab")
					h.m.render()
				}
			}
		})
	}
}

// BenchmarkGuidedMove moves between the answers of the theme cards and of
// the window's first page.
func BenchmarkGuidedMove(b *testing.B) {
	for _, page := range []struct {
		name       string
		step, page int
	}{{"cards", gColours, 2}, {"window", gWindow, 0}} {
		for _, r := range renderers {
			b.Run(page.name+"/"+r.name, func(b *testing.B) {
				h := newProfileBench(b, ghostty.ProfileGuided, r.pixel)
				g := h.guide()
				if cmd := g.goTo(h.m, page.step, page.page); cmd != nil {
					h.runCmd(cmd)
				}
				h.m.render()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					h.key("down")
					h.m.render()
					h.key("down")
					h.m.render()
					h.key("up")
					h.m.render()
					h.key("up")
					h.m.render()
				}
			})
		}
	}
}

// BenchmarkGuidedPages draws every page of every step, in order.
func BenchmarkGuidedPages(b *testing.B) {
	for _, r := range renderers {
		b.Run(r.name, func(b *testing.B) {
			h := newProfileBench(b, ghostty.ProfileGuided, r.pixel)
			g := h.guide()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for step := 0; step < gSteps; step++ {
					for page := range g.pages(h.m, step) {
						if cmd := g.goTo(h.m, step, page); cmd != nil {
							h.runCmd(cmd)
						}
						h.m.render()
					}
				}
			}
		})
	}
}

// BenchmarkAtelierCells types a letter into the themes' search and takes it
// back, then moves along the results, drawn in cells.
func BenchmarkAtelierCells(b *testing.B) {
	b.Run("type", func(b *testing.B) {
		h := newProfileBench(b, ghostty.ProfileAtelier, false)
		h.m.setPromptText("Theme ")
		h.m.render()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			h.key("s")
			h.m.render()
			h.key("backspace")
			h.m.render()
		}
	})
	b.Run("move", func(b *testing.B) {
		h := newProfileBench(b, ghostty.ProfileAtelier, false)
		h.m.setPromptText("Theme ")
		h.m.render()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			h.key("down")
			h.m.render()
			h.key("up")
			h.m.render()
		}
	})
}

// TestFramesRecord drives a scripted session in each profile and, when
// GHOSTTY_CONFIG_FRAMES names a directory, writes every frame there: the
// cells' strings as drawn, the pictures as a digest of their pixels. Two
// runs compare with diff, so a change made for speed is seen to draw the
// same.
func TestFramesRecord(t *testing.T) {
	dir := os.Getenv("GHOSTTY_CONFIG_FRAMES")
	if dir == "" {
		t.Skip("GHOSTTY_CONFIG_FRAMES is not set")
	}
	for _, r := range renderers {
		for _, profile := range []string{ghostty.ProfilePalette, ghostty.ProfileGuided, ghostty.ProfileAtelier} {
			var out bytes.Buffer
			h := newProfileHarness(t, profile, r.pixel)
			frame := func(what string) {
				s := h.m.render()
				out.WriteString("== " + what + "\n")
				if r.pixel {
					sum := sha256.Sum256(h.m.px.canvas.Img.Pix)
					out.WriteString(hex.EncodeToString(sum[:]) + "\n")
					// The placeholders name the image by a number of the run:
					// the pixels are the frame.
					return
				}
				out.WriteString(s + "\n")
			}
			keys := func(ks ...string) {
				for _, k := range ks {
					h.key(k)
					frame(k)
				}
			}
			switch profile {
			case ghostty.ProfilePalette:
				frame("home")
				keys("c", "u", "r", "s", "o", "r", "down", "down", "down", "up", "right", "tab", "tab", "down", "tab", "tab", "tab", "tab", "tab")
				keys("ctrl+u", "s", "o", "tab", "down", "down", "tab", "tab", "down", "enter", "down", "esc", "esc", "esc")
				keys("d", "a", "r", "k", "space", "m", "down", "down", "tab", "down")
				// Each kind whole, nothing typed.
				keys("ctrl+u", "tab", "down", "tab", "down", "tab", "pgdown", "down", "tab", "down", "tab", "tab", "down", "tab")
			case ghostty.ProfileGuided:
				g := h.guide()
				for step := 0; step < gSteps; step++ {
					for page := range g.pages(h.m, step) {
						if cmd := g.goTo(h.m, step, page); cmd != nil {
							h.runCmd(cmd)
						}
						frame("page " + itoa(step) + itoa(page))
						if step != gDone {
							keys("down", "down", "up", "tab", "down", "tab")
						}
					}
				}
				if cmd := g.goTo(h.m, gColours, 2); cmd != nil {
					h.runCmd(cmd)
				}
				keys("m", "down", "m", "up")
			case ghostty.ProfileAtelier:
				h.m.setPromptText("Theme ")
				frame("themes")
				keys("s", "o", "down", "right", "backspace", "backspace", "down", "down")
				h.m.setPromptText("")
				frame("menu")
				keys("w", "i", "n", "down", "down", "up")
			}
			os.MkdirAll(dir, 0o755)
			os.WriteFile(filepath.Join(dir, r.name+"-"+profile+".txt"), out.Bytes(), 0o644)
		}
	}
}
