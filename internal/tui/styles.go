// Package tui is the editor: a Bubble Tea application whose chrome is drawn
// in the colours of the theme it is showing, so the editor always looks like
// the thing it is editing.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// chrome holds the colours the interface is drawn with. Every value is
// derived from the theme on screen; accents are lifted until they clear WCAG
// AA against the background so the interface stays legible on any theme.
type chrome struct {
	bg, fg, muted, faint, accent, accent2, warn, danger, ok, selBg, selFg, border string
	// clear leaves the background to the terminal. bg is still what the
	// colours are worked out against.
	clear bool
}

func defaultChrome() chrome {
	return buildChrome("#101014", "#d8d8e0", "#7aa2f7", "#7dcfff", "#e0af68", "#f7768e", "#9ece6a")
}

// chromeFor derives the chrome from a theme. Bright variants read better as
// accents on dark backgrounds; the normal set is the fallback.
func chromeFor(t *ghostty.Theme) chrome {
	if t == nil {
		return defaultChrome()
	}
	return chromeOn(t, t.Background())
}

// chromeOn is the theme's chrome on another background: the accents are the
// theme's, lifted until they read on bg, and so is the text.
func chromeOn(t *ghostty.Theme, bg string) chrome {
	if t == nil {
		return defaultChrome()
	}
	pick := func(bright, normal int, fallback string) string {
		for _, k := range []string{paletteKey(bright), paletteKey(normal)} {
			if v := t.Get(k); color.IsHex(v) {
				return color.Normalize(v, fallback)
			}
		}
		return fallback
	}
	fg := t.Foreground()
	if bg != t.Background() {
		fg = color.EnsureReadable(fg, bg)
	}
	return buildChrome(bg, fg,
		pick(12, 4, "#7aa2f7"), pick(14, 6, "#7dcfff"), pick(11, 3, "#e0af68"), pick(9, 1, "#f7768e"), pick(10, 2, "#9ece6a"))
}

func paletteKey(i int) string { return "palette." + itoa(i) }

func buildChrome(bg, fg, blue, cyan, yellow, red, green string) chrome {
	c := chrome{
		bg:      bg,
		fg:      fg,
		muted:   color.Blend(fg, bg, 0.45),
		faint:   color.Blend(fg, bg, 0.72),
		accent:  color.EnsureReadable(blue, bg),
		accent2: color.EnsureReadable(cyan, bg),
		warn:    color.EnsureReadable(yellow, bg),
		danger:  color.EnsureReadable(red, bg),
		ok:      color.EnsureReadable(green, bg),
		border:  color.Blend(fg, bg, 0.70),
	}
	c.selBg = color.Blend(c.accent, bg, 0.68)
	if color.Contrast(fg, c.selBg) >= color.Contrast(bg, c.selBg) {
		c.selFg = fg
	} else {
		c.selFg = bg
	}
	return c
}

// text is the one style factory: every run of text carries the background,
// so a nested reset can never punch a hole in a filled row.
func (c chrome) text(fg string) lipgloss.Style {
	return on(fg, c.paint())
}

// paint is the background cells are painted with: none when clear.
func (c chrome) paint() string {
	if c.clear {
		return ""
	}
	return c.bg
}

func (c chrome) base() lipgloss.Style   { return c.text(c.fg) }
func (c chrome) mutedS() lipgloss.Style { return c.text(c.muted) }
func (c chrome) faintS() lipgloss.Style { return c.text(c.faint) }
func (c chrome) accentS() lipgloss.Style {
	return c.text(c.accent)
}
func (c chrome) bold(fg string) lipgloss.Style { return c.text(fg).Bold(true) }

// selected renders a highlighted row.
func (c chrome) selected() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c.selFg)).Background(lipgloss.Color(c.selBg))
}

// swatch draws a block of the given colour on the chrome background.
func (c chrome) swatch(hex string, width int) string {
	if !color.IsHex(hex) {
		return c.faintS().Render(strings.Repeat("░", width))
	}
	return c.text(hex).Render(strings.Repeat("█", width))
}

// onSwatch draws text in a colour, for the preview.
func (c chrome) on(hex string) lipgloss.Style {
	if !color.IsHex(hex) {
		return c.base()
	}
	return c.text(hex)
}

// grade colours a contrast ratio the way the palette panel does.
func (c chrome) grade(ratio float64) string {
	switch {
	case ratio >= color.ContrastAA:
		return c.ok
	case ratio >= color.ContrastLow:
		return c.warn
	}
	return c.danger
}

// fill pads or truncates a rendered line to exactly width cells, on the
// chrome background.
func (c chrome) fill(line string, width int) string {
	if width <= 0 {
		return ""
	}
	w := lipgloss.Width(line)
	if w > width {
		return ansi.Truncate(line, width, "")
	}
	if w < width {
		return line + c.base().Render(strings.Repeat(" ", width-w))
	}
	return line
}

