package ghostty

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// State is this tool's own memory: favourites, and the few preferences that
// belong to the editor rather than to Ghostty.
type State struct {
	Favorites  []string `json:"favorites"`
	Welcomed   bool     `json:"welcomed"`
	AutoReload *bool    `json:"auto_reload,omitempty"`
	MonoOnly   *bool    `json:"mono_only,omitempty"`
	// Interface is the colour scheme of the editor's own chrome, kept apart
	// from the theme being looked at: "clear", "paper", "graphite" or "theme".
	Interface string `json:"interface,omitempty"`
	// The rest of the editor's look, each empty for the look it always had:
	// Accent is the one colour that marks what is on ("theme" or one of the
	// tool's own), Contrast how far the quiet text and grounds stand from the
	// background, UISize how large the picture interface draws everything,
	// and Cards how wide a theme's card is on the wall.
	Accent   string `json:"accent,omitempty"`
	Contrast string `json:"contrast,omitempty"`
	UISize   string `json:"ui_size,omitempty"`
	Cards    string `json:"cards,omitempty"`
	// Profile is how the editor is laid out: ProfileAtelier, ProfilePalette
	// or ProfileGuided. Empty until one is chosen, which the editor asks for
	// before anything else.
	Profile string `json:"profile,omitempty"`
	// What the command line can say for one launch, kept for every launch:
	// Drawing is how the interface is drawn ("auto" or empty, "images",
	// "cells" or "text", as -images, -cells and -no-images), Plain keeps to
	// glyphs every font has (-plain), and UpdateCheck, nil for yes, whether
	// GitHub is asked for a newer release (-no-update-check). A flag given
	// wins over what is kept here.
	Drawing     string `json:"drawing,omitempty"`
	Plain       bool   `json:"plain,omitempty"`
	UpdateCheck *bool  `json:"update_check,omitempty"`
	// UpdateCheckedAt and LatestRelease remember the last look at GitHub,
	// so the network is asked at most once a day.
	UpdateCheckedAt time.Time `json:"update_checked_at,omitempty"`
	LatestRelease   string    `json:"latest_release,omitempty"`
	path            string
	favSet          map[string]bool
}

// The layouts of the editor, as the state file names them.
const (
	ProfileAtelier = "atelier"
	ProfilePalette = "palette"
	ProfileGuided  = "guided"
)

// LoadState reads state.json, or starts fresh.
func LoadState(paths Paths) *State {
	s := &State{path: filepath.Join(paths.StateDir, "state.json"), favSet: make(map[string]bool)}
	if data, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(data, s)
	}
	for _, f := range s.Favorites {
		s.favSet[f] = true
	}
	return s
}

// Save writes the state.
func (s *State) Save() error {
	s.Favorites = s.Favorites[:0]
	for name := range s.favSet {
		s.Favorites = append(s.Favorites, name)
	}
	sort.Strings(s.Favorites)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// IsFavorite reports whether a theme is starred.
func (s *State) IsFavorite(name string) bool { return s.favSet[name] }

// ToggleFavorite flips a star and reports the new state.
func (s *State) ToggleFavorite(name string) bool {
	if s.favSet[name] {
		delete(s.favSet, name)
		return false
	}
	s.favSet[name] = true
	return true
}

// FavoriteCount is how many themes are starred.
func (s *State) FavoriteCount() int { return len(s.favSet) }

// AutoReloadEnabled defaults to true.
func (s *State) AutoReloadEnabled() bool { return s.AutoReload == nil || *s.AutoReload }

// SetAutoReload records the preference.
func (s *State) SetAutoReload(v bool) { s.AutoReload = &v }

// UpdateCheckEnabled defaults to true.
func (s *State) UpdateCheckEnabled() bool { return s.UpdateCheck == nil || *s.UpdateCheck }

// SetUpdateCheck records the preference.
func (s *State) SetUpdateCheck(v bool) { s.UpdateCheck = &v }

// MonoOnlyEnabled defaults to true.
func (s *State) MonoOnlyEnabled() bool { return s.MonoOnly == nil || *s.MonoOnly }

// SetMonoOnly records the preference.
func (s *State) SetMonoOnly(v bool) { s.MonoOnly = &v }

// ProfileChosen is whether a layout was chosen, one this build knows.
func (s *State) ProfileChosen() bool {
	switch s.Profile {
	case ProfileAtelier, ProfilePalette, ProfileGuided:
		return true
	}
	return false
}

// ActiveProfile is the layout in effect: the one chosen, Atelier until then.
func (s *State) ActiveProfile() string {
	if s.ProfileChosen() {
		return s.Profile
	}
	return ProfileAtelier
}

// Returning is whether the editor was used before: its state carries what
// only a session leaves behind. A state file alone does not tell, since
// leaving from the very first screen writes one.
func (s *State) Returning() bool { return s.Welcomed || len(s.favSet) > 0 }
