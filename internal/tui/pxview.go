package tui

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"strconv"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi/kitty"
	"golang.org/x/image/font"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
	"github.com/vitruves/ghostty-config/internal/kimg"
	"github.com/vitruves/ghostty-config/internal/pix"
)

// The screens of the picture interface. From the top: the name of the tool,
// the sections and the search field; a column on the left that says where
// the highlight is; the body; and a footer that says what the highlighted
// thing is, with the keys and the one action Enter takes. Nothing is framed:
// a group is told by a title and by space, the highlight by a quiet ground,
// and the accent marks only what is on.

// pxPal is the interface's colours as the canvas wants them.
type pxPal struct {
	ui                                              chrome
	bg, fg, text2, muted, accent, onAccent          stdcolor.NRGBA
	field, rowSel, railSel, keycap, track, hairline stdcolor.NRGBA
}

func newPxPal(ui chrome) pxPal {
	h := func(s string) stdcolor.NRGBA { return pix.Hex(s, 1) }
	// tone is a ground between the text and the background, t of the way to
	// the background; high contrast takes it further from it.
	k := 1.0
	if ui.high {
		k = 1.9
	}
	tone := func(t float64) stdcolor.NRGBA { return h(color.Blend(ui.fg, ui.bg, 1-(1-t)*k)) }
	on := ui.bg
	if color.Contrast(ui.fg, ui.accent) > color.Contrast(ui.bg, ui.accent) {
		on = ui.fg
	}
	return pxPal{
		ui: ui,
		bg: h(ui.bg), fg: h(ui.fg), text2: tone(0.18), muted: h(ui.muted),
		accent: h(ui.accent), onAccent: h(on),
		field:    tone(0.94),
		rowSel:   tone(0.93),
		railSel:  tone(0.90),
		keycap:   tone(0.88),
		track:    tone(0.83),
		hairline: tone(0.90),
	}
}

// pxGeo is where the parts of the screen sit, in logical units.
type pxGeo struct {
	W, H    float32
	M, T    float32 // margins: left and right, top
	railX   float32 // the column on the left; railW is 0 when there is none
	railW   float32
	bodyX   float32
	bodyW   float32
	bodyY   float32
	footY   float32
	viewEnd float32 // the bottom of what the body may draw
}

func pxLayout(c *pix.Canvas) pxGeo {
	g := pxGeo{W: c.W(), H: c.H(), M: 16, T: 6}
	g.railX = g.M
	if g.W >= 820 {
		g.railW = 188
		g.bodyX = g.M + 212
	} else {
		g.bodyX = g.M
	}
	g.bodyW = g.W - g.M - g.bodyX
	g.bodyY = g.T + 60
	g.footY = g.H - 50
	g.viewEnd = g.footY - 10
	return g
}

// Fonts, by role.
type pxFonts struct {
	title, nav, navB, row, rowB, body, small, smallB, caps, h1, h2, h3, mono, monoS, cardAa, cardMono font.Face
}

func pxFaces(c *pix.Canvas) pxFonts {
	return pxFonts{
		title: c.Face(false, true, 15), nav: c.Face(false, false, 14), navB: c.Face(false, true, 14),
		row: c.Face(false, false, 14), rowB: c.Face(false, true, 14), body: c.Face(false, false, 13.5),
		small: c.Face(false, false, 12.5), smallB: c.Face(false, true, 12.5), caps: c.Face(false, true, 11), h1: c.Face(false, true, 22),
		h2: c.Face(false, true, 15), h3: c.Face(false, true, 18), mono: c.Face(true, false, 12.5),
		monoS: c.Face(true, false, 11.5), cardAa: c.Face(false, true, 22), cardMono: c.Face(true, false, 10),
	}
}

// drawScreen paints the whole frame.
func (m *Model) drawScreen(c *pix.Canvas) {
	p := newPxPal(m.panelChrome())
	f := pxFaces(c)
	c.Clear(p.bg)
	switch m.overlay {
	case overlayWelcome:
		m.pxWelcome(c, p, f)
		return
	case overlayLayout:
		m.pxChooser(c, p, f)
		return
	}
	g := pxLayout(c)
	m.profile().drawPixel(m, c, p, f, g)
	if m.overlay != overlayNone {
		m.pxOverlay(c, p, f, g)
	}
}

// --- small pieces ------------------------------------------------------------

// caps writes a label in spaced capitals.
func pxCaps(c *pix.Canvas, f font.Face, x, y float32, s string, col stdcolor.Color) {
	for _, r := range strings.ToUpper(s) {
		x += c.Text(f, x, y, string(r), col) + 0.9
	}
}

// mid is the baseline that centres text of size px in a box.
func mid(y, h, px float32) float32 { return y + h/2 + px*0.36 }

// keyIcon draws a key's label in a keycap: arrows from the mono font, the
// control and return keys as small strokes since no embedded font has them.
func pxKeyLabel(c *pix.Canvas, f font.Face, x, base float32, key string, col stdcolor.Color, draw bool) float32 {
	w := float32(0)
	rest := key
	if strings.HasPrefix(rest, "^") || strings.HasPrefix(strings.ToLower(rest), "ctrl+") {
		// ⌃: a caret.
		if draw {
			c.Line(x+w+1, base-4, x+w+4, base-8, 1.3, col)
			c.Line(x+w+4, base-8, x+w+7, base-4, 1.3, col)
		}
		w += 9
		rest = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(rest, "^"), "Ctrl+"), "ctrl+")
		rest = strings.ToUpper(rest)
	}
	rest = strings.NewReplacer("⇧", "Shift ", "⌫", "Bksp", "Enter", "Enter").Replace(rest)
	if draw {
		w += c.Text(f, x+w, base, rest, col)
	} else {
		w += c.Width(f, rest)
	}
	return w
}

// pxReturn draws ⏎.
func pxReturn(c *pix.Canvas, x, base float32, col stdcolor.Color) float32 {
	c.Line(x+9, base-10, x+9, base-4, 1.3, col)
	c.Line(x+9, base-4, x+1.5, base-4, 1.3, col)
	c.Line(x+1, base-4, x+4, base-7, 1.3, col)
	c.Line(x+1, base-4, x+4, base-1, 1.3, col)
	return 11
}

// pxMagnifier draws the search glass.
func pxMagnifier(c *pix.Canvas, x, y float32, col stdcolor.Color) {
	c.Ring(x, y, 10, 10, 5, 1.6, col)
	c.Line(x+8.5, y+8.5, x+12.5, y+12.5, 1.8, col)
}

// pxSwatch is a theme's ground with its cursor colour, the footer's chip.
func pxSwatch(c *pix.Canvas, t *ghostty.Theme, x, y, size float32) {
	c.Rect(x, y, size, size, size*0.23, pix.Hex(t.Background(), 1))
	c.Ring(x, y, size, size, size*0.23, 1, pix.Hex(t.Foreground(), 0.12))
	cur := color.Normalize(t.Get("cursor-color"), t.Foreground())
	c.Rect(x+size/2-2.5, y+size/2-7, 5, 14, 2, pix.Hex(cur, 1))
}

// pxDot is the accent mark of a changed or current value.
func pxDot(c *pix.Canvas, cx, cy float32, col stdcolor.Color) { c.Circle(cx, cy, 3, col) }

// displayValue shortens a config word for a control.
func displayValue(v string) string {
	v = strings.TrimPrefix(v, "macos-")
	return strings.NewReplacer("-", " ", "_", " ").Replace(v)
}

// changed is whether a setting differs from Ghostty's default.
func (m *Model) changed(s *setting) bool {
	if s == nil || s.fallback == "" {
		return false
	}
	return !strings.EqualFold(s.get(m), s.fallback)
}

// --- header --------------------------------------------------------------------

