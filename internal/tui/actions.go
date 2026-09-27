package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// reloadLibrary re-reads the themes directory after a save or delete.
func (m *Model) reloadLibrary() {
	m.lib = ghostty.LoadLibrary(m.paths)
}

// showTheme loads a theme into the editor and pushes its colours into the
// terminal. Nothing is written: the config still names whatever it named.
func (m *Model) showTheme(t *ghostty.Theme) {
	if t == nil {
		return
	}
	m.cur = t.Clone()
	m.base = t.Clone()
	m.dirty = false
	m.discardArmed = false
	m.previewScrl = 0
	m.c = chromeFor(m.cur)
	m.live.Reset()
	m.live.Show(m.cur.Colors)
}

// --- applying -------------------------------------------------------------

// applyTheme makes a theme the configured one: the config's `theme =` is
// rewritten in whichever file wins, colour keys that would shadow it are
// disabled, and the caller writes and reloads.
func (m *Model) applyTheme(t *ghostty.Theme) bool {
	if t == nil || t.Source == ghostty.SourceDraft {
		return false
	}
	disabled := 0
	if len(m.overrides) > 0 {
		m.tree.DisableColourOverrides()
		disabled = len(m.overrides)
		m.overrides = nil
	}
	value := m.setting.With(t.Name, t.IsDark())
	m.tree.Set("theme", value)
	m.setting = ghostty.ParseThemeSetting(value)
	m.applied = t.Name
	switch {
	case disabled > 0:
		m.warn("Applied %s · disabled %d colour line(s) in your config that overrode it (commented out, backup kept)", t.Name, disabled)
	case m.setting.Pair:
		m.info("Applied %s as the %s half of your light/dark pair", t.Name, styleWord(t))
	default:
		m.info("Applied %s", t.Name)
	}
	return true
}

// applyNow applies the theme on screen; edits are saved to a theme file
// first so the apply means something durable.
func (m *Model) applyNow() tea.Cmd {
	if m.cur == nil {
		return nil
	}
	if m.dirty || m.cur.Source == ghostty.SourceDraft {
		if _, ok := m.persistCurrent(); !ok {
			return nil
		}
		return m.commitConfig()
	}
	if !m.applyTheme(m.cur) {
		return nil
	}
	return m.commitConfig()
}

// commitConfig writes changed config files now and reloads.
func (m *Model) commitConfig() tea.Cmd {
	m.writeSeq++
	written, err := m.tree.Save()
	if err != nil {
		m.fail("%v", err)
		return nil
	}
	if len(written) == 0 {
		return nil
	}
	if !m.autoReloadWorks() {
		m.status += " — " + m.reloadHint()
	}
	return m.reload()
}

// persistCurrent writes the working colours to a theme file and applies it.
// Themes this tool did not write are forked under a new name.
func (m *Model) persistCurrent() (string, bool) {
	name := m.cur.Name
	if !m.cur.Writable() {
		name = m.suggestName()
	}
	saved, err := ghostty.SaveTheme(m.paths, m.cur, name)
	if err != nil {
		m.fail("Could not save %s: %v", name, err)
		return "", false
	}
	m.afterSave(saved)
	m.applyTheme(m.cur)
	return name, true
}

// afterSave swaps the working copy for the saved one and refreshes the list.
func (m *Model) afterSave(saved *ghostty.Theme) {
	m.cur = saved.Clone()
	m.base = saved.Clone()
	m.dirty = false
	m.discardArmed = false
	m.reloadLibrary()
	m.recompute()
}

// saveTheme saves in place; only themes this tool wrote qualify.
func (m *Model) saveTheme() tea.Cmd {
	saved, err := ghostty.SaveTheme(m.paths, m.cur, m.cur.Name)
	if err != nil {
		m.fail("Could not save: %v", err)
		return nil
	}
	m.afterSave(saved)
	m.applyTheme(m.cur)
	return m.commitConfig()
}

