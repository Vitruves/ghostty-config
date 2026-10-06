package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/extensions"
)

// The Extensions section holds what other people have made for Ghostty and
// this tool can fetch: shaders and packs of themes. Ghostty has no plugins as
// such; these are files, put where Ghostty looks for them, with the one line
// of config a shader needs.

// keepDelay is how long a shader that was just switched on stays without an
// answer before it is put back: a shader that does not compile can leave the
// window black, and then nothing on screen can be read to undo it.
var keepDelay = 10 * time.Second

type (
	shaderDoneMsg struct {
		name, path string
		err        error
	}
	packDoneMsg struct {
		name    string
		written int
		err     error
	}
	shaderExpireMsg struct{ seq int }
)

// shaderNow is the shader the config names: its path, and its name when it
// is one of the list or a file of any other origin.
func (m *Model) shaderNow() (path, name string) {
	path = m.tree.Value("custom-shader", "")
	if path == "" {
		return "", ""
	}
	return path, strings.TrimSuffix(filepath.Base(path), ".glsl")
}

// extensionCommands are the commands of the section.
func (m *Model) extensionCommands() []*command {
	return []*command{
		{name: "shader", group: "Extensions", syntax: "shader <name>|none",
			desc:    "Install a shader and switch it on: an effect that follows the cursor, or one over the whole window. One at a time; none switches it off",
			filters: true,
			options: func(m *Model, arg string) []option { return m.shaderOptions(arg) },
			run: func(m *Model, arg string, opt *option) tea.Cmd {
				if opt == nil {
					m.warn("No shader matches %q", arg)
					return nil
				}
				return m.useShader(opt.value)
			},
			preview: previewShaderPanel,
		},
		{name: "pack", group: "Extensions", syntax: "pack <name>",
			desc:    "Install a pack of themes made by another project into your themes directory; they join the wall under Yours",
			filters: true,
			options: func(m *Model, arg string) []option { return m.packOptions() },
			run: func(m *Model, arg string, opt *option) tea.Cmd {
				if opt == nil {
					return nil
				}
				return m.installPack(opt.value)
			},
			preview: previewPackPanel,
		},
	}
}

// shaderOptions lists none, then the shaders by kind, each with whether it is
// on, installed, or still to be downloaded. A shader the config names that is
// not of the list is shown too, so switching away from it is not a surprise.
func (m *Model) shaderOptions(arg string) []option {
	titled := strings.TrimSpace(arg) == ""
	path, now := m.shaderNow()
	out := []option{{label: "none", detail: "no shader", value: "none", current: path == ""}}
	known := false
	group := ""
	for _, s := range extensions.Shaders {
		if titled && s.Group != group {
			group = s.Group
			out = append(out, option{header: true}, option{label: group, header: true})
		}
		state := "download"
		if s.Installed(m.paths.ConfigDir) {
			state = "installed"
		}
		on := path != "" && path == s.File(m.paths.ConfigDir)
		if on {
			state, known = "on", true
		}
		out = append(out, option{label: s.Name, detail: state, value: s.Name, current: on})
	}
	if path != "" && !known {
		out = append(out, option{header: true}, option{label: "Yours", header: true},
			option{label: now, detail: "on", value: "@own", current: true})
	}
	return out
}

// useShader switches a shader on, downloading it first when it is not there,
// or switches shaders off.
func (m *Model) useShader(name string) tea.Cmd {
	switch name {
	case "@own":
		m.info("That shader is already on")
		return nil
	case "none":
		if path, _ := m.shaderNow(); path == "" {
			m.info("No shader is on")
			return nil
		}
		m.tree.Unset("custom-shader", "disabled by ghostty-config: no shader")
		m.info("Shader off — written; %s", m.reloadWord())
		return m.flushWritesCmd()
	}
	s, ok := extensions.FindShader(name)
	if !ok {
		m.warn("No shader is called %q", name)
		return nil
	}
	if s.Installed(m.paths.ConfigDir) {
		return m.switchShaderOn(s.Name, s.File(m.paths.ConfigDir))
	}
	m.busyText = "Downloading " + s.Name + " from " + s.Repo + "…"
	m.overlay = overlayBusy
	dir := m.paths.ConfigDir
	return func() tea.Msg {
		path, err := extensions.InstallShader(dir, s)
		return shaderDoneMsg{name: s.Name, path: path, err: err}
	}
}

