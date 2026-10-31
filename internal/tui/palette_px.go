package tui

import (
	stdcolor "image/color"
	"strings"

	"golang.org/x/image/font"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
	"github.com/vitruves/ghostty-config/internal/pix"
)

// Palette in the picture interface: a column down the middle of the window,
// the search line at its top, the filters under it, then the list. The
// highlighted setting opens into a quiet ground of its own; the highlighted
// theme is shown at the foot of the column in its own colours. Nothing is
// framed: a group is told by a title in small capitals, the highlight by a
// ground, and the accent marks only what is on.

// palPx is where the parts of the column sit, in logical units.
type palPx struct {
	x, w           float32 // the column
	fieldY, fieldH float32
	chipsY         float32
	listY, listEnd float32
	stripY, stripH float32 // the theme's strip; stripH is 0 without one
	statusY, footY float32
}

const (
	palChipH = 26
	palHeadH = 28
	palNoteH = 36
	palLineH = 20 // a line of an opened setting's explanation
)

func palPxLayout(c *pix.Canvas, strip bool) palPx {
	W, H := c.W(), c.H()
	l := palPx{w: W - 32}
	if W > 900 {
		// Wide windows keep the column to a reading width.
		l.w = minF(maxF(W*0.72, 860), 980)
	}
	l.x = (W - l.w) / 2
	compact := H < 600
	l.fieldY = clampF(H*0.035, 12, 30)
	l.fieldH = 52
	if compact {
		l.fieldH = 46
	}
	l.chipsY = l.fieldY + l.fieldH + 12
	l.listY = l.chipsY + palChipH + 14
	l.footY = H - 40
	l.statusY = l.footY - 26
	end := l.statusY - 8
	if strip {
		l.stripH = 84
		if compact {
			l.stripH = 70
		}
		l.stripY = end - l.stripH
		end = l.stripY - 12
	}
	l.listEnd = end
	return l
}

func (p *palette) drawPixel(m *Model, c *pix.Canvas, pal pxPal, f pxFonts, g pxGeo) {
	p.refresh(m)
	r := p.current()
	strip := r != nil && r.kind == palTheme
	l := palPxLayout(c, strip)
	p.pxField(m, c, pal, f, l)
	p.pxChips(m, c, pal, f, l)
	p.pxList(m, c, pal, f, l)
	if strip {
		p.pxStrip(m, c, pal, l, r.theme)
	}
	p.pxStatus(m, c, pal, f, l)
	p.pxFooter(m, c, pal, f, l)
}

// pxField is the search line: the glass, what is typed or what may be, and
// how many of each kind it found.
func (p *palette) pxField(m *Model, c *pix.Canvas, pal pxPal, f pxFonts, l palPx) {
	y, h := l.fieldY, l.fieldH
	c.Rect(l.x, y, l.w, h, 13, pal.field)
	m.pxRegion(c, l.x, y, l.w, h, hitPrompt, 0, "")
	cy := y + h/2
	c.Ring(l.x+20, cy-9, 15, 15, 7.5, 1.8, pal.muted)
	c.Line(l.x+32.5, cy+3.5, l.x+37.5, cy+8.5, 2, pal.muted)

	size := float32(19)
	if h < 50 {
		size = 17.5
	}
	big := c.Face(false, false, size)
	tx := l.x + 54
	right := l.x + l.w - 20
	if count := p.countLine(); count != "" {
		cw := c.Width(f.small, count)
		c.Text(f.small, right-cw, mid(y, h, 12.5), count, pal.muted)
		right -= cw + 24
	}
	if p.scope != nil {
		// The command whose values are listed, as a scope before the text.
		label := palLabel(p.scope)
		lw := c.Width(f.smallB, label) + 20
		c.Rect(tx, cy-13, lw, 26, 7, pal.keycap)
		c.Text(f.smallB, tx+10, mid(cy-13, 26, 12.5), label, pal.fg)
		tx += lw + 12
	}
	base := mid(y, h, size)
	if p.query == "" {
		ph := "Search themes, fonts, settings, shortcuts and commands"
		if p.scope != nil {
			ph = "Filter its values"
		}
		c.Rect(tx-1, cy-12, 2, 24, 1, pal.accent)
		c.Text(big, tx+8, base, c.Fit(big, ph, right-tx-8), pal.muted)
		return
	}
	// The end of what is typed stays in view, where the caret is.
	shown := p.query
	for c.Width(big, shown) > right-tx-8 && shown != "" {
		shown = string([]rune(shown)[1:])
	}
	w := c.Text(big, tx, base, shown, pal.fg)
	c.Rect(tx+w+2, cy-12, 2, 24, 1, pal.accent)
}

