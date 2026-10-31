package tui

import (
	"fmt"
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/image/font"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
	"github.com/vitruves/ghostty-config/internal/kimg"
	"github.com/vitruves/ghostty-config/internal/pix"
)

// The guide in the picture interface: the name and the six steps across the
// top, one question set large, its answers as rows or cards, the picture of
// what the highlighted answer would do beside them, and the way on at the
// bottom. Sizes follow the window: the page is laid out for a large one and
// scaled down to the sizes the rest of the interface reads at, never below.

// gpx is the scale of a frame and the faces it is set in.
type gpx struct {
	sc                                         float32
	M, T, B                                    float32
	brand, step, eyebrow, h1, h2, name, desc   font.Face
	pill, pillB, btn, btnB, small, mono, cardN font.Face
	h1px, h2px, namePx, descPx, btnPx, monoPx  float32
}

func newGpx(c *pix.Canvas) gpx {
	W, H := c.W(), c.H()
	sc := minF(W/1360, H/862)
	sz := func(px, least float32) float32 { return maxF(px*sc, least) }
	g := gpx{sc: sc, M: sz(56, 24), T: sz(28, 14), B: sz(26, 14)}
	g.h1px, g.h2px, g.namePx, g.descPx, g.btnPx, g.monoPx = sz(40, 22), sz(23, 16), sz(19, 14.5), sz(15, 12.5), sz(16, 13), sz(16, 10.5)
	g.brand = c.Face(false, true, sz(18, 14))
	g.step = c.Face(false, false, sz(13, 11.5))
	g.eyebrow = c.Face(false, false, sz(15, 12.5))
	g.h1 = c.Face(false, true, g.h1px)
	g.h2 = c.Face(false, true, g.h2px)
	g.name = c.Face(false, true, g.namePx)
	g.desc = c.Face(false, false, g.descPx)
	g.pill = c.Face(false, false, g.btnPx)
	g.pillB = c.Face(false, true, g.btnPx)
	g.btn = c.Face(false, false, g.btnPx)
	g.btnB = c.Face(false, true, g.btnPx)
	g.small = c.Face(false, false, sz(14, 12))
	g.mono = c.Face(true, false, g.monoPx)
	g.cardN = c.Face(false, true, sz(22, 14))
	return g
}

// s scales a length of the large layout, keeping it at least least.
func (k gpx) s(v, least float32) float32 { return maxF(v*k.sc, least) }

func (g *guided) drawPixel(m *Model, c *pix.Canvas, p pxPal, f pxFonts, geo pxGeo) {
	if g.away {
		atelier{}.drawPixel(m, c, p, f, geo)
		return
	}
	pg, pages := g.at(m)
	k := newGpx(c)
	W := c.W()
	headEnd := g.pxProgress(m, c, p, k)
	barTop := g.pxBottom(m, c, p, k, pg, pages)

	// The status line, when it says something worth reading, sits over the
	// way on.
	foot := barTop
	if m.status != "" && m.statusKind != statusPlain {
		col := p.muted
		if m.statusKind != statusInfo {
			col = pix.Hex(p.ui.warn, 1)
		}
		line := c.Fit(k.small, m.status, W-2*k.M)
		c.Text(k.small, W-k.M-c.Width(k.small, line), barTop-k.s(14, 9), line, col)
		foot = barTop - k.s(34, 22)
	}

	top := headEnd + k.s(52, 22)
	switch {
	case g.step == gDone:
		g.pxDone(m, c, p, k, pg, top, foot)
	case pg.qs[0].kind == gCards:
		g.pxCards(m, c, p, k, pg, top, foot)
	default:
		g.pxPage(m, c, p, k, pg, top, foot)
	}
}

// pxProgress draws the name and the six steps, and returns where they end.
func (g *guided) pxProgress(m *Model, c *pix.Canvas, p pxPal, k gpx) float32 {
	W := c.W()
	y := k.T
	bw := c.Text(k.brand, k.M, y+k.s(18, 15), "ghostty-config", p.fg)
	x0 := k.M + bw + k.s(24, 18)
	gap := k.s(8, 6)
	sw := (W - k.M - x0 - gap*(gSteps-1)) / gSteps
	for i, name := range gStepNames {
		x := x0 + float32(i)*(sw+gap)
		bar, text := p.track, p.muted
		switch {
		case i == g.step:
			bar, text = p.accent, p.fg
		case i <= g.reached || i < g.step:
			bar, text = p.accent, p.text2
		}
		c.Rect(x, y+2, sw, 4, 2, bar)
		face := k.step
		if i == g.step {
			face = c.Face(false, true, maxF(13*k.sc, 11.5))
		}
		c.Text(face, x, y+6+k.s(6, 5)+k.s(13, 11.5), c.Fit(face, name, sw), text)
		m.pxRegion(c, x, y-4, sw, k.s(34, 28), hitButton, i, "g-step")
	}
	return y + 6 + k.s(6, 5) + k.s(13, 11.5)
}

// --- the way on --------------------------------------------------------------------

// gButton is one item of the bottom bar.
type gButton struct {
	label, name string
	kind        int // 0 a link, 1 the quiet button, 2 the accent one, 3 a dot
	drop        int // the order items go in when the bar is too narrow: high first
}

