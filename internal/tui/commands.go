package tui

import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/collection"
	"github.com/vitruves/ghostty-config/internal/color"
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
	key     *keyRow // an action of the shortcut editor
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
	// adjust marks a setting whose values form a short scale: on the menu,
	// ← and → step it without opening it.
	adjust bool
	// set is the setting a command was made from, nil for other commands.
	set *setting
	// label names the command on screen in place of its word.
	label string
	// transient marks a command that comes and goes with what it answers,
	// which the help and the rail beside an open command leave out.
	transient bool
}

// title is what the command is called on screen.
func (c *command) title() string {
	if c.label != "" {
		return c.label
	}
	return title(c.name)
}

// Groups, in the order the menu shows them.
var groups = []string{"Themes", "Fonts", "Window", "Input", "Session", "Extensions", "Tool"}

// commands is the registry, built once per model because the closures need
// platform facts.
func (m *Model) buildCommands() []*command {
	var cmds []*command
	add := func(c *command) { cmds = append(cmds, c) }

	// --- Themes -----------------------------------------------------------
	add(&command{name: "theme", group: "Themes", syntax: "theme <name>", desc: "Browse themes; moving only looks, Enter applies the highlighted one",
		filters: true,
		options: func(m *Model, arg string) []option { return m.themeOptions(arg) },
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
	add(&command{name: "revert", group: "Themes", syntax: "revert", desc: "Drop the unsaved edits of the theme on screen, back to its saved colours; undo, in Tool, takes back changes to the config",
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
	add(&command{name: "delete", group: "Themes", syntax: "delete <name>", desc: "Delete a theme file from your themes directory: yours, the collection's, or one someone else put there. Bundled themes are Ghostty's and stay",
		filters: true,
		options: func(m *Model, arg string) []option {
			var out []option
			installed := 0
			for _, t := range m.lib.Themes {
				if t.Source == ghostty.SourceCollection {
					installed++
				}
			}
			if installed > 0 && (arg == "" || fuzzyMatch("collection", strings.TrimSpace(arg))) {
				out = append(out, option{label: "collection", detail: fmt.Sprintf("every installed collection theme · %d files", installed), value: "@collection"})
			}
			for _, t := range m.lib.Themes {
				if t.Deletable() {
					o := m.themeOption(t)
					o.detail = sourceWord(t)
					out = append(out, o)
				}
			}
			if len(out) == 0 {
				out = append(out, option{label: "Nothing to delete", detail: "the themes directory has none of your files", header: true})
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
			if opt == nil {
				return nil
			}
			if opt.value == "@collection" {
				var ts []*ghostty.Theme
				for _, t := range m.lib.Themes {
					if t.Source == ghostty.SourceCollection {
						ts = append(ts, t)
					}
				}
				m.deleteThemes(ts)
				return nil
			}
			if opt.theme != nil {
				m.deleteThemes([]*ghostty.Theme{opt.theme})
			}
			return nil
		},
	})

	// --- Fonts ------------------------------------------------------------
	if fix := m.fixFontCommand(); fix != nil {
		add(fix)
	}
	add(&command{name: "font", group: "Fonts", syntax: "font <family>", desc: "Pick the font family, or install one: the families Ghostty can load are listed monospace first, the ones that can be downloaded under them",
		filters: true,
		options: func(m *Model, arg string) []option { return m.fontOptions(arg) },
		onMove: func(m *Model, opt *option) tea.Cmd {
			if opt == nil || opt.family == nil {
				return nil
			}
			if m.browsesFonts() {
				// The panel sets the family in itself; writing it would
				// reload Ghostty and resize every cell under the picture.
				return nil
			}
			if !m.browsed {
				// The list was only opened or filtered: the highlight rests on
				// whatever matched first, which nobody chose. Typing a name
				// slowly must not write every family it passes through.
				return nil
			}
			return m.pickFamily(*opt.family)
		},
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if opt == nil {
				return nil
			}
			if archive, ok := strings.CutPrefix(opt.value, downloadPrefix); ok {
				return m.installArchive(archive)
			}
			if opt.family == nil {
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
			// Only a move by hand writes: typing filters the list, and its
			// first match is nobody's choice.
			if opt == nil || m.browsesFonts() || !m.browsed {
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
			// Typing "size 2" on the way to 20 must not write 20 first.
			if opt == nil || !m.browsed {
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
	add(presetCommand())

	// --- Window and text settings, generated from the setting table -----------
	for _, s := range m.buildSettings() {
		s := s
		add(settingCommand(s))
	}
	if m.since14() {
		add(sshHostsCommand())
	}

	add(keysCommand())

	// --- Extensions: shaders and packs of themes ---------------------------
	for _, c := range m.extensionCommands() {
		add(c)
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
	add(&command{name: "autoreload", group: "Tool", syntax: "autoreload on|off", desc: "Whether every change asks the running Ghostty to reload",
		adjust: true,
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
	add(&command{name: "interface", group: "Tool", syntax: "interface auto|midnight|graphite|paper|theme|clear", desc: "The colours of the editor itself, kept apart from the theme being looked at",
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
				// Walking the list shows each interface, and only Enter keeps
				// one: remember what was in effect to put it back.
				if !m.previewingInterface {
					m.interfaceWas, m.previewingInterface = m.state.Interface, true
				}
				m.state.Interface = opt.value
			}
			return nil
		},
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if opt == nil {
				return nil
			}
			m.previewingInterface = false
			m.state.Interface = opt.value
			_ = m.state.Save()
			m.info("Interface: %s", opt.value)
			return nil
		},
		preview: previewOverview,
	})
	for _, c := range m.safetyCommands() {
		add(c)
	}
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
	// A list found at run time is chosen from, not stepped through.
	c := &command{name: s.cmd, group: s.group, syntax: s.syntax, desc: s.detail, adjust: s.list == nil}
	c.set = &s
	c.options = func(m *Model, arg string) []option {
		if s.list != nil {
			return s.list(m, arg)
		}
		current := s.get(m)
		var out []option
		if s.numeric {
			if s.unset != "" && (arg == "" || strings.HasPrefix(s.unset, strings.TrimSpace(arg))) {
				out = append(out, option{label: s.unset, value: s.unset, current: current == s.unset})
			}
			// The config may write a value its own way, 0.9 for 0.90: compare
			// numbers, not labels.
			now, nowOK := parseNumber(current)
			for v := s.min; v <= s.max+1e-9; v += s.step {
				label := s.format(roundStep(v, s.step))
				if arg != "" && !strings.HasPrefix(label, strings.TrimSpace(arg)) {
					continue
				}
				at, _ := parseNumber(label)
				isCurrent := label == current || (nowOK && sameNumber(at, now, s.step))
				detail := ""
				if isCurrent {
					detail = "current"
				}
				out = append(out, option{label: label, detail: detail, value: label, current: isCurrent})
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
		if s.list != nil {
			// What was typed only filtered the list, or was turned into a
			// line of it: the highlighted line is the value.
			v = ""
			if opt != nil {
				v = opt.value
			}
		}
		if v == "" {
			return nil
		}
		if s.numeric && !strings.EqualFold(v, s.unset) {
			f, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
			if err != nil {
				m.warn("%s takes a number between %s and %s", s.cmd, s.format(s.min), s.format(s.max))
				return nil
			}
			v = s.format(roundStep(color.Clamp(f, s.min, s.max), s.step))
		} else if s.numeric {
			v = s.unset
		}
		seq := m.statusSeq
		cmd := s.set(m, v)
		if m.statusSeq != seq {
			// The setting said something of its own: a question, a download.
			return cmd
		}
		note := ""
		if s.note != "" {
			note = " · " + s.note
		}
		m.info("%s: %s — written; %s%s", s.label, s.shown(v), m.reloadWord(), note)
		return tea.Batch(cmd, m.flushWritesCmd())
	}
	c.preview = func(m *Model, opt *option, width int) []string { return previewSettingPanel(m, s, opt, width) }
	return c
}

// parseNumber reads a numeric setting as the config writes it, a percent
// sign allowed.
func parseNumber(v string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(v), "%"), 64)
	return f, err == nil
}

// sameNumber reports whether two values of a setting are the same step,
// allowing for the error that adding steps up accumulates.
func sameNumber(a, b, step float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < step/1000
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
	// fallback is the value in effect when the config does not set one.
	fallback string
	// unset is the word a numeric setting shows while the config leaves it
	// to Ghostty, offered before the numbers; unsetAt is where its slider
	// rests then.
	unset   string
	unsetAt float64
	// list replaces options for a setting whose values are only known at
	// run time, files on disk among them. Its options carry the value to
	// write, a typed one included, so Enter writes the highlighted line.
	list func(m *Model, arg string) []option
	// show is how a value reads in the menu, when not as it is written.
	show func(v string) string
	// canon turns a value spelled the way an older Ghostty took it, true for
	// copy-on-select, into the word the menu offers; presets are written once
	// for every version.
	canon func(v string) string
	// describe explains a value that explain has no entry for.
	describe func(m *Model, v string) string
	// hint is said under every value: what else the setting needs.
	hint string
	// writes is the line a value puts in the file, "" for none, for a
	// setting whose line is not key = value.
	writes func(m *Model, v string) string
	// hidden keeps a setting out of the menu while it cannot matter.
	hidden func(m *Model) bool
	// part names the part of its section, for a setting made at run time
	// that subsections cannot name in advance.
	part string
	// flag marks one switch of a flag list, which the help leaves to the
	// section: a dozen of them would crowd every other command out.
	flag bool
}

// shown is a value as the menu writes it.
func (s *setting) shown(v string) string {
	if s.show != nil {
		return s.show(v)
	}
	return v
}

// listed is whether a command belongs in the menu now.
func (m *Model) listed(c *command) bool {
	return c.set == nil || c.set.hidden == nil || !c.set.hidden(m)
}

// inHelp is whether the help names a command: not the ones a font brings,
// which come and go with it, nor the switches of a flag list.
func (m *Model) inHelp(c *command) bool {
	return m.listed(c) && !c.transient && (c.set == nil || (c.set.part == "" && !c.set.flag))
}

func keySetting(cmd, key, label, detail string, options []string, fallback string) setting {
	return setting{
		cmd: cmd, group: "Window", label: label, syntax: cmd + " " + strings.Join(options, "|"), detail: detail, options: options, key: key, fallback: fallback,
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
		numeric: true, min: min, max: max, step: step, format: format, key: key, fallback: format(fallback),
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

// flagSetting is one flag of a flag-set key, bell-features or
// shell-integration-features, as a switch of its own. defaults is the key's
// default list as Ghostty writes it, which says what a flag is when the
// config leaves it out. A change rewrites only the flag's own word of the
// list, so flags this tool does not know survive it.
func flagSetting(cmd, key, flag, label, detail, defaults string) setting {
	s := boolSetting(cmd, key, label, detail, ghostty.Flags(defaults, defaults)[flag])
	s.flag = true
	on := func(m *Model) bool { return ghostty.Flags(m.tree.Value(key, ""), defaults)[flag] }
	s.get = func(m *Model) string {
		if on(m) {
			return "on"
		}
		return "off"
	}
	s.set = func(m *Model, v string) tea.Cmd {
		m.tree.SetFlag(key, flag, v == "on", defaults)
		return nil
	}
	s.writes = func(m *Model, v string) string {
		if next := ghostty.WithFlag(m.tree.Value(key, ""), flag, v == "on", defaults); next != "" {
			return key + " = " + next
		}
		return ""
	}
	return s
}

func plain(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
func percent(v float64) string {
	return plain(v) + "%"
}

// since14 is whether the Ghostty installed reads the keys and values 1.4
// added. An older one's validator rejects them, so they are only offered to
// a binary that knows them; one that cannot say counts as 1.4.
func (m *Model) since14() bool {
	return ghostty.BinaryVersion(m.paths.Binary).AtLeast(1, 4)
}

// buildSettings assembles the window and text settings for this platform.
func (m *Model) buildSettings() []setting {
	mac := runtime.GOOS == "darwin"
	v14 := m.since14()
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
			key: "macos-titlebar-style", fallback: "transparent",
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
	if !mac {
		// Linux takes no strength: any value turns blur on, and whether the
		// desktop blurs is up to the compositor. On Wayland 1.3 speaks KDE's
		// protocol, 1.4 the shared ext-background-effect-v1, through GTK's
		// own blur from GTK 4.23.3.
		bl.detail = "Only matters when opacity is below 1; on Linux any strength just turns it on, and the compositor draws it"
		bl.hint = "On Wayland only KDE's KWin blurs; on X11 too, only under KWin."
		if v14 {
			bl.hint = "On Wayland, any compositor with ext-background-effect-v1 blurs, Niri among them; from GTK 4.23.3 GTK's own blur covers the window more exactly. On X11, only KWin."
		}
		bl.explain = map[string]string{"off": valueNotes["blur"]["off"]}
		for _, v := range blur[1:] {
			bl.explain[v] = "blur on: Linux ignores the strength"
		}
	}
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
	s = append(s, paddingBalance(v14))
	s = append(s, keySetting("paddingcolor", "window-padding-color", "Padding colour", "extend continues the nearest cell's background into the padding", []string{"background", "extend", "extend-always"}, "background"))
	s = append(s, keySetting("windowtheme", "window-theme", "Window chrome theme", "auto follows the terminal background · system follows the desktop", []string{"auto", "system", "light", "dark"}, "auto"))
	if mac {
		s = append(s, keySetting("colorspace", "window-colorspace", "Colour space", "display-p3 renders theme colours in the wider gamut", []string{"srgb", "display-p3"}, "srgb"))
	}
	lh := numberSetting("lineheight", "adjust-cell-height", "Line height", "Percent added to each row; negative tightens", 0, -20, 50, 2, percent)
	lh.group = "Fonts"
	s = append(s, lh)
	lig := setting{
		cmd: "ligatures", group: "Fonts", label: "Ligatures", syntax: "ligatures on|off", detail: "Programming ligatures (calt/liga/dlig), for fonts that have them", options: []string{"on", "off"}, fallback: "on",
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
	input := func(st setting) { st.group = "Input"; s = append(s, st) }
	input(numberSetting("cursoropacity", "cursor-opacity", "Cursor opacity", "Below 1 the character under the cursor stays readable", 1, 0.2, 1, 0.1, func(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }))
	s = append(s, numberSetting("splitopacity", "unfocused-split-opacity", "Unfocused split opacity", "How far splits you are not typing in fade back; 1 keeps them as bright as the focused one", 0.7, 0.15, 1, 0.05, func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }))
	faint := numberSetting("faint", "faint-opacity", "Faint text opacity", "How dim SGR 2 (dim) text is drawn; raise it if dimmed output gets hard to read", 0.5, 0.2, 1, 0.05, func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) })
	faint.group = "Fonts"
	s = append(s, faint)
	s = append(s, boolSetting("opaquecells", "background-opacity-cells", "Opacity on coloured cells", "Also make cells with their own background (selections, status lines, TUIs) translucent", false))
	input(keySetting("cursor", "cursor-style", "Cursor", "Shell integration may still ask for a bar at the prompt", []string{"block", "bar", "underline", "block_hollow"}, "block"))
	input(keySetting("blink", "cursor-style-blink", "Cursor blink", "default leaves it to the shell and DEC mode 12", []string{"default", "true", "false"}, "default"))
	cw := numberSetting("cellwidth", "adjust-cell-width", "Letter spacing", "Percent added to each column; negative tightens, positive airs the text out", 0, -20, 50, 2, percent)
	cw.group = "Fonts"
	s = append(s, cw)
	if mac {
		ts := numberSetting("thickness", "font-thicken-strength", "Thicken strength", "How heavy thickened strokes are, 0 to 255; only matters when thicken is on", 255, 0, 255, 15, plain)
		ts.group = "Fonts"
		s = append(s, ts)
	}

	// --- Input and Session: how the terminal acts rather than how it looks. -----------
	session := func(st setting) { st.group = "Session"; s = append(s, st) }
	input(copyOnSelect(mac, v14))
	input(boolSetting("clearselect", "selection-clear-on-typing", "Clear selection on typing", "Typing drops the selection instead of leaving it highlighted", true))
	input(boolSetting("trimspaces", "clipboard-trim-trailing-spaces", "Trim copied spaces", "Strip trailing spaces from every copied line", true))
	input(boolSetting("pasteguard", "clipboard-paste-protection", "Paste protection", "Ask before pasting text that looks like it would run a command", true))
	for _, st := range clipboardAccess(v14) {
		input(st)
	}
	input(boolSetting("clicktomove", "cursor-click-to-move", "Click to move the cursor", "Option-click at a shell prompt moves the cursor there", true))
	input(boolSetting("hidemouse", "mouse-hide-while-typing", "Hide mouse while typing", "The pointer disappears when a key is pressed and returns when it moves", false))
	input(boolSetting("focusmouse", "focus-follows-mouse", "Focus follows mouse", "Hovering a split focuses it without a click", false))
	input(keySetting("scrollbar", "scrollbar", "Scrollbar", "system follows the OS setting; never removes the widget but keeps scrolling", []string{"system", "never"}, "system"))
	if v14 {
		for _, st := range inputSettings14(mac) {
			input(st)
		}
	}
	for _, st := range scrollbackSettings(v14) {
		session(st)
	}
	session(keySetting("confirmclose", "confirm-close-surface", "Confirm before closing", "true asks only when a process is running; always asks every time", []string{"true", "always", "false"}, "true"))
	session(keySetting("notify", "notify-on-command-finish", "Notify after a command", "Needs shell integration; unfocused only pings when you are looking elsewhere (Ghostty 1.3)", []string{"never", "unfocused", "always"}, "never"))
	session(keySetting("resizeoverlay", "resize-overlay", "Resize overlay", "The columns × rows popup shown while a window is being resized", []string{"after-first", "always", "never"}, "after-first"))
	session(boolSetting("inheritcwd", "window-inherit-working-directory", "Inherit working directory", "New windows, tabs and splits open in the directory of the focused one", true))
	session(boolSetting("quitlast", "quit-after-last-window-closed", "Quit after last window", "Closing the final window quits Ghostty instead of leaving it in the dock", false))
	if mac {
		input(keySetting("optionalt", "macos-option-as-alt", "Option as Alt", "true makes Option behave as Alt (readline, vim, emacs); false keeps accented characters", []string{"default", "true", "left", "right", "false"}, "default"))
		ss := keySetting("savestate", "window-save-state", "Restore windows", "default follows the macOS setting; always restores windows on every launch", []string{"default", "never", "always"}, "default")
		if v14 {
			// 1.4's never also keeps macOS from taking the snapshots it
			// restores windows with, which could stall a change of focus.
			ss.explain = map[string]string{
				"default": valueNotes["savestate"]["default"],
				"never":   "always start with a clean window; macOS takes no window snapshots either, which could slow switching windows",
				"always":  valueNotes["savestate"]["always"],
			}
		}
		session(ss)
		session(boolSetting("stepresize", "window-step-resize", "Resize in whole cells", "Window edges snap to the cell grid instead of pixels", false))
	} else {
		session(keySetting("tabbar", "window-show-tab-bar", "Tab bar", "auto shows it from two tabs up (GTK)", []string{"auto", "always", "never"}, "auto"))
		if v14 {
			session(boolSetting("tabsinverted", "gtk-tabs-inverted", "Close button first", "Each tab's close button at its start and its icon at its end, in the tab bar and the overview", false))
			session(boolSetting("tabswipe", "gtk-horizontal-tab-scroll", "Swipe between tabs", "Two-finger horizontal scrolling on a touchpad switches tabs: left to the next, right to the previous", true))
		}
	}
	if v14 {
		dh := keySetting("draghandle", "drag-handle", "Split drag handle", "The handle that drags a split to another place in the window", []string{"auto", "always", "never"}, "auto")
		dh.explain = map[string]string{
			"auto":   "shown once there are two splits or more",
			"always": "shown even over a terminal that is not split",
			"never":  "never shown: splits cannot be rearranged with the mouse",
		}
		if mac {
			dh.explain["auto"] = "shown while the pointer is over it; hidden for a lone terminal in full screen"
		}
		s = append(s, dh)
		for _, st := range programSettings14(mac) {
			session(st)
		}
	}
	s = append(s, m.windowExtraSettings(mac)...)
	s = append(s, m.sessionExtraSettings(mac)...)
	s = append(s, m.fontExtraSettings()...)
	return s
}
