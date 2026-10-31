package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// palette is the Palette profile: one search line over everything the editor
// can change, answered in one list grouped by kind. Themes, fonts, settings,
// shortcuts and the tool's own commands are found the way Atelier's prompt
// finds them; a setting is changed where it is listed, and the highlighted one
// opens in place to say what its value does and what would be written.
//
// The search line always has the keyboard: what is typed filters, the arrows
// up and down move, left and right change the highlighted setting. Enter
// applies a theme or a font, opens a setting's long list of values, runs a
// command. Esc clears what was typed, then the filter, then leaves.
type palette struct {
	query  string
	filter int
	rows   []palRow
	sel    int

	// built is what the rows were built for, sig that and everything else they
	// depend on; want is a row to land on when they are next built.
	built, sig, want string
	// shown is the row the last key left highlighted, so that only a move
	// onto another row previews it.
	shown string

	// scope is the command whose values the list shows after Enter opened
	// it, nil on the results; wasQuery is what was typed before.
	scope    *command
	wasQuery string

	// recent are the settings changed this session, the newest first.
	recent []string
	// counts are how many of each kind matched, before the list was cut.
	counts map[palKind]int
	// wantFonts and wantKeys say the list needs what is only read on demand.
	wantFonts, wantKeys bool

	// Where the last frame scrolled to, in rows of cells and in the picture's
	// units, and how many rows made a page.
	topCells int
	topPx    float32
	page     int
}

func newPalette() profile { return &palette{} }

// palKind is what a line of the list is.
type palKind int

const (
	palHead palKind = iota // the title of a group
	palNote                // a line that says something, not a result
	palTheme
	palFont
	palSetting
	palShortcut
	palCommand
	palValue // one value of the command Enter opened
)

// palFilters are the chips under the search line, in the order Tab walks them.
var palFilters = []string{"All", "Themes", "Fonts", "Settings", "Shortcuts", "Commands", "Changed"}

// palKinds are the kinds of result, in the order the list shows them when
// they match equally well, with what their group is called.
var palKinds = []struct {
	kind        palKind
	title, noun string
	cap         int // how many the All filter shows; Tab to the kind lists every one
}{
	{palTheme, "Themes", "theme", 6},
	{palFont, "Fonts", "font", 4},
	{palSetting, "Settings", "setting", 8},
	{palShortcut, "Shortcuts", "shortcut", 5},
	{palCommand, "Commands", "command", 5},
}

// palRow is one line of the list.
type palRow struct {
	kind  palKind
	text  string // a group's title, a note
	count string // beside a title
	theme *ghostty.Theme
	opt   option // a font, a shortcut, a value
	cmd   *command
	score int
}

func (r palRow) selectable() bool { return r.kind >= palTheme }

// id names a row across rebuilds of the list, so the highlight stays on it.
func (r palRow) id() string {
	switch r.kind {
	case palTheme:
		return "t:" + r.theme.Name
	case palSetting, palCommand:
		return "c:" + r.cmd.name
	case palShortcut:
		return "k:" + r.opt.value
	case palFont, palValue:
		return "v:" + r.opt.value
	}
	return ""
}

// The tool's commands that change a theme in place belong to Atelier's slot
// editor and its wall; the rest of Palette finds themes and fonts as rows.
var palNotCommands = map[string]bool{
	"theme": true, "favs": true, "fav": true, "new": true, "random": true, "edit": true, "revert": true,
	"save": true, "fork": true, "delete": true, "font": true, "keys": true,
}

// palOften are the settings people reach for, offered before anything is
// typed beside the ones already changed.
var palOften = []string{"size", "opacity", "paddingx", "cursor", "copy", "titlebar", "decoration", "lineheight", "blink", "layout"}

// palHome are the commands offered before anything is typed.
var palHome = []string{"undo", "reload", "check", "changes", "backup", "restore"}

// kindOf is whether a command is listed as a setting, as a command, or not at
// all: a setting has values to step through, a command does something.
func (m *Model) palKindOf(c *command) (palKind, bool) {
	if !m.listed(c) || palNotCommands[c.name] {
		return 0, false
	}
	if c.set != nil || c.adjust || uiPrefNamed(c.name) != nil || c.name == "size" || c.name == "style" {
		return palSetting, true
	}
	if c.run == nil {
		return 0, false
	}
	return palCommand, true
}

// palLabel is what a command is called on its row.
func palLabel(c *command) string {
	if c.set != nil && c.set.label != "" {
		return c.set.label
	}
	return c.title()
}

// palWhere is where a setting lives: its section, and the part of it.
func palWhere(c *command) string {
	part, _ := subOf(c)
	if part == c.group {
		return c.group
	}
	return c.group + " · " + part
}