// bottomItems are the bar's items, the left group and the right one.
func (g *guided) bottomItems(m *Model, pg gPage, pages []gPage) ([]gButton, []gButton) {
	var left, right []gButton
	if g.step > 0 || g.page > 0 {
		left = append(left, gButton{label: "← Back", name: "g-back", kind: 1})
	}
	// Help and the editor's own settings answer on every page, as F1 and F4
	// do everywhere else; they go only after the page's own links.
	left = append(left, gButton{label: "F1 Help", name: "g-help", drop: 1},
		gButton{label: "F4 Settings", name: "g-settings", drop: 1})
	if pg.qs[0].kind == gCards {
		left = append(left, gButton{label: "Three more like these", name: "g-more", drop: 2},
			gButton{label: "·", kind: 3, drop: 3},
			gButton{label: fmt.Sprintf("See all %d", len(m.lib.Themes)), name: "g-all-themes", drop: 3})
	}
	if g.step != gDone {
		if pg.qs[0].kind != gCards {
			right = append(right, gButton{label: "All " + strings.ToLower(gStepNames[g.step]) + " settings", name: "g-all", drop: 4},
				gButton{label: "·", kind: 3, drop: 4})
		}
		right = append(right, gButton{label: "Skip this step", name: "g-skip", drop: 1})
	}
	if label := g.primary(m, pg, pages); label != "" {
		right = append(right, gButton{label: label, name: "g-next", kind: 2})
	}
	return left, right
}

// fitBar drops the items that do not fit, the most droppable first.
func fitBar(left, right []gButton, room float32, width func(gButton) float32, gap float32) ([]gButton, []gButton) {
	total := func() float32 {
		w := float32(0)
		for _, b := range append(append([]gButton(nil), left...), right...) {
			w += width(b) + gap
		}
		return w
	}
	for level := 4; level >= 1 && total() > room; level-- {
		keep := func(bs []gButton) []gButton {
			var out []gButton
			for _, b := range bs {
				if b.drop != level {
					out = append(out, b)
				}
			}
			return out
		}
		left, right = keep(left), keep(right)
	}
	return left, right
}

// pxBottom draws the bar at the bottom and returns its top.
func (g *guided) pxBottom(m *Model, c *pix.Canvas, p pxPal, k gpx, pg gPage, pages []gPage) float32 {
	W, H := c.W(), c.H()
	h := k.s(44, 32)
	y := H - k.B - h
	padX := k.s(20, 14)
	gap := k.s(16, 12)
	width := func(b gButton) float32 {
		face := k.btn
		if b.kind == 2 {
			face = k.btnB
		}
		w := c.Width(face, b.label)
		if b.kind == 1 || b.kind == 2 {
			w += 2 * padX
		}
		return w
	}
	left, right := g.bottomItems(m, pg, pages)
	left, right = fitBar(left, right, W-2*k.M, width, gap)
	draw := func(b gButton, x float32) {
		w := width(b)
		base := mid(y, h, k.btnPx*1.1)
		switch b.kind {
		case 1:
			c.Rect(x, y, w, h, k.s(10, 7), p.field)
			c.Text(k.btn, x+padX, base, b.label, p.text2)
		case 2:
			c.Rect(x, y, w, h, k.s(10, 7), p.accent)
			c.Text(k.btnB, x+padX, base, b.label, p.onAccent)
		default:
			c.Text(k.btn, x, base, b.label, p.text2)
		}
		if b.name != "" {
			m.pxRegion(c, x, y, w, h, hitButton, 0, b.name)
		}
	}
	x := k.M
	for _, b := range left {
		draw(b, x)
		x += width(b) + gap
	}
	x = W - k.M
	for i := len(right) - 1; i >= 0; i-- {
		x -= width(right[i])
		draw(right[i], x)
		x -= gap
	}
	return y
}

// --- a page of questions --------------------------------------------------------------

// pxHead draws the line over the question and the question itself, and
// returns where the answers start.
func (g *guided) pxHead(c *pix.Canvas, p pxPal, k gpx, pg gPage, q gQuestion, x, y, w float32) float32 {
	c.Text(k.eyebrow, x, y+k.s(15, 12.5), c.Fit(k.eyebrow, g.eyebrow(pg), w), p.muted)
	y += k.s(15, 12.5) + k.s(12, 8)
	lines := c.Wrap(k.h1, q.title, w)
	for _, l := range lines {
		y += k.h1px * 1.18
		c.Text(k.h1, x, y, l, p.fg)
	}
	return y + k.s(28, 14)
}

// pxPage is a page of rows and pills, with its picture beside it when the
// window leaves room for one.
func (g *guided) pxPage(m *Model, c *pix.Canvas, p pxPal, k gpx, pg gPage, top, foot float32) {
	W := c.W()
	colW := k.s(560, 340)
	gap := k.s(56, 28)
	prevW := W - 2*k.M - colW - gap
	if prevW < 300 {
		colW, prevW = W-2*k.M, 0
	}
	y := g.pxHead(c, p, k, pg, pg.qs[0], k.M, top, colW)
	write := g.willWrite(m, pg)

	// Rows with every answer explained, unless that would not fit: then only
	// the highlighted one is.
	bottom := foot - k.s(18, 10)
	if prevW == 0 && write != "" {
		bottom -= k.s(30, 22)
	}
	compact := g.pxQuestions(m, c, p, k, pg, k.M, y, colW, false, true) > bottom
	y = g.pxQuestions(m, c, p, k, pg, k.M, y, colW, compact, false)

	if prevW > 0 {
		px := k.M + colW + gap
		ph := foot - top - k.s(40, 26)
		g.pxPreview(m, c, p, k, pg, px, top, prevW, ph)
		if write != "" {
			wy := top + ph + k.s(26, 18)
			for _, l := range c.Wrap(k.small, write, prevW) {
				c.Text(k.small, px, wy, l, p.muted)
				wy += k.s(19, 16)
			}
		}
	} else if write != "" {
		c.Text(k.small, k.M, y+k.s(16, 14), c.Fit(k.small, write, colW), p.muted)
	}
}

