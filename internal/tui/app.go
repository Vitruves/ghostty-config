package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vitruves/ghostty-config/internal/ghostty"
	"github.com/vitruves/ghostty-config/internal/update"
)

// overlay is a dialog drawn over the screen and owning the keyboard.
type overlay int

const (
	overlayNone overlay = iota
	overlayWelcome
	overlayHelp
	overlayConfirm
	overlayBusy
	overlayMessage
)

// Layout: header, prompt, rule, body, status, keys.
const (
	chromeTop = 3
	chromeBot = 2
)

// writeDebounce is how long the highlight must rest before a font or a
// setting is written and Ghostty asked to reload, so walking a list of
// families costs one rewrite rather than one per row.
var writeDebounce = 220 * time.Millisecond

// Options tune the editor from the command line.
type Options struct {
	NoReload bool
	// Plain keeps to glyphs every terminal and font can draw.
	Plain bool
	// Version is this build's version, compared with the newest release.
	Version string
	// NoUpdateCheck never asks GitHub for a newer release.
	NoUpdateCheck bool
}

// Model is the whole editor: one prompt, a list of what it can mean, and a
// panel showing what the highlighted line would do.
type Model struct {
	paths ghostty.Paths
	tree  *ghostty.Tree
	lib   *ghostty.Library
	state *ghostty.State
	fonts *ghostty.FontCatalog
	live  *ghostty.Live
	opts  Options

	width, height int
	overlay       overlay
	c             chrome
	caps          termCaps

	// The prompt and what it currently resolves to.
	cmds     []*command
	prompt   textinput.Model
	cmd      *command // nil while the prompt shows the menu
	arg      string
	results  []option
	sel      scroller
	centre   bool // put the highlight mid-list on the next draw
	lastText string

	// The group whose commands the empty prompt lists, and whether the
	// palette is hidden to look at the terminal behind it.
	group int
	peek  bool

	// Palette edit mode: the prompt becomes a slot editor.
	editing  bool
	editKey  string
	editHex  string
	editText string

	// The theme being shown and edited. cur is the working copy; base is
	// the snapshot it was loaded from, so undo never touches the disk.
	cur          *ghostty.Theme
	base         *ghostty.Theme
	dirty        bool
	discardArmed bool
	applied      string
	setting      ghostty.ThemeSetting
	darkDesktop  bool
	overrides    []ghostty.Entry
	previewMode  int
	previewScrl  int

	// Fonts.
	families    []ghostty.Family
	fontsLoaded bool
	fontLoading bool
	fontFamily  string
	fontStyle   string
	fontSize    float64

	// Mouse: what is clickable on the current frame, and the last click.
	regions   []region
	lastClick clickMemory

	// Dialogs.
	confirmText string
	confirmYes  func(*Model) tea.Cmd
	confirmNo   func(*Model) tea.Cmd // nil means "no" only closes the question
	busyText    string
	messageText string
	spin        spinner.Model

	// Status line.
	status     string
	statusKind statusKind

	// Debounced writes: only the newest scheduled write survives.
	writeSeq  int
	reloadSeq int

	reloadFailed bool
	// top is the terminal row the editor's first row sits on: it draws in
	// the bottom rows, under what was on screen.
	top        int
	exitNote   string
	updateNote string // a newer release, told again on the way out
	quitting   bool
}

// New builds the editor over loaded configuration.
func New(paths ghostty.Paths, tree *ghostty.Tree, lib *ghostty.Library, state *ghostty.State, opts Options) *Model {
	m := &Model{
		paths: paths, tree: tree, lib: lib, state: state, opts: opts,
		fonts: ghostty.NewFontCatalog(paths.Binary),
		c:     defaultChrome(),
		caps:  detectTerminal(opts.Plain),
	}
	// Retinting the terminal itself only makes sense in Ghostty, where the
	// colours being previewed are the ones it will end up with. Anywhere
	// else the screen is painted cell by cell and the terminal is left alone.
	if m.caps.inGhostty {
		m.live = ghostty.OpenLive()
	}
	m.darkDesktop = ghostty.DarkDesktop()
	m.spin = spinner.New(spinner.WithSpinner(spinner.MiniDot))
	m.prompt = textinput.New()
	m.prompt.Prompt = "❯ "
	m.prompt.CharLimit = 96
	m.prompt.Focus()
	m.readApplied()
	m.readFont()
	m.landOnApplied()
	// Open on the themes, the applied one highlighted: colour is what people
	// come for, and Esc is one keystroke from every other command.
	m.prompt.SetValue("Theme ")
	m.prompt.CursorEnd()
	m.lastText = "Theme "
	m.recompute()
	m.renderStatus()
	if !state.Welcomed {
		m.overlay = overlayWelcome
	}
	return m
}

// readApplied reads which theme the config points at.
func (m *Model) readApplied() {
	m.setting = ghostty.ParseThemeSetting(m.tree.Value("theme", ""))
	m.applied = m.setting.Active(m.darkDesktop)
	m.overrides = m.tree.ColourOverrides()
}

