package ghostty

import "strings"

// The session's history. Every edit the tool makes belongs to a step, one
// per thing the user did: applying a theme, a preset of a dozen keys, a font.
// The editor marks where a step begins with Checkpoint; Undo takes the last
// one back.
//
// A step is kept as the text of each file it changed, just before and just
// after its edits, never as the edits themselves. Replaying the session minus
// its last step would also replay the earlier steps over whatever the user
// changed by hand in between, and an older `font-size = 14` would quietly
// beat the 16 they typed in their editor since. A step's own before and after
// say exactly which lines it touched, so undoing it is a three-way merge: the
// change from after back to before is carried onto the file as it is now, and
// every other line, the user's edits included, stays as it is. A line the
// user changed since, on top of the step's own, is left alone and counted.
//
// The texts are taken from the documents in memory, which only see an edit
// made elsewhere when Save rereads the files. That reread cuts the open step
// in two at that moment, so that neither half ever holds the user's edit:
// the step undoes as both halves, newest first.

// segment is the before and after of the files a stretch of a step changed,
// by path, as raw lines.
type segment struct {
	before, after map[string][]string
}

// step is everything one user action changed.
type step struct {
	segs []segment
}

// openStep is the step being recorded: its finished segments, and the files
// as they were when the current segment began.
type openStep struct {
	key    string
	before map[string][]string
	segs   []segment
}

// snapshot copies the raw lines of every loaded document.
func (t *Tree) snapshot() map[string][]string {
	out := make(map[string][]string, len(t.Docs))
	for _, d := range t.Docs {
		out[d.Path] = rawLines(d)
	}
	return out
}

func rawLines(d *Document) []string {
	out := make([]string, len(d.Lines))
	for i, l := range d.Lines {
		out[i] = l.Raw
	}
	return out
}

// Checkpoint begins a new step: whatever is edited from now until the next
// checkpoint is undone in one go. A key that is not empty and the same as
// the open step's keeps that step open instead, so that walking a list of
// fonts or nudging a setting with the arrows, each move written, is undone
// as the one change it is to the person making it. A step that changed
// nothing leaves no trace.
func (t *Tree) Checkpoint(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.open != nil && key != "" && key == t.open.key {
		return
	}
	t.seal()
	t.open = &openStep{key: key, before: t.snapshot()}
}

// cut closes the open step's current segment at the documents as they are
// now and starts the next one from here. The caller holds mu.
func (t *Tree) cut() {
	if t.open == nil {
		return
	}
	now := t.snapshot()
	seg := segment{before: map[string][]string{}, after: map[string][]string{}}
	for path, lines := range now {
		// A document that was not loaded when the segment began came in with
		// a reread: it holds nothing of this step.
		was, ok := t.open.before[path]
		if !ok || sameLines(was, lines) {
			continue
		}
		seg.before[path], seg.after[path] = was, lines
	}
	if len(seg.after) > 0 {
		t.open.segs = append(t.open.segs, seg)
	}
	t.open.before = now
}

// seal files the open step into the history. A step that exactly takes back
// the one before it, such as a setting put back by hand or a shader that was
// not kept, removes both: there is nothing left to undo of either. The caller
// holds mu.
func (t *Tree) seal() {
	if t.open == nil {
		return
	}
	t.cut()
	segs := t.open.segs
	t.open = nil
	if len(segs) == 0 {
		return
	}
	s := step{segs: segs}
	if n := len(t.steps); n > 0 && cancels(t.steps[n-1], s) {
		t.steps = t.steps[:n-1]
		return
	}
	t.steps = append(t.steps, s)
}

// cancels reports whether next puts every file prev changed back the way
// prev found it, and touches nothing else.
func cancels(prev, next step) bool {
	if len(prev.segs) != 1 || len(next.segs) != 1 {
		return false
	}
	p, n := prev.segs[0], next.segs[0]
	if len(p.before) != len(n.after) {
		return false
	}
	for path, before := range p.before {
		after, ok := n.after[path]
		if !ok || !sameLines(before, after) || !sameLines(p.after[path], n.before[path]) {
			return false
		}
	}
	return true
}

// UndoSteps is how many steps Undo can take back.
func (t *Tree) UndoSteps() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := len(t.steps)
	if t.open != nil {
		if len(t.open.segs) > 0 {
			return n + 1
		}
		now := t.snapshot()
		for path, was := range t.open.before {
			if lines, ok := now[path]; ok && !sameLines(was, lines) {
				return n + 1
			}
		}
	}
	return n
}

