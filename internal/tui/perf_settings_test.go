package tui

import (
	"bytes"
	"image"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The benchmarks of the settings screens, Fonts to Tool: a frame of a
// section, a key that moves down it, a value stepped where it is listed.
// Each part of a key is timed alone as well, to say which one a slow key
// owes its time to.

// settingSections are the sections timed: the longest, and one of switches.
var settingSections = []string{"Window", "Session"}

// newSettingsBench is the picture interface on a section of the menu.
func newSettingsBench(b *testing.B, section string) *harness {
	b.Helper()
	h := newBenchHarness(b, 120, 34)
	h.m.jumpToGroup(section)
	h.pxFrame()
	return h
}

// onSetting puts the highlight on a setting of the section shown.
func (h *harness) onSetting(b *testing.B, name string) {
	b.Helper()
	for i := range h.m.results {
		if h.m.results[i].value == name {
			h.m.sel.cursor = i
			return
		}
	}
	b.Fatalf("no %s in the section", name)
}

// BenchmarkSettingsFrame draws a section that shows nothing new.
func BenchmarkSettingsFrame(b *testing.B) {
	for _, section := range settingSections {
		b.Run(section, func(b *testing.B) {
			h := newSettingsBench(b, section)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h.pxFrame()
			}
		})
	}
}

// BenchmarkSettingsDraw is the drawing of that frame alone, without the
// rows being hashed and sent.
func BenchmarkSettingsDraw(b *testing.B) {
	for _, section := range settingSections {
		b.Run(section, func(b *testing.B) {
			h := newSettingsBench(b, section)
			c := h.m.px.canvas
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h.m.regions = h.m.regions[:0]
				h.m.drawScreen(c)
			}
		})
	}
}

// BenchmarkSettingsMove moves one setting down and back, a frame after each.
func BenchmarkSettingsMove(b *testing.B) {
	for _, section := range settingSections {
		b.Run(section, func(b *testing.B) {
			h := newSettingsBench(b, section)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h.key("down")
				h.pxFrame()
				h.key("up")
				h.pxFrame()
			}
		})
	}
}

// BenchmarkSettingsMovePicture moves between two settings of the window,
// whose picture stays on screen from one frame to the next.
func BenchmarkSettingsMovePicture(b *testing.B) {
	h := newSettingsBench(b, "Window")
	h.onSetting(b, "opacity")
	h.pxFrame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.key("down")
		h.pxFrame()
		h.key("up")
		h.pxFrame()
	}
}

// BenchmarkSettingsMoveKey is the two keys of that move, no frame drawn.
func BenchmarkSettingsMoveKey(b *testing.B) {
	h := newSettingsBench(b, "Window")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.key("down")
		h.key("up")
	}
}

// BenchmarkSettingsRecompute rebuilds the rows of a section, as every
// change of a value does.
func BenchmarkSettingsRecompute(b *testing.B) {
	for _, section := range settingSections {
		b.Run(section, func(b *testing.B) {
			h := newSettingsBench(b, section)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h.m.recompute()
			}
		})
	}
}

// BenchmarkSettingsRows asks every row of a section what it is set to and
// whether that is Ghostty's default, as a frame does.
func BenchmarkSettingsRows(b *testing.B) {
	h := newSettingsBench(b, "Window")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range h.m.results {
			if c := h.m.results[j].cmd; c != nil {
				h.m.listed(c)
				h.m.currentValueOf(c)
				if c.set != nil {
					c.set.get(h.m)
					h.m.changed(c.set)
				}
			}
		}
	}
}

// BenchmarkSettingsStep steps the opacity down and back up with ← and →,
// each step written to the config; with a frame after each, and without.
func BenchmarkSettingsStep(b *testing.B) {
	for _, frame := range []bool{false, true} {
		name := "key"
		if frame {
			name = "key+frame"
		}
		b.Run(name, func(b *testing.B) {
			h := newSettingsBench(b, "Window")
			h.onSetting(b, "opacity")
			h.pxFrame()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h.key("left")
				if frame {
					h.pxFrame()
				}
				h.key("right")
				if frame {
					h.pxFrame()
				}
			}
		})
	}
}

// BenchmarkSettingsStepUpdate is the part of a step the interface waits
// for: Update alone, the commands it returns left unrun.
func BenchmarkSettingsStepUpdate(b *testing.B) {
	h := newSettingsBench(b, "Window")
	h.onSetting(b, "opacity")
	left, right := tea.Msg(keyPress("left")), tea.Msg(keyPress("right"))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.m.Update(left)
		h.m.Update(right)
	}
}

// BenchmarkSettingsOpen draws an opened setting, the list of its values,
// and moves down it and back.
func BenchmarkSettingsOpen(b *testing.B) {
	h := newSettingsBench(b, "Window")
	h.m.setPromptText("Opacity ")
	h.pxFrame()
	b.Run("frame", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			h.pxFrame()
		}
	})
	b.Run("move", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			h.key("down")
			h.pxFrame()
			h.key("up")
			h.pxFrame()
		}
	})
}

// BenchmarkSettingsNewPicture draws the frame that follows a step of a
// setting the picture of the window shows, at a value it was not drawn for
// yet: the picture is drawn anew, as it is at every new step of the
// opacity or the padding.
func BenchmarkSettingsNewPicture(b *testing.B) {
	h := newSettingsBench(b, "Window")
	h.onSetting(b, "opacity")
	h.pxFrame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.m.px.pics = map[string]*image.RGBA{}
		h.pxFrame()
	}
}

// BenchmarkSettingsMoveSent is the frame after a move down the section the
// first time it is made, when the terminal holds none of the rows that
// changed: they are compressed and queued, a dozen of them, which a move
// back and forth over rows already sent never times.
func BenchmarkSettingsMoveSent(b *testing.B) {
	for _, section := range settingSections {
		b.Run(section, func(b *testing.B) {
			h := newSettingsBench(b, section)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				h.key("up")
				h.m.imgs.dropAll()
				h.pxFrame()
				h.key("down")
				b.StartTimer()
				h.pxFrame()
			}
		})
	}
}

// TestTheWindowPictureIsCopiedBack draws the settings of the window frame
// after frame: the picture on its ground is copied from the last frame
// while it stays the same, and every frame is the one drawing it all would
// make, on a menu, in an opened setting and after a value is stepped.
func TestTheWindowPictureIsCopiedBack(t *testing.T) {
	for _, iface := range []string{"midnight", "paper"} {
		h := newPixHarness(t, iface)
		h.m.imgReady = true
		h.m.jumpToGroup("Window")
		for i := range h.m.results {
			if h.m.results[i].value == "opacity" {
				h.m.sel.cursor = i
			}
		}
		h.m.render()
		mm := h.m.px.cards
		for _, k := range []string{"down", "down", "left", "", "right", "up", "enter", "down", "", "esc"} {
			hits := mm.Hits
			if k != "" {
				h.key(k)
			}
			h.m.render()
			if k == "" && mm.Hits == hits {
				t.Errorf("%s: a frame that shows the same picture drew it again", iface)
			}
			if mm.Refused > 0 {
				t.Fatalf("%s: the picture was drawn past its bounds", iface)
			}
			got := bytes.Clone(h.m.px.canvas.Img.Pix)
			mm.Reset()
			c := h.m.pixCanvas()
			h.m.drawScreen(c)
			if !bytes.Equal(got, c.Img.Pix) {
				t.Fatalf("%s: after %q the frame differs from one drawn afresh", iface, k)
			}
			// Put the frame back, as the memo had it.
			h.m.render()
		}
	}
}
