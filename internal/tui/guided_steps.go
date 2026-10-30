package tui

import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The questions of each step, in the words a newcomer would use, each answer
// mapped onto the settings the other profiles list. Where a setting explains
// its values, the answer says it in those words.

// --- Colours -------------------------------------------------------------------

// Themes carry no word for how warm they are: it is read from their colours.
// A ground and a text that lean to red and yellow are warm, to blue and green
// cool; the third of themes that lean least either way are neutral.

// themeWarmth is how far a theme leans to red over blue, -1 to 1; the ground
// counts most, being most of what is seen.
func themeWarmth(t *ghostty.Theme) float64 {
	lean := func(hex string) float64 {
		rgb, err := color.ParseHex(hex)
		if err != nil {
			return 0
		}
		return (float64(rgb.R) - float64(rgb.B)) / 255
	}
	return 0.6*lean(t.Background()) + 0.4*lean(t.Foreground())
}

// themeTemps sorts the themes of one tone by warmth, a third each way. The
// sort is kept until the library changes: every frame asks for it.
func (g *guided) themeTemps(m *Model, dark bool) map[string][]*ghostty.Theme {
	key := fmt.Sprintf("%v|%d", dark, len(m.lib.Themes))
	if len(m.lib.Themes) > 0 {
		key += "|" + m.lib.Themes[0].Name + "|" + m.lib.Themes[len(m.lib.Themes)-1].Name
	}
	if got, ok := g.temps[key]; ok {
		return got
	}
	if g.temps == nil || len(g.temps) > 4 {
		g.temps = map[string]map[string][]*ghostty.Theme{}
	}
	out := sortTemps(m, dark)
	g.temps[key] = out
	return out
}

func sortTemps(m *Model, dark bool) map[string][]*ghostty.Theme {
	var ts []*ghostty.Theme
	for _, t := range m.lib.Themes {
		if t.IsDark() == dark && t.Source != ghostty.SourceDraft {
			ts = append(ts, t)
		}
	}
	w := make(map[string]float64, len(ts))
	for _, t := range ts {
		w[t.Name] = themeWarmth(t)
	}
	sort.SliceStable(ts, func(a, b int) bool { return w[ts[a].Name] < w[ts[b].Name] })
	n := len(ts)
	out := map[string][]*ghostty.Theme{
		"cool":    append([]*ghostty.Theme(nil), ts[:n/3]...),
		"neutral": append([]*ghostty.Theme(nil), ts[n/3:n-n/3]...),
		"warm":    append([]*ghostty.Theme(nil), ts[n-n/3:]...),
	}
	// Within a kind, the ones that show it most come first, and among
	// those the ones that read best.
	strength := func(kind string, t *ghostty.Theme) float64 {
		switch kind {
		case "cool":
			return -w[t.Name]
		case "warm":
			return w[t.Name]
		}
		v := w[t.Name]
		if v < 0 {
			v = -v
		}
		return -v
	}
	for kind, list := range out {
		kind := kind
		good := map[string]bool{}
		for _, t := range list {
			good[t.Name] = readsWell(t)
		}
		sort.SliceStable(list, func(a, b int) bool {
			ga, gb := good[list[a].Name], good[list[b].Name]
			if ga != gb {
				return ga
			}
			return strength(kind, list[a]) > strength(kind, list[b])
		})
	}
	return out
}

// readsWell is whether a theme's text clears AAA on its ground.
func readsWell(t *ghostty.Theme) bool {
	return color.Contrast(t.Foreground(), t.Background()) >= 7
}

// themeTemp is the kind a theme is among those of its tone.
func (g *guided) themeTemp(m *Model, t *ghostty.Theme) string {
	if t == nil {
		return "neutral"
	}
	for kind, list := range g.themeTemps(m, t.IsDark()) {
		for _, x := range list {
			if x.Name == t.Name {
				return kind
			}
		}
	}
	return "neutral"
}

// appliedTheme is the theme the config names, or the one shown.
func (m *Model) appliedTheme() *ghostty.Theme {
	if t, ok := m.lib.Get(m.applied); ok {
		return t
	}
	return m.cur
}

