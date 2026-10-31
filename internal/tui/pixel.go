package tui

import (
	"fmt"
	"hash/maphash"
	"image"
	stdcolor "image/color"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/image/font"

	"github.com/vitruves/ghostty-config/internal/ghostty"
	"github.com/vitruves/ghostty-config/internal/kimg"
	"github.com/vitruves/ghostty-config/internal/pix"
)

// The picture interface. Where the terminal speaks the Kitty graphics
// protocol, the whole screen is drawn as one picture, in real type with real
// controls, and sent as one image per row of cells. A row whose pixels did not
// change is not sent again, so moving the highlight costs the two or three
// rows it touches. The text interface stays for every other terminal.
//
// The pictures are shown through Unicode placeholders, like the cards of the
// text interface: each row of the screen is a row of placeholder cells naming
// that row's image, so the program that draws the screen only ever writes
// text, and a row that changes is a row of text that changes.

// pixUnit is how many logical units a row of cells is tall at the normal
// size: the layout is written in these units and scales with the terminal's
// font, and with the size chosen for the interface.
const pixUnit = 22

// pixText is how much larger than the layout's own sizes the text is drawn.
const pixText = 1.1

type pixEngine struct {
	cw, ch  int // a cell in device pixels, exactly
	viaFile bool
	seq     int
	seed    maphash.Seed

	// What the last frame laid out, for the keys that page and move.
	cols     int // cards per row on the wall
	cardRows int // rows of cards on screen
	listRows int // rows of a list on screen

	// Scroll positions, in logical units.
	galTop, listTop float32

	// Pictures too slow to draw on every frame, by what they show.
	pics map[string]*image.RGBA

	// The cards of the wall as last drawn: a frame draws mostly the same
	// ones, and copying one back is far cheaper than drawing it.
	cards *pix.Memo
	// The canvas of the last frame, drawn over by the next.
	canvas *pix.Canvas
	// The placeholder text of each row's picture, by the picture's number.
	rowText map[rowKey]string
}

type rowKey struct {
	id   uint32
	cols int
	bg   string
}

func newPixEngine(o Options) *pixEngine {
	e := &pixEngine{cw: o.PixCell[0], ch: o.PixCell[1], seed: maphash.MakeSeed(), pics: map[string]*image.RGBA{}, cards: pix.NewMemo()}
	e.viaFile = filesReachTerminal(o.ImgOut)
	// Files an earlier run left behind, because the terminal never read
	// them or the run was killed, would pile up in the temporary directory
	// for good: sweep them out, without holding up the first frame.
	sweepOnce.Do(func() { go sweepTempFiles(os.TempDir(), staleTempFile) })
	return e
}

var sweepOnce sync.Once

// tempPrefix starts the name of every file the pixels go through. Ghostty
// only reads a temporary file whose name says it is meant for it.
const tempPrefix = "tty-graphics-protocol-ghostty-config-"

// staleTempFile is how old a file of ours must be to count as abandoned.
// The terminal reads one within milliseconds of the row that names it.
const staleTempFile = 5 * time.Minute

// filesReachTerminal is whether the pixels can go through temporary files
// the terminal reads and deletes. A temporary file costs nothing on the pty,
// but only a terminal on this machine reads it, and only when the pictures
// are written to that terminal itself: written anywhere else, as in tests,
// nothing would ever read the files or delete them.
func filesReachTerminal(out io.Writer) bool {
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return false
	}
	if out == nil {
		out = os.Stdout
	}
	f, ok := out.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}

// sweepTempFiles removes the files of ours in dir older than age.
func sweepTempFiles(dir string, age time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-age)
	for _, ent := range entries {
		if !strings.HasPrefix(ent.Name(), tempPrefix) || !ent.Type().IsRegular() {
			continue
		}
		if info, err := ent.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, ent.Name()))
		}
	}
}

// removeTempFiles removes what this run left in the temporary directory:
// on the way out, files the terminal has not read yet never will be.
func (e *pixEngine) removeTempFiles() {
	if e == nil || e.seq == 0 {
		return
	}
	paths, _ := filepath.Glob(filepath.Join(os.TempDir(), fmt.Sprintf("%s%d-*", tempPrefix, os.Getpid())))
	for _, p := range paths {
		_ = os.Remove(p)
	}
}

// pixelOn is whether this frame is drawn as a picture.
func (m *Model) pixelOn() bool {
	return m.px != nil && m.imgs != nil && m.px.cw > 0 && m.px.ch > 0 && m.width > 0 && m.height > 0
}

