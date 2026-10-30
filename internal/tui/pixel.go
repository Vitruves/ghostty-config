package tui

import (
	"fmt"
	"hash/maphash"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/term"

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
}

func newPixEngine(o Options) *pixEngine {
	e := &pixEngine{cw: o.PixCell[0], ch: o.PixCell[1], seed: maphash.MakeSeed(), pics: map[string]*image.RGBA{}}
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
	e.pics = map[string]*image.RGBA{}
	return true
}

// tempFile writes pixels where the terminal can read them, or says it could
// not, and they go inline instead.
func (e *pixEngine) tempFile(p []byte) string {
	e.seq++
	path := filepath.Join(os.TempDir(), fmt.Sprintf("%s%d-%d", tempPrefix, os.Getpid(), e.seq))
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
	c := m.pixCanvas()
	m.drawScreen(c)

	ui := m.panelChrome()
	var file func([]byte) string
	if e.viaFile {
		file = e.tempFile
	}
	rows := make([]string, m.height)
	stride := c.Img.Stride
	for r := 0; r < m.height; r++ {
		band := c.Img.Pix[r*e.ch*stride : (r+1)*e.ch*stride]
		key := fmt.Sprintf("band|%d|%d|%x", W, e.ch, maphash.Bytes(e.seed, band))
		id := m.imgs.ensureRGBA(key, band, W, e.ch, m.width, 1, file)
		rows[r] = pictureRows(id, m.width, 1, ui.paint())[0]
	}
	return strings.Join(rows, "\n")
}

// pixCanvas is a canvas the size of the window. Its scale is the cell's at
// the size chosen for the interface: another size changes what every row
// shows, never the cell, so the rows are sent again by their pixels alone
// and nothing held by the terminal needs throwing away.
func (m *Model) pixCanvas() *pix.Canvas {
	e := m.px
	c := pix.New(m.width*e.cw, m.height*e.ch, float32(e.ch)/pixUnit*m.uiScale())
	c.TextScale = pixText
	return c
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
