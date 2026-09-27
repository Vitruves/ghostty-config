// Package collection is the curated set: one hundred and fifty-two palettes
// that ship inside the binary and install into the user's themes directory.
//
// The first fifty-two are grouped by the mood their dominant hue is
// conventionally associated with, two of them homages to a console people
// already know. The hundred that follow are grouped by the named account of
// colour their structure comes from — Goethe, Itten, Hering, Kobayashi,
// Valdez and Mehrabian, Birren, scotopic vision, circadian light, Luscher,
// and the colour order systems of Munsell and Chevreul.
//
// A word on what this is and is not. Colour–mood links are mostly cultural
// convention, and the experimental literature on them is thin. Two effects
// are better supported than the rest and are the ones leaned on here:
// short-wavelength light suppresses melatonin, which is why the evening
// palettes pull blue out; and strong red raises arousal, which is why it is
// an accent and never a field. Everything else is stated design intent, not
// a claim about your brain. Naming a theory says where a palette's logic
// comes from, not that the theory is right.
//
// What is not a matter of taste: every palette is contrast-checked by the
// test suite, AAA for body text and AA for every ANSI colour.
package collection

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/vitruves/ghostty-config/internal/color"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// Curated is a designed identity — background, text, accent — from which the
// sixteen ANSI slots are derived deterministically.
type Curated struct {
	Name       string
	Family     string
	Note       string
	Dark       bool
	Background string
	Foreground string
	Accent     string
	Energy     float64 // 0 muted … 1 vivid, drives ANSI saturation
}

// Slug is the file name the theme is stored under. It matches the names the
// alacritty-colors collection uses, so the two tools agree on a theme.
func (c Curated) Slug() string { return strings.ToLower(strings.ReplaceAll(c.Name, " ", "-")) }

// Scheme expands the identity into a full palette. It is deterministic: the
// same Curated always yields the same colours.
func (c Curated) Scheme() *color.Scheme {
	s := &color.Scheme{Name: c.Slug(), Dark: c.Dark, Background: color.Normalize(c.Background, "#000000")}
	s.Foreground = color.EnsureContrast(color.Normalize(c.Foreground, "#ffffff"), s.Background, color.ContrastAAA)

	accentHue := 0.0
	if rgb, err := color.ParseHex(c.Accent); err == nil {
		accentHue = rgb.ToHSL().H
	}
	s.DeriveANSI(accentHue, 0.42+color.Clamp(c.Energy, 0, 1)*0.40)
	s.Finish()

	if ramp, ok := fixedRamps[c.Name]; ok {
		applyFixedRamp(s, ramp)
	}

	// The accent is the theme's signature, so it also drives the cursor.
	s.Cursor = color.EnsureContrast(color.Normalize(c.Accent, s.Foreground), s.Background, color.ContrastAA)
	s.CursorText = s.Background
	return s
}

// Theme renders the curated entry as a Ghostty theme.
func (c Curated) Theme() *ghostty.Theme {
	t := ghostty.FromScheme(c.Scheme(), fmt.Sprintf("%s · %s", c.Family, c.Note))
	t.Name = c.Slug()
	t.Source = ghostty.SourceCollection
	return t
}

// Text is the theme file content.
func (c Curated) Text() string {
	return c.Theme().Render(ghostty.CollectionHeader)
}

// fixedRamp is an ANSI ramp taken from an existing terminal rather than
// derived from the accent. Every slot is still lifted to AA against the
// background, so a homage stays as readable as the rest of the collection.
type fixedRamp struct{ Normal, Bright [8]string }

