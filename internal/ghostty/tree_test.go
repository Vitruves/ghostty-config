package ghostty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, files map[string]string) (Paths, string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := Paths{
		ConfigDir: filepath.Join(dir, "xdg"),
		Roots: []string{
			filepath.Join(dir, "xdg", "config"),
			filepath.Join(dir, "xdg", "config.ghostty"),
			filepath.Join(dir, "support", "config"),
			filepath.Join(dir, "support", "config.ghostty"),
		},
		ThemesDir: filepath.Join(dir, "xdg", "themes"),
		StateDir:  filepath.Join(dir, "state"),
	}
	return p, dir
}

func TestLoadOrderMatchesGhostty(t *testing.T) {
	p, _ := fixture(t, map[string]string{
		"xdg/config":             "font-size = 10\nconfig-file = inc/extra\nwindow-width = 1\n",
		"xdg/config.ghostty":     "font-size = 11\n",
		"xdg/inc/extra":          "font-size = 12\nwindow-padding-x = 33\n",
		"support/config.ghostty": "font-size = 14.5\ntheme = powershell-vivid\n",
	})
	tree, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	// Roots first (config, config.ghostty, support), then includes: the
	// include of the first file is loaded after every root, so it wins.
	if got := tree.Value("font-size", ""); got != "12" {
		t.Fatalf("font-size = %q, want 12 (include loads after all roots)", got)
	}
	if got := tree.Value("window-padding-x", ""); got != "33" {
		t.Fatalf("window-padding-x = %q", got)
	}
	if filepath.Base(tree.Primary.Path) != "config.ghostty" || !strings.Contains(tree.Primary.Path, "support") {
		t.Fatalf("primary should be the last existing root, got %s", tree.Primary.Path)
	}
	if len(tree.Docs) != 4 {
		t.Fatalf("expected 4 documents, got %d", len(tree.Docs))
	}
}

func TestSetRewritesTheWinningLineInPlace(t *testing.T) {
	p, dir := fixture(t, map[string]string{
		"xdg/config":             "# my config\ntheme = Old\nfont-size = 10\n",
		"support/config.ghostty": "theme = Newer\n",
	})
	tree, _ := Load(p)
	changed := tree.Set("theme", "Catppuccin Mocha")
	if filepath.Base(filepath.Dir(changed.Path)) != "support" {
		t.Fatalf("theme should be rewritten where it wins, got %s", changed.Path)
	}
	// A key nobody sets goes to the primary.
	tree.Set("window-padding-y", "16")
	if _, err := tree.Save(); err != nil {
		t.Fatal(err)
	}
	xdg, _ := os.ReadFile(filepath.Join(dir, "xdg", "config"))
	if string(xdg) != "# my config\ntheme = Old\nfont-size = 10\n" {
		t.Fatalf("the shadowed file must not change, got %q", xdg)
	}
	support, _ := os.ReadFile(filepath.Join(dir, "support", "config.ghostty"))
	if string(support) != "theme = Catppuccin Mocha\n\nwindow-padding-y = 16\n" {
		t.Fatalf("got %q", support)
	}
	// A backup of the original exists.
	backups, _ := os.ReadDir(filepath.Join(dir, "state", "backups"))
	if len(backups) != 1 {
		t.Fatalf("expected one backup, got %d", len(backups))
	}
}

func TestNoConfigCreatesTheXDGOne(t *testing.T) {
	p, dir := fixture(t, map[string]string{})
	tree, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	tree.Set("theme", "Dracula")
	if _, err := tree.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "xdg", "config"))
	if err != nil || string(data) != "theme = Dracula\n" {
		t.Fatalf("got %q, %v", data, err)
	}
}

func TestPrimaryFontKeepsFallbacks(t *testing.T) {
	p, _ := fixture(t, map[string]string{
		"xdg/config": "font-family = \"\"\nfont-family = Menlo\nfont-family = Symbols Nerd Font\n",
	})
	tree, _ := Load(p)
	if got := tree.FontFamilies(); strings.Join(got, "|") != "Menlo|Symbols Nerd Font" {
		t.Fatalf("families = %v", got)
	}
	tree.SetPrimaryFont("Hack")
	if got := tree.FontFamilies(); strings.Join(got, "|") != "Hack|Symbols Nerd Font" {
		t.Fatalf("families after = %v", got)
	}
	if got := tree.Primary.Text(); got != "font-family = \"\"\nfont-family = Hack\nfont-family = Symbols Nerd Font\n" {
		t.Fatalf("got %q", got)
	}
}

func TestColourOverridesAreFoundAndDisabled(t *testing.T) {
	p, _ := fixture(t, map[string]string{
		"xdg/config": "theme = Dracula\nbackground = #ff0000\npalette = 0=#000000\nbold-color = #fff\n",
	})
	tree, _ := Load(p)
	if got := len(tree.ColourOverrides()); got != 2 {
		t.Fatalf("expected 2 overrides, got %d", got)
	}
	tree.DisableColourOverrides()
	if got := len(tree.ColourOverrides()); got != 0 {
		t.Fatalf("overrides remain: %d", got)
	}
	text := tree.Primary.Text()
	if !strings.Contains(text, "#background = #ff0000  # disabled by ghostty-config") || !strings.Contains(text, "bold-color = #fff\n") {
		t.Fatalf("got %q", text)
	}
	if _, err := tree.Save(); err != nil {
		t.Fatalf("save must accept commented-out lines: %v", err)
	}
}

func TestLigatures(t *testing.T) {
	p, _ := fixture(t, map[string]string{"xdg/config": "font-feature = -calt\n"})
	tree, _ := Load(p)
	if !tree.LigaturesDisabled() {
		t.Fatal("should read as disabled")
	}
	tree.SetLigatures(true)
	if tree.LigaturesDisabled() {
		t.Fatal("should be enabled now")
	}
	tree.SetLigatures(false)
	if !tree.LigaturesDisabled() {
		t.Fatal("should be disabled again")
	}
}

func TestPaletteValueForms(t *testing.T) {
	for in, want := range map[string]int{"0=#fff": 0, "0x1=#fff": 1, "0o7=#fff": 7, "0b10=#fff": 2, "255=#fff": 255} {
		idx, _, ok := parsePaletteValue(in)
		if !ok || idx != want {
			t.Errorf("%q → %d %v, want %d", in, idx, ok, want)
		}
	}
	if _, _, ok := parsePaletteValue("256=#fff"); ok {
		t.Error("256 must be rejected")
	}
}
