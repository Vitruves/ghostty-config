package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The screen is a terminal, painted in the theme being judged, with one
// palette floating over it. The palette holds a prompt, the four groups,
// one short list, and what the highlighted line means. Everything the
// editor can do is reachable from it, and only one list is ever on screen.

// layout is where the palette sits for the current window and content.
type layout struct {
	x, y, w, h     int
	listH, detailH int
	gap            int // a row of air under the prompt, when there is room
}

const (
	minListRows   = 5
	minDetailRows = 3
	paletteFixed  = 5 // prompt, groups, two rules, footer
	frameRows     = 2 // the border above and below the card
)

// layout sizes the palette to its content: as many rows as the list needs,
// the rest to the explanation, never taller than the window allows.
func (m *Model) layout() layout {
	w := clampInt(m.width*72/100, 52, 80)
	if m.width < 66 {
		w = m.width - 4
	}
	if w < 20 {
		w = maxInt(m.width-2, 1)
	}
	gap := 0
	if m.height >= 26 {
		gap = 1
	}
	avail := m.height - 2 - frameRows - paletteFixed - gap // header and status lines
	if avail < minListRows+minDetailRows {
		avail = minListRows + minDetailRows
	}
	// The explanation gets what it asks for, up to a point; the list takes
	// everything else, so a long list shows as many rows as the window allows.
	wanted := clampInt(m.detailWanted(w-6), minDetailRows, 7)
	maxList := avail - wanted
	if maxList < minListRows {
		maxList = minListRows
	}
	listH := clampInt(len(m.results), minListRows, maxList)
	if m.editing {
		listH = clampInt(len(ghostty.ColorKeys), minListRows, maxList)
	}
	detailH := clampInt(m.detailWanted(w-6), minDetailRows, avail-listH)
	x := (m.width - w) / 2
	if x < 1 {
		x = 1
	}
	// x and y are where the card itself starts; its border sits one column
	// and one row further out.
	return layout{x: x, y: 2, w: w, h: listH + detailH + paletteFixed + gap, listH: listH, detailH: detailH, gap: gap}
}

// detailWanted is how many rows the explanation would like.
func (m *Model) detailWanted(width int) int {
	return len(m.detailLines(width))
}

// Interfaces are the colour schemes the editor's own chrome can wear. The
// terminal behind the palette is always painted in the theme being judged;
// the palette, the header and the status line are the editor. They are light
// by default: most terminal themes are dark, and a light card on a dark
// terminal is the one that cannot be missed.
var interfaceNames = []string{"paper", "graphite", "theme"}

var interfaceNotes = map[string]string{
	"paper":    "light, whatever the theme",
	"graphite": "dark grey, whatever the theme",
	"theme":    "a lighter shade of the theme being shown",
}

// interfaceName is the scheme in effect.
func (m *Model) interfaceName() string {
	for _, n := range interfaceNames {
		if n == m.state.Interface {
			return n
		}
	}
	return "paper"
}

// panelChrome is the chrome of the editor itself. Its border is a colour of
// middling lightness, so it stands out from the card and from the terminal
// whether that is dark or light.
func (m *Model) panelChrome() chrome {
	switch m.interfaceName() {
	case "paper":
		p := buildChrome("#f6f3ec", "#23201c", "#1f55c0", "#0f7b8a", "#8a5a00", "#b3261e", "#1f7a3a")
		p.faint = "#d9d3c5"
		p.muted = "#6b6357"
		p.selBg = "#cdddf8"
		p.selFg = "#14233f"
		p.border = "#3b82f6"
		return p
	case "graphite":
		p := buildChrome("#1f2126", "#e9e7e3", "#8ab4f8", "#7fd1c7", "#f0c674", "#f28b82", "#8fd19e")
		p.faint = "#3a3d45"
		p.border = "#5b9cf5"
		return p
	}
	p := m.c
	p.bg = color.Blend(m.c.bg, m.c.fg, 0.09)
	p.faint = color.Blend(m.c.fg, p.bg, 0.72)
	p.muted = color.EnsureContrast(color.Blend(m.c.fg, p.bg, 0.40), p.bg, 4.5)
	p.selBg = color.Blend(m.c.accent, p.bg, 0.62)
	if color.Contrast(m.c.fg, p.selBg) >= color.Contrast(p.bg, p.selBg) {
		p.selFg = m.c.fg
	} else {
		p.selFg = p.bg
	}
	p.border = color.EnsureContrast(m.c.accent, m.c.bg, 3)
	return p
}

// viewMain draws the terminal behind, the palette over it, and the two
// lines that frame them.
func (m *Model) viewMain() string {
	c := m.c
	lines := make([]string, m.height)
	lines[0] = m.viewHeader()
	back := m.backdrop(m.width, m.height-2)
	for i := 1; i < m.height-1; i++ {
		lines[i] = c.fill(back[i-1], m.width)
	}
	lines[m.height-1] = m.viewStatus()
	if m.peek {
		ui := m.panelChrome()
		lines[m.height-1] = rebase(ui.fill(ui.base().Render(" ")+ui.keyHintFit(m.width-2, "any key", "brings the palette back"), m.width), ui.base())
		return strings.Join(lines, "\n")
	}

	l := m.layout()
	framed := strings.Split(m.viewPalette(l), "\n")
	for i, pl := range framed {
		row := l.y - 1 + i
		if row <= 0 || row >= m.height-1 {
			continue
		}
		lines[row] = truncateExact(lines[row], l.x-1) + pl + dropLeft(lines[row], l.x+l.w+1)
	}
	return strings.Join(lines, "\n")
}

