package pix

import (
	"bytes"
	"image/color"
	"testing"

	"github.com/vitruves/ghostty-config/internal/kimg"
)

// card is a drawing with rounded corners, a ring and text, the kind a memo
// keeps.
func card(c *Canvas, x, y float32) {
	c.Rect(x, y, 80, 50, 8, color.NRGBA{40, 60, 90, 255})
	c.Ring(x, y, 80, 50, 8, 1, color.NRGBA{255, 255, 255, 30})
	c.Text(c.Face(false, true, 13), x+6, y+30, "Aa ›", color.NRGBA{230, 230, 230, 255})
}

func TestMemoCopiesBackWhatDrawingMakes(t *testing.T) {
	direct := New(450, 120, 1.7)
	memo := New(450, 120, 1.7)
	mm := NewMemo()
	for frame := 0; frame < 3; frame++ {
		for _, c := range []*Canvas{direct, memo} {
			c.Clear(color.NRGBA{10, 10, 10, 255})
			// A shadow under the second card on the middle frame changes
			// what is under it, so that card is drawn again, then again
			// on the last frame: what the first frame drew is let go.
			if frame == 1 {
				c.Shadow(150, 10, 80, 50, 8, 10, 4, 0.4)
			}
		}
		mm.Frame()
		for i, x := range []float32{5, 150} {
			card(direct, x, 10)
			mm.Draw(memo, i, x-2, 8, 84, 54, func() { card(memo, x, 10) })
		}
		if !bytes.Equal(direct.Img.Pix, memo.Img.Pix) {
			t.Fatalf("frame %d: the memo's pixels differ from drawing", frame)
		}
	}
	if mm.Hits != 2 || mm.Misses != 4 || mm.Refused != 0 {
		t.Fatalf("hits %d misses %d refused %d, want 2 4 0", mm.Hits, mm.Misses, mm.Refused)
	}
}

func TestMemoDoesNotKeepADrawingPastItsBounds(t *testing.T) {
	direct := New(200, 100, 1)
	memo := New(200, 100, 1)
	mm := NewMemo()
	for frame := 0; frame < 2; frame++ {
		mm.Frame()
		card(direct, 10, 10)
		// The bounds given are too small: the card is drawn every time.
		mm.Draw(memo, "card", 10, 10, 40, 20, func() { card(memo, 10, 10) })
		if !bytes.Equal(direct.Img.Pix, memo.Img.Pix) {
			t.Fatalf("frame %d: drawing past the bounds lost pixels", frame)
		}
	}
	if mm.Hits != 0 || mm.Refused != 2 {
		t.Fatalf("hits %d refused %d, want 0 2", mm.Hits, mm.Refused)
	}
}

func TestRenewIsBlank(t *testing.T) {
	c := New(40, 20, 1)
	c.Clear(color.NRGBA{1, 2, 3, 255})
	c.Clip(0, 0, 5, 5)
	r := Renew(c, 40, 20, 2)
	if r != c || r.S != 2 || r.clip != r.Img.Bounds() {
		t.Fatal("Renew did not reuse the canvas as a new one")
	}
	if !bytes.Equal(r.Img.Pix, New(40, 20, 2).Img.Pix) {
		t.Fatal("a renewed canvas is not blank")
	}
	if Renew(c, 41, 20, 1) == c {
		t.Fatal("a canvas of another size was reused")
	}
}

func TestCachedGlyphsDrawTheSame(t *testing.T) {
	// The faces kimg hands out keep their glyphs; a face that does not is
	// the reference.
	plain := New(400, 60, 1.37)
	cached := New(400, 60, 1.37)
	f := cached.Face(false, false, 13)
	ref := kimg.UncachedFace(false, false, float64(13*cached.S))
	for i := 0; i < 2; i++ {
		for _, x := range []float32{3, 3.3, 7.71} {
			plain.Text(ref, x, 20+x, "Theme · AAA 12.5 ›", color.NRGBA{200, 220, 240, 255})
			cached.Text(f, x, 20+x, "Theme · AAA 12.5 ›", color.NRGBA{200, 220, 240, 255})
		}
	}
	if !bytes.Equal(plain.Img.Pix, cached.Img.Pix) {
		t.Fatal("text drawn from kept glyphs differs")
	}
}
