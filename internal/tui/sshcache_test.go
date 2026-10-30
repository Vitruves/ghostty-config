package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeSSHGhostty stands a script in for Ghostty 1.4 whose +ssh-cache keeps
// its cache in a file beside it, and logs every action it is asked for.
func fakeSSHGhostty(t *testing.T, h *harness, cache string) (dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a shell")
	}
	dir = t.TempDir()
	os.WriteFile(filepath.Join(dir, "cache"), []byte(cache), 0o644)
	bin := filepath.Join(dir, "ghostty")
	os.WriteFile(bin, []byte(`#!/bin/sh
dir="$(dirname "$0")"
case "$1" in
--version) echo 'Ghostty 1.4.0'; exit 0 ;;
+ssh-cache) ;;
*) exit 0 ;;
esac
[ -n "$2" ] && echo "$2" >> "$dir/log"
case "$2" in
"") cat "$dir/cache" ;;
--remove=*) grep -v "^${2#--remove=} " "$dir/cache" > "$dir/new"; mv "$dir/new" "$dir/cache" ;;
--prune=*) echo "Pruned cache entries: 1" ;;
--clear) : > "$dir/cache" ;;
esac
`), 0o755)
	h.m.paths.Binary = bin
	h.m.cmds = nil
	return dir
}

func TestSSHHostsListsAndForgets(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	dir := fakeSSHGhostty(t, h, "other.org:2222  2026-09-01T08:00:00Z (59 days ago)\nuser@example.com  2026-10-29T10:00:00Z (1 day ago)\n")
	h.do("sshhosts", false)
	var labels []string
	for _, o := range h.m.results {
		if !o.header {
			labels = append(labels, o.label)
		}
	}
	if got := strings.Join(labels, " | "); got != "other.org:2222 | user@example.com | Forget hosts older than 30 days | Forget every host" {
		t.Fatalf("the list: %s\n%s", got, h.view())
	}
	if o := h.m.selected(); o == nil || o.value != "host:other.org:2222" || o.detail != "59 days ago" {
		t.Fatalf("highlighted %+v", o)
	}
	if panel := h.panel(); !strings.Contains(panel, "--remove=other.org:2222") {
		t.Fatalf("the panel:\n%s", panel)
	}
	// One host goes at once: its next connection only installs the terminfo again.
	h.key("enter")
	if h.m.overlay == overlayConfirm {
		t.Fatal("forgetting one host asked")
	}
	if o := h.m.selected(); o == nil || o.value != "host:user@example.com" {
		t.Fatalf("after forgetting, highlighted %+v", o)
	}
	// Clearing asks; no keeps the cache.
	h.do("sshhosts forget every", false)
	h.key("enter")
	if h.m.overlay != overlayConfirm || !strings.Contains(h.m.confirmText, "1 host") {
		t.Fatalf("clearing did not ask: %q", h.m.confirmText)
	}
	h.key("n")
	h.do("sshhosts forget every", true)
	h.key("y")
	log, _ := os.ReadFile(filepath.Join(dir, "log"))
	if got := string(log); got != "--remove=other.org:2222\n--clear\n" {
		t.Fatalf("Ghostty was asked:\n%s", got)
	}
	if !strings.Contains(h.m.status, "Forgot every host") {
		t.Errorf("status: %s", h.m.status)
	}
}

// The cache command is 1.4's; Ghostty 1.3 keeps its own wording of the ssh
// features, and both read the clipboard keys, 1.4 naming OSC 5522 as well.
func TestSSHAndClipboardFollowTheVersion(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	fakeSSHGhostty(t, h, "")
	if !h.has("sshhosts") || !h.has("clipread") || !h.has("clipwrite") {
		t.Fatal("1.4 is missing sshhosts or the clipboard keys")
	}
	if d := h.command("sshterminfo").set.detail; !strings.Contains(d, "ghostty +ssh") {
		t.Errorf("ssh-terminfo on 1.4: %s", d)
	}
	if d := h.command("clipread").set.detail; !strings.Contains(d, "OSC 5522") {
		t.Errorf("clipboard-read on 1.4: %s", d)
	}
	if e := h.command("clipread").set.explain["ask"]; !strings.Contains(e, "preview") {
		t.Errorf("ask on 1.4: %s", e)
	}
	if got := strings.Join(h.options("clipread"), " "); got != "ask allow deny" {
		t.Errorf("clipboard-read takes %s", got)
	}
	h.do("sshhosts", false)
	if o := h.m.results; len(o) != 1 || !strings.HasPrefix(o[0].label, "No host") {
		t.Errorf("an empty cache lists %+v", o)
	}

	asGhostty13(t, h)
	if h.has("sshhosts") {
		t.Error("sshhosts offered to Ghostty 1.3")
	}
	if !h.has("clipread") || !h.has("clipwrite") {
		t.Fatal("the clipboard keys are 1.3's too")
	}
	if d := h.command("sshterminfo").set.detail; strings.Contains(d, "+ssh") {
		t.Errorf("ssh-terminfo on 1.3: %s", d)
	}
	if d := h.command("clipread").set.detail; strings.Contains(d, "5522") {
		t.Errorf("clipboard-read on 1.3: %s", d)
	}
	h.set("clipwrite", "deny")
	h.set("clipread", "ask")
	if got := activeLines(h.config(), "clipboard-write"); len(got) != 1 || got[0] != "deny" {
		t.Errorf("clipboard-write wrote %v", got)
	}
	if got := activeLines(h.config(), "clipboard-read"); len(got) != 0 {
		t.Errorf("the default clipboard-read left %v", got)
	}
}
