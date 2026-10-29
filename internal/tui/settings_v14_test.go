package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// asGhostty13 stands a script in for a Ghostty 1.3 binary, which names its
// version and accepts any config, and rebuilds the menu for it.
func asGhostty13(t *testing.T, h *harness) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "ghostty")
	os.WriteFile(bin, []byte("#!/bin/sh\n[ \"$1\" = --version ] && echo 'Ghostty 1.3.1'\nexit 0\n"), 0o755)
	h.m.paths.Binary = bin
	h.m.cmds = nil
}

func (h *harness) has(name string) bool {
	for _, c := range h.m.commands() {
		if c.name == name {
			return true
		}
	}
	return false
}

func (h *harness) options(name string) []string {
	var out []string
	for _, o := range h.command(name).options(h.m, "") {
		out = append(out, o.value)
	}
	return out
}

// 1.4's keys are offered by platform; a binary that cannot name its version
// counts as 1.4.
func TestGhostty14SettingsFollowThePlatform(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	mac := runtime.GOOS == "darwin"
	for name, want := range map[string]bool{
		"middleclick": true, "osc8": true, "exittime": true, "vtresize": true, "cliplimit": true,
		"scrollbacklines": true, "compression": true, "draghandle": true, "quickscreen": true,
		"tabsinverted": !mac, "tabswipe": !mac,
	} {
		if h.has(name) != want {
			t.Errorf("%s offered %v on %s, want %v", name, h.has(name), runtime.GOOS, want)
		}
	}
	if s := h.command("scrollback").set; s.key != "scrollback-limit-bytes" || s.fallback != "50000000" {
		t.Errorf("scrollback on 1.4: %s, default %s", s.key, s.fallback)
	}
	if got := strings.Join(h.options("balance"), " "); got != "false true equal" {
		t.Errorf("balance on 1.4: %s", got)
	}
	want := "none primary clipboard both"
	if mac {
		want = "none clipboard"
	}
	if got := strings.Join(h.options("copy"), " "); got != want {
		t.Errorf("copy on 1.4: %s, want %s", got, want)
	}
}

// A 1.3 binary is offered only what its validator accepts.
func TestGhostty13KeepsItsOwnSettings(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	asGhostty13(t, h)
	for _, name := range []string{"middleclick", "osc8", "exittime", "vtresize", "cliplimit", "scrollbacklines", "compression", "draghandle", "tabsinverted", "tabswipe"} {
		if h.has(name) {
			t.Errorf("%s offered to Ghostty 1.3", name)
		}
	}
	if h.has("quickscreen") != (runtime.GOOS == "darwin") {
		t.Errorf("quickscreen offered on %s to Ghostty 1.3", runtime.GOOS)
	}
	if s := h.command("scrollback").set; s.key != "scrollback-limit" || s.fallback != "10000000" {
		t.Errorf("scrollback on 1.3: %s, default %s", s.key, s.fallback)
	}
	if got := strings.Join(h.options("balance"), " "); got != "false true" {
		t.Errorf("balance on 1.3: %s", got)
	}
	if got := strings.Join(h.options("copy"), " "); got != "true clipboard false" {
		t.Errorf("copy on 1.3: %s", got)
	}
	if h.value("copy") != "true" {
		t.Errorf("copy on 1.3 defaults to true, reads %s", h.value("copy"))
	}
}

// true and false, written for an older Ghostty, read as what 1.4 makes of
// them, and a preset written in the old words writes the new ones.
func TestCopyOnSelectReadsTheOldWords(t *testing.T) {
	mac := runtime.GOOS == "darwin"
	h := newHarness(t, "copy-on-select = true\n")
	want := map[bool]string{true: "clipboard", false: "primary"}[mac]
	if h.value("copy") != want {
		t.Fatalf("copy-on-select = true reads %s, want %s", h.value("copy"), want)
	}
	h = newHarness(t, "copy-on-select = false\n")
	if h.value("copy") != "none" {
		t.Fatalf("copy-on-select = false reads %s", h.value("copy"))
	}
	// The power preset asks for copy true: on macOS that is clipboard, on
	// Linux primary, the default, which leaves no line.
	h = newHarness(t, "theme = Dracula\n")
	for _, p := range presets {
		if p.name == "power" {
			h.m.applyPreset(p)
		}
	}
	if got := activeLines(h.config(), "copy-on-select"); mac && (len(got) != 1 || got[0] != "clipboard") || !mac && len(got) != 0 {
		t.Fatalf("power preset wrote copy-on-select %v", got)
	}
	if got := activeLines(h.config(), "window-padding-balance"); len(got) != 0 {
		t.Fatalf("power preset touched balance: %v", got)
	}
}

// A change to the scrollback writes 1.4's name for it and comments out the
// old one, which still gives the value until then.
func TestScrollbackMovesToTheNewKey(t *testing.T) {
	h := newHarness(t, "scrollback-limit = 1000000\n")
	if h.value("scrollback") != "1000000" {
		t.Fatalf("the old key is read: %s", h.value("scrollback"))
	}
	h.set("scrollback", "100000000")
	cfg := h.config()
	if got := activeLines(cfg, "scrollback-limit-bytes"); len(got) != 1 || got[0] != "100000000" {
		t.Fatalf("new key: %v\n%s", got, cfg)
	}
	if got := activeLines(cfg, "scrollback-limit"); len(got) != 0 {
		t.Fatalf("the old key is still in effect: %v\n%s", got, cfg)
	}
	// Back to the default leaves neither.
	h.set("scrollback", "50000000")
	if got := activeLines(h.config(), "scrollback-limit-bytes"); len(got) != 0 {
		t.Fatalf("default kept a line: %v", got)
	}
	if h.value("scrollback") != "50000000" {
		t.Fatalf("default reads %s", h.value("scrollback"))
	}
}