// themeMeta is the line under a theme card: where it comes from and how
// well it reads.
func themeMeta(t *ghostty.Theme) string {
	fam := themeFamily(t)
	switch fam {
	case "bundled":
		fam = "Ghostty"
	case "yours", "installed", "collection":
		fam = title(fam)
	}
	return fam + " · " + color.Grade(color.Contrast(t.Foreground(), t.Background()))
}

func (g *guided) colourPages(m *Model) []gPage {
	now := m.appliedTheme()
	nowDark := now == nil || now.IsDark()
	tone := gQuestion{id: "tone", title: "Light or dark?", kind: gRadio, doc: "theme", filter: true, preview: "themes"}
	tone.choices = []gChoice{
		{label: "Dark", desc: "Light text on a dark ground: easy on the eyes, at night above all.", value: "dark", is: func(*Model) bool { return nowDark }},
		{label: "Light", desc: "Dark text on a light ground, like a page: best in a bright room.", value: "light", is: func(*Model) bool { return !nowDark }},
	}
	dark := tone.choices[g.sel(m, tone)].value == "dark"

	nowTemp := g.themeTemp(m, now)
	temp := gQuestion{id: "temp", title: "Warm, cool or neutral?", kind: gRadio, doc: "theme", filter: true, preview: "themes"}
	for _, k := range [][3]string{
		{"warm", "Warm", "Reds, ambers and browns lean into the ground: a lamp-lit room."},
		{"cool", "Cool", "Blues and greens: a clear sky, a calm sea."},
		{"neutral", "Neutral", "Greys with hardly a tint: nothing but the text."},
	} {
		k := k
		temp.choices = append(temp.choices, gChoice{label: k[1], desc: k[2], value: k[0], is: func(*Model) bool { return nowTemp == k[0] }})
	}
	kind := temp.choices[g.sel(m, temp)].value

	cards := gQuestion{id: "theme", title: "Which one feels right?", kind: gCards, doc: "theme", preview: "themes"}
	for _, t := range g.offered(m, dark, kind) {
		t := t
		cards.choices = append(cards.choices, gChoice{label: t.Name, desc: themeMeta(t), theme: t,
			writes: []gWrite{{cmd: "theme", opt: option{value: t.Name, theme: t}}}})
	}
	toneWord := map[bool]string{true: "dark", false: "light"}[dark]
	return []gPage{
		{qs: []gQuestion{tone}},
		{eyebrow: "you chose " + toneWord, qs: []gQuestion{temp}},
		{eyebrow: "you chose " + toneWord + ", " + kind, qs: []gQuestion{cards}},
	}
}

// offered are the three themes the cards show: the next three of the kind
// chosen, starting over once every one was shown. The theme applied comes
// first when it is of the kind.
func (g *guided) offered(m *Model, dark bool, kind string) []*ghostty.Theme {
	list := g.themeTemps(m, dark)[kind]
	if now := m.appliedTheme(); now != nil && g.more == 0 {
		for i, t := range list {
			if t.Name == now.Name {
				list = append([]*ghostty.Theme{t}, append(append([]*ghostty.Theme(nil), list[:i]...), list[i+1:]...)...)
				break
			}
		}
	}
	if len(list) == 0 {
		return nil
	}
	if len(list) <= 3 {
		return list
	}
	start := (g.more * 3) % len(list)
	var out []*ghostty.Theme
	for i := 0; i < 3; i++ {
		out = append(out, list[(start+i)%len(list)])
	}
	return out
}

// --- Font ----------------------------------------------------------------------

// guideFamilies are the families the font question offers: the one in use,
// then those most people know, then any other monospace one, five at most.
var guideFamilies = []string{"JetBrains Mono", "SF Mono", "Menlo", "Fira Code", "Cascadia Code", "Hack", "Iosevka", "Source Code Pro", "IBM Plex Mono", "Ubuntu Mono", "DejaVu Sans Mono", "Monaco"}

