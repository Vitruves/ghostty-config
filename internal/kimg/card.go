package kimg

import "fmt"

// CardSpec is what one theme card shows.
type CardSpec struct {
	BG, FG   string
	Accent   string // the cursor colour, the card's signature
	Pal      [16]string
	Selected bool
	Screen   string // the colour of the screen the card sits on
	Ring     string // the colour of the ring round a selected card; the accent when empty
}

// mix moves colour a towards b by t, 0 being a and 1 being b.
func mix(a, b string, t float64) string {
	ca, cb := RGBA(a, 1), RGBA(b, 1)
	f := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", f(ca.R, cb.R), f(ca.G, cb.G), f(ca.B, cb.B))
}
