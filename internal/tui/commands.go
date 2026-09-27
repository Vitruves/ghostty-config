package tui

import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vitruves/ghostty-config/internal/collection"
	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/fontdl"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// option is one line of the results list: what Enter would do.
type option struct {
	label   string
	detail  string
	value   string
	theme   *ghostty.Theme
	family  *ghostty.Family
	slot    string
	swatch  string
	current bool
	header  bool // a group title in the menu, not selectable
	cmd     *command
}

// command is one word the prompt understands. Typing it shows its options;
// Enter runs it with the highlighted option or with whatever was typed.
type command struct {
	name   string
	group  string
	syntax string
	desc   string
	// options lists what the command accepts, already filtered by arg when
	// filters is false. nil means the command takes no list.
	options func(m *Model, arg string) []option
	filters bool
	// run executes the command. opt is the highlighted option, nil when the
	// list is empty or the command has none; arg is the typed remainder.
	run func(m *Model, arg string, opt *option) tea.Cmd
	// onMove fires when the highlight moves: the live preview of a theme, or
	// the debounced write of a font.
	onMove func(m *Model, opt *option) tea.Cmd
	// preview draws the right panel for the highlighted option.
	preview func(m *Model, opt *option, width int) []string
	// keepArg leaves the typed argument alone after Enter; otherwise the
	// prompt is cleared back to the command word.
	keepArg bool
}

// Groups, in the order the menu shows them.
var groups = []string{"Themes", "Fonts", "Window", "Tool"}