func (m *Model) guideFamilies() []*ghostty.Family {
	var out []*ghostty.Family
	seen := map[string]bool{}
	add := func(f *ghostty.Family) {
		if f != nil && f.Mono && !seen[f.Name] && len(out) < 5 {
			seen[f.Name] = true
			out = append(out, f)
		}
	}
	find := func(name string) *ghostty.Family {
		for i := range m.families {
			if strings.EqualFold(m.families[i].Name, name) {
				return &m.families[i]
			}
		}
		return nil
	}
	add(find(m.fontFamily))
	for _, n := range guideFamilies {
		add(find(n))
	}
	for i := range m.families {
		add(&m.families[i])
	}
	return out
}

// guideSizes are the sizes the guide offers, around Ghostty's own 13.
var guideSizes = [][3]string{
	{"12", "Smaller", "More lines and columns on the screen."},
	{"13", "Normal", "Ghostty's own size."},
	{"15", "Larger", "Easier to read, from further away."},
	{"17", "Much larger", "For a large screen, or tired eyes."},
}

func (g *guided) fontPages(m *Model) []gPage {
	face := gQuestion{id: "font", title: "Which typeface?", kind: gFonts, doc: "font-family", preview: "font"}
	if m.fontFamily == "" {
		face.choices = append(face.choices, gChoice{label: ghosttyBuiltin, desc: "Built into Ghostty: what it uses when you choose none.",
			is: func(m *Model) bool { return m.fontFamily == "" }})
	}
	for _, f := range m.guideFamilies() {
		f := f
		desc := "Monospace"
		if len(f.Styles) > 1 {
			desc += fmt.Sprintf(", %d styles", len(f.Styles))
		}
		face.choices = append(face.choices, gChoice{label: f.Name, desc: desc + ".", family: f,
			writes: []gWrite{{cmd: "font", opt: option{value: f.Name, family: f}}}})
	}
	if !m.fontsLoaded {
		face.title = "Which typeface?"
		face.choices = append(face.choices, gChoice{label: "Listing your fonts…", desc: "A moment: the fonts Ghostty can load are being read."})
	}

	size := gQuestion{id: "size", title: "How large?", kind: gRadio, doc: "font-size", preview: "font"}
	for _, s := range guideSizes {
		size.choices = append(size.choices, gChoice{label: s[1], desc: s[0] + " points. " + s[2], writes: []gWrite{{cmd: "size", opt: option{value: s[0]}}}})
	}
	lig := m.onOffQuestion("ligatures", "Join symbols like -> and != into one?", "ligatures", gPills,
		[2]string{"Join them", "Ligatures, in fonts that have them."}, [2]string{"Keep them apart", "Every character on its own."})
	lig.preview = "font"
	return []gPage{{qs: []gQuestion{face}}, {qs: []gQuestion{size, lig}}}
}

// --- Window --------------------------------------------------------------------

// glassBlur is the blur the frosted window asks for: Liquid Glass where the
// blur setting offers it, a plain blur elsewhere.
func (m *Model) glassBlur() string {
	if c := m.commandNamed("blur"); c != nil && c.set != nil {
		for _, o := range c.set.options {
			if o == "macos-glass-regular" {
				return o
			}
		}
	}
	return "20"
}

