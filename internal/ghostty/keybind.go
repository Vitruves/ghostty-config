package ghostty

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Keyboard shortcuts. `keybind` is the most repeatable key Ghostty has: every
// line adds a binding on top of Ghostty's defaults, a later line for the same
// trigger replaces an earlier one, `trigger=unbind` removes one, and
// `keybind = clear` drops everything before it, defaults included. What is in
// effect is therefore worked out by replaying the lines over the defaults,
// and every edit here is described by values, triggers and actions, never by
// line numbers, so that it can be replayed onto a file someone else changed.

// Mods are the modifiers a chord holds. Ghostty knows four: hyper and meta
// are not among them.
type Mods uint8

const (
	ModShift Mods = 1 << iota
	ModCtrl
	ModAlt
	ModSuper
)

// Chord is one step of a trigger: the modifiers held and one key. Key is a
// single character, lower case, or a W3C key name in Ghostty's snake case
// (arrow_up, page_down, key_a, f5).
type Chord struct {
	Mods Mods
	Key  string
}

// Trigger is what sets a binding off: one chord, or a sequence of them
// typed one after the other (ctrl+a>n).
type Trigger []Chord

// Binding is one `keybind` value, read.
type Binding struct {
	// Table is the key table the binding belongs to, empty for the default
	// one. TableClear is a bare `name/`, which empties that table.
	Table      string
	TableClear bool
	// Prefixes are global, all, unconsumed and performable, in the order
	// they were written.
	Prefixes []string
	Trigger  Trigger
	// Action is the action with its parameter, as in goto_tab:3.
	Action string
	// Clear is `keybind = clear`, which drops every binding before it.
	Clear bool
	// Chain is `chain=action`: one more action for the binding above it.
	Chain bool
}

// keyPrefixes are the words a trigger may start with, each followed by `:`.
var keyPrefixes = []string{"global", "all", "unconsumed", "performable"}

// modWords are the modifier names Ghostty accepts, aliases included.
var modWords = map[string]Mods{
	"shift": ModShift,
	"ctrl":  ModCtrl, "control": ModCtrl,
	"alt": ModAlt, "opt": ModAlt, "option": ModAlt,
	"super": ModSuper, "cmd": ModSuper, "command": ModSuper,
}

// ParseBinding reads a `keybind` value the way Ghostty splits it: the first
// `=` that does not follow a `+` or a `>` ends the trigger, so `super+==x`
// binds the = key.
func ParseBinding(value string) (Binding, error) {
	v := strings.TrimSpace(value)
	if v == "clear" {
		return Binding{Clear: true}, nil
	}
	eq := -1
	for i := 1; i < len(v); i++ {
		if v[i] != '=' {
			continue
		}
		// After a separator the = is the key; a + or > that is itself
		// the key (super++) is no separator.
		if p := v[i-1]; (p == '+' || p == '>') && i >= 2 && v[i-2] != '+' && v[i-2] != '>' {
			continue
		}
		eq = i
		break
	}
	if eq < 0 {
		if name, ok := strings.CutSuffix(v, "/"); ok && name != "" && !strings.ContainsAny(name, "+=>/") {
			return Binding{Table: name, TableClear: true}, nil
		}
		return Binding{}, fmt.Errorf("%q has no action: write trigger=action", v)
	}
	trig, action := v[:eq], v[eq+1:]
	if action == "" {
		return Binding{}, fmt.Errorf("%q has no action after the =", v)
	}
	var b Binding
	b.Action = action
	if trig == "chain" {
		b.Chain = true
		return b, nil
	}
	// A table name holds none of + = > /, which tells it from a trigger such
	// as ctrl+/ whose key is a slash.
	if i := strings.IndexByte(trig, '/'); i > 0 && !strings.ContainsAny(trig[:i], "+=>") {
		b.Table, trig = trig[:i], trig[i+1:]
	}
	for found := true; found; {
		found = false
		for _, p := range keyPrefixes {
			if rest, ok := strings.CutPrefix(trig, p+":"); ok {
				b.Prefixes = append(b.Prefixes, p)
				trig, found = rest, true
			}
		}
	}
	t, err := ParseTrigger(trig)
	if err != nil {
		return Binding{}, err
	}
	if len(t) > 1 && (b.Has("global") || b.Has("all")) {
		return Binding{}, fmt.Errorf("a global: or all: shortcut cannot be a sequence")
	}
	b.Trigger = t
	return b, nil
}