// viewMainBackdrop is the screen without the palette, for dialogs to sit on.
func (m *Model) viewMainBackdrop() string {
	lines := make([]string, m.height)
	lines[0] = m.viewHeader()
	back := m.backdrop(m.width, m.height-2)
	for i := 1; i < m.height-1; i++ {
		lines[i] = m.c.fill(back[i-1], m.width)
	}
	lines[m.height-1] = m.viewStatus()
	return strings.Join(lines, "\n")
}

// viewHeader is the top line: wordmark, the applied theme, warnings, the
// config file being written.
func (m *Model) viewHeader() string {
	c := m.panelChrome()
	left := c.bold(c.accent).Render(" ghostty-config ")
	applied := "no theme set"
	if m.applied != "" {
		applied = m.applied
	}
	mid := c.mutedS().Render("applied ") + c.base().Render(applied)
	if m.cur != nil && m.cur.Name != m.applied {
		mid += c.mutedS().Render("  ·  showing ") + c.text(c.warn).Render(m.cur.Name)
	}
	if m.dirty {
		mid += c.text(c.warn).Render(" ● unsaved")
	}
	right := c.mutedS().Render(filepath.Base(m.tree.Primary.Path) + " ")
	if len(m.overrides) > 0 {
		right = c.text(c.warn).Render(fmt.Sprintf("%d colour line(s) override the theme  ", len(m.overrides))) + right
	}
	if m.reloadFailed {
		right = c.text(c.warn).Render("reload by hand: "+ghostty.ReloadHint(m.paths.Binary)+"  ") + right
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(mid) - lipgloss.Width(right)
	if gap < 2 {
		right = ""
		gap = m.width - lipgloss.Width(left) - lipgloss.Width(mid)
		if gap < 1 {
			gap = 1
		}
	}
	return rebase(c.fill(left+mid+c.base().Render(strings.Repeat(" ", gap))+right, m.width), c.base())
}

// shortenPath swaps the home directory for ~.
func shortenPath(p string) string {
	if home := homeDir(); home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// viewStatus is the bottom line: what just happened, or what the theme is.
func (m *Model) viewStatus() string {
	c := m.panelChrome()
	tone := c.muted
	switch m.statusKind {
	case statusInfo:
		tone = c.accent2
	case statusWarn:
		tone = c.warn
	case statusFail:
		tone = c.danger
	}
	return rebase(c.fill(c.base().Render(" ")+c.text(tone).Render(truncate(m.status, m.width-2)), m.width), c.base())
}

// --- the terminal behind ----------------------------------------------------

// judgingColours reports whether the list on screen is about colour, in
// which case the terminal behind is shown at full strength.
func (m *Model) judgingColours() bool {
	if m.editing || m.peek {
		return true
	}
	if m.cmd == nil {
		return false
	}
	switch m.cmd.name {
	case "theme", "favs", "edit", "new", "random", "undo", "save", "fork", "fav":
		return true
	}
	return false
}

// backdrop draws a slice of real terminal output in the theme on screen:
// a listing, a git status, some code, a test run. It is what the theme and
// the font are judged on, so it is drawn in the exact colours being edited.
func (m *Model) backdrop(width, height int) []string {
	c := m.c
	if m.cur == nil {
		return make([]string, height)
	}
	dim := 0.38
	if m.judgingColours() {
		dim = 0
	}
	t := m.cur
	bg := t.Background()
	col := func(i int, fallback string) lipgloss.Style {
		v := color.Normalize(t.Get(paletteKey(i)), fallback)
		return c.text(color.Blend(v, bg, dim))
	}
	fg := c.text(color.Blend(t.Foreground(), bg, dim))
	red, green, yellow, blue, magenta, cyan := col(1, "#ff0000"), col(2, "#00ff00"), col(3, "#ffff00"), col(4, "#0000ff"), col(5, "#ff00ff"), col(6, "#00ffff")
	grey := col(8, "#808080")
	prompt := func(cmd, arg string) string {
		return green.Render("user") + fg.Render("@") + green.Render("host") + fg.Render(" ") + blue.Render("~/projects") + fg.Render(" $ ") + blue.Render(cmd) + fg.Render(" ") + cyan.Render(arg)
	}
	file := func(mode, name string, s lipgloss.Style) string {
		return grey.Render(mode) + fg.Render(" ") + yellow.Render("user staff") + fg.Render(" ") + s.Render(name)
	}
	rows := []string{
		"",
		prompt("ls", "-la"),
		file("drwxr-xr-x", "src/", blue),
		file("-rwxr-xr-x", "ghostty-config", green),
		file("-rw-r--r--", "README.md", fg),
		file("-rw-r--r--", ".gitignore", cyan),
		file("-rw-r--r--", "backup.tar.gz", magenta),
		file("lrwxrwxrwx", "broken -> missing", red),
		"",
		prompt("git", "status"),
		fg.Render("On branch ") + green.Render("main"),
		fg.Render("  ") + green.Render("modified:   internal/tui/panels.go"),
		fg.Render("  ") + red.Render("deleted:    internal/tui/gallery.go"),
		fg.Render("  ") + yellow.Render("untracked:  notes.md"),
		"",
		magenta.Render("func") + fg.Render(" ") + blue.Render("Render") + fg.Render("(n ") + cyan.Render("int") + fg.Render(") ") + cyan.Render("error") + fg.Render(" {"),
		fg.Render("    ") + cyan.Render("fmt") + fg.Render(".") + blue.Render("Println") + fg.Render("(") + yellow.Render("\"hello\"") + fg.Render(", ") + magenta.Render("42") + fg.Render(")  ") + grey.Render("// a comment sits here"),
		fg.Render("    ") + magenta.Render("return") + fg.Render(" ") + red.Render("nil"),
		fg.Render("}"),
		"",
		prompt("make", "test"),
		green.Render("ok") + fg.Render("    internal/ghostty   0.19s"),
		green.Render("ok") + fg.Render("    internal/tui       1.07s"),
		"",
		fg.Render("il1I| oO0 rn m  -> => != <=   0123456789  {}[]()<>"),
	}
	// The right margin carries the sixteen slots by name, normal beside
	// bright, so the ramp is in view whatever the palette covers.
	var ramp []string
	if width >= 60 {
		ramp = append(ramp, "")
		for i := 0; i < 8; i++ {
			n := color.Blend(color.Normalize(t.Get(paletteKey(i)), bg), bg, dim)
			b := color.Blend(color.Normalize(t.Get(paletteKey(i+8)), bg), bg, dim)
			ramp = append(ramp, c.text(n).Render("██")+c.text(b).Render("██")+fg.Render(" ")+grey.Render(pad(color.ANSI[i], 8)))
		}
	}
	const rampW = 13
	out := make([]string, height)
	for i := 0; i < height; i++ {
		line := ""
		if i < len(rows) && rows[i] != "" {
			line = c.base().Render("  ") + rows[i]
		}
		if i < len(ramp) && ramp[i] != "" {
			line = c.fill(truncateExact(line, width-rampW), width-rampW) + ramp[i]
		}
		out[i] = line
	}
	return out
}

// --- the palette ------------------------------------------------------------

// on is a style with both colours given, for the cells where the card
// meets the terminal and neither chrome applies.
func on(fg, bg string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Background(lipgloss.Color(bg))
}

// The card and its border are drawn with sextants, a cell cut in two columns
// and three rows, and with half blocks. The border is a band a third of a
// row high above and below and half a cell wide on the sides, so it is about
// as thick all round. At each corner the card's own cell is cut in steps, the
// border showing through the cut, and the cell outside the corner is left
// empty: both the card and its border read as rounded. Modern terminals draw
// these glyphs themselves; where that cannot be counted on the border is
// made of half blocks and the corners stay square.
const (
	cornerTL = "\U0001FB35" // cells 4 5 6
	cornerTR = "\U0001FB31" // cells 3 5 6
	cornerBL = "\U0001FB0A" // cells 1 2 4
	cornerBR = "\U0001FB06" // cells 1 2 3
	bandTop  = "\U0001FB2D" // the lower third of a cell
	bandBot  = "\U0001FB02" // the upper third of a cell
)

// frame wraps the rows of a card, each exactly width cells, in the border.
// top and bottom are the colours of the first and last rows, which may
// differ from each other when the card ends in a strip.
func (m *Model) frame(rows []string, width int, top, bottom, border string) []string {
	outer := m.c.bg
	blank := on(outer, outer).Render(" ")
	left := on(border, outer).Render("▐")
	right := on(border, outer).Render("▌")
	above, below := bandTop, bandBot
	if !m.caps.roundCaps {
		above, below = "▄", "▀"
	}
	out := make([]string, 0, len(rows)+2)
	out = append(out, blank+on(border, outer).Render(strings.Repeat(above, width))+blank)
	for i, r := range rows {
		if m.caps.roundCaps && width >= 4 {
			switch i {
			case 0:
				r = on(top, border).Render(cornerTL) + truncateExact(dropLeft(r, 1), width-2) + on(top, border).Render(cornerTR)
			case len(rows) - 1:
				r = on(bottom, border).Render(cornerBL) + truncateExact(dropLeft(r, 1), width-2) + on(bottom, border).Render(cornerBR)
			}
		}
		out = append(out, left+r+right)
	}
	return append(out, blank+on(border, outer).Render(strings.Repeat(below, width))+blank)
}

// viewPalette draws the floating card: a filled panel inside a border, with
// a strip of keys at its foot.
func (m *Model) viewPalette(l layout) string {
	p := m.panelChrome()
	foot := p
	foot.bg = color.Blend(p.bg, "#000000", 0.22)
	if !color.IsDark(p.bg) {
		foot.bg = color.Blend(p.bg, "#000000", 0.06)
	}
	foot.muted = color.EnsureContrast(color.Blend(p.fg, foot.bg, 0.40), foot.bg, 4.5)
	w := l.w

	row := func(ch chrome, content string) string { return rebase(ch.fill(content, w), ch.base()) }
	rule := row(p, p.faintS().Render(strings.Repeat("─", w)))

	var rows []string
	rows = append(rows, row(p, m.viewPromptLine(p, w, l)))
	if l.gap > 0 {
		rows = append(rows, row(p, ""))
	}
	rows = append(rows, row(p, m.viewGroups(p, w, l)))
	rows = append(rows, rule)
	list := m.viewResults(p, w, l, len(rows))
	for i := 0; i < l.listH; i++ {
		line := ""
		if i < len(list) {
			line = list[i]
		}
		rows = append(rows, row(p, line))
	}
	rows = append(rows, rule)
	detail := m.detailStyled(p, w-6)
	if m.previewScrl > len(detail)-l.detailH {
		m.previewScrl = maxInt(0, len(detail)-l.detailH)
	}
	if m.previewScrl > 0 {
		detail = detail[m.previewScrl:]
	}
	m.addRegion(l.x, l.y+len(rows), w, l.detailH, hitPreview, 0, "")
	for i := 0; i < l.detailH; i++ {
		line := ""
		if i < len(detail) {
			line = p.base().Render("   ") + detail[i]
		}
		rows = append(rows, row(p, line))
	}
	rows = append(rows, row(foot, foot.base().Render("   ")+m.keyLine(foot, w-6)))
	return strings.Join(m.frame(rows, w, p.bg, foot.bg, p.border), "\n")
}

// card draws a dialog in the editor's colours, inside the same border. It
// is width+2 cells wide.
func (m *Model) card(title, hint string, lines []string, width int) string {
	p := m.panelChrome()
	row := func(content string) string { return rebase(p.fill(content, width), p.base()) }
	head := p.base().Render("   ") + p.bold(p.accent).Render(truncate(title, width-6))
	if hint != "" {
		gap := width - lipgloss.Width(head) - lipgloss.Width(hint) - 3
		if gap > 0 {
			head += p.base().Render(strings.Repeat(" ", gap)) + p.mutedS().Render(hint)
		}
	}
	rows := []string{row(head), row(p.faintS().Render(strings.Repeat("─", width)))}
	for _, l := range lines {
		rows = append(rows, row(p.base().Render(" ")+l))
	}
	rows = append(rows, row(""))
	return strings.Join(m.frame(rows, width, p.bg, p.bg, p.border), "\n")
}

// viewPromptLine is the prompt, or the slot editor while editing.
func (m *Model) viewPromptLine(p chrome, width int, l layout) string {
	var left string
	if m.editing {
		v := m.cur.Colors[m.editKey]
		typed := ""
		if m.editText != "" {
			typed = p.text(p.warn).Render("   #" + m.editText + "▏")
		}
		left = p.base().Render("   ") + p.bold(p.accent).Render("✎ "+ghostty.Label(m.editKey)) + p.base().Render("  ") + p.swatch(v, 3) + p.base().Render(" "+v) + typed
	} else {
		m.prompt.PromptStyle = p.bold(p.warn)
		m.prompt.TextStyle = p.bold(p.fg)
		m.prompt.PlaceholderStyle = p.mutedS()
		m.prompt.Cursor.Style = on(p.bg, p.warn)
		m.prompt.Cursor.TextStyle = p.mutedS()
		m.prompt.Placeholder = "type a command, or pick one below"
		left = p.base().Render("   ") + m.prompt.View()
	}
	right := p.mutedS().Render(m.countWord() + "   ")
	m.addRegion(l.x, l.y, width-lipgloss.Width(right), 1, hitPrompt, 0, "")
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return truncate(left, width)
	}
	return left + p.base().Render(strings.Repeat(" ", gap)) + right
}

// countWord says how much the list holds.
func (m *Model) countWord() string {
	n := 0
	for _, o := range m.results {
		if !o.header {
			n++
		}
	}
	switch {
	case m.editing:
		return "22 slots"
	case m.cmd == nil && strings.TrimSpace(m.prompt.Value()) == "":
		return fmt.Sprintf("%d commands", n)
	case m.cmd != nil && m.cmd.options == nil:
		return "Enter runs it"
	case m.cmd != nil && m.cmd.name == "theme":
		return fmt.Sprintf("%d of %d", n, len(m.lib.Themes))
	case n == 0:
		return "nothing matches"
	case n == 1:
		return "1 match"
	}
	return fmt.Sprintf("%d matches", n)
}

// viewGroups draws the four groups as pills, the active one filled. They
// are the way to everything: Tab walks them, a click opens one.
func (m *Model) viewGroups(p chrome, width int, l layout) string {
	var b strings.Builder
	b.WriteString(p.base().Render("  "))
	x := 2
	y := l.y + 1 + l.gap
	searching := m.cmd == nil && strings.TrimSpace(m.prompt.Value()) != ""
	for i, g := range groups {
		if i == m.group && !searching {
			b.WriteString(m.capL(p.accent, p.bg) + on(p.bg, p.accent).Bold(true).Render(" "+g+" ") + m.capR(p.accent, p.bg))
		} else {
			b.WriteString(p.base().Render(" ") + p.mutedS().Render(" "+g+" ") + p.base().Render(" "))
		}
		chip := len([]rune(g)) + 4
		m.addRegion(l.x+x, y, chip, 1, hitGroup, i, g)
		x += chip
	}
	left := b.String()
	right := ""
	switch {
	case m.cmd != nil:
		right = p.mutedS().Render(title(m.cmd.syntax) + "   ")
	case searching:
		right = p.mutedS().Render("searching every group   ")
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + p.base().Render(strings.Repeat(" ", gap)) + right
}

// viewResults renders the visible window of the list and registers each
// row as clickable.
func (m *Model) viewResults(p chrome, width int, l layout, top int) []string {
	if m.editing {
		return m.viewSlots(p, width, l)
	}
	if len(m.results) == 0 {
		if m.cmd != nil && m.cmd.options == nil {
			return []string{p.base().Render("   ") + p.base().Render(truncate(title(m.cmd.syntax), width-6)), p.base().Render("   ") + p.mutedS().Render("Enter runs it · Esc goes back")}
		}
		return []string{p.base().Render("   ") + p.mutedS().Render("nothing matches — Esc clears")}
	}
	if m.centre {
		m.centre = false
		m.sel.offset = m.sel.cursor - l.listH/2
	}
	m.sel.clamp(len(m.results), l.listH)
	return m.sel.window(len(m.results), l.listH, func(i int, selected bool) string {
		o := m.results[i]
		if o.header {
			return p.base().Render("   ") + p.mutedS().Render(truncate(o.label+"  "+o.detail, width-6))
		}
		m.addRegion(l.x, l.y+top+(i-m.sel.offset), width, 1, hitResult, i, "")
		return m.renderOption(p, o, selected, width)
	})
}

// viewSlots is the list while the slot editor is open: all 22 slots, the
// one being edited highlighted.
func (m *Model) viewSlots(p chrome, width int, l layout) []string {
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
		o := option{label: slotLabel(k), detail: detail, slot: k, swatch: v}
		return m.renderOption(p, o, selected, width)
	})
}

