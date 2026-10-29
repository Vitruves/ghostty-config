package ghostty

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Problem is one complaint `ghostty +validate-config` has about the
// configuration: a key it does not know, a value it cannot read, a theme it
// cannot find.
type Problem struct {
	// Path and Line say where, Line counted from 1. Ghostty gives them when
	// it reads its own default files; for a file named on its command line it
	// gives only the key, and Place finds the line.
	Path string
	Line int
	// Key is the setting complained about, when the message names one.
	Key     string
	Message string
}

// Location renders file:line for messages, or just the file.
func (p Problem) Location() string {
	switch {
	case p.Path == "":
		return ""
	case p.Line == 0:
		return filepath.Base(p.Path)
	}
	return fmt.Sprintf("%s:%d", filepath.Base(p.Path), p.Line)
}

// ErrNoBinary is returned when there is no ghostty executable to ask.
var ErrNoBinary = errors.New("the ghostty executable was not found")

// ValidateTimeout bounds one run of the validator. It takes about ten
// milliseconds; anything near this means it hangs.
var ValidateTimeout = 10 * time.Second

// validateRuns lists the argument lists that make Ghostty read exactly the
// files the tool edits. Without arguments it loads its own default files, the
// same ones DefaultPaths lists, and names each line it complains about. A
// config given on the command line replaces them, and is named to it.
func validateRuns(paths Paths) [][]string {
	if paths.Defaults {
		return [][]string{{"+validate-config"}}
	}
	var runs [][]string
	for _, root := range paths.Roots {
		if _, err := os.Stat(root); err == nil {
			runs = append(runs, []string{"+validate-config", "--config-file=" + root})
		}
	}
	return runs
}

// Validate asks Ghostty what it makes of the configuration. Ghostty's own
// reader is the only one that knows every key and value of the version
// installed, which this tool cannot. Problems come back in Ghostty's order,
// each once.
func Validate(paths Paths) ([]Problem, error) {
	if paths.Binary == "" {
		return nil, ErrNoBinary
	}
	var out []Problem
	seen := map[Problem]bool{}
	for _, args := range validateRuns(paths) {
		ctx, cancel := context.WithTimeout(context.Background(), ValidateTimeout)
		cmd := exec.CommandContext(ctx, paths.Binary, args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		cancel()
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			return out, fmt.Errorf("could not run ghostty +validate-config: %w", err)
		}
		problems := ParseValidate(stdout.String())
		if err != nil && len(problems) == 0 {
			// It failed without saying why on its standard output, as it
			// does for a file it cannot open.
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = exit.String()
			}
			return out, fmt.Errorf("ghostty +validate-config failed: %s", msg)
		}
		for _, p := range problems {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out, nil
}

var (
	// /path/config:12:font-size: invalid value "abc"
	located = regexp.MustCompile(`^(.+):(\d+):([A-Za-z0-9_-]+): (.+)$`)
	// font-size: invalid value "abc"
	keyed = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9_-]*): (.+)$`)
	// theme "x" not found, tried path "/a/x"
	missingTheme = regexp.MustCompile(`^theme "(.*)" not found`)
)

// ParseValidate reads what `ghostty +validate-config` prints, one problem a
// line. A theme it cannot find is reported once for every directory it
// tried, and for each half of a light and dark pair: it is told once.
func ParseValidate(output string) []Problem {
	var out []Problem
	seen := map[Problem]bool{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var p Problem
		if m := located.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[2])
			p = Problem{Path: m[1], Line: n, Key: m[3], Message: m[4]}
		} else if m := missingTheme.FindStringSubmatch(line); m != nil {
			p = Problem{Key: "theme", Message: fmt.Sprintf("theme %q not found", m[1])}
		} else if m := keyed.FindStringSubmatch(line); m != nil {
			p = Problem{Key: m[1], Message: m[2]}
		} else {
			p = Problem{Message: line}
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// quotedValue is the value an "invalid value" message quotes.
var quotedValue = regexp.MustCompile(`"(.*)"`)

// Place finds the line of each problem Ghostty did not locate: the
// assignment of its key, the one whose value the message quotes when there
// are several, otherwise the one in effect.
func (t *Tree) Place(problems []Problem) []Problem {
	out := make([]Problem, len(problems))
	for i, p := range problems {
		out[i] = p
		if p.Line > 0 || p.Key == "" {
			continue
		}
		all := t.All(p.Key)
		if len(all) == 0 {
			continue
		}
		e := all[len(all)-1]
		if m := quotedValue.FindStringSubmatch(p.Message); m != nil {
			for _, cand := range all {
				if cand.Value == m[1] || strings.Contains(cand.Doc.Lines[cand.Line].Raw, m[1]) {
					e = cand
				}
			}
		}
		out[i].Path, out[i].Line = e.Doc.Path, e.Line+1
	}
	return out
}

// LineAt returns the text of a problem's line as the tree holds it.
func (t *Tree) LineAt(p Problem) (string, bool) {
	if p.Path == "" || p.Line == 0 {
		return "", false
	}
	d := t.doc(p.Path)
	if d == nil || p.Line > len(d.Lines) {
		return "", false
	}
	return d.Lines[p.Line-1].Raw, true
}
