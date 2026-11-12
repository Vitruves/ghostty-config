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

	h.typeText("program ")
	v := h.view()
	for _, want := range []string{"Shell", "Images in the terminal", "Clickable links", "starship", "yazi", "download"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the list of programs lacks %q:\n%s", want, v)
		}
	}
	h.typeText("starsh")
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
