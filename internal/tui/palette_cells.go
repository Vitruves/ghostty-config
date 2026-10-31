package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// Palette in the text interface: the same column as in the picture, row for
// row, in the terminal's cells. The search line and the opened setting sit
// on a ground a little apart from the screen's; nothing is framed.

// palCells is where the column sits, in cells.
type palCells struct {
	x, w        int
	listY, list int // the first row of the list and how many it has
	strip       bool
}

func (p *palette) cellsLayout(m *Model, strip bool) palCells {
	l := palCells{w: m.width - 4, strip: strip}
	if m.width > 84 {
		l.w = clampInt(m.width*76/100, 80, 104)
	}
	l.x = (m.width - l.w) / 2
	l.listY = 5 // a row of air, the search line, air, the filters, air
	bottom := 3 // air, the status line, the keys
	if strip {
		bottom += 4 // the theme's three rows and air above them
	}
	l.list = maxInt(m.height-l.listY-bottom, 1)
	return l
}

func (p *palette) viewCells(m *Model) string {
	p.refresh(m)
	ui := m.panelChrome()
	r := p.current()
	strip := r != nil && r.kind == palTheme && m.height >= 18
	l := p.cellsLayout(m, strip)
	at := func(s string) string { return ui.base().Render(strings.Repeat(" ", l.x)) + s }

	rows := []string{"", at(p.cellField(m, ui, l)), "", at(p.cellChips(m, ui, l)), ""}
	list := p.cellList(m, ui, l)
	for i := 0; i < l.list; i++ {
		line := ""
		if i < len(list) {
			line = at(list[i])
		}
		rows = append(rows, line)
	}
	if strip {
		rows = append(rows, "")
		for _, s := range p.cellStrip(m, l, r.theme) {
			rows = append(rows, at(s))
		}
	}
	rows = append(rows, "", at(p.cellStatus(m, ui, l)), p.cellFooter(m, ui))
	return strings.Join(rows, "\n")
}

// palTone is the ground of the search line and of an opened setting.
func palTone(ui chrome) string {
	k := 0.92
	if ui.high {
		k = 0.86
	}
	return color.Blend(ui.fg, ui.bg, k)
}

// onAccentOf is the text that reads on the accent.
func onAccentOf(ui chrome) string {
	if color.Contrast(ui.fg, ui.accent) > color.Contrast(ui.bg, ui.accent) {
		return ui.fg
	}
	return ui.bg
}

// spread sets left and right on one line of exactly w cells on bg, cutting
// left when both do not fit.
func spread(bg string, w int, left, right string) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if lw+rw+1 > w {
		left = truncate(left, maxInt(w-rw-1, 0))
		lw = lipgloss.Width(left)
	}
	if lw+rw > w {
		right, rw = "", 0
	}
	return fitTo(left+on("", bg).Render(strings.Repeat(" ", maxInt(w-lw-rw, 0)))+right, w, bg)
}

func (p *palette) cellField(m *Model, ui chrome, l palCells) string {
	bg := palTone(ui)
	st := func(fg string) lipgloss.Style { return on(fg, bg) }
	left := st(ui.accent).Bold(true).Render(" ❯ ")
	if p.scope != nil {
		left += st(ui.fg).Bold(true).Render(palLabel(p.scope)) + st(ui.muted).Render(" › ")
	}
	if p.query == "" {
		ph := "Search themes, fonts, settings, shortcuts and commands"
		if p.scope != nil {
			ph = "filter its values"
		}
		left += st(ui.accent).Render("▏") + st(ui.muted).Render(ph)
	} else {
		left += st(ui.fg).Render(p.query) + st(ui.accent).Render("▏")
	}
	right := ""
	if count := p.countLine(); count != "" {
		right = st(ui.muted).Render(count + " ")
	}
	m.addRegion(l.x, 1, l.w, 1, hitPrompt, 0, "")
	return spread(bg, l.w, left, right)
}