func (g *guided) windowPages(m *Model) []gPage {
	opacity := func(m *Model) float64 {
		v, err := strconv.ParseFloat(m.settingNow("opacity"), 64)
		if err != nil {
			return 1
		}
		return v
	}
	blurred := func(m *Model) bool { b := m.settingNow("blur"); return b != "off" && b != "" }
	glass := m.glassBlur()
	frostDesc := "Through, but blurred, like frosted glass."
	if strings.HasPrefix(glass, "macos-glass") {
		frostDesc = "Through, but blurred: macOS Liquid Glass."
	}
	look := gQuestion{id: "look", title: "See through the window?", kind: gRadio, doc: "background-opacity", preview: "window"}
	look.choices = []gChoice{
		{label: "Solid", desc: "The theme's colour, and nothing behind it shows.",
			writes: []gWrite{setWrite("opacity", "1.00"), setWrite("blur", "off")},
			is:     func(m *Model) bool { return opacity(m) >= 1 }},
		{label: "Translucent", desc: "The desktop shows through, a little.",
			writes: []gWrite{setWrite("opacity", "0.85"), setWrite("blur", "off")},
			is:     func(m *Model) bool { return opacity(m) < 1 && !blurred(m) }},
		{label: "Frosted glass", desc: frostDesc,
			writes: []gWrite{setWrite("opacity", "0.85"), setWrite("blur", glass)},
			is:     func(m *Model) bool { return opacity(m) < 1 && blurred(m) }},
	}
	room := gQuestion{id: "room", title: "Room around the text", kind: gPills, doc: "window-padding-x", preview: "window"}
	for _, r := range [][3]string{{"2", "Snug", "Ghostty's own margin."}, {"12", "Comfortable", "A margin the text can breathe in."}, {"24", "Airy", "Wide margins all round."}} {
		room.choices = append(room.choices, gChoice{label: r[1], desc: r[2], writes: []gWrite{setWrite("paddingx", r[0]), setWrite("paddingy", r[0])}})
	}
	pages := []gPage{{qs: []gQuestion{look, room}}}
	if runtime.GOOS == "darwin" && m.commandNamed("titlebar") != nil {
		bar := gQuestion{id: "titlebar", title: "The bar at the top", kind: gRadio, doc: "macos-titlebar-style", preview: "window"}
		for _, v := range [][2]string{{"transparent", "In the theme's colour"}, {"native", "The system's own"}, {"tabs", "Tabs in the bar"}, {"hidden", "No bar"}} {
			bar.choices = append(bar.choices, m.choice("titlebar", v[0], v[1], ""))
		}
		pages = append(pages, gPage{eyebrow: "new windows only", qs: []gQuestion{bar}})
	}
	return pages
}

// --- Cursor --------------------------------------------------------------------

func (g *guided) cursorPages(m *Model) []gPage {
	shape := gQuestion{id: "cursor", title: "What shape of cursor?", kind: gRadio, doc: "cursor-style", preview: "cursor"}
	for _, v := range [][2]string{{"block", "Block"}, {"bar", "Bar"}, {"underline", "Underline"}} {
		shape.choices = append(shape.choices, m.choice("cursor", v[0], v[1], ""))
	}
	blink := gQuestion{id: "blink", title: "Should it blink?", kind: gPills, doc: "cursor-style-blink", preview: "cursor"}
	blink.choices = []gChoice{
		m.choice("blink", "true", "Blink", ""),
		m.choice("blink", "false", "Stay still", ""),
		m.choice("blink", "default", "Let programs say", "The shell and the program running decide."),
	}
	return []gPage{{eyebrow: "your shell may still ask for a bar at the prompt", qs: []gQuestion{shape, blink}}}
}

// --- Copy & paste ------------------------------------------------------------------

// copyWords name copy-on-select's values for a newcomer, by what they do.
var copyWords = map[string]string{
	"none": "Only when I copy", "false": "Only when I copy",
	"clipboard": "Selecting copies", "true": "Selecting copies",
	"primary": "To the selection clipboard", "both": "To both clipboards",
}

func (g *guided) copyPages(m *Model) []gPage {
	cp := gQuestion{id: "copy", title: "Copy text as you select it?", kind: gRadio, doc: "copy-on-select", preview: "copy"}
	if c := m.commandNamed("copy"); c != nil && c.set != nil {
		for _, v := range c.set.options {
			label := copyWords[v]
			if label == "" {
				label = title(v)
			}
			if runtime.GOOS != "darwin" && v == "clipboard" && len(c.set.options) == 4 {
				label = "To the system clipboard"
			}
			ch := m.choice("copy", v, label, "")
			if v == "none" || v == "false" {
				ch.desc = "Nothing is copied until you press the copy key."
			}
			cp.choices = append(cp.choices, ch)
		}
	}
	trim := m.onOffQuestion("trim", "Trim spaces at the ends of copied lines?", "trimspaces", gPills,
		[2]string{"Trim them", "Copied lines end where their text does."}, [2]string{"Keep them", "Copied lines keep every space."})
	trim.preview = "copy"

	paste := m.onOffQuestion("paste", "Ask before a paste that would run a command?", "pasteguard", gRadio,
		[2]string{"Ask me first", "Text with a line break could run as soon as it is pasted: Ghostty shows it first."},
		[2]string{"Paste anything", "Pasting never asks. Only for those who know what they paste."})
	paste.preview = "paste"

	read := gQuestion{id: "clipread", title: "May programs read your clipboard?", kind: gRadio, doc: "clipboard-read", preview: "paste"}
	for _, v := range [][2]string{{"ask", "Ask me each time"}, {"allow", "Always"}, {"deny", "Never"}} {
		read.choices = append(read.choices, m.choice("clipread", v[0], v[1], ""))
	}
	write := gQuestion{id: "clipwrite", title: "And set it, as vim and tmux do?", kind: gPills, doc: "clipboard-write", preview: "paste"}
	for _, v := range [][2]string{{"allow", "Always"}, {"ask", "Ask me"}, {"deny", "Never"}} {
		write.choices = append(write.choices, m.choice("clipwrite", v[0], v[1], ""))
	}
	return []gPage{
		{qs: []gQuestion{cp, trim}},
		{qs: []gQuestion{paste}},
		{eyebrow: "programs over ssh too", qs: []gQuestion{read, write}},
	}
}

