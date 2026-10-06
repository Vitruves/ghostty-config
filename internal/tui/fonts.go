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
// and under them the ones this tool can fetch. Installing a font is done
// where fonts are chosen: Enter on a family that is not there yet downloads
// it, and it joins the list above.

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

	missing := m.missingDownloads()
	if len(missing) == 0 {
		return out
	}
	if titled {
		out = append(out, option{header: true}, option{label: "Not installed yet", detail: "Enter installs one", header: true})
	}
	for _, d := range missing {
		out = append(out, option{label: d.Name, detail: "download", value: downloadPrefix + d.Archive})
	}
	if titled && len(missing) > 1 {
		out = append(out, option{label: fmt.Sprintf("All %d of them", len(missing)), detail: "one after another", value: downloadPrefix + "*"})
	}
	return out
}

// installArchive installs one family of the list, or every missing one.
func (m *Model) installArchive(archive string) tea.Cmd {
	missing := m.missingDownloads()
	if archive == "*" {
		if len(missing) == 0 {
			m.info("Every family on offer is already installed")
			return nil
		}
		return m.installFonts(missing)
	}
	for _, d := range fontdl.AvailableDownloads {
		if d.Archive == archive {
			return m.installFonts([]fontdl.FontDownload{d})
		}
	}
	return nil
}

// downloadOf finds the family a download line stands for; nil for the line
// that installs them all.
func downloadOf(opt *option) *fontdl.FontDownload {
	archive := strings.TrimPrefix(opt.value, downloadPrefix)
	for i := range fontdl.AvailableDownloads {
		if fontdl.AvailableDownloads[i].Archive == archive {
			return &fontdl.AvailableDownloads[i]
		}
	}
	return nil
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
	family := m.fontFamily
	var styles []string
	mono := true
	if opt != nil && opt.family != nil {
		family, styles, mono = opt.family.Name, opt.family.Styles, opt.family.Mono
	} else if f, ok := m.findFamily(m.fontFamily); ok {
		styles, mono = f.Styles, f.Mono
	}
	kind := "monospace"
	if !mono {
		kind = "proportional: columns will not line up"
	}
	lines := []string{
		c.bold(c.fg).Render(truncate(orDash(family), width)),
		c.mutedS().Render(truncate(fmt.Sprintf("%s · %s · %s pt", kind, m.currentStyle(), ghostty.FormatSize(m.fontSize)), width)),
	}
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
	if m.autoReloadWorks() {
		lines = append(lines, para(c.mutedS(), "Moving the highlight writes it and reloads Ghostty: this whole screen is redrawn in the font as you go.", width)...)
	} else {
		lines = append(lines, para(c.text(c.warn), "Moving the highlight writes it; "+m.reloadHint()+".", width)...)
	}
	return lines
}

// previewDownloadPanel says what installing the highlighted family does.
func previewDownloadPanel(m *Model, opt *option, width int) []string {
	c := m.c
	dir, _ := fontdl.FontInstallDir()
	d := downloadOf(opt)
	var lines []string
	source := "ryanoasis/nerd-fonts, latest release"
	if d == nil {
		lines = append(lines, c.bold(c.fg).Render(truncate("Every family not installed yet", width)))
		lines = append(lines, para(c.mutedS(), "One archive after another, roughly a minute in all.", width)...)
	} else {
		lines = append(lines, c.bold(c.fg).Render(truncate(d.Name, width)))
		lines = append(lines, para(c.mutedS(), d.Note+".", width)...)
		if d.URL != "" {
			source = strings.TrimPrefix(d.URL, "https://")
			if parts := strings.SplitN(source, "/", 4); len(parts) >= 3 {
				source = parts[1] + "/" + parts[2]
			}
		}
	}
	lines = append(lines, "")
	lines = append(lines, truncate(c.mutedS().Render(pad("source", 8))+c.base().Render(source), width))
	lines = append(lines, truncate(c.mutedS().Render(pad("writes", 8))+c.base().Render(shortenPath(dir)), width))
	lines = append(lines, "")
	lines = append(lines, m.entry("Enter", "downloads and installs it, icons and powerline glyphs included. No administrator rights.", 8, width, c.bold(c.accent))...)
	lines = append(lines, "")
	return append(lines, para(c.mutedS(), "It cannot be shown before it is on this machine: once installed it joins the list above, where moving the highlight tries it.", width)...)
}
