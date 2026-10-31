package kimg

import (
	"image"
	"image/draw"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// A face draws each glyph from its outline every time it is asked, and a
// frame of the picture interface asks for thousands, mostly the same. What
// an outline face makes of a glyph depends on the glyph and on where the dot
// falls within a pixel, nothing else: drawn a whole pixel further on, the
// same mask lands a pixel further on. So the masks are kept, by glyph and
// by the dot's fraction of a pixel, and moved to where they are asked for.

// glyphFace is a face that keeps the masks it has drawn, and what it
// answers about each glyph and pair of glyphs. It is safe to share between
// goroutines: the outline face it wraps, which is not, is only asked under
// the lock, and a mask once kept is never written again.
type glyphFace struct {
	font.Face
	mu      sync.RWMutex
	glyphs  map[glyphKey]*glyph
	bounds  map[rune]glyphBounds
	advance map[rune]glyphAdvance
	kern    map[[2]rune]fixed.Int26_6
	metrics font.Metrics
}

type glyphKey struct {
	r      rune
	fx, fy fixed.Int26_6 // the dot within its pixel
}

type glyph struct {
	dr      image.Rectangle // relative to the dot's pixel
	mask    *image.Alpha
	maskp   image.Point
	advance fixed.Int26_6
	ok      bool
}

type glyphBounds struct {
	bounds  fixed.Rectangle26_6
	advance fixed.Int26_6
	ok      bool
}

type glyphAdvance struct {
	advance fixed.Int26_6
	ok      bool
}

// maxGlyphs bounds the masks a face keeps, and each of the other things it
// keeps; past it that part starts over.
const maxGlyphs = 4096

// CacheGlyphs wraps an outline face so that it keeps the masks it draws.
func CacheGlyphs(f font.Face) font.Face {
	return &glyphFace{
		Face:    f,
		glyphs:  map[glyphKey]*glyph{},
		bounds:  map[rune]glyphBounds{},
		advance: map[rune]glyphAdvance{},
		kern:    map[[2]rune]fixed.Int26_6{},
		metrics: f.Metrics(),
	}
}

func (g *glyphFace) Glyph(dot fixed.Point26_6, r rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	at := image.Pt(dot.X.Floor(), dot.Y.Floor())
	k := glyphKey{r, dot.X & 63, dot.Y & 63}
	g.mu.RLock()
	gl, ok := g.glyphs[k]
	g.mu.RUnlock()
	if !ok {
		g.mu.Lock()
		if gl, ok = g.glyphs[k]; !ok {
			gl = g.draw(k)
			if len(g.glyphs) >= maxGlyphs {
				g.glyphs = map[glyphKey]*glyph{}
			}
			g.glyphs[k] = gl
		}
		g.mu.Unlock()
	}
	if !gl.ok {
		return image.Rectangle{}, nil, image.Point{}, gl.advance, false
	}
	return gl.dr.Add(at), gl.mask, gl.maskp, gl.advance, true
}

// draw asks the outline face for a glyph, under the lock.
func (g *glyphFace) draw(k glyphKey) *glyph {
	gl := &glyph{}
	dr, mask, maskp, advance, found := g.Face.Glyph(fixed.Point26_6{X: k.fx, Y: k.fy}, k.r)
	gl.advance, gl.ok = advance, found
	if found {
		// The face draws every glyph into the same buffer: keep a copy.
		gl.dr, gl.maskp = dr, maskp
		b := mask.Bounds()
		gl.mask = image.NewAlpha(b)
		if a, isAlpha := mask.(*image.Alpha); isAlpha {
			for y := b.Min.Y; y < b.Max.Y; y++ {
				copy(gl.mask.Pix[gl.mask.PixOffset(b.Min.X, y):][:b.Dx()], a.Pix[a.PixOffset(b.Min.X, y):])
			}
		} else {
			draw.Draw(gl.mask, b, mask, b.Min, draw.Src)
		}
	}
	return gl
}

func (g *glyphFace) GlyphBounds(r rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	g.mu.RLock()
	b, ok := g.bounds[r]
	g.mu.RUnlock()
	if !ok {
		g.mu.Lock()
		if b, ok = g.bounds[r]; !ok {
			b.bounds, b.advance, b.ok = g.Face.GlyphBounds(r)
			if len(g.bounds) >= maxGlyphs {
				g.bounds = map[rune]glyphBounds{}
			}
			g.bounds[r] = b
		}
		g.mu.Unlock()
	}
	return b.bounds, b.advance, b.ok
}

func (g *glyphFace) GlyphAdvance(r rune) (fixed.Int26_6, bool) {
	g.mu.RLock()
	a, ok := g.advance[r]
	g.mu.RUnlock()
	if !ok {
		g.mu.Lock()
		if a, ok = g.advance[r]; !ok {
			a.advance, a.ok = g.Face.GlyphAdvance(r)
			if len(g.advance) >= maxGlyphs {
				g.advance = map[rune]glyphAdvance{}
			}
			g.advance[r] = a
		}
		g.mu.Unlock()
	}
	return a.advance, a.ok
}

func (g *glyphFace) Kern(r0, r1 rune) fixed.Int26_6 {
	k := [2]rune{r0, r1}
	g.mu.RLock()
	v, ok := g.kern[k]
	g.mu.RUnlock()
	if !ok {
		g.mu.Lock()
		if v, ok = g.kern[k]; !ok {
			v = g.Face.Kern(r0, r1)
			if len(g.kern) >= maxGlyphs {
				g.kern = map[[2]rune]fixed.Int26_6{}
			}
			g.kern[k] = v
		}
		g.mu.Unlock()
	}
	return v
}

// Metrics are the outline face's, read once.
func (g *glyphFace) Metrics() font.Metrics { return g.metrics }

// Close closes the outline face, under the lock.
func (g *glyphFace) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.Face.Close()
}
