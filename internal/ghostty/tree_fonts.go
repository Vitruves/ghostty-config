package ghostty

import (
	"strconv"
	"strings"
)

// The repeatable font keys. font-feature, font-variation and
// font-codepoint-map stack: every line contributes, and a line may hold
// several values. An edit touches only the words about what it changes, so a
// line the user wrote with three features in it keeps the other two, and
// `-calt, -liga, -dlig`, which the ligatures setting owns, is never touched
// here. Each edit is recorded as what it means, a feature on or an axis at a
// value, so a replay onto a file edited elsewhere does the same to whatever
// the file holds by then.

// featureWord reads one feature setting in any of the forms Ghostty accepts:
// `ss01`, `+ss01`, `ss01 on`, `ss01=1`, `ss01 = 2` for on, `-ss01`,
// `ss01 off`, `ss01=0` for off, quotes allowed round the name.
func featureWord(w string) (tag string, on bool, ok bool) {
	w = strings.TrimSpace(w)
	if w == "" {
		return "", false, false
	}
	on = true
	switch w[0] {
	case '-':
		on, w = false, w[1:]
	case '+':
		w = w[1:]
	}
	name, rest := w, ""
	if i := strings.IndexAny(w, "= \t"); i >= 0 {
		name, rest = w[:i], strings.TrimSpace(strings.TrimLeft(w[i:], "= \t"))
	}
	name = strings.Trim(name, "\"'")
	if len(name) != 4 {
		return "", false, false
	}
	switch strings.ToLower(rest) {
	case "", "on":
	case "off":
		on = false
	default:
		if n, err := strconv.Atoi(rest); err == nil {
			on = on && n != 0
		}
	}
	return name, on, true
}

// FontFeatureOn reports whether the configuration turns a feature on: the
// last word about it decides.
func (t *Tree) FontFeatureOn(tag string) bool {
	on := false
	for _, f := range t.FontFeatures() {
		if name, state, ok := featureWord(f); ok && name == tag {
			on = state
		}
	}
	return on
}

// SetFontFeature turns an optional feature on or off. Off is no word at all,
// which is what Ghostty does with a feature nobody mentions; on is a line of
// its own, `font-feature = ss01`. Every other word about the feature goes,
// wherever it was written.
func (t *Tree) SetFontFeature(tag string, on bool) []*Document {
	t.mu.Lock()
	defer t.mu.Unlock()
	var changed []*Document
	t.record(func(t *Tree) { changed = t.setFontFeature(tag, on) })
	return changed
}

func (t *Tree) setFontFeature(tag string, on bool) []*Document {
	mentioned := false
	for _, f := range t.FontFeatures() {
		if name, _, ok := featureWord(f); ok && name == tag {
			mentioned = true
		}
	}
	if on == t.FontFeatureOn(tag) && (on || !mentioned) {
		return nil
	}
	var changed []*Document
	for _, e := range t.All("font-feature") {
		var kept []string
		dropped := false
		for _, w := range strings.Split(e.Value, ",") {
			if strings.TrimSpace(w) == "" {
				continue
			}
			if name, _, ok := featureWord(w); ok && name == tag {
				dropped = true
				continue
			}
			kept = append(kept, strings.TrimSpace(w))
		}
		if !dropped {
			continue
		}
		if len(kept) == 0 {
			e.Doc.CommentOut(e.Line, "disabled by ghostty-config: "+tag+" turned off")
		} else {
			e.Doc.SetLine(e.Line, "font-feature", strings.Join(kept, ", "))
		}
		changed = appendDoc(changed, e.Doc)
	}
	if on {
		t.Primary.Append("font-feature", tag)
		changed = appendDoc(changed, t.Primary)
	}
	t.index()
	return changed
}

// variationAxis reads the axis of a font-variation value, `wght=350`.
func variationAxis(v string) (tag, value string, ok bool) {
	tag, value, ok = strings.Cut(v, "=")
	return strings.Trim(strings.TrimSpace(tag), "\"'"), strings.TrimSpace(value), ok
}

// FontVariation is the value key gives an axis, the last line about it
// winning; key is font-variation or one of its -bold, -italic and
// -bold-italic forms.
func (t *Tree) FontVariation(key, tag string) (string, bool) {
	value, found := "", false
	for _, e := range t.All(key) {
		if axis, v, ok := variationAxis(e.Value); ok && axis == tag {
			value, found = v, true
		}
	}
	return value, found
}

// SetFontVariation sets an axis of a variable font, or with an empty value
// takes it back to the font's default. The last line about the axis is
// rewritten and earlier ones are switched off, so one line says it.
func (t *Tree) SetFontVariation(key, tag, value string) []*Document {
	t.mu.Lock()
	defer t.mu.Unlock()
	var changed []*Document
	t.record(func(t *Tree) { changed = t.setFontVariation(key, tag, value) })
	return changed
}

func (t *Tree) setFontVariation(key, tag, value string) []*Document {
	var lines []Entry
	for _, e := range t.All(key) {
		if axis, _, ok := variationAxis(e.Value); ok && axis == tag {
			lines = append(lines, e)
		}
	}
	want := tag + "=" + value
	reason := "disabled by ghostty-config: " + tag + " is set on a later line"
	if value == "" {
		reason = "disabled by ghostty-config: " + tag + " back to the font's default"
	}
	var changed []*Document
	for i, e := range lines {
		if value != "" && i == len(lines)-1 {
			if e.Value != want {
				e.Doc.SetLine(e.Line, key, want)
				changed = appendDoc(changed, e.Doc)
			}
			continue
		}
		e.Doc.CommentOut(e.Line, reason)
		changed = appendDoc(changed, e.Doc)
	}
	if value != "" && len(lines) == 0 {
		t.Primary.Append(key, want)
		changed = appendDoc(changed, t.Primary)
	}
	t.index()
	return changed
}

// codepointFont is the family a font-codepoint-map value maps to, `ranges=font`.
func codepointFont(v string) string {
	i := strings.LastIndex(v, "=")
	if i < 0 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(v[i+1:]), "\"")
}

// CodepointMapped reports whether any line maps code points to family.
func (t *Tree) CodepointMapped(family string) bool {
	for _, e := range t.All("font-codepoint-map") {
		if strings.EqualFold(codepointFont(e.Value), family) {
			return true
		}
	}
	return false
}

// SetCodepointMap maps ranges to family, or takes every mapping to family
// away. A mapping to it that is already there is kept as it is, with
// whatever ranges the user gave it.
func (t *Tree) SetCodepointMap(ranges, family string, on bool) []*Document {
	t.mu.Lock()
	defer t.mu.Unlock()
	var changed []*Document
	t.record(func(t *Tree) { changed = t.setCodepointMap(ranges, family, on) })
	return changed
}

func (t *Tree) setCodepointMap(ranges, family string, on bool) []*Document {
	if on {
		if t.CodepointMapped(family) {
			return nil
		}
		t.Primary.Append("font-codepoint-map", ranges+"="+family)
		t.index()
		return []*Document{t.Primary}
	}
	var changed []*Document
	for _, e := range t.All("font-codepoint-map") {
		if strings.EqualFold(codepointFont(e.Value), family) {
			e.Doc.CommentOut(e.Line, "disabled by ghostty-config: no longer mapped to "+family)
			changed = appendDoc(changed, e.Doc)
		}
	}
	t.index()
	return changed
}