// pxQuestions lays out the questions of a page under one another and
// returns where they end; measure only measures.
func (g *guided) pxQuestions(m *Model, c *pix.Canvas, p pxPal, k gpx, pg gPage, x, y, w float32, compact, measure bool) float32 {
	for qi, q := range pg.qs {
		focused := qi == g.focus && len(pg.qs) > 1
		if qi > 0 {
			y += k.s(36, 16)
			col := p.fg
			if !focused {
				col = p.text2
			}
			for _, l := range c.Wrap(k.h2, q.title, w) {
				y += k.h2px * 1.15
				if !measure {
					c.Text(k.h2, x, y, l, col)
				}
			}
			y += k.s(16, 10)
		}
		sel := g.sel(m, q)
		switch q.kind {
		case gPills:
			y = g.pxPills(m, c, p, k, q, qi, sel, x, y, w, measure)
		default:
			y = g.pxRows(m, c, p, k, q, qi, sel, x, y, w, compact, measure, focused || len(pg.qs) == 1)
		}
	}
	return y
}

// pxRows draws answers as rows with a radio each, and returns where they end.
func (g *guided) pxRows(m *Model, c *pix.Canvas, p pxPal, k gpx, q gQuestion, qi, sel int, x, y, w float32, compact, measure, focused bool) float32 {
	padY, padX := k.s(16, 9), k.s(18, 12)
	if compact {
		padY = k.s(10, 6)
	}
	dot := k.s(22, 16)
	tx := x + padX + dot + k.s(16, 12)
	for i, ch := range q.choices {
		nameF := k.name
		if q.kind == gFonts && ch.family != nil {
			if sf := m.guideSpecimen(*ch.family); sf != nil {
				nameF = c.FaceIn(sf, k.namePx*1.1)
			}
		}
		descs := c.Wrap(k.desc, ch.desc, w-(tx-x)-padX)
		if ch.desc == "" || (compact && i != sel) {
			descs = nil
		}
		rh := 2*padY + k.namePx*1.15
		if len(descs) > 0 {
			rh += k.s(5, 3) + float32(len(descs))*k.descPx*1.3
		}
		if !measure {
			if i == sel {
				c.Rect(x, y, w, rh, k.s(12, 8), p.rowSel)
			}
			cy := y + padY + k.namePx*0.6
			cx := x + padX + dot/2
			switch {
			case i == sel && focused:
				c.Circle(cx, cy, dot/2, p.accent)
				c.Circle(cx, cy, dot/2-dot*0.32, p.rowSel)
			case i == sel:
				c.Ring(cx-dot/2, cy-dot/2, dot, dot, dot/2, dot*0.32, p.text2)
			default:
				c.Ring(cx-dot/2, cy-dot/2, dot, dot, dot/2, 2, p.track)
			}
			ny := y + padY + k.namePx*0.95
			c.Text(nameF, tx, ny, c.Fit(nameF, ch.label, w-(tx-x)-padX), p.fg)
			dy := ny + k.s(5, 3)
			for _, l := range descs {
				dy += k.descPx * 1.3
				c.Text(k.desc, tx, dy-k.descPx*0.25, l, p.text2)
			}
			m.pxRegion(c, x, y, w, rh, hitButton, i, fmt.Sprintf("g-q-%d", qi))
		}
		y += rh + k.s(10, 4)
	}
	return y
}

// pxPills draws answers as a row of words, with what the chosen one means
// under them.
func (g *guided) pxPills(m *Model, c *pix.Canvas, p pxPal, k gpx, q gQuestion, qi, sel int, x, y, w float32, measure bool) float32 {
	h := k.s(42, 30)
	padX := k.s(18, 12)
	gap := k.s(8, 6)
	xx := x
	for i, ch := range q.choices {
		face := k.pill
		if i == sel {
			face = k.pillB
		}
		pw := c.Width(face, ch.label) + 2*padX
		if xx+pw > x+w && xx > x {
			xx = x
			y += h + gap
		}
		if !measure {
			if i == sel {
				c.Rect(xx, y, pw, h, k.s(10, 7), p.accent)
				c.Text(face, xx+padX, mid(y, h, k.btnPx*1.1), ch.label, p.onAccent)
			} else {
				c.Rect(xx, y, pw, h, k.s(10, 7), p.field)
				c.Text(face, xx+padX, mid(y, h, k.btnPx*1.1), ch.label, p.text2)
			}
			m.pxRegion(c, xx, y, pw, h, hitButton, i, fmt.Sprintf("g-q-%d", qi))
		}
		xx += pw + gap
	}
	y += h
	if sel >= 0 && q.choices[sel].desc != "" {
		y += k.descPx * 0.2
		for _, l := range c.Wrap(k.desc, q.choices[sel].desc, w) {
			y += k.descPx * 1.3
			if !measure {
				c.Text(k.desc, x, y, l, p.muted)
			}
		}
	}
	return y
}

