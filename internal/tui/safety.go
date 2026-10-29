package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// What keeps the user's files safe from this tool: Ghostty is asked what it
// makes of every write, every step of the session can be taken back, what
// the session changed can be read line by line, and a backup can be put back.

// validateDebounce is how long writes must rest before Ghostty is asked to
// validate them, so a burst of writes, a preset or a walk through a list,
// costs one check.
var validateDebounce = 400 * time.Millisecond

// checkMode says who asked for a check, which decides what is said of it.
type checkMode int

const (
	// checkQuiet learns what the config looked like before the session
	// wrote anything, so that later checks tell only of what is new.
	checkQuiet checkMode = iota
	// checkWrite follows a write.
	checkWrite
	// checkAsked is the check command.
	checkAsked
)

type (
	validateTickMsg struct{ seq int }
	validateDoneMsg struct {
		seq      int
		mode     checkMode
		problems []ghostty.Problem
		err      error
		took     time.Duration
	}
)

// scheduleValidate arms the check that follows a write; a newer write
// rearms it, and only the newest one runs.
func (m *Model) scheduleValidate() tea.Cmd {
	if m.paths.Binary == "" {
		return nil
	}
	m.validateSeq++
	seq := m.validateSeq
	return tea.Tick(validateDebounce, func(time.Time) tea.Msg { return validateTickMsg{seq} })
}

// runCheck asks Ghostty now, in the background. Only the answer to the
// newest question is used: one that comes back after another was asked
// describes files that have changed since.
func (m *Model) runCheck(mode checkMode) tea.Cmd {
	if m.paths.Binary == "" {
		return nil
	}
	m.validateSeq++
	seq := m.validateSeq
	m.checking = true
	paths := m.paths
	return func() tea.Msg {
		start := time.Now()
		problems, err := ghostty.Validate(paths)
		return validateDoneMsg{seq: seq, mode: mode, problems: problems, err: err, took: time.Since(start)}
	}
}

// finishCheck takes in Ghostty's answer.
func (m *Model) finishCheck(msg validateDoneMsg) {
	if msg.seq != m.validateSeq {
		return
	}
	m.checking = false
	m.checkTook = msg.took
	defer func() {
		if m.cmd != nil && m.cmd.name == "check" {
			m.recompute()
		}
	}()
	if msg.err != nil {
		m.checkErr = msg.err
		if msg.mode == checkAsked {
			m.fail("Could not check: %v", msg.err)
		}
		return
	}
	before := m.problems
	m.problems, m.checked, m.checkErr = m.tree.Place(msg.problems), true, nil
	switch msg.mode {
	case checkAsked:
		if len(m.problems) == 0 {
			m.info("Ghostty accepts every line of your config · checked in %s", tookWord(msg.took))
		} else {
			m.warn("Ghostty rejects %s · checked in %s", countWord(len(m.problems), "line"), tookWord(msg.took))
		}
	case checkWrite:
		m.tellProblems(before)
	}
}

// tellProblems says what a check after a write found, without getting in
// the way: one line in the status, about the line most likely to be this
// session's doing. Problems that were already there before are not repeated
// on every write; the check command lists them.
func (m *Model) tellProblems(before []ghostty.Problem) {
	old := map[string]bool{}
	for _, p := range before {
		old[m.problemID(p)] = true
	}
	var fresh, ours []ghostty.Problem
	for _, p := range m.problems {
		if m.sessionWrote(p) {
			ours = append(ours, p)
		}
		if !old[m.problemID(p)] {
			fresh = append(fresh, p)
		}
	}
	if len(ours) == 0 && len(fresh) == 0 {
		if len(before) > 0 && len(m.problems) == 0 {
			m.info("Ghostty accepts every line of your config again")
		}
		return
	}
	p := firstOf(ours, fresh)
	// Who wrote the line and the way out come first: the status line is
	// cut at its end, and the message Ghostty gave can be long.
	var msg string
	switch {
	case len(ours) > 0 && m.tree.UndoSteps() > 0:
		msg = "Ghostty rejects a line this session wrote, Ctrl+Z takes it back"
	case len(ours) > 0:
		msg = "Ghostty rejects a line this session wrote"
	default:
		msg = "Ghostty rejects a line this session did not write"
	}
	msg += " · " + problemWord(p)
	if n := len(m.problems); n > 1 {
		msg += fmt.Sprintf(" · check lists all %d", n)
	}
	m.warn("%s", msg)
}