var fixedRamps = map[string]fixedRamp{
	// The Windows Console "Campbell" scheme, the default ramp of the Windows
	// PowerShell console since 2017. Its dark blue and magenta all but vanish
	// on the console's own blue, so those slots are lifted.
	"PowerShell": {
		Normal: [8]string{"#0c0c0c", "#c50f1f", "#13a10e", "#c19c00", "#0037da", "#881798", "#3a96dd", "#cccccc"},
		Bright: [8]string{"#767676", "#e74856", "#16c60c", "#f9f1a5", "#3b78ff", "#b4009e", "#61d6d6", "#f2f2f2"},
	},
	// The same blue field, with a ramp built around what PSReadLine actually
	// paints: commands in bright yellow, variables in bright green, strings
	// in cyan, numbers in bright white, parameters and operators in bright
	// black. Every slot clears AA with room.
	"PowerShell Vivid": {
		Normal: [8]string{"#0c0c0c", "#ff6b6b", "#5fe07a", "#ffd75f", "#6cb4ff", "#ee88ff", "#5fe0e0", "#d8dce6"},
		Bright: [8]string{"#9aa6c4", "#ff9a9a", "#8bf09c", "#fff0a0", "#9ccbff", "#f7b0ff", "#9cf0f0", "#ffffff"},
	},
}

func applyFixedRamp(s *color.Scheme, ramp fixedRamp) {
	exempt := 0 // black on a dark theme sits on the background side
	if !s.Dark {
		exempt = 7
	}
	for i := 0; i < 8; i++ {
		normal := color.Normalize(ramp.Normal[i], s.Normal[i])
		bright := color.Normalize(ramp.Bright[i], s.Bright[i])
		if i != exempt {
			normal = color.EnsureContrast(normal, s.Background, color.MinContrastRatio)
			bright = color.EnsureContrast(bright, s.Background, color.MinContrastRatio)
		}
		isNeutral := i == 0 || i == 7
		nh, bh := hslOf(normal), hslOf(bright)
		// Lifting can leave bright darker than normal, which reads backwards.
		if !isNeutral && bh.L <= nh.L {
			bh.L = math.Min(0.95, nh.L+0.10)
			bright = bh.Hex()
		}
		s.Normal[i], s.Bright[i] = normal, bright
	}
	s.SetSelectionFromBlue()
}

func hslOf(hex string) color.HSL {
	c, err := color.ParseHex(hex)
	if err != nil {
		return color.HSL{}
	}
	return c.ToHSL()
}

// Install writes every curated theme into dir, overwriting only files the
// collection itself wrote. It returns how many were written and how many
// were left alone because someone else had written them.
func Install(dir string) (written, skipped int, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, 0, err
	}
	for _, c := range Collection {
		path := filepath.Join(dir, c.Slug())
		if existing, readErr := os.ReadFile(path); readErr == nil {
			if !strings.HasPrefix(string(existing), ghostty.CollectionHeader) {
				skipped++
				continue
			}
		}
		if writeErr := os.WriteFile(path, []byte(c.Text()), 0o644); writeErr != nil {
			return written, skipped, fmt.Errorf("failed to write %s: %w", path, writeErr)
		}
		written++
	}
	return written, skipped, nil
}

// Families lists the groups in the order they appear.
func Families() []string {
	var out []string
	seen := make(map[string]bool)
	for _, c := range Collection {
		if !seen[c.Family] {
			seen[c.Family] = true
			out = append(out, c.Family)
		}
	}
	return out
}