// --- the themes -------------------------------------------------------------------------

// gSample is the terminal every picture of the guide shows: a prompt, git
// saying what changed, a failure, and the prompt again with the cursor.
var gSample = [][]struct {
	text string
	pal  int
}{
	{{"~/src", 2}, {" › git status", -1}},
	{{"M", 3}, {" app.go", -1}},
	{{"A", 2}, {" window.go", -1}},
	{{"✗ 1 test failed", 1}},
	{{"internal/", 4}, {" ", -1}, {"v1.4.0", 5}},
	{{"~/src", 2}, {" ›", -1}},
}

// pxCards is the page of three themes.
func (g *guided) pxCards(m *Model, c *pix.Canvas, p pxPal, k gpx, pg gPage, top, foot float32) {
	W := c.W()
	q := pg.qs[0]
	y := g.pxHead(c, p, k, pg, q, k.M, top, W-2*k.M)
	y -= k.s(28, 14) - k.s(36, 16)
	gap := k.s(28, 14)
	cw := (W - 2*k.M - 2*gap) / 3
	pad := k.s(10, 6)
	nameH := k.s(48, 34)
	sh := minF(k.s(340, 120), foot-k.s(24, 10)-y-2*pad-nameH)
	sel := g.sel(m, q)
	if len(q.choices) == 0 {
		c.Text(k.desc, k.M, y+20, "No theme of this kind is installed: go back and choose another, or see them all.", p.text2)
		return
	}
	for i, ch := range q.choices {
		x := k.M + float32(i)*(cw+gap)
		h := 2*pad + sh + nameH
		if i == sel {
			c.Rect(x, y, cw, h, k.s(18, 12), p.rowSel)
			c.Ring(x, y, cw, h, k.s(18, 12), k.s(3, 2), p.accent)
		}
		m.pxThemeSample(c, k, ch.theme, x+pad, y+pad, cw-2*pad, sh, k.s(12, 8))
		ny := y + pad + sh + k.s(14, 8) + k.s(22, 14)
		meta := ch.desc
		mw := c.Width(k.desc, meta)
		c.Text(k.cardN, x+pad+4, ny, c.Fit(k.cardN, ch.label, cw-2*pad-mw-16), p.fg)
		c.Text(k.desc, x+cw-pad-4-mw, ny, meta, p.muted)
		m.pxRegion(c, x, y, cw, h, hitButton, i, "g-q-0")
	}
}

// sampleKey is everything a theme's sample is drawn from, for the memo.
type sampleKey struct {
	spec          kimg.CardSpec
	mono          font.Face
	monoPx, pad   float32
	s, ts         float32
	x, y, w, h, r float32
}

// pxThemeSample is a theme's ground with the sample terminal in its colours,
// through the memo of cards: the cards and the mosaic draw the same samples
// on most frames.
func (m *Model) pxThemeSample(c *pix.Canvas, k gpx, t *ghostty.Theme, x, y, w, h, r float32) {
	sp := m.factsOf(t).spec
	pad := k.s(22, 10)
	draw := func() {
		c.Rect(x, y, w, h, r, pix.Hex(sp.BG, 1))
		c.Ring(x, y, w, h, r, 1, pix.Hex(sp.FG, 0.08))
		m.pxTerminal(c, k.mono, k.monoPx, sp.FG, sp.Pal, sp.Accent, sp.BG, x+pad, y+pad, w-2*pad, h-pad, "block")
	}
	if m.px == nil || m.px.cards == nil {
		draw()
		return
	}
	key := sampleKey{spec: sp, mono: k.mono, monoPx: k.monoPx, pad: pad, s: c.S, ts: c.TextScale, x: x, y: y, w: w, h: h, r: r}
	m.px.cards.Draw(c, key, x, y, w, h, draw)
}

// pxTerminal writes the sample in a theme's colours, as many lines as fit,
// with the cursor drawn in the shape given.
func (m *Model) pxTerminal(c *pix.Canvas, face font.Face, px float32, fg string, pal [16]string, cursor, bg string, x, y, w, h float32, shape string) {
	lh := px * 1.1 * 1.7
	c.Clip(x, y, w, h)
	defer c.Unclip()
	n := minInt(len(gSample), int(h/lh))
	for i := 0; i < n; i++ {
		line := gSample[i]
		if i == n-1 && n < len(gSample) {
			line = gSample[len(gSample)-1]
		}
		base := y + float32(i)*lh + px*1.1*1.05
		xx := x
		for _, run := range line {
			col := fg
			if run.pal >= 0 {
				col = pal[run.pal]
			}
			xx += c.Text(face, xx, base, run.text, pix.Hex(col, 1))
		}
		if i == n-1 {
			cw := c.Width(face, "M")
			ch := px * 1.1 * 1.3
			cx, cy := xx+cw, base-px*1.1*0.95
			switch shape {
			case "bar":
				c.Rect(cx, cy, maxF(cw*0.16, 1.5), ch, 0, pix.Hex(cursor, 1))
			case "underline":
				c.Rect(cx, cy+ch-maxF(ch*0.1, 2), cw, maxF(ch*0.1, 2), 0, pix.Hex(cursor, 1))
			default:
				c.Rect(cx, cy, cw, ch, 0, pix.Hex(cursor, 1))
			}
		}
	}
}

