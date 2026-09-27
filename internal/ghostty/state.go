package ghostty

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// State is this tool's own memory: favourites, and the few preferences that
// belong to the editor rather than to Ghostty.
type State struct {
	Favorites  []string `json:"favorites"`
	Welcomed   bool     `json:"welcomed"`
	AutoReload *bool    `json:"auto_reload,omitempty"`
	MonoOnly   *bool    `json:"mono_only,omitempty"`
	// Interface is the colour scheme of the editor's own chrome, kept apart
	// from the theme being looked at: "graphite", "paper" or "theme".
	Interface string `json:"interface,omitempty"`
	path      string
	favSet    map[string]bool
}

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

// MonoOnlyEnabled defaults to true.
func (s *State) MonoOnlyEnabled() bool { return s.MonoOnly == nil || *s.MonoOnly }

// SetMonoOnly records the preference.
func (s *State) SetMonoOnly(v bool) { s.MonoOnly = &v }
