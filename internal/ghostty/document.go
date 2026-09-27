// Package ghostty knows how Ghostty is configured: where its files live, how
// they are parsed and in what order they override each other, what a theme
// file contains, which fonts it can resolve, and how to show a palette in the
// running terminal before anything is written.
package ghostty

import (
	"fmt"
	"os"
	"strings"
)

// LineKind classifies one line of a config file.
type LineKind int

const (
	LineBlank   LineKind = iota
	LineComment          // starts with #, after optional indentation
	LineEntry            // key = value, or a bare key meaning true
)

// Line is one line of a config file, kept verbatim so a rewrite changes
// nothing the user did not ask to change.
type Line struct {
	Raw   string
	Kind  LineKind
	Key   string
	Value string // unquoted, trimmed
}

// Document is one config file with its lines. Comments, blank lines and
// formatting survive a round trip untouched; only the entries the tool sets
// are rewritten, and always as a whole line.
type Document struct {
	Path   string
	Lines  []Line
	Exists bool
	Dirty  bool
	// Ephemeral documents are theme files or fixtures parsed from text.
	trailingNewline bool
}

// ParseDocument reads a config file. A missing file yields an empty,
// non-existent document that can still be appended to and saved.
func ParseDocument(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Document{Path: path, trailingNewline: true}, nil
		}
		return nil, fmt.Errorf("could not read %s: %w", path, err)
	}
	doc := ParseText(string(data))
	doc.Path = path
	doc.Exists = true
	return doc, nil
}

// ParseText parses config text. It follows Ghostty's own reader: lines are
// trimmed, `#` starts a comment only at the start of a line, the first `=`
// splits key from value, and a value wrapped in double quotes has them
// removed. A trailing `# ...` is NOT a comment to Ghostty and is kept as part
// of the value, exactly as Ghostty would (mis)read it.
func ParseText(text string) *Document {
	doc := &Document{trailingNewline: strings.HasSuffix(text, "\n") || text == ""}
	body := strings.TrimSuffix(text, "\n")
	if body == "" && text != "" {
		return doc
	}
	if text == "" {
		return doc
	}
	for _, raw := range strings.Split(body, "\n") {
		doc.Lines = append(doc.Lines, parseLine(raw))
	}
	return doc
}

func parseLine(raw string) Line {
	trimmed := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
	switch {
	case trimmed == "":
		return Line{Raw: raw, Kind: LineBlank}
	case strings.HasPrefix(trimmed, "#"):
		return Line{Raw: raw, Kind: LineComment}
	}
	key, value, found := strings.Cut(trimmed, "=")
	key = strings.TrimSpace(key)
	if !found {
		// A bare key is a boolean set to true.
		return Line{Raw: raw, Kind: LineEntry, Key: key, Value: ""}
	}
	return Line{Raw: raw, Kind: LineEntry, Key: key, Value: unquote(strings.TrimSpace(value))}
}

// unquote removes one pair of surrounding double quotes, as Ghostty does.
func unquote(v string) string {
	if len(v) >= 2 && strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"") {
		return v[1 : len(v)-1]
	}
	return v
}

// FormatValue renders a value so Ghostty reads it back unchanged. Values are
// written bare; quotes are added only when they are needed to survive the
// trimming Ghostty applies.
func FormatValue(v string) string {
	if v == "" {
		return ""
	}
	if v != strings.TrimSpace(v) || strings.HasPrefix(v, "\"") {
		return "\"" + v + "\""
	}
	return v
}

// separatorOf reproduces the spacing a line used around its `=`, so a file
// written as `key=value` stays that way after an edit.
func separatorOf(raw string) string {
	trimmed := strings.TrimSpace(raw)
	i := strings.IndexByte(trimmed, '=')
	if i < 0 {
		return " = "
	}
	left := trimmed[:i]
	before := left[len(strings.TrimRight(left, " \t")):]
	after := trimmed[i+1:]
	afterWS := after[:len(after)-len(strings.TrimLeft(after, " \t"))]
	if before == "" && afterWS == "" {
		return "="
	}
	return before + "=" + afterWS
}

