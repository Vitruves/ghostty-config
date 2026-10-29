package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// panel is the explanation beside the list, as plain text.
func (h *harness) panel() string {
	return ansi.Strip(strings.Join(h.m.detailStyled(h.m.panelChrome(), 80), "\n"))
}

// do types a line on a fresh prompt and, with enter, presses Enter.
func (h *harness) do(text string, enter bool) {
	h.t.Helper()
	h.m.setPromptText("")
	h.typeText(text)
	if enter {
		h.key("enter")
	}
}

// fakeValidator stands a script in for ghostty that answers +validate-config
// with output and exit code 1, or 0 when output is empty.
func fakeValidator(t *testing.T, h *harness, output string) {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	os.WriteFile(out, []byte(output), 0o644)
	code := "1"
	if output == "" {
		code = "0"
	}
	bin := filepath.Join(dir, "ghostty")
	os.WriteFile(bin, []byte("#!/bin/sh\ncat '"+out+"'\nexit "+code+"\n"), 0o755)
	h.m.paths.Binary = bin
	old := validateDebounce
	validateDebounce = time.Millisecond
	t.Cleanup(func() { validateDebounce = old })
}

// Each thing the user does is one step: a preset of many keys, a setting, a
// theme. Ctrl+Z takes them back one at a time, newest first, down to the
// file as it was.
func TestCtrlZUndoesOneActionAtATime(t *testing.T) {
	original := "theme = Dracula\n"
	h := newHarness(t, original)
	h.do("preset focus", true)
	h.do("cursor underline", true)
	h.do("theme Catppuccin Mocha", true)
	if !strings.Contains(h.config(), "theme = Catppuccin Mocha") || !strings.Contains(h.config(), "cursor-style = underline") {
		t.Fatalf("the actions were not written:\n%s", h.config())
	}

	h.key("ctrl+z")
	if cfg := h.config(); !strings.Contains(cfg, "theme = Dracula") || !strings.Contains(cfg, "cursor-style = underline") {
		t.Fatalf("the first Ctrl+Z should take back the theme alone:\n%s", cfg)
	}
	if h.m.applied != "Dracula" || !strings.Contains(h.m.status, "Undone: theme") {
		t.Fatalf("applied %q, status %q", h.m.applied, h.m.status)
	}
	h.key("ctrl+z")
	if cfg := h.config(); strings.Contains(cfg, "underline") || !strings.Contains(cfg, "cursor-style = bar") {
		t.Fatalf("the second should take back the cursor, back to the preset's bar:\n%s", cfg)
	}
	h.key("ctrl+z")
	if cfg := h.config(); cfg != original {
		t.Fatalf("the third should take back the whole preset:\n%q", cfg)
	}
	h.key("ctrl+z")
	if !strings.Contains(h.m.status, "Nothing to undo") {
		t.Fatalf("status %q", h.m.status)
	}
	// The undo command does the same as the key.
	h.do("size 16", true)
	h.do("undo", true)
	if cfg := h.config(); cfg != original {
		t.Fatalf("the undo command left:\n%q", cfg)
	}
}

// Stepping a setting with the arrows writes on every press, and is undone as
// the one change it is.
func TestAWalkWithTheArrowsIsOneStep(t *testing.T) {
	h := newHarness(t, "theme = Dracula\nwindow-padding-x = 10\n")
	h.selectSetting("Window", "paddingx")
	h.key("right", "right", "right")
	if !strings.Contains(h.config(), "window-padding-x = 16") {
		t.Fatalf("the arrows did not write:\n%s", h.config())
	}
	h.key("ctrl+z")
	if got := h.config(); got != "theme = Dracula\nwindow-padding-x = 10\n" {
		t.Fatalf("one Ctrl+Z should take back the whole walk:\n%q", got)
	}
}

// An edit made in another editor while the tool runs is never taken back,
// whether it came before or after the step being undone.
func TestUndoKeepsWhatWasEditedElsewhere(t *testing.T) {
	h := newHarness(t, "theme = Dracula\nfont-size = 13\n")
	h.do("size 15", true)
	path := filepath.Join(h.dir, harnessXDG, "config")
	os.WriteFile(path, []byte("# mine\ntheme = Dracula\nfont-size = 15\nwindow-padding-y = 7\n"), 0o644)
	h.do("cursor bar", true) // the save reads the edit in
	h.key("ctrl+z", "ctrl+z")
	if got := h.config(); got != "# mine\ntheme = Dracula\nfont-size = 13\nwindow-padding-y = 7\n" {
		t.Fatalf("after both undos:\n%q", got)
	}
	if h.m.fontSize != 13 {
		t.Fatalf("the editor still thinks the size is %v", h.m.fontSize)
	}
}

