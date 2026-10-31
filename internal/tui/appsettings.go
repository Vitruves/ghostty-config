package tui

import (
	stdcolor "image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/vitruves/ghostty-config/internal/pix"
)

// F4 opens the editor's own settings from anywhere: its look, the rows of
// Tool → Interface, and its behaviour, what the flags say for one launch.
// The rows are the preferences themselves, so a value is shown and kept the
// way the menu does it: the arrows show it, Enter keeps it, and closing puts
// back whatever was not kept.

// Lock is a preference a flag decided for the session: the flag, and the
// value it stands for.
type Lock struct{ Flag, Value string }

// settingsView is the open dialog.
type settingsView struct {
	at int // the highlighted row
	// list is whether the values of the row are listed, listAt the one
	// highlighted, and from the row's value as the list opened, which Esc
	// puts back.
	list   bool
	listAt int
	from   string
	// layout is the layout the arrows came to, switched to by Enter: the
	// layout takes the whole screen, so it is never shown from under F4.
	layout string
}

// settingsRows are the rows in effect, the look first; look is how many of
// them are of the look.
func (m *Model) settingsRows() (rows []*uiPref, look int) {
	for _, p := range uiPrefs {
		if !p.pixel || m.pixelOn() {
			rows = append(rows, p)
		}
	}
	return append(rows, toolPrefs...), len(rows)
}

func (m *Model) settingsRow() *uiPref {
	rows, _ := m.settingsRows()
	return rows[clampInt(m.ps.at, 0, len(rows)-1)]
}

// lockOf is the flag that decided p for the session, if one did.
func (m *Model) lockOf(p *uiPref) (Lock, bool) {
	l, ok := m.opts.Locked[p.name]
	return l, ok
}

// shownValue is the value the row shows: the flag's, the layout the arrows
// came to, or the preference's.
func (m *Model) shownValue(p *uiPref) string {
	if l, ok := m.lockOf(p); ok {
		return l.Value
	}
	if p == prefLayout && m.ps.layout != "" {
		return m.ps.layout
	}
	return p.value(m)
}

func (p *uiPref) title(v string) string {
	if t, ok := p.titles[v]; ok {
		return t
	}
	return v
}

// listed is whether Enter lists the values: too many to step through, or
// the layout, which only Enter switches.
func (p *uiPref) listed() bool { return len(p.values) > 3 || p.apply != nil }

// unkeptRow is whether the row shows what was not kept.
func (m *Model) unkeptRow(p *uiPref) bool {
	if p == prefLayout {
		return m.ps.layout != "" && m.ps.layout != p.value(m)
	}
	_, ok := p.unkept(m)
	return ok
}

// openSettings opens the dialog on its first row. A look being walked in a
// menu is put back first: it was not kept.
func (m *Model) openSettings() {
	m.endPrefPreview()
	m.ps = settingsView{}
	m.overlay = overlaySettings
}

// closeSettings closes the dialog and puts back what was not kept.
func (m *Model) closeSettings() {
	rows, _ := m.settingsRows()
	unkept := false
	for _, p := range rows {
		unkept = unkept || m.unkeptRow(p)
	}
	m.endPrefPreview()
	m.ps = settingsView{}
	m.overlay = overlayNone
	if unkept {
		m.info("Put back what was not kept with Enter")
	}
}

// updateSettings answers the keys of the dialog.
func (m *Model) updateSettings(key string) (tea.Model, tea.Cmd) {
	rows, _ := m.settingsRows()
	p := m.settingsRow()
	if key == "f4" {
		m.closeSettings()
		return m, nil
	}
	if m.ps.list {
		switch key {
		case "up", "k", "ctrl+p":
			m.listTo(p, m.ps.listAt-1)
		case "down", "j", "ctrl+n":
			m.listTo(p, m.ps.listAt+1)
		case "home":
			m.listTo(p, 0)
		case "end":
			m.listTo(p, len(p.values)-1)
		case "enter":
			return m, m.keepSetting(p, p.values[m.ps.listAt])
		case "esc", "backspace", "left", "q":
			if p != prefLayout {
				p.preview(m, m.ps.from)
			}
			m.ps.list = false
		}
		return m, nil
	}
	switch key {
	case "up", "k", "ctrl+p", "shift+tab":
		m.ps.at = max(0, m.ps.at-1)
	case "down", "j", "ctrl+n", "tab":
		m.ps.at = min(len(rows)-1, m.ps.at+1)
	case "home":
		m.ps.at = 0
	case "end":
		m.ps.at = len(rows) - 1
	case "left", "h":
		m.stepSetting(p, -1)
	case "right", "l", "space", " ":
		m.stepSetting(p, 1)
	case "enter":
		if l, ok := m.lockOf(p); ok {
			m.info("%s is set by %s for this session", p.label, l.Flag)
			return m, nil
		}
		if p.listed() && !(p == prefLayout && m.unkeptRow(p)) {
			m.openList(p)
			return m, nil
		}
		return m, m.keepSetting(p, m.shownValue(p))
	case "esc", "q":
		m.closeSettings()
	}
	return m, nil
}

