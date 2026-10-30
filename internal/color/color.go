// Package color holds the colour arithmetic the editor is built on: hex
// parsing, HSL round-trips, WCAG contrast and the small adjustments the
// palette editor applies one keystroke at a time. Nothing here knows about
// Ghostty or the terminal; it is plain maths on sRGB values.
package color

import (
	"fmt"
	"math"
	"strings"
)

// RGB is an 8-bit sRGB colour.
type RGB struct{ R, G, B int }

// Hex renders the colour as #rrggbb, lower case.
func (c RGB) Hex() string {
	if c.R < 0 || c.R > 255 || c.G < 0 || c.G > 255 || c.B < 0 || c.B > 255 {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	// What Sprintf writes for a channel in range, without its cost: the
	// wall of themes asks for a few thousand colours a frame.
	const digits = "0123456789abcdef"
	return string([]byte{'#', digits[c.R>>4], digits[c.R&15], digits[c.G>>4], digits[c.G&15], digits[c.B>>4], digits[c.B&15]})
}

// HSL is a colour in hue (0-360), saturation (0-1), lightness (0-1).
type HSL struct{ H, S, L float64 }

// ParseHex accepts "#rrggbb", "rrggbb", "#rgb" and "rgb", in any case.
func ParseHex(s string) (RGB, error) {
	t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#"))
	if len(t) == 3 {
		t = string([]byte{t[0], t[0], t[1], t[1], t[2], t[2]})
	}
	// Every character must be a hex digit: a scanner would accept a sign or
	// stop at the first bad digit, turning "#ff000g" into a plausible red.
	if len(t) != 6 {
		return RGB{}, fmt.Errorf("not a hex colour: %q", s)
	}
	var v [6]int
	for i := 0; i < 6; i++ {
		d, ok := hexDigit(t[i])
		if !ok {
			return RGB{}, fmt.Errorf("not a hex colour: %q", s)
		}
		v[i] = d
	}
	return RGB{v[0]<<4 | v[1], v[2]<<4 | v[3], v[4]<<4 | v[5]}, nil
}

func hexDigit(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	}
	return 0, false
}

// Normalize returns hex in canonical #rrggbb form, or fallback when it does
// not parse. Themes ship a mix of casings and lengths; one form keeps the
// columns calm and makes value comparisons meaningful.
func Normalize(hex, fallback string) string {
	if canonical(hex) {
		return hex
	}
	c, err := ParseHex(hex)
	if err != nil {
		return fallback
	}
	return c.Hex()
}

