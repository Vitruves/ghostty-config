package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The editor takes the whole terminal. From the top: the sections, the
// prompt, the body, a line that says what just happened, and the keys. The
// body is a wall of themes, or a list with the explanation of the highlighted
// line beside it. Nothing is framed: what belongs together is told by where
// it sits and by how heavy the text is, on the terminal's own background.

// screenKind is what the body shows.
type screenKind int

const (
	screenMenu    screenKind = iota // the commands of a section, with their values
	screenOptions                   // the values one command accepts
	screenGallery                   // themes, as a wall of cards
	screenEditor                    // the colour slots of the theme being edited
)

// layout is where everything sits for the current window and content.
type layout struct {
	kind         screenKind
	bodyY, bodyH int
	railW        int // the column on the left; 0 when the window is too narrow
	mainX, mainW int
	sideX, sideW int // the explanation beside the list; 0 when it goes under it
	listH        int // rows of list on screen, which is also a page
	detailH      int // rows of explanation under the list when sideW is 0
	cols         int // cards per row of the wall
}

const (
	marginX  = 2 // empty columns on either side of the screen
	headRows = 3 // sections, prompt, a row of air
	tileW    = 18
	tileGap  = 2
	tilePicH = 6
	tileH    = tilePicH + 2 // the picture, the name, a note
	tileRowH = tileH + 1    // and a row of air under it
)

// screen is what the body shows for the prompt as it stands.
func (m *Model) screen() screenKind {
	switch {
	case m.editing:
		return screenEditor
	case m.cmd == nil:
		return screenMenu
	case themeCommand(m.cmd):
		return screenGallery
	}
	return screenOptions
}

// layout sizes the body: the wall gets as many columns as fit, a list keeps
// to a readable width and leaves the rest to the explanation.
func (m *Model) layout() layout {
	l := layout{kind: m.screen(), bodyY: headRows, mainX: marginX}
	inner := maxInt(m.width-2*marginX, 8)
	foot := 3 // a row of air, the status line, the keys
	if l.kind == screenGallery {
		foot = 4 // and the line about the highlighted theme
	}
	l.bodyH = maxInt(m.height-headRows-foot, 1)

	if l.kind == screenGallery {
		l.mainW = inner
		if inner >= 96 {
			l.railW = 18
			l.mainX += l.railW + 2
			l.mainW = inner - l.railW - 2
		}
		l.cols = maxInt((l.mainW+tileGap)/(tileW+tileGap), 1)
		l.listH = maxInt(l.bodyH/tileRowH, 1) * l.cols
		return l
	}

	rest := inner
	if l.kind == screenOptions && inner >= 104 {
		l.railW = 26
		l.mainX += l.railW + 2
		rest = inner - l.railW - 2
	}
	l.mainW = clampInt(rest*46/100, 34, 54)
	if side := rest - l.mainW - 3; side >= 30 {
		l.sideX = l.mainX + l.mainW + 3
		l.sideW = minInt(side, 72)
		l.listH = l.bodyH
		return l
	}
	l.mainW = rest
	l.detailH = clampInt(l.bodyH/3, 2, 7)
	l.listH = maxInt(l.bodyH-l.detailH-1, 1)
	return l
}

// viewMain draws the screen, top to bottom. Rows come back unpadded; the
// caller fills them to the width of the window.
func (m *Model) viewMain() string {
	ui := m.panelChrome()
	l := m.layout()
	rows := make([]string, 0, m.height)
	rows = append(rows, m.viewHeader(ui), m.viewPromptLine(ui), "")
	rows = append(rows, m.viewBody(ui, l)...)
	rows = append(rows, "")
	if l.kind == screenGallery {
		rows = append(rows, m.viewThemeLine(ui))
	}
	rows = append(rows, m.viewStatus(ui, l), m.viewKeys(ui))
	return strings.Join(rows, "\n")
}

// indent is the left margin of a row.
func indent(ui chrome) string {
	return ui.base().Render(strings.Repeat(" ", marginX))
}

