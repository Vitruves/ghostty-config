package kimg

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"runtime"
	"sync"

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
	pix := c.Img.Pix[i : i+4 : i+4]
	sa := float64(src.A) / 255 * cov
	if sa <= 0 {
		return
	}
	if sa == 1 {
		// An opaque source replaces the pixel: the sums below come to
		// exactly this, and most of a filled shape is such pixels.
		pix[0], pix[1], pix[2], pix[3] = src.R, src.G, src.B, 255
		return
	}
	da := float64(pix[3]) / 255
	oa := sa + da*(1-sa)
	if oa <= 0 {
		return
	}
	// pix is premultiplied.
	pix[0] = top8(float64(src.R)*sa + float64(pix[0])*(1-sa))
	pix[1] = top8(float64(src.G)*sa + float64(pix[1])*(1-sa))
	pix[2] = top8(float64(src.B)*sa + float64(pix[2])*(1-sa))
	pix[3] = top8(oa * 255)
}

// top8 is a channel as a byte, 255 at most.
func top8(v float64) uint8 {
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// rrDist is the signed distance from (px, py) to a rounded rectangle:
// negative inside, positive outside.
func rrDist(px, py, x, y, w, h, r float64) float64 {
	cx, cy := x+w/2, y+h/2
	qx := math.Abs(px-cx) - (w/2 - r)
	qy := math.Abs(py-cy) - (h/2 - r)
	// Only past a corner is the distance a hypotenuse; everywhere else,
	// which is nearly everywhere, it comes to one of these, to the bit.
	switch {
	case qx <= 0 && qy <= 0:
		return math.Max(qx, qy) - r
	case qy <= 0:
		return qx - r
	case qx <= 0:
		return qy - r
	}
	return math.Hypot(qx, qy) + math.Min(math.Max(qx, qy), 0) - r
}

// inside is the block of pixels well inside a rounded rectangle: every one
// of them is further in than deep logical units, with a pixel to spare, so
// what covers it is known without measuring. Empty for a disc, or a shape
// too small to have a middle.
func (c *Canvas) inside(x, y, w, h, r, deep float64) image.Rectangle {
	m := math.Max(r, deep) + 1/c.S
	// Not image.Rect, which would turn a shape with no middle inside out.
	in := image.Rectangle{
		Min: image.Pt(int(math.Ceil((x+m)*c.S-0.5)), int(math.Ceil((y+m)*c.S-0.5))),
		Max: image.Pt(int(math.Floor((x+w-m)*c.S-0.5))+1, int(math.Floor((y+h-m)*c.S-0.5))+1),
	}
	return in.Intersect(c.Img.Bounds())
}

// FillFunc paints a rounded rectangle, anti-aliased, with a colour that may
// vary across it. Coordinates are logical.
func (c *Canvas) FillFunc(x, y, w, h, r float64, colour func(px, py float64) color.NRGBA) {
	rows := c.Img.Bounds()
	c.fillRows(x, y, w, h, r, colour, rows.Min.Y, rows.Max.Y)
}

// fillRows is FillFunc for the rows of the canvas from top to bottom.
func (c *Canvas) fillRows(x, y, w, h, r float64, colour func(px, py float64) color.NRGBA, top, bottom int) {
	s := c.S
	x0, y0 := int(math.Floor((x-1)*s)), int(math.Floor((y-1)*s))
	x1, y1 := int(math.Ceil((x+w+1)*s)), int(math.Ceil((y+h+1)*s))
	b := c.Img.Bounds()
	// The middle is covered whole: only the rim is measured.
	in := c.inside(x, y, w, h, r, 0.5/s)
	for py := maxI(y0, top); py < minI(y1, bottom); py++ {
		ly := (float64(py) + 0.5) / s
		for px := maxI(x0, b.Min.X); px < minI(x1, b.Max.X); px++ {
			lx := (float64(px) + 0.5) / s
			if py >= in.Min.Y && py < in.Max.Y && px >= in.Min.X && px < in.Max.X {
				c.over(px, py, colour(lx, ly), 1)
				continue
			}
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
	colour := func(float64, float64) color.NRGBA { return col }
	// One colour asks nothing of the rows above: a large shape is painted
	// in stripes, side by side.
	b := c.Img.Bounds()
	top, bottom := maxI(int(math.Floor((y-1)*c.S)), b.Min.Y), minI(int(math.Ceil((y+h+1)*c.S)), b.Max.Y)
	sideBySide(bottom-top, int(w*c.S), func(from, to int) { c.fillRows(x, y, w, h, r, colour, top+from, top+to) })
}

// Ring strokes the outline of a rounded rectangle, inside its edge.
func (c *Canvas) Ring(x, y, w, h, r, width float64, col color.NRGBA) {
	s := c.S
	x0, y0 := int(math.Floor((x-1)*s)), int(math.Floor((y-1)*s))
	x1, y1 := int(math.Ceil((x+w+1)*s)), int(math.Ceil((y+h+1)*s))
	b := c.Img.Bounds()
	// The middle is inside the inner edge too, and gets nothing.
	in := c.inside(x, y, w, h, r, width+0.5/s)
	for py := maxI(y0, b.Min.Y); py < minI(y1, b.Max.Y); py++ {
		for px := maxI(x0, b.Min.X); px < minI(x1, b.Max.X); px++ {
			if py >= in.Min.Y && py < in.Max.Y && px >= in.Min.X && px < in.Max.X {
				px = in.Max.X - 1
				continue
			}
			lx, ly := (float64(px)+0.5)/s, (float64(py)+0.5)/s
			d := rrDist(lx, ly, x, y, w, h, r) * s
			// Inside the outer edge, outside the inner one.
			cov := unit(0.5-d) - unit(0.5-(d+width*s))
			if cov > 0 {
				c.over(px, py, col, cov)
			}
		}
	}
}

// unit keeps a coverage between nothing and the whole pixel.
func unit(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v > 0 {
		return v
	}
	return 0
}

// Circle fills a disc.
func (c *Canvas) Circle(cx, cy, r float64, col color.NRGBA) {
	c.Fill(cx-r, cy-r, 2*r, 2*r, r, col)
}

// Shadow draws a blurred, offset copy of a rounded rectangle's shape.
func (c *Canvas) Shadow(x, y, w, h, r, blur, dy float64, col color.NRGBA) {
	pad := blur*3 + 2
	// The shadow of a shape is the same wherever it falls: the tiles of a
	// wall share one, and so do the pictures of a window as its settings
	// are stepped through.
	sub := kept(shadowKey{w, h, r, blur, c.S, col}, func() *Canvas {
		sub := NewCanvas(int(math.Ceil((w+2*pad)*c.S)), int(math.Ceil((h+2*pad)*c.S)), c.S)
		sub.Fill(pad, pad, w, h, r, col)
		sub.Blur(blur)
		return sub
	})
	c.Draw(sub, x-pad, y-pad+dy)
}

type shadowKey struct {
	w, h, r, blur, s float64
	col              color.NRGBA
}

// layers are the last few blurred layers drawn, newest last. Blurring one
// is most of what a picture with a shadow costs, and the next picture
// nearly always wants the same ones. They are only read once kept.
var layers struct {
	sync.Mutex
	list []layer
}

type layer struct {
	key any
	c   *Canvas
}

// maxLayers bounds the layers kept, a megabyte or two each at the size of
// the window's picture.
const maxLayers = 6

// kept is the layer drawn for key, drawn now if it is not among the last
// few. key must be comparable and say everything the drawing depends on.
func kept(key any, draw func() *Canvas) *Canvas {
	layers.Lock()
	for i, l := range layers.list {
		if l.key == key {
			copy(layers.list[i:], layers.list[i+1:])
			layers.list[len(layers.list)-1] = l
			layers.Unlock()
			return l.c
		}
	}
	layers.Unlock()
	// Drawn outside the lock: two pictures at once may each draw the same
	// layer, which costs time and nothing else.
	c := draw()
	layers.Lock()
	if len(layers.list) >= maxLayers {
		layers.list = append(layers.list[:0], layers.list[1:]...)
	}
	layers.list = append(layers.list, layer{key, c})
	layers.Unlock()
	return c
}

// Draw composites another canvas at a logical position.
func (c *Canvas) Draw(o *Canvas, x, y float64) {
	ox, oy := int(math.Round(x*c.S)), int(math.Round(y*c.S))
	b, cb := o.Img.Bounds(), c.Img.Bounds()
	// Only the part of o that lands on this canvas.
	px0, px1 := maxI(0, cb.Min.X-ox), minI(b.Dx(), cb.Max.X-ox)
	py0, py1 := maxI(0, cb.Min.Y-oy), minI(b.Dy(), cb.Max.Y-oy)
	if px0 >= px1 || py0 >= py1 {
		return
	}
	sideBySide(py1-py0, px1-px0, func(from, to int) {
		for py := py0 + from; py < py0+to; py++ {
			row := o.Img.Pix[o.Img.PixOffset(px0, py):]
			for px := px0; px < px1; px, row = px+1, row[4:] {
				if row[3] == 0 {
					continue
				}
				a := float64(row[3]) / 255
				// Un-premultiply, then composite.
				src := color.NRGBA{top8(float64(row[0]) / a), top8(float64(row[1]) / a), top8(float64(row[2]) / a), 255}
				c.over(ox+px, oy+py, src, a)
			}
		}
	})
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
	pix := c.Img.Pix
	if w == 0 || h == 0 {
		return
	}
	// A channel that is one value everywhere comes out of every pass as it
	// went in, exactly, and is left alone: a shadow is one colour, so only
	// its alpha is blurred.
	var chans []int
	for ch := 0; ch < 4; ch++ {
		for i := ch; i < len(pix); i += 4 {
			if pix[i] != pix[ch] {
				chans = append(chans, ch)
				break
			}
		}
	}
	if len(chans) == 0 {
		return
	}
	sp, tp := floats(w*h*4), floats(w*h*4)
	defer floatPool.Put(sp)
	defer floatPool.Put(tp)
	src, tmp := *sp, *tp
	sideBySide(h, w, func(y0, y1 int) {
		for _, ch := range chans {
			for i := y0*w*4 + ch; i < y1*w*4; i += 4 {
				src[i] = float64(pix[i])
			}
		}
	})
	for pass := 0; pass < 3; pass++ {
		sideBySide(h, w, func(y0, y1 int) { boxH(src, tmp, w, r, chans, y0, y1) })
		sideBySide(w, h, func(x0, x1 int) { boxV(tmp, src, w, h, r, chans, x0, x1) })
	}
	sideBySide(h, w, func(y0, y1 int) {
		for _, ch := range chans {
			for i := y0*w*4 + ch; i < y1*w*4; i += 4 {
				// Rounded; a mean of bytes is never below nothing.
				pix[i] = top8(src[i] + 0.5)
			}
		}
	})
}

// floatPool keeps the working copies of a blur, tens of megabytes for a
// picture of the window, from being made anew for each one.
var floatPool sync.Pool

// floats is n numbers from the pool, holding whatever they held.
func floats(n int) *[]float64 {
	if p, ok := floatPool.Get().(*[]float64); ok && cap(*p) >= n {
		*p = (*p)[:n]
		return p
	}
	s := make([]float64, n)
	return &s
}

// sideBySide runs work over n lines of a picture, each across pixels, in a
// few stripes at once when there is enough to share out. The lines must not
// read or write one another's pixels.
func sideBySide(n, across int, work func(from, to int)) {
	if n <= 0 {
		return
	}
	workers := minI(minI(runtime.GOMAXPROCS(0), 8), n)
	if workers < 2 || n*across < 1<<15 {
		work(0, n)
		return
	}
	var wg sync.WaitGroup
	for i := 1; i < workers; i++ {
		wg.Go(func() { work(i*n/workers, (i+1)*n/workers) })
	}
	work(0, n/workers)
	wg.Wait()
}

// boxH is one horizontal pass over the rows y0 to y1: each pixel becomes
// the mean of the 2r+1 around it, the edge pixel standing for what lies
// past the edge.
func boxH(src, dst []float64, w, r int, chans []int, y0, y1 int) {
	n := float64(2*r + 1)
	for y := y0; y < y1; y++ {
		for _, ch := range chans {
			in, out := src[y*w*4+ch:], dst[y*w*4+ch:]
			var sum float64
			for x := -r; x <= r; x++ {
				sum += in[minI(maxI(x, 0), w-1)*4]
			}
			for x := 0; x < w; x++ {
				out[x*4] = sum / n
				sum += in[minI(x+r+1, w-1)*4] - in[maxI(x-r, 0)*4]
			}
		}
	}
}

// boxV is the vertical pass over the columns x0 to x1. It goes down the
// rows with a running sum for each column, which adds up each column in the
// order going down it alone would, and reads memory in the order it lies.
func boxV(src, dst []float64, w, h, r int, chans []int, x0, x1 int) {
	n := float64(2*r + 1)
	// first is the first number of a row's part, step the distance to the
	// next one of the same channel, count how many there are: every
	// channel in one sweep when all four are blurred.
	sweep := func(first, step, count int) {
		row := func(y int) []float64 { return src[(minI(maxI(y, 0), h-1)*w+x0)*4+first:] }
		sums := make([]float64, count)
		for y := -r; y <= r; y++ {
			in := row(y)
			for i := range sums {
				sums[i] += in[i*step]
			}
		}
		for y := 0; y < h; y++ {
			out, add, sub := dst[(y*w+x0)*4+first:], row(y+r+1), row(y-r)
			for i := range sums {
				out[i*step] = sums[i] / n
				sums[i] += add[i*step] - sub[i*step]
			}
		}
	}
	if len(chans) == 4 {
		sweep(0, 1, (x1-x0)*4)
		return
	}
	for _, ch := range chans {
		sweep(ch, 4, x1-x0)
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
