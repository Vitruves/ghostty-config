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

// settingNow is what a setting is set to, by the word of its command. The
// commands hold every setting of the table, in its order, made from it when
// the commands were last built: the table is not built again for one value,
// as pictures that ask for several on every frame would have it.
func (m *Model) settingNow(cmd string) string {
	for _, c := range m.commands() {
		if c.set != nil && c.set.cmd == cmd {
			return c.set.get(m)
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
		if s := o.cmd.set; s != nil && s.numeric {
			// Between two steps, or off the scale: the arrow goes to the
			// nearest step its way, never back to the bottom of the scale.
			now, ok := parseNumber(s.get(m))
			if !ok {
				m.warn("%s is set to %s here, which is not a number the arrows can step: type %s <value>", s.label, s.get(m), s.cmd)
				return nil, true
			}
			next = nearestStep(opts, now, dir)
		}
	}
	if next < 0 || next >= len(opts) {
		// The end of the scale: nothing further that way.
		return nil, true
	}
	cmd := o.cmd.run(m, "", &opts[next])
	m.recompute()
	return cmd, true
}

// nearestStep is the index of the first option above now, going up, or the
// last one below it, going down; -1 or len(opts) when there is none that way.
func nearestStep(opts []option, now float64, dir int) int {
	if dir > 0 {
		for i, o := range opts {
			if v, ok := parseNumber(o.value); ok && v > now {
				return i
			}
		}
		return len(opts)
	}
	for i := len(opts) - 1; i >= 0; i-- {
		if v, ok := parseNumber(opts[i].value); ok && v < now {
			return i
		}
	}
	return -1
}

// resetSelected puts the highlighted setting of the menu back to its
// default, which removes its line from the config.
func (m *Model) resetSelected() (tea.Cmd, bool) {
	o := m.selected()
	if o == nil || o.cmd == nil || o.cmd.set == nil || o.cmd.set.fallback == "" {
		return nil, false
	}
	s := o.cmd.set
	if strings.EqualFold(s.get(m), s.fallback) {
		return nil, true
	}
	cmd := o.cmd.run(m, "", &option{value: s.fallback})
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
	spec := m.windowSpec(cmd, opt)
	ui := m.c
	cols, rows := minInt(width, 48), 9
	key := strings.Join([]string{"window", m.cur.Name, spec.BG, spec.FG, spec.Cursor, strings.Join(spec.Pal[:], ""),
		spec.Titlebar, boolKey(spec.Shadow), fmt.Sprintf("%.2f", spec.Opacity), boolKey(spec.Blur), itoa(spec.PadX), itoa(spec.PadY),
		boolKey(color.IsDark(ui.bg)), ui.accent, ui.warn, itoa(cols), fmt.Sprintf("%p %v", spec.Backdrop.Img, spec.Backdrop.Fit+spec.Backdrop.Position),
		fmt.Sprintf("%.2f %v", spec.Backdrop.Opacity, spec.Backdrop.Repeat)}, "|")
	id := m.imgs.ensure(key, cols, rows, func() []byte {
		return kimg.Window(cols, rows, m.cellW, m.cellH, spec).PNG()
	})
	return pictureRows(id, cols, rows, ui.paint())
}

// windowSpec is the window as the settings have it, with the setting named
// cmd taking the highlighted value instead of its own.
func (m *Model) windowSpec(cmd string, opt *option) kimg.WindowSpec {
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
	// The background image, once it is read: until then the window shows
	// without it rather than making the highlight wait.
	if img := value(imageFiles.cmd, "none"); img != "none" {
		if t := m.thumbOf(m.filePath(imageFiles.key, img)); t != nil {
			op, err := strconv.ParseFloat(value("bgopacity", "1"), 64)
			if err != nil {
				op = 1
			}
			spec.Backdrop = kimg.Backdrop{Img: t.img, W: t.w, H: t.h, Opacity: op,
				Fit: value("bgfit", "contain"), Position: value("bgposition", "center"), Repeat: value("bgrepeat", "off") == "on"}
		}
	}
	return spec
}
