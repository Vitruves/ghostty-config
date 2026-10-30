package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/pix"
)

// The chooser asks how the editor should be laid out, before anything else
// and until it is answered: the three profiles side by side, each with a
// small drawing of its screen. Esc does not skip it, since there is nothing
// behind it to go back to; Ctrl+C still leaves. A new user goes on to the
// welcome when the choice is Atelier, whose keys it describes; someone who
// used the tool before is told the layout they had is still there.

// chooserLead is what the chooser says under its question.
func (m *Model) chooserLead() string {
	if m.state.Returning() {
		return "This version adds two new ways to lay the tool out. Pick one to go on; Atelier is the one you had."
	}
	return "Three layouts, the same settings underneath. Pick the one that suits you."
}

// chooserBadge is the corner note: why the question is asked now.
func (m *Model) chooserBadge() string {
	if !m.state.Returning() {
		return "first launch"
	}
	if v := m.opts.Version; v != "" {
		return "updated to " + v
	}
	return "updated"
}

// chooserTag marks Atelier: the default for a new user, the layout a
// returning one had.
func (m *Model) chooserTag() string {
	if m.state.Returning() {
		return "current"
	}
	return "default"
}

const chooserNote = "You can change it any time: Tool → Interface → Layout."

// updateChooser moves between the layouts and keeps one.
func (m *Model) updateChooser(key string) (tea.Model, tea.Cmd) {
	n := len(profileKinds)
	switch key {
	case "left", "h", "shift+tab", "up", "k":
		m.layoutAt = (m.layoutAt + n - 1) % n
	case "right", "l", "tab", "down", "j":
		m.layoutAt = (m.layoutAt + 1) % n
	case "1", "2", "3":
		if i := int(key[0] - '1'); i < n {
			m.layoutAt = i
		}
	case "enter", "space":
		m.overlay = overlayNone
		return m, m.switchProfile(profileKinds[m.layoutAt].name)
	}
	return m, nil
}

// --- the drawings ------------------------------------------------------------

// sketchTone is the colour of a mark in a drawing, by role, so that one
// drawing serves both interfaces and every scheme.
type sketchTone int

const (
	skInk    sketchTone = iota // text that leads
	skLine                     // text that follows
	skFaint                    // text that recedes
	skAccent                   // what is on
	skField                    // a field or a quiet ground
	skSel                      // the highlighted row
	skHex                      // the colour of a theme
)

// sketchMark is one rectangle of a drawing, on a box 100 wide and 60 tall.
// dot draws it as a disc, ring as the outline of one, outline as a ring
// round a rectangle that the text interface is too coarse to draw.
type sketchMark struct {
	x, y, w, h         float32
	tone               sketchTone
	hex                string
	dot, ring, outline bool
}

// sketchW and sketchH are the drawing's box.
const sketchW, sketchH = 100, 60

// sketchThemes are a few themes of the library, grounds and texts, for the
// cards and previews of the drawings.
func (m *Model) sketchThemes() [][2]string {
	out := make([][2]string, 0, 8)
	if n := len(m.lib.Themes); n > 0 {
		for i := 0; i < 8; i++ {
			t := m.lib.Themes[(i*n/8+i*7)%n]
			out = append(out, [2]string{t.Background(), t.Foreground()})
		}
	}
	for len(out) < 8 {
		out = append(out, [2]string{"#1e1e2e", "#cdd6f4"})
	}
	return out
}

