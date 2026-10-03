package ghostty

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vitruves/ghostty-config/internal/color"
)

// Source says where a theme file came from, which decides what may be done
// to it.
type Source int

const (
	SourceBuiltin    Source = iota // shipped with Ghostty, read only
	SourceUser                     // in the user's themes directory, written by someone else
	SourceOwned                    // in the user's themes directory, written by this tool
	SourceCollection               // the curated collection, installed by this tool
	SourceDraft                    // generated, not on disk yet
)

// Headers mark files this tool wrote. A file with OwnedHeader may be
// overwritten in place; a collection file is refreshed on reinstall but
// editing it forks; anything else is never touched.
const (
	OwnedHeader      = "# ghostty-config ·"
	CollectionHeader = "# ghostty-config collection ·"
)

// ColorKeys are the theme settings the editor manages, in display order.
// Palette slots are "palette.N".
var ColorKeys = buildColorKeys()

func buildColorKeys() []string {
	keys := []string{"background", "foreground", "cursor-color", "cursor-text", "selection-background", "selection-foreground"}
	for i := 0; i < 16; i++ {
		keys = append(keys, fmt.Sprintf("palette.%d", i))
	}
	return keys
}

// Section groups the keys for the palette panel.
type Section struct {
	Title string
	Keys  []string
}

// Sections is the panel layout: primary pair, cursor pair, selection pair,
// then the two ANSI rows.
var Sections = buildSections()

func buildSections() []Section {
	normal, bright := make([]string, 8), make([]string, 8)
	for i := 0; i < 8; i++ {
		normal[i] = fmt.Sprintf("palette.%d", i)
		bright[i] = fmt.Sprintf("palette.%d", i+8)
	}
	return []Section{
		{"Primary", []string{"background", "foreground"}},
		{"Cursor", []string{"cursor-color", "cursor-text"}},
		{"Selection", []string{"selection-background", "selection-foreground"}},
		{"Normal", normal},
		{"Bright", bright},
	}
}

// Label renders a key the way the panel shows it: palette slots by their
// ANSI name, the rest by their second half.
func Label(key string) string {
	if idx, ok := PaletteIndex(key); ok {
		return color.ANSI[idx%8]
	}
	switch key {
	case "background", "foreground":
		return key
	}
	_, rest, _ := strings.Cut(key, "-")
	return rest
}

// PaletteIndex reads the N of "palette.N".
func PaletteIndex(key string) (int, bool) {
	if !strings.HasPrefix(key, "palette.") {
		return 0, false
	}
	var n int
	if _, err := fmt.Sscanf(key, "palette.%d", &n); err != nil {
		return 0, false
	}
	return n, true
}

// Theme is one colour scheme: the managed colours, plus every other line of
// the file kept verbatim so that saving a fork loses nothing.
type Theme struct {
	Name   string
	Path   string
	Source Source
	Colors map[string]string
	// Passthrough holds entries this editor does not manage (a 256-colour
	// palette extension, bold-color, and so on). They are written back after
	// the managed colours.
	Passthrough []string
	// Note is the first comment of the file, shown as the theme's description.
	Note string
}

// Clone returns an independent copy.
func (t *Theme) Clone() *Theme {
	c := *t
	c.Colors = make(map[string]string, len(t.Colors))
	for k, v := range t.Colors {
		c.Colors[k] = v
	}
	c.Passthrough = append([]string(nil), t.Passthrough...)
	return &c
}

// Get returns a colour or "".
func (t *Theme) Get(key string) string {
	if t == nil || t.Colors == nil {
		return ""
	}
	return t.Colors[key]
}

// Background and Foreground fall back to Ghostty's own defaults.
func (t *Theme) Background() string { return color.Normalize(t.Get("background"), "#282c34") }
func (t *Theme) Foreground() string { return color.Normalize(t.Get("foreground"), "#ffffff") }

// IsDark reports whether the theme has a dark background.
func (t *Theme) IsDark() bool { return color.IsDark(t.Background()) }

// Writable reports whether saving may overwrite the file in place.
func (t *Theme) Writable() bool { return t.Source == SourceOwned }

// Equal compares the managed colours of two themes.
func (t *Theme) Equal(o *Theme) bool {
	if t == nil || o == nil {
		return t == o
	}
	if len(t.Colors) != len(o.Colors) {
		return false
	}
	for k, v := range t.Colors {
		if o.Colors[k] != v {
			return false
		}
	}
	return true
}

