// Package fontinfo reads what a font file offers beyond its glyphs: the
// OpenType features it can switch on, the axes of a variable font, and
// whether it carries the Nerd Font icons. Ghostty passes features and axis
// values through to the shaper by tag, so the only way to offer the ones a
// font really has is to read them from the file: the GSUB and GPOS feature
// lists, the fvar axes, and the names both point into.
//
// The sfnt table directory is read here rather than through x/image/font,
// which keeps the tables it does not need to itself.
package fontinfo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"unicode/utf16"

	"golang.org/x/image/font/sfnt"
)

// Feature is one OpenType feature of a font. Name is what the font itself
// calls it, for the stylistic sets and character variants that carry a
// name; empty otherwise.
type Feature struct {
	Tag  string
	Name string
}

// Axis is one axis of a variable font, in the units the font uses.
type Axis struct {
	Tag               string
	Name              string
	Min, Default, Max float64
}

// Info is what one face of a font file offers.
type Info struct {
	PostScript string
	// Features are every feature tag of GSUB and GPOS, once each, sorted.
	Features []Feature
	// Axes are the axes of a variable font, in the font's order, hidden
	// ones left out; none for a static font.
	Axes []Axis
	// NerdIcons is whether the face draws the Nerd Font icons itself.
	NerdIcons bool
}

// Read reads one face of a font file. In a collection the face named
// postScript is the one taken, or else the one at index, as pix.OpenFont
// picks it.
func Read(path, postScript string, index int) (*Info, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data, postScript, index)
}

// errShort is what a table that ends early reads as.
var errShort = errors.New("fontinfo: truncated table")

// Parse reads one face of a font file held in memory.
func Parse(data []byte, postScript string, index int) (*Info, error) {
	offsets, err := faceOffsets(data)
	if err != nil {
		return nil, err
	}
	pick := -1
	if postScript != "" && len(offsets) > 1 {
		for i, off := range offsets {
			tables, err := tableDirectory(data, off)
			if err != nil {
				continue
			}
			if postScriptName(tables) == postScript {
				pick = i
				break
			}
		}
	}
	if pick < 0 {
		pick = min(max(index, 0), len(offsets)-1)
	}
	tables, err := tableDirectory(data, offsets[pick])
	if err != nil {
		return nil, err
	}
	info := &Info{PostScript: postScriptName(tables)}
	names := func(id uint16) string { return nameOf(tables, id) }

	seen := map[string]int{}
	for _, tag := range []string{"GSUB", "GPOS"} {
		t, ok := tables[tag]
		if !ok {
			continue
		}
		feats, err := featureList(t)
		if err != nil {
			continue
		}
		for _, f := range feats {
			name := ""
			if f.nameID != 0 {
				name = names(f.nameID)
			}
			if i, ok := seen[f.tag]; ok {
				if info.Features[i].Name == "" {
					info.Features[i].Name = name
				}
				continue
			}
			seen[f.tag] = len(info.Features)
			info.Features = append(info.Features, Feature{Tag: f.tag, Name: name})
		}
	}
	sort.Slice(info.Features, func(a, b int) bool { return info.Features[a].Tag < info.Features[b].Tag })

	if t, ok := tables["fvar"]; ok {
		if axes, err := fvarAxes(t, names); err == nil {
			info.Axes = axes
		}
	}
	info.NerdIcons = coversNerdIcons(data, pick)
	return info, nil
}

// faceOffsets is where each face's table directory starts: one for a plain
// font file, one per face for a collection.
func faceOffsets(data []byte) ([]uint32, error) {
	if len(data) < 12 {
		return nil, errShort
	}
	if string(data[:4]) != "ttcf" {
		return []uint32{0}, nil
	}
	n := int(binary.BigEndian.Uint32(data[8:]))
	if n == 0 || 12+4*n > len(data) {
		return nil, fmt.Errorf("fontinfo: a collection of %d faces does not fit its file", n)
	}
	out := make([]uint32, n)
	for i := range out {
		out[i] = binary.BigEndian.Uint32(data[12+4*i:])
	}
	return out, nil
}

// tableDirectory maps each table tag of a face to its bytes.
func tableDirectory(data []byte, off uint32) (map[string][]byte, error) {
	o := int(off)
	if o < 0 || o+12 > len(data) {
		return nil, errShort
	}
	n := int(binary.BigEndian.Uint16(data[o+4:]))
	if o+12+16*n > len(data) {
		return nil, errShort
	}
	tables := make(map[string][]byte, n)
	for i := 0; i < n; i++ {
		rec := data[o+12+16*i:]
		start, length := binary.BigEndian.Uint32(rec[8:]), binary.BigEndian.Uint32(rec[12:])
		if uint64(start)+uint64(length) > uint64(len(data)) {
			continue
		}
		tables[string(rec[:4])] = data[start : start+length]
	}
	return tables, nil
}

func postScriptName(tables map[string][]byte) string {
	return nameOf(tables, 6)
}