// slotLabel names a slot with its section, as the edit list does.
func slotLabel(key string) string {
	for _, section := range ghostty.Sections {
		for _, k := range section.Keys {
			if k == key {
				if key == "background" || key == "foreground" {
					return key
				}
				return strings.ToLower(section.Title) + " " + ghostty.Label(key)
			}
		}
	}
	return key
}

// renderOption draws one row: what it is on the left, its value or a short
// note right-aligned, the name never sacrificed to the note. The selected
// row is a pill with rounded ends.
func (m *Model) renderOption(p chrome, o option, selected bool, width int) string {
	inner := width - 6 // a margin, the pill's ends, and the marker's cell
	bg := p.bg
	text, note := p.base(), p.mutedS()
	if o.cmd != nil && m.cmd == nil && strings.TrimSpace(m.prompt.Value()) == "" {
		note = p.text(p.accent2)
	}
	mark := p.accentS()
	if selected {
		bg = p.selBg
		text = on(p.selFg, bg).Bold(true)
		note = on(p.selFg, bg)
		mark = on(p.selFg, bg)
	}
	fillS := on(p.fg, bg)

	marker := " "
	if o.current {
		marker = "●"
	}
	detail := o.detail
	prefix, prefixW := "", 0
	if o.slot != "" {
		if color.IsHex(o.swatch) {
			prefix = on(o.swatch, bg).Render("██") + fillS.Render(" ")
		} else {
			prefix = on(p.faint, bg).Render("░░") + fillS.Render(" ")
		}
		prefixW = 3
	}
	ramp, rampW := "", 0
	if o.theme != nil && inner >= 44 {
		ramp, rampW = miniRampOn(o.theme, 2, bg), 17
	}
	if o.theme != nil && o.cmd == nil {
		// A fixed column, so the ramps line up whatever the note says.
		detail = pad(truncate(detail, 11), 11)
	}
	detailW := lipgloss.Width(detail)
	if maxDetail := inner / 2; detailW > maxDetail {
		detail = truncate(detail, maxDetail)
		detailW = lipgloss.Width(detail)
	}
	labelW := inner - 2 - prefixW - rampW - detailW - 1
	if labelW < 12 {
		detail, detailW = "", 0
		labelW = inner - 2 - prefixW - rampW
	}
	label := pad(truncate(o.label, labelW), labelW)
	body := mark.Render(marker) + fillS.Render(" ") + prefix + text.Render(label) + ramp + note.Render(" "+detail)
	if w := lipgloss.Width(body); w < inner {
		body += fillS.Render(strings.Repeat(" ", inner-w))
	}
	if selected {
		return p.base().Render("  ") + m.capL(bg, p.bg) + body + m.capR(bg, p.bg) + p.base().Render("  ")
	}
	return p.base().Render("   ") + body + p.base().Render("   ")
}

