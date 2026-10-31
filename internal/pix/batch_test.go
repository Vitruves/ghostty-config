package pix

import (
	"bytes"
	"image/color"
	"testing"
)

// TestBatchDrawsAsInOrder draws a wall of cards frame after frame, once in
// order on a canvas and once through a Batch, with a highlighted card whose
// shadow reaches over its neighbours, titles drawn between the cards, a card
// that draws past its bounds and one under a title, and wants the same
// pixels every frame.
func TestBatchDrawsAsInOrder(t *testing.T) {
	direct := New(900, 260, 1.5)
	batched := New(900, 260, 1.5)
	mm := NewMemo()
	title := func(c *Canvas, y float32, s string) {
		c.Text(c.Face(false, true, 14), 6, y, s, color.NRGBA{220, 220, 220, 255})
	}
	type spot struct {
		x, y float32
		key  int
	}
	frames := [][]int{
		{1, 2, 3, 4, 5, 6, 7, 8},
		{1, 2, 3, 4, 5, 6, 7, 8},
		{9, 10, 11, 12, 13, 14, 15, 16},
		{9, 10, 11, 12, 13, 14, 15, 16},
		{1, 2, 3, 4, 5, 6, 7, 8},
	}
	for f, keys := range frames {
		sel := f % 3
		var spots []spot
		for i, k := range keys {
			spots = append(spots, spot{x: 8 + float32(i%4)*95, y: 20 + float32(i/4)*80, key: k})
		}
		drawCard := func(c *Canvas, s spot, i int) {
			if i == sel {
				c.Shadow(s.x, s.y, 80, 50, 8, 10, 4, 0.4)
				c.Ring(s.x-3, s.y-3, 86, 56, 10, 2, color.NRGBA{120, 160, 250, 255})
			}
			card(c, s.x, s.y)
			if s.key%7 == 0 {
				// Past its bounds, to the right.
				c.Rect(s.x+82, s.y+20, 30, 4, 0, color.NRGBA{250, 80, 80, 255})
			}
		}
		bounds := func(s spot, i int) (x, y, w, h float32) {
			if i == sel {
				return s.x - 16, s.y - 8, 112, 74
			}
			return s.x - 2, s.y - 2, 84, 54
		}
		for _, c := range []*Canvas{direct, batched} {
			c.Clear(color.NRGBA{12, 14, 18, 255})
			c.Clip(0, 4, 600, 170)
		}
		mm.Frame()
		b := mm.Batch(batched)
		for i, s := range spots {
			if i == 4 {
				// A title between the rows, reaching under the card
				// beside it on odd frames.
				ty := s.y - 4
				if f%2 == 1 {
					ty = s.y + 30
				}
				title(direct, ty, "Second row")
				b.Do(func() { title(batched, ty, "Second row") })
			}
			drawCard(direct, s, i)
			x, y, w, h := bounds(s, i)
			key := [2]int{s.key, boolInt(i == sel)}
			b.Draw(key, x, y, w, h, func(c *Canvas) { drawCard(c, s, i) })
		}
		b.Run()
		for _, c := range []*Canvas{direct, batched} {
			c.Unclip()
		}
		if !bytes.Equal(direct.Img.Pix, batched.Img.Pix) {
			t.Fatalf("frame %d: the batch's pixels differ from drawing in order", f)
		}
	}
	if mm.Hits == 0 || mm.Misses == 0 || mm.Refused == 0 {
		t.Fatalf("hits %d misses %d refused %d: the test should copy back, draw and refuse", mm.Hits, mm.Misses, mm.Refused)
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
