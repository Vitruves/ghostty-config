package extensions

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// Programs are command-line tools made by other people that put what Ghostty
// can do to use: images drawn with the Kitty graphics protocol, links a click
// opens (OSC 8), a prompt and completions set in a Nerd Font. Unlike shaders
// and themes they are executables, so they are installed the way the system
// already installs programs, with Homebrew, or else from the project's own
// GitHub release, checked against the SHA-256 GitHub publishes for the file.
// Nothing runs as root and nothing is installed outside the home directory.

// Program is one tool of the list.
type Program struct {
	Name    string // what is typed to run it, and its name in the list
	Group   string
	Note    string
	Uses    string // what of Ghostty it puts to use
	Repo    string // owner/name on GitHub, where its releases are
	Licence string
	// Brew is its Homebrew formula; empty when it has none in homebrew/core,
	// and then it is always installed from its release.
	Brew string
	// Bins are the executables taken out of a release archive, the first one
	// being the one that tells whether the program is installed.
	Bins []string
	// Skip are words of release file names that are not this program: a
	// server built beside it, a build for old systems.
	Skip []string
	// Init is the line, per shell, that starts the program with the shell;
	// none means it needs no setup.
	Init map[string]string
	// Mark is what tells a line of the user's own that already starts it.
	Mark []string
	// SelfSetup means the program writes its own lines into the shell's
	// file with "<name> install <shell>" and takes them out with
	// "<name> uninstall <shell>", because where they go is its business.
	SelfSetup bool
	// Caveat is said before it is installed.
	Caveat string
}

const (
	groupShell    = "Shell"
	groupImages   = "Images in the terminal"
	groupLinks    = "Clickable links"
	groupSessions = "Sessions"
)