func (m *Model) pxHeader(c *pix.Canvas, p pxPal, f pxFonts, g pxGeo) {
	base := g.T + 22
	fieldW := clampF(g.W*0.26, 220, 300)
	fieldX := g.W - g.M - fieldW

	navX := g.M
	navW := float32(0)
	for _, name := range groups {
		navW += c.Width(f.nav, name) + 24
	}
	if g.M+170+navW < fieldX-16 {
		c.Text(f.title, g.M, base, "ghostty-config", p.fg)
		navX = g.M + 170
	}
	searching := m.cmd == nil && strings.TrimSpace(m.prompt.Value()) != ""
	x := navX
	for i, name := range groups {
		w := c.Width(f.nav, name)
		if x+w > fieldX-12 {
			break
		}
		if i == m.group && !searching {
			c.Text(f.navB, x, base, name, p.fg)
			c.Rect(x, g.T+33, c.Width(f.navB, name), 2, 1, p.accent)
		} else {
			c.Text(f.nav, x, base, name, p.muted)
		}
		m.pxRegion(c, x-6, g.T, w+12, 36, hitGroup, i, name)
		x += w + 24
	}

	// The search field: what is typed after the command word, or the slot
	// being edited.
	fy := g.T + 2
	c.Rect(fieldX, fy, fieldW, 34, 9, p.field)
	m.pxRegion(c, fieldX, fy, fieldW, 34, hitPrompt, 0, "")
	tb := mid(fy, 34, 13)
	if m.editing {
		v := m.cur.Colors[m.editKey]
		tx := fieldX + 12
		if color.IsHex(v) {
			c.Rect(tx, fy+10, 14, 14, 3, pix.Hex(v, 1))
			tx += 22
		}
		tx += c.Text(f.small, tx, tb, ghostty.Label(m.editKey)+"  ", p.fg)
		label := v
		if m.editText != "" {
			label = "#" + m.editText
		}
		w := c.Text(f.mono, tx, tb, label, p.accent)
		c.Rect(tx+w+1, fy+9, 1.5, 16, 0, p.accent)
		return
	}
	pxMagnifier(c, fieldX+12, fy+11, p.muted)
	tx := fieldX + 34
	value := m.prompt.Value()
	prefix := ""
	if m.cmd != nil {
		// The command word reads as a scope, not as typed text.
		word, arg, _ := strings.Cut(value, " ")
		prefix, value = title(word), arg
	}
	room := fieldW - (tx - fieldX) - 34
	if prefix != "" && m.cmd.name != "theme" {
		pw := c.Text(f.small, tx, tb, prefix, p.muted)
		tx += pw + 6
		room -= pw + 6
	}
	pos := len([]rune(value))
	if m.cmd == nil {
		pos = m.prompt.Position()
	}
	if value == "" {
		ph := "Search " + itoa(len(m.commands())) + " options"
		if m.cmd != nil && m.cmd.name == "theme" {
			ph = "Search " + itoa(len(m.lib.Themes)) + " themes"
		} else if m.cmd != nil {
			ph = "filter"
		}
		c.Rect(tx-1, fy+9, 1.5, 16, 0, p.accent)
		c.Text(f.small, tx+3, tb, c.Fit(f.small, ph, room), p.muted)
	} else {
		shown := c.Fit(f.small, value, room)
		c.Text(f.small, tx, tb, shown, p.fg)
		r := []rune(value)
		if pos > len(r) {
			pos = len(r)
		}
		cx := tx + c.Width(f.small, string(r[:pos]))
		if cx > tx+room {
			cx = tx + room
		}
		c.Rect(cx, fy+9, 1.5, 16, 0, p.accent)
	}
	// "/" as a keycap at the right of the field.
	kw := c.Width(f.monoS, "/") + 14
	c.Rect(fieldX+fieldW-10-kw, fy+8, kw, 18, 5, p.keycap)
	c.Text(f.monoS, fieldX+fieldW-10-kw+7, mid(fy+8, 18, 11.5), "/", p.fg)
}

