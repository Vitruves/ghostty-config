package ghostty

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Ghostty 1.4 prints the documentation of one option or keybind action with
// `+explain-config`, as written for the version installed. It is markdown
// wrapped at 80 columns; ParseDoc turns it into blocks an interface can wrap
// to its own width.

// ErrExplainUnsupported is what an older Ghostty, without +explain-config,
// answers.
var ErrExplainUnsupported = errors.New("reading Ghostty's own documentation needs Ghostty 1.4 or later")

// explainTimeout bounds a call: the answer takes milliseconds.
var explainTimeout = 5 * time.Second

// explained caches answers per binary and name: the documentation does not
// change while the tool runs. Timeouts are not kept, so a slow start is
// asked again.
var explained = struct {
	sync.Mutex
	m map[string]explainResult
}{m: map[string]explainResult{}}

type explainResult struct {
	text string
	err  error
}

// Explain returns Ghostty's documentation of an option, or of a keybind
// action when action is true. An action's parameter, goto_tab:1, is dropped.
func Explain(binary, name string, action bool) (string, error) {
	if binary == "" {
		return "", ErrNoBinary
	}
	if action {
		name, _, _ = strings.Cut(name, ":")
	}
	flag := "--option="
	if action {
		flag = "--keybind="
	}
	ck := binary + "\x00" + flag + name
	explained.Lock()
	r, ok := explained.m[ck]
	explained.Unlock()
	if ok {
		return r.text, r.err
	}
	ctx, cancel := context.WithTimeout(context.Background(), explainTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "+explain-config", "--no-pager", flag+name).CombinedOutput()
	if ctx.Err() != nil {
		return "", fmt.Errorf("Ghostty took too long to answer")
	}
	r = explainResult{text: strings.TrimSpace(string(out))}
	if err != nil {
		r.text = ""
		r.err = explainError(string(out), name)
	}
	explained.Lock()
	explained.m[ck] = r
	explained.Unlock()
	return r.text, r.err
}

// explainError reads a failed call: Ghostty 1.4 says Unknown: 'name' for
// a name it does not document; anything else is an older Ghostty that
// does not know the command.
func explainError(out, name string) error {
	if strings.Contains(out, "Unknown: '") {
		return fmt.Errorf("Ghostty has no documentation for %s", name)
	}
	return ErrExplainUnsupported
}

// DocKind is what a block of documentation is.
type DocKind int

const (
	DocPara    DocKind = iota // prose, to be wrapped
	DocBullet                 // an item of a list, its marker dropped
	DocCode                   // lines kept as they are
	DocHeading                // a markdown heading, its hashes dropped
)

// DocBlock is a paragraph, an item, a heading or a block of code.
type DocBlock struct {
	Kind DocKind
	// Indent is the column the text starts at in the source; an item's is
	// the column of its marker.
	Indent int
	// Text is the block's lines joined by spaces, inline markdown kept, for
	// DocSpans to read; Lines are a code block's lines.
	Text  string
	Lines []string
	// Gap is whether an empty line came before it.
	Gap bool
}

// ParseDoc splits the documentation into blocks. A paragraph indented
// under no list is taken for an example and kept line for line, as fenced
// code is.
func ParseDoc(text string) []DocBlock {
	var out []DocBlock
	var cur *DocBlock
	gap, fenced := false, false
	end := func() { cur = nil }
	listed := func() bool {
		if len(out) == 0 {
			return false
		}
		last := out[len(out)-1]
		return last.Kind == DocBullet || (last.Kind == DocPara && last.Indent > 0)
	}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\t", "    "), "\n") {
		line = strings.TrimRight(line, " \r")
		trim := strings.TrimLeft(line, " ")
		ind := len(line) - len(trim)
		if strings.HasPrefix(trim, "```") {
			if !fenced {
				out = append(out, DocBlock{Kind: DocCode, Indent: ind, Gap: gap})
				cur, gap = &out[len(out)-1], false
			} else {
				end()
			}
			fenced = !fenced
			continue
		}
		if fenced {
			cur.Lines = append(cur.Lines, strings.TrimPrefix(line, strings.Repeat(" ", min(ind, cur.Indent))))
			continue
		}
		if trim == "" {
			end()
			gap = len(out) > 0
			continue
		}
		if strings.HasPrefix(trim, "#") {
			if h := strings.TrimLeft(trim, "#"); strings.HasPrefix(h, " ") {
				out = append(out, DocBlock{Kind: DocHeading, Text: strings.TrimSpace(h), Gap: gap})
				end()
				gap = false
				continue
			}
		}
		if strings.HasPrefix(trim, "* ") || strings.HasPrefix(trim, "- ") {
			out = append(out, DocBlock{Kind: DocBullet, Indent: ind, Text: strings.TrimSpace(trim[2:]), Gap: gap})
			cur, gap = &out[len(out)-1], false
			continue
		}
		if cur != nil {
			if cur.Kind == DocCode {
				cur.Lines = append(cur.Lines, line[min(ind, cur.Indent):])
			} else {
				cur.Text += " " + trim
			}
			continue
		}
		if ind > 0 && !listed() {
			out = append(out, DocBlock{Kind: DocCode, Indent: ind, Lines: []string{trim}, Gap: gap})
		} else {
			out = append(out, DocBlock{Kind: DocPara, Indent: ind, Text: trim, Gap: gap})
		}
		cur, gap = &out[len(out)-1], false
	}
	return out
}

// DocSpan is a run of text, in code or not.
type DocSpan struct {
	Text string
	Code bool
}

// DocSpans reads the inline markdown of a block's text: `code` loses its
// backticks and is marked, a link [text](url) reads text (url), and the
// stars of *emphasis* go.
func DocSpans(text string) []DocSpan {
	var out []DocSpan
	add := func(s string, code bool) {
		if s == "" {
			return
		}
		if n := len(out); n > 0 && out[n-1].Code == code {
			out[n-1].Text += s
			return
		}
		out = append(out, DocSpan{s, code})
	}
	for text != "" {
		i := strings.IndexByte(text, '`')
		if i < 0 {
			add(plainMarkdown(text), false)
			break
		}
		j := strings.IndexByte(text[i+1:], '`')
		if j < 0 {
			add(plainMarkdown(text), false)
			break
		}
		add(plainMarkdown(text[:i]), false)
		add(text[i+1:i+1+j], true)
		text = text[i+2+j:]
	}
	return out
}

// plainMarkdown drops the markup of prose outside code.
func plainMarkdown(s string) string {
	// Links: [text](url) reads text (url), or the url alone when they match.
	for {
		i := strings.Index(s, "](")
		if i < 0 {
			break
		}
		open := strings.LastIndexByte(s[:i], '[')
		close := strings.IndexByte(s[i+2:], ')')
		if open < 0 || close < 0 {
			break
		}
		label, url := s[open+1:i], s[i+2:i+2+close]
		repl := label + " (" + url + ")"
		if label == url {
			repl = url
		}
		s = s[:open] + repl + s[i+3+close:]
	}
	// Emphasis: a star against a word on one side and not the other. A
	// star alone, as in 5 * 2 or a glob, stays.
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '*' {
			prev, next := byte(' '), byte(' ')
			if i > 0 {
				prev = s[i-1]
			}
			if i+1 < len(s) {
				next = s[i+1]
			}
			if (prev == ' ' || prev == '(') != (next == ' ' || next == '.' || next == ',' || next == ')') && next != '*' {
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
