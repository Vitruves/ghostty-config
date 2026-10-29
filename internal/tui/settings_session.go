package tui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The quick terminal, the bell and shell integration: how the terminal
// behaves around the work rather than how it looks. Every key here was read
// against Ghostty 1.4's own documentation, platform by platform: a key only
// one runtime implements is only offered there, and a key or value 1.4 added
// only to a Ghostty that reads it, since 1.3's validator rejects it.

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
	} else if m.since14() {
		// Wayland places it on a chosen screen since 1.4; macos-menu-bar is
		// taken there as main, so it is not offered.
		sc := keySetting("quickscreen", "quick-terminal-screen", "Screen", "Which screen it opens on, with more than one, on Wayland", []string{"main", "mouse"}, "main")
		sc.explain = map[string]string{
			"main":  "the primary output the compositor names, or else the first monitor",
			"mouse": "the screen the pointer is on",
		}
		quick(sc)
	}
	if mac {
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

// scrollbackSettings are how much history a terminal keeps. Ghostty 1.4
// renamed scrollback-limit to scrollback-limit-bytes, raised its default
// from 10 MB to 50 MB, and added a cap in lines and the compression of idle
// history. Before 1.4 only the old key is offered; from it, a change writes
// the new name and comments the old one out, which 1.4 still reads.
func scrollbackSettings(v14 bool) []setting {
	sizes := []string{"1000000", "10000000", "50000000", "100000000", "500000000"}
	explain := map[string]string{
		"1000000":   "about 1 MB per terminal: a few thousand lines",
		"10000000":  "about 10 MB per terminal: tens of thousands of lines",
		"50000000":  "about 50 MB per terminal: long build logs stay",
		"100000000": "about 100 MB per terminal: very deep history",
		"500000000": "about 500 MB per terminal: for people who never clear",
		"unlimited": "no limit in bytes: history grows as long as memory lasts, or up to the cap in lines",
	}
	const detail = "Bytes of history kept per terminal, not lines; memory is only taken as the history fills"
	if !v14 {
		st := keySetting("scrollback", "scrollback-limit", "Scrollback size", detail, sizes, "10000000")
		explain["10000000"] += " (the default)"
		st.explain = explain
		return []setting{st}
	}
	const key, old = "scrollback-limit-bytes", "scrollback-limit"
	const fallback = "50000000"
	explain[fallback] += " (the default)"
	size := keySetting("scrollback", key, "Scrollback size", detail, append(sizes, "unlimited"), fallback)
	size.explain = explain
	size.get = func(m *Model) string {
		if v := m.tree.Value(key, ""); v != "" {
			return v
		}
		return m.tree.Value(old, fallback)
	}
	size.set = func(m *Model, v string) tea.Cmd {
		if len(m.tree.All(old)) > 0 {
			m.tree.Unset(old, "disabled by ghostty-config: renamed scrollback-limit-bytes in Ghostty 1.4")
		}
		if v == fallback {
			m.tree.Unset(key, "disabled by ghostty-config: back to the default")
		} else {
			m.tree.Set(key, v)
		}
		return nil
	}
	lines := keySetting("scrollbacklines", "scrollback-limit-lines", "Scrollback lines", "A cap in lines as well; whichever limit is reached first trims the history, a page of lines at a time", []string{"unlimited", "10000", "100000", "1000000"}, "unlimited")
	lines.explain = map[string]string{
		"unlimited": "only the size in bytes limits the history (the default)",
		"10000":     "about ten thousand lines; trimming by the page keeps a few more",
		"100000":    "about a hundred thousand lines, if the size in bytes allows them",
		"1000000":   "about a million lines, if the size in bytes allows them",
	}
	return []setting{
		size,
		lines,
		boolSetting("compression", "scrollback-compression", "Compress scrollback", "History out of sight is compressed while the terminal is idle: as many lines, in less memory", true),
	}
}

// programSettings14 are what Ghostty 1.4 lets a program running in the
// terminal do, or how it reads one that stops at once. Both permissions are
// guard rails: a program over ssh can use them as well as a local one.
func programSettings14(mac bool) []setting {
	exitDetail := "Milliseconds: a command that exits sooner with a non-zero code shows an error message"
	if mac {
		exitDetail = "Milliseconds: a command that exits sooner shows an error message, whatever its exit code"
	}
	limit := keySetting("cliplimit", "clipboard-write-limit-bytes", "Clipboard write limit", "The most a program may copy at once through the Kitty clipboard protocol, OSC 5522; OSC 52 is not affected", []string{"0", "16777216", "67108864", "268435456", "unlimited"}, "67108864")
	limit.explain = map[string]string{
		"0":         "every write that is not empty is refused; clipboard-write = deny refuses them all",
		"16777216":  "16 MiB: below what the protocol expects, so a large copy may fail",
		"67108864":  "64 MiB, the least the protocol asks a terminal to accept (the default)",
		"268435456": "256 MiB: a write is held in memory while it arrives",
		"unlimited": "no limit but memory: a program may make Ghostty hold as much as it sends",
	}
	return []setting{
		numberSetting("exittime", "abnormal-command-exit-runtime", "Too-quick exit", exitDetail, 250, 0, 2000, 50, plain),
		boolSetting("vtresize", "vt-window-resize-allowed", "Programs may resize the window", "Any program, one over ssh too, may resize the window with CSI 8 t; ignored in a split, beside other tabs, in the quick terminal, or full screen, maximized or tiled", false),
		limit,
	}
}
