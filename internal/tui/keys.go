package tui

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The shortcut editor. `keys` lists what Ghostty can do from the keyboard,
// grouped the way its menus are, each action with the shortcuts in effect:
// Ghostty's own and the user's. Enter asks for a new shortcut, pressed or
// typed; Delete puts Ghostty's back; Ctrl+X unbinds; Ctrl+G makes the quick
// terminal's shortcut work from any app. Every change is a keybind line,
// written next to the user's own.

// keysMac is whether shortcuts are spelled with the Mac's symbols, ⌘ for
// super; elsewhere they are words.
var keysMac = runtime.GOOS == "darwin"

// loadKeyCatalog asks Ghostty for its actions and defaults; a variable so
// tests never run a binary.
var loadKeyCatalog = ghostty.LoadKeyCatalog

type keysLoadedMsg struct{ cat ghostty.KeyCatalog }

// keyboard is what the shortcut editor knows and what it is asking for.
type keyboard struct {
	cat             ghostty.KeyCatalog
	loaded, loading bool
	action, text    string // the capture: for which action, what is typed
	global          bool
	err             string
	// rows are the list as keyRows last built it, and users the config's
	// own bindings, kept while rowsFor says the same.
	rows    []*keyRow
	users   []ghostty.Binding
	rowsFor keyRowsKey
}

// keyRowsKey is what the list of actions is built from: the config's
// entries, the catalog, how keys are spelled.
type keyRowsKey struct {
	tree              *ghostty.Tree
	rev               uint64
	actions, defaults int
	firstDef          *ghostty.Binding
	firstDoc          *ghostty.ActionDoc
	symbols, loaded   bool
	mac               bool
}

// keysFresh is whether the list kept is still the one keyRows would build,
// and forgets it when not.
func (m *Model) keysFresh() bool {
	k := keyRowsKey{tree: m.tree, rev: m.tree.Rev(), actions: len(m.kb.cat.Actions), defaults: len(m.kb.cat.Defaults),
		symbols: m.keySymbols(), loaded: m.kb.loaded, mac: keysMac}
	if len(m.kb.cat.Defaults) > 0 {
		k.firstDef = &m.kb.cat.Defaults[0]
	}
	if len(m.kb.cat.Actions) > 0 {
		k.firstDoc = &m.kb.cat.Actions[0]
	}
	if k == m.kb.rowsFor {
		return true
	}
	m.kb.rows, m.kb.users, m.kb.rowsFor = nil, nil, k
	return false
}

// userShortcuts is the config's own bindings, m.tree.UserShortcuts() kept
// until the config changes. The slice is shared: it is not to be changed.
func (m *Model) userShortcuts() []ghostty.Binding {
	if !m.keysFresh() || m.kb.users == nil {
		m.kb.users = m.tree.UserShortcuts()
		if m.kb.users == nil {
			m.kb.users = []ghostty.Binding{}
		}
	}
	return m.kb.users
}

// keyRow is one action of the list, with what it is bound to now.
type keyRow struct {
	action, label, group, summary string
	shortcuts                     []ghostty.Shortcut
	// defaults are Ghostty's own triggers for it, in effect or not.
	defaults []ghostty.Trigger
	// hay is what matches searches, in lower case, once it was asked for.
	hay string
}

// keyGroups orders the actions as Ghostty's menus do, in their common
// forms: an action that takes a value is listed with the values people bind,
// goto_tab:1 to 9 and the four directions of a split, not with every value it
// could take. Forms bound in the config or by default join their action's
// group whatever they are.
var keyGroups = [][2]string{
	{"Tabs", "new_tab close_tab:this previous_tab next_tab last_tab goto_tab:1 goto_tab:2 goto_tab:3 goto_tab:4 goto_tab:5 goto_tab:6 goto_tab:7 goto_tab:8 goto_tab:9 move_tab:-1 move_tab:1 move_tab_to_new_window prompt_tab_title toggle_tab_overview"},
	{"Splits", "new_split:right new_split:down new_split:left new_split:up new_split:auto goto_split:previous goto_split:next goto_split:up goto_split:down goto_split:left goto_split:right toggle_split_zoom equalize_splits resize_split:up,10 resize_split:down,10 resize_split:left,10 resize_split:right,10"},
	{"Windows", "new_window close_surface close_window close_all_windows goto_window:previous goto_window:next toggle_fullscreen toggle_maximize toggle_window_decorations toggle_window_float_on_top toggle_background_opacity reset_window_size prompt_surface_title prompt_window_title toggle_readonly"},
	{"Quick terminal", "toggle_quick_terminal toggle_visibility"},
	{"Clipboard", "copy_to_clipboard:mixed paste_from_clipboard paste_from_selection copy_url_to_clipboard copy_title_to_clipboard select_all"},
	{"Selection", "adjust_selection:left adjust_selection:right adjust_selection:up adjust_selection:down adjust_selection:page_up adjust_selection:page_down adjust_selection:home adjust_selection:end adjust_selection:beginning_of_line adjust_selection:end_of_line"},
	{"Font size", "increase_font_size:1 decrease_font_size:1 reset_font_size"},
	{"Scrolling", "scroll_to_top scroll_to_bottom scroll_page_up scroll_page_down scroll_to_selection jump_to_prompt:-1 jump_to_prompt:1"},
	{"Search", "start_search search_selection navigate_search:next navigate_search:previous end_search"},
	{"Screen", "clear_screen reset write_screen_file:open write_scrollback_file:open write_selection_file:open toggle_mouse_reporting"},
	{"Config", "open_config reload_config"},
	{"App", "toggle_command_palette undo redo inspector:toggle toggle_secure_input check_for_updates quit show_gtk_inspector show_on_screen_keyboard"},
	{"Other", "end_key_sequence deactivate_key_table deactivate_all_key_tables"},
}

