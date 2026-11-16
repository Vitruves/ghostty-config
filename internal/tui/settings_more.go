package tui

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/color"
)

// The rest of Ghostty's options: the ones fewer people reach for, each read
// against Ghostty 1.4's own documentation and offered where it works. A key
// only one runtime implements is only offered there. All but the checksum
// keys are as old as 1.3, the oldest Ghostty the editor writes for; values
// 1.4 added are only offered to a Ghostty that reads them.

// textSetting is a setting whose value is typed: a title, a command, a
// colour. Its list offers what was typed, the word that leaves the key to
// Ghostty, the value in effect and a few suggestions. check says why a typed
// value cannot be used, or gives the value as it is written.
func textSetting(cmd, key, label, detail, unset, unsetNote string, suggest func(m *Model) []option, check func(m *Model, v string) (string, string)) setting {
	st := setting{
		cmd: cmd, group: "Window", label: label, syntax: cmd + " " + unset + "|<text>", detail: detail, key: key, fallback: unset,
		get: func(m *Model) string {
			if v := m.tree.Value(key, ""); v != "" {
				return v
			}
			return unset
		},
		set: func(m *Model, v string) tea.Cmd {
			if v == unset {
				m.tree.Unset(key, "disabled by ghostty-config: back to the default")
			} else {
				m.tree.Set(key, v)
			}
			return nil
		},
		writes: func(m *Model, v string) string {
			if v == unset {
				return ""
			}
			return key + " = " + v
		},
	}
	suggestions := func(m *Model) []option {
		if suggest == nil {
			return nil
		}
		return suggest(m)
	}
	st.list = func(m *Model, arg string) []option {
		now := st.get(m)
		needle := strings.TrimSpace(arg)
		known := suggestions(m)
		var out []option
		seen := map[string]bool{}
		add := func(o option) {
			if o.value != "" && seen[o.value] {
				return
			}
			seen[o.value] = true
			o.current = o.value == now
			out = append(out, o)
		}
		// What is typed comes first, so Enter writes it rather than a
		// suggestion it happens to resemble.
		if needle != "" && needle != unset {
			v, why := needle, ""
			if check != nil {
				v, why = check(m, needle)
			}
			if why != "" {
				out = append(out, option{label: needle, detail: why, header: true})
			} else {
				o := option{label: v, detail: "as typed", value: v}
				if color.IsHex(v) {
					o.swatch = color.Normalize(v, "")
				}
				add(o)
			}
		}
		if needle == "" || fuzzyMatch(unset, needle) {
			o := option{label: unset, detail: "the default", value: unset}
			if color.IsHex(unset) {
				o.swatch = unset
			}
			add(o)
		}
		if now != unset && (needle == "" || fuzzyMatch(now, needle)) {
			o := option{label: now, detail: "current", value: now}
			if color.IsHex(now) {
				o.swatch = color.Normalize(now, "")
			}
			for _, k := range known {
				if k.value == now {
					o = k
				}
			}
			add(o)
		}
		for _, k := range known {
			if needle == "" || fuzzyMatch(k.label, needle) {
				add(k)
			}
		}
		return out
	}
	st.describe = func(m *Model, v string) string {
		if v == unset {
			return unsetNote
		}
		for _, k := range suggestions(m) {
			if k.value == v && k.detail != "" {
				return k.detail
			}
		}
		return ""
	}
	return st
}

// themeColours are the colours of the theme on screen, to pick one from
// instead of typing it.
func themeColours(m *Model) []option {
	if m.cur == nil {
		return nil
	}
	out := []option{
		{label: m.cur.Foreground(), detail: "the theme's foreground", value: m.cur.Foreground(), swatch: m.cur.Foreground()},
		{label: m.cur.Background(), detail: "the theme's background", value: m.cur.Background(), swatch: m.cur.Background()},
	}
	for i, name := range []string{"red", "green", "yellow", "blue", "magenta", "cyan"} {
		if v := color.Normalize(m.cur.Get(paletteKey(i+1)), ""); v != "" {
			out = append(out, option{label: v, detail: "the theme's " + name, value: v, swatch: v})
		}
	}
	return out
}

// checkColour takes a typed colour as hex; Ghostty also reads X11 names,
// which only it can check.
func checkColour(m *Model, v string) (string, string) {
	if !color.IsHex(v) {
		return v, "type a colour as #rrggbb"
	}
	return color.Normalize(v, v), ""
}

// colourSetting is a setting whose value is a colour: one of the theme's,
// one of the words Ghostty takes in its place, or a hex that is typed.
func colourSetting(cmd, key, label, detail, unset, unsetNote string, words ...option) setting {
	st := textSetting(cmd, key, label, detail, unset, unsetNote, func(m *Model) []option {
		return append(append([]option{}, words...), themeColours(m)...)
	}, func(m *Model, v string) (string, string) {
		for _, w := range words {
			if w.value == v {
				return v, ""
			}
		}
		return checkColour(m, v)
	})
	st.syntax = cmd + " " + unset + "|<#rrggbb>"
	return st
}

// optionalNumber is a number Ghostty picks itself until the config names
// one: default, then a slider that rests at rest while it is left alone.
func optionalNumber(cmd, key, label, detail, unsetNote string, lo, hi, step, rest float64, format func(float64) string) setting {
	st := numberSetting(cmd, key, label, detail, 0, lo, hi, step, format)
	st.unset, st.unsetAt, st.fallback = "default", rest, "default"
	st.syntax = cmd + " default|<" + format(lo) + "–" + format(hi) + ">"
	st.explain = map[string]string{"default": unsetNote}
	st.get = func(m *Model) string { return m.tree.Value(key, "default") }
	st.set = func(m *Model, v string) tea.Cmd {
		if v == "default" {
			m.tree.Unset(key, "disabled by ghostty-config: back to the default")
		} else {
			m.tree.Set(key, v)
		}
		return nil
	}
	st.writes = func(m *Model, v string) string {
		if v == "default" {
			return ""
		}
		return key + " = " + v
	}
	return st
}

