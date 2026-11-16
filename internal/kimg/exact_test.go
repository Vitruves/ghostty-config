package kimg

import (
	"bytes"
	"image/color"
	"math"
	"testing"
)

// The pictures are drawn by shortcuts: the middle of a shape is not
// measured, a channel of one value is not blurred, the rows of a blur are
// shared out. These tests draw the same things the long way, a pixel and a
// channel at a time, and want the same bytes.

// slowDist is the distance to a rounded rectangle with no case set apart.
func slowDist(px, py, x, y, w, h, r float64) float64 {
	cx, cy := x+w/2, y+h/2
	qx := math.Abs(px-cx) - (w/2 - r)
	qy := math.Abs(py-cy) - (h/2 - r)
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
}

// slowOver composites one pixel with no case set apart.
func slowOver(c *Canvas, x, y int, src color.NRGBA, cov float64) {
	i := c.Img.PixOffset(x, y)
	pix := c.Img.Pix
	sa := float64(src.A) / 255 * cov
	if sa <= 0 {
		return
	}
	pix[i+0] = uint8(math.Min(255, float64(src.R)*sa+float64(pix[i+0])*(1-sa)))
	pix[i+1] = uint8(math.Min(255, float64(src.G)*sa+float64(pix[i+1])*(1-sa)))
	pix[i+2] = uint8(math.Min(255, float64(src.B)*sa+float64(pix[i+2])*(1-sa)))
	pix[i+3] = uint8(math.Min(255, (sa+float64(pix[i+3])/255*(1-sa))*255))
}

// slowShape fills a rounded rectangle, or strokes it when width is not 0,
// measuring every pixel of the canvas.
func slowShape(c *Canvas, x, y, w, h, r, width float64, col color.NRGBA) {
	b := c.Img.Bounds()
	for py := 0; py < b.Dy(); py++ {
		for px := 0; px < b.Dx(); px++ {
			d := slowDist((float64(px)+0.5)/c.S, (float64(py)+0.5)/c.S, x, y, w, h, r) * c.S
			cov := math.Min(math.Max(0.5-d, 0), 1)
			if width != 0 {
				cov -= math.Min(math.Max(0.5-(d+width*c.S), 0), 1)
			}
			if cov > 0 {
				slowOver(c, px, py, col, cov)
			}
		}
	}
}

// slowBlur is three box passes, a row or a column of a channel at a time.
func slowBlur(c *Canvas, radius float64) {
	r := int(math.Round(radius * c.S / 1.7))
	if r < 1 {
		return
	}
	w, h := c.Img.Bounds().Dx(), c.Img.Bounds().Dy()
	a, b := make([]float64, w*h*4), make([]float64, w*h*4)
	for i, v := range c.Img.Pix {
		a[i] = float64(v)
	}
	// line blurs the count numbers from first, step apart.
	line := func(src, dst []float64, first, step, count int) {
		at := func(i int) float64 { return src[first+min(max(i, 0), count-1)*step] }
		var sum float64
		for i := -r; i <= r; i++ {
			sum += at(i)
		}
		for i := 0; i < count; i++ {
			dst[first+i*step] = sum / float64(2*r+1)
			sum += at(i+r+1) - at(i-r)
		}
	}
	for pass := 0; pass < 3; pass++ {
		for y := 0; y < h; y++ {
			for ch := 0; ch < 4; ch++ {
				line(a, b, y*w*4+ch, 4, w)
			}
		}
		for x := 0; x < w; x++ {
			for ch := 0; ch < 4; ch++ {
				line(b, a, x*4+ch, w*4, h)
			}
		}
	}
	for i := range c.Img.Pix {
		c.Img.Pix[i] = uint8(math.Min(255, math.Max(0, a[i]+0.5)))
	}
}

// noise fills a canvas with pixels that follow no pattern, premultiplied
// as the canvas keeps them.
func noise(c *Canvas, seed uint32) {
	for i := 0; i < len(c.Img.Pix); i += 4 {
		seed = seed*1664525 + 1013904223
		a := uint8(seed >> 24)
		c.Img.Pix[i+3] = a
		for k := 0; k < 3; k++ {
			seed = seed*1664525 + 1013904223
			c.Img.Pix[i+k] = uint8(uint32(seed>>24) * uint32(a) / 255)
		}
	}
}

func TestShapesAreDrawnAsMeasuringEveryPixelWould(t *testing.T) {
	shapes := [][5]float64{{12, 8, 96, 60, 11}, {3.3, 4.7, 80.2, 31.9, 0}, {20, 10, 40, 40, 20}, {-9, -6, 70, 50, 6.5}, {60, 40, 90, 70, 14}, {30, 30, 2, 13, 1}, {5, 5, 110, 3, 1.5}, {40, 20, 7, 1.2, 0}, {40, 20, 1.2, 13, 0}, {50, 50, 7, 13, 1.5}, {10, 60, 9, 21, 2}, {70, 10, 0.4, 0.4, 0}}
	for _, scale := range []float64{1, 1.8125, 2.34} {
		for n, sh := range shapes {
			for _, col := range []color.NRGBA{{12, 28, 49, 255}, {200, 120, 40, 140}} {
				for _, width := range []float64{0, 1, 2.2} {
					got, want := NewCanvas(int(120*scale), int(80*scale), scale), NewCanvas(int(120*scale), int(80*scale), scale)
					noise(got, uint32(n))
					noise(want, uint32(n))
					if width == 0 {
						got.Fill(sh[0], sh[1], sh[2], sh[3], sh[4], col)
					} else {
						got.Ring(sh[0], sh[1], sh[2], sh[3], sh[4], width, col)
					}
					slowShape(want, sh[0], sh[1], sh[2], sh[3], sh[4], width, col)
					if !bytes.Equal(got.Img.Pix, want.Img.Pix) {
						t.Fatalf("shape %v at scale %v, width %v, colour %v: not the pixels measuring each one gives", sh, scale, width, col)
					}
				}
			}
		}
	}
}

