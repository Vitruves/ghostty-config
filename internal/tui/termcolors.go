package tui

import (
	"fmt"
	stdcolor "image/color"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// termColours is what the terminal says it is drawing with. The interface
// takes its own colours from them, so it reads in a light terminal as well
// as in a dark one and follows the terminal when its theme changes.
type termColours struct {
	bg, fg string // "#rrggbb"; empty until the terminal has answered
}

// A terminal that knows DEC private mode 2031 says when it goes from light
// to dark or back; the colours are asked for again when it does.
const (
	watchScheme   = "\x1b[?2031h"
	unwatchScheme = "\x1b[?2031l"
)

// hexOf writes a colour the terminal reported the way the rest of the tool
// writes colours.
func hexOf(c stdcolor.Color) string {
	if c == nil {
		return ""
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// askTerminalColours asks for the background and the text colour.
func askTerminalColours() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, tea.RequestForegroundColor)
}

// Interfaces are the colour schemes the editor's own chrome can wear. The
// default, auto, has no colours of its own: it reads the terminal's and
// paints nothing behind the text. The others are kept for whoever wants the
// tool to look the same everywhere.
var interfaceNames = []string{"auto", "midnight", "graphite", "paper", "theme", "clear"}

var interfaceNotes = map[string]string{
	"auto":     "the terminal's own colours, light or dark, nothing painted behind the text",
	"midnight": "near-black, with vivid accents, whatever the theme",
	"graphite": "dark grey, whatever the theme",
	"paper":    "light, whatever the theme",
	"theme":    "follows the theme being shown",
	"clear":    "the colours of the theme being shown, on the terminal's own background",
}

// interfaceName is the scheme in effect.
func (m *Model) interfaceName() string {
	for _, n := range interfaceNames {
		if n == m.state.Interface {
			return n
		}
	}
	return "auto"
}

// autoChrome works the interface out from the terminal itself. The ground and
// the text are the terminal's; the accents are those of the theme the config
// names, which is what the terminal is running, lifted until they read on
// that ground. When the terminal has not answered, the configured theme
// stands in for it.
func (m *Model) autoChrome() chrome {
	var src *ghostty.Theme
	if t, ok := m.lib.Get(m.applied); ok {
		src = t
	}
	bg, fg := m.term.bg, m.term.fg
	if bg == "" {
		bg, fg = "#101014", "#d8d8e0"
		if src != nil {
			bg, fg = src.Background(), src.Foreground()
		}
	}
	dark := color.IsDark(bg)
	if fg == "" || color.Contrast(fg, bg) < 3 {
		fg = "#1d2125"
		if dark {
			fg = "#d8d8e0"
		}
	}
	// Bright variants read better on a dark ground, the normal ones on a
	// light one.
	pick := func(bright, normal int, fallback string) string {
		order := []int{bright, normal}
		if !dark {
			order = []int{normal, bright}
		}
		if src != nil {
			for _, i := range order {
				if v := src.Get(paletteKey(i)); color.IsHex(v) {
					return color.Normalize(v, fallback)
				}
			}
		}
		return fallback
	}
	c := buildChrome(bg, fg,
		pick(12, 4, "#7aa2f7"), pick(14, 6, "#7dcfff"), pick(11, 3, "#e0af68"), pick(9, 1, "#f7768e"), pick(10, 2, "#9ece6a"))
	c.clear = true
	c.muted = color.EnsureContrast(color.Blend(fg, bg, 0.42), bg, 4.5)
	c.faint = color.Blend(fg, bg, 0.80)
	c.selBg = color.Blend(c.accent, bg, 0.80)
	if !dark {
		c.selBg = color.Blend(c.accent, bg, 0.86)
	}
	c.selFg = fg
	c.border = c.faint
	return c
}

// panelChrome is the chrome of the editor itself.
func (m *Model) panelChrome() chrome {
	switch m.interfaceName() {
	case "auto":
		return m.autoChrome()
	case "midnight":
		p := buildChrome("#0a0a0f", "#d9dbe3", "#7aa2f7", "#6fd3e8", "#e6b450", "#f7768e", "#8fdc7b")
		p.faint = "#2c2c3a"
		p.muted = "#8a8da0"
		p.selBg = "#1f2540"
		p.selFg = "#f2f4fb"
		p.border = p.faint
		return p
	case "clear":
		// The colours are worked out against the theme's background, which
		// is what the terminal shows while the theme is previewed.
		p := m.c
		p.clear = true
		p.faint = color.Blend(p.fg, p.bg, 0.78)
		p.muted = color.EnsureContrast(color.Blend(p.fg, p.bg, 0.40), p.bg, 4.5)
		p.selBg = color.Blend(p.accent, p.bg, 0.78)
		p.selFg = p.fg
		p.border = p.faint
		return p
	case "paper":
		p := buildChrome("#f6f3ec", "#23201c", "#1f55c0", "#0f7b8a", "#8a5a00", "#b3261e", "#1f7a3a")
		p.faint = "#d9d3c5"
		p.muted = "#6b6357"
		p.selBg = "#dfe6f3"
		p.selFg = "#14233f"
		p.border = p.faint
		return p
	case "graphite":
		p := buildChrome("#1f2126", "#e9e7e3", "#8ab4f8", "#7fd1c7", "#f0c674", "#f28b82", "#8fd19e")
		p.faint = "#3a3d45"
		p.selBg = "#2f3644"
		p.selFg = p.fg
		p.border = p.faint
		return p
	}
	p := m.c
	p.faint = color.Blend(m.c.fg, p.bg, 0.78)
	p.muted = color.EnsureContrast(color.Blend(m.c.fg, p.bg, 0.40), p.bg, 4.5)
	p.selBg = color.Blend(m.c.accent, p.bg, 0.78)
	p.selFg = m.c.fg
	p.border = p.faint
	return p
}

// dialogChrome is the chrome of a dialog: the interface's own, on a ground
// lifted a little towards the text so the dialog reads as a surface laid over
// the screen without a line drawn around it.
func (m *Model) dialogChrome() chrome {
	p := m.panelChrome()
	p.clear = false
	p.bg = color.Blend(p.bg, p.fg, 0.07)
	p.muted = color.EnsureContrast(p.muted, p.bg, 4.5)
	p.faint = color.Blend(p.fg, p.bg, 0.78)
	p.selBg = color.Blend(p.accent, p.bg, 0.74)
	return p
}
