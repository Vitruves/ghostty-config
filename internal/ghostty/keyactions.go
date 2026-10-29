package ghostty

import (
	"bufio"
	"context"
	_ "embed"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// What Ghostty can bind and what it binds on its own. Both come from the
// binary, `+list-actions --docs` and `+list-keybinds --default`, so they
// match the version installed. A copy of what Ghostty 1.4 answers on macOS is
// kept in keydata for when the binary cannot be found or run.

//go:embed keydata/list-actions-docs.txt
var builtinActionDocs string

//go:embed keydata/list-keybinds-default-macos.txt
var builtinDefaultsMac string

// ActionDoc is one action Ghostty can bind, with its documentation.
type ActionDoc struct {
	Name string
	// Summary is the first paragraph of the documentation, Doc all of it.
	Summary string
	Doc     string
}

// KeyCatalog is every action and Ghostty's default bindings.
type KeyCatalog struct {
	Actions  []ActionDoc
	Defaults []Binding
	// Live is whether both came from the installed binary; otherwise they
	// are the copy built in. DefaultsKnown is false when no copy fits this
	// platform: the defaults differ between macOS and Linux.
	Live          bool
	DefaultsKnown bool
}

// Action finds an action's documentation by name, parameter ignored.
func (c KeyCatalog) Action(name string) (ActionDoc, bool) {
	name = ActionName(name)
	for _, a := range c.Actions {
		if a.Name == name {
			return a, true
		}
	}
	return ActionDoc{}, false
}

// DefaultsFor lists Ghostty's own triggers for an action, parameter
// included.
func (c KeyCatalog) DefaultsFor(action string) []Trigger {
	var out []Trigger
	for _, d := range c.Defaults {
		if d.Action == action {
			out = append(out, d.Trigger)
		}
	}
	return out
}

// BuiltinKeyCatalog is the copy kept in the binary. The defaults are those
// of macOS, so withDefaults should only be true there.
func BuiltinKeyCatalog(withDefaults bool) KeyCatalog {
	c := KeyCatalog{Actions: ParseActionDocs(builtinActionDocs)}
	if withDefaults {
		c.Defaults = ParseKeybindList(builtinDefaultsMac)
		c.DefaultsKnown = true
	}
	return c
}

// keyCommandTimeout bounds each call to the binary: it answers in a few
// milliseconds, and a stuck one must not leave the list empty forever.
var keyCommandTimeout = 5 * time.Second

// LoadKeyCatalog asks the binary for its actions and default bindings. It
// runs two processes, so the interface calls it in the background. When the
// binary is missing or fails, the copy built in stands in.
func LoadKeyCatalog(binary string) KeyCatalog {
	fallback := BuiltinKeyCatalog(runtime.GOOS == "darwin")
	if binary == "" {
		return fallback
	}
	run := func(args ...string) (string, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), keyCommandTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, binary, args...).Output()
		return string(out), err == nil
	}
	docs, ok1 := run("+list-actions", "--docs")
	defaults, ok2 := run("+list-keybinds", "--default", "--plain")
	actions := ParseActionDocs(docs)
	if !ok1 || !ok2 || len(actions) == 0 {
		return fallback
	}
	return KeyCatalog{Actions: actions, Defaults: ParseKeybindList(defaults), Live: true, DefaultsKnown: true}
}

// ParseActionDocs reads `+list-actions --docs`: an action name alone on a
// line ending in a colon, its documentation indented under it.
func ParseActionDocs(text string) []ActionDoc {
	var out []ActionDoc
	var doc []string
	flush := func() {
		if len(out) == 0 {
			return
		}
		body := strings.TrimSpace(strings.Join(doc, "\n"))
		out[len(out)-1].Doc = body
		para, _, _ := strings.Cut(body, "\n\n")
		out[len(out)-1].Summary = strings.Join(strings.Fields(para), " ")
		doc = nil
	}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := sc.Text()
		if name, ok := strings.CutSuffix(line, ":"); ok && name != "" && !strings.HasPrefix(line, " ") && !strings.ContainsAny(name, " \t") {
			flush()
			out = append(out, ActionDoc{Name: name})
			continue
		}
		if len(out) > 0 {
			doc = append(doc, strings.TrimPrefix(strings.TrimRight(line, " "), "  "))
		}
	}
	flush()
	// Without --docs the binary prints bare names, one per line.
	if len(out) == 0 {
		for _, line := range strings.Split(text, "\n") {
			if name := strings.TrimSpace(line); name != "" && !strings.Contains(name, " ") {
				out = append(out, ActionDoc{Name: name})
			}
		}
	}
	return out
}

// ParseKeybindList reads `+list-keybinds --plain`: one `keybind = value`
// per line. Ghostty 1.4 prints each value as a config file holds it, but 1.3
// doubled every backslash, so its `text:\\x05` reads as `text:\x05`.
func ParseKeybindList(text string) []Binding {
	escaped := backslashesDoubled(text)
	var out []Binding
	for _, line := range strings.Split(text, "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "keybind")
		if !ok {
			continue
		}
		value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "="))
		if escaped {
			value = strings.ReplaceAll(value, `\\`, `\`)
		}
		if b, err := ParseBinding(value); err == nil && !b.Clear && !b.Chain && b.Table == "" {
			out = append(out, b)
		}
	}
	return out
}

// backslashesDoubled tells a 1.3 listing from a 1.4 one: in 1.3 every run of
// backslashes is even. The macOS defaults hold `text:\x05`, so a 1.4
// listing of them always has an odd run; the Linux ones hold no backslash.
func backslashesDoubled(text string) bool {
	seen, run := false, 0
	for i := 0; i <= len(text); i++ {
		if i < len(text) && text[i] == '\\' {
			seen = true
			run++
			continue
		}
		if run%2 == 1 {
			return false
		}
		run = 0
	}
	return seen
}