// Programs is the list on offer, short on purpose: tools that are maintained,
// built for macOS and Linux on ARM and Intel, and that either use something
// Ghostty does well or are what a Ghostty setup is usually built from.
var Programs = []Program{
	{Name: "starship", Group: groupShell, Note: "A fast prompt that shows the folder, the git branch, the language versions and how long the last command took",
		Uses: "Nerd Font glyphs and true colour", Repo: "starship/starship", Licence: "ISC", Brew: "starship", Bins: []string{"starship"},
		Init: map[string]string{"zsh": `eval "$(starship init zsh)"`, "bash": `eval "$(starship init bash)"`, "fish": "starship init fish | source"},
		Mark: []string{"starship init"}},
	{Name: "zoxide", Group: groupShell, Note: "A cd that learns: z proj goes to the folder you visit most whose name matches",
		Uses: "nothing special: a shell helper", Repo: "ajeetdsouza/zoxide", Licence: "MIT", Brew: "zoxide", Bins: []string{"zoxide"},
		Init: map[string]string{"zsh": `eval "$(zoxide init zsh)"`, "bash": `eval "$(zoxide init bash)"`, "fish": "zoxide init fish | source"},
		Mark: []string{"zoxide init"}},
	{Name: "fzf", Group: groupShell, Note: "Fuzzy search: Ctrl+T finds a file, Ctrl+R a past command, Alt+C a folder",
		Uses: "true colour", Repo: "junegunn/fzf", Licence: "MIT", Brew: "fzf", Bins: []string{"fzf"},
		Init: map[string]string{"zsh": "source <(fzf --zsh)", "bash": `eval "$(fzf --bash)"`, "fish": "fzf --fish | source"},
		Mark: []string{"fzf --zsh", "fzf --bash", "fzf --fish"}},
	{Name: "atuin", Group: groupShell, Note: "Your shell history in a searchable database, with the folder, the time and the exit code of every command; Ctrl+R and ↑ open it",
		Uses: "true colour", Repo: "atuinsh/atuin", Licence: "MIT", Brew: "atuin", Bins: []string{"atuin"}, Skip: []string{"server"},
		Init:   map[string]string{"zsh": `eval "$(atuin init zsh)"`, "fish": "atuin init fish | source"},
		Mark:   []string{"atuin init"},
		Caveat: "It takes Ctrl+R and ↑, which fzf's Ctrl+R also wants: the last one started wins."},
	{Name: "carapace", Group: groupShell, Note: "Completions for about a thousand commands in zsh, bash and fish, through the shell's own menu",
		Uses: "nothing special: the shell's own completion", Repo: "carapace-sh/carapace-bin", Licence: "MIT", Brew: "carapace", Bins: []string{"carapace"},
		Init: map[string]string{
			"zsh":  "autoload -Uz compinit; (( $+functions[compdef] )) || compinit; source <(carapace _carapace)",
			"bash": "source <(carapace _carapace)",
			"fish": "carapace _carapace | source"},
		Mark: []string{"carapace _carapace"}},
	{Name: "lig", Group: groupShell, Note: "IDE-style completion: a popup under the cursor with subcommands, options and live values as you type",
		Uses: "Nerd Font glyphs and true colour; keeps Ghostty's shell integration from version 1.2.2", Repo: "KevenGoncalves/lig", Licence: "MIT", Bins: []string{"lig"},
		Mark: ligMarks, SelfSetup: true,
		Caveat: "Young project. It runs your shell inside itself and takes Tab and the arrows while its popup is open; lig uninstall takes it out."},

	{Name: "yazi", Group: groupImages, Note: "A fast file manager with previews of images, videos and PDFs beside the list",
		Uses: "Kitty graphics, named Ghostty in its own documentation", Repo: "sxyazi/yazi", Licence: "MIT", Brew: "yazi", Bins: []string{"yazi", "ya"}},
	{Name: "superfile", Group: groupImages, Note: "A file manager with panels, image previews and mouse support; started with spf",
		Uses: "Kitty graphics, Ghostty listed as supported", Repo: "yorukot/superfile", Licence: "MIT", Brew: "superfile", Bins: []string{"spf"}},
	{Name: "presenterm", Group: groupImages, Note: "Slides written in Markdown, shown in the terminal with their images and code",
		Uses: "Kitty graphics, Ghostty detected by name", Repo: "mfontanini/presenterm", Licence: "BSD-2-Clause", Brew: "presenterm", Bins: []string{"presenterm"}},
	{Name: "fastfetch", Group: groupImages, Note: "Your system at a glance; in Ghostty its logo can be an image",
		Uses: "Kitty graphics; reads Ghostty's own config for the font", Repo: "fastfetch-cli/fastfetch", Licence: "MIT", Brew: "fastfetch", Bins: []string{"fastfetch"}, Skip: []string{"polyfilled"}},
	{Name: "kitten", Group: groupImages, Note: "kitten icat photo.jpg shows an image in the terminal: the reference viewer of the protocol Ghostty draws images with",
		Uses: "Kitty graphics", Repo: "kovidgoyal/kitty", Licence: "GPL-3.0", Bins: []string{"kitten"}},

	{Name: "ripgrep", Group: groupLinks, Note: "Searches files fast; with the alias below, a click on a file name opens it",
		Uses: "OSC 8 hyperlinks", Repo: "BurntSushi/ripgrep", Licence: "MIT or Unlicense", Brew: "ripgrep", Bins: []string{"rg"},
		Init: map[string]string{"zsh": "alias rg='rg --hyperlink-format=default'", "bash": "alias rg='rg --hyperlink-format=default'", "fish": "alias rg 'rg --hyperlink-format=default'"},
		Mark: []string{"alias rg"}},
	{Name: "eza", Group: groupLinks, Note: "ls with colours, icons and git status; ll lists the folder with clickable names",
		Uses: "OSC 8 hyperlinks and Nerd Font icons", Repo: "eza-community/eza", Licence: "EUPL-1.2", Brew: "eza", Bins: []string{"eza"}, Skip: []string{"no_libgit"},
		Init: map[string]string{"zsh": "alias ll='eza -l --hyperlink --icons=auto'", "bash": "alias ll='eza -l --hyperlink --icons=auto'", "fish": "alias ll 'eza -l --hyperlink --icons=auto'"},
		Mark: []string{"alias ll"}},

	{Name: "zmx", Group: groupSessions, Note: "Keeps a shell running when its window closes: zmx attach name brings it back, scrollback included",
		Uses: "built on libghostty-vt, Ghostty's own terminal engine", Repo: "neurosnap/zmx", Licence: "MIT", Bins: []string{"zmx"}},
}

// ligMarks find lig's lines, which name it bare or by a quoted path:
// eval ('/home/me/.local/bin/lig' init fish pre | string collect).
var ligMarks = []string{"lig init", "lig' init", `lig" init`}

// FindProgram looks a program up by name.
func FindProgram(name string) (Program, bool) {
	for _, p := range Programs {
		if p.Name == name {
			return p, true
		}
	}
	return Program{}, false
}

// Bin is the executable that tells whether the program is installed.
func (p Program) Bin() string { return p.Bins[0] }

// LocalBin is where programs installed from a release go.
func LocalBin(home string) string { return filepath.Join(home, ".local", "bin") }

// brewPaths are where Homebrew lives when it is not on the PATH.
var brewPaths = []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew", "/home/linuxbrew/.linuxbrew/bin/brew"}

