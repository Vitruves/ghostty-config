package tui

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/vitruves/ghostty-config/internal/collection"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

func randInt(n int) int { return rand.Intn(n) }

func fileExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// sourceWord says where a theme comes from, for the status line.
func sourceWord(t *ghostty.Theme) string {
	switch t.Source {
	case ghostty.SourceBuiltin:
		return "bundled with Ghostty"
	case ghostty.SourceCollection:
		return "curated collection"
	case ghostty.SourceOwned:
		return "yours, saved by ghostty-config"
	case ghostty.SourceDraft:
		return "draft, not saved yet"
	}
	return "your themes directory"
}

// styleWord is dark or light, for messages.
func styleWord(t *ghostty.Theme) string {
	if t.IsDark() {
		return "dark"
	}
	return "light"
}

// homeDir is the user's home directory, or "".
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// themeCommand is whether the open command browses themes.
func themeCommand(c *command) bool {
	if c == nil {
		return false
	}
	switch c.name {
	case "theme", "favs", "delete":
		return true
	}
	return false
}

// themeFamily is the short label for where a theme comes from: the family of
// a collection theme, or its source.
func themeFamily(t *ghostty.Theme) string {
	if t.Source == ghostty.SourceCollection {
		if fam, _, ok := strings.Cut(t.Note, " · "); ok {
			return fam
		}
		return "collection"
	}
	switch t.Source {
	case ghostty.SourceBuiltin:
		return "bundled"
	case ghostty.SourceOwned:
		return "yours"
	case ghostty.SourceDraft:
		return "draft"
	}
	return "installed"
}

// familyName reads a typed filter as a family of the collection when it is
// exactly one, so `theme Gestalt` lists that family.
func familyName(needle string) string {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return ""
	}
	for _, f := range collection.Families() {
		if strings.EqualFold(f, needle) {
			return f
		}
	}
	for _, f := range []string{"bundled", "yours"} {
		if strings.EqualFold(f, needle) {
			return f
		}
	}
	return ""
}
