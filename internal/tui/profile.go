package tui

import (
	"image"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/ghostty"
	"github.com/vitruves/ghostty-config/internal/pix"
)

// The editor can be laid out three ways, its profiles: Atelier, the sections
// and the wall of themes; Palette, one search line over everything; and
// Guided, a few steps through what matters most. They share everything
// underneath, so a profile is only a way of showing the Model and of
// turning keys into what the Model already does.
//
// What a profile implements is the profile interface below, in files of its
// own (atelier.go, palette*.go, guided*.go), and it is registered once, in
// profileKinds. The Model routes to the profile in effect, in both renderers:
// View draws it under the overlays (render, drawScreen), and Update hands it
// the keys and the mouse that nothing shared took.
//
// What stays the Model's, and a profile only calls:
//   - the settings: m.commandNamed(name) and m.commands() are every command,
//     a setting's among them (c.set: get, label, explain); c.options(m, arg)
//     lists its values and m.runCommand(c, arg, opt) runs one, which writes
//     the file, asks Ghostty to reload and says so on the status line;
//     m.listed(c) is whether it matters now.
//   - themes and fonts: m.lib.Themes, m.themeOptions(arg), m.previewTheme(t)
//     to show one, the "theme" command to apply it, m.cur the one shown;
//     m.families after m.ensureFonts(), the "font" command to pick one.
//   - the status line, m.status and m.statusKind, set by m.info, m.warn and
//     m.fail; the profile draws it where it likes.
//   - the dialogs: m.openConfirm, m.openMessage, overlayHelp, and the
//     overlays drawn over the profile in both renderers. They own the
//     keyboard while open, before the profile sees a key.
//   - the keys every profile has, handled before it: Ctrl+C and Ctrl+D,
//     F2 to step aside, F3 to read Ghostty's page on what explainTarget
//     names, Ctrl+Z to undo the last step (m.undoStep). Ctrl+T, trying the
//     theme shown in this terminal, is m.tryHere for the profile to bind.
//   - m.quit, the way out every profile's Esc ends in, and m.reload.
//   - the look: m.panelChrome() for the cells, newPxPal and pxFaces for the
//     picture; m.addRegion and m.pxRegion for what a click reaches, read
//     back with m.hit.
//   - m.switchProfile, which the chooser and Tool → Interface → Layout call:
//     it ends what the last profile was in the middle of, throws away every
//     picture the terminal holds and sends the screen again, then enters
//     the new profile.

// profile is one layout of the editor.
type profile interface {
	// viewCells draws the screen in the text interface, row by row; the
	// rows are filled to the window and the overlays laid over them.
	viewCells(m *Model) string
	// drawPixel paints the screen in the picture interface on a cleared
	// canvas; the overlays are painted over it.
	drawPixel(m *Model, c *pix.Canvas, p pxPal, f pxFonts, g pxGeo)
	// key takes a key no dialog and no shared key took.
	key(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd)
	// mouse takes a click of the left button or a turn of the wheel while
	// no dialog is open.
	mouse(m *Model, msg mouseEv) (tea.Model, tea.Cmd)
	// explainTarget is what F3 reads Ghostty's page on: an option, or an
	// action when action is true; "" when there is none.
	explainTarget(m *Model) (name string, action bool)
	// enter is called when the profile comes into effect: at the start, and
	// after a switch.
	enter(m *Model) tea.Cmd
}

// profileKind is a profile as the chooser offers it.
type profileKind struct {
	name, title, blurb string
	make               func() profile
}

// profileKinds are the profiles, in the order the chooser shows them. This
// is the one place a profile is registered.
var profileKinds = []profileKind{
	{ghostty.ProfileAtelier, "Atelier", "Sections, a wall of theme cards, a live preview beside every setting. Browse and compare.", func() profile { return atelier{} }},
	{ghostty.ProfilePalette, "Palette", "One search line. Type what you want: themes, settings and commands answer in one list.", newPalette},
	{ghostty.ProfileGuided, "Guided", "Six short steps through what matters most: colours, font, window, cursor, copy and paste.", newGuided},
}

// profileIndex is where a profile sits among profileKinds, 0 for none.
func profileIndex(name string) int {
	for i, k := range profileKinds {
		if k.name == name {
			return i
		}
	}
	return 0
}

// profile is the profile in effect. Each is made once per session, so one
// switched away from and back to finds itself as it was left.
func (m *Model) profile() profile {
	name := m.state.ActiveProfile()
	if p, ok := m.profs[name]; ok {
		return p
	}
	if m.profs == nil {
		m.profs = map[string]profile{}
	}
	p := profileKinds[profileIndex(name)].make()
	m.profs[name] = p
	return p
}

// switchProfile puts another profile in effect and keeps it. What the last
// one was in the middle of ends; the pictures the terminal holds are thrown
// away and the whole screen is sent again, as after a resize, since the
// rows of one profile have nothing in common with the rows of another.
func (m *Model) switchProfile(name string) tea.Cmd {
	m.endPrefPreview()
	if m.editing {
		m.stopEdit()
	}
	m.state.Profile = name
	_ = m.state.Save()
	m.regions = m.regions[:0]
	m.previewScrl = 0
	m.centre = true
	if m.px != nil {
		m.px.galTop, m.px.listTop = 0, 0
		m.px.pics = map[string]*image.RGBA{}
	}
	return tea.Batch(m.profile().enter(m), m.armImages())
}

// commandNamed is the command a word names exactly, nil for none.
func (m *Model) commandNamed(name string) *command {
	for _, c := range m.commands() {
		if c.name == name {
			return c
		}
	}
	return nil
}

// runCommand runs a command as Enter would, without the prompt: opt is the
// value chosen, arg what was typed, either may be empty. A command that said
// nothing leaves the status line telling what is shown.
func (m *Model) runCommand(c *command, arg string, opt *option) tea.Cmd {
	if c == nil || c.run == nil {
		return nil
	}
	seq := m.statusSeq
	cmd := c.run(m, arg, opt)
	if m.statusSeq == seq {
		m.renderStatus()
	}
	return cmd
}