// truncate shortens a string to width cells with an ellipsis.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return ansi.Truncate(s, width, "")
	}
	return ansi.Truncate(s, width, "…")
}

// pad left-aligns s in a field of width cells, counting cells not bytes.
func pad(s string, width int) string {
	if w := lipgloss.Width(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

// padLeft right-aligns s in a field of width cells.
func padLeft(s string, width int) string {
	if w := lipgloss.Width(s); w < width {
		return strings.Repeat(" ", width-w) + s
	}
	return s
}

// box draws a rounded panel with the title set into the top border. The
// content is clipped and padded to the inner size, so a panel is always
// exactly width × height cells.
func (c chrome) box(title, hint string, lines []string, width, height int, focused bool) string {
	if width < 4 || height < 2 {
		return ""
	}
	borderColor := c.border
	titleColor := c.muted
	if focused {
		borderColor, titleColor = c.accent, c.accent
	}
	b := c.text(borderColor)
	inner := width - 2
	rows := make([]string, 0, height)

	top := "╭"
	if title != "" {
		label := " " + truncate(title, inner-4) + " "
		used := 2 + lipgloss.Width(label)
		right := ""
		if hint != "" && inner-used-lipgloss.Width(hint)-3 > 0 {
			right = c.mutedS().Render(" "+hint+" ") + b.Render("─")
		}
		fillW := inner + 2 - used - lipgloss.Width(right) - 1
		if fillW < 0 {
			fillW = 0
		}
		top = b.Render("╭─") + c.bold(titleColor).Render(label) + b.Render(strings.Repeat("─", fillW)) + right + b.Render("╮")
	} else {
		top = b.Render("╭" + strings.Repeat("─", inner) + "╮")
	}
	rows = append(rows, c.fill(top, width))

	for i := 0; i < height-2; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		rows = append(rows, b.Render("│")+c.fill(line, inner)+b.Render("│"))
	}
	rows = append(rows, b.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return strings.Join(rows, "\n")
}

// keyHint renders "key description" pairs for the status line.
func (c chrome) keyHint(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, c.text(c.fg).Render(pairs[i])+c.mutedS().Render(" "+pairs[i+1]))
	}
	return strings.Join(parts, c.faintS().Render("  ·  "))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// truncateExact cuts a rendered string to exactly width cells, no ellipsis.
func truncateExact(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "")
}

// dropLeft removes the first n cells of a rendered string.
func dropLeft(s string, n int) string {
	if n <= 0 {
		return s
	}
	w := lipgloss.Width(s)
	if n >= w {
		return ""
	}
	return ansi.TruncateLeft(s, n, "")
}

// placeX indents every line of a block by x cells on the chrome background.
func placeX(c chrome, block string, x int) string {
	if x <= 0 {
		return block
	}
	lines := strings.Split(block, "\n")
	pad := c.base().Render(strings.Repeat(" ", x))
	for i := range lines {
		lines[i] = pad + lines[i]
	}
	return strings.Join(lines, "\n")
}

// fillWith pads a rendered line to width cells with the given background.
func (c chrome) fillWith(line string, width int, bg string) string {
	w := lipgloss.Width(line)
	if w > width {
		return ansi.Truncate(line, width, "")
	}
	if w < width {
		return line + lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render(strings.Repeat(" ", width-w))
	}
	return line
}

// keyHintFit renders as many "key description" pairs as fit in width,
// dropping from the end rather than cutting one in half.
func (c chrome) keyHintFit(width int, pairs ...string) string {
	sep := c.faintS().Render("  ·  ")
	out, used := "", 0
	for i := 0; i+1 < len(pairs); i += 2 {
		part := c.text(c.fg).Render(pairs[i]) + c.mutedS().Render(" "+pairs[i+1])
		w := lipgloss.Width(part)
		if used > 0 {
			w += 5
		}
		if used+w > width {
			break
		}
		if used > 0 {
			out += sep
		}
		out += part
		used += w
	}
	return out
}

// keyHintFitSep is keyHintFit with a tighter separator, for narrow boxes.
func (c chrome) keyHintFitSep(width int, sep string, pairs ...string) string {
	out, used := "", 0
	for i := 0; i+1 < len(pairs); i += 2 {
		part := c.text(c.fg).Render(pairs[i]) + c.mutedS().Render(" "+pairs[i+1])
		w := lipgloss.Width(part)
		if used > 0 {
			w += lipgloss.Width(sep)
		}
		if used+w > width {
			break
		}
		if used > 0 {
			out += c.base().Render(sep)
		}
		out += part
		used += w
	}
	return out
}
