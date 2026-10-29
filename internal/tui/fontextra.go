package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/fontdl"
	"github.com/vitruves/ghostty-config/internal/fontinfo"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The font's own options: the OpenType features it can switch on, the axes
// of a variable font, and where the Nerd Font icons come from. Features and
// axes are read from the file of the family and style in use, so a font
// with no stylistic sets offers none and a static font no axes; they are
// read in the background, once per file, and the Fonts section gains its
// rows when they arrive. The commands are built again then, since each
// feature and each axis is a setting of its own.

// fontExtras is what the Fonts section knows about the font in use.
type fontExtras struct {
	infos map[string]fontInfo
	// built is the file the commands were last built for.
	built string
	// afterInstall runs once a download finishes well: the icons mapped to
	// the family that was just installed.
	afterInstall func(*Model) tea.Cmd
	// afterFonts runs once the families are listed again, after a download:
	// the configured family found under the name it came back with.
	afterFonts func(*Model) tea.Cmd
	// hinted is set once the status line has said what is wrong with the
	// configured family, or found nothing to say.
	hinted bool
}

// fontInfo is one file read: nil with no error while it is read.
type fontInfo struct {
	info *fontinfo.Info
	err  error
}

type fontInfoMsg struct {
	key  string
	info fontInfo
}

// fontInfoCap is how many files are remembered.
const fontInfoCap = 32

// currentFontFile is the file of the family and style in use.
func (m *Model) currentFontFile() (ghostty.FontFile, bool) {
	f, ok := m.findFamily(m.fontFamily)
	if !ok {
		return ghostty.FontFile{}, false
	}
	file, ok := f.File(m.currentStyle())
	return file, ok && file.Path != ""
}

// currentFontInfo is what the font in use offers, nil until it is read.
func (m *Model) currentFontInfo() (*fontinfo.Info, string) {
	file, ok := m.currentFontFile()
	if !ok {
		return nil, ""
	}
	key := fileKey(file)
	return m.fx.infos[key].info, key
}

// wantFontInfo starts reading the font in use while the Fonts section is on
// screen, listing the families first if nobody has yet; and in the font
// list, the highlighted family, whose panel says whether it has icons.
func (m *Model) wantFontInfo() tea.Cmd {
	if groups[m.group] != "Fonts" {
		return nil
	}
	if !m.fontsLoaded {
		return m.ensureFonts()
	}
	var files []ghostty.FontFile
	if file, ok := m.currentFontFile(); ok {
		files = append(files, file)
	}
	if o := m.selected(); m.cmd != nil && m.cmd.name == "font" && o != nil && o.family != nil && o.family.Mono {
		if _, known := m.nerdIconsOf(*o.family); !known {
			if file, ok := o.family.File(m.currentStyle()); ok && file.Path != "" {
				files = append(files, file)
			}
		}
	}
	var cmds []tea.Cmd
	for _, file := range files {
		key := fileKey(file)
		if _, seen := m.fx.infos[key]; seen {
			continue
		}
		if m.fx.infos == nil || len(m.fx.infos) >= fontInfoCap {
			m.fx.infos = map[string]fontInfo{}
		}
		m.fx.infos[key] = fontInfo{}
		cmds = append(cmds, func() tea.Msg {
			info, err := fontinfo.Read(file.Path, file.PostScript, file.Index)
			return fontInfoMsg{key, fontInfo{info, err}}
		})
	}
	return tea.Batch(cmds...)
}

// syncFontCommands builds the commands again when the font they were built
// for is no longer the one in use, or has just been read, or when what is
// wrong with it changed.
func (m *Model) syncFontCommands() {
	info, key := m.currentFontInfo()
	if info == nil {
		key = ""
	}
	if trouble := m.fontTrouble().key(); trouble != "" {
		key += "\x00" + trouble
	}
	if key == m.fx.built {
		return
	}
	m.fx.built = key
	if m.cmds != nil {
		m.cmds = nil
		m.recompute()
	}
}

