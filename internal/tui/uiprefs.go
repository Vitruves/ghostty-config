package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The editor's own look: its colours and, in the picture interface, its
// sizes. Each is a word kept in the state file, empty for the look the tool
// always had, and each is chosen the same way as the interface: walking the
// list shows every value at once, Enter keeps one, leaving puts back the one
// in effect.

// uiPref is one preference of the editor's look.
type uiPref struct {
	name, label, desc string
	values            []string // in the order the arrows step them
	def               string   // the value in effect when none is kept
	notes             map[string]string
	field             func(s *ghostty.State) *string
	// pixel marks what only the picture interface can honour: the text one
	// draws a fixed grid of cells and leaves it out of the menu.
	pixel bool
}

var (
	prefInterface = &uiPref{
		name: "interface", label: "Scheme", desc: "The colours of the editor itself, kept apart from the theme being looked at",
		values: interfaceNames, def: "auto", notes: interfaceNotes,
		field: func(s *ghostty.State) *string { return &s.Interface },
	}
	prefAccent = &uiPref{
		name: "accent", label: "Accent", desc: "The one colour that marks what is on, lifted until it reads on the scheme's ground",
		values: []string{"theme", "mono", "blue", "violet", "teal", "green", "amber", "rose"}, def: "theme",
		notes: map[string]string{
			"theme":  "the scheme's own; the theme's blue where the scheme follows the theme",
			"mono":   "no colour: the text's own, by weight alone",
			"blue":   "a clear, quiet blue",
			"violet": "a soft violet",
			"teal":   "a blue-green",
			"green":  "a leaf green",
			"amber":  "a warm yellow",
			"rose":   "a muted red",
		},
		field: func(s *ghostty.State) *string { return &s.Accent },
	}
	prefContrast = &uiPref{
		name: "uicontrast", label: "Contrast", desc: "How far quiet text, rules and highlights stand from the ground",
		values: []string{"normal", "high"}, def: "normal",
		notes: map[string]string{
			"normal": "quiet text and grounds kept soft",
			"high":   "quiet text at 7:1, firmer highlights and grounds",
		},
		field: func(s *ghostty.State) *string { return &s.Contrast },
	}
	prefSize = &uiPref{
		name: "uisize", label: "Size", desc: "How large the picture interface draws its text and spacing, against the terminal's cells",
		values: []string{"small", "normal", "large"}, def: "normal",
		notes: map[string]string{
			"small":  "more on screen",
			"normal": "the size the layout was drawn for",
			"large":  "easier to read from afar; fewer lines on screen",
		},
		field: func(s *ghostty.State) *string { return &s.UISize },
		pixel: true,
	}
	prefCards = &uiPref{
		name: "cards", label: "Theme cards", desc: "How wide a theme's card is on the wall: smaller cards, more of them in a row",
		values: []string{"small", "normal", "large"}, def: "normal",
		notes: map[string]string{
			"small":  "more themes in a row",
			"normal": "the cards as they always were",
			"large":  "fewer, larger cards",
		},
		field: func(s *ghostty.State) *string { return &s.Cards },
		pixel: true,
	}
)

// uiPrefs are the preferences of Tool → Interface, in the order it lists them.
// Another word of the look, a layout among several, is one more entry here
// with a field of its own in the state.
var uiPrefs = []*uiPref{prefInterface, prefAccent, prefContrast, prefSize, prefCards}

// uiPrefNamed is the preference a command word names, nil for none.
func uiPrefNamed(name string) *uiPref {
	for _, p := range uiPrefs {
		if p.name == name {
			return p
		}
	}
	return nil
}

// value is the preference in effect: the word kept when it is one of the
// values, the default otherwise.
func (p *uiPref) value(m *Model) string {
	v := *p.field(m.state)
	for _, x := range p.values {
		if x == v {
			return x
		}
	}
	return p.def
}

// The scale of the picture interface, against the 22 units of a row of cells.
var uiScales = map[string]float32{"small": 0.88, "normal": 1, "large": 1.15}

func (m *Model) uiScale() float32 { return uiScales[prefSize.value(m)] }

// The narrowest a card may be on the wall, in logical units.
var cardWidths = map[string]float32{"small": 128, "normal": 165, "large": 215}

