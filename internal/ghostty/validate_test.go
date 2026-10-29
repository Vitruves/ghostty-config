package ghostty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// What Ghostty 1.3 prints for its default files, which it locates, then for
// a file named on its command line, which it does not, and for a theme it
// cannot find, once per directory tried.
const validateOutput = `/home/u/.config/ghostty/config:1:font-size: invalid value "abc"
/home/u/.config/ghostty/config:2:bogus-key: unknown field
window-padding-x: invalid value "zz"
theme "Nope" not found, tried path "/home/u/.config/ghostty/themes/Nope"
theme "Nope" not found, tried path "/usr/share/ghostty/themes/Nope"
something else entirely
`

func TestParseValidate(t *testing.T) {
	got := ParseValidate(validateOutput)
	want := []Problem{
		{Path: "/home/u/.config/ghostty/config", Line: 1, Key: "font-size", Message: `invalid value "abc"`},
		{Path: "/home/u/.config/ghostty/config", Line: 2, Key: "bogus-key", Message: "unknown field"},
		{Key: "window-padding-x", Message: `invalid value "zz"`},
		{Key: "theme", Message: `theme "Nope" not found`},
		{Message: "something else entirely"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d problems: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("problem %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if loc := got[0].Location(); loc != "config:1" {
		t.Fatalf("location = %q", loc)
	}
}

// fakeGhostty writes a script that answers +validate-config with output and
// exits with code, logging its arguments.
func fakeGhostty(t *testing.T, output string, code int) (bin, log string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "ghostty")
	log = filepath.Join(dir, "args")
	out := filepath.Join(dir, "out")
	os.WriteFile(out, []byte(output), 0o644)
	script := "#!/bin/sh\necho \"$@\" >> '" + log + "'\ncat '" + out + "'\nexit " + string(rune('0'+code)) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

func TestValidateAsksForTheFilesTheToolEdits(t *testing.T) {
	p, _ := fixture(t, map[string]string{"xdg/config": "font-size = 13\nwindow-padding-x = zz\n"})
	bin, log := fakeGhostty(t, "window-padding-x: invalid value \"zz\"\n", 1)
	p.Binary = bin

	// A config given on the command line is named to the validator.
	problems, err := Validate(p)
	if err != nil {
		t.Fatal(err)
	}
	if args := read(t, log); strings.TrimSpace(args) != "+validate-config --config-file="+p.Roots[0] {
		t.Fatalf("ran with %q", args)
	}
	tree, _ := Load(p)
	placed := tree.Place(problems)
	if len(placed) != 1 || placed[0].Path != p.Roots[0] || placed[0].Line != 2 {
		t.Fatalf("placed = %+v", placed)
	}
	if line, ok := tree.LineAt(placed[0]); !ok || line != "window-padding-x = zz" {
		t.Fatalf("line = %q", line)
	}

	// Ghostty's default files are what it reads by itself.
	os.Remove(log)
	p.Defaults = true
	if _, err := Validate(p); err != nil {
		t.Fatal(err)
	}
	if args := read(t, log); strings.TrimSpace(args) != "+validate-config" {
		t.Fatalf("ran with %q", args)
	}
}

func TestValidateReportsAFailureWithoutProblems(t *testing.T) {
	p, _ := fixture(t, map[string]string{"xdg/config": "font-size = 13\n"})
	bin, _ := fakeGhostty(t, "", 1)
	p.Binary = bin
	if _, err := Validate(p); err == nil {
		t.Fatal("a silent failure passed for a clean config")
	}
	clean, _ := fakeGhostty(t, "", 0)
	p.Binary = clean
	if problems, err := Validate(p); err != nil || len(problems) != 0 {
		t.Fatalf("clean config: %v, %v", problems, err)
	}
	p.Binary = ""
	if _, err := Validate(p); err != ErrNoBinary {
		t.Fatalf("no binary: %v", err)
	}
}

// The real validator, when it is installed, on a file of the test's own:
// never the user's configuration.
func TestValidateWithTheInstalledGhostty(t *testing.T) {
	bin := findBinary()
	if bin == "" {
		t.Skip("ghostty is not installed")
	}
	p, _ := fixture(t, map[string]string{"xdg/config": "font-size = abc\nbogus-key = 1\nfont-size = 13\n"})
	p.Binary = bin
	start := time.Now()
	problems, err := Validate(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ghostty +validate-config took %v", time.Since(start))
	tree, _ := Load(p)
	placed := tree.Place(problems)
	found := map[string]int{}
	for _, pr := range placed {
		found[pr.Key] = pr.Line
	}
	if found["font-size"] != 1 || found["bogus-key"] != 2 {
		t.Fatalf("problems = %+v", placed)
	}
}