func (p *palette) cellChips(m *Model, ui chrome, l palCells) string {
	if p.scope != nil {
		back := ui.mutedS().Render("‹ Back")
		m.addRegion(l.x, 3, 6, 1, hitRail, -1, "back")
		return truncate(back+ui.base().Render("    ")+ui.mutedS().Render(p.scope.desc), l.w)
	}
	var b strings.Builder
	x := l.x
	for i, name := range palFilters {
		chip := " " + name + " "
		if i == p.filter {
			b.WriteString(on(onAccentOf(ui), ui.accent).Bold(true).Render(chip))
		} else {
			b.WriteString(ui.mutedS().Render(chip))
		}
		w := len([]rune(chip))
		m.addRegion(x, 3, w, 1, hitRail, i, name)
		b.WriteString(ui.base().Render(" "))
		x += w + 1
	}
	return truncate(b.String(), l.w)
}

// cellList is the visible window of the list, each row registered as
// clickable where it is. Only the highlighted row can open to more than a
// line, so where every row sits is known without drawing it, and only the
// rows in the window are drawn.
func (p *palette) cellList(m *Model, ui chrome, l palCells) []string {
	type block struct{ start, n int }
	blocks := make([]block, len(p.rows))
	draw := func(i int) ([]string, []cellHit) {
		r := p.rows[i]
		switch r.kind {
		case palHead:
			head := ui.bold(ui.muted).Render(" " + strings.ToUpper(r.text))
			if r.count != "" {
				head += ui.faintS().Render("  " + r.count)
			}
			return []string{truncate(head, l.w)}, nil
		case palNote:
			return []string{ui.mutedS().Render(truncate(" "+r.text, l.w))}, nil
		}
		return p.cellRow(m, ui, r, i == p.sel, l.w)
	}
	var selOut []string
	var selHits []cellHit
	total := 0
	for i, r := range p.rows {
		if r.kind == palHead && i > 0 {
			total++ // a line of air over a title
		}
		n := 1
		if i == p.sel && r.kind != palHead && r.kind != palNote {
			selOut, selHits = draw(i)
			n = len(selOut)
		}
		blocks[i] = block{total, n}
		total += n
	}

	p.page = maxInt(l.list-1, 1)
	if p.sel >= 0 && p.sel < len(blocks) {
		b := blocks[p.sel]
		top := b.start
		if p.sel > 0 && p.rows[p.sel-1].kind == palHead {
			top = blocks[p.sel-1].start
		}
		if top < p.topCells {
			p.topCells = top
		}
		if b.start+b.n > p.topCells+l.list {
			p.topCells = b.start + b.n - l.list
		}
	}
	p.topCells = clampInt(p.topCells, 0, maxInt(total-l.list, 0))
	top, end := p.topCells, minInt(p.topCells+l.list, total)

	lines := make([]string, 0, maxInt(end-top, 0))
	controls := map[int][]cellHit{}
	for i, r := range p.rows {
		b := blocks[i]
		if r.kind == palHead && i > 0 && b.start-1 >= top && b.start-1 < end {
			lines = append(lines, "")
		}
		if b.start+b.n <= top || b.start >= end {
			continue
		}
		out, hits := selOut, selHits
		if i != p.sel || r.kind == palHead || r.kind == palNote {
			out, hits = draw(i)
		}
		controls[i] = hits
		for j, line := range out {
			if y := b.start + j; y >= top && y < end {
				lines = append(lines, line)
			}
		}
	}

	for i, r := range p.rows {
		if !r.selectable() {
			continue
		}
		b := blocks[i]
		y0, y1 := maxInt(b.start, p.topCells), minInt(b.start+b.n, p.topCells+l.list)
		if y1 <= y0 {
			continue
		}
		m.addRegion(l.x, l.listY+y0-p.topCells, l.w, y1-y0, hitResult, i, "")
		for _, h := range controls[i] {
			if y := b.start + h.line; y >= y0 && y < y1 {
				m.addRegion(l.x+h.x, l.listY+y-p.topCells, h.w, 1, hitValue, i, h.value)
			}
		}
	}
	return lines
}