// scrollSpeed is one half of mouse-scroll-multiplier, which holds the speed
// of a wheel and of a trackpad in one line; a bare number there is both.
func scrollSpeed(cmd, device, label, detail string, fallback, lo, hi, step float64) setting {
	const key = "mouse-scroll-multiplier"
	read := func(m *Model) map[string]string {
		speeds := map[string]string{"precision": "1", "discrete": "3"}
		for _, part := range strings.Split(m.tree.Value(key, ""), ",") {
			name, v, named := strings.Cut(strings.TrimSpace(part), ":")
			switch {
			case named && speeds[name] != "":
				speeds[name] = strings.TrimSpace(v)
			case !named && name != "":
				speeds["precision"], speeds["discrete"] = name, name
			}
		}
		return speeds
	}
	line := func(m *Model, v string) string {
		speeds := read(m)
		speeds[device] = v
		p, _ := parseNumber(speeds["precision"])
		d, _ := parseNumber(speeds["discrete"])
		if p == 1 && d == 3 {
			return ""
		}
		return "precision:" + speeds["precision"] + ",discrete:" + speeds["discrete"]
	}
	st := numberSetting(cmd, key, label, detail, fallback, lo, hi, step, plain)
	st.get = func(m *Model) string { return read(m)[device] }
	st.set = func(m *Model, v string) tea.Cmd {
		if next := line(m, v); next == "" {
			m.tree.Unset(key, "disabled by ghostty-config: back to the default")
		} else {
			m.tree.Set(key, next)
		}
		return nil
	}
	st.writes = func(m *Model, v string) string {
		if next := line(m, v); next != "" {
			return key + " = " + next
		}
		return ""
	}
	return st
}

// iconFiles are the pictures macos-custom-icon takes.
var iconFiles = fileKind{
	cmd: "customicon", key: "macos-custom-icon", label: "Icon file",
	detail: "The PNG, JPEG or ICNS file the custom icon is read from; type a path to take one from anywhere",
	none:   "no file named: Ghostty looks for ~/.config/ghostty/Ghostty.icns",
	exts:   []string{".icns", ".png", ".jpg", ".jpeg"},
	places: func(m *Model) []place {
		home := homeDir()
		return []place{{m.paths.ConfigDir, 2}, {filepath.Join(home, "Pictures"), 2}, {filepath.Join(home, "Downloads"), 1}}
	},
}

// shells are the login shells this machine lists, for the command a
// terminal starts.
func shells(m *Model) []option {
	data, err := os.ReadFile("/etc/shells")
	if err != nil {
		return nil
	}
	var out []option
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "/") {
			continue
		}
		if _, err := os.Stat(l); err == nil {
			out = append(out, option{label: l, detail: "a shell listed in /etc/shells", value: l})
		}
	}
	return out
}

// checkDirectory takes a typed working directory: one of Ghostty's two
// words, or a directory that is there, written in full or from ~/.
func checkDirectory(m *Model, v string) (string, string) {
	if v == "home" || v == "inherit" {
		return v, ""
	}
	if !strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "~/") {
		return v, "a full path, or one from ~/"
	}
	p := v
	if rest, ok := strings.CutPrefix(v, "~/"); ok {
		p = filepath.Join(homeDir(), rest)
	}
	if st, err := os.Stat(p); err != nil || !st.IsDir() {
		return v, "no such directory"
	}
	return v, ""
}

// familyOptions are the font families on this machine, the monospaced ones
// alone or all of them.
func familyOptions(mono bool) func(m *Model) []option {
	return func(m *Model) []option {
		var out []option
		for _, f := range m.families {
			if mono && !f.Mono {
				continue
			}
			detail := "proportional"
			if f.Mono {
				detail = "monospace"
			}
			out = append(out, option{label: f.Name, detail: detail, value: f.Name})
		}
		return out
	}
}