// pxChips are the filters, or, in a command's values, the way back.
func (p *palette) pxChips(m *Model, c *pix.Canvas, pal pxPal, f pxFonts, l palPx) {
	y, h := l.chipsY, float32(palChipH)
	tb := mid(y, h, 12.5)
	if p.scope != nil {
		x := l.x + 2
		c.Line(x+8, y+h/2-4, x+4, y+h/2, 1.4, pal.muted)
		c.Line(x+4, y+h/2, x+8, y+h/2+4, 1.4, pal.muted)
		bw := c.Text(f.small, x+14, tb, "Back", pal.muted)
		m.pxRegion(c, l.x, y, bw+24, h, hitRail, -1, "back")
		x += bw + 36
		c.Text(f.small, x, tb, c.Fit(f.small, p.scope.desc, l.x+l.w-x), pal.muted)
		return
	}
	x := l.x
	for i, name := range palFilters {
		w := c.Width(f.small, name) + 24
		if i == p.filter {
			c.Rect(x, y, w, h, 7, pal.accent)
			c.Text(f.smallB, x+(w-c.Width(f.smallB, name))/2, tb, name, pal.onAccent)
		} else {
			c.Text(f.small, x+12, tb, name, pal.muted)
		}
		m.pxRegion(c, x, y, w, h, hitRail, i, name)
		x += w + 6
	}
}

// palSpot is where a row of the list sits, from the top of the whole list.
type palSpot struct{ y, h float32 }

// expands is whether the highlighted row opens in place: a setting, or a
// value of one, says what it does and what it writes.
func (p *palette) expands(m *Model, r palRow) bool {
	switch r.kind {
	case palSetting:
		return true
	case palValue:
		k, _ := m.palKindOf(p.scope)
		return k == palSetting
	}
	return false
}

// expandedCmd is the setting an opened row explains, and its value.
func (p *palette) expandedCmd(m *Model, r palRow) (*command, string) {
	if r.kind == palValue {
		return p.scope, r.opt.value
	}
	_, v := m.palFace(r.cmd)
	return r.cmd, v
}

// pxSpots lays the list out: rows, titles with air above them, and the
// highlighted setting as tall as what it says.
func (p *palette) pxSpots(m *Model, c *pix.Canvas, pal pxPal, f pxFonts, w float32) ([]palSpot, float32) {
	spots := make([]palSpot, len(p.rows))
	y := float32(0)
	for i, r := range p.rows {
		h := float32(rowH)
		switch r.kind {
		case palHead:
			if i > 0 {
				y += 12
			}
			h = palHeadH
		case palNote:
			h = palNoteH
		}
		if i == p.sel && p.expands(m, r) {
			cmd, v := p.expandedCmd(m, r)
			h = p.pxBody(m, c, pal, f, cmd, v, 0, 0, w, false)
			y += 3
		}
		spots[i] = palSpot{y, h}
		y += h
		if i == p.sel && p.expands(m, r) {
			y += 3
		}
	}
	return spots, y
}

