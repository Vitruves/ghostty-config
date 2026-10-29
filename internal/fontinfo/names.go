package fontinfo

import (
	"strconv"
	"strings"
)

// A font lists many features, most of them machinery the shaper applies on
// its own: kerning, marks, localised forms, the contextual alternates of a
// script. The ones worth a switch are the optional ones, which a designer
// leaves off and a reader turns on; they are named and explained here.

// optional names the optional features, with what each does.
var optional = map[string][2]string{
	"zero": {"Slashed zero", "the zero gets a slash or a dot, so it never reads as the letter O"},
	"tnum": {"Tabular figures", "every figure takes the same width, so numbers line up in columns"},
	"pnum": {"Proportional figures", "each figure takes its own width, the way running text sets them"},
	"onum": {"Old-style figures", "figures with ascenders and descenders, that sit in lowercase text like letters"},
	"lnum": {"Lining figures", "figures all as tall as capitals"},
	"case": {"Case-sensitive forms", "brackets, dashes and other punctuation raised to sit with capitals"},
	"frac": {"Fractions", "figures either side of a slash become a fraction, 1/2 drawn as one"},
	"sups": {"Superscript", "figures and letters drawn as superscripts"},
	"subs": {"Subscript", "figures and letters drawn as subscripts"},
	"sinf": {"Scientific inferiors", "subscript figures set low, for chemical formulas"},
	"ordn": {"Ordinals", "the a and o after a figure raised, as in 1a and 2o"},
	"salt": {"Stylistic alternates", "the designer's alternate forms of some letters"},
	"smcp": {"Small capitals", "lowercase letters drawn as small capitals"},
	"c2sc": {"Capitals to small capitals", "capitals drawn as small capitals too"},
	"pcap": {"Petite capitals", "lowercase letters drawn as capitals smaller still than small capitals"},
	"unic": {"Unicase", "capitals and lowercase drawn at one height"},
	"swsh": {"Swash", "letters with flourishes"},
	"titl": {"Titling", "capitals drawn for large sizes"},
	"hist": {"Historical forms", "letter forms from older typography, such as the long s"},
	"hlig": {"Historical ligatures", "ligatures of older typography"},
	"ornm": {"Ornaments", "decorative glyphs in place of some characters"},
	"nalt": {"Alternate annotation forms", "figures and letters in circles or boxes"},
	"mgrk": {"Mathematical Greek", "Greek letters in their mathematical forms"},
	"cpsp": {"Capital spacing", "a little more room between capitals"},
}

// Ligatures are the one family of features the tool already switches as a
// whole, through the ligatures setting; they are never offered one by one.
var ligatures = map[string]bool{"calt": true, "liga": true, "dlig": true}

// IsLigature is whether a tag is one of the features the ligatures setting
// switches.
func IsLigature(tag string) bool { return ligatures[tag] }

// Optional is whether a feature is one a reader turns on or off, as opposed
// to machinery the shaper runs by itself. The ligatures are left out: the
// ligatures setting owns them.
func Optional(tag string) bool {
	if ligatures[tag] {
		return false
	}
	if _, ok := optional[tag]; ok {
		return true
	}
	return numbered(tag, "ss", 1, 20) > 0 || numbered(tag, "cv", 1, 99) > 0
}

// numbered reads the number of a tag like ss03 or cv12, or 0.
func numbered(tag, prefix string, lo, hi int) int {
	rest, ok := strings.CutPrefix(tag, prefix)
	if !ok || len(rest) != 2 {
		return 0
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < lo || n > hi {
		return 0
	}
	return n
}

// Label is what a feature is called in a list: the font's own name for a
// stylistic set or a character variant when it gives one, otherwise the
// standard name of the tag.
func (f Feature) Label() string {
	if f.Name != "" {
		return f.Name
	}
	if n := numbered(f.Tag, "ss", 1, 20); n > 0 {
		return "Stylistic set " + strconv.Itoa(n)
	}
	if n := numbered(f.Tag, "cv", 1, 99); n > 0 {
		return "Character variant " + strconv.Itoa(n)
	}
	if o, ok := optional[f.Tag]; ok {
		return o[0]
	}
	return f.Tag
}

// Note says what turning a feature on does.
func (f Feature) Note() string {
	if o, ok := optional[f.Tag]; ok {
		return o[1]
	}
	switch {
	case numbered(f.Tag, "ss", 1, 20) > 0:
		if f.Name != "" {
			return "a set of alternate glyphs the designer grouped and named " + quote(f.Name)
		}
		return "a set of alternate glyphs the designer grouped; the font gives it no name, its documentation says what it changes"
	case numbered(f.Tag, "cv", 1, 99) > 0:
		if f.Name != "" {
			return "an alternate form of one character: " + quote(f.Name)
		}
		return "an alternate form of one character; the font gives it no name, its documentation says which"
	}
	return "an OpenType feature of the font"
}

func quote(s string) string { return "“" + s + "”" }

// axisNames are the registered axes, by tag.
var axisNames = map[string]string{
	"wght": "Weight", "wdth": "Width", "slnt": "Slant", "ital": "Italic", "opsz": "Optical size",
	"GRAD": "Grade", "XHGT": "x-height", "CASL": "Casual", "MONO": "Monospace", "CRSV": "Cursive",
}

// Label is what an axis is called: the font's own name, or the registered
// one, or its tag.
func (a Axis) Label() string {
	if a.Name != "" {
		return a.Name
	}
	if n, ok := axisNames[a.Tag]; ok {
		return n
	}
	return a.Tag
}