// --- the pictures beside a page -----------------------------------------------------------

// pxPreview draws what the highlighted answers would do.
func (g *guided) pxPreview(m *Model, c *pix.Canvas, p pxPal, k gpx, pg gPage, x, y, w, h float32) {
	if m.cur == nil || h < 60 {
		return
	}
	picked := func(id string) string {
		for _, q := range pg.qs {
			if q.id == id {
				if i := g.sel(m, q); i >= 0 && len(q.choices[i].writes) > 0 {
					return q.choices[i].writes[0].opt.value
				}
			}
		}
		return ""
	}
	switch pg.qs[0].preview {
	case "themes":
		g.pxThemeMosaic(m, c, p, k, pg, x, y, w, h)
	case "window":
		g.pxWindowLook(m, c, p, k, pg, x, y, w, h)
	case "cursor":
		shape := picked("cursor")
		if shape == "" {
			shape = m.settingNow("cursor")
		}
		g.pxPanelTerminal(m, c, p, k, x, y, w, h, nil, shape)
		return
	case "font":
		g.pxFontLook(m, c, p, k, pg, x, y, w, h)
	default:
		th := g.pxPanelTerminal(m, c, p, k, x, y, w, h, nil, "block")
		g.pxClipNote(m, c, p, k, pg, x, y+th+k.s(20, 12), w, h-th-k.s(20, 12))
	}
}

// pxPanelTerminal is the theme shown as a large terminal, in face, or the
// interface's own monospace, no taller than its text; it returns its height.
func (g *guided) pxPanelTerminal(m *Model, c *pix.Canvas, p pxPal, k gpx, x, y, w, h float32, face font.Face, shape string) float32 {
	sp := m.themeSpec(m.cur)
	r := k.s(18, 12)
	px := k.s(18, 12)
	if face == nil {
		face = c.Face(true, false, px)
	}
	pad := k.s(36, 18)
	h = minF(h, 2*pad+float32(len(gSample))*px*1.1*1.7)
	c.Rect(x, y, w, h, r, pix.Hex(sp.BG, 1))
	c.Ring(x, y, w, h, r, 1, pix.Hex(sp.FG, 0.08))
	m.pxTerminal(c, face, px, sp.FG, sp.Pal, sp.Accent, sp.BG, x+pad, y+pad, w-2*pad, h-2*pad, shape)
	return h
}

// pxThemeMosaic shows a few themes of the kind highlighted, while the tone
// and the warmth are being chosen.
func (g *guided) pxThemeMosaic(m *Model, c *pix.Canvas, p pxPal, k gpx, pg gPage, x, y, w, h float32) {
	all := g.colourPages(m)
	dark := all[0].qs[0].choices[g.sel(m, all[0].qs[0])].value == "dark"
	kind := all[1].qs[0].choices[g.sel(m, all[1].qs[0])].value
	list := g.themeTemps(m, dark)[kind]
	cols, rows := 2, 3
	gap := k.s(14, 10)
	tw := (w - gap*float32(cols-1)) / float32(cols)
	th := (h - gap*float32(rows-1)) / float32(rows)
	for i := 0; i < cols*rows && i < len(list); i++ {
		t := list[i*len(list)/(cols*rows)]
		tx := x + float32(i%cols)*(tw+gap)
		ty := y + float32(i/cols)*(th+gap)
		m.pxThemeSample(c, k, t, tx, ty, tw, th, k.s(12, 8))
	}
}

// pxFontLook is the terminal set in the family and at the size highlighted.
func (g *guided) pxFontLook(m *Model, c *pix.Canvas, p pxPal, k gpx, pg gPage, x, y, w, h float32) {
	family := m.fontFamily
	size := m.fontSize
	for _, q := range pg.qs {
		i := g.sel(m, q)
		if i < 0 {
			continue
		}
		switch q.id {
		case "font":
			if f := q.choices[i].family; f != nil {
				family = f.Name
			}
		case "size":
			fmt.Sscan(q.choices[i].writes[0].opt.value, &size)
		}
	}
	if size <= 0 {
		size = 13
	}
	px := k.s(18, 12) * float32(size) / 13
	var face font.Face
	if f, ok := m.findFamily(family); ok {
		if sf := m.guideSpecimen(f); sf != nil {
			face = c.FaceIn(sf, px*1.1)
		}
	}
	if face == nil {
		face = c.Face(true, false, px)
	}
	sp := m.themeSpec(m.cur)
	r := k.s(18, 12)
	pad := k.s(36, 18)
	h = minF(h, 2*pad+px*7+float32(len(gSample))*px*1.1*1.7)
	c.Rect(x, y, w, h, r, pix.Hex(sp.BG, 1))
	big := face
	if f, ok := m.findFamily(family); ok {
		if sf := m.guideSpecimen(f); sf != nil {
			big = c.FaceIn(sf, px*3)
		}
	}
	if big == face {
		big = c.Face(true, false, px*2.7)
	}
	c.Text(big, x+pad, y+pad+px*3, "Aa 0O 1lI", pix.Hex(sp.FG, 1))
	c.Text(face, x+pad, y+pad+px*3+px*2.2, "-> => != <= {} [] ()", pix.Hex(color.Blend(sp.FG, sp.BG, 0.35), 1))
	top := y + pad + px*3 + px*4
	m.pxTerminal(c, face, px, sp.FG, sp.Pal, sp.Accent, sp.BG, x+pad, top, w-2*pad, y+h-pad-top, m.settingNow("cursor"))
}

