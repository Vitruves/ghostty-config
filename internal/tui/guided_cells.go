package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The guide in the text interface: the same pages as the picture, in rows
// of cells. Weight and tone do what size does in the picture; the
// highlighted answer stands on a quiet ground, and the picture beside the
// page is kept to windows wide enough for it.

func (g *guided) viewCells(m *Model) string {
	if g.away {
		return atelier{}.viewCells(m)
	}
	ui := m.panelChrome()
	W, H := m.width, m.height
	mx := 2
	if W >= 100 {
		mx = 4
	}
	inner := maxInt(W-2*mx, 20)
	left := strings.Repeat(" ", mx)
	pg, pages := g.at(m)
	var rows []string
	add := func(s string) { rows = append(rows, left+s) }

	// The name and the steps, on one line when the window is wide, the
	// steps under the name otherwise.
	bars, labels, at := g.cellProgress(m, ui, inner, W >= 110)
	if W >= 110 {
		head := ui.bold(ui.fg).Render("ghostty-config") + ui.base().Render("   ")
		hw := lipgloss.Width(head)
		bars, labels, at = g.cellProgress(m, ui, inner-hw, true)
		add(head + bars)
		add(ui.base().Render(strings.Repeat(" ", hw)) + labels)
		for i, x := range at {
			m.addRegion(mx+hw+x[0], 0, x[1], 2, hitButton, i, "g-step")
		}
	} else {
		add(ui.bold(ui.fg).Render("ghostty-config"))
		add(bars)
		add(labels)
		for i, x := range at {
			m.addRegion(mx+x[0], 1, x[1], 2, hitButton, i, "g-step")
		}
	}

	// The way on is the last row, the status over it.
	barLine, barRegions := g.cellBottom(m, ui, pg, pages, inner)
	status := ""
	if m.status != "" && m.statusKind != statusPlain {
		st := ui.mutedS()
		if m.statusKind != statusInfo {
			st = ui.text(ui.warn)
		}
		status = st.Render(truncate(m.status, inner))
	}

	room := H - len(rows) - 2
	if status != "" {
		room--
	}
	var body []string
	g.cregs = g.cregs[:0]
	switch {
	case g.step == gDone:
		body = g.cellDone(m, ui, pg, inner, room)
	case pg.qs[0].kind == gCards:
		body = g.cellCards(m, ui, pg, inner, room)
	default:
		body = g.cellPage(m, ui, pg, inner, room)
	}
	if room >= len(body)+2 {
		add("")
	}
	for _, r := range g.cregs {
		m.addRegion(mx+r.x, len(rows)+r.y, r.w, r.h, hitButton, r.index, r.name)
	}
	for _, l := range body {
		add(l)
	}
	for len(rows) < H-1-boolInt(status != "") {
		add("")
	}
	if status != "" {
		add(ui.base().Render(strings.Repeat(" ", maxInt(inner-lipgloss.Width(status), 0))) + status)
	}
	add(barLine)
	for _, r := range barRegions {
		m.addRegion(mx+r.x, len(rows)-1, r.w, 1, hitButton, 0, r.name)
	}
	if len(rows) > H {
		rows = rows[:H]
	}
	for i := range rows {
		rows[i] = rebase(ui.fill(rows[i], W), ui.base())
	}
	return strings.Join(rows, "\n")
}

