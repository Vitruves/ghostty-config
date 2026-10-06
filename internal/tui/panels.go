package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/vitruves/ghostty-config/internal/collection"
	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/extensions"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// What the highlighted line means: the explanations shown beside a list, one
// function per kind of command.

// shortenPath swaps the home directory for ~.
func shortenPath(p string) string {
	if home := homeDir(); home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// on is a style with both colours given, for the cells where the card
// meets the terminal and neither chrome applies.
// An empty colour is left to the terminal.
func on(fg, bg string) lipgloss.Style {
	s := lipgloss.NewStyle()
	if fg != "" {
		s = s.Foreground(lipgloss.Color(fg))
	}
	if bg != "" {
		s = s.Background(lipgloss.Color(bg))
	}
	return s
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
	case m.cmd == nil && n > 0:
		return fmt.Sprintf("%d in all sections", n)
	case n == 0:
		return "nothing matches"
	case n == 1:
		return "1 match"
	}
	return fmt.Sprintf("%d matches", n)
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

// --- what the highlighted line means -----------------------------------------

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
// does, and for a setting of the window, the window as it stands.
func (m *Model) detailCommand(cmd *command, width int) []string {
	c := m.c
	lines := wrapSyntax(c.bold(c.fg), title(cmd.syntax), width)
	lines = append(lines, para(c.mutedS(), cmd.desc+".", width)...)
	if cmd.adjust {
		lines = append(lines, "", truncate(c.base().Render("←→")+c.mutedS().Render(" change it here   ")+c.base().Render("Enter")+c.mutedS().Render(" every value"), width))
	}
	if cmd.group == "Window" {
		if pic := m.windowPicture(width, "", nil); pic != nil {
			lines = append(append(lines, ""), pic...)
		}
	}
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
	case "interface":
		return m.interfaceName()
	case "shader":
		if _, name := m.shaderNow(); name != "" {
			return name
		}
		return "none"
	case "pack":
		n := 0
		for _, p := range extensions.Packs {
			if p.Installed(m.paths.ThemesDir) == len(p.Files) {
				n++
			}
		}
		return fmt.Sprintf("%d of %d installed", n, len(extensions.Packs))
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
		return fmt.Sprintf("%d themes", len(collection.Collection))
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

// previewSettingPanel explains a setting and the highlighted value; for a
// setting of the window, a picture shows the window with that value.
func previewSettingPanel(m *Model, s setting, opt *option, width int) []string {
	c := m.c
	lines := []string{
		c.bold(c.fg).Render(truncate(s.label, width)),
		truncate(c.mutedS().Render(s.key+"  ·  now ")+c.base().Render(s.get(m)), width),
		"",
	}
	explain := s.explain
	if explain == nil {
		explain = valueNotes[s.cmd]
	}
	if explain != nil && opt != nil && explain[opt.value] != "" {
		lines = append(lines, m.entry(opt.value, explain[opt.value]+".", len([]rune(opt.value))+2, width, c.base())...)
	} else {
		lines = append(lines, para(c.mutedS(), s.detail+".", width)...)
	}
	if s.note != "" {
		lines = append(lines, c.text(c.warn).Render(truncate(s.note, width)))
	}
	if s.group == "Window" {
		if pic := m.windowPicture(width, s.cmd, opt); pic != nil {
			lines = append(append(lines, ""), pic...)
		}
	}
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
