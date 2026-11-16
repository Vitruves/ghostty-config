package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ghosttyBinary is the Ghostty on this machine, or a skipped test.
func ghosttyBinary(t *testing.T) string {
	t.Helper()
	if p, err := exec.LookPath("ghostty"); err == nil {
		return p
	}
	if p := "/Applications/Ghostty.app/Contents/MacOS/ghostty"; runtime.GOOS == "darwin" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("no ghostty on this machine")
	return ""
}

// moreKeyed are the settings of moreSettings that stand for a key, for the
// platform the tests run on and Ghostty 1.4.
func moreKeyed(h *harness) []setting {
	var out []setting
	for _, s := range h.m.moreSettings(runtime.GOOS == "darwin", true) {
		if s.key != "" {
			out = append(out, s)
		}
	}
	return out
}

// Every fallback is what Ghostty itself defaults to: with Ghostty's own
// default written out in the config, each setting still reads as its fallback.
func TestMoreSettingsDefaultsAreGhosttys(t *testing.T) {
	bin := ghosttyBinary(t)
	out, err := exec.Command(bin, "+show-config", "--default").Output()
	if err != nil {
		t.Skipf("ghostty +show-config: %v", err)
	}
	defaults := map[string]string{}
	for _, l := range strings.Split(string(out), "\n") {
		if k, v, ok := strings.Cut(l, " ="); ok {
			if _, seen := defaults[k]; !seen {
				defaults[k] = strings.TrimSpace(v)
			}
		}
	}
	h := newHarness(t, "theme = Dracula\n")
	var config strings.Builder
	config.WriteString("theme = Dracula\n")
	for _, s := range moreKeyed(h) {
		def, known := defaults[s.key]
		if !known {
			t.Errorf("%s: Ghostty has no key %s", s.cmd, s.key)
			continue
		}
		if s.key == "selection-word-chars" {
			// Its default holds a tab and quotes, which +show-config does
			// not print the way the config would have to write them.
			continue
		}
		if def == "" {
			// Left to Ghostty: the menu has a word for that.
			if s.fallback != "default" && s.fallback != "none" {
				t.Errorf("%s: %s is unset by default, the fallback is %q", s.cmd, s.key, s.fallback)
			}
			continue
		}
		if !strings.Contains(config.String(), "\n"+s.key+" = ") {
			config.WriteString(s.key + " = " + def + "\n")
		}
	}
	h = newHarness(t, config.String())
	for _, s := range moreKeyed(h) {
		if defaults[s.key] == "" || s.key == "selection-word-chars" {
			continue
		}
		got := s.get(h.m)
		a, aok := parseNumber(got)
		b, bok := parseNumber(s.fallback)
		if got != s.fallback && !(s.numeric && aok && bok && a == b) {
			t.Errorf("%s: with %s = %s it reads %q, the fallback is %q", s.cmd, s.key, defaults[s.key], got, s.fallback)
		}
	}
}

// Every value the new settings offer is one Ghostty's validator takes.
func TestMoreSettingsValuesValidate(t *testing.T) {
	bin := ghosttyBinary(t)
	h := newHarness(t, "theme = Dracula\n")
	var lines []string
	for _, s := range moreKeyed(h) {
		var values []string
		switch {
		case s.numeric:
			values = []string{s.format(s.min), s.format(s.max)}
		case s.list != nil:
			for _, o := range s.list(h.m, "") {
				// A file or a folder of this machine proves nothing.
				if !o.header && !strings.HasPrefix(o.value, "/") {
					values = append(values, o.value)
				}
			}
		default:
			values = s.options
		}
		for _, v := range values {
			h.set(s.cmd, v)
			for _, got := range activeLines(h.config(), s.key) {
				lines = append(lines, s.key+" = "+got)
			}
		}
	}
	if len(lines) < 150 {
		t.Fatalf("only %d lines were written", len(lines))
	}
	file := filepath.Join(t.TempDir(), "config")
	os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	if out, err := exec.Command(bin, "+validate-config", "--config-file="+file).CombinedOutput(); err != nil {
		t.Fatalf("ghostty rejects a value: %v\n%s", err, out)
	}
}

