package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Copying, pasting and links: the settings of the Input section whose values
// changed with Ghostty 1.4, or that 1.4 added. Read against Ghostty 1.4's own
// documentation; an older Ghostty is offered what it knows.

// copyOnSelect is copy-on-select. Before 1.4 it took true, clipboard and
// false; 1.4 names the clipboards, none, primary, clipboard and both, keeps
// true and false as aliases, and defaults to none on macOS, where there is
// no selection clipboard: primary does nothing there and both is clipboard.
func copyOnSelect(mac, v14 bool) setting {
	const key = "copy-on-select"
	if !v14 {
		return keySetting("copy", key, "Copy on select", "true copies a selection as you make it; clipboard also fills the system clipboard on Linux", []string{"true", "clipboard", "false"}, "true")
	}
	options, fallback := []string{"none", "primary", "clipboard", "both"}, "primary"
	detail := "Copy a selection as you make it: to the selection clipboard, the system one, or both"
	if mac {
		options, fallback = []string{"none", "clipboard"}, "none"
		detail = "Copy a selection to the clipboard as you make it"
	}
	// canon reads a value as 1.4 does here: the old words, and on macOS the
	// ones that mean the same as another.
	canon := func(v string) string {
		switch v = strings.ToLower(strings.TrimSpace(v)); v {
		case "true":
			if mac {
				return "clipboard"
			}
			return "primary"
		case "false":
			return "none"
		case "primary":
			if mac {
				return "none"
			}
		case "both":
			if mac {
				return "clipboard"
			}
		}
		return v
	}
	st := keySetting("copy", key, "Copy on select", detail, options, fallback)
	st.canon = canon
	st.get = func(m *Model) string {
		if v := canon(m.tree.Value(key, "")); v != "" {
			return v
		}
		return fallback
	}
	st.set = func(m *Model, v string) tea.Cmd {
		if v = canon(v); v == fallback {
			m.tree.Unset(key, "disabled by ghostty-config: back to the default")
		} else {
			m.tree.Set(key, v)
		}
		return nil
	}
	st.explain = map[string]string{
		"none":      "nothing is copied until you press the copy key",
		"primary":   "selecting fills the selection clipboard, the one a middle click pastes (the default)",
		"clipboard": "selecting fills the system clipboard only",
		"both":      "selecting fills the selection clipboard and the system clipboard",
	}
	if mac {
		st.explain["none"] = "nothing is copied until you press the copy key (the default)"
		st.explain["clipboard"] = "selecting text copies it to the clipboard at once"
	}
	return st
}

// clipboardAccess is whether a program in the terminal may read and write
// the clipboard. Both keys are older than 1.4; 1.4 has them govern the
// Kitty clipboard protocol, OSC 5522, as well as OSC 52, and its prompt
// shows what is on the clipboard before a read.
func clipboardAccess(v14 bool) []setting {
	via := "through OSC 52"
	if v14 {
		via = "through OSC 52 or the Kitty clipboard protocol, OSC 5522"
	}
	read := keySetting("clipread", "clipboard-read", "Programs read the clipboard", "Whether a program in the terminal, one over ssh too, may read the clipboard "+via, []string{"ask", "allow", "deny"}, "ask")
	read.explain = map[string]string{
		"ask":   "Ghostty asks each time a program wants what is on the clipboard (the default)",
		"allow": "any program may read the clipboard without asking, passwords copied a moment ago included",
		"deny":  "no program may read the clipboard; pasting yourself still works",
	}
	if v14 {
		read.explain["ask"] = "Ghostty asks each time, showing a preview of what is on the clipboard, images too (the default)"
	}
	write := keySetting("clipwrite", "clipboard-write", "Programs set the clipboard", "Whether a program in the terminal, one over ssh too, may replace the clipboard "+via, []string{"allow", "ask", "deny"}, "allow")
	write.explain = map[string]string{
		"allow": "a program may copy to the clipboard without asking: vim and tmux yanks reach it (the default)",
		"ask":   "Ghostty asks before a program replaces what is on the clipboard",
		"deny":  "no program may set the clipboard; copying yourself still works",
	}
	return []setting{read, write}
}

// inputSettings14 are the Input settings Ghostty 1.4 added: the middle
// button and OSC 8 links.
func inputSettings14(mac bool) []setting {
	mc := keySetting("middleclick", "middle-click-action", "Middle click", "What a middle click on the terminal does", []string{"primary-paste", "clipboard-paste", "ignore"}, "primary-paste")
	mc.explain = map[string]string{
		"primary-paste":   "paste the selection clipboard (the default)",
		"clipboard-paste": "paste the system clipboard",
		"ignore":          "nothing: the click is ignored",
	}
	if mac {
		mc.explain["primary-paste"] = "nothing happens: macOS has no selection clipboard (the default)"
	}
	return []setting{
		mc,
		boolSetting("osc8", "link-osc8", "Links from programs", "OSC 8 hyperlinks a program prints are highlighted, previewed, copied and opened; plain URLs are matched either way", true),
	}
}
