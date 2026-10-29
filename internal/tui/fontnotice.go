package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/fontdl"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// When the family the config names is not installed, Ghostty says nothing:
// it draws with its own font instead. When it is proportional, the columns
// of every program drift. Either is told where the fonts are, without a
// dialog: the font list opens on a way out, the Fonts section gains a line
// that takes it in one Enter, and the status line says so once per run.

// ghosttyBuiltin is the family Ghostty carries inside itself: naming it
// works whether or not it is installed.
const ghosttyBuiltin = "JetBrains Mono"

// fontTrouble is what is wrong with the configured family.
type fontTrouble struct {
	family  string // as the config names it
	missing bool   // not installed; otherwise installed but proportional
	// install is the download that brings a missing family back.
	install *fontdl.FontDownload
}

// fontTrouble says what is wrong with the configured family, nil when
// nothing is or the families are not listed yet.
func (m *Model) fontTrouble() *fontTrouble {
	if !m.fontsLoaded || m.fontFamily == "" || strings.EqualFold(m.fontFamily, ghosttyBuiltin) {
		return nil
	}
	for _, f := range m.families {
		if strings.EqualFold(f.Name, m.fontFamily) {
			if f.Mono {
				return nil
			}
			return &fontTrouble{family: m.fontFamily}
		}
	}
	t := &fontTrouble{family: m.fontFamily, missing: true}
	for i := range fontdl.AvailableDownloads {
		if fontdl.AvailableDownloads[i].Provides(m.fontFamily) {
			t.install = &fontdl.AvailableDownloads[i]
			break
		}
	}
	return t
}

// key tells one trouble from another, so the commands are built again when
// it changes.
func (t *fontTrouble) key() string {
	if t == nil {
		return ""
	}
	key := fmt.Sprintf("%s|%v", t.family, t.missing)
	if t.install != nil {
		key += "|" + t.install.Archive
	}
	return key
}

// sentence says what is wrong, without a full stop.
func (t *fontTrouble) sentence() string {
	if t.missing {
		return t.family + " is not " + thisMachine() + ": Ghostty draws with its own font instead"
	}
	return t.family + " is proportional: columns will not line up"
}

// renamed reports whether the download installs the family under another
// name than the config gives: the original's name, "Fira Code", for what
// Nerd Fonts calls "FiraCode Nerd Font". font-family follows it then.
func (t *fontTrouble) renamed() bool {
	if t.install == nil {
		return false
	}
	d := t.install
	same := strings.EqualFold(strings.ReplaceAll(t.family, " ", ""), strings.ReplaceAll(d.Name, " ", ""))
	return same && (d.URL == "" || len(d.Families) > 0)
}

// installs says what installing the configured family does.
func (t *fontTrouble) installs() string {
	if t.renamed() {
		return "installs it, and font-family is set to the name it comes with"
	}
	return "installs it, and Ghostty draws with it once it reloads"
}

// state is the trouble in a word or two, for the end of a line.
func (t *fontTrouble) state() string {
	if t.missing {
		return "not " + thisMachine()
	}
	return "proportional"
}

// troubleOptions are the lines the font list opens with: the family to
// install, or a few monospaced ones to choose from.
func (m *Model) troubleOptions(t *fontTrouble) []option {
	if t.install != nil {
		return []option{
			{label: "Your font", detail: t.state(), header: true},
			{label: t.family, detail: "install", value: downloadPrefix + t.install.Archive, current: true},
		}
	}
	out := []option{{label: "Your font", detail: t.family + " is " + t.state(), header: true}}
	return append(out, m.monoSuggestions(t.family, 3)...)
}

// monoSuggestions are a few good monospaced families from the table: the
// installed ones first, then popular ones to download.
func (m *Model) monoSuggestions(not string, n int) []option {
	var out []option
	installed := map[string]bool{}
	brought := m.downloadFamilies()
	for _, d := range fontdl.AvailableDownloads {
		f, ok := bestOf(brought[d.Archive])
		if !ok {
			continue
		}
		installed[d.Name] = true
		if f.Mono && !strings.EqualFold(f.Name, not) && len(out) < n {
			out = append(out, option{label: f.Name, detail: "monospace · installed", value: f.Name, family: f})
		}
	}
	for _, d := range fontdl.AvailableDownloads {
		if len(out) >= n {
			break
		}
		if d.Popular && !installed[d.Name] {
			out = append(out, option{label: d.Name, detail: "download", value: downloadPrefix + d.Archive})
		}
	}
	return out
}

