package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// previewModeNames name the two ways the preview can paint.
//
// Exact mode paints with the values being edited. Terminal mode uses the
// ANSI slots as the terminal resolves them, which after the live preview
// has landed should agree with exact mode; when it does not, something
// between this editor and the window disagrees, which is worth knowing.
var previewModeNames = []string{"exact colours", "as the terminal draws them"}

// previewPalette resolves the colours the preview paints with.
type previewPalette struct {
	fg, bg                                                string
	black, red, green, yellow, blue, magenta, cyan, white string
	brightBlack                                           string
	terminal                                              bool
}

func (m *Model) previewColors() previewPalette {
	if m.previewMode == 1 {
		return previewPalette{terminal: true,
			black: "0", red: "1", green: "2", yellow: "3", blue: "4", magenta: "5", cyan: "6", white: "7", brightBlack: "8"}
	}
	t := m.cur
	pick := func(i int, fallback string) string {
		return color.Normalize(t.Get(paletteKey(i)), fallback)
	}
	return previewPalette{
		fg: t.Foreground(), bg: t.Background(),
		black: pick(0, "#000000"), red: pick(1, "#ff0000"), green: pick(2, "#00ff00"), yellow: pick(3, "#ffff00"),
		blue: pick(4, "#0000ff"), magenta: pick(5, "#ff00ff"), cyan: pick(6, "#00ffff"), white: pick(7, "#ffffff"),
		brightBlack: pick(8, "#808080"),
	}
}

// paint returns a style for one preview colour. In terminal mode the ANSI
// index is used directly and the background is left to the terminal.
func (m *Model) paint(p previewPalette, v string) lipgloss.Style {
	if p.terminal {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(v))
	}
	return m.c.on(v)
}

// previewRows renders a slice of real terminal output painted in the theme,
// then grades the palette's readability.
func (m *Model) previewRows(width, height int) []string {
	if m.cur == nil {
		return nil
	}
	c := m.c
	p := m.previewColors()
	s := func(v string) lipgloss.Style { return m.paint(p, v) }
	plain := c.base()
	if p.terminal {
		plain = lipgloss.NewStyle()
	}
	var rows []string
	add := func(parts ...string) { rows = append(rows, strings.Join(parts, "")) }

	// The sixteen slots as two aligned rows.
	for _, group := range []struct {
		label string
		from  int
	}{{"normal", 0}, {"bright", 8}} {
		line := c.mutedS().Render(pad(group.label, 8))
		for i := 0; i < 8; i++ {
			line += c.swatch(m.cur.Get(paletteKey(group.from+i)), 3)
		}
		add(line)
	}
	rows = append(rows, "")

	// A prompt, split the way a shell highlighter splits it.
	add(s(p.green).Render("user"), plain.Render("@"), s(p.green).Render("host"), plain.Render(" "), s(p.blue).Render("~/projects"), plain.Render(" $ "), s(p.blue).Render("ls"), plain.Render(" "), s(p.cyan).Render("-la"))
	for _, row := range []struct{ mode, name, colour string }{
		{"drwxr-xr-x", "src/", p.blue},
		{"-rwxr-xr-x", "ghostty-config", p.green},
		{"-rw-r--r--", "README.md", p.fg},
		{"-rw-r--r--", ".gitignore", p.cyan},
		{"-rw-r--r--", "backup.tar.gz", p.magenta},
		{"lrwxrwxrwx", "broken -> missing", p.red},
	} {
		colour := row.colour
		if colour == "" {
			colour = p.fg
		}
		add(s(p.brightBlack).Render(row.mode), plain.Render(" "), s(p.yellow).Render("user staff"), plain.Render(" "), s(colour).Render(row.name))
	}
	rows = append(rows, "")

	add(plain.Render("On branch "), s(p.green).Render("main"))
	add(plain.Render("  "), s(p.green).Render("modified:   internal/tui/browser.go"))
	add(plain.Render("  "), s(p.red).Render("deleted:    internal/tui/legacy.go"))
	add(plain.Render("  "), s(p.yellow).Render("untracked:  notes.md"))
	rows = append(rows, "")

	add(s(p.magenta).Render("func"), plain.Render(" "), s(p.blue).Render("Render"), plain.Render("(n "), s(p.cyan).Render("int"), plain.Render(") "), s(p.cyan).Render("error"), plain.Render(" {"))
	add(plain.Render("    "), s(p.cyan).Render("fmt"), plain.Render("."), s(p.blue).Render("Println"), plain.Render("("), s(p.yellow).Render("\"hello\""), plain.Render(", "), s(p.magenta).Render("42"), plain.Render(")"))
	add(plain.Render("    "), s(p.brightBlack).Render("// a comment sits here"))
	add(plain.Render("    "), s(p.magenta).Render("return"), plain.Render(" "), s(p.red).Render("nil"))
	add(plain.Render("}"))
	rows = append(rows, "")

	// Readability: the part that turns "looks nice" into "usable at 2am".
	bg := m.cur.Background()
	fgRatio := color.Contrast(m.cur.Foreground(), bg)
	add(c.mutedS().Render("readability vs background"))
	add(plain.Render("  text     "), c.text(c.grade(fgRatio)).Render(fmt.Sprintf("%4.1f:1 %s", fgRatio, color.Grade(fgRatio))))
	worst, worstName := 21.0, ""
	for i := 0; i < 8; i++ {
		v := m.cur.Get(paletteKey(i))
		if v == "" {
			continue
		}
		if r := color.Contrast(v, bg); r < worst {
			worst, worstName = r, color.ANSI[i]
		}
	}
	if worstName != "" {
		add(plain.Render("  weakest  "), c.text(c.grade(worst)).Render(fmt.Sprintf("%4.1f:1 %s", worst, color.Grade(worst))), c.mutedS().Render("  "+worstName))
	}
	if opacity := m.tree.Value("background-opacity", "1"); opacity != "1" && opacity != "" {
		rows = append(rows, "")
		add(c.text(c.warn).Render("background-opacity " + opacity + ": the real background is"))
		add(c.text(c.warn).Render("blended with the desktop, so these ratios are a best case"))
	}
	if len(m.overrides) > 0 {
		rows = append(rows, "")
		add(c.text(c.warn).Render(fmt.Sprintf("%d colour line(s) in your config override the theme;", len(m.overrides))))
		add(c.text(c.warn).Render("applying a theme disables them (p → colour overrides)"))
	}

	if m.previewScrl > len(rows)-height {
		m.previewScrl = len(rows) - height
	}
	if m.previewScrl < 0 {
		m.previewScrl = 0
	}
	if m.previewScrl > 0 && m.previewScrl < len(rows) {
		rows = rows[m.previewScrl:]
	}
	for i := range rows {
		rows[i] = truncate(rows[i], width)
	}
	return rows
}

