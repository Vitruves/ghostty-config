package ghostty

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Family is one font family Ghostty can load, with the style names it
// advertises and whether it is monospaced.
type Family struct {
	Name   string
	Styles []string
	Mono   bool
	// Faces are the full face names Ghostty reported, kept for the sample.
	Faces []string
}

// FontCatalog caches the scan: it costs a few hundred milliseconds and the
// browser asks for it on every keystroke.
type FontCatalog struct {
	mu       sync.Mutex
	families []Family
	scanned  bool
	binary   string
}

// NewFontCatalog builds a catalog that lists through the given ghostty
// binary, or through the platform when it is empty.
func NewFontCatalog(binary string) *FontCatalog {
	return &FontCatalog{binary: binary}
}

// Refresh drops the cache so the next call rescans, after fonts were installed.
func (c *FontCatalog) Refresh() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.families, c.scanned = nil, false
}

// Families returns the installed families. Only names Ghostty can resolve are
// offered: `ghostty +list-fonts` uses the exact discovery Ghostty itself uses,
// so nothing listed can produce a font that fails to load. Style names and
// the monospace flag come from the platform's font database, which is the
// same one Ghostty reads through.
func (c *FontCatalog) Families() []Family {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.scanned {
		return c.families
	}
	c.scanned = true

	platform := platformFamilies()
	ghosttyNames, faces := listFontsViaGhostty(c.binary)

	var out []Family
	if len(ghosttyNames) > 0 {
		for _, name := range ghosttyNames {
			f := Family{Name: name, Faces: faces[name]}
			if p, ok := platform[name]; ok {
				f.Styles, f.Mono = p.Styles, p.Mono
			} else {
				f.Styles = stylesFromFaces(name, faces[name])
				f.Mono = looksMonospaced(name)
			}
			out = append(out, f)
		}
	} else {
		for name, p := range platform {
			out = append(out, Family{Name: name, Styles: p.Styles, Mono: p.Mono})
		}
	}
	for i := range out {
		if len(out[i].Styles) == 0 {
			out[i].Styles = []string{"Regular"}
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	c.families = out
	return out
}

// Find returns a family by name.
func (c *FontCatalog) Find(name string) (Family, bool) {
	for _, f := range c.Families() {
		if f.Name == name {
			return f, true
		}
	}
	return Family{}, false
}

// listFontsViaGhostty parses `ghostty +list-fonts`: a family name on its own
// line, its faces indented below, families separated by blank lines.
func listFontsViaGhostty(binary string) ([]string, map[string][]string) {
	if binary == "" {
		return nil, nil
	}
	cmd := exec.Command(binary, "+list-fonts")
	cmd.Env = append(os.Environ(), "LC_ALL=en_US.UTF-8")
	output, err := cmd.Output()
	if err != nil {
		return nil, nil
	}
	var names []string
	faces := make(map[string][]string)
	current := ""
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			current = ""
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if current != "" {
				faces[current] = append(faces[current], strings.TrimSpace(line))
			}
			continue
		}
		current = strings.TrimSpace(line)
		if strings.HasPrefix(current, ".") {
			// Private system faces: listed, but not a name anyone should set.
			current = ""
			continue
		}
		if _, seen := faces[current]; !seen {
			names = append(names, current)
			faces[current] = nil
		}
	}
	return names, faces
}

// stylesFromFaces derives style names by stripping the family prefix from
// face names, for platforms with no better answer.
func stylesFromFaces(family string, faces []string) []string {
	var styles []string
	seen := make(map[string]bool)
	compactFamily := strings.ReplaceAll(strings.ToLower(family), " ", "")
	for _, face := range faces {
		style := face
		compactFace := strings.ReplaceAll(strings.ToLower(face), " ", "")
		if strings.HasPrefix(compactFace, compactFamily) {
			// Walk the original string to keep the style's own spelling.
			n := 0
			for i, r := range face {
				if r != ' ' {
					n++
				}
				if n == len(compactFamily) {
					style = strings.TrimSpace(face[i+1:])
					break
				}
			}
		}
		if style == "" {
			style = "Regular"
		}
		if !seen[style] {
			seen[style] = true
			styles = append(styles, style)
		}
	}
	return styles
}

func looksMonospaced(name string) bool {
	n := strings.ToLower(name)
	for _, hint := range []string{"mono", "code", "nerd", "console", "terminal", "courier", "menlo", "monaco", "fixed", "hack", "iosevka", "inconsolata", "plex mono"} {
		if strings.Contains(n, hint) {
			return true
		}
	}
	return false
}

type platformFamily struct {
	Styles []string
	Mono   bool
}

// platformFamilies asks the platform font database for every family, its
// style names as it spells them, and whether it is monospaced.
func platformFamilies() map[string]platformFamily {
	switch runtime.GOOS {
	case "darwin":
		if fams, err := coreTextFamilies(); err == nil {
			return fams
		}
	case "windows":
		return nil
	}
	return fontconfigFamilies()
}

// fontconfigFamilies reads fc-list. Spacing 100 is monospace, 90 dual width.
func fontconfigFamilies() map[string]platformFamily {
	cmd := exec.Command("fc-list", "--format", "%{family[0]}\t%{style[0]}\t%{spacing}\n")
	cmd.Env = append(os.Environ(), "LC_ALL=en_US.UTF-8")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	out := make(map[string]platformFamily)
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) < 3 || strings.TrimSpace(parts[0]) == "" {
			continue
		}
		name, style, spacing := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
		f := out[name]
		if style != "" && !containsString(f.Styles, style) {
			f.Styles = append(f.Styles, style)
		}
		if spacing == "100" || spacing == "90" {
			f.Mono = true
		}
		out[name] = f
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// NoFontsMessage explains an empty browser in terms of what is missing.
func NoFontsMessage() string {
	intro := "No font could be listed.\n\nOnly families Ghostty can genuinely resolve are offered, so nothing here can fail to load — but that also means an empty list when the font database cannot be read.\n\n"
	switch runtime.GOOS {
	case "darwin":
		return intro + "Neither `ghostty +list-fonts` nor Core Text answered. Check that Ghostty.app is installed in /Applications."
	case "windows":
		return intro + "Font browsing is not implemented on Windows yet. Set font-family by hand; everything else here works."
	default:
		return intro + "Install fontconfig and check that `fc-list` returns something, or make sure `ghostty` is on your PATH."
	}
}

// FontSize limits and the step the browser uses.
const (
	FontSizeMin  = 6.0
	FontSizeMax  = 48.0
	FontSizeStep = 0.5
	DefaultSize  = 13.0
)

// FormatSize renders a size the way Ghostty likes it: no trailing zeros.
func FormatSize(size float64) string {
	if size == float64(int(size)) {
		return fmt.Sprintf("%d", int(size))
	}
	return strings.TrimRight(fmt.Sprintf("%.2f", size), "0")
}