// installConfigured downloads the family the config names. When it comes
// back under that name the config is left alone and Ghostty only has to
// reload; when it comes back under its Nerd Fonts name, "JetBrainsMono
// Nerd Font" for a config that says "JetBrains Mono", font-family is set
// to that, the one name that now works.
func (m *Model) installConfigured(d fontdl.FontDownload) tea.Cmd {
	want := m.fontFamily
	m.fx.afterInstall = func(m *Model) tea.Cmd {
		m.fx.afterFonts = func(m *Model) tea.Cmd {
			for _, f := range m.families {
				if strings.EqualFold(f.Name, want) {
					m.info("Installed %s — the config names it already; %s", f.Name, m.reloadWord())
					return m.reload()
				}
			}
			if f, ok := m.familyFor(d); ok {
				cmd := m.pickFamily(*f)
				m.info("Installed %s as %s — font-family now names it; %s", d.Name, f.Name, m.reloadWord())
				return tea.Batch(cmd, m.flushWritesCmd())
			}
			return nil
		}
		return nil
	}
	return m.installFonts([]fontdl.FontDownload{d})
}

// hintFontTrouble says once per run, on the status line, what is wrong
// with the configured family. A message already there is not pushed off:
// the hint waits for a quieter moment.
func (m *Model) hintFontTrouble() {
	if m.fx.hinted || !m.fontsLoaded || m.overlay != overlayNone || m.statusKind == statusWarn || m.statusKind == statusFail {
		return
	}
	m.fx.hinted = true
	t := m.fontTrouble()
	switch {
	case t == nil:
	case t.install != nil:
		m.warn("Your font, %s, is not %s — Fonts › Install %s", t.family, thisMachine(), t.install.Name)
	case t.missing:
		m.warn("Your font, %s, is not %s — Fonts › Choose a monospace font", t.family, thisMachine())
	default:
		m.warn("Your font, %s, is proportional — Fonts › Choose a monospace font", t.family)
	}
}

// fixFontCommand is the line at the top of the Fonts section while the
// configured family is in trouble: Enter installs it, or opens the font list
// on a few monospaced families.
func (m *Model) fixFontCommand() *command {
	t := m.fontTrouble()
	if t == nil {
		return nil
	}
	label, does := "Choose a monospace font", "Enter lists a few monospaced families, the installed ones first"
	if t.install != nil {
		label = "Install " + t.install.Name
		does = fmt.Sprintf("Enter downloads %s from %s and %s", t.install.Name, t.install.Source(), t.installs())
	}
	return &command{name: "fixfont", label: label, transient: true, group: "Fonts", syntax: "fixfont",
		desc: t.sentence() + ". " + does,
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			t := m.fontTrouble()
			switch {
			case t == nil:
				return nil
			case t.install != nil:
				return m.installConfigured(*t.install)
			}
			// The list opens on the suggestions, above the family in use.
			m.setPromptText("Font ")
			for i, o := range m.results {
				if !o.header {
					m.sel.cursor, m.sel.offset = i, 0
					break
				}
			}
			if m.px != nil {
				m.px.listTop = 0
			}
			return m.afterMove()
		},
	}
}

// nerdIconsOf reports whether a family draws the Nerd Font icons itself;
// known is false until its file has been read.
func (m *Model) nerdIconsOf(f ghostty.Family) (has, known bool) {
	name := strings.ToLower(f.Name)
	if strings.Contains(name, "nerd font") || strings.HasSuffix(name, " nf") || strings.HasSuffix(name, " nfm") || strings.HasSuffix(name, " nfp") {
		return true, true
	}
	file, ok := f.File(m.currentStyle())
	if !ok || file.Path == "" {
		return false, false
	}
	info := m.fx.infos[fileKey(file)].info
	if info == nil {
		return false, false
	}
	return info.NerdIcons, true
}

// iconsNote says briefly where the icons come from for a family that has
// none of its own; empty otherwise. Ghostty carries Symbols Nerd Font, so
// they are drawn either way; Icons only pins them to a downloaded copy.
func (m *Model) iconsNote(f ghostty.Family) string {
	if !f.Mono {
		return ""
	}
	if has, known := m.nerdIconsOf(f); has || !known {
		return ""
	}
	if m.tree.CodepointMapped(nerdSymbolsFamily) {
		return "No Nerd Font icons of its own: Icons draws them from " + nerdSymbolsFamily + "."
	}
	return "No Nerd Font icons of its own: Ghostty fills them in from the Symbols Nerd Font it carries. Icons can pin them to a downloaded copy."
}
