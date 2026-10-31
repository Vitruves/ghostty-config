package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/color"
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
	hitRail
	hitValue // a value of a setting's control: name is the value, or "slider"
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

// mouseEv is a mouse event reduced to what the editor asks of it, with the
// row counted from the top of the editor rather than of the terminal.
type mouseEv struct {
	X, Y                 int
	Button               tea.MouseButton
	press, motion, wheel bool
}

// updateMouse routes clicks, drags, hovering and the wheel.
func (m *Model) updateMouse(raw tea.MouseMsg) (tea.Model, tea.Cmd) {
	mo := raw.Mouse()
	msg := mouseEv{X: mo.X, Y: mo.Y - m.top, Button: mo.Button}
	switch raw.(type) {
	case tea.MouseClickMsg:
		msg.press = true
	case tea.MouseMotionMsg:
		msg.motion = true
	case tea.MouseWheelMsg:
		msg.wheel = true
	}
	if m.peek {
		if msg.press {
			m.peek = false
			return m, m.armImages()
		}
		return m, nil
	}
	if msg.motion {
		return m, nil
	}
	if m.overlay != overlayNone {
		if msg.wheel {
			return m.wheel(msg)
		}
		if !msg.press || msg.Button != tea.MouseLeft {
			return m, nil
		}
		if m.overlay == overlaySettings {
			return m.clickSettings(msg)
		}
		if r, ok := m.hit(msg.X, msg.Y); ok && r.kind == hitButton {
			return m.pressButton(r.name)
		}
		if m.overlay == overlayWelcome || m.overlay == overlayHelp || m.overlay == overlayMessage || m.overlay == overlayExplain {
			return m.updateOverlay(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		return m, nil
	}
	if !msg.wheel && (!msg.press || msg.Button != tea.MouseLeft) {
		return m, nil
	}
	return m.profile().mouse(m, msg)
}

// atelierMouse is Atelier's answer to a click or the wheel.
func (m *Model) atelierMouse(msg mouseEv) (tea.Model, tea.Cmd) {
	if msg.wheel {
		return m.wheel(msg)
	}
	r, ok := m.hit(msg.X, msg.Y)
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
		m.browsed = true
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
	case hitRail:
		// A group of the wall: go to its first theme, its title at the top.
		m.sel.cursor = r.index
		m.skipHeaders(1)
		m.galAlign = true
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
	case hitValue:
		return m.clickValue(r, msg.X)
	}
	return m, nil
}

// clickValue sets a setting from its control: a pill, a switch, or a point
// along a slider.
func (m *Model) clickValue(r region, x int) (tea.Model, tea.Cmd) {
	if r.index < 0 || r.index >= len(m.results) {
		return m, nil
	}
	m.sel.cursor = r.index
	m.browsed = true
	move := m.afterMove()
	o := m.results[r.index]
	if o.cmd == nil || o.cmd.set == nil {
		return m, move
	}
	s := o.cmd.set
	v := r.name
	if v == "slider" {
		// The first cell of the track is the minimum, the last the maximum.
		frac := float64(x-r.x) / float64(maxInt(r.w-1, 1))
		v = s.format(roundStep(color.Clamp(s.min+frac*(s.max-s.min), s.min, s.max), s.step))
	}
	if strings.EqualFold(v, s.get(m)) {
		return m, move
	}
	cmd := o.cmd.run(m, "", &option{value: v})
	m.recompute()
	return m, tea.Batch(move, cmd)
}

// pressButton runs a dialog button.
func (m *Model) pressButton(name string) (tea.Model, tea.Cmd) {
	if i, ok := strings.CutPrefix(name, "layout-"); ok && m.overlay == overlayLayout && len(i) == 1 {
		// A layout of the chooser: a click picks it, and a click on the
		// one picked goes on with it.
		at := int(i[0] - '0')
		if at == m.layoutAt {
			return m.updateOverlay(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		m.layoutAt = at
		return m, nil
	}
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
	case "update-install":
		return m, m.startUpdate()
	case "capture-set":
		return m, m.commitCapture()
	case "capture-cancel":
		m.overlay = overlayNone
	case "capture-global":
		m.kb.global = !m.kb.global
	case "update-later":
		m.overlay = overlayNone
	case "layout-continue":
		return m.updateOverlay(tea.KeyPressMsg{Code: tea.KeyEnter})
	case "enter":
		// The footer's button does what Enter does.
		return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	return m, nil
}

// wheel scrolls the results, or the panel when the pointer is over it.
func (m *Model) wheel(msg mouseEv) (tea.Model, tea.Cmd) {
	delta := 1
	if msg.Button == tea.MouseWheelUp {
		delta = -1
	}
	if m.overlay == overlayExplain {
		m.ex.scrollBy(delta * 3)
		return m, nil
	}
	if m.overlay == overlaySettings {
		return m.updateSettings(map[int]string{-1: "up", 1: "down"}[delta])
	}
	if m.overlay != overlayNone {
		return m, nil
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
	if m.screen() == screenGallery {
		return m, m.gridMove(0, delta)
	}
	return m, m.move(delta)
}