// fontExtraSettings are the features and axes of the font in use, and the
// icons.
func (m *Model) fontExtraSettings() []setting {
	var s []setting
	add := func(st setting) {
		st.group = "Fonts"
		s = append(s, st)
	}
	info, _ := m.currentFontInfo()
	if info != nil {
		family := orDash(m.fontFamily)
		for _, f := range info.Features {
			if fontinfo.Optional(f.Tag) {
				add(featureSetting(f, family))
			}
		}
		for _, a := range info.Axes {
			add(axisSetting(a, family))
		}
	}
	add(m.iconSetting())
	return s
}

// featureSetting is one optional feature of the font, as a switch.
func featureSetting(f fontinfo.Feature, family string) setting {
	tag := f.Tag
	note := f.Note()
	return setting{
		cmd: tag, part: "Features", label: f.Label(), syntax: tag + " on|off",
		detail:  strings.ToUpper(note[:1]) + note[1:],
		options: []string{"on", "off"}, key: "font-feature", fallback: "off",
		hint: fmt.Sprintf("Read from %s's own file. Ghostty applies a feature to every font it draws, fallbacks included.", family),
		get: func(m *Model) string {
			if m.tree.FontFeatureOn(tag) {
				return "on"
			}
			return "off"
		},
		set: func(m *Model, v string) tea.Cmd {
			m.tree.SetFontFeature(tag, v == "on")
			return nil
		},
		writes: func(m *Model, v string) string {
			if v == "on" {
				return "font-feature = " + tag
			}
			return ""
		},
	}
}

// axisNames are the command words of the registered axes; any other is
// axis and its tag.
var axisWords = map[string]string{"wght": "weight", "wdth": "width", "slnt": "slant", "ital": "italic", "opsz": "opticalsize", "GRAD": "grade"}

// axisSetting is one axis of a variable font, as a slider from its minimum
// to its maximum, with default leaving it to the font.
func axisSetting(a fontinfo.Axis, family string) setting {
	step := niceStep((a.Max - a.Min) / 20)
	lo, hi := math.Ceil(a.Min/step)*step, math.Floor(a.Max/step)*step
	if hi < lo {
		lo, hi = a.Min, a.Max
	}
	decimals := 0
	for d := step; d < 1 && decimals < 3; d *= 10 {
		decimals++
	}
	format := func(v float64) string { return strconv.FormatFloat(v, 'f', decimals, 64) }
	word := axisWords[a.Tag]
	if word == "" {
		word = "axis" + strings.ToLower(a.Tag)
	}
	tag := a.Tag
	const key = "font-variation"
	def := strconv.FormatFloat(a.Default, 'f', -1, 64)
	return setting{
		cmd: word, part: "Variable axes", label: a.Label(), key: key,
		syntax:  fmt.Sprintf("%s default|<%s–%s>", word, format(lo), format(hi)),
		detail:  fmt.Sprintf("The %s axis of %s, from %s to %s, %s by default", tag, family, format(lo), format(hi), def),
		numeric: true, min: lo, max: hi, step: step, format: format,
		unset: "default", unsetAt: a.Default, fallback: "default",
		explain: map[string]string{"default": "the font's own " + strings.ToLower(a.Label()) + ", " + def},
		hint:    "Set for the regular face; bold and italic keep their own.",
		get: func(m *Model) string {
			if v, ok := m.tree.FontVariation(key, tag); ok {
				return v
			}
			return "default"
		},
		set: func(m *Model, v string) tea.Cmd {
			if v == "default" {
				v = ""
			}
			m.tree.SetFontVariation(key, tag, v)
			return nil
		},
		writes: func(m *Model, v string) string {
			if v == "default" {
				return ""
			}
			return key + " = " + tag + "=" + v
		},
	}
}

// niceStep rounds a step up to 1, 2 or 5 times a power of ten.
func niceStep(raw float64) float64 {
	if raw <= 0 {
		return 1
	}
	p := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, k := range []float64{1, 2, 5, 10} {
		if raw <= k*p {
			return k * p
		}
	}
	return 10 * p
}

