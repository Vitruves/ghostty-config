package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// guided is the Guided profile: six steps, Colours, Font, Window, Cursor,
// Copy & paste and Done, each a page or two of plain questions with a few
// answers each. An answer is a handful of settings the other profiles reach
// one by one; Next writes the answers that were changed through the same
// commands, so validation, the session's history and the reload follow as
// they do anywhere else, and Ctrl+Z takes back a page in one step.
//
// Nothing is written while an answer is only highlighted: the picture beside
// it shows what it would do, and a line under it says what would be written.
// The profile is made once per session, so leaving for Atelier and coming
// back finds the step where it was left.
type guided struct {
	started bool
	// step and page are where the guide is; focus is the question of the
	// page the arrows move in.
	step, page, focus int
	// reached is the furthest step seen, whose bars the progress fills.
	reached int
	// picks are the answers moved to by hand since the page was last
	// written, by question; an answer not picked is read from the config.
	picks map[string]int
	// more is how many sets of three themes "Three more" has passed over.
	more int
	// away is whether the guide stepped aside for one of Atelier's screens,
	// the wall of every theme or every setting of a step; Esc comes back.
	away bool
	// temps are the themes sorted by warmth, kept until the library changes.
	temps map[string]map[string][]*ghostty.Theme
	// cregs are the clickable spans of the text interface's body, relative
	// to its first row, while it is drawn.
	cregs []region
	// kept are pictures of the text interface by what they were drawn
	// from; see cellsKept.
	kept map[string][]string
}

func newGuided() profile { return &guided{} }

// --- what the guide asks ------------------------------------------------------

// The steps, in order.
const (
	gColours = iota
	gFont
	gWindow
	gCursor
	gCopy
	gDone
	gSteps
)

// gStepNames are the steps as the progress bar names them.
var gStepNames = [gSteps]string{"Colours", "Font", "Window", "Cursor", "Copy & paste", "Done"}

// gStepGroups are the sections of Atelier that hold every setting of a step.
var gStepGroups = [gSteps]string{"Themes", "Fonts", "Window", "Input", "Input", ""}

// gKind is how a question shows its answers.
type gKind int

const (
	gRadio  gKind = iota // rows, one line of explanation each
	gPills               // a row of short words, for a second question
	gCards               // three themes, each a terminal in its colours
	gFonts               // rows set in each family's own face
	gAction              // what Done offers: rows that act at once
)

// gWrite is one command an answer runs, with the value it runs it with.
type gWrite struct {
	cmd string
	opt option
}

// gChoice is one answer.
type gChoice struct {
	label, desc string
	writes      []gWrite
	// is reports whether the config says this answer now; nil compares
	// every write with what its setting is set to.
	is func(m *Model) bool
	// value is what an answer that only narrows the next question chose.
	value  string
	theme  *ghostty.Theme
	family *ghostty.Family
	// act is what an answer of Done does.
	act func(m *Model, g *guided) tea.Cmd
}

// gQuestion is one question and its answers.
type gQuestion struct {
	id, title string
	kind      gKind
	// doc is the option F3 reads Ghostty's page on.
	doc     string
	choices []gChoice
	// filter marks a question whose answer writes nothing: it only narrows
	// what the next one offers.
	filter bool
	// preview is what the picture beside the page shows for it.
	preview string
}

// gPage is what the screen shows at once: a question, and sometimes a
// second, smaller one under it.
type gPage struct {
	eyebrow string
	qs      []gQuestion
}

// pages are the pages of a step, as this platform and this Ghostty have
// its settings.
func (g *guided) pages(m *Model, step int) []gPage {
	switch step {
	case gColours:
		return g.colourPages(m)
	case gFont:
		return g.fontPages(m)
	case gWindow:
		return g.windowPages(m)
	case gCursor:
		return g.cursorPages(m)
	case gCopy:
		return g.copyPages(m)
	}
	return g.donePages(m)
}

// setWrite is the write of a setting's command at a value.
func setWrite(cmd, value string) gWrite { return gWrite{cmd: cmd, opt: option{value: value}} }