// palChanged is whether a setting is set otherwise than its default.
func (m *Model) palChanged(c *command) bool {
	if c.set != nil {
		return m.changed(c.set)
	}
	if p := uiPrefNamed(c.name); p != nil {
		return p.value(m) != p.def
	}
	return c.name == "size" && m.fontSize != ghostty.DefaultSize
}

// --- the list ---------------------------------------------------------------------

func (p *palette) scopeName() string {
	if p.scope == nil {
		return ""
	}
	return p.scope.name
}

// current is the highlighted row, nil when there is none.
func (p *palette) current() *palRow {
	if p.sel < 0 || p.sel >= len(p.rows) || !p.rows[p.sel].selectable() {
		return nil
	}
	return &p.rows[p.sel]
}

// refresh builds the list again when anything it shows may have changed: what
// is typed, the filter, the fonts and actions read since, a write (each says
// so on the status line). The highlight stays on its row while what is typed
// stays the same, and goes to the first row when it changes.
func (p *palette) refresh(m *Model) {
	key := p.query + "\x00" + itoa(p.filter) + "\x00" + p.scopeName()
	sig := fmt.Sprint(key, "\x00", m.fontsLoaded, len(m.families), m.kb.loaded, m.statusSeq, m.applied, len(m.lib.Themes), m.state.FavoriteCount(), m.pixelOn(), m.tree.UndoSteps())
	if sig == p.sig {
		return
	}
	keep := p.want
	if keep == "" && key == p.built {
		if r := p.current(); r != nil {
			keep = r.id()
		}
	}
	p.want = ""
	moved := key != p.built
	p.rows = p.build(m)
	p.built, p.sig = key, sig
	p.sel = -1
	for i, r := range p.rows {
		if keep != "" && r.selectable() && r.id() == keep {
			p.sel = i
			break
		}
	}
	if p.sel < 0 && p.scope != nil {
		// A list of values opens on the one in effect.
		for i, r := range p.rows {
			if r.kind == palValue && r.opt.current {
				p.sel = i
				break
			}
		}
	}
	if p.sel < 0 {
		for i, r := range p.rows {
			if r.selectable() {
				p.sel = i
				break
			}
		}
	}
	if moved {
		p.topCells, p.topPx = 0, 0
	}
}

// build is the list for what is typed and the filter.
func (p *palette) build(m *Model) []palRow {
	p.wantFonts, p.wantKeys = false, false
	p.counts = map[palKind]int{}
	if p.scope != nil {
		return p.scopeRows(m)
	}
	q := strings.ToLower(strings.TrimSpace(p.query))
	if q == "" && p.filter == 0 {
		return p.homeRows(m)
	}
	words := strings.Fields(q)
	filter := palFilters[p.filter]
	// The All filter keeps to close matches: a theme whose name only holds
	// the letters scattered is left to the Themes filter.
	strict := filter == "All"
	changed := filter == "Changed"
	wants := func(k palKind, title string) bool {
		return filter == "All" || filter == title || (changed && (k == palSetting || k == palShortcut))
	}

	found := map[palKind][]palRow{}
	if wants(palTheme, "Themes") && !changed {
		found[palTheme] = m.palThemes(words, strict)
	}
	if wants(palFont, "Fonts") && !changed {
		if m.fontsLoaded {
			found[palFont] = m.palFonts(q, words, strict)
		} else {
			p.wantFonts = true
			if filter == "Fonts" {
				found[palFont] = []palRow{{kind: palNote, text: "Listing the fonts Ghostty can load…"}}
			}
		}
	}
	if wants(palSetting, "Settings") || wants(palCommand, "Commands") {
		for _, c := range m.commands() {
			k, ok := m.palKindOf(c)
			if !ok || !wants(k, map[palKind]string{palSetting: "Settings", palCommand: "Commands"}[k]) {
				continue
			}
			if changed && (k != palSetting || !m.palChanged(c)) {
				continue
			}
			if score, ok := palCommandScore(c, words); ok {
				found[k] = append(found[k], palRow{kind: k, cmd: c, score: score})
			}
		}
	}
	if wants(palShortcut, "Shortcuts") {
		if m.kb.loaded {
			found[palShortcut] = m.palShortcuts(q, changed)
		} else {
			p.wantKeys = true
			if filter == "Shortcuts" {
				found[palShortcut] = []palRow{{kind: palNote, text: "Reading Ghostty's actions…"}}
			}
		}
	}

	// The groups, the one that matches best first; each, best match first.
	type group struct {
		kind  palKind
		title string
		cap   int
		rows  []palRow
		best  int
	}
	var groups []group
	for _, k := range palKinds {
		rows := found[k.kind]
		if len(rows) == 0 {
			continue
		}
		best := 1 << 30
		for _, r := range rows {
			if r.kind != palNote {
				p.counts[k.kind]++
			}
			best = minInt(best, r.score)
		}
		sortRows(rows)
		groups = append(groups, group{k.kind, k.title, k.cap, rows, best})
	}
	if q != "" {
		for i := 1; i < len(groups); i++ {
			for j := i; j > 0 && groups[j].best < groups[j-1].best; j-- {
				groups[j], groups[j-1] = groups[j-1], groups[j]
			}
		}
	}

	var out []palRow
	for _, g := range groups {
		rows := g.rows
		count := itoa(p.counts[g.kind])
		if strict && len(rows) > g.cap {
			count = fmt.Sprintf("%d of %d · Tab shows all", g.cap, len(rows))
			rows = rows[:g.cap]
		}
		if p.counts[g.kind] == 0 {
			count = ""
		}
		if q == "" && !strict {
			// A whole kind is long: it reads as its parts.
			out = append(out, p.parted(m, g.title, rows)...)
			continue
		}
		out = append(out, palRow{kind: palHead, text: g.title, count: count})
		out = append(out, rows...)
	}
	if len(out) == 0 {
		out = append(out, palRow{kind: palNote, text: "Nothing matches “" + strings.TrimSpace(p.query) + "” — Esc clears"})
	}
	return out
}