func clampF(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// --- the left column -------------------------------------------------------------

type railItem struct {
	label, count string
	active       bool
	kind         hitKind
	index        int
	name         string
	gapBefore    bool
	dot          bool
}

func (m *Model) pxRail(c *pix.Canvas, p pxPal, f pxFonts, g pxGeo, title string, items []railItem, cursor, pinned int) {
	if g.railW == 0 {
		return
	}
	pxCaps(c, f.caps, g.railX+10, g.T+76, title, p.muted)
	y := g.T + 88
	// The first pinned lines stay; the rest scroll to keep the active one
	// on screen when the list is long.
	room := int((g.viewEnd-y)/28) - pinned
	start := pinned
	if cursor-pinned >= room && room > 0 {
		start = cursor - room/2
	}
	order := make([]int, 0, len(items))
	for i := 0; i < pinned && i < len(items); i++ {
		order = append(order, i)
	}
	for i := start; i < len(items); i++ {
		order = append(order, i)
	}
	for n, i := range order {
		it := items[i]
		if it.gapBefore && n > 0 {
			y += 12
		}
		if y+28 > g.viewEnd {
			break
		}
		if it.active {
			c.Rect(g.railX, y, g.railW, 28, 7, p.railSel)
		}
		lx := g.railX + 10
		if it.dot {
			pxDot(c, lx+3, y+14, p.accent)
			lx += 14
		}
		face, col := f.body, p.text2
		if it.active {
			face, col = f.rowB, p.fg
		}
		// A long value, a font family's name, gives way before the label.
		count := c.Fit(f.small, it.count, g.railW*0.55)
		cw := c.Width(f.small, count)
		c.Text(face, lx, mid(y, 28, 13.5), c.Fit(face, it.label, g.railW-(lx-g.railX)-cw-20), col)
		c.Text(f.small, g.railX+g.railW-10-cw, mid(y, 28, 12), count, p.muted)
		if it.kind != hitNone {
			m.pxRegion(c, g.railX, y, g.railW, 28, it.kind, it.index, it.name)
		}
		y += 28
	}
}

// --- the wall of themes --------------------------------------------------------------

// cardGap is the air between two cards; how wide they are is a preference.
const cardGap = 18

func (m *Model) pxGallery(c *pix.Canvas, p pxPal, f pxFonts, g pxGeo) {
	// The families, with the one the highlight is in marked.
	items := []railItem{
		{label: "All", count: itoa(len(m.lib.Themes)), kind: hitCommand, name: "theme"},
		{label: "Favourites", count: itoa(m.state.FavoriteCount()), kind: hitCommand, index: 1, name: "favs"},
	}
	cur := -1
	first := true
	for i, o := range m.results {
		if !o.header {
			continue
		}
		n := 0
		for j := i + 1; j < len(m.results) && !m.results[j].header; j++ {
			n++
			if j == m.sel.cursor {
				cur = len(items)
			}
		}
		items = append(items, railItem{label: o.label, count: itoa(n), kind: hitRail, index: i, name: o.label, gapBefore: first})
		first = false
	}
	if cur >= 0 {
		items[cur].active = true
	}
	m.pxRail(c, p, f, g, "Families", items, maxInt(cur, 2), 2)

	if len(m.results) == 0 {
		c.Text(f.body, g.bodyX, g.bodyY+20, "Nothing matches — Esc clears", p.muted)
		return
	}

	cols := maxInt(int((g.bodyW+cardGap)/(m.cardMinW()+cardGap)), 1)
	cw := (g.bodyW - cardGap*float32(cols-1)) / float32(cols)
	ph := cw * 0.624
	pitch := ph + 68
	const headH = 44

	// Lay the wall out: where each result sits, from the top of the wall.
	type spot struct {
		x, y   float32
		kind   int
		height float32
	}
	spots := make([]spot, len(m.results))
	y, col := float32(0), 0
	started := false
	for i, o := range m.results {
		if (o.header || o.theme == nil) && col > 0 {
			y += pitch
			col = 0
		}
		switch {
		case o.header:
			if started && col == 0 && i > 0 && !m.results[i-1].header && m.results[i-1].theme == nil {
				y += 10
			}
			spots[i] = spot{0, y, galHead, headH}
			y += headH
		case o.theme == nil:
			spots[i] = spot{0, y, galRow, 32}
			y += 40
		default:
			spots[i] = spot{float32(col) * (cw + cardGap), y, galTile, ph + 50}
			col++
			if col == cols {
				col = 0
				y += pitch
			}
		}
		started = true
	}
	if col > 0 {
		y += pitch
	}
	total := y

	e := m.px
	e.cols = cols
	viewH := g.viewEnd - g.bodyY
	e.cardRows = maxInt(int(viewH/pitch), 1)

	if cur := m.sel.cursor; cur >= 0 && cur < len(spots) {
		s := spots[cur]
		lead := float32(0)
		if s.kind == galTile && s.x == 0 && cur > 0 && m.results[cur-1].header {
			lead = headH
		}
		if m.galAlign || m.centre {
			e.galTop = s.y - lead
			if m.centre {
				e.galTop = s.y - (viewH-s.height)/2
			}
			// Show the whole group from its title when the card fits under it.
			for j := cur; j >= 0; j-- {
				if m.results[j].header {
					if s.y+s.height-spots[j].y <= viewH {
						e.galTop = spots[j].y
					}
					break
				}
			}
		}
		m.galAlign, m.centre = false, false
		if s.y-lead < e.galTop {
			e.galTop = s.y - lead
		}
		if s.y+s.height > e.galTop+viewH {
			e.galTop = s.y + s.height - viewH
		}
	}
	if e.galTop > total-viewH {
		e.galTop = total - viewH
	}
	if e.galTop < 0 {
		e.galTop = 0
	}
	// Never start the view inside a row: the first thing on screen is a
	// title or a whole row of cards.
	snap := total
	for _, s := range spots {
		if s.y >= e.galTop-0.5 && s.y < snap {
			snap = s.y
		}
	}
	if snap < total {
		e.galTop = snap
	}

	c.Clip(0, g.bodyY-6, g.W, g.viewEnd-g.bodyY+6)
	defer c.Unclip()
	// The cards are drawn side by side when many are new, the titles
	// between them in their turn.
	batch := e.cards.Batch(c)
	defer batch.Run()
	for i, s := range spots {
		top := g.bodyY + s.y - e.galTop
		// Only what fits whole is drawn: no half cards at the edges, and no
		// title without the row under it.
		need := s.height
		if s.kind == galHead {
			need += ph + 50
		}
		if top < g.bodyY-0.5 || top+need > g.viewEnd+0.5 {
			continue
		}
		o := m.results[i]
		switch s.kind {
		case galHead:
			batch.Do(func() {
				w := c.Text(f.h1, g.bodyX, top+21, o.label, p.fg)
				c.Text(f.body, g.bodyX+w+12, top+21, o.detail, p.muted)
			})
		case galRow:
			batch.Do(func() {
				c.Text(f.row, g.bodyX, top+20, o.label, p.fg)
				c.Text(f.small, g.bodyX+c.Width(f.row, o.label)+12, top+20, o.detail, p.muted)
			})
			m.pxRegion(c, g.bodyX, top, g.bodyW, 32, hitResult, i, "")
		case galTile:
			x := g.bodyX + s.x
			m.pxCardMemo(batch, c, p, f, o.theme, x, top, cw, ph, i == m.sel.cursor)
			m.pxRegion(c, x, top, cw, ph+50, hitResult, i, "")
		}
	}
}

// pxCard is one theme: its ground, a specimen in its text and cursor
// colours, a command line in its palette, all sixteen colours, its name and
// how well it reads.
func (m *Model) pxCard(c *pix.Canvas, p pxPal, f pxFonts, t *ghostty.Theme, x, y, w, h float32, sel bool) {
	sp := m.themeSpec(t)
	bg, fg := pix.Hex(sp.BG, 1), pix.Hex(sp.FG, 1)
	if sel {
		c.Shadow(x, y, w, h, 10, 14, 8, 0.35)
		c.Ring(x-4, y-4, w+8, h+8, 14, 2, p.accent)
	}
	c.Rect(x, y, w, h, 10, bg)
	c.Ring(x, y, w, h, 10, 1, pix.Hex(sp.FG, 0.08))

	aw := c.Text(f.cardAa, x+13, y+11+21, "Aa", fg)
	c.Rect(x+13+aw+6, y+15, 6, 18, 3, pix.Hex(sp.Accent, 1))

	dim := pix.Hex(color.Blend(sp.FG, sp.BG, 0.4), 1)
	l1 := y + h - 11 - 5 - 9 - 14
	l2 := l1 + 14
	xx := x + 13
	xx += c.Text(f.cardMono, xx, l1, "›", pix.Hex(sp.Pal[2], 1))
	xx += c.Text(f.cardMono, xx, l1, " git diff ", fg)
	c.Text(f.cardMono, xx, l1, "--stat", dim)
	xx = x + 13
	xx += c.Text(f.cardMono, xx, l2, "app.go ", pix.Hex(sp.Pal[4], 1))
	xx += c.Text(f.cardMono, xx, l2, "+12 ", pix.Hex(sp.Pal[2], 1))
	c.Text(f.cardMono, xx, l2, "−3", pix.Hex(sp.Pal[1], 1))

	sx, sw := x+13, w-26
	seg := sw / 16
	for i := 0; i < 16; i++ {
		c.Rect(sx+float32(i)*seg, y+h-16, seg+0.6, 5, 0, pix.Hex(sp.Pal[i], 1))
	}

	if t.Name == m.applied {
		label := "Applied"
		lw := c.Width(f.small, label) * 0.86
		c.Rect(x+w-9-lw-14, y+9, lw+14, 18, 9, pix.Hex(sp.FG, 0.14))
		c.Text(c.Face(false, false, 10.5), x+w-9-lw-7, mid(y+9, 18, 10.5), label, fg)
	}

	name, nameCol := c.Face(false, false, 13.5), p.text2
	if sel {
		name, nameCol = c.Face(false, true, 13.5), p.fg
	}
	c.Text(name, x, y+h+9+13, c.Fit(name, t.Name, w), nameCol)
	kind := "dark"
	if !t.IsDark() {
		kind = "light"
	}
	ratio := color.Contrast(t.Foreground(), t.Background())
	meta := fmt.Sprintf("%s · %s %.1f", kind, color.Grade(ratio), ratio)
	mw := c.Text(f.small, x, y+h+9+31, meta, p.muted)
	if m.state.IsFavorite(t.Name) {
		c.Star(x+mw+10, y+h+9+27, 5, p.accent)
	}
}

// --- the menu: a section's settings and commands -------------------------------------

const (
	rowH    = 38
	headH   = 28
	groupGp = 18
)

// menuSpot is where a line of the menu sits.
type menuSpot struct {
	y, h  float32
	head  string // a title drawn above the line, when it starts a part
	headY float32
}

func (m *Model) pxMenuLayout() ([]menuSpot, float32) {
	spots := make([]menuSpot, len(m.results))
	y := float32(0)
	last := ""
	searching := strings.TrimSpace(m.prompt.Value()) != ""
	for i, o := range m.results {
		if o.header {
			spots[i] = menuSpot{y: y, h: 0}
			continue
		}
		sub := ""
		if o.cmd != nil {
			if searching {
				sub = o.cmd.group
			} else {
				sub, _ = subOf(o.cmd)
			}
		}
		if sub != last {
			if i > 0 {
				y += groupGp
			}
			spots[i].head, spots[i].headY = sub, y
			y += headH
			last = sub
		}
		spots[i].y, spots[i].h = y, rowH
		y += rowH
	}
	return spots, y
}

func (m *Model) pxMenu(c *pix.Canvas, p pxPal, f pxFonts, g pxGeo) {
	spots, total := m.pxMenuLayout()

	// The column on the left: the parts of the section, then what changed.
	var items []railItem
	cur := -1
	changed := 0
	for i, o := range m.results {
		if o.cmd != nil && m.changed(o.cmd.set) {
			changed++
		}
		if spots[i].head != "" {
			items = append(items, railItem{label: spots[i].head, kind: hitResult, index: i})
		}
		if len(items) > 0 && !o.header {
			n, _ := strconv.Atoi(items[len(items)-1].count)
			items[len(items)-1].count = itoa(n + 1)
			if i == m.sel.cursor {
				cur = len(items) - 1
			}
		}
	}
	if cur >= 0 {
		items[cur].active = true
	}
	if changed > 0 {
		items = append(items, railItem{label: "Changed", count: itoa(changed), dot: true, gapBefore: true})
	}
	title := groups[m.group]
	if strings.TrimSpace(m.prompt.Value()) != "" {
		title = "Results"
	}
	m.pxRail(c, p, f, g, title, items, maxInt(cur, 0), 0)

	listW := minF(488, g.bodyW*0.52)
	sideX := g.bodyX + listW + 30
	sideW := g.W - g.M - sideX
	if sideW < 260 {
		listW, sideW = g.bodyW, 0
	}

	if len(m.results) == 0 {
		c.Text(f.body, g.bodyX+14, g.bodyY+20, "Nothing matches — Esc clears", p.muted)
		return
	}
	m.pxScrollList(spots, total, g)

	e := m.px
	c.Clip(g.bodyX-4, g.bodyY, listW+8, g.viewEnd-g.bodyY)
	for i, o := range m.results {
		s := spots[i]
		if s.head != "" {
			hy := g.bodyY + s.headY - e.listTop
			if hy >= g.bodyY-0.5 && hy+headH+rowH <= g.viewEnd+0.5 {
				c.Text(f.h2, g.bodyX+14, mid(hy, headH, 15), s.head, p.fg)
			}
		}
		if o.header || s.h == 0 {
			continue
		}
		y := g.bodyY + s.y - e.listTop
		if y < g.bodyY-0.5 || y+rowH > g.viewEnd+0.5 {
			continue
		}
		// The row first, so the control's own regions lie on top of it.
		m.pxRegion(c, g.bodyX, y, listW, rowH, hitResult, i, "")
		m.pxMenuRow(c, p, f, o, i, i == m.sel.cursor, g.bodyX, y, listW)
	}
	c.Unclip()

	if sideW > 0 {
		if o := m.selected(); o != nil && o.cmd != nil {
			m.pxCommandSide(c, p, f, o.cmd, nil, sideX, g.bodyY, sideW, g.viewEnd-g.bodyY)
		}
	}
}

func minF(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// pxScrollList keeps the highlighted line of a list on screen.
func (m *Model) pxScrollList(spots []menuSpot, total float32, g pxGeo) {
	e := m.px
	viewH := g.viewEnd - g.bodyY
	e.listRows = maxInt(int(viewH/rowH), 1)
	if cur := m.sel.cursor; cur >= 0 && cur < len(spots) {
		s := spots[cur]
		top := s.y
		if s.head != "" {
			top = s.headY
		}
		if m.centre {
			e.listTop = s.y - (viewH-s.h)/2
			m.centre = false
		}
		if top < e.listTop {
			e.listTop = top
		}
		if s.y+s.h > e.listTop+viewH {
			e.listTop = s.y + s.h - viewH
		}
	}
	if e.listTop > total-viewH {
		e.listTop = total - viewH
	}
	if e.listTop < 0 {
		e.listTop = 0
	}
	// Start the view on a line or a title, never inside one.
	snap := total
	for _, s := range spots {
		top := s.y
		if s.head != "" {
			top = s.headY
		}
		if top >= e.listTop-0.5 && top < snap {
			snap = top
		}
	}
	if snap < total {
		e.listTop = snap
	}
}

// pxMenuRow is a line of the menu: a mark when the setting is changed, its
// name, and on the right the control that shows its value.
func (m *Model) pxMenuRow(c *pix.Canvas, p pxPal, f pxFonts, o option, idx int, sel bool, x, y, w float32) {
	if sel {
		c.Rect(x, y, w, rowH, 8, p.rowSel)
	}
	var s *setting
	if o.cmd != nil {
		s = o.cmd.set
	}
	if m.changed(s) {
		pxDot(c, x+6, y+rowH/2, p.accent)
	}
	face, col := f.row, p.text2
	if sel {
		face, col = f.rowB, p.fg
	}
	label := o.label
	if s != nil {
		label = s.label
		// Under a title that already names it, the setting is its style.
		if sub, _ := subOf(o.cmd); sub == label {
			label = "Style"
		}
	}
	right := x + w - 12
	base := mid(y, rowH, 14)
	labelW := c.Width(face, label)

	if s == nil {
		// A command: its value, quiet, and a chevron that says Enter opens it.
		right -= 10
		c.Line(right+3, y+rowH/2-4, right+7, y+rowH/2, 1.4, p.muted)
		c.Line(right+7, y+rowH/2, right+3, y+rowH/2+4, 1.4, p.muted)
		v := o.detail
		vx := right - 8
		if o.theme != nil {
			pxSwatch(c, o.theme, vx-22, y+8, 22)
			vx -= 30
		}
		room := vx - (x + 14 + labelW + 20)
		v = c.Fit(f.small, v, room)
		vc := p.muted
		if !quietValue(v) {
			vc = p.text2
		}
		c.Text(f.small, vx-c.Width(f.small, v), mid(y, rowH, 12.5), v, vc)
		c.Text(face, x+14, base, label, col)
		return
	}

	v := s.get(m)
	ctrlLeft := m.pxControl(c, p, f, s, v, idx, right, y, x+14+labelW+24)
	c.Text(face, x+14, base, c.Fit(face, label, ctrlLeft-(x+14)-16), col)
}

// pxControl draws the control of a setting with its right edge at right and
// returns where it starts. A number is a slider, on and off a switch, a few
// words a row of pills, more words the value between arrows.
func (m *Model) pxControl(c *pix.Canvas, p pxPal, f pxFonts, s *setting, v string, idx int, right, y, minLeft float32) float32 {
	cy := y + rowH/2
	switch {
	case s.list != nil:
		// A file: its name, and a chevron that says Enter lists the others.
		x := right - 10
		c.Line(x+3, cy-4, x+7, cy, 1.4, p.muted)
		c.Line(x+7, cy, x+3, cy+4, 1.4, p.muted)
		label := s.shown(v)
		vc := p.fg
		if quietValue(label) {
			vc = p.muted
		}
		label = c.Fit(f.small, label, maxF(right-18-minLeft, 40))
		x -= 8 + c.Width(f.small, label)
		c.Text(f.small, x, mid(y, rowH, 12.5), label, vc)
		return x

	case s.numeric:
		val, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
		if err != nil {
			val = s.min
		}
		vc := p.fg
		if s.unset != "" && strings.EqualFold(v, s.unset) {
			// Left to Ghostty: the knob rests where its own value is.
			val, vc = s.unsetAt, p.muted
		}
		frac := float32((val - s.min) / (s.max - s.min))
		frac = clampF(frac, 0, 1)
		vw := maxF(40, c.Width(f.mono, v))
		c.Text(f.mono, right-c.Width(f.mono, v), mid(y, rowH, 13), v, vc)
		tw := float32(170)
		tx := right - vw - 12 - tw
		c.Rect(tx, cy-2, tw, 4, 2, p.track)
		c.Rect(tx, cy-2, tw*frac, 4, 2, p.accent)
		kx := tx + (tw-14)*frac
		c.Circle(kx+7, cy+0.5, 8, stdcolor.NRGBA{0, 0, 0, 64})
		c.Circle(kx+7, cy, 7, pix.Hex("#f6f7f9", 1))
		m.pxRegion(c, tx, y, tw, rowH, hitValue, idx, "slider")
		return tx

	case len(s.options) == 2 && s.options[0] == "on" && s.options[1] == "off":
		on := v == "on"
		tx := right - 34
		bg := p.track
		kx := tx + 2
		if on {
			bg = p.accent
			kx = tx + 16
		}
		c.Rect(tx, cy-10, 34, 20, 10, bg)
		c.Circle(kx+8, cy+0.4, 8.4, stdcolor.NRGBA{0, 0, 0, 40})
		c.Circle(kx+8, cy, 8, pix.Hex("#f6f7f9", 1))
		other := "on"
		if on {
			other = "off"
		}
		m.pxRegion(c, tx-4, y, 42, rowH, hitValue, idx, other)
		return tx
	}

	// Pills, when they fit.
	widths := make([]float32, len(s.options))
	total := float32(0)
	for i, o := range s.options {
		widths[i] = c.Width(f.smallB, displayValue(o)) + 20
		total += widths[i] + 2
	}
	if right-total >= minLeft && len(s.options) <= 6 {
		x := right - total + 2
		for i, o := range s.options {
			label := displayValue(o)
			if strings.EqualFold(o, v) {
				c.Rect(x, cy-12, widths[i], 24, 6, p.accent)
				c.Text(f.smallB, x+(widths[i]-c.Width(f.smallB, label))/2, mid(cy-12, 24, 12.5), label, p.onAccent)
			} else {
				c.Text(f.small, x+(widths[i]-c.Width(f.small, label))/2, mid(cy-12, 24, 12.5), label, p.muted)
			}
			m.pxRegion(c, x, y, widths[i], rowH, hitValue, idx, o)
			x += widths[i] + 2
		}
		return right - total
	}

	// Too many words: the value between the arrows that step it.
	label := displayValue(v)
	lw := c.Width(f.small, label)
	x := right - 10
	c.Line(x+2, cy-4, x+6, cy, 1.4, p.muted)
	c.Line(x+6, cy, x+2, cy+4, 1.4, p.muted)
	x -= 10 + lw
	c.Text(f.small, x, mid(y, rowH, 12.5), label, p.fg)
	x -= 16
	c.Line(x+6, cy-4, x+2, cy, 1.4, p.muted)
	c.Line(x+2, cy, x+6, cy+4, 1.4, p.muted)
	return x
}

// pxCommandSide is the panel beside a list: for a setting, the window as it
// would look, what the value does and the line that would be written; for
// another command, what it does.
func (m *Model) pxCommandSide(c *pix.Canvas, p pxPal, f pxFonts, cmd *command, opt *option, x, y, w, h float32) {
	c.Clip(x, y, w, h)
	defer c.Unclip()
	s := cmd.set
	if s == nil {
		c.Text(f.h3, x, y+20, cmd.title(), p.fg)
		c.Text(f.mono, x, y+46, c.Fit(f.mono, cmd.syntax, w), p.muted)
		yy := y + 78
		for _, line := range c.Wrap(f.body, cmd.desc+".", w-12) {
			c.Text(f.body, x, yy, line, p.text2)
			yy += 21
		}
		if cmd.adjust {
			yy += 8
		}
		m.pxRegion(c, x, y, w, h, hitPreview, 0, "")
		return
	}

	value := s.get(m)
	if opt != nil && opt.value != "" {
		value = opt.value
	}
	yy := y
	if s.group == "Window" && m.cur != nil {
		ph := minF(250, w*0.6)
		m.pxWindowPicture(c, p, s.cmd, value, x, yy, w, ph)
		yy += ph + 22
	}
	c.Text(f.h3, x, yy+18, s.label, p.fg)
	yy += 30
	kx := x + c.Text(f.mono, x, yy+14, s.key, p.muted)
	if s.fallback != "" {
		def := "· default " + displayValue(s.fallback)
		if s.unset != "" && s.fallback == s.unset {
			def = "· not set by default"
		}
		c.Text(f.small, kx+10, yy+14, def, p.muted)
	}
	yy += 34

	explain := s.explain
	if explain == nil {
		explain = valueNotes[s.cmd]
	}
	text, lead := s.detail+".", ""
	if explain != nil && explain[value] != "" {
		lead = displayValue(value)
		text = explain[value] + "."
		if len(text) > 0 {
			text = strings.ToLower(text[:1]) + text[1:]
		}
	}
	first := true
	for _, line := range c.Wrap(f.body, strings.TrimSpace(lead+" "+text), w-12) {
		lx := x
		if first && lead != "" && strings.HasPrefix(line, lead) {
			lx += c.Text(f.rowB, x, yy+14, strings.ToUpper(lead[:1])+lead[1:], p.fg)
			line = strings.TrimPrefix(line, lead)
		}
		c.Text(f.body, lx, yy+14, line, p.text2)
		yy += 21
		first = false
	}
	if s.describe != nil && lead == "" {
		// What the highlighted value is: the file, where the icons come from.
		yy += 8
		for _, line := range c.Wrap(f.body, title(s.describe(m, value))+".", w-12) {
			c.Text(f.body, x, yy+14, line, p.fg)
			yy += 21
		}
	}
	if s.hint != "" {
		yy += 8
		for _, line := range c.Wrap(f.small, s.hint, w-12) {
			c.Text(f.small, x, yy+13, line, p.muted)
			yy += 19
		}
	}
	if s.note != "" {
		yy += 10
		note := strings.ToUpper(s.note[:1]) + s.note[1:]
		nw := c.Width(f.small, note)
		c.Rect(x, yy, nw+20, 24, 12, p.railSel)
		c.Text(f.small, x+10, mid(yy, 24, 12), note, p.text2)
		yy += 24
	}
	yy += 26
	pxCaps(c, f.caps, x, yy+10, "Will write", p.muted)
	yy += 20
	line := s.key + " = " + value
	switch {
	case s.writes != nil:
		line = s.writes(m, value)
	case strings.EqualFold(value, s.fallback):
		line = ""
	}
	if line == "" {
		c.Text(f.mono, x, yy+14, "# "+s.key+" removed: Ghostty's default", p.muted)
		yy += 24
	}
	for _, l := range strings.Split(line, "\n") {
		if l == "" {
			continue
		}
		px := c.Text(f.mono, x, yy+14, "+ ", p.accent)
		c.Text(f.mono, x+px, yy+14, c.Fit(f.mono, l, w-px), p.fg)
		yy += 24
	}
	c.Text(f.small, x, yy+12, shortenPath(m.tree.Primary.Path), p.muted)
	m.pxRegion(c, x, y, w, h, hitPreview, 0, "")
}

// pxWindowPicture draws the window as its settings would have it, with the
// setting named cmd at value.
func (m *Model) pxWindowPicture(c *pix.Canvas, p pxPal, cmd, value string, x, y, w, h float32) {
	opt := &option{value: value}
	spec := m.windowSpec(cmd, opt)
	spec.Screen = color.Blend(p.ui.fg, p.ui.bg, 0.94)
	wpx, hpx := int(w*c.S), int(h*c.S)
	key := fmt.Sprintf("win|%d|%d|%v", wpx, hpx, spec)
	img := m.px.cached(key, func() *image.RGBA {
		cols := 40
		cw := float64(wpx) / float64(cols)
		ch := cw * 2
		rows := int(float64(hpx)/ch + 0.5)
		return kimg.Window(cols, maxInt(rows, 4), cw, ch, spec).Img
	})
	c.Rect(x, y, w, h, 12, pix.Hex(spec.Screen, 1))
	c.Picture(img, x, y)
}

// --- a command opened: its values --------------------------------------------------------

func (m *Model) pxOptions(c *pix.Canvas, p pxPal, f pxFonts, g pxGeo) {
	// The left column: the other commands of the section, one click away.
	var items []railItem
	cur := 0
	for _, cmd := range m.commands() {
		if cmd.group != groups[m.group] || !m.listed(cmd) || cmd.transient {
			continue
		}
		if cmd == m.cmd {
			cur = len(items)
		}
		items = append(items, railItem{label: cmd.title(), count: m.currentValueOf(cmd), active: cmd == m.cmd, kind: hitCommand, index: len(items), name: cmd.name, dot: cmd.set != nil && m.changed(cmd.set)})
	}
	m.pxRail(c, p, f, g, groups[m.group], items, cur, 0)

	label := m.cmd.title()
	if m.cmd.set != nil {
		label = m.cmd.set.label
	}
	hw := c.Text(f.h1, g.bodyX, g.bodyY+21, label, p.fg)
	listW := minF(488, g.bodyW*0.52)
	sideX := g.bodyX + listW + 30
	sideW := g.W - g.M - sideX
	if sideW < 260 {
		listW, sideW = g.bodyW, 0
	}
	c.Text(f.body, g.bodyX+hw+12, g.bodyY+21, c.Fit(f.body, m.cmd.desc, listW-hw-12), p.muted)

	top := g.bodyY + 44
	if len(m.results) == 0 {
		msg := "Nothing matches — Esc clears"
		if m.cmd.options == nil {
			msg = "Enter runs it · Esc goes back"
		}
		c.Text(f.body, g.bodyX+14, top+20, msg, p.muted)
	} else {
		spots := make([]menuSpot, len(m.results))
		y := float32(0)
		for i, o := range m.results {
			if o.header {
				if i > 0 {
					y += 10
				}
				spots[i] = menuSpot{y: y, h: headH}
				y += headH
				continue
			}
			spots[i] = menuSpot{y: y, h: 36}
			y += 36
		}
		g2 := g
		g2.bodyY = top
		m.pxScrollList(spots, y, g2)
		e := m.px
		c.Clip(g.bodyX-4, top, listW+8, g.viewEnd-top)
		for i, o := range m.results {
			sy := top + spots[i].y - e.listTop
			if sy < top-0.5 || sy+spots[i].h > g.viewEnd+0.5 {
				continue
			}
			if o.header {
				w := c.Text(f.h2, g.bodyX+14, mid(sy, headH, 15), o.label, p.fg)
				c.Text(f.small, g.bodyX+14+w+10, mid(sy, headH, 12.5), c.Fit(f.small, o.detail, listW-w-40), p.muted)
				continue
			}
			m.pxOptionRow(c, p, f, o, i == m.sel.cursor, g.bodyX, sy, listW)
			m.pxRegion(c, g.bodyX, sy, listW, 36, hitResult, i, "")
		}
		c.Unclip()
	}

	if sideW > 0 {
		switch {
		case m.cmd.set != nil:
			m.pxCommandSide(c, p, f, m.cmd, m.selected(), sideX, g.bodyY, sideW, g.viewEnd-g.bodyY)
		case m.cmd.group == "Fonts" && m.pxFontSide(c, p, f, m.selected(), sideX, g.bodyY, sideW, g.viewEnd-g.bodyY):
		default:
			m.pxDetail(c, p, sideX, g.bodyY, sideW, g.viewEnd-g.bodyY)
		}
	}
}

// pxOptionRow is one value of an open command.
func (m *Model) pxOptionRow(c *pix.Canvas, p pxPal, f pxFonts, o option, sel bool, x, y, w float32) {
	const h = 36
	if sel {
		c.Rect(x, y, w, h, 8, p.rowSel)
	}
	if o.current {
		pxDot(c, x+7, y+h/2, p.accent)
	}
	lx := x + 14
	switch {
	case o.theme != nil:
		pxSwatch(c, o.theme, lx, y+8, 20)
		lx += 30
	case o.slot != "" || color.IsHex(o.swatch):
		if color.IsHex(o.swatch) {
			c.Rect(lx, y+10, 16, 16, 4, pix.Hex(o.swatch, 1))
		} else {
			c.Ring(lx, y+10, 16, 16, 4, 1.2, p.muted)
		}
		lx += 26
	}
	face, col := f.row, p.text2
	if sel {
		face, col = f.rowB, p.fg
	}
	label := o.label
	if m.cmd != nil && m.cmd.set != nil {
		label = displayValue(label)
	}
	detail := o.detail
	if detail == "current" {
		detail = ""
	}
	dw := c.Width(f.small, detail)
	maxD := (w - (lx - x)) * 0.55
	if dw > maxD {
		detail = c.Fit(f.small, detail, maxD)
		dw = c.Width(f.small, detail)
	}
	c.Text(face, lx, mid(y, h, 14), c.Fit(face, label, x+w-12-dw-16-lx), col)
	c.Text(f.small, x+w-12-dw, mid(y, h, 12.5), detail, p.muted)
}

// --- the slot editor ---------------------------------------------------------------------

func (m *Model) pxEditor(c *pix.Canvas, p pxPal, f pxFonts, g pxGeo) {
	keys := ghostty.ColorKeys
	var items []railItem
	cur := 0
	for i, k := range keys {
		if k == m.editKey {
			cur = i
		}
		_ = i
	}
	items = append(items, railItem{label: m.cur.Name, count: "", active: true})
	m.pxRail(c, p, f, g, "Editing", items, 0, 0)

	hw := c.Text(f.h1, g.bodyX, g.bodyY+21, "Colour slots", p.fg)
	c.Text(f.body, g.bodyX+hw+12, g.bodyY+21, "the terminal behind shows every change", p.muted)
	listW := minF(488, g.bodyW*0.52)
	sideX := g.bodyX + listW + 30
	sideW := g.W - g.M - sideX
	top := g.bodyY + 44
	spots := make([]menuSpot, len(keys))
	for i := range keys {
		spots[i] = menuSpot{y: float32(i) * 32, h: 32}
	}
	save := m.sel.cursor
	m.sel.cursor = cur
	g2 := g
	g2.bodyY = top
	m.pxScrollList(spots, float32(len(keys))*32, g2)
	m.sel.cursor = save
	bg := m.cur.Background()
	c.Clip(g.bodyX-4, top, listW+8, g.viewEnd-top)
	for i, k := range keys {
		y := top + spots[i].y - m.px.listTop
		if y < top-0.5 || y+32 > g.viewEnd+0.5 {
			continue
		}
		v := m.cur.Colors[k]
		sel := i == cur
		if sel {
			c.Rect(g.bodyX, y, listW, 32, 8, p.rowSel)
		}
		if color.IsHex(v) {
			c.Rect(g.bodyX+14, y+8, 16, 16, 4, pix.Hex(v, 1))
		} else {
			c.Ring(g.bodyX+14, y+8, 16, 16, 4, 1.2, p.muted)
		}
		face, col := f.row, p.text2
		if sel {
			face, col = f.rowB, p.fg
		}
		c.Text(face, g.bodyX+40, mid(y, 32, 14), slotLabel(k), col)
		detail := "not set"
		if v != "" {
			detail = v
			if k != "background" {
				r := color.Contrast(v, bg)
				detail += fmt.Sprintf("   %.1f:1 %s", r, color.Grade(r))
			}
		}
		c.Text(f.mono, g.bodyX+listW-12-c.Width(f.mono, detail), mid(y, 32, 12.5), detail, p.muted)
	}
	c.Unclip()
	if sideW >= 260 {
		m.pxDetail(c, p, sideX, g.bodyY, sideW, g.viewEnd-g.bodyY)
	}
}

// --- the footer ----------------------------------------------------------------------------

func (m *Model) pxFooter(c *pix.Canvas, p pxPal, f pxFonts, g pxGeo) {
	y := g.footY
	cy := y + 24
	x := g.M

	// The right: the keys, and Enter as the one button.
	pairs := m.keyPairs()
	enter := ""
	var keys [][2]string
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i] == "Enter" && enter == "" {
			enter = pairs[i+1]
			continue
		}
		keys = append(keys, [2]string{pairs[i], pairs[i+1]})
	}
	right := g.W - g.M
	if enter != "" {
		label := strings.ToUpper(enter[:1]) + enter[1:]
		bw := c.Width(f.rowB, label) + 14 + 11 + 28
		c.Rect(right-bw, cy-16, bw, 32, 8, p.accent)
		lw := c.Text(c.Face(false, true, 13), right-bw+14, mid(cy-16, 32, 13), label, p.onAccent)
		pxReturn(c, right-bw+14+lw+8, mid(cy-16, 32, 13)+1, stdcolor.NRGBA{p.onAccent.R, p.onAccent.G, p.onAccent.B, 180})
		m.pxRegion(c, right-bw, cy-16, bw, 32, hitButton, 0, "enter")
		right -= bw + 16
	}

	// Left: what just happened, or what the highlighted thing is.
	leftEnd := m.pxFooterInfo(c, p, f, g, x, cy)

	// The keys, as many as fit between the two.
	type cap struct {
		key, word string
		w         float32
	}
	// The global keys (help, docs, settings) are kept first; up to four of
	// the screen's own fill the room left before them.
	var local, global []cap
	for _, k := range keys {
		kw := pxKeyLabel(c, f.monoS, 0, 0, k[0], p.fg, false) + 14
		cp := cap{k[0], k[1], kw + 6 + c.Width(f.small, title(k[1]))}
		if globalKeys[k[0]] {
			global = append(global, cp)
		} else {
			local = append(local, cp)
		}
	}
	total := float32(0)
	fit := func(list []cap, max int) []cap {
		n := 0
		for _, k := range list {
			if right-total-k.w-16 < leftEnd+24 || n == max {
				break
			}
			total += k.w + 16
			n++
		}
		return list[:n]
	}
	global = fit(global, len(global))
	caps := append(fit(local, 4), global...)
	kx := right - total + 16
	for _, k := range caps {
		kw := pxKeyLabel(c, f.monoS, 0, 0, k.key, p.fg, false) + 14
		c.Rect(kx, cy-10, kw, 20, 5, p.keycap)
		pxKeyLabel(c, f.monoS, kx+7, mid(cy-10, 20, 11.5), k.key, p.fg, true)
		c.Text(f.small, kx+kw+6, mid(cy-10, 20, 12.5), title(k.word), p.muted)
		kx += k.w + 16
	}
}

