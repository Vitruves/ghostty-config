package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Everything on screen that answers to a click registers a region while it
// is drawn. View rebuilds the list on every frame, so a region is never
// stale: it describes exactly what is on screen now.
type hitKind int

const (
	hitNone hitKind = iota
	hitPrompt
	hitResult
	hitCommand
	hitGroup
	hitPreview
	hitButton
)

type region struct {
	x, y, w, h int
	kind       hitKind
	index      int
	name       string
}

func (m *Model) addRegion(x, y, w, h int, kind hitKind, index int, name string) {
	if w <= 0 || h <= 0 {
		return
	}
	m.regions = append(m.regions, region{x, y, w, h, kind, index, name})
}

// hit finds the topmost region under a cell.
func (m *Model) hit(x, y int) (region, bool) {
	for i := len(m.regions) - 1; i >= 0; i-- {
		r := m.regions[i]
		if x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h {
			return r, true
		}
	}
	return region{}, false
}

// doubleClickWindow is how close two clicks must be to count as one double
// click, which is how a result is run with the mouse.
const doubleClickWindow = 450 * time.Millisecond

type clickMemory struct {
	kind  hitKind
	index int
	at    time.Time
}

// isDouble reports whether this click repeats the previous one in time.
func (m *Model) isDouble(r region) bool {
	now := time.Now()
	same := m.lastClick.kind == r.kind && m.lastClick.index == r.index && now.Sub(m.lastClick.at) < doubleClickWindow
	m.lastClick = clickMemory{r.kind, r.index, now}
	return same
}

// updateMouse routes clicks and wheel movement.
func (m *Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.peek {
		if msg.Action == tea.MouseActionPress {
			m.peek = false
		}
		return m, nil
	}
	if tea.MouseEvent(msg).IsWheel() {
		return m.wheel(msg)
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	r, ok := m.hit(msg.X, msg.Y)
	if m.overlay != overlayNone {
		if ok && r.kind == hitButton {
			return m.pressButton(r.name)
		}
		if m.overlay == overlayWelcome || m.overlay == overlayHelp || m.overlay == overlayMessage {
			return m.updateOverlay(tea.KeyMsg{Type: tea.KeyEnter})
		}
		return m, nil
	}
	if !ok {
		return m, nil
	}
	switch r.kind {
	case hitResult:
		double := m.isDouble(r)
		if m.editing {
			m.stopEdit()
		}
		m.sel.cursor = r.index
		m.sel.clamp(len(m.results), m.resultsHeight())
		cmd := m.afterMove()
		if double {
			return m, tea.Batch(cmd, m.run())
		}
		return m, cmd
	case hitCommand:
		if m.editing {
			m.stopEdit()
		}
		m.setPromptText(title(r.name) + " ")
		return m, m.afterMove()
	case hitGroup:
		if m.editing {
			m.stopEdit()
		}
		m.jumpToGroup(r.name)
		m.renderStatus()
		return m, nil
	case hitPrompt:
		if m.editing {
			m.stopEdit()
		}
		return m, nil
	case hitButton:
		return m.pressButton(r.name)
	}
	return m, nil
}

// pressButton runs a dialog button.
func (m *Model) pressButton(name string) (tea.Model, tea.Cmd) {
	switch name {
	case "confirm-yes":
		m.overlay = overlayNone
		if m.confirmYes != nil {
			return m, m.confirmYes(m)
		}
	case "confirm-no":
		m.overlay = overlayNone
		if m.confirmNo != nil {
			return m, m.confirmNo(m)
		}
	case "close":
		m.overlay = overlayNone
	}
	return m, nil
}

// wheel scrolls the results, or the panel when the pointer is over it.
func (m *Model) wheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.overlay != overlayNone {
		return m, nil
	}
	delta := 1
	if msg.Button == tea.MouseButtonWheelUp {
		delta = -1
	}
	if r, ok := m.hit(msg.X, msg.Y); ok && r.kind == hitPreview {
		m.previewScrl += delta * 3
		if m.previewScrl < 0 {
			m.previewScrl = 0
		}
		return m, nil
	}
	if m.editing {
		m.moveEditSlot(delta)
		return m, nil
	}
	return m, m.move(delta)
}