// miniRampOn draws a theme's eight normal colours on a given background.
func miniRampOn(t *ghostty.Theme, cell int, bg string) string {
	var b strings.Builder
	b.WriteString(on(bg, bg).Render(" "))
	for i := 0; i < 8; i++ {
		v := t.Get(paletteKey(i))
		if color.IsHex(v) {
			b.WriteString(on(v, bg).Render(strings.Repeat("█", cell)))
		} else {
			b.WriteString(on(bg, bg).Render(strings.Repeat(" ", cell)))
		}
	}
	return b.String()
}

// swatchLine draws eight slots of a theme as wide blocks, labelled.
func (m *Model) swatchLine(t *ghostty.Theme, label string, from int) string {
	c := m.c
	var b strings.Builder
	b.WriteString(c.mutedS().Render(pad(label, 8)))
	for i := 0; i < 8; i++ {
		b.WriteString(c.swatch(t.Get(paletteKey(from+i)), 4))
	}
	return b.String()
}

// keyLine is what you can press here, most useful first, as many as fit.
func (m *Model) keyLine(p chrome, width int) string {
	var pairs []string
	switch {
	case m.editing:
		pairs = []string{"←→", "light", "⇧←→", "hue", "-+", "sat.", "[ ]", "lum.", "Esc", "done", "0-9a-f", "hex", "u", "undo", "↑↓", "slot"}
	case m.cmd == nil:
		pairs = []string{"↑↓", "choose", "Enter", "open", "Tab", "group", "Esc", "leave", "F2", "peek", "F1", "help", "type", "to search"}
	case m.cmd.options == nil:
		pairs = []string{"Enter", "run", "Esc", "back", "F2", "peek", "F1", "help"}
	default:
		pairs = []string{"↑↓", "move", "Enter", "apply", "Esc", "back", "Tab", "complete", "F2", "peek", "F1", "help"}
	}
	return p.keyHintFitSep(width, "  ", pairs...)
}