// viewHeader is the top line: the name of the tool, the sections, and on the
// right the applied theme with whatever needs saying about the config.
func (m *Model) viewHeader(ui chrome) string {
	searching := m.cmd == nil && strings.TrimSpace(m.prompt.Value()) != ""
	var b strings.Builder
	b.WriteString(indent(ui))
	x := marginX
	if m.width >= 96 {
		b.WriteString(ui.bold(ui.fg).Render("ghostty-config") + ui.base().Render("   "))
		x += 17
	}
	for i, g := range groups {
		if i > 0 {
			b.WriteString(ui.base().Render("   "))
			x += 3
		}
		st := ui.mutedS()
		if i == m.group && !searching {
			st = ui.bold(ui.fg).Underline(true).UnderlineColor(lipgloss.Color(ui.accent))
		}
		b.WriteString(st.Render(g))
		w := len([]rune(g))
		m.addRegion(x-1, 0, w+2, 1, hitGroup, i, g)
		x += w
	}
	left := b.String()

	applied := "no theme set"
	if m.applied != "" {
		applied = m.applied
	}
	theme := ui.mutedS().Render("applied ") + ui.base().Render(applied)
	if m.cur != nil && m.cur.Name != m.applied {
		theme += ui.mutedS().Render("  ·  showing ") + ui.text(ui.warn).Render(m.cur.Name)
	}
	if m.dirty {
		theme += ui.text(ui.warn).Render(" ● unsaved")
	}
	notes := ""
	if len(m.overrides) > 0 {
		notes += ui.text(ui.warn).Render(fmt.Sprintf("%d colour line(s) override the theme   ", len(m.overrides)))
	}
	if m.reloadFailed {
		notes += ui.text(ui.warn).Render("reload by hand: " + ghostty.ReloadHint(m.paths.Binary) + "   ")
	}
	file := ui.mutedS().Render("   " + filepath.Base(m.tree.Primary.Path))
	// Say as much as fits, the applied theme last to go.
	for _, right := range []string{notes + theme + file, notes + theme, theme} {
		if gap := m.width - x - lipgloss.Width(right) - marginX; gap >= 3 {
			return left + ui.base().Render(strings.Repeat(" ", gap)) + right
		}
	}
	return left
}

// viewPromptLine is the prompt, or the slot editor while a colour is edited,
// with a count of what the list holds on the right.
func (m *Model) viewPromptLine(ui chrome) string {
	var left string
	if m.editing {
		v := m.cur.Colors[m.editKey]
		typed := ""
		if m.editText != "" {
			typed = ui.text(ui.warn).Render("   #" + m.editText + "▏")
		}
		left = indent(ui) + ui.bold(ui.accent).Render("✎ "+ghostty.Label(m.editKey)) + ui.base().Render("  ") + ui.swatch(v, 3) + ui.base().Render(" "+v) + typed
	} else {
		st := m.prompt.Styles()
		st.Focused.Prompt = ui.bold(ui.accent)
		st.Focused.Text = ui.base()
		st.Focused.Placeholder = ui.mutedS()
		st.Cursor.Color = lipgloss.Color(ui.accent)
		st.Cursor.Shape = tea.CursorBar
		st.Cursor.Blink = false
		m.prompt.SetStyles(st)
		m.prompt.SetVirtualCursor(true)
		// v2 shows a placeholder only as wide as the input is: give it the room
		// the row has, minus the margins, the prompt and the count on the right.
		m.prompt.SetWidth(maxInt(m.width-30, 12))
		m.prompt.Placeholder = "type a command, or pick one below"
		left = indent(ui) + m.prompt.View()
	}
	right := ui.mutedS().Render(m.countWord())
	m.addRegion(0, 1, m.width-lipgloss.Width(right)-marginX, 1, hitPrompt, 0, "")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - marginX
	if gap < 1 {
		return truncate(left, m.width)
	}
	return left + ui.base().Render(strings.Repeat(" ", gap)) + right
}

// block is a column of the body: where it starts, how wide it is, its rows.
type block struct {
	x, w int
	rows []string
}

// columns sets blocks side by side, each padded to its own width, on h rows.
func columns(ui chrome, h int, blocks []block) []string {
	out := make([]string, h)
	for i := range out {
		var b strings.Builder
		at := 0
		for _, bl := range blocks {
			if bl.x > at {
				b.WriteString(ui.base().Render(strings.Repeat(" ", bl.x-at)))
			}
			row := ""
			if i < len(bl.rows) {
				row = bl.rows[i]
			}
			b.WriteString(ui.fill(row, bl.w))
			at = bl.x + bl.w
		}
		out[i] = b.String()
	}
	return out
}

