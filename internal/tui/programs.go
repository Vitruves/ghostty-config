package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/extensions"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The Programs part of Extensions installs command-line tools that put
// Ghostty to use, and starts them with the shell. A program is installed
// with Homebrew when there is Homebrew, else from its GitHub release into
// ~/.local/bin; the line that starts it goes in a block of the shell's start
// file that this tool owns, backed up before its first change. What the
// block holds is the state: nothing about programs is remembered elsewhere.

type programDoneMsg struct {
	name, path string
	err        error // the install failed
	setupErr   error // installed, but its own setup failed
}

type programSetupMsg struct {
	name, verb string
	err        error
}

// programsView is what the list shows about every program, read from the
// disk: where each is installed and what the shell's start file holds.
type programsView struct {
	home, shell string
	rc          string          // the file the block goes in
	text, user  string          // that file's text, and the user's own file's
	on          map[string]bool // programs the block starts
	where       map[string]string
}

// programsSeen keeps the view until the status line says something new, as
// it does after every command: the menu asks on every frame, and finding a
// program stats a few directories.
var programsSeen struct {
	sync.Mutex
	key  string
	seq  int
	view *programsView
}

func (m *Model) programs() *programsView {
	home, shell := homeDir(), extensions.UserShell()
	key := home + "\x00" + shell + "\x00" + os.Getenv("PATH")
	programsSeen.Lock()
	defer programsSeen.Unlock()
	if programsSeen.view != nil && programsSeen.key == key && programsSeen.seq == m.statusSeq {
		return programsSeen.view
	}
	v := &programsView{home: home, shell: shell, on: map[string]bool{}, where: map[string]string{}}
	if shell != "" {
		v.rc = extensions.RcFile(shell, home)
		data, _ := os.ReadFile(v.rc)
		v.text = string(data)
		v.user = v.text
		if own := extensions.UserRcFile(shell, home); own != v.rc {
			data, _ := os.ReadFile(own)
			v.user = string(data)
		}
		for _, n := range extensions.Enabled(v.text) {
			v.on[n] = true
		}
	}
	for _, p := range extensions.Programs {
		v.where[p.Name] = p.Where(home)
	}
	programsSeen.key, programsSeen.seq, programsSeen.view = key, m.statusSeq, v
	return v
}

// forgetPrograms makes the next look read the disk again.
func forgetPrograms() {
	programsSeen.Lock()
	programsSeen.view = nil
	programsSeen.Unlock()
}

// starts reports whether the program is started with the shell, and whether
// by a line of the user's own rather than by the block.
func (v *programsView) starts(p extensions.Program) (on, own bool) {
	if v.shell == "" {
		return false, false
	}
	if extensions.OwnLine(v.user, p) || (v.user != v.text && extensions.OwnLine(v.text, p)) {
		return true, true
	}
	return v.on[p.Name], false
}

// startable is whether the program has something to start in this shell.
func (v *programsView) startable(p extensions.Program) bool {
	return v.shell != "" && (p.SelfSetup || p.Init[v.shell] != "")
}

func (v *programsView) installedCount() int {
	n := 0
	for _, p := range extensions.Programs {
		if v.where[p.Name] != "" {
			n++
		}
	}
	return n
}

// programCommand is the command of the Programs part.
func (m *Model) programCommand() *command {
	return &command{name: "program", group: "Extensions", syntax: "program <name>",
		desc:    "Install a command-line tool that puts Ghostty to use, and start it with your shell: a prompt, completions, images in the terminal, clickable links",
		filters: true,
		options: func(m *Model, arg string) []option { return m.programOptions(arg) },
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if opt == nil {
				m.warn("No program matches %q", arg)
				return nil
			}
			return m.useProgram(opt.value)
		},
		preview: previewProgramPanel,
	}
}