// --- what the highlighted line means -----------------------------------------

// detailLines is the explanation as plain rows, used to size the palette.
func (m *Model) detailLines(width int) []string {
	return m.detailStyled(m.panelChrome(), width)
}

// detailStyled explains the highlighted line in the palette's colours.
func (m *Model) detailStyled(p chrome, width int) []string {
	saved := m.c
	m.c = p
	defer func() { m.c = saved }()
	switch {
	case m.editing:
		return m.detailEditing(width)
	case m.cmd != nil && m.cmd.preview != nil:
		return m.cmd.preview(m, m.selected(), width)
	}
	if o := m.selected(); o != nil && o.cmd != nil {
		return m.detailCommand(o.cmd, width)
	}
	return []string{m.c.mutedS().Render("No command matches.")}
}

// detailCommand describes a command of the menu: what to type, what it
// does, and what it is set to now.
func (m *Model) detailCommand(cmd *command, width int) []string {
	c := m.c
	head := c.bold(c.fg).Render(truncate(title(cmd.syntax), width))
	lines := []string{head}
	lines = append(lines, para(c.mutedS(), cmd.desc+".", width)...)
	return lines
}

func (m *Model) detailEditing(width int) []string {
	c := m.c
	v := m.cur.Colors[m.editKey]
	lines := []string{c.base().Render(truncate(color.Describe(v, m.cur.Background(), m.editKey != "background"), width))}
	lines = append(lines, para(c.mutedS(), "The terminal behind shows every change as you make it. Nothing is written until you save.", width)...)
	return lines
}

