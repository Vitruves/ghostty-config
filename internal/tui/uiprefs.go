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
// in effect. F4 lists them too, with the editor's behaviour after them.

// uiPref is one preference of the editor's own, of its look or its
// behaviour.
type uiPref struct {
	name, label, desc string
	values            []string // in the order the arrows step them
	def               string   // the value in effect when none is kept
	notes             map[string]string
	field             func(s *ghostty.State) *string
	// read and write stand for field where the state keeps the preference
	// otherwise than as a word; write also brings it into effect.
	read  func(s *ghostty.State) string
	write func(m *Model, v string)
	// later says the value is honoured from the next launch on.
	later bool
	// pixel marks what only the picture interface can honour: the text one
	// draws a fixed grid of cells and leaves it out of the menu.
	pixel bool
	// apply, when set, is how a value comes into effect: walking the list
	// then shows nothing, and only Enter changes anything. The layout takes
	// the whole screen, so it is never looked at from under its own menu.
	apply func(m *Model, v string) tea.Cmd
	// titles name the values on screen, when not by their word.
	titles map[string]string
}

var (
	prefLayout = &uiPref{
		name: "layout", label: "Layout", desc: "How the editor is laid out: the same settings underneath, shown one of three ways",
		values: []string{ghostty.ProfileAtelier, ghostty.ProfilePalette, ghostty.ProfileGuided}, def: ghostty.ProfileAtelier,
		field: func(s *ghostty.State) *string { return &s.Profile },
	}
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
var uiPrefs = []*uiPref{prefLayout, prefInterface, prefAccent, prefContrast, prefSize, prefCards}

// The editor's behaviour, which F4 lists after its look. Each is what a
// flag says for one launch, but for -no-reload, which Tool → Ghostty keeps as
// autoreload.
var (
	prefAutoReload = &uiPref{
		name: "autoreload", label: "Reload Ghostty after each change", desc: "Whether every change asks the running Ghostty to re-read its files",
		values: []string{"on", "off"}, def: "on",
		notes: map[string]string{
			"on":  "fonts, padding and the title bar follow at once",
			"off": "reload Ghostty yourself to see a change",
		},
		read: func(s *ghostty.State) string { return onOffWord(s.AutoReloadEnabled()) },
		write: func(m *Model, v string) {
			m.state.SetAutoReload(v == "on")
			m.reloadFailed = false
		},
	}
	prefUpdates = &uiPref{
		name: "updatecheck", label: "Check for updates", desc: "Whether GitHub is asked, as the editor starts and at most once a day, for a newer release",
		values: []string{"on", "off"}, def: "on",
		notes: map[string]string{
			"on":  "a newer release is offered when there is one",
			"off": "the network is never asked",
		},
		read:  func(s *ghostty.State) string { return onOffWord(s.UpdateCheckEnabled()) },
		write: func(m *Model, v string) { m.state.SetUpdateCheck(v == "on") },
		later: true,
	}
	prefDrawing = &uiPref{
		name: "drawing", label: "Drawing", desc: "How the editor is drawn: as one picture, from cells with pictures of the themes, or from text alone",
		values: []string{"auto", "images", "cells", "text"}, def: "auto",
		titles: map[string]string{"auto": "auto", "images": "pictures", "cells": "cells", "text": "text only"},
		notes: map[string]string{
			"auto":   "one picture where the terminal is known to show pictures, text elsewhere",
			"images": "one picture, even where this terminal is not known to show pictures",
			"cells":  "cells, with pictures only for the theme cards",
			"text":   "cells alone, no pictures at all",
		},
		field: func(s *ghostty.State) *string { return &s.Drawing },
		later: true,
	}
	prefGlyphs = &uiPref{
		name: "glyphs", label: "Simple glyphs", desc: "Only glyphs every terminal and font can draw: square ends where round ones would be",
		values: []string{"off", "on"}, def: "off",
		notes: map[string]string{
			"off": "the round ends of pills where the terminal draws them",
			"on":  "square ends, for a font that lacks the round ones",
		},
		read: func(s *ghostty.State) string { return onOffWord(s.Plain) },
		write: func(m *Model, v string) {
			m.state.Plain = v == "on"
			m.caps = detectTerminal(m.state.Plain)
		},
	}
)

// toolPrefs are the preferences of the editor's behaviour, in F4's order.
var toolPrefs = []*uiPref{prefAutoReload, prefUpdates, prefDrawing, prefGlyphs}

func onOffWord(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// prefNamed is the preference of either kind a name names, nil for none.
func prefNamed(name string) *uiPref {
	if p := uiPrefNamed(name); p != nil {
		return p
	}
	for _, p := range toolPrefs {
		if p.name == name {
			return p
		}
	}
	return nil
}

// raw is the preference as the state keeps it.
func (p *uiPref) raw(m *Model) string {
	if p.read != nil {
		return p.read(m.state)
	}
	return *p.field(m.state)
}

// put sets the preference, and brings it into effect.
func (p *uiPref) put(m *Model, v string) {
	if p.write != nil {
		p.write(m, v)
		return
	}
	*p.field(m.state) = v
}

// preview shows a value without keeping it: the value before is remembered
// the first time, to be put back unless keep follows.
func (p *uiPref) preview(m *Model, v string) {
	if _, ok := m.prefWas[p.name]; !ok {
		if m.prefWas == nil {
			m.prefWas = map[string]string{}
		}
		m.prefWas[p.name] = p.raw(m)
	}
	p.put(m, v)
}

// keep makes a value the one kept, and saves it.
func (p *uiPref) keep(m *Model, v string) {
	delete(m.prefWas, p.name)
	p.put(m, v)
	m.saveState()
}

// unkept is whether the preference shows a value that was not kept, and
// the one that was.
func (p *uiPref) unkept(m *Model) (was string, ok bool) {
	was, ok = m.prefWas[p.name]
	return was, ok && was != p.raw(m)
}

// saveState writes the state as it was kept: what is only being looked at
// is put back for the write, and shown again after it.
func (m *Model) saveState() {
	shown := map[string]string{}
	for name, was := range m.prefWas {
		if p := prefNamed(name); p != nil {
			shown[name] = p.raw(m)
			p.put(m, was)
		}
	}
	_ = m.state.Save()
	for name, v := range shown {
		prefNamed(name).put(m, v)
	}
}

// applyLayout puts the layout chosen in effect.
func applyLayout(m *Model, v string) tea.Cmd {
	if v == m.state.ActiveProfile() {
		return nil
	}
	// The prompt belongs to Atelier: the next profile starts from its own
	// first screen, and Atelier from its menu.
	m.setPromptText("")
	return m.switchProfile(v)
}

func init() {
	prefLayout.apply = applyLayout
	// The layouts are named and described where they are registered.
	prefLayout.titles, prefLayout.notes = map[string]string{}, map[string]string{}
	for _, k := range profileKinds {
		prefLayout.titles[k.name], prefLayout.notes[k.name] = k.title, k.blurb
	}
}

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
	v := p.raw(m)
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
					if t, ok := p.titles[v]; ok {
						o.label = t
					}
					if p == prefAccent {
						o.swatch = m.accentSample(v)
					}
					out = append(out, o)
				}
				return out
			},
			onMove: func(m *Model, opt *option) tea.Cmd {
				if opt != nil && p.apply == nil {
					// Walking the list shows each value, and only Enter keeps
					// one: what was in effect is put back otherwise.
					m.previewing = p.name
					p.preview(m, opt.value)
				}
				return nil
			},
			run: func(m *Model, arg string, opt *option) tea.Cmd {
				if opt == nil {
					return nil
				}
				if p.apply != nil {
					cmd := p.apply(m, opt.value)
					m.info("%s: %s", p.label, opt.label)
					return cmd
				}
				if m.previewing == p.name {
					m.previewing = ""
				}
				p.keep(m, opt.value)
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

// endPrefPreview puts back the value of every preference that was in effect
// before it was looked at, when no Enter chose one, so that a look is never
// saved with the rest of the state.
func (m *Model) endPrefPreview() {
	for name, was := range m.prefWas {
		if p := prefNamed(name); p != nil {
			p.put(m, was)
		}
	}
	m.prefWas, m.previewing = nil, ""
}
