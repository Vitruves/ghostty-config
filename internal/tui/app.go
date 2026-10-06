package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	uv "github.com/charmbracelet/ultraviolet"

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

// writeDebounce is how long the highlight must rest before a font or a
// setting is written and Ghostty asked to reload, so walking a list of
// families costs one rewrite rather than one per row.
var writeDebounce = 220 * time.Millisecond

// Options tune the editor from the command line.
type Options struct {
	NoReload bool
	// Plain keeps to glyphs every terminal and font can draw.
	Plain bool
	// Images shows the highlighted theme as a picture through the Kitty
	// graphics protocol. CellW and CellH override the cell size in pixels,
	// which is otherwise read from the terminal; ImgOut is where the pictures
	// are written, os.Stdout by default.
	Images       bool
	CellW, CellH float64
	ImgOut       io.Writer
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

	// browsed is whether the highlight was moved by hand since the prompt
	// last changed: opening the list of fonts must not write the first one.
	browsed bool

	// The section whose commands the empty prompt lists, and whether the
	// editor has stepped aside to show the terminal it was started from.
	group int
	peek  bool

	// What the terminal says its own colours are, where the wall of themes is
	// scrolled to, and the theme painted into the terminal for a try, if any.
	term     termColours
	galTop   int
	galAlign bool
	tried    string

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

	// Pictures of themes, when the terminal can show them, and the size of
	// a cell in pixels they are drawn for.
	imgs         *imageStore
	cellW, cellH float64
	// applyLive tints the terminal once, in the theme Enter applied.
	applyLive *ghostty.Live

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
	confirmEsc  func(*Model) tea.Cmd // nil means Esc only closes the question
	shaderSeq   int                  // which "keep this shader?" is still waiting
	busyText    string
	messageText string
	spin        spinner.Model

	// Status line.
	status     string
	statusKind statusKind
	statusSeq  int

	// Debounced writes: only the newest scheduled write survives.
	writeSeq  int
	reloadSeq int

	reloadFailed bool
	// top is the terminal row the editor's first row sits on: always the
	// first, now that it takes the whole screen.
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
	if opts.Images {
		out := opts.ImgOut
		if out == nil {
			out = os.Stdout
		}
		m.imgs = newImageStore(out)
		m.refreshCell()
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

// Init starts the spinner and the update check, asks the terminal for its
// colours, and asks to be told when it goes from light to dark.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.checkUpdate(), askTerminalColours(), tea.Raw(watchScheme))
}

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
		m.width, m.height = msg.Width, msg.Height
		m.top = 0
		if m.opts.Images {
			m.refreshCell()
		}
		return m, nil
	case tea.BackgroundColorMsg:
		m.term.bg = hexOf(msg.Color)
		return m, nil
	case tea.ForegroundColorMsg:
		m.term.fg = hexOf(msg.Color)
		return m, nil
	case uv.DarkColorSchemeEvent, uv.LightColorSchemeEvent:
		// The terminal changed its scheme: its colours are no longer the
		// ones it reported.
		return m, askTerminalColours()
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
			return m, nil
		}
		// Ghostty has re-read its files: the terminal may wear new colours.
		return m, askTerminalColours()
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
	case shaderDoneMsg:
		return m, m.finishShader(msg)
	case shaderExpireMsg:
		return m, m.expireShader(msg)
	case packDoneMsg:
		return m, m.finishPack(msg)
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
	case tea.KeyPressMsg:
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
			// Any key brings the editor back.
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

// View hands the frame to Bubble Tea. The editor takes the whole terminal on
// the alternate screen, and leaves it while peeking so that what was in the
// terminal shows again; the mouse is asked for with the view, as Bubble Tea
// v2 wants.
func (m *Model) View() tea.View {
	content := m.render()
	if m.imgs != nil {
		// The pictures go out before the frame that shows them.
		m.imgs.flush()
	}
	v := tea.NewView(content)
	v.AltScreen = !m.peek
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "ghostty-config"
	return v
}

// render draws the frame with any overlay on top.
func (m *Model) render() string {
	if m.width == 0 || m.height == 0 || m.quitting {
		return ""
	}
	m.regions = m.regions[:0]
	if m.peek {
		// One line, on the terminal's own screen, under what was there.
		ui := m.panelChrome()
		return rebase(ui.fill(ui.base().Render(" ")+ui.keyHintFit(m.width-2, "any key", "brings ghostty-config back"), m.width), ui.base())
	}
	body := m.viewMain()
	if m.overlay != overlayNone {
		body = m.viewOverlay(body)
	}
	return m.fillScreen(body)
}

// fillScreen pads the frame to the terminal size so every cell carries the
// interface's colours.
func (m *Model) fillScreen(body string) string {
	lines := strings.Split(body, "\n")
	c := m.panelChrome()
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
	m.statusSeq++
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
	return func() tea.Msg { return reloadDoneMsg{seq, reloadGhostty(binary)} }
}

// reloadGhostty sends the reload keystroke; a variable so tests never send one.
var reloadGhostty = ghostty.Reload

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
		m.confirmEsc = nil
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
	if m.tried != "" {
		// A theme was painted into the terminal for a try: put back what the
		// configuration says.
		m.live.Reset()
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
	return tea.Sequence(m.reload(), tea.Raw(unwatchScheme), tea.Quit)
}

// abandon leaves without writing anything and puts the terminal's colours
// back to what its configuration says.
func (m *Model) abandon() (tea.Model, tea.Cmd) {
	m.live.Reset()
	m.live.Close()
	_ = m.state.Save()
	m.quitting = true
	m.exitNote = "Left without writing anything"
	return m, tea.Sequence(tea.Raw(unwatchScheme), tea.Quit)
}