// currentValueOf reports what a command's setting is now, when it has one.
func (m *Model) currentValueOf(cmd *command) string {
	switch cmd.name {
	case "theme":
		return m.applied
	case "font":
		return orDash(m.fontFamily)
	case "style":
		return m.currentStyle()
	case "size":
		return ghostty.FormatSize(m.fontSize) + " pt"
	case "favs":
		return fmt.Sprintf("%d starred", m.state.FavoriteCount())
	case "edit":
		return "22 slots"
	case "download":
		return "Nerd Fonts"
	case "interface":
		return m.interfaceName()
	case "autoreload":
		if m.state.AutoReloadEnabled() {
			return "on"
		}
		return "off"
	case "overrides":
		if len(m.overrides) == 0 {
			return "none"
		}
		return fmt.Sprintf("%d in your config", len(m.overrides))
	case "collection":
		return "152 themes"
	case "save", "undo":
		if m.dirty {
			return "unsaved edits"
		}
		return ""
	}
	for _, s := range m.buildSettings() {
		if s.cmd == cmd.name {
			return s.get(m)
		}
	}
	return ""
}

func (m *Model) titlebarWord() string {
	for _, s := range m.buildSettings() {
		if s.cmd == "titlebar" || s.cmd == "decoration" {
			return s.get(m)
		}
	}
	return ""
}

// wrapPlain breaks plain text into lines of at most width cells.
func wrapPlain(text string, width int) []string {
	if width < 8 {
		width = 8
	}
	var out []string
	for _, paragraph := range strings.Split(text, "\n") {
		line := ""
		for _, w := range strings.Fields(paragraph) {
			switch {
			case line == "":
				line = w
			case lipgloss.Width(line)+1+lipgloss.Width(w) > width:
				out = append(out, line)
				line = w
			default:
				line += " " + w
			}
		}
		out = append(out, line)
	}
	return out
}

// para renders wrapped text in one style.
func para(style lipgloss.Style, text string, width int) []string {
	var out []string
	for _, l := range wrapPlain(text, width) {
		out = append(out, style.Render(l))
	}
	return out
}

// entry renders "key  text" with the text wrapped and hanging under itself.
func (m *Model) entry(key, text string, keyW, width int, keyStyle lipgloss.Style) []string {
	c := m.c
	var out []string
	for i, l := range wrapPlain(text, width-keyW) {
		if i == 0 {
			out = append(out, keyStyle.Render(pad(key, keyW))+c.mutedS().Render(l))
		} else {
			out = append(out, c.base().Render(strings.Repeat(" ", keyW))+c.mutedS().Render(l))
		}
	}
	return out
}

