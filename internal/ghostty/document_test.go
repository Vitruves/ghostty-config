package ghostty

import (
	"strings"
	"testing"
)

func TestParseTextFollowsGhosttyRules(t *testing.T) {
	doc := ParseText("# comment\n  # indented comment\nfont-family = \"Andale Mono\"\nfont-thicken\nwindow-padding-x=7 # not a comment\n\ntheme = Catppuccin Mocha  \n")
	want := []struct {
		kind       LineKind
		key, value string
	}{
		{LineComment, "", ""},
		{LineComment, "", ""},
		{LineEntry, "font-family", "Andale Mono"},
		{LineEntry, "font-thicken", ""},
		{LineEntry, "window-padding-x", "7 # not a comment"},
		{LineBlank, "", ""},
		{LineEntry, "theme", "Catppuccin Mocha"},
	}
	if len(doc.Lines) != len(want) {
		t.Fatalf("got %d lines, want %d", len(doc.Lines), len(want))
	}
	for i, w := range want {
		l := doc.Lines[i]
		if l.Kind != w.kind || l.Key != w.key || l.Value != w.value {
			t.Errorf("line %d: got %+v, want %+v", i, l, w)
		}
	}
}

func TestRoundTripIsByteExact(t *testing.T) {
	src := "# header\n\nfont-family=Menlo\n  theme = x   \nkeybind = super+shift+,=reload_config\n"
	doc := ParseText(src)
	if got := doc.Text(); got != src {
		t.Fatalf("round trip changed the file:\n%q\n%q", src, got)
	}
	noNewline := "a = 1"
	if got := ParseText(noNewline).Text(); got != noNewline {
		t.Fatalf("trailing newline was invented: %q", got)
	}
}

func TestSetLineKeepsSpacingStyle(t *testing.T) {
	doc := ParseText("font-family=Menlo\n  font-size = 12\n")
	doc.SetLine(0, "font-family", "Hack")
	doc.SetLine(1, "font-size", "14")
	want := "font-family=Hack\n  font-size = 14\n"
	if got := doc.Text(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAppendSeparatesParagraph(t *testing.T) {
	doc := ParseText("theme = x\n")
	doc.Append("font-size", "13")
	if got := doc.Text(); got != "theme = x\n\nfont-size = 13\n" {
		t.Fatalf("got %q", got)
	}
	empty := ParseText("")
	empty.Append("theme", "y")
	if got := empty.Text(); got != "theme = y\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCommentOutExplainsItself(t *testing.T) {
	doc := ParseText("background = #ff0000\n")
	doc.CommentOut(0, "why")
	if got := doc.Text(); !strings.HasPrefix(got, "#background = #ff0000  # why") {
		t.Fatalf("got %q", got)
	}
	if doc.Lines[0].Kind != LineComment {
		t.Fatal("line should now be a comment")
	}
}

func TestFormatValueQuotesOnlyWhenNeeded(t *testing.T) {
	cases := map[string]string{"Andale Mono": "Andale Mono", " x": "\" x\"", "\"q\"": "\"\"q\"\"", "": ""}
	for in, want := range cases {
		if got := FormatValue(in); got != want {
			t.Errorf("FormatValue(%q) = %q, want %q", in, got, want)
		}
	}
}