// canonical reports whether hex is already #rrggbb in lower case, which
// is what Normalize would make of it.
func canonical(hex string) bool {
	if len(hex) != 7 || hex[0] != '#' {
		return false
	}
	for i := 1; i < 7; i++ {
		if c := hex[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// IsHex reports whether s parses as a colour.
func IsHex(s string) bool {
	_, err := ParseHex(s)
	return err == nil
}

// ToHSL converts an RGB colour to HSL.
func (c RGB) ToHSL() HSL {
	rf, gf, bf := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	maxV := math.Max(rf, math.Max(gf, bf))
	minV := math.Min(rf, math.Min(gf, bf))
	d := maxV - minV
	l := (maxV + minV) / 2
	if d == 0 {
		return HSL{0, 0, l}
	}
	var s float64
	if l > 0.5 {
		s = d / (2 - maxV - minV)
	} else {
		s = d / (maxV + minV)
	}
	var h float64
	switch maxV {
	case rf:
		h = (gf - bf) / d
		if gf < bf {
			h += 6
		}
	case gf:
		h = (bf-rf)/d + 2
	default:
		h = (rf-gf)/d + 4
	}
	return HSL{h * 60, s, l}
}

// ToRGB converts an HSL colour to RGB.
func (h HSL) ToRGB() RGB {
	hue := NormalizeHue(h.H) / 360
	s := Clamp(h.S, 0, 1)
	l := Clamp(h.L, 0, 1)
	if s == 0 {
		v := int(l*255 + 0.5)
		return RGB{v, v, v}
	}
	hue2rgb := func(p, q, t float64) float64 {
		if t < 0 {
			t++
		}
		if t > 1 {
			t--
		}
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	return RGB{
		ClampInt(int(hue2rgb(p, q, hue+1.0/3)*255+0.5), 0, 255),
		ClampInt(int(hue2rgb(p, q, hue)*255+0.5), 0, 255),
		ClampInt(int(hue2rgb(p, q, hue-1.0/3)*255+0.5), 0, 255),
	}
}

// Hex builds a hex colour from HSL components.
func (h HSL) Hex() string { return h.ToRGB().Hex() }

// HSLHex is a shorthand for building a hex colour from three numbers.
func HSLHex(h, s, l float64) string { return HSL{h, s, l}.Hex() }

// NormalizeHue folds a hue into 0-360.
func NormalizeHue(h float64) float64 {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	return h
}

// HueDistance is the shortest angular distance between two hues.
func HueDistance(a, b float64) float64 {
	d := math.Abs(NormalizeHue(a) - NormalizeHue(b))
	if d > 180 {
		d = 360 - d
	}
	return d
}

// Clamp bounds v to [lo, hi].
func Clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// ClampInt bounds v to [lo, hi].
func ClampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Luminance is the WCAG relative luminance of a colour.
func (c RGB) Luminance() float64 {
	ch := func(v int) float64 {
		if v >= 0 && v <= 255 {
			return linear[v]
		}
		return toLinear(v)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

// toLinear is a channel of 0 to 255 as linear light.
func toLinear(v int) float64 {
	f := float64(v) / 255
	if f <= 0.03928 {
		return f / 12.92
	}
	return math.Pow((f+0.055)/1.055, 2.4)
}

// linear is toLinear of every channel value, worked out once.
var linear = func() (t [256]float64) {
	for v := range t {
		t[v] = toLinear(v)
	}
	return t
}()

// Contrast returns the WCAG contrast ratio between two hex colours, 1 to 21.
// Unparseable input counts as no contrast at all, which errs towards flagging.
func Contrast(a, b string) float64 {
	ca, err1 := ParseHex(a)
	cb, err2 := ParseHex(b)
	if err1 != nil || err2 != nil {
		return 1
	}
	la, lb := ca.Luminance(), cb.Luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// WCAG thresholds.
const (
	ContrastAA  = 4.5
	ContrastAAA = 7.0
	ContrastLow = 3.0 // below this a pair is hard to read at all
)

// Grade labels a ratio the way a theme author thinks about it.
func Grade(ratio float64) string {
	switch {
	case ratio >= ContrastAAA:
		return "AAA"
	case ratio >= ContrastAA:
		return "AA"
	case ratio >= ContrastLow:
		return "AA+"
	}
	return "low"
}

// IsDark reports whether a background reads as dark.
func IsDark(hex string) bool {
	c, err := ParseHex(hex)
	if err != nil {
		return true
	}
	return c.Luminance() < 0.5
}

// Blend mixes a towards b; ratio 0 returns a, ratio 1 returns b.
func Blend(a, b string, ratio float64) string {
	ca, err1 := ParseHex(a)
	cb, err2 := ParseHex(b)
	if err1 != nil || err2 != nil {
		return a
	}
	mix := func(x, y int) int {
		return ClampInt(int(float64(x)*(1-ratio)+float64(y)*ratio+0.5), 0, 255)
	}
	return RGB{mix(ca.R, cb.R), mix(ca.G, cb.G), mix(ca.B, cb.B)}.Hex()
}

// EnsureContrast walks a colour's lightness away from bg until it clears the
// ratio, keeping hue and saturation. It returns the closest it managed.
func EnsureContrast(hex, bg string, ratio float64) string {
	if Contrast(hex, bg) >= ratio {
		return hex
	}
	c, err := ParseHex(hex)
	if err != nil {
		return hex
	}
	b, err := ParseHex(bg)
	if err != nil {
		return hex
	}
	hsl := c.ToHSL()
	lighten := b.Luminance() < 0.5
	best, bestRatio := hex, Contrast(hex, bg)
	for i := 0; i < 48; i++ {
		if lighten {
			hsl.L = math.Min(0.98, hsl.L+0.02)
		} else {
			hsl.L = math.Max(0.02, hsl.L-0.02)
		}
		candidate := hsl.Hex()
		r := Contrast(candidate, bg)
		if r > bestRatio {
			best, bestRatio = candidate, r
		}
		if r >= ratio {
			return candidate
		}
	}
	return best
}

// EnsureReadable lifts fg until it clears AA against bg, falling back to
// plain black or white when the hue simply cannot get there.
func EnsureReadable(fg, bg string) string {
	if Contrast(fg, bg) >= ContrastAA {
		return fg
	}
	out := EnsureContrast(fg, bg, ContrastAA)
	if Contrast(out, bg) >= ContrastAA {
		return out
	}
	if IsDark(bg) {
		return "#ffffff"
	}
	return "#000000"
}

// Editing steps, one keystroke each.
const (
	BrightnessStep = 8    // RGB units
	HueStep        = 8.0  // degrees
	SaturationStep = 0.04 // HSL saturation
	LightnessStep  = 0.05 // HSL lightness
	MinSaturation  = 0.15 // below this a hue change would be invisible
)

// Adjustment names the dimensions the palette editor can nudge.
type Adjustment int

const (
	AdjustBrightness Adjustment = iota
	AdjustHue
	AdjustSaturation
	AdjustLightness
)

// Adjust applies one step of the adjustment to a hex colour.
func Adjust(hex string, kind Adjustment, up bool) string {
	c, err := ParseHex(hex)
	if err != nil {
		return hex
	}
	sign := -1.0
	if up {
		sign = 1
	}
	switch kind {
	case AdjustBrightness:
		d := int(sign) * BrightnessStep
		return RGB{ClampInt(c.R+d, 0, 255), ClampInt(c.G+d, 0, 255), ClampInt(c.B+d, 0, 255)}.Hex()
	case AdjustHue:
		h := c.ToHSL()
		h.H = NormalizeHue(h.H + sign*HueStep)
		if h.S < MinSaturation {
			h.S = MinSaturation
		}
		return h.Hex()
	case AdjustSaturation:
		h := c.ToHSL()
		h.S = Clamp(h.S+sign*SaturationStep, 0, 1)
		return h.Hex()
	default:
		h := c.ToHSL()
		h.L = Clamp(h.L+sign*LightnessStep, 0, 1)
		return h.Hex()
	}
}

// Describe renders the status-bar readout for a colour: hex, rgb, hsl and,
// unless it is the background itself, its contrast against the background.
func Describe(hex, bg string, withContrast bool) string {
	c, err := ParseHex(hex)
	if err != nil {
		return hex
	}
	h := c.ToHSL()
	out := fmt.Sprintf("%s  rgb(%d,%d,%d)  hsl(%.0f°,%.0f%%,%.0f%%)", hex, c.R, c.G, c.B, h.H, h.S*100, h.L*100)
	if !withContrast {
		return out
	}
	r := Contrast(hex, bg)
	return fmt.Sprintf("%s  %.1f:1 %s", out, r, Grade(r))
}
