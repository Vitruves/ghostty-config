package vector

import (
	"bytes"
	"image"
	"image/color"
	"math/rand"
	"testing"

	upstream "golang.org/x/image/vector"
)

// path is a shape drawn on both rasterisers alike.
type pather interface {
	MoveTo(x, y float32)
	LineTo(x, y float32)
	CubeTo(bx, by, cx, cy, dx, dy float32)
	ClosePath()
}

// roundRect winds a rounded rectangle, backwards to cut a hole.
func roundRect(z pather, x, y, w, h, r float32, reverse bool) {
	k := 0.5523 * r
	if !reverse {
		z.MoveTo(x+r, y)
		z.LineTo(x+w-r, y)
		z.CubeTo(x+w-r+k, y, x+w, y+r-k, x+w, y+r)
		z.LineTo(x+w, y+h-r)
		z.CubeTo(x+w, y+h-r+k, x+w-r+k, y+h, x+w-r, y+h)
		z.LineTo(x+r, y+h)
		z.CubeTo(x+r-k, y+h, x, y+h-r+k, x, y+h-r)
		z.LineTo(x, y+r)
		z.CubeTo(x, y+r-k, x+r-k, y, x+r, y)
	} else {
		z.MoveTo(x+r, y)
		z.CubeTo(x+r-k, y, x, y+r-k, x, y+r)
		z.LineTo(x, y+h-r)
		z.CubeTo(x, y+h-r+k, x+r-k, y+h, x+r, y+h)
		z.LineTo(x+w-r, y+h)
		z.CubeTo(x+w-r+k, y+h, x+w, y+h-r+k, x+w, y+h-r)
		z.LineTo(x+w, y+r)
		z.CubeTo(x+w, y+r-k, x+w-r+k, y, x+w-r, y)
	}
	z.ClosePath()
}

// TestSameAsUpstream draws rings, rounded rectangles and polygons, opaque
// and translucent, small enough for fixed point math and large enough for
// floating point, over noise, and wants every byte upstream's would make.
func TestSameAsUpstream(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	// One of each, reused, as the canvas reuses its own.
	var u upstream.Rasterizer
	var z Rasterizer
	for i := 0; i < 1000; i++ {
		w, h := 1+rng.Intn(300), 1+rng.Intn(120)
		if i%4 == 0 {
			w = 500 + rng.Intn(900)
		}
		dstW, dstH := w+rng.Intn(20), h+rng.Intn(20)
		bg := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
		rng.Read(bg.Pix)
		if i%2 == 0 {
			// Runs of one colour, as under most shapes of the interface.
			for j := 4; j < len(bg.Pix); j += 4 {
				if rng.Intn(40) != 0 {
					copy(bg.Pix[j:j+4], bg.Pix[j-4:j])
				}
			}
		}
		for j := 0; j < len(bg.Pix); j += 4 {
			// Premultiplied: no channel above alpha.
			for c := 0; c < 3; c++ {
				bg.Pix[j+c] = min(bg.Pix[j+c], bg.Pix[j+3])
			}
		}
		a := uint8(rng.Intn(256))
		if i%3 == 0 {
			a = 255
		}
		col := color.NRGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), a}
		x0, y0 := rng.Intn(dstW-w+1), rng.Intn(dstH-h+1)
		b := image.Rect(x0, y0, x0+w, y0+h)
		fx, fy := rng.Float32()*2, rng.Float32()*2
		fw, fh := float32(w)-fx-rng.Float32()*2, float32(h)-fy-rng.Float32()*2
		rr := rng.Float32() * min(fw, fh) / 2
		th := 0.5 + rng.Float32()*4
		kind := i % 3

		draw := func(z pather) {
			switch kind {
			case 0:
				roundRect(z, fx, fy, fw, fh, rr, false)
			case 1:
				roundRect(z, fx, fy, fw, fh, rr, false)
				roundRect(z, fx+th, fy+th, fw-2*th, fh-2*th, max(rr-th, 0), true)
			default:
				z.MoveTo(fx, fy)
				for k := 0; k < 6; k++ {
					z.LineTo(rng.Float32()*float32(w), rng.Float32()*float32(h))
				}
				z.ClosePath()
			}
		}
		want := image.NewRGBA(bg.Rect)
		copy(want.Pix, bg.Pix)
		got := image.NewRGBA(bg.Rect)
		copy(got.Pix, bg.Pix)
		seed := rng.Int63()

		u.Reset(w, h)
		rng.Seed(seed)
		draw(&u)
		u.Draw(want, b, image.NewUniform(col), image.Point{})

		z.Reset(w, h)
		rng.Seed(seed)
		draw(&z)
		z.Draw(got, b, image.NewUniform(col), image.Point{})

		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("shape %d (%dx%d, kind %d, %v): pixels differ from upstream", i, w, h, kind, col)
		}
	}
}
