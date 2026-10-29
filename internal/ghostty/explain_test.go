package ghostty

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// scriptGhostty writes a script that stands in for the binary.
func scriptGhostty(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a shell")
	}
	p := filepath.Join(t.TempDir(), "ghostty")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExplain(t *testing.T) {
	bin := scriptGhostty(t, `
count="$(dirname "$0")/count"
echo x >> "$count"
case "$3" in
--option=copy-on-select) printf 'Copy the selection.\n\n* `+"`none`"+` - nothing\n' ;;
--keybind=goto_tab) echo 'Go to a tab.' ;;
*) echo "Unknown: '${3#*=}'."; exit 1 ;;
esac`)
	got, err := Explain(bin, "copy-on-select", false)
	if err != nil || !strings.HasPrefix(got, "Copy the selection.") {
		t.Fatalf("option: %q, %v", got, err)
	}
	if got, err := Explain(bin, "goto_tab:3", true); err != nil || got != "Go to a tab." {
		t.Fatalf("action with a parameter: %q, %v", got, err)
	}
	if _, err := Explain(bin, "no-such-key", false); err == nil || errors.Is(err, ErrExplainUnsupported) || !strings.Contains(err.Error(), "no-such-key") {
		t.Fatalf("unknown name: %v", err)
	}
	// Asked again, the answers come from the cache.
	Explain(bin, "copy-on-select", false)
	Explain(bin, "no-such-key", false)
	calls, _ := os.ReadFile(filepath.Join(filepath.Dir(bin), "count"))
	if n := strings.Count(string(calls), "x"); n != 3 {
		t.Errorf("the binary ran %d times, want 3", n)
	}
}

func TestExplainOldGhostty(t *testing.T) {
	bin := scriptGhostty(t, `echo "Error: an invalid action or multiple actions were specified." >&2; exit 1`)
	if _, err := Explain(bin, "font-size", false); !errors.Is(err, ErrExplainUnsupported) {
		t.Fatalf("old Ghostty: %v", err)
	}
	if !strings.Contains(ErrExplainUnsupported.Error(), "1.4") {
		t.Errorf("the error should name the version: %v", ErrExplainUnsupported)
	}
	if _, err := Explain("", "font-size", false); !errors.Is(err, ErrNoBinary) {
		t.Fatalf("no binary: %v", err)
	}
}

func TestParseDoc(t *testing.T) {
	doc := strings.Join([]string{
		"Some additional notes for triggers:",
		"",
		"  * modifiers cannot repeat, `ctrl+ctrl+a` is invalid.",
		"",
		"  * modifiers and keys can be in any order, `shift+a+ctrl` is *weird*,",
		"    but valid.",
		"  * a third, right under.",
		"",
		"    An explanation under the item.",
		"",
		"## Key Tables",
		"",
		"```ini",
		"keybind = ctrl+a=new_window",
		"",
		"keybind = chain=goto_split:left",
		"````",
		"Examples:",
		"",
		"  ghostty +explain-config font-size",
		"  ghostty +explain-config copy_to_clipboard",
	}, "\n")
	got := ParseDoc(doc)
	want := []DocBlock{
		{Kind: DocPara, Text: "Some additional notes for triggers:"},
		{Kind: DocBullet, Indent: 2, Text: "modifiers cannot repeat, `ctrl+ctrl+a` is invalid.", Gap: true},
		{Kind: DocBullet, Indent: 2, Text: "modifiers and keys can be in any order, `shift+a+ctrl` is *weird*, but valid.", Gap: true},
		{Kind: DocBullet, Indent: 2, Text: "a third, right under."},
		{Kind: DocPara, Indent: 4, Text: "An explanation under the item.", Gap: true},
		{Kind: DocHeading, Text: "Key Tables", Gap: true},
		{Kind: DocCode, Lines: []string{"keybind = ctrl+a=new_window", "", "keybind = chain=goto_split:left"}, Gap: true},
		{Kind: DocPara, Text: "Examples:"},
		{Kind: DocCode, Indent: 2, Lines: []string{"ghostty +explain-config font-size", "ghostty +explain-config copy_to_clipboard"}, Gap: true},
	}
	if !reflect.DeepEqual(got, want) {
		for i := range max(len(got), len(want)) {
			var g, w DocBlock
			if i < len(got) {
				g = got[i]
			}
			if i < len(want) {
				w = want[i]
			}
			if !reflect.DeepEqual(g, w) {
				t.Errorf("block %d:\n got %+v\nwant %+v", i, g, w)
			}
		}
	}
}

func TestDocSpans(t *testing.T) {
	cases := []struct {
		in   string
		want []DocSpan
	}{
		{"The format is `trigger=action`.", []DocSpan{{"The format is ", false}, {"trigger=action", true}, {".", false}}},
		{"Trigger: `+`-separated", []DocSpan{{"Trigger: ", false}, {"+", true}, {"-separated", false}}},
		{"is *weird*, but", []DocSpan{{"is weird, but", false}}},
		{"5 * 2 and *.txt", []DocSpan{{"5 * 2 and *.txt", false}}},
		{"see [fontdrop.info](https://fontdrop.info).", []DocSpan{{"see fontdrop.info (https://fontdrop.info).", false}}},
		{"an `unclosed tick", []DocSpan{{"an `unclosed tick", false}}},
	}
	for _, c := range cases {
		if got := DocSpans(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("DocSpans(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}