// previewThemePanel says what the highlighted theme is and what Enter does;
// the theme itself is on show behind the palette.
func previewThemePanel(m *Model, opt *option, width int) []string {
	t := m.cur
	if opt != nil && opt.theme != nil {
		t = opt.theme
	}
	if t == nil {
		return nil
	}
	c := m.c
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
	lines := []string{truncate(c.bold(c.fg).Render(t.Name)+c.mutedS().Render("  "+sourceWord(t)), width)}
	lines = append(lines, m.swatchLine(t, "normal", 0)+c.base().Render("   ")+c.swatch(t.Background(), 4)+c.mutedS().Render(" "+t.Background()))
	lines = append(lines, m.swatchLine(t, "bright", 8)+c.base().Render("   ")+c.swatch(t.Foreground(), 4)+c.mutedS().Render(" "+t.Foreground()))
	lines = append(lines, truncate(c.mutedS().Render("text ")+c.text(c.grade(fgRatio)).Render(fmt.Sprintf("%.1f:1 %s", fgRatio, color.Grade(fgRatio)))+c.mutedS().Render("   weakest "+worstName+" ")+c.text(c.grade(worst)).Render(fmt.Sprintf("%.1f:1", worst)), width))
	switch {
	case m.cur != nil && t.Name == m.cur.Name && t.Name == m.applied:
		lines = append(lines, c.mutedS().Render("This is the configured theme."))
	case m.cur != nil && t.Name == m.cur.Name:
		lines = append(lines, m.entry("Enter", "writes theme = "+t.Name+" and reloads Ghostty", 7, width, c.base())...)
	}
	if t.Note != "" {
		lines = append(lines, para(c.mutedS(), t.Note, width)...)
	}
	return lines
}

// previewThemeCurrent is for the commands that act on the theme on screen.
func previewThemeCurrent(m *Model, opt *option, width int) []string {
	if m.cur == nil {
		return nil
	}
	c := m.c
	lines := []string{truncate(c.bold(c.fg).Render(m.cur.Name)+c.mutedS().Render("  "+sourceWord(m.cur)), width)}
	if m.cmd != nil {
		lines = append(lines, para(c.mutedS(), m.cmd.desc+".", width)...)
	}
	if m.dirty {
		lines = append(lines, para(c.text(c.warn), "Unsaved edits: save [name] keeps them, undo drops them.", width)...)
	}
	return lines
}

// previewNewPanel explains the creator.
func previewNewPanel(m *Model, opt *option, width int) []string {
	c := m.c
	var lines []string
	if m.cur != nil && m.cur.Source == ghostty.SourceDraft {
		lines = append(lines, truncate(c.bold(c.fg).Render(m.cur.Name)+c.text(c.warn).Render("  draft, shown behind"), width))
		lines = append(lines, para(c.mutedS(), "Enter makes another. save <name> keeps it, undo goes back.", width)...)
		return lines
	}
	if opt != nil {
		lines = append(lines, truncate(c.bold(c.fg).Render(opt.label)+c.mutedS().Render("  "+opt.detail), width))
	}
	lines = append(lines, para(c.mutedS(), "Enter generates it and shows it behind. Add light or dark, or a #hex colour to seed the hue. Every colour is held at 4.5:1 or better.", width)...)
	return lines
}

// previewPalettePanel explains the slot list.
func previewPalettePanel(m *Model, opt *option, width int) []string {
	c := m.c
	var lines []string
	if opt != nil && opt.slot != "" {
		v := m.cur.Colors[opt.slot]
		if v == "" {
			lines = append(lines, c.base().Render(opt.label)+c.mutedS().Render("  not set; Ghostty picks its own"))
		} else {
			lines = append(lines, c.base().Render(truncate(color.Describe(v, m.cur.Background(), opt.slot != "background"), width)))
		}
	}
	lines = append(lines, para(c.mutedS(), "Enter opens the slot editor: arrows nudge it, or type a hex. edit red #ff5555 sets it outright.", width)...)
	return lines
}

// previewFontPanel describes the highlighted family.
func previewFontPanel(m *Model, opt *option, width int) []string {
	c := m.c
	if !m.fontsLoaded {
		return []string{c.mutedS().Render(m.spin.View() + " Listing the fonts Ghostty can load…")}
	}
	family := m.fontFamily
	var styles []string
	mono := true
	if opt != nil && opt.family != nil {
		family, styles, mono = opt.family.Name, opt.family.Styles, opt.family.Mono
	} else if f, ok := m.findFamily(m.fontFamily); ok {
		styles, mono = f.Styles, f.Mono
	}
	kind := "monospace"
	if !mono {
		kind = "proportional: columns will not line up"
	}
	lines := []string{truncate(c.bold(c.fg).Render(orDash(family))+c.mutedS().Render(fmt.Sprintf("  %s · %s · %s pt", kind, m.currentStyle(), ghostty.FormatSize(m.fontSize))), width)}
	if len(styles) > 1 {
		lines = append(lines, m.entry("styles", strings.Join(styles, ", "), 8, width, c.mutedS())...)
	}
	if m.autoReloadWorks() {
		lines = append(lines, para(c.mutedS(), "Moving the highlight writes it and reloads Ghostty: the terminal behind changes as you go.", width)...)
	} else {
		lines = append(lines, para(c.text(c.warn), "Moving the highlight writes it; "+m.reloadHint()+".", width)...)
	}
	return lines
}