// pxClipNote lays over the terminal what the highlighted answer of a
// clipboard question would show: the question Ghostty asks, or what is
// copied.
func (g *guided) pxClipNote(m *Model, c *pix.Canvas, p pxPal, k gpx, pg gPage, x, y, w, h float32) {
	q := pg.qs[g.focus]
	i := g.sel(m, q)
	if i < 0 || len(q.choices[i].writes) == 0 {
		return
	}
	v := q.choices[i].writes[0].opt.value
	var head, body string
	switch q.id {
	case "copy":
		head, body = "Copied as you select", "Select with the mouse and the text is on the clipboard."
		if v == "none" || v == "false" {
			head, body = "Copied when you ask", "Select, then press the copy key."
		}
	case "trim":
		head, body = "Copied lines", "\"git status\" — no trailing spaces"
		if v == "off" {
			head, body = "Copied lines", "\"git status      \" — spaces kept"
		}
	case "paste":
		head, body = "Paste this text?", "It has a line break: it could run as soon as it is pasted."
		if v == "off" {
			head, body = "Pasted at once", "Even text that runs a command."
		}
	case "clipread":
		say := map[string][2]string{
			"ask":   {"Let this program read the clipboard?", "Ghostty shows what it would read."},
			"allow": {"Read without asking", "Any program may read the clipboard."},
			"deny":  {"Clipboard closed", "No program may read it; you can still paste."},
		}[v]
		head, body = say[0], say[1]
	case "clipwrite":
		head = map[string]string{"allow": "Programs may copy", "ask": "Let this program set the clipboard?", "deny": "Programs may not copy"}[v]
		body = "As vim's and tmux's yanks do."
	}
	if head == "" {
		return
	}
	cw := w
	ch := k.s(96, 64)
	if h < ch {
		return
	}
	cx, cy := x, y
	c.Rect(cx, cy, cw, ch, k.s(12, 8), p.field)
	pad := k.s(20, 12)
	c.Text(k.name, cx+pad, cy+pad+k.namePx, c.Fit(k.name, head, cw-2*pad), p.fg)
	c.Text(k.desc, cx+pad, cy+pad+k.namePx+k.descPx*1.6, c.Fit(k.desc, body, cw-2*pad), p.text2)
}

// pxWindowLook is a window over a desktop, as opaque, as blurred and with
// as much room as the highlighted answers say.
func (g *guided) pxWindowLook(m *Model, c *pix.Canvas, p pxPal, k gpx, pg gPage, x, y, w, h float32) {
	opacity, blur, padding, bar := 1.0, false, 2, m.settingNow("titlebar")
	if o := m.settingNow("opacity"); o != "" {
		fmt.Sscan(o, &opacity)
	}
	blur = m.settingNow("blur") != "off"
	padding = atoiDefault(m.settingNow("paddingx"), 2)
	for _, q := range pg.qs {
		i := g.sel(m, q)
		if i < 0 {
			continue
		}
		for _, wr := range q.choices[i].writes {
			switch wr.cmd {
			case "opacity":
				fmt.Sscan(wr.opt.value, &opacity)
			case "blur":
				blur = wr.opt.value != "off"
			case "paddingx":
				padding = atoiDefault(wr.opt.value, 2)
			case "titlebar":
				bar = wr.opt.value
			}
		}
	}
	sp := m.themeSpec(m.cur)
	r := k.s(18, 12)
	inset := k.s(36, 18)
	wx, wy, ww, wh := x+inset, y+inset, w-2*inset, h-2*inset
	key := fmt.Sprintf("gwin|%.0f|%.0f|%.2f|%v|%s|%v", w*c.S, h*c.S, opacity, blur, sp.BG, color.IsDark(p.ui.bg))
	img := m.px.cached(key, func() *image.RGBA {
		return desktopPicture(int(w*c.S), int(h*c.S), int(r*c.S), image.Rect(int(inset*c.S), int(inset*c.S), int((w-inset)*c.S), int((h-inset)*c.S)), int(k.s(12, 8)*c.S), opacity, blur, sp.BG, !color.IsDark(p.ui.bg))
	})
	c.Picture(img, x, y)

	// The bar, then the text inside the room around it.
	barH := k.s(34, 22)
	dots := pix.Hex(color.Blend(sp.BG, sp.FG, 0.25), 1)
	switch bar {
	case "native":
		c.Rect(wx, wy, ww, barH, k.s(12, 8), pix.Hex(color.Blend(p.ui.bg, p.ui.fg, 0.12), 1))
		c.Rect(wx, wy+barH-k.s(12, 8), ww, k.s(12, 8), 0, pix.Hex(color.Blend(p.ui.bg, p.ui.fg, 0.12), 1))
	case "tabs":
		tw := minF(ww*0.3, k.s(160, 90))
		c.Rect(wx+k.s(80, 50), wy+5, tw, barH-10, 6, pix.Hex(sp.FG, 0.10))
		c.Text(k.small, wx+k.s(80, 50)+10, mid(wy+5, barH-10, 12), "~/src", pix.Hex(sp.FG, 0.8))
	}
	if bar != "hidden" {
		for i := 0; i < 3; i++ {
			c.Circle(wx+k.s(20, 13)+float32(i)*k.s(19, 12), wy+barH/2, k.s(5.5, 4), dots)
		}
	} else {
		barH = 0
	}
	pad := float32(padding) * k.s(1.1, 0.7)
	px := k.s(16, 11)
	m.pxTerminal(c, c.Face(true, false, px), px, sp.FG, sp.Pal, sp.Accent, sp.BG, wx+pad+k.s(8, 6), wy+barH+pad+k.s(6, 4), ww-2*pad-k.s(16, 12), wh-barH-2*pad-k.s(12, 8), m.settingNow("cursor"))
}

