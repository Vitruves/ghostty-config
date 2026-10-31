package pix

import (
	"image"
	"runtime"
	"sync"
)

// Batch queues parts for a Memo, and drawing between them, and draws them
// in order with the parts the memo does not hold drawn side by side: the
// cards of a wall typed into or scrolled are all drawn anew.
//
// A part drawn side by side is drawn on a canvas of its own pixels, over a
// copy of what lies under it, and then copied in, so it comes out as drawn
// in order as long as nothing before it in the batch draws where it lies.
// Parts that overlap others, parts whose place something before them drew
// over, and parts that draw past their bounds are drawn in order on the
// picture, as Memo.Draw does. A part's draw may run on another goroutine:
// it must only read what is shared, and draw only on the canvas it is
// given. Faces from kimg are safe to share.
type Batch struct {
	mm   *Memo
	c    *Canvas
	jobs []batchJob
}

type batchJob struct {
	key        any
	x, y, w, h float32
	clip       image.Rectangle
	draw       func(*Canvas)
	op         func() // drawing between parts, done in order

	// Worked out before the parts are drawn.
	alone   bool // drawn side by side, its place untouched before it
	r       image.Rectangle
	k       memoKey
	part    *memoPart // held by the memo
	pix     []byte    // drawn side by side
	touched image.Rectangle
}

// minBatch is how many parts the memo must lack before drawing them side
// by side is worth the goroutines.
const minBatch = 3

// maxDrawers bounds the goroutines that draw parts.
const maxDrawers = 4

// Batch starts a batch of drawing on c. A memo has one batch, so the
// last must have been run.
func (mm *Memo) Batch(c *Canvas) *Batch {
	if mm.batch == nil {
		mm.batch = &Batch{mm: mm}
	}
	mm.batch.c = c
	mm.batch.jobs = mm.batch.jobs[:0]
	return mm.batch
}

// Draw queues a part, as Memo.Draw draws one, with draw given the canvas
// to draw on.
func (b *Batch) Draw(key any, x, y, w, h float32, draw func(*Canvas)) {
	b.jobs = append(b.jobs, batchJob{key: key, x: x, y: y, w: w, h: h, clip: b.c.clip, draw: draw})
}

// Do queues drawing on the canvas, done in its turn. It must leave the
// clip as it found it.
func (b *Batch) Do(op func()) {
	b.jobs = append(b.jobs, batchJob{op: op})
}

// Run draws what was queued.
func (b *Batch) Run() {
	mm, c := b.mm, b.c
	clip := c.clip
	defer func() { c.clip = clip }()

	// What the memo lacks of the parts that lie alone, drawn side by side
	// over a copy of what is under them.
	var placed []image.Rectangle
	var todo []int
	for i := range b.jobs {
		j := &b.jobs[i]
		if j.op != nil {
			continue
		}
		c.clip = j.clip
		j.r = c.dev(j.x, j.y, j.w, j.h).Intersect(j.clip)
		if j.r.Empty() {
			continue
		}
		alone := true
		for _, p := range placed {
			if p.Overlaps(j.r) {
				alone = false
				break
			}
		}
		placed = append(placed, j.r)
		if !alone {
			continue
		}
		j.alone = true
		j.k = memoKey{key: j.key, r: j.r, under: mm.hash(c, j.r)}
		if part, ok := mm.lookup(j.k); ok {
			j.part = part
			mm.letGoUnder(j.r)
			continue
		}
		mm.letGoUnder(j.r)
		// Taken now, while what was let go under it is at hand.
		j.pix = mm.buffer(j.r.Dx() * 4 * j.r.Dy())
		todo = append(todo, i)
	}
	if len(todo) < minBatch {
		// Too few: drawn on the picture in their turn.
		for _, i := range todo {
			mm.release(b.jobs[i].pix)
			b.jobs[i].pix = nil
		}
	} else {
		for _, i := range todo {
			j := &b.jobs[i]
			row := j.r.Dx() * 4
			for y, o := j.r.Min.Y, 0; y < j.r.Max.Y; y, o = y+1, o+row {
				copy(j.pix[o:o+row], c.Img.Pix[c.Img.PixOffset(j.r.Min.X, y):][:row])
			}
		}
		b.drawSideBySide(todo)
	}

	// In order: what lies alone and was drawn as it is is copied in; the
	// rest is drawn on the picture.
	var drawn []image.Rectangle // drawn in order, maybe past bounds
	for i := range b.jobs {
		j := &b.jobs[i]
		if j.op != nil {
			c.clip = clip
			drawn = append(drawn, watch(c, j.op))
			continue
		}
		c.clip = j.clip
		ok := j.alone
		for _, d := range drawn {
			if ok && d.Overlaps(j.r) {
				ok = false
			}
		}
		draw := j.draw
		if ok && j.part == nil && j.pix == nil {
			// Too few to draw side by side: drawn on the picture, under
			// the key worked out for it.
			drawn = append(drawn, mm.drawMiss(c, j.k, func() { draw(c) }))
			continue
		}
		if ok && j.part == nil && !j.touched.In(j.r) {
			// Drawn past its bounds: drawn again on the picture, where
			// it reaches as far as it does.
			ok = false
		}
		if !ok {
			if j.pix != nil {
				mm.release(j.pix)
			}
			drawn = append(drawn, mm.draw(c, j.key, j.x, j.y, j.w, j.h, func() { draw(c) }))
			continue
		}
		if j.part != nil {
			mm.Hits++
			mm.put(c, j.r, j.part.pix)
			continue
		}
		mm.Misses++
		mm.put(c, j.r, j.pix)
		c.touch(j.touched)
		mm.cur[j.k] = &memoPart{pix: j.pix, frame: mm.frame}
	}
	// Let go of what the drawing closed over.
	clear(b.jobs)
	b.jobs = b.jobs[:0]
	b.c = nil
}

// drawSideBySide draws the parts todo names, each on a canvas of its own.
func (b *Batch) drawSideBySide(todo []int) {
	n := min(runtime.GOMAXPROCS(0), maxDrawers, len(todo))
	canvases := make([]*Canvas, n)
	for w := range canvases {
		canvases[w] = b.mm.canvas(w, b.c)
	}
	var next sync.Mutex
	at := 0
	work := func(w int) {
		wc := canvases[w]
		for {
			next.Lock()
			i := at
			at++
			next.Unlock()
			if i >= len(todo) {
				return
			}
			b.drawPart(wc, &b.jobs[todo[i]])
		}
	}
	var wg sync.WaitGroup
	for w := 1; w < n; w++ {
		wg.Go(func() { work(w) })
	}
	work(0)
	wg.Wait()
}

// drawPart draws a part on wc, over the pixels of its own buffer.
func (b *Batch) drawPart(wc *Canvas, j *batchJob) {
	wc.Img = &image.RGBA{Pix: j.pix, Stride: j.r.Dx() * 4, Rect: j.r}
	wc.clip = j.clip
	j.touched = image.Rectangle{}
	wc.touched = &j.touched
	j.draw(wc)
	wc.touched = nil
	wc.Img = nil
}

// canvas is the w-th canvas for drawing parts of c: its scale, and a
// rasteriser of its own.
func (mm *Memo) canvas(w int, c *Canvas) *Canvas {
	for len(mm.drawers) <= w {
		mm.drawers = append(mm.drawers, &Canvas{})
	}
	wc := mm.drawers[w]
	wc.S, wc.TextScale, wc.whole = c.S, c.TextScale, c.bounds()
	return wc
}