// The theme editor's own revert keeps working beside the session undo.
func TestRevertIsTheThemeEditorsUndo(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.do("edit red #ff0000", true)
	if !h.m.dirty {
		t.Fatal("the edit did not mark the theme")
	}
	h.do("revert", true)
	if h.m.dirty || h.m.cur.Colors["palette.1"] == "#ff0000" {
		t.Fatal("revert did not drop the edit")
	}
	if h.config() != "theme = Dracula\n" {
		t.Fatalf("revert wrote the config:\n%s", h.config())
	}
}

func TestChangesShowsTheSessionsLines(t *testing.T) {
	h := newHarness(t, "theme = Dracula\nfont-size = 13\n")
	h.do("changes", false)
	if h.m.selected() != nil || !strings.Contains(h.view(), "Nothing has changed") {
		t.Fatalf("a fresh session lists changes:\n%s", h.view())
	}
	h.do("size 15", true)
	h.do("changes", false)
	o := h.m.selected()
	if o == nil || !strings.HasSuffix(o.value, "config") || o.detail != "+1 −1" {
		t.Fatalf("changes lists %+v", o)
	}
	panel := h.panel()
	for _, want := range []string{"- font-size = 13", "+ font-size = 15", "  theme = Dracula"} {
		if !strings.Contains(panel, want) {
			t.Fatalf("the diff lacks %q:\n%s", want, panel)
		}
	}
}

func TestDiffFoldsUnchangedRuns(t *testing.T) {
	h := newHarness(t, "")
	var old []string
	for i := 0; i < 20; i++ {
		old = append(old, "line "+itoa(i))
	}
	new := append([]string{}, old...)
	new[10] = "changed"
	got := ansi.Strip(strings.Join(h.m.diffLines(ghostty.Diff(old, new), 60), "\n"))
	for _, want := range []string{"··· 8 unchanged lines", "  line 8", "- line 10", "+ changed", "  line 12", "··· 7 unchanged lines"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the diff lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "line 7\n") {
		t.Fatalf("too much context:\n%s", got)
	}
}

// A backup goes back over its file after a question; the file as it was is
// backed up first, and the session starts over from the restored file.
func TestRestoreRoundTrip(t *testing.T) {
	original := "theme = Dracula\nfont-size = 13\n"
	h := newHarness(t, original)
	h.do("size 15", true) // backs the original up before writing
	h.do("restore", false)
	o := h.m.selected()
	if o == nil || !strings.HasPrefix(o.label, "Today ") || o.detail != "+1 −1" {
		t.Fatalf("restore lists %+v:\n%s", o, h.view())
	}
	panel := h.panel()
	if !strings.Contains(panel, "- font-size = 15") || !strings.Contains(panel, "+ font-size = 13") || !strings.Contains(panel, "over ") {
		t.Fatalf("the panel does not show what restoring changes:\n%s", panel)
	}
	h.key("enter")
	if h.m.overlay != overlayConfirm {
		t.Fatal("Enter restored without asking")
	}
	h.key("n")
	if h.config() == original {
		t.Fatal("no restored the backup")
	}
	h.key("enter", "y")
	if got := h.config(); got != original {
		t.Fatalf("restored:\n%q", got)
	}
	if h.m.fontSize != 13 || h.m.tree.UndoSteps() != 0 {
		t.Fatalf("after the restore: size %v, %d steps", h.m.fontSize, h.m.tree.UndoSteps())
	}
	backups, _ := ghostty.ListBackups(h.paths)
	if len(backups) != 2 {
		t.Fatalf("%d backups, want the original and the file it replaced", len(backups))
	}
	if data, _ := os.ReadFile(backups[0].Path); !strings.Contains(string(data), "font-size = 15") {
		t.Fatalf("the newest backup holds %q", data)
	}
	// The list now leads with that copy, which restoring would undo the
	// restore with.
	if o := h.m.selected(); o == nil {
		t.Fatal("the list emptied")
	}
}

