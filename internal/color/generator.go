package color

import (
	"fmt"
	"math"
	"math/rand"
)

// ANSI names the sixteen slots in palette order: index i is palette i for
// the normal set and palette i+8 for the bright set.
var ANSI = []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}

// Scheme is a complete sixteen-colour terminal palette plus the primary,
// cursor and selection pairs, in a form the generator and the curated
// collection can both produce.
type Scheme struct {
	Name       string
	Dark       bool
	Background string
	Foreground string
	Cursor     string
	CursorText string
	Selection  string
	SelectText string
	Normal     [8]string
	Bright     [8]string
}

// Contrast is the text/background ratio, the headline quality number.
func (s *Scheme) Contrast() float64 { return Contrast(s.Foreground, s.Background) }

// StyleName renders dark/light for display.
func (s *Scheme) StyleName() string {
	if s.Dark {
		return "dark"
	}
	return "light"
}

// Harmony is a colour-wheel relationship the generator builds around.
type Harmony int

const (
	Complementary      Harmony = iota // two opposite hues
	Analogous                         // neighbouring hues
	Triadic                           // three hues, evenly spaced
	SplitComplementary                // base plus the two beside its complement
	Tetradic                          // four hues
	Monochromatic                     // one hue, varied in saturation
)

// HarmonyNames and HarmonyDescriptions feed the creator's list.
var HarmonyNames = []string{"Complementary", "Analogous", "Triadic", "Split-complementary", "Tetradic", "Monochromatic"}

var HarmonyDescriptions = []string{
	"Two opposite hues, maximum separation",
	"Neighbouring hues, calm and cohesive",
	"Three hues evenly spaced",
	"Vibrant but less tense than complementary",
	"Four hues, the richest palette",
	"One hue, varied in light and saturation",
}

func (h Harmony) slug() string {
	return [...]string{"complement", "analogous", "triadic", "split", "tetradic", "mono"}[int(h)%6]
}

// Contrast targets used while generating.
const (
	MinContrastRatio    = 4.5 // WCAG AA: every ANSI colour must clear this
	TargetContrastRatio = 8.0 // foreground against background
)

var hueNames = []struct {
	max  float64
	name string
}{
	{15, "ember"}, {40, "amber"}, {65, "citron"}, {95, "moss"},
	{140, "fern"}, {170, "jade"}, {195, "lagoon"}, {225, "azure"},
	{255, "cobalt"}, {285, "iris"}, {315, "orchid"}, {345, "rose"}, {360, "ember"},
}

// HueName gives a generated theme a readable name instead of a timestamp.
func HueName(hue float64) string {
	hue = NormalizeHue(hue)
	for _, e := range hueNames {
		if hue < e.max {
			return e.name
		}
	}
	return "ember"
}

// A terminal palette is semantic before it is decorative: red must stay
// recognisably red, or every tool that colours its output by convention
// breaks. Each chromatic slot therefore has a canonical hue and a tolerance
// it may drift within. The bands are not equal: yellow turns to orange or
// olive within a few degrees, while blue and magenta stay recognisable much
// further out.
var semanticHues = map[string]float64{"red": 2, "yellow": 48, "green": 128, "cyan": 184, "blue": 222, "magenta": 302}

var hueTolerance = map[string]float64{"red": 20, "yellow": 15, "green": 28, "cyan": 20, "blue": 26, "magenta": 26}

// semanticRamp lists the chromatic slots in ascending hue so separation can
// work on plain unwrapped degrees.
var semanticRamp = []string{"red", "yellow", "green", "cyan", "blue", "magenta"}

var semanticOrder = map[string]int{"red": 0, "yellow": 1, "green": 2, "cyan": 3, "blue": 4, "magenta": 5}

var ansiIndex = map[string]int{"black": 0, "red": 1, "green": 2, "yellow": 3, "blue": 4, "magenta": 5, "cyan": 6, "white": 7}

// minHueSeparation keeps neighbouring slots apart, so a harmony cannot
// collapse blue and cyan onto one anchor and lose the distinction syntax
// highlighting relies on.
const minHueSeparation = 22.0