// cellHit is a value of a control on a row: where it is, and the value.
type cellHit struct {
	line, x, w int
	value      string
}

// cellRow draws one result, and the controls on it that a click reaches.
func (p *palette) cellRow(m *Model, ui chrome, r palRow, sel bool, w int) ([]string, []cellHit) {
	bg := ui.paint()
	if sel {
		bg = ui.selBg
	}
	opened := sel && p.expands(m, r)
	if opened {
		bg = palTone(ui)
	}
	st := func(fg string) lipgloss.Style { return on(fg, bg) }
	nameS := st(ui.fg)
	if sel {
		nameS = nameS.Bold(true)
	}
	mark := st(ui.fg).Render("  ")
	labelW := clampInt(w*36/100, 18, 34)

	switch r.kind {
	case palTheme:
		t := r.theme
		facts := m.factsOf(t)
		sp := facts.spec
		strip := ""
		cell := " "
		if w >= 90 {
			cell = "  "
		}
		for _, h := range []string{sp.BG, sp.Pal[1], sp.Pal[2], sp.Pal[3], sp.Pal[4], sp.Pal[5], sp.Pal[6], sp.FG} {
			strip += on("", h).Render(cell)
		}
		left := mark + strip + st(ui.fg).Render("  ") + nameS.Render(pad(truncate(t.Name, labelW-2), labelW))
		if w >= 76 {
			left += st(ui.muted).Render(palThemeFamily(t))
		}
		right := st(ui.muted).Render(facts.palMeta)
		if t.Name == m.applied {
			right += st(ui.accent).Render("  applied")
		}
		if m.state.IsFavorite(t.Name) {
			right += st(ui.accent).Render("  ★")
		}
		return []string{spread(bg, w, left, right+st(ui.fg).Render(" "))}, nil

	case palFont:
		o := r.opt
		detail := o.detail
		if strings.HasPrefix(o.value, downloadPrefix) {
			detail = "not installed · Enter downloads it"
		}
		left := mark + nameS.Render(pad(truncate(o.label, labelW-2), labelW)) + st(ui.muted).Render(detail)
		right := ""
		if o.current {
			right = st(ui.accent).Render("in use ")
		}
		return []string{spread(bg, w, left, right)}, nil

	case palShortcut:
		o := r.opt
		if m.keysCustomised(o.key) {
			mark = st(ui.accent).Render(" ●")
		}
		left := mark + st(ui.fg).Render(" ") + nameS.Render(pad(truncate(o.label, labelW-2), labelW)) + st(ui.muted).Render("Shortcut · "+o.key.group)
		keys := m.shortcutsTextPlain(o.key.shortcuts)
		if keys == "" {
			keys = "unbound"
		}
		return []string{spread(bg, w, left, st(palQuiet(ui)).Render(keys+" "))}, nil

	case palCommand:
		c := r.cmd
		left := mark + st(ui.fg).Render(" ") + nameS.Render(pad(truncate(palLabel(c), labelW-2), labelW)) + st(ui.muted).Render(c.desc)
		right := ""
		if v := m.currentValueOf(c); v != "" {
			right = st(palQuiet(ui)).Render(truncate(v, w/4))
		}
		if c.options != nil {
			right += st(ui.muted).Render(" ›")
		}
		return []string{spread(bg, w, left, right+st(ui.fg).Render(" "))}, nil

	case palValue:
		o := r.opt
		if o.current {
			mark = st(ui.accent).Render(" ●")
		}
		label := o.label
		if p.scope.set != nil {
			label = displayValue(label)
		}
		left := mark + st(ui.fg).Render(" ")
		if color.IsHex(o.swatch) {
			left += st(o.swatch).Render("██") + st(ui.fg).Render(" ")
		}
		left += nameS.Render(label)
		detail := o.detail
		if detail == "current" {
			detail = ""
		}
		lines := []string{spread(bg, w, left, st(ui.muted).Render(truncate(detail, w/2)+" "))}
		if opened {
			lines = append(lines, p.cellBody(m, ui, bg, p.scope, o.value, w)...)
		}
		return lines, nil
	}

	// A setting.
	c := r.cmd
	if m.palChanged(c) {
		mark = st(ui.accent).Render(" ●")
	}
	s, v := m.palFace(c)
	left := mark + st(ui.fg).Render(" ") + nameS.Render(pad(truncate(palLabel(c), labelW-2), labelW))
	where := st(ui.muted).Render(palWhere(c))
	ctrl, hits := cellControl(ui, bg, s, v, opened)
	cw := lipgloss.Width(ctrl)
	if lipgloss.Width(left)+lipgloss.Width(where)+cw+3 <= w {
		left += where
	}
	if opened && lipgloss.Width(left)+cw+2 > w {
		// The pills do not fit beside the name: the value between arrows.
		ctrl, hits = cellControl(ui, bg, s, v, false)
		cw = lipgloss.Width(ctrl)
	}
	line := spread(bg, w, left, ctrl+st(ui.fg).Render(" "))
	for i := range hits {
		hits[i].x += w - 1 - cw
	}
	lines := []string{line}
	if opened {
		lines = append(lines, p.cellBody(m, ui, bg, c, v, w)...)
	}
	return lines, hits
}

