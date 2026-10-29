package ghostty

import (
	"os"
	"strings"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func save(t *testing.T, tree *Tree) {
	t.Helper()
	if _, err := tree.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestDiffKeepsTheLinesThatStayed(t *testing.T) {
	old := []string{"a", "b", "c", "d"}
	new := []string{"a", "x", "c", "d", "e"}
	var got []string
	for _, l := range Diff(old, new) {
		got = append(got, map[DiffKind]string{DiffSame: " ", DiffDel: "-", DiffAdd: "+"}[l.Kind]+l.Text)
	}
	if want := " a -b +x  c  d +e"; strings.Join(got, " ") != want {
		t.Fatalf("diff = %q, want %q", strings.Join(got, " "), want)
	}
	if a, r := DiffCounts(Diff(old, new)); a != 2 || r != 1 {
		t.Fatalf("counts = +%d -%d", a, r)
	}
}

func TestMergeCarriesAChangeOverEditsMadeElsewhere(t *testing.T) {
	base := []string{"a", "b", "font-size = 14"}
	// Undoing the appended line while the user appended one of their own
	// under it, and changed the first line.
	ours := []string{"A", "b", "font-size = 14", "user = 1"}
	theirs := []string{"a", "b"}
	got, clashes := merge3(base, ours, theirs)
	if strings.Join(got, "|") != "A|b|user = 1" || clashes != 0 {
		t.Fatalf("merge = %q with %d clashes", got, clashes)
	}
	// The same line changed on both sides: the text as it is now wins.
	got, clashes = merge3([]string{"x = 1"}, []string{"x = 3"}, []string{"x = 2"})
	if strings.Join(got, "|") != "x = 3" || clashes != 1 {
		t.Fatalf("clash merge = %q with %d clashes", got, clashes)
	}
	// Both sides making the same change is no clash.
	got, clashes = merge3([]string{"x = 1"}, []string{"x = 2"}, []string{"x = 2"})
	if strings.Join(got, "|") != "x = 2" || clashes != 0 {
		t.Fatalf("same change merge = %q with %d clashes", got, clashes)
	}
}

func TestUndoTakesBackOneStepAtATime(t *testing.T) {
	original := "# mine\nfont-size = 13\ntheme = Dracula\n"
	p, _ := fixture(t, map[string]string{"xdg/config": original})
	tree, _ := Load(p)

	tree.Checkpoint("")
	tree.Set("font-size", "14")
	tree.Set("window-padding-x", "8")
	save(t, tree)
	tree.Checkpoint("")
	tree.Set("theme", "Nord")
	save(t, tree)
	tree.Checkpoint("")
	if n := tree.UndoSteps(); n != 2 {
		t.Fatalf("%d steps, want 2", n)
	}

	done, ok := tree.Undo()
	if !ok || strings.Join(done.Keys, ",") != "theme" {
		t.Fatalf("undo = %+v, %v", done, ok)
	}
	save(t, tree)
	if got := read(t, p.Roots[0]); !strings.Contains(got, "theme = Dracula") || !strings.Contains(got, "font-size = 14") {
		t.Fatalf("after one undo:\n%s", got)
	}
	// The second undo takes out the appended line too, which the check
	// against lost settings must allow.
	if _, ok := tree.Undo(); !ok {
		t.Fatal("nothing left to undo")
	}
	save(t, tree)
	if got := read(t, p.Roots[0]); got != original {
		t.Fatalf("after two undos:\n%q\nwant\n%q", got, original)
	}
	if _, ok := tree.Undo(); ok {
		t.Fatal("undo past the start of the session")
	}
}

// The case that matters: a file edited by hand while the tool runs. Undo
// must take back the tool's own lines and nothing the user typed, before or
// after the save that read their edit in.
func TestUndoNeverRevertsAnEditMadeElsewhere(t *testing.T) {
	p, _ := fixture(t, map[string]string{"xdg/config": "font-size = 13\ntheme = Dracula\n"})
	tree, _ := Load(p)
	tree.Checkpoint("")
	tree.Set("font-size", "14")
	save(t, tree)

	// The user edits the file in their editor: a line of their own, and a
	// change to the theme.
	os.WriteFile(p.Roots[0], []byte("# by hand\nfont-size = 14\ntheme = Nord\nwindow-padding-x = 9\n"), 0o644)

	tree.Checkpoint("")
	tree.Set("background-opacity", "0.9")
	save(t, tree) // reads the edit in and replays the opacity onto it
	tree.Checkpoint("")

	if _, ok := tree.Undo(); !ok {
		t.Fatal("no step to undo")
	}
	save(t, tree)
	got := read(t, p.Roots[0])
	if strings.Contains(got, "background-opacity") {
		t.Fatalf("the opacity was not taken back:\n%s", got)
	}
	if _, ok := tree.Undo(); !ok {
		t.Fatal("the step from before the edit was lost")
	}
	save(t, tree)
	got = read(t, p.Roots[0])
	want := "# by hand\nfont-size = 13\ntheme = Nord\nwindow-padding-x = 9\n"
	if got != want {
		t.Fatalf("after both undos:\n%q\nwant\n%q", got, want)
	}
}

// An edit made elsewhere to the very line a step wrote wins: the undo
// leaves it and says so.
func TestUndoLeavesALineEditedElsewhereSince(t *testing.T) {
	p, _ := fixture(t, map[string]string{"xdg/config": "font-size = 13\n"})
	tree, _ := Load(p)
	tree.Checkpoint("")
	tree.Set("font-size", "14")
	save(t, tree)
	os.WriteFile(p.Roots[0], []byte("font-size = 16\n"), 0o644)
	save(t, tree) // nothing pending, nothing reread yet
	tree.Checkpoint("")
	done, ok := tree.Undo()
	if !ok {
		t.Fatal("no step")
	}
	save(t, tree)
	if got := read(t, p.Roots[0]); got != "font-size = 16\n" {
		t.Fatalf("the user's 16 was reverted:\n%s", got)
	}
	if done.Kept != 1 {
		t.Fatalf("kept = %d, want 1", done.Kept)
	}
}

// A step that stays open across a save that rereads an edited file, as a
// walk through a list of fonts does, is cut there: undoing it takes back
// both halves and keeps the edit.
func TestAStepOpenAcrossARereadKeepsTheEdit(t *testing.T) {
	p, _ := fixture(t, map[string]string{"xdg/config": "font-family = Menlo\n"})
	tree, _ := Load(p)
	tree.Checkpoint("move:font")
	tree.SetPrimaryFont("Hack")
	save(t, tree)
	os.WriteFile(p.Roots[0], []byte("font-family = Hack\nwindow-padding-y = 4\n"), 0o644)
	tree.Checkpoint("move:font") // the same walk: no new step
	tree.SetPrimaryFont("Iosevka")
	save(t, tree)
	tree.Checkpoint("")
	if n := tree.UndoSteps(); n != 1 {
		t.Fatalf("%d steps, want the walk as one", n)
	}
	tree.Undo()
	save(t, tree)
	if got := read(t, p.Roots[0]); got != "font-family = Menlo\nwindow-padding-y = 4\n" {
		t.Fatalf("after undoing the walk:\n%q", got)
	}
}

// An undo is replayed like any edit when a save finds the file changed
// before the undo reached it.
func TestAnUndoSurvivesAReread(t *testing.T) {
	p, _ := fixture(t, map[string]string{"xdg/config": "font-size = 13\n"})
	tree, _ := Load(p)
	tree.Checkpoint("")
	tree.Set("font-size", "14")
	save(t, tree)
	tree.Checkpoint("")
	tree.Undo()
	os.WriteFile(p.Roots[0], []byte("font-size = 14\ncursor-style = bar\n"), 0o644)
	save(t, tree)
	if got := read(t, p.Roots[0]); got != "font-size = 13\ncursor-style = bar\n" {
		t.Fatalf("after the reread:\n%q", got)
	}
}

func TestAStepThatTakesBackTheLastOneLeavesNothing(t *testing.T) {
	p, _ := fixture(t, map[string]string{"xdg/config": "font-size = 13\n"})
	tree, _ := Load(p)
	tree.Checkpoint("")
	tree.Set("font-size", "14")
	tree.Checkpoint("")
	tree.Set("font-size", "13")
	tree.Checkpoint("")
	if n := tree.UndoSteps(); n != 0 {
		t.Fatalf("%d steps, want 0", n)
	}
	// A checkpoint with nothing edited since leaves no step either.
	tree.Checkpoint("")
	tree.Checkpoint("")
	if n := tree.UndoSteps(); n != 0 {
		t.Fatalf("%d steps after empty checkpoints", n)
	}
}

func TestWroteTellsTheSessionsLinesFromTheUsers(t *testing.T) {
	p, _ := fixture(t, map[string]string{"xdg/config": "font-size = 13\n"})
	tree, _ := Load(p)
	tree.Checkpoint("")
	tree.Set("bogus-key", "1")
	if !tree.Wrote(p.Roots[0], "bogus-key = 1") {
		t.Fatal("the line just appended is not the session's")
	}
	if tree.Wrote(p.Roots[0], "font-size = 13") {
		t.Fatal("the user's line counted as the session's")
	}
	tree.Checkpoint("")
	save(t, tree)
	if !tree.Wrote(p.Roots[0], "bogus-key = 1") {
		t.Fatal("a sealed step's line is not the session's")
	}
	tree.ResetHistory()
	if tree.Wrote(p.Roots[0], "bogus-key = 1") {
		t.Fatal("the history survived a reset")
	}
}
