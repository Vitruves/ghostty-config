package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/pix"
)

// A list too long for the window can be shown a page at a time: its options
// begin with a header whose tabs name the pages, the open one marked. The
// arrows and a click turn them; typing searches every page at once, since
// the command then lists them all.

// tabRow is the row of tabs of the open list, when it has one.
func (m *Model) tabRow() *option {
	if m.cmd == nil || m.cmd.tabTo == nil {
		return nil
	}
	for i := range m.results {
		if len(m.results[i].tabs) > 0 {
			return &m.results[i]
		}
	}
	return nil
}

// turnTab opens page i and highlights its first line.
func (m *Model) turnTab(i int) tea.Cmd {
	if m.cmd == nil || m.cmd.tabTo == nil {
		return nil
	}
	m.cmd.tabTo(m, i)
	m.recompute()
	m.sel.cursor, m.sel.offset = 0, 0
	m.skipHeaders(1)
	m.previewScrl = 0
	if m.px != nil {
		m.px.listTop = 0
	}
	return m.afterMove()
}

// tabWindow chooses the tabs that fit in width, the open one among them:
// from it, one more to the right then one to the left, while they fit with
// the marks that say some are left out.
func tabWindow(widths []int, open, width, gap, mark int) (from, to int) {
	from, to = open, open+1
	used := widths[open]
	fits := func(from, to, used int) bool {
		if from > 0 {
			used += mark
		}
		if to < len(widths) {
			used += mark
		}
		return used <= width
	}
	for grew := true; grew; {
		grew = false
		if to < len(widths) && fits(from, to+1, used+gap+widths[to]) {
			used += gap + widths[to]
			to++
			grew = true
		}
		if from > 0 && fits(from-1, to, used+gap+widths[from-1]) {
			from--
			used += gap + widths[from]
			grew = true
		}
	}
	return from, to
}

// cellTabs draws the row of tabs in text, each a place to click.
func (m *Model) cellTabs(ui chrome, o option, x, y, width int) string {
	const gap, mark = 3, 2
	widths := make([]int, len(o.tabs))
	for i, t := range o.tabs {
		widths[i] = len([]rune(t))
	}
	from, to := tabWindow(widths, o.tab, width, gap, mark)
	var b strings.Builder
	at := 0
	if from > 0 {
		b.WriteString(ui.mutedS().Render("‹ "))
		at += mark
	}
	for i := from; i < to; i++ {
		if i > from {
			b.WriteString(ui.base().Render(strings.Repeat(" ", gap)))
			at += gap
		}
		style := ui.mutedS()
		if i == o.tab {
			style = ui.accentS().Bold(true).Underline(true)
		}
		b.WriteString(style.Render(o.tabs[i]))
		m.addRegion(x+at, y, widths[i], 1, hitTab, i, o.tabs[i])
		at += widths[i]
	}
	if to < len(o.tabs) {
		b.WriteString(ui.mutedS().Render(" ›"))
	}
	return fitTo(b.String(), width, ui.paint())
}

// pxTabs draws the row of tabs as an image: the open one in the text colour
// over a bar of the accent, the others muted.
func (m *Model) pxTabs(c *pix.Canvas, p pxPal, f pxFonts, o option, x, y, width float32) {
	const gap, mark = 22, 18
	widths := make([]int, len(o.tabs))
	for i, t := range o.tabs {
		widths[i] = int(c.Width(f.body, t) + 0.5)
	}
	from, to := tabWindow(widths, o.tab, int(width), gap, mark)
	at := x
	ty := mid(y, headH, 13)
	if from > 0 {
		c.Text(f.body, at, ty, "‹", p.muted)
		at += mark
	}
	for i := from; i < to; i++ {
		if i > from {
			at += gap
		}
		col := p.muted
		if i == o.tab {
			col = p.fg
			c.Rect(at, y+headH-4, float32(widths[i]), 2.5, 1, p.accent)
		}
		c.Text(f.body, at, ty, o.tabs[i], col)
		m.pxRegion(c, at-4, y, float32(widths[i])+8, headH, hitTab, i, o.tabs[i])
		at += float32(widths[i])
	}
	if to < len(o.tabs) {
		c.Text(f.body, at+6, ty, "›", p.muted)
	}
}