// palQuiet is the tone of text that follows what leads.
func palQuiet(c chrome) string { return color.Blend(c.fg, c.bg, 0.22) }

// cellControl is a setting's value as its row shows it: a slider for a
// number, on or off for a switch, the value between arrows for a word, or,
// opened, every value as a row of pills.
func cellControl(ui chrome, bg string, s *setting, v string, opened bool) (string, []cellHit) {
	st := func(fg string) lipgloss.Style { return on(fg, bg) }
	switch {
	case s.list != nil:
		return st(ui.fg).Render(truncate(s.shown(v), 28)) + st(ui.muted).Render(" ›"), nil
	case s.numeric:
		val, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
		if err != nil {
			val = s.min
		}
		tone := ui.fg
		if s.unset != "" && strings.EqualFold(v, s.unset) {
			val, tone = s.unsetAt, ui.muted
		}
		const n = 12
		k := clampInt(int((val-s.min)/(s.max-s.min)*n+0.5), 0, n-1)
		track := st(ui.accent).Render(strings.Repeat("━", k)) + st(ui.fg).Bold(true).Render("●") + st(ui.faint).Render(strings.Repeat("─", n-1-k))
		return track + st(tone).Render(padLeft(v, 6)), nil
	}
	if opened && len(s.options) <= 6 {
		var b strings.Builder
		var hits []cellHit
		x := 0
		for i, o := range s.options {
			if i > 0 {
				b.WriteString(st(ui.fg).Render(" "))
				x++
			}
			pill := " " + displayValue(o) + " "
			if strings.EqualFold(o, v) {
				b.WriteString(on(onAccentOf(ui), ui.accent).Bold(true).Render(pill))
			} else {
				b.WriteString(st(ui.muted).Render(pill))
			}
			hits = append(hits, cellHit{x: x, w: len([]rune(pill)), value: o})
			x += len([]rune(pill))
		}
		return b.String(), hits
	}
	if len(s.options) == 2 && s.options[0] == "on" && s.options[1] == "off" {
		if v == "on" {
			return st(ui.accent).Render("◉ on"), []cellHit{{w: 4, value: "off"}}
		}
		return st(ui.muted).Render("○ off"), []cellHit{{w: 5, value: "on"}}
	}
	return st(ui.muted).Render("‹ ") + st(ui.fg).Render(displayValue(v)) + st(ui.muted).Render(" ›"), nil
}