// sketchOf is the drawing of a profile's screen.
func sketchOf(name string, th [][2]string) []sketchMark {
	bar := func(x, y, w, h float32, t sketchTone) sketchMark { return sketchMark{x: x, y: y, w: w, h: h, tone: t} }
	hex := func(x, y, w, h float32, v string) sketchMark {
		return sketchMark{x: x, y: y, w: w, h: h, tone: skHex, hex: v}
	}
	var out []sketchMark
	switch name {
	case "palette":
		// One search line, one list under it, and what the line highlighted
		// would look like.
		out = append(out, bar(8, 5, 84, 8, skField), sketchMark{x: 11, y: 7.5, w: 3, h: 3, tone: skLine, ring: true}, bar(17, 8.2, 22, 1.8, skInk))
		out = append(out, bar(8, 16, 84, 6, skSel), bar(11, 18.3, 28, 1.6, skInk), bar(80, 18.3, 9, 1.6, skAccent))
		for i, w := range []float32{36, 24, 31} {
			y := 24 + float32(i)*6
			out = append(out, bar(11, y+2.3, w, 1.6, skLine), bar(80, y+2.3, 9, 1.6, skFaint))
		}
		out = append(out, hex(8, 45, 84, 11, th[2][0]), hex(12, 48, 18, 1.6, th[2][1]), hex(12, 51.5, 30, 1.6, th[2][1]))
	case "guided":
		// The steps across the top, a question with its answers, and the
		// window the answer would make.
		for i := 0; i < 6; i++ {
			x := 4 + float32(i)*15.6
			t, l := skLine, skFaint
			if i < 3 {
				t = skAccent
			}
			if i == 2 {
				l = skInk
			}
			out = append(out, bar(x, 4, 13.6, 1.4, t), bar(x, 7.6, 8, 1.3, l))
		}
		out = append(out, bar(4, 15, 16, 1.4, skLine), bar(4, 19, 34, 3, skInk))
		for i := 0; i < 3; i++ {
			y := 27 + float32(i)*9
			if i == 2 {
				out = append(out, bar(4, y-1, 48, 8, skSel), sketchMark{x: 6.5, y: y + 1.5, w: 3.4, h: 3.4, tone: skAccent, dot: true})
			} else {
				out = append(out, sketchMark{x: 6.5, y: y + 1.5, w: 3.4, h: 3.4, tone: skLine, ring: true})
			}
			out = append(out, bar(13, y+1.2, 16, 1.8, skInk), bar(13, y+4.2, 26, 1.3, skLine))
		}
		out = append(out, hex(58, 15, 38, 41, th[5][0]))
		for i, w := range []float32{14, 22, 17} {
			out = append(out, hex(62, 21+float32(i)*4.5, w, 1.6, th[5][1]))
		}
		out = append(out, bar(62, 34.5, 2.2, 2.6, skAccent))
	default:
		// The sections across the top, the families down the side, and the
		// wall of theme cards.
		out = append(out, bar(4, 4, 15, 2.4, skInk), bar(23, 4.4, 8, 1.6, skAccent))
		for i := 0; i < 3; i++ {
			out = append(out, bar(34+float32(i)*11, 4.4, 8, 1.6, skLine))
		}
		for i := 0; i < 4; i++ {
			t := skLine
			if i == 2 {
				t = skFaint
			}
			out = append(out, bar(4, 13+float32(i)*4.5, 14, 1.6, t))
		}
		cw := float32(96-23-3*2.5) / 4
		for r := 0; r < 3; r++ {
			for col := 0; col < 4; col++ {
				i := (r*4 + col) % len(th)
				mk := hex(23+float32(col)*(cw+2.5), 12+float32(r)*15, cw, 12.5, th[i][0])
				if r == 0 && col == 1 {
					mk.outline = true
				}
				out = append(out, mk)
			}
		}
	}
	return out
}

// sketchColours are the tones of a drawing on a ground.
type sketchColours [skHex]string

func newSketchColours(fg, bg, accent, sel string) sketchColours {
	return sketchColours{
		skInk: fg, skLine: color.Blend(fg, bg, 0.74), skFaint: color.Blend(fg, bg, 0.86),
		skAccent: accent, skField: color.Blend(fg, bg, 0.9), skSel: sel,
	}
}

func (s sketchColours) of(mk sketchMark) string {
	if mk.tone == skHex {
		return mk.hex
	}
	return s[mk.tone]
}

// --- the picture interface ---------------------------------------------------------

