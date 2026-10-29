package tui

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg" // background images are PNG or JPEG
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	xdraw "golang.org/x/image/draw"
)

// Settings whose value is a file: the background image and the bell's
// sound. The list offers what a few likely places hold, found once and
// remembered; a path can be typed instead, `~` allowed, and is checked
// before it can be chosen. What is written is the full path, which Ghostty
// reads the same whatever directory it was started from.

// fileKind is one kind of file a setting takes.
type fileKind struct {
	cmd, key, label, detail string
	// none says what the setting does without a file.
	none string
	exts []string
	// magic tells a real file of the kind from one only named like it;
	// nil trusts the name.
	magic func(head []byte) bool
	// places are where to look, each with how deep.
	places func(m *Model) []place
}

type place struct {
	dir   string
	depth int
}

// fileCap bounds a scan: a list of thousands is no list, and a home
// directory can hold that many.
const fileCap = 400

var imageFiles = fileKind{
	cmd: "bgimage", key: "background-image", label: "Image",
	detail: "A PNG or JPEG drawn behind the text of every terminal; type a path to take one from anywhere",
	none:   "no image: the theme's background alone",
	exts:   []string{".png", ".jpg", ".jpeg"},
	magic: func(h []byte) bool {
		return bytes.HasPrefix(h, []byte("\x89PNG\r\n\x1a\n")) || bytes.HasPrefix(h, []byte{0xff, 0xd8, 0xff})
	},
	places: func(m *Model) []place {
		home := homeDir()
		return []place{{m.paths.ConfigDir, 2}, {filepath.Join(home, "Pictures"), 2}, {filepath.Join(home, "Desktop"), 1}, {filepath.Join(home, "Downloads"), 1}}
	},
}

var soundFiles = fileKind{
	cmd: "bellfile", key: "bell-audio-path", label: "Sound",
	detail: "The sound file the bell plays; type a path to take one from anywhere",
	none:   "no file: the custom sound stays silent",
	exts:   []string{".aiff", ".aif", ".wav", ".mp3", ".m4a", ".caf", ".ogg", ".oga", ".flac"},
	places: func(m *Model) []place {
		home := homeDir()
		if runtime.GOOS == "darwin" {
			return []place{{m.paths.ConfigDir, 2}, {filepath.Join(home, "Library", "Sounds"), 1}, {"/System/Library/Sounds", 1}}
		}
		return []place{{m.paths.ConfigDir, 2}, {filepath.Join(home, ".local", "share", "sounds"), 3}, {"/usr/share/sounds", 3}}
	},
}

// fileState is what the file settings remember: the files found, by
// setting, and the small copies of images drawn in the window picture.
type fileState struct {
	found  map[string][]string
	thumbs map[string]thumb
}

// fileSetting is a setting whose value is a file of kind k.
func (m *Model) fileSetting(k fileKind) setting {
	return setting{
		cmd: k.cmd, label: k.label, syntax: k.cmd + " none|<path>", detail: k.detail, key: k.key, fallback: "none",
		list: func(m *Model, arg string) []option { return m.fileOptions(k, arg) },
		get: func(m *Model) string {
			if v := m.tree.Value(k.key, ""); v != "" {
				return v
			}
			return "none"
		},
		set: func(m *Model, v string) tea.Cmd {
			if v == "none" {
				m.tree.Unset(k.key, "disabled by ghostty-config: no file")
			} else {
				m.tree.Set(k.key, v)
			}
			return nil
		},
		show: func(v string) string {
			if v == "none" {
				return v
			}
			return filepath.Base(v)
		},
		describe: func(m *Model, v string) string { return m.describeFile(k, v) },
	}
}

// expandPath reads a typed path: `~` is the home directory, and a relative
// path is taken from the config directory, as Ghostty takes it.
func (m *Model) expandPath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "\"")
	if p == "~" {
		return homeDir()
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(homeDir(), rest)
	}
	if !filepath.IsAbs(p) {
		return filepath.Join(m.paths.ConfigDir, p)
	}
	return filepath.Clean(p)
}