// ParseThemeFile reads a theme from disk.
func ParseThemeFile(path string, source Source) (*Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	t := ParseThemeText(string(data))
	t.Name = filepath.Base(path)
	t.Path = path
	t.Source = source
	if source == SourceUser {
		switch {
		case strings.HasPrefix(string(data), CollectionHeader):
			t.Source = SourceCollection
		case strings.HasPrefix(string(data), OwnedHeader):
			t.Source = SourceOwned
		}
	}
	return t, nil
}

// ParseThemeText reads a theme from config text. A theme file uses the same
// syntax as the config itself.
func ParseThemeText(text string) *Theme {
	t := &Theme{Colors: make(map[string]string)}
	doc := ParseText(text)
	noteDone := false
	for _, l := range doc.Lines {
		switch l.Kind {
		case LineComment:
			if !noteDone {
				note := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l.Raw), "#"))
				if strings.HasPrefix(l.Raw, OwnedHeader) || strings.HasPrefix(l.Raw, CollectionHeader) {
					continue
				}
				if note != "" {
					t.Note = note
					noteDone = true
				}
			}
		case LineEntry:
			noteDone = true
			switch l.Key {
			case "background", "foreground", "cursor-color", "cursor-text", "selection-background", "selection-foreground":
				if color.IsHex(l.Value) {
					t.Colors[l.Key] = color.Normalize(l.Value, "")
				} else {
					t.Passthrough = append(t.Passthrough, strings.TrimSpace(l.Raw))
				}
			case "palette":
				idx, value, ok := parsePaletteValue(l.Value)
				if ok && idx < 16 && color.IsHex(value) {
					t.Colors[fmt.Sprintf("palette.%d", idx)] = color.Normalize(value, "")
				} else {
					t.Passthrough = append(t.Passthrough, strings.TrimSpace(l.Raw))
				}
			default:
				t.Passthrough = append(t.Passthrough, strings.TrimSpace(l.Raw))
			}
		}
	}
	return t
}