// choice is an answer that sets one setting to one value, explained by the
// setting's own words when the guide has none of its own.
func (m *Model) choice(cmd, value, label, desc string) gChoice {
	if desc == "" {
		if c := m.commandNamed(cmd); c != nil && c.set != nil {
			if c.set.explain != nil {
				desc = c.set.explain[value]
			}
			if desc == "" {
				desc = valueNotes[cmd][value]
			}
		}
		desc = sentence(desc)
	}
	return gChoice{label: label, desc: desc, writes: []gWrite{setWrite(cmd, value)}}
}

// onOffQuestion is a question answered on or off, by a setting of that kind.
func (m *Model) onOffQuestion(id, title, cmd string, kind gKind, on, off [2]string) gQuestion {
	q := gQuestion{id: id, title: title, kind: kind, doc: m.docOf(cmd)}
	q.choices = []gChoice{m.choice(cmd, "on", on[0], on[1]), m.choice(cmd, "off", off[0], off[1])}
	return q
}

// docOf is the option behind a command, for F3.
func (m *Model) docOf(cmd string) string {
	if c := m.commandNamed(cmd); c != nil && c.set != nil && c.set.key != "" {
		return c.set.key
	}
	return explainCommands[cmd]
}

// sentence starts a phrase with a capital and ends it with a full stop.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// --- state --------------------------------------------------------------------

// at is the page shown, kept within the pages the step has now.
func (g *guided) at(m *Model) (gPage, []gPage) {
	pages := g.pages(m, g.step)
	g.page = clampInt(g.page, 0, len(pages)-1)
	pg := pages[g.page]
	g.focus = clampInt(g.focus, 0, len(pg.qs)-1)
	return pg, pages
}

// sel is the answer of a question shown as chosen: the one picked by hand,
// or else the one the config says now, or -1 for none.
func (g *guided) sel(m *Model, q gQuestion) int {
	if i, ok := g.picks[q.id]; ok && i < len(q.choices) {
		return i
	}
	if q.kind == gAction {
		return 0
	}
	for i, c := range q.choices {
		if m.choiceIs(c) {
			return i
		}
	}
	if q.kind == gCards || q.filter {
		return 0
	}
	return -1
}

// choiceIs is whether the config says an answer now.
func (m *Model) choiceIs(c gChoice) bool {
	if c.is != nil {
		return c.is(m)
	}
	if c.theme != nil {
		return c.theme.Name == m.applied
	}
	if len(c.writes) == 0 {
		return false
	}
	for _, w := range c.writes {
		if !m.writeIs(w) {
			return false
		}
	}
	return true
}

// writeIs is whether a write would change nothing.
func (m *Model) writeIs(w gWrite) bool {
	switch w.cmd {
	case "theme":
		return w.opt.theme != nil && w.opt.theme.Name == m.applied
	case "font":
		return w.opt.family != nil && w.opt.family.Name == m.fontFamily
	case "size":
		v, err := strconv.ParseFloat(w.opt.value, 64)
		return err == nil && v == m.fontSize
	}
	c := m.commandNamed(w.cmd)
	if c == nil || c.set == nil {
		return false
	}
	now := c.set.get(m)
	if c.set.numeric {
		a, okA := parseNumber(now)
		b, okB := parseNumber(w.opt.value)
		return okA && okB && sameNumber(a, b, c.set.step)
	}
	return strings.EqualFold(now, w.opt.value)
}

// pick moves a question's answer by hand.
func (g *guided) pick(m *Model, q gQuestion, i int) tea.Cmd {
	if i < 0 || i >= len(q.choices) {
		return nil
	}
	if g.picks == nil {
		g.picks = map[string]int{}
	}
	g.picks[q.id] = i
	ch := q.choices[i]
	switch {
	case ch.theme != nil:
		// A theme is shown as soon as it is chosen, as on the wall.
		m.previewTheme(ch.theme)
	case q.id == "tone" || q.id == "temp":
		// The themes on offer change with the answer: start them over.
		g.more = 0
		delete(g.picks, "theme")
	}
	if q.kind == gFonts {
		return m.wantGuideSpecimens(g)
	}
	return nil
}

