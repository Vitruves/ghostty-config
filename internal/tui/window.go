package tui

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/kimg"
)

// Settings are changed where they are listed: ← and → step the highlighted
// one through its values, and each step is written. Beside the window's own
// settings sits a picture of the window as they draw it.

// settingNow is what a setting is set to, by the word of its command.
func (m *Model) settingNow(cmd string) string {
	for _, s := range m.buildSettings() {
		if s.cmd == cmd {
			return s.get(m)
		}
	}
	return ""
}

// adjustSelected steps the highlighted setting of the menu to its previous or
// next value and writes it. It reports false when the highlighted line is not
// a setting that can be stepped, so the caller can do something else with the
// key.
func (m *Model) adjustSelected(dir int) (tea.Cmd, bool) {
	o := m.selected()
	if o == nil || o.cmd == nil || !o.cmd.adjust || o.cmd.options == nil {
		return nil, false
	}
	opts := o.cmd.options(m, "")
	cur := -1
	for i, x := range opts {
		if x.current {
			cur = i
		}
	}
	next := cur + dir
	if cur < 0 {
		next = 0
	}
	if next < 0 || next >= len(opts) {
		// The end of the scale: nothing further that way.
		return nil, true
	}
	cmd := o.cmd.run(m, "", &opts[next])
	m.recompute()
	return cmd, true
}

// wrapSyntax writes a command's syntax, breaking a long list of choices after
// a bar rather than cutting it.
func wrapSyntax(style lipgloss.Style, s string, width int) []string {
	if lipgloss.Width(s) <= width {
		return []string{style.Render(s)}
	}
	var out []string
	line := ""
	for i, part := range strings.Split(s, "|") {
		if i > 0 {
			part = "|" + part
		}
		if line != "" && lipgloss.Width(line)+lipgloss.Width(part) > width {
			out = append(out, style.Render(line))
			line = strings.TrimPrefix(part, "|")
			continue
		}
		line += part
	}
	return append(out, style.Render(truncate(line, width)))
}

// windowPicture draws the window as the settings have it, with the setting
// named cmd taking the highlighted value instead of its own. It returns nil
// where the terminal shows no pictures.
func (m *Model) windowPicture(width int, cmd string, opt *option) []string {
	if !m.imagesOn() || m.cur == nil || width < 30 {
		return nil
	}
	value := func(name, fallback string) string {
		if name == cmd && opt != nil && opt.value != "" {
			return opt.value
		}
		if v := m.settingNow(name); v != "" {
			return v
		}
		return fallback
	}
	titlebar := value("titlebar", "transparent")
	if runtime.GOOS != "darwin" {
		titlebar = "native"
		if value("decoration", "auto") == "none" {
			titlebar = "none"
		}
	}
	opacity, err := strconv.ParseFloat(value("opacity", "1"), 64)
	if err != nil {
		opacity = 1
	}
	blur := value("blur", "off")
	ui := m.c
	t := m.themeSpec(m.cur)
	spec := kimg.WindowSpec{
		BG: t.BG, FG: t.FG, Cursor: t.Accent, Pal: t.Pal,
		Screen:   ui.bg,
		Behind:   [2]string{ui.accent, ui.warn},
		Titlebar: titlebar,
		Shadow:   value("shadow", "on") != "off",
		Opacity:  opacity,
		Blur:     blur != "off" && blur != "",
		PadX:     clampInt(atoiDefault(value("paddingx", "2"), 2), 0, 40),
		PadY:     clampInt(atoiDefault(value("paddingy", "2"), 2), 0, 30),
	}
	cols, rows := minInt(width, 48), 9
	key := strings.Join([]string{"window", m.cur.Name, spec.BG, spec.FG, spec.Cursor, strings.Join(spec.Pal[:], ""),
		titlebar, boolKey(spec.Shadow), fmt.Sprintf("%.2f", opacity), boolKey(spec.Blur), itoa(spec.PadX), itoa(spec.PadY),
		boolKey(color.IsDark(ui.bg)), ui.accent, ui.warn, itoa(cols)}, "|")
	id := m.imgs.ensure(key, cols, rows, func() []byte {
		return kimg.Window(cols, rows, m.cellW, m.cellH, spec).PNG()
	})
	return pictureRows(id, cols, rows, ui.paint())
}