// moreSettings are the options beyond the everyday ones, section by section.
func (m *Model) moreSettings(mac, v14 bool) []setting {
	var s []setting
	in := func(group string) func(st setting) {
		return func(st setting) {
			st.group = group
			// A hundred more words would push the last sections off the
			// help, which leaves these to their section as it does flags.
			st.flag = true
			s = append(s, st)
		}
	}
	fonts, window, input, session := in("Fonts"), in("Window"), in("Input"), in("Session")
	// valueIs is whether a key reads as a word, def being what it reads as
	// when the config leaves it out.
	valueIs := func(key, def, word string) func(m *Model) bool {
		return func(m *Model) bool { return strings.ToLower(m.tree.Value(key, def)) == word }
	}
	newOnly := "new windows only"
	newTerminals := "new terminals only"
	restart := "needs a Ghostty restart"

	// --- Fonts ---------------------------------------------------------------
	fonts(flagSetting("shapingbreak", "font-shaping-break", "cursor", "Split ligatures at the cursor", "A ligature under the cursor is drawn as its separate characters, so each can be edited", "cursor"))

	face := func(cmd, key, label, what string) {
		st := textSetting(cmd, key, label, "The family "+what+" text is drawn with, when it should not be the main one's own "+what, "default", "the "+what+" of the main family, or one Ghostty makes up from its regular", familyOptions(true), nil)
		st.syntax = cmd + " default|<family>"
		st.hint = "Type a name to take a family that is not listed."
		fonts(st)
	}
	face("boldfont", "font-family-bold", "Bold font", "bold")
	face("italicfont", "font-family-italic", "Italic font", "italic")
	face("bolditalicfont", "font-family-bold-italic", "Bold italic font", "bold italic")
	styles := func(m *Model) []option {
		out := []option{{label: "false", detail: "never drawn: such text falls back to the regular style", value: "false"}}
		if f, ok := m.findFamily(m.fontFamily); ok {
			for _, name := range f.Styles {
				out = append(out, option{label: name, detail: "a style of " + f.Name, value: name})
			}
		}
		return out
	}
	faceStyle := func(cmd, key, label, what string) {
		st := textSetting(cmd, key, label, "The named style "+what+" text is drawn with, Heavy or Oblique for one", "default", "the style the font itself calls "+what, styles, nil)
		st.syntax = cmd + " default|false|<style>"
		fonts(st)
	}
	faceStyle("boldstyle", "font-style-bold", "Bold style", "bold")
	faceStyle("italicstyle", "font-style-italic", "Italic style", "italic")
	faceStyle("bolditalicstyle", "font-style-bold-italic", "Bold italic style", "bold italic")
	const synthetic = "bold,italic,bold-italic"
	fonts(flagSetting("fakebold", "font-synthetic-style", "bold", "Synthetic bold", "A font with no bold face gets one made by thickening its regular; off draws such text regular", synthetic))
	fonts(flagSetting("fakeitalic", "font-synthetic-style", "italic", "Synthetic italic", "A font with no italic face gets one made by slanting its regular; off draws such text upright", synthetic))
	fonts(flagSetting("fakebolditalic", "font-synthetic-style", "bold-italic", "Synthetic bold italic", "A font with no bold italic face gets one made up; it is switched apart from bold and from italic", synthetic))
	fonts(colourSetting("boldcolor", "bold-color", "Bold text colour", "A colour of its own for bold text", "default", "bold text keeps the colour it would have as regular",
		option{label: "bright", detail: "bold text takes the bright variant of its colour: bold red is bright red", value: "bright"}))

	metric := func(cmd, key, label, detail string, lo, hi, step float64, format func(float64) string) setting {
		return optionalNumber(cmd, key, label, detail, "the font's own measure", lo, hi, step, 0, format)
	}
	fonts(metric("iconheight", "adjust-icon-height", "Icon height", "Percent added to the tallest a Nerd Font icon is drawn; Powerline symbols keep to the cell", -50, 50, 5, percent))
	fonts(metric("baseline", "adjust-font-baseline", "Baseline", "Pixels the text is raised in its cell; negative lowers it", -10, 10, 1, plain))
	fonts(metric("underlinepos", "adjust-underline-position", "Underline position", "Pixels the underline is moved down; negative moves it up", -10, 10, 1, plain))
	fonts(metric("underlinethick", "adjust-underline-thickness", "Underline thickness", "Pixels added to the underline; negative thins it", -3, 8, 1, plain))
	fonts(metric("strikepos", "adjust-strikethrough-position", "Strikethrough position", "Pixels the strikethrough is moved down; negative moves it up", -10, 10, 1, plain))
	fonts(metric("strikethick", "adjust-strikethrough-thickness", "Strikethrough thickness", "Pixels added to the strikethrough; negative thins it", -3, 8, 1, plain))
	fonts(metric("overlinepos", "adjust-overline-position", "Overline position", "Pixels the overline is moved down; negative moves it up", -10, 10, 1, plain))
	fonts(metric("overlinethick", "adjust-overline-thickness", "Overline thickness", "Pixels added to the overline; negative thins it", -3, 8, 1, plain))
	fonts(metric("boxthick", "adjust-box-thickness", "Box line thickness", "Pixels added to the lines of box-drawing characters, the frames of a TUI; negative thins them", -3, 8, 1, plain))

	// --- Window --------------------------------------------------------------
	anim := keySetting("shaderanimation", "custom-shader-animation", "Shader animation", "When a custom shader keeps redrawing, which is what lets it move", []string{"true", "false", "always"}, "true")
	anim.explain = map[string]string{
		"true":   "the shader animates in the terminal that has the focus (the default)",
		"false":  "the shader is only redrawn when the terminal changes: less work, no movement",
		"always": "the shader animates in every terminal, focused or not, which can cost a lot",
	}
	anim.hidden = func(m *Model) bool { return m.tree.Value("custom-shader", "") == "" }
	window(anim)

	if mac {
		wb := keySetting("windowbuttons", "macos-window-buttons", "Window buttons", "The close, minimise and zoom buttons at the top left", []string{"visible", "hidden"}, "visible")
		wb.explain = map[string]string{
			"visible": "the three buttons are shown (the default)",
			"hidden":  "no buttons; a hidden title bar or no decorations never has them anyway",
		}
		wb.note = newOnly
		window(wb)
		pi := keySetting("proxyicon", "macos-titlebar-proxy-icon", "Folder icon in the title", "The small icon of the working directory beside the title, as in Terminal.app; native title bar only", []string{"visible", "hidden"}, "visible")
		pi.explain = map[string]string{
			"visible": "the folder's icon is shown, and can be dragged (the default)",
			"hidden":  "no icon; an open window follows after its next change of directory",
		}
		window(pi)
	} else {
		ts := keySetting("titlestyle", "gtk-titlebar-style", "Title bar style", "Whether the tabs sit in the title bar or under it", []string{"native", "tabs"}, "native")
		ts.explain = map[string]string{
			"native": "a title bar with the title and buttons; the tab bar shows under it (the default)",
			"tabs":   "tabs and title bar merged, which saves a row; the window no longer drags by its title",
		}
		window(ts)
	}
	fixed := textSetting("title", "title", "Window title", "A title every window keeps, whatever programs ask for", "default", "the shell and the programs in it set the title", nil, nil)
	fixed.hint = "For a blank title, type a space between double quotes."
	window(fixed)
	if !mac {
		sub := keySetting("subtitle", "window-subtitle", "Subtitle", "A second line under the window title", []string{"false", "working-directory"}, "false")
		sub.explain = map[string]string{
			"false":             "no subtitle (the default)",
			"working-directory": "the directory the focused terminal is in",
		}
		window(sub)
	}
	tf := textSetting("titlefont", "window-title-font-family", "Title font", "The family window and tab titles are drawn with; it need not be monospaced", "default", "the system's own font", familyOptions(false), nil)
	tf.syntax = "titlefont default|<family>"
	window(tf)
	if !mac {
		ghosttyTheme := "Only used while the window chrome theme is ghostty."
		bg := colourSetting("titlebarbg", "window-titlebar-background", "Title bar background", "The colour of the title bar", "default", "the terminal's background")
		bg.hint = ghosttyTheme
		window(bg)
		fg := colourSetting("titlebarfg", "window-titlebar-foreground", "Title bar text", "The colour of the title and the buttons", "default", "the terminal's foreground")
		fg.hint = ghosttyTheme
		window(fg)
		window(boolSetting("hidetitlemax", "gtk-titlebar-hide-when-maximized", "Hide when maximised", "The title bar goes away while the window is maximised and comes back after", false))
		tb := keySetting("toolbarstyle", "gtk-toolbar-style", "Bar style", "How the title and tab bars stand out from the terminal", []string{"flat", "raised", "raised-border"}, "raised")
		tb.explain = map[string]string{
			"flat":          "the bars are level with the terminal",
			"raised":        "the bars cast a shadow on the terminal (the default)",
			"raised-border": "a thin border in place of the shadow",
		}
		window(tb)
	}

	window(boolSetting("maximize", "maximize", "Start maximised", "New windows open as large as the screen allows", false))
	if mac {
		fs := keySetting("fullscreen", "fullscreen", "Start in full screen", "New windows open in full screen", []string{"false", "true", "non-native", "non-native-visible-menu", "non-native-padded-notch"}, "false")
		fs.explain = map[string]string{
			"false":                   "new windows open as windows (the default)",
			"true":                    "in macOS's own full screen, a Space of its own; needs window decorations",
			"non-native":              "full screen at once, without the animation, menu bar hidden; tabs do not work",
			"non-native-visible-menu": "full screen at once with the menu bar kept; tabs do not work",
			"non-native-padded-notch": "full screen at once, menu bar hidden, the notch left clear; tabs do not work",
		}
		fs.note = newOnly
		window(fs)
		nn := keySetting("fullscreenstyle", "macos-non-native-fullscreen", "Full screen style", "What going full screen does: macOS's own way, or at once on the same Space", []string{"false", "true", "visible-menu", "padded-notch"}, "false")
		nn.explain = map[string]string{
			"false":        "macOS's own full screen, animated, in a Space of its own (the default)",
			"true":         "at once, without the animation, menu bar hidden; tabs do not work in it",
			"visible-menu": "at once with the menu bar kept; tabs do not work in it",
			"padded-notch": "at once, menu bar hidden, the notch left clear; tabs do not work in it",
		}
		nn.note = "from the next time a window goes full screen"
		window(nn)
	} else {
		fs := boolSetting("fullscreen", "fullscreen", "Start in full screen", "New windows open in full screen", false)
		fs.note = newOnly
		window(fs)
	}

	nt := keySetting("newtab", "window-new-tab-position", "New tabs open", "Where a new tab goes among the others", []string{"current", "end"}, "current")
	nt.explain = map[string]string{
		"current": "right after the tab in use (the default)",
		"end":     "after the last tab",
	}
	window(nt)
	if !mac {
		places := []string{"top", "bottom"}
		if v14 {
			places = append(places, "hidden")
		}
		tl := keySetting("tabslocation", "gtk-tabs-location", "Tab bar side", "The edge of the window the tab bar keeps to", places, "top")
		tl.explain = map[string]string{
			"top":    "under the title bar (the default)",
			"bottom": "along the bottom of the window",
			"hidden": "no tab bar: a button in the title bar counts the tabs and opens their overview",
		}
		window(tl)
		window(boolSetting("widetabs", "gtk-wide-tabs", "Wide tabs", "Tabs share the whole bar, the GNOME way; off makes each as wide as its title", true))
	}

	window(colourSetting("splitfill", "unfocused-split-fill", "Unfocused split tint", "The colour laid over splits you are not typing in; the opacity above says how much of it", "default", "the theme's background"))
	window(colourSetting("dividercolor", "split-divider-color", "Divider colour", "The colour of the line between splits", "default", "a colour Ghostty picks from the theme"))
	window(flagSetting("splitzoom", "split-preserve-zoom", "navigation", "Stay zoomed between splits", "Moving to another split while one is zoomed zooms that one, instead of showing them all again", "no-navigation"))

	pg := boolSetting("palettegen", "palette-generate", "Derive the 256 colours", "Colours 16 to 255 are worked out from the theme's sixteen, instead of xterm's fixed ones; a theme's own are kept", false)
	pg.hint = "Programs that count on xterm's 256 colours may look odd."
	window(pg)
	ph := boolSetting("paletteharmonious", "palette-harmonious", "Reverse derived colours", "The derived colours run from light to dark, so programs that use them read well on a light theme too", false)
	ph.hidden = valueIs("palette-generate", "false", "false")
	window(ph)
	cellWords := []option{
		{label: "cell-foreground", detail: "the text colour of each cell it covers", value: "cell-foreground"},
		{label: "cell-background", detail: "the background colour of each cell it covers", value: "cell-background"},
	}
	window(colourSetting("searchbg", "search-background", "Search match", "The background of every match of a search but the one you are on", "#ffe082", "golden yellow, Ghostty's own", cellWords...))
	window(colourSetting("searchfg", "search-foreground", "Search match text", "The text of every match of a search but the one you are on", "#000000", "black, Ghostty's own", cellWords...))
	window(colourSetting("searchselbg", "search-selected-background", "Current match", "The background of the match a search is on", "#f2a57e", "soft peach, Ghostty's own", cellWords...))
	window(colourSetting("searchselfg", "search-selected-foreground", "Current match text", "The text of the match a search is on", "#000000", "black, Ghostty's own", cellWords...))

	// --- Input ---------------------------------------------------------------
	input(optionalNumber("cursorthick", "adjust-cursor-thickness", "Cursor thickness", "Pixels added to the bar cursor and to the outline of the hollow block; negative thins them", "the thickness the font gives", -3, 8, 1, 0, plain))
	input(optionalNumber("cursorheight", "adjust-cursor-height", "Cursor height", "Percent added to the height of the cursor, whatever its shape; negative shortens it", "as tall as the cell", -50, 50, 5, 0, percent))

	rc := keySetting("rightclick", "right-click-action", "Right click", "What a right click on the terminal does", []string{"context-menu", "paste", "copy", "copy-or-paste", "ignore"}, "context-menu")
	rc.explain = map[string]string{
		"context-menu":  "open the menu: copy, paste, split and the rest (the default)",
		"paste":         "paste the clipboard",
		"copy":          "copy the selection",
		"copy-or-paste": "copy the selection when there is one, paste otherwise",
		"ignore":        "nothing: the click is ignored",
	}
	input(rc)
	input(scrollSpeed("scrollwheel", "discrete", "Scroll speed, wheel", "How far one notch of a mouse wheel scrolls, as a multiple", 3, 0.5, 10, 0.5))
	input(scrollSpeed("scrolltrackpad", "precision", "Scroll speed, trackpad", "How far a trackpad or a smooth wheel scrolls, as a multiple", 1, 0.25, 5, 0.25))
	ci := optionalNumber("clickinterval", "click-repeat-interval", "Double-click time", "Milliseconds within which a second click makes a double click", map[bool]string{true: "the time set in the macOS settings", false: "500 milliseconds"}[mac], 100, 1000, 50, 500, plain)
	ci.get = func(m *Model) string {
		// Ghostty writes its default as 0, which is no time at all.
		if v := m.tree.Value("click-repeat-interval", ""); v != "" && v != "0" {
			return v
		}
		return "default"
	}
	input(ci)
	mr := boolSetting("mousereporting", "mouse-reporting", "Programs get the mouse", "Programs that ask for mouse events receive them; off keeps the mouse for selecting, in vim and tmux too", true)
	mr.hint = "The toggle_mouse_reporting action switches it from the keyboard."
	input(mr)
	sc := keySetting("shiftcapture", "mouse-shift-capture", "Shift-click", "Whether a program that takes the mouse also gets clicks made with Shift, which otherwise extend the selection", []string{"false", "true", "never", "always"}, "false")
	sc.explain = map[string]string{
		"false":  "Shift-click selects, unless the program asks for it (the default)",
		"true":   "Shift-click goes to the program, unless it gives it up",
		"never":  "Shift-click always selects, whatever the program asks",
		"always": "Shift-click always goes to the program",
	}
	input(sc)

	input(boolSetting("clearoncopy", "selection-clear-on-copy", "Clear selection on copy", "Copying with the copy key drops the selection; copy on select never does", false))
	wc := textSetting("wordchars", "selection-word-chars", "Word boundaries", "The characters a double click stops at when it selects a word", "default", "spaces, tabs, quotes, brackets and | : ; , $", func(m *Model) []option {
		return []option{
			{label: `" \t"`, detail: "spaces and tabs only: a double click takes a whole path, URL or quoted string", value: `" \t"`},
			{label: `" \t'\"│` + "`" + `|:;,()[]{}<>$/"`, detail: "the default and the slash: a double click takes one part of a path", value: `" \t'\"│` + "`" + `|:;,()[]{}<>$/"`},
		}
	}, nil)
	wc.syntax = `wordchars default|"<characters>"`
	wc.hint = "Type the characters between double quotes; \\t is a tab."
	input(wc)
	input(boolSetting("bracketedsafe", "clipboard-paste-bracketed-safe", "Trust bracketed paste", "A paste into a program that asked for bracketed paste is taken as safe and never questioned", true))
	lp := keySetting("linkpreviews", "link-previews", "Link previews", "The address shown at the bottom of the window while the pointer is on a link", []string{"true", "osc8", "false"}, "true")
	lp.explain = map[string]string{
		"true":  "shown for every link (the default)",
		"osc8":  "only for links a program made, whose text may differ from their address",
		"false": "never shown",
	}
	input(lp)

	// --- Session -------------------------------------------------------------
	session(boolSetting("tabinheritcwd", "tab-inherit-working-directory", "New tabs keep the directory", "A new tab opens in the directory of the tab in use", true))
	session(boolSetting("splitinheritcwd", "split-inherit-working-directory", "New splits keep the directory", "A new split opens in the directory of the split in use", true))
	session(boolSetting("inheritfontsize", "window-inherit-font-size", "Keep the zoomed font size", "New windows and tabs open at the font size of the one in use, zoomed or not, instead of the configured size", true))
	if !mac {
		qd := keySetting("quitdelay", "quit-after-last-window-closed-delay", "Quit delay", "How long Ghostty stays running after its last window closes", []string{"default", "5s", "1m", "10m", "1h"}, "default")
		qd.explain = map[string]string{"default": "it quits at once"}
		qd.hidden = valueIs("quit-after-last-window-closed", "false", "false")
		session(qd)
	} else {
		ut := keySetting("undotimeout", "undo-timeout", "Undo closing for", "How long a closed window, tab or split can be brought back with undo", []string{"0s", "5s", "10s", "30s", "1m"}, "5s")
		ut.explain = map[string]string{
			"0s": "closing cannot be undone",
			"5s": "five seconds (the default); the programs inside keep running that long",
		}
		session(ut)
	}
	overlayOff := valueIs("resize-overlay", "after-first", "never")
	rp := keySetting("resizeposition", "resize-overlay-position", "Overlay position", "Where the size popup appears in the window",
		[]string{"center", "top-left", "top-center", "top-right", "bottom-left", "bottom-center", "bottom-right"}, "center")
	rp.hidden = overlayOff
	session(rp)
	rd := keySetting("resizeduration", "resize-overlay-duration", "Overlay time", "How long the size popup stays after the resizing stops", []string{"250ms", "750ms", "1500ms", "3s"}, "750ms")
	rd.hidden = overlayOff
	session(rd)

	command := textSetting("command", "command", "Command", "What every new terminal runs, usually a shell; arguments may follow", "default", "your login shell, from SHELL or the user database", shells, nil)
	command.syntax = "command default|<command>"
	command.hint = "With arguments it runs through /bin/sh -c; start it with direct: to skip that."
	command.note = newTerminals
	session(command)
	first := textSetting("initialcommand", "initial-command", "First command", "What the first terminal runs when Ghostty starts; the others run the command above", "default", "the same as every other terminal", shells, nil)
	first.syntax = "initialcommand default|<command>"
	session(first)
	wd := textSetting("workdir", "working-directory", "Working directory", "Where the first terminal starts; the next ones inherit theirs unless that is switched off", "default", "the home directory from the Dock or a launcher, the directory Ghostty was started in from a shell", func(m *Model) []option {
		return []option{
			{label: "home", detail: "always the home directory", value: "home"},
			{label: "inherit", detail: "the directory of the process that started Ghostty", value: "inherit"},
		}
	}, checkDirectory)
	wd.syntax = "workdir default|home|inherit|<path>"
	session(wd)
	session(boolSetting("initialwindow", "initial-window", "Open a window at launch", "Ghostty opens a window when it starts; off starts it with none, for the quick terminal alone", true))
	wait := boolSetting("waitafter", "wait-after-command", "Keep open after exit", "A terminal whose command has ended stays until a key is pressed, so its last output can be read", false)
	wait.note = newTerminals
	session(wait)

	const scrollTo = "keystroke,no-output"
	session(flagSetting("scrollonkey", "scroll-to-bottom", "keystroke", "Jump to the bottom on typing", "Typing while scrolled back returns to the last line", scrollTo))
	session(flagSetting("scrollonoutput", "scroll-to-bottom", "output", "Jump to the bottom on output", "New output while scrolled back returns to the last line", scrollTo))

	notifyOff := valueIs("notify-on-command-finish", "never", "never")
	na := keySetting("notifyafter", "notify-on-command-finish-after", "Only after", "How long a command must have run before its end is worth a notification", []string{"1s", "5s", "10s", "30s", "1m", "5m"}, "5s")
	na.hidden = notifyOff
	session(na)
	const notifyHow = "bell,no-notify"
	nb := flagSetting("notifybell", "notify-on-command-finish-action", "bell", "Ring the bell", "A finished command rings the bell, with whatever the bell is set to do", notifyHow)
	nb.hidden = notifyOff
	session(nb)
	nd := flagSetting("notifydesktop", "notify-on-command-finish-action", "notify", "Desktop notification", "A finished command shows a notification of the desktop", notifyHow)
	nd.hidden = notifyOff
	session(nd)
	session(boolSetting("desktopnotify", "desktop-notifications", "Notifications from programs", "A program may show a desktop notification, with OSC 9 or OSC 777", true))
	if !mac {
		const toasts = "clipboard-copy,config-reload"
		session(flagSetting("toastcopy", "app-notifications", "clipboard-copy", "Say when text is copied", "A short message over the terminal each time text is copied", toasts))
		session(flagSetting("toastreload", "app-notifications", "config-reload", "Say when the config reloads", "A short message over the terminal each time the config is read again", toasts))
		ns := textSetting("quickns", "gtk-quick-terminal-namespace", "Namespace", "The name the compositor and its rules know the quick terminal's layer by, on Wayland", "ghostty-quick-terminal", "Ghostty's own name for it", nil, nil)
		ns.hint = quickHint
		session(ns)
	}

	if v14 {
		// 1.3.0 does not read the key, and the editor only tells 1.3 from 1.4.
		session(boolSetting("progress", "progress-style", "Progress bars", "A program may show its progress as a bar across the top of the terminal, with OSC 9;4", true))
	}
	il := keySetting("imagelimit", "image-storage-limit", "Image memory", "Bytes of pictures a terminal keeps, for the Kitty image protocol and the like", []string{"0", "100000000", "320000000", "1000000000", "4294967295"}, "320000000")
	il.explain = map[string]string{
		"0":          "no pictures at all: every image protocol is switched off",
		"100000000":  "about 100 MB per screen",
		"320000000":  "about 320 MB per screen (the default)",
		"1000000000": "about 1 GB per screen",
		"4294967295": "4 GiB per screen, the most Ghostty takes",
	}
	il.note = newTerminals
	session(il)

	if mac {
		au := keySetting("autoupdate", "auto-update", "Check for updates", "Whether Ghostty looks for a new version of itself", []string{"default", "off", "check", "download"}, "default")
		au.explain = map[string]string{
			"default":  "what you answered when Ghostty first asked, kept in the macOS preferences",
			"off":      "never look for updates",
			"check":    "look, and say when there is one; nothing is downloaded",
			"download": "look, download and say; installing still waits for you",
		}
		session(au)
		ch := keySetting("updatechannel", "auto-update-channel", "Release channel", "Which versions updates offer", []string{"default", "stable", "tip"}, "default")
		ch.explain = map[string]string{
			"default": "the channel of the Ghostty installed",
			"stable":  "tagged releases only",
			"tip":     "a build of every change to Ghostty: newer, with more bugs",
		}
		ch.note = restart
		session(ch)

		ic := keySetting("icon", "macos-icon", "App icon", "The icon in the Dock and the app switcher; the Finder keeps the official one",
			[]string{"official", "blueprint", "chalkboard", "microchip", "glass", "holographic", "paper", "retro", "xray", "custom", "custom-style"}, "official")
		ic.explain = map[string]string{
			"official":     "Ghostty's own icon (the default)",
			"custom":       "an icon file of yours, picked below",
			"custom-style": "the official icon in a frame and colours of yours, picked below; experimental",
		}
		for _, v := range []string{"blueprint", "chalkboard", "microchip", "glass", "holographic", "paper", "retro", "xray"} {
			ic.explain[v] = "one of the variants artists drew for Ghostty"
		}
		session(ic)
		customStyle := func(m *Model) bool { return m.tree.Value("macos-icon", "") != "custom-style" }
		fr := keySetting("iconframe", "macos-icon-frame", "Icon frame", "The material of the frame round the icon", []string{"aluminum", "beige", "plastic", "chrome"}, "aluminum")
		fr.explain = map[string]string{
			"aluminum": "brushed aluminium (the default)",
			"beige":    "the beige of a nineties computer",
			"plastic":  "glossy dark plastic",
			"chrome":   "shiny chrome",
		}
		fr.hidden = customStyle
		session(fr)
		gh := colourSetting("iconghost", "macos-icon-ghost-color", "Ghost colour", "The colour of the ghost in the icon; the custom style needs it", "default", "not set: the custom style is not drawn without it")
		gh.hidden = customStyle
		session(gh)
		scr := colourSetting("iconscreen", "macos-icon-screen-color", "Screen colour", "The colour of the screen behind the ghost; the custom style needs it", "default", "not set: the custom style is not drawn without it")
		scr.hint = "The config file takes several, separated by commas, for a gradient from the bottom up."
		scr.hidden = customStyle
		session(scr)
		file := m.fileSetting(iconFiles)
		file.hidden = func(m *Model) bool { return m.tree.Value("macos-icon", "") != "custom" }
		session(file)
		dd := keySetting("dockdrop", "macos-dock-drop-behavior", "Folder dropped on the Dock", "Where a file or folder dropped on Ghostty's Dock icon opens", []string{"new-tab", "new-window"}, "new-tab")
		dd.explain = map[string]string{
			"new-tab":    "a new tab of the window in front, or a window when there is none (the default)",
			"new-window": "always a new window",
		}
		session(dd)
		hd := keySetting("machidden", "macos-hidden", "Hide from the Dock", "Keep Ghostty out of the Dock and the app switcher, for those who live in the quick terminal", []string{"never", "always"}, "never")
		hd.explain = map[string]string{
			"never":  "Ghostty is in the Dock and the app switcher (the default)",
			"always": "it is in neither; macOS then stops switching the keyboard layout for it",
		}
		session(hd)
		session(boolSetting("secureinput", "macos-auto-secure-input", "Secure input at passwords", "Ghostty turns on macOS's Secure Input when it sees a password prompt, so no other app reads the keys; it cannot tell over ssh", true))
		session(boolSetting("secureindicator", "macos-secure-input-indication", "Show secure input", "A sign in the window while Secure Input is on", true))
		sh := keySetting("shortcuts", "macos-shortcuts", "Shortcuts app", "Whether the Shortcuts app may drive Ghostty: open terminals, type into them, run any action", []string{"ask", "allow", "deny"}, "ask")
		sh.explain = map[string]string{
			"ask":   "Ghostty asks once and remembers the answer (the default)",
			"allow": "any shortcut may drive Ghostty without asking",
			"deny":  "no shortcut may",
		}
		session(sh)
		session(boolSetting("applescript", "macos-applescript", "AppleScript", "Scripts may list and drive Ghostty's windows, tabs and terminals; off answers none", true))
	} else {
		si := keySetting("singleinstance", "gtk-single-instance", "Single instance", "Whether every ghostty command opens a window of the one Ghostty running, or starts another", []string{"detect", "true", "false"}, "detect")
		si.explain = map[string]string{
			"detect": "one Ghostty, unless it is started from a terminal or with arguments (the default)",
			"true":   "always one Ghostty: a second launch opens a window in the first",
			"false":  "every launch is a Ghostty of its own",
		}
		si.note = restart
		session(si)
		cl := textSetting("class", "class", "Application ID", "The Wayland app ID, the X11 class and the D-Bus name; window rules match on it", "default", "com.mitchellh.ghostty", nil, nil)
		cl.hint = "Another ID makes a separate instance, and may stop desktop launchers and D-Bus activation from finding Ghostty."
		cl.note = restart
		session(cl)
		x11 := textSetting("x11name", "x11-instance-name", "X11 instance name", "The instance half of WM_CLASS, under X11 only", "default", "ghostty", nil, nil)
		x11.note = restart
		session(x11)
		lang := textSetting("language", "language", "Language", "The language of Ghostty's own menus and dialogs; programs in the terminal keep theirs", "default", "the language of the desktop", func(m *Model) []option {
			var out []option
			for _, l := range [][2]string{{"en", "English"}, {"de", "German"}, {"es", "Spanish"}, {"fr", "French"}, {"it", "Italian"}, {"ja", "Japanese"}, {"nl", "Dutch"}, {"pl", "Polish"}, {"pt_BR", "Brazilian Portuguese"}, {"ru", "Russian"}, {"tr", "Turkish"}, {"uk", "Ukrainian"}, {"zh_CN", "Simplified Chinese"}} {
				out = append(out, option{label: l[0], detail: l[1], value: l[0]})
			}
			return out
		}, nil)
		lang.syntax = "language default|<code>"
		lang.note = restart
		session(lang)
	}

	// --- Advanced: what terminals argue about ----------------------------------
	term := textSetting("term", "term", "TERM", "The terminal type programs are told they run in", "xterm-ghostty", "Ghostty's own terminfo entry, which describes everything it can do", func(m *Model) []option {
		return []option{{label: "xterm-256color", detail: "known everywhere, ssh hosts included, but programs miss some of what Ghostty can do", value: "xterm-256color"}}
	}, nil)
	term.note = newTerminals
	session(term)
	session(textSetting("enquiry", "enquiry-response", "Answer to ENQ", "What Ghostty sends back when a program sends the ENQ character", "default", "nothing is sent", nil, nil))
	osc := keySetting("oscreport", "osc-color-report-format", "Colour reports", "How Ghostty answers a program that asks for its colours, with OSC 4, 10 or 11", []string{"16-bit", "8-bit", "none"}, "16-bit")
	osc.explain = map[string]string{
		"16-bit": "rrrr/gggg/bbbb, as most terminals answer (the default)",
		"8-bit":  "rr/gg/bb, for old programs that expect it",
		"none":   "no answer: programs cannot tell a dark theme from a light one",
	}
	session(osc)
	session(boolSetting("titlereport", "title-report", "Programs read the title", "A program may ask for the window title with CSI 21 t; a known way to trick a shell into running text, so off unless you need it", false))
	session(boolSetting("kam", "vt-kam-allowed", "Keyboard lock (KAM)", "A program may switch the keyboard off with ANSI mode 2; hardly anything needs it", false))
	if v14 {
		session(boolSetting("checksum", "vt-xt-checksum-report", "Screen checksums", "A program may ask for a checksum of part of the screen with DECRQCRA; test suites use it, and it lets a program read the screen back", false))
		const sums = "negate,attributes,trim,no-undrawn,no-full"
		sum := func(cmd, flag, label, detail string) {
			st := flagSetting(cmd, "vt-xt-checksum-extension", flag, label, detail, sums)
			st.hidden = valueIs("vt-xt-checksum-report", "false", "false")
			st.hint = "The defaults are the sum of a real DEC terminal; a program can change them until the next reset."
			session(st)
		}
		sum("checksumnegate", "negate", "Checksum: negated", "Report the negated sum, as DEC terminals do")
		sum("checksumattributes", "attributes", "Checksum: attributes", "Bold, underline, blink, inverse, invisible and protected add to the value of each cell")
		sum("checksumtrim", "trim", "Checksum: skip spaces", "Plain spaces are left out, but for the first cell of the area")
		sum("checksumundrawn", "undrawn", "Checksum: empty cells", "Cells never written to count as spaces instead of being skipped")
		sum("checksumfull", "full", "Checksum: full characters", "The whole code point of each cell is summed, not its DEC 8-bit value")
	}
	gw := keySetting("graphemewidth", "grapheme-width-method", "Character widths", "How wide Ghostty takes emoji and combined characters to be", []string{"unicode", "legacy"}, "unicode")
	gw.explain = map[string]string{
		"unicode": "by the Unicode standard: right, though a shell that counts the old way may misplace the cursor (the default)",
		"legacy":  "the old wcswidth way: agrees with old programs, wrong for emoji with a skin tone and the like",
	}
	gw.note = newTerminals
	session(gw)
	blend := "native"
	if !mac {
		blend = "linear-corrected"
	}
	ab := keySetting("alphablending", "alpha-blending", "Text blending", "The colour space the edges of letters are blended into their background in", []string{"native", "linear", "linear-corrected"}, blend)
	ab.explain = map[string]string{
		"native":           "the system's own space: Display P3 on macOS, sRGB on Linux",
		"linear":           "no dark fringes between colours like red and green, but dark text looks thinner and light text thicker",
		"linear-corrected": "linear, corrected so that text keeps the weight it has in native",
	}
	ab.explain[blend] += " (the default)"
	session(ab)
	if mac {
		vs := boolSetting("vsync", "window-vsync", "Vertical sync", "Drawing waits for the screen's refresh: no tearing, a little latency; off has caused kernel panics on macOS 14.4 and later", true)
		vs.note = newTerminals
		session(vs)
	} else {
		be := keySetting("asyncbackend", "async-backend", "Event backend", "The kernel interface Ghostty waits for input and output with; they perform alike", []string{"auto", "epoll", "io_uring"}, "auto")
		be.explain = map[string]string{
			"auto":     "the best one this kernel offers (the default)",
			"epoll":    "epoll, which every kernel has",
			"io_uring": "io_uring, which hardened kernels may forbid; Ghostty then falls back",
		}
		be.note = restart
		session(be)
		const freetype = "hinting,no-force-autohint,no-monochrome,autohint,light"
		ft := func(cmd, flag, label, detail string) {
			session(flagSetting(cmd, "freetype-load-flags", flag, label, detail, freetype))
		}
		ft("hinting", "hinting", "Font hinting", "Glyphs are fitted to the pixel grid, which sharpens small text")
		ft("autohint", "autohint", "Auto-hinter", "FreeType's own hinter is used for fonts that carry no hints")
		ft("forceautohint", "force-autohint", "Always auto-hint", "FreeType's hinter replaces the font's own hints")
		ft("lighthinting", "light", "Light hinting", "Hint gently, keeping the shapes of the letters, as GTK apps do")
		ft("monochrome", "monochrome", "No anti-aliasing", "One-bit rendering, for pixel fonts; light hinting does nothing with it")
		session(boolSetting("opengldebug", "gtk-opengl-debug", "OpenGL debug logs", "GTK logs what its OpenGL context reports, for chasing a rendering bug", false))
		cg := keySetting("cgroup", "linux-cgroup", "Isolate terminals", "Each terminal runs in a systemd scope of its own, so one that eats memory is stopped alone; costs a little at startup", []string{"single-instance", "always", "never"}, "single-instance")
		cg.explain = map[string]string{
			"single-instance": "only when Ghostty runs as a single instance (the default)",
			"always":          "always",
			"never":           "never: every terminal shares Ghostty's own cgroup",
		}
		cg.note = newTerminals
		session(cg)
		noCgroup := valueIs("linux-cgroup", "single-instance", "never")
		hf := boolSetting("cgroupfail", "linux-cgroup-hard-fail", "Fail without isolation", "A terminal that cannot get its scope does not open at all; off opens it anyway", false)
		hf.hidden = noCgroup
		session(hf)
		ml := keySetting("cgroupmemory", "linux-cgroup-memory-limit", "Memory limit", "Bytes a terminal's programs may use before systemd squeezes them (MemoryHigh)", []string{"default", "1000000000", "4000000000", "16000000000"}, "default")
		ml.explain = map[string]string{
			"default":     "no limit",
			"1000000000":  "about 1 GB per terminal",
			"4000000000":  "about 4 GB per terminal",
			"16000000000": "about 16 GB per terminal",
		}
		ml.hidden = noCgroup
		session(ml)
		pl := keySetting("cgroupprocs", "linux-cgroup-processes-limit", "Process limit", "How many processes a terminal may hold at once (TasksMax), which stops a fork bomb", []string{"default", "1000", "10000", "100000"}, "default")
		pl.explain = map[string]string{"default": "no limit"}
		pl.hidden = noCgroup
		session(pl)
	}
	return s
}