// Every word a part of a section names is a command on macOS or on Linux.
func TestSubsectionWordsAreCommands(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	known := map[string]bool{}
	for _, c := range h.m.commands() {
		known[c.name] = true
	}
	for _, mac := range []bool{true, false} {
		for _, list := range [][]setting{h.m.moreSettings(mac, true), h.m.windowExtraSettings(mac), h.m.sessionExtraSettings(mac)} {
			for _, s := range list {
				known[s.cmd] = true
			}
		}
	}
	// What buildSettings itself only makes on the other platform.
	for _, w := range strings.Fields("titlebar shadow colorspace thicken thickness optionalt savestate stepresize decoration gtktitlebar tabbar tabsinverted tabswipe fixfont") {
		known[w] = true
	}
	for group, parts := range subsections {
		seen := map[string]bool{}
		for _, part := range parts {
			for _, w := range splitWords(part[1]) {
				if !known[w] {
					t.Errorf("%s → %s names %q, which no command answers to", group, part[0], w)
				}
				if seen[w] {
					t.Errorf("%s names %q twice", group, w)
				}
				seen[w] = true
			}
		}
	}
	// And every new setting has a place of its own.
	for _, mac := range []bool{true, false} {
		for _, s := range h.m.moreSettings(mac, true) {
			if _, ok := subIndex()[s.group][s.cmd]; !ok {
				t.Errorf("%s is in no part of %s", s.cmd, s.group)
			}
		}
	}
}

// The keys 1.4 added stay away from a Ghostty 1.3, whose validator rejects
// them; the rest are as old as it.
func TestMoreSettingsFollowTheVersion(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	for _, name := range []string{"checksum", "checksumfull", "progress", "rightclick", "command", "boldcolor"} {
		if !h.has(name) {
			t.Errorf("%s is not offered to Ghostty 1.4", name)
		}
	}
	asGhostty13(t, h)
	for _, name := range []string{"checksum", "checksumfull", "progress"} {
		if h.has(name) {
			t.Errorf("%s offered to Ghostty 1.3", name)
		}
	}
	for _, name := range []string{"rightclick", "command", "boldcolor", "kam"} {
		if !h.has(name) {
			t.Errorf("%s is not offered to Ghostty 1.3", name)
		}
	}
}

// A typed value is the first line of its list, checked before it can be
// chosen, and the default leaves no line.
func TestTypedSettings(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	bold := h.command("boldcolor")
	if opts := bold.options(h.m, "#ff"); len(opts) == 0 || !opts[0].header {
		t.Fatalf("half a colour can be chosen: %+v", opts)
	}
	opts := bold.options(h.m, "FF8800")
	if len(opts) == 0 || opts[0].value != "#ff8800" {
		t.Fatalf("a typed colour is not the first line: %+v", opts)
	}
	bold.run(h.m, "FF8800", &opts[0])
	if got := activeLines(h.config(), "bold-color"); len(got) != 1 || got[0] != "#ff8800" {
		t.Fatalf("bold-color: %v", got)
	}
	h.set("boldcolor", "bright")
	if h.value("boldcolor") != "bright" {
		t.Fatalf("bright reads %s", h.value("boldcolor"))
	}
	h.set("boldcolor", "default")
	if got := activeLines(h.config(), "bold-color"); len(got) != 0 {
		t.Fatalf("default kept a line: %v", got)
	}

	title := h.command("title")
	opts = title.options(h.m, "build box")
	if len(opts) == 0 || opts[0].value != "build box" {
		t.Fatalf("a typed title is not the first line: %+v", opts)
	}
	title.run(h.m, "build box", &opts[0])
	if got := activeLines(h.config(), "title"); len(got) != 1 || got[0] != "build box" {
		t.Fatalf("title: %v", got)
	}
	if opts := h.command("workdir").options(h.m, "/no/such/place"); len(opts) == 0 || !opts[0].header {
		t.Fatalf("a directory that is not there can be chosen: %+v", opts)
	}
}

// The wheel and the trackpad share one key: each keeps the other's speed,
// and a bare number is both.
func TestScrollSpeedsShareAKey(t *testing.T) {
	const key = "mouse-scroll-multiplier"
	h := newHarness(t, "theme = Dracula\n")
	if h.value("scrollwheel") != "3" || h.value("scrolltrackpad") != "1" {
		t.Fatalf("defaults read %s and %s", h.value("scrollwheel"), h.value("scrolltrackpad"))
	}
	h.set("scrollwheel", "5")
	h.set("scrolltrackpad", "0.5")
	if got := activeLines(h.config(), key); len(got) != 1 || got[0] != "precision:0.5,discrete:5" {
		t.Fatalf("both speeds: %v", got)
	}
	h.set("scrollwheel", "3")
	h.set("scrolltrackpad", "1")
	if got := activeLines(h.config(), key); len(got) != 0 {
		t.Fatalf("defaults kept a line: %v", got)
	}
	h = newHarness(t, key+" = 2\n")
	if h.value("scrollwheel") != "2" || h.value("scrolltrackpad") != "2" {
		t.Fatalf("a bare 2 reads %s and %s", h.value("scrollwheel"), h.value("scrolltrackpad"))
	}
}