func TestBlurIsTheBlurOfEveryChannelAlone(t *testing.T) {
	for _, size := range [][2]int{{97, 61}, {7, 300}, {300, 5}, {260, 200}, {1, 1}} {
		for _, radius := range []float64{0.9, 2, 13, 120} {
			for kind := 0; kind < 3; kind++ {
				got, want := NewCanvas(size[0], size[1], 1.8125), NewCanvas(size[0], size[1], 1.8125)
				switch kind {
				case 0:
					noise(got, 7)
				case 1:
					// One colour, as a shadow is: only its alpha varies.
					got.Fill(4, 3, 30, 20, 5, color.NRGBA{0, 0, 0, 140})
				case 2:
					got.Circle(20, 15, 12, color.NRGBA{122, 162, 247, 255})
				}
				copy(want.Img.Pix, got.Img.Pix)
				got.Blur(radius)
				slowBlur(want, radius)
				if !bytes.Equal(got.Img.Pix, want.Img.Pix) {
					t.Fatalf("%dx%d blurred by %v, content %d: not the pixels blurring each channel alone gives", size[0], size[1], radius, kind)
				}
			}
		}
	}
}

// A layer kept is the layer drawn: the second picture of a window is the
// first one's bytes, and a picture that shares its shadow with another is
// what it would be alone.
func TestKeptLayersChangeNoPicture(t *testing.T) {
	forget := func() {
		layers.Lock()
		layers.list = nil
		layers.Unlock()
	}
	spec := WindowSpec{BG: "#0c1c31", FG: "#d6dae0", Cursor: "#e2cb9c", Pal: testPal(), Screen: "#101014", Behind: [2]string{"#7aa2f7", "#e0af68"}, Titlebar: "tabs", Shadow: true, Opacity: 0.85, Blur: true, PadX: 14, PadY: 12}
	other := spec
	other.Opacity, other.PadX, other.Titlebar = 0.6, 3, "native"
	forget()
	alone := Window(40, 9, 14.5, 29, other).Img.Pix
	forget()
	first := Window(40, 9, 14.5, 29, spec).Img.Pix
	if again := Window(40, 9, 14.5, 29, spec).Img.Pix; !bytes.Equal(first, again) {
		t.Fatal("the same window drawn twice differs")
	}
	if shared := Window(40, 9, 14.5, 29, other).Img.Pix; !bytes.Equal(alone, shared) {
		t.Fatal("a window drawn over kept layers differs from the one drawn alone")
	}
	// Other colours behind, another blur, another size: other layers.
	for _, change := range []func(*WindowSpec){
		func(s *WindowSpec) { s.Behind[0] = "#ff0000" },
		func(s *WindowSpec) { s.Blur = false },
		func(s *WindowSpec) { s.Screen = "#f4f5f7" },
	} {
		s := spec
		change(&s)
		warm := Window(40, 9, 14.5, 29, s).Img.Pix
		forget()
		if cold := Window(40, 9, 14.5, 29, s).Img.Pix; !bytes.Equal(warm, cold) {
			t.Fatal("a kept layer was used for a picture it does not belong to")
		}
	}
	for i := 0; i < 3*maxLayers; i++ {
		c := NewCanvas(60, 40, 1)
		c.Shadow(10, 10, 20+float64(i), 12, 3, 2, 1, color.NRGBA{0, 0, 0, 128})
	}
	if n := len(layers.list); n != maxLayers {
		t.Fatalf("%d layers kept, want the last %d", n, maxLayers)
	}
}

// BenchmarkWindow draws the picture of the window at the size the settings
// show it: with its blurred layers kept from the last picture, as when a
// setting is stepped, and from nothing.
func BenchmarkWindow(b *testing.B) {
	spec := WindowSpec{BG: "#0c1c31", FG: "#d6dae0", Cursor: "#e2cb9c", Pal: testPal(), Screen: "#101014", Behind: [2]string{"#7aa2f7", "#e0af68"}, Titlebar: "tabs", Shadow: true, Opacity: 0.85, Blur: true, PadX: 14, PadY: 12}
	for _, kept := range []bool{true, false} {
		name := "stepped"
		if !kept {
			name = "first"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if !kept {
					layers.Lock()
					layers.list = nil
					layers.Unlock()
				}
				Window(40, 12, 18.125, 36.25, spec)
			}
		})
	}
}