// Has reports whether the binding carries a prefix.
func (b Binding) Has(prefix string) bool {
	for _, p := range b.Prefixes {
		if p == prefix {
			return true
		}
	}
	return false
}

// ActionName is the action without its parameter.
func (b Binding) ActionName() string { return ActionName(b.Action) }

// ActionName strips the parameter from an action: goto_tab:3 is goto_tab.
func ActionName(action string) string {
	name, _, _ := strings.Cut(action, ":")
	return name
}

// Value renders the binding as a `keybind` value, the trigger in Ghostty's
// own spelling.
func (b Binding) Value() string {
	switch {
	case b.Clear:
		return "clear"
	case b.Chain:
		return "chain=" + b.Action
	case b.TableClear:
		return b.Table + "/"
	}
	var s strings.Builder
	if b.Table != "" {
		s.WriteString(b.Table + "/")
	}
	for _, p := range b.Prefixes {
		s.WriteString(p + ":")
	}
	s.WriteString(b.Trigger.String() + "=" + b.Action)
	return s.String()
}

// ParseTrigger reads a trigger: chords joined by `>`, each a `+`-separated
// list of modifiers and exactly one key. A `+` or `>` right after a `+` is
// the key itself, as in super++.
func ParseTrigger(s string) (Trigger, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("the shortcut is empty")
	}
	var steps []string
	var cur strings.Builder
	for _, r := range s {
		if r == '>' && cur.Len() > 0 && !strings.HasSuffix(cur.String(), "+") {
			steps = append(steps, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
	}
	steps = append(steps, cur.String())
	var t Trigger
	for _, step := range steps {
		c, err := parseChord(step)
		if err != nil {
			return nil, err
		}
		t = append(t, c)
	}
	return t, nil
}

func parseChord(s string) (Chord, error) {
	if s == "" {
		return Chord{}, fmt.Errorf("a step of the sequence is empty")
	}
	var parts []string
	var cur strings.Builder
	for _, r := range s {
		if r == '+' && cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	var c Chord
	for _, p := range parts {
		if m, ok := modWords[strings.ToLower(p)]; ok {
			if c.Mods&m != 0 {
				return Chord{}, fmt.Errorf("%s is held twice in %q", p, s)
			}
			c.Mods |= m
			continue
		}
		if c.Key != "" {
			return Chord{}, fmt.Errorf("%q names two keys, %s and %s: a chord has one", s, c.Key, p)
		}
		key, ok := normalKey(p)
		if !ok {
			if strings.EqualFold(p, "hyper") || strings.EqualFold(p, "meta") || strings.EqualFold(p, "fn") {
				return Chord{}, fmt.Errorf("Ghostty has no %s modifier: use super, ctrl, alt or shift", strings.ToLower(p))
			}
			return Chord{}, fmt.Errorf("%q is not a key Ghostty knows", p)
		}
		c.Key = key
	}
	if c.Key == "" {
		return Chord{}, fmt.Errorf("%q holds modifiers but no key", s)
	}
	return c, nil
}

// normalKey spells a key the way Ghostty prints it: one character in lower
// case, W3C names in snake case (KeyA is key_a, F5 is f5), and the names
// older versions used (up, kp_add) as their current ones.
func normalKey(k string) (string, bool) {
	if utf8.RuneCountInString(k) == 1 {
		r, _ := utf8.DecodeRuneInString(k)
		if r == '>' || unicode.IsControl(r) || unicode.IsSpace(r) {
			return "", false
		}
		return string(unicode.ToLower(r)), true
	}
	name := snakeKey(k)
	if knownKeys[name] {
		return name, true
	}
	if modern, ok := legacyKeys[name]; ok {
		return modern, true
	}
	return "", false
}

// snakeKey turns a W3C code into Ghostty's snake case: ArrowUp is
// arrow_up, Digit1 is digit_1, but F1 stays f1.
func snakeKey(k string) string {
	lower := strings.ToLower(k)
	if lower == k {
		return k
	}
	if len(k) >= 2 && (k[0] == 'F' || k[0] == 'f') && strings.Trim(k[1:], "0123456789") == "" {
		return lower
	}
	var b strings.Builder
	prev := rune(0)
	for i, r := range k {
		if i > 0 && (unicode.IsUpper(r) || (unicode.IsDigit(r) && !unicode.IsDigit(prev))) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
		prev = r
	}
	return b.String()
}

// knownKeys are the key names Ghostty 1.3 accepts, as its documentation
// lists them after the W3C UI Events codes.
var knownKeys = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range strings.Fields(`backquote backslash bracket_left bracket_right comma equal
		intl_backslash intl_ro intl_yen minus period quote semicolon slash
		alt_left alt_right backspace caps_lock context_menu control_left control_right enter
		meta_left meta_right shift_left shift_right space tab convert kana_mode non_convert
		delete end help home insert page_down page_up arrow_down arrow_left arrow_right arrow_up
		num_lock numpad_add numpad_backspace numpad_clear numpad_clear_entry numpad_comma
		numpad_decimal numpad_divide numpad_enter numpad_equal numpad_memory_add
		numpad_memory_clear numpad_memory_recall numpad_memory_store numpad_memory_subtract
		numpad_multiply numpad_paren_left numpad_paren_right numpad_subtract numpad_separator
		numpad_up numpad_down numpad_right numpad_left numpad_begin numpad_home numpad_end
		numpad_insert numpad_delete numpad_page_up numpad_page_down escape fn fn_lock
		print_screen scroll_lock pause browser_back browser_favorites browser_forward
		browser_home browser_refresh browser_search browser_stop eject launch_app_1
		launch_app_2 launch_mail media_play_pause media_select media_stop media_track_next
		media_track_previous power sleep audio_volume_down audio_volume_mute audio_volume_up
		wake_up copy cut paste catch_all`) {
		m[k] = true
	}
	for c := 'a'; c <= 'z'; c++ {
		m["key_"+string(c)] = true
	}
	for d := 0; d <= 9; d++ {
		m[fmt.Sprintf("digit_%d", d)] = true
		m[fmt.Sprintf("numpad_%d", d)] = true
	}
	for f := 1; f <= 25; f++ {
		m[fmt.Sprintf("f%d", f)] = true
	}
	return m
}()

// legacyKeys are names from before Ghostty 1.2 that it still reads.
var legacyKeys = func() map[string]string {
	m := map[string]string{
		"up": "arrow_up", "down": "arrow_down", "left": "arrow_left", "right": "arrow_right",
		"grave_accent": "backquote", "left_bracket": "bracket_left", "right_bracket": "bracket_right",
		"apostrophe": "quote", "plus": "+",
		"left_shift": "shift_left", "right_shift": "shift_right",
		"left_control": "control_left", "right_control": "control_right",
		"left_alt": "alt_left", "right_alt": "alt_right",
		"left_super": "meta_left", "right_super": "meta_right",
		"kp_add": "numpad_add", "kp_subtract": "numpad_subtract", "kp_multiply": "numpad_multiply",
		"kp_divide": "numpad_divide", "kp_decimal": "numpad_decimal", "kp_enter": "numpad_enter",
		"kp_equal": "numpad_equal",
	}
	for d, w := range strings.Fields("zero one two three four five six seven eight nine") {
		m[w] = fmt.Sprintf("digit_%d", d)
		m[fmt.Sprintf("kp_%d", d)] = fmt.Sprintf("numpad_%d", d)
	}
	return m
}()

// printedAs is the character a physical key types on a US layout. Ghostty
// tries physical keys first, so super+digit_1 and super+1 are one shortcut
// in practice, and the physical one wins.
var printedAs = func() map[string]string {
	m := map[string]string{
		"comma": ",", "period": ".", "slash": "/", "semicolon": ";", "quote": "'",
		"bracket_left": "[", "bracket_right": "]", "backslash": "\\", "minus": "-",
		"equal": "=", "backquote": "`",
	}
	for c := 'a'; c <= 'z'; c++ {
		m["key_"+string(c)] = string(c)
	}
	for d := 0; d <= 9; d++ {
		m[fmt.Sprintf("digit_%d", d)] = fmt.Sprint(d)
	}
	return m
}()

// String spells the chord as Ghostty prints it: super, ctrl, alt, shift,
// then the key.
func (c Chord) String() string {
	var b strings.Builder
	for _, m := range []struct {
		bit  Mods
		word string
	}{{ModSuper, "super"}, {ModCtrl, "ctrl"}, {ModAlt, "alt"}, {ModShift, "shift"}} {
		if c.Mods&m.bit != 0 {
			b.WriteString(m.word + "+")
		}
	}
	b.WriteString(c.Key)
	return b.String()
}

// Same reports whether two chords are pressed the same way, a physical key
// and the character it types counting as one.
func (c Chord) Same(o Chord) bool {
	return c.Mods == o.Mods && c.typed() == o.typed()
}

func (c Chord) typed() string {
	if p, ok := printedAs[c.Key]; ok {
		return p
	}
	return c.Key
}

// String spells the trigger as Ghostty prints it.
func (t Trigger) String() string {
	parts := make([]string, len(t))
	for i, c := range t {
		parts[i] = c.String()
	}
	return strings.Join(parts, ">")
}

// Same reports whether two triggers are pressed the same way.
func (t Trigger) Same(o Trigger) bool {
	if len(t) != len(o) {
		return false
	}
	for i := range t {
		if !t[i].Same(o[i]) {
			return false
		}
	}
	return true
}

// prefixOf reports whether t is a shorter sequence that o starts with.
func (t Trigger) prefixOf(o Trigger) bool {
	return len(t) < len(o) && t.Same(o[:len(t)])
}

// --- what is in effect ------------------------------------------------------------

// Shortcut is a binding in effect.
type Shortcut struct {
	Binding
	// User is whether the configuration set it, rather than Ghostty.
	User bool
	// Entry is the line that set it, for one of the user's.
	Entry Entry
	// Chains are the actions `chain=` lines add after the first.
	Chains []string
}

// KeyMap is every binding in effect in the default key table, in the order
// they were set.
type KeyMap struct {
	Active []Shortcut
	// Cleared is whether a `keybind = clear` dropped Ghostty's defaults.
	Cleared bool
}

// KeyMap replays the configuration's keybind lines over Ghostty's defaults.
// Lines that do not parse are skipped, as Ghostty skips them; key tables are
// left out, since they are not in effect until an action activates them.
func (t *Tree) KeyMap(defaults []Binding) KeyMap {
	var km KeyMap
	for _, d := range defaults {
		km.set(Shortcut{Binding: d})
	}
	last := -1
	for _, e := range t.All("keybind") {
		b, err := ParseBinding(e.Value)
		if err != nil {
			continue
		}
		switch {
		case b.Clear:
			km.Active, km.Cleared, last = nil, true, -1
		case b.Chain:
			if last >= 0 {
				km.Active[last].Chains = append(km.Active[last].Chains, b.Action)
			}
		case b.Table != "" || b.TableClear:
			last = -1
		case b.Action == "unbind":
			km.remove(func(s Shortcut) bool { return s.Trigger.String() == b.Trigger.String() })
			last = -1
		default:
			last = km.set(Shortcut{Binding: b, User: true, Entry: e})
		}
	}
	return km
}

// set adds a binding the way Ghostty does: it replaces the one with the same
// trigger, and a sequence and the chord it starts with cannot both be bound,
// so the newer takes over from the older. It returns where the binding sits.
func (km *KeyMap) set(s Shortcut) int {
	key := s.Trigger.String()
	km.remove(func(o Shortcut) bool {
		ot := o.Trigger.String()
		return ot == key || o.Trigger.prefixOf(s.Trigger) || s.Trigger.prefixOf(o.Trigger)
	})
	km.Active = append(km.Active, s)
	return len(km.Active) - 1
}

func (km *KeyMap) remove(match func(Shortcut) bool) {
	kept := km.Active[:0]
	for _, s := range km.Active {
		if !match(s) {
			kept = append(kept, s)
		}
	}
	km.Active = kept
}

// For lists the shortcuts in effect for an action, parameter included.
func (km KeyMap) For(action string) []Shortcut {
	var out []Shortcut
	for _, s := range km.Active {
		if s.Action == action {
			out = append(out, s)
		}
	}
	return out
}

// Lookup finds what a trigger does now: the binding with exactly that
// spelling, or else one pressed the same way.
func (km KeyMap) Lookup(t Trigger) (Shortcut, bool) {
	var near *Shortcut
	for i, s := range km.Active {
		if s.Trigger.String() == t.String() {
			return s, true
		}
		if near == nil && s.Trigger.Same(t) {
			near = &km.Active[i]
		}
	}
	if near != nil {
		return *near, true
	}
	return Shortcut{}, false
}

// Conflicts lists what binding t to action would take from other actions:
// the bindings pressed the same way, and a chord a new sequence starts with,
// or the sequences a new chord starts.
func (km KeyMap) Conflicts(t Trigger, action string) []Shortcut {
	var out []Shortcut
	for _, s := range km.Active {
		if s.Action == action {
			continue
		}
		if s.Trigger.Same(t) || s.Trigger.prefixOf(t) || t.prefixOf(s.Trigger) {
			out = append(out, s)
		}
	}
	return out
}

// Shadows lists the bindings that would keep working after t is bound to
// action because they are spelled differently: super+digit_1 outlives a new
// super+1, and is the one Ghostty picks. They are what must be unbound.
func (km KeyMap) Shadows(t Trigger, action string) []Trigger {
	var out []Trigger
	for _, s := range km.Active {
		if !s.User && s.Action != action && s.Trigger.Same(t) && s.Trigger.String() != t.String() {
			out = append(out, s.Trigger)
		}
	}
	return out
}

// --- the user's lines ---------------------------------------------------------------

// bindLine is one of the user's bindings in effect, with the chain lines that
// follow it.
type bindLine struct {
	e      Entry
	b      Binding
	chains []Entry
}

// keyLines lists the user's bindings that still count: those after the
// last `keybind = clear`, in the default table, in load order.
func (t *Tree) keyLines() []bindLine {
	var out []bindLine
	last := -1
	for _, e := range t.All("keybind") {
		b, err := ParseBinding(e.Value)
		if err != nil {
			continue
		}
		switch {
		case b.Clear:
			out, last = nil, -1
		case b.Chain:
			if last >= 0 {
				out[last].chains = append(out[last].chains, e)
			}
		case b.Table != "" || b.TableClear:
			last = -1
		default:
			out = append(out, bindLine{e: e, b: b})
			last = len(out) - 1
		}
	}
	return out
}

// UserShortcuts lists the bindings the configuration sets that still count,
// unbinds included, in load order.
func (t *Tree) UserShortcuts() []Binding {
	var out []Binding
	for _, l := range t.keyLines() {
		out = append(out, l.b)
	}
	return out
}

// Reasons the shortcut editor gives the lines it switches off.
const (
	reasonKeyReplaced = "disabled by ghostty-config: replaced by a new shortcut"
	reasonKeyDefault  = "disabled by ghostty-config: back to Ghostty's default shortcut"
	reasonKeyRemoved  = "disabled by ghostty-config: shortcut removed"
)

// disable comments a binding out with the chain lines that belong to it, so
// they never attach to the binding above.
func (l bindLine) disable(reason string) {
	l.e.Doc.CommentOut(l.e.Line, reason)
	l.dropChains(reason)
}

func (l bindLine) dropChains(reason string) {
	for _, c := range l.chains {
		c.Doc.CommentOut(c.Line, reason)
	}
}

// keyInsertAt is where a new binding goes: after the user's last binding
// still in effect and its chain lines, so the bindings stay together and in
// the order the user wrote them, or at the end of the primary file when the
// configuration has none.
func (t *Tree) keyInsertAt(lines []bindLine) (*Document, int) {
	if len(lines) == 0 {
		return t.Primary, -1
	}
	l := lines[len(lines)-1]
	return l.e.Doc, l.e.lineEnd(l.chains)
}

// insertKey writes keybind = value at a place keyInsertAt chose, and
// returns the line it landed on.
func insertKey(doc *Document, at int, value string) int {
	if at < 0 {
		return doc.Append("keybind", value)
	}
	return doc.InsertAfter(at, "keybind", value)
}

// SetShortcut makes b the user's shortcut for its action. A line of the
// user's that binds the same trigger, to anything, or that binds the action
// to another trigger, is rewritten in place; the other such lines are
// switched off, so the action ends with this one shortcut of the user's
// beside Ghostty's own. With no line to rewrite, the binding is added next
// to the user's other bindings. unbind lists defaults spelled differently
// that would still win (see KeyMap.Shadows); each gets an unbind line.
func (t *Tree) SetShortcut(b Binding, unbind []Trigger) []*Document {
	t.mu.Lock()
	defer t.mu.Unlock()
	var changed []*Document
	t.record(func(t *Tree) { changed = t.setShortcut(b, unbind) })
	return changed
}

func (t *Tree) setShortcut(b Binding, unbind []Trigger) []*Document {
	lines := t.keyLines()
	var changed []*Document
	var target *bindLine
	for i := range lines {
		l := lines[i]
		same := l.b.Trigger.Same(b.Trigger)
		own := l.b.Action == b.Action
		if !same && !own {
			continue
		}
		if target == nil {
			target = &lines[i]
			continue
		}
		l.disable(reasonKeyReplaced)
		changed = appendDoc(changed, l.e.Doc)
	}
	var doc *Document
	at := -1
	if target != nil {
		doc, at = target.e.Doc, target.e.lineEnd(target.chains)
		if target.b.Value() != b.Value() {
			doc.SetLine(target.e.Line, "keybind", b.Value())
			changed = appendDoc(changed, doc)
		}
		if target.b.Action != b.Action {
			// The chain belonged to the action the trigger used to run.
			target.dropChains(reasonKeyReplaced)
		}
		t.index()
	} else {
		doc, at = t.keyInsertAt(lines)
		at = insertKey(doc, at, b.Value())
		changed = appendDoc(changed, doc)
		t.index()
	}
	return t.addUnbinds(doc, at, unbind, changed)
}

// addUnbinds writes an unbind line for each trigger no line unbinds yet,
// one after the other from line at of doc.
func (t *Tree) addUnbinds(doc *Document, at int, unbind []Trigger, changed []*Document) []*Document {
	for _, u := range unbind {
		if t.hasUnbind(u) {
			continue
		}
		at = insertKey(doc, at, Binding{Trigger: u, Action: "unbind"}.Value())
		changed = appendDoc(changed, doc)
		t.index()
	}
	return changed
}

// lineEnd is the last line of a binding and of the chain lines under it in
// the same file.
func (e Entry) lineEnd(chains []Entry) int {
	at := e.Line
	for _, c := range chains {
		if c.Doc == e.Doc && c.Line > at {
			at = c.Line
		}
	}
	return at
}

// hasUnbind reports whether a line already unbinds exactly this trigger.
func (t *Tree) hasUnbind(u Trigger) bool {
	for _, l := range t.keyLines() {
		if l.b.Action == "unbind" && l.b.Trigger.String() == u.String() {
			return true
		}
	}
	return false
}

// ResetShortcuts puts an action back to Ghostty's defaults: the user's lines
// that bind it are switched off, and so are the lines that unbind one of
// defaults, the action's own default triggers.
func (t *Tree) ResetShortcuts(action string, defaults []Trigger) []*Document {
	t.mu.Lock()
	defer t.mu.Unlock()
	var changed []*Document
	t.record(func(t *Tree) { changed = t.resetShortcuts(action, defaults) })
	return changed
}

func (t *Tree) resetShortcuts(action string, defaults []Trigger) []*Document {
	var changed []*Document
	for _, l := range t.keyLines() {
		drop := l.b.Action == action
		if l.b.Action == "unbind" {
			for _, d := range defaults {
				drop = drop || l.b.Trigger.Same(d)
			}
		}
		if drop {
			l.disable(reasonKeyDefault)
			changed = appendDoc(changed, l.e.Doc)
		}
	}
	t.index()
	return changed
}

// RemoveShortcut takes the trigger tr away from action: the user's lines
// that bind it are switched off, and each of Ghostty's defaults in unbind,
// the action's default triggers pressed like tr, gets an unbind line.
func (t *Tree) RemoveShortcut(action string, tr Trigger, unbind []Trigger) []*Document {
	t.mu.Lock()
	defer t.mu.Unlock()
	var changed []*Document
	t.record(func(t *Tree) { changed = t.removeShortcut(action, tr, unbind) })
	return changed
}

func (t *Tree) removeShortcut(action string, tr Trigger, unbind []Trigger) []*Document {
	var changed []*Document
	lines := t.keyLines()
	for _, l := range lines {
		if l.b.Action == action && l.b.Trigger.Same(tr) {
			l.disable(reasonKeyRemoved)
			changed = appendDoc(changed, l.e.Doc)
		}
	}
	t.index()
	doc, at := t.keyInsertAt(t.keyLines())
	return t.addUnbinds(doc, at, unbind, changed)
}

// SetShortcutPrefix adds or removes a prefix, global: say, on every line of
// the user's that binds action.
func (t *Tree) SetShortcutPrefix(action, prefix string, on bool) []*Document {
	t.mu.Lock()
	defer t.mu.Unlock()
	var changed []*Document
	t.record(func(t *Tree) { changed = t.setShortcutPrefix(action, prefix, on) })
	return changed
}

func (t *Tree) setShortcutPrefix(action, prefix string, on bool) []*Document {
	var changed []*Document
	for _, l := range t.keyLines() {
		if l.b.Action != action || l.b.Has(prefix) == on {
			continue
		}
		b := l.b
		b.Prefixes = nil
		for _, p := range l.b.Prefixes {
			if p != prefix {
				b.Prefixes = append(b.Prefixes, p)
			}
		}
		if on {
			b.Prefixes = append([]string{prefix}, b.Prefixes...)
		}
		l.e.Doc.SetLine(l.e.Line, "keybind", b.Value())
		changed = appendDoc(changed, l.e.Doc)
	}
	t.index()
	return changed
}
