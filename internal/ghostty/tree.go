package ghostty

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Entry is one assignment somewhere in the loaded configuration.
type Entry struct {
	Doc   *Document
	Line  int
	Key   string
	Value string
}

// Location renders file:line for messages.
func (e Entry) Location() string {
	return fmt.Sprintf("%s:%d", filepath.Base(e.Doc.Path), e.Line+1)
}

// Tree is the whole configuration Ghostty would load: every root file that
// exists, every file they include, in load order. It answers "what value is
// in effect for this key, and which line put it there", which is what makes
// an edit land in the right place whatever the user's layout is.
//
// Load order, verified against Ghostty 1.3: the root files in the order Paths
// lists them, then their `config-file` includes, breadth first, each loaded
// only after every file before it in the queue. For a scalar key the last
// assignment wins. For list keys (font-family, palette) every assignment
// contributes, and an empty value resets the list.
type Tree struct {
	Paths   Paths
	Docs    []*Document // load order
	Primary *Document   // where new keys are appended
	entries []Entry
	mu      sync.Mutex
}

// Load reads the configuration. Missing roots are skipped; if none exists the
// first root becomes the primary and is created on first save.
func Load(paths Paths) (*Tree, error) {
	t := &Tree{Paths: paths}
	seen := make(map[string]bool)
	var queue []string
	for _, root := range paths.Roots {
		if _, err := os.Stat(root); err == nil {
			queue = append(queue, root)
		}
	}
	if len(queue) == 0 {
		doc, err := ParseDocument(paths.Roots[0])
		if err != nil {
			return nil, err
		}
		t.Docs = []*Document{doc}
		t.Primary = doc
		t.index()
		return t, nil
	}

	// Breadth first: a file's includes are queued behind everything already
	// waiting, which is how Ghostty orders them.
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		abs, _ := filepath.Abs(path)
		if seen[abs] {
			continue
		}
		seen[abs] = true
		doc, err := ParseDocument(path)
		if err != nil {
			return nil, err
		}
		if !doc.Exists {
			continue
		}
		t.Docs = append(t.Docs, doc)
		for _, l := range doc.Lines {
			if l.Kind != LineEntry || l.Key != "config-file" || l.Value == "" {
				continue
			}
			include := strings.TrimPrefix(l.Value, "?")
			if !filepath.IsAbs(include) {
				include = filepath.Join(filepath.Dir(path), include)
			}
			queue = append(queue, include)
		}
	}
	// The primary is the last root that exists: whatever is appended there
	// outranks every other root.
	for i := len(t.Docs) - 1; i >= 0; i-- {
		for _, root := range paths.Roots {
			if sameFile(t.Docs[i].Path, root) {
				t.Primary = t.Docs[i]
				break
			}
		}
		if t.Primary != nil {
			break
		}
	}
	if t.Primary == nil {
		t.Primary = t.Docs[0]
	}
	t.index()
	return t, nil
}

func sameFile(a, b string) bool {
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return aa == bb
}

// index rebuilds the flat entry list from the documents.
func (t *Tree) index() {
	t.entries = t.entries[:0]
	for _, doc := range t.Docs {
		for i, l := range doc.Lines {
			if l.Kind == LineEntry {
				t.entries = append(t.entries, Entry{Doc: doc, Line: i, Key: l.Key, Value: l.Value})
			}
		}
	}
}

// Get returns the assignment in effect for a scalar key: the last one in
// load order.
func (t *Tree) Get(key string) (Entry, bool) {
	var out Entry
	found := false
	for _, e := range t.entries {
		if e.Key == key {
			out, found = e, true
		}
	}
	return out, found
}

// Value returns the effective value of a scalar key, or fallback.
func (t *Tree) Value(key, fallback string) string {
	if e, ok := t.Get(key); ok {
		return e.Value
	}
	return fallback
}

// All returns every assignment of a key in load order.
func (t *Tree) All(key string) []Entry {
	var out []Entry
	for _, e := range t.entries {
		if e.Key == key {
			out = append(out, e)
		}
	}
	return out
}

