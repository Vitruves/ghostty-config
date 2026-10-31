package pix

import (
	"bytes"
	"image"
	"image/draw"
	"math"
	"math/rand"
	"testing"
)

// picture is w by h pixels of noise, opaque but for a see-through band and
// a translucent corner.
func picture(rng *rand.Rand, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rng.Read(img.Pix)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := img.PixOffset(x, y)
			a := uint8(0xff)
			switch {
			case y == h/2:
				a = 0
			case x < 5 && y < 5:
				a = uint8(rng.Intn(256))
			}
			img.Pix[o+3] = a
			for i := 0; i < 3; i++ {
				img.Pix[o+i] = min(img.Pix[o+i], a)
			}
		}
	}
	return img
}

// TestPictureIsComposited wants what compositing the picture over the
// canvas makes, clipped or not, from a picture whose bounds start anywhere.
func TestPictureIsComposited(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 50; i++ {
		c := New(120, 80, 1.5)
		rng.Read(c.Img.Pix)
		want := image.NewRGBA(c.Img.Rect)
		copy(want.Pix, c.Img.Pix)
		var img image.Image = picture(rng, 10+rng.Intn(100), 10+rng.Intn(60))
		if i%3 == 0 {
			b := img.Bounds()
			img = img.(*image.RGBA).SubImage(image.Rect(b.Min.X+2, b.Min.Y+3, b.Max.X, b.Max.Y))
		}
		x, y := rng.Float32()*80-20, rng.Float32()*50-10
		if i%2 == 0 {
			c.Clip(10, 5, 50, 30)
		}
		at := image.Pt(int(math.Round(float64(x*c.S))), int(math.Round(float64(y*c.S))))
		r := img.Bounds().Sub(img.Bounds().Min).Add(at).Intersect(c.clip)
		draw.Draw(want, r, img, img.Bounds().Min.Add(r.Min.Sub(at)), draw.Over)
		c.Picture(img, x, y)
		if !bytes.Equal(c.Img.Pix, want.Pix) {
			t.Fatalf("picture %d: pixels differ from compositing", i)
		}
	}
}

func BenchmarkPictureOpaque(b *testing.B) {
	c := New(1600, 900, 1)
	img := image.NewRGBA(image.Rect(0, 0, 1400, 800))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.Picture(img, 100, 50)
	}
}