// The icons: Nerd Fonts' symbols-only family, and the code points its
// icons sit at, as the Nerd Fonts 3.4 tables list them. Box drawing and the
// handful of icons that are ordinary Unicode, a heart or a lightning bolt,
// are left to the font in use.
const (
	nerdSymbolsFamily = "Symbols Nerd Font Mono"
	nerdSymbolsRanges = "U+23FB-U+23FE,U+2B58,U+E000-U+E00A,U+E0A0-U+E0A3,U+E0B0-U+E0D7,U+E200-U+E2A9,U+E300-U+E3E3,U+E5FA-U+E6B7,U+E700-U+E8EF,U+EA60-U+EC1E,U+ED00-U+F2FF,U+F300-U+F381,U+F400-U+F533,U+F0001-U+F1AF0"
)

var nerdSymbolsDownload = fontdl.FontDownload{Name: "Symbols Nerd Font", Archive: "NerdFontsSymbolsOnly", Note: "The Nerd Font icons alone, to draw beside any font"}

// iconSetting maps the Nerd Font icons to Symbols Nerd Font Mono. When it is
// not installed, turning the icons on downloads it and maps them in one go:
// the Enter that chose on is the consent, and the archive is small.
func (m *Model) iconSetting() setting {
	const key = "font-codepoint-map"
	mapIt := func(m *Model) tea.Cmd {
		m.tree.SetCodepointMap(nerdSymbolsRanges, nerdSymbolsFamily, true)
		m.info("Nerd Font icons: drawn from %s — written; %s", nerdSymbolsFamily, m.reloadWord())
		return m.flushWritesCmd()
	}
	return setting{
		cmd: "icons", label: "Nerd Font icons", syntax: "icons on|off", key: key, fallback: "off",
		detail:  "Draw every Nerd Font icon from " + nerdSymbolsFamily + ", whatever font is in use",
		options: []string{"on", "off"},
		get: func(m *Model) string {
			if m.tree.CodepointMapped(nerdSymbolsFamily) {
				return "on"
			}
			return "off"
		},
		set: func(m *Model, v string) tea.Cmd {
			if v != "on" {
				m.tree.SetCodepointMap("", nerdSymbolsFamily, false)
				return nil
			}
			if !m.fontsLoaded {
				m.warn("Still listing the fonts on this machine; try again in a moment")
				return m.ensureFonts()
			}
			if _, ok := m.findFamily(nerdSymbolsFamily); ok {
				m.tree.SetCodepointMap(nerdSymbolsRanges, nerdSymbolsFamily, true)
				return nil
			}
			m.fx.afterInstall = mapIt
			m.info("Downloading %s, then the icons are drawn from it", nerdSymbolsFamily)
			return m.installFonts([]fontdl.FontDownload{nerdSymbolsDownload})
		},
		describe: func(m *Model, v string) string { return m.describeIcons(v) },
		writes: func(m *Model, v string) string {
			if v == "on" {
				return key + " = " + nerdSymbolsRanges + "=" + nerdSymbolsFamily
			}
			return ""
		},
	}
}

// describeIcons says where the icons come from with each value.
func (m *Model) describeIcons(v string) string {
	family := orDash(m.fontFamily)
	has := ""
	if info, _ := m.currentFontInfo(); info != nil {
		if info.NerdIcons {
			has = family + " has icons of its own, which this replaces. "
		} else {
			has = family + " has no icons of its own. "
		}
	}
	_, installed := m.findFamily(nerdSymbolsFamily)
	if v == "on" {
		if !installed && m.fontsLoaded {
			dir, _ := fontdl.FontInstallDir()
			return has + "It is not on this machine yet: Enter downloads it from Nerd Fonts, about 3 MB, into " + shortenPath(dir) + ", and draws every icon from it"
		}
		return has + "Every icon is drawn from " + nerdSymbolsFamily + ", one cell wide, the same in every font"
	}
	return has + "Icons come from the font in use, or where it has none from the copy of Symbols Nerd Font 3.4 that Ghostty carries; on only pins them to a copy of your own"
}