// sortRows puts the best matches first and keeps the order of the rest.
func sortRows(rows []palRow) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].score < rows[j-1].score; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// parted titles a whole kind by its parts: themes by family, settings and
// commands by section, shortcuts by the menu they belong to.
func (p *palette) parted(m *Model, kind string, rows []palRow) []palRow {
	partOf := func(r palRow) string {
		switch r.kind {
		case palTheme:
			return title(themeFamily(r.theme))
		case palSetting, palCommand:
			return r.cmd.group
		case palShortcut:
			return r.opt.key.group
		case palFont:
			if r.opt.family == nil {
				return "To download"
			}
			return "Installed"
		}
		return kind
	}
	if len(rows) > 0 && (rows[0].kind == palSetting || rows[0].kind == palCommand) {
		// The sections in the order of the editor's, each in its parts.
		rank := func(r palRow) int {
			for i, g := range groups {
				if g == r.cmd.group {
					_, sub := subOf(r.cmd)
					return i*100 + sub
				}
			}
			return len(groups) * 100
		}
		for i := 1; i < len(rows); i++ {
			for j := i; j > 0 && rank(rows[j]) < rank(rows[j-1]); j-- {
				rows[j], rows[j-1] = rows[j-1], rows[j]
			}
		}
	}
	var out []palRow
	last := ""
	for _, r := range rows {
		if part := partOf(r); part != last && r.kind != palNote {
			n := 0
			for _, o := range rows {
				if partOf(o) == part {
					n++
				}
			}
			out = append(out, palRow{kind: palHead, text: part, count: itoa(n)})
			last = part
		}
		out = append(out, r)
	}
	return out
}

// homeRows is the list before anything is typed: the theme in effect and the
// starred ones, the font, what is changed, what was changed this session,
// the settings people reach for, and the commands that look after the files.
func (p *palette) homeRows(m *Model) []palRow {
	var out []palRow
	seen := map[string]bool{}
	group := func(title, count string, rows []palRow) {
		if len(rows) > 0 {
			out = append(out, palRow{kind: palHead, text: title, count: count})
			out = append(out, rows...)
		}
	}

	var themes []palRow
	if t, ok := m.lib.Get(m.applied); ok {
		themes = append(themes, palRow{kind: palTheme, theme: t})
	}
	for _, t := range m.lib.Themes {
		if len(themes) >= 4 {
			break
		}
		if m.state.IsFavorite(t.Name) && t.Name != m.applied {
			themes = append(themes, palRow{kind: palTheme, theme: t})
		}
	}
	count := "applied"
	if len(themes) > 1 {
		count = "applied and starred"
	}
	group("Themes", count, themes)

	if m.fontsLoaded && m.fontFamily != "" {
		for _, o := range m.fontOptions(m.fontFamily) {
			if o.family != nil && o.family.Name == m.fontFamily {
				group("Font", "in use", []palRow{{kind: palFont, opt: o}})
				break
			}
		}
	}

	setting := func(name string) (palRow, bool) {
		c := m.commandNamed(name)
		if c == nil || seen[name] {
			return palRow{}, false
		}
		if k, ok := m.palKindOf(c); !ok || k != palSetting {
			return palRow{}, false
		}
		seen[name] = true
		return palRow{kind: palSetting, cmd: c}, true
	}
	var changed []palRow
	for _, c := range m.commands() {
		if k, ok := m.palKindOf(c); ok && k == palSetting && m.palChanged(c) {
			if r, ok := setting(c.name); ok {
				changed = append(changed, r)
			}
		}
	}
	group("Changed", itoa(len(changed)), changed)

	var recent []palRow
	for _, name := range p.recent {
		if r, ok := setting(name); ok {
			recent = append(recent, r)
		}
	}
	group("This session", "", recent)

	var often []palRow
	for _, name := range palOften {
		if len(often) == 6 {
			break
		}
		if r, ok := setting(name); ok {
			often = append(often, r)
		}
	}
	group("Often used", "", often)

	var cmds []palRow
	for _, name := range palHome {
		if c := m.commandNamed(name); c != nil && m.listed(c) {
			cmds = append(cmds, palRow{kind: palCommand, cmd: c})
		}
	}
	group("Commands", "", cmds)
	return out
}

