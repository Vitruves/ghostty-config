package kimg

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// Canvas is an RGBA picture drawn in logical units: a terminal cell is taken
// to be 10 by 20 of them, and S is how many device pixels one unit is, so the
// same drawing code serves a plain display and a Retina one.
type Canvas struct {
	Img *image.RGBA
	S   float64
}

// NewCanvas makes a transparent canvas of w by h device pixels.
func NewCanvas(w, h int, s float64) *Canvas {
	return &Canvas{Img: image.NewRGBA(image.Rect(0, 0, w, h)), S: s}
}

// W and H are the size in logical units.
func (c *Canvas) W() float64 { return float64(c.Img.Bounds().Dx()) / c.S }
func (c *Canvas) H() float64 { return float64(c.Img.Bounds().Dy()) / c.S }

// RGBA parses #rrggbb with an alpha in 0..1.
func RGBA(hex string, a float64) color.NRGBA {
	var r, g, b int
	if len(hex) == 7 && hex[0] == '#' {
		for i, p := range []*int{&r, &g, &b} {
			v := 0
			for _, ch := range hex[1+2*i : 3+2*i] {
				v <<= 4
				switch {
				case ch >= '0' && ch <= '9':
					v |= int(ch - '0')
				case ch >= 'a' && ch <= 'f':
					v |= int(ch-'a') + 10
				case ch >= 'A' && ch <= 'F':
					v |= int(ch-'A') + 10
				}
			}
			*p = v
		}
	}
	return color.NRGBA{uint8(r), uint8(g), uint8(b), uint8(math.Round(a * 255))}
}

// over composites src, with extra coverage, onto the pixel at (x, y).
func (c *Canvas) over(x, y int, src color.NRGBA, cov float64) {
	if cov <= 0 {
		return
	}
	i := c.Img.PixOffset(x, y)
	pix := c.Img.Pix
	sa := float64(src.A) / 255 * cov
	if sa <= 0 {
		return
	}
	da := float64(pix[i+3]) / 255
	oa := sa + da*(1-sa)
	if oa <= 0 {
		return
	}
	// pix is premultiplied.
	pix[i+0] = uint8(math.Min(255, float64(src.R)*sa+float64(pix[i+0])*(1-sa)))
	pix[i+1] = uint8(math.Min(255, float64(src.G)*sa+float64(pix[i+1])*(1-sa)))
	pix[i+2] = uint8(math.Min(255, float64(src.B)*sa+float64(pix[i+2])*(1-sa)))
	pix[i+3] = uint8(math.Min(255, oa*255))
}

// rrDist is the signed distance from (px, py) to a rounded rectangle:
// negative inside, positive outside.
func rrDist(px, py, x, y, w, h, r float64) float64 {
	cx, cy := x+w/2, y+h/2
	qx := math.Abs(px-cx) - (w/2 - r)
	qy := math.Abs(py-cy) - (h/2 - r)
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
}

// FillFunc paints a rounded rectangle, anti-aliased, with a colour that may
// vary across it. Coordinates are logical.
func (c *Canvas) FillFunc(x, y, w, h, r float64, colour func(px, py float64) color.NRGBA) {
	s := c.S
	x0, y0 := int(math.Floor((x-1)*s)), int(math.Floor((y-1)*s))
	x1, y1 := int(math.Ceil((x+w+1)*s)), int(math.Ceil((y+h+1)*s))
	b := c.Img.Bounds()
	for py := maxI(y0, b.Min.Y); py < minI(y1, b.Max.Y); py++ {
		for px := maxI(x0, b.Min.X); px < minI(x1, b.Max.X); px++ {
			lx, ly := (float64(px)+0.5)/s, (float64(py)+0.5)/s
			d := rrDist(lx, ly, x, y, w, h, r) * s
			cov := 0.5 - d
			if cov <= 0 {
				continue
			}
			if cov > 1 {
				cov = 1
			}
			c.over(px, py, colour(lx, ly), cov)
		}
	}
}

// Fill paints a rounded rectangle in one colour.
func (c *Canvas) Fill(x, y, w, h, r float64, col color.NRGBA) {
	c.FillFunc(x, y, w, h, r, func(float64, float64) color.NRGBA { return col })
}

// Ring strokes the outline of a rounded rectangle, inside its edge.
func (c *Canvas) Ring(x, y, w, h, r, width float64, col color.NRGBA) {
	s := c.S
	x0, y0 := int(math.Floor((x-1)*s)), int(math.Floor((y-1)*s))
	x1, y1 := int(math.Ceil((x+w+1)*s)), int(math.Ceil((y+h+1)*s))
	b := c.Img.Bounds()
	for py := maxI(y0, b.Min.Y); py < minI(y1, b.Max.Y); py++ {
		for px := maxI(x0, b.Min.X); px < minI(x1, b.Max.X); px++ {
			lx, ly := (float64(px)+0.5)/s, (float64(py)+0.5)/s
			d := rrDist(lx, ly, x, y, w, h, r) * s
			// Inside the outer edge, outside the inner one.
			outer := 0.5 - d
			inner := 0.5 - (d + width*s)
			cov := math.Min(math.Max(outer, 0), 1) - math.Min(math.Max(inner, 0), 1)
			if cov > 0 {
				c.over(px, py, col, cov)
			}
		}
	}
}

