package kimg

import "image/color"

// darkHex reports whether a colour is nearer black than white.
func darkHex(hex string) bool {
	c := RGBA(hex, 1)
	return 0.2126*float64(c.R)+0.7152*float64(c.G)+0.0722*float64(c.B) < 128
}

// Tile draws a theme as one card of the wall of themes: its ground, a
// specimen of its text with the cursor beside it, two lines of output and
// its sixteen colours. Every tile shows the same words, so two themes side
// by side differ only in what the theme decides.
func Tile(cols, rows int, cw, ch float64, s CardSpec) *Canvas {
	scale := cw / 10
	c := NewCanvas(int(float64(cols)*cw+0.5), int(float64(rows)*ch+0.5), scale)
	W, H := c.W(), c.H()
	bx, by, bw, bh := 6.0, 5.0, W-12, H-13
	const r = 11.0

	ring := s.Ring
	if ring == "" {
		ring = s.Accent
	}
	if s.Selected {
		c.Shadow(bx-2, by-2, bw+4, bh+4, r+2, 8, 0, RGBA(ring, 0.5))
	} else {
		// A shadow is only seen on a light screen if it stays light.
		alpha := 0.16
		if s.Screen == "" || darkHex(s.Screen) {
			alpha = 0.5
		}
		c.Shadow(bx, by, bw, bh, r, 7, 3, RGBA("#000000", alpha))
	}
	c.Fill(bx, by, bw, bh, r, RGBA(s.BG, 1))
	// A hairline so a card does not melt into a screen of its own colour.
	c.Ring(bx, by, bw, bh, r, 1, RGBA(mix(s.BG, s.FG, 0.16), 1))
	if s.Selected {
		c.Ring(bx-1, by-1, bw+2, bh+2, r+1, 2.2, RGBA(ring, 1))
	}

	pal := func(i int) string {
		if i >= 0 && i < 16 && s.Pal[i] != "" {
			return s.Pal[i]
		}
		return s.FG
	}
	left := bx + 13

	// The specimen and the cursor.
	big := c.F(false, true, 25)
	x := left + c.Text(big, left, by+32, "Aa", RGBA(s.FG, 1))
	c.Fill(x+5, by+12, 9, 21, 2, RGBA(s.Accent, 1))

	// Two lines of output: a prompt, a path, an addition and a removal.
	mono := c.F(true, false, 10.5)
	dim := mix(s.FG, s.BG, 0.5)
	put := func(y float64, parts ...[2]string) {
		px := left
		for _, p := range parts {
			px += c.Text(mono, px, y, p[1], RGBA(p[0], 1))
		}
	}
	put(by+55, [2]string{pal(2), "› "}, [2]string{s.FG, "git diff "}, [2]string{dim, "--stat"})
	put(by+70, [2]string{pal(4), "app.go "}, [2]string{pal(2), "+12 "}, [2]string{pal(1), "-3"})

	// The sixteen colours as one rounded strip.
	sw, sy, sh := bw-26, by+bh-16, 6.0
	c.FillFunc(left, sy, sw, sh, sh/2, func(px, _ float64) color.NRGBA {
		i := int((px - left) / sw * 16)
		if i < 0 {
			i = 0
		}
		if i > 15 {
			i = 15
		}
		return RGBA(pal(i), 1)
	})
	return c
}
