package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// openConfirm asks a yes/no question.
func (m *Model) openConfirm(text string, yes func(*Model) tea.Cmd) {
	m.confirmText = text
	m.confirmYes = yes
	m.confirmNo = nil
	m.overlay = overlayConfirm
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
	case overlayHelp, overlayMessage:
		m.overlay = overlayNone
		return m, nil
	case overlayBusy:
		return m, nil
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
		}
	}
	return m, nil
}

// viewOverlay draws the open dialog centred over the screen.
func (m *Model) viewOverlay(body string) string {
	c := m.panelChrome()
	var lines []string
	title, width := "", 0
	switch m.overlay {
	case overlayWelcome:
		return m.viewWelcome()
	case overlayHelp:
		return m.viewHelp()
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
	}
	if width > m.width-2 {
		width = m.width - 2
	}
	box := m.card(title, "", lines, width)
	// The card adds a border row, a title and a rule above the lines, and a
	// border column to the left.
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
	c := m.panelChrome()
	yes := " y  yes "
	no := " n  no "
	m.addRegion(3, row, lipgloss.Width(yes)+2, 1, hitButton, 0, "confirm-yes")
	m.addRegion(3+lipgloss.Width(yes)+4, row, lipgloss.Width(no)+2, 1, hitButton, 1, "confirm-no")
	return m.capL(c.accent, c.bg) + on(c.bg, c.accent).Bold(true).Render(yes) + m.capR(c.accent, c.bg) + c.base().Render("  ") + m.capL(c.selBg, c.bg) + on(c.selFg, c.selBg).Render(no) + m.capR(c.selBg, c.bg) + c.mutedS().Render("   Esc stays")
}

// overlayCentered composes a dialog over the body by replacing the rows it
// covers, so the screen behind stays visible around it.
func overlayCentered(m *Model, body, box string) string {
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
		orig := m.c.fill(bodyLines[row], m.width)
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
	c := m.panelChrome()
	group := func(title string, pairs ...string) []string {
		lines := []string{c.bold(c.accent).Render(" " + title)}
		for i := 0; i+1 < len(pairs); i += 2 {
			lines = append(lines, "  "+c.text(c.warn).Render(pad(pairs[i], 12))+c.base().Render(pairs[i+1]))
		}
		return append(lines, "")
	}
	left := concat(
		group("The palette", "type", "A command, then its argument", "↑ ↓", "Walk the list", "Enter", "Open, or apply the line", "Tab", "Next group, or complete", "Esc", "Back; on an empty prompt, leave", "F2", "Hide the palette to look", "⇧↑ ⇧↓", "Scroll the explanation", "Ctrl+C", "Leave, writing nothing", "F1", "This help"),
		group("Slot editor", "← →", "Brighten / darken", "⇧← ⇧→", "Rotate hue", "- +", "Saturation", "[ ]", "Lightness", "hex digits", "Type an exact value", "⌫", "Clear an optional slot", "u", "Undo this slot", "↑ ↓", "Next slot", "Esc", "Back to the prompt"),
	)
	var right []string
	for _, g := range groups {
		right = append(right, c.bold(c.accent).Render(" "+g))
		for _, cmd := range m.commands() {
			if cmd.group == g {
				right = append(right, "  "+c.text(c.warn).Render(pad(truncate(title(cmd.syntax), 24), 25))+c.mutedS().Render(truncate(cmd.desc, 34)))
			}
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
	return overlayCentered(m, m.viewMainBackdrop(), box)
}

func concat(groups ...[]string) []string {
	var out []string
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// viewWelcome fills the screen with the introduction, drawn in the palette
// that is already live.
func (m *Model) viewWelcome() string {
	c := m.panelChrome()
	features := [][3]string{
		{"Theme rose", "Browse themes", "every match shows behind the palette as you move; Enter applies"},
		{"edit red", "Tune a colour", "brightness, hue, saturation, lightness, or type a hex"},
		{"new triadic", "Create a theme", "from a harmony rule; light, dark, or seeded from a colour"},
		{"font jet", "Pick a font", "only families Ghostty can load; style and size the same way"},
		{"size 14", "Font size", "half points allowed"},
		{"titlebar", "Window", "transparent, tabs, hidden, none; opacity, blur, padding, cursor…"},
		{"save mine", "Keep your edits", "into ~/.config/ghostty/themes, never over a bundled theme"},
		{"", "", ""},
		{"↑ ↓ Enter", "The list", "walk it, apply the highlighted line; clicks work too"},
		{"Esc", "Back", "to the four groups, and from there out; leaving applies nothing"},
		{"F1", "All keys", "and the full list of commands"},
	}
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
	add(center(c.mutedS().Render("one prompt for themes, fonts and the window, previewed where you use them")))
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