// Collection is the curated set. Each entry names a background, a text
// colour and an accent; the ANSI ramp is derived from them.
var Collection = []Curated{
	// --- Focus: cool, low-chroma fields. Blue-leaning light is the one hue
	// with a defensible link to alertness, so it anchors the working palettes.
	{"Deep Current", "Focus", "Deep blue field for long sessions", true, "#0b1220", "#c6d4e6", "#4c8fd6", 0.50},
	{"Cold Reason", "Focus", "Slate blue, cool and unhurried", true, "#10151d", "#ccd3dd", "#6f9dc6", 0.45},
	{"Blue Hour", "Focus", "Indigo of the minutes after sunset", true, "#0d1024", "#c8cce4", "#6272d0", 0.55},
	{"Still Water", "Focus", "Teal, quiet and even", true, "#08171a", "#c2d6d6", "#3fa6a6", 0.50},
	{"Glacier Mind", "Focus", "Bright ice for daylight rooms", false, "#f2f6fb", "#1e2833", "#2f6fb0", 0.55},
	{"Signal Clarity", "Focus", "Crisp cyan, everything legible at a glance", true, "#06121a", "#cfe2ec", "#24a8d4", 0.60},
	{"North Study", "Focus", "Navy, steady and impersonal", true, "#0a1424", "#c9d6e8", "#3f79c0", 0.50},
	{"Quiet Harbor", "Focus", "Blue-grey with the volume down", true, "#121820", "#ccd4dc", "#7fa0bb", 0.40},
	{"Meridian", "Focus", "Azure at its most exact", true, "#071a24", "#c4dbe4", "#2b9ec4", 0.55},
	{"Cobalt Discipline", "Focus", "Hard cobalt, little warmth", true, "#0a0f1e", "#c8cfe2", "#3d5fd6", 0.60},
	{"PowerShell", "Focus", "The Windows PowerShell console, Campbell ramp lifted to AA", true, "#012456", "#eeedf0", "#f9f1a5", 0.70},
	{"PowerShell Vivid", "Focus", "The console's blue, with every PSReadLine token given its own readable colour", true, "#012456", "#eeedf0", "#ffd75f", 0.80},

	// --- Calm: green fields, the conventional colour of rest.
	{"Forest Rest", "Calm", "Green depth, easy on a long afternoon", true, "#0b1610", "#c9d8cc", "#46a35e", 0.48},
	{"Moss Hour", "Calm", "Olive and earth, softly lit", true, "#12150e", "#d0d4c4", "#8a9a4a", 0.45},
	{"Verdant Calm", "Calm", "Emerald, cool but alive", true, "#071612", "#c4d8d0", "#2fa77e", 0.52},
	{"Fern Light", "Calm", "Pale green daylight", false, "#f4f8f2", "#1f2a22", "#37803f", 0.50},
	{"Sage Counsel", "Calm", "Muted sage, nothing insistent", true, "#141a16", "#ccd4cc", "#86a68c", 0.35},
	{"Evergreen Watch", "Calm", "Pine at dusk", true, "#08120e", "#c2d2c6", "#2f7d55", 0.50},
	{"Meadow Morning", "Calm", "Warm green under early sun", false, "#f7faf1", "#232b1e", "#4f8a2c", 0.55},
	{"Jade Patience", "Calm", "Jade, cool and slow", true, "#06181a", "#c4d9d8", "#23a396", 0.50},
	{"Cedar Quiet", "Calm", "Green over brown, wooden and low", true, "#121611", "#ced3c6", "#6f8f4e", 0.42},
	{"Aloe", "Calm", "Mint on white, clean and cool", false, "#f1f9f4", "#1d2a24", "#2f8c68", 0.50},

	// --- Warmth: amber fields with the blue pulled down, for late hours.
	{"Amber Lamp", "Warmth", "Lamplight amber, low blue for evenings", true, "#16120b", "#e0d5c2", "#d99a3c", 0.60},
	{"Hearth", "Warmth", "Warm brown, close and settled", true, "#1a120e", "#e2d2c6", "#c07a4a", 0.55},
	{"Candle Study", "Warmth", "Candlelit, deliberately short on blue", true, "#14100a", "#e4d8c0", "#d8a44a", 0.58},
	{"Honeyed Dusk", "Warmth", "Gold going down", true, "#171208", "#e6d9bd", "#e0b040", 0.62},
	{"Terracotta Evening", "Warmth", "Fired clay, warm and matte", true, "#1a1310", "#e4d3c8", "#c8683f", 0.58},
	{"Parchment", "Warmth", "Aged paper, warm light theme", false, "#faf5e9", "#2b2419", "#a06a1e", 0.55},
	{"Linen Morning", "Warmth", "Warm neutral, kind at 8am", false, "#faf7f2", "#2a2622", "#9a6b3a", 0.48},
	{"Toasted Oat", "Warmth", "Beige with body", false, "#f7f2e6", "#2d2820", "#8c6a2a", 0.50},
	{"Copper Patina", "Warmth", "Copper against oxidised teal", true, "#0e1618", "#d2dcdb", "#c07a45", 0.55},
	{"Saffron Study", "Warmth", "Saffron accent on dark spice", true, "#15120c", "#e2d8c4", "#e0a52e", 0.62},

	// --- Energy: red as accent, never as field. Large areas of saturated red
	// are tiring; a red signal on a dark ground is not.
	{"Ember Drive", "Energy", "Red accent on near-black, for pushing through", true, "#170e0e", "#e4cfcc", "#d64c4c", 0.80},
	{"Redshift", "Energy", "Crimson, urgent and cold-edged", true, "#14090c", "#e0c8cd", "#cf3a5a", 0.82},
	{"Kiln", "Energy", "Orange-red at working heat", true, "#180f09", "#e4d0c2", "#d96a2a", 0.80},
	{"Alarum", "Energy", "Maximum signal, minimum ambiguity", true, "#100808", "#edd6d4", "#e33b3b", 0.85},
	{"Firebrand", "Energy", "Scarlet, unmistakably awake", true, "#1a0c0c", "#e6cccb", "#e0452e", 0.82},

	// --- Imagination: violet and magenta, the conventional palette of novelty.
	{"Violet Reverie", "Imagination", "Violet field for wandering work", true, "#120e1c", "#d5cee6", "#8b6cd8", 0.68},
	{"Orchid Hour", "Imagination", "Orchid, soft and strange", true, "#170f1a", "#dfcfe2", "#b064c4", 0.68},
	{"Neon Reverie", "Imagination", "Magenta turned up", true, "#0f0a16", "#ded0ea", "#c04ce0", 0.80},
	{"Mulberry Dusk", "Imagination", "Deep berry, quieter than it looks", true, "#150c14", "#ddcbd8", "#a8447e", 0.62},
	{"Iris Bloom", "Imagination", "Violet on white paper", false, "#f7f4fc", "#262233", "#6a4bb8", 0.62},

	// --- Neutral: near-zero chroma, so nothing on screen competes for notice.
	{"Graphite", "Neutral", "Pure grey, no hue to argue with", true, "#121212", "#d4d4d4", "#8a8a8a", 0.35},
	{"Slate Discipline", "Neutral", "Grey with a trace of blue", true, "#14181c", "#ccd2d8", "#7c8b99", 0.38},
	{"Paper Mind", "Neutral", "White page, black ink", false, "#fafafa", "#232323", "#555555", 0.40},
	{"Ash Study", "Neutral", "Warm ash, restrained", true, "#16181a", "#d0d4d6", "#93999e", 0.36},
	{"Newsprint", "Neutral", "Off-white, low glare", false, "#f6f6f4", "#26272a", "#4a4d52", 0.42},

	// --- Contrast & hour: the practical end of the collection.
	{"High Noon", "Contrast", "Maximum legibility in a bright room", false, "#ffffff", "#101010", "#1a56c4", 0.60},
	{"Midnight Contrast", "Contrast", "True black, maximum separation", true, "#000000", "#f2f2f2", "#4d8ef0", 0.62},
	{"Soft Focus", "Contrast", "Gentle on tired eyes, still above AA", true, "#191b1e", "#c2c6cc", "#7f96b0", 0.40},
	{"Nocturne", "Contrast", "Blue pulled out for the last hours of the day", true, "#14100e", "#ddd2c6", "#c08a4a", 0.52},
	{"Dawn Patrol", "Contrast", "Cool and bright, for starting early", false, "#f4f7fa", "#1c242c", "#2b6ea8", 0.55},

	// ---------------------------------------------------------------------
	// The theory-led families below take their structure from named accounts
	// of colour rather than from a mood word. The same caution as above
	// applies, and more sharply: these are historical systems, and citing one
	// is a statement about where a palette's logic comes from, not a claim
	// that the system is correct. Goethe was wrong about Newton. Luscher's
	// test does not measure what it claims. What each of them did leave is a
	// coherent and well-argued way of arranging colour, which is exactly what
	// a sixteen-slot terminal palette needs.

	// --- Goethe, Zur Farbenlehre (1810). His polarity of an active "plus"
	// side (yellow through red) against a passive "minus" side (blue through
	// violet), and the affective character he assigned to each hue.
	{"Plus Side", "Goethe", "The active pole, yellow through red", true, "#16130a", "#e6dcc2", "#e0b93a", 0.62},
	{"Minus Side", "Goethe", "The passive pole, blue and unhurried", true, "#0b1020", "#c6cee6", "#4a68c8", 0.52},
	{"Turbid Medium", "Goethe", "Colour as light seen through haze", true, "#14161a", "#d2d6dc", "#9aa8bc", 0.38},
	{"Vermilion Peak", "Goethe", "The hue he thought most energetic", true, "#180d0a", "#e6cec4", "#d4522a", 0.78},
	{"Yellow Serene", "Goethe", "Serene, gay, softly exciting", true, "#171408", "#e8dfc4", "#d8bc3c", 0.58},
	{"Orange Powerful", "Goethe", "Yellow-red, the energy raised a step", true, "#1a1108", "#e6d6c0", "#d8802a", 0.72},
	{"Purple Grave", "Goethe", "Gravity and dignity in one hue", true, "#150b12", "#ddcad6", "#a8407a", 0.60},
	{"Green Contentment", "Goethe", "Blue and yellow met, a real satisfaction", true, "#0c150e", "#cbd8c8", "#4c9e56", 0.50},
	{"Sulphur Anomaly", "Goethe", "Yellow tipped just past pleasantness", true, "#14160a", "#dbdfc2", "#b8bc2e", 0.65},
	{"Steel Repose", "Goethe", "The passive side at rest", true, "#101418", "#ccd2da", "#6f8ca8", 0.40},

	// --- Itten, Kunst der Farbe (1961). The seven contrasts he taught at the
	// Bauhaus, and the colour sphere that ordered them.
	{"Cold Warm Divide", "Itten", "His cold-warm contrast, held in balance", true, "#0d1418", "#d0d8dc", "#d88a44", 0.60},
	{"Light Dark Ladder", "Itten", "Contrast of value, nothing else", true, "#0a0a0c", "#e4e4e8", "#8890a0", 0.42},
	{"Simultaneous Ghost", "Itten", "The colour an eye invents beside another", true, "#14120f", "#dcd6cc", "#7ea87e", 0.45},
	{"Saturation Step", "Itten", "One hue, walked down in chroma", true, "#101216", "#ced2da", "#5a8fd0", 0.55},
	{"Extension Ratio", "Itten", "Contrast of proportion, after Goethe", true, "#12100c", "#dcd6c8", "#c8a038", 0.58},
	{"Complement Tension", "Itten", "Opposites held apart", true, "#0e1410", "#ccd6cc", "#c04a6a", 0.70},
	{"Autumn Sphere", "Itten", "The warm, muted quarter of his sphere", true, "#16110c", "#e0d2c0", "#b8763a", 0.55},
	{"Winter Sphere", "Itten", "Cool and clear, the opposite quarter", true, "#0a0e14", "#ccd6e2", "#4a9ad4", 0.58},
	{"Spring Sphere", "Itten", "Light and warm, on paper", false, "#f6f8ee", "#232a1e", "#5a9a34", 0.55},
	{"Summer Sphere", "Itten", "Light and cool, on paper", false, "#f2f6fa", "#1e2632", "#3a7ab4", 0.52},

	// --- Hering's opponent-process account (1892), the one piece of
	// nineteenth-century colour theory modern vision science kept: colour is
	// coded on red-green, blue-yellow and black-white axes.
	{"Opponent Channels", "Opponent", "The three axes, given equal weight", true, "#0c1016", "#ccd4de", "#4a8ec4", 0.52},
	{"Red Green Axis", "Opponent", "One axis pushed, the other quiet", true, "#101210", "#d4d8d2", "#46a05a", 0.62},
	{"Blue Yellow Axis", "Opponent", "The second axis takes the lead", true, "#0c0f18", "#ccd2e0", "#d0b040", 0.58},
	{"Achromatic Axis", "Opponent", "Only the black-white channel left", true, "#101010", "#dcdcdc", "#9a9a9a", 0.30},
	{"Unique Hues", "Opponent", "The four hues that look mixed with nothing", true, "#0e1014", "#d0d4dc", "#c85a4a", 0.65},
	{"Afterimage", "Opponent", "What the eye supplies when you look away", true, "#12100e", "#d8d2ca", "#56b0a0", 0.55},
	{"Chromatic Cancel", "Opponent", "Opposites cancelling towards grey", true, "#0f1214", "#ccd2d6", "#7a9ab0", 0.40},
	{"Hering Balance", "Opponent", "Neither axis allowed to win", true, "#101418", "#cfd6dc", "#5f96b8", 0.48},
	{"Opponent Night", "Opponent", "The axes at low light", true, "#080a10", "#c6cede", "#3f6fb8", 0.50},
	{"Cardinal Direction", "Opponent", "Four bearings on the colour plane", true, "#0e1216", "#ccd4dc", "#4ea0a8", 0.52},

	// --- Kobayashi's Color Image Scale (1991), which placed palettes on a
	// warm-cool and soft-hard plane and gave each region an image word. The
	// mapping is a designer's convention, not a finding.
	{"Romantic Haze", "Image Scale", "Soft and warm, the tender quarter", false, "#fbf4f7", "#2e2430", "#b0608c", 0.48},
	{"Clear Field", "Image Scale", "Cool and clean, nothing muddied", false, "#f4faff", "#1c2630", "#2e86c8", 0.55},
	{"Natural Ground", "Image Scale", "Warm and soft, undyed", false, "#f8f5ec", "#2a2820", "#7a8a3a", 0.48},
	{"Elegant Reserve", "Image Scale", "Cool, soft, and deliberately quiet", true, "#14121a", "#d6d0dc", "#9a80b8", 0.42},
	{"Chic Monochrome", "Image Scale", "Hard and cool with the hue removed", true, "#131313", "#d6d6d6", "#8e8e8e", 0.28},
	{"Dynamic Surge", "Image Scale", "Warm and hard, the loudest corner", true, "#100c0a", "#e0d4cc", "#e0642a", 0.82},
	{"Modern Edge", "Image Scale", "Cool and hard, cleanly lit", true, "#0a0c10", "#ccd2dc", "#3ea0d8", 0.66},
	{"Classic Weight", "Image Scale", "Warm, deep, unhurried", true, "#14110c", "#ded4c4", "#a07a3a", 0.45},
	{"Casual Ease", "Image Scale", "Mid-plane, nothing insisting", false, "#f7f7f2", "#2a2b26", "#5a9a6a", 0.52},
	{"Gorgeous Depth", "Image Scale", "Saturated and dark, the rich corner", true, "#120a14", "#ddccdc", "#b0409c", 0.72},

	// --- Valdez and Mehrabian (1994) measured pleasure, arousal and dominance
	// against colour and found brightness and saturation, not hue, did most of
	// the predicting. These vary those two axes and leave hue as a passenger.
	{"Pleasure Curve", "Affect", "Bright and moderately saturated", true, "#0e1218", "#d2d8e0", "#52a0d0", 0.55},
	{"Arousal Peak", "Affect", "Saturation taken as far as it reads", true, "#120a0a", "#e2ccc8", "#e04040", 0.85},
	{"Low Arousal", "Affect", "Chroma pulled down to almost nothing", true, "#12161a", "#ccd2d8", "#7d94a4", 0.30},
	{"Bright Pleasure", "Affect", "The brightest end of the scale", false, "#fafcff", "#1e2530", "#2a80c8", 0.58},
	{"Saturated Signal", "Affect", "High chroma on a dark ground", true, "#0a0e12", "#ccd6e0", "#00a8c8", 0.80},
	{"Muted Submission", "Affect", "Low brightness, low chroma", true, "#141618", "#ccced2", "#808c94", 0.26},
	{"Valence Positive", "Affect", "Bright and green-leaning", true, "#0c1410", "#ccd8ce", "#4aa868", 0.58},
	{"Affective Neutral", "Affect", "The middle of every axis", true, "#121416", "#d0d4d6", "#8c969c", 0.32},
	{"Chroma Lift", "Affect", "Saturation raised, brightness held", true, "#0e0c14", "#d4cee0", "#8a5ad8", 0.74},
	{"Luminance Lift", "Affect", "Brightness raised, saturation held", false, "#ffffff", "#1a1a1a", "#2060c0", 0.55},

	// --- Birren's functional colour (1950s onward), written for factories and
	// hospitals: colour chosen for the task rather than for the room.
	{"Functional Blue Green", "Birren", "The hue he prescribed for sustained work", true, "#08161a", "#c8dcdc", "#2e9aa0", 0.52},
	{"Safety Green", "Birren", "The signal green of his colour codes", true, "#0a140e", "#c8d8ca", "#3aa050", 0.55},
	{"Machinery Grey", "Birren", "The ground everything else sat against", true, "#141618", "#d0d4d8", "#8a949c", 0.32},
	{"Focal Point", "Birren", "One warm accent to hold the eye", true, "#0c1014", "#ccd4dc", "#d8a030", 0.60},
	{"Visual Rest", "Birren", "The colour to look at between tasks", true, "#0e1512", "#cad6d0", "#5a9c84", 0.42},
	{"Colour Conditioning", "Birren", "His term for designing a room by its use", true, "#101418", "#ccd4da", "#5f8fb4", 0.45},
	{"Tint Tone Shade", "Birren", "His triangle, walked from tint to shade", true, "#121212", "#d4d4d4", "#9a8a7a", 0.35},
	{"Institutional Calm", "Birren", "The green of a room meant to settle you", false, "#f4f7f6", "#212a28", "#34786e", 0.48},
	{"Task Light", "Birren", "Warm light over the work surface", false, "#fbfbf7", "#26261f", "#8a6a20", 0.50},
	{"Peripheral Quiet", "Birren", "Nothing at the edge of vision competing", true, "#101212", "#ced0d0", "#7c8a88", 0.30},

	// --- Scotopic vision. In dim light the rods take over, sensitivity shifts
	// towards blue (the Purkinje shift), and red all but disappears — which is
	// why red light preserves dark adaptation.
	{"Purkinje Shift", "Scotopic", "The blue-ward shift as light drops", true, "#06080e", "#c2cade", "#4a72c0", 0.55},
	{"Rod Vision", "Scotopic", "What is left when the cones give up", true, "#05070c", "#bec6d8", "#3f6ab0", 0.50},
	{"Scotopic Blue", "Scotopic", "Peak rod sensitivity, near 500 nanometres", true, "#04060e", "#bcc6dc", "#3868c4", 0.58},
	{"Mesopic Hour", "Scotopic", "Rods and cones both half working", true, "#0a0c12", "#c6cdda", "#6a80b0", 0.44},
	{"Dark Adaptation", "Scotopic", "Twenty minutes to full sensitivity", true, "#060606", "#c8c8c8", "#7a7a7a", 0.28},
	{"Night Watch", "Scotopic", "Dim, cool, and steady for hours", true, "#070a0c", "#c2cccf", "#4f8a92", 0.46},
	{"Red Preserve", "Scotopic", "Red light, because rods barely see it", true, "#0c0505", "#dcc2c0", "#c03030", 0.70},
	{"Astronomer Red", "Scotopic", "The observatory convention, kept dark", true, "#0a0404", "#d8bcba", "#b82828", 0.72},
	{"Twilight Threshold", "Scotopic", "The minutes the shift happens in", true, "#090b14", "#c4cadc", "#5a6ec0", 0.52},
	{"Cone Silence", "Scotopic", "Colour vision almost switched off", true, "#05070a", "#bec6d0", "#46707e", 0.42},

	// --- Circadian light. Melanopsin in the retina responds most to short
	// wavelengths, which is the best-supported effect in this whole file:
	// evening blue light delays melatonin. The evening palettes take it out.
	{"Melanopic Low", "Circadian", "Short wavelengths pulled right out", true, "#150f08", "#e0d2bc", "#d09a38", 0.56},
	{"Melatonin Guard", "Circadian", "Warm enough to leave the night alone", true, "#140e08", "#ded0ba", "#c88a30", 0.58},
	{"Evening Filter", "Circadian", "The last working hours, filtered", true, "#16110a", "#e2d4bc", "#d8a344", 0.60},
	{"Blue Suppressed", "Circadian", "Amber throughout, by design", true, "#171208", "#e4d6b8", "#dcae3a", 0.62},
	{"Circadian Dusk", "Circadian", "Warm and dimming, like the hour", true, "#130f0c", "#ded2c6", "#b8804a", 0.52},
	{"Late Shift", "Circadian", "For the hours the body did not ask for", true, "#120e0a", "#ded0c0", "#c08a48", 0.50},
	{"Sunrise Signal", "Circadian", "Warm light on a bright ground", false, "#fff8ee", "#2c2419", "#b06a1a", 0.58},
	{"Daylight Alert", "Circadian", "Short-wavelength rich, for the morning", false, "#f2f8ff", "#1a2430", "#1a6ec8", 0.62},
	{"Zeitgeber", "Circadian", "The cue that sets the clock", true, "#0a1018", "#c8d2e0", "#3a86cc", 0.60},
	{"Chronotype Owl", "Circadian", "Built for whoever is up at two", true, "#0d0b12", "#d2cbdc", "#7a5cc0", 0.56},

	// --- Luscher's eight test colours (1947). The test itself does not
	// measure personality and has not survived scrutiny, but the eight colours
	// were chosen with care and still make a coherent set. That is the whole
	// of the claim being made here.
	{"Deep Contentment", "Luscher", "His dark blue, first of the eight", true, "#08101c", "#c6d0e2", "#3a6ab8", 0.52},
	{"Persistent Teal", "Luscher", "Blue-green, the second", true, "#061618", "#c2d6d8", "#26928e", 0.54},
	{"Orange Desire", "Luscher", "Orange-red, the third", true, "#180f08", "#e2d0bc", "#d87a24", 0.74},
	{"Yellow Aspiration", "Luscher", "Bright yellow, the fourth", true, "#16150a", "#e2dfc0", "#d4c22e", 0.66},
	{"Violet Identity", "Luscher", "Violet, between red and blue", true, "#100c18", "#d4cee0", "#8456c8", 0.66},
	{"Brown Comfort", "Luscher", "Brown, the bodily one", true, "#16110d", "#ded0c2", "#a06a42", 0.46},
	{"Black Renunciation", "Luscher", "Black, the refusal", true, "#050505", "#d0d0d0", "#808080", 0.26},
	{"Grey Detachment", "Luscher", "Grey, standing outside the set", true, "#131315", "#d2d2d4", "#8c8c90", 0.28},
	{"Preference Order", "Luscher", "The eight, ranked and read", true, "#0e1014", "#ced2da", "#5a80a8", 0.44},
	{"Eight Colour Test", "Luscher", "The full set, taken as a palette", true, "#0c0e12", "#ccd0da", "#4a8ab0", 0.50},

	// --- Colour order systems. Munsell arranged colour by hue, value and
	// chroma in perceptually even steps; Chevreul, at the Gobelins dye works,
	// worked out that a colour is changed by whatever it sits beside.
	{"Munsell Value Five", "Colour Order", "The middle rung of his value scale", true, "#131313", "#d6d6d6", "#8f8f8f", 0.34},
	{"Chroma Neutral", "Colour Order", "Value and hue held, chroma near zero", true, "#121314", "#d2d4d6", "#7f888e", 0.32},
	{"Hue Circle Ten", "Colour Order", "His ten principal hues, evenly spaced", true, "#0e1014", "#ced2d8", "#5a8cc0", 0.56},
	{"Chevreul Law", "Colour Order", "Simultaneous contrast, stated in 1839", true, "#101210", "#d0d4ce", "#5fa070", 0.52},
	{"Gobelins Grey", "Colour Order", "Where he found the effect, in the dye works", true, "#141414", "#d4d4d4", "#8a8a92", 0.30},
	{"Value Scale", "Colour Order", "Nine steps from black to white", false, "#f7f7f7", "#242424", "#4a4a4a", 0.36},
	{"Chroma Step", "Colour Order", "One hue, walked out from the axis", true, "#0f1116", "#ced2da", "#6a80b8", 0.48},
	{"Balanced Neutral", "Colour Order", "Equidistant from every hue", true, "#121416", "#d0d4d6", "#869098", 0.34},
	{"Ordered System", "Colour Order", "Colour given three coordinates", true, "#0e1012", "#ced2d4", "#6e8894", 0.40},
	{"Colour Solid", "Colour Order", "The whole arrangement, in three dimensions", true, "#0c0e12", "#ccd0d8", "#5c7ea8", 0.46},
}