// pxFooterInfo writes the left of the footer and returns where it ends.
func (m *Model) pxFooterInfo(c *pix.Canvas, p pxPal, f pxFonts, g pxGeo, x, cy float32) float32 {
	room := g.W * 0.5
	if m.statusKind != statusPlain || (m.editing && m.status != "") {
		tone := p.muted
		switch m.statusKind {
		case statusInfo:
			tone = pix.Hex(p.ui.accent2, 1)
		case statusWarn:
			tone = pix.Hex(p.ui.warn, 1)
		case statusFail:
			tone = pix.Hex(p.ui.danger, 1)
		}
		pxDot(c, x+3, cy, tone)
		text := c.Fit(f.body, m.status, room)
		return x + 14 + c.Text(f.body, x+14, mid(cy-10, 20, 13.5), text, tone)
	}
	if m.screen() == screenGallery {
		t := m.cur
		if o := m.selected(); o != nil && o.theme != nil {
			t = o.theme
		}
		if t == nil {
			return x
		}
		pxSwatch(c, t, x, cy-15, 30)
		bg := t.Background()
		fgRatio := color.Contrast(t.Foreground(), bg)
		worst, worstName := 21.0, ""
		for i := 1; i < 8; i++ {
			if v := t.Get(paletteKey(i)); v != "" {
				if r := color.Contrast(v, bg); r < worst {
					worst, worstName = r, color.ANSI[i]
				}
			}
		}
		kind := "dark"
		if !t.IsDark() {
			kind = "light"
		}
		meta := themeFamily(t)
		if meta == "" || meta == "installed" || meta == "collection" {
			meta = sourceWord(t)
		}
		meta = strings.ToUpper(meta[:1]) + meta[1:] + " · " + kind + fmt.Sprintf(" · text %.1f:1 %s", fgRatio, color.Grade(fgRatio))
		if worstName != "" {
			meta += fmt.Sprintf(" · weakest %s %.1f:1", worstName, worst)
		}
		nw := c.Text(f.rowB, x+42, cy-2, t.Name, p.fg)
		mw := c.Text(f.small, x+42, cy+15, c.Fit(f.small, meta, room), p.muted)
		return x + 42 + maxF(nw, mw)
	}
	changed := 0
	for _, cmd := range m.commands() {
		if cmd.group == groups[m.group] && cmd.set != nil && m.changed(cmd.set) {
			changed++
		}
	}
	if changed > 0 {
		pxDot(c, x+3, cy, p.accent)
		w := c.Text(f.body, x+14, mid(cy-10, 20, 13.5), fmt.Sprintf("%d changed", changed), p.fg)
		w2 := c.Text(f.small, x+14+w+12, mid(cy-10, 20, 12.5), "each file is backed up before its first change", p.muted)
		return x + 14 + w + 12 + w2
	}
	applied := m.applied
	if applied == "" {
		applied = "no theme set"
	}
	w := c.Text(f.small, x, mid(cy-10, 20, 12.5), "applied ", p.muted)
	w += c.Text(f.small, x+w, mid(cy-10, 20, 12.5), applied, p.fg)
	if m.dirty {
		w += c.Text(f.small, x+w, mid(cy-10, 20, 12.5), "   unsaved changes", pix.Hex(p.ui.warn, 1))
	}
	return x + w
}

