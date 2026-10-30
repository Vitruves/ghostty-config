package tui

import (
	"context"
	"errors"
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
	overlayUpdate
	overlayCapture // the shortcut editor asking for a shortcut
	overlayExplain // Ghostty's own documentation of a setting or an action
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
	// Pixel draws the whole interface as pictures, which needs Images and
	// the exact size of a cell, PixCell, in pixels.
	Pixel   bool
	PixCell [2]int
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
	// painted is whether any colour was pushed into the terminal this
	// session, by a try or by Enter, so that every way out puts them back;
	// appliedHere is whether a theme was applied this session.
	painted     bool
	appliedHere bool
	// interfaceWas is the interface in effect before the list of them was
	// walked, put back unless Enter chose one; previewingInterface says a
	// walk is under way.
	interfaceWas        string
	previewingInterface bool

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
	// px draws the picture interface; nil for the text one.
	px *pixEngine
	// imgReady is whether the alternate screen is up, so that pictures sent
	// now land on the screen that shows them; imgSeq tells the newest wait
	// from older ones.
	imgReady bool
	imgSeq   int
	// applyLive tints the terminal once, in the theme Enter applied.
	applyLive *ghostty.Live

	// Fonts.
	families    []ghostty.Family
	fontsLoaded bool
	fontLoading bool
	// specimens are the font files the picture interface has read, by file.
	specimens  map[string]specimenFont
	fontFamily string
	fontStyle  string
	fontSize   float64
	// fx is what the font in use offers; files what the file settings found.
	fx    fontExtras
	files fileState

	// The shortcut editor: Ghostty's actions, and the shortcut being asked for.
	kb keyboard

	// Ghostty's own documentation of the highlighted line, while it is open.
	ex explainView

	// Mouse: what is clickable on the current frame, and the last click.
	regions   []region
	lastClick clickMemory

	// Dialogs.
	confirmText string
	confirmYes  func(*Model) tea.Cmd
	confirmNo   func(*Model) tea.Cmd // nil means "no" only closes the question
	confirmEsc  func(*Model) tea.Cmd // nil means Esc only closes the question
	// confirmSeq tells each question asked from the ones before it, so that a
	// timer meant for one never answers another.
	confirmSeq int
	// shaderRevert puts the previous shader back while "keep this shader?"
	// waits for an answer; nil otherwise.
	shaderRevert func(*Model) tea.Cmd
	busyText     string
	messageText  string
	spin         spinner.Model

	// Status line.
	status     string
	statusKind statusKind
	statusSeq  int

	// Debounced writes: only the newest scheduled write survives.
	writeSeq  int
	reloadSeq int

	// What Ghostty said of the config when last asked, and whether it has
	// been; validateSeq tells the newest question from older ones, whose
	// answers describe files that changed since. checkOpened is whether the
	// check list, open now, has asked already.
	problems    []ghostty.Problem
	checked     bool
	checking    bool
	checkErr    error
	checkTook   time.Duration
	checkOpened bool
	validateSeq int
	// startText is every config file as the session found it, which the
	// changes command compares with.
	startText map[string][]string
	// restoreList is the backups the restore list was built from.
	restoreList []restorable
	// sshHosts is the SSH terminfo cache as last read, when, and what went
	// wrong reading it; the list and its panel share the one read.
	sshHosts    []ghostty.SSHCacheEntry
	sshHostsErr error
	sshHostsAt  time.Time

	reloadFailed bool
	// top is the terminal row the editor's first row sits on: always the
	// first, now that it takes the whole screen.
	top        int
	exitNote   string
	updateNote string // a newer release, told again on the way out
	// updateLatest is the newer release on offer; updatePending holds its
	// dialog back while another one is open.
	updateLatest  string
	updatePending bool
	quitting      bool
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
		if opts.Pixel && opts.PixCell[0] > 0 && opts.PixCell[1] > 0 {
			m.px = newPixEngine(opts)
		}
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
	m.startText = tree.Texts()
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
// colours, and asks to be told when it goes from light to dark. Ghostty is
// asked about the config as it is, so that a later write is told only of the
// problems it brought. When the config names a font, the families are
// listed in the background, so that a family that is missing or
// proportional can be told.
func (m *Model) Init() tea.Cmd {
	var fonts tea.Cmd
	if m.fontFamily != "" {
		fonts = m.ensureFonts()
	}
	return tea.Batch(m.spin.Tick, m.checkUpdate(), askTerminalColours(), tea.Raw(watchScheme), m.runCheck(checkQuiet), fonts)
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
	updateInstalledMsg struct {
		version, path string
		err           error
	}
)

// Update dispatches to the overlay or the prompt.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	// An update offered while another dialog was open, or while the editor
	// stepped aside, is shown as soon as both are over, whichever way the
	// dialog was closed.
	m.showPendingUpdate()
	if load := m.wantSpecimen(); load != nil {
		cmd = tea.Batch(cmd, load)
	}
	m.syncFontCommands()
	m.hintFontTrouble()
	for _, load := range []tea.Cmd{m.wantFontInfo(), m.wantThumbs()} {
		if load != nil {
			cmd = tea.Batch(cmd, load)
		}
	}
	return model, cmd
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.top = 0
		if m.opts.Images {
			m.refreshCell()
		}
		// The renderer clears the whole screen after every resize, and the
		// terminal drops every picture with it: send them all again once it
		// has, at the cell size the terminal reports now.
		if m.px != nil {
			// The layout changes with the size: bring the highlight and its
			// group back into view.
			m.centre = true
			return m, tea.Batch(tea.Raw(askCellSize), m.armImages())
		}
		return m, m.armImages()
	case uv.CellSizeEvent:
		if m.px != nil && m.setPixCell(msg.Width, msg.Height) {
			return m, m.armImages()
		}
		return m, nil
	case imgReadyMsg:
		if msg.seq == m.imgSeq && m.imgs != nil {
			// Whatever was sent before went to the other screen: send it all
			// again, here.
			m.imgs.reset()
			m.imgReady = true
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
	case validateTickMsg:
		if msg.seq != m.validateSeq {
			return m, nil
		}
		return m, m.runCheck(checkWrite)
	case validateDoneMsg:
		m.finishCheck(msg)
		return m, nil
	case reloadDoneMsg:
		if msg.seq != m.reloadSeq {
			return m, nil
		}
		if errors.Is(msg.err, ghostty.ErrNotFrontmost) {
			// Another app was in front, so no keystroke was sent: the next
			// change tries again, and this one needs a hand.
			m.warn("Ghostty was not in front: %s", m.reloadHint())
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
		if after := m.fx.afterFonts; after != nil {
			m.fx.afterFonts = nil
			return m, after(m)
		}
		return m, nil
	case explainMsg:
		m.explainLoaded(msg)
		return m, nil
	case keysLoadedMsg:
		m.kb.cat, m.kb.loaded, m.kb.loading = msg.cat, true, false
		if m.cmd != nil && m.cmd.name == "keys" {
			m.recompute()
			if !msg.cat.DefaultsKnown {
				m.warn("Ghostty's binary was not found: its default shortcuts are not shown")
			}
		}
		return m, nil
	case specimenMsg:
		for k, v := range msg.fonts {
			if _, wanted := m.specimens[k]; wanted {
				m.specimens[k] = v
			}
		}
		return m, nil
	case fontInfoMsg:
		if _, wanted := m.fx.infos[msg.key]; wanted {
			m.fx.infos[msg.key] = msg.info
		}
		return m, nil
	case thumbMsg:
		for k, v := range msg.thumbs {
			if _, wanted := m.files.thumbs[k]; wanted {
				m.files.thumbs[k] = v
			}
		}
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
			m.offerUpdate(msg.latest)
		}
		return m, nil
	case updateInstalledMsg:
		return m, m.finishUpdate(msg)
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
		switch msg.(type) {
		case tea.MouseClickMsg:
			m.tree.Checkpoint("")
		case tea.MouseWheelMsg:
			m.tree.Checkpoint(m.stepKey())
		}
		return m.updateMouse(msg)
	case tea.KeyPressMsg:
		// Every key the user presses begins a step of the session's history,
		// so that whatever it writes is undone in one go.
		m.tree.Checkpoint(m.stepKeyFor(msg.String()))
		if m.overlay == overlayCapture && !m.peek && msg.String() != "ctrl+c" {
			// Asking for a shortcut, every key is a candidate, ctrl+d too.
			return m.updateCapture(msg)
		}
		switch msg.String() {
		case "ctrl+c":
			return m.abandon()
		case "ctrl+d":
			return m.quit()
		}
		if m.peek {
			// Any key brings the editor back, on a fresh alternate screen. It
			// does nothing else: a dialog that opened meanwhile was not on
			// screen, and must be seen before a key can answer it.
			m.peek = false
			return m, m.armImages()
		}
		if m.overlay != overlayNone {
			return m.updateOverlay(msg)
		}
		if msg.String() == "f2" {
			m.peek = true
			return m, nil
		}
		if msg.String() == "f3" {
			return m, m.openExplain()
		}
		if msg.String() == "ctrl+z" {
			return m, m.undoStep()
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
		// The pictures go out before the frame that shows them, once the
		// screen that shows them is up.
		if m.imgReady {
			m.imgs.flush()
		} else {
			m.imgs.queue.Reset()
		}
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
	if m.pixelOn() {
		if !m.imgReady {
			// A moment of plain ground while the alternate screen comes up.
			return m.fillScreen("")
		}
		return m.renderPixel()
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
	return tea.Batch(m.reload(), m.scheduleValidate())
}

// reload asks the running Ghostty to re-read its files, when allowed. The
// live preview already shows colours; this is what makes fonts, padding and
// the title bar follow, and what carries a theme into every other window.
func (m *Model) reload() tea.Cmd {
	// Before Ghostty 1.3 the reload is a keystroke sent to the frontmost
	// window, so it must only ever be sent from inside Ghostty.
	if m.opts.NoReload || !m.caps.inGhostty || !m.state.AutoReloadEnabled() || m.reloadFailed {
		return nil
	}
	m.reloadSeq++
	seq := m.reloadSeq
	binary := m.paths.Binary
	return func() tea.Msg { return reloadDoneMsg{seq, reloadGhostty(binary)} }
}

// reloadGhostty asks Ghostty to reload; a variable so tests never do.
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
	// A shader still waiting for "keep it?" is put back before anything else:
	// leaving is no answer, and its timer must not answer the next question.
	unkept, _ := m.dropUnkeptShader()
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
		m.confirmSeq++
		return m, unkept
	}
	return m, tea.Sequence(unkept, m.leave())
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
	m.RestoreColours()
	if m.cur != nil && m.cur.Name != m.applied && m.exitNote == "" && m.applied != "" {
		m.exitNote = fmt.Sprintf("%s was only previewed; %s is still applied", m.cur.Name, m.applied)
	}
	m.endInterfacePreview()
	_ = m.state.Save()
	m.quitting = true
	var reload tea.Cmd
	if len(written) > 0 {
		// Only a file that changed is worth a reload.
		reload = m.reload()
	}
	return tea.Sequence(reload, tea.Raw(unwatchScheme), m.dropPictures(), tea.Quit)
}

// RestoreColours puts the terminal's colours back to what its configuration
// says when anything was painted into it this session, whichever way the
// editor ends: main calls it as well when the program is interrupted, killed
// or panics. Where Ghostty is not re-reading its files, the window still
// knows its old theme, so a theme applied here is shown again on top. It is
// safe to call more than once.
func (m *Model) RestoreColours() {
	if !m.painted {
		return
	}
	m.painted = false
	m.live.Reset()
	m.applyLive.Reset()
	if m.caps.inGhostty && m.appliedHere && !m.autoReloadWorks() {
		if t, ok := m.lib.Get(m.applied); ok {
			live := m.applyLive
			if live == nil {
				live = m.live
			}
			live.Show(t.Colors)
		}
	}
	m.live.Close()
	m.applyLive.Close()
	m.live, m.applyLive = nil, nil
}

// abandon leaves without writing anything and puts the terminal's colours
// back to what its configuration says.
func (m *Model) abandon() (tea.Model, tea.Cmd) {
	m.exitNote = "Left without writing anything"
	// A shader waiting for "keep it?" was written already: put it back and
	// say so, rather than leave it behind a note that nothing was written.
	unkept, dropped := m.dropUnkeptShader()
	if dropped {
		m.exitNote = "Left; the shader that was not kept is put back, nothing else was written"
	}
	m.RestoreColours()
	m.endInterfacePreview()
	_ = m.state.Save()
	m.quitting = true
	return m, tea.Sequence(unkept, tea.Raw(unwatchScheme), m.dropPictures(), tea.Quit)
}