// Brew is the brew executable, or "" when Homebrew is not installed. It is a
// variable so tests choose.
var Brew = func() string {
	if p, err := exec.LookPath("brew"); err == nil {
		return p
	}
	for _, p := range brewPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Where finds the program's executable: on the PATH, in ~/.local/bin, or
// beside brew. It returns "" when it is not installed.
func (p Program) Where(home string) string {
	if path, err := exec.LookPath(p.Bin()); err == nil {
		return path
	}
	dirs := []string{LocalBin(home)}
	if b := Brew(); b != "" {
		dirs = append(dirs, filepath.Dir(b))
	}
	for _, d := range dirs {
		path := filepath.Join(d, p.Bin())
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

// Route is how a program would be installed on this system.
type Route struct {
	Brew string // the brew executable, when Homebrew installs it
	// Release is set when it comes from the project's GitHub release.
	Release bool
	// Why says why neither is possible, when Brew is empty and Release false.
	Why string
}

// Plan says how the program would be installed here: Homebrew when the
// system has it and the program is in homebrew/core, else the release, when
// one is built for this system.
func (p Program) Plan() Route {
	if b := Brew(); b != "" && p.Brew != "" {
		return Route{Brew: b}
	}
	if releaseBuilt(p, runtime.GOOS, runtime.GOARCH) {
		return Route{Release: true}
	}
	return Route{Why: fmt.Sprintf("%s publishes no build for %s/%s; install Homebrew, then try again", p.Name, runtime.GOOS, runtime.GOARCH)}
}

// releaseBuilt knows which programs ship no build for a system, so the
// list can say so without asking GitHub.
func releaseBuilt(p Program, goos, goarch string) bool {
	switch goos + "/" + goarch {
	case "darwin/arm64", "darwin/amd64", "linux/arm64", "linux/amd64":
	default:
		return false
	}
	// eza builds only for Linux; on macOS it comes from Homebrew.
	return !(p.Name == "eza" && goos == "darwin")
}

// InstallTimeout bounds an install: brew can take a while.
const InstallTimeout = 10 * time.Minute

// Install puts the program on the system along the route Plan gives, and
// returns where its executable is.
func Install(ctx context.Context, p Program, home string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, InstallTimeout)
	defer cancel()
	route := p.Plan()
	switch {
	case route.Brew != "":
		if err := brewInstall(ctx, route.Brew, p.Brew); err != nil {
			return "", err
		}
	case route.Release:
		if err := installRelease(ctx, p, LocalBin(home)); err != nil {
			return "", err
		}
	default:
		return "", errors.New(route.Why)
	}
	if path := p.Where(home); path != "" {
		return path, nil
	}
	return "", fmt.Errorf("%s was installed but %s cannot be found", p.Name, p.Bin())
}

// brewInstall runs brew install, without the update of every formula brew
// would do first, and returns brew's last words when it fails.
var brewInstall = func(ctx context.Context, brew, formula string) error {
	cmd := exec.CommandContext(ctx, brew, "install", formula)
	cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ENV_HINTS=1", "HOMEBREW_NO_INSTALL_CLEANUP=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("brew install %s failed: %s", formula, lastLine(out, err))
	}
	return nil
}

func lastLine(out []byte, err error) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}
	return err.Error()
}

// apiURL is GitHub's API, a variable so tests serve their own.
var apiURL = "https://api.github.com"

// maxProgram bounds a release download; the largest of the list is ~30 MB.
const maxProgram = 96 << 20

// Asset is one file of a release, as GitHub's API lists it.
type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"` // "sha256:<hex>", computed by GitHub on upload
}

// installRelease downloads the release file built for this system, checks
// it against the SHA-256 GitHub computed when it was uploaded, and puts its
// executables in dir.
func installRelease(ctx context.Context, p Program, dir string) error {
	var release struct {
		Tag    string  `json:"tag_name"`
		Assets []Asset `json:"assets"`
	}
	body, err := fetch(ctx, apiURL+"/repos/"+p.Repo+"/releases/latest", 4<<20)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return fmt.Errorf("GitHub's answer about %s could not be read: %w", p.Repo, err)
	}
	asset, err := PickAsset(release.Assets, runtime.GOOS, runtime.GOARCH, p.Skip)
	if err != nil {
		return fmt.Errorf("%s %s: %w", p.Name, release.Tag, err)
	}
	want, ok := strings.CutPrefix(asset.Digest, "sha256:")
	if !ok || len(want) != 64 {
		return fmt.Errorf("GitHub gives no SHA-256 for %s, so it is not installed", asset.Name)
	}
	data, err := fetch(ctx, asset.URL, maxProgram)
	if err != nil {
		return err
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != strings.ToLower(want) {
		return fmt.Errorf("%s does not match the SHA-256 GitHub gives for it, so it is not installed", asset.Name)
	}
	files, err := executablesIn(asset.Name, data, p.Bins)
	if err != nil {
		return err
	}
	for _, name := range p.Bins {
		if err := writeExecutable(filepath.Join(dir, name), files[name]); err != nil {
			return err
		}
	}
	return nil
}