// indentOf returns the leading whitespace of a raw line.
func indentOf(raw string) string {
	return raw[:len(raw)-len(strings.TrimLeft(raw, " \t"))]
}

// SetLine rewrites line i as key = value, keeping its indentation and the
// spacing style of the original.
func (d *Document) SetLine(i int, key, value string) {
	if i < 0 || i >= len(d.Lines) {
		return
	}
	old := d.Lines[i]
	raw := indentOf(old.Raw) + key + separatorOf(old.Raw) + FormatValue(value)
	if old.Kind != LineEntry {
		raw = key + " = " + FormatValue(value)
	}
	d.Lines[i] = Line{Raw: raw, Kind: LineEntry, Key: key, Value: value}
	d.Dirty = true
}

// Append adds key = value at the end of the file, after a blank line when the
// last line has content, so the addition reads as its own paragraph.
func (d *Document) Append(key, value string) int {
	if n := len(d.Lines); n > 0 && d.Lines[n-1].Kind != LineBlank {
		d.Lines = append(d.Lines, Line{Kind: LineBlank})
	}
	raw := key + " = " + FormatValue(value)
	d.Lines = append(d.Lines, Line{Raw: raw, Kind: LineEntry, Key: key, Value: value})
	d.Dirty = true
	return len(d.Lines) - 1
}

// InsertAfter inserts key = value right after line i.
func (d *Document) InsertAfter(i int, key, value string) int {
	raw := key + " = " + FormatValue(value)
	line := Line{Raw: raw, Kind: LineEntry, Key: key, Value: value}
	if i < 0 || i >= len(d.Lines) {
		return d.Append(key, value)
	}
	d.Lines = append(d.Lines, Line{})
	copy(d.Lines[i+2:], d.Lines[i+1:])
	d.Lines[i+1] = line
	d.Dirty = true
	return i + 1
}

// CommentOut turns line i into a comment that explains itself, so the user
// can see what was disabled and put it back with one keystroke.
func (d *Document) CommentOut(i int, reason string) {
	if i < 0 || i >= len(d.Lines) || d.Lines[i].Kind != LineEntry {
		return
	}
	old := d.Lines[i]
	raw := indentOf(old.Raw) + "#" + strings.TrimSpace(old.Raw) + "  # " + reason
	d.Lines[i] = Line{Raw: raw, Kind: LineComment}
	d.Dirty = true
}

// RemoveLine deletes line i.
func (d *Document) RemoveLine(i int) {
	if i < 0 || i >= len(d.Lines) {
		return
	}
	d.Lines = append(d.Lines[:i], d.Lines[i+1:]...)
	d.Dirty = true
}

// Text renders the document back to config text.
func (d *Document) Text() string {
	if len(d.Lines) == 0 {
		return ""
	}
	var b strings.Builder
	for i, line := range d.Lines {
		b.WriteString(line.Raw)
		if i < len(d.Lines)-1 || d.trailingNewline || d.Dirty {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// Entries returns the index of every entry line, in file order.
func (d *Document) Entries() []int {
	var out []int
	for i, l := range d.Lines {
		if l.Kind == LineEntry {
			out = append(out, i)
		}
	}
	return out
}

// Get returns the last value of key in this file alone.
func (d *Document) Get(key string) (string, bool) {
	value, found := "", false
	for _, l := range d.Lines {
		if l.Kind == LineEntry && l.Key == key {
			value, found = l.Value, true
		}
	}
	return value, found
}

// CountEntries is the cheap proxy for "did this rewrite throw away part of
// the user's config": an edit may add or change lines, never lose entries
// it was not asked to remove.
func (d *Document) CountEntries() int {
	return len(d.Entries())
}