// filePath is where the file a key names is: a relative path is taken from
// the file that set it.
func (m *Model) filePath(key, v string) string {
	if v == "" || v == "none" {
		return ""
	}
	if rest, ok := strings.CutPrefix(v, "~/"); ok {
		return filepath.Join(homeDir(), rest)
	}
	if !filepath.IsAbs(v) {
		if e, ok := m.tree.Get(key); ok {
			return filepath.Join(filepath.Dir(e.Doc.Path), v)
		}
		return filepath.Join(m.paths.ConfigDir, v)
	}
	return v
}

// looksLikePath is whether what was typed is a path rather than a filter.
func looksLikePath(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../")
}

// checkFile says why a path cannot be used, or nothing when it can.
func checkFile(k fileKind, p string) string {
	st, err := os.Stat(p)
	switch {
	case err != nil:
		return "no such file"
	case st.IsDir():
		return "a directory, not a file"
	}
	ext := strings.ToLower(filepath.Ext(p))
	known := false
	for _, e := range k.exts {
		known = known || e == ext
	}
	if !known {
		return "not one of " + strings.Join(k.exts, " ")
	}
	if k.magic != nil {
		f, err := os.Open(p)
		if err != nil {
			return "cannot be read"
		}
		defer f.Close()
		head := make([]byte, 8)
		n, _ := f.Read(head)
		if !k.magic(head[:n]) {
			return "named " + ext + " but not one inside"
		}
	}
	return ""
}