// fetch downloads a URL whole, up to limit bytes.
func fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(url, apiURL) {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is larger than expected", url)
	}
	return data, nil
}

// PickAsset chooses, among a release's files, the program built for a
// system. Projects name their files each their own way: aarch64-apple-darwin,
// darwin_arm64, macos-aarch64, macos-universal, linux-amd64. On Linux a
// static musl build is preferred, since it runs on any distribution, and a
// tar.gz over a zip of the same build.
func PickAsset(assets []Asset, goos, goarch string, skip []string) (Asset, error) {
	osWords := map[string][]string{"darwin": {"darwin", "macos", "apple"}, "linux": {"linux"}}[goos]
	archWords := map[string][]string{"arm64": {"aarch64", "arm64"}, "amd64": {"x86_64", "amd64", "x86-64"}}[goarch]
	if osWords == nil || archWords == nil {
		return Asset{}, fmt.Errorf("no build for %s/%s", goos, goarch)
	}
	best, bestScore, tie := Asset{}, -1, false
	for _, a := range assets {
		name := strings.ToLower(a.Name)
		score, ok := assetScore(name)
		if !ok || !containsAny(name, osWords) || containsAny(name, skip) {
			continue
		}
		if !containsAny(name, archWords) && !(goos == "darwin" && strings.Contains(name, "universal")) {
			continue
		}
		if strings.Contains(name, "musl") {
			score += 2
		}
		switch {
		case score > bestScore:
			best, bestScore, tie = a, score, false
		case score == bestScore:
			tie = true
		}
	}
	if bestScore < 0 {
		return Asset{}, fmt.Errorf("no build for %s/%s in the release", goos, goarch)
	}
	if tie {
		return Asset{}, fmt.Errorf("several builds for %s/%s in the release, and nothing tells which one", goos, goarch)
	}
	return best, nil
}

// assetScore tells an archive or a bare executable from the checksums,
// signatures and packages beside it.
func assetScore(name string) (int, bool) {
	switch {
	case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".tgz"):
		return 1, true
	case strings.HasSuffix(name, ".zip"):
		return 0, true
	case !strings.Contains(name, "."):
		return 0, true // a bare executable, as kitten-darwin-arm64
	}
	return 0, false
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// executablesIn takes the named executables out of a downloaded file: a
// tar.gz, a zip, or the executable itself.
func executablesIn(asset string, data []byte, bins []string) (map[string][]byte, error) {
	out := map[string][]byte{}
	want := map[string]bool{}
	for _, b := range bins {
		want[b] = true
	}
	lower := strings.ToLower(asset)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s is not gzip: %w", asset, err)
		}
		tr := tar.NewReader(zr)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("%s is damaged: %w", asset, err)
			}
			name := filepath.Base(h.Name)
			if h.Typeflag == tar.TypeReg && want[name] && out[name] == nil {
				if out[name], err = io.ReadAll(io.LimitReader(tr, maxProgram)); err != nil {
					return nil, err
				}
			}
		}
	case strings.HasSuffix(lower, ".zip"):
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("%s is not a zip: %w", asset, err)
		}
		for _, f := range zr.File {
			name := filepath.Base(f.Name)
			if f.FileInfo().IsDir() || !want[name] || out[name] != nil {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			out[name], err = io.ReadAll(io.LimitReader(rc, maxProgram))
			rc.Close()
			if err != nil {
				return nil, err
			}
		}
	default:
		if len(bins) == 1 {
			out[bins[0]] = data
		}
	}
	for _, b := range bins {
		if len(out[b]) == 0 {
			return nil, fmt.Errorf("%s holds no %s", asset, b)
		}
	}
	return out, nil
}

// writeExecutable puts an executable in place in one rename, so a program
// that is running is never seen half written.
func writeExecutable(target string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".install-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}

// SelfSetup runs the program's own "install" or "uninstall" for a shell,
// which writes or removes its lines in the shell's file.
var SelfSetup = func(ctx context.Context, path, verb, shell string) error {
	out, err := exec.CommandContext(ctx, path, verb, shell).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s %s failed: %s", filepath.Base(path), verb, shell, lastLine(out, err))
	}
	return nil
}
