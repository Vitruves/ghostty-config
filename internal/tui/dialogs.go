package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// openConfirm asks a yes/no question.
func (m *Model) openConfirm(text string, yes func(*Model) tea.Cmd) {
	m.confirmText = text
	m.confirmYes = yes
	m.confirmNo = nil
	m.confirmEsc = nil
	m.overlay = overlayConfirm
	m.confirmSeq++
}

// openMessage shows a note until a key is pressed.
func (m *Model) openMessage(text string) {
	m.messageText = text
	m.overlay = overlayMessage
}

// updateOverlay routes keys to whichever dialog is open.
func (m *Model) updateOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch m.overlay {
	case overlayWelcome:
		m.overlay = overlayNone
		m.state.Welcomed = true
		_ = m.state.Save()
		return m, nil
	case overlayLayout:
		return m.updateChooser(key)
	case overlayUpdate:
		switch key {
		case "enter", "y", "Y", "i", "I":
			return m, m.startUpdate()
		case "esc", "n", "N", "q":
			m.overlay = overlayNone
		}
		return m, nil
	case overlayHelp, overlayMessage:
		m.overlay = overlayNone
		return m, nil
	case overlayBusy:
		return m, nil
	case overlayExplain:
		return m.updateExplain(key)
	case overlayConfirm:
		switch key {
		case "y", "Y", "enter":
			m.overlay = overlayNone
			if m.confirmYes != nil {
				return m, m.confirmYes(m)
			}
		case "n", "N":
			m.overlay = overlayNone
			if m.confirmNo != nil {
				return m, m.confirmNo(m)
			}
		case "esc", "q":
			m.overlay = overlayNone
			if m.confirmEsc != nil {
				return m, m.confirmEsc(m)
			}
		}
	}
	return m, nil
}

// viewOverlay draws the open dialog centred over the screen.
func (m *Model) viewOverlay(body string) string {
	c := m.dialogChrome()
	var lines []string
	title, width := "", 0
	switch m.overlay {
	case overlayWelcome:
		return m.viewWelcome()
	case overlayLayout:
		return m.viewChooser()
	case overlayHelp:
		return m.viewHelp()
	case overlayExplain:
		return m.viewExplain(body)
	case overlayConfirm:
		title = "Confirm"
		lines = append([]string{""}, wrapLines(m.confirmText, 52, c.base())...)
		lines = append(lines, "", c.base().Render("  ")+m.confirmButtons(len(lines)+4))
		width = 58
	case overlayBusy:
		title = "Working"
		lines = []string{"", " " + c.accentS().Render(m.spin.View()) + " " + c.base().Render(m.busyText)}
		width = 66
	case overlayMessage:
		title = "Note"
		lines = append([]string{""}, wrapLines(m.messageText, 70, c.base())...)
		lines = append(lines, "", c.mutedS().Render("  any key closes"))
		width = 76
	case overlayCapture:
		title = "New shortcut for " + keyLabel(m.kb.action)
		lines, width = m.captureLines(c, 58), 64
	case overlayUpdate:
		title = "Update available"
		lines = append([]string{""}, wrapLines(m.updateText(), 52, c.base())...)
		lines = append(lines, "", c.base().Render("  ")+m.updateButtons(len(lines)+4))
		width = 58
	}
	if width > m.width-2 {
		width = m.width - 2
	}
	box := m.card(title, "", lines, width)
	// The card adds a row of air, a title and another row of air above the
	// lines, and a column to the left.
	top := (m.height - (len(lines) + 5)) / 2
	left := (m.width-(width+2))/2 + 1
	for i := range m.regions {
		if m.regions[i].kind == hitButton {
			m.regions[i].x += left
			m.regions[i].y += top
		}
	}
	return overlayCentered(m, body, box)
}

// confirmButtons renders Yes / No and registers them relative to the box.
func (m *Model) confirmButtons(row int) string {
	c := m.dialogChrome()
	yes := " y  yes "
	no := " n  no "
	m.addRegion(3, row, lipgloss.Width(yes)+2, 1, hitButton, 0, "confirm-yes")
	m.addRegion(3+lipgloss.Width(yes)+4, row, lipgloss.Width(no)+2, 1, hitButton, 1, "confirm-no")
	return m.capL(c.accent, c.paint()) + on(c.bg, c.accent).Bold(true).Render(yes) + m.capR(c.accent, c.paint()) + c.base().Render("  ") + m.capL(c.selBg, c.paint()) + on(c.selFg, c.selBg).Render(no) + m.capR(c.selBg, c.paint()) + c.mutedS().Render("   Esc stays")
}