// switchShaderOn writes the config line. Where Ghostty reloads by itself the
// shader is on screen at once, so the question is asked whether to keep it,
// and no answer within keepDelay puts the previous state back.
func (m *Model) switchShaderOn(name, path string) tea.Cmd {
	before, _ := m.shaderNow()
	if before == path {
		m.info("%s is already on", name)
		return nil
	}
	m.tree.Set("custom-shader", path)
	write := m.flushWritesCmd()
	if !m.autoReloadWorks() {
		m.info("Shader: %s — written; %s. If the window goes wrong: shader none", name, m.reloadHint())
		return write
	}
	m.shaderSeq++
	seq := m.shaderSeq
	revert := func(m *Model) tea.Cmd {
		m.shaderSeq++
		if before == "" {
			m.tree.Unset("custom-shader", "disabled by ghostty-config: no shader")
		} else {
			m.tree.Set("custom-shader", before)
		}
		m.info("%s was not kept — the config is as it was", name)
		return m.flushWritesCmd()
	}
	m.confirmText = fmt.Sprintf("%s is on. Keep it? Without an answer it is put back in %d seconds, in case the window can no longer be read.", name, int(keepDelay/time.Second))
	m.confirmYes = func(m *Model) tea.Cmd {
		m.shaderSeq++
		m.info("Shader: %s — kept", name)
		return nil
	}
	m.confirmNo = revert
	m.confirmEsc = revert
	m.overlay = overlayConfirm
	return tea.Batch(write, tea.Tick(keepDelay, func(time.Time) tea.Msg { return shaderExpireMsg{seq} }))
}

// finishShader is the download coming back.
func (m *Model) finishShader(msg shaderDoneMsg) tea.Cmd {
	m.overlay = overlayNone
	if msg.err != nil {
		m.fail("Could not install %s: %v", msg.name, msg.err)
		return nil
	}
	cmd := m.switchShaderOn(msg.name, msg.path)
	m.recompute()
	return cmd
}

// expireShader puts the previous state back when the question went unanswered.
func (m *Model) expireShader(msg shaderExpireMsg) tea.Cmd {
	if msg.seq != m.shaderSeq || m.overlay != overlayConfirm || m.confirmNo == nil {
		return nil
	}
	m.overlay = overlayNone
	cmd := m.confirmNo(m)
	m.recompute()
	return cmd
}

// packOptions lists the packs with how much of each is installed.
func (m *Model) packOptions() []option {
	var out []option
	for _, p := range extensions.Packs {
		state := "download"
		switch n := p.Installed(m.paths.ThemesDir); {
		case n == len(p.Files):
			state = "installed"
		case n > 0:
			state = fmt.Sprintf("%d of %d installed", n, len(p.Files))
		}
		out = append(out, option{label: p.Name, detail: state, value: p.Name})
	}
	return out
}

// installPack downloads a pack behind the spinner.
func (m *Model) installPack(name string) tea.Cmd {
	p, ok := extensions.FindPack(name)
	if !ok {
		m.warn("No pack is called %q", name)
		return nil
	}
	if p.Installed(m.paths.ThemesDir) == len(p.Files) {
		m.info("%s is already installed — its themes are on the wall under Yours", p.Name)
		return nil
	}
	m.busyText = "Downloading " + p.Name + " from " + p.Repo + "…"
	m.overlay = overlayBusy
	dir := m.paths.ThemesDir
	return func() tea.Msg {
		written, err := extensions.InstallPack(dir, p)
		return packDoneMsg{name: p.Name, written: written, err: err}
	}
}