// keyLabels name the common forms the way a menu would.
var keyLabels = map[string]string{
	"close_tab:this": "Close tab", "move_tab:-1": "Move tab left", "move_tab:1": "Move tab right", "move_tab_to_new_window": "Move tab to a new window",
	"prompt_tab_title": "Rename tab", "toggle_tab_overview": "Tab overview",
	"new_split:auto": "Split along the longer side", "goto_split:previous": "Previous split", "goto_split:next": "Next split",
	"goto_split:up": "Split above", "goto_split:down": "Split below", "goto_split:left": "Split on the left", "goto_split:right": "Split on the right",
	"toggle_split_zoom": "Zoom split", "close_surface": "Close",
	"goto_window:previous": "Previous window", "goto_window:next": "Next window", "toggle_fullscreen": "Full screen",
	"toggle_maximize": "Maximize", "toggle_window_decorations": "Window decorations", "toggle_window_float_on_top": "Float on top",
	"toggle_background_opacity": "Background opacity", "prompt_surface_title": "Rename terminal", "prompt_window_title": "Rename window", "toggle_readonly": "Read-only",
	"toggle_quick_terminal": "Quick terminal", "toggle_visibility": "Show or hide Ghostty",
	"copy_to_clipboard": "Copy", "copy_to_clipboard:mixed": "Copy", "paste_from_clipboard": "Paste", "paste_from_selection": "Paste the selection",
	"copy_url_to_clipboard": "Copy link", "copy_title_to_clipboard": "Copy title",
	"increase_font_size:1": "Bigger text", "decrease_font_size:1": "Smaller text", "reset_font_size": "Text size back to the config's",
	"jump_to_prompt:-1": "Previous prompt", "jump_to_prompt:1": "Next prompt",
	"start_search": "Find", "search_selection": "Find the selection", "navigate_search:next": "Next match",
	"navigate_search:previous": "Previous match", "end_search": "Close find",
	"reset": "Reset the terminal", "toggle_mouse_reporting": "Mouse reporting", "open_config": "Open the config", "reload_config": "Reload the config",
	"toggle_command_palette": "Command palette", "inspector:toggle": "Terminal inspector", "toggle_secure_input": "Secure input",
	"show_gtk_inspector": "GTK inspector", "show_on_screen_keyboard": "On-screen keyboard",
}

// keyLabel names an action form.
func keyLabel(action string) string {
	if l, ok := keyLabels[action]; ok {
		return l
	}
	name, param, _ := strings.Cut(action, ":")
	switch name {
	case "goto_tab":
		return "Go to tab " + param
	case "new_split":
		return "Split " + param
	case "resize_split":
		dir, _, _ := strings.Cut(param, ",")
		return "Resize split " + dir
	case "adjust_selection":
		return "Extend selection " + strings.ReplaceAll(param, "_", " ")
	case "write_screen_file", "write_scrollback_file", "write_selection_file":
		what := strings.TrimSuffix(strings.TrimPrefix(name, "write_"), "_file")
		mode, _, _ := strings.Cut(param, ",")
		switch mode {
		case "copy":
			return "Copy the " + what + " as a file"
		case "paste":
			return "Paste the " + what + " as a file"
		}
		return "Open the " + what + " in an editor"
	case "text", "csi", "esc":
		return "Send " + name + " " + param
	}
	l := humanize(name)
	if param != "" {
		l += " " + humanize(param)
	}
	return l
}