func (p *palette) pxList(m *Model, c *pix.Canvas, pal pxPal, f pxFonts, l palPx) {
	spots, total := p.pxSpots(m, c, pal, f, l.w)
	viewH := l.listEnd - l.listY
	p.page = maxInt(int(viewH/rowH)-1, 1)
	if p.sel >= 0 && p.sel < len(spots) {
		s := spots[p.sel]
		top := s.y
		if p.sel > 0 && p.rows[p.sel-1].kind == palHead {
			// A first row brings its title with it.
			top = spots[p.sel-1].y
		}
		if top < p.topPx {
			p.topPx = top
		}
		if s.y+s.h > p.topPx+viewH {
			p.topPx = s.y + s.h - viewH
		}
	}
	p.topPx = clampF(p.topPx, 0, maxF(total-viewH, 0))

	first := len(m.regions)
	c.Clip(l.x-8, l.listY, l.w+16, viewH)
	for i, r := range p.rows {
		s := spots[i]
		y := l.listY + s.y - p.topPx
		// Only whole rows are drawn, and a title only with its first row.
		if y < l.listY-0.5 || y+s.h > l.listEnd+0.5 {
			continue
		}
		if r.kind == palHead && i+1 < len(spots) && l.listY+spots[i+1].y-p.topPx+spots[i+1].h > l.listEnd+0.5 {
			continue
		}
		sel := i == p.sel
		if r.selectable() {
			// The row first, so the regions of its control lie on top of it.
			y0, y1 := maxF(y, l.listY), minF(y+s.h, l.listEnd)
			m.pxRegion(c, l.x, y0, l.w, y1-y0, hitResult, i, "")
		}
		switch {
		case r.kind == palHead:
			w := palCaps(c, f, l.x+14, y+19, r.text, pal.muted)
			if r.count != "" {
				c.Text(f.small, l.x+14+w+10, y+19, r.count, pal.muted)
			}
		case r.kind == palNote:
			c.Text(f.body, l.x+14, mid(y, palNoteH, 13.5), c.Fit(f.body, r.text, l.w-28), pal.muted)
		case sel && p.expands(m, r):
			cmd, v := p.expandedCmd(m, r)
			c.Rect(l.x, y, l.w, s.h, 12, pal.field)
			if r.kind == palValue {
				p.pxValue(m, c, pal, f, r, true, l.x, y, l.w)
			} else {
				p.pxSetting(m, c, pal, f, r, i, true, l.x, y, l.w)
			}
			p.pxBody(m, c, pal, f, cmd, v, l.x, y, l.w, true)
		case r.kind == palTheme:
			p.pxTheme(m, c, pal, f, r.theme, sel, l.x, y, l.w)
		case r.kind == palFont:
			p.pxFont(m, c, pal, f, r.opt, sel, l.x, y, l.w)
		case r.kind == palSetting:
			p.pxSetting(m, c, pal, f, r, i, sel, l.x, y, l.w)
		case r.kind == palShortcut:
			p.pxShortcut(m, c, pal, f, r.opt, sel, l.x, y, l.w)
		case r.kind == palCommand:
			p.pxCommand(m, c, pal, f, r.cmd, sel, l.x, y, l.w)
		case r.kind == palValue:
			p.pxValue(m, c, pal, f, r, sel, l.x, y, l.w)
		}
	}
	c.Unclip()
	// A control half out of the list must not answer a click beside it.
	e := m.px
	top := int(l.listY*c.S/float32(e.ch) + 0.5)
	bottom := int(l.listEnd*c.S/float32(e.ch) + 0.5)
	kept := m.regions[:first]
	for _, rg := range m.regions[first:] {
		y0, y1 := maxInt(rg.y, top), minInt(rg.y+rg.h, bottom)
		if y1 > y0 {
			rg.y, rg.h = y0, y1-y0
			kept = append(kept, rg)
		}
	}
	m.regions = kept
}

// palCaps writes a title in spaced capitals and returns its width.
func palCaps(c *pix.Canvas, f pxFonts, x, y float32, s string, col stdcolor.Color) float32 {
	w := float32(0)
	for _, r := range strings.ToUpper(s) {
		w += c.Text(f.caps, x+w, y, string(r), col) + 0.9
	}
	return w
}

// pxSel grounds the highlighted row.
func pxSel(c *pix.Canvas, pal pxPal, sel bool, x, y, w float32) {
	if sel {
		c.Rect(x, y, w, rowH, 9, pal.rowSel)
	}
}

// rowFace is the weight and tone of a row's name: the highlighted one leads.
func rowFace(pal pxPal, f pxFonts, sel bool) (font.Face, stdcolor.NRGBA) {
	if sel {
		return f.rowB, pal.fg
	}
	return f.row, pal.text2
}