// pending are the writes Next would run for a page: the answers moved by
// hand that the config does not say already, and the theme of the cards.
func (g *guided) pending(m *Model, pg gPage) []gWrite {
	var out []gWrite
	for _, q := range pg.qs {
		if q.filter || q.kind == gAction {
			continue
		}
		i := g.sel(m, q)
		if _, picked := g.picks[q.id]; i < 0 || (!picked && q.kind != gCards) {
			continue
		}
		for _, w := range q.choices[i].writes {
			if !m.writeIs(w) {
				out = append(out, w)
			}
		}
	}
	return out
}

// writeLine is what a write puts in the config, the way the panel beside a
// setting says it.
func (m *Model) writeLine(w gWrite) string {
	switch w.cmd {
	case "theme":
		if w.opt.theme != nil {
			return "theme = " + w.opt.theme.ConfigValue()
		}
	case "font":
		if w.opt.family != nil {
			return "font-family = " + w.opt.family.Name
		}
	case "size":
		return "font-size = " + w.opt.value
	case "ligatures":
		if w.opt.value == "off" {
			return "font-feature = -calt, -liga, -dlig"
		}
		return "font-feature: ligatures back on"
	}
	c := m.commandNamed(w.cmd)
	if c == nil || c.set == nil {
		return w.cmd + " " + w.opt.value
	}
	s := c.set
	if s.writes != nil {
		if line := s.writes(m, w.opt.value); line != "" {
			return strings.ReplaceAll(line, "\n", " · ")
		}
	}
	if strings.EqualFold(w.opt.value, s.fallback) || (s.numeric && sameValue(w.opt.value, s.fallback)) {
		return s.key + " back to its default"
	}
	return s.key + " = " + w.opt.value
}

func sameValue(a, b string) bool {
	x, okA := parseNumber(a)
	y, okB := parseNumber(b)
	return okA && okB && x == y
}

// willWrite is the line under a page: what Next would write.
func (g *guided) willWrite(m *Model, pg gPage) string {
	ws := g.pending(m, pg)
	if len(ws) == 0 {
		for _, q := range pg.qs {
			if !q.filter && q.kind != gAction {
				return "Nothing to write: Next keeps what you have"
			}
		}
		return ""
	}
	lines := make([]string, len(ws))
	for i, w := range ws {
		lines[i] = m.writeLine(w)
	}
	return "Will write " + strings.Join(lines, " · ")
}

// commit writes a page's answers through their commands, as one step of the
// session's history: the key that asked for it began that step.
func (g *guided) commit(m *Model, pg gPage) tea.Cmd {
	ws := g.pending(m, pg)
	var cmds []tea.Cmd
	for _, w := range ws {
		opt := w.opt
		cmds = append(cmds, m.runCommand(m.commandNamed(w.cmd), "", &opt))
	}
	for _, q := range pg.qs {
		if !q.filter {
			delete(g.picks, q.id)
		}
	}
	if len(ws) > 1 {
		// Each command said what it wrote; one line says it for the page.
		m.info("%s — written; %s", keysWord(writtenKeys(m, ws)), m.reloadWord())
	}
	return tea.Batch(cmds...)
}

// writtenKeys names the options a list of writes set.
func writtenKeys(m *Model, ws []gWrite) []string {
	var out []string
	for _, w := range ws {
		line := m.writeLine(w)
		key, _, _ := strings.Cut(line, " ")
		out = append(out, strings.TrimSuffix(key, ":"))
	}
	return out
}

// --- moving through the steps -----------------------------------------------------

// goTo shows a page. A theme only looked at on the way is put back unless it
// was chosen.
func (g *guided) goTo(m *Model, step, page int) tea.Cmd {
	step = clampInt(step, 0, gSteps-1)
	if g.step == gColours && (step != gColours || page != g.page) {
		g.unpreview(m)
	}
	g.step, g.page, g.focus = step, page, 0
	g.reached = maxInt(g.reached, step)
	if step == gFont {
		return tea.Batch(m.ensureGuideFonts(g), m.wantGuideSpecimens(g))
	}
	return nil
}