// stepSetting shows the value before or after the one shown.
func (m *Model) stepSetting(p *uiPref, d int) {
	if l, ok := m.lockOf(p); ok {
		m.info("%s is set by %s for this session", p.label, l.Flag)
		return
	}
	i := indexOf(p.values, m.shownValue(p))
	v := p.values[(i+d+len(p.values))%len(p.values)]
	if p == prefLayout {
		m.ps.layout = v
		return
	}
	p.preview(m, v)
}

// openList lists the values of a row, the one shown highlighted.
func (m *Model) openList(p *uiPref) {
	m.ps.list, m.ps.from = true, p.raw(m)
	m.ps.listAt = max(0, indexOf(p.values, m.shownValue(p)))
}

// listTo highlights a value of the list, and shows it unless it is a layout.
func (m *Model) listTo(p *uiPref, i int) {
	m.ps.listAt = clampInt(i, 0, len(p.values)-1)
	if p == prefLayout {
		m.ps.layout = p.values[m.ps.listAt]
		return
	}
	p.preview(m, p.values[m.ps.listAt])
}

// keepSetting keeps a value, and says so. A layout is switched to through
// the profiles' seam; the dialog stays open over it.
func (m *Model) keepSetting(p *uiPref, v string) tea.Cmd {
	m.ps.list = false
	if p.apply != nil {
		m.ps.layout = ""
		// Switching puts back the rest that was not kept, as leaving would.
		cmd := p.apply(m, v)
		m.info("%s: %s", p.label, p.title(v))
		return cmd
	}
	p.keep(m, v)
	if p.later {
		m.info("%s: %s, from the next launch", p.label, p.title(p.value(m)))
	} else {
		m.info("%s: %s", p.label, p.title(p.value(m)))
	}
	return nil
}

func indexOf(values []string, v string) int {
	for i, x := range values {
		if x == v {
			return i
		}
	}
	return 0
}

// settingNote is what the line under the rows says of the highlighted one.
func (m *Model) settingNote() string {
	p := m.settingsRow()
	if l, ok := m.lockOf(p); ok {
		return sentence(p.desc) + " Set by " + l.Flag + " for this session: a flag wins over what is kept here."
	}
	v := m.shownValue(p)
	if m.ps.list {
		v = p.values[m.ps.listAt]
	}
	note := sentence(p.desc)
	if n := p.notes[v]; n != "" {
		note = title(p.title(v)) + ": " + sentence(n)
	}
	return note
}

// settingTag is what a row says after its value: the flag that set it, or
// when it is honoured.
func (m *Model) settingTag(p *uiPref) string {
	if l, ok := m.lockOf(p); ok {
		return "set by " + l.Flag
	}
	if p.later {
		return "applies at next launch"
	}
	return ""
}

// settingKeys are the keys of the dialog, as key and word.
func (m *Model) settingKeys() []string {
	p := m.settingsRow()
	if m.ps.list {
		word := "keep"
		if p == prefLayout {
			word = "switch"
		}
		return []string{"↑↓", "choose", "Enter", word, "Esc", "back"}
	}
	pairs := []string{"↑↓", "move"}
	if _, locked := m.lockOf(p); !locked {
		pairs = append(pairs, "←→", "change")
		switch {
		case p == prefLayout && m.unkeptRow(p):
			pairs = append(pairs, "Enter", "switch")
		case p.listed():
			pairs = append(pairs, "Enter", "list")
		case m.unkeptRow(p):
			pairs = append(pairs, "Enter", "keep")
		}
	}
	return append(pairs, "Esc", "close")
}

// settingsTitle names the dialog.
const settingsTitle = "ghostty-config settings"