// askCellSize is sent after every resize. The cell only changes with the
// font size, and only the terminal knows it exactly: the window's pixel size
// includes whatever part of a cell is left over at the edges, which changes
// with the window, so a size worked out from it is often a pixel off, and
// rows drawn a pixel off are stretched by the terminal into a smear.
const askCellSize = "\x1b[16t"

// setPixCell takes the cell size the terminal reported. A new size throws
// the pictures away; they are drawn and sent again for it.
func (m *Model) setPixCell(cw, ch int) bool {
	e := m.px
	if cw <= 0 || ch <= 0 || (cw == e.cw && ch == e.ch) {
		return false
	}
	e.cw, e.ch = cw, ch
	e.forget()
	return true
}

// forget lets go of what the engine keeps between frames.
func (e *pixEngine) forget() {
	e.pics = map[string]*image.RGBA{}
	e.cards.Reset()
	e.canvas = nil
	e.rowText = nil
}

// tempFile writes pixels where the terminal can read them, or says it could
// not, and they go inline instead.
func (e *pixEngine) tempFile(p []byte) string {
	e.seq++
	return e.tempFileNumbered(e.seq, p)
}

// tempFileNumbered is tempFile under a number already taken from e.seq.
func (e *pixEngine) tempFileNumbered(seq int, p []byte) string {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("%s%d-%d", tempPrefix, os.Getpid(), seq))
	if err := os.WriteFile(path, p, 0o600); err != nil {
		return ""
	}
	return path
}

// cached returns a picture drawn once per key.
func (e *pixEngine) cached(key string, draw func() *image.RGBA) *image.RGBA {
	if img, ok := e.pics[key]; ok {
		return img
	}
	if len(e.pics) > 48 {
		e.pics = map[string]*image.RGBA{}
	}
	img := draw()
	e.pics[key] = img
	return img
}

// renderPixel draws the frame and returns the rows of placeholders that
// show it; the rows whose pixels changed are queued for the terminal.
func (m *Model) renderPixel() string {
	e := m.px
	W := m.width * e.cw
	c := pix.Renew(e.canvas, m.width*e.cw, m.height*e.ch, m.pixScale())
	c.TextScale = pixText
	e.canvas = c
	e.cards.Frame()
	m.drawScreen(c)

	ui := m.panelChrome()
	var file func([]byte) string
	if e.viaFile {
		file = e.tempFile
	}
	rows := make([]string, m.height)
	keys := make([]string, m.height)
	bands := make([][]byte, m.height)
	stride := c.Img.Stride
	for r := 0; r < m.height; r++ {
		bands[r] = c.Img.Pix[r*e.ch*stride : (r+1)*e.ch*stride]
	}
	for r, h := range hashBands(e.seed, bands) {
		keys[r] = fmt.Sprintf("band|%d|%d|%x", W, e.ch, h)
	}
	ready := e.readyBands(m.imgs, keys, bands)
	for r := 0; r < m.height; r++ {
		band := bands[r]
		id := m.imgs.ensureEncoded(keys[r], W, e.ch, m.width, 1, func() rgbaPayload {
			if pl, ok := ready[keys[r]]; ok {
				return pl
			}
			return encodeRGBA(band, file)
		})
		rows[r] = e.placeholders(id, m.width, ui.paint())
	}
	return strings.Join(rows, "\n")
}

// maxHashers bounds the goroutines hashBands starts: past four, waking
// another thread costs about what it saves.
const maxHashers = 4

// hashBands sums the rows of a frame side by side: every frame hashes
// every row, ten megabytes at 120x34 cells, and alone that is most of a
// frame that changes nothing.
func hashBands(seed maphash.Seed, bands [][]byte) []uint64 {
	sums := make([]uint64, len(bands))
	workers := min(runtime.GOMAXPROCS(0), maxHashers, len(bands))
	if workers < 2 {
		for r, b := range bands {
			sums[r] = maphash.Bytes(seed, b)
		}
		return sums
	}
	// Every worker takes every workers-th row, rows being one size; this
	// goroutine is one of them, which spares waking a thread.
	stripe := func(w int) {
		for r := w; r < len(bands); r += workers {
			sums[r] = maphash.Bytes(seed, bands[r])
		}
	}
	var wg sync.WaitGroup
	for w := 1; w < workers; w++ {
		wg.Go(func() { stripe(w) })
	}
	stripe(0)
	wg.Wait()
	return sums
}