// pxTheme is a theme: its colours as a strip, its name, its family, how it
// reads, and whether it is applied or starred.
func (p *palette) pxTheme(m *Model, c *pix.Canvas, pal pxPal, f pxFonts, t *ghostty.Theme, sel bool, x, y, w float32) {
	pxSel(c, pal, sel, x, y, w)
	facts := m.factsOf(t)
	sp := facts.spec
	cols := []string{sp.BG, sp.Pal[1], sp.Pal[2], sp.Pal[3], sp.Pal[4], sp.Pal[5], sp.Pal[6], sp.FG}
	sw, sh := clampF(w*0.16, 80, 150), float32(16)
	sx, sy := x+14, y+(rowH-sh)/2
	seg := sw / float32(len(cols))
	c.Rect(sx, sy, sw, sh, 4, pix.Hex(cols[len(cols)-1], 1))
	c.Rect(sx, sy, seg+5, sh, 4, pix.Hex(cols[0], 1))
	for i := 1; i < len(cols)-1; i++ {
		c.Rect(sx+float32(i)*seg, sy, seg+0.6, sh, 0, pix.Hex(cols[i], 1))
	}
	c.Ring(sx, sy, sw, sh, 4, 1, pix.Hex(pal.ui.fg, 0.10))

	right := x + w - 14
	tb := mid(y, rowH, 12.5)
	if m.state.IsFavorite(t.Name) {
		c.Star(right-5, y+rowH/2, 5, pal.accent)
		right -= 18
	}
	if t.Name == m.applied {
		aw := c.Width(f.small, "applied")
		c.Text(f.small, right-aw, tb, "applied", pal.accent)
		right -= aw + 16
	}
	meta := facts.palMeta
	mw := c.Width(f.small, meta)
	c.Text(f.small, right-mw, tb, meta, pal.muted)
	right -= mw + 20

	face, col := rowFace(pal, f, sel)
	nx := sx + sw + 18
	nameW := clampF(w*0.27, 150, 250)
	if w < 640 || nx+nameW > right {
		nameW = right - nx
	}
	c.Text(face, nx, mid(y, rowH, 14), c.Fit(face, t.Name, nameW-12), col)
	if fx := nx + nameW; fx < right-40 {
		c.Text(f.small, fx, tb, c.Fit(f.small, palThemeFamily(t), right-fx), pal.muted)
	}
}

// pxFont is a family: its name, what it is, and whether it is the one in use.
func (p *palette) pxFont(m *Model, c *pix.Canvas, pal pxPal, f pxFonts, o option, sel bool, x, y, w float32) {
	pxSel(c, pal, sel, x, y, w)
	right := x + w - 14
	tb := mid(y, rowH, 12.5)
	if o.current {
		aw := c.Width(f.small, "in use")
		c.Text(f.small, right-aw, tb, "in use", pal.accent)
		right -= aw + 16
	}
	face, col := rowFace(pal, f, sel)
	nameW := clampF(w*0.4, 180, 360)
	c.Text(face, x+14, mid(y, rowH, 14), c.Fit(face, o.label, nameW-12), col)
	detail := o.detail
	if strings.HasPrefix(o.value, downloadPrefix) {
		detail = "not installed · Enter downloads it"
	}
	c.Text(f.small, x+14+nameW, tb, c.Fit(f.small, detail, right-x-14-nameW), pal.muted)
}