// --- in cells ------------------------------------------------------------------------------

// viewSettings draws the dialog as a card over the screen, in cells.
func (m *Model) viewSettings(body string) string {
	c := m.dialogChrome()
	width := min(78, m.width-2)
	inner := width - 2
	rows, look := m.settingsRows()
	labelW := 0
	for _, p := range rows {
		labelW = max(labelW, lipgloss.Width(p.label))
	}
	labelW = min(labelW+2, inner/2)
	// The widest value a row can show, its arrows and a swatch included.
	valW := 0
	for _, p := range rows {
		for _, v := range p.values {
			valW = max(valW, lipgloss.Width(p.title(v))+6)
		}
	}
	type hit struct {
		line, x, w, index int
		name              string
	}
	var lines []string
	var hits []hit
	cur := m.settingsRow()
	if m.ps.list {
		lines = append(lines, c.bold(c.fg).Render(cur.label))
		for _, l := range wrapPlain(sentence(cur.desc), inner) {
			lines = append(lines, c.mutedS().Render(l))
		}
		lines = append(lines, "")
		valW = 0
		for _, v := range cur.values {
			valW = max(valW, lipgloss.Width(cur.title(v)))
		}
		for i, v := range cur.values {
			st, mute, bg := c.base(), c.mutedS(), c.paint()
			if i == m.ps.listAt {
				st, mute, bg = on(c.selFg, c.selBg), on(c.selFg, c.selBg), c.selBg
			}
			mark := "  "
			if v == cur.value(m) && cur != prefAccent {
				mark = "• "
			}
			row := st.Render(" " + mark)
			if cur == prefAccent {
				row = st.Render(" ") + on(m.accentSample(v), bg).Render("●") + st.Render(" ")
			}
			row += st.Render(pad(cur.title(v), valW+3)) + mute.Render(truncate(cur.notes[v], inner-valW-6))
			hits = append(hits, hit{len(lines), 0, width, i, "ps-pick"})
			lines = append(lines, fillOn(row, inner, st))
		}
	} else {
		for i, p := range rows {
			switch i {
			case 0:
				lines = append(lines, c.mutedS().Render("LOOK"))
			case look:
				lines = append(lines, "", c.mutedS().Render("BEHAVIOUR"))
			}
			sel := i == m.ps.at
			st, mute, val, bg := c.base(), c.mutedS(), c.base(), c.paint()
			if sel {
				st, bg = on(c.selFg, c.selBg), c.selBg
				mute, val = st, st.Bold(true)
			}
			_, locked := m.lockOf(p)
			if m.unkeptRow(p) {
				val = val.Foreground(lipgloss.Color(c.accent))
			}
			if locked {
				val = mute
			}
			v := m.shownValue(p)
			row := st.Render(" " + pad(p.label, labelW))
			vx := 1 + labelW
			lt, rt := "  ", "  "
			if sel && !locked {
				lt, rt = "‹ ", " ›"
			}
			if p == prefAccent {
				row += st.Render(lt) + on(m.accentSample(v), bg).Render("●") + val.Render(" "+p.title(v)) + st.Render(rt)
			} else {
				row += st.Render(lt) + val.Render(p.title(v)) + st.Render(rt)
			}
			vw := lipgloss.Width(row) - vx
			if tag := m.settingTag(p); tag != "" {
				// The tags line up in a column of their own, said shortly
				// where the card is narrow.
				gap := max(1, valW-vw+2)
				if p.later && lipgloss.Width(row)+gap+lipgloss.Width(tag) > inner {
					tag = "next launch"
				}
				row += mute.Render(strings.Repeat(" ", gap) + tag)
			}
			hits = append(hits, hit{len(lines), 0, width, i, "ps-row"}, hit{len(lines), vx + 2, vw - 4, i, "ps-value"})
			if sel && !locked {
				hits = append(hits, hit{len(lines), vx, 2, i, "ps-prev"}, hit{len(lines), vx + vw - 2, 2, i, "ps-next"})
			}
			lines = append(lines, fillOn(row, inner, st))
		}
	}
	lines = append(lines, "")
	note := wrapPlain(m.settingNote(), inner)
	if len(note) > 2 {
		note = note[:2]
		note[1] = truncate(note[1]+" …", inner)
	}
	for _, l := range note {
		lines = append(lines, c.mutedS().Render(l))
	}
	for len(note) < 2 {
		note = append(note, "")
		lines = append(lines, "")
	}
	lines = append(lines, c.keyHintFit(inner, m.settingKeys()...))
	if len(lines)+5 > m.height {
		lines = lines[:max(1, m.height-5)]
	}
	// Where the card lands, as overlayCentered places it.
	top := max(0, (m.height-(len(lines)+5))/2)
	left := max(0, (m.width-(width+2))/2)
	m.addRegion(left, top, width+2, len(lines)+5, hitButton, 0, "ps-card")
	for _, h := range hits {
		if h.line < len(lines) {
			m.addRegion(left+2+h.x, top+3+h.line, h.w, 1, hitButton, h.index, h.name)
		}
	}
	return overlayCentered(m, body, m.card(settingsTitle, "Esc closes", lines, width))
}

