package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/vitruves/ghostty-config/internal/ghostty"
	"github.com/vitruves/ghostty-config/internal/pix"
)

// F3 opens Ghostty's own documentation of the highlighted line: the option a
// setting writes, or the action of a shortcut. The tool's labels say what a
// setting does in a line; this is the whole page, as the installed Ghostty
// has it, in a dialog that scrolls.

// explainDoc asks Ghostty for a page; a variable so tests never run a
// binary.
var explainDoc = ghostty.Explain

// explainCommands are the Ghostty options behind the commands that are not
// made from one setting.
var explainCommands = map[string]string{
	"theme": "theme", "favs": "theme", "font": "font-family", "style": "font-style",
	"size": "font-size", "ligatures": "font-feature", "edit": "palette",
}

// explainView is the open page.
type explainView struct {
	name    string
	action  bool
	loading bool
	blocks  []ghostty.DocBlock
	scroll  int
	// maxScroll is how far the page can scroll at the size it was last
	// drawn, so the keys stop where the page ends.
	maxScroll int
	page      int
}

type explainMsg struct {
	name   string
	action bool
	text   string
	err    error
}

// explainTarget is what the highlighted line is about in Ghostty's terms: an
// option, or an action when action is true. It is empty for lines Ghostty
// has no page on, the tool's own commands among them.
func (m *Model) explainTarget() (name string, action bool) {
	if m.editing {
		if strings.HasPrefix(m.editKey, "palette.") {
			return "palette", false
		}
		return m.editKey, false
	}
	of := func(cmd *command) string {
		if cmd == nil {
			return ""
		}
		if cmd.set != nil && cmd.set.key != "" {
			return cmd.set.key
		}
		return explainCommands[cmd.name]
	}
	if m.cmd != nil && m.cmd.name == "keys" {
		if r := m.selectedKey(); r != nil {
			return r.action, true
		}
		return "keybind", false
	}
	if m.cmd != nil {
		return of(m.cmd), false
	}
	if o := m.selected(); o != nil {
		return of(o.cmd), false
	}
	return "", false
}

// openExplain opens the page of the highlighted line and asks Ghostty for
// it in the background.
func (m *Model) openExplain() tea.Cmd {
	name, action := m.profile().explainTarget(m)
	if name == "" {
		m.info("Ghostty has no page on this line: F3 reads one for a setting or a shortcut's action")
		return nil
	}
	m.ex = explainView{name: name, action: action, loading: true}
	m.overlay = overlayExplain
	binary := m.paths.Binary
	return func() tea.Msg {
		text, err := explainDoc(binary, name, action)
		return explainMsg{name, action, text, err}
	}
}

// explainLoaded fills the page, if it is still the one open.
func (m *Model) explainLoaded(msg explainMsg) {
	if m.overlay != overlayExplain || m.ex.name != msg.name || m.ex.action != msg.action {
		return
	}
	m.ex.loading = false
	if msg.err != nil {
		m.ex.blocks = []ghostty.DocBlock{{Kind: ghostty.DocPara, Text: msg.err.Error() + "."}}
		return
	}
	m.ex.blocks = ghostty.ParseDoc(msg.text)
}

// updateExplain scrolls the page; Esc, Enter, q or F3 again close it.
func (m *Model) updateExplain(key string) (tea.Model, tea.Cmd) {
	page := maxInt(1, m.ex.page-1)
	switch key {
	case "esc", "enter", "q", "f3", "backspace":
		m.overlay = overlayNone
	case "up", "k", "ctrl+p":
		m.ex.scrollBy(-1)
	case "down", "j", "ctrl+n":
		m.ex.scrollBy(1)
	case "pgup", "b", "shift+up":
		m.ex.scrollBy(-page)
	case "pgdown", "space", " ", "shift+down":
		m.ex.scrollBy(page)
	case "home", "g":
		m.ex.scroll = 0
	case "end", "G":
		m.ex.scroll = m.ex.maxScroll
	}
	return m, nil
}

func (v *explainView) scrollBy(n int) {
	v.scroll = min(max(v.scroll+n, 0), v.maxScroll)
}

// explainTitle names the page: the option, or the action as the editor
// spells it with its name.
func (m *Model) explainTitle() string {
	if m.ex.action {
		return keyLabel(m.ex.name) + " · " + m.ex.name
	}
	return m.ex.name
}

// explainBlocks is the page, or what stands in for it while Ghostty answers.
func (m *Model) explainBlocks() []ghostty.DocBlock {
	if m.ex.loading {
		return []ghostty.DocBlock{{Kind: ghostty.DocPara, Text: "Asking Ghostty…"}}
	}
	return m.ex.blocks
}

// docLine is one line of the page as laid out at a width.
type docLine struct {
	kind   ghostty.DocKind
	indent int  // in spaces
	bullet bool // the first line of an item: its marker sits before indent
	spans  []ghostty.DocSpan
}

// layoutDoc wraps the blocks to width, in the units measure counts in, a
// cell or a point; a column of indent is unit wide. Code is kept line for
// line, and cut by whoever draws it.
func layoutDoc(blocks []ghostty.DocBlock, width, unit float64, measure func(s string, code bool) float64) []docLine {
	space := measure(" ", false)
	var out []docLine
	for i, b := range blocks {
		if b.Gap && i > 0 {
			out = append(out, docLine{})
		}
		ind := min(b.Indent, 8)
		if b.Kind == ghostty.DocCode {
			for _, l := range b.Lines {
				out = append(out, docLine{kind: b.Kind, indent: ind + 2, spans: []ghostty.DocSpan{{Text: l, Code: true}}})
			}
			continue
		}
		cur := docLine{kind: b.Kind, indent: ind}
		if b.Kind == ghostty.DocBullet {
			cur.indent, cur.bullet = ind+2, true
		}
		avail := width - float64(cur.indent)*unit
		used := 0.0
		for _, w := range docWords(ghostty.DocSpans(b.Text)) {
			ww := 0.0
			for _, s := range w {
				ww += measure(s.Text, s.Code)
			}
			if used > 0 && used+space+ww > avail {
				out = append(out, cur)
				cur = docLine{kind: b.Kind, indent: cur.indent}
				used = 0
			}
			if used > 0 {
				cur.spans = appendSpan(cur.spans, ghostty.DocSpan{Text: " "})
				used += space
			}
			for _, s := range w {
				cur.spans = appendSpan(cur.spans, s)
			}
			used += ww
		}
		out = append(out, cur)
	}
	return out
}