// pxChooser is the chooser, drawn to fill the window: the question at the
// top, the layouts across the middle, the way on at the bottom.
func (m *Model) pxChooser(c *pix.Canvas, p pxPal, f pxFonts) {
	W, H := c.W(), c.H()
	M := float32(36)
	if W < 900 {
		M = 24
	}
	ui := p.ui
	c.Text(f.title, M, 34, "ghostty-config", p.fg)
	badge := m.chooserBadge()
	c.Text(f.small, W-M-c.Width(f.small, badge), 34, badge, p.muted)

	big := c.Face(false, true, minF(34, W/30))
	lead := c.Face(false, false, 15.5)
	leads := c.Wrap(lead, m.chooserLead(), W-2*M)

	// The way on, at the bottom.
	footY := H - 30
	bw := c.Width(f.rowB, "Continue") + 14 + 11 + 28
	bx := W - M - bw
	c.Rect(bx, footY-24, bw, 36, 8, p.accent)
	lw := c.Text(f.rowB, bx+14, mid(footY-24, 36, 14), "Continue", p.onAccent)
	pxReturn(c, bx+14+lw+8, mid(footY-24, 36, 14)+1, p.onAccent)
	m.pxRegion(c, bx, footY-24, bw, 36, hitButton, 0, "layout-continue")
	kx := bx - 24 - c.Width(f.body, "Choose")
	c.Text(f.body, kx, mid(footY-24, 36, 13.5), "Choose", p.muted)
	kw := pxKeyLabel(c, f.monoS, 0, 0, "← →", p.fg, false) + 14
	c.Rect(kx-8-kw, footY-16, kw, 20, 5, p.keycap)
	pxKeyLabel(c, f.monoS, kx-8-kw+7, footY-1, "← →", p.fg, true)
	if note := strings.ReplaceAll(chooserNote, "→", "›"); kx-kw-40 > M+c.Width(f.body, note) {
		c.Text(f.body, M, mid(footY-24, 36, 13.5), note, p.muted)
	}

	// The layouts, side by side, as tall as the room between allows.
	gap := float32(24)
	if W < 900 {
		gap = 16
	}
	cw := (W - 2*M - 2*gap) / 3
	// The question and the layouts are one block, set in the middle of the
	// room between the name at the top and the way on at the bottom.
	head := 58 + float32(len(leads))*23 + 18
	y := float32(70)
	bottom := footY - 44
	top := y + head
	pad := float32(16)
	if cw < 240 {
		pad = 12
	}
	nameF := f.h3
	blurbs := make([][]string, len(profileKinds))
	most := 0
	for i, k := range profileKinds {
		blurbs[i] = c.Wrap(f.body, k.blurb, cw-2*pad)
		most = maxInt(most, len(blurbs[i]))
	}
	textH := 14 + 22 + float32(most)*20
	sw := cw - 2*pad
	sh := minF(sw*sketchH/sketchW, bottom-top-2*pad-textH)
	sh = maxF(sh, 0)
	cardH := 2*pad + sh + textH
	if slack := bottom - top - cardH; slack > 0 {
		y += slack * 0.4
		top += slack * 0.4
	}
	c.Text(big, M, y+30, "How do you want to work?", p.fg)
	for i, l := range leads {
		c.Text(lead, M, y+58+float32(i)*23, l, p.text2)
	}
	sk := newSketchColours(ui.fg, ui.bg, ui.accent, color.Blend(ui.fg, ui.bg, 0.86))
	th := m.sketchThemes()
	for i, k := range profileKinds {
		x := M + float32(i)*(cw+gap)
		if i == m.layoutAt {
			c.Rect(x, top, cw, cardH, 14, p.rowSel)
			c.Ring(x, top, cw, cardH, 14, 2, p.accent)
		} else {
			c.Rect(x, top, cw, cardH, 14, p.field)
		}
		m.pxRegion(c, x, top, cw, cardH, hitButton, i, "layout-"+string(rune('0'+i)))
		if sh > 0 {
			bx, by := x+pad, top+pad
			c.Rect(bx, by, sw, sh, 8, p.bg)
			m.pxSketch(c, sk, sketchOf(k.name, th), bx+sw*0.03, by+sh*0.04, sw*0.94, sh*0.92)
		}
		ty := top + pad + sh + 14 + 18
		nw := c.Text(nameF, x+pad, ty, k.title, p.fg)
		if i == 0 {
			tag := m.chooserTag()
			tw := c.Width(f.smallB, tag) + 14
			c.Rect(x+pad+nw+10, ty-15, tw, 20, 5, p.accent)
			c.Text(f.smallB, x+pad+nw+17, ty, tag, p.onAccent)
		}
		yy := ty + 26
		for _, l := range blurbs[i] {
			c.Text(f.body, x+pad, yy, l, p.text2)
			yy += 20
		}
	}
}