// pxSetting is a setting: a mark when it is changed, its name, where it
// lives, and its control. The highlighted one shows all its values.
func (p *palette) pxSetting(m *Model, c *pix.Canvas, pal pxPal, f pxFonts, r palRow, idx int, sel bool, x, y, w float32) {
	if sel && !p.expands(m, r) {
		pxSel(c, pal, sel, x, y, w)
	}
	cmd := r.cmd
	s, v := m.palFace(cmd)
	if m.palChanged(cmd) {
		pxDot(c, x+11, y+rowH/2, pal.accent)
	}
	face, col := rowFace(pal, f, sel)
	label := palLabel(cmd)
	lx := x + 26
	labelW := clampF(w*0.36, 170, 330)
	right := x + w - 14
	minLeft := lx + minF(labelW, c.Width(face, label)+24)
	if !sel && !s.numeric && s.list == nil && !isSwitchSetting(s) {
		// A word beside the others reads as the value between the arrows.
		minLeft = right
	}
	ctrl := m.pxControl(c, pal, f, s, v, idx, right, y, minLeft)
	where := lx + labelW
	room := labelW - 14
	if where > ctrl-60 {
		room = ctrl - lx - 16
	}
	c.Text(face, lx, mid(y, rowH, 14), c.Fit(face, label, room), col)
	if where <= ctrl-60 {
		c.Text(f.small, where, mid(y, rowH, 12.5), c.Fit(f.small, palWhere(cmd), ctrl-where-18), pal.muted)
	}
}

func isSwitchSetting(s *setting) bool {
	return len(s.options) == 2 && s.options[0] == "on" && s.options[1] == "off"
}

// pxValue is one value of the command Enter opened.
func (p *palette) pxValue(m *Model, c *pix.Canvas, pal pxPal, f pxFonts, r palRow, sel bool, x, y, w float32) {
	if sel && !p.expands(m, r) {
		pxSel(c, pal, sel, x, y, w)
	}
	o := r.opt
	if o.current {
		pxDot(c, x+11, y+rowH/2, pal.accent)
	}
	lx := x + 26
	if color.IsHex(o.swatch) {
		c.Rect(lx, y+rowH/2-8, 16, 16, 4, pix.Hex(o.swatch, 1))
		lx += 26
	}
	face, col := rowFace(pal, f, sel)
	label := o.label
	if p.scope.set != nil {
		label = displayValue(label)
	}
	detail := o.detail
	if detail == "current" {
		detail = ""
	}
	dw := minF(c.Width(f.small, detail), (w-(lx-x))*0.55)
	detail = c.Fit(f.small, detail, dw+1)
	c.Text(face, lx, mid(y, rowH, 14), c.Fit(face, label, x+w-14-dw-20-lx), col)
	c.Text(f.small, x+w-14-c.Width(f.small, detail), mid(y, rowH, 12.5), detail, pal.muted)
}

// pxShortcut is an action of Ghostty's with the shortcuts it has.
func (p *palette) pxShortcut(m *Model, c *pix.Canvas, pal pxPal, f pxFonts, o option, sel bool, x, y, w float32) {
	pxSel(c, pal, sel, x, y, w)
	if m.keysCustomised(o.key) {
		pxDot(c, x+11, y+rowH/2, pal.accent)
	}
	face, col := rowFace(pal, f, sel)
	lx := x + 26
	labelW := clampF(w*0.36, 170, 330)
	left := m.pxShortcuts(c, pal, f, o.key, x+w-14, y+rowH/2, lx+labelW*0.6)
	room := labelW - 14
	if lx+labelW > left-40 {
		room = left - lx - 16
	}
	c.Text(face, lx, mid(y, rowH, 14), c.Fit(face, o.label, room), col)
	if where := lx + labelW; where < left-40 {
		c.Text(f.small, where, mid(y, rowH, 12.5), c.Fit(f.small, "Shortcut · "+o.key.group, left-where-18), pal.muted)
	}
}

// pxCommand is one of the tool's commands: what it does, and what it says
// of itself now.
func (p *palette) pxCommand(m *Model, c *pix.Canvas, pal pxPal, f pxFonts, cmd *command, sel bool, x, y, w float32) {
	pxSel(c, pal, sel, x, y, w)
	face, col := rowFace(pal, f, sel)
	lx := x + 26
	right := x + w - 14
	tb := mid(y, rowH, 12.5)
	if cmd.options != nil {
		// A chevron: Enter opens its list.
		right -= 8
		c.Line(right+1, y+rowH/2-4, right+5, y+rowH/2, 1.4, pal.muted)
		c.Line(right+5, y+rowH/2, right+1, y+rowH/2+4, 1.4, pal.muted)
		right -= 10
	}
	if v := m.currentValueOf(cmd); v != "" {
		v = c.Fit(f.small, v, w*0.3)
		vw := c.Width(f.small, v)
		c.Text(f.small, right-vw, tb, v, pal.text2)
		right -= vw + 20
	}
	labelW := clampF(w*0.36, 170, 330)
	c.Text(face, lx, mid(y, rowH, 14), c.Fit(face, palLabel(cmd), labelW-14), col)
	if where := lx + labelW; where < right-60 {
		c.Text(f.small, where, tb, c.Fit(f.small, cmd.desc, right-where), pal.muted)
	}
}

