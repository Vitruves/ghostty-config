package tui

import (
	stdcolor "image/color"
	"strings"
	"unicode"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
	"github.com/vitruves/ghostty-config/internal/pix"
)

// The shortcut editor in the picture interface: the actions in their groups
// with their shortcuts as key caps, the column on the left naming the
// groups, and beside the list what the highlighted action does and where
// each of its shortcuts comes from.

// capTone is how a key cap is painted: Ghostty's own on the quiet ground of
// every key cap, the user's tinted with the accent, a default the config
// took away hollow and struck through.
type capTone int

const (
	capDefault capTone = iota
	capUser
	capGone
)

// pxModGlyph draws a modifier's symbol in a square s wide, centred on cy,
// with strokes: the embedded fonts have none of ⌃⌥⇧⌘.
func pxModGlyph(c *pix.Canvas, mod ghostty.Mods, x, cy, s float32, col stdcolor.Color) {
	w := maxF(s*0.12, 1.1)
	top := cy - s/2
	at := func(fx, fy float32) (float32, float32) { return x + fx*s, top + fy*s }
	line := func(x1, y1, x2, y2 float32) {
		ax, ay := at(x1, y1)
		bx, by := at(x2, y2)
		c.Line(ax, ay, bx, by, w, col)
	}
	switch mod {
	case ghostty.ModCtrl:
		line(0.12, 0.62, 0.5, 0.2)
		line(0.5, 0.2, 0.88, 0.62)
	case ghostty.ModShift:
		pts := [][2]float32{{0.5, 0.04}, {0.97, 0.52}, {0.72, 0.52}, {0.72, 0.96}, {0.28, 0.96}, {0.28, 0.52}, {0.03, 0.52}}
		for i := range pts {
			a, b := pts[i], pts[(i+1)%len(pts)]
			line(a[0], a[1], b[0], b[1])
		}
	case ghostty.ModAlt:
		line(0.02, 0.2, 0.34, 0.2)
		line(0.34, 0.2, 0.68, 0.82)
		line(0.68, 0.82, 0.98, 0.82)
		line(0.6, 0.2, 0.98, 0.2)
	case ghostty.ModSuper:
		// Four loops joined by the sides of a square, the looped square.
		r := s * 0.2
		p, q := 0.38*s, 0.62*s
		for _, cxy := range [][2]float32{{p - r, p - r}, {q + r, p - r}, {p - r, q + r}, {q + r, q + r}} {
			c.Ring(x+cxy[0]-r, top+cxy[1]-r, 2*r, 2*r, r, w, col)
		}
		c.Line(x+p, top+p-r, x+p, top+q+r, w, col)
		c.Line(x+q, top+p-r, x+q, top+q+r, w, col)
		c.Line(x+p-r, top+p, x+q+r, top+p, w, col)
		c.Line(x+p-r, top+q, x+q+r, top+q, w, col)
	}
}

// pxChord draws one chord in a key cap h tall, centred on cy, and returns
// its width; with draw false it only measures. On a Mac the modifiers are
// its symbols, elsewhere words. A lone key sits in a square cap.
func (m *Model) pxChord(c *pix.Canvas, p pxPal, ch ghostty.Chord, x, cy, h float32, tone capTone, draw bool) float32 {
	ink := m.pxChordInk(c, p, ch, 0, cy, h, tone, false)
	w := maxF(ink+h*0.68, h)
	if draw {
		m.pxCapGround(c, p, x, cy, w, h, tone)
		m.pxChordInk(c, p, ch, x+(w-ink)/2, cy, h, tone, true)
	}
	return w
}

// pxCapGround paints a key cap's ground.
func (m *Model) pxCapGround(c *pix.Canvas, p pxPal, x, cy, w, h float32, tone capTone) {
	r := h * 0.24
	switch tone {
	case capUser:
		c.Rect(x, cy-h/2, w, h, r, pix.Hex(color.Blend(p.ui.accent, p.ui.bg, 0.72), 1))
	case capGone:
		c.Ring(x, cy-h/2, w, h, r, 1, p.track)
		c.Line(x+3, cy, x+w-3, cy, 1.2, p.muted)
	default:
		c.Rect(x, cy-h/2, w, h, r, p.keycap)
	}
}