// humanize turns a snake_case word into a phrase with a capital.
func humanize(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// keyValueActions take a value nobody can guess, so they are only listed
// once something binds them.
var keyValueActions = map[string]bool{
	"text": true, "csi": true, "esc": true, "cursor_key": true, "set_font_size": true, "scroll_to_row": true,
	"scroll_page_fractional": true, "scroll_page_lines": true, "search": true, "set_surface_title": true,
	"set_tab_title": true, "activate_key_table": true, "activate_key_table_once": true, "crash": true,
	"ignore": true, "unbind": true,
}

// availableHere reads an action's documentation for the platforms it works
// on, so that macOS is not offered Linux's tab overview and the reverse.
// Only the opening paragraphs say so: further down, the quick terminal's
// notes mention what is "only supported on Linux" about its tabs.
func availableHere(doc string) bool {
	paras := strings.SplitN(doc, "\n\n", 4)
	if len(paras) > 3 {
		paras = paras[:3]
	}
	d := strings.ToLower(strings.Join(paras, " "))
	if keysMac {
		return !strings.Contains(d, "only implemented on linux") && !strings.Contains(d, "only supported on linux") && !strings.Contains(d, "has no effect on macos") && !strings.Contains(d, "no effect on either")
	}
	return !strings.Contains(d, "only implemented on macos")
}

// globalAction is whether an action makes sense from any app: the quick
// terminal and showing Ghostty. Global shortcuts are offered on macOS.
func globalAction(action string) bool {
	return keysMac && (action == "toggle_quick_terminal" || action == "toggle_visibility")
}

// keyMap is the shortcuts in effect now.
func (m *Model) keyMap() ghostty.KeyMap {
	return m.tree.KeyMap(m.kb.cat.Defaults)
}

// ensureKeys reads the actions from Ghostty the first time the list opens.
func (m *Model) ensureKeys() tea.Cmd {
	if m.kb.loaded || m.kb.loading {
		return nil
	}
	m.kb.loading = true
	binary := m.paths.Binary
	return func() tea.Msg { return keysLoadedMsg{loadKeyCatalog(binary)} }
}

// keyRows builds the list: every common form of every action this platform
// has, and every form something binds, in their groups.
func (m *Model) keyRows() []*keyRow {
	if m.keysFresh() && m.kb.rows != nil {
		return m.kb.rows
	}
	rows := m.buildKeyRows()
	if rows == nil {
		rows = []*keyRow{}
	}
	m.kb.rows = rows
	return rows
}

func (m *Model) buildKeyRows() []*keyRow {
	km := m.keyMap()
	cat := m.kb.cat
	bound := map[string]bool{}
	for _, s := range km.Active {
		bound[s.Action] = true
	}
	for _, d := range cat.Defaults {
		bound[d.Action] = true
	}
	for _, b := range m.userShortcuts() {
		bound[b.Action] = true
	}
	delete(bound, "unbind")

	groupOf := map[string]string{}
	var forms [][2]string // group, action
	listed := map[string]bool{}
	for _, g := range keyGroups {
		for _, a := range strings.Fields(g[1]) {
			groupOf[ghostty.ActionName(a)] = g[0]
			forms = append(forms, [2]string{g[0], a})
			listed[a] = true
		}
	}
	// Forms bound but not listed join their action's group; the actions
	// no group names go to the last one.
	for _, d := range cat.Actions {
		if _, ok := groupOf[d.Name]; !ok && !keyValueActions[d.Name] {
			forms = append(forms, [2]string{"Other", d.Name})
			listed[d.Name] = true
			groupOf[d.Name] = "Other"
		}
	}
	var extra []string
	for a := range bound {
		if !listed[a] {
			extra = append(extra, a)
		}
	}
	sort.Strings(extra)
	for _, a := range extra {
		g := groupOf[ghostty.ActionName(a)]
		if g == "" {
			g = "Other"
		}
		forms = append(forms, [2]string{g, a})
	}

	var rows []*keyRow
	for _, g := range keyGroups {
		for _, f := range forms {
			if f[0] != g[0] {
				continue
			}
			a := f[1]
			doc, known := cat.Action(a)
			if !bound[a] {
				if !known || !availableHere(doc.Doc) || strings.Contains(strings.ToLower(doc.Doc), "deprecated") {
					continue
				}
				if m.boundOtherwise(a, bound) {
					continue
				}
			}
			rows = append(rows, &keyRow{
				action: a, label: keyLabel(a), group: g[0], summary: doc.Summary,
				shortcuts: km.For(a), defaults: cat.DefaultsFor(a),
			})
		}
	}
	return rows
}

// boundOtherwise reports whether a common form is better left out because
// the action is bound with a fuller value: Ghostty binds close_tab:this, so
// a bare close_tab beside it would only say the same thing twice.
func (m *Model) boundOtherwise(form string, bound map[string]bool) bool {
	name, param, _ := strings.Cut(form, ":")
	for b := range bound {
		bn, bp, _ := strings.Cut(b, ":")
		if bn == name && b != form && (param == "" || strings.HasPrefix(bp, param+",")) {
			return true
		}
	}
	return false
}

// keyOptions is the list for the prompt: the rows that match what is
// typed, under the titles of their groups. A typed `trigger=action` is a
// binding to write as it is.
func (m *Model) keyOptions(arg string) []option {
	if !m.kb.loaded {
		return []option{{label: "Reading Ghostty's actions…", header: true}}
	}
	needle := strings.TrimSpace(arg)
	if strings.Contains(needle, "=") {
		b, err := ghostty.ParseBinding(needle)
		if err != nil || b.Clear || b.Chain || b.Table != "" {
			msg := "not a binding Ghostty reads"
			if err != nil {
				msg = err.Error()
			}
			return []option{{label: "Write it as trigger=action", detail: msg, header: true}}
		}
		row := &keyRow{action: b.Action, label: "Bind " + m.triggerText(b.Trigger) + " to " + keyLabel(b.Action), group: "Your binding"}
		return []option{{label: row.label, detail: b.Value(), value: "@bind", key: row}}
	}
	var typed ghostty.Trigger
	if strings.ContainsAny(needle, "+>") {
		typed, _ = ghostty.ParseTrigger(needle)
	}
	words := strings.Fields(strings.ToLower(needle))
	var out []option
	group := ""
	for _, r := range m.keyRows() {
		if needle != "" && !r.matches(m, words, typed) {
			continue
		}
		if r.group != group {
			group = r.group
			out = append(out, option{label: group, header: true})
		}
		out = append(out, option{label: r.label, detail: m.shortcutsText(r.shortcuts), value: r.action, current: m.keysCustomised(r), key: r})
	}
	return out
}

// matches reports whether every typed word is found in the row's name,
// action, description or shortcuts, or the typed trigger is one of its
// shortcuts.
func (r *keyRow) matches(m *Model, words []string, typed ghostty.Trigger) bool {
	if typed != nil {
		for _, s := range r.shortcuts {
			if s.Trigger.Same(typed) {
				return true
			}
		}
	}
	if r.hay == "" {
		var hay strings.Builder
		hay.WriteString(strings.ToLower(r.label + " " + r.action + " " + strings.ReplaceAll(r.action, "_", " ") + " " + r.summary + " " + r.group))
		for _, s := range r.shortcuts {
			t := s.Trigger.String()
			hay.WriteString(" " + t + " " + strings.ReplaceAll(t, "super+", "cmd+") + " " + strings.ToLower(m.triggerText(s.Trigger)))
		}
		r.hay = hay.String()
	}
	h := r.hay
	for _, w := range words {
		if !strings.Contains(h, w) {
			return false
		}
	}
	return true
}

// keysCommand is the `keys` command.
func keysCommand() *command {
	return &command{name: "keys", group: "Input", syntax: "keys [action or shortcut]",
		desc:    "Keyboard shortcuts: what each Ghostty action is bound to, yours and Ghostty's; Enter sets a new one",
		options: func(m *Model, arg string) []option { return m.keyOptions(arg) },
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if opt == nil || opt.key == nil {
				return nil
			}
			if opt.value == "@bind" {
				b, err := ghostty.ParseBinding(strings.TrimSpace(arg))
				if err != nil {
					m.warn("%v", err)
					return nil
				}
				m.setPromptText("Keys ")
				return m.offerBinding(b)
			}
			m.startCapture(opt.key.action)
			return nil
		},
		preview: previewKeysPanel,
	}
}

