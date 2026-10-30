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

// glyphFace is a face that keeps the masks it has drawn.
type glyphFace struct {
	font.Face
	mu     sync.Mutex
	glyphs map[glyphKey]*glyph
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

// maxGlyphs bounds the masks a face keeps; past it the face starts over.
const maxGlyphs = 4096

// CacheGlyphs wraps an outline face so that it keeps the masks it draws.
func CacheGlyphs(f font.Face) font.Face {
	return &glyphFace{Face: f, glyphs: map[glyphKey]*glyph{}}
}

func (g *glyphFace) Glyph(dot fixed.Point26_6, r rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	at := image.Pt(dot.X.Floor(), dot.Y.Floor())
	k := glyphKey{r, dot.X & 63, dot.Y & 63}
	g.mu.Lock()
	defer g.mu.Unlock()
	gl, ok := g.glyphs[k]
	if !ok {
		gl = &glyph{}
		dr, mask, maskp, advance, found := g.Face.Glyph(fixed.Point26_6{X: k.fx, Y: k.fy}, r)
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
		if len(g.glyphs) >= maxGlyphs {
			g.glyphs = map[glyphKey]*glyph{}
		}
		g.glyphs[k] = gl
	}
	if !gl.ok {
		return image.Rectangle{}, nil, image.Point{}, gl.advance, false
	}
	return gl.dr.Add(at), gl.mask, gl.maskp, gl.advance, true
}