// palThemes are the themes every typed word finds: in the name, by the
// letters of Atelier's search, or in the family and the description; dark
// and light find the themes of that kind.
func (m *Model) palThemes(words []string, strict bool) []palRow {
	var out []palRow
	lower := make([]string, len(words))
	for i, w := range words {
		// As fuzzyMatch has them: the names are kept in lower case.
		lower[i] = strings.ToLower(w)
	}
	for _, t := range m.themeOrder() {
		if score, ok := palThemeScore(m.factsOf(t), lower, strict); ok {
			out = append(out, palRow{kind: palTheme, theme: t, score: score})
		}
	}
	return out
}

func palThemeScore(f *themeFacts, words []string, strict bool) (int, bool) {
	total := 0
	for _, w := range words {
		if w == "dark" || w == "light" {
			if (w == "dark") != f.dark {
				return 0, false
			}
			continue
		}
		if fuzzyMatchLower(f.lowerName, w) {
			if score := fuzzyScoreLower(f.lowerName, w); !strict || score < 3 {
				total += score
				continue
			}
		}
		if strings.Contains(f.lowerFind, w) {
			total += 3
			continue
		}
		return 0, false
	}
	return total, true
}

// palFonts are the families the font command lists, installed and to
// download, that every typed word finds.
func (m *Model) palFonts(q string, words []string, strict bool) []palRow {
	var out []palRow
	for _, o := range m.fontOptions(q) {
		if o.header || (o.family == nil && !strings.HasPrefix(o.value, downloadPrefix)) {
			continue
		}
		total, ok := 0, true
		for _, w := range words {
			if !fuzzyMatch(o.label, w) || (strict && fuzzyScore(o.label, w) == 3) {
				ok = false
				break
			}
			total += fuzzyScore(o.label, w)
		}
		if ok {
			out = append(out, palRow{kind: palFont, opt: o, score: total})
		}
	}
	return out
}

// palCommandScore finds a setting or a command the way Atelier's menu does,
// by its word or a word of what it does, and by its name on screen and where
// it lives: every typed word must be found. A name that starts with the word
// ranks before one that only holds it.
func palCommandScore(c *command, words []string) (int, bool) {
	label := strings.ToLower(palLabel(c))
	where := strings.ToLower(palWhere(c))
	total := 0
	for _, w := range words {
		best := minInt(fuzzyScore(label, w), fuzzyScore(c.name, w))
		if best == 3 && !menuMatches(c, w) && !strings.Contains(where, w) {
			return 0, false
		}
		total += best
	}
	return total, true
}

// palShortcuts are the actions of the shortcut editor that what is typed
// finds, by the editor's own matching.
func (m *Model) palShortcuts(q string, changedOnly bool) []palRow {
	var out []palRow
	for _, o := range m.keyOptions(q) {
		if o.header || o.key == nil || o.value == "@bind" {
			continue
		}
		if changedOnly && !m.keysCustomised(o.key) {
			continue
		}
		out = append(out, palRow{kind: palShortcut, opt: o, score: 2})
	}
	return out
}

// scopeRows are the values of the command Enter opened, filtered by what is
// typed as the command filters them.
func (p *palette) scopeRows(m *Model) []palRow {
	c := p.scope
	q := strings.TrimSpace(p.query)
	arg := q
	if c.filters {
		arg = ""
	}
	var out []palRow
	for _, o := range c.options(m, arg) {
		if o.header {
			if o.label != "" {
				out = append(out, palRow{kind: palHead, text: o.label, count: o.detail})
			}
			continue
		}
		if c.filters && q != "" && !fuzzyMatch(o.label, q) {
			continue
		}
		out = append(out, palRow{kind: palValue, opt: o, score: fuzzyScore(o.label, q)})
	}
	if len(out) == 0 {
		out = append(out, palRow{kind: palNote, text: "Nothing to choose here — Esc goes back"})
	}
	return out
}