// docWords splits spans at their spaces into words, each a run of spans: a
// word can be part code, as in +-separated.
func docWords(spans []ghostty.DocSpan) [][]ghostty.DocSpan {
	var out [][]ghostty.DocSpan
	var word []ghostty.DocSpan
	flush := func() {
		if len(word) > 0 {
			out = append(out, word)
			word = nil
		}
	}
	for _, s := range spans {
		for i, part := range strings.Split(s.Text, " ") {
			if i > 0 {
				flush()
			}
			if part != "" {
				word = append(word, ghostty.DocSpan{Text: part, Code: s.Code})
			}
		}
	}
	flush()
	return out
}

func appendSpan(spans []ghostty.DocSpan, s ghostty.DocSpan) []ghostty.DocSpan {
	if n := len(spans); n > 0 && spans[n-1].Code == s.Code {
		spans[n-1].Text += s.Text
		return spans
	}
	return append(spans, s)
}

// explainScroll clamps the page to what fits, and returns where it starts.
func (m *Model) explainScroll(total, visible int) int {
	m.ex.page = visible
	m.ex.maxScroll = max(0, total-visible)
	m.ex.scroll = min(m.ex.scroll, m.ex.maxScroll)
	return m.ex.scroll
}

// explainHint says where the page is and how to move in it.
func (m *Model) explainHint(total, visible int) string {
	if total <= visible {
		return "Esc closes"
	}
	end := min(m.ex.scroll+visible, total)
	return fmt.Sprintf("%d–%d of %d · ↑↓ scroll · Esc closes", m.ex.scroll+1, end, total)
}

// viewExplain draws the page as a card over the screen, in cells.
func (m *Model) viewExplain(body string) string {
	c := m.dialogChrome()
	width := min(86, m.width-2)
	textW := width - 6
	lines := layoutDoc(m.explainBlocks(), float64(textW), 1, func(s string, _ bool) float64 { return float64(lipgloss.Width(s)) })
	visible := max(1, min(len(lines), m.height-7))
	from := m.explainScroll(len(lines), visible)
	var rows []string
	for _, l := range lines[from:min(from+visible, len(lines))] {
		row := strings.Repeat(" ", l.indent)
		if l.bullet {
			row = strings.Repeat(" ", l.indent-2) + c.mutedS().Render("•") + " "
		}
		row = c.base().Render("  ") + c.base().Render(row)
		for _, s := range l.spans {
			switch {
			case l.kind == ghostty.DocHeading:
				row += c.bold(c.fg).Render(s.Text)
			case s.Code:
				row += c.text(c.accent).Render(s.Text)
			default:
				row += c.base().Render(s.Text)
			}
		}
		rows = append(rows, truncate(row, width-2))
	}
	hint := m.explainHint(len(lines), visible)
	return overlayCentered(m, body, m.card(truncate(m.explainTitle(), max(8, width-lipgloss.Width(hint)-6)), hint, rows, width))
}

// pxExplain draws the page in the picture interface: prose in the body face,
// code in the monospace one, in the accent.
func (m *Model) pxExplain(c *pix.Canvas, p pxPal, f pxFonts, g pxGeo, card func(w, h float32) (float32, float32)) {
	const lineH = 21
	w := minF(780, g.W-40)
	textW := w - 56
	// Indents are counted in the monospace face's columns: a proportional
	// space is too narrow to set an item's text off its dot.
	unit := c.Width(f.mono, " ")
	lines := layoutDoc(m.explainBlocks(), float64(textW), float64(unit), func(s string, code bool) float64 {
		if code {
			return float64(c.Width(f.mono, s))
		}
		return float64(c.Width(f.body, s))
	})
	room := int((g.H - 40 - 72 - 40) / lineH)
	visible := max(1, min(len(lines), room))
	h := float32(72+visible*lineH) + 40
	x, y := card(w, h)
	from := m.explainScroll(len(lines), visible)
	hint := m.explainHint(len(lines), visible)
	hw := c.Width(f.small, hint)
	c.Text(f.h3, x+28, y+40, c.Fit(f.h3, m.explainTitle(), w-56-hw-16), p.fg)
	c.Text(f.small, x+w-28-hw, y+40, hint, p.muted)
	c.Clip(x+28, y+56, textW, h-56-16)
	defer c.Unclip()
	yy := y + 76
	for _, l := range lines[from:min(from+visible, len(lines))] {
		xx := x + 28 + float32(l.indent)*unit
		if l.bullet {
			pxDot(c, xx-unit*2+3, yy-4.5, p.muted)
		}
		for _, s := range l.spans {
			switch {
			case l.kind == ghostty.DocHeading:
				xx += c.Text(f.rowB, xx, yy, s.Text, p.fg)
			case s.Code:
				xx += c.Text(f.mono, xx, yy, s.Text, p.accent)
			default:
				xx += c.Text(f.body, xx, yy, s.Text, p.text2)
			}
		}
		yy += lineH
	}
}