// commands is the registry, built once per model because the closures need
// platform facts.
func (m *Model) buildCommands() []*command {
	var cmds []*command
	add := func(c *command) { cmds = append(cmds, c) }

	// --- Themes -----------------------------------------------------------
	add(&command{name: "theme", group: "Themes", syntax: "theme <name>", desc: "Browse and apply a theme; moving previews it in this window",
		filters: true,
		options: func(m *Model, arg string) []option {
			var out []option
			for _, t := range m.lib.Themes {
				out = append(out, m.themeOption(t))
			}
			return out
		},
		onMove: func(m *Model, opt *option) tea.Cmd {
			if opt != nil && opt.theme != nil {
				m.previewTheme(opt.theme)
			}
			return nil
		},
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if opt == nil || opt.theme == nil {
				m.warn("No theme matches %q", arg)
				return nil
			}
			if m.cur == nil || m.cur.Name != opt.theme.Name {
				m.previewTheme(opt.theme)
			}
			return m.applyNow()
		},
		preview: previewThemePanel,
	})
	add(&command{name: "favs", group: "Themes", syntax: "favs", desc: "Only the starred themes",
		filters: true,
		options: func(m *Model, arg string) []option {
			var out []option
			for _, t := range m.lib.Themes {
				if m.state.IsFavorite(t.Name) {
					out = append(out, m.themeOption(t))
				}
			}
			if len(out) == 0 {
				out = append(out, option{label: "No favourites yet", detail: "fav stars the theme on screen", header: true})
			}
			return out
		},
		onMove: func(m *Model, opt *option) tea.Cmd {
			if opt != nil && opt.theme != nil {
				m.previewTheme(opt.theme)
			}
			return nil
		},
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if opt != nil && opt.theme != nil {
				m.previewTheme(opt.theme)
				return m.applyNow()
			}
			return nil
		},
		preview: previewThemePanel,
	})
	add(&command{name: "fav", group: "Themes", syntax: "fav", desc: "Star or unstar the theme on screen",
		run:     func(m *Model, arg string, opt *option) tea.Cmd { m.toggleFavorite(); return nil },
		preview: previewOverview,
	})
	add(&command{name: "new", group: "Themes", syntax: "new [light|dark] [harmony] [#seed]", desc: "Create a theme from a colour harmony; Enter again regenerates",
		options: func(m *Model, arg string) []option {
			words := strings.Fields(strings.ToLower(arg))
			var out []option
			names := append([]string{"Random"}, color.HarmonyNames...)
			descs := append([]string{"Anything goes, within reason"}, color.HarmonyDescriptions...)
			for i, n := range names {
				if len(words) > 0 && !harmonyMatches(n, words) && !onlyModifiers(words) {
					continue
				}
				out = append(out, option{label: n, detail: descs[i], value: strconv.Itoa(i)})
			}
			return out
		},
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if opt == nil {
				return nil
			}
			method, _ := strconv.Atoi(opt.value)
			m.generateDraft(method, arg)
			return nil
		},
		keepArg: true,
		preview: previewNewPanel,
	})
	add(&command{name: "random", group: "Themes", syntax: "random", desc: "A harmonious palette on the spot, in the current light or dark register",
		run:     func(m *Model, arg string, opt *option) tea.Cmd { m.surprise(); return nil },
		preview: previewThemeCurrent,
	})
	add(&command{name: "edit", group: "Themes", syntax: "edit [slot] [#hex]", desc: "Edit one of the 22 colour slots; Enter opens the slot editor",
		options: func(m *Model, arg string) []option {
			if m.cur == nil {
				return nil
			}
			// A trailing colour is the value to set, not part of the slot name.
			needle := strings.TrimSpace(arg)
			if hex := lastHex(arg); hex != "" {
				words := strings.Fields(arg)
				needle = strings.Join(words[:len(words)-1], " ")
			}
			var out []option
			for _, section := range ghostty.Sections {
				for _, key := range section.Keys {
					v := m.cur.Colors[key]
					label := strings.ToLower(section.Title) + " " + ghostty.Label(key)
					if key == "background" || key == "foreground" {
						label = key
					}
					detail := "not set"
					if v != "" {
						detail = v
						if key != "background" {
							r := color.Contrast(v, m.cur.Background())
							detail += fmt.Sprintf("   %.1f:1 %s", r, color.Grade(r))
						}
					}
					if needle != "" && !fuzzyMatch(label, needle) {
						continue
					}
					out = append(out, option{label: label, detail: detail, value: key, slot: key, swatch: v})
				}
			}
			if needle != "" {
				sort.SliceStable(out, func(a, b int) bool { return fuzzyScore(out[a].label, needle) < fuzzyScore(out[b].label, needle) })
			}
			return out
		},
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if opt == nil || opt.slot == "" {
				return nil
			}
			// `edit red #ff0000` sets it outright; `edit red` opens the editor.
			if hex := lastHex(arg); hex != "" {
				m.setColor(opt.slot, hex)
				m.info("%s set to %s", ghostty.Label(opt.slot), hex)
				return nil
			}
			m.startEdit(opt.slot)
			return nil
		},
		preview: previewPalettePanel,
	})
	add(&command{name: "undo", group: "Themes", syntax: "undo", desc: "Revert the theme on screen to its saved colours",
		run:     func(m *Model, arg string, opt *option) tea.Cmd { m.revert(); return nil },
		preview: previewThemeCurrent,
	})
	add(&command{name: "save", group: "Themes", syntax: "save [name]", desc: "Save the edited theme; a theme you did not write is saved under a new name",
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if m.cur == nil {
				return nil
			}
			if arg != "" {
				return m.saveAs(arg)
			}
			if !m.dirty && m.cur.Source != ghostty.SourceDraft {
				m.info("Nothing to save — %s is unchanged", m.cur.Name)
				return nil
			}
			if !m.cur.Writable() {
				m.setPromptText("Save " + m.suggestName())
				m.warn("%s is not yours to overwrite — Enter saves it under this name, or change it", m.cur.Name)
				return nil
			}
			return m.saveTheme()
		},
		keepArg: true,
		preview: previewThemeCurrent,
	})
	add(&command{name: "fork", group: "Themes", syntax: "fork", desc: "Copy the theme on screen into your themes directory and switch to the copy",
		run:     func(m *Model, arg string, opt *option) tea.Cmd { return m.forkTheme() },
		preview: previewThemeCurrent,
	})
	add(&command{name: "delete", group: "Themes", syntax: "delete", desc: "Delete the theme on screen, if this tool wrote it",
		run:     func(m *Model, arg string, opt *option) tea.Cmd { m.deleteTheme(); return nil },
		preview: previewThemeCurrent,
	})

	// --- Fonts ------------------------------------------------------------
	add(&command{name: "font", group: "Fonts", syntax: "font <family>", desc: "Pick the font family; only families Ghostty can load are listed, monospace first",
		filters: true,
		options: func(m *Model, arg string) []option {
			if !m.fontsLoaded {
				return []option{{label: "Listing the fonts Ghostty can load…", header: true}}
			}
			var out []option
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
			sort.SliceStable(out, func(a, b int) bool { return out[a].family.Mono && !out[b].family.Mono })
			return out
		},
		onMove: func(m *Model, opt *option) tea.Cmd {
			if opt == nil || opt.family == nil {
				return nil
			}
			return m.pickFamily(*opt.family)
		},
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if opt == nil || opt.family == nil {
				return nil
			}
			cmd := m.pickFamily(*opt.family)
			m.info("Font: %s — written; %s", opt.family.Name, m.reloadWord())
			return tea.Batch(cmd, m.flushWritesCmd())
		},
		preview: previewFontPanel,
	})
	add(&command{name: "style", group: "Fonts", syntax: "style <name>", desc: "The face of the current family to use for regular text",
		filters: true,
		options: func(m *Model, arg string) []option {
			if !m.fontsLoaded {
				return []option{{label: "Listing the fonts Ghostty can load…", header: true}}
			}
			f, ok := m.findFamily(m.fontFamily)
			if !ok {
				return []option{{label: "Pick a family first: font <name>", header: true}}
			}
			var out []option
			for _, s := range f.Styles {
				out = append(out, option{label: s, value: s, current: strings.EqualFold(s, m.currentStyle())})
			}
			return out
		},
		onMove: func(m *Model, opt *option) tea.Cmd {
			if opt == nil {
				return nil
			}
			m.setFontStyle(opt.value)
			return m.scheduleWrite()
		},
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if opt == nil {
				return nil
			}
			m.setFontStyle(opt.value)
			m.info("Style: %s — written; %s", opt.value, m.reloadWord())
			return m.flushWritesCmd()
		},
		preview: previewFontPanel,
	})
	add(&command{name: "size", group: "Fonts", syntax: "size <points>", desc: "Font size in points, half points allowed",
		options: func(m *Model, arg string) []option {
			var out []option
			for _, s := range fontSizes {
				label := ghostty.FormatSize(s) + " pt"
				if arg != "" && !strings.HasPrefix(ghostty.FormatSize(s), strings.TrimSpace(arg)) {
					continue
				}
				out = append(out, option{label: label, value: ghostty.FormatSize(s), current: s == m.fontSize})
			}
			return out
		},
		onMove: func(m *Model, opt *option) tea.Cmd {
			if opt == nil {
				return nil
			}
			size, _ := strconv.ParseFloat(opt.value, 64)
			return m.setSize(size)
		},
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			value := ""
			if opt != nil {
				value = opt.value
			}
			if v, err := strconv.ParseFloat(strings.TrimSpace(arg), 64); err == nil {
				value = ghostty.FormatSize(v)
			}
			size, err := strconv.ParseFloat(value, 64)
			if err != nil {
				m.warn("size takes a number of points, like 14 or 13.5")
				return nil
			}
			cmd := m.setSize(size)
			m.info("Size: %s pt — written; %s", ghostty.FormatSize(m.fontSize), m.reloadWord())
			return tea.Batch(cmd, m.flushWritesCmd())
		},
		preview: previewFontPanel,
	})
	add(&command{name: "download", group: "Fonts", syntax: "download [family]", desc: "Install a monospace family from the Nerd Fonts archive into your user fonts",
		filters: true,
		options: func(m *Model, arg string) []option {
			installed := m.installedDownloadables()
			missing := 0
			for _, d := range fontdl.AvailableDownloads {
				if !installed[d.Name] {
					missing++
				}
			}
			out := []option{{label: fmt.Sprintf("Everything (%d missing)", missing), detail: "one archive after another, roughly a minute", value: "*"}}
			for _, d := range fontdl.AvailableDownloads {
				note := d.Note
				if installed[d.Name] {
					note += " · already installed"
				}
				out = append(out, option{label: d.Name, detail: note, value: d.Archive})
			}
			return out
		},
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if opt == nil {
				return nil
			}
			if opt.value == "*" {
				return m.installFonts(fontdl.AvailableDownloads)
			}
			for _, d := range fontdl.AvailableDownloads {
				if d.Archive == opt.value {
					return m.installFonts([]fontdl.FontDownload{d})
				}
			}
			return nil
		},
		preview: previewDownloadPanel,
	})

	// --- Window and text settings, generated from the setting table -----------
	for _, s := range m.buildSettings() {
		s := s
		add(settingCommand(s))
	}

	// --- Tool -------------------------------------------------------------
	add(&command{name: "reload", group: "Tool", syntax: "reload", desc: "Ask the running Ghostty to re-read its files now",
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			m.reloadFailed = false
			m.info("Reloading Ghostty…")
			return m.reload()
		},
		preview: previewOverview,
	})
	add(&command{name: "autoreload", group: "Tool", syntax: "autoreload on|off", desc: "Whether every change sends the reload keystroke to Ghostty (macOS, needs Accessibility)",
		options: func(m *Model, arg string) []option {
			return onOff(m.state.AutoReloadEnabled())
		},
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			v := pickValue(arg, opt)
			m.state.SetAutoReload(v == "on")
			m.reloadFailed = false
			_ = m.state.Save()
			m.info("Auto-reload %s", v)
			return nil
		},
		preview: previewOverview,
	})
	add(&command{name: "interface", group: "Tool", syntax: "interface graphite|paper|theme", desc: "The colours of the editor itself, kept apart from the theme being looked at",
		options: func(m *Model, arg string) []option {
			var out []option
			for _, n := range interfaceNames {
				if arg != "" && !fuzzyMatch(n, strings.TrimSpace(arg)) {
					continue
				}
				out = append(out, option{label: n, detail: interfaceNotes[n], value: n, current: n == m.interfaceName()})
			}
			return out
		},
		onMove: func(m *Model, opt *option) tea.Cmd {
			if opt != nil {
				m.state.Interface = opt.value
			}
			return nil
		},
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if opt == nil {
				return nil
			}
			m.state.Interface = opt.value
			_ = m.state.Save()
			m.info("Interface: %s", opt.value)
			return nil
		},
		preview: previewOverview,
	})
	add(&command{name: "overrides", group: "Tool", syntax: "overrides", desc: "Comment out the background/foreground/palette lines in your config that shadow every theme",
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if len(m.overrides) == 0 {
				m.info("Your config has no colour lines overriding the theme")
				return nil
			}
			n := len(m.overrides)
			m.tree.DisableColourOverrides()
			m.overrides = nil
			m.info("Disabled %d colour line(s) — commented out, backup in %s/backups", n, m.paths.StateDir)
			return m.commitConfig()
		},
		preview: previewOverridesPanel,
	})
	add(&command{name: "collection", group: "Tool", syntax: "collection", desc: fmt.Sprintf("Install or refresh the %d curated themes", len(collection.Collection)),
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			m.busyText = "Installing the curated collection…"
			m.overlay = overlayBusy
			dir := m.paths.ThemesDir
			return func() tea.Msg {
				written, skipped, err := collection.Install(dir)
				return collectionMsg{written, skipped, err}
			}
		},
		preview: previewOverview,
	})
	add(&command{name: "backup", group: "Tool", syntax: "backup", desc: "Copy every Ghostty config file into the backups directory now",
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			files, err := ghostty.BackupNow(m.paths)
			if err != nil {
				m.fail("Backup failed: %v", err)
				return nil
			}
			m.info("Backed up %d file(s) to %s/backups", len(files), m.paths.StateDir)
			return nil
		},
		preview: previewOverview,
	})
	add(&command{name: "paths", group: "Tool", syntax: "paths", desc: "Where the config, themes, favourites and backups are",
		run:     func(m *Model, arg string, opt *option) tea.Cmd { m.openMessage(m.pathsText()); return nil },
		preview: previewPathsPanel,
	})
	add(&command{name: "help", group: "Tool", syntax: "help", desc: "Keys and how the editor treats your files",
		run:     func(m *Model, arg string, opt *option) tea.Cmd { m.overlay = overlayHelp; return nil },
		preview: previewOverview,
	})
	add(&command{name: "quit", group: "Tool", syntax: "quit", desc: "Leave. Nothing is applied on the way out: only Enter on a theme applies it",
		run:     func(m *Model, arg string, opt *option) tea.Cmd { _, c := m.quit(); return c },
		preview: previewOverview,
	})
	return cmds
}