// pxSketch paints a drawing into a box.
func (m *Model) pxSketch(c *pix.Canvas, sk sketchColours, marks []sketchMark, x, y, w, h float32) {
	sx, sy := w/sketchW, h/sketchH
	for _, mk := range marks {
		X, Y, MW, MH := x+mk.x*sx, y+mk.y*sy, mk.w*sx, mk.h*sy
		col := pix.Hex(sk.of(mk), 1)
		switch {
		case mk.dot:
			r := minF(MW, MH) / 2
			c.Circle(X+MW/2, Y+MH/2, r, col)
		case mk.ring:
			r := minF(MW, MH)
			c.Ring(X, Y, r, r, r/2, 1.5, col)
		default:
			rad := minF(minF(MW, MH)*0.45, 6)
			if mk.tone == skHex && MH > 12 {
				rad = 4
			}
			c.Rect(X, Y, MW, MH, rad, col)
			if mk.tone == skHex && MH > 12 {
				// A theme as light or as dark as the ground still shows its edge.
				c.Ring(X, Y, MW, MH, rad, 1, pix.Hex(sk[skFaint], 1))
			}
			if mk.outline {
				c.Ring(X-1.5, Y-1.5, MW+3, MH+3, rad+1.5, 1.5, pix.Hex(sk[skAccent], 1))
			}
		}
	}
}

// --- the text interface --------------------------------------------------------

