package tui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The quick terminal, the bell and shell integration: how the terminal
// behaves around the work rather than how it looks. Every key here was read
// against Ghostty 1.3's own documentation, platform by platform: a key only
// one runtime implements is only offered there.

// Ghostty's default flag lists, as `ghostty +show-config --default` prints them.
const (
	bellDefaults  = "no-system,no-audio,attention,title,no-border"
	shellDefaults = "cursor,no-sudo,title,no-ssh-env,no-ssh-terminfo,path"
)

// quickHint is said under every setting of the quick terminal: without a
// key bound to it, it never opens.
const quickHint = "Nothing opens it until it has a shortcut: set one with keys toggle_quick_terminal, usually as a global: key."

// sessionExtraSettings are the quick terminal, the bell and shell integration.
func (m *Model) sessionExtraSettings(mac bool) []setting {
	var s []setting
	add := func(st setting) {
		st.group = "Session"
		s = append(s, st)
	}
	quick := func(st setting) {
		st.hint = quickHint
		add(st)
	}

	pos := keySetting("quickposition", "quick-terminal-position", "Position", "The edge of the screen it slides in from", []string{"top", "bottom", "left", "right", "center"}, "top")
	pos.explain = map[string]string{
		"top":    "it drops down from the top edge, like the console of a game",
		"bottom": "it rises from the bottom edge",
		"left":   "it slides in from the left edge",
		"right":  "it slides in from the right edge",
		"center": "it appears in the middle of the screen, over everything",
	}
	if mac {
		pos.note = "needs a Ghostty restart on macOS"
	}
	quick(pos)
	quick(m.quickSize())
	if mac {
		sc := keySetting("quickscreen", "quick-terminal-screen", "Screen", "Which screen it opens on, with more than one", []string{"main", "mouse", "macos-menu-bar"}, "main")
		sc.explain = map[string]string{
			"main":           "the screen macOS gives the keyboard to, the one you are working on",
			"mouse":          "the screen the pointer is on",
			"macos-menu-bar": "the screen that holds the main menu bar, set in the Displays settings",
		}
		quick(sc)
		quick(numberSetting("quickanimation", "quick-terminal-animation-duration", "Animation", "Seconds it takes to slide in and out; 0 shows and hides it at once", 0.2, 0, 1, 0.05, func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }))
	}
	quick(boolSetting("quickhide", "quick-terminal-autohide", "Hide when it loses focus", "It slides away as soon as another window is clicked; off keeps it open until the shortcut closes it", mac))
	if mac {
		sp := keySetting("quickspaces", "quick-terminal-space-behavior", "Across Spaces", "What it does when you switch to another Space or a full-screen app", []string{"move", "remain"}, "move")
		sp.explain = map[string]string{
			"move":   "it follows you to the Space you switch to",
			"remain": "it stays on the Space it was opened on",
		}
		quick(sp)
	} else {
		kb := keySetting("quickkeyboard", "quick-terminal-keyboard-interactivity", "Keyboard", "When it takes the keyboard, on Wayland; the compositor has the last word", []string{"none", "on-demand", "exclusive"}, "on-demand")
		kb.explain = map[string]string{
			"none":      "it never takes keyboard input",
			"on-demand": "it takes the keyboard while it has the focus",
			"exclusive": "it keeps the keyboard even while another window has the focus",
		}
		quick(kb)
		ly := keySetting("quicklayer", "gtk-quick-terminal-layer", "Layer", "How high it sits among the windows, on Wayland", []string{"overlay", "top", "bottom", "background"}, "top")
		ly.explain = map[string]string{
			"overlay":    "in front of every window, full-screen ones included",
			"top":        "in front of normal windows, behind lock screens and the like",
			"bottom":     "behind normal windows, in front of the wallpaper",
			"background": "behind every window",
		}
		quick(ly)
	}

	// The bell: what happens when a program rings it.
	bell := func(cmd, flag, label, detail string) {
		add(flagSetting(cmd, "bell-features", flag, label, detail, bellDefaults))
	}
	if mac {
		bell("bellsound", "system", "System alert", "Play the macOS alert sound, the one chosen in the Sound settings")
	} else {
		bell("bellsound", "system", "System alert", "Let the desktop signal it in its own way: a sound, a flash, a notification")
	}
	bell("bellaudio", "audio", "Custom sound", "Play a sound of your choosing, picked below")
	audioOff := func(m *Model) bool { return !ghostty.Flags(m.tree.Value("bell-features", ""), bellDefaults)["audio"] }
	sound := m.fileSetting(soundFiles)
	sound.hidden = audioOff
	add(sound)
	vol := numberSetting("bellvolume", "bell-audio-volume", "Volume", "How loud the sound plays, relative to the system volume", 0.5, 0, 1, 0.05, func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) })
	vol.hidden = audioOff
	add(vol)
	if mac {
		bell("bellbounce", "attention", "Bounce the Dock icon", "Once, while Ghostty is not the app in front")
	} else {
		bell("bellbounce", "attention", "Ask for attention", "Until Ghostty has the focus again; the desktop decides how: a highlighted task bar entry, a notification, a border")
	}
	bell("belltitle", "title", "Bell in the title", "A bell before the title of the terminal that rang, until you type in it")
	bell("bellborder", "border", "Border round the terminal", "A border round the terminal that rang, until you type in it")

	// Shell integration: what Ghostty teaches the shell.
	sh := keySetting("shell", "shell-integration", "Shell", "Which shell Ghostty injects its integration into: prompt marks, the working directory of new tabs, quiet closing at a prompt", []string{"detect", "none", "bash", "zsh", "fish", "elvish", "nushell"}, "detect")
	sh.explain = map[string]string{
		"detect":  "inject the integration of the shell Ghostty starts, told by its name",
		"none":    "nothing is injected; source the integration yourself if you want it",
		"bash":    "inject the bash integration, whatever the shell is called",
		"zsh":     "inject the zsh integration, whatever the shell is called",
		"fish":    "inject the fish integration, whatever the shell is called",
		"elvish":  "inject the elvish integration, whatever the shell is called",
		"nushell": "inject the nushell integration, whatever the shell is called",
	}
	add(sh)
	feature := func(cmd, flag, label, detail string) {
		st := flagSetting(cmd, "shell-integration-features", flag, label, detail, shellDefaults)
		st.hint = "Needs shell integration, injected or sourced by hand."
		add(st)
	}
	feature("shellcursor", "cursor", "Bar cursor at the prompt", "The cursor turns to a bar while the shell waits for a command")
	feature("shelltitle", "title", "Title from the shell", "The shell sets the window title to the command running, or the directory")
	feature("shellsudo", "sudo", "Keep terminfo under sudo", "Wrap sudo so programs run through it still know the terminal")
	feature("shellpath", "path", "Ghostty on the PATH", "Add the directory of the ghostty command to PATH, even where startup scripts reset it")
	feature("sshenv", "ssh-env", "SSH environment", "Over ssh, TERM falls back to xterm-256color and COLORTERM and TERM_PROGRAM go along")
	feature("sshterminfo", "ssh-terminfo", "Install terminfo over SSH", "Copy Ghostty's terminfo to hosts that lack it, once per host, with infocmp and tic")
	return s
}