// keysValue is what the menu says of the shortcuts: how many are the user's.
func (m *Model) keysValue() string {
	n := 0
	for _, b := range m.tree.UserShortcuts() {
		if b.Action != "unbind" {
			n++
		}
	}
	switch n {
	case 0:
		return "Ghostty's"
	case 1:
		return "1 of yours"
	}
	return fmt.Sprintf("%d of yours", n)
}

// selectedKey is the highlighted row of the list, nil elsewhere.
func (m *Model) selectedKey() *keyRow {
	if m.cmd == nil || m.cmd.name != "keys" {
		return nil
	}
	if o := m.selected(); o != nil && o.key != nil && o.value != "@bind" {
		return o.key
	}
	return nil
}

// keysKey handles the keys the list adds to the prompt's: Delete for the
// defaults, Ctrl+X to unbind, Ctrl+G for a global shortcut.
func (m *Model) keysKey(key string) (tea.Cmd, bool) {
	r := m.selectedKey()
	if r == nil {
		return nil, false
	}
	switch key {
	case "delete":
		if m.prompt.Position() < len(m.prompt.Value()) {
			return nil, false
		}
		return m.resetKeys(r), true
	case "ctrl+x":
		return m.unbindKeys(r), true
	case "ctrl+g":
		return m.toggleGlobal(r), true
	}
	return nil, false
}

// --- the capture ----------------------------------------------------------------

// startCapture asks for a new shortcut for action.
func (m *Model) startCapture(action string) {
	m.kb.action, m.kb.text, m.kb.err = action, "", ""
	m.kb.global = action == "toggle_quick_terminal" && globalAction(action)
	for _, s := range m.keyMap().For(action) {
		if s.User && s.Has("global") {
			m.kb.global = true
		}
	}
	m.overlay = overlayCapture
}

// capturedKeys are the keys Bubble Tea names, as Ghostty spells them.
var capturedKeys = map[rune]string{
	tea.KeyUp: "arrow_up", tea.KeyDown: "arrow_down", tea.KeyLeft: "arrow_left", tea.KeyRight: "arrow_right",
	tea.KeyHome: "home", tea.KeyEnd: "end", tea.KeyPgUp: "page_up", tea.KeyPgDown: "page_down",
	tea.KeyInsert: "insert", tea.KeyDelete: "delete", tea.KeyBackspace: "backspace", tea.KeyTab: "tab",
	tea.KeyEnter: "enter", tea.KeyEscape: "escape", tea.KeySpace: "space",
	tea.KeyKpEnter: "numpad_enter", tea.KeyKpPlus: "numpad_add", tea.KeyKpMinus: "numpad_subtract",
	tea.KeyKpMultiply: "numpad_multiply", tea.KeyKpDivide: "numpad_divide", tea.KeyKpDecimal: "numpad_decimal",
	tea.KeyKpEqual: "numpad_equal", tea.KeyKpComma: "numpad_comma",
}

func init() {
	for i := 0; i < 25; i++ {
		capturedKeys[tea.KeyF1+rune(i)] = fmt.Sprintf("f%d", i+1)
	}
	for i := 0; i < 10; i++ {
		capturedKeys[tea.KeyKp0+rune(i)] = fmt.Sprintf("numpad_%d", i)
	}
}