// unpreview shows the applied theme again when another was only looked at.
func (g *guided) unpreview(m *Model) {
	delete(g.picks, "theme")
	if m.cur != nil && m.cur.Name != m.applied && !m.dirty {
		if t, ok := m.lib.Get(m.applied); ok {
			m.previewTheme(t)
		}
	}
}

// next writes the page and goes on: to the next page of the step, or to the
// first of the next step. On Done it does what is highlighted.
func (g *guided) next(m *Model) tea.Cmd {
	pg, pages := g.at(m)
	if g.step == gDone {
		q := pg.qs[0]
		if i := g.sel(m, q); i >= 0 && q.choices[i].act != nil {
			return q.choices[i].act(m, g)
		}
		return nil
	}
	cmd := g.commit(m, pg)
	if g.page+1 < len(pages) {
		return tea.Batch(cmd, g.goTo(m, g.step, g.page+1))
	}
	return tea.Batch(cmd, g.goTo(m, g.step+1, 0))
}

// back goes to the page before, which may be the last of the step before.
func (g *guided) back(m *Model) tea.Cmd {
	if g.page > 0 {
		return g.goTo(m, g.step, g.page-1)
	}
	if g.step == 0 {
		return nil
	}
	return g.goTo(m, g.step-1, len(g.pages(m, g.step-1))-1)
}

// skip leaves the step as it is: nothing on its pages is written.
func (g *guided) skip(m *Model) tea.Cmd {
	for _, pg := range g.pages(m, g.step) {
		for _, q := range pg.qs {
			if !q.filter {
				delete(g.picks, q.id)
			}
		}
	}
	return g.goTo(m, g.step+1, 0)
}

// stepAside opens one of Atelier's screens, for more than the guide asks:
// the wall of every theme, or every setting of the step's section.
func (g *guided) stepAside(m *Model, themes bool) tea.Cmd {
	if g.step == gDone {
		return nil
	}
	g.away = true
	if themes || g.step == gColours {
		g.unpreview(m)
		m.setPromptText("Theme ")
	} else {
		m.jumpToGroup(gStepGroups[g.step])
	}
	m.info("Esc goes back to the guide")
	return nil
}

// comeBack ends a step aside.
func (g *guided) comeBack(m *Model) {
	g.away = false
	if m.editing {
		m.stopEdit()
	}
	m.setPromptText("")
	m.renderStatus()
}

// startAgain is Done's way back to the first step.
func (g *guided) startAgain(m *Model) tea.Cmd {
	g.picks = nil
	g.more = 0
	g.reached = 0
	return g.goTo(m, 0, 0)
}

// --- the profile --------------------------------------------------------------

func (g *guided) enter(m *Model) tea.Cmd {
	// The guide says what it is on every page: there is no welcome first.
	if !g.started {
		g.started = true
		g.step, g.page = 0, 0
	}
	if g.step == gFont {
		return tea.Batch(m.ensureGuideFonts(g), m.wantGuideSpecimens(g))
	}
	return nil
}

func (g *guided) explainTarget(m *Model) (string, bool) {
	if g.away {
		return m.explainTarget()
	}
	pg, _ := g.at(m)
	return pg.qs[g.focus].doc, false
}

func (g *guided) key(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if g.away {
		if msg.String() == "esc" && !m.editing {
			g.comeBack(m)
			return m, nil
		}
		return atelier{}.key(m, msg)
	}
	pg, _ := g.at(m)
	q := pg.qs[g.focus]
	switch msg.String() {
	case "up", "k", "shift+up":
		return m, g.pick(m, q, maxInt(g.sel(m, q), 0)-1)
	case "down", "j", "shift+down":
		return m, g.pick(m, q, g.sel(m, q)+1)
	case "tab":
		g.focus = (g.focus + 1) % len(pg.qs)
	case "shift+tab":
		g.focus = (g.focus + len(pg.qs) - 1) % len(pg.qs)
	case "enter", "right", "l", "space":
		return m, g.next(m)
	case "left", "h", "backspace":
		return m, g.back(m)
	case "esc":
		if g.step == 0 {
			return m.quit()
		}
		return m, g.goTo(m, g.step-1, 0)
	case "s":
		if g.step != gDone {
			return m, g.skip(m)
		}
	case "m":
		if q.kind == gCards {
			g.more++
			delete(g.picks, "theme")
		}
	case "/":
		if g.step == gColours {
			return m, g.stepAside(m, true)
		}
	case "a":
		return m, g.stepAside(m, false)
	case "1", "2", "3", "4", "5", "6":
		return m, g.goTo(m, int(msg.String()[0]-'1'), 0)
	case "f1", "?":
		m.openMessage(guidedHelp)
	case "ctrl+t":
		return m, m.tryHere()
	case "q":
		return m.quit()
	}
	return m, nil
}