// cellBody is what an opened setting says under its row: what the value
// does, and the line it puts in the config.
func (p *palette) cellBody(m *Model, ui chrome, bg string, c *command, value string, w int) []string {
	st := func(fg string) lipgloss.Style { return on(fg, bg) }
	lead, text := m.palExplain(c, value)
	in := st(ui.fg).Render("    ")
	var out []string
	wrapped := wrapPlain(strings.TrimSpace(lead+" "+text), w-6)
	if len(wrapped) > 3 {
		wrapped = append(wrapped[:2], truncate(strings.Join(wrapped[2:], " "), w-6))
	}
	for i, line := range wrapped {
		body := st(palQuiet(ui)).Render(line)
		if i == 0 && lead != "" && strings.HasPrefix(line, lead) {
			body = st(ui.fg).Bold(true).Render(lead) + st(palQuiet(ui)).Render(strings.TrimPrefix(line, lead))
		}
		out = append(out, fitTo(in+body, w, bg))
	}
	// The line names its key: the cells have no room to say it twice.
	key, lines, note := m.palWrites(c, value)
	for _, l := range lines {
		out = append(out, fitTo(in+st(ui.ok).Render("+ "+l), w, bg))
	}
	if note != "" {
		if key != "" {
			note = key + ": " + note
		}
		out = append(out, fitTo(in+st(ui.muted).Render(note), w, bg))
	}
	return out
}

// cellStrip is the highlighted theme in its own colours.
func (p *palette) cellStrip(m *Model, l palCells, t *ghostty.Theme) []string {
	facts := m.factsOf(t)
	sp := facts.spec
	st := func(fg string) lipgloss.Style { return on(fg, sp.BG) }
	text, weak, weakName := facts.ratio, facts.weak, facts.weakName
	meta := "text " + ratioText(text)
	if weakName != "" {
		meta += " · weakest " + weakName + " " + ratioText(weak)
	}
	path := st(sp.Pal[2]).Render("~/src/ghostty-config ") + st(sp.Pal[3]).Render("main ")
	return []string{
		spread(sp.BG, l.w, st(sp.FG).Render(" ")+path+st(sp.FG).Render("› git diff --stat"), st(sp.FG).Bold(true).Render(t.Name)+st(sp.FG).Render(" ")),
		spread(sp.BG, l.w, st(sp.FG).Render(" ")+st(sp.Pal[4]).Render("internal/tui/app.go ")+st(sp.FG).Render("| ")+st(sp.Pal[2]).Render("+12 ")+st(sp.Pal[1]).Render("-3"),
			st(color.Blend(sp.FG, sp.BG, 0.3)).Render(meta+" ")),
		spread(sp.BG, l.w, st(sp.FG).Render(" ")+path+st(sp.FG).Render("› ")+st(sp.Accent).Render("█"), ""),
	}
}

func (p *palette) cellStatus(m *Model, ui chrome, l palCells) string {
	text, kind, dot := p.statusText(m)
	tone, dotTone := ui.muted, ui.accent
	switch kind {
	case statusInfo:
		tone, dotTone = ui.accent2, ui.accent2
	case statusWarn:
		tone, dotTone = ui.warn, ui.warn
	case statusFail:
		tone, dotTone = ui.danger, ui.danger
	}
	lead := " "
	if dot {
		lead = ui.text(dotTone).Render(" ●") + ui.base().Render(" ")
	}
	return truncate(lead+ui.text(tone).Render(text), l.w)
}

// cellFooter is the name of the tool and the layout, and the keys.
func (p *palette) cellFooter(m *Model, ui chrome) string {
	brand := ui.mutedS().Render("ghostty-config · ") + ui.text(palQuiet(ui)).Render("Palette")
	room := m.width - 4 - lipgloss.Width(brand) - 3
	keys := ui.keyHintFitGlobal(room, "   ", p.keyPairs(m))
	gap := maxInt(m.width-4-lipgloss.Width(brand)-lipgloss.Width(keys), 1)
	return ui.base().Render("  ") + brand + ui.base().Render(strings.Repeat(" ", gap)) + keys
}
