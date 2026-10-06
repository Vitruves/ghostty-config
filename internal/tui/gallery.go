package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/vitruves/ghostty-config/internal/collection"
	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The wall of themes. Every theme is a card: a picture where the terminal
// draws pictures, cells in the theme's own colours where it does not. The
// cards are grouped by where a theme comes from, your own first, then each
// family of the collection, then the ones Ghostty ships, and every group runs
// from dark to light, so a theme is found by eye before it is found by name.

// themeOptions lists the library the way the wall shows it. A typed needle
// that is not a family gives the plain list back, for the prompt to filter
// and rank; a family name gives that family alone.
func (m *Model) themeOptions(arg string) []option {
	needle := strings.TrimSpace(arg)
	only := familyName(needle)
	if needle != "" && only == "" {
		out := make([]option, 0, len(m.lib.Themes))
		for _, t := range m.lib.Themes {
			out = append(out, m.themeOption(t))
		}
		return out
	}

	groupOf := func(t *ghostty.Theme) string {
		switch fam := themeFamily(t); fam {
		case "installed", "collection", "draft":
			// A file in the themes directory that belongs to no family.
			return "yours"
		default:
			return fam
		}
	}
	by := map[string][]*ghostty.Theme{}
	lum := map[string]float64{}
	for _, t := range m.lib.Themes {
		g := groupOf(t)
		by[g] = append(by[g], t)
		if rgb, err := color.ParseHex(t.Background()); err == nil {
			lum[t.Name] = rgb.Luminance()
		}
	}
	order := append([]string{"yours"}, collection.Families()...)
	order = append(order, "bundled")
	known := map[string]bool{}
	for _, g := range order {
		known[g] = true
	}
	var extra []string
	for g := range by {
		if !known[g] {
			extra = append(extra, g)
		}
	}
	sort.Strings(extra)
	order = append(order, extra...)

	var out []option
	for _, g := range order {
		themes := by[g]
		if len(themes) == 0 || (only != "" && !strings.EqualFold(g, only)) {
			continue
		}
		sort.SliceStable(themes, func(a, b int) bool {
			if lum[themes[a].Name] != lum[themes[b].Name] {
				return lum[themes[a].Name] < lum[themes[b].Name]
			}
			return themes[a].Name < themes[b].Name
		})
		label := g
		switch g {
		case "yours":
			label = "Yours"
		case "bundled":
			label = "Bundled"
		}
		word := "themes"
		if len(themes) == 1 {
			word = "theme"
		}
		out = append(out, option{label: label, detail: fmt.Sprintf("%d %s, dark to light", len(themes), word), header: true})
		for _, t := range themes {
			out = append(out, m.themeOption(t))
		}
	}
	return out
}

// What a line of the results is on the wall.
const (
	galHead = iota // the title of a group
	galRow         // a result that is not a theme, drawn as a line
	galTile        // a theme, drawn as a card
)

// galItem is where one result sits on the wall, counted in rows from the top
// of the whole wall, of which the screen shows a window.
type galItem struct {
	kind int
	col  int
	y    int
	lead int // rows above it that belong with it: the title over a first row
}