// countLine says how many of each kind matched, for the search line.
func (p *palette) countLine() string {
	if p.scope != nil || (strings.TrimSpace(p.query) == "" && p.filter == 0) {
		return ""
	}
	var parts []string
	for _, k := range palKinds {
		if n := p.counts[k.kind]; n > 0 {
			parts = append(parts, countWord(n, k.noun))
		}
	}
	if len(parts) == 0 {
		return "no match"
	}
	return strings.Join(parts, " · ")
}

// --- keys ---------------------------------------------------------------------------

func (p *palette) key(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p.refresh(m)
	cmd := p.press(m, msg)
	if m.quitting {
		return m, cmd
	}
	p.refresh(m)
	return m, tea.Batch(cmd, p.settle(m))
}

// settle previews the theme the highlight moved onto and asks for what the
// list needs read.
func (p *palette) settle(m *Model) tea.Cmd {
	var cmds []tea.Cmd
	id := ""
	if r := p.current(); r != nil {
		id = r.id()
		if id != p.shown && r.kind == palTheme {
			m.previewTheme(r.theme)
		}
	}
	p.shown = id
	if p.wantFonts {
		cmds = append(cmds, m.ensureFonts())
	}
	if p.wantKeys {
		cmds = append(cmds, m.ensureKeys())
	}
	return tea.Batch(cmds...)
}

func (p *palette) press(m *Model, msg tea.KeyPressMsg) tea.Cmd {
	r := p.current()
	switch msg.String() {
	case "esc":
		return p.escape(m)
	case "enter":
		return p.enterRow(m)
	case "tab", "shift+tab":
		if p.scope == nil {
			n := len(palFilters)
			p.filter = (p.filter + map[string]int{"tab": 1, "shift+tab": n - 1}[msg.String()]) % n
		}
		return nil
	case "up", "ctrl+p":
		p.move(-1)
	case "down", "ctrl+n":
		p.move(1)
	case "pgup":
		p.move(-maxInt(p.page, 1))
	case "pgdown":
		p.move(maxInt(p.page, 1))
	case "home":
		p.sel = -1
		p.move(1)
	case "end":
		p.sel = len(p.rows)
		p.move(-1)
	case "left", "right":
		if r != nil && r.kind == palSetting {
			return p.step(m, r.cmd, map[string]int{"left": -1, "right": 1}[msg.String()])
		}
	case "delete":
		return p.reset(m)
	case "backspace", "ctrl+h":
		if _, size := utf8.DecodeLastRuneInString(p.query); size > 0 {
			p.query = p.query[:len(p.query)-size]
		}
	case "ctrl+w", "alt+backspace":
		q := strings.TrimRight(p.query, " ")
		if i := strings.LastIndex(q, " "); i >= 0 {
			p.query = q[:i+1]
		} else {
			p.query = ""
		}
	case "ctrl+u":
		p.query = ""
	case "ctrl+t":
		if r != nil && r.kind == palTheme {
			m.previewTheme(r.theme)
			return m.tryHere()
		}
	case "ctrl+f":
		if r != nil && r.kind == palTheme {
			m.previewTheme(r.theme)
			m.toggleFavorite()
		}
	case "ctrl+x":
		if r != nil && r.kind == palShortcut {
			return m.unbindKeys(r.opt.key)
		}
	case "ctrl+g":
		if r != nil && r.kind == palShortcut && globalAction(r.opt.key.action) {
			return m.toggleGlobal(r.opt.key)
		}
	case "f1", "ctrl+_":
		m.overlay = overlayHelp
	default:
		if t := msg.Text; t != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 && utf8.RuneCountInString(p.query) < 96 {
			p.query += t
		}
	}
	return nil
}

// escape steps back: out of an opened list, then what is typed, then the
// filter; on the bare list it leaves, asking first as Atelier does.
func (p *palette) escape(m *Model) tea.Cmd {
	switch {
	case p.scope != nil:
		p.closeScope()
	case p.query != "":
		p.query = ""
		// Themes were only looked at: the one in effect comes back.
		if m.cur != nil && m.cur.Name != m.applied && !m.dirty {
			m.landOnApplied()
			p.shown = ""
		}
		m.renderStatus()
	case p.filter != 0:
		p.filter = 0
	default:
		_, cmd := m.quit()
		return cmd
	}
	return nil
}

// move steps the highlight over the titles and notes.
func (p *palette) move(delta int) {
	dir := 1
	if delta < 0 {
		dir, delta = -1, -delta
	}
	at := p.sel
	for ; delta > 0; delta-- {
		j := at + dir
		for j >= 0 && j < len(p.rows) && !p.rows[j].selectable() {
			j += dir
		}
		if j < 0 || j >= len(p.rows) {
			break
		}
		at = j
	}
	if at >= 0 && at < len(p.rows) && p.rows[at].selectable() {
		p.sel = at
	}
}

