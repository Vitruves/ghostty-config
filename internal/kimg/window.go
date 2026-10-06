package kimg

// WindowSpec is what the picture of a Ghostty window shows: the theme it
// wears and the settings that change its frame.
type WindowSpec struct {
	BG, FG string
	Cursor string
	Pal    [16]string
	Screen string // the colour of the screen the picture sits on
	// Behind are the two colours of what lies behind the window, which a
	// translucent background lets through.
	Behind [2]string

	Titlebar string  // native, transparent, tabs, hidden or none
	Shadow   bool    // the window casts one
	Opacity  float64 // 1 is opaque
	Blur     bool    // what shows through is blurred
	PadX     int     // points between the text and the window's edge
	PadY     int
}

// Window draws a Ghostty window as its settings would have it: the title bar
// in one of its styles, the padding round the text, the shadow, and, when the
// background is translucent, what lies behind showing through.
func Window(cols, rows int, cw, ch float64, s WindowSpec) *Canvas {
	scale := cw / 10
	c := NewCanvas(int(float64(cols)*cw+0.5), int(float64(rows)*ch+0.5), scale)
	W, H := c.W(), c.H()
	wx, wy, ww, wh := 12.0, 8.0, W-24, H-24
	bare := s.Titlebar == "none"
	r := 11.0
	if bare {
		r = 0
	}

	// What lies behind: two soft discs. They only matter through a
	// translucent window, where blur decides how sharp they stay.
	if s.Opacity < 1 {
		behind := NewCanvas(int(float64(cols)*cw+0.5), int(float64(rows)*ch+0.5), scale)
		behind.Circle(wx+ww*0.26, wy+wh*0.52, wh*0.34, RGBA(s.Behind[0], 1))
		behind.Circle(wx+ww*0.74, wy+wh*0.60, wh*0.40, RGBA(s.Behind[1], 1))
		if s.Blur {
			behind.Blur(13)
		} else {
			behind.Blur(2)
		}
		c.Draw(behind, 0, 0)
	}

	if s.Shadow && !bare {
		alpha := 0.2
		if s.Screen == "" || darkHex(s.Screen) {
			alpha = 0.55
		}
		c.Shadow(wx, wy, ww, wh, r, 13, 6, RGBA("#000000", alpha))
	}
	opacity := s.Opacity
	if opacity <= 0 || opacity > 1 {
		opacity = 1
	}
	c.Fill(wx, wy, ww, wh, r, RGBA(s.BG, opacity))
	c.Ring(wx, wy, ww, wh, r, 1, RGBA(mix(s.BG, s.FG, 0.2), 1))

	// The title bar.
	bar := 0.0
	if s.Titlebar == "native" || s.Titlebar == "transparent" || s.Titlebar == "tabs" {
		bar = 24
		fill := ""
		switch s.Titlebar {
		case "native":
			// The system's own grey, whatever the theme.
			fill = "#e6e6e8"
			if s.Screen == "" || darkHex(s.Screen) {
				fill = "#3a3a3d"
			}
		case "tabs":
			fill = mix(s.BG, s.FG, 0.13)
		}
		if fill != "" {
			c.Fill(wx, wy, ww, bar, r, RGBA(fill, 1))
			c.Fill(wx, wy+bar-r, ww, r, 0, RGBA(fill, 1))
		}
		for i := 0; i < 3; i++ {
			c.Circle(wx+15+float64(i)*14, wy+bar/2, 4.2, RGBA(mix(s.BG, s.FG, 0.38), 1))
		}
		if s.Titlebar == "tabs" {
			c.Fill(wx+66, wy+5, 96, bar-5, 6, RGBA(s.BG, 1))
			c.Fill(wx+66, wy+bar-6, 96, 6, 0, RGBA(s.BG, 1))
			c.Fill(wx+168, wy+5, 96, bar-5, 6, RGBA(mix(s.BG, s.FG, 0.06), 1))
			c.Fill(wx+168, wy+bar-6, 96, 6, 0, RGBA(mix(s.BG, s.FG, 0.06), 1))
		}
	}

	// The text, as far from the edges as the padding says.
	pal := func(i int) string {
		if s.Pal[i] != "" {
			return s.Pal[i]
		}
		return s.FG
	}
	mono := c.F(true, false, 10.5)
	left := wx + 6 + float64(s.PadX)
	y := wy + bar + 15 + float64(s.PadY)
	put := func(parts ...[2]string) {
		px := left
		for _, p := range parts {
			px += c.Text(mono, px, y, p[1], RGBA(p[0], 1))
		}
		y += 16
	}
	dim := mix(s.FG, s.BG, 0.5)
	prompt := [][2]string{{pal(6), "~/src/ghostty-config "}, {pal(5), "main "}, {pal(2), "› "}}
	put(append(prompt, [2]string{s.FG, "git status --short"})...)
	put([2]string{pal(3), " M "}, [2]string{s.FG, "internal/tui/screen.go"})
	put([2]string{pal(2), " A "}, [2]string{s.FG, "internal/kimg/window.go"})
	put([2]string{dim, "?? notes.md"})
	if y+16 < wy+wh-float64(s.PadY) {
		px := left
		for _, p := range prompt {
			px += c.Text(mono, px, y, p[1], RGBA(p[0], 1))
		}
		c.Fill(px+1, y-10, 7, 13, 1.5, RGBA(s.Cursor, 1))
	}
	return c
}