// readyBands readies the rows the terminal does not hold, side by side:
// compressing a row, or writing it to its file, is most of what sending it
// costs, and a first frame sends every row. The files are numbered in the
// order of the rows, as they would be one at a time.
func (e *pixEngine) readyBands(s *imageStore, keys []string, bands [][]byte) map[string]rgbaPayload {
	type job struct {
		key  string
		band []byte
		seq  int
	}
	var jobs []job
	seen := map[string]bool{}
	for r, k := range keys {
		if _, held := s.ids[k]; held || seen[k] {
			continue
		}
		seen[k] = true
		jobs = append(jobs, job{key: k, band: bands[r]})
	}
	if len(jobs) < 2 {
		// A row alone is readied as fast where it is sent.
		return nil
	}
	if e.viaFile {
		for i := range jobs {
			e.seq++
			jobs[i].seq = e.seq
		}
	}
	out := make([]rgbaPayload, len(jobs))
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < min(runtime.GOMAXPROCS(0), len(jobs)); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				j := jobs[i]
				var file func([]byte) string
				if e.viaFile {
					file = func(p []byte) string { return e.tempFileNumbered(j.seq, p) }
				}
				out[i] = encodeRGBA(j.band, file)
			}
		}()
	}
	for i := range jobs {
		next <- i
	}
	close(next)
	wg.Wait()
	ready := make(map[string]rgbaPayload, len(jobs))
	for i, j := range jobs {
		ready[j.key] = out[i]
	}
	return ready
}

// placeholders is pictureRows for a picture one row tall, kept: the text
// is the same every time, and building it again for every row is a tenth
// of a frame that changes nothing.
func (e *pixEngine) placeholders(id uint32, cols int, bg string) string {
	k := rowKey{id, cols, bg}
	if s, ok := e.rowText[k]; ok {
		return s
	}
	if e.rowText == nil || len(e.rowText) >= 1024 {
		e.rowText = map[rowKey]string{}
	}
	s := pictureRows(id, cols, 1, bg)[0]
	e.rowText[k] = s
	return s
}

// pixCanvas is a canvas the size of the window. Its scale is the cell's at
// the size chosen for the interface: another size changes what every row
// shows, never the cell, so the rows are sent again by their pixels alone
// and nothing held by the terminal needs throwing away.
func (m *Model) pixCanvas() *pix.Canvas {
	e := m.px
	c := pix.New(m.width*e.cw, m.height*e.ch, m.pixScale())
	c.TextScale = pixText
	return c
}

// pixScale is how many device pixels a logical unit is.
func (m *Model) pixScale() float32 { return float32(m.px.ch) / pixUnit * m.uiScale() }

// cardKey is everything a card of the wall is drawn from, but the pixels
// under it.
type cardKey struct {
	name                     string
	spec                     kimg.CardSpec
	sel, applied, fav        bool
	fg, text2, muted, accent stdcolor.NRGBA
	aa, mono, small          font.Face
	s, ts, x, y, w, h        float32
}

// pxCardMemo is pxCard through the memo of cards. The bounds cover the
// shadow and ring of a highlighted card and the two lines under each, and
// keep clear of the shadow a highlighted neighbour casts into the gap, so
// moving the highlight draws again only the cards it leaves and lands on.
// The memo checks the bounds.
func (m *Model) pxCardMemo(c *pix.Canvas, p pxPal, f pxFonts, t *ghostty.Theme, x, y, w, h float32, sel bool) {
	k := cardKey{
		name: t.Name, spec: m.themeSpec(t),
		sel: sel, applied: t.Name == m.applied, fav: m.state.IsFavorite(t.Name),
		fg: p.fg, text2: p.text2, muted: p.muted, accent: p.accent,
		aa: f.cardAa, mono: f.cardMono, small: f.small,
		s: c.S, ts: c.TextScale, x: x, y: y, w: w, h: h,
	}
	side, top := float32(3), float32(3)
	if sel {
		side, top = 16, 8
	}
	m.px.cards.Draw(c, k, x-side, y-top, w+2*side, h+top+52, func() { m.pxCard(c, p, f, t, x, y, w, h, sel) })
}

// pxRegion registers a logical rectangle as clickable, as the cells whose
// centres it covers.
func (m *Model) pxRegion(c *pix.Canvas, x, y, w, h float32, kind hitKind, index int, name string) {
	e := m.px
	s := c.S
	x0 := int(x*s/float32(e.cw) + 0.5)
	x1 := int((x+w)*s/float32(e.cw) + 0.5)
	y0 := int(y*s/float32(e.ch) + 0.5)
	y1 := int((y+h)*s/float32(e.ch) + 0.5)
	if y1 <= y0 {
		y1 = y0 + 1
	}
	if x1 <= x0 {
		x1 = x0 + 1
	}
	m.addRegion(x0, y0, x1-x0, y1-y0, kind, index, name)
}