// programOptions lists the programs by kind, each with what Enter would do.
func (m *Model) programOptions(arg string) []option {
	v := m.programs()
	titled := strings.TrimSpace(arg) == ""
	var out []option
	group := ""
	for _, p := range extensions.Programs {
		if titled && p.Group != group {
			if group != "" {
				out = append(out, option{header: true})
			}
			group = p.Group
			out = append(out, option{label: group, header: true})
		}
		on, _ := v.starts(p)
		out = append(out, option{label: p.Name, detail: programState(v, p), value: p.Name, current: on})
	}
	return out
}

// programState is the word the list shows beside a program.
func programState(v *programsView, p extensions.Program) string {
	if v.where[p.Name] == "" {
		route := p.Plan()
		switch {
		case route.Brew != "":
			return "brew"
		case route.Release:
			return "download"
		}
		return "not for this system"
	}
	if !v.startable(p) {
		return "installed"
	}
	if on, _ := v.starts(p); on {
		return "on"
	}
	return "installed, off"
}

// useProgram is Enter on a program: install it, start it with the shell,
// or stop starting it.
func (m *Model) useProgram(name string) tea.Cmd {
	p, ok := extensions.FindProgram(name)
	if !ok {
		m.warn("No program is called %q", name)
		return nil
	}
	v := m.programs()
	path := v.where[p.Name]
	if path == "" {
		route := p.Plan()
		if route.Why != "" {
			m.warn("%s", route.Why)
			return nil
		}
		m.openConfirm(installQuestion(v, p, route), func(m *Model) tea.Cmd { return m.installProgram(p) })
		return nil
	}
	if !v.startable(p) {
		if v.shell == "" && (p.SelfSetup || len(p.Init) > 0) {
			m.warn("%s is installed. Your shell is not zsh, bash or fish, so start it yourself", p.Name)
			return nil
		}
		if len(p.Init) > 0 {
			m.info("%s is installed. It has no line for %s here: see %s", p.Name, v.shell, "github.com/"+p.Repo)
			return nil
		}
		m.info("%s is installed: run %s", p.Name, p.Bin())
		return nil
	}
	on, own := v.starts(p)
	switch {
	case own && !p.SelfSetup:
		m.info("%s already starts %s with a line of your own; ghostty-config leaves it alone", shortenPath(v.rc), p.Name)
		return nil
	case on && p.SelfSetup:
		m.openConfirm(fmt.Sprintf("Stop starting %s with %s? This runs %s uninstall %s, which takes its lines out of your shell's file. The program stays installed.", p.Name, v.shell, p.Name, v.shell),
			func(m *Model) tea.Cmd { return m.runSelfSetup(p, path, "uninstall", v.shell) })
		return nil
	case on:
		m.openConfirm(fmt.Sprintf("Stop starting %s with %s? Its line leaves %s; the program stays installed.", p.Name, v.shell, shortenPath(v.rc)),
			func(m *Model) tea.Cmd { return m.setProgramLine(p, false) })
		return nil
	case p.SelfSetup:
		return m.runSelfSetup(p, path, "install", v.shell)
	}
	return m.setProgramLine(p, true)
}

// installQuestion says exactly what installing will do.
func installQuestion(v *programsView, p extensions.Program, route extensions.Route) string {
	how := fmt.Sprintf("with Homebrew (brew install %s)", p.Brew)
	if route.Release {
		how = fmt.Sprintf("from its GitHub release (%s) into %s, checked against the SHA-256 GitHub gives for the file", p.Repo, shortenPath(extensions.LocalBin(v.home)))
	}
	text := fmt.Sprintf("Install %s %s", p.Name, how)
	switch on, _ := v.starts(p); {
	case !v.startable(p) || on:
	case p.SelfSetup:
		text += fmt.Sprintf(", then run %s install %s to start it with your shell", p.Name, v.shell)
	default:
		text += fmt.Sprintf(", and start it from %s", shortenPath(v.rc))
	}
	text += "?"
	if p.Caveat != "" {
		text += " " + p.Caveat
	}
	return text
}