// Circle fills a disc.
func (c *Canvas) Circle(cx, cy, r float64, col color.NRGBA) {
	c.Fill(cx-r, cy-r, 2*r, 2*r, r, col)
}

// Shadow draws a blurred, offset copy of a rounded rectangle's shape.
func (c *Canvas) Shadow(x, y, w, h, r, blur, dy float64, col color.NRGBA) {
	pad := blur*3 + 2
	sub := NewCanvas(int(math.Ceil((w+2*pad)*c.S)), int(math.Ceil((h+2*pad)*c.S)), c.S)
	sub.Fill(pad, pad, w, h, r, col)
	sub.Blur(blur)
	c.Draw(sub, x-pad, y-pad+dy)
}

// Draw composites another canvas at a logical position.
func (c *Canvas) Draw(o *Canvas, x, y float64) {
	ox, oy := int(math.Round(x*c.S)), int(math.Round(y*c.S))
	b := o.Img.Bounds()
	for py := 0; py < b.Dy(); py++ {
		for px := 0; px < b.Dx(); px++ {
			tx, ty := ox+px, oy+py
			if !(image.Point{tx, ty}).In(c.Img.Bounds()) {
				continue
			}
			i := o.Img.PixOffset(px, py)
			a := float64(o.Img.Pix[i+3]) / 255
			if a <= 0 {
				continue
			}
			// Un-premultiply, then composite.
			src := color.NRGBA{
				uint8(math.Min(255, float64(o.Img.Pix[i+0])/a)),
				uint8(math.Min(255, float64(o.Img.Pix[i+1])/a)),
				uint8(math.Min(255, float64(o.Img.Pix[i+2])/a)), 255}
			c.over(tx, ty, src, a)
		}
	}
}

// Text draws a string with its baseline at y, left edge at x.
func (c *Canvas) Text(f font.Face, x, y float64, s string, col color.NRGBA) float64 {
	d := &font.Drawer{Dst: c.Img, Src: image.NewUniform(col), Face: f, Dot: fixed.Point26_6{X: fixed.I(int(math.Round(x * c.S))), Y: fixed.I(int(math.Round(y * c.S)))}}
	d.DrawString(s)
	return float64(d.Dot.X.Round())/c.S - x
}

// Width measures a string in logical units.
func (c *Canvas) Width(f font.Face, s string) float64 {
	d := &font.Drawer{Face: f}
	return float64(d.MeasureString(s).Round()) / c.S
}

// Blur softens the whole canvas with three box passes, which approximate a
// Gaussian of the given radius in logical units.
func (c *Canvas) Blur(radius float64) {
	r := int(math.Round(radius * c.S / 1.7))
	if r < 1 {
		return
	}
	b := c.Img.Bounds()
	w, h := b.Dx(), b.Dy()
	tmp := make([]float64, w*h*4)
	src := make([]float64, w*h*4)
	for i, v := range c.Img.Pix {
		src[i] = float64(v)
	}
	for pass := 0; pass < 3; pass++ {
		boxH(src, tmp, w, h, r)
		boxV(tmp, src, w, h, r)
	}
	for i := range c.Img.Pix {
		c.Img.Pix[i] = uint8(math.Min(255, math.Max(0, src[i]+0.5)))
	}
}

func boxH(src, dst []float64, w, h, r int) {
	n := float64(2*r + 1)
	for y := 0; y < h; y++ {
		for ch := 0; ch < 4; ch++ {
			var sum float64
			at := func(x int) float64 {
				if x < 0 {
					x = 0
				}
				if x >= w {
					x = w - 1
				}
				return src[(y*w+x)*4+ch]
			}
			for x := -r; x <= r; x++ {
				sum += at(x)
			}
			for x := 0; x < w; x++ {
				dst[(y*w+x)*4+ch] = sum / n
				sum += at(x+r+1) - at(x-r)
			}
		}
	}
}

func boxV(src, dst []float64, w, h, r int) {
	n := float64(2*r + 1)
	for x := 0; x < w; x++ {
		for ch := 0; ch < 4; ch++ {
			var sum float64
			at := func(y int) float64 {
				if y < 0 {
					y = 0
				}
				if y >= h {
					y = h - 1
				}
				return src[(y*w+x)*4+ch]
			}
			for y := -r; y <= r; y++ {
				sum += at(y)
			}
			for y := 0; y < h; y++ {
				dst[(y*w+x)*4+ch] = sum / n
				sum += at(y+r+1) - at(y-r)
			}
		}
	}
}

// PNG encodes the canvas.
func (c *Canvas) PNG() []byte {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	_ = enc.Encode(&buf, c.Img)
	return buf.Bytes()
}

func maxI(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func minI(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// F is a face of the given size in logical units, scaled to this canvas.
func (c *Canvas) F(mono, bold bool, px float64) font.Face { return face(mono, bold, px*c.S) }
