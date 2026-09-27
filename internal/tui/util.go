package tui

import (
	"math/rand"
	"os"
	"path/filepath"

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
