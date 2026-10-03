package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/fontdl"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// commands is built lazily so its closures can reach the model.
func (m *Model) commands() []*command {
	if m.cmds == nil {
		m.cmds = m.buildCommands()
	}
	return m.cmds
}

// resolve reads the prompt: the first word names a command, exactly or by a
// unique prefix; the rest is its argument. Anything else shows the menu.
func (m *Model) resolve(text string) (*command, string) {
	text = strings.TrimLeft(text, " ")
	word, rest, _ := strings.Cut(text, " ")
	word = strings.ToLower(word)
	if word == "" {
		return nil, ""
	}
	var prefix *command
	matches := 0
	for _, c := range m.commands() {
		if c.name == word {
			return c, strings.TrimLeft(rest, " ")
		}
		if strings.HasPrefix(c.name, word) {
			prefix = c
			matches++
		}
	}
	if matches == 1 && strings.Contains(text, " ") {
		// "the " resolves to theme once a space commits to it.
		return prefix, strings.TrimLeft(rest, " ")
	}
	return nil, ""
}

// recompute rebuilds the results for the prompt text, keeping the highlight
// on the same value when it survives.
func (m *Model) recompute() {
	text := m.prompt.Value()
	keep := m.selectedValue()
	m.cmd, m.arg = m.resolve(text)
	if m.cmd != nil {
		for i, g := range groups {
			if g == m.cmd.group {
				m.group = i
			}
		}
	}
	if m.cmd == nil {
		m.results = m.menuOptions(strings.TrimSpace(text))
	} else if m.cmd.options == nil {
		m.results = nil
	} else {
		m.results = m.cmd.options(m, m.arg)
		if fam := familyName(m.arg); m.cmd.filters && fam != "" && themeCommand(m.cmd) {
			// A family name filters by family, in the order of the list.
			var kept []option
			for _, o := range m.results {
				if o.header || (o.theme != nil && strings.EqualFold(themeFamily(o.theme), fam)) {
					kept = append(kept, o)
				}
			}
			m.results = kept
		} else if m.cmd.filters && strings.TrimSpace(m.arg) != "" {
			needle := strings.TrimSpace(m.arg)
			var kept []option
			for _, o := range m.results {
				if o.header || fuzzyMatch(o.label, needle) {
					kept = append(kept, o)
				}
			}
			sort.SliceStable(kept, func(a, b int) bool { return fuzzyScore(kept[a].label, needle) < fuzzyScore(kept[b].label, needle) })
			m.results = kept
		}
	}
	m.sel.cursor = 0
	if keep != "" {
		for i, o := range m.results {
			if o.value == keep && !o.header {
				m.sel.cursor = i
				break
			}
		}
	}
	// A command opened with nothing typed after it lands on what is in
	// effect, never on the first row: for themes that is the theme on
	// screen, so opening the list previews nothing new.
	if m.cmd != nil && strings.TrimSpace(m.arg) == "" {
		m.landOnCurrentValue()
	}
	m.skipHeaders(1)
}

// landOnCurrentValue puts the highlight on the option marked current.
func (m *Model) landOnCurrentValue() {
	land := func(i int) {
		m.sel.cursor = i
		// Show it with its neighbours rather than at the edge of the list.
		m.centre = true
	}
	if m.cur != nil {
		for i, o := range m.results {
			if o.theme != nil && o.cmd == nil && o.theme.Name == m.cur.Name {
				land(i)
				return
			}
		}
	}
	for i, o := range m.results {
		if o.current {
			land(i)
			return
		}
	}
}

// title capitalises a command word. Commands are written with a capital so
// they never read as a theme, most of which are lower case.
func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// menuOptions is what an empty or unresolved prompt shows. With nothing
// typed it is one group at a time, each command with what it is set to now,
// so the list stays short; typing searches every group at once.
func (m *Model) menuOptions(filter string) []option {
	var out []option
	if filter == "" {
		for _, c := range m.commands() {
			if c.group == groups[m.group] {
				o := option{label: title(c.name), detail: m.currentValueOf(c), value: c.name, cmd: c}
				if c.name == "theme" && m.cur != nil {
					// The theme row carries its colours, like every theme does.
					o.theme = m.cur
					o.detail = m.cur.Name
				}
				out = append(out, o)
			}
		}
		return out
	}
	for _, c := range m.commands() {
		if menuMatches(c, filter) {
			out = append(out, option{label: title(c.name), detail: c.group, value: c.name, cmd: c})
		}
	}
	return out
}

