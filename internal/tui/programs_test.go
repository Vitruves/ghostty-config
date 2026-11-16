package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vitruves/ghostty-config/internal/extensions"
)

// programHome gives the test a home of its own, zsh, no Homebrew and an
// empty PATH, and makes an install drop a stub in ~/.local/bin.
func programHome(t *testing.T, zshrc string) (home string, installs *[]string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", "")
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", t.TempDir())
	if zshrc != "" {
		os.WriteFile(filepath.Join(home, ".zshrc"), []byte(zshrc), 0o644)
	}
	oldBrew, oldInstall := extensions.Brew, installProgram
	extensions.Brew = func() string { return "" }
	var asked []string
	installProgram = func(_ context.Context, p extensions.Program, home string) (string, error) {
		asked = append(asked, p.Name)
		path := filepath.Join(extensions.LocalBin(home), p.Bin())
		os.MkdirAll(filepath.Dir(path), 0o755)
		return path, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755)
	}
	t.Cleanup(func() { extensions.Brew, installProgram = oldBrew, oldInstall })
	forgetPrograms()
	return home, &asked
}

// A program is installed only after a yes, its line goes in the block of
// .zshrc with the PATH line its folder needs, and switching it off gives the
// file back as it was; the first change backs the file up.
func TestAProgramIsInstalledAndStartedWithTheShell(t *testing.T) {
	const zshrc = "export EDITOR=vim\nalias g=git\n"
	home, asked := programHome(t, zshrc)
	h := newHarness(t, "theme = Dracula\n")
	rc := filepath.Join(home, ".zshrc")

	// One group at a time, under a row of tabs the arrows and a click turn.
	h.typeText("program ")
	v := h.view()
	for _, want := range []string{"Shell", "Images", "Links", "starship", "lig", "download", "←→ group"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the first group lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "yazi") {
		t.Fatalf("other groups wait behind their tab:\n%s", v)
	}
	h.key("right")
	if o := h.m.selected(); o == nil || o.value != "yazi" || !strings.Contains(h.view(), "kitten") {
		t.Fatalf("→ should open Images on its first program: %+v", o)
	}
	h.key("left", "left")
	if o := h.m.tabRow(); o == nil || o.tabs[o.tab] != "Sessions" {
		t.Fatalf("← from the first group wraps to the last: %+v", o)
	}
	if v := h.view(); !strings.Contains(v, "‹") || strings.Contains(v, "Shell   Images") {
		t.Fatalf("the row keeps the open tab in view and says tabs are left out:\n%s", v)
	}
	r, ok := h.find(hitTab, -1, "Editors")
	if !ok {
		t.Fatal("the tabs should be places to click")
	}
	h.click(r.x, r.y-h.m.top)
	if o := h.m.selected(); o == nil || o.value != "neovim" {
		t.Fatalf("a click on Editors opens it: %+v", o)
	}
	// Typing searches every group.
	h.typeText("yaz")
	if o := h.m.selected(); o == nil || o.value != "yazi" || h.m.tabRow() != nil {
		t.Fatalf("a search lists every group, without tabs: %+v", o)
	}
	h.m.setPromptText("")
	h.typeText("program starsh")
	if v := h.view(); !strings.Contains(v, "starship/starship") || !strings.Contains(v, "ISC") || !strings.Contains(v, `eval "$(starship init zsh)"`) {
		t.Fatalf("the panel should say where it comes from and what it adds:\n%s", v)
	}
	h.key("enter")
	if h.m.overlay != overlayConfirm || !strings.Contains(h.m.confirmText, "GitHub release") || !strings.Contains(h.m.confirmText, "SHA-256") {
		t.Fatalf("installing should ask first and say how: %v %q", h.m.overlay, h.m.confirmText)
	}
	h.key("n")
	if len(*asked) != 0 {
		t.Fatal("no means nothing is installed")
	}
	h.key("enter", "y")
	if strings.Join(*asked, " ") != "starship" {
		t.Fatalf("yes installs it: %v", *asked)
	}
	data, _ := os.ReadFile(rc)
	text := string(data)
	if !strings.HasPrefix(text, zshrc) || !strings.Contains(text, `eval "$(starship init zsh)"  # ghostty-config:starship`) || !strings.Contains(text, ".local/bin") {
		t.Fatalf("the block should start starship after putting ~/.local/bin on the PATH:\n%s", text)
	}
	backups, _ := os.ReadDir(filepath.Join(h.paths.StateDir, "backups"))
	if len(backups) == 0 || !strings.HasPrefix(backups[0].Name(), ".zshrc.") {
		t.Fatalf(".zshrc should be backed up before its first change: %v", backups)
	}

	h.key("esc")
	h.typeText("program starsh")
	if o := h.m.selected(); o == nil || o.detail != "on" || !o.current {
		t.Fatalf("starship should be listed as on: %+v", o)
	}
	h.key("enter", "y")
	if data, _ := os.ReadFile(rc); string(data) != zshrc {
		t.Fatalf("switching it off should give .zshrc back as it was:\n%q", data)
	}
	if _, err := os.Stat(filepath.Join(extensions.LocalBin(home), "starship")); err != nil {
		t.Fatal("switching it off keeps the program")
	}
}