// themePreviewBlock renders a compact preview of an arbitrary theme in its
// own colours, used by the creator.
func (m *Model) themePreviewBlock(t *ghostty.Theme, width int) []string {
	c := m.c
	var rows []string
	add := func(parts ...string) { rows = append(rows, truncate(strings.Join(parts, ""), width)) }
	bg := t.Background()
	on := func(v string) lipgloss.Style { return c.on(color.Normalize(v, t.Foreground())) }
	fgRatio := color.Contrast(t.Foreground(), bg)
	add(c.bold(t.Foreground()).Render(t.Name), c.mutedS().Render("  "+styleWord(t)))
	rows = append(rows, "")
	add(c.swatch(bg, 3), c.base().Render(" background "), c.mutedS().Render(bg))
	add(c.swatch(t.Foreground(), 3), c.base().Render(" foreground "), c.mutedS().Render(t.Foreground()), c.text(c.grade(fgRatio)).Render(fmt.Sprintf("   %.1f:1 %s", fgRatio, color.Grade(fgRatio))))
	rows = append(rows, "")
	for _, group := range []struct {
		label string
		from  int
	}{{"normal", 0}, {"bright", 8}} {
		line := c.mutedS().Render(pad(group.label, 8))
		for i := 0; i < 8; i++ {
			line += c.swatch(t.Get(paletteKey(group.from+i)), 3)
		}
		add(line)
	}
	rows = append(rows, "")
	p := func(i int) string { return t.Get(paletteKey(i)) }
	add(on(p(2)).Render("user"), c.base().Render("@"), on(p(2)).Render("host"), c.base().Render(" "), on(p(4)).Render("~/code"), c.base().Render(" $ "), on(p(11)).Render("git status"))
	add(on(p(1)).Render("modified:"), c.base().Render("   main.go"))
	add(on(p(2)).Render("new file:"), c.base().Render("   palette.go"))
	rows = append(rows, "")
	add(on(p(5)).Render("func"), c.base().Render(" "), on(p(12)).Render("main"), c.base().Render("() {"))
	add(c.base().Render("    "), on(p(6)).Render("fmt"), c.base().Render("."), on(p(12)).Render("Println"), c.base().Render("("), on(p(3)).Render("\"hello\""), c.base().Render(")  "), on(p(8)).Render("// comment"))
	add(c.base().Render("}"))
	return rows
}