// enterRow is Enter: a theme or a font is applied, a shortcut asked for, a
// command run or opened; a setting is switched, stepped, or its values
// listed when they are too many to step through.
func (p *palette) enterRow(m *Model) tea.Cmd {
	r := p.current()
	if r == nil {
		return nil
	}
	switch r.kind {
	case palTheme:
		return m.runCommand(m.commandNamed("theme"), "", &option{label: r.theme.Name, value: r.theme.Name, theme: r.theme})
	case palFont:
		o := r.opt
		return m.runCommand(m.commandNamed("font"), "", &o)
	case palShortcut:
		m.startCapture(r.opt.key.action)
		return nil
	case palCommand:
		if r.cmd.options == nil {
			return m.runCommand(r.cmd, "", nil)
		}
		return p.openScope(m, r.cmd)
	case palSetting:
		c := r.cmd
		vals := palValues(m, c)
		cur := -1
		for i, o := range vals {
			if o.current {
				cur = i
			}
		}
		switch {
		case isSwitch(vals) && cur >= 0:
			return p.write(m, c, &vals[1-cur])
		case c.name == "layout" || len(vals) > 6 || (c.set != nil && (c.set.list != nil || c.set.numeric)) || cur < 0:
			return p.openScope(m, c)
		}
		return p.write(m, c, &vals[(cur+1)%len(vals)])
	case palValue:
		o := r.opt
		c := p.scope
		cmd := m.runCommand(c, "", &o)
		if k, _ := m.palKindOf(c); k == palSetting && m.overlay == overlayNone && p.scope == c {
			// A value is chosen: back to the results, on the setting.
			p.touch(c.name)
			p.closeScope()
		}
		return cmd
	}
	return nil
}

// openScope lists a command's values in place of the results.
func (p *palette) openScope(m *Model, c *command) tea.Cmd {
	p.scope, p.wasQuery, p.query = c, p.query, ""
	p.want = ""
	if c.onMove != nil && c.group == "Tool" && uiPrefNamed(c.name) == nil {
		// The check asks Ghostty when its list is opened, as in Atelier.
		return c.onMove(m, nil)
	}
	return nil
}

// closeScope goes back to the results, on the command that was opened.
func (p *palette) closeScope() {
	if p.scope == nil {
		return
	}
	p.want = "c:" + p.scope.name
	p.query, p.scope = p.wasQuery, nil
}

// palValues are the values a setting steps through, the one in effect marked.
func palValues(m *Model, c *command) []option {
	if c.options == nil {
		return nil
	}
	var out []option
	for _, o := range c.options(m, "") {
		if !o.header {
			out = append(out, o)
		}
	}
	return out
}

func isSwitch(vals []option) bool {
	return len(vals) == 2 && vals[0].value == "on" && vals[1].value == "off"
}

// step moves a setting to its previous or next value and writes it, the way
// the arrows do on Atelier's menu. A value between two steps goes to the
// nearest one the arrow's way; the end of the scale goes nowhere.
func (p *palette) step(m *Model, c *command, dir int) tea.Cmd {
	if c.set != nil && c.set.list != nil {
		m.info("%s lists what is on disk: Enter shows the list", palLabel(c))
		return nil
	}
	vals := palValues(m, c)
	cur := -1
	for i, o := range vals {
		if o.current {
			cur = i
		}
	}
	next := cur + dir
	if cur < 0 {
		next = 0
		if s := c.set; s != nil && s.numeric {
			now, ok := parseNumber(s.get(m))
			if !ok {
				m.warn("%s is set to %s here, which is not a number the arrows can step", s.label, s.get(m))
				return nil
			}
			next = nearestStep(vals, now, dir)
		}
	}
	if next < 0 || next >= len(vals) {
		return nil
	}
	return p.write(m, c, &vals[next])
}

// reset is Delete: a setting goes back to Ghostty's default, which takes its
// line out of the config; a shortcut goes back to Ghostty's own.
func (p *palette) reset(m *Model) tea.Cmd {
	r := p.current()
	if r == nil {
		return nil
	}
	switch r.kind {
	case palShortcut:
		return m.resetKeys(r.opt.key)
	case palSetting:
		c := r.cmd
		switch pref := uiPrefNamed(c.name); {
		case c.set != nil && c.set.fallback != "":
			if !strings.EqualFold(c.set.get(m), c.set.fallback) {
				return p.write(m, c, &option{value: c.set.fallback})
			}
		case pref != nil && pref != prefLayout:
			if pref.value(m) != pref.def {
				return p.write(m, c, &option{label: pref.def, value: pref.def})
			}
		case c.name == "size":
			if m.fontSize != ghostty.DefaultSize {
				return p.write(m, c, &option{value: ghostty.FormatSize(ghostty.DefaultSize)})
			}
		}
	}
	return nil
}