func separateHues(hues map[string]float64) {
	for pass := 0; pass < 4; pass++ {
		for i := 1; i < len(semanticRamp); i++ {
			prev, cur := semanticRamp[i-1], semanticRamp[i]
			gap := hues[cur] - hues[prev]
			if gap >= minHueSeparation {
				continue
			}
			push := (minHueSeparation - gap) / 2
			hues[cur] = clampToTolerance(cur, hues[cur]+push)
			hues[prev] = clampToTolerance(prev, hues[prev]-push)
		}
	}
}

func clampToTolerance(name string, hue float64) float64 {
	c, t := semanticHues[name], hueTolerance[name]
	return Clamp(hue, c-t, c+t)
}

func harmonyAnchors(base float64, h Harmony) []float64 {
	var offsets []float64
	switch h {
	case Complementary:
		offsets = []float64{0, 180}
	case Analogous:
		offsets = []float64{-30, 0, 30}
	case Triadic:
		offsets = []float64{0, 120, 240}
	case SplitComplementary:
		offsets = []float64{0, 150, 210}
	case Tetradic:
		offsets = []float64{0, 90, 180, 270}
	default:
		offsets = []float64{0}
	}
	out := make([]float64, len(offsets))
	for i, o := range offsets {
		out[i] = NormalizeHue(base + o)
	}
	return out
}

func nearestAnchor(anchors []float64, target float64) float64 {
	best, bestD := anchors[0], HueDistance(anchors[0], target)
	for _, a := range anchors[1:] {
		if d := HueDistance(a, target); d < bestD {
			best, bestD = a, d
		}
	}
	return best
}

// pullTowards moves from towards to by at most limit degrees, the short way,
// leaving the result unwrapped so separation can compare arithmetically.
func pullTowards(from, to, limit float64) float64 {
	delta := math.Mod(to-from+540, 360) - 180
	return from + Clamp(delta, -limit, limit)
}

// PullTowardsAccent is pullTowards with one guard: when the accent sits
// nearly opposite the canonical hue, "towards" has no meaningful direction.
// Leaving the hue alone beats sending an amber theme's blue off into violet.
func PullTowardsAccent(canonical, accent, limit float64) float64 {
	delta := math.Mod(accent-canonical+540, 360) - 180
	if math.Abs(delta) > 150 {
		return canonical
	}
	return pullTowards(canonical, accent, limit)
}

// NewScheme seeds a scheme with a background and a foreground built from a
// base hue, ready for the ANSI slots to be filled in.
func NewScheme(dark bool, baseHue float64) *Scheme {
	s := &Scheme{Dark: dark}
	if dark {
		s.Background = HSLHex(baseHue, 0.06+rand.Float64()*0.12, 0.07+rand.Float64()*0.06)
		s.Foreground = EnsureContrast(HSLHex(baseHue, 0.05+rand.Float64()*0.08, 0.82), s.Background, TargetContrastRatio)
	} else {
		s.Background = HSLHex(baseHue, 0.03+rand.Float64()*0.07, 0.93+rand.Float64()*0.05)
		s.Foreground = EnsureContrast(HSLHex(baseHue, 0.05+rand.Float64()*0.08, 0.18), s.Background, TargetContrastRatio)
	}
	return s
}

// LightnessBands returns the normal and bright lightness for the style.
func (s *Scheme) LightnessBands() (normal, bright float64) {
	if s.Dark {
		return 0.56, 0.70
	}
	return 0.42, 0.32
}

// SetANSI writes one chromatic slot's normal and bright colours from a hue.
func (s *Scheme) SetANSI(name string, hue, sat float64) {
	i := ansiIndex[name]
	normalL, brightL := s.LightnessBands()
	s.Normal[i] = EnsureContrast(HSLHex(hue, sat, normalL), s.Background, MinContrastRatio)
	s.Bright[i] = EnsureContrast(HSLHex(hue, math.Max(0, sat-0.08), brightL), s.Background, MinContrastRatio)
}