// captureChord turns a key press into a chord, when it is one: a key held
// with ctrl, alt or super (Cmd), a named key held with shift, or a function
// key. A character typed alone, or with shift, is text to type instead, and
// a named key alone (an arrow, Enter) is a poor shortcut and stays out.
//
// What arrives is what the terminal reports. In Ghostty, which reports keys
// as the Kitty protocol does, Cmd comes through as super; a combination
// Ghostty already binds never comes through at all, since Ghostty acts on it
// first, so those are typed.
func captureChord(k tea.Key) (ghostty.Chord, bool) {
	var mods ghostty.Mods
	if k.Mod&tea.ModCtrl != 0 {
		mods |= ghostty.ModCtrl
	}
	if k.Mod&(tea.ModAlt|tea.ModMeta) != 0 {
		mods |= ghostty.ModAlt
	}
	if k.Mod&tea.ModShift != 0 {
		mods |= ghostty.ModShift
	}
	if k.Mod&tea.ModSuper != 0 {
		mods |= ghostty.ModSuper
	}
	name, named := capturedKeys[k.Code]
	if !named {
		if k.Code > unicode.MaxRune || !unicode.IsPrint(k.Code) || unicode.IsSpace(k.Code) {
			return ghostty.Chord{}, false
		}
		name = string(unicode.ToLower(k.Code))
	}
	held := mods&^ghostty.ModShift != 0
	function := len(name) > 1 && name[0] == 'f' && strings.Trim(name[1:], "0123456789") == ""
	if !held && !function && !(named && mods != 0) {
		return ghostty.Chord{}, false
	}
	return ghostty.Chord{Mods: mods, Key: name}, true
}

// updateCapture handles keys while a shortcut is being asked for. Esc,
// Enter, Backspace and Tab alone drive the dialog; a combination is the
// shortcut; characters are typed.
func (m *Model) updateCapture(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.Key()
	if k.Mod == 0 {
		switch k.Code {
		case tea.KeyEscape:
			m.overlay = overlayNone
			return m, nil
		case tea.KeyEnter:
			return m, m.commitCapture()
		case tea.KeyBackspace:
			if r := []rune(m.kb.text); len(r) > 0 {
				m.kb.text = string(r[:len(r)-1])
			}
			m.kb.err = ""
			return m, nil
		case tea.KeyTab:
			if globalAction(m.kb.action) {
				m.kb.global = !m.kb.global
			}
			return m, nil
		}
	}
	if k.Mod&tea.ModHyper != 0 {
		m.kb.err = "Ghostty has no hyper modifier: use super, ctrl, alt or shift"
		return m, nil
	}
	if c, ok := captureChord(k); ok {
		// After a > the chord is the next step of a sequence; otherwise it
		// replaces what was there.
		if m.kb.text == "" || strings.HasSuffix(m.kb.text, ">") {
			m.kb.text += c.String()
		} else {
			m.kb.text = c.String()
		}
		m.kb.err = ""
		return m, nil
	}
	if k.Text != "" && k.Mod&^tea.ModShift == 0 && strings.TrimSpace(k.Text) != "" {
		m.kb.text += k.Text
		m.kb.err = ""
	}
	return m, nil
}

// captured is the trigger typed so far, when it reads.
func (m *Model) captured() (ghostty.Trigger, error) {
	return ghostty.ParseTrigger(m.kb.text)
}

// commitCapture is Enter in the dialog: the shortcut is checked, then
// written, or the question of what it would take from another action is
// asked first.
func (m *Model) commitCapture() tea.Cmd {
	tr, err := m.captured()
	if err != nil {
		if strings.TrimSpace(m.kb.text) == "" {
			m.kb.err = "Press a shortcut, or type one"
		} else {
			m.kb.err = capitalise(err.Error())
		}
		return nil
	}
	if m.kb.global && len(tr) > 1 {
		m.kb.err = "A global shortcut is one chord: a sequence cannot be global"
		return nil
	}
	action := m.kb.action
	b := ghostty.Binding{Trigger: tr, Action: action}
	if m.kb.global {
		b.Prefixes = []string{"global"}
	}
	// The user's own unconsumed: or performable: stays with the action.
	for _, s := range m.keyMap().For(action) {
		if !s.User {
			continue
		}
		for _, p := range s.Prefixes {
			if p != "global" && p != "all" && !b.Has(p) {
				b.Prefixes = append(b.Prefixes, p)
			}
		}
	}
	m.overlay = overlayNone
	return m.offerBinding(b)
}

// offerBinding writes a binding, asking first when it takes a shortcut from
// another action.
func (m *Model) offerBinding(b ghostty.Binding) tea.Cmd {
	km := m.keyMap()
	if s, ok := km.Lookup(b.Trigger); ok && s.Action == b.Action && s.Trigger.String() == b.Trigger.String() && s.Has("global") == b.Has("global") {
		who := "Ghostty's default"
		if s.User {
			who = "set in your config"
		}
		m.info("%s already runs %s — %s", m.triggerText(b.Trigger), keyLabel(b.Action), who)
		return nil
	}
	conflicts := km.Conflicts(b.Trigger, b.Action)
	if len(conflicts) == 0 {
		return m.bindNow(b)
	}
	m.openConfirm(m.conflictText(b, conflicts), func(m *Model) tea.Cmd { return m.bindNow(b) })
	return nil
}