// Render writes the theme as a Ghostty theme file, with the header that
// marks it as this tool's own.
func (t *Theme) Render(header string) string {
	var b strings.Builder
	if header != "" {
		fmt.Fprintf(&b, "%s %s\n", header, t.Name)
	}
	if t.Note != "" {
		fmt.Fprintf(&b, "# %s\n", t.Note)
	}
	if header != "" || t.Note != "" {
		b.WriteString("\n")
	}
	write := func(key string) {
		if v := t.Colors[key]; v != "" {
			fmt.Fprintf(&b, "%s = %s\n", key, v)
		}
	}
	write("background")
	write("foreground")
	write("cursor-color")
	write("cursor-text")
	write("selection-background")
	write("selection-foreground")
	b.WriteString("\n")
	for i := 0; i < 16; i++ {
		if v := t.Colors[fmt.Sprintf("palette.%d", i)]; v != "" {
			fmt.Fprintf(&b, "palette = %d=%s\n", i, v)
		}
	}
	if len(t.Passthrough) > 0 {
		b.WriteString("\n")
		for _, line := range t.Passthrough {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// FromScheme turns a generated scheme into a theme.
func FromScheme(s *color.Scheme, note string) *Theme {
	t := &Theme{Name: s.Name, Source: SourceDraft, Colors: make(map[string]string, 22), Note: note}
	t.Colors["background"] = s.Background
	t.Colors["foreground"] = s.Foreground
	t.Colors["cursor-color"] = s.Cursor
	t.Colors["cursor-text"] = s.CursorText
	t.Colors["selection-background"] = s.Selection
	t.Colors["selection-foreground"] = s.SelectText
	for i := 0; i < 8; i++ {
		t.Colors[fmt.Sprintf("palette.%d", i)] = s.Normal[i]
		t.Colors[fmt.Sprintf("palette.%d", i+8)] = s.Bright[i]
	}
	return t
}

// Library is every theme Ghostty can resolve by name, user directory first:
// a user file shadows a bundled one of the same name, exactly as Ghostty
// resolves `theme =`.
type Library struct {
	Themes []*Theme
	byName map[string]*Theme
}

// LoadLibrary scans the user themes directory and the resources directories.
func LoadLibrary(paths Paths) *Library {
	lib := &Library{byName: make(map[string]*Theme)}
	add := func(dir string, source Source) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || strings.HasPrefix(name, ".") {
				continue
			}
			if _, taken := lib.byName[name]; taken {
				continue
			}
			t, err := ParseThemeFile(filepath.Join(dir, name), source)
			if err != nil || len(t.Colors) == 0 {
				continue
			}
			lib.byName[name] = t
			lib.Themes = append(lib.Themes, t)
		}
	}
	add(paths.ThemesDir, SourceUser)
	for _, dir := range paths.ResourceThemeDirs {
		add(dir, SourceBuiltin)
	}
	sort.Slice(lib.Themes, func(i, j int) bool {
		return strings.ToLower(lib.Themes[i].Name) < strings.ToLower(lib.Themes[j].Name)
	})
	return lib
}

// Get finds a theme by name. Ghostty also accepts an absolute path.
func (l *Library) Get(name string) (*Theme, bool) {
	if t, ok := l.byName[name]; ok {
		return t, true
	}
	if filepath.IsAbs(name) {
		if t, err := ParseThemeFile(name, SourceUser); err == nil {
			return t, true
		}
	}
	return nil, false
}

// Names returns every theme name in display order.
func (l *Library) Names() []string {
	out := make([]string, len(l.Themes))
	for i, t := range l.Themes {
		out[i] = t.Name
	}
	return out
}

// Exists reports whether a name is taken in the user directory.
func Exists(paths Paths, name string) bool {
	_, err := os.Stat(filepath.Join(paths.ThemesDir, name))
	return err == nil
}

// SaveTheme writes a theme into the user directory under name, marked as
// owned, and returns the resulting theme.
func SaveTheme(paths Paths, t *Theme, name string) (*Theme, error) {
	if err := os.MkdirAll(paths.ThemesDir, 0o755); err != nil {
		return nil, err
	}
	saved := t.Clone()
	saved.Name = name
	saved.Path = filepath.Join(paths.ThemesDir, name)
	saved.Source = SourceOwned
	if err := writeFileAtomic(saved.Path, []byte(saved.Render(OwnedHeader)), 0o644); err != nil {
		return nil, err
	}
	return saved, nil
}

// Deletable reports whether the theme is a file in the user's themes
// directory. Bundled themes belong to Ghostty and are read only.
func (t *Theme) Deletable() bool {
	switch t.Source {
	case SourceOwned, SourceCollection, SourceUser:
		return t.Path != ""
	}
	return false
}

// DeleteTheme removes a theme file from the user's themes directory. The
// caller is expected to have asked first when the file was written by
// someone else.
func DeleteTheme(t *Theme) error {
	if !t.Deletable() {
		return fmt.Errorf("%s is bundled with Ghostty and cannot be deleted", t.Name)
	}
	return os.Remove(t.Path)
}

// SanitizeName reduces user input to a file name Ghostty accepts as a theme
// name: no path separators, nothing a shell would trip on.
func SanitizeName(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.', r == ' ':
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), ". ")
}

// ThemeSetting reads the effective `theme =` value, resolving the
// light/dark form against the desktop appearance.
type ThemeSetting struct {
	Raw   string
	Light string
	Dark  string
	Pair  bool
}

// ParseThemeSetting reads a `theme =` value.
func ParseThemeSetting(raw string) ThemeSetting {
	s := ThemeSetting{Raw: strings.TrimSpace(raw)}
	if !strings.Contains(raw, "light:") && !strings.Contains(raw, "dark:") {
		return s
	}
	for _, part := range strings.Split(raw, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "light":
			s.Light = strings.TrimSpace(v)
		case "dark":
			s.Dark = strings.TrimSpace(v)
		}
	}
	s.Pair = s.Light != "" || s.Dark != ""
	return s
}

// Active returns the theme name in effect.
func (s ThemeSetting) Active(darkDesktop bool) string {
	if !s.Pair {
		return s.Raw
	}
	if darkDesktop && s.Dark != "" {
		return s.Dark
	}
	if s.Light != "" {
		return s.Light
	}
	return s.Dark
}

// With returns the setting text after replacing the slot a theme belongs to.
// A plain setting becomes the name; a light/dark pair keeps its other half.
func (s ThemeSetting) With(name string, dark bool) string {
	if !s.Pair {
		return name
	}
	light, darkName := s.Light, s.Dark
	if dark {
		darkName = name
	} else {
		light = name
	}
	return fmt.Sprintf("light:%s,dark:%s", light, darkName)
}