func firstOf(lists ...[]ghostty.Problem) ghostty.Problem {
	for _, l := range lists {
		if len(l) > 0 {
			return l[0]
		}
	}
	return ghostty.Problem{}
}

// problemID tells problems apart across checks by what they say and the
// line they are about, not where it sits: a line appended above moves it.
func (m *Model) problemID(p ghostty.Problem) string {
	line, _ := m.tree.LineAt(p)
	return p.Key + "\x00" + p.Message + "\x00" + filepath.Base(p.Path) + "\x00" + line
}

// sessionWrote is whether this session's steps put the problem's line there.
func (m *Model) sessionWrote(p ghostty.Problem) bool {
	line, ok := m.tree.LineAt(p)
	return ok && m.tree.Wrote(p.Path, line)
}

// problemWord renders a problem for one line: where, which key, what.
func problemWord(p ghostty.Problem) string {
	s := p.Message
	if p.Key != "" && !strings.Contains(p.Message, p.Key) {
		s = p.Key + ": " + p.Message
	}
	if loc := p.Location(); loc != "" {
		s = loc + " " + s
	}
	return s
}

func tookWord(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1f s", d.Seconds())
}

func countWord(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// --- undo -----------------------------------------------------------------

// undoStep takes back the last step of the session, writes the files and
// reloads Ghostty, as for any change.
func (m *Model) undoStep() tea.Cmd {
	theme := m.tree.Value("theme", "")
	done, ok := m.tree.Undo()
	if !ok {
		m.info("Nothing to undo: this session has not changed your config")
		return nil
	}
	m.readApplied()
	m.readFont()
	if m.tree.Value("theme", "") != theme && !m.dirty {
		// The theme went back: show the one in effect, as Enter would have.
		m.landOnApplied()
		m.tintApplied()
	}
	what := "your last change"
	if len(done.Keys) > 0 {
		what = keysWord(done.Keys)
	}
	note := ""
	if done.Kept > 0 {
		note = fmt.Sprintf(" · %s edited elsewhere since left as it is", countWord(done.Kept, "place"))
	}
	m.info("Undone: %s — written; %s%s", what, m.reloadWord(), note)
	cmd := m.flushWritesCmd()
	m.recompute()
	return cmd
}

// keysWord names a few settings and counts the rest.
func keysWord(keys []string) string {
	if len(keys) <= 3 {
		return strings.Join(keys, ", ")
	}
	return strings.Join(keys[:2], ", ") + fmt.Sprintf(" and %d more", len(keys)-2)
}

// --- the commands -----------------------------------------------------------

// safetyCommands are the commands of the Tool section that look after the
// files: undo, changes, check and restore.
func (m *Model) safetyCommands() []*command {
	return []*command{
		{name: "undo", group: "Tool", syntax: "undo",
			desc:    "Take back the last change this session made to your config files, a theme, a preset, a font, one step at a time; Ctrl+Z does it from anywhere. A line edited elsewhere since stays as it is",
			run:     func(m *Model, arg string, opt *option) tea.Cmd { return m.undoStep() },
			preview: previewUndoPanel,
		},
		{name: "changes", group: "Tool", syntax: "changes",
			desc:    "What is different in each config file since the editor started, line by line",
			options: func(m *Model, arg string) []option { return m.changesOptions() },
			run: func(m *Model, arg string, opt *option) tea.Cmd {
				if m.tree.UndoSteps() > 0 {
					m.info("Ctrl+Z takes back the last step; restore puts back a backup")
				}
				return nil
			},
			preview: previewChangesPanel,
		},
		{name: "check", group: "Tool", syntax: "check",
			desc:    "Ask Ghostty to read your config and list every line it rejects; it is asked after every write too",
			options: func(m *Model, arg string) []option { return m.checkOptions() },
			onMove: func(m *Model, opt *option) tea.Cmd {
				// Opening the list asks, unless an answer is on its way.
				if !m.checking && !m.checkOpened {
					m.checkOpened = true
					return m.runCheck(checkAsked)
				}
				return nil
			},
			run: func(m *Model, arg string, opt *option) tea.Cmd {
				if m.paths.Binary == "" {
					m.warn("Ghostty's program was not found: there is nothing to check with")
					return nil
				}
				m.info("Asking Ghostty…")
				return m.runCheck(checkAsked)
			},
			keepArg: true,
			preview: previewCheckPanel,
		},
		{name: "restore", group: "Tool", syntax: "restore",
			desc:    "Put a backup back over the file it was taken of, after asking; the file as it is now is backed up first",
			options: func(m *Model, arg string) []option { return m.restoreOptions() },
			run: func(m *Model, arg string, opt *option) tea.Cmd {
				if opt == nil || opt.value == "" {
					return nil
				}
				m.confirmRestore(opt.value)
				return nil
			},
			preview: previewRestorePanel,
		},
	}
}

// --- changes ------------------------------------------------------------------

// changedFiles lists the files whose text differs from the session's start,
// in load order, with the diff of each.
func (m *Model) changedFiles() []fileDiff {
	now := m.tree.Texts()
	var out []fileDiff
	for _, d := range m.tree.Docs {
		lines := now[d.Path]
		start := m.startText[d.Path]
		if sameStrings(start, lines) {
			continue
		}
		out = append(out, fileDiff{path: d.Path, diff: ghostty.Diff(start, lines)})
	}
	return out
}

type fileDiff struct {
	path string
	diff []ghostty.DiffLine
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (m *Model) changesOptions() []option {
	files := m.changedFiles()
	if len(files) == 0 {
		return []option{{label: "Nothing has changed since the editor started", header: true}}
	}
	var out []option
	for _, f := range files {
		out = append(out, option{label: fileWord(f.path), detail: diffCountWord(f.diff), value: f.path})
	}
	return out
}

// fileWord names a config file by its directory and name, ghostty/config,
// which tells the XDG file from the one in Application Support without the
// whole path.
func fileWord(path string) string {
	return filepath.Join(filepath.Base(filepath.Dir(path)), filepath.Base(path))
}

// diffCountWord is "+3 −1", leaving out a side with nothing.
func diffCountWord(d []ghostty.DiffLine) string {
	added, removed := ghostty.DiffCounts(d)
	if added == 0 && removed == 0 {
		return "same as now"
	}
	var parts []string
	if added > 0 {
		parts = append(parts, fmt.Sprintf("+%d", added))
	}
	if removed > 0 {
		parts = append(parts, fmt.Sprintf("−%d", removed))
	}
	return strings.Join(parts, " ")
}

func previewChangesPanel(m *Model, opt *option, width int) []string {
	c := m.c
	if opt == nil {
		return para(c.mutedS(), "Every change this session makes is listed here, file by file, against the file as the editor found it. Ctrl+Z takes back the last step.", width)
	}
	for _, f := range m.changedFiles() {
		if f.path != opt.value {
			continue
		}
		lines := []string{
			c.bold(c.fg).Render(truncate(fileWord(f.path), width)),
			c.mutedS().Render(truncate(shortenPath(f.path), width)),
			c.mutedS().Render(truncate("since the editor started  ·  "+diffCountWord(f.diff), width)),
			"",
		}
		return append(lines, m.diffLines(f.diff, width)...)
	}
	return nil
}

// --- undo panel -----------------------------------------------------------------

func previewUndoPanel(m *Model, opt *option, width int) []string {
	c := m.c
	lines := []string{c.bold(c.fg).Render(truncate("Undo", width))}
	lines = append(lines, para(c.mutedS(), m.cmd.desc+".", width)...)
	lines = append(lines, "")
	switch n := m.tree.UndoSteps(); n {
	case 0:
		lines = append(lines, c.mutedS().Render(truncate("Nothing to undo yet.", width)))
	default:
		lines = append(lines, c.base().Render(truncate(countWord(n, "step")+" can be taken back. Enter takes the last one.", width)))
	}
	return lines
}

// --- check ----------------------------------------------------------------------

func (m *Model) checkOptions() []option {
	switch {
	case m.paths.Binary == "":
		return []option{{label: "Ghostty's program was not found", detail: "nothing to check with", header: true}}
	case m.checkErr != nil && !m.checking:
		return []option{{label: "Could not ask Ghostty", detail: m.checkErr.Error(), header: true}}
	case !m.checked:
		return []option{{label: "Asking Ghostty…", header: true}}
	case len(m.problems) == 0:
		return []option{{label: "No problems", detail: "Ghostty accepts every line · " + tookWord(m.checkTook), header: true}}
	}
	var out []option
	for i, p := range m.problems {
		label := p.Location()
		if label == "" {
			label = "config"
		}
		if p.Key != "" {
			label += "  " + p.Key
		}
		out = append(out, option{label: label, detail: p.Message, value: strconv.Itoa(i)})
	}
	return out
}

func previewCheckPanel(m *Model, opt *option, width int) []string {
	c := m.c
	if opt == nil {
		lines := []string{c.bold(c.fg).Render(truncate("Check", width))}
		lines = append(lines, para(c.mutedS(), "Ghostty reads the config the way it would at a reload, and says which lines it rejects: a key it does not know, a value it cannot read, a theme it cannot find. Enter asks again.", width)...)
		if m.checked && m.checkErr == nil {
			lines = append(lines, "", c.text(c.ok).Render(truncate("Checked in "+tookWord(m.checkTook)+".", width)))
		}
		return lines
	}
	i, err := strconv.Atoi(opt.value)
	if err != nil || i < 0 || i >= len(m.problems) {
		return nil
	}
	p := m.problems[i]
	lines := []string{
		truncate(c.bold(c.fg).Render(orWord(p.Key, "Problem"))+c.mutedS().Render("  "+p.Location()), width),
	}
	lines = append(lines, para(c.text(c.danger), p.Message, width)...)
	lines = append(lines, "")
	// The line and its neighbours, as the file has them.
	if doc := m.docAt(p.Path); doc != nil && p.Line > 0 && p.Line <= len(doc.Lines) {
		numW := len(strconv.Itoa(minInt(len(doc.Lines), p.Line+2)))
		for n := maxInt(1, p.Line-2); n <= minInt(len(doc.Lines), p.Line+2); n++ {
			num := padLeft(strconv.Itoa(n), numW) + "  "
			text := doc.Lines[n-1].Raw
			if n == p.Line {
				lines = append(lines, m.tint(c.danger, num, text, true, width))
			} else {
				lines = append(lines, truncate(c.faintS().Render(num)+c.mutedS().Render(text), width))
			}
		}
		lines = append(lines, "")
	}
	switch {
	case m.sessionWrote(p) && m.tree.UndoSteps() > 0:
		lines = append(lines, para(c.base(), "This session wrote this line. Ctrl+Z takes back the step that did.", width)...)
	case m.sessionWrote(p):
		lines = append(lines, para(c.base(), "This session wrote this line.", width)...)
	default:
		lines = append(lines, para(c.mutedS(), "This line was not written by this session. Fix it in the file, or restore a backup from before it.", width)...)
	}
	if p.Path != "" {
		lines = append(lines, "", c.mutedS().Render(truncate(shortenPath(p.Path), width)))
	}
	return lines
}

// docAt finds the loaded file a path names.
func (m *Model) docAt(path string) *ghostty.Document {
	for _, d := range m.tree.Docs {
		if d.Path == path {
			return d
		}
	}
	return nil
}

func orWord(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// --- restore --------------------------------------------------------------------

// restorable is a backup with the file it would go back over.
type restorable struct {
	b       ghostty.Backup
	target  string
	guessed bool
}

// restorables lists the backups grouped by the file each was taken of, the
// files in load order and the backups newest first.
func (m *Model) restorables() []restorable {
	backups, _ := ghostty.ListBackups(m.paths)
	byTarget := map[string][]restorable{}
	var order []string
	for _, b := range backups {
		target, guessed := m.tree.BackupTarget(b)
		if _, ok := byTarget[target]; !ok {
			order = append(order, target)
		}
		byTarget[target] = append(byTarget[target], restorable{b, target, guessed})
	}
	rank := func(path string) int {
		for i, d := range m.tree.Docs {
			if d.Path == path {
				return i
			}
		}
		return len(m.tree.Docs)
	}
	// Files in the order Ghostty loads them; ListBackups gave the backups of
	// each newest first already.
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && rank(order[j]) < rank(order[j-1]); j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
	var out []restorable
	for _, t := range order {
		out = append(out, byTarget[t]...)
	}
	return out
}

func (m *Model) restoreOptions() []option {
	rs := m.restorables()
	// The panel is drawn on every frame; it looks the highlighted backup up
	// in this list rather than reading the directory again.
	m.restoreList = rs
	if len(rs) == 0 {
		return []option{{label: "No backups yet", detail: "each file is backed up before its first change", header: true}}
	}
	minutes := map[string]int{}
	for _, r := range rs {
		minutes[r.b.Taken.Format("200601021504")]++
	}
	var out []option
	group := "\x00"
	now := time.Now()
	for _, r := range rs {
		if r.target != group {
			group = r.target
			title, where := "Unknown file", ""
			if r.target != "" {
				title, where = fileWord(r.target), shortenPath(filepath.Dir(r.target))
			}
			if len(out) > 0 {
				out = append(out, option{header: true})
			}
			out = append(out, option{label: title, detail: where, header: true})
		}
		label := whenWord(r.b.Taken, now, minutes[r.b.Taken.Format("200601021504")] > 1)
		detail := "same as now"
		if d := m.restoreDiff(r); d != nil {
			detail = diffCountWord(d)
		}
		out = append(out, option{label: label, detail: detail, value: r.b.Path})
	}
	return out
}

// restoreDiff is what restoring would change: the file as it is now against
// the backup. nil when they are the same.
func (m *Model) restoreDiff(r restorable) []ghostty.DiffLine {
	backup, err := readLines(r.b.Path)
	if err != nil {
		return nil
	}
	now, _ := readLines(r.target)
	if sameStrings(now, backup) {
		return nil
	}
	return ghostty.Diff(now, backup)
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ghostty.SplitLines(string(data)), nil
}

// whenWord says when a backup was taken the way a person would: today and
// yesterday by name, this week by weekday, then by date. Seconds are shown
// only to tell apart backups of the same minute.
func whenWord(t, now time.Time, seconds bool) string {
	clock := t.Format("15:04")
	if seconds {
		clock = t.Format("15:04:05")
	}
	day := func(x time.Time) time.Time {
		y, mo, d := x.Date()
		return time.Date(y, mo, d, 0, 0, 0, 0, x.Location())
	}
	days := int(day(now).Sub(day(t)).Hours()/24 + 0.5)
	switch {
	case days == 0:
		return "Today " + clock
	case days == 1:
		return "Yesterday " + clock
	case days > 1 && days < 7:
		return t.Format("Monday") + " " + clock
	case t.Year() == now.Year():
		return t.Format("2 January") + " " + clock
	}
	return t.Format("2 January 2006") + " " + clock
}

func (m *Model) findRestorable(path string) (restorable, bool) {
	for _, r := range m.restoreList {
		if r.b.Path == path {
			return r, true
		}
	}
	for _, r := range m.restorables() {
		if r.b.Path == path {
			return r, true
		}
	}
	return restorable{}, false
}

func previewRestorePanel(m *Model, opt *option, width int) []string {
	c := m.c
	if opt == nil {
		return para(c.mutedS(), "Each config file is copied into the backups before the editor first changes it, and again when it finds the file edited elsewhere. Pick one to see what putting it back would change.", width)
	}
	r, ok := m.findRestorable(opt.value)
	if !ok {
		return nil
	}
	lines := []string{
		truncate(c.bold(c.fg).Render(whenWord(r.b.Taken, time.Now(), true))+c.mutedS().Render("  "+r.b.Taken.Format("Mon 2 Jan 2006")), width),
	}
	if r.target == "" {
		return append(lines, para(c.text(c.warn), "No loaded config file is called "+r.b.Base+": there is nothing to restore it over.", width)...)
	}
	lines = append(lines, c.mutedS().Render(truncate("over "+shortenPath(r.target), width)))
	if r.guessed {
		lines = append(lines, para(c.text(c.warn), "An older backup that does not say which file it was taken of: matched to this one by its content.", width)...)
	}
	lines = append(lines, "")
	d := m.restoreDiff(r)
	if d == nil {
		return append(lines, c.text(c.ok).Render(truncate("Same as the file now: restoring changes nothing.", width)))
	}
	lines = append(lines, c.mutedS().Render(truncate("Restoring would change "+diffCountWord(d)+":", width)), "")
	lines = append(lines, m.diffLines(d, width)...)
	return append(append(lines, ""), para(c.mutedS(), "Enter asks first, then backs up the file as it is and puts this one back.", width)...)
}

// confirmRestore asks before a backup goes back over its file.
func (m *Model) confirmRestore(path string) {
	r, ok := m.findRestorable(path)
	if !ok {
		m.warn("That backup is gone")
		return
	}
	if r.target == "" {
		m.warn("No loaded config file is called %s", r.b.Base)
		return
	}
	// The directory and the name tell the files apart; a whole path would
	// not fit the question.
	text := fmt.Sprintf("Put back %s as it was %s? The file as it is now is backed up first. Undo starts over from the restored file.", fileWord(r.target), strings.ToLower(whenWord(r.b.Taken, time.Now(), false)))
	m.openConfirm(text, func(m *Model) tea.Cmd { return m.restore(r) })
}

// restore puts a backup back. Pending edits are written first, so that the
// copy taken of the file before it is replaced is the file as it really is.
// The tree is read again, the session's steps are forgotten, since they were
// made on a file that is gone, and Ghostty reloads as for any change.
func (m *Model) restore(r restorable) tea.Cmd {
	m.writeSeq++
	if _, err := m.tree.Save(); err != nil {
		m.fail("%v", err)
		return nil
	}
	if _, err := ghostty.RestoreBackup(m.paths, r.b, r.target); err != nil {
		m.fail("Could not restore: %v", err)
		return nil
	}
	tree, err := ghostty.Load(m.paths)
	if err != nil {
		m.fail("Restored, but the config could not be read again: %v", err)
		return nil
	}
	m.tree = tree
	m.readApplied()
	m.readFont()
	if !m.dirty {
		m.landOnApplied()
	}
	m.info("Restored %s from %s — the file it replaced is in the backups; %s", filepath.Base(r.target), strings.ToLower(whenWord(r.b.Taken, time.Now(), false)), m.reloadWord())
	m.recompute()
	return tea.Batch(m.reload(), m.scheduleValidate())
}

// --- the diff -----------------------------------------------------------------

// diffContext is how many unchanged lines are kept around each change.
const diffContext = 2

// diffLines draws a diff for the panels: added lines in green and removed
// ones in red, each on a faint wash of its colour, unchanged lines muted, and
// the runs of them between changes folded to a count.
func (m *Model) diffLines(d []ghostty.DiffLine, width int) []string {
	c := m.c
	keep := make([]bool, len(d))
	for i, l := range d {
		if l.Kind == ghostty.DiffSame {
			continue
		}
		for j := maxInt(0, i-diffContext); j <= minInt(len(d)-1, i+diffContext); j++ {
			keep[j] = true
		}
	}
	var out []string
	for i := 0; i < len(d); {
		if !keep[i] {
			j := i
			for j < len(d) && !keep[j] {
				j++
			}
			out = append(out, c.faintS().Render(truncate("  ··· "+countWord(j-i, "unchanged line"), width)))
			i = j
			continue
		}
		l := d[i]
		switch l.Kind {
		case ghostty.DiffSame:
			out = append(out, c.mutedS().Render(truncate("  "+l.Text, width)))
		case ghostty.DiffAdd:
			out = append(out, m.tint(c.ok, "+ ", l.Text, false, width))
		case ghostty.DiffDel:
			out = append(out, m.tint(c.danger, "- ", l.Text, false, width))
		}
		i++
	}
	return out
}

// tint writes a line in a colour on a wash of it across the panel's width:
// lead, a mark or a line number, in bold, then the text.
func (m *Model) tint(hex, lead, text string, strong bool, width int) string {
	wash := color.Blend(hex, m.c.bg, 0.86)
	st := on(hex, wash)
	line := st.Bold(true).Render(lead) + st.Bold(strong).Render(text)
	return m.c.fillWith(truncate(line, width), width, wash)
}

// --- steps ----------------------------------------------------------------------

// stepKeyFor is the history key of a key press. The keys that walk a list or
// step a value keep one step open while they act on the same command, so a
// walk through the fonts, each one written, is undone as one change; every
// other key begins a step of its own.
func (m *Model) stepKeyFor(key string) string {
	switch key {
	case "up", "down", "left", "right", "pgup", "pgdown", "home", "end", "ctrl+p", "ctrl+n":
		return m.stepKey()
	}
	return ""
}

// stepKey names what the arrows act on now: the open command, or on the
// menu the highlighted one.
func (m *Model) stepKey() string {
	switch {
	case m.editing:
		return "move:edit"
	case m.cmd != nil:
		return "move:" + m.cmd.name
	}
	if o := m.selected(); o != nil && o.cmd != nil {
		return "move:" + o.cmd.name
	}
	return "move:"
}