// saveAs writes under a chosen name and applies it.
func (m *Model) saveAs(name string) tea.Cmd {
	name = ghostty.SanitizeName(name)
	if name == "" {
		m.warn("Theme name cannot be empty")
		return nil
	}
	if t, taken := m.lib.Get(name); taken && t.Source == ghostty.SourceBuiltin {
		m.info("%s is a bundled theme; your copy will shadow it", name)
	}
	saved, err := ghostty.SaveTheme(m.paths, m.cur, name)
	if err != nil {
		m.fail("Could not save: %v", err)
		return nil
	}
	m.afterSave(saved)
	m.applyTheme(m.cur)
	m.setPromptText("")
	m.info("Saved as %s and applied", name)
	return m.commitConfig()
}

// suggestName proposes a free name derived from the current theme.
func (m *Model) suggestName() string {
	base := m.cur.Name
	if base == "" {
		base = "custom"
	}
	if m.cur.Source == ghostty.SourceDraft && !ghostty.Exists(m.paths, base) {
		return base
	}
	if _, taken := m.lib.Get(base + "-custom"); !taken && !ghostty.Exists(m.paths, base+"-custom") {
		return base + "-custom"
	}
	for n := 2; n < 100; n++ {
		candidate := fmt.Sprintf("%s-custom-%d", base, n)
		if _, taken := m.lib.Get(candidate); !taken {
			return candidate
		}
	}
	return base + "-custom-x"
}

// forkTheme copies the theme on screen so it can be edited freely.
func (m *Model) forkTheme() tea.Cmd {
	if m.cur == nil {
		return nil
	}
	name := m.suggestName()
	saved, err := ghostty.SaveTheme(m.paths, m.cur, name)
	if err != nil {
		m.fail("Could not create the copy: %v", err)
		return nil
	}
	m.afterSave(saved)
	m.applyTheme(m.cur)
	m.info("Editing copy %s — the original is untouched", name)
	return m.commitConfig()
}

// deleteTheme removes the theme on screen after confirmation.
func (m *Model) deleteTheme() {
	t := m.cur
	if t == nil {
		return
	}
	if t.Source != ghostty.SourceOwned {
		m.warn("%s was not written by this tool — fork it instead", t.Name)
		return
	}
	name := t.Name
	m.openConfirm(fmt.Sprintf("Delete %s? This cannot be undone.", name), func(m *Model) tea.Cmd {
		if err := ghostty.DeleteTheme(t); err != nil {
			m.fail("Could not delete: %v", err)
			return nil
		}
		if m.state.IsFavorite(name) {
			m.state.ToggleFavorite(name)
		}
		m.reloadLibrary()
		if len(m.lib.Themes) > 0 {
			next, ok := m.lib.Get(m.applied)
			if !ok || next.Name == name {
				next = m.lib.Themes[0]
			}
			m.showTheme(next)
		}
		m.recompute()
		m.info("Deleted %s", name)
		return nil
	})
}

// revert restores the theme to its saved version, or drops a draft.
func (m *Model) revert() {
	if m.cur == nil {
		return
	}
	if m.cur.Source == ghostty.SourceDraft {
		if t, ok := m.lib.Get(m.applied); ok {
			m.showTheme(t)
			m.info("Discarded the draft, back to %s", t.Name)
		} else if len(m.lib.Themes) > 0 {
			m.showTheme(m.lib.Themes[0])
		}
		m.recompute()
		return
	}
	m.cur = m.base.Clone()
	m.dirty = false
	m.discardArmed = false
	m.c = chromeFor(m.cur)
	m.live.Reset()
	m.live.Show(m.cur.Colors)
	m.recompute()
	m.info("Reverted %s to the saved version", m.cur.Name)
}

// --- colour editing -------------------------------------------------------

// setColor is the single funnel for every colour change: hex entry, arrow
// nudges and generated themes all land here.
func (m *Model) setColor(key, hex string) {
	hex = color.Normalize(hex, m.cur.Colors[key])
	if hex == m.cur.Colors[key] {
		return
	}
	m.cur.Colors[key] = hex
	m.dirty = !m.cur.Equal(m.base) || m.cur.Source == ghostty.SourceDraft
	m.discardArmed = false
	m.c = chromeFor(m.cur)
	m.live.Show(map[string]string{key: hex})
}

