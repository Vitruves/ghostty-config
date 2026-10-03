package fontdl

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Package fontdl installs monospace families from the Nerd Fonts release
// archive into the user font directory.
//
// Downloadable monospace fonts, taken from the Nerd Fonts release archive.
//
// Nerd Fonts is used rather than each project's own release because it patches
// every family the same way — the powerline and icon glyphs a terminal prompt
// expects are already there — and because one URL shape covers all of them.
// The "latest" download endpoint is stable, so no version is pinned here and
// no release index has to be fetched first.

// nerdFontsRelease is the redirecting endpoint for the newest release assets.
const nerdFontsRelease = "https://github.com/ryanoasis/nerd-fonts/releases/latest/download/"

// FontDownload is one installable family.
type FontDownload struct {
	Name    string // display name
	Archive string // asset file name, without .zip
	Note    string
	// URL is the archive's address when it is not in the Nerd Fonts release.
	URL string
}

// AvailableDownloads is the offered set: widely used, actively maintained, and
// all genuinely monospaced. Kept short on purpose — a list of ninety families
// is a worse answer to "I need a good terminal font" than a list of fifteen.
var AvailableDownloads = []FontDownload{
	{"Google Sans Code", "GoogleSansCode", "Google's monospace, the font this tool draws its own pictures in",
		"https://github.com/googlefonts/googlesans-code/releases/download/v7.001/GoogleSansCode-v7.001.zip"},
	{"JetBrains Mono", "JetBrainsMono", "Tall x-height, made for long reading", ""},
	{"Fira Code", "FiraCode", "The one with the programming ligatures", ""},
	{"Hack", "Hack", "Plain, legible, no surprises", ""},
	{"Meslo LG", "Meslo", "Menlo redrawn, the powerline default", ""},
	{"Source Code Pro", "SourceCodePro", "Adobe, conservative and even", ""},
	{"Cascadia Code", "CascadiaCode", "Microsoft, ships with Windows Terminal", ""},
	{"Iosevka", "Iosevka", "Narrow, fits more columns per line", ""},
	{"Ubuntu Mono", "UbuntuMono", "Humanist, unusually compact", ""},
	{"DejaVu Sans Mono", "DejaVuSansMono", "Enormous glyph coverage", ""},
	{"Roboto Mono", "RobotoMono", "Neutral and unfussy", ""},
	{"Inconsolata", "Inconsolata", "A classic, still excellent", ""},
	{"Victor Mono", "VictorMono", "Cursive italics for comments", ""},
	{"Space Mono", "SpaceMono", "Distinctive, with real character", ""},
	{"IBM Plex Mono", "IBMPlexMono", "Corporate in the good sense", ""},
	{"Anonymous Pro", "AnonymousPro", "Designed for small sizes", ""},
	{"Terminus", "Terminus", "Bitmap heritage, sharp at small sizes", ""},
}

// downloadTimeout is generous: some of these archives are tens of megabytes.
const downloadTimeout = 5 * time.Minute

// FontInstallDir is where user fonts belong on this platform.
func FontInstallDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Fonts"), nil
	case "windows":
		return filepath.Join(home, "AppData", "Local", "Microsoft", "Windows", "Fonts"), nil
	default:
		return filepath.Join(home, ".local", "share", "fonts"), nil
	}
}

// Install downloads a font archive and unpacks its faces into the user font
// directory. progress is called with human-readable steps; it may be nil.
//
// Nothing is installed system-wide and nothing needs elevated rights: user font
// directories are picked up by both Core Text and fontconfig.
func Install(font FontDownload, progress func(string)) error {
	report := func(format string, args ...interface{}) {
		if progress != nil {
			progress(fmt.Sprintf(format, args...))
		}
	}

	// Windows does not install a font by having the file present.
	//
	// Copying into the per-user font directory leaves the face unregistered:
	// Windows also wants a value under HKCU\...\CurrentVersion\Fonts and a
	// broadcast telling running programs to reload. Doing half of that would
	// report success for a font the terminal still cannot use, which is worse
	// than declining. Say so instead, and name the one step that does work.
	if runtime.GOOS == "windows" {
		return fmt.Errorf(
			"installing fonts is not supported on Windows yet — download %s.zip from "+
				"github.com/ryanoasis/nerd-fonts/releases and open the files to install them",
			font.Archive)
	}

	dir, err := FontInstallDir()
	if err != nil {
		return fmt.Errorf("could not locate the font directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("could not create %s: %w", dir, err)
	}

	url := nerdFontsRelease + font.Archive + ".zip"
	if font.URL != "" {
		url = font.URL
	}
	report("Fetching %s…", font.Name)

	archive, err := downloadToTemp(url)
	if err != nil {
		return err
	}
	defer os.Remove(archive)

	report("Unpacking %s…", font.Name)
	installed, err := extractFaces(archive, dir)
	if err != nil {
		return err
	}
	if installed == 0 {
		return fmt.Errorf("%s contained no font files", font.Name)
	}

	// fontconfig caches aggressively; Core Text notices new files by itself.
	if runtime.GOOS == "linux" {
		if err := exec.Command("fc-cache", "-f", dir).Run(); err != nil {
			report("Installed, but fc-cache failed: %v", err)
		}
	}

	report("Installed %d faces of %s", installed, font.Name)
	return nil
}

// downloadToTemp streams url to a temporary file and returns its path.
func downloadToTemp(url string) (string, error) {
	client := &http.Client{Timeout: downloadTimeout}

	response, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: the server answered %s", response.Status)
	}

	file, err := os.CreateTemp("", "ghostty-config-font-*.zip")
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(file, response.Body); err != nil {
		file.Close()
		os.Remove(file.Name())
		return "", fmt.Errorf("download interrupted: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(file.Name())
		return "", err
	}

	return file.Name(), nil
}

// extractFaces writes every font file in the archive into dir, flattened, and
// returns how many it wrote.
func extractFaces(archivePath, dir string) (int, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return 0, fmt.Errorf("could not read the archive: %w", err)
	}
	defer reader.Close()

	installed := 0
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() || !isFontFile(entry.Name) {
			continue
		}

		// Flatten to the base name, and reject anything that tries to escape
		// the target directory through its path.
		name := filepath.Base(filepath.Clean(entry.Name))
		if name == "." || name == string(filepath.Separator) || strings.HasPrefix(name, "..") {
			continue
		}

		if err := writeEntry(entry, filepath.Join(dir, name)); err != nil {
			return installed, err
		}
		installed++
	}

	return installed, nil
}

// writeEntry copies one archive entry to disk.
func writeEntry(entry *zip.File, target string) error {
	source, err := entry.Open()
	if err != nil {
		return fmt.Errorf("could not read %s from the archive: %w", entry.Name, err)
	}
	defer source.Close()

	destination, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("could not write %s: %w", target, err)
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return fmt.Errorf("could not write %s: %w", target, err)
	}
	return nil
}

// isFontFile reports whether a name looks like an installable face.
//
// Nerd Fonts archives carry several cuts of the same family. The "Windows
// Compatible" ones differ only in their internal naming and would clutter the
// list with duplicates, so they are skipped.
func isFontFile(name string) bool {
	if strings.Contains(name, "Windows Compatible") {
		return false
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ttf", ".otf":
		return true
	}
	return false
}