// pxChordInk draws a chord's symbols and key from x and returns how wide
// they are; with draw false it only measures.
func (m *Model) pxChordInk(c *pix.Canvas, p pxPal, ch ghostty.Chord, x, cy, h float32, tone capTone, draw bool) float32 {
	size := h * 0.56
	glyph := h * 0.42
	ink := stdcolor.Color(p.fg)
	if tone == capGone {
		ink = p.muted
	}
	base := cy + size*0.36
	w := float32(0)
	label := chordText(ch, false)
	if keysMac {
		for _, mo := range modOrder {
			if ch.Mods&mo.bit != 0 {
				if draw {
					pxModGlyph(c, mo.bit, x+w, cy, glyph, ink)
				}
				w += glyph + h*0.12
			}
		}
		if w > 0 {
			w += h * 0.04
		}
		label = keyName(ch.Key, false)
		if ch.Key == "enter" {
			if draw {
				pxReturn(c, x+w, base, ink)
			}
			return w + 11
		}
	}
	face := c.Face(false, true, size)
	if r := []rune(label); strings.ContainsAny(label, "←↑→↓") || len(r) == 1 && !unicode.IsLetter(r[0]) && !unicode.IsDigit(r[0]) {
		// Arrows and punctuation read better in the monospace face: a
		// backquote in the sans one is a speck.
		face = c.Face(true, true, size)
	}
	if draw {
		return w + c.Text(face, x+w, base, label, ink)
	}
	return w + c.Width(face, label)
}

// pxTrigger draws a trigger as key caps, the steps of a sequence apart, and
// returns its width; with draw false it only measures.
func (m *Model) pxTrigger(c *pix.Canvas, p pxPal, t ghostty.Trigger, x, cy, h float32, tone capTone, draw bool) float32 {
	w := float32(0)
	sep := c.Face(false, false, h*0.6)
	for i, ch := range t {
		if i > 0 {
			gap := h * 0.25
			if draw {
				c.Text(sep, x+w+gap, cy+h*0.6*0.36, "›", p.muted)
			}
			w += gap*2 + c.Width(sep, "›")
		}
		w += m.pxChord(c, p, ch, x+w, cy, h, tone, draw)
	}
	return w
}

// pxShortcuts draws a row's shortcuts right-aligned at right, as many as fit
// from left, and returns where they start.
func (m *Model) pxShortcuts(c *pix.Canvas, p pxPal, f pxFonts, r *keyRow, right, cy, left float32) float32 {
	const h = 22
	ss := uniqueShortcuts(r.shortcuts)
	x := right
	global := false
	for i := len(ss) - 1; i >= 0; i-- {
		s := ss[i]
		tone := capDefault
		if s.User {
			tone = capUser
		}
		w := m.pxTrigger(c, p, s.Trigger, 0, cy, h, tone, false)
		if x-w < left {
			break
		}
		x -= w
		m.pxTrigger(c, p, s.Trigger, x, cy, h, tone, true)
		x -= 8
		global = global || s.Has("global")
	}
	if len(ss) == 0 && len(r.defaults) > 0 {
		word := "unbound"
		w := c.Width(f.small, word)
		c.Text(f.small, right-w, mid(cy-10, 20, 12.5), word, p.muted)
		return right - w
	}
	if global {
		word := "global"
		w := c.Width(f.small, word)
		if x-w-4 >= left {
			x -= w + 4
			c.Text(f.small, x, mid(cy-10, 20, 12.5), word, p.accent)
		}
	}
	return x
}