// nameOf reads a string of the name table, preferring the English one of
// Windows, then any Unicode one, then the Macintosh Roman one.
func nameOf(tables map[string][]byte, id uint16) string {
	t := tables["name"]
	if len(t) < 6 {
		return ""
	}
	count := int(binary.BigEndian.Uint16(t[2:]))
	storage := int(binary.BigEndian.Uint16(t[4:]))
	best, bestRank := "", 99
	for i := 0; i < count; i++ {
		r := 6 + 12*i
		if r+12 > len(t) {
			break
		}
		platform := binary.BigEndian.Uint16(t[r:])
		encoding := binary.BigEndian.Uint16(t[r+2:])
		lang := binary.BigEndian.Uint16(t[r+4:])
		if binary.BigEndian.Uint16(t[r+6:]) != id {
			continue
		}
		length := int(binary.BigEndian.Uint16(t[r+8:]))
		start := storage + int(binary.BigEndian.Uint16(t[r+10:]))
		if start+length > len(t) {
			continue
		}
		raw := t[start : start+length]
		rank, text := 99, ""
		switch {
		case platform == 3 && (encoding == 1 || encoding == 10):
			rank, text = 1, utf16BE(raw)
			if lang != 0x409 {
				rank = 2
			}
		case platform == 0:
			rank, text = 3, utf16BE(raw)
		case platform == 1 && encoding == 0:
			rank, text = 4, latin(raw)
		}
		if rank < bestRank && text != "" {
			best, bestRank = text, rank
		}
	}
	return strings.TrimSpace(best)
}

func utf16BE(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.BigEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}

// latin reads Mac Roman text as far as it is ASCII, which is what names are
// in practice.
func latin(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

type rawFeature struct {
	tag    string
	nameID uint16
}

// featureList reads the FeatureList of a GSUB or GPOS table, with the name
// a stylistic set or a character variant gives itself in its parameters.
func featureList(t []byte) ([]rawFeature, error) {
	if len(t) < 10 {
		return nil, errShort
	}
	list := int(binary.BigEndian.Uint16(t[6:]))
	if list == 0 || list+2 > len(t) {
		return nil, errShort
	}
	n := int(binary.BigEndian.Uint16(t[list:]))
	var out []rawFeature
	for i := 0; i < n; i++ {
		r := list + 2 + 6*i
		if r+6 > len(t) {
			return out, errShort
		}
		f := rawFeature{tag: string(t[r : r+4])}
		feature := list + int(binary.BigEndian.Uint16(t[r+4:]))
		if feature+2 <= len(t) && (strings.HasPrefix(f.tag, "ss") || strings.HasPrefix(f.tag, "cv")) {
			if params := int(binary.BigEndian.Uint16(t[feature:])); params != 0 && feature+params+4 <= len(t) {
				// Both kinds keep their name ID in the second field: after
				// a version for a stylistic set, after a format for a
				// character variant.
				f.nameID = binary.BigEndian.Uint16(t[feature+params+2:])
			}
		}
		out = append(out, f)
	}
	return out, nil
}

// fvarAxes reads the axes of a variable font, leaving out the ones the font
// marks hidden: those are for the font's own machinery, not for people.
func fvarAxes(t []byte, names func(uint16) string) ([]Axis, error) {
	if len(t) < 16 {
		return nil, errShort
	}
	start := int(binary.BigEndian.Uint16(t[4:]))
	count := int(binary.BigEndian.Uint16(t[8:]))
	size := int(binary.BigEndian.Uint16(t[10:]))
	if size < 20 {
		return nil, fmt.Errorf("fontinfo: an fvar axis of %d bytes", size)
	}
	var out []Axis
	for i := 0; i < count; i++ {
		r := start + size*i
		if r+20 > len(t) {
			return out, errShort
		}
		if binary.BigEndian.Uint16(t[r+16:])&1 != 0 {
			continue
		}
		a := Axis{
			Tag:     string(t[r : r+4]),
			Min:     fixed(t[r+4:]),
			Default: fixed(t[r+8:]),
			Max:     fixed(t[r+12:]),
		}
		a.Name = names(binary.BigEndian.Uint16(t[r+18:]))
		out = append(out, a)
	}
	return out, nil
}

// fixed reads a 16.16 number, rounded to what a person would type.
func fixed(b []byte) float64 {
	v := float64(int32(binary.BigEndian.Uint32(b))) / 65536
	return math.Round(v*100) / 100
}

// nerdProbe is a code point only fonts patched with the Nerd Font icons
// draw: the folder of Font Awesome, in the private use area.
const nerdProbe = 0xF07B

func coversNerdIcons(data []byte, face int) bool {
	coll, err := sfnt.ParseCollection(data)
	if err != nil || face >= coll.NumFonts() {
		return false
	}
	f, err := coll.Font(face)
	if err != nil {
		return false
	}
	var b sfnt.Buffer
	g, err := f.GlyphIndex(&b, nerdProbe)
	return err == nil && g != 0
}
