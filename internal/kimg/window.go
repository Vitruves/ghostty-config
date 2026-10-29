package kimg

import (
	"image"
	"image/color"
	"math"
	"strings"
)

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
	// Backdrop is the background image, when one is set.
	Backdrop Backdrop
}

// Backdrop is a background image as the window would draw it. Img is a
// small copy of the picture; W and H are the size of the file itself, which
// is what fit none and the position work from.
type Backdrop struct {
	Img      *image.NRGBA
	W, H     int
	Opacity  float64
	Fit      string // contain, cover, stretch or none
	Position string // top-left … bottom-right, center
	Repeat   bool
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
	bar := 0.0
	if s.Titlebar == "native" || s.Titlebar == "transparent" || s.Titlebar == "tabs" {
		bar = 24
	}
	if s.Backdrop.Img != nil {
		drawBackdrop(c, s.Backdrop, opacity, wx, wy+bar, ww, wh-bar, r, bar > 0)
	}
	c.Ring(wx, wy, ww, wh, r, 1, RGBA(mix(s.BG, s.FG, 0.2), 1))

	// The title bar.
	if bar > 0 {
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

// noneScale is how many logical units of the picture one pixel of an image
// drawn at its own size takes: the window shown stands for one about two
// thousand pixels wide.
const noneScale = 1.0 / 2000

// drawBackdrop lays the background image into the terminal area of the
// window, x, y, w, h, the way Ghostty fits and places it, over the
// background colour and under the text. Its opacity is relative to the
// window's, as Ghostty's is. square keeps the top corners square, under a
// title bar.
func drawBackdrop(c *Canvas, b Backdrop, window float64, x, y, w, h, r float64, square bool) {
	iw, ih := float64(b.W), float64(b.H)
	if iw <= 0 || ih <= 0 {
		iw, ih = float64(b.Img.Bounds().Dx()), float64(b.Img.Bounds().Dy())
	}
	var sx, sy float64
	switch b.Fit {
	case "cover":
		sx = math.Max(w/iw, h/ih)
		sy = sx
	case "stretch":
		sx, sy = w/iw, h/ih
	case "none":
		sx = w * noneScale
		sy = sx
	default: // contain
		sx = math.Min(w/iw, h/ih)
		sy = sx
	}
	dw, dh := iw*sx, ih*sy
	ax, ay := 0.5, 0.5
	pos := b.Position
	switch {
	case strings.HasPrefix(pos, "top-"):
		ay = 0
	case strings.HasPrefix(pos, "bottom-"):
		ay = 1
	}
	switch {
	case strings.HasSuffix(pos, "-left"):
		ax = 0
	case strings.HasSuffix(pos, "-right"):
		ax = 1
	}
	ox, oy := x+(w-dw)*ax, y+(h-dh)*ay
	alpha := b.Opacity
	if alpha <= 0 {
		alpha = 1
	}
	alpha = math.Min(1, alpha*window)

	img := b.Img
	tw, th := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	top, fy, fh := y, y, h
	if square {
		// Start a radius higher, so the rounded corners fall under the bar.
		fy, fh = y-r, h+r
	}
	c.FillFunc(x, fy, w, fh, r, func(px, py float64) color.NRGBA {
		if py < top {
			return color.NRGBA{}
		}
		u, v := (px-ox)/dw, (py-oy)/dh
		if b.Repeat {
			u, v = u-math.Floor(u), v-math.Floor(v)
		} else if u < 0 || u >= 1 || v < 0 || v >= 1 {
			return color.NRGBA{}
		}
		col := sample(img, u*tw, v*th)
		col.A = uint8(float64(col.A) * alpha)
		return col
	})
}

// sample reads a picture between its pixels, bilinearly.
func sample(img *image.NRGBA, fx, fy float64) color.NRGBA {
	b := img.Bounds()
	fx, fy = fx-0.5, fy-0.5
	x0, y0 := int(math.Floor(fx)), int(math.Floor(fy))
	tx, ty := fx-float64(x0), fy-float64(y0)
	at := func(x, y int) [4]float64 {
		x = minI(maxI(x, 0), b.Dx()-1)
		y = minI(maxI(y, 0), b.Dy()-1)
		i := img.PixOffset(b.Min.X+x, b.Min.Y+y)
		p := img.Pix[i : i+4]
		return [4]float64{float64(p[0]), float64(p[1]), float64(p[2]), float64(p[3])}
	}
	a, bb, cc, d := at(x0, y0), at(x0+1, y0), at(x0, y0+1), at(x0+1, y0+1)
	var out [4]uint8
	for k := 0; k < 4; k++ {
		top := a[k]*(1-tx) + bb[k]*tx
		bot := cc[k]*(1-tx) + d[k]*tx
		out[k] = uint8(math.Round(top*(1-ty) + bot*ty))
	}
	return color.NRGBA{out[0], out[1], out[2], out[3]}
}