// write runs a setting with a value: the setting writes the file, asks
// Ghostty to reload and says so, as everywhere in the editor.
func (p *palette) write(m *Model, c *command, o *option) tea.Cmd {
	cmd := m.runCommand(c, "", o)
	p.touch(c.name)
	return cmd
}

// touch remembers a setting changed this session.
func (p *palette) touch(name string) {
	out := []string{name}
	for _, n := range p.recent {
		if n != name && len(out) < 6 {
			out = append(out, n)
		}
	}
	p.recent = out
}

// --- the mouse -------------------------------------------------------------------------

func (p *palette) mouse(m *Model, msg mouseEv) (tea.Model, tea.Cmd) {
	p.refresh(m)
	if msg.wheel {
		p.move(map[bool]int{true: -1, false: 1}[msg.Button == tea.MouseWheelUp])
		return m, p.settle(m)
	}
	r, ok := m.hit(msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	var cmd tea.Cmd
	switch r.kind {
	case hitRail:
		if r.index < 0 {
			p.closeScope()
		} else if r.index < len(palFilters) {
			p.filter = r.index
		}
	case hitResult:
		double := m.isDouble(r)
		if r.index >= 0 && r.index < len(p.rows) && p.rows[r.index].selectable() {
			p.sel = r.index
			if double {
				cmd = p.enterRow(m)
			}
		}
	case hitValue:
		cmd = p.clickValue(m, r, msg.X)
	}
	if m.quitting {
		return m, cmd
	}
	p.refresh(m)
	return m, tea.Batch(cmd, p.settle(m))
}

// clickValue sets a setting from its control: a pill, a switch, a point
// along a slider.
func (p *palette) clickValue(m *Model, r region, x int) tea.Cmd {
	if r.index < 0 || r.index >= len(p.rows) || p.rows[r.index].kind != palSetting {
		return nil
	}
	p.sel = r.index
	c := p.rows[r.index].cmd
	v := r.name
	if s := c.set; v == "slider" && s != nil {
		frac := float64(x-r.x) / float64(maxInt(r.w-1, 1))
		v = s.format(roundStep(color.Clamp(s.min+frac*(s.max-s.min), s.min, s.max), s.step))
	}
	for _, o := range palValues(m, c) {
		if strings.EqualFold(o.value, v) {
			if o.current {
				return nil
			}
			return p.write(m, c, &o)
		}
	}
	if c.set != nil && c.set.numeric {
		return p.write(m, c, &option{value: v})
	}
	return nil
}

// --- what F3 reads ------------------------------------------------------------------------

func (p *palette) explainTarget(m *Model) (string, bool) {
	p.refresh(m)
	of := func(c *command) string {
		if c.set != nil && c.set.key != "" {
			return c.set.key
		}
		return explainCommands[c.name]
	}
	if p.scope != nil {
		return of(p.scope), false
	}
	r := p.current()
	switch {
	case r == nil:
		return "", false
	case r.kind == palShortcut:
		return r.opt.key.action, true
	case r.kind == palFont:
		return "font-family", false
	case r.kind == palTheme:
		return "theme", false
	case r.cmd != nil:
		return of(r.cmd), false
	}
	return "", false
}

// enter needs no welcome: the search line says what it does, and the line
// under the list says how to begin until something happens.
func (p *palette) enter(m *Model) tea.Cmd {
	p.sig, p.shown = "", ""
	return nil
}

// --- what the screen says, in both interfaces ----------------------------------------

// keyPairs are the keys of the footer, as key and word, most useful first.
func (p *palette) keyPairs(m *Model) []string {
	pairs := []string{"↑↓", "Move"}
	if p.scope == nil {
		pairs = append(pairs, "Tab", "Filter")
	}
	r := p.current()
	switch {
	case r == nil:
	case r.kind == palTheme:
		pairs = append(pairs, "Enter", "Apply", "^T", "Try here", "^F", "Star")
	case r.kind == palSetting:
		enter := "Next"
		vals := palValues(m, r.cmd)
		switch {
		case isSwitch(vals):
			enter = "Switch"
		case r.cmd.name == "layout" || len(vals) > 6 || (r.cmd.set != nil && (r.cmd.set.list != nil || r.cmd.set.numeric)):
			enter = "Open"
		}
		pairs = append(pairs, "←→", "Change", "Enter", enter, "Delete", "Default")
	case r.kind == palFont, r.kind == palValue:
		pairs = append(pairs, "Enter", "Apply")
	case r.kind == palShortcut:
		pairs = append(pairs, "Enter", "Set", "Delete", "Default")
	case r.kind == palCommand:
		word := "Run"
		if r.cmd.options != nil {
			word = "Open"
		}
		pairs = append(pairs, "Enter", word)
	}
	if name, _ := p.explainTarget(m); name != "" {
		pairs = append(pairs, "F3", "Docs")
	}
	esc := "Leave"
	switch {
	case p.scope != nil:
		esc = "Back"
	case p.query != "" || p.filter != 0:
		esc = "Clear"
	}
	return append(pairs, "Esc", esc, "F1", "Help")
}

// statusText is the line under the list: what just happened, or else what
// is changed, or how to begin.
func (p *palette) statusText(m *Model) (text string, kind statusKind, dot bool) {
	if m.statusKind != statusPlain && m.status != "" {
		return m.status, m.statusKind, true
	}
	n := 0
	for _, c := range m.commands() {
		if k, ok := m.palKindOf(c); ok && k == palSetting && m.palChanged(c) {
			n++
		}
	}
	if n > 0 {
		return countWord(n, "setting") + " changed · each file is backed up before its first change", statusPlain, true
	}
	return "Type a theme, a font, a setting or a shortcut · Tab filters by kind · Esc leaves", statusPlain, false
}

// palValueText is a setting's value as its row says it.
func (m *Model) palValueText(c *command) string {
	if c.set != nil {
		return c.set.shown(c.set.get(m))
	}
	return m.currentValueOf(c)
}

// palFace is the setting a row draws its control from: the setting itself,
// or, for a command with values of its own, one made of those values.
func (m *Model) palFace(c *command) (*setting, string) {
	if c.set != nil {
		return c.set, c.set.get(m)
	}
	s := &setting{cmd: c.name, label: palLabel(c)}
	cur := ""
	for _, o := range palValues(m, c) {
		s.options = append(s.options, o.value)
		if o.current {
			cur = o.value
		}
	}
	return s, cur
}

// palExplain is what a value of a setting does: the value, as a lead, and a
// sentence; or what the setting is, when its values have no note.
func (m *Model) palExplain(c *command, value string) (lead, text string) {
	lower := func(s string) string {
		if s == "" {
			return s
		}
		return strings.ToLower(s[:1]) + s[1:]
	}
	s := c.set
	if s == nil {
		if pref := uiPrefNamed(c.name); pref != nil && pref.notes[value] != "" {
			name := value
			if t, ok := pref.titles[value]; ok {
				name = t
			}
			return title(displayValue(name)), lower(pref.notes[value]) + "."
		}
		return "", c.desc + "."
	}
	explain := s.explain
	if explain == nil {
		explain = valueNotes[s.cmd]
	}
	if explain != nil && explain[value] != "" {
		return title(displayValue(value)), lower(explain[value]) + "."
	}
	text = s.detail + "."
	if s.describe != nil && value != "" {
		text += " " + title(s.describe(m, value)) + "."
	}
	return "", text
}

// palWrites is what a value puts in the config: its key, the lines, or a
// note when it puts none.
func (m *Model) palWrites(c *command, value string) (key string, lines []string, note string) {
	s := c.set
	if s == nil {
		if c.name == "size" {
			return "font-size", []string{"font-size = " + value}, ""
		}
		if c.name == "style" {
			return "font-style", []string{"font-style = " + value}, ""
		}
		return "", nil, "kept by ghostty-config itself, not in Ghostty's config"
	}
	line := s.key + " = " + value
	switch {
	case s.writes != nil:
		line = s.writes(m, value)
	case strings.EqualFold(value, s.fallback):
		line = ""
	}
	for _, l := range strings.Split(line, "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return s.key, nil, "line removed: Ghostty's default"
	}
	return s.key, lines, ""
}

// palContrast is how well a theme reads: its text, and its weakest colour.
func palContrast(t *ghostty.Theme) (text float64, weakest float64, weakestName string) {
	bg := t.Background()
	text = color.Contrast(t.Foreground(), bg)
	weakest = 21
	for i := 1; i < 8; i++ {
		if v := t.Get(paletteKey(i)); v != "" {
			if r := color.Contrast(v, bg); r < weakest {
				weakest, weakestName = r, color.ANSI[i]
			}
		}
	}
	return text, weakest, weakestName
}

// palThemeMeta is a theme's kind and grade, as its row says it.
func palThemeMeta(t *ghostty.Theme) string {
	kind := "dark"
	if !t.IsDark() {
		kind = "light"
	}
	ratio := color.Contrast(t.Foreground(), t.Background())
	return fmt.Sprintf("%s · %s %.1f", kind, color.Grade(ratio), ratio)
}

// palThemeFamily is where a theme comes from, as its row says it.
func palThemeFamily(t *ghostty.Theme) string {
	switch fam := themeFamily(t); fam {
	case "bundled":
		return "Ghostty"
	case "installed", "collection":
		return "Your themes"
	default:
		return title(fam)
	}
}
