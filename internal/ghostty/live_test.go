package ghostty

import (
	"slices"
	"strings"
	"testing"
)

// System Events types into whichever app is in front, whatever process the
// script names: the keystroke must only go out while that app is Ghostty.
func TestReloadKeystrokeOnlyGoesToGhosttyInFront(t *testing.T) {
	script := reloadScript("com.mitchellh.ghostty", ",", "command down, shift down")
	guard := strings.Index(script, "if frontmost of ghostty is false then return \""+notFrontmost+"\"")
	send := strings.Index(script, "tell ghostty to keystroke \",\" using {command down, shift down}")
	if guard < 0 || send < 0 || guard > send {
		t.Fatalf("the script should check that Ghostty is in front before typing:\n%s", script)
	}
}

// The AppleScript reload must never launch a Ghostty, nor fail on one with
// no terminal open, and it targets the app the tool runs in.
func TestPerformScriptGuardsBeforeActing(t *testing.T) {
	script := performScript("com.mitchellh.ghostty.debug")
	running := strings.Index(script, `if application id "com.mitchellh.ghostty.debug" is not running then return "`+noTerminal+`"`)
	tell := strings.Index(script, `tell application id "com.mitchellh.ghostty.debug"`)
	empty := strings.Index(script, `if (count of terminals) is 0 then return "`+noTerminal+`"`)
	perform := strings.Index(script, `perform action "reload_config" on terminal 1`)
	if running < 0 || tell < running || empty < tell || perform < empty {
		t.Fatalf("the script should check Ghostty runs and has a terminal before acting:\n%s", script)
	}
}

func TestMacBundleIDFollowsTheHostApp(t *testing.T) {
	t.Setenv("__CFBundleIdentifier", "com.mitchellh.ghostty.debug")
	if got := macBundleID(); got != "com.mitchellh.ghostty.debug" {
		t.Fatalf("a debug build should reload itself, got %q", got)
	}
	t.Setenv("__CFBundleIdentifier", "com.apple.Terminal")
	if got := macBundleID(); got != "com.mitchellh.ghostty" {
		t.Fatalf("another host should reach the release app, got %q", got)
	}
}

// GApplication exports its actions at a path made from the app id, the way
// Ghostty's own D-Bus client computes it.
func TestGTKObjectPath(t *testing.T) {
	for id, want := range map[string]string{
		"com.mitchellh.ghostty":       "/com/mitchellh/ghostty",
		"com.mitchellh.ghostty-debug": "/com/mitchellh/ghostty_debug",
		"org.example.my-term":         "/org/example/my_term",
	} {
		if got := gtkObjectPath(id); got != want {
			t.Errorf("%s → %s, want %s", id, got, want)
		}
	}
	if ids := gtkAppIDs(""); !slices.Equal(ids, []string{"com.mitchellh.ghostty", "com.mitchellh.ghostty-debug"}) {
		t.Errorf("without a binary the release and debug ids stand, got %v", ids)
	}
}