// A program the user already starts with a line of their own is left alone,
// and a program with nothing to start just says how to run it.
func TestAUsersOwnLineIsLeftAlone(t *testing.T) {
	const zshrc = "eval \"$(zoxide init zsh)\"\n"
	home, _ := programHome(t, zshrc)
	for _, b := range []string{"zoxide", "yazi"} {
		os.MkdirAll(extensions.LocalBin(home), 0o755)
		os.WriteFile(filepath.Join(extensions.LocalBin(home), b), []byte("#!/bin/sh\n"), 0o755)
	}
	h := newHarness(t, "theme = Dracula\n")

	h.typeText("program zoxide")
	if o := h.m.selected(); o == nil || o.detail != "on" {
		t.Fatalf("zoxide started by the user's own line is on: %+v", o)
	}
	h.key("enter")
	if h.m.overlay != overlayNone || !strings.Contains(h.m.status, "line of your own") {
		t.Fatalf("Enter should say the line is the user's: %v %q", h.m.overlay, h.m.status)
	}
	if data, _ := os.ReadFile(filepath.Join(home, ".zshrc")); string(data) != zshrc {
		t.Fatalf(".zshrc should be untouched:\n%s", data)
	}
	h.key("esc")
	h.typeText("program yazi")
	if o := h.m.selected(); o == nil || o.detail != "installed" {
		t.Fatalf("yazi needs no line, so it is just installed: %+v", o)
	}
	h.key("enter")
	if !strings.Contains(h.m.status, "run yazi") {
		t.Fatalf("Enter on yazi says how to run it: %q", h.m.status)
	}
}

// Ctrl+X uninstalls a program after a yes: its line leaves the block and
// its files leave ~/.local/bin. lig's own uninstall runs first, while lig is
// still there to run it.
func TestCtrlXUninstallsAProgram(t *testing.T) {
	const zshrc = "eval \"$(lig init zsh pre)\"\nalias g=git\neval \"$(lig init zsh post)\"\n"
	home, _ := programHome(t, zshrc)
	h := newHarness(t, "theme = Dracula\n")
	rc := filepath.Join(home, ".zshrc")
	var setups []string
	oldSetup := extensions.SelfSetup
	extensions.SelfSetup = func(_ context.Context, path, verb, shell string) error {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s %s ran after its file was deleted", path, verb)
		}
		setups = append(setups, filepath.Base(path)+" "+verb+" "+shell)
		return nil
	}
	t.Cleanup(func() { extensions.SelfSetup = oldSetup })

	h.typeText("program starsh")
	h.key("enter", "y")
	if data, _ := os.ReadFile(rc); !strings.Contains(string(data), "ghostty-config:starship") {
		t.Fatalf("starship should be on:\n%s", data)
	}
	if v := h.view(); !strings.Contains(v, "^X") || !strings.Contains(v, "uninstall") {
		t.Fatalf("the footer and panel should offer Ctrl+X once it is installed:\n%s", v)
	}
	h.key("ctrl+x")
	if h.m.overlay != overlayConfirm || !strings.Contains(h.m.confirmText, "leaves ~/.zshrc") || !strings.Contains(h.m.confirmText, "starship is deleted from ~/.local/bin") {
		t.Fatalf("Ctrl+X should ask and say what goes: %q", h.m.confirmText)
	}
	h.key("n")
	if _, err := os.Stat(filepath.Join(extensions.LocalBin(home), "starship")); err != nil {
		t.Fatal("no keeps it")
	}
	h.key("ctrl+x", "y")
	if _, err := os.Stat(filepath.Join(extensions.LocalBin(home), "starship")); err == nil {
		t.Fatal("yes deletes it")
	}
	if data, _ := os.ReadFile(rc); string(data) != zshrc {
		t.Fatalf("its line should be gone and the rest kept:\n%q", data)
	}
	if o := h.m.selected(); o == nil || o.detail != "download" {
		t.Fatalf("starship should be listed as not installed: %+v", o)
	}

	// lig set itself up: its own uninstall takes its lines out first.
	os.WriteFile(filepath.Join(extensions.LocalBin(home), "lig"), []byte("#!/bin/sh\n"), 0o755)
	h.key("esc")
	h.typeText("program lig")
	h.key("ctrl+x")
	if !strings.Contains(h.m.confirmText, "lig uninstall zsh") {
		t.Fatalf("the question should name lig's own uninstall: %q", h.m.confirmText)
	}
	h.key("y")
	if strings.Join(setups, ",") != "lig uninstall zsh" {
		t.Fatalf("lig uninstall should have run once: %v", setups)
	}
	if _, err := os.Stat(filepath.Join(extensions.LocalBin(home), "lig")); err == nil {
		t.Fatal("lig should be deleted after its uninstall")
	}
}
