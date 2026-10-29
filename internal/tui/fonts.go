package tui

import (
	"fmt"
	"runtime"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/fontdl"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The font list holds two kinds of line: the families Ghostty can load now,
// and under them the ones this tool can fetch, the popular ones first and
// the rest A to Z. Installing a font is done where fonts are chosen: Enter
// on a family that is not there yet downloads it, and it joins the list
// above. There is no line that installs them all: some seventy families
// come to several gigabytes, and a font is chosen one at a time anyway.

// downloadPrefix marks the value of a line that installs instead of picking.
const downloadPrefix = "@download:"

// installFont fetches and installs one family; a variable so the tests never
// reach the network.
var installFont = fontdl.Install

// here names the machine in the list's title.
func here() string {
	if runtime.GOOS == "darwin" {
		return "On this Mac"
	}
	return "On this machine"
}

// thisMachine is here() in the middle of a sentence.
func thisMachine() string {
	return strings.ToLower(here()[:1]) + here()[1:]
}

// missingDownloads are the families on offer that are not installed yet.
func (m *Model) missingDownloads() []fontdl.FontDownload {
	installed := m.installedDownloadables()
	var out []fontdl.FontDownload
	for _, d := range fontdl.AvailableDownloads {
		if !installed[d.Name] {
			out = append(out, d)
		}
	}
	return out
}

// downloadFamilies are the installed families each download brought, or the
// originals they were made from, by archive.
func (m *Model) downloadFamilies() map[string][]*ghostty.Family {
	out := map[string][]*ghostty.Family{}
	for i := range m.families {
		if n := fontdl.Owner(m.families[i].Name); n >= 0 {
			archive := fontdl.AvailableDownloads[n].Archive
			out[archive] = append(out[archive], &m.families[i])
		}
	}
	return out
}

// bestOf is the family to use of those a download brought: the monospaced
// cut with the shortest name, "JetBrainsMono Nerd Font" rather than its
// Propo cut.
func bestOf(families []*ghostty.Family) (*ghostty.Family, bool) {
	var best *ghostty.Family
	for _, f := range families {
		if best == nil || (f.Mono && !best.Mono) || (f.Mono == best.Mono && len(f.Name) < len(best.Name)) {
			best = f
		}
	}
	return best, best != nil
}

// familyFor is the installed family a download brought.
func (m *Model) familyFor(d fontdl.FontDownload) (*ghostty.Family, bool) {
	return bestOf(m.downloadFamilies()[d.Archive])
}

// fontOptions lists the installed families, monospace first, then the ones
// that can be installed. While something is typed the titles are left out,
// so the prompt can rank every line together.
func (m *Model) fontOptions(arg string) []option {
	if !m.fontsLoaded {
		return []option{{label: "Listing the fonts Ghostty can load…", header: true}}
	}
	titled := strings.TrimSpace(arg) == ""
	var out []option
	if titled {
		if trouble := m.fontTrouble(); trouble != nil {
			out = append(out, m.troubleOptions(trouble)...)
			out = append(out, option{header: true})
		}
		out = append(out, option{label: here(), detail: "monospace first", header: true})
	}
	first := len(out)
	for i := range m.families {
		f := &m.families[i]
		detail := "proportional"
		if f.Mono {
			detail = "monospace"
		}
		if len(f.Styles) > 1 {
			detail += fmt.Sprintf(" · %d styles", len(f.Styles))
		}
		out = append(out, option{label: f.Name, detail: detail, value: f.Name, family: f, current: f.Name == m.fontFamily})
	}
	families := out[first:]
	sort.SliceStable(families, func(a, b int) bool { return families[a].family.Mono && !families[b].family.Mono })

	var popular, more []option
	for _, d := range m.missingDownloads() {
		o := option{label: d.Name, detail: "download", value: downloadPrefix + d.Archive}
		if d.Popular {
			popular = append(popular, o)
		} else {
			more = append(more, o)
		}
	}
	for _, group := range []struct {
		lines         []option
		title, detail string
	}{
		{popular, "Popular", "not installed · Enter installs one"},
		{more, "More", "not installed · A to Z"},
	} {
		if len(group.lines) == 0 {
			continue
		}
		if titled {
			out = append(out, option{header: true}, option{label: group.title, detail: group.detail, header: true})
		}
		out = append(out, group.lines...)
	}
	return out
}

// installArchive installs one family of the list.
func (m *Model) installArchive(archive string) tea.Cmd {
	d, ok := fontdl.Find(archive)
	if !ok {
		return nil
	}
	if t := m.fontTrouble(); t != nil && t.install != nil && t.install.Archive == archive {
		return m.installConfigured(*t.install)
	}
	return m.installFonts([]fontdl.FontDownload{d})
}

// downloadOf finds the family a download line stands for.
func downloadOf(opt *option) *fontdl.FontDownload {
	d, ok := fontdl.Find(strings.TrimPrefix(opt.value, downloadPrefix))
	if !ok {
		return nil
	}
	return &d
}

// previewFontPanel shows the highlighted family: what it is, a specimen, and
// what moving the highlight does. The specimen is drawn by the terminal, so
// once Ghostty has reloaded it is set in the family itself.
func previewFontPanel(m *Model, opt *option, width int) []string {
	c := m.c
	if !m.fontsLoaded {
		return []string{c.mutedS().Render(m.spin.View() + " Listing the fonts Ghostty can load…")}
	}
	if opt != nil && strings.HasPrefix(opt.value, downloadPrefix) {
		return previewDownloadPanel(m, opt, width)
	}
	var lines []string
	if t := m.fontTrouble(); t != nil && t.missing && (m.cmd == nil || m.cmd.name != "font" || opt == nil || opt.family == nil) {
		// The style list shows the family in use: say first that it is
		// missing. A proportional one is said to be so below.
		lines = append(lines, para(c.text(c.warn), t.sentence()+".", width)...)
		lines = append(lines, "")
	}
	family := m.fontFamily
	var fam *ghostty.Family
	var styles []string
	mono := true
	if opt != nil && opt.family != nil {
		fam = opt.family
	} else if f, ok := m.findFamily(m.fontFamily); ok {
		fam = &f
	}
	if fam != nil {
		family, styles, mono = fam.Name, fam.Styles, fam.Mono
	}
	kind := "monospace"
	if !mono {
		kind = "proportional: columns will not line up"
	}
	lines = append(lines,
		c.bold(c.fg).Render(truncate(orDash(family), width)),
		c.mutedS().Render(truncate(fmt.Sprintf("%s · %s · %s pt", kind, m.currentStyle(), ghostty.FormatSize(m.fontSize)), width)),
	)
	if len(styles) > 1 {
		lines = append(lines, m.entry("styles", strings.Join(styles, ", "), 8, width, c.mutedS())...)
	}
	lines = append(lines, "")
	for _, s := range []string{"ABCDEFGHIJKLMNOPQRSTUVWXYZ", "abcdefghijklmnopqrstuvwxyz", "0123456789  {}[]()<>  !@#$%&*"} {
		lines = append(lines, c.base().Render(truncate(s, width)))
	}
	lines = append(lines,
		c.bold(c.fg).Render(truncate("The quick brown fox, in bold", width)),
		c.base().Italic(true).Render(truncate("The quick brown fox, in italic", width)),
		"")
	for _, row := range [][2]string{{"look-alikes", "0O  1lI|  5S  8B  `'\""}, {"ligatures", "-> => != <= >= === ::"}} {
		lines = append(lines, c.mutedS().Render(pad(row[0], 13))+c.base().Render(truncate(row[1], width-13)))
	}
	lines = append(lines, "")
	if fam != nil {
		if note := m.iconsNote(*fam); note != "" {
			lines = append(lines, para(c.mutedS(), note, width)...)
			lines = append(lines, "")
		}
	}
	switch {
	case m.browsesFonts():
		lines = append(lines, para(c.mutedS(), "Moving the highlight only looks; Enter writes it, then "+m.reloadWord()+".", width)...)
	case m.autoReloadWorks():
		lines = append(lines, para(c.mutedS(), "Moving the highlight writes it and reloads Ghostty: this whole screen is redrawn in the font as you go.", width)...)
	default:
		lines = append(lines, para(c.text(c.warn), "Moving the highlight writes it; "+m.reloadHint()+".", width)...)
	}
	return lines
}

// previewDownloadPanel says what installing the highlighted family does.
func previewDownloadPanel(m *Model, opt *option, width int) []string {
	c := m.c
	dir, _ := fontdl.FontInstallDir()
	d := downloadOf(opt)
	if d == nil {
		return nil
	}
	var lines []string
	if t := m.fontTrouble(); opt.current && t != nil && t.install != nil {
		// The line that brings back the family the config names.
		lines = append(lines, c.bold(c.fg).Render(truncate(t.family, width)))
		lines = append(lines, para(c.text(c.warn), "Your config names it, but it is not "+thisMachine()+": Ghostty draws with its own font instead.", width)...)
		lines = append(lines, "")
		lines = append(lines, c.base().Render(truncate(d.Name, width)))
		lines = append(lines, para(c.mutedS(), d.Note+".", width)...)
	} else {
		lines = append(lines, c.bold(c.fg).Render(truncate(d.Name, width)))
		lines = append(lines, para(c.mutedS(), d.Note+".", width)...)
	}
	source := d.Source()
	if d.URL == "" {
		source += ", latest release"
	}
	lines = append(lines, "")
	lines = append(lines, truncate(c.mutedS().Render(pad("source", 8))+c.base().Render(source), width))
	lines = append(lines, truncate(c.mutedS().Render(pad("writes", 8))+c.base().Render(shortenPath(dir)), width))
	lines = append(lines, "")
	does := "downloads and installs it. No administrator rights."
	if d.HasIcons() {
		does = "downloads and installs it, icons and powerline glyphs included. No administrator rights."
	}
	if t := m.fontTrouble(); opt.current && t != nil && t.install != nil {
		does = "downloads and " + t.installs() + ". No administrator rights."
	}
	lines = append(lines, m.entry("Enter", does, 8, width, c.bold(c.accent))...)
	lines = append(lines, "")
	return append(lines, para(c.mutedS(), "It cannot be shown before it is on this machine: once installed it joins the list above, where moving the highlight tries it.", width)...)
}
