// Package pix is a small, fast 2D canvas for drawing the whole interface as
// one picture: rounded rectangles and discs through an anti-aliasing
// rasteriser, text in the fonts kimg embeds, other pictures composited in.
// Coordinates are logical; S is how many device pixels one logical unit is,
// so the same layout serves every font size and every display density.
package pix

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/vitruves/ghostty-config/internal/kimg"
	"github.com/vitruves/ghostty-config/internal/pix/vector"
)

// Canvas is an RGBA picture drawn in logical units.
type Canvas struct {
	Img *image.RGBA
	S   float32
	// TextScale enlarges every face handed out, leaving the layout alone.
	TextScale float32
	clip      image.Rectangle
	ras       vector.Rasterizer
	// touched, while a Memo watches, grows to cover every pixel drawn.
	touched *image.Rectangle
}

// New makes a canvas of w by h device pixels.
func New(w, h int, s float32) *Canvas {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	return &Canvas{Img: img, S: s, TextScale: 1, clip: img.Bounds()}
}

// Renew is a canvas of w by h device pixels at scale s, made from c's
// pixels when c has that size, as blank as a new one.
func Renew(c *Canvas, w, h int, s float32) *Canvas {
	if c == nil || c.Img.Rect.Dx() != w || c.Img.Rect.Dy() != h {
		return New(w, h, s)
	}
	clear(c.Img.Pix)
	c.S, c.TextScale, c.clip, c.touched = s, 1, c.Img.Bounds(), nil
	return c
}

// W and H are the size in logical units.
func (c *Canvas) W() float32 { return float32(c.Img.Bounds().Dx()) / c.S }
func (c *Canvas) H() float32 { return float32(c.Img.Bounds().Dy()) / c.S }

// Hex is a colour from #rrggbb with an alpha in 0..1.
func Hex(hex string, a float64) color.NRGBA { return kimg.RGBA(hex, a) }

// touch records that the pixels of b may have been drawn.
func (c *Canvas) touch(b image.Rectangle) {
	if c.touched != nil {
		*c.touched = c.touched.Union(b)
	}
}

// Clear paints the whole canvas.
func (c *Canvas) Clear(col color.Color) {
	c.touch(c.Img.Bounds())
	draw.Draw(c.Img, c.Img.Bounds(), image.NewUniform(col), image.Point{}, draw.Src)
}

// Clip limits drawing to a logical rectangle until Unclip.
func (c *Canvas) Clip(x, y, w, h float32) {
	c.clip = c.dev(x, y, w, h).Intersect(c.Img.Bounds())
}

// Unclip lets drawing reach the whole canvas again.
func (c *Canvas) Unclip() { c.clip = c.Img.Bounds() }

func (c *Canvas) dev(x, y, w, h float32) image.Rectangle {
	s := c.S
	return image.Rect(int(math.Floor(float64(x*s))), int(math.Floor(float64(y*s))), int(math.Ceil(float64((x+w)*s))), int(math.Ceil(float64((y+h)*s))))
}