// guidedHelp is F1 in the guide.
const guidedHelp = `The guide asks a few questions in six steps. Nothing is written until you go on.

↑ ↓  choose an answer        Tab  the next question on the page
Enter or →  write it and go on        ← or Backspace  the page before
Esc  the step before        1 to 6  a step        s  skip this step
m  three more themes        /  every theme        a  every setting of the step
F3  Ghostty's page on the setting        Ctrl+Z  undo the last step        F2  the terminal behind
F4  ghostty-config's own settings

Fine-tune everything later in Atelier or Palette: Tool → Interface → Layout, or the last step.`

func (g *guided) mouse(m *Model, msg mouseEv) (tea.Model, tea.Cmd) {
	if g.away {
		return atelier{}.mouse(m, msg)
	}
	if msg.wheel {
		pg, _ := g.at(m)
		q := pg.qs[g.focus]
		d := 1
		if msg.Button == tea.MouseWheelUp {
			d = -1
		}
		return m, g.pick(m, q, clampInt(g.sel(m, q)+d, 0, len(q.choices)-1))
	}
	r, ok := m.hit(msg.X, msg.Y)
	if !ok || r.kind != hitButton {
		return m, nil
	}
	return m, g.press(m, r.name, r.index)
}

// press answers a click on one of the guide's regions.
func (g *guided) press(m *Model, name string, index int) tea.Cmd {
	switch {
	case name == "g-next":
		return g.next(m)
	case name == "g-back":
		return g.back(m)
	case name == "g-skip":
		return g.skip(m)
	case name == "g-help":
		m.openMessage(guidedHelp)
	case name == "g-settings":
		m.openSettings()
	case name == "g-more":
		g.more++
		delete(g.picks, "theme")
	case name == "g-all-themes":
		return g.stepAside(m, true)
	case name == "g-all":
		return g.stepAside(m, false)
	case name == "g-step":
		return g.goTo(m, index, 0)
	case strings.HasPrefix(name, "g-q-"):
		// g-q-<question>: an answer, by its place among the answers.
		pg, _ := g.at(m)
		qi, _ := strconv.Atoi(strings.TrimPrefix(name, "g-q-"))
		if qi < 0 || qi >= len(pg.qs) {
			return nil
		}
		g.focus = qi
		q := pg.qs[qi]
		// A second click on an answer goes on with it, as Enter would.
		double := m.isDouble(region{kind: hitButton, index: qi*100 + index, name: name})
		cmd := g.pick(m, q, index)
		if q.kind == gAction || double {
			return tea.Batch(cmd, g.next(m))
		}
		return cmd
	}
	return nil
}

// primary is what the accent button says.
func (g *guided) primary(m *Model, pg gPage, pages []gPage) string {
	if g.step == gDone {
		q := pg.qs[0]
		if i := g.sel(m, q); i >= 0 {
			return q.choices[i].label + " →"
		}
		return ""
	}
	for _, q := range pg.qs {
		if q.kind == gCards {
			if i := g.sel(m, q); i >= 0 && !m.choiceIs(q.choices[i]) {
				return "Use " + q.choices[i].label + " →"
			}
		}
	}
	if g.page+1 < len(pages) {
		return "Next →"
	}
	return "Next: " + strings.ToLower(gStepNames[g.step+1]) + " →"
}

// eyebrow is the line over the question: where the guide is, and what was
// answered on the way.
func (g *guided) eyebrow(pg gPage) string {
	s := fmt.Sprintf("Step %d of %d · %s", g.step+1, gSteps, gStepNames[g.step])
	if pg.eyebrow != "" {
		s += " · " + pg.eyebrow
	}
	return s
}