// viewBody draws the rows between the prompt and the status line.
func (m *Model) viewBody(ui chrome, l layout) []string {
	var blocks []block
	if l.kind == screenGallery {
		if l.railW > 0 {
			blocks = append(blocks, block{marginX, l.railW, m.viewFamilyRail(ui, l)})
		}
		blocks = append(blocks, block{l.mainX, l.mainW, m.viewGallery(ui, l)})
		return columns(ui, l.bodyH, blocks)
	}
	if l.railW > 0 {
		blocks = append(blocks, block{marginX, l.railW, m.viewCommandRail(ui, l)})
	}
	list := m.viewList(ui, l)
	if l.sideW > 0 {
		blocks = append(blocks, block{l.mainX, l.mainW, list}, block{l.sideX, l.sideW, m.viewSide(ui, l.sideX, l.bodyY, l.sideW, l.bodyH)})
		return columns(ui, l.bodyH, blocks)
	}
	// Too narrow for two columns: the explanation goes under the list.
	rows := make([]string, l.listH+1, l.bodyH)
	copy(rows, list)
	rows = append(rows, m.viewSide(ui, l.mainX, l.bodyY+l.listH+1, l.mainW, l.detailH)...)
	blocks = append(blocks, block{l.mainX, l.mainW, rows})
	return columns(ui, l.bodyH, blocks)
}

// viewList renders the visible window of the list and registers each row as
// clickable.
func (m *Model) viewList(ui chrome, l layout) []string {
	if m.editing {
		return m.viewSlots(ui, l)
	}
	if len(m.results) == 0 {
		if m.cmd != nil && m.cmd.options == nil {
			return []string{ui.base().Render(truncate(title(m.cmd.syntax), l.mainW)), ui.mutedS().Render(truncate("Enter runs it · Esc goes back", l.mainW))}
		}
		return []string{ui.mutedS().Render(truncate("nothing matches — Esc clears", l.mainW))}
	}
	if m.centre {
		m.centre = false
		m.sel.offset = m.sel.cursor - l.listH/2
	}
	m.sel.clamp(len(m.results), l.listH)
	return m.sel.window(len(m.results), l.listH, func(i int, selected bool) string {
		o := m.results[i]
		if o.header {
			return ui.mutedS().Render(truncate(strings.TrimSpace(o.label+"  "+o.detail), l.mainW))
		}
		m.addRegion(l.mainX, l.bodyY+(i-m.sel.offset), l.mainW, 1, hitResult, i, "")
		return m.listRow(ui, o, selected, l.mainW)
	})
}

// viewSlots is the list while the slot editor is open: all 22 slots, the
// one being edited highlighted.
func (m *Model) viewSlots(ui chrome, l layout) []string {
	keys := ghostty.ColorKeys
	cursor := 0
	for i, k := range keys {
		if k == m.editKey {
			cursor = i
		}
	}
	s := scroller{cursor: cursor, offset: m.sel.offset}
	s.clamp(len(keys), l.listH)
	m.sel.offset = s.offset
	bg := m.cur.Background()
	return s.window(len(keys), l.listH, func(i int, selected bool) string {
		k := keys[i]
		v := m.cur.Colors[k]
		detail := "not set"
		if v != "" {
			detail = v
			if k != "background" {
				r := color.Contrast(v, bg)
				detail += fmt.Sprintf("  %4.1f:1 %s", r, color.Grade(r))
			}
		}
		return m.listRow(ui, option{label: slotLabel(k), detail: detail, slot: k, swatch: v}, selected, l.mainW)
	})
}

// quietValue is a value that says nothing is switched on: it is drawn in the
// muted colour, so a list of settings shows at a glance which ones are set.
func quietValue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "off", "false", "never", "none", "default", "auto", "system":
		return true
	}
	return false
}