// Set makes key = value the effective value. When an assignment already wins
// it is rewritten in place, in whichever file holds it; otherwise the key is
// appended to the primary file, where nothing can outrank it because nothing
// else assigns it at all. It reports which file changed.
func (t *Tree) Set(key, value string) *Document {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.Get(key); ok {
		if e.Value == value {
			return nil
		}
		e.Doc.SetLine(e.Line, key, value)
		t.index()
		return e.Doc
	}
	t.Primary.Append(key, value)
	t.index()
	return t.Primary
}

// Unset removes every assignment of a key, which puts Ghostty's default back
// in effect. Lines are commented out rather than deleted so the previous
// value stays visible in the file.
func (t *Tree) Unset(key, reason string) []*Document {
	t.mu.Lock()
	defer t.mu.Unlock()
	var changed []*Document
	for _, e := range t.All(key) {
		e.Doc.CommentOut(e.Line, reason)
		changed = appendDoc(changed, e.Doc)
	}
	t.index()
	return changed
}

// FontFamilies returns the effective font-family list: entries accumulate
// across files in load order, and an empty value resets the list. The first
// element is the primary font; the rest are fallbacks Ghostty consults for
// codepoints the primary lacks.
func (t *Tree) FontFamilies() []string {
	var list []string
	for _, e := range t.All("font-family") {
		if e.Value == "" {
			list = list[:0]
			continue
		}
		list = append(list, e.Value)
	}
	return list
}

// SetPrimaryFont changes the family Ghostty draws with while leaving every
// fallback family in place. The line that produced the first effective
// family is rewritten; if there is none, the family is appended.
func (t *Tree) SetPrimaryFont(family string) *Document {
	t.mu.Lock()
	defer t.mu.Unlock()
	var first *Entry
	for _, e := range t.All("font-family") {
		if e.Value == "" {
			first = nil
			continue
		}
		if first == nil {
			e := e
			first = &e
		}
	}
	if first != nil {
		if first.Value == family {
			return nil
		}
		first.Doc.SetLine(first.Line, "font-family", family)
		t.index()
		return first.Doc
	}
	t.Primary.Append("font-family", family)
	t.index()
	return t.Primary
}

// Palette returns the effective palette overrides set outside theme files,
// index by index, last assignment winning.
func (t *Tree) Palette() map[int]string {
	out := make(map[int]string)
	for _, e := range t.All("palette") {
		idx, value, ok := parsePaletteValue(e.Value)
		if ok {
			out[idx] = value
		}
	}
	return out
}

// ColourOverrideKeys are the colour settings a config file may carry that
// take precedence over the theme, wherever they sit in the file. Ghostty
// replays them on top of the theme, so a `background =` left over from an
// earlier hand-written scheme silently defeats every theme chosen after it.
var ColourOverrideKeys = []string{
	"background", "foreground", "cursor-color", "cursor-text",
	"selection-background", "selection-foreground", "palette",
}

// ColourOverrides lists every assignment in the config that would shadow the
// theme's colours.
func (t *Tree) ColourOverrides() []Entry {
	var out []Entry
	for _, e := range t.entries {
		for _, k := range ColourOverrideKeys {
			if e.Key == k {
				out = append(out, e)
			}
		}
	}
	return out
}

// DisableColourOverrides comments out every colour override so the theme
// shows as intended. It returns the files it touched.
func (t *Tree) DisableColourOverrides() []*Document {
	t.mu.Lock()
	defer t.mu.Unlock()
	var changed []*Document
	for _, e := range t.ColourOverrides() {
		e.Doc.CommentOut(e.Line, "disabled by ghostty-config: it overrode the theme")
		changed = appendDoc(changed, e.Doc)
	}
	t.index()
	return changed
}

// FontFeatures returns the effective font-feature settings, flattened.
func (t *Tree) FontFeatures() []string {
	var out []string
	for _, e := range t.All("font-feature") {
		for _, f := range strings.Split(e.Value, ",") {
			if f = strings.TrimSpace(f); f != "" {
				out = append(out, f)
			}
		}
	}
	return out
}