// fillOn pads a row to width in its own style, so a highlight runs to the
// end of the row.
func fillOn(row string, width int, st lipgloss.Style) string {
	if w := lipgloss.Width(row); w < width {
		row += st.Render(strings.Repeat(" ", width-w))
	}
	return truncate(row, width)
}

// --- the mouse ----------------------------------------------------------------------------

// clickSettings answers a click: a row is highlighted, its value stepped or
// listed, a value of a list kept; a click off the card closes it.
func (m *Model) clickSettings(msg mouseEv) (tea.Model, tea.Cmd) {
	r, ok := m.hit(msg.X, msg.Y)
	if !ok {
		m.closeSettings()
		return m, nil
	}
	rows, _ := m.settingsRows()
	switch r.name {
	case "ps-pick":
		p := m.settingsRow()
		m.listTo(p, r.index)
		return m, m.keepSetting(p, p.values[m.ps.listAt])
	case "ps-row":
		m.ps.at = clampInt(r.index, 0, len(rows)-1)
	case "ps-prev", "ps-next":
		d := 1
		if r.name == "ps-prev" {
			d = -1
		}
		m.stepSetting(m.settingsRow(), d)
	case "ps-value":
		m.ps.at = clampInt(r.index, 0, len(rows)-1)
		p := m.settingsRow()
		if _, locked := m.lockOf(p); !locked && p.listed() {
			m.openList(p)
			return m, nil
		}
		m.stepSetting(p, 1)
	}
	return m, nil
}

// --- in the picture interface ---------------------------------------------------------------