// previewDownloadPanel explains the downloader.
func previewDownloadPanel(m *Model, opt *option, width int) []string {
	c := m.c
	var lines []string
	if opt != nil {
		lines = append(lines, truncate(c.bold(c.fg).Render(opt.label)+c.mutedS().Render("  "+opt.detail), width))
	}
	lines = append(lines, para(c.mutedS(), "Enter installs it from the Nerd Fonts archive into your user fonts, icons and powerline glyphs included. No administrator rights.", width)...)
	return lines
}

// previewSettingPanel explains a setting and the highlighted value, with a
// sketch of the window when the setting changes its frame.
func previewSettingPanel(m *Model, s setting, opt *option, width int) []string {
	c := m.c
	current := s.get(m)
	lines := []string{truncate(c.bold(c.fg).Render(s.label)+c.mutedS().Render("  "+s.key+"  now ")+c.text(c.accent2).Render(current), width)}
	if s.explain != nil && opt != nil && s.explain[opt.value] != "" {
		lines = append(lines, m.entry(opt.value, s.explain[opt.value]+".", len([]rune(opt.value))+2, width, c.base())...)
	} else {
		lines = append(lines, para(c.mutedS(), s.detail+".", width)...)
	}
	if s.note != "" {
		lines = append(lines, c.text(c.warn).Render(truncate(s.note, width)))
	}
	if s.cmd == "titlebar" || s.cmd == "decoration" || s.cmd == "paddingx" || s.cmd == "paddingy" {
		lines = append(lines, m.windowMock(width, opt)...)
	}
	return lines
}

// windowMock sketches the window frame as the setting would draw it.
func (m *Model) windowMock(width int, opt *option) []string {
	c := m.c
	style := m.titlebarWord()
	if opt != nil {
		switch opt.value {
		case "native", "transparent", "tabs", "hidden", "none":
			style = opt.value
		}
	}
	w := clampInt(width, 30, 44)
	px := clampInt(atoiDefault(m.tree.Value("window-padding-x", "2"), 2)/3, 1, 8)
	if m.cmd != nil && m.cmd.name == "paddingx" && opt != nil {
		px = clampInt(atoiDefault(opt.value, 2)/3, 1, 8)
	}
	b := c.text(c.border)
	dots := c.text(c.danger).Render(" ● ") + c.text(c.warn).Render("● ") + c.text(c.ok).Render("● ")
	rest := w - 2 - 7
	var lines []string
	switch style {
	case "native":
		lines = append(lines, b.Render("╭"+strings.Repeat("─", w-2)+"╮"))
		lines = append(lines, b.Render("│")+dots+lipgloss.NewStyle().Foreground(lipgloss.Color(c.bg)).Background(lipgloss.Color(c.muted)).Render(pad(" system colour", rest))+b.Render("│"))
	case "transparent":
		lines = append(lines, b.Render("╭"+strings.Repeat("─", w-2)+"╮"))
		lines = append(lines, b.Render("│")+dots+c.mutedS().Render(pad(" theme background", rest))+b.Render("│"))
	case "tabs":
		lines = append(lines, b.Render("╭"+strings.Repeat("─", w-2)+"╮"))
		lines = append(lines, b.Render("│")+dots+c.selected().Render(" fish ")+c.mutedS().Render(pad(" zsh  +", rest-6))+b.Render("│"))
	case "hidden":
		lines = append(lines, b.Render("╭"+strings.Repeat("─", w-2)+"╮"))
	default:
		lines = append(lines, c.faintS().Render("┌"+strings.Repeat("─", w-2)+"┐"))
	}
	lines = append(lines, b.Render("│")+c.base().Render(pad(strings.Repeat(" ", px)+"user@host ~ $ ls", w-2))+b.Render("│"))
	lines = append(lines, b.Render("╰"+strings.Repeat("─", w-2)+"╯"))
	return lines
}

// previewOverview is for the one-shot tool commands.
func previewOverview(m *Model, opt *option, width int) []string {
	c := m.c
	if m.cmd == nil {
		return nil
	}
	lines := []string{c.bold(c.fg).Render(truncate(title(m.cmd.syntax), width))}
	return append(lines, para(c.mutedS(), m.cmd.desc+".", width)...)
}

// previewOverridesPanel lists the offending lines.
func previewOverridesPanel(m *Model, opt *option, width int) []string {
	c := m.c
	if len(m.overrides) == 0 {
		return append([]string{c.text(c.ok).Render("No colour line in your config overrides the theme.")}, para(c.mutedS(), "Ghostty applies background, foreground and palette lines from your config on top of any theme.", width)...)
	}
	var lines []string
	for _, e := range m.overrides {
		lines = append(lines, truncate(c.text(c.warn).Render(e.Location())+c.base().Render("  "+e.Key+" = "+e.Value), width))
	}
	return append(lines, para(c.mutedS(), "These defeat every theme. Enter comments them out with a note; a backup is kept.", width)...)
}

// previewPathsPanel shows where everything is.
func previewPathsPanel(m *Model, opt *option, width int) []string {
	c := m.c
	return []string{
		truncate(c.mutedS().Render("config  ")+c.base().Render(shortenPath(m.tree.Primary.Path)), width),
		truncate(c.mutedS().Render("themes  ")+c.base().Render(shortenPath(m.paths.ThemesDir)), width),
		truncate(c.mutedS().Render("state   ")+c.base().Render(shortenPath(m.paths.StateDir)), width),
	}
}

func orDash(v string) string {
	if v == "" {
		return "default"
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
