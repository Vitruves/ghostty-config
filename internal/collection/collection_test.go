package collection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

func TestCollectionSize(t *testing.T) {
	if len(Collection) != 152 {
		t.Fatalf("expected 152 themes, got %d", len(Collection))
	}
	seen := make(map[string]bool)
	for _, c := range Collection {
		if seen[c.Slug()] {
			t.Errorf("duplicate slug %s", c.Slug())
		}
		seen[c.Slug()] = true
	}
}

// Every palette is contrast-checked: AAA for body text, AA for every ANSI
// colour against its background.
func TestCollectionIsReadable(t *testing.T) {
	for _, c := range Collection {
		s := c.Scheme()
		if s.Contrast() < color.ContrastAAA {
			t.Errorf("%s: text %.1f:1 below AAA", c.Name, s.Contrast())
		}
		exempt := 0
		if !c.Dark {
			exempt = 7
		}
		for i := 0; i < 8; i++ {
			if i == exempt {
				continue
			}
			if r := color.Contrast(s.Normal[i], s.Background); r < color.ContrastAA {
				t.Errorf("%s: normal %s %.1f:1", c.Name, color.ANSI[i], r)
			}
			if r := color.Contrast(s.Bright[i], s.Background); r < color.ContrastAA {
				t.Errorf("%s: bright %s %.1f:1", c.Name, color.ANSI[i], r)
			}
		}
		if r := color.Contrast(s.Cursor, s.Background); r < color.ContrastAA {
			t.Errorf("%s: cursor %.1f:1", c.Name, r)
		}
	}
}

func TestRenderedThemeParsesBack(t *testing.T) {
	c := Collection[0]
	text := c.Text()
	if !strings.HasPrefix(text, ghostty.CollectionHeader) {
		t.Fatalf("header: %q", text[:40])
	}
	th := ghostty.ParseThemeText(text)
	if len(th.Colors) != 22 {
		t.Fatalf("expected 22 colours, got %d", len(th.Colors))
	}
	if !strings.Contains(th.Note, c.Family) {
		t.Fatalf("note %q lacks family", th.Note)
	}
}

func TestInstallNeverOverwritesEditedFiles(t *testing.T) {
	dir := t.TempDir()
	written, skipped, err := Install(dir)
	if err != nil || written != 152 || skipped != 0 {
		t.Fatalf("first install: %d %d %v", written, skipped, err)
	}
	edited := filepath.Join(dir, Collection[3].Slug())
	if err := os.WriteFile(edited, []byte("# mine\nbackground = #000000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	written, skipped, err = Install(dir)
	if err != nil || written != 151 || skipped != 1 {
		t.Fatalf("second install: %d %d %v", written, skipped, err)
	}
	data, _ := os.ReadFile(edited)
	if string(data) != "# mine\nbackground = #000000\n" {
		t.Fatal("edited file was overwritten")
	}
}

func TestPowerShellHomageUsesCampbell(t *testing.T) {
	for _, c := range Collection {
		if c.Name != "PowerShell" {
			continue
		}
		s := c.Scheme()
		if s.Normal[0] != "#0c0c0c" {
			t.Fatalf("black should be Campbell's, got %s", s.Normal[0])
		}
		return
	}
	t.Fatal("PowerShell theme missing")
}
