package tui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
)

// The background image and the size and place a new window opens at. Both
// are drawn by the window picture beside the Window section: the image as
// Ghostty fits and places it, the rest as a window, since a size in cells
// only means something on a real screen.

// windowExtraSettings are the background image and the window at launch.
func (m *Model) windowExtraSettings(mac bool) []setting {
	var s []setting
	add := func(st setting) {
		st.group = "Window"
		s = append(s, st)
	}
	noImage := func(m *Model) bool { return m.tree.Value(imageFiles.key, "") == "" }
	withImage := func(st setting) {
		st.hidden = noImage
		add(st)
	}

	add(m.fileSetting(imageFiles))
	withImage(numberSetting("bgopacity", "background-image-opacity", "Image opacity", "Relative to the background opacity: below 1 the theme's background shows through the image, above 1 the image stays more opaque than a translucent window", 1, 0.05, 1.5, 0.05, func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }))
	fit := keySetting("bgfit", "background-image-fit", "Fit", "How the image is scaled to the terminal", []string{"contain", "cover", "stretch", "none"}, "contain")
	fit.explain = map[string]string{
		"contain": "the whole image, as large as it fits, its proportions kept",
		"cover":   "the image fills the terminal, its proportions kept; what overflows is cut off",
		"stretch": "the image fills the terminal exactly, stretched out of its proportions",
		"none":    "the image at its own size, not scaled at all",
	}
	withImage(fit)
	withImage(keySetting("bgposition", "background-image-position", "Position", "Where the image sits when it does not fill the terminal",
		[]string{"top-left", "top-center", "top-right", "center-left", "center", "center-right", "bottom-left", "bottom-center", "bottom-right"}, "center"))
	withImage(boolSetting("bgrepeat", "background-image-repeat", "Repeat", "Tile the image over the space it leaves free", false))

	// The size at launch: in cells, both or neither. Ghostty ignores one
	// without the other, so setting one fills in a missing partner.
	launch := func(cmd, key, label, detail string, lo, hi, step float64, partner, partnerValue, note string) setting {
		st := numberSetting(cmd, key, label, detail, 0, lo, hi, step, plain)
		st.unset, st.unsetAt, st.fallback = "default", lo, "default"
		st.syntax = cmd + " default|<" + plain(lo) + "–" + plain(hi) + ">"
		st.hint = note
		st.get = func(m *Model) string { return m.tree.Value(key, "default") }
		st.set = func(m *Model, v string) tea.Cmd {
			if v == "default" {
				m.tree.Unset(key, "disabled by ghostty-config: back to the default")
				return nil
			}
			m.tree.Set(key, v)
			if m.tree.Value(partner, "") == "" {
				m.tree.Set(partner, partnerValue)
			}
			return nil
		}
		st.writes = func(m *Model, v string) string {
			if v == "default" {
				return ""
			}
			line := key + " = " + v
			if m.tree.Value(partner, "") == "" {
				line += "\n" + partner + " = " + partnerValue
			}
			return line
		}
		return st
	}
	sizeNote := "Ghostty needs both columns and rows, so setting one fills in the other when it is missing. New windows only."
	cols := launch("columns", "window-width", "Columns", "How wide a new window opens, in cells; at least 10, and never wider than the screen", 10, 300, 10, "window-height", "24", sizeNote)
	cols.explain = map[string]string{"default": "the size Ghostty picks, or the last one when it restores windows"}
	add(cols)
	rows := launch("rows", "window-height", "Rows", "How tall a new window opens, in cells; at least 4, and never taller than the screen", 4, 100, 2, "window-width", "80", sizeNote)
	rows.explain = cols.explain
	add(rows)
	if mac {
		// GTK leaves windows to the compositor: only macOS places them.
		posNote := "Ghostty needs both, so setting one fills in the other with 0 when it is missing. New windows only."
		x := launch("windowx", "window-position-x", "From the left", "Where a new window opens, in pixels from the left of the main screen", 0, 3000, 20, "window-position-y", "0", posNote)
		x.explain = map[string]string{"default": "the window opens where macOS places it"}
		add(x)
		y := launch("windowy", "window-position-y", "From the top", "Where a new window opens, in pixels down from the menu bar of the main screen", 0, 2000, 20, "window-position-x", "0", posNote)
		y.explain = x.explain
		add(y)
	}
	return s
}
