package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// explainPage is a page as Ghostty 1.4 prints it, long enough to scroll.
const explainPage = "Whether to automatically copy selected text to the clipboard.\n\nValid values:\n\n* `none` - Do not copy selected text automatically.\n\n* `primary` - On Linux, copy to the selection clipboard only. This has no\n  effect on macOS. (Available since: 1.4.0)\n\n* `clipboard` - Copy text to the system clipboard only.\n\nFor backward compatibility and convenience, a value of `true` is the same as\n`primary` on Linux and `clipboard` on macOS, and `false` is an alias for\n`none`.\n\n```ini\ncopy-on-select = clipboard\n```\n\nThe default value is `primary` on Linux and `none` otherwise."

// stubExplain answers every page with text, or with err, and keeps what was
// asked.
func stubExplain(t *testing.T, text string, err error) *[]string {
	t.Helper()
	var asked []string
	old := explainDoc
	explainDoc = func(_, name string, action bool) (string, error) {
		asked = append(asked, fmt.Sprintf("%s %v", name, action))
		return text, err
	}
	t.Cleanup(func() { explainDoc = old })
	return &asked
}

func f3() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyF3} }

func TestF3ReadsGhosttysPageOnTheHighlightedSetting(t *testing.T) {
	asked := stubExplain(t, strings.Repeat(explainPage+"\n\n", 4), nil)
	h := newHarness(t, "")
	h.send(tea.WindowSizeMsg{Width: 100, Height: 30})
	h.m.group = 2
	h.m.recompute()
	h.highlightCommand("opacity")
	key := h.m.selected().cmd.set.key
	if key == "" {
		t.Fatal("opacity should write an option")
	}
	if !strings.Contains(h.view(), "F3 docs") {
		t.Error("the keys line should offer F3 on a setting")
	}
	h.send(f3())
	if h.m.overlay != overlayExplain || len(*asked) != 1 || (*asked)[0] != key+" false" {
		t.Fatalf("F3 should ask for %s's page: overlay %d, asked %v", key, h.m.overlay, *asked)
	}
	v := h.view()
	for _, want := range []string{key, "Whether to automatically copy", "• none - Do not copy", "↑↓ scroll", "Esc closes"} {
		if !strings.Contains(v, want) {
			t.Errorf("the page should show %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "`") {
		t.Error("backticks should be dropped")
	}
	h.dump("explain-1-cells")
	h.key("down", "pgdown")
	if h.m.ex.scroll != 1+h.m.ex.page-1 {
		t.Errorf("down then a page: scroll %d, page %d", h.m.ex.scroll, h.m.ex.page)
	}
	h.key("end")
	h.view()
	if h.m.ex.scroll != h.m.ex.maxScroll || h.m.ex.maxScroll == 0 {
		t.Errorf("End should reach the bottom: %d of %d", h.m.ex.scroll, h.m.ex.maxScroll)
	}
	h.key("down")
	if h.m.ex.scroll != h.m.ex.maxScroll {
		t.Error("the page should stop at its end")
	}
	h.key("esc")
	if h.m.overlay != overlayNone {
		t.Fatal("Esc should close the page")
	}
	// The help says so.
	h.send(keyPress("f1"))
	if !strings.Contains(h.view(), "Ghostty's docs for the line") {
		t.Error("the help should list F3")
	}
}

func TestF3OnAShortcutReadsItsAction(t *testing.T) {
	stubKeys(t)
	asked := stubExplain(t, "Go to the tab with the specific index, starting from 1.", nil)
	h := newHarness(t, "")
	h.openKeys("")
	h.row("goto_tab:1")
	h.send(f3())
	if len(*asked) != 1 || (*asked)[0] != "goto_tab:1 true" {
		t.Fatalf("F3 should ask for the action: %v", *asked)
	}
	if !strings.Contains(h.view(), "Go to the tab with the specific index") {
		t.Errorf("the page should show:\n%s", h.view())
	}
}

func TestF3OnAnOlderGhosttySaysWhatItNeeds(t *testing.T) {
	stubExplain(t, "", ghostty.ErrExplainUnsupported)
	h := newHarness(t, "")
	h.m.group = 2
	h.m.recompute()
	h.highlightCommand("opacity")
	h.send(f3())
	if v := h.view(); !strings.Contains(v, "needs Ghostty 1.4") {
		t.Errorf("an older Ghostty should be named:\n%s", v)
	}
	// The tool's own commands have no page: F3 says so and opens nothing.
	h.key("esc")
	h.m.group = len(groups) - 1
	h.m.recompute()
	h.highlightCommand("backup")
	h.send(f3())
	if h.m.overlay != overlayNone {
		t.Error("a command of the tool has no page to open")
	}
}

func TestExplainInThePictureInterface(t *testing.T) {
	stubExplain(t, explainPage, nil)
	h := newPixHarness(t, "")
	h.m.group = 2
	h.m.recompute()
	h.highlightCommand("opacity")
	h.send(f3())
	if h.m.overlay != overlayExplain {
		t.Fatal("F3 should open the page")
	}
	h.shot("explain")
	h.dump("explain-2-pixel")
}
