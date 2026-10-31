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
// a part that stays on screen stays in the memo, one that leaves goes. A
// part of the last frame goes as soon as this frame draws a part over the
// middle of where it was: parts sit side by side, so nothing else in this
// frame is likely to ask for it, and a wall drawn anew, as when typing or
// scrolling, takes the place of the last one instead of being kept beside
// it. (A part let go that is asked for after all is drawn again: it costs
// time, never pixels.)
type Memo struct {
	cur, prev map[memoKey]*memoPart
	seed      maphash.Seed
	frame     int
	// free holds the pixels of parts let go, for new parts to reuse: the
	// cards of a wall are all one size.
	free [][]byte
	// drawers are the canvases a Batch draws parts on side by side.
	drawers []*Canvas
	batch   *Batch
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
	pix   []byte // the part's rows, one after the other
	frame int    // the last frame that drew it
}

// maxFree bounds the pixels kept for reuse: parts are mostly let go one
// at a time as their place is drawn over, and taken again at once.
const maxFree = 8

// NewMemo is an empty memo.
func NewMemo() *Memo {
	return &Memo{cur: map[memoKey]*memoPart{}, prev: map[memoKey]*memoPart{}, seed: maphash.MakeSeed()}
}

// Frame starts a frame: what the frame before last drew is let go.
func (mm *Memo) Frame() {
	for _, part := range mm.prev {
		if part.frame < mm.frame {
			mm.release(part.pix)
		}
	}
	mm.frame++
	mm.prev, mm.cur = mm.cur, map[memoKey]*memoPart{}
}

// Reset forgets everything, as when the canvas changes scale.
func (mm *Memo) Reset() {
	mm.cur, mm.prev, mm.free = map[memoKey]*memoPart{}, map[memoKey]*memoPart{}, nil
}

// Draw runs draw, or copies back what it drew before. x, y, w, h are the
// logical bounds of everything draw touches; key must be comparable and
// tell apart every way draw can come out.
func (mm *Memo) Draw(c *Canvas, key any, x, y, w, h float32, draw func()) {
	mm.draw(c, key, x, y, w, h, draw)
}

// draw is Draw, and returns what it may have drawn.
func (mm *Memo) draw(c *Canvas, key any, x, y, w, h float32, draw func()) image.Rectangle {
	r := c.dev(x, y, w, h).Intersect(c.clip)
	if r.Empty() {
		return watch(c, draw)
	}
	k := memoKey{key: key, r: r, under: mm.hash(c, r)}
	part, ok := mm.lookup(k)
	mm.letGoUnder(r)
	if ok {
		mm.Hits++
		mm.put(c, r, part.pix)
		return r
	}
	return mm.drawMiss(c, k, draw)
}

// drawMiss draws a part the memo lacks, and keeps it unless it drew past
// its bounds.
func (mm *Memo) drawMiss(c *Canvas, k memoKey, draw func()) image.Rectangle {
	r := k.r
	touched := watch(c, draw)
	if !touched.In(r) {
		mm.Refused++
		return touched
	}
	mm.Misses++
	part := &memoPart{pix: mm.buffer(r.Dx() * 4 * r.Dy()), frame: mm.frame}
	img := c.Img
	row := r.Dx() * 4
	for y, i := r.Min.Y, 0; y < r.Max.Y; y, i = y+1, i+row {
		o := img.PixOffset(r.Min.X, y)
		copy(part.pix[i:i+row], img.Pix[o:o+row])
	}
	mm.cur[k] = part
	return touched
}

// watch runs draw and returns what it may have drawn, which the canvas
// records as drawn as well.
func watch(c *Canvas, draw func()) image.Rectangle {
	outer := c.touched
	touched := image.Rectangle{}
	c.touched = &touched
	draw()
	c.touched = outer
	c.touch(touched)
	return touched
}

// lookup finds a part drawn this frame or the last, and keeps it for the
// next.
func (mm *Memo) lookup(k memoKey) (*memoPart, bool) {
	part, ok := mm.cur[k]
	if !ok {
		if part, ok = mm.prev[k]; ok {
			mm.cur[k] = part
		}
	}
	if ok {
		part.frame = mm.frame
	}
	return part, ok
}

// put copies a part's rows onto the canvas at r.
func (mm *Memo) put(c *Canvas, r image.Rectangle, pix []byte) {
	row := r.Dx() * 4
	for y, i := r.Min.Y, 0; y < r.Max.Y; y, i = y+1, i+row {
		o := c.Img.PixOffset(r.Min.X, y)
		copy(c.Img.Pix[o:o+row], pix[i:i+row])
	}
}

// letGoUnder lets go of the parts of the last frame whose middle r covers
// and that this frame has not drawn. Their edges may reach under r: the
// highlighted card, with its shadow, reaches over its neighbours' margins.
func (mm *Memo) letGoUnder(r image.Rectangle) {
	for k, part := range mm.prev {
		mid := k.r.Min.Add(k.r.Max).Div(2)
		if part.frame < mm.frame && mid.In(r) {
			delete(mm.prev, k)
			mm.release(part.pix)
		}
	}
}

// release keeps pixels let go for a new part. Past maxFree the smallest
// buffer kept goes: the highlighted card is drawn larger than the others,
// and drawn anew whenever the highlight moves.
func (mm *Memo) release(pix []byte) {
	if len(mm.free) < maxFree {
		mm.free = append(mm.free, pix)
		return
	}
	least := 0
	for i, b := range mm.free {
		if cap(b) < cap(mm.free[least]) {
			least = i
		}
	}
	if cap(pix) > cap(mm.free[least]) {
		mm.free[least] = pix
	}
}

// buffer is n bytes for a new part, from the smallest buffer let go that
// is big enough.
func (mm *Memo) buffer(n int) []byte {
	best := -1
	for i, b := range mm.free {
		if cap(b) >= n && (best < 0 || cap(b) < cap(mm.free[best])) {
			best = i
		}
	}
	if best < 0 {
		return make([]byte, n)
	}
	b := mm.free[best]
	mm.free[best] = mm.free[len(mm.free)-1]
	mm.free = mm.free[:len(mm.free)-1]
	return b[:n]
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