// switchGroup shows another group of the menu.
func (m *Model) switchGroup(delta int) {
	m.group = (m.group + delta + len(groups)) % len(groups)
	m.recompute()
	m.sel.cursor, m.sel.offset = 0, 0
}

// jumpToGroup clears the prompt and shows a group by name.
func (m *Model) jumpToGroup(title string) {
	for i, g := range groups {
		if g == title {
			m.group = i
		}
	}
	m.setPromptText("")
	m.sel.cursor, m.sel.offset = 0, 0
}

func (m *Model) selected() *option {
	if m.sel.cursor < 0 || m.sel.cursor >= len(m.results) || m.results[m.sel.cursor].header {
		return nil
	}
	return &m.results[m.sel.cursor]
}

func (m *Model) selectedValue() string {
	if o := m.selected(); o != nil {
		return o.value
	}
	return ""
}

// skipHeaders moves the cursor off a group title in the given direction.
func (m *Model) skipHeaders(dir int) {
	n := len(m.results)
	for i := 0; i < n && m.sel.cursor >= 0 && m.sel.cursor < n && m.results[m.sel.cursor].header; i++ {
		m.sel.cursor += dir
		if m.sel.cursor < 0 {
			m.sel.cursor = n - 1
		}
		if m.sel.cursor >= n {
			m.sel.cursor = 0
		}
	}
}