// listRow draws one row: a mark when it is the value in effect, what it is,
// and its value or a short note at the right. The highlighted row is told by
// a tint across its whole width and by its weight, nothing else.
func (m *Model) listRow(ui chrome, o option, selected bool, width int) string {
	bg := ui.paint()
	label, note, mark := ui.base(), ui.mutedS(), ui.accentS()
	if o.cmd != nil && m.cmd == nil && !quietValue(o.detail) {
		note = ui.base()
	}
	if selected {
		bg = ui.selBg
		label = on(ui.selFg, bg).Bold(true)
		note = on(ui.selFg, bg)
		mark = on(ui.accent, bg)
	}
	fill := on(ui.fg, bg)

	marker := "  "
	if o.current {
		marker = "● "
	}
	prefix, prefixW := "", 0
	switch {
	case o.slot != "":
		if color.IsHex(o.swatch) {
			prefix = on(o.swatch, bg).Render("██") + fill.Render(" ")
		} else {
			prefix = on(ui.faint, bg).Render("░░") + fill.Render(" ")
		}
		prefixW = 3
	case o.theme != nil:
		// The theme's own ground and text, as a two-letter sample.
		prefix = on(o.theme.Foreground(), o.theme.Background()).Bold(true).Render(" Aa ") + fill.Render(" ")
		prefixW = 5
	}

	inner := width - 2 - prefixW - 1 // the mark, the sample, a cell at the right
	detail := o.detail
	if limit := inner * 3 / 5; lipgloss.Width(detail) > limit {
		detail = truncate(detail, limit)
	}
	detailW := lipgloss.Width(detail)
	labelW := inner - detailW - 1
	if labelW < 10 {
		detail, detailW = "", 0
		labelW = inner - 1
	}
	if labelW < 1 {
		labelW = 1
	}
	body := mark.Render(marker) + prefix + label.Render(pad(truncate(o.label, labelW), labelW)) + fill.Render(" ") + note.Render(detail) + fill.Render(" ")
	return fitTo(body, width, bg)
}

// fitTo pads or cuts a rendered row to exactly width cells on a background.
func fitTo(line string, width int, bg string) string {
	w := lipgloss.Width(line)
	if w > width {
		return truncateExact(line, width)
	}
	if w < width {
		return line + on("", bg).Render(strings.Repeat(" ", width-w))
	}
	return line
}

// viewCommandRail is the left column while a command is open: the commands of
// its section with what each is set to, so the neighbours stay one click away.
func (m *Model) viewCommandRail(ui chrome, l layout) []string {
	var cmds []*command
	cur := 0
	for _, c := range m.commands() {
		if c.group != groups[m.group] {
			continue
		}
		if c == m.cmd {
			cur = len(cmds)
		}
		cmds = append(cmds, c)
	}
	s := scroller{cursor: cur}
	s.clamp(len(cmds), l.bodyH)
	return s.window(len(cmds), l.bodyH, func(i int, selected bool) string {
		c := cmds[i]
		m.addRegion(marginX, l.bodyY+(i-s.offset), l.railW, 1, hitCommand, i, c.name)
		name := title(c.name)
		value := truncate(m.currentValueOf(c), l.railW-lipgloss.Width(name)-4)
		gap := l.railW - 2 - lipgloss.Width(name) - lipgloss.Width(value)
		if gap < 1 {
			value, gap = "", l.railW-2-lipgloss.Width(name)
		}
		if selected {
			st := on(ui.selFg, ui.selBg)
			return st.Bold(true).Render(" "+name) + st.Render(strings.Repeat(" ", gap)+value+" ")
		}
		return ui.mutedS().Render(" " + name + strings.Repeat(" ", gap) + value + " ")
	})
}

// viewSide is the explanation of the highlighted line: what the command does,
// what the value means, what Enter would write.
func (m *Model) viewSide(ui chrome, x, y, width, height int) []string {
	detail := m.detailStyled(ui, width)
	if m.previewScrl > len(detail)-height {
		m.previewScrl = maxInt(0, len(detail)-height)
	}
	if m.previewScrl > 0 {
		detail = detail[m.previewScrl:]
	}
	m.addRegion(x, y, width, height, hitPreview, 0, "")
	if len(detail) > height {
		detail = detail[:height]
	}
	return detail
}

// viewThemeLine says what the highlighted theme is and how readable it is.
// The wall shows what it looks like; this is what cannot be seen.
func (m *Model) viewThemeLine(ui chrome) string {
	t := m.cur
	if o := m.selected(); o != nil && o.theme != nil {
		t = o.theme
	}
	if t == nil {
		return ""
	}
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
	right := ui.mutedS().Render("text ") + ui.text(ui.grade(fgRatio)).Render(fmt.Sprintf("%.1f:1 %s", fgRatio, color.Grade(fgRatio)))
	if worstName != "" {
		right += ui.mutedS().Render("   weakest "+worstName+" ") + ui.text(ui.grade(worst)).Render(fmt.Sprintf("%.1f:1", worst))
	}
	note := t.Note
	if note == "" {
		note = sourceWord(t)
	}
	room := m.width - 2*marginX - lipgloss.Width(right) - 3
	if room < 12 {
		return indent(ui) + truncate(ui.bold(ui.fg).Render(t.Name), m.width-2*marginX)
	}
	left := truncate(ui.bold(ui.fg).Render(t.Name)+ui.mutedS().Render("  "+note), room)
	gap := m.width - 2*marginX - lipgloss.Width(left) - lipgloss.Width(right)
	return indent(ui) + left + ui.base().Render(strings.Repeat(" ", gap)) + right
}