// quickSize is the size of the quick terminal along the axis it slides on:
// height from the top or bottom, width from a side. Ghostty also takes a
// second size across, after a comma; it is kept as it was.
func (m *Model) quickSize() setting {
	const key = "quick-terminal-size"
	st := numberSetting("quicksize", key, "Size", "How far it reaches into the screen: its height from the top or bottom, its width from a side", 0, 10, 100, 5, percent)
	st.unset, st.unsetAt, st.fallback = "default", 10, "default"
	st.syntax = "quicksize default|<10%–100%>"
	st.explain = map[string]string{"default": "the size Ghostty gives the edge it is on"}
	split := func(v string) (string, string) {
		first, rest, _ := strings.Cut(v, ",")
		return strings.TrimSpace(first), strings.TrimSpace(rest)
	}
	st.get = func(m *Model) string {
		first, _ := split(m.tree.Value(key, ""))
		if first == "" {
			return "default"
		}
		return first
	}
	write := func(m *Model, v string) string {
		if v == "default" {
			return ""
		}
		if _, across := split(m.tree.Value(key, "")); across != "" {
			return v + "," + across
		}
		return v
	}
	st.set = func(m *Model, v string) tea.Cmd {
		if line := write(m, v); line == "" {
			m.tree.Unset(key, "disabled by ghostty-config: back to the default")
		} else {
			m.tree.Set(key, line)
		}
		return nil
	}
	st.writes = func(m *Model, v string) string {
		if line := write(m, v); line != "" {
			return key + " = " + line
		}
		return ""
	}
	return st
}