// filesOf finds the files of a kind in its places, once.
func (m *Model) filesOf(k fileKind) []string {
	if files, ok := m.files.found[k.cmd]; ok {
		return files
	}
	var out []string
	seen := map[string]bool{}
	for _, pl := range k.places(m) {
		if pl.dir == "" {
			continue
		}
		var found []string
		scanDir(k, pl.dir, pl.depth, &found)
		sort.Strings(found)
		for _, f := range found {
			if !seen[f] && len(out) < fileCap {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	if m.files.found == nil {
		m.files.found = map[string][]string{}
	}
	m.files.found[k.cmd] = out
	return out
}

func scanDir(k fileKind, dir string, depth int, out *[]string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if len(*out) >= fileCap {
			return
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		p := filepath.Join(dir, name)
		if e.IsDir() {
			// A photo library or an app is a world of its own.
			if depth > 1 && filepath.Ext(name) == "" {
				scanDir(k, p, depth-1, out)
			}
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		for _, x := range k.exts {
			// Only a file that is what its name says is offered.
			if x == ext && (k.magic == nil || checkFile(k, p) == "") {
				*out = append(*out, p)
				break
			}
		}
	}
}

// fileOptions lists the values of a file setting: none, a typed path when
// one is typed, the file in effect, then what the places hold.
func (m *Model) fileOptions(k fileKind, arg string) []option {
	now := m.filePath(k.key, m.tree.Value(k.key, ""))
	needle := strings.TrimSpace(arg)
	out := []option{}
	if needle == "" || fuzzyMatch("none", needle) {
		out = append(out, option{label: "none", value: "none", current: now == ""})
	}
	if looksLikePath(needle) {
		p := m.expandPath(needle)
		if why := checkFile(k, p); why != "" {
			out = append(out, option{label: shortenPath(p), detail: why, header: true})
		} else {
			out = append(out, option{label: filepath.Base(p), detail: shortenPath(filepath.Dir(p)), value: p, current: p == now})
		}
		return out
	}
	files := m.filesOf(k)
	if now != "" && !containsString(files, now) {
		files = append([]string{now}, files...)
	}
	n := 0
	for _, f := range files {
		if needle != "" && !fuzzyMatch(filepath.Base(f), needle) {
			continue
		}
		out = append(out, option{label: filepath.Base(f), detail: shortenPath(filepath.Dir(f)), value: f, current: f == now})
		n++
	}
	if n == 0 && needle == "" {
		var where []string
		for _, pl := range k.places(m) {
			where = append(where, shortenPath(pl.dir))
		}
		out = append(out, option{label: "Nothing in " + strings.Join(where, ", "), detail: "type a path, ~ allowed", header: true})
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

// describeFile says what the highlighted file is.
func (m *Model) describeFile(k fileKind, v string) string {
	if v == "" || v == "none" {
		return k.none
	}
	p := m.filePath(k.key, v)
	st, err := os.Stat(p)
	if err != nil {
		return shortenPath(p) + " is not there: Ghostty will skip it"
	}
	kind := strings.ToUpper(strings.TrimPrefix(filepath.Ext(p), "."))
	if kind == "JPG" {
		kind = "JPEG"
	}
	what := fmt.Sprintf("%s, %s", kind, byteSize(st.Size()))
	if t, ok := m.files.thumbs[p]; ok && t.img != nil {
		what = fmt.Sprintf("%d × %d %s", t.w, t.h, what)
	}
	return what + ", in " + shortenPath(filepath.Dir(p))
}

func byteSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}

// --- thumbnails -------------------------------------------------------------

// thumb is a small copy of an image file, for the window picture: nil with
// no error while it is read.
type thumb struct {
	img  *image.NRGBA
	w, h int // the size of the file itself
	err  error
}

type thumbMsg struct{ thumbs map[string]thumb }

// Thumbnails are this many pixels on their longer side: enough for the
// window picture on a dense display, little enough to keep a dozen.
const (
	thumbSide = 720
	thumbCap  = 12
	// thumbPixels is the largest image read at all: a picture of hundreds
	// of megapixels would take seconds and gigabytes to decode.
	thumbPixels = 120_000_000
)

// thumbOf is the small copy of an image, nil while it is read or when it
// cannot be.
func (m *Model) thumbOf(path string) *thumb {
	if t, ok := m.files.thumbs[path]; ok && t.img != nil {
		return &t
	}
	return nil
}

// wantThumbs starts reading the images the window picture is about to
// show: the one in effect while the Window section is on screen, and the
// highlighted one while the images are listed. Moving the highlight never
// waits for one; the picture shows it once it is read.
func (m *Model) wantThumbs() tea.Cmd {
	if !m.pixelOn() && !m.imagesOn() {
		return nil
	}
	var need []string
	want := func(p string) {
		if p == "" || containsString(need, p) {
			return
		}
		if _, seen := m.files.thumbs[p]; !seen {
			need = append(need, p)
		}
	}
	if groups[m.group] == "Window" {
		want(m.filePath(imageFiles.key, m.tree.Value(imageFiles.key, "")))
	}
	if m.cmd != nil && m.cmd.name == imageFiles.cmd {
		if o := m.selected(); o != nil && o.value != "none" {
			want(o.value)
		}
	}
	if len(need) == 0 {
		return nil
	}
	if m.files.thumbs == nil || len(m.files.thumbs)+len(need) > thumbCap {
		m.files.thumbs = map[string]thumb{}
	}
	for _, p := range need {
		m.files.thumbs[p] = thumb{}
	}
	return func() tea.Msg {
		out := map[string]thumb{}
		for _, p := range need {
			out[p] = readThumb(p)
		}
		return thumbMsg{out}
	}
}

// readThumb decodes an image and scales it down.
func readThumb(path string) thumb {
	f, err := os.Open(path)
	if err != nil {
		return thumb{err: err}
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return thumb{err: err}
	}
	if cfg.Width*cfg.Height > thumbPixels {
		return thumb{w: cfg.Width, h: cfg.Height, err: fmt.Errorf("too large to preview")}
	}
	if _, err := f.Seek(0, 0); err != nil {
		return thumb{err: err}
	}
	src, _, err := image.Decode(f)
	if err != nil {
		return thumb{err: err}
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	tw, th := w, h
	if w > thumbSide || h > thumbSide {
		if w >= h {
			tw, th = thumbSide, max(1, h*thumbSide/w)
		} else {
			tw, th = max(1, w*thumbSide/h), thumbSide
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, tw, th))
	if tw == w && th == h {
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	} else {
		xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	}
	return thumb{img: dst, w: w, h: h}
}
