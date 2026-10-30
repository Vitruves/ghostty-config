package pix

import (
	"hash/maphash"
	"image"
)

// Memo keeps what parts of a canvas looked like once drawn, so that a part
// drawn again the same way, over the same pixels, is copied back instead of
// rasterised again: a wall of cards draws the same cards on most frames.
//
// A part is known by a key, which must say everything its drawing depends
// on, and by the pixels under it before it is drawn, so the copy is exactly
// what drawing would have made. The first drawing is watched: one that
// reached past the bounds it was given is not kept, and is drawn again the
// next time, so a wrong bound costs time, never pixels.
//
// What was drawn in the last frame and this one is kept, nothing older:
// a part that stays on screen stays in the memo, one that leaves goes.
type Memo struct {
	cur, prev map[memoKey]*memoPart
	seed      maphash.Seed
	// What became of the parts drawn: copied back, drawn, or drawn past
	// their bounds and so not kept.
	Hits, Misses, Refused int
}

type memoKey struct {
	key   any
	r     image.Rectangle
	under uint64
}

type memoPart struct {
	pix []byte // the part's rows, one after the other
}

// NewMemo is an empty memo.
func NewMemo() *Memo {
	return &Memo{cur: map[memoKey]*memoPart{}, prev: map[memoKey]*memoPart{}, seed: maphash.MakeSeed()}
}

// Frame starts a frame: what the frame before last drew is let go.
func (mm *Memo) Frame() {
	mm.prev, mm.cur = mm.cur, map[memoKey]*memoPart{}
}

// Reset forgets everything, as when the canvas changes scale.
func (mm *Memo) Reset() {
	mm.cur, mm.prev = map[memoKey]*memoPart{}, map[memoKey]*memoPart{}
}

// Draw runs draw, or copies back what it drew before. x, y, w, h are the
// logical bounds of everything draw touches; key must be comparable and
// tell apart every way draw can come out.
func (mm *Memo) Draw(c *Canvas, key any, x, y, w, h float32, draw func()) {
	r := c.dev(x, y, w, h).Intersect(c.clip)
	if r.Empty() {
		draw()
		return
	}
	k := memoKey{key: key, r: r, under: mm.hash(c, r)}
	part, ok := mm.cur[k]
	if !ok {
		if part, ok = mm.prev[k]; ok {
			mm.cur[k] = part
		}
	}
	img := c.Img
	row := r.Dx() * 4
	if ok {
		mm.Hits++
		for y, i := r.Min.Y, 0; y < r.Max.Y; y, i = y+1, i+row {
			o := img.PixOffset(r.Min.X, y)
			copy(img.Pix[o:o+row], part.pix[i:i+row])
		}
		return
	}
	outer := c.touched
	touched := image.Rectangle{}
	c.touched = &touched
	draw()
	c.touched = outer
	c.touch(touched)
	if !touched.In(r) {
		mm.Refused++
		return
	}
	mm.Misses++
	part = &memoPart{pix: make([]byte, row*r.Dy())}
	for y, i := r.Min.Y, 0; y < r.Max.Y; y, i = y+1, i+row {
		o := img.PixOffset(r.Min.X, y)
		copy(part.pix[i:i+row], img.Pix[o:o+row])
	}
	mm.cur[k] = part
}

// hash sums the pixels of a part of the canvas.
func (mm *Memo) hash(c *Canvas, r image.Rectangle) uint64 {
	var h maphash.Hash
	h.SetSeed(mm.seed)
	row := r.Dx() * 4
	for y := r.Min.Y; y < r.Max.Y; y++ {
		o := c.Img.PixOffset(r.Min.X, y)
		h.Write(c.Img.Pix[o : o+row])
	}
	return h.Sum64()
}