// pxBody is what an opened setting says under its row: what the value does,
// and the line it puts in the config. It returns the height of the whole
// opened row; it only draws when draw is set.
func (p *palette) pxBody(m *Model, c *pix.Canvas, pal pxPal, f pxFonts, cmd *command, value string, x, y, w float32, draw bool) float32 {
	lead, text := m.palExplain(cmd, value)
	key, lines, note := m.palWrites(cmd, value)
	side := w >= 640
	bx := x + 26
	ew := w - 26 - 24
	dw := float32(0)
	if side {
		dw = clampF(w*0.36, 220, 330)
		ew = w - 26 - 28 - dw - 20
	}
	wrapped := c.Wrap(f.body, strings.TrimSpace(lead+" "+text), ew)
	if len(wrapped) > 3 {
		wrapped = append(wrapped[:2], c.Fit(f.body, strings.Join(wrapped[2:], " "), ew))
	}
	diff := 0
	if key != "" {
		diff++
	}
	diff += len(lines)
	if note != "" {
		diff++
	}
	by := y + rowH + 2
	h := rowH + 2 + 14 + float32(len(wrapped))*palLineH
	if side {
		h = rowH + 2 + 14 + float32(maxInt(len(wrapped), diff))*palLineH
	} else {
		h += 8 + float32(diff)*palLineH
	}
	if !draw {
		return h
	}

	for i, line := range wrapped {
		ly := by + 14 + float32(i)*palLineH
		lx := bx
		if i == 0 && lead != "" && strings.HasPrefix(line, lead) {
			lx += c.Text(f.rowB, bx, ly, lead, pal.fg)
			line = strings.TrimPrefix(line, lead)
		}
		c.Text(f.body, lx, ly, line, pal.text2)
	}
	dx, dy := bx, by+float32(len(wrapped))*palLineH+8
	if side {
		dx, dy = x+w-20-dw, by
	}
	ok := pix.Hex(pal.ui.ok, 1)
	if key != "" {
		c.Text(f.monoS, dx, dy+14, c.Fit(f.monoS, key, dw+ew), pal.muted)
		dy += palLineH
	}
	room := dw
	if !side {
		room = ew
	}
	for _, l := range lines {
		c.Text(f.monoS, dx, dy+14, c.Fit(f.monoS, "+ "+l, room), ok)
		dy += palLineH
	}
	if note != "" {
		c.Text(f.small, dx, dy+14, c.Fit(f.small, note, room), pal.muted)
	}
	return h
}