// Ghostty is asked after a write. A line it rejects that this session wrote
// is said to be so, with the way out.
func TestAProblemAfterAWriteIsTold(t *testing.T) {
	h := newHarness(t, "theme = Dracula\nfont-size = 13\n")
	fakeValidator(t, h, "font-size: invalid value \"15\"\n")
	h.do("size 15", true)
	if !h.m.checked || len(h.m.problems) != 1 {
		t.Fatalf("no check followed the write: %+v", h.m.problems)
	}
	p := h.m.problems[0]
	if p.Line != 2 || p.Key != "font-size" {
		t.Fatalf("problem placed at %+v", p)
	}
	if h.m.statusKind != statusWarn || !strings.Contains(h.m.status, "config:2 font-size: invalid value") || !strings.Contains(h.m.status, "a line this session wrote, Ctrl+Z takes it back") {
		t.Fatalf("status %q", h.m.status)
	}

	// The check command lists it, with the line in its file.
	h.do("check", false)
	if o := h.m.selected(); o == nil || o.label != "config:2  font-size" {
		t.Fatalf("check lists %+v:\n%s", o, h.view())
	}
	panel := h.panel()
	if !strings.Contains(panel, "2  font-size = 15") || !strings.Contains(panel, "This session wrote this line") {
		t.Fatalf("check panel:\n%s", panel)
	}

	// A problem that was there before is not told again on every write.
	fakeValidator(t, h, "bogus-key: unknown field\n")
	h.m.problems = []ghostty.Problem{{Path: filepath.Join(h.dir, harnessXDG, "config"), Line: 1, Key: "bogus-key", Message: "unknown field"}}
	os.WriteFile(filepath.Join(h.dir, harnessXDG, "config"), []byte("bogus-key = 1\ntheme = Dracula\nfont-size = 15\n"), 0o644)
	h.do("cursor bar", true)
	if h.m.statusKind == statusWarn {
		t.Fatalf("an old problem was told again: %q", h.m.status)
	}

	// No problems: the list says so.
	fakeValidator(t, h, "")
	h.do("check", false)
	if !strings.Contains(h.view(), "No problems") || h.m.currentValueOf(h.m.cmd) != "no problems" {
		t.Fatalf("a clean check:\n%s", h.view())
	}
}

// Only the newest question is answered: a check that comes back after
// another was asked describes files that changed since, and is dropped. A
// burst of writes asks once.
func TestStaleChecksAreDropped(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	fakeValidator(t, h, "font-size: invalid value \"x\"\n")
	first := h.m.runCheck(checkAsked)
	second := h.m.runCheck(checkAsked)
	late := first().(validateDoneMsg)
	h.m.Update(second())
	if !h.m.checked {
		t.Fatal("the newest answer was dropped")
	}
	h.m.problems, h.m.checked = nil, false
	h.m.Update(late)
	if h.m.checked {
		t.Fatal("a stale answer was taken")
	}

	// Two writes in a row: the first one's timer finds itself overtaken.
	a := h.m.scheduleValidate()
	b := h.m.scheduleValidate()
	if _, cmd := h.m.Update(a()); cmd != nil {
		t.Fatal("the overtaken timer still asked")
	}
	if _, cmd := h.m.Update(b()); cmd == nil {
		t.Fatal("the newest timer did not ask")
	}
}

// Without a ghostty program nothing is checked, and nothing is said.
func TestNoGhosttyNoCheck(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.m.paths.Binary = ""
	if h.m.scheduleValidate() != nil || h.m.runCheck(checkWrite) != nil {
		t.Fatal("a check was started with nothing to run")
	}
	h.do("size 15", true)
	if h.m.statusKind == statusWarn || h.m.checked {
		t.Fatalf("status %q", h.m.status)
	}
}

func TestWhenWord(t *testing.T) {
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.Local)
	for _, c := range []struct {
		at   time.Time
		want string
	}{
		{time.Date(2026, 10, 20, 9, 5, 0, 0, time.Local), "Today 09:05"},
		{time.Date(2026, 10, 19, 23, 59, 0, 0, time.Local), "Yesterday 23:59"},
		{time.Date(2026, 10, 16, 8, 0, 0, 0, time.Local), "Friday 08:00"},
		{time.Date(2026, 3, 2, 8, 0, 0, 0, time.Local), "2 March 08:00"},
		{time.Date(2025, 3, 2, 8, 0, 0, 0, time.Local), "2 March 2025 08:00"},
	} {
		if got := whenWord(c.at, now, false); got != c.want {
			t.Errorf("whenWord(%v) = %q, want %q", c.at, got, c.want)
		}
	}
}