// viewStatus is the line above the keys: what just happened, or, on the wall
// with nothing to report, what Enter would do to the highlighted theme.
func (m *Model) viewStatus(ui chrome, l layout) string {
	room := m.width - 2*marginX
	if l.kind == screenGallery && m.statusKind == statusPlain {
		if o := m.selected(); o != nil && o.theme != nil {
			t := o.theme
			line := ui.mutedS().Render("This is the configured theme.")
			if t.Name != m.applied {
				line = ui.base().Render("Enter") + ui.mutedS().Render(" writes theme = "+t.Name+" and reloads Ghostty")
			}
			if t.Deletable() {
				line += ui.mutedS().Render("   ·   ") + ui.base().Render("Ctrl+X") + ui.mutedS().Render(" deletes this theme file, after asking")
			}
			return indent(ui) + truncate(line, room)
		}
	}
	if m.statusKind == statusPlain && !m.editing {
		// Nothing happened: the line stays empty rather than repeat what the
		// top of the screen says.
		return ""
	}
	tone := ui.muted
	switch m.statusKind {
	case statusInfo:
		tone = ui.accent2
	case statusWarn:
		tone = ui.warn
	case statusFail:
		tone = ui.danger
	}
	return indent(ui) + ui.text(tone).Render(truncate(m.status, room))
}

// viewKeys is the last line: what can be pressed here.
func (m *Model) viewKeys(ui chrome) string {
	return indent(ui) + m.keyLine(ui, m.width-2*marginX)
}

// keyLine is what you can press here, most useful first, as many as fit.
func (m *Model) keyLine(ui chrome, width int) string {
	var pairs []string
	switch {
	case m.editing:
		pairs = []string{"←→", "light", "⇧←→", "hue", "-+", "sat.", "[ ]", "lum.", "Esc", "done", "0-9a-f", "hex", "u", "undo", "↑↓", "slot"}
	case m.cmd == nil:
		pairs = []string{"↑↓", "choose"}
		if o := m.selected(); o != nil && o.cmd != nil && o.cmd.adjust {
			pairs = append(pairs, "←→", "change")
		}
		pairs = append(pairs, "Enter", "open", "Tab", "section", "Esc", "leave", "F2", "peek", "F1", "help", "type", "to search")
	case themeCommand(m.cmd):
		pairs = []string{"←↓↑→", "move", "Enter", "apply", "^T", "try here", "^F", "favourite"}
		if o := m.selected(); o != nil && o.theme != nil && o.theme.Deletable() {
			pairs = append(pairs, "^X", "delete")
		}
		pairs = append(pairs, "Tab", "section", "Esc", "back", "F2", "peek", "F1", "help")
	case m.cmd.options == nil:
		pairs = []string{"Enter", "run", "Esc", "back", "F2", "peek", "F1", "help"}
	default:
		pairs = []string{"↑↓", "move", "Enter", "apply", "Esc", "back", "Tab", "complete", "F2", "peek", "F1", "help"}
	}
	return ui.keyHintFitSep(width, "   ", pairs...)
}

// card draws a dialog as a surface laid over the screen: a ground a little
// apart from the screen's, a title, the lines. No line is drawn around it.
// It is width+2 cells wide and len(lines)+5 rows high.
func (m *Model) card(title, hint string, lines []string, width int) string {
	p := m.dialogChrome()
	row := func(content string) string {
		return rebase(p.fill(p.base().Render(" ")+content, width+2), p.base())
	}
	head := p.base().Render("  ") + p.bold(p.fg).Render(truncate(title, width-6))
	if hint != "" {
		if gap := width - lipgloss.Width(head) - lipgloss.Width(hint) - 2; gap > 0 {
			head += p.base().Render(strings.Repeat(" ", gap)) + p.mutedS().Render(hint)
		}
	}
	rows := []string{row(""), row(head), row("")}
	for _, l := range lines {
		rows = append(rows, row(p.base().Render(" ")+l))
	}
	rows = append(rows, row(""), row(""))
	return strings.Join(rows, "\n")
}

// settingValueStyle draws the value of a setting: quiet when it says nothing
// is switched on, plain otherwise.
func settingValueStyle(c chrome, v string) lipgloss.Style {
	if quietValue(v) {
		return c.mutedS()
	}
	return c.base()
}