// galleryLayout places every result: titles and lines take a row and a row
// of air, cards fill rows of cols.
func (m *Model) galleryLayout(cols int) ([]galItem, int) {
	items := make([]galItem, len(m.results))
	y, col, lead := 0, 0, 0
	for i, o := range m.results {
		if (o.header || o.theme == nil) && col > 0 {
			y += tileRowH
			col = 0
		}
		switch {
		case o.header:
			items[i] = galItem{kind: galHead, y: y}
			y += 2
			lead = 2
		case o.theme == nil:
			items[i] = galItem{kind: galRow, y: y, lead: lead}
			y += 2
			lead = 0
		default:
			items[i] = galItem{kind: galTile, col: col, y: y, lead: lead}
			if col++; col == cols {
				col, lead = 0, 0
				y += tileRowH
			}
		}
	}
	if col > 0 {
		y += tileRowH
	}
	return items, y
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// gridMove steps the highlight over the wall: sideways to the neighbour, up
// and down to the card nearest in the next row, across group titles.
func (m *Model) gridMove(dx, dy int) tea.Cmd {
	n := len(m.results)
	if n == 0 {
		return nil
	}
	cur := clampInt(m.sel.cursor, 0, n-1)
	next := cur
	if dx != 0 {
		for i := cur + dx; i >= 0 && i < n; i += dx {
			if !m.results[i].header {
				next = i
				break
			}
		}
	}
	if dy != 0 {
		items, _ := m.galleryLayout(m.layout().cols)
		step := 1
		if dy < 0 {
			step = -1
		}
		for ; dy != 0; dy -= step {
			from := items[next]
			rowY, best := -1, -1
			for i := next + step; i >= 0 && i < n; i += step {
				if m.results[i].header || items[i].y == from.y {
					continue
				}
				if rowY < 0 {
					rowY = items[i].y
				}
				if items[i].y != rowY {
					break
				}
				if best < 0 || absInt(items[i].col-from.col) < absInt(items[best].col-from.col) {
					best = i
				}
			}
			if best < 0 {
				break
			}
			next = best
		}
	}
	if next == cur {
		return nil
	}
	m.sel.cursor = next
	return m.afterMove()
}

// galleryPage is how many rows of cards the screen shows.
func (m *Model) galleryPage() int {
	return maxInt(m.layout().bodyH/tileRowH, 1)
}

// gridJumpGroup moves to the first theme of the next group, or back to the
// start of this one and then of the one before.
func (m *Model) gridJumpGroup(dir int) tea.Cmd {
	n := len(m.results)
	first := func(head int) int {
		if head+1 < n && !m.results[head+1].header {
			return head + 1
		}
		return -1
	}
	cur := m.sel.cursor
	target := -1
	if dir > 0 {
		for i := cur + 1; i < n; i++ {
			if m.results[i].header {
				target = first(i)
				break
			}
		}
	} else {
		for i := cur - 1; i >= 0; i-- {
			if m.results[i].header {
				if f := first(i); f >= 0 && f != cur {
					target = f
					break
				}
			}
		}
	}
	if target < 0 || target == cur {
		return nil
	}
	m.sel.cursor = target
	m.galAlign = true
	return m.afterMove()
}

// viewGallery draws the window of the wall that holds the highlight. Only
// the cards on screen are drawn, so only their pictures are ever sent.
func (m *Model) viewGallery(ui chrome, l layout) []string {
	if len(m.results) == 0 {
		return []string{ui.mutedS().Render(truncate("nothing matches — Esc clears", l.mainW))}
	}
	items, total := m.galleryLayout(l.cols)
	viewH := l.bodyH

	// Keep the highlight on screen, with the title of its group when it is in
	// the first row under it.
	if cur := m.sel.cursor; cur >= 0 && cur < len(items) {
		it := items[cur]
		span := tileH
		if it.kind != galTile {
			span = 1
		}
		switch {
		case m.galAlign:
			m.galTop = it.y - it.lead
		case m.centre:
			m.galTop = it.y - (viewH-span)/2
		}
		m.galAlign, m.centre = false, false
		if it.y-it.lead < m.galTop {
			m.galTop = it.y - it.lead
		}
		if it.y+span > m.galTop+viewH {
			m.galTop = it.y + span - viewH
		}
	}
	if m.galTop > total-viewH {
		m.galTop = total - viewH
	}
	if m.galTop < 0 {
		m.galTop = 0
	}
	top := m.galTop

	whole := make([]string, viewH)   // rows a title or a line takes whole
	cells := make([][]string, viewH) // rows made of cards, one string per column
	for i, it := range items {
		o := m.results[i]
		span := 1
		if it.kind == galTile {
			span = tileH
		}
		if it.y+span <= top || it.y >= top+viewH {
			continue
		}
		selected := i == m.sel.cursor
		switch it.kind {
		case galHead:
			whole[it.y-top] = ui.bold(ui.fg).Render(o.label) + ui.mutedS().Render("  "+o.detail)
		case galRow:
			whole[it.y-top] = m.listRow(ui, o, selected, minInt(l.mainW, 72))
			m.addRegion(l.mainX, l.bodyY+it.y-top, minInt(l.mainW, 72), 1, hitResult, i, "")
		case galTile:
			rows := m.tileRows(ui, o, selected)
			first, last := -1, -1
			for r, row := range rows {
				line := it.y + r - top
				if line < 0 || line >= viewH {
					continue
				}
				if cells[line] == nil {
					cells[line] = make([]string, l.cols)
				}
				cells[line][it.col] = row
				if first < 0 {
					first = line
				}
				last = line
			}
			if first >= 0 {
				m.addRegion(l.mainX+it.col*(tileW+tileGap), l.bodyY+first, tileW, last-first+1, hitResult, i, "")
			}
		}
	}

	gap := ui.base().Render(strings.Repeat(" ", tileGap))
	blank := ui.base().Render(strings.Repeat(" ", tileW))
	out := make([]string, viewH)
	for i := range out {
		switch {
		case whole[i] != "":
			out[i] = whole[i]
		case cells[i] != nil:
			var b strings.Builder
			for c, cell := range cells[i] {
				if c > 0 {
					b.WriteString(gap)
				}
				if cell == "" {
					cell = blank
				}
				b.WriteString(cell)
			}
			out[i] = b.String()
		}
	}
	return out
}

// tileRows is one card: its picture, its name, and a line that says whether
// it is dark or light and how well its text reads.
func (m *Model) tileRows(ui chrome, o option, selected bool) []string {
	t := o.theme
	var rows []string
	if m.imagesOn() {
		rows = m.themeTile(t, tileW, tilePicH, selected, ui)
	} else {
		rows = tileCells(t)
	}
	name := pad(truncate(t.Name, tileW), tileW)
	if selected {
		rows = append(rows, on(ui.selFg, ui.selBg).Bold(true).Render(name))
	} else {
		rows = append(rows, ui.base().Render(name))
	}
	kind, kindS := "dark", ui.mutedS()
	if !t.IsDark() {
		kind = "light"
	}
	if t.Name == m.applied {
		kind, kindS = "applied", ui.accentS()
	}
	ratio := color.Contrast(t.Foreground(), t.Background())
	meta := fmt.Sprintf(" · %s %.1f", color.Grade(ratio), ratio)
	if m.state.IsFavorite(t.Name) {
		meta += " ♥"
	}
	return append(rows, ui.fill(kindS.Render(kind)+ui.mutedS().Render(meta), tileW))
}

// tileCells draws a card from cells, for a terminal that shows no pictures:
// the same specimen, the same two lines and the same sixteen colours, on the
// theme's own ground.
func tileCells(t *ghostty.Theme) []string {
	bg, fg := t.Background(), t.Foreground()
	pal := func(i int) string {
		if v := t.Get(paletteKey(i)); color.IsHex(v) {
			return color.Normalize(v, fg)
		}
		return fg
	}
	text := func(hex, s string) string { return on(hex, bg).Render(s) }
	blank := text(fg, strings.Repeat(" ", tileW))
	cursor := color.Normalize(t.Get("cursor-color"), fg)

	specimen := on(fg, bg).Bold(true).Render(" Aa ") + text(cursor, "█") + text(fg, strings.Repeat(" ", tileW-5))
	first := text(pal(2), " › ") + text(fg, "git diff ") + text(color.Blend(fg, bg, 0.5), "--stat")
	second := text(pal(4), " app.go ") + text(pal(2), "+12 ") + text(pal(1), "-3") + text(fg, strings.Repeat(" ", tileW-14))
	var strip strings.Builder
	strip.WriteString(text(fg, " "))
	for i := 0; i < 16; i++ {
		strip.WriteString(text(pal(i), "▄"))
	}
	strip.WriteString(text(fg, " "))
	return []string{specimen, blank, first, second, blank, strip.String()}
}

// viewFamilyRail is the column beside the wall: everything, the favourites,
// and the groups on the wall with how many themes each holds. The group the
// highlight is in is marked, and a click on one goes to it.
func (m *Model) viewFamilyRail(ui chrome, l layout) []string {
	w := l.railW
	line := func(label, count string, st lipgloss.Style) string {
		label = truncate(label, w-2)
		gap := w - 2 - lipgloss.Width(label) - lipgloss.Width(count)
		if gap < 1 {
			count, gap = "", maxInt(w-2-lipgloss.Width(label), 0)
		}
		return st.Render(" " + label + strings.Repeat(" ", gap) + count + " ")
	}
	var rows []string
	scope := func(name, label, count string) {
		st := ui.mutedS()
		if m.cmd != nil && m.cmd.name == name {
			st = ui.bold(ui.fg)
		}
		m.addRegion(marginX, l.bodyY+len(rows), w, 1, hitCommand, len(rows), name)
		rows = append(rows, line(label, count, st))
	}
	scope("theme", "All", itoa(len(m.lib.Themes)))
	scope("favs", "Favourites", itoa(m.state.FavoriteCount()))
	rows = append(rows, "")

	type family struct {
		index, count int
		label        string
	}
	var fams []family
	cur := -1
	for i, o := range m.results {
		if o.header {
			fams = append(fams, family{index: i, label: o.label})
			continue
		}
		if len(fams) == 0 {
			continue
		}
		fams[len(fams)-1].count++
		if i == m.sel.cursor {
			cur = len(fams) - 1
		}
	}
	room := l.bodyH - len(rows)
	if room < 1 || len(fams) < 2 {
		return rows
	}
	s := scroller{cursor: maxInt(cur, 0)}
	if len(fams) > room {
		s.offset = s.cursor - room/2
	}
	s.clamp(len(fams), room)
	for i := s.offset; i < len(fams) && i < s.offset+room; i++ {
		st := ui.mutedS()
		if i == cur {
			st = on(ui.selFg, ui.selBg).Bold(true)
		}
		m.addRegion(marginX, l.bodyY+len(rows), w, 1, hitRail, fams[i].index, fams[i].label)
		rows = append(rows, line(fams[i].label, itoa(fams[i].count), st))
	}
	return rows
}

// tryHere paints this terminal in the highlighted theme without writing a
// thing, so it can be judged on real output: F2 shows the terminal alone.
// Leaving puts back what the config says.
func (m *Model) tryHere() tea.Cmd {
	if m.cur == nil {
		return nil
	}
	if m.live == nil {
		m.live = openLive()
	}
	if m.live == nil {
		m.warn("This terminal cannot be repainted from here")
		return nil
	}
	m.live.Reset()
	m.live.Show(m.cur.Colors)
	m.tried = m.cur.Name
	m.term = termColours{bg: m.cur.Background(), fg: m.cur.Foreground()}
	applied := "your own colours"
	if m.applied != "" {
		applied = m.applied
	}
	m.info("Trying %s in this window, nothing is written — Enter applies it, leaving puts %s back", m.cur.Name, applied)
	return askTerminalColours()
}

// openLive attaches to the terminal; a variable so tests never write to one.
var openLive = ghostty.OpenLive
