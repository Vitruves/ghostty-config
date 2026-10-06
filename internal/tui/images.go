package tui

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
	"github.com/vitruves/ghostty-config/internal/kimg"
)

// Pictures. When the terminal speaks the Kitty graphics protocol the wall of
// themes is made of real images, with rounded corners and soft shadows, in
// Google Sans. The rest stays text. Where the terminal does not, or the cell
// size is unknown, the same cards are drawn from cells.

// imageStore remembers which pictures the terminal already holds, queues the
// ones it does not, and writes them out before the frame that uses them.
type imageStore struct {
	ids   map[string]uint32
	used  map[string]int
	tick  int
	next  uint32
	queue strings.Builder
	out   io.Writer
	// dumpDir, when set, also writes each picture there as <id>.png: a way
	// to look at a frame offline, with tools/ansi2png.py.
	dumpDir string
}

func newImageStore(out io.Writer) *imageStore {
	return &imageStore{ids: map[string]uint32{}, used: map[string]int{}, next: 0x000100, out: out}
}

// maxImages bounds what the terminal holds: past it the oldest are freed.
const maxImages = 360

func (s *imageStore) touch(key string) {
	s.tick++
	s.used[key] = s.tick
}

// evict frees the least recently used pictures once there are too many.
func (s *imageStore) evict() {
	if len(s.ids) <= maxImages {
		return
	}
	type kv struct {
		key  string
		used int
	}
	var all []kv
	for k := range s.ids {
		all = append(all, kv{k, s.used[k]})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].used < all[j].used })
	for _, e := range all[:maxImages/4] {
		id := s.ids[e.key]
		s.queue.WriteString(kimg.Delete(id))
		delete(s.ids, e.key)
		delete(s.used, e.key)
	}
}

// ensure returns the number of the picture with this key, drawing and
// queueing it first if it has not been sent.
func (s *imageStore) ensure(key string, cols, rows int, draw func() []byte) uint32 {
	if id, ok := s.ids[key]; ok {
		s.touch(key)
		return id
	}
	id := s.next
	s.next++
	s.ids[key] = id
	s.touch(key)
	data := draw()
	s.dump(id, data)
	s.queue.WriteString(kimg.Transmit(id, data, cols, rows))
	s.evict()
	return id
}

// flush writes what is queued, in pieces the terminal reads whole.
func (s *imageStore) flush() {
	if s.queue.Len() == 0 || s.out == nil {
		return
	}
	data := s.queue.String()
	s.queue.Reset()
	_, _ = io.WriteString(s.out, data)
}

// reset forgets everything and frees it in the terminal.
func (s *imageStore) reset() {
	s.ids = map[string]uint32{}
	s.used = map[string]int{}
	s.queue.Reset()
	if s.out != nil {
		_, _ = io.WriteString(s.out, kimg.DeleteAll())
	}
}

// supportsImages reads the environment: Ghostty and Kitty draw Unicode
// placeholders correctly, in full colour, and outside a multiplexer.
func supportsImages() bool {
	if os.Getenv("TMUX") != "" || os.Getenv("STY") != "" {
		return false
	}
	if c := strings.ToLower(os.Getenv("COLORTERM")); c != "truecolor" && c != "24bit" {
		return false
	}
	program := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	return program == "ghostty" || os.Getenv("KITTY_WINDOW_ID") != "" || strings.Contains(strings.ToLower(os.Getenv("TERM")), "kitty") || strings.Contains(strings.ToLower(os.Getenv("TERM")), "ghostty")
}

// imagesOn is whether pictures are drawn this frame.
func (m *Model) imagesOn() bool {
	return m.opts.Images && m.imgs != nil && m.cellW > 0 && m.cellH > 0
}

// refreshCell reads the cell size again, as after a resize; a change throws
// away the pictures drawn for the old one.
func (m *Model) refreshCell() {
	w, h := cellPixels()
	if m.opts.CellW > 0 && m.opts.CellH > 0 {
		w, h = m.opts.CellW, m.opts.CellH
	}
	if w != m.cellW || h != m.cellH {
		m.cellW, m.cellH = w, h
		if m.imgs != nil {
			m.imgs.reset()
		}
	}
}

// pictureRows turns a picture into the text rows that show it.
func pictureRows(id uint32, cols, rows int, bg string) []string {
	r, g, b := kimg.IDColour(id)
	idc := "#" + hex2(r) + hex2(g) + hex2(b)
	out := make([]string, rows)
	for i := range out {
		out[i] = on(idc, bg).Render(kimg.PlaceholderRow(i, cols))
	}
	return out
}

func hex2(v uint8) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>4], digits[v&15]})
}

// themeSpec describes a theme for a card.
func (m *Model) themeSpec(t *ghostty.Theme) kimg.CardSpec {
	fg := t.Foreground()
	spec := kimg.CardSpec{
		BG: t.Background(), FG: fg,
		Accent: color.Normalize(t.Get("cursor-color"), color.Normalize(t.Get(paletteKey(12)), fg)),
	}
	for i := 0; i < 16; i++ {
		spec.Pal[i] = color.Normalize(t.Get(paletteKey(i)), fg)
	}
	return spec
}

// themeTile is one card of the wall, cols by rows cells, drawn as a picture
// and laid out as the placeholder cells that show it. The picture depends on
// the screen it sits on: its shadow is lighter on a light one, and the ring
// round the highlighted card is the interface's accent.
func (m *Model) themeTile(t *ghostty.Theme, cols, rows int, selected bool, ui chrome) []string {
	spec := m.themeSpec(t)
	spec.Selected, spec.Ring, spec.Screen = selected, ui.accent, ui.bg
	key := strings.Join([]string{"tile", t.Name, spec.BG, spec.FG, spec.Accent, strings.Join(spec.Pal[:], ""), boolKey(selected), ui.accent, boolKey(color.IsDark(ui.bg)), itoa(cols), itoa(rows)}, "|")
	id := m.imgs.ensure(key, cols, rows, func() []byte {
		return kimg.Tile(cols, rows, m.cellW, m.cellH, spec).PNG()
	})
	return pictureRows(id, cols, rows, ui.paint())
}

func boolKey(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// ImagesSupported reports whether this terminal can show the pictures.
func ImagesSupported() bool { return supportsImages() }

func (s *imageStore) dump(id uint32, data []byte) {
	if s.dumpDir == "" {
		return
	}
	_ = os.MkdirAll(s.dumpDir, 0o755)
	_ = os.WriteFile(filepath.Join(s.dumpDir, strconv.Itoa(int(id))+".png"), data, 0o644)
}
