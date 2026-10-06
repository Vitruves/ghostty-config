// Package extensions fetches what other people have made for Ghostty and puts
// it where Ghostty looks for it. Ghostty has no plugins as such; what it can
// be extended with are files: GLSL shaders named by `custom-shader`, and
// themes dropped into the themes directory. This package knows a short list
// of each, where they live, and how to install them.
package extensions

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Shader is one GLSL shader of the list.
type Shader struct {
	Name    string // the file name without .glsl, which is also what is typed
	Group   string // the title it is listed under
	Note    string
	Repo    string // owner/name on GitHub
	Path    string // the file's path in the repository
	Licence string // empty when the repository states none
}

// Pack is a set of themes published by one project.
type Pack struct {
	Name    string
	Note    string
	Repo    string
	Licence string
	Files   []PackFile
}

// PackFile is one theme of a pack: where it is in the repository and the
// name it takes in the themes directory, which is the name of the theme.
type PackFile struct {
	Path, Theme string
}

const (
	cursorEffects = "Cursor effects"
	screenEffects = "Screen effects"

	krone = "KroneCorylus/ghostty-shader-playground"
	hckr  = "0xhckr/ghostty-shaders"
)

// Shaders is the list on offer: a few that follow the cursor and a few that
// cover the whole window. Kept short on purpose, like the list of fonts.
var Shaders = []Shader{
	{"cursor_smear", cursorEffects, "The cursor stretches towards where it jumps", krone, "public/shaders/cursor_smear.glsl", "MIT"},
	{"cursor_smear_fade", cursorEffects, "A smear that fades out along its length", krone, "public/shaders/cursor_smear_fade.glsl", "MIT"},
	{"cursor_blaze", cursorEffects, "A bright trail behind the cursor", krone, "public/shaders/cursor_blaze.glsl", "MIT"},
	{"bloom", screenEffects, "Soft glow around bright text", hckr, "bloom.glsl", ""},
	{"crt", screenEffects, "Curved glass and scanlines", hckr, "crt.glsl", ""},
	{"retro-terminal", screenEffects, "Scanlines and a phosphor tint", hckr, "retro-terminal.glsl", ""},
	{"starfield", screenEffects, "Stars drifting behind the text", hckr, "starfield.glsl", ""},
	{"underwater", screenEffects, "Slow ripples over the whole window", hckr, "underwater.glsl", ""},
	{"vhs", screenEffects, "Tape noise and colour bleed", hckr, "vhs.glsl", ""},
}

// Packs is the list of theme packs on offer.
var Packs = []Pack{
	{"catppuccin", "Latte, Frappé, Macchiato and Mocha", "catppuccin/ghostty", "MIT", []PackFile{
		{"themes/catppuccin-latte.conf", "catppuccin-latte"},
		{"themes/catppuccin-frappe.conf", "catppuccin-frappe"},
		{"themes/catppuccin-macchiato.conf", "catppuccin-macchiato"},
		{"themes/catppuccin-mocha.conf", "catppuccin-mocha"},
	}},
	{"rose-pine", "Rosé Pine, Moon and Dawn", "rose-pine/ghostty", "MIT", []PackFile{
		{"dist/rose-pine", "rose-pine"},
		{"dist/rose-pine-moon", "rose-pine-moon"},
		{"dist/rose-pine-dawn", "rose-pine-dawn"},
	}},
}

// maxSize bounds a download: a shader or a theme is a few kilobytes.
const maxSize = 1 << 20

// Get fetches a URL. It is a variable so tests never reach the network.
var Get = func(url string) ([]byte, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSize {
		return nil, fmt.Errorf("%s is larger than expected", url)
	}
	return data, nil
}

func rawURL(repo, path string) string {
	return "https://raw.githubusercontent.com/" + repo + "/main/" + path
}

// URL is where the shader is downloaded from.
func (s Shader) URL() string { return rawURL(s.Repo, s.Path) }

// File is where the shader is kept once installed: a shaders directory
// beside the config.
func (s Shader) File(configDir string) string {
	return filepath.Join(configDir, "shaders", s.Name+".glsl")
}

// Installed reports whether the shader's file is there.
func (s Shader) Installed(configDir string) bool {
	_, err := os.Stat(s.File(configDir))
	return err == nil
}

// FindShader looks a shader up by name.
func FindShader(name string) (Shader, bool) {
	for _, s := range Shaders {
		if s.Name == name {
			return s, true
		}
	}
	return Shader{}, false
}

// FindPack looks a pack up by name.
func FindPack(name string) (Pack, bool) {
	for _, p := range Packs {
		if p.Name == name {
			return p, true
		}
	}
	return Pack{}, false
}

// ErrNotAShader is returned when what was downloaded is not something
// Ghostty could run: it wants a mainImage function, as Shadertoy does.
var ErrNotAShader = errors.New("the file downloaded has no mainImage function, so it is not a shader Ghostty can run")

// InstallShader downloads the shader unless it is already there, and returns
// the path of its file.
func InstallShader(configDir string, s Shader) (string, error) {
	target := s.File(configDir)
	if s.Installed(configDir) {
		return target, nil
	}
	data, err := Get(s.URL())
	if err != nil {
		return "", err
	}
	if !strings.Contains(string(data), "mainImage") {
		return "", ErrNotAShader
	}
	if err := write(target, data); err != nil {
		return "", err
	}
	return target, nil
}

// RemoveShader deletes the shader's file; a file that is not there is fine.
func RemoveShader(configDir string, s Shader) error {
	err := os.Remove(s.File(configDir))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Installed reports how many of the pack's themes are in the directory.
func (p Pack) Installed(themesDir string) int {
	n := 0
	for _, f := range p.Files {
		if _, err := os.Stat(filepath.Join(themesDir, f.Theme)); err == nil {
			n++
		}
	}
	return n
}

// InstallPack downloads every theme of the pack into the themes directory.
// A theme already there under the same name is left as it is, so a file the
// user edited is never overwritten.
func InstallPack(themesDir string, p Pack) (written int, err error) {
	for _, f := range p.Files {
		target := filepath.Join(themesDir, f.Theme)
		if _, statErr := os.Stat(target); statErr == nil {
			continue
		}
		data, getErr := Get(rawURL(p.Repo, f.Path))
		if getErr != nil {
			return written, getErr
		}
		text := string(data)
		if !strings.Contains(text, "background") && !strings.Contains(text, "palette") {
			return written, fmt.Errorf("%s does not look like a theme", f.Path)
		}
		if err := write(target, data); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// write puts a file in place whole or not at all.
func write(target string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".download-*")
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
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}