// viewChooser is the chooser in cells, laid out like the picture: the
// question, the layouts in three columns, the way on in the last row.
func (m *Model) viewChooser() string {
	ui := m.panelChrome()
	W, H := m.width, m.height
	mx := 2
	if W >= 100 {
		mx = 4
	}
	inner := maxInt(W-2*mx, 20)
	gap := 3
	cw := maxInt((inner-2*gap)/3, 8)
	left := strings.Repeat(" ", mx)
	var rows []string
	add := func(s string) { rows = append(rows, s) }

	badge := m.chooserBadge()
	head := ui.bold(ui.fg).Render("ghostty-config")
	add(left + head + ui.base().Render(strings.Repeat(" ", maxInt(inner-lipgloss.Width(head)-len(badge), 1))) + ui.mutedS().Render(badge))
	lead := wrapPlain(m.chooserLead(), inner)
	blurbs := make([][]string, len(profileKinds))
	most := 0
	for i, k := range profileKinds {
		blurbs[i] = wrapPlain(k.blurb, cw)
		most = maxInt(most, len(blurbs[i]))
	}
	// Rows besides the drawings: the header, the question and its lead,
	// the mark and the name under each drawing, the blurbs, the way on, and
	// the air between them.
	fixed := 1 + 1 + 1 + len(lead) + 1 + 1 + 1 + most + 1 + 1
	air := 0
	sh := clampInt(H-fixed, 0, cw*sketchH/sketchW/2+1)
	if spare := H - fixed - sh; spare > 0 {
		air = minInt(spare/2, 4)
	}
	for i := 0; i < 1+air; i++ {
		add("")
	}
	add(left + ui.bold(ui.fg).Render("How do you want to work?"))
	for _, l := range lead {
		add(left + ui.mutedS().Render(l))
	}
	add("")
	top := len(rows)

	sk := newSketchColours(ui.fg, ui.bg, ui.accent, color.Blend(ui.fg, ui.bg, 0.84))
	th := m.sketchThemes()
	ground := color.Blend(ui.fg, ui.bg, 0.94)
	cols := make([][]string, len(profileKinds))
	for i, k := range profileKinds {
		picked := i == m.layoutAt
		var col []string
		col = append(col, cellSketch(sk, sketchOf(k.name, th), ground, cw, sh)...)
		if picked {
			col = append(col, ui.text(ui.accent).Render(strings.Repeat("▀", cw)))
		} else {
			col = append(col, ui.base().Render(strings.Repeat(" ", cw)))
		}
		name := ui.bold(ui.fg).Render(k.title)
		if picked {
			name = ui.bold(ui.accent).Render(k.title)
		}
		if i == 0 {
			name += ui.base().Render(" ") + on(ui.bg, ui.accent).Bold(true).Render(" "+m.chooserTag()+" ")
		}
		col = append(col, name)
		for _, l := range blurbs[i] {
			col = append(col, ui.mutedS().Render(l))
		}
		cols[i] = col
		m.addRegion(mx+i*(cw+gap), top, cw, sh+2+most, hitButton, i, "layout-"+string(rune('0'+i)))
	}
	for r := 0; r < sh+2+most; r++ {
		line := left
		for i := range cols {
			cell := ""
			if r < len(cols[i]) {
				cell = cols[i][r]
			}
			if i > 0 {
				line += ui.base().Render(strings.Repeat(" ", gap))
			}
			line += ui.fill(cell, cw)
		}
		add(line)
	}

	for len(rows) < H-1 {
		add("")
	}
	go1 := on(ui.bg, ui.accent).Bold(true).Render(" Continue ⏎ ")
	keys := ui.bold(ui.fg).Render("← →") + ui.mutedS().Render(" choose   ") + go1
	kw := lipgloss.Width(keys)
	note := chooserNote
	if lipgloss.Width(note)+kw+2 > inner {
		note = ""
	}
	m.addRegion(mx+inner-lipgloss.Width(go1), H-1, lipgloss.Width(go1), 1, hitButton, 0, "layout-continue")
	add(left + ui.mutedS().Render(note) + ui.base().Render(strings.Repeat(" ", maxInt(inner-lipgloss.Width(note)-kw, 1))) + keys)
	if len(rows) > H {
		rows = rows[len(rows)-H:]
	}
	for i := range rows {
		rows[i] = rebase(ui.fill(rows[i], W), ui.base())
	}
	return strings.Join(rows, "\n")
}

// cellSketch draws a drawing in half blocks: each cell is two pixels, its
// upper half the foreground of ▀ and its lower half the background.
func cellSketch(sk sketchColours, marks []sketchMark, ground string, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	px := make([][]string, 2*h)
	for y := range px {
		px[y] = make([]string, w)
		for x := range px[y] {
			px[y][x] = ground
		}
	}
	sx, sy := float32(w)/sketchW, float32(2*h)/sketchH
	for _, mk := range marks {
		if mk.ring {
			// Too fine for a cell: a ring reads as the quieter dot.
			mk.tone = skFaint
		}
		x0, x1 := int(mk.x*sx+0.5), int((mk.x+mk.w)*sx+0.5)
		y0, y1 := int(mk.y*sy+0.5), int((mk.y+mk.h)*sy+0.5)
		if x1 <= x0 {
			x1 = x0 + 1
		}
		if y1 <= y0 {
			y1 = y0 + 1
		}
		col := sk.of(mk)
		for y := maxInt(y0, 0); y < minInt(y1, 2*h); y++ {
			for x := maxInt(x0, 0); x < minInt(x1, w); x++ {
				px[y][x] = col
			}
		}
	}
	out := make([]string, h)
	for r := 0; r < h; r++ {
		var b strings.Builder
		for x := 0; x < w; x++ {
			top, bot := px[2*r][x], px[2*r+1][x]
			if top == bot {
				b.WriteString(on(top, top).Render(" "))
			} else {
				b.WriteString(on(top, bot).Render("▀"))
			}
		}
		out[r] = b.String()
	}
	return out
}