// desktopPicture draws a desktop with a window over it: dusk over hills,
// a low sun, and the window's ground as opaque as asked, the desktop
// through it blurred or not. The corners outside the rounding are clear.
func desktopPicture(w, h, r int, win image.Rectangle, wr int, opacity float64, blur bool, ground string, light bool) *image.RGBA {
	if w <= 0 || h <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	stops := [3]string{"#2b3a55", "#5a3d52", "#8a5a3c"}
	sun, hill := "#e8a35c", "#1f2433"
	if light {
		stops = [3]string{"#9db8e0", "#d6a9c0", "#f2c59a"}
		sun, hill = "#fff1d6", "#6f7fa3"
	}
	desk := image.NewRGBA(image.Rect(0, 0, w, h))
	rgb := func(hex string) [3]float64 {
		c, _ := color.ParseHex(hex)
		return [3]float64{float64(c.R), float64(c.G), float64(c.B)}
	}
	s0, s1, s2, su, hi := rgb(stops[0]), rgb(stops[1]), rgb(stops[2]), rgb(sun), rgb(hill)
	lerp := func(a, b [3]float64, t float64) [3]float64 {
		return [3]float64{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
	}
	sx, sy, sr := float64(w)*0.72, float64(h)*0.42, float64(minInt(w, h))*0.16
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			t := (float64(px)/float64(w) + float64(py)/float64(h)) / 2
			var c [3]float64
			if t < 0.55 {
				c = lerp(s0, s1, t/0.55)
			} else {
				c = lerp(s1, s2, (t-0.55)/0.45)
			}
			dx, dy := float64(px)-sx, float64(py)-sy
			if d := dx*dx + dy*dy; d < sr*sr {
				c = su
			}
			// Two hills along the bottom.
			fx := float64(px) / float64(w)
			ridge := 0.74 + 0.08*sinApprox(fx*6.3) - 0.05*sinApprox(fx*13+1)
			if float64(py)/float64(h) > ridge {
				c = lerp(hi, c, 0.2)
			}
			i := desk.PixOffset(px, py)
			desk.Pix[i], desk.Pix[i+1], desk.Pix[i+2], desk.Pix[i+3] = uint8(c[0]), uint8(c[1]), uint8(c[2]), 255
		}
	}
	behind := desk
	if blur {
		behind = boxBlur(desk, maxInt(w/40, 4), 3)
	}
	g := rgb(ground)
	out := image.NewRGBA(desk.Rect)
	in := func(px, py int, rc image.Rectangle, rad int) bool {
		if !(image.Pt(px, py).In(rc)) {
			return false
		}
		cx := clampInt(px, rc.Min.X+rad, rc.Max.X-1-rad)
		cy := clampInt(py, rc.Min.Y+rad, rc.Max.Y-1-rad)
		dx, dy := px-cx, py-cy
		return dx*dx+dy*dy <= rad*rad
	}
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			if !in(px, py, desk.Rect, r) {
				continue
			}
			i := desk.PixOffset(px, py)
			src := desk
			a := 0.0
			if in(px, py, win, wr) {
				// The picture is small beside the window it stands for: what
				// shows through is made a little plainer than it would be.
				src, a = behind, opacity*opacity
			}
			for ch := 0; ch < 3; ch++ {
				out.Pix[i+ch] = uint8(float64(src.Pix[i+ch])*(1-a) + g[ch]*a + 0.5)
			}
			out.Pix[i+3] = 255
		}
	}
	return out
}

// sinApprox is a sine good enough for a skyline.
func sinApprox(x float64) float64 {
	const tau = 6.283185307179586
	x -= tau * float64(int(x/tau))
	if x < 0 {
		x += tau
	}
	// Bhaskara's approximation, on each half of the turn.
	sign := 1.0
	if x > tau/2 {
		x -= tau / 2
		sign = -1
	}
	const pi = tau / 2
	return sign * 16 * x * (pi - x) / (5*pi*pi - 4*x*(pi-x))
}