// installProgram installs behind the spinner, and runs the program's own
// setup there too when it has one: both run programs, which can take time.
func (m *Model) installProgram(p extensions.Program) tea.Cmd {
	v := m.programs()
	route := p.Plan()
	m.busyText = "Installing " + p.Name + " from " + p.Repo + "…"
	if route.Brew != "" {
		m.busyText = "Installing " + p.Name + " with Homebrew…"
	}
	m.overlay = overlayBusy
	home, shell := v.home, v.shell
	selfSetup := p.SelfSetup && v.startable(p)
	if on, _ := v.starts(p); on {
		selfSetup = false
	}
	return func() tea.Msg {
		path, err := installProgram(context.Background(), p, home)
		msg := programDoneMsg{name: p.Name, path: path, err: err}
		if err == nil && selfSetup {
			msg.setupErr = extensions.SelfSetup(context.Background(), path, "install", shell)
		}
		return msg
	}
}

// installProgram is extensions.Install, a variable so tests install nothing.
var installProgram = extensions.Install

// finishProgram is the install coming back: the line that starts it is
// written now, on the UI's side, like every other write.
func (m *Model) finishProgram(msg programDoneMsg) tea.Cmd {
	m.overlay = overlayNone
	forgetPrograms()
	defer m.recompute()
	if msg.err != nil {
		m.fail("Could not install %s: %v", msg.name, msg.err)
		return nil
	}
	p, _ := extensions.FindProgram(msg.name)
	v := m.programs()
	where := shortenPath(msg.path)
	switch on, _ := v.starts(p); {
	case msg.setupErr != nil:
		m.warn("Installed %s at %s, but %v", p.Name, where, msg.setupErr)
	case p.SelfSetup && v.startable(p):
		m.info("Installed %s at %s and set it up for %s; open a new tab to use it", p.Name, where, v.shell)
	case v.startable(p) && !on:
		return m.setProgramLine(p, true)
	default:
		m.info("Installed %s at %s: run %s", p.Name, where, p.Bin())
	}
	return nil
}

// setProgramLine adds the program's line to the block, or takes it out.
func (m *Model) setProgramLine(p extensions.Program, on bool) tea.Cmd {
	v := m.programs()
	names := []string{}
	for n := range v.on {
		if n != p.Name {
			names = append(names, n)
		}
	}
	if on {
		names = append(names, p.Name)
	}
	// ~/.local/bin goes on the PATH when a program started there lives in it.
	local := false
	bin := extensions.LocalBin(v.home)
	for _, n := range names {
		if filepath.Dir(v.where[n]) == bin {
			local = true
		}
	}
	text := extensions.WithBlock(v.text, v.shell, names, local)
	err := ghostty.WriteOther(m.paths, v.rc, []byte(text))
	forgetPrograms()
	m.recompute()
	switch {
	case err != nil:
		m.fail("Could not write %s: %v", shortenPath(v.rc), err)
	case on:
		m.info("%s starts with %s now (%s); open a new tab to see it", p.Name, v.shell, shortenPath(v.rc))
	default:
		m.info("%s no longer starts with %s; it is still installed", p.Name, v.shell)
	}
	return nil
}

// runSelfSetup runs "<program> install|uninstall <shell>" behind the spinner.
func (m *Model) runSelfSetup(p extensions.Program, path, verb, shell string) tea.Cmd {
	m.busyText = fmt.Sprintf("Running %s %s %s…", p.Name, verb, shell)
	m.overlay = overlayBusy
	return func() tea.Msg {
		return programSetupMsg{name: p.Name, verb: verb, err: extensions.SelfSetup(context.Background(), path, verb, shell)}
	}
}

func (m *Model) finishProgramSetup(msg programSetupMsg) tea.Cmd {
	m.overlay = overlayNone
	forgetPrograms()
	defer m.recompute()
	switch {
	case msg.err != nil:
		m.fail("%v", msg.err)
	case msg.verb == "install":
		m.info("%s is set up for your shell; open a new tab to use it", msg.name)
	default:
		m.info("%s no longer starts with your shell; it is still installed", msg.name)
	}
	return nil
}