// pxSettings draws the dialog in the picture interface, in the language of
// F3's: a title, quiet group names in capitals, the highlighted row on a
// ground of its own, and the accent only for a value not kept yet.
func (m *Model) pxSettings(c *pix.Canvas, p pxPal, f pxFonts, g pxGeo, card func(w, h float32) (float32, float32)) {
	rows, look := m.settingsRows()
	cur := m.settingsRow()
	w := minF(640, g.W-40)
	textW := w - 56
	note := c.Wrap(f.small, m.settingNote(), textW)
	if len(note) > 2 {
		note = append(note[:1], c.Fit(f.small, note[1]+" "+note[2], textW))
	}
	var desc []string
	n := len(rows)
	// What is not rows: the title, the note and the keys, and above the
	// rows the group names, or the row's own name and what it does.
	fixed := float32(64 + 16 + 2*19 + 30 + 14)
	if m.ps.list {
		desc = c.Wrap(f.small, sentence(cur.desc), textW)
		n = len(cur.values)
		fixed += 28 + float32(len(desc))*18 + 8
	} else {
		fixed += 2*24 + 10
	}
	rh := clampF((g.H-32-fixed)/float32(n), 20, 30)
	h := fixed + float32(n)*rh
	x, y := card(w, h)
	m.pxRegion(c, x, y, w, h, hitButton, 0, "ps-card")
	hint := "Esc closes"
	c.Text(f.h3, x+28, y+40, settingsTitle, p.fg)
	c.Text(f.small, x+w-28-c.Width(f.small, hint), y+40, hint, p.muted)
	yy := y + 64
	chevron := func(cx, cy float32, d float32, col stdcolor.NRGBA) {
		c.Line(cx-2*d, cy-4, cx+2*d, cy, 1.4, col)
		c.Line(cx+2*d, cy, cx-2*d, cy+4, 1.4, col)
	}
	if m.ps.list {
		c.Text(f.rowB, x+28, yy+16, cur.label, p.fg)
		yy += 28
		for _, l := range desc {
			c.Text(f.small, x+28, yy+12, l, p.muted)
			yy += 18
		}
		yy += 8
		valW := float32(0)
		for _, v := range cur.values {
			valW = maxF(valW, c.Width(f.rowB, cur.title(v)))
		}
		for i, v := range cur.values {
			sel := i == m.ps.listAt
			if sel {
				c.Rect(x+16, yy, w-32, rh, 7, p.keycap)
			}
			base := mid(yy, rh, 14)
			vx := x + 40
			if v == cur.value(m) {
				pxDot(c, x+30, base-4.5, p.muted)
			}
			if cur == prefAccent {
				c.Circle(vx+5, base-4.5, 5, pix.Hex(m.accentSample(v), 1))
				vx += 16
			}
			face, col := f.row, p.text2
			if sel {
				face, col = f.rowB, p.fg
			}
			c.Text(face, vx, base, cur.title(v), col)
			nx := vx + valW + 20
			c.Text(f.small, nx, base, c.Fit(f.small, cur.notes[v], x+w-28-nx), p.muted)
			m.pxRegion(c, x+16, yy, w-32, rh, hitButton, i, "ps-pick")
			yy += rh
		}
	} else {
		labelW := float32(0)
		for _, r := range rows {
			labelW = maxF(labelW, c.Width(f.row, r.label))
		}
		labelW = minF(labelW+28, textW/2)
		// The tags line up after the widest value any row can show.
		valW := float32(0)
		for _, r := range rows {
			for _, v := range r.values {
				valW = maxF(valW, c.Width(f.rowB, r.title(v)))
			}
		}
		for i, r := range rows {
			switch i {
			case 0:
				pxCaps(c, f.caps, x+28, yy+15, "Look", p.muted)
				yy += 24
			case look:
				yy += 10
				pxCaps(c, f.caps, x+28, yy+15, "Behaviour", p.muted)
				yy += 24
			}
			sel := i == m.ps.at
			_, locked := m.lockOf(r)
			if sel {
				c.Rect(x+16, yy, w-32, rh, 7, p.keycap)
			}
			base := mid(yy, rh, 14)
			labelCol := p.text2
			if sel {
				labelCol = p.fg
			}
			c.Text(f.row, x+28, base, c.Fit(f.row, r.label, labelW-12), labelCol)
			v := m.shownValue(r)
			vx := x + 28 + labelW
			xx := vx + 14
			if r == prefAccent {
				c.Circle(xx+5, base-4.5, 5, pix.Hex(m.accentSample(v), 1))
				xx += 16
			}
			face, col := f.row, p.text2
			switch {
			case locked:
				col = p.muted
			case m.unkeptRow(r):
				face, col = f.rowB, p.accent
			case sel:
				face, col = f.rowB, p.fg
			}
			xx += c.Text(face, xx, base, r.title(v), col)
			if sel && !locked {
				chevron(vx+4, base-4.5, -1, p.muted)
				chevron(xx+10, base-4.5, 1, p.muted)
				m.pxRegion(c, vx-4, yy, 18, rh, hitButton, i, "ps-prev")
				m.pxRegion(c, xx, yy, 20, rh, hitButton, i, "ps-next")
			}
			m.pxRegion(c, x+16, yy, w-32, rh, hitButton, i, "ps-row")
			m.pxRegion(c, vx+14, yy, xx-vx-14, rh, hitButton, i, "ps-value")
			if tag := m.settingTag(r); tag != "" {
				tx := maxF(xx+26, x+28+labelW+14+16+valW+30)
				c.Text(f.small, tx, base, c.Fit(f.small, tag, x+w-28-tx), p.muted)
			}
			yy += rh
		}
	}
	// The note, its last line a little above the keys.
	ky := y + h - 24
	yy = ky - 30 - float32(len(note)-1)*19
	for _, l := range note {
		c.Text(f.small, x+28, yy, l, p.text2)
		yy += 19
	}
	// The keys, as the footers draw them: the key, then its word.
	kx := x + 28
	pairs := m.settingKeys()
	for i := 0; i+1 < len(pairs); i += 2 {
		kx += pxKeyLabel(c, f.monoS, kx, ky, pairs[i], p.fg, true) + 6
		kx += c.Text(f.small, kx, ky, pairs[i+1], p.muted) + 18
	}
}