// conflictText says what a new shortcut takes from other actions.
func (m *Model) conflictText(b ghostty.Binding, conflicts []ghostty.Shortcut) string {
	var parts []string
	var losers []string
	seen := map[string]bool{}
	for _, s := range conflicts {
		who := "Ghostty's default"
		if s.User {
			who = "set in your config"
		}
		switch {
		case len(s.Trigger) < len(b.Trigger):
			parts = append(parts, fmt.Sprintf("%s runs %s (%s); a sequence that starts with it takes it over.", m.triggerText(s.Trigger), keyLabel(s.Action), who))
		case len(s.Trigger) > len(b.Trigger):
			parts = append(parts, fmt.Sprintf("%s runs %s (%s); binding %s alone ends that sequence.", m.triggerText(s.Trigger), keyLabel(s.Action), who, m.triggerText(b.Trigger)))
		default:
			if !seen[s.Action+"|same"] {
				parts = append(parts, fmt.Sprintf("%s already runs %s (%s).", m.triggerText(b.Trigger), keyLabel(s.Action), who))
			}
			seen[s.Action+"|same"] = true
		}
		if !seen[s.Action] {
			losers = append(losers, keyLabel(s.Action))
			seen[s.Action] = true
		}
	}
	verb := "loses it"
	if len(losers) > 1 {
		verb = "lose it"
	}
	return strings.Join(parts, " ") + fmt.Sprintf(" Use it for %s? %s %s.", keyLabel(b.Action), strings.Join(losers, " and "), verb)
}

// bindNow writes a binding: the user's line for the trigger or the action
// is rewritten, or a line is added beside their others, and a default that
// is spelled differently but pressed the same way is unbound so it cannot
// win.
func (m *Model) bindNow(b ghostty.Binding) tea.Cmd {
	km := m.keyMap()
	m.tree.SetShortcut(b, km.Shadows(b.Trigger, b.Action))
	note := ""
	if b.Has("global") {
		note = " · global shortcuts need Ghostty in System Settings → Privacy & Security → Accessibility"
	}
	m.info("%s: %s — written; %s%s", keyLabel(b.Action), m.triggerText(b.Trigger), m.reloadWord(), note)
	m.recompute()
	return m.flushWritesCmd()
}

// resetKeys is Delete: the action goes back to Ghostty's shortcuts.
func (m *Model) resetKeys(r *keyRow) tea.Cmd {
	if !m.keysCustomised(r) {
		m.info("%s already has Ghostty's own shortcuts", r.label)
		return nil
	}
	m.tree.ResetShortcuts(r.action, r.defaults)
	what := "no shortcut"
	if len(r.defaults) > 0 {
		what = m.triggersText(r.defaults)
	}
	m.info("%s: back to Ghostty's %s — written; %s", r.label, what, m.reloadWord())
	m.recompute()
	return m.flushWritesCmd()
}

// keysCustomised reports whether the config changes an action's shortcuts:
// it binds the action, or unbinds one of its defaults.
func (m *Model) keysCustomised(r *keyRow) bool {
	for _, b := range m.userShortcuts() {
		if b.Action == r.action {
			return true
		}
		if b.Action == "unbind" {
			for _, d := range r.defaults {
				if d.Same(b.Trigger) {
					return true
				}
			}
		}
	}
	return false
}

// unbindKeys is Ctrl+X: the action is left with no shortcut. The user's
// lines are switched off and Ghostty's defaults get unbind lines.
func (m *Model) unbindKeys(r *keyRow) tea.Cmd {
	if len(r.shortcuts) == 0 {
		m.info("%s has no shortcut to unbind", r.label)
		return nil
	}
	for _, s := range r.shortcuts {
		var defaults []ghostty.Trigger
		for _, d := range r.defaults {
			if d.Same(s.Trigger) {
				defaults = append(defaults, d)
			}
		}
		m.tree.RemoveShortcut(r.action, s.Trigger, defaults)
	}
	m.info("%s: %s unbound — written; %s · Delete gives Ghostty's back", r.label, m.shortcutsText(r.shortcuts), m.reloadWord())
	m.recompute()
	return m.flushWritesCmd()
}