// settingCommand turns a setting row into a command with its values as options.
func settingCommand(s setting) *command {
	c := &command{name: s.cmd, group: s.group, syntax: s.syntax, desc: s.detail}
	c.options = func(m *Model, arg string) []option {
		current := s.get(m)
		var out []option
		if s.numeric {
			for v := s.min; v <= s.max+1e-9; v += s.step {
				label := s.format(roundStep(v, s.step))
				if arg != "" && !strings.HasPrefix(label, strings.TrimSpace(arg)) {
					continue
				}
				detail := ""
				if label == current {
					detail = "current"
				}
				out = append(out, option{label: label, detail: detail, value: label, current: label == current})
			}
			return out
		}
		for _, o := range s.options {
			if arg != "" && !fuzzyMatch(o, strings.TrimSpace(arg)) {
				continue
			}
			// What each value means is in the panel, in full.
			detail := ""
			if strings.EqualFold(o, current) {
				detail = "current"
			}
			out = append(out, option{label: o, detail: detail, value: o, current: strings.EqualFold(o, current)})
		}
		return out
	}
	c.run = func(m *Model, arg string, opt *option) tea.Cmd {
		v := pickValue(arg, opt)
		if v == "" {
			return nil
		}
		if s.numeric {
			f, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
			if err != nil {
				m.warn("%s takes a number between %s and %s", s.cmd, s.format(s.min), s.format(s.max))
				return nil
			}
			v = s.format(roundStep(color.Clamp(f, s.min, s.max), s.step))
		}
		cmd := s.set(m, v)
		note := ""
		if s.note != "" {
			note = " · " + s.note
		}
		m.info("%s: %s — written; %s%s", s.label, v, m.reloadWord(), note)
		return tea.Batch(cmd, m.flushWritesCmd())
	}
	c.preview = func(m *Model, opt *option, width int) []string { return previewSettingPanel(m, s, opt, width) }
	return c
}