// Finish fills the neutral slots and the cursor and selection pairs.
func (s *Scheme) Finish() {
	bg, _ := ParseHex(s.Background)
	hue := bg.ToHSL().H
	// Greys carry a trace of the background hue so they never look muddy.
	grey := func(l float64) string { return HSLHex(hue, 0.05, l) }
	if s.Dark {
		s.Normal[0], s.Bright[0] = grey(0.18), grey(0.34)
		s.Normal[7], s.Bright[7] = grey(0.78), grey(0.94)
	} else {
		s.Normal[0], s.Bright[0] = grey(0.28), grey(0.14)
		s.Normal[7], s.Bright[7] = grey(0.62), grey(0.46)
	}
	s.Cursor = s.Foreground
	s.CursorText = s.Background
	s.SetSelectionFromBlue()
}

// SetSelectionFromBlue derives the selection band from the palette's blue
// and makes sure the text on it stays legible.
func (s *Scheme) SetSelectionFromBlue() {
	s.Selection = Blend(s.Background, s.Normal[4], 0.38)
	s.SelectText = s.Foreground
	if Contrast(s.SelectText, s.Selection) < ContrastLow {
		s.SelectText = EnsureContrast(s.Foreground, s.Selection, ContrastAA)
	}
}

// Random builds a loosely constrained scheme. The hue of each slot wanders
// around its canonical value; variety comes mostly from saturation and
// lightness. Random means wide, not wrong: red never comes out green.
func Random(dark bool, baseHue float64, seeded bool) *Scheme {
	if !seeded {
		baseHue = rand.Float64() * 360
	}
	s := NewScheme(dark, baseHue)
	s.Name = fmt.Sprintf("%s-wild-%s", HueName(baseHue), s.StyleName())
	hues := make(map[string]float64, len(semanticHues))
	for name, canonical := range semanticHues {
		hues[name] = canonical + (rand.Float64()*2-1)*hueTolerance[name]*1.4
	}
	separateHues(hues)
	for name, hue := range hues {
		s.SetANSI(name, hue, 0.45+rand.Float64()*0.45)
	}
	s.Finish()
	return s
}

// Harmonious builds a scheme around a harmony rule. Each ANSI hue starts at
// its canonical position and is pulled towards the nearest anchor of the
// harmony, never past its tolerance.
func Harmonious(dark bool, h Harmony, baseHue float64, seeded bool) *Scheme {
	if !seeded {
		baseHue = rand.Float64() * 360
	}
	s := NewScheme(dark, baseHue)
	s.Name = fmt.Sprintf("%s-%s-%s", HueName(baseHue), h.slug(), s.StyleName())
	anchors := harmonyAnchors(baseHue, h)
	hues := make(map[string]float64, len(semanticHues))
	for name, canonical := range semanticHues {
		hues[name] = pullTowards(canonical, nearestAnchor(anchors, canonical), hueTolerance[name])
	}
	separateHues(hues)
	for name := range semanticHues {
		hue, sat := hues[name], 0.62+rand.Float64()*0.22
		if h == Monochromatic {
			hue = baseHue
			sat = 0.25 + float64(semanticOrder[name])*0.11
		}
		s.SetANSI(name, hue, sat)
	}
	s.Finish()
	return s
}

// SemanticHue returns the canonical hue and tolerance of a chromatic slot.
func SemanticHue(name string) (hue, tolerance float64, ok bool) {
	hue, ok = semanticHues[name]
	return hue, hueTolerance[name], ok
}

// DeriveANSI fills the chromatic slots by pulling each canonical hue part of
// the way towards an accent hue, which is how the curated collection turns a
// three-colour identity into a full ramp.
func (s *Scheme) DeriveANSI(accentHue, saturation float64) {
	hues := make(map[string]float64, len(semanticHues))
	for name, canonical := range semanticHues {
		hues[name] = PullTowardsAccent(canonical, accentHue, hueTolerance[name]*0.65)
	}
	separateHues(hues)
	for name, hue := range hues {
		s.SetANSI(name, hue, saturation)
	}
}
