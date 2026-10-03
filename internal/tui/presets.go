package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// A preset is a named bundle of settings, each given as the command that
// owns it and the value that command would take. Applying one is nothing
// more than running those commands in turn, so every value goes through the
// same writer, and every setting a platform lacks is skipped quietly.
type preset struct {
	name, detail string
	values       [][2]string
}

// preset values are written the way the commands print them.
var presets = []preset{
	{"default", "Everything a preset touches, back to Ghostty's own default", [][2]string{
		{"opacity", "1.00"}, {"blur", "off"}, {"titlebar", "transparent"}, {"shadow", "on"}, {"paddingx", "2"}, {"paddingy", "2"},
		{"balance", "off"}, {"paddingcolor", "background"}, {"lineheight", "0%"}, {"cellwidth", "0%"}, {"faint", "0.50"},
		{"splitopacity", "0.70"}, {"cursor", "block"}, {"blink", "default"}, {"cursoropacity", "1.0"}, {"scrollbar", "system"},
		{"resizeoverlay", "after-first"}, {"contrast", "1"}, {"hidemouse", "off"}, {"notify", "never"},
	}},
	{"glass", "Translucent and blurred, the desktop showing through, roomy padding", [][2]string{
		{"opacity", "0.85"}, {"blur", "macos-glass-regular"}, {"titlebar", "transparent"}, {"shadow", "on"},
		{"paddingx", "14"}, {"paddingy", "12"}, {"balance", "on"}, {"paddingcolor", "extend"}, {"contrast", "3"},
	}},
	{"minimal", "No title bar, no overlays, no scrollbar: the window is just text", [][2]string{
		{"titlebar", "hidden"}, {"shadow", "off"}, {"opacity", "1.00"}, {"blur", "off"}, {"paddingx", "16"}, {"paddingy", "12"},
		{"balance", "on"}, {"paddingcolor", "extend"}, {"scrollbar", "never"}, {"resizeoverlay", "never"},
	}},
	{"focus", "Wide margins, a calm bar cursor, fading splits, a quiet pointer", [][2]string{
		{"paddingx", "28"}, {"paddingy", "22"}, {"balance", "on"}, {"paddingcolor", "extend"}, {"lineheight", "10%"},
		{"cursor", "bar"}, {"blink", "false"}, {"splitopacity", "0.45"}, {"scrollbar", "never"}, {"hidemouse", "on"}, {"notify", "unfocused"},
	}},
	{"compact", "Tight rows and margins to fit as much as the screen allows", [][2]string{
		{"titlebar", "tabs"}, {"paddingx", "6"}, {"paddingy", "4"}, {"balance", "off"}, {"lineheight", "-6%"},
		{"cellwidth", "-2%"}, {"scrollbar", "never"}, {"resizeoverlay", "never"},
	}},
	{"reading", "Airy lines and letters for long text, with dimmed text kept readable", [][2]string{
		{"lineheight", "22%"}, {"cellwidth", "2%"}, {"paddingx", "22"}, {"paddingy", "16"}, {"balance", "on"},
		{"faint", "0.70"}, {"cursor", "bar"}, {"blink", "true"}, {"contrast", "4.5"},
	}},
	{"presentation", "Large margins, solid background and a contrast floor, for projectors", [][2]string{
		{"opacity", "1.00"}, {"blur", "off"}, {"contrast", "7"}, {"faint", "0.85"}, {"lineheight", "12%"},
		{"cursor", "block"}, {"blink", "false"}, {"cursoropacity", "1.0"}, {"paddingx", "24"}, {"paddingy", "20"},
		{"balance", "on"}, {"thicken", "on"}, {"resizeoverlay", "never"},
	}},
	{"power", "Fast keys: copy on select, Option as Alt, deep scrollback, quiet notifications", [][2]string{
		{"copy", "true"}, {"optionalt", "true"}, {"scrollback", "100000000"}, {"clicktomove", "on"}, {"inheritcwd", "on"},
		{"notify", "unfocused"}, {"clearselect", "on"}, {"trimspaces", "on"}, {"confirmclose", "true"},
	}},
	{"careful", "Guard rails: confirm closes, protect pastes, restore windows", [][2]string{
		{"confirmclose", "always"}, {"pasteguard", "on"}, {"savestate", "always"}, {"quitlast", "off"}, {"copy", "false"},
	}},
}

// presetChanges lists what applying p would change on this machine: the
// setting, what it is now, what it would become, and what it does.
func (m *Model) presetChanges(p preset) [][4]string {
	settings := map[string]setting{}
	for _, s := range m.buildSettings() {
		settings[s.cmd] = s
	}
	var out [][4]string
	for _, kv := range p.values {
		s, ok := settings[kv[0]]
		if !ok {
			continue
		}
		if now := s.get(m); !strings.EqualFold(now, kv[1]) {
			out = append(out, [4]string{s.label, now, kv[1], s.detail})
		}
	}
	return out
}