// previewProgramPanel says what the highlighted program is, where it comes
// from, how it would be installed and what it adds to the shell's file.
func previewProgramPanel(m *Model, opt *option, width int) []string {
	c := m.c
	if opt == nil {
		return previewOverview(m, opt, width)
	}
	p, ok := extensions.FindProgram(opt.value)
	if !ok {
		return nil
	}
	v := m.programs()
	field := func(key, value string) string {
		return truncate(c.mutedS().Render(pad(key, 9))+value, width)
	}
	lines := []string{
		c.bold(c.fg).Render(truncate(p.Name, width)),
		c.mutedS().Render(truncate("program · "+strings.ToLower(p.Group), width)),
	}
	lines = append(lines, para(c.mutedS(), p.Note+".", width)...)
	lines = append(lines, "",
		field("source", c.base().Render(p.Repo)),
		field("licence", licenceLine(c, p.Licence)))
	lines = append(lines, m.entry("uses", p.Uses+".", 9, width, c.mutedS())...)
	lines = append(lines, "")
	path := v.where[p.Name]
	if path != "" {
		lines = append(lines, field("found", c.base().Render(shortenPath(path))))
	} else {
		route := p.Plan()
		switch {
		case route.Brew != "":
			lines = append(lines, field("installs", c.base().Render("brew install "+p.Brew)))
		case route.Release:
			lines = append(lines, field("installs", c.base().Render(shortenPath(filepath.Join(extensions.LocalBin(v.home), strings.Join(p.Bins, ", "))))))
			lines = append(lines, field("", c.mutedS().Render("from the latest release, SHA-256 checked")))
		default:
			lines = append(lines, field("installs", c.text(c.warn).Render("no build for this system")))
		}
	}
	on, own := v.starts(p)
	switch {
	case v.shell == "" && (p.SelfSetup || len(p.Init) > 0):
		lines = append(lines, field("adds", c.mutedS().Render("nothing: your shell is not zsh, bash or fish")))
	case p.SelfSetup:
		lines = append(lines, field("adds", c.text(c.accent).Render(fmt.Sprintf("%s install %s", p.Name, v.shell))))
		lines = append(lines, field("", c.mutedS().Render("which writes its own first and last lines")))
	case p.Init[v.shell] != "":
		lines = append(lines, field("adds", c.text(c.accent).Render(p.Init[v.shell])))
		lines = append(lines, field("to", c.base().Render(shortenPath(v.rc))))
	case len(p.Init) > 0:
		lines = append(lines, field("adds", c.mutedS().Render("nothing: no line for "+v.shell)))
	default:
		lines = append(lines, field("adds", c.mutedS().Render("nothing: run "+p.Bin())))
	}
	lines = append(lines, "")
	if p.Caveat != "" {
		lines = append(lines, para(c.text(c.warn), p.Caveat, width)...)
		lines = append(lines, "")
	}
	var enter string
	switch {
	case path == "":
		enter = "asks, then installs it"
		if v.startable(p) && !on {
			enter += " and starts it with your shell"
		}
		enter += "."
	case own && !p.SelfSetup:
		enter = "nothing: a line of your own already starts it."
	case on:
		enter = "asks, then stops starting it with your shell. The program stays installed."
	case v.startable(p):
		enter = "starts it with your shell. The file is backed up before its first change."
	default:
		enter = "says how to run it."
	}
	lines = append(lines, m.entry("Enter", enter, 9, width, c.bold(c.accent))...)
	how := ""
	switch dir := filepath.Dir(path); {
	case path == "":
	case dir == extensions.LocalBin(v.home):
		how = "delete " + strings.Join(p.Bins, ", ") + " from " + shortenPath(dir)
	case p.Brew != "" && extensions.Brew() != "" && dir == filepath.Dir(extensions.Brew()):
		how = "brew uninstall " + p.Brew
	}
	if how != "" {
		lines = append(lines, "")
		lines = append(lines, para(c.mutedS(), "To remove the program itself: "+how+".", width)...)
	}
	return lines
}
