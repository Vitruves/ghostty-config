package ghostty

import "strings"

// Flag sets. A few keys, bell-features and shell-integration-features among
// them, hold a set of switches written as one list: `audio,no-title`. Ghostty
// starts each line from its defaults and applies the list in order, so a flag
// left out keeps its default and the last line wins as a whole. A bare
// `true` or `false` switches every flag at once.
//
// An edit changes the one flag it is about and leaves every other word of
// the list as the user wrote it, flags this tool does not know included:
// they may belong to a newer Ghostty.

// Flags is the state of every flag a value sets, on top of defaults, which
// is written the same way.
func Flags(value, defaults string) map[string]bool {
	out := map[string]bool{}
	for _, tok := range flagWords(defaults) {
		name, on := flagWord(tok)
		out[name] = on
	}
	if b, ok := flagBool(value); ok {
		for name := range out {
			out[name] = b
		}
		return out
	}
	for _, tok := range flagWords(value) {
		name, on := flagWord(tok)
		out[name] = on
	}
	return out
}

// WithFlag rewrites a flag list so that flag is on or off. The word for it
// is dropped when its default already says so, and put where the old one
// stood otherwise; an empty result means the key can go.
func WithFlag(value, flag string, on bool, defaults string) string {
	words := flagWords(value)
	if b, ok := flagBool(value); ok {
		// `true` or `false` stands for every flag: spell them out, so one
		// can differ.
		words = nil
		for _, tok := range flagWords(defaults) {
			name, _ := flagWord(tok)
			words = append(words, flagText(name, b))
		}
	}
	at := -1
	var kept []string
	for _, w := range words {
		if name, _ := flagWord(w); name == flag {
			if at < 0 {
				at = len(kept)
			}
			continue
		}
		kept = append(kept, w)
	}
	if Flags(defaults, defaults)[flag] != on {
		if at < 0 {
			at = len(kept)
		}
		kept = append(kept[:at], append([]string{flagText(flag, on)}, kept[at:]...)...)
	}
	return strings.Join(kept, ",")
}

func flagWords(v string) []string {
	var out []string
	for _, w := range strings.Split(v, ",") {
		if w = strings.TrimSpace(w); w != "" {
			out = append(out, w)
		}
	}
	return out
}

func flagWord(w string) (string, bool) {
	if name, ok := strings.CutPrefix(w, "no-"); ok {
		return name, false
	}
	return w, true
}

func flagText(name string, on bool) string {
	if on {
		return name
	}
	return "no-" + name
}

func flagBool(v string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}

// SetFlag switches one flag of a flag-set key. The edit is recorded as the
// flag, not as the line it produced, so a replay onto a file edited
// elsewhere changes that flag of whatever list the file holds by then.
func (t *Tree) SetFlag(key, flag string, on bool, defaults string) []*Document {
	t.mu.Lock()
	defer t.mu.Unlock()
	var changed []*Document
	t.record(func(t *Tree) { changed = t.setFlag(key, flag, on, defaults) })
	return changed
}

func (t *Tree) setFlag(key, flag string, on bool, defaults string) []*Document {
	now := t.Value(key, "")
	next := WithFlag(now, flag, on, defaults)
	if next == now {
		return nil
	}
	if next == "" {
		return t.unset(key, "disabled by ghostty-config: back to the default")
	}
	if d := t.set(key, next); d != nil {
		return []*Document{d}
	}
	return nil
}