// --- Done ----------------------------------------------------------------------

// sessionChanges are the settings this session changed, as lines a person
// reads: what each is set to now, or that it went back to its default.
func (m *Model) sessionChanges() []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range m.changedFiles() {
		added := map[string]bool{}
		for _, d := range f.diff {
			if d.Kind != ghostty.DiffAdd {
				continue
			}
			key, value, ok := configLine(d.Text)
			if !ok {
				continue
			}
			added[key] = true
			if line := key + " = " + value; !seen[line] {
				seen[line] = true
				out = append(out, line)
			}
		}
		for _, d := range f.diff {
			if d.Kind != ghostty.DiffDel {
				continue
			}
			if key, _, ok := configLine(d.Text); ok && !added[key] && !seen[key] {
				seen[key] = true
				out = append(out, key+": back to Ghostty's default")
			}
		}
	}
	return out
}

// configLine reads a line of a config file as a key and its value; comments
// and blank lines are not settings.
func configLine(s string) (string, string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "#") {
		return "", "", false
	}
	k, v, ok := strings.Cut(s, "=")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(k), strings.TrimSpace(v), true
}

func (g *guided) donePages(m *Model) []gPage {
	next := gQuestion{id: "done", title: "That's it. What now?", kind: gAction}
	switchTo := func(name string) func(m *Model, g *guided) tea.Cmd {
		return func(m *Model, g *guided) tea.Cmd { return m.switchProfile(name) }
	}
	next.choices = []gChoice{
		{label: "Fine-tune everything", desc: "Atelier: every setting in its section, and a wall of themes to browse.", act: switchTo(ghostty.ProfileAtelier)},
		{label: "Search for anything", desc: "Palette: one search line over every theme, setting and command.", act: switchTo(ghostty.ProfilePalette)},
	}
	if len(m.changedFiles()) > 0 && m.caps.inGhostty && !m.opts.NoReload && !m.autoReloadWorks() {
		next.choices = append(next.choices, gChoice{label: "Reload Ghostty", desc: "Ghostty is not reloading by itself: ask it to read your config now.",
			act: func(m *Model, g *guided) tea.Cmd { return m.runCommand(m.commandNamed("reload"), "", nil) }})
	}
	next.choices = append(next.choices,
		gChoice{label: "Start again", desc: "Back to the first step; what you chose stays written.", act: func(m *Model, g *guided) tea.Cmd { return g.startAgain(m) }},
		gChoice{label: "Leave", desc: "Close ghostty-config. Everything you chose is written.", act: func(m *Model, g *guided) tea.Cmd { _, c := m.quit(); return c }},
	)
	return []gPage{{qs: []gQuestion{next}}}
}

// doneNote is what Done says about reloading.
func (m *Model) doneNote() string {
	if len(m.changedFiles()) == 0 {
		return ""
	}
	if m.autoReloadWorks() {
		return "Ghostty has reloaded: open windows show it now. Some changes only reach new windows."
	}
	return "To see it, " + m.reloadHint() + "."
}
