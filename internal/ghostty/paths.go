package ghostty

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Paths is everything the tool needs to find on disk.
type Paths struct {
	// ConfigDir is $XDG_CONFIG_HOME/ghostty, the directory Ghostty searches
	// for user themes and the natural home for a config file that does not
	// exist yet.
	ConfigDir string
	// Roots are the default config files in the order Ghostty loads them.
	// Not all of them exist; the ones that do are read, and new keys go to
	// the last existing one, which is the one whose values win.
	Roots []string
	// ThemesDir is where user themes live. Ghostty searches only this
	// directory and its own resources; it never looks under Application
	// Support for themes, even on macOS.
	ThemesDir string
	// ResourceThemeDirs are the directories Ghostty ships its themes in.
	ResourceThemeDirs []string
	// StateDir holds this tool's own files: favourites, settings, backups.
	StateDir string
	// Binary is the ghostty executable, used for +list-fonts. Empty when it
	// cannot be found; the font browser then falls back to the platform's
	// own font database.
	Binary string
}

// DefaultPaths resolves the standard locations for this platform. A non-empty
// configOverride replaces the default roots with that single file.
func DefaultPaths(configOverride, themesOverride string) Paths {
	home, _ := os.UserHomeDir()
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	p := Paths{
		ConfigDir: filepath.Join(xdg, "ghostty"),
		StateDir:  filepath.Join(xdg, "ghostty-config"),
	}

	// Ghostty 1.3 reads both the historical `config` and the newer
	// `config.ghostty`, in that order, first from XDG and then, on macOS,
	// from Application Support. Later files override earlier ones.
	p.Roots = []string{
		filepath.Join(p.ConfigDir, "config"),
		filepath.Join(p.ConfigDir, "config.ghostty"),
	}
	if runtime.GOOS == "darwin" {
		support := filepath.Join(home, "Library", "Application Support", "com.mitchellh.ghostty")
		p.Roots = append(p.Roots,
			filepath.Join(support, "config"),
			filepath.Join(support, "config.ghostty"),
		)
	}
	if configOverride != "" {
		p.Roots = []string{configOverride}
	}

	p.ThemesDir = filepath.Join(p.ConfigDir, "themes")
	if themesOverride != "" {
		p.ThemesDir = themesOverride
	}

	p.ResourceThemeDirs = resourceThemeDirs(home)
	p.Binary = findBinary()
	return p
}

// resourceThemeDirs lists where a Ghostty install keeps its bundled themes.
// The environment variable is set inside every Ghostty terminal and is the
// authoritative answer when present.
func resourceThemeDirs(home string) []string {
	var dirs []string
	if env := os.Getenv("GHOSTTY_RESOURCES_DIR"); env != "" {
		dirs = append(dirs, filepath.Join(env, "themes"))
	}
	candidates := []string{
		"/Applications/Ghostty.app/Contents/Resources/ghostty/themes",
		filepath.Join(home, "Applications", "Ghostty.app", "Contents", "Resources", "ghostty", "themes"),
		"/usr/share/ghostty/themes",
		"/usr/local/share/ghostty/themes",
		"/opt/homebrew/share/ghostty/themes",
		filepath.Join(home, ".local", "share", "ghostty", "themes"),
		"/var/lib/flatpak/app/com.mitchellh.ghostty/current/active/files/share/ghostty/themes",
		"/snap/ghostty/current/share/ghostty/themes",
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			dirs = append(dirs, c)
		}
	}
	return uniqueStrings(dirs)
}

// findBinary locates the ghostty executable. On macOS the CLI lives inside
// the app bundle and is rarely on PATH.
func findBinary() string {
	if path, err := exec.LookPath("ghostty"); err == nil {
		return path
	}
	if env := os.Getenv("GHOSTTY_BIN_DIR"); env != "" {
		if candidate := filepath.Join(env, "ghostty"); isExecutable(candidate) {
			return candidate
		}
	}
	home, _ := os.UserHomeDir()
	for _, candidate := range []string{
		"/Applications/Ghostty.app/Contents/MacOS/ghostty",
		filepath.Join(home, "Applications", "Ghostty.app", "Contents", "MacOS", "ghostty"),
		"/usr/bin/ghostty", "/usr/local/bin/ghostty", "/opt/homebrew/bin/ghostty",
	} {
		if isExecutable(candidate) {
			return candidate
		}
	}
	return ""
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0111 != 0
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// InsideGhostty reports whether this process is running in a Ghostty
// terminal, which is what makes the live preview meaningful.
func InsideGhostty() bool {
	return os.Getenv("TERM_PROGRAM") == "ghostty" || os.Getenv("GHOSTTY_RESOURCES_DIR") != ""
}
