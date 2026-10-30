package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/pix"
)

// atelier is the editor as it always was: the sections across the top, the
// prompt under them, and a wall of themes or a list with its explanation.
// It keeps no state of its own; the prompt and the highlight are the Model's.
type atelier struct{}

func (atelier) viewCells(m *Model) string { return m.viewMain() }

func (atelier) drawPixel(m *Model, c *pix.Canvas, p pxPal, f pxFonts, g pxGeo) {
	m.pxHeader(c, p, f, g)
	switch m.screen() {
	case screenGallery:
		m.pxGallery(c, p, f, g)
	case screenMenu:
		m.pxMenu(c, p, f, g)
	case screenOptions:
		if m.cmd.name == "keys" {
			m.pxKeys(c, p, f, g)
		} else {
			m.pxOptions(c, p, f, g)
		}
	case screenEditor:
		m.pxEditor(c, p, f, g)
	}
	m.pxFooter(c, p, f, g)
}

func (atelier) key(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.editing {
		return m.updateEdit(msg)
	}
	return m.updatePrompt(msg)
}

func (atelier) mouse(m *Model, msg mouseEv) (tea.Model, tea.Cmd) { return m.atelierMouse(msg) }

func (atelier) explainTarget(m *Model) (string, bool) { return m.explainTarget() }

// enter shows the welcome the first time: its pages are Atelier's keys.
func (atelier) enter(m *Model) tea.Cmd {
	if !m.state.Welcomed && m.overlay == overlayNone {
		m.overlay = overlayWelcome
	}
	return nil
}