// move steps the highlight and fires the command's live preview.
func (m *Model) move(delta int) tea.Cmd {
	if len(m.results) == 0 {
		return nil
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	m.sel.cursor += delta
	if m.sel.cursor < 0 {
		m.sel.cursor = 0
	}
	if m.sel.cursor >= len(m.results) {
		m.sel.cursor = len(m.results) - 1
	}
	m.skipHeaders(dir)
	m.sel.clamp(len(m.results), m.resultsHeight())
	return m.afterMove()
}

func (m *Model) afterMove() tea.Cmd {
	m.previewScrl = 0
	var cmds []tea.Cmd
	if m.cmdNeedsFonts() {
		cmds = append(cmds, m.ensureFonts())
	}
	if m.cmd != nil && m.cmd.onMove != nil {
		cmds = append(cmds, m.cmd.onMove(m, m.selected()))
	}
	return tea.Batch(cmds...)
}

// cmdNeedsFonts reports whether the active command lists fonts.
func (m *Model) cmdNeedsFonts() bool {
	if m.cmd == nil {
		return false
	}
	switch m.cmd.name {
	case "font", "style", "download":
		return true
	}
	return false
}

// setPromptText replaces the prompt and recomputes.
func (m *Model) setPromptText(text string) {
	m.prompt.SetValue(text)
	m.prompt.CursorEnd()
	m.recompute()
}

// complete is Tab: a menu entry becomes the command word; an option fills
// the argument so Enter runs with exactly it.
func (m *Model) complete() {
	o := m.selected()
	if o == nil {
		return
	}
	if o.cmd != nil {
		m.setPromptText(title(o.cmd.name) + " ")
		return
	}
	if m.cmd != nil {
		m.setPromptText(title(m.cmd.name) + " " + o.value)
		for i, r := range m.results {
			if r.value == o.value {
				m.sel.cursor = i
			}
		}
	}
}

// run is Enter.
func (m *Model) run() tea.Cmd {
	o := m.selected()
	if m.cmd == nil {
		if o != nil && o.cmd != nil {
			m.setPromptText(title(o.cmd.name) + " ")
			if o.cmd.options == nil {
				// A command with nothing to choose runs at once.
				cmd := o.cmd.run(m, "", nil)
				m.setPromptText("")
				return cmd
			}
			return m.afterMove()
		}
		return nil
	}
	active := m.cmd
	seq := m.statusSeq
	cmd := active.run(m, m.arg, o)
	// The command may have rewritten the prompt itself (save proposing a
	// name, saveAs clearing it); only tidy up when it did not.
	if m.cmd == active && !active.keepArg && !m.editing {
		if active.options == nil {
			// A one-shot command is done; hand the prompt back.
			m.setPromptText("")
		} else if m.prompt.Value() != title(active.name)+" " {
			// Keep the command, drop the argument, so the next choice is a keystroke away.
			m.setPromptText(title(active.name) + " ")
		}
	}
	if m.statusSeq == seq {
		// A command that said something keeps its message on screen.
		m.renderStatus()
	}
	return cmd
}

// updatePrompt handles keys while the prompt owns the keyboard.
func (m *Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.prompt.Value() != "" {
			m.setPromptText("")
			m.renderStatus()
			return m, nil
		}
		return m.quit()
	case "enter":
		return m, m.run()
	case "tab", "shift+tab":
		// On the menu, Tab walks the groups; inside a command it completes.
		if m.cmd == nil && strings.TrimSpace(m.prompt.Value()) == "" {
			m.switchGroup(map[string]int{"tab": 1, "shift+tab": -1}[msg.String()])
			return m, nil
		}
		// A bare command word, as the tool opens on, has nothing to complete:
		// Tab leaves it for the neighbouring group, as it does on the menu.
		if m.cmd != nil && strings.EqualFold(strings.TrimSpace(m.prompt.Value()), m.cmd.name) {
			for i, g := range groups {
				if g == m.cmd.group {
					m.group = i
				}
			}
			m.setPromptText("")
			m.switchGroup(map[string]int{"tab": 1, "shift+tab": -1}[msg.String()])
			return m, m.afterMove()
		}
		m.complete()
		return m, m.afterMove()
	case "up", "ctrl+p":
		return m, m.move(-1)
	case "down", "ctrl+n":
		return m, m.move(1)
	case "pgup":
		return m, m.move(-m.resultsHeight())
	case "pgdown":
		return m, m.move(m.resultsHeight())
	case "left", "right":
		// On an empty prompt there is no text to move through, so the
		// arrows walk the columns of the menu instead.
		if m.prompt.Value() == "" && m.cmd == nil {
			m.switchGroup(map[string]int{"left": -1, "right": 1}[msg.String()])
			return m, nil
		}
	case "home":
		if m.prompt.Value() == "" || m.prompt.Position() == 0 {
			m.sel.cursor = 0
			m.skipHeaders(1)
			return m, m.afterMove()
		}
	case "end":
		if m.prompt.Value() == "" || m.prompt.Position() == len(m.prompt.Value()) {
			m.sel.cursor = len(m.results) - 1
			m.skipHeaders(-1)
			return m, m.afterMove()
		}
	case "f1", "ctrl+_":
		m.overlay = overlayHelp
		return m, nil
	case "ctrl+x":
		// Delete the highlighted theme file, from any list of themes.
		if o := m.selected(); m.cmd != nil && o != nil && o.theme != nil {
			m.deleteThemes([]*ghostty.Theme{o.theme})
			return m, nil
		}
	case "ctrl+u":
		m.setPromptText("")
		return m, nil
	case "shift+up", "shift+down":
		m.previewScrl += map[string]int{"shift+up": -3, "shift+down": 3}[msg.String()]
		if m.previewScrl < 0 {
			m.previewScrl = 0
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.prompt, cmd = m.prompt.Update(msg)
	if text := m.prompt.Value(); text != m.lastText {
		m.lastText = text
		m.recompute()
		m.renderStatus()
		return m, tea.Batch(cmd, m.afterMove())
	}
	return m, cmd
}

// --- palette edit mode ----------------------------------------------------

// startEdit turns the prompt into a slot editor.
func (m *Model) startEdit(key string) {
	if m.cur == nil {
		return
	}
	m.editing = true
	m.editKey = key
	m.editText = ""
	if m.cur.Colors[key] == "" {
		m.setColor(key, m.derivedDefault(key))
	}
	m.renderStatus()
}

func (m *Model) stopEdit() {
	m.editing = false
	m.editText = ""
	m.recompute()
	m.renderStatus()
}

// updateEdit handles keys in the slot editor: arrows nudge, hex digits
// build an exact value, Esc or Enter return to the prompt.
func (m *Model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := m.editKey
	switch msg.String() {
	case "esc", "enter":
		m.stopEdit()
		return m, nil
	case "up":
		m.moveEditSlot(-1)
		return m, nil
	case "down":
		m.moveEditSlot(1)
		return m, nil
	case "left":
		m.adjust(key, color.AdjustBrightness, false)
	case "right":
		m.adjust(key, color.AdjustBrightness, true)
	case "shift+left":
		m.adjust(key, color.AdjustHue, false)
	case "shift+right":
		m.adjust(key, color.AdjustHue, true)
	case "-", "_":
		m.adjust(key, color.AdjustSaturation, false)
	case "+", "=":
		m.adjust(key, color.AdjustSaturation, true)
	case "[":
		m.adjust(key, color.AdjustLightness, false)
	case "]":
		m.adjust(key, color.AdjustLightness, true)
	case "u", "U":
		m.undoColor(key)
	case "backspace":
		if m.editText != "" {
			m.editText = m.editText[:len(m.editText)-1]
		} else if !strings.HasPrefix(key, "palette.") && key != "background" && key != "foreground" {
			m.clearColor(key)
			m.info("Cleared %s — Ghostty will pick its own", ghostty.Label(key))
		}
	default:
		r := msg.String()
		if len(r) == 1 && strings.ContainsAny(r, "0123456789abcdefABCDEF#") {
			if r == "#" {
				m.editText = ""
			} else {
				m.editText += strings.ToLower(r)
			}
			if len(m.editText) == 6 || len(m.editText) == 3 {
				if hex := color.Normalize(m.editText, ""); hex != "" && len(m.editText) == 6 {
					m.setColor(key, hex)
					m.editText = ""
				}
			}
		}
	}
	m.recompute()
	m.renderStatus()
	return m, nil
}

// moveEditSlot walks to the neighbouring slot while staying in edit mode.
func (m *Model) moveEditSlot(delta int) {
	keys := ghostty.ColorKeys
	for i, k := range keys {
		if k == m.editKey {
			j := (i + delta + len(keys)) % len(keys)
			m.editKey = keys[j]
			m.editText = ""
			if m.cur.Colors[m.editKey] == "" {
				m.setColor(m.editKey, m.derivedDefault(m.editKey))
			}
			for r, o := range m.results {
				if o.slot == m.editKey {
					m.sel.cursor = r
				}
			}
			return
		}
	}
}

// --- theme helpers used by commands --------------------------------------

// previewTheme shows a theme in this window without writing anything.
func (m *Model) previewTheme(t *ghostty.Theme) {
	if m.cur != nil && m.cur.Name == t.Name && m.cur.Source != ghostty.SourceDraft {
		return
	}
	if m.dirty && !m.discardArmed {
		m.discardArmed = true
		m.warn("%s has unsaved edits — save keeps them, undo drops them, moving again leaves them behind", m.cur.Name)
		return
	}
	m.showTheme(t)
	m.renderStatus()
}

// generateDraft builds a candidate from the new command's argument.
func (m *Model) generateDraft(method int, arg string) {
	dark := true
	if m.cur != nil {
		dark = m.cur.IsDark()
	}
	hue, seeded := 0.0, false
	for _, w := range strings.Fields(strings.ToLower(arg)) {
		switch {
		case w == "light":
			dark = false
		case w == "dark":
			dark = true
		case color.IsHex(w):
			rgb, _ := color.ParseHex(w)
			hue, seeded = rgb.ToHSL().H, true
		}
	}
	var s *color.Scheme
	note := "Generated · random"
	if method == 0 {
		s = color.Random(dark, hue, seeded)
	} else {
		s = color.Harmonious(dark, color.Harmony(method-1), hue, seeded)
		note = "Generated · " + strings.ToLower(color.HarmonyNames[method-1]) + " harmony"
	}
	m.loadDraft(s, note)
	m.info("Generated %s — Enter again for another, save <name> keeps it, undo goes back", s.Name)
}

// --- fonts ----------------------------------------------------------------

// ensureFonts starts the scan the first time a font command is used.
func (m *Model) ensureFonts() tea.Cmd {
	if m.fontsLoaded || m.fontLoading {
		return nil
	}
	m.fontLoading = true
	catalog := m.fonts
	return func() tea.Msg { return fontsLoadedMsg{catalog.Families()} }
}

func (m *Model) currentStyle() string {
	if m.fontStyle == "" || m.fontStyle == "default" {
		return "Regular"
	}
	return m.fontStyle
}

// pickFamily makes a family the one Ghostty draws with, keeping a style the
// family has and dropping one it lacks.
func (m *Model) pickFamily(f ghostty.Family) tea.Cmd {
	if f.Name == m.fontFamily {
		return nil
	}
	m.fontFamily = f.Name
	m.tree.SetPrimaryFont(f.Name)
	has := false
	for _, s := range f.Styles {
		if strings.EqualFold(s, m.currentStyle()) {
			has = true
		}
	}
	if !has {
		m.setFontStyle("Regular")
	}
	return m.scheduleWrite()
}

// setFontStyle writes font-style, or removes it for the regular face, which
// is what Ghostty picks on its own.
func (m *Model) setFontStyle(style string) {
	m.fontStyle = style
	if style == "Regular" || style == "" {
		m.tree.Unset("font-style", "disabled by ghostty-config: the regular style is the default")
		return
	}
	m.tree.Set("font-style", style)
}

func (m *Model) setSize(size float64) tea.Cmd {
	size = color.Clamp(size, ghostty.FontSizeMin, ghostty.FontSizeMax)
	if size == m.fontSize {
		return nil
	}
	m.fontSize = size
	m.tree.Set("font-size", ghostty.FormatSize(size))
	return m.scheduleWrite()
}

// fontSizes are the sizes offered: half points through the range people
// read at, whole points once the exact value stops mattering.
var fontSizes = buildFontSizes()

func buildFontSizes() []float64 {
	var out []float64
	for s := ghostty.FontSizeMin; s <= 20; s += ghostty.FontSizeStep {
		out = append(out, s)
	}
	for s := 21.0; s <= ghostty.FontSizeMax; s++ {
		out = append(out, s)
	}
	return out
}

// findFamily looks a family up in the loaded list.
func (m *Model) findFamily(name string) (ghostty.Family, bool) {
	for _, f := range m.families {
		if f.Name == name {
			return f, true
		}
	}
	return ghostty.Family{}, false
}

// installedDownloadables is the set of downloadable families already on the
// system, matched by the display name being a prefix of the installed one:
// "JetBrains Mono" is installed as "JetBrainsMono Nerd Font".
func (m *Model) installedDownloadables() map[string]bool {
	present := make(map[string]bool)
	for _, candidate := range fontdl.AvailableDownloads {
		compact := strings.ToLower(strings.ReplaceAll(candidate.Name, " ", ""))
		for _, f := range m.families {
			if strings.HasPrefix(strings.ToLower(strings.ReplaceAll(f.Name, " ", "")), compact) {
				present[candidate.Name] = true
				break
			}
		}
	}
	return present
}

// installFonts downloads each candidate in turn behind the spinner. One
// archive failing does not stop the rest; failures are reported at the end.
func (m *Model) installFonts(candidates []fontdl.FontDownload) tea.Cmd {
	m.busyText = fmt.Sprintf("Downloading %d font archive(s) from Nerd Fonts…", len(candidates))
	m.overlay = overlayBusy
	catalog := m.fonts
	return func() tea.Msg {
		var installed, failed []string
		for _, candidate := range candidates {
			if err := fontdl.Install(candidate, nil); err != nil {
				failed = append(failed, candidate.Name)
				continue
			}
			installed = append(installed, candidate.Name)
		}
		catalog.Refresh()
		return installDoneMsg{installed, failed}
	}
}

func (m *Model) finishInstall(msg installDoneMsg) tea.Cmd {
	m.overlay = overlayNone
	if len(msg.installed) == 0 {
		m.fail("Nothing installed: %s", strings.Join(msg.failed, ", "))
		return nil
	}
	switch {
	case len(msg.failed) > 0:
		m.warn("Installed %d — %s failed", len(msg.installed), strings.Join(msg.failed, ", "))
	case len(msg.installed) == 1:
		m.info("Installed %s — type font and its name to use it", msg.installed[0])
	default:
		m.info("Installed %d font families — type font to browse them", len(msg.installed))
	}
	m.fontsLoaded = false
	return m.ensureFonts()
}

// pathsText describes where everything is.
func (m *Model) pathsText() string {
	var b strings.Builder
	b.WriteString("Config files Ghostty loads, in order (later wins):\n")
	for _, d := range m.tree.Docs {
		b.WriteString("  " + d.Path + "\n")
	}
	b.WriteString("New keys go to:\n  " + m.tree.Primary.Path + "\n")
	b.WriteString("Your themes:\n  " + m.paths.ThemesDir + "\n")
	b.WriteString("Favourites, settings, backups:\n  " + m.paths.StateDir)
	if m.paths.Binary != "" {
		b.WriteString("\nGhostty binary:\n  " + m.paths.Binary)
	}
	return b.String()
}

// resultsHeight is how many result rows fit.
func (m *Model) resultsHeight() int {
	return m.layout().listH
}

// sizeIndex finds a size in the list, for the wheel.
func sizeIndex(size float64) int {
	for i, s := range fontSizes {
		if s == size {
			return i
		}
	}
	return -1
}

func atoiDefault(s string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return v
	}
	return def
}

// menuMatches keeps the menu filter tight: the command name by prefix or
// substring, or a word of its description, never a scattered subsequence.
func menuMatches(c *command, filter string) bool {
	f := strings.ToLower(strings.TrimSpace(filter))
	if strings.Contains(c.name, f) {
		return true
	}
	for _, w := range strings.Fields(strings.ToLower(c.desc)) {
		if strings.HasPrefix(strings.Trim(w, ",.;:()"), f) {
			return true
		}
	}
	return false
}