// creg registers a clickable span of the body, relative to where the body
// starts: whether a row of air goes over it is only known once it is laid out.
func (g *guided) creg(x, y, w, h, index int, name string) {
	g.cregs = append(g.cregs, region{x: x, y: y, w: w, h: h, kind: hitButton, index: index, name: name})
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// cellProgress is the six steps as a row of bars and a row of names, with
// where each step sits.
func (g *guided) cellProgress(m *Model, ui chrome, width int, _ bool) (string, string, [][2]int) {
	gap := 2
	widths := make([]int, gSteps)
	sum := 0
	for i, n := range gStepNames {
		widths[i] = lipgloss.Width(n)
		sum += widths[i]
	}
	even := (width - gap*(gSteps-1)) / gSteps
	most := 0
	for _, w := range widths {
		most = maxInt(most, w)
	}
	if even >= most {
		for i := range widths {
			widths[i] = even
		}
	} else {
		spare := maxInt(width-gap*(gSteps-1)-sum, 0)
		for i := range widths {
			widths[i] += spare / gSteps
		}
	}
	var bars, labels strings.Builder
	var at [][2]int
	x := 0
	for i, n := range gStepNames {
		if i > 0 {
			bars.WriteString(ui.base().Render(strings.Repeat(" ", gap)))
			labels.WriteString(ui.base().Render(strings.Repeat(" ", gap)))
			x += gap
		}
		w := widths[i]
		bar, text := ui.faint, ui.text(ui.muted)
		switch {
		case i == g.step:
			bar, text = ui.accent, ui.bold(ui.fg)
		case i <= g.reached:
			bar, text = ui.accent, ui.base()
		}
		bars.WriteString(ui.text(bar).Render(strings.Repeat("▀", w)))
		labels.WriteString(ui.fill(text.Render(truncate(n, w)), w))
		at = append(at, [2]int{x, w})
		x += w
	}
	return bars.String(), labels.String(), at
}

// cellRegion is a clickable span of a row.
type cellRegion struct {
	x, w int
	name string
}

// cellBottom is the way on: the quiet buttons on the left, the accent one on
// the right, what does not fit left out.
func (g *guided) cellBottom(m *Model, ui chrome, pg gPage, pages []gPage, width int) (string, []cellRegion) {
	left, right := g.bottomItems(m, pg, pages)
	render := func(b gButton) string {
		switch b.kind {
		case 1:
			return on(ui.fg, color.Blend(ui.fg, ui.bg, 0.88)).Render(" " + b.label + " ")
		case 2:
			return on(ui.bg, ui.accent).Bold(true).Render(" " + b.label + " ")
		case 3:
			return ui.faintS().Render(b.label)
		}
		return ui.mutedS().Render(b.label)
	}
	left, right = fitBar(left, right, float32(width), func(b gButton) float32 { return float32(lipgloss.Width(render(b))) }, 2)
	var regions []cellRegion
	var line strings.Builder
	x := 0
	put := func(b gButton) {
		s := render(b)
		if b.name != "" {
			regions = append(regions, cellRegion{x, lipgloss.Width(s), b.name})
		}
		line.WriteString(s)
		x += lipgloss.Width(s)
	}
	sp := func(n int) {
		line.WriteString(ui.base().Render(strings.Repeat(" ", maxInt(n, 0))))
		x += maxInt(n, 0)
	}
	for i, b := range left {
		if i > 0 {
			sp(2)
		}
		put(b)
	}
	rw := 0
	for i, b := range right {
		if i > 0 {
			rw += 2
		}
		rw += lipgloss.Width(render(b))
	}
	sp(width - x - rw)
	for i, b := range right {
		if i > 0 {
			sp(2)
		}
		put(b)
	}
	return line.String(), regions
}

// cellHead is the line over the question and the question.
func (g *guided) cellHead(ui chrome, pg gPage, q gQuestion, width int) []string {
	out := []string{ui.mutedS().Render(truncate(g.eyebrow(pg), width))}
	for _, l := range wrapPlain(q.title, width) {
		out = append(out, ui.bold(ui.fg).Render(l))
	}
	return append(out, "")
}

// cellPage is a page of rows and pills, the picture beside it when there is
// room for one.
func (g *guided) cellPage(m *Model, ui chrome, pg gPage, width, room int) []string {
	colW := width
	prevW := 0
	if width >= 96 {
		colW = minInt(56, width*11/20)
		prevW = width - colW - 4
	}
	out := g.cellHead(ui, pg, pg.qs[0], colW)
	write := g.willWrite(m, pg)
	full := g.cellQuestions(m, ui, pg, colW, false, len(out), true)
	compact := len(out)+len(full)+2 > room
	out = append(out, g.cellQuestions(m, ui, pg, colW, compact, len(out), false)...)
	if prevW > 0 {
		pic := g.cellPreview(m, ui, pg, prevW, minInt(room-2, 16))
		if write != "" {
			pic = append(pic, "")
			pic = append(pic, para(ui.mutedS(), write, prevW)...)
		}
		out = sideBySide(ui, out, colW, pic, 4)
	} else if lines := para(ui.mutedS(), write, colW); write != "" && len(out)+len(lines) < room {
		out = append(out, "")
		out = append(out, lines...)
	}
	return out
}

// sideBySide sets two columns of lines next to each other.
func sideBySide(ui chrome, a []string, aw int, b []string, gap int) []string {
	n := maxInt(len(a), len(b))
	out := make([]string, n)
	for i := 0; i < n; i++ {
		l, r := "", ""
		if i < len(a) {
			l = a[i]
		}
		if i < len(b) {
			r = b[i]
		}
		out[i] = ui.fill(l, aw) + ui.base().Render(strings.Repeat(" ", gap)) + r
	}
	return out
}

// cellQuestions lays the questions of a page under one another; measure
// registers nothing.
func (g *guided) cellQuestions(m *Model, ui chrome, pg gPage, width int, compact bool, top int, measure bool) []string {
	var out []string
	for qi, q := range pg.qs {
		focused := qi == g.focus && len(pg.qs) > 1
		if qi > 0 {
			if !compact {
				out = append(out, "")
			}
			st := ui.bold(ui.fg)
			if !focused {
				st = ui.base()
			}
			out = append(out, st.Render(truncate(q.title, width)))
		}
		sel := g.sel(m, q)
		name := fmt.Sprintf("g-q-%d", qi)
		if q.kind == gPills {
			var line strings.Builder
			x := 0
			for i, ch := range q.choices {
				if i > 0 {
					line.WriteString(ui.base().Render(" "))
					x++
				}
				s := on(ui.muted, color.Blend(ui.fg, ui.bg, 0.9)).Render(" " + ch.label + " ")
				if i == sel {
					s = on(ui.bg, ui.accent).Bold(true).Render(" " + ch.label + " ")
				}
				if !measure {
					g.creg(x, top+len(out), lipgloss.Width(s), 1, i, name)
				}
				line.WriteString(s)
				x += lipgloss.Width(s)
			}
			out = append(out, line.String())
			if sel >= 0 && q.choices[sel].desc != "" && !compact {
				out = append(out, ui.mutedS().Render(truncate(q.choices[sel].desc, width)))
			}
			continue
		}
		out = append(out, g.cellRows(m, ui, q, qi, sel, width, compact, top+len(out), measure, focused || len(pg.qs) == 1)...)
	}
	return out
}

// cellRows are answers as rows: a radio and a name, and under it what the
// answer does, on a quiet ground for the one highlighted.
func (g *guided) cellRows(m *Model, ui chrome, q gQuestion, qi, sel, width int, compact bool, top int, measure, focused bool) []string {
	var out []string
	ground := color.Blend(ui.fg, ui.bg, 0.9)
	name := fmt.Sprintf("g-q-%d", qi)
	for i, ch := range q.choices {
		start := len(out)
		mark, mst := "○", ui.text(color.Blend(ui.fg, ui.bg, 0.6))
		if i == sel {
			mark = "●"
			mst = on(ui.accent, ground)
			if !focused {
				mst = on(ui.muted, ground)
			}
		}
		descs := wrapPlain(ch.desc, width-4)
		if ch.desc == "" || (compact && i != sel) {
			descs = nil
		}
		if i == sel {
			row := mst.Render(" "+mark+" ") + on(ui.fg, ground).Bold(true).Render(truncate(ch.label, width-4))
			out = append(out, ui.fillWith(row, width, ground))
			for _, d := range descs {
				out = append(out, ui.fillWith(on(ui.fg, ground).Render("   ")+on(ui.muted, ground).Render(d), width, ground))
			}
		} else {
			out = append(out, mst.Render(" "+mark+" ")+ui.bold(ui.fg).Render(truncate(ch.label, width-4)))
			for _, d := range descs {
				out = append(out, ui.base().Render("   ")+ui.mutedS().Render(d))
			}
		}
		if !measure {
			g.creg(0, top+start, width, len(out)-start, i, name)
		}
		if !compact && len(descs) > 0 && i < len(q.choices)-1 && i != sel && i+1 != sel {
			out = append(out, "")
		} else if !compact && len(descs) > 0 && i < len(q.choices)-1 {
			out = append(out, "")
		}
	}
	return out
}

// --- the themes ---------------------------------------------------------------------

// cellCards are the three themes side by side, each a terminal in its own
// colours.
func (g *guided) cellCards(m *Model, ui chrome, pg gPage, width, room int) []string {
	q := pg.qs[0]
	out := g.cellHead(ui, pg, q, width)
	if len(q.choices) == 0 {
		return append(out, ui.mutedS().Render("No theme of this kind is installed: go back and choose another, or see them all."))
	}
	gap := 3
	cw := (width - 2*gap) / 3
	sh := clampInt(room-len(out)-3, 3, len(gSample)+2)
	sel := g.sel(m, q)
	cols := make([][]string, len(q.choices))
	for i, ch := range q.choices {
		col := g.cellTerminal(m.themeSpecOf(ch.theme), cw, sh, "block")
		under := ui.base().Render(strings.Repeat(" ", cw))
		if i == sel {
			under = ui.text(ui.accent).Render(strings.Repeat("▀", cw))
		}
		col = append(col, under)
		nm := ui.bold(ui.fg).Render(truncate(ch.label, cw))
		if i == sel {
			nm = ui.bold(ui.accent).Render(truncate(ch.label, cw))
		}
		col = append(col, nm, ui.mutedS().Render(truncate(ch.desc, cw)))
		cols[i] = col
		g.creg(i*(cw+gap), len(out), cw, len(col), i, "g-q-0")
	}
	for r := 0; r < len(cols[0]); r++ {
		var line strings.Builder
		for i := range cols {
			if i > 0 {
				line.WriteString(ui.base().Render(strings.Repeat(" ", gap)))
			}
			cell := ""
			if r < len(cols[i]) {
				cell = cols[i][r]
			}
			line.WriteString(ui.fill(cell, cw))
		}
		out = append(out, line.String())
	}
	return out
}

// themeSpecOf is the colours of a sample terminal: a theme's.
func (m *Model) themeSpecOf(t *ghostty.Theme) gTerm {
	sp := m.themeSpec(t)
	return gTerm{bg: sp.BG, fg: sp.FG, cursor: sp.Accent, pal: sp.Pal}
}

// gTerm is the colours a sample terminal is written in.
type gTerm struct {
	bg, fg, cursor string
	pal            [16]string
}

// cellsKept is what draw returns, kept by key while the guide is drawn the
// same way: the pictures beside a page come out the same on every frame
// until an answer changes them, and the cells of each are many. The lines
// handed back are the caller's to change.
func (g *guided) cellsKept(key string, draw func() []string) []string {
	lines, ok := g.kept[key]
	if !ok {
		if g.kept == nil || len(g.kept) >= 32 {
			g.kept = map[string][]string{}
		}
		lines = draw()
		g.kept[key] = lines
	}
	return append([]string(nil), lines...)
}

// cellTerminal is cellTerminal, kept.
func (g *guided) cellTerminal(t gTerm, w, h int, shape string) []string {
	return g.cellsKept(fmt.Sprintf("terminal|%v|%d|%d|%s", t, w, h, shape), func() []string { return cellTerminal(t, w, h, shape) })
}

// cellTerminal writes the sample in a theme's colours, w by h cells, with a
// margin of one cell round it.
func cellTerminal(t gTerm, w, h int, shape string) []string {
	ground := on(t.fg, t.bg)
	blank := ground.Render(strings.Repeat(" ", maxInt(w, 0)))
	out := []string{blank}
	n := minInt(len(gSample), h-2)
	for i := 0; i < n; i++ {
		line := gSample[i]
		if i == n-1 {
			line = gSample[len(gSample)-1]
		}
		var b strings.Builder
		b.WriteString(ground.Render(" "))
		used := 1
		for _, run := range line {
			col := t.fg
			if run.pal >= 0 {
				col = t.pal[run.pal]
			}
			s := truncate(run.text, w-used-1)
			b.WriteString(on(col, t.bg).Render(s))
			used += lipgloss.Width(s)
		}
		if i == n-1 && used+2 < w {
			b.WriteString(ground.Render(" "))
			used++
			switch shape {
			case "bar":
				b.WriteString(on(t.cursor, t.bg).Render("▏"))
			case "underline":
				b.WriteString(on(t.cursor, t.bg).Render("▁"))
			default:
				b.WriteString(on(t.bg, t.cursor).Render(" "))
			}
			used++
		}
		b.WriteString(ground.Render(strings.Repeat(" ", maxInt(w-used, 0))))
		out = append(out, b.String())
	}
	for len(out) < h {
		out = append(out, blank)
	}
	return out
}

// --- the pictures beside a page ------------------------------------------------------------

// cellPreview is what the highlighted answers would do, as far as cells can
// show it: the theme as a terminal, the cursor in its shape, the window
// over a desktop as translucent as asked.
func (g *guided) cellPreview(m *Model, ui chrome, pg gPage, w, h int) []string {
	if m.cur == nil || h < 5 {
		return nil
	}
	t := m.themeSpecOf(m.cur)
	shape := m.settingNow("cursor")
	switch pg.qs[0].preview {
	case "themes":
		all := g.colourPages(m)
		dark := all[0].qs[0].choices[g.sel(m, all[0].qs[0])].value == "dark"
		kind := all[1].qs[0].choices[g.sel(m, all[1].qs[0])].value
		list := g.themeTemps(m, dark)[kind]
		var out []string
		for i := 0; i < 2 && i < len(list); i++ {
			th := list[i*len(list)/2]
			out = append(out, g.cellTerminal(m.themeSpecOf(th), w, minInt(h/2, len(gSample)+2), "block")...)
			out = append(out, ui.mutedS().Render(truncate(th.Name, w)))
		}
		return out
	case "window":
		return g.cellWindow(m, ui, pg, t, w, h)
	case "cursor":
		for _, q := range pg.qs {
			if q.id == "cursor" {
				if i := g.sel(m, q); i >= 0 {
					shape = q.choices[i].writes[0].opt.value
				}
			}
		}
	}
	return g.cellTerminal(t, w, minInt(h, len(gSample)+2), shape)
}

// cellWindow is the window over a desktop: where it is translucent, the
// desktop's colour mixes into the ground; where it is frosted, the
// desktop's colours blur into one.
func (g *guided) cellWindow(m *Model, ui chrome, pg gPage, t gTerm, w, h int) []string {
	opacity := 1.0
	fmt.Sscan(m.settingNow("opacity"), &opacity)
	blur := m.settingNow("blur") != "off"
	pad := atoiDefault(m.settingNow("paddingx"), 2)
	for _, q := range pg.qs {
		if i := g.sel(m, q); i >= 0 {
			for _, wr := range q.choices[i].writes {
				switch wr.cmd {
				case "opacity":
					fmt.Sscan(wr.opt.value, &opacity)
				case "blur":
					blur = wr.opt.value != "off"
				case "paddingx":
					pad = atoiDefault(wr.opt.value, 2)
				}
			}
		}
	}
	light := !color.IsDark(ui.bg)
	key := fmt.Sprintf("window|%v|%v|%d|%v|%v|%d|%d", opacity, blur, pad, t, light, w, h)
	return g.cellsKept(key, func() []string { return cellWindowLines(t, w, h, opacity, blur, pad, light) })
}

// cellWindowLines draws cellWindow's picture.
func cellWindowLines(t gTerm, w, h int, opacity float64, blur bool, pad int, light bool) []string {
	stops := []string{"#2b3a55", "#5a3d52", "#8a5a3c", "#e8a35c"}
	if light {
		stops = []string{"#9db8e0", "#d6a9c0", "#f2c59a", "#fff1d6"}
	}
	desk := func(x, y int) string {
		tt := (float64(x)/float64(w) + float64(y)/float64(h)) / 2
		c := color.Blend(stops[0], stops[1], tt*1.8)
		if tt > 0.55 {
			c = color.Blend(stops[1], stops[2], (tt-0.55)/0.45)
		}
		if dx, dy := float64(x)-float64(w)*0.72, (float64(y)-float64(h)*0.42)*2; dx*dx+dy*dy < float64(w*w)/60 {
			c = stops[3]
		}
		return c
	}
	frost := color.Blend(stops[1], stops[3], 0.25)
	inside := func(x, y int) bool { return x >= 3 && x < w-3 && y >= 1 && y < h-1 }
	px := clampInt(pad/6, 0, 3) + 1
	var out []string
	for y := 0; y < h; y++ {
		var b strings.Builder
		for x := 0; x < w; x++ {
			if !inside(x, y) {
				b.WriteString(on(desk(x, y), desk(x, y)).Render(" "))
				continue
			}
			behind := desk(x, y)
			if blur {
				behind = frost
			}
			ground := color.Blend(behind, t.bg, opacity)
			ch, fg := " ", t.fg
			row, col := y-1-1, x-3-px
			if y == 1 {
				// The bar, with its three buttons.
				if x == 5 || x == 7 || x == 9 {
					ch, fg = "●", color.Blend(t.bg, t.fg, 0.3)
				}
			} else if row >= 0 && row < len(gSample) && col >= 0 {
				ch, fg = sampleAt(t, row, col)
				if row == len(gSample)-1 && col == sampleLen(len(gSample)-1)+1 {
					ground = t.cursor
				}
			}
			b.WriteString(on(fg, ground).Render(ch))
		}
		out = append(out, b.String())
	}
	return out
}

// sampleLen is how many characters a row of the sample has.
func sampleLen(row int) int {
	n := 0
	for _, run := range gSample[row] {
		n += len([]rune(run.text))
	}
	return n
}

// sampleAt is the character of the sample at a row and column, and its
// colour.
func sampleAt(t gTerm, row, col int) (string, string) {
	x := 0
	for _, run := range gSample[row] {
		r := []rune(run.text)
		if col < x+len(r) {
			c := t.fg
			if run.pal >= 0 {
				c = t.pal[run.pal]
			}
			return string(r[col-x]), c
		}
		x += len(r)
	}
	return " ", t.fg
}

// --- Done ----------------------------------------------------------------------------

func (g *guided) cellDone(m *Model, ui chrome, pg gPage, width, room int) []string {
	q := pg.qs[0]
	colW, sideW := width, 0
	if width >= 96 {
		colW = minInt(56, width*11/20)
		sideW = width - colW - 4
	}
	out := g.cellHead(ui, pg, q, colW)
	changes := m.sessionChanges()
	summary := func(w, most int) []string {
		lines := []string{ui.mutedS().Render("Changed this session")}
		if len(changes) == 0 {
			lines = append(lines, para(ui.base(), "Nothing yet: every step left your settings as they were.", w)...)
		}
		for i, l := range changes {
			if i == most-1 && len(changes) > most {
				lines = append(lines, ui.mutedS().Render(fmt.Sprintf("and %d more", len(changes)-i)))
				break
			}
			lines = append(lines, ui.accentS().Render("+ ")+ui.base().Render(truncate(l, w-2)))
		}
		if note := m.doneNote(); note != "" {
			lines = append(lines, "")
			lines = append(lines, para(ui.mutedS(), note, w)...)
		}
		return lines
	}
	if sideW > 0 {
		rows := g.cellRows(m, ui, q, 0, g.sel(m, q), colW, len(out)+len(q.choices)*3 > room, len(out), false, true)
		return sideBySide(ui, append(out, rows...), colW, summary(sideW, room-4), 4)
	}
	out = append(out, summary(width, clampInt(room-len(out)-len(q.choices)*2-4, 1, 8))...)
	out = append(out, "")
	return append(out, g.cellRows(m, ui, q, 0, g.sel(m, q), width, len(out)+len(q.choices)*3 > room, len(out), false, true)...)
}