// pxKeys is the screen of the shortcut editor.
func (m *Model) pxKeys(c *pix.Canvas, p pxPal, f pxFonts, g pxGeo) {
	// The left column: the groups, with how many actions each lists.
	var items []railItem
	cur := -1
	for i, o := range m.results {
		if o.header {
			items = append(items, railItem{label: o.label, kind: hitRail, index: i, name: o.label})
			continue
		}
		if len(items) > 0 {
			n := atoiDefault(items[len(items)-1].count, 0)
			items[len(items)-1].count = itoa(n + 1)
			if o.current {
				items[len(items)-1].dot = true
			}
			if i == m.sel.cursor {
				cur = len(items) - 1
			}
		}
	}
	if cur >= 0 {
		items[cur].active = true
	}
	m.pxRail(c, p, f, g, "Shortcuts", items, maxInt(cur, 0), 0)

	hw := c.Text(f.h1, g.bodyX, g.bodyY+21, "Keyboard shortcuts", p.fg)
	listW := minF(488, g.bodyW*0.52)
	sideX := g.bodyX + listW + 30
	sideW := g.W - g.M - sideX
	if sideW < 260 {
		listW, sideW = g.bodyW, 0
	}
	note := ""
	if m.kb.loaded {
		n, mine := 0, 0
		for _, o := range m.results {
			if !o.header {
				n++
				if o.current {
					mine++
				}
			}
		}
		note = itoa(n) + " actions"
		if mine > 0 {
			note += " · " + itoa(mine) + " with yours"
		}
		if !m.kb.cat.DefaultsKnown {
			note += " · Ghostty's defaults unknown"
		}
	}
	c.Text(f.body, g.bodyX+hw+12, g.bodyY+21, c.Fit(f.body, note, listW-hw-12), p.muted)

	top := g.bodyY + 44
	if len(m.results) == 0 {
		c.Text(f.body, g.bodyX+14, top+20, "Nothing matches — Esc clears", p.muted)
		return
	}
	const rowKeyH = 38
	spots := make([]menuSpot, len(m.results))
	y := float32(0)
	for i, o := range m.results {
		if o.header {
			if i > 0 {
				y += 12
			}
			spots[i] = menuSpot{y: y, h: headH}
			y += headH
			continue
		}
		spots[i] = menuSpot{y: y, h: rowKeyH}
		y += rowKeyH
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
			c.Text(f.h2, g.bodyX+14, mid(sy, headH, 15), o.label, p.fg)
			if o.detail != "" {
				w := c.Width(f.h2, o.label)
				c.Text(f.small, g.bodyX+14+w+10, mid(sy, headH, 12.5), c.Fit(f.small, o.detail, listW-w-40), p.muted)
			}
			continue
		}
		m.pxKeyRow(c, p, f, o, i == m.sel.cursor, g.bodyX, sy, listW, rowKeyH)
		m.pxRegion(c, g.bodyX, sy, listW, rowKeyH, hitResult, i, "")
	}
	c.Unclip()

	if sideW > 0 {
		m.pxKeySide(c, p, f, m.selected(), sideX, g.bodyY, sideW, g.viewEnd-g.bodyY)
	}
}

// pxKeyRow is one action: a mark when the user set its shortcut, its name,
// and its shortcuts as caps at the right.
func (m *Model) pxKeyRow(c *pix.Canvas, p pxPal, f pxFonts, o option, sel bool, x, y, w, h float32) {
	if sel {
		c.Rect(x, y, w, h, 8, p.rowSel)
	}
	if o.current {
		pxDot(c, x+7, y+h/2, p.accent)
	}
	face, col := f.row, p.text2
	if sel {
		face, col = f.rowB, p.fg
	}
	labelW := c.Width(face, o.label)
	left := x + 14 + minF(labelW, w*0.45) + 16
	start := x + w - 12
	if o.key != nil && o.value != "@bind" {
		start = m.pxShortcuts(c, p, f, o.key, x+w-12, y+h/2, left)
	} else if o.detail != "" {
		dw := c.Width(f.mono, o.detail)
		c.Text(f.mono, x+w-12-dw, mid(y, h, 12.5), c.Fit(f.mono, o.detail, w*0.5), p.muted)
		start = x + w - 12 - dw
	}
	c.Text(face, x+14, mid(y, h, 14), c.Fit(face, o.label, start-(x+14)-12), col)
}