// finishPack is the download coming back.
func (m *Model) finishPack(msg packDoneMsg) tea.Cmd {
	m.overlay = overlayNone
	if msg.written > 0 {
		m.reloadLibrary()
	}
	m.recompute()
	if msg.err != nil {
		m.fail("Could not install %s: %v", msg.name, msg.err)
		return nil
	}
	m.info("Installed %s: %d themes — they are on the wall under Yours", msg.name, msg.written)
	return nil
}

// licenceLine says under what terms something is published, and says so
// plainly when its repository states none.
func licenceLine(c chrome, licence string) string {
	if licence == "" {
		return c.text(c.warn).Render("none stated: read it before you keep it")
	}
	return c.base().Render(licence)
}

// previewShaderPanel says what the highlighted shader is, where it comes
// from, and exactly what Enter writes.
func previewShaderPanel(m *Model, opt *option, width int) []string {
	c := m.c
	if opt == nil {
		return previewOverview(m, opt, width)
	}
	field := func(key, value string) string {
		return truncate(c.mutedS().Render(pad(key, 9))+value, width)
	}
	switch opt.value {
	case "none":
		lines := []string{c.bold(c.fg).Render("none")}
		lines = append(lines, para(c.mutedS(), "No shader: the custom-shader line of your config is commented out. The files stay where they are.", width)...)
		return lines
	case "@own":
		path, name := m.shaderNow()
		lines := []string{c.bold(c.fg).Render(truncate(name, width)), ""}
		lines = append(lines, field("file", c.base().Render(shortenPath(path))))
		return append(lines, para(c.mutedS(), "A shader of your own, named by your config. Choosing another replaces the line; the file is left alone.", width)...)
	}
	s, ok := extensions.FindShader(opt.value)
	if !ok {
		return nil
	}
	lines := []string{
		c.bold(c.fg).Render(truncate(s.Name, width)),
		c.mutedS().Render(truncate("shader · "+strings.ToLower(s.Group), width)),
	}
	lines = append(lines, para(c.mutedS(), s.Note+".", width)...)
	lines = append(lines, "",
		field("source", c.base().Render(s.Repo)),
		field("licence", licenceLine(c, s.Licence)),
		"",
		field("writes", c.base().Render(shortenPath(s.File(m.paths.ConfigDir)))),
		field("adds", c.text(c.accent).Render("custom-shader = …/shaders/"+s.Name+".glsl")),
		"")
	lines = append(lines, m.entry("Enter", "installs it and switches it on. You are then asked whether to keep it: with no answer it is put back, because a shader that fails can leave the window unreadable.", 9, width, c.bold(c.accent))...)
	return lines
}

// previewPackPanel says what the highlighted pack holds and where it goes.
func previewPackPanel(m *Model, opt *option, width int) []string {
	c := m.c
	if opt == nil {
		return previewOverview(m, opt, width)
	}
	p, ok := extensions.FindPack(opt.value)
	if !ok {
		return nil
	}
	field := func(key, value string) string {
		return truncate(c.mutedS().Render(pad(key, 9))+value, width)
	}
	lines := []string{
		c.bold(c.fg).Render(truncate(p.Name, width)),
		c.mutedS().Render(truncate(fmt.Sprintf("theme pack · %d themes", len(p.Files)), width)),
	}
	lines = append(lines, para(c.mutedS(), p.Note+".", width)...)
	lines = append(lines, "",
		field("source", c.base().Render(p.Repo)),
		field("licence", licenceLine(c, p.Licence)),
		"",
		field("writes", c.base().Render(shortenPath(m.paths.ThemesDir))))
	for _, f := range p.Files {
		lines = append(lines, field("", c.base().Render(f.Theme)))
	}
	lines = append(lines, field("adds", c.mutedS().Render("nothing: pick one on the wall of themes")), "")
	lines = append(lines, m.entry("Enter", "downloads them. A theme of the same name that is already there is left as it is.", 9, width, c.bold(c.accent))...)
	return lines
}
