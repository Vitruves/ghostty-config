package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The editor configures Ghostty but runs in whatever terminal it is started
// from. Nothing on screen depends on the terminal's own colours: every cell
// is painted, so a theme looks the same in Terminal.app as in Ghostty. What
// does vary is which glyphs a terminal can draw and how many colours it has,
// and both are detected rather than assumed.
type termCaps struct {
	// roundCaps is whether the half circles that end a pill can be drawn.
	// They live in the private use area, so a terminal either draws them
	// itself or needs a patched font; elsewhere a pill is a plain bar.
	roundCaps bool
	// inGhostty is whether this is a Ghostty window, the only place where
	// retinting the terminal itself and asking for a reload make sense.
	inGhostty bool
}

// detectTerminal reads the environment. GHOSTTY_CONFIG_GLYPHS=plain or
// =round overrides the guess about glyphs.
func detectTerminal(plain bool) termCaps {
	caps := termCaps{inGhostty: ghostty.InsideGhostty()}
	program := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	term := strings.ToLower(os.Getenv("TERM"))
	switch {
	case caps.inGhostty,
		program == "wezterm",
		program == "iterm.app",
		os.Getenv("KITTY_WINDOW_ID") != "",
		strings.Contains(term, "kitty"):
		caps.roundCaps = true
	}
	switch strings.ToLower(os.Getenv("GHOSTTY_CONFIG_GLYPHS")) {
	case "plain":
		caps.roundCaps = false
	case "round":
		caps.roundCaps = true
	}
	if plain {
		caps.roundCaps = false
	}
	return caps
}

// capL and capR are the ends of a pill: half circles where the terminal
// draws them, otherwise a cell of the pill's own colour.
func (m *Model) capL(fill, around string) string {
	if m.caps.roundCaps {
		return on(fill, around).Render("")
	}
	return on(fill, fill).Render(" ")
}

func (m *Model) capR(fill, around string) string {
	if m.caps.roundCaps {
		return on(fill, around).Render("")
	}
	return on(fill, fill).Render(" ")
}

// sgrOf returns the escape sequence a style opens with, or "" when the
// terminal has no colour at all.
func sgrOf(style lipgloss.Style) string {
	const marker = "\x00"
	rendered := style.Render(marker)
	i := strings.Index(rendered, marker)
	if i <= 0 {
		return ""
	}
	return rendered[:i]
}

// rebase makes a line carry a base style through every gap between styled
// runs: the base is asserted at the start and again after each reset, so a
// cell nobody styled takes the base colours instead of the terminal's own.
// It is what keeps the screen whole in a terminal whose background is not
// the theme's.
func rebase(line string, base lipgloss.Style) string {
	seq := sgrOf(base)
	if seq == "" {
		return line
	}
	const reset = "\x1b[0m"
	return seq + strings.ReplaceAll(line, reset, reset+seq) + reset
}

// InlineHeight is how many rows of a terminal h rows high the editor takes.
// It draws in the bottom rows and leaves the rest to what was on screen; a
// short terminal is given over whole, since the palette needs room.
func InlineHeight(h int) int {
	if h <= 28 {
		return h
	}
	return minInt(maxInt(28, h*2/3), 40)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