// toggleGlobal is Ctrl+G: the quick terminal's shortcut works from any app,
// or only in Ghostty again.
func (m *Model) toggleGlobal(r *keyRow) tea.Cmd {
	if !globalAction(r.action) {
		if keysMac {
			m.warn("Only the quick terminal and showing Ghostty make sense from other apps")
		} else {
			m.warn("Global shortcuts are offered on macOS")
		}
		return nil
	}
	on := true
	user := false
	for _, s := range r.shortcuts {
		if s.User {
			user = true
			if s.Has("global") {
				on = false
			}
		}
	}
	switch {
	case user:
		m.tree.SetShortcutPrefix(r.action, "global", on)
	case len(r.shortcuts) > 0 && len(r.shortcuts[0].Trigger) == 1:
		m.tree.SetShortcut(ghostty.Binding{Prefixes: []string{"global"}, Trigger: r.shortcuts[0].Trigger, Action: r.action}, nil)
	default:
		// Nothing to make global yet: ask for the shortcut, global already.
		m.startCapture(r.action)
		m.kb.global = true
		return nil
	}
	if on {
		m.info("%s works from any app now — written; %s · it needs Ghostty in System Settings → Privacy & Security → Accessibility", r.label, m.reloadWord())
	} else {
		m.info("%s works only in Ghostty now — written; %s", r.label, m.reloadWord())
	}
	m.recompute()
	return m.flushWritesCmd()
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// --- spelling shortcuts -------------------------------------------------------------

// keySymbols is whether the cells interface may use the Mac's symbols.
func (m *Model) keySymbols() bool { return keysMac && !m.opts.Plain }

// keyName names a key for people: T, ↑, Page Up, F5.
func keyName(key string, symbols bool) string {
	switch key {
	case "arrow_up":
		return "↑"
	case "arrow_down":
		return "↓"
	case "arrow_left":
		return "←"
	case "arrow_right":
		return "→"
	case "enter":
		if symbols {
			return "↩"
		}
		return "Enter"
	case "backspace":
		if symbols {
			return "⌫"
		}
		return "Backspace"
	case "escape":
		return "Esc"
	case "delete":
		return "Del"
	case "page_up":
		return "PgUp"
	case "page_down":
		return "PgDn"
	case "backquote":
		return "`"
	case "space", "tab", "home", "end", "insert":
		return humanize(key)
	}
	if len([]rune(key)) == 1 {
		return strings.ToUpper(key)
	}
	if strings.HasPrefix(key, "key_") || strings.HasPrefix(key, "digit_") {
		_, c, _ := strings.Cut(key, "_")
		return strings.ToUpper(c)
	}
	if c, ok := map[string]string{"comma": ",", "period": ".", "slash": "/", "semicolon": ";", "quote": "'", "bracket_left": "[", "bracket_right": "]", "backslash": "\\", "minus": "-", "equal": "="}[key]; ok {
		return c
	}
	if len(key) > 1 && key[0] == 'f' && strings.Trim(key[1:], "0123456789") == "" {
		return strings.ToUpper(key)
	}
	if n, ok := strings.CutPrefix(key, "numpad_"); ok {
		return "Num " + keyName(n, symbols)
	}
	return humanize(key)
}

// modOrder is the order a Mac prints modifiers in: ⌃⌥⇧⌘.
var modOrder = []struct {
	bit          ghostty.Mods
	symbol, word string
}{{ghostty.ModCtrl, "⌃", "Ctrl"}, {ghostty.ModAlt, "⌥", "Alt"}, {ghostty.ModShift, "⇧", "Shift"}, {ghostty.ModSuper, "⌘", "Super"}}

// chordText spells a chord: ⇧⌘T with the Mac's symbols, Ctrl+Shift+T in
// words.
func chordText(c ghostty.Chord, symbols bool) string {
	var b strings.Builder
	for _, mo := range modOrder {
		if c.Mods&mo.bit == 0 {
			continue
		}
		if symbols {
			b.WriteString(mo.symbol)
		} else {
			word := mo.word
			if word == "Super" && keysMac {
				word = "Cmd"
			}
			if word == "Alt" && keysMac {
				word = "Opt"
			}
			b.WriteString(word + "+")
		}
	}
	b.WriteString(keyName(c.Key, symbols))
	return b.String()
}

// triggerText spells a trigger, the steps of a sequence apart.
func (m *Model) triggerText(t ghostty.Trigger) string {
	parts := make([]string, len(t))
	for i, c := range t {
		parts[i] = chordText(c, m.keySymbols())
	}
	return strings.Join(parts, " › ")
}

// triggersText spells several triggers, leaving out the ones pressed the
// same way as one already said: super+digit_1 and super+1 are both ⌘1.
func (m *Model) triggersText(ts []ghostty.Trigger) string {
	var out []string
	for i, t := range ts {
		dup := false
		for _, o := range ts[:i] {
			dup = dup || o.Same(t)
		}
		if !dup {
			out = append(out, m.triggerText(t))
		}
	}
	return strings.Join(out, ", ")
}

// shortcutsText spells a row's shortcuts for the cells interface.
func (m *Model) shortcutsText(ss []ghostty.Shortcut) string {
	var ts []ghostty.Trigger
	for _, s := range ss {
		ts = append(ts, s.Trigger)
	}
	out := m.triggersText(ts)
	for _, s := range ss {
		if s.Has("global") {
			return out + "  global"
		}
	}
	return out
}

// uniqueShortcuts drops the shortcuts pressed like one before them.
func uniqueShortcuts(ss []ghostty.Shortcut) []ghostty.Shortcut {
	var out []ghostty.Shortcut
	for _, s := range ss {
		dup := false
		for _, o := range out {
			dup = dup || o.Trigger.Same(s.Trigger)
		}
		if !dup {
			out = append(out, s)
		}
	}
	return out
}

// --- what the panel says ----------------------------------------------------------

// keyFacts is what the panel says of a row, for both interfaces: each
// shortcut with where it comes from, and Ghostty's own the config took away.
type keyFact struct {
	trigger ghostty.Trigger
	note    string
	user    bool
	gone    bool // a default no longer in effect
	global  bool
}

func (m *Model) keyFacts(r *keyRow) []keyFact {
	var out []keyFact
	km := m.keyMap()
	for _, s := range uniqueShortcuts(r.shortcuts) {
		f := keyFact{trigger: s.Trigger, user: s.User, global: s.Has("global"), note: "Ghostty's default"}
		if s.User {
			f.note = "yours · " + s.Entry.Location()
		}
		if len(s.Chains) > 0 {
			f.note += " · then " + strings.Join(s.Chains, ", ")
		}
		out = append(out, f)
	}
	for i, d := range r.defaults {
		live := false
		for _, s := range r.shortcuts {
			live = live || s.Trigger.Same(d)
		}
		for _, o := range r.defaults[:i] {
			live = live || o.Same(d)
		}
		if live {
			continue
		}
		f := keyFact{trigger: d, gone: true, note: "Ghostty's default, unbound in your config"}
		if s, ok := km.Lookup(d); ok && s.Action != r.action {
			f.note = "Ghostty's default, now runs " + keyLabel(s.Action)
		}
		out = append(out, f)
	}
	return out
}

// keyDoes says what Enter, Delete, Ctrl+X and Ctrl+G would do to a row, as
// key and sentence.
func (m *Model) keyDoes(r *keyRow) [][2]string {
	var mine []ghostty.Trigger
	for _, s := range r.shortcuts {
		if s.User {
			mine = append(mine, s.Trigger)
		}
	}
	enter := "Press a new shortcut"
	if len(mine) > 0 {
		enter = "Press a new shortcut to replace " + m.triggersText(mine)
	}
	out := [][2]string{{"Enter", enter}}
	switch {
	case !m.keysCustomised(r):
		out = append(out, [2]string{"Delete", "Nothing to reset: these are Ghostty's"})
	case len(r.defaults) > 0:
		out = append(out, [2]string{"Delete", "Back to Ghostty's " + m.triggersText(r.defaults)})
	default:
		out = append(out, [2]string{"Delete", "Back to Ghostty's: no shortcut"})
	}
	if len(r.shortcuts) > 0 {
		out = append(out, [2]string{"^X", "Unbind " + m.shortcutsTextPlain(r.shortcuts)})
	}
	if globalAction(r.action) {
		g := "Make it work from any app"
		for _, s := range r.shortcuts {
			if s.User && s.Has("global") {
				g = "Make it work only in Ghostty"
			}
		}
		out = append(out, [2]string{"^G", g})
	}
	return out
}

// shortcutsTextPlain spells shortcuts without the global mark.
func (m *Model) shortcutsTextPlain(ss []ghostty.Shortcut) string {
	var ts []ghostty.Trigger
	for _, s := range ss {
		ts = append(ts, s.Trigger)
	}
	return m.triggersText(ts)
}

// previewKeysPanel is the panel of the text interface: what the action
// does, its shortcuts and where they come from, and what the keys do.
func previewKeysPanel(m *Model, opt *option, width int) []string {
	c := m.c
	if opt == nil || opt.key == nil {
		if !m.kb.loaded {
			return []string{c.mutedS().Render("Reading the actions from Ghostty…")}
		}
		return para(c.mutedS(), "Type an action, a word of what it does, or a shortcut like cmd+t. trigger=action writes a binding as it is.", width)
	}
	r := opt.key
	if opt.value == "@bind" {
		return append([]string{c.bold(c.fg).Render(truncate(r.label, width))}, para(c.mutedS(), "Enter writes keybind = "+opt.detail+" next to your other bindings.", width)...)
	}
	lines := []string{
		c.bold(c.fg).Render(truncate(r.label, width)),
		c.mutedS().Render(truncate(r.action, width)),
		"",
	}
	if r.summary != "" {
		lines = append(lines, para(c.mutedS(), r.summary, width)...)
		lines = append(lines, "")
	}
	facts := m.keyFacts(r)
	if len(facts) == 0 {
		lines = append(lines, c.mutedS().Render("No shortcut."))
	}
	for _, f := range facts {
		key := m.triggerText(f.trigger)
		st := c.base()
		if f.user {
			st = c.accentS()
		}
		if f.gone {
			st = c.faintS()
		}
		note := f.note
		if f.global {
			note = "global · " + note
		}
		lines = append(lines, m.entry(key, note, maxInt(lipgloss.Width(key)+2, 10), width, st)...)
	}
	if globalAction(r.action) {
		lines = append(lines, "")
		lines = append(lines, para(c.mutedS(), "A global shortcut works when Ghostty is not in front; macOS asks to allow Ghostty under Privacy & Security → Accessibility.", width)...)
	}
	lines = append(lines, "")
	for _, d := range m.keyDoes(r) {
		lines = append(lines, m.entry(d[0], d[1], 8, width, c.base())...)
	}
	return lines
}

// captureLines are the dialog of the text interface that asks for a
// shortcut: what to do, what is typed so far, and what Tab toggles.
func (m *Model) captureLines(c chrome, width int) []string {
	lines := []string{""}
	lines = append(lines, wrapLines("Press the keys, or type them: super+shift+t, or ctrl+a>n for a sequence.", width, c.mutedS())...)
	lines = append(lines, "")
	field := c.bold(c.accent).Render("  ❯ ") + c.base().Render(m.kb.text) + c.accentS().Render("▏")
	if tr, err := m.captured(); err == nil {
		field += c.mutedS().Render("   " + m.triggerText(tr))
	}
	lines = append(lines, field, "")
	if m.kb.err != "" {
		lines = append(lines, wrapLines(m.kb.err, width, c.text(c.warn))...)
	} else {
		lines = append(lines, wrapLines("Shortcuts Ghostty already uses never reach this window: type those.", width, c.faintS())...)
	}
	if globalAction(m.kb.action) {
		state := "off, it works while Ghostty is in front"
		if m.kb.global {
			state = "on, it works from any app; needs Accessibility"
		}
		lines = append(lines, "", "  "+c.base().Render("Tab")+c.mutedS().Render("  global: "+state))
	}
	lines = append(lines, "", "  "+c.keyHint("Enter", "set", "Esc", "cancel", "⌫", "erase"))
	return lines
}