// pxKeySide is the panel beside the list: what the action does, each of its
// shortcuts with where it comes from, and what the keys would do.
func (m *Model) pxKeySide(c *pix.Canvas, p pxPal, f pxFonts, o *option, x, y, w, h float32) {
	c.Clip(x, y, w, h)
	defer c.Unclip()
	m.pxRegion(c, x, y, w, h, hitPreview, 0, "")
	if o == nil || o.key == nil {
		msg := "Type an action, a word of what it does, or a shortcut like cmd+t. trigger=action writes a binding as it is."
		if !m.kb.loaded {
			msg = "Reading the actions from Ghostty…"
		}
		yy := y + 20
		for _, l := range c.Wrap(f.body, msg, w-12) {
			c.Text(f.body, x, yy, l, p.muted)
			yy += 21
		}
		return
	}
	r := o.key
	yy := y
	c.Text(f.h3, x, yy+18, c.Fit(f.h3, r.label, w), p.fg)
	yy += 30
	if o.value == "@bind" {
		c.Text(f.mono, x, yy+14, c.Fit(f.mono, "keybind = "+o.detail, w), p.muted)
		yy += 34
		for _, l := range c.Wrap(f.body, "Enter writes this line next to your other bindings, asking first when it takes a shortcut from another action.", w-12) {
			c.Text(f.body, x, yy+14, l, p.text2)
			yy += 21
		}
		return
	}
	c.Text(f.mono, x, yy+14, c.Fit(f.mono, r.action, w), p.muted)
	yy += 34
	if r.summary != "" {
		lines := c.Wrap(f.body, r.summary, w-12)
		if len(lines) > 5 {
			lines = append(lines[:4], c.Fit(f.body, lines[4]+" …", w-12))
			lines = lines[:5]
		}
		for _, l := range lines {
			c.Text(f.body, x, yy+14, l, p.text2)
			yy += 21
		}
	}

	yy += 24
	pxCaps(c, f.caps, x, yy+10, "Shortcuts", p.muted)
	yy += 22
	facts := m.keyFacts(r)
	if len(facts) == 0 {
		c.Text(f.body, x, yy+20, "None yet", p.muted)
		yy += 32
	}
	global := false
	for _, fa := range facts {
		tone := capDefault
		switch {
		case fa.gone:
			tone = capGone
		case fa.user:
			tone = capUser
		}
		cy := yy + 16
		cw := m.pxTrigger(c, p, fa.trigger, x, cy, 24, tone, true)
		note := fa.note
		if fa.global {
			note = "global · " + note
			global = true
		}
		nx := x + cw + 14
		col := p.muted
		if fa.user {
			col = p.text2
		}
		c.Text(f.small, nx, mid(cy-10, 20, 12.5), c.Fit(f.small, note, x+w-nx), col)
		yy += 34
	}
	if global || globalAction(r.action) {
		yy += 6
		text := "Global shortcuts work from any app once Ghostty is allowed under Privacy & Security → Accessibility"
		for _, l := range c.Wrap(f.small, text, w-12) {
			c.Text(f.small, x, yy+12, l, p.muted)
			yy += 18
		}
	}

	yy += 22
	pxCaps(c, f.caps, x, yy+10, "Keys", p.muted)
	yy += 24
	keyCol := float32(0)
	does := m.keyDoes(r)
	for _, d := range does {
		keyCol = maxF(keyCol, pxKeyLabel(c, f.monoS, 0, 0, d[0], p.fg, false)+14)
	}
	for _, d := range does {
		kw := pxKeyLabel(c, f.monoS, 0, 0, d[0], p.fg, false) + 14
		c.Rect(x, yy, kw, 20, 5, p.keycap)
		pxKeyLabel(c, f.monoS, x+7, mid(yy, 20, 11.5), d[0], p.fg, true)
		c.Text(f.small, x+keyCol+12, mid(yy, 20, 12.5), c.Fit(f.small, d[1], w-keyCol-12), p.text2)
		yy += 28
	}
	yy += 14
	c.Text(f.small, x, yy+12, c.Fit(f.small, shortenPath(m.tree.Primary.Path), w), p.muted)
}