// LigaturesDisabled reports whether calt has been switched off, which is the
// conventional way to disable programming ligatures.
func (t *Tree) LigaturesDisabled() bool {
	for _, f := range t.FontFeatures() {
		norm := strings.ReplaceAll(strings.ToLower(f), " ", "")
		if norm == "-calt" || norm == "caltoff" || norm == "calt=0" {
			return true
		}
	}
	return false
}

// noLigatures is the line written to turn programming ligatures off.
const noLigatures = "-calt, -liga, -dlig"

// SetLigatures enables or disables ligatures. Disabling adds the standard
// line; enabling removes the lines that disable calt, whoever wrote them.
func (t *Tree) SetLigatures(enabled bool) []*Document {
	t.mu.Lock()
	defer t.mu.Unlock()
	var changed []*Document
	if enabled {
		for _, e := range t.All("font-feature") {
			norm := strings.ReplaceAll(strings.ToLower(e.Value), " ", "")
			if strings.Contains(norm, "-calt") || strings.Contains(norm, "calt=0") || strings.Contains(norm, "caltoff") {
				e.Doc.CommentOut(e.Line, "disabled by ghostty-config: ligatures turned back on")
				changed = appendDoc(changed, e.Doc)
			}
		}
	} else if !t.LigaturesDisabled() {
		t.Primary.Append("font-feature", noLigatures)
		changed = append(changed, t.Primary)
	}
	t.index()
	return changed
}

// Dirty lists the documents with unsaved changes.
func (t *Tree) Dirty() []*Document {
	var out []*Document
	for _, d := range t.Docs {
		if d.Dirty {
			out = append(out, d)
		}
	}
	return out
}

// Save writes every changed file atomically, after backing up the original
// once. A rewrite that lost entries is refused: this file belongs to the
// user, and losing part of it is worse than failing.
func (t *Tree) Save() ([]string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var written []string
	for _, doc := range t.Docs {
		if !doc.Dirty {
			continue
		}
		if doc.Exists {
			original, err := os.ReadFile(doc.Path)
			if err == nil {
				before := ParseText(string(original)).CountEntries()
				disabled := 0
				for _, l := range doc.Lines {
					if l.Kind == LineComment && strings.Contains(l.Raw, "# disabled by ghostty-config") {
						disabled++
					}
				}
				if after := doc.CountEntries() + disabled; after < before {
					return written, fmt.Errorf("refusing to write %s: the rewrite would lose %d setting(s)", doc.Path, before-after)
				}
				if err := backup(t.Paths.StateDir, doc.Path, original); err != nil {
					return written, err
				}
			}
		}
		if err := writeFileAtomic(doc.Path, []byte(doc.Text()), 0o644); err != nil {
			return written, err
		}
		doc.Dirty = false
		doc.Exists = true
		written = append(written, doc.Path)
	}
	return written, nil
}

func appendDoc(docs []*Document, d *Document) []*Document {
	for _, x := range docs {
		if x == d {
			return docs
		}
	}
	return append(docs, d)
}

// parsePaletteValue reads "N=#rrggbb" in the forms Ghostty accepts.
func parsePaletteValue(v string) (int, string, bool) {
	idxText, colour, ok := strings.Cut(v, "=")
	if !ok {
		return 0, "", false
	}
	idxText = strings.TrimSpace(strings.ToLower(idxText))
	base := 10
	switch {
	case strings.HasPrefix(idxText, "0x"):
		base, idxText = 16, idxText[2:]
	case strings.HasPrefix(idxText, "0o"):
		base, idxText = 8, idxText[2:]
	case strings.HasPrefix(idxText, "0b"):
		base, idxText = 2, idxText[2:]
	}
	var idx int
	if _, err := fmt.Sscanf(idxText, map[int]string{10: "%d", 16: "%x", 8: "%o", 2: "%b"}[base], &idx); err != nil {
		return 0, "", false
	}
	if idx < 0 || idx > 255 {
		return 0, "", false
	}
	return idx, strings.TrimSpace(colour), true
}