// overlayCentered composes a dialog over the body by replacing the rows it
// covers, so the screen behind stays visible around it.
func overlayCentered(m *Model, body, box string) string {
	ui := m.panelChrome()
	bodyLines := strings.Split(body, "\n")
	boxLines := strings.Split(box, "\n")
	boxW := lipgloss.Width(boxLines[0])
	top := (m.height - len(boxLines)) / 2
	left := (m.width - boxW) / 2
	if top < 0 {
		top = 0
	}
	if left < 0 {
		left = 0
	}
	for i, bl := range boxLines {
		row := top + i
		if row >= len(bodyLines) {
			break
		}
		orig := ui.fill(bodyLines[row], m.width)
		bodyLines[row] = truncateExact(orig, left) + bl + dropLeft(orig, left+boxW)
	}
	return strings.Join(bodyLines, "\n")
}

// wrapLines breaks text into lines of at most width cells.
func wrapLines(text string, width int, style lipgloss.Style) []string {
	var out []string
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		line := ""
		for _, w := range words {
			if line == "" {
				line = w
				continue
			}
			if lipgloss.Width(line)+1+lipgloss.Width(w) > width {
				out = append(out, "  "+style.Render(line))
				line = w
				continue
			}
			line += " " + w
		}
		out = append(out, "  "+style.Render(line))
	}
	return out
}

// viewHelp shows the keyboard reference and the command list.
func (m *Model) viewHelp() string {
	c := m.dialogChrome()
	group := func(title string, pairs ...string) []string {
		lines := []string{c.bold(c.accent).Render(" " + title)}
		for i := 0; i+1 < len(pairs); i += 2 {
			lines = append(lines, "  "+c.text(c.warn).Render(pad(pairs[i], 12))+c.base().Render(pairs[i+1]))
		}
		return append(lines, "")
	}
	var left []string
	for _, g := range helpKeyGroups() {
		left = append(left, group(g.title, g.pairs...)...)
	}
	var right []string
	right = append(right, c.bold(c.accent).Render(" Commands"), c.mutedS().Render("  type one, then Enter; Tab completes"), "")
	for _, g := range groups {
		right = append(right, "  "+c.bold(c.fg).Render(g))
		var names []string
		for _, cmd := range m.commands() {
			if cmd.group == g && m.inHelp(cmd) {
				names = append(names, title(cmd.name))
			}
		}
		for _, l := range wrapPlain(strings.Join(names, "  "), 56) {
			right = append(right, "    "+c.text(c.warn).Render(l))
		}
		right = append(right, "")
	}
	for len(left) < len(right) {
		left = append(left, "")
	}
	for len(right) < len(left) {
		right = append(right, "")
	}
	var lines []string
	for i := range left {
		lines = append(lines, pad(truncate(left[i], 41), 42)+right[i])
	}
	width := 104
	if width > m.width-2 {
		width = m.width - 2
	}
	if len(lines)+4 > m.height {
		lines = lines[:maxInt(1, m.height-4)]
	}
	box := m.card("Keys and commands", "any key closes", lines, width)
	return overlayCentered(m, m.viewMain(), box)
}

// helpGroup is a titled list of keys and what they do.
type helpGroup struct {
	title string
	pairs []string
}

func helpKeyGroups() []helpGroup {
	return []helpGroup{
		{"Everywhere", []string{"type", "A command, then its argument", "↑ ↓", "Walk the list", "Enter", "Open, or apply the line", "Tab", "Next section, or complete", "Esc", "Back; on an empty prompt, leave", "F2", "Step aside to see the terminal", "⇧↑ ⇧↓", "Scroll the explanation", "F3", "Ghostty's docs for the line", "Ctrl+Z", "Undo the last change", "Ctrl+C", "Leave, writing nothing", "F1", "This help"}},
		{"Settings", []string{"← →", "Change the highlighted setting", "Delete", "Put it back to Ghostty's default"}},
		{"The wall of themes", []string{"← ↓ ↑ →", "Walk the cards", "⇧↑ ⇧↓", "Previous, next group", "Enter", "Apply the highlighted theme", "Ctrl+T", "Try it in this terminal", "Ctrl+F", "Star or unstar it", "Ctrl+X", "Delete its file, after asking"}},
		{"Slot editor", []string{"← →", "Brighten / darken", "⇧← ⇧→", "Rotate hue", "- +", "Saturation", "[ ]", "Lightness", "hex digits", "Type an exact value", "⌫", "Clear an optional slot", "u", "Undo this slot", "↑ ↓", "Next slot", "Esc", "Back to the prompt"}},
	}
}