// path runs a rounded rectangle, in device pixels relative to o, through the
// rasteriser. reverse winds it the other way, to cut a hole.
func (c *Canvas) path(x, y, w, h, r float32, o image.Point, reverse bool) {
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	x -= float32(o.X)
	y -= float32(o.Y)
	k := 0.5523 * r
	z := &c.ras
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

// area prepares the rasteriser for a shape whose device bounds are b, and
// returns the part of it that may be drawn.
func (c *Canvas) area(b image.Rectangle) (image.Rectangle, bool) {
	b = b.Intersect(c.clip)
	if b.Empty() {
		return b, false
	}
	c.touch(b)
	c.ras.Reset(b.Dx(), b.Dy())
	return b, true
}

// Rect fills a rounded rectangle.
func (c *Canvas) Rect(x, y, w, h, r float32, col color.Color) {
	if w <= 0 || h <= 0 {
		return
	}
	s := c.S
	b, ok := c.area(c.dev(x, y, w, h))
	if !ok {
		return
	}
	if r <= 0 && x*s == float32(int(x*s)) && y*s == float32(int(y*s)) && (x+w)*s == float32(int((x+w)*s)) && (y+h)*s == float32(int((y+h)*s)) {
		draw.Draw(c.Img, b, image.NewUniform(col), image.Point{}, draw.Over)
		return
	}
	c.path(x*s, y*s, w*s, h*s, r*s, b.Min, false)
	c.ras.Draw(c.Img, b, image.NewUniform(col), image.Point{})
}

// Ring strokes the inside of a rounded rectangle's edge, t thick.
func (c *Canvas) Ring(x, y, w, h, r, t float32, col color.Color) {
	s := c.S
	b, ok := c.area(c.dev(x, y, w, h))
	if !ok {
		return
	}
	c.path(x*s, y*s, w*s, h*s, r*s, b.Min, false)
	ir := r - t
	if ir < 0 {
		ir = 0
	}
	c.path((x+t)*s, (y+t)*s, (w-2*t)*s, (h-2*t)*s, ir*s, b.Min, true)
	c.ras.Draw(c.Img, b, image.NewUniform(col), image.Point{})
}

// Circle fills a disc.
func (c *Canvas) Circle(cx, cy, r float32, col color.Color) {
	c.Rect(cx-r, cy-r, 2*r, 2*r, r, col)
}

// Shadow is a soft shadow under a rounded rectangle: a few widening,
// fading layers, cheap enough to draw on every frame.
func (c *Canvas) Shadow(x, y, w, h, r, spread, dy float32, alpha float64) {
	const steps = 6
	for i := steps; i >= 1; i-- {
		d := spread * float32(i) / steps
		a := alpha / steps * 1.6 * (1 - float64(i-1)/steps)
		c.Rect(x-d, y-d+dy, w+2*d, h+2*d, r+d, color.NRGBA{0, 0, 0, uint8(math.Min(255, a*255))})
	}
}

// Face is a face of the embedded fonts at a logical size.
func (c *Canvas) Face(mono, bold bool, px float32) font.Face {
	return kimg.Face(mono, bold, float64(px*c.S*c.TextScale))
}

// Font is a font file of this machine, read to draw text in it.
type Font struct {
	f     *opentype.Font
	mu    sync.Mutex
	faces map[int]font.Face
}

// OpenFont reads a font file. In a collection the face named postScript is
// the one taken, or else the one at index.
func OpenFont(path, postScript string, index int) (*Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	coll, err := opentype.ParseCollection(data)
	if err != nil {
		return nil, err
	}
	n := coll.NumFonts()
	if n == 0 {
		return nil, fmt.Errorf("%s holds no font", path)
	}
	pick := -1
	if postScript != "" && n > 1 {
		var b sfnt.Buffer
		for i := 0; i < n && pick < 0; i++ {
			if f, err := coll.Font(i); err == nil {
				if name, err := f.Name(&b, sfnt.NameIDPostScript); err == nil && name == postScript {
					pick = i
				}
			}
		}
	}
	if pick < 0 {
		pick = min(max(index, 0), n-1)
	}
	f, err := coll.Font(pick)
	if err != nil {
		return nil, err
	}
	return &Font{f: f, faces: map[int]font.Face{}}, nil
}

// FaceIn is a face of a font file at a logical size.
func (c *Canvas) FaceIn(f *Font, px float32) font.Face {
	key := int(px*c.S*c.TextScale*10 + 0.5)
	f.mu.Lock()
	defer f.mu.Unlock()
	if face, ok := f.faces[key]; ok {
		return face
	}
	of, err := opentype.NewFace(f.f, &opentype.FaceOptions{Size: float64(key) / 10, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return c.Face(true, false, px)
	}
	face := kimg.CacheGlyphs(of)
	if len(f.faces) >= maxFaces {
		// Every scale the canvas is drawn at makes faces of its own, and
		// the scale follows the terminal's font; past a bound the cache
		// starts over. The faces handed out stay good: nothing looks
		// them up here.
		f.faces = map[int]font.Face{}
	}
	f.faces[key] = face
	return face
}

// maxFaces bounds the faces a font file keeps.
const maxFaces = 64

// substitutes stand in for symbols no embedded font draws.
var substitutes = map[rune]string{
	'⌘': "Cmd+", '⌥': "Opt+", '⌃': "Ctrl+", '⇧': "Shift+", '⌫': "Bksp", '⏎': "Enter",
	'●': "•", '♥': "♥", '✎': "", '❯': "›", '▏': "|", '─': "-", '│': "|",
	'░': " ", '█': " ", '▄': " ", '▀': " ", '╭': " ", '╮': " ", '╯': " ", '╰': " ", '✓': "ok", '✗': "x",
}

// run is a stretch of text one face draws.
type run struct {
	f font.Face
	s string
}

// runs splits text by the face that has its glyphs: the face asked for,
// the monospace one for arrows, or a word in place of a symbol.
func runs(f font.Face, s string) []run {
	var out []run
	var cur strings.Builder
	var curF font.Face = f
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, run{curF, cur.String()})
			cur.Reset()
		}
	}
	for _, r := range s {
		g, text := f, string(r)
		if r > 127 && !kimg.Covers(f, r) {
			if m := kimg.MonoOf(f); kimg.Covers(m, r) {
				g = m
			} else if sub, ok := substitutes[r]; ok && sub != string(r) {
				text = sub
			} else {
				text = "?"
				if ok {
					text = ""
				}
			}
		}
		if g != curF {
			flush()
			curF = g
		}
		cur.WriteString(text)
	}
	flush()
	return out
}