// pxCapture is the dialog that asks for a shortcut: the keys pressed so far
// as caps, what to do, and the global switch where it applies.
func (m *Model) pxCapture(c *pix.Canvas, p pxPal, f pxFonts, g pxGeo, card func(w, h float32) (float32, float32)) {
	w := minF(580, g.W-40)
	intro := c.Wrap(f.body, "Press the keys, or type them: super+shift+t, or ctrl+a>n for a sequence.", w-56)
	hint := m.kb.err
	hintCol := pix.Hex(p.ui.warn, 1)
	if hint == "" {
		hint = "Shortcuts Ghostty already uses never reach this window: type those."
		hintCol = p.muted
	}
	hints := c.Wrap(f.small, hint, w-56)
	global := globalAction(m.kb.action)
	h := float32(70+len(intro)*21) + 4 + 64 + 14 + float32(len(hints))*18 + 28 + 52
	if global {
		h += 48
	}
	x, y := card(w, h)
	tw := c.Text(f.h3, x+28, y+40, "New shortcut", p.fg)
	c.Text(f.body, x+28+tw+12, y+40, c.Fit(f.body, keyLabel(m.kb.action), w-56-tw-12), p.muted)
	yy := y + 70
	for _, l := range intro {
		c.Text(f.body, x+28, yy, l, p.text2)
		yy += 21
	}
	yy += 4

	// The field: the shortcut as caps once it reads, the typed text until
	// then.
	fx, fw := x+28, w-56
	c.Rect(fx, yy, fw, 64, 10, p.bg)
	cy := yy + 32
	tr, err := m.captured()
	switch {
	case m.kb.text == "":
		c.Rect(fx+20, cy-11, 1.5, 22, 0, p.accent)
		c.Text(f.body, fx+28, mid(cy-10, 20, 13.5), "Waiting for keys…", p.muted)
	case err == nil:
		cw := m.pxTrigger(c, p, tr, fx+18, cy, 34, capUser, true)
		if strings.HasSuffix(m.kb.text, ">") {
			c.Rect(fx+18+cw+10, cy-11, 1.5, 22, 0, p.accent)
		}
		raw := tr.String()
		if m.kb.global {
			raw = "global:" + raw
		}
		rw := c.Width(f.monoS, raw)
		if fx+18+cw+24+rw < fx+fw-16 {
			c.Text(f.monoS, fx+fw-16-rw, mid(cy-10, 20, 11.5), raw, p.muted)
		}
	default:
		tw := c.Text(f.mono, fx+20, mid(cy-10, 20, 13), c.Fit(f.mono, m.kb.text, fw-40), p.fg)
		c.Rect(fx+20+tw+2, cy-11, 1.5, 22, 0, p.accent)
	}
	yy += 64 + 14
	for _, l := range hints {
		c.Text(f.small, x+28, yy+12, l, hintCol)
		yy += 18
	}

	if global {
		yy += 16
		sy := yy + 14
		on := m.kb.global
		bg, kx := p.track, x+28+2
		if on {
			bg, kx = p.accent, x+28+16
		}
		c.Rect(x+28, sy-10, 34, 20, 10, bg)
		c.Circle(kx+8, sy, 8, pix.Hex("#f6f7f9", 1))
		lw := c.Text(f.rowB, x+28+46, mid(sy-10, 20, 14), "Global", p.fg)
		c.Text(f.small, x+28+46+lw+10, mid(sy-10, 20, 12.5), c.Fit(f.small, "works from any app · needs Accessibility", w-56-46-lw-10-60), p.muted)
		kw := c.Width(f.monoS, "Tab") + 14
		c.Rect(x+w-28-kw, sy-10, kw, 20, 5, p.keycap)
		c.Text(f.monoS, x+w-28-kw+7, mid(sy-10, 20, 11.5), "Tab", p.fg)
		m.pxRegion(c, x+28, sy-14, w-56, 28, hitButton, 2, "capture-global")
		yy += 32
	}

	by := y + h - 52
	bx := x + 28
	label := "Set"
	sw := c.Width(f.rowB, label) + 14 + 11 + 28
	c.Rect(bx, by, sw, 32, 8, p.accent)
	lw := c.Text(f.rowB, bx+14, mid(by, 32, 14), label, p.onAccent)
	pxReturn(c, bx+14+lw+8, mid(by, 32, 14)+1, p.onAccent)
	m.pxRegion(c, bx, by, sw, 32, hitButton, 0, "capture-set")
	bx += sw + 10
	cancel := "Cancel"
	nw := c.Width(f.row, cancel) + c.Width(f.monoS, "Esc") + 44
	c.Rect(bx, by, nw, 32, 8, p.keycap)
	cw := c.Text(f.row, bx+14, mid(by, 32, 14), cancel, p.fg)
	c.Text(f.monoS, bx+14+cw+10, mid(by, 32, 11.5), "Esc", p.muted)
	m.pxRegion(c, bx, by, nw, 32, hitButton, 1, "capture-cancel")
	c.Text(f.small, bx+nw+16, mid(by, 32, 12.5), "Backspace erases", p.muted)
}