func concat(groups ...[]string) []string {
	var out []string
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// viewWelcome fills the screen with the introduction, drawn in the colours
// the interface already wears.
func (m *Model) viewWelcome() string {
	c := m.panelChrome()
	features := welcomeFeatures()
	var lines []string
	add := func(s string) { lines = append(lines, s) }
	center := func(s string) string {
		w := lipgloss.Width(s)
		if w >= m.width {
			return s
		}
		return strings.Repeat(" ", (m.width-w)/2) + s
	}
	blank := (m.height - len(features) - 12) / 2
	for i := 0; i < blank; i++ {
		add("")
	}
	add(center(c.bold(c.accent).Render("g h o s t t y   c o n f i g")))
	add(center(c.faintS().Render(strings.Repeat("─", 30))))
	add(center(c.mutedS().Render("themes, fonts, the window and extensions for Ghostty, in your terminal's own colours")))
	add("")
	if m.cur != nil {
		for _, from := range []int{0, 8} {
			line := ""
			for i := 0; i < 8; i++ {
				line += c.swatch(m.cur.Get(paletteKey(from+i)), 3)
			}
			add(center(line))
		}
	}
	add("")
	blockW := 92
	indent := (m.width - blockW) / 2
	if indent < 0 {
		indent = 0
	}
	for _, f := range features {
		if f[0] == "" && f[1] == "" {
			add("")
			continue
		}
		add(strings.Repeat(" ", indent) + c.bold(c.accent).Render(pad(f[0], 13)) + c.base().Render(pad(f[1], 17)) + c.mutedS().Render(f[2]))
	}
	add("")
	add(center(c.mutedS().Render(fmt.Sprintf("%d themes · config: %s", len(m.lib.Themes), m.tree.Primary.Path))))
	add(center(c.mutedS().Render("Looking changes nothing: a theme is applied only when you press Enter on it")))
	add("")
	add(center(c.bold(c.ok).Render("Enter") + c.mutedS().Render(" to begin")))
	for len(lines) < m.height {
		add("")
	}
	for i := range lines {
		lines[i] = rebase(c.fill(lines[i], m.width), c.base())
	}
	return strings.Join(lines[:m.height], "\n")
}

// welcomeFeatures are the lines of the first screen: a key, what it opens,
// and a word on it. An empty line is a pause.
func welcomeFeatures() [][3]string {
	return [][3]string{
		{"← ↓ ↑ →", "The wall of themes", "every theme is a card, grouped by family, dark to light; Enter applies"},
		{"type", "Search, or a command", "catp finds a theme; edit red, new triadic and save mine are commands"},
		{"Tab", "The sections", "Themes, Fonts, Window, Input, Session, Extensions, Tool"},
		{"← →", "Change a setting", "where it is listed; Enter shows every value and what it means"},
		{"font", "Pick or install a font", "the families Ghostty can load, and under them the ones to download"},
		{"shader", "Extensions", "shaders and packs of themes, fetched and switched on for you"},
		{"", "", ""},
		{"Ctrl+T", "Try a theme here", "paints this terminal and writes nothing; F2 steps aside to look"},
		{"Esc", "Back", "to the section's list, and from there out; leaving applies nothing"},
		{"F1", "All keys", "and the full list of commands"},
	}
}

// updateText is what the update dialog says.
func (m *Model) updateText() string {
	return fmt.Sprintf("ghostty-config %s is available. You have %s. Install downloads it, checks it and replaces this binary; the new version starts next time.", m.updateLatest, m.opts.Version)
}

// updateButtons renders Install / Next time and registers them relative to
// the box, like the buttons of a question.
func (m *Model) updateButtons(row int) string {
	c := m.dialogChrome()
	yes := " Enter  install "
	no := " Esc  next time "
	m.addRegion(3, row, lipgloss.Width(yes)+2, 1, hitButton, 0, "update-install")
	m.addRegion(3+lipgloss.Width(yes)+4, row, lipgloss.Width(no)+2, 1, hitButton, 1, "update-later")
	return m.capL(c.accent, c.paint()) + on(c.bg, c.accent).Bold(true).Render(yes) + m.capR(c.accent, c.paint()) + c.base().Render("  ") + m.capL(c.selBg, c.paint()) + on(c.selFg, c.selBg).Render(no) + m.capR(c.selBg, c.paint())
}