func roundStep(v, step float64) float64 {
	if step <= 0 {
		return v
	}
	return float64(int(v/step+0.5*sign(v))) * step
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// pickValue prefers what was typed over what is highlighted.
func pickValue(arg string, opt *option) string {
	if a := strings.TrimSpace(arg); a != "" {
		return a
	}
	if opt != nil {
		return opt.value
	}
	return ""
}

func onOff(on bool) []option {
	return []option{{label: "on", value: "on", current: on}, {label: "off", value: "off", current: !on}}
}

func harmonyMatches(name string, words []string) bool {
	for _, w := range words {
		if w == "light" || w == "dark" || strings.HasPrefix(w, "#") || color.IsHex(w) {
			continue
		}
		if fuzzyMatch(name, w) {
			return true
		}
	}
	return false
}

func onlyModifiers(words []string) bool {
	for _, w := range words {
		if w != "light" && w != "dark" && !color.IsHex(w) {
			return false
		}
	}
	return true
}

// lastHex returns the last word of arg when it is a colour.
func lastHex(arg string) string {
	words := strings.Fields(arg)
	if len(words) == 0 {
		return ""
	}
	if color.IsHex(words[len(words)-1]) {
		return color.Normalize(words[len(words)-1], "")
	}
	return ""
}

// themeOption renders a theme as a result line.
func (m *Model) themeOption(t *ghostty.Theme) option {
	tone := "dark"
	if !t.IsDark() {
		tone = "light"
	}
	ratio := color.Contrast(t.Foreground(), t.Background())
	src := "bundled"
	switch t.Source {
	case ghostty.SourceCollection:
		src = "collection"
	case ghostty.SourceOwned:
		src = "yours"
	case ghostty.SourceUser:
		src = "your file"
	}
	// The list keeps to what tells themes apart at a glance; where a theme
	// comes from is in the panel.
	_ = src
	detail := fmt.Sprintf("%s %s", tone, color.Grade(ratio))
	if m.state.IsFavorite(t.Name) {
		detail += " ♥"
	}
	return option{label: t.Name, detail: detail, value: t.Name, theme: t, current: t.Name == m.applied}
}

// reloadWord says whether Ghostty is being reloaded or needs a hand.
func (m *Model) reloadWord() string {
	if m.autoReloadWorks() {
		return "Ghostty reloads"
	}
	return m.reloadHint()
}

// flushWritesCmd writes now rather than after the debounce.
func (m *Model) flushWritesCmd() tea.Cmd {
	m.writeSeq++
	return m.flushWrites()
}

// --- the settings table ---------------------------------------------------

// A setting is one Ghostty key with the values that make sense for it.
type setting struct {
	cmd, group, label, syntax, detail string
	options                           []string
	explain                           map[string]string
	get                               func(m *Model) string
	set                               func(m *Model, value string) tea.Cmd
	numeric                           bool
	min, max, step                    float64
	format                            func(v float64) string
	note                              string
	key                               string
}

func keySetting(cmd, key, label, detail string, options []string, fallback string) setting {
	return setting{
		cmd: cmd, group: "Window", label: label, syntax: cmd + " " + strings.Join(options, "|"), detail: detail, options: options, key: key,
		get: func(m *Model) string {
			if v := m.tree.Value(key, ""); v != "" {
				return v
			}
			return fallback
		},
		set: func(m *Model, v string) tea.Cmd {
			if v == fallback {
				m.tree.Unset(key, "disabled by ghostty-config: back to the default")
			} else {
				m.tree.Set(key, v)
			}
			return nil
		},
	}
}

func boolSetting(cmd, key, label, detail string, fallback bool) setting {
	def := "false"
	if fallback {
		def = "true"
	}
	s := keySetting(cmd, key, label, detail, []string{"on", "off"}, map[bool]string{true: "on", false: "off"}[fallback])
	s.syntax = cmd + " on|off"
	s.get = func(m *Model) string {
		v := strings.ToLower(m.tree.Value(key, def))
		if v == "" || v == "true" {
			return "on"
		}
		return "off"
	}
	s.set = func(m *Model, v string) tea.Cmd {
		want := "true"
		if v == "off" {
			want = "false"
		}
		if want == def {
			m.tree.Unset(key, "disabled by ghostty-config: back to the default")
		} else {
			m.tree.Set(key, want)
		}
		return nil
	}
	return s
}

func numberSetting(cmd, key, label, detail string, fallback, min, max, step float64, format func(float64) string) setting {
	return setting{
		cmd: cmd, group: "Window", label: label, syntax: fmt.Sprintf("%s <%s–%s>", cmd, format(min), format(max)), detail: detail,
		numeric: true, min: min, max: max, step: step, format: format, key: key,
		get: func(m *Model) string { return m.tree.Value(key, format(fallback)) },
		set: func(m *Model, v string) tea.Cmd {
			if v == format(fallback) {
				m.tree.Unset(key, "disabled by ghostty-config: back to the default")
			} else {
				m.tree.Set(key, v)
			}
			return nil
		},
	}
}

func plain(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
func percent(v float64) string {
	return plain(v) + "%"
}

// buildSettings assembles the window and text settings for this platform.
func (m *Model) buildSettings() []setting {
	mac := runtime.GOOS == "darwin"
	var s []setting
	if mac {
		s = append(s, setting{
			cmd: "titlebar", group: "Window", label: "Title bar", syntax: "titlebar native|transparent|tabs|hidden|none",
			detail:  "How the top of the window is drawn; transparent lets the theme background reach it",
			options: []string{"native", "transparent", "tabs", "hidden", "none"},
			explain: map[string]string{
				"native":      "the system title bar, in the system's own colour",
				"transparent": "the theme background under the bar; buttons kept (default)",
				"tabs":        "the tab bar lives in the title bar; always theme-coloured",
				"hidden":      "no bar, frame and rounded corners kept; drag with option-click",
				"none":        "no frame at all and no tabs (window-decoration = none)",
			},
			key: "macos-titlebar-style",
			get: func(m *Model) string {
				if dec := strings.ToLower(m.tree.Value("window-decoration", "auto")); dec == "none" || dec == "false" {
					return "none"
				}
				return m.tree.Value("macos-titlebar-style", "transparent")
			},
			set: func(m *Model, v string) tea.Cmd {
				if v == "none" {
					m.tree.Set("window-decoration", "none")
					return nil
				}
				if dec := strings.ToLower(m.tree.Value("window-decoration", "auto")); dec == "none" || dec == "false" {
					m.tree.Set("window-decoration", "auto")
				}
				if v == "transparent" {
					m.tree.Unset("macos-titlebar-style", "disabled by ghostty-config: transparent is the default")
				} else {
					m.tree.Set("macos-titlebar-style", v)
				}
				return nil
			},
			note: "new windows only",
		})
		s = append(s, boolSetting("shadow", "macos-window-shadow", "Window shadow", "Off looks cleaner with a translucent window", true))
	} else {
		s = append(s, keySetting("decoration", "window-decoration", "Window decorations", "auto follows the desktop · client draws Ghostty's own · server asks the window manager · none removes them", []string{"auto", "client", "server", "none"}, "auto"))
		s = append(s, boolSetting("gtktitlebar", "gtk-titlebar", "GTK title bar", "Whether the GTK header bar is shown", true))
	}
	op := numberSetting("opacity", "background-opacity", "Background opacity", "1 is opaque; below it the desktop shows through and contrast ratios become a best case", 1, 0.3, 1, 0.05, func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) })
	if mac {
		op.note = "needs a Ghostty restart on macOS"
	}
	s = append(s, op)
	blur := []string{"off", "10", "20", "30", "40"}
	if mac {
		blur = append(blur, "macos-glass-regular", "macos-glass-clear")
	}
	bl := keySetting("blur", "background-blur", "Background blur", "Only matters when opacity is below 1; the glass values need macOS 26", blur, "off")
	bl.get = func(m *Model) string {
		v := strings.ToLower(m.tree.Value("background-blur", "false"))
		switch v {
		case "false", "0", "":
			return "off"
		case "true":
			return "20"
		}
		return v
	}
	bl.set = func(m *Model, v string) tea.Cmd {
		if v == "off" {
			m.tree.Unset("background-blur", "disabled by ghostty-config: blur off is the default")
		} else {
			m.tree.Set("background-blur", v)
		}
		return nil
	}
	s = append(s, bl)
	s = append(s, numberSetting("paddingx", "window-padding-x", "Padding, left and right", "Points between the cells and the window edge", 2, 0, 60, 2, plain))
	s = append(s, numberSetting("paddingy", "window-padding-y", "Padding, top and bottom", "With a transparent title bar, extra top padding keeps the first line clear of the buttons", 2, 0, 60, 2, plain))
	s = append(s, boolSetting("balance", "window-padding-balance", "Balance leftover padding", "Spread the space the grid does not fill over all four edges", false))
	s = append(s, keySetting("paddingcolor", "window-padding-color", "Padding colour", "extend continues the nearest cell's background into the padding", []string{"background", "extend", "extend-always"}, "background"))
	s = append(s, keySetting("windowtheme", "window-theme", "Window chrome theme", "auto follows the terminal background · system follows the desktop", []string{"auto", "system", "light", "dark"}, "auto"))
	if mac {
		s = append(s, keySetting("colorspace", "window-colorspace", "Colour space", "display-p3 renders theme colours in the wider gamut", []string{"srgb", "display-p3"}, "srgb"))
	}
	lh := numberSetting("lineheight", "adjust-cell-height", "Line height", "Percent added to each row; negative tightens", 0, -20, 50, 2, percent)
	lh.group = "Fonts"
	s = append(s, lh)
	lig := setting{
		cmd: "ligatures", group: "Fonts", label: "Ligatures", syntax: "ligatures on|off", detail: "Programming ligatures (calt/liga/dlig), for fonts that have them", options: []string{"on", "off"},
		get: func(m *Model) string {
			if m.tree.LigaturesDisabled() {
				return "off"
			}
			return "on"
		},
		set: func(m *Model, v string) tea.Cmd { m.tree.SetLigatures(v == "on"); return nil },
	}
	s = append(s, lig)
	if mac {
		th := boolSetting("thicken", "font-thicken", "Thicken strokes", "Heavier rendering, the way Terminal.app draws", false)
		th.group = "Fonts"
		s = append(s, th)
	}
	s = append(s, numberSetting("contrast", "minimum-contrast", "Minimum contrast", "Ghostty pushes text towards black or white until it clears this ratio; 1 is off", 1, 1, 7, 0.5, plain))
	s = append(s, keySetting("cursor", "cursor-style", "Cursor", "Shell integration may still ask for a bar at the prompt", []string{"block", "bar", "underline", "block_hollow"}, "block"))
	s = append(s, keySetting("blink", "cursor-style-blink", "Cursor blink", "default leaves it to the shell and DEC mode 12", []string{"default", "true", "false"}, "default"))
	return s
}