// Undone says what an undo took back.
type Undone struct {
	// Keys are the settings whose lines changed, in file order.
	Keys []string
	// Files are the documents rewritten.
	Files []*Document
	// Kept counts the places where the file was edited elsewhere on top of
	// the step's own lines, which were left as they are now.
	Kept int
}

// Undo takes back the last step. It is recorded like any other edit, so a
// save that rereads files changed elsewhere does it again on the fresh copy,
// and it starts a new step of its own that holds nothing yet: the next
// Checkpoint does not mistake the undo for something to undo.
func (t *Tree) Undo() (Undone, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seal()
	defer func() { t.open = &openStep{before: t.snapshot()} }()
	if len(t.steps) == 0 {
		return Undone{}, false
	}
	s := t.steps[len(t.steps)-1]
	t.steps = t.steps[:len(t.steps)-1]
	var done Undone
	t.ops = append(t.ops, func(t *Tree) { t.revertStep(s) })
	done.Files, done.Kept = t.revertStep(s)
	done.Keys = stepKeys(s)
	return done, true
}

// revertStep carries every segment of a step, newest first, from its after
// back to its before onto the documents as they are.
func (t *Tree) revertStep(s step) ([]*Document, int) {
	var changed []*Document
	kept := 0
	for i := len(s.segs) - 1; i >= 0; i-- {
		seg := s.segs[i]
		for path, after := range seg.after {
			doc := t.doc(path)
			if doc == nil {
				// The file is no longer part of the configuration: what the
				// step wrote there went with it.
				continue
			}
			now := rawLines(doc)
			merged, clashes := merge3(after, now, seg.before[path])
			kept += clashes
			if sameLines(merged, now) {
				continue
			}
			entries := doc.CountEntries()
			doc.Lines = doc.Lines[:0]
			for _, raw := range merged {
				doc.Lines = append(doc.Lines, parseLine(raw))
			}
			// Taking back a line this tool appended removes an entry, which
			// the check in Save would otherwise read as a lost setting.
			if lost := entries - doc.CountEntries(); lost > 0 {
				doc.removed += lost
			}
			doc.Dirty = true
			changed = appendDoc(changed, doc)
		}
	}
	t.index()
	return changed, kept
}

// stepKeys lists the settings whose lines a step changed, live or commented
// out, in the order they first appear.
func stepKeys(s step) []string {
	seen := map[string]bool{}
	var out []string
	for _, seg := range s.segs {
		for path, after := range seg.after {
			for _, l := range Diff(seg.before[path], after) {
				if l.Kind == DiffSame {
					continue
				}
				if k := lineKey(l.Text); k != "" && !seen[k] {
					seen[k] = true
					out = append(out, k)
				}
			}
		}
	}
	return out
}

// lineKey is the key a line assigns, or the key of the assignment a line
// this tool commented out still shows.
func lineKey(raw string) string {
	l := parseLine(raw)
	if l.Kind == LineEntry {
		return l.Key
	}
	if l.Kind == LineComment {
		body := strings.TrimPrefix(strings.TrimSpace(raw), "#")
		if strings.Contains(body, "# disabled by ghostty-config") {
			if inner := parseLine(body); inner.Kind == LineEntry {
				return inner.Key
			}
		}
	}
	return ""
}

// ResetHistory forgets every step, as after a backup was restored over the
// files they were made on.
func (t *Tree) ResetHistory() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.steps = nil
	t.open = &openStep{before: t.snapshot()}
}

// Wrote reports whether the line raw in the file at path is one this
// session's steps put there.
func (t *Tree) Wrote(path, raw string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	segs := []segment{}
	for _, s := range t.steps {
		segs = append(segs, s.segs...)
	}
	if t.open != nil {
		segs = append(segs, t.open.segs...)
		if d := t.doc(path); d != nil {
			if was, ok := t.open.before[d.Path]; ok {
				segs = append(segs, segment{before: map[string][]string{d.Path: was}, after: map[string][]string{d.Path: rawLines(d)}})
			}
		}
	}
	for _, seg := range segs {
		for p, after := range seg.after {
			if !sameFile(p, path) {
				continue
			}
			for _, l := range Diff(seg.before[p], after) {
				if l.Kind == DiffAdd && l.Text == raw {
					return true
				}
			}
		}
	}
	return false
}

// Texts is the raw lines of every loaded file, by path.
func (t *Tree) Texts() map[string][]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshot()
}