// boxBlur blurs a picture with a box of radius r, passes times over.
func boxBlur(src *image.RGBA, r, passes int) *image.RGBA {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	a := image.NewRGBA(src.Rect)
	copy(a.Pix, src.Pix)
	b := image.NewRGBA(src.Rect)
	for n := 0; n < passes; n++ {
		for _, horiz := range []bool{true, false} {
			lines, length := h, w
			if !horiz {
				lines, length = w, h
			}
			for l := 0; l < lines; l++ {
				at := func(i int) int {
					i = clampInt(i, 0, length-1)
					if horiz {
						return a.PixOffset(i, l)
					}
					return a.PixOffset(l, i)
				}
				var sum [3]int
				for i := -r; i <= r; i++ {
					o := at(i)
					sum[0], sum[1], sum[2] = sum[0]+int(a.Pix[o]), sum[1]+int(a.Pix[o+1]), sum[2]+int(a.Pix[o+2])
				}
				n := 2*r + 1
				for i := 0; i < length; i++ {
					o := at(i)
					b.Pix[o], b.Pix[o+1], b.Pix[o+2], b.Pix[o+3] = uint8(sum[0]/n), uint8(sum[1]/n), uint8(sum[2]/n), 255
					out, in := at(i-r), at(i+r+1)
					for ch := 0; ch < 3; ch++ {
						sum[ch] += int(a.Pix[in+ch]) - int(a.Pix[out+ch])
					}
				}
			}
			a, b = b, a
		}
	}
	return a
}

// --- Done --------------------------------------------------------------------------

func (g *guided) pxDone(m *Model, c *pix.Canvas, p pxPal, k gpx, pg gPage, top, foot float32) {
	W := c.W()
	q := pg.qs[0]
	colW := k.s(560, 340)
	gap := k.s(56, 28)
	sideW := W - 2*k.M - colW - gap
	if sideW < 280 {
		colW, sideW = W-2*k.M, 0
	}
	y := g.pxHead(c, p, k, pg, q, k.M, top, colW)
	changes := m.sessionChanges()
	note := m.doneNote()

	summary := func(x, y, w float32, most int) float32 {
		pxCaps(c, c.Face(false, true, k.s(12, 10.5)), x, y+k.s(12, 10.5), "Changed this session", p.muted)
		y += k.s(12, 10.5) + k.s(14, 8)
		if len(changes) == 0 {
			for _, l := range c.Wrap(k.desc, "Nothing yet: every step left your settings as they were.", w) {
				y += k.descPx * 1.4
				c.Text(k.desc, x, y, l, p.text2)
			}
			return y
		}
		mono := c.Face(true, false, k.s(14, 11.5))
		for i, l := range changes {
			if i == most-1 && len(changes) > most {
				y += k.descPx * 1.5
				c.Text(k.desc, x, y, fmt.Sprintf("and %d more", len(changes)-i), p.muted)
				break
			}
			y += k.descPx * 1.5
			pw := c.Text(mono, x, y, "+ ", p.accent)
			c.Text(mono, x+pw, y, c.Fit(mono, l, w-pw), p.fg)
		}
		if note != "" {
			y += k.s(10, 6)
			for _, l := range c.Wrap(k.desc, note, w) {
				y += k.descPx * 1.4
				c.Text(k.desc, x, y, l, p.text2)
			}
		}
		return y
	}
	if sideW > 0 {
		x := k.M + colW + gap
		bottom := summary(x, top+k.s(4, 2), sideW, int((foot-top)/(k.descPx*1.5))-4)
		_ = bottom
	} else {
		y = summary(k.M, y-k.s(14, 8), colW, 4) + k.s(18, 10)
	}
	bottom := foot - k.s(10, 6)
	compact := g.pxRows(m, c, p, k, q, 0, g.sel(m, q), k.M, y, colW, false, true, true) > bottom
	g.pxRows(m, c, p, k, q, 0, g.sel(m, q), k.M, y, colW, compact, false, true)
}

// --- fonts -----------------------------------------------------------------------------

// guideSpecimen is a family's regular file, once read.
func (m *Model) guideSpecimen(f ghostty.Family) *pix.Font {
	file, ok := f.File("Regular")
	if !ok {
		return nil
	}
	return m.specimenFont(file)
}

// ensureGuideFonts lists the families, and reads the files of those offered
// once they are.
func (m *Model) ensureGuideFonts(g *guided) tea.Cmd {
	if m.fontsLoaded || m.fontLoading {
		return nil
	}
	cmd := m.ensureFonts()
	if m.fx.afterFonts == nil {
		m.fx.afterFonts = func(m *Model) tea.Cmd { return m.wantGuideSpecimens(g) }
	}
	return cmd
}

// wantGuideSpecimens starts reading the regular file of every family the
// font question offers, so that each is set in itself.
func (m *Model) wantGuideSpecimens(g *guided) tea.Cmd {
	if !m.pixelOn() || !m.fontsLoaded || g.step != gFont {
		return nil
	}
	var need []ghostty.FontFile
	for _, f := range m.guideFamilies() {
		if file, ok := f.File("Regular"); ok && file.Path != "" {
			if _, seen := m.specimens[fileKey(file)]; !seen {
				need = append(need, file)
			}
		}
	}
	if len(need) == 0 {
		return nil
	}
	if m.specimens == nil || len(m.specimens)+len(need) > specimenCap {
		m.specimens = map[string]specimenFont{}
	}
	for _, f := range need {
		m.specimens[fileKey(f)] = specimenFont{}
	}
	return func() tea.Msg {
		out := map[string]specimenFont{}
		for _, f := range need {
			font, err := pix.OpenFont(f.Path, f.PostScript, f.Index)
			out[fileKey(f)] = specimenFont{font, err}
		}
		return specimenMsg{out}
	}
}