// readFont reads the configured font.
func (m *Model) readFont() {
	m.fontFamily = ""
	if families := m.tree.FontFamilies(); len(families) > 0 {
		m.fontFamily = families[0]
	}
	m.fontStyle = m.tree.Value("font-style", "")
	m.fontSize = ghostty.DefaultSize
	fmt.Sscanf(m.tree.Value("font-size", ""), "%g", &m.fontSize)
}

// landOnApplied shows the configured theme, or the first one when the
// config names none that is installed; the latter is only previewed.
func (m *Model) landOnApplied() {
	if t, ok := m.lib.Get(m.applied); ok {
		m.showTheme(t)
		return
	}
	if len(m.lib.Themes) > 0 {
		m.showTheme(m.lib.Themes[0])
	}
}

// ExitNote is printed after the UI tears down.
func (m *Model) ExitNote() string {
	switch {
	case m.updateNote == "":
		return m.exitNote
	case m.exitNote == "":
		return m.updateNote
	}
	return m.exitNote + "\n" + m.updateNote
}

// Init starts the spinner and the update check.
func (m *Model) Init() tea.Cmd { return tea.Batch(m.spin.Tick, m.checkUpdate()) }

// checkUpdate asks GitHub for the newest release in the background. The UI
// never waits on it: offline, it times out quietly. A recent answer is reused
// from the state file instead of asking again.
func (m *Model) checkUpdate() tea.Cmd {
	if m.opts.NoUpdateCheck || m.opts.Version == "" {
		return nil
	}
	if time.Since(m.state.UpdateCheckedAt) < update.Interval {
		latest := m.state.LatestRelease
		return func() tea.Msg { return updateMsg{latest: latest, cached: true} }
	}
	return func() tea.Msg {
		latest, err := update.Latest(context.Background())
		return updateMsg{latest: latest, err: err}
	}
}

// Messages.
type (
	writeTickMsg  struct{ seq int }
	reloadDoneMsg struct {
		seq int
		err error
	}
	fontsLoadedMsg struct{ families []ghostty.Family }
	installDoneMsg struct{ installed, failed []string }
	collectionMsg  struct {
		written, skipped int
		err              error
	}
	updateMsg struct {
		latest string
		cached bool
		err    error
	}
)

// Update dispatches to the overlay or the prompt.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, InlineHeight(msg.Height)
		m.top = msg.Height - m.height
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case writeTickMsg:
		if msg.seq != m.writeSeq {
			return m, nil
		}
		return m, m.flushWrites()
	case reloadDoneMsg:
		if msg.seq != m.reloadSeq {
			return m, nil
		}
		if msg.err != nil {
			m.reloadFailed = true
			m.warn("%v", msg.err)
		}
		return m, nil
	case fontsLoadedMsg:
		m.families = msg.families
		m.fontsLoaded, m.fontLoading = true, false
		if m.overlay == overlayBusy {
			m.overlay = overlayNone
		}
		m.recompute()
		m.landOnCurrentValue()
		return m, nil
	case installDoneMsg:
		return m, m.finishInstall(msg)
	case updateMsg:
		if msg.err != nil {
			// Offline or rate limited: try again next start.
			return m, nil
		}
		if !msg.cached {
			m.state.UpdateCheckedAt = time.Now()
			m.state.LatestRelease = msg.latest
		}
		if update.Newer(msg.latest, m.opts.Version) {
			m.updateNote = fmt.Sprintf("ghostty-config %s is available (you have %s): %s", msg.latest, m.opts.Version, update.InstallHint)
			// Only take a line that is showing the selection, never a
			// message the user may still be reading.
			if m.statusKind == statusPlain {
				m.info("ghostty-config %s is available — details on exit", msg.latest)
			}
		}
		return m, nil
	case collectionMsg:
		m.overlay = overlayNone
		if msg.err != nil {
			m.fail("Could not install the collection: %v", msg.err)
			return m, nil
		}
		m.reloadLibrary()
		m.recompute()
		if msg.skipped > 0 {
			m.info("Installed %d themes · left %d edited copies alone", msg.written, msg.skipped)
		} else {
			m.info("Installed %d curated themes — type theme and a name to find them", msg.written)
		}
		return m, nil
	case tea.MouseMsg:
		return m.updateMouse(msg)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m.abandon()
		case "ctrl+d":
			return m.quit()
		}
		if m.overlay != overlayNone {
			return m.updateOverlay(msg)
		}
		if m.peek {
			// Any key brings the palette back.
			m.peek = false
			return m, nil
		}
		if msg.String() == "f2" {
			m.peek = true
			return m, nil
		}
		if m.editing {
			return m.updateEdit(msg)
		}
		return m.updatePrompt(msg)
	}
	return m, nil
}

// View draws the frame with any overlay on top.
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 || m.quitting {
		return ""
	}
	m.regions = m.regions[:0]
	body := m.viewMain()
	if m.overlay != overlayNone {
		body = m.viewOverlay(body)
	}
	return m.fillScreen(body)
}