func (m *Model) cardMinW() float32 { return cardWidths[prefCards.value(m)] }

// The accents of the tool's own, as a hue and a saturation: the lightness is
// chosen for the ground they land on.
var accentHues = map[string][2]float64{
	"blue": {217, 0.82}, "violet": {262, 0.62}, "teal": {174, 0.58},
	"green": {142, 0.50}, "amber": {38, 0.85}, "rose": {350, 0.66},
}

// tuneChrome lays the accent and the contrast chosen over a scheme's chrome.
// The scheme keeps its ground and its text: only what is derived from them
// moves.
func (m *Model) tuneChrome(c chrome) chrome {
	dark := color.IsDark(c.bg)
	high := prefContrast.value(m) == "high"
	switch a := prefAccent.value(m); a {
	case "theme":
	case "mono":
		c.accent = c.fg
		c.selBg = color.Blend(c.fg, c.bg, 0.84)
		c.selFg = c.fg
	default:
		h := accentHues[a]
		l := 0.68
		if !dark {
			l = 0.42
		}
		c.accent = color.EnsureContrast(color.HSLHex(h[0], h[1], l), c.bg, 4.5)
		c.selBg = color.Blend(c.accent, c.bg, 0.80)
		if !dark {
			c.selBg = color.Blend(c.accent, c.bg, 0.86)
		}
		c.selFg = c.fg
	}
	if high {
		c.high = true
		c.muted = color.EnsureContrast(color.Blend(c.fg, c.bg, 0.25), c.bg, 7)
		c.faint = color.Blend(c.fg, c.bg, 0.60)
		c.border = c.faint
		c.accent = color.EnsureContrast(c.accent, c.bg, 6)
		if c.accent != c.fg {
			c.selBg = color.Blend(c.accent, c.bg, 0.66)
			if color.Contrast(c.fg, c.selBg) < color.Contrast(c.bg, c.selBg) {
				c.selFg = c.bg
			}
		}
	}
	return c
}

// accentSample is the colour an accent would take on the scheme in effect,
// for the swatch beside its name.
func (m *Model) accentSample(name string) string {
	was := m.state.Accent
	m.state.Accent = name
	c := m.panelChrome()
	m.state.Accent = was
	return c.accent
}

// uiPrefCommands are the commands of Tool → Interface.
func uiPrefCommands() []*command {
	var out []*command
	for _, p := range uiPrefs {
		p := p
		c := &command{
			name: p.name, group: "Tool", label: p.label, syntax: p.name + " " + strings.Join(p.values, "|"), desc: p.desc,
			// A short scale steps from the menu with the arrows.
			adjust: len(p.values) <= 3,
			options: func(m *Model, arg string) []option {
				var out []option
				for _, v := range p.values {
					if arg != "" && !fuzzyMatch(v, strings.TrimSpace(arg)) {
						continue
					}
					o := option{label: v, detail: p.notes[v], value: v, current: v == p.value(m)}
					if p == prefAccent {
						o.swatch = m.accentSample(v)
					}
					out = append(out, o)
				}
				return out
			},
			onMove: func(m *Model, opt *option) tea.Cmd {
				if opt != nil {
					// Walking the list shows each value, and only Enter keeps
					// one: remember what was in effect to put it back.
					if m.previewing != p.name {
						m.prefWas, m.previewing = *p.field(m.state), p.name
					}
					*p.field(m.state) = opt.value
				}
				return nil
			},
			run: func(m *Model, arg string, opt *option) tea.Cmd {
				if opt == nil {
					return nil
				}
				if m.previewing == p.name {
					m.previewing = ""
				}
				*p.field(m.state) = opt.value
				_ = m.state.Save()
				m.info("%s: %s", p.label, opt.value)
				return nil
			},
			preview: previewOverview,
		}
		if p.pixel {
			c.hidden = func(m *Model) bool { return !m.pixelOn() }
		}
		out = append(out, c)
	}
	return out
}

// endPrefPreview puts back the value of the look that was in effect before
// its list was walked, when no Enter chose one, so that a look is never
// saved with the rest of the state.
func (m *Model) endPrefPreview() {
	if p := uiPrefNamed(m.previewing); p != nil {
		*p.field(m.state) = m.prefWas
	}
	m.previewing = ""
}
