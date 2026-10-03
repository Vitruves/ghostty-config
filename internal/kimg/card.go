package kimg

import (
	"fmt"
	"image/color"
	"strings"
)

// CardSpec is what one theme card shows.
type CardSpec struct {
	Name, Family string
	Dark         bool
	BG, FG       string
	Accent       string // the cursor colour, the card's signature
	Pal          [16]string
	Ratio        float64
	Grade        string
	Selected     bool
	Hot          bool   // the pointer is over it
	HotColour    string // the ring colour for a hovered card
	Applied, Fav bool
	Screen       string // the colour of the screen the card sits on
}

func mix(a, b string, t float64) string {
	ca, cb := RGBA(a, 1), RGBA(b, 1)
	f := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", f(ca.R, cb.R), f(ca.G, cb.G), f(ca.B, cb.B))
}

func tokColour(s CardSpec, t Tok) string {
	switch {
	case t.Idx == Fg:
		return s.FG
	case t.Idx == Dim:
		if s.Pal[8] != "" {
			return s.Pal[8]
		}
		return mix(s.FG, s.BG, 0.5)
	case t.Idx == Cursor:
		return s.Accent
	case t.Idx >= 0 && t.Idx < 16 && s.Pal[t.Idx] != "":
		return s.Pal[t.Idx]
	}
	return s.FG
}

// glyphs the embedded fonts do not carry are replaced by ones they do.
var glyphFix = strings.NewReplacer("❯", "›", "✓", "+", "☾", "", "☀", "")

// Card draws a theme card cols by rows cells large, a cell being cw by ch
// device pixels.
func Card(cols, rows int, cw, ch float64, s CardSpec) *Canvas {
	scale := cw / 10
	c := NewCanvas(int(float64(cols)*cw+0.5), int(float64(rows)*ch+0.5), scale)
	W, H := c.W(), c.H()
	bx, by, bw, bh := 8.0, 5.0, W-16, H-15
	const r = 14.0

	accent := s.Accent
	switch {
	case s.Selected:
		c.Shadow(bx-4, by-4, bw+8, bh+8, r+4, 12, 0, RGBA(accent, 0.55))
	case s.Hot:
		hc := s.HotColour
		if hc == "" {
			hc = accent
		}
		c.Shadow(bx-2, by-2, bw+4, bh+4, r+2, 9, 0, RGBA(hc, 0.4))
	default:
		c.Shadow(bx, by, bw, bh, r, 8, 4, RGBA("#000000", 0.5))
	}
	c.Fill(bx, by, bw, bh, r, RGBA(s.BG, 1))
	// A hairline so a dark card does not melt into a dark screen.
	edge := mix(s.BG, s.FG, 0.16)
	c.Ring(bx, by, bw, bh, r, 1, RGBA(edge, 1))
	switch {
	case s.Selected:
		c.Ring(bx-1, by-1, bw+2, bh+2, r+1, 2.2, RGBA(accent, 1))
	case s.Hot:
		hc := s.HotColour
		if hc == "" {
			hc = accent
		}
		c.Ring(bx-0.5, by-0.5, bw+1, bh+1, r+0.5, 1.6, RGBA(hc, 1))
	}

	// Title and family chip.
	x := bx + 16
	if s.Applied {
		c.Circle(x+3, by+19, 3.2, RGBA(accent, 1))
		x += 12
	}
	c.Text(c.F(false, true, 13.5), x, by+24, s.Name, RGBA(accent, 1))
	if s.Fav {
		heart(c, bx+bw-16-7, by+14, 11, RGBA("#f7768e", 1))
	}
	chipF := c.F(false, false, 10)
	cwid := c.Width(chipF, s.Family) + 16
	cx := bx + bw - 14 - cwid
	if s.Fav {
		cx -= 18
	}
	c.Fill(cx, by+11, cwid, 16, 8, RGBA(accent, 0.16))
	c.Text(chipF, cx+8, by+22.5, s.Family, RGBA(accent, 1))

	// Sample output.
	mono := c.F(true, false, 11.5)
	for i, line := range Sample(s.Name) {
		px := bx + 12
		y := by + 48 + float64(i)*17
		for _, t := range line {
			text := glyphFix.Replace(t.Text)
			if t.Idx == Cursor {
				c.Fill(px+1, y-10.5, 7, 14, 1.5, RGBA(tokColour(s, t), 1))
				px += c.Width(mono, " ")
				continue
			}
			px += c.Text(mono, px, y, text, RGBA(tokColour(s, t), 1))
		}
	}

	// The sixteen colours as one rounded strip.
	ry, rh := by+bh-34, 9.0
	rw := bw - 28
	segs := 16
	c.FillFunc(bx+14, ry, rw, rh, rh/2, func(px, _ float64) color.NRGBA {
		i := int((px - (bx + 14)) / rw * float64(segs))
		if i < 0 {
			i = 0
		}
		if i >= segs {
			i = segs - 1
		}
		hex := s.Pal[i]
		if hex == "" {
			hex = s.FG
		}
		return RGBA(hex, 1)
	})

	// Readability, in small type under the strip.
	dim := mix(s.FG, s.BG, 0.42)
	info := c.F(false, false, 10.5)
	dot := "#8fdc7b"
	if s.Ratio < 7 {
		dot = "#e6b450"
	}
	c.Circle(bx+19, by+bh-10.5, 3, RGBA(dot, 1))
	c.Text(info, bx+27, by+bh-7, fmt.Sprintf("%s  %.1f:1", s.Grade, s.Ratio), RGBA(dim, 1))
	return c
}

func heart(c *Canvas, cx, cy, size float64, col color.NRGBA) {
	r := size * 0.27
	c.Circle(cx-r, cy-r*0.4, r*1.05, col)
	c.Circle(cx+r, cy-r*0.4, r*1.05, col)
	// the point: a diamond below the two lobes
	for i := 0; i < int(size); i++ {
		t := float64(i) / size
		w := (1 - t) * size * 0.56
		c.Fill(cx-w, cy+float64(i)*0.55-r*0.2, 2*w, 1.2, 0.5, col)
	}
}