func (m *Model) applyPreset(p preset) tea.Cmd {
	settings := map[string]setting{}
	for _, s := range m.buildSettings() {
		settings[s.cmd] = s
	}
	n := 0
	var cmds []tea.Cmd
	for _, kv := range p.values {
		s, ok := settings[kv[0]]
		if !ok || strings.EqualFold(s.get(m), kv[1]) {
			continue
		}
		if cmd := s.set(m, kv[1]); cmd != nil {
			cmds = append(cmds, cmd)
		}
		n++
	}
	if n == 0 {
		m.info("Preset %s: already in place", p.name)
		return nil
	}
	m.info("Preset %s: %d setting(s) written; %s", p.name, n, m.reloadWord())
	return tea.Batch(append(cmds, m.flushWritesCmd())...)
}

func presetCommand() *command {
	find := func(opt *option, arg string) *preset {
		name := pickValue(arg, opt)
		for i := range presets {
			if presets[i].name == strings.ToLower(name) {
				return &presets[i]
			}
		}
		return nil
	}
	return &command{name: "preset", group: "Window", syntax: "preset " + presetNames(), desc: "Apply a bundle of window, font and input settings in one go; the panel lists what would change",
		filters: true,
		options: func(m *Model, arg string) []option {
			var out []option
			for _, p := range presets {
				if arg != "" && !fuzzyMatch(p.name, strings.TrimSpace(arg)) {
					continue
				}
				out = append(out, option{label: p.name, detail: p.detail, value: p.name})
			}
			return out
		},
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			p := find(opt, arg)
			if p == nil {
				m.warn("No preset matches %q", arg)
				return nil
			}
			return m.applyPreset(*p)
		},
		preview: func(m *Model, opt *option, width int) []string {
			c := m.c
			p := find(opt, "")
			if p == nil {
				return nil
			}
			lines := []string{c.bold(c.fg).Render(truncate("Preset "+p.name, width))}
			lines = append(lines, para(c.mutedS(), p.detail+".", width)...)
			changes := m.presetChanges(*p)
			if len(changes) == 0 {
				return append(lines, c.text(c.ok).Render("Already in place"))
			}
			lines = append(lines, c.mutedS().Render(fmt.Sprintf("Enter changes %d setting(s):", len(changes))))
			for _, ch := range changes {
				lines = append(lines, truncate(c.bold(c.fg).Render(pad(ch[0], 26))+c.mutedS().Render(ch[1]+" → ")+valueStyle(c, ch[2]).Bold(true).Render(ch[2]), width))
				lines = append(lines, para(c.mutedS(), "  "+ch[3]+".", width)...)
			}
			return lines
		},
	}
}

func presetNames() string {
	var n []string
	for _, p := range presets {
		n = append(n, p.name)
	}
	return strings.Join(n, "|")
}

// valueNotes say, for settings that take a word, what each word does. The
// panel shows the one under the highlight, so a value is never a guess.
var valueNotes = map[string]map[string]string{
	"copy":          {"true": "selecting text copies it at once (to the selection clipboard where there is one)", "clipboard": "selecting copies to the selection clipboard and the system clipboard", "false": "nothing is copied until you press the copy key"},
	"confirmclose":  {"true": "ask only when a process is still running in the terminal", "always": "ask every time, even at an idle prompt", "false": "close immediately, never ask"},
	"notify":        {"never": "no notification when a command finishes", "unfocused": "notify only if the terminal is not the window you are looking at", "always": "notify after every command, focused or not"},
	"resizeoverlay": {"after-first": "show the size popup when you resize, not on the first draw", "always": "always show the size popup, including at start", "never": "never show it"},
	"optionalt":     {"default": "let macOS decide from your keyboard layout", "true": "both Option keys act as Alt: readline and vim shortcuts work, accents on Option do not", "left": "only the left Option is Alt; the right still types accents", "right": "only the right Option is Alt; the left still types accents", "false": "Option types accented characters; Alt shortcuts are lost"},
	"savestate":     {"default": "follow the macOS setting for restoring windows", "never": "always start with a clean window", "always": "always bring back the windows from last time"},
	"scrollbar":     {"system": "show the scrollbar the way macOS or the desktop does", "never": "no scrollbar; scrolling still works"},
	"scrollback":    {"1000000": "about 1 MB per terminal: a few thousand lines", "10000000": "about 10 MB per terminal: the default, tens of thousands of lines", "50000000": "about 50 MB per terminal: long build logs stay", "100000000": "about 100 MB per terminal: very deep history", "500000000": "about 500 MB per terminal: for people who never clear"},
	"paddingcolor":  {"background": "the margin is the theme background", "extend": "the nearest row's colour continues into the margin, so full-screen apps look seamless", "extend-always": "extend even where the background would otherwise stay plain"},
	"windowtheme":   {"auto": "dark or light chrome chosen from the theme background", "system": "chrome follows the desktop's light or dark mode", "light": "always light chrome", "dark": "always dark chrome"},
	"colorspace":    {"srgb": "theme colours as written, the safe choice", "display-p3": "render in the wider P3 gamut: colours get richer on a P3 display"},
	"cursor":        {"block": "a filled cell", "bar": "a thin vertical line, like a text editor", "underline": "a line under the character", "block_hollow": "an outlined cell that lets the character show"},
	"blink":         {"default": "let the shell and terminal mode decide", "true": "the cursor always blinks", "false": "the cursor never blinks"},
	"blur":          {"off": "the desktop behind a translucent window stays sharp", "macos-glass-regular": "macOS Liquid Glass, the standard frosted look", "macos-glass-clear": "macOS Liquid Glass, the clearer variant"},
}