// Text draws a string with its baseline at y and returns its logical width.
func (c *Canvas) Text(f font.Face, x, y float32, s string, col color.Color) float32 {
	dst := c.Img
	if c.clip != c.Img.Bounds() {
		dst = c.Img.SubImage(c.clip).(*image.RGBA)
	}
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(col), Dot: fixed.Point26_6{X: fixed.Int26_6(x * c.S * 64), Y: fixed.Int26_6(y * c.S * 64)}}
	for _, r := range runs(f, s) {
		d.Face = r.f
		if c.touched != nil {
			// Where the glyphs land, a pixel wider all round for the
			// rounding of their masks.
			bb, _ := d.BoundString(r.s)
			c.touch(image.Rect(bb.Min.X.Floor()-1, bb.Min.Y.Floor()-1, bb.Max.X.Ceil()+1, bb.Max.Y.Ceil()+1).Intersect(dst.Rect))
		}
		d.DrawString(r.s)
	}
	return float32(d.Dot.X)/64/c.S - x
}

// Width measures a string in logical units.
func (c *Canvas) Width(f font.Face, s string) float32 {
	w := fixed.Int26_6(0)
	for _, r := range runs(f, s) {
		w += (&font.Drawer{Face: r.f}).MeasureString(r.s)
	}
	return float32(w) / 64 / c.S
}

// Fit cuts a string to a logical width, with an ellipsis when it was cut.
func (c *Canvas) Fit(f font.Face, s string, w float32) string {
	if c.Width(f, s) <= w {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if c.Width(f, string(r[:mid])+"…") <= w {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		return ""
	}
	return strings.TrimRight(string(r[:lo]), " ") + "…"
}

// Wrap breaks text into lines no wider than w.
func (c *Canvas) Wrap(f font.Face, text string, w float32) []string {
	var out []string
	for _, para := range strings.Split(text, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			try := word
			if line != "" {
				try = line + " " + word
			}
			if line != "" && c.Width(f, try) > w {
				out = append(out, line)
				line = word
				continue
			}
			line = try
		}
		out = append(out, line)
	}
	return out
}

// Picture composites another picture with its top left corner at a logical
// point, at its own pixel size.
func (c *Canvas) Picture(img image.Image, x, y float32) {
	at := image.Pt(int(math.Round(float64(x*c.S))), int(math.Round(float64(y*c.S))))
	r := img.Bounds().Sub(img.Bounds().Min).Add(at)
	c.touch(r.Intersect(c.clip))
	draw.Draw(c.Img, r.Intersect(c.clip), img, img.Bounds().Min.Add(r.Intersect(c.clip).Min.Sub(at)), draw.Over)
}

// Dim lays a translucent colour over the whole canvas.
func (c *Canvas) Dim(col color.Color) {
	c.touch(c.Img.Bounds())
	draw.Draw(c.Img, c.Img.Bounds(), image.NewUniform(col), image.Point{}, draw.Over)
}

// Poly fills a polygon given as logical x, y pairs.
func (c *Canvas) Poly(pts []float32, col color.Color) {
	if len(pts) < 6 {
		return
	}
	s := c.S
	minX, minY, maxX, maxY := pts[0], pts[1], pts[0], pts[1]
	for i := 0; i+1 < len(pts); i += 2 {
		minX, maxX = min(minX, pts[i]), max(maxX, pts[i])
		minY, maxY = min(minY, pts[i+1]), max(maxY, pts[i+1])
	}
	b, ok := c.area(c.dev(minX, minY, maxX-minX, maxY-minY))
	if !ok {
		return
	}
	ox, oy := float32(b.Min.X), float32(b.Min.Y)
	c.ras.MoveTo(pts[0]*s-ox, pts[1]*s-oy)
	for i := 2; i+1 < len(pts); i += 2 {
		c.ras.LineTo(pts[i]*s-ox, pts[i+1]*s-oy)
	}
	c.ras.ClosePath()
	c.ras.Draw(c.Img, b, image.NewUniform(col), image.Point{})
}

// Line strokes a segment w thick, with square ends.
func (c *Canvas) Line(x1, y1, x2, y2, w float32, col color.Color) {
	dx, dy := x2-x1, y2-y1
	l := float32(math.Hypot(float64(dx), float64(dy)))
	if l == 0 {
		return
	}
	nx, ny := -dy/l*w/2, dx/l*w/2
	ex, ey := dx/l*w/2, dy/l*w/2
	c.Poly([]float32{x1 + nx - ex, y1 + ny - ey, x2 + nx + ex, y2 + ny + ey, x2 - nx + ex, y2 - ny + ey, x1 - nx - ex, y1 - ny - ey}, col)
}

// Star fills a five-pointed star of outer radius r.
func (c *Canvas) Star(cx, cy, r float32, col color.Color) {
	var pts []float32
	for i := 0; i < 10; i++ {
		rr := r
		if i%2 == 1 {
			rr = r * 0.45
		}
		a := math.Pi/2 + float64(i)*math.Pi/5
		pts = append(pts, cx+rr*float32(math.Cos(a)), cy-rr*float32(math.Sin(a)))
	}
	c.Poly(pts, col)
}