func maxF(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// --- dialogs --------------------------------------------------------------------------------

func (m *Model) pxOverlay(c *pix.Canvas, p pxPal, f pxFonts, g pxGeo) {
	// The screen behind steps back, and stops answering to the mouse: a
	// click on the dimmed footer's Enter would otherwise press Enter in the
	// dialog, and confirm what it asks.
	m.regions = m.regions[:0]
	c.Dim(stdcolor.NRGBA{p.bg.R, p.bg.G, p.bg.B, 170})
	surface := pix.Hex(color.Blend(p.ui.bg, p.ui.fg, 0.07), 1)
	card := func(w, h float32) (float32, float32) {
		x, y := (g.W-w)/2, (g.H-h)/2
		c.Shadow(x, y, w, h, 14, 24, 12, 0.5)
		c.Rect(x, y, w, h, 14, surface)
		return x, y
	}
	switch m.overlay {
	case overlayConfirm:
		w := minF(520, g.W-40)
		lines := c.Wrap(f.body, m.confirmText, w-56)
		h := float32(64+len(lines)*21) + 72
		x, y := card(w, h)
		c.Text(f.h3, x+28, y+40, "Confirm", p.fg)
		yy := y + 70
		for _, l := range lines {
			c.Text(f.body, x+28, yy, l, p.text2)
			yy += 21
		}
		by := y + h - 52
		bx := x + 28
		yes := c.Width(f.rowB, "Yes") + 44
		c.Rect(bx, by, yes, 32, 8, p.accent)
		c.Text(f.rowB, bx+14, mid(by, 32, 14), "Yes", p.onAccent)
		c.Text(f.monoS, bx+yes-22, mid(by, 32, 11.5), "y", p.onAccent)
		m.pxRegion(c, bx, by, yes, 32, hitButton, 0, "confirm-yes")
		bx += yes + 10
		no := c.Width(f.row, "No") + 44
		c.Rect(bx, by, no, 32, 8, p.keycap)
		c.Text(f.row, bx+14, mid(by, 32, 14), "No", p.fg)
		c.Text(f.monoS, bx+no-22, mid(by, 32, 11.5), "n", p.muted)
		m.pxRegion(c, bx, by, no, 32, hitButton, 1, "confirm-no")
		c.Text(f.small, bx+no+16, mid(by, 32, 12.5), "Esc stays", p.muted)
	case overlayBusy:
		w := minF(520, g.W-40)
		x, y := card(w, 110)
		c.Text(f.h3, x+28, y+40, "Working", p.fg)
		// The spinner's braille frames are in no face the canvas carries: a
		// dot in the accent says the same.
		pxDot(c, x+31, y+71, p.accent)
		sw := float32(6)
		c.Text(f.body, x+28+sw+10, y+76, c.Fit(f.body, m.busyText, w-70-sw), p.text2)
	case overlayMessage:
		w := minF(600, g.W-40)
		lines := c.Wrap(f.body, m.messageText, w-56)
		h := float32(64+len(lines)*21) + 56
		x, y := card(w, h)
		c.Text(f.h3, x+28, y+40, "Note", p.fg)
		yy := y + 70
		for _, l := range lines {
			c.Text(f.body, x+28, yy, l, p.text2)
			yy += 21
		}
		c.Text(f.small, x+28, y+h-22, "Any key closes", p.muted)
	case overlayHelp:
		m.pxHelp(c, p, f, g, card)
	case overlayCapture:
		m.pxCapture(c, p, f, g, card)
	case overlayExplain:
		m.pxExplain(c, p, f, g, card)
	case overlaySettings:
		m.pxSettings(c, p, f, g, card)
	case overlayUpdate:
		w := minF(540, g.W-40)
		lines := c.Wrap(f.body, "Install downloads it for this system, checks it against the release checksums and replaces this binary. The new version starts next time.", w-56)
		h := float32(104+len(lines)*21) + 72
		x, y := card(w, h)
		c.Text(f.h3, x+28, y+40, "Update available", p.fg)
		vx := x + 28
		vx += c.Text(f.rowB, vx, y+72, "ghostty-config "+m.updateLatest, p.fg)
		c.Text(f.body, vx+10, y+72, "· you have "+m.opts.Version, p.muted)
		yy := y + 104
		for _, l := range lines {
			c.Text(f.body, x+28, yy, l, p.text2)
			yy += 21
		}
		by := y + h - 52
		bx := x + 28
		label := "Install"
		iw := c.Width(f.rowB, label) + 14 + 11 + 28
		c.Rect(bx, by, iw, 32, 8, p.accent)
		lw := c.Text(f.rowB, bx+14, mid(by, 32, 14), label, p.onAccent)
		pxReturn(c, bx+14+lw+8, mid(by, 32, 14)+1, p.onAccent)
		m.pxRegion(c, bx, by, iw, 32, hitButton, 0, "update-install")
		bx += iw + 10
		later := "Next time"
		nw := c.Width(f.row, later) + c.Width(f.monoS, "Esc") + 44
		c.Rect(bx, by, nw, 32, 8, p.keycap)
		tw := c.Text(f.row, bx+14, mid(by, 32, 14), later, p.fg)
		c.Text(f.monoS, bx+14+tw+10, mid(by, 32, 11.5), "Esc", p.muted)
		m.pxRegion(c, bx, by, nw, 32, hitButton, 1, "update-later")
	}
}

// pxHelp is every key and every command, in two columns.
func (m *Model) pxHelp(c *pix.Canvas, p pxPal, f pxFonts, g pxGeo, card func(w, h float32) (float32, float32)) {
	w := minF(980, g.W-32)
	h := minF(g.H-32, 640)
	x, y := card(w, h)
	c.Text(f.h3, x+28, y+40, "Keys and commands", p.fg)
	c.Text(f.small, x+w-28-c.Width(f.small, "Any key closes"), y+40, "Any key closes", p.muted)
	c.Clip(x, y, w, h-12)
	defer c.Unclip()
	colW := (w - 56 - 32) / 2
	yy := y + 76
	keyCol := float32(0)
	for _, grp := range helpKeyGroups() {
		for i := 0; i+1 < len(grp.pairs); i += 2 {
			keyCol = maxF(keyCol, pxKeyLabel(c, f.monoS, 0, 0, grp.pairs[i], p.fg, false)+14)
		}
	}
	keyCol += 16
	for _, grp := range helpKeyGroups() {
		// A group cut in half reads as the whole of it; one that does not
		// fit is left to the footer of its own screen, which lists its keys.
		if yy+24+float32(len(grp.pairs)/2)*23 > y+h-20 {
			c.Text(f.small, x+28, yy, c.Fit(f.small, "The "+strings.ToLower(grp.title)+" lists its own keys at the foot of its screen", colW), p.muted)
			break
		}
		c.Text(f.h2, x+28, yy, grp.title, p.accent)
		yy += 24
		for i := 0; i+1 < len(grp.pairs); i += 2 {
			kw := pxKeyLabel(c, f.monoS, 0, 0, grp.pairs[i], p.fg, false) + 14
			c.Rect(x+28, yy-14, kw, 19, 5, p.keycap)
			pxKeyLabel(c, f.monoS, x+35, yy, grp.pairs[i], p.fg, true)
			c.Text(f.small, x+28+keyCol, yy, c.Fit(f.small, grp.pairs[i+1], colW-keyCol), p.text2)
			yy += 23
		}
		yy += 10
	}
	rx := x + 28 + colW + 32
	yy = y + 76
	c.Text(f.h2, rx, yy, "Commands", p.accent)
	c.Text(f.small, rx+c.Width(f.h2, "Commands")+10, yy, "type one, then Enter; Tab completes", p.muted)
	yy += 26
	for _, grp := range groups {
		c.Text(f.rowB, rx, yy, grp, p.fg)
		yy += 20
		var names []string
		for _, cmd := range m.commands() {
			if cmd.group == grp && m.inHelp(cmd) {
				names = append(names, title(cmd.name))
			}
		}
		for _, l := range c.Wrap(f.mono, strings.Join(names, "  "), colW) {
			c.Text(f.mono, rx, yy, l, p.text2)
			yy += 19
		}
		yy += 8
	}
}

// pxWelcome is the first screen: what the tool does and how to move in it.
func (m *Model) pxWelcome(c *pix.Canvas, p pxPal, f pxFonts) {
	W, H := c.W(), c.H()
	feats := welcomeFeatures()
	blockH := float32(140 + 30*len(feats) + 120)
	y := (H - blockH) / 2
	if y < 20 {
		y = 20
	}
	big := c.Face(false, true, 34)
	tw := c.Width(big, "ghostty-config")
	c.Text(big, (W-tw)/2, y+34, "ghostty-config", p.fg)
	sub := "Themes, fonts, the window and extensions for Ghostty"
	c.Text(f.body, (W-c.Width(f.body, sub))/2, y+64, sub, p.muted)
	if m.cur != nil {
		sw := float32(16*22 + 15*6)
		sx := (W - sw) / 2
		for i := 0; i < 16; i++ {
			v := color.Normalize(m.cur.Get(paletteKey(i)), m.cur.Foreground())
			c.Rect(sx+float32(i)*28, y+86, 22, 22, 6, pix.Hex(v, 1))
		}
	}
	yy := y + 150
	colX := (W - minF(760, W-40)) / 2
	for _, ft := range feats {
		if ft[0] == "" {
			yy += 12
			continue
		}
		kw := pxKeyLabel(c, f.monoS, 0, 0, ft[0], p.fg, false) + 14
		c.Rect(colX, yy-14, kw, 20, 5, p.keycap)
		pxKeyLabel(c, f.monoS, colX+7, yy+1, ft[0], p.fg, true)
		c.Text(f.rowB, colX+110, yy, ft[1], p.fg)
		c.Text(f.small, colX+300, yy, c.Fit(f.small, ft[2], minF(760, W-40)-300), p.muted)
		yy += 30
	}
	yy += 20
	note := fmt.Sprintf("%d themes · config: %s", len(m.lib.Themes), shortenPath(m.tree.Primary.Path))
	c.Text(f.small, (W-c.Width(f.small, note))/2, yy, note, p.muted)
	note = "Looking changes nothing: a theme is applied only when you press Enter on it"
	c.Text(f.small, (W-c.Width(f.small, note))/2, yy+20, note, p.muted)
	bw := c.Width(f.rowB, "Begin") + 14 + 11 + 28
	bx := (W - bw) / 2
	c.Rect(bx, yy+44, bw, 34, 8, p.accent)
	lw := c.Text(f.rowB, bx+14, mid(yy+44, 34, 14), "Begin", p.onAccent)
	pxReturn(c, bx+14+lw+8, mid(yy+44, 34, 14)+1, p.onAccent)
}

// pxDetail is the text interface's explanation of the highlighted line,
// laid out for the panel's width.
func (m *Model) pxDetail(c *pix.Canvas, p pxPal, x, y, w, h float32) {
	adv := c.Width(c.Face(true, false, 12.5), "0")
	m.pxBridge(c, p, m.detailStyled(m.panelChrome(), maxInt(int(w/adv)-1, 10)), x, y, w, h)
}

// --- the text bridge ------------------------------------------------------------------------

// pxBridge draws lines the text interface rendered, styles and all, in the
// monospace face: the panels that have no picture of their own.
func (m *Model) pxBridge(c *pix.Canvas, p pxPal, lines []string, x, y, w, h float32) {
	mono := c.Face(true, false, 12.5)
	bold := c.Face(true, true, 12.5)
	adv := c.Width(mono, "0")
	cols := int(w / adv)
	const lh = 19
	if cols < 4 || len(lines) == 0 {
		return
	}
	// Shift+arrows and the wheel scroll what does not fit, never past the
	// last line, as in the text interface.
	fit := int(h / lh)
	if m.previewScrl > len(lines)-fit {
		m.previewScrl = maxInt(0, len(lines)-fit)
	}
	lines = lines[m.previewScrl:]
	// Symbols the embedded fonts lack become words before the cells are laid
	// out, so the words take the cells they need.
	words := strings.NewReplacer("⌘", "Cmd+", "⇧", "Shift+", "⌥", "Opt+", "⌃", "Ctrl+", "⏎", "Enter", "⌫", "Bksp", "●", "•", "❯", "›", "✎", "")
	buf := uv.NewScreenBuffer(cols, len(lines))
	for i, l := range lines {
		l = words.Replace(l)
		uv.NewStyledString(l).Draw(buf, uv.Rect(0, i, cols, 1))
	}
	c.Clip(x, y, w, h)
	defer c.Unclip()
	m.pxRegion(c, x, y, w, h, hitPreview, 0, "")
	placeholder := string(kitty.Placeholder)
	for row := 0; row < len(lines); row++ {
		by := y + float32(row)*lh
		if by+lh > y+h {
			// A line cut in half reads as a smudge; it waits for a scroll.
			break
		}
		// Grounds first, a run of cells of one colour as one rectangle: cell
		// by cell, the soft edges of neighbours would show as seams.
		for col := 0; col < cols; {
			bg := pxCellGround(buf.CellAt(col, row), p)
			end := col + 1
			for end < cols && bg != nil && pxCellGround(buf.CellAt(end, row), p) == bg {
				end++
			}
			if bg != nil {
				c.Rect(x+float32(col)*adv, by, adv*float32(end-col), lh, 0, bg)
			}
			col = end
		}
		for col := 0; col < cols; {
			cell := buf.CellAt(col, row)
			if cell == nil || cell.Width == 0 {
				col++
				continue
			}
			cx := x + float32(col)*adv
			var ink stdcolor.Color = p.fg
			if cell.Style.Fg != nil {
				ink = cell.Style.Fg
			}
			switch cell.Content {
			case "█":
				c.Rect(cx, by, adv*float32(cell.Width), lh, 0, ink)
				col += cell.Width
				continue
			case "▄":
				c.Rect(cx, by+lh/2, adv*float32(cell.Width), lh/2, 0, ink)
				col += cell.Width
				continue
			case "▀":
				c.Rect(cx, by, adv*float32(cell.Width), lh/2, 0, ink)
				col += cell.Width
				continue
			}
			if cell.Content != "" && cell.Content != " " && !strings.HasPrefix(cell.Content, placeholder) {
				var fg stdcolor.Color = p.fg
				if cell.Style.Fg != nil {
					fg = cell.Style.Fg
				}
				face := mono
				if cell.Style.Attrs&uv.AttrBold != 0 {
					face = bold
				}
				c.Text(face, cx, by+14, cell.Content, fg)
			}
			col += cell.Width
		}
	}
}

// pxCellGround is the ground a cell of the text bridge paints, nil where it
// is the screen's own.
func pxCellGround(cell *uv.Cell, p pxPal) stdcolor.Color {
	if cell == nil || cell.Style.Bg == nil {
		return nil
	}
	r, g, b, a := cell.Style.Bg.RGBA()
	if a == 0 || (uint8(r>>8) == p.bg.R && uint8(g>>8) == p.bg.G && uint8(b>>8) == p.bg.B) {
		return nil
	}
	return stdcolor.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}