// fillScreen pads the frame to the terminal size so every cell carries the
// chrome background.
func (m *Model) fillScreen(body string) string {
	lines := strings.Split(body, "\n")
	c := m.clearChrome()
	base := c.base()
	out := make([]string, 0, m.height)
	for i := 0; i < m.height; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, rebase(c.fill(line, m.width), base))
	}
	return strings.Join(out, "\n")
}

// --- status ---------------------------------------------------------------

// statusKind is the tone of the status line; the colour comes from the
// interface in effect when it is drawn.
type statusKind int

const (
	statusPlain statusKind = iota
	statusInfo
	statusWarn
	statusFail
)

func (m *Model) setStatus(kind statusKind, format string, a ...interface{}) {
	m.status = fmt.Sprintf(format, a...)
	m.statusKind = kind
}

func (m *Model) info(format string, a ...interface{}) { m.setStatus(statusInfo, format, a...) }
func (m *Model) warn(format string, a ...interface{}) { m.setStatus(statusWarn, format, a...) }
func (m *Model) fail(format string, a ...interface{}) { m.setStatus(statusFail, format, a...) }

// --- debounced writes -----------------------------------------------------

// scheduleWrite arms the debounce; the write happens when it fires unless a
// newer one was scheduled since.
func (m *Model) scheduleWrite() tea.Cmd {
	m.writeSeq++
	seq := m.writeSeq
	return tea.Tick(writeDebounce, func(time.Time) tea.Msg { return writeTickMsg{seq} })
}

// flushWrites saves every changed config file and asks Ghostty to reload.
func (m *Model) flushWrites() tea.Cmd {
	written, err := m.tree.Save()
	if err != nil {
		m.fail("%v", err)
		return nil
	}
	if len(written) == 0 {
		return nil
	}
	return m.reload()
}

// reload asks the running Ghostty to re-read its files, when allowed. The
// live preview already shows colours; this is what makes fonts, padding and
// the title bar follow, and what carries a theme into every other window.
func (m *Model) reload() tea.Cmd {
	// The reload is a keystroke sent to the frontmost window, so it must
	// only ever be sent from inside Ghostty.
	if m.opts.NoReload || !m.caps.inGhostty || !m.state.AutoReloadEnabled() || m.reloadFailed {
		return nil
	}
	m.reloadSeq++
	seq := m.reloadSeq
	binary := m.paths.Binary
	return func() tea.Msg { return reloadDoneMsg{seq, ghostty.Reload(binary)} }
}

// reloadHint tells the user how to reload when the tool cannot.
func (m *Model) reloadHint() string {
	return "press " + ghostty.ReloadHint(m.paths.Binary) + " in Ghostty to reload"
}

// autoReloadWorks reports whether changes reach Ghostty without help.
func (m *Model) autoReloadWorks() bool {
	return !m.opts.NoReload && m.state.AutoReloadEnabled() && !m.reloadFailed && m.caps.inGhostty
}

// --- quitting -------------------------------------------------------------

// quit leaves. Nothing is applied on the way out: a theme becomes the
// configured one only when Enter is pressed on it. Looking at themes and then
// leaving must never change the configuration. Unsaved edits are the one
// thing worth a question, because leaving would lose them.
func (m *Model) quit() (tea.Model, tea.Cmd) {
	if m.cur != nil && m.dirty {
		name := m.cur.Name
		if !m.cur.Writable() {
			name = m.suggestName()
		}
		m.confirmText = fmt.Sprintf("You have unsaved edits. Save them as %s and apply it before leaving?", name)
		m.confirmYes = func(m *Model) tea.Cmd {
			saved, ok := m.persistCurrent()
			if !ok {
				return nil
			}
			m.exitNote = "Saved and applied " + saved
			return m.leave()
		}
		m.confirmNo = func(m *Model) tea.Cmd { return m.leave() }
		m.overlay = overlayConfirm
		return m, nil
	}
	return m, m.leave()
}

// leave writes what was explicitly changed, puts the terminal's colours
// back to the configured theme if another was only being looked at, and ends.
func (m *Model) leave() tea.Cmd {
	m.writeSeq++
	written, err := m.tree.Save()
	switch {
	case err != nil:
		m.exitNote = err.Error()
	case len(written) > 0 && !m.autoReloadWorks():
		if m.exitNote != "" {
			m.exitNote += " — "
		}
		m.exitNote += m.reloadHint()
	}
	if m.cur != nil && m.cur.Name != m.applied {
		m.live.Reset()
		if t, ok := m.lib.Get(m.applied); ok {
			m.live.Show(t.Colors)
		}
		if m.exitNote == "" && m.applied != "" {
			m.exitNote = fmt.Sprintf("%s was only previewed; %s is still applied", m.cur.Name, m.applied)
		}
	}
	_ = m.state.Save()
	m.quitting = true
	m.live.Close()
	return tea.Sequence(m.reload(), tea.Quit)
}

// abandon leaves without writing anything and puts the terminal's colours
// back to what its configuration says.
func (m *Model) abandon() (tea.Model, tea.Cmd) {
	m.live.Reset()
	m.live.Close()
	_ = m.state.Save()
	m.quitting = true
	m.exitNote = "Left without writing anything"
	return m, tea.Quit
}