// clearColor empties a slot, which is only possible for the optional pairs.
func (m *Model) clearColor(key string) {
	delete(m.cur.Colors, key)
	m.dirty = !m.cur.Equal(m.base)
	m.live.Reset()
	m.live.Show(m.cur.Colors)
}

// adjust nudges a slot one step.
func (m *Model) adjust(key string, kind color.Adjustment, up bool) {
	if key == "" || m.cur == nil {
		return
	}
	current := m.cur.Colors[key]
	if current == "" {
		m.setColor(key, m.derivedDefault(key))
		return
	}
	m.setColor(key, color.Adjust(current, kind, up))
}

// undoColor restores a slot from the snapshot.
func (m *Model) undoColor(key string) {
	if key == "" || m.cur == nil {
		return
	}
	original, ok := m.base.Colors[key]
	if !ok {
		m.clearColor(key)
		return
	}
	m.setColor(key, original)
	m.info("Reverted %s to %s", ghostty.Label(key), original)
}

// derivedDefault proposes a value for an empty slot so filling one in starts
// from something sensible instead of black.
func (m *Model) derivedDefault(key string) string {
	bg, fg := m.cur.Background(), m.cur.Foreground()
	switch key {
	case "background":
		return bg
	case "foreground", "cursor-color", "selection-foreground":
		return fg
	case "cursor-text":
		return bg
	case "selection-background":
		return color.Blend(bg, color.Normalize(m.cur.Get("palette.4"), m.c.accent), 0.38)
	}
	idx, _ := ghostty.PaletteIndex(key)
	fallback := []string{"#000000", "#ff0000", "#00ff00", "#ffff00", "#0000ff", "#ff00ff", "#00ffff", "#ffffff"}[idx%8]
	if idx >= 8 {
		return color.Blend(color.Normalize(m.cur.Get(paletteKey(idx-8)), fallback), fg, 0.30)
	}
	return color.Normalize(m.cur.Get(paletteKey(idx+8)), fallback)
}

// --- generated themes -----------------------------------------------------

// loadDraft puts a generated palette on screen as an unsaved draft.
func (m *Model) loadDraft(s *color.Scheme, note string) {
	t := ghostty.FromScheme(s, note)
	m.cur = t
	m.base = t.Clone()
	m.dirty = true
	m.discardArmed = false
	m.c = chromeFor(m.cur)
	m.live.Reset()
	m.live.Show(m.cur.Colors)
	m.recompute()
}

// surprise is the random command: a harmonious palette in the current
// light or dark register.
func (m *Model) surprise() {
	dark := true
	if m.cur != nil {
		dark = m.cur.IsDark()
	}
	h := color.Harmony(randInt(len(color.HarmonyNames)))
	s := color.Harmonious(dark, h, 0, false)
	m.loadDraft(s, fmt.Sprintf("Generated · %s harmony", strings.ToLower(color.HarmonyNames[h])))
	m.info("Generated %s — random again for another, save <name> keeps it, undo goes back", s.Name)
}

// --- favourites -----------------------------------------------------------

func (m *Model) toggleFavorite() {
	if m.cur == nil || m.cur.Source == ghostty.SourceDraft {
		m.warn("Save the draft first")
		return
	}
	name := m.cur.Name
	on := m.state.ToggleFavorite(name)
	_ = m.state.Save()
	m.recompute()
	if on {
		m.info("♥ %s added to favourites — favs lists them", name)
	} else {
		m.info("%s removed from favourites", name)
	}
}

// --- status line ----------------------------------------------------------

// renderStatus rebuilds the info line from the selection.
func (m *Model) renderStatus() {
	if m.cur == nil {
		m.status, m.statusKind = "", statusPlain
		return
	}
	if m.editing {
		v := m.cur.Colors[m.editKey]
		m.status = ghostty.Label(m.editKey) + "  " + color.Describe(v, m.cur.Background(), m.editKey != "background")
		m.statusKind = statusPlain
		return
	}
	var b strings.Builder
	b.WriteString(m.cur.Name + "  ·  " + sourceWord(m.cur))
	b.WriteString("  ·  font " + orDash(m.fontFamily) + " " + ghostty.FormatSize(m.fontSize))
	m.status, m.statusKind = b.String(), statusPlain
}