// pxStrip shows the highlighted theme in its own colours: a prompt, a diff,
// its name and how well it reads.
func (p *palette) pxStrip(m *Model, c *pix.Canvas, pal pxPal, l palPx, t *ghostty.Theme) {
	facts := m.factsOf(t)
	sp := facts.spec
	x, y, w, h := l.x, l.stripY, l.w, l.stripH
	fg := pix.Hex(sp.FG, 1)
	c.Rect(x, y, w, h, 14, pix.Hex(sp.BG, 1))
	c.Ring(x, y, w, h, 14, 1, pix.Hex(pal.ui.fg, 0.08))

	mono := c.Face(true, false, 13)
	lh := float32(21)
	if h < 80 {
		lh = 18
	}
	base := y + (h-3*lh)/2 + lh*0.74
	tx := x + 22
	col := func(i int) stdcolor.NRGBA { return pix.Hex(sp.Pal[i], 1) }
	run := func(x, y float32, parts ...any) float32 {
		for i := 0; i+1 < len(parts); i += 2 {
			x += c.Text(mono, x, y, parts[i].(string), parts[i+1].(stdcolor.NRGBA))
		}
		return x
	}
	run(tx, base, "~/src/ghostty-config ", col(2), "main ", col(3), "› git diff --stat", fg)
	run(tx, base+lh, "internal/tui/app.go ", col(4), "| ", fg, "+12 ", col(2), "-3", col(1))
	end := run(tx, base+2*lh, "~/src/ghostty-config ", col(2), "main ", col(3), "› ", fg)
	c.Rect(end, base+2*lh-12, 8, 15, 1, pix.Hex(sp.Accent, 1))

	text, weak, weakName := facts.ratio, facts.weak, facts.weakName
	meta := "text " + ratioText(text)
	if weakName != "" {
		meta += " · weakest " + weakName + " " + ratioText(weak)
	}
	nameF := c.Face(false, true, 15)
	small := c.Face(false, false, 12.5)
	right := x + w - 22
	cy := y + h/2
	c.Text(nameF, right-c.Width(nameF, t.Name), cy-3, t.Name, fg)
	dim := pix.Hex(color.Blend(sp.FG, sp.BG, 0.3), 1)
	c.Text(small, right-c.Width(small, meta), cy+16, meta, dim)
}

// ratioText writes a contrast ratio as the panels do, 13.0:1.
func ratioText(r float64) string { return ftoa1(r) + ":1" }

// ftoa1 writes a ratio with one decimal.
func ftoa1(v float64) string {
	n := int(v*10 + 0.5)
	return itoa(n/10) + "." + itoa(n%10)
}

// pxStatus is the line under the list.
func (p *palette) pxStatus(m *Model, c *pix.Canvas, pal pxPal, f pxFonts, l palPx) {
	text, kind, dot := p.statusText(m)
	tone, dotTone := pal.muted, pal.accent
	switch kind {
	case statusInfo:
		tone = pix.Hex(pal.ui.accent2, 1)
		dotTone = tone
	case statusWarn:
		tone = pix.Hex(pal.ui.warn, 1)
		dotTone = tone
	case statusFail:
		tone = pix.Hex(pal.ui.danger, 1)
		dotTone = tone
	}
	x := l.x + 6
	cy := l.statusY + 11
	if dot {
		pxDot(c, x+3, cy, dotTone)
		x += 16
	}
	c.Text(f.small, x, mid(l.statusY, 22, 12.5), c.Fit(f.small, text, l.x+l.w-x), tone)
}

// pxFooter is the name of the tool and the layout, and the keys.
func (p *palette) pxFooter(m *Model, c *pix.Canvas, pal pxPal, f pxFonts, l palPx) {
	W := c.W()
	cy := l.footY + 20
	tb := mid(cy-10, 20, 12.5)
	x := float32(16)
	x += c.Text(f.small, x, tb, "ghostty-config · ", pal.muted)
	x += c.Text(f.small, x, tb, "Palette", pal.text2)

	pairs := p.keyPairs(m)
	type keycap struct {
		key, word string
		kw, w     float32
	}
	// The global keys (help, docs, settings) are given room first, then the
	// screen's own, in their order, while they fit.
	total := float32(0)
	fit := func(list []string) []keycap {
		var out []keycap
		for i := 0; i+1 < len(list); i += 2 {
			kw := pxKeyLabel(c, f.monoS, 0, 0, list[i], pal.fg, false) + 14
			k := keycap{list[i], list[i+1], kw, kw + 6 + c.Width(f.small, list[i+1])}
			if W-16-total-k.w < x+32 {
				break
			}
			out = append(out, k)
			total += k.w + 18
		}
		return out
	}
	local, global := splitGlobalKeys(pairs)
	globalCaps := fit(global)
	caps := append(fit(local), globalCaps...)
	kx := W - 16 - total + 18
	for _, k := range caps {
		c.Rect(kx, cy-10, k.kw, 20, 5, pal.keycap)
		pxKeyLabel(c, f.monoS, kx+7, mid(cy-10, 20, 11.5), k.key, pal.fg, true)
		c.Text(f.small, kx+k.kw+6, tb, k.word, pal.muted)
		kx += k.w + 18
	}
}
