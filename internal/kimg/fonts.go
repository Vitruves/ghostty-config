// Package kimg draws the pictures the full-screen interface shows through the
// Kitty graphics protocol: cards with rounded corners and soft shadows, a
// window with a real blur. Everything is drawn here in plain Go, text
// included, in Google Sans Code and Google Sans Flex (both SIL OFL, kept in
// fonts/), so the pictures look the same whatever font the terminal uses.
package kimg

import (
	_ "embed"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

//go:embed fonts/code-regular.ttf
var codeRegular []byte

//go:embed fonts/code-bold.ttf
var codeBold []byte

//go:embed fonts/sans-regular.ttf
var sansRegular []byte

//go:embed fonts/sans-bold.ttf
var sansBold []byte

type faceKey struct {
	mono, bold bool
	px         int
}

var (
	fontOnce sync.Once
	fonts    [4]*opentype.Font
	faceMu   sync.Mutex
	faces    = map[faceKey]font.Face{}
)

func loadFonts() {
	fontOnce.Do(func() {
		for i, data := range [][]byte{codeRegular, codeBold, sansRegular, sansBold} {
			f, err := opentype.Parse(data)
			if err != nil {
				panic("kimg: bad embedded font: " + err.Error())
			}
			fonts[i] = f
		}
	})
}

// face returns a font face of the given pixel size, cached. The sizes are
// rounded to whole tenths of a pixel so that a few distinct faces cover every
// text the cards draw.
func face(mono, bold bool, px float64) font.Face {
	loadFonts()
	key := faceKey{mono, bold, int(px*10 + 0.5)}
	faceMu.Lock()
	defer faceMu.Unlock()
	if f, ok := faces[key]; ok {
		return f
	}
	idx := 2
	if mono {
		idx = 0
	}
	if bold {
		idx++
	}
	of, err := opentype.NewFace(fonts[idx], &opentype.FaceOptions{Size: float64(key.px) / 10, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		panic("kimg: face: " + err.Error())
	}
	f := CacheGlyphs(of)
	if len(faces) >= maxFaces {
		// The window's preview sizes its text by the panel, so every width
		// the terminal passes through while it is resized makes faces of
		// its own; past a bound the cache starts over. What the faces
		// handed out last are is kept one round longer, for the frame
		// being drawn may still use them.
		faces = map[faceKey]font.Face{}
		lastInfo, faceInfo = faceInfo, map[font.Face]faceKey{}
	}
	faces[key] = f
	faceInfo[f] = key
	return f
}

// UncachedFace is a face like Face that keeps none of its glyphs, for
// tests to compare with.
func UncachedFace(mono, bold bool, px float64) font.Face {
	loadFonts()
	idx := 2
	if mono {
		idx = 0
	}
	if bold {
		idx++
	}
	f, err := opentype.NewFace(fonts[idx], &opentype.FaceOptions{Size: float64(int(px*10+0.5)) / 10, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		panic("kimg: face: " + err.Error())
	}
	return f
}

// maxFaces bounds the faces kept: a frame draws in a dozen or so.
const maxFaces = 64

// Face is a face of the embedded fonts at a size in device pixels, for
// drawing code outside this package.
func Face(mono, bold bool, px float64) font.Face { return face(mono, bold, px) }

// faceInfo remembers what each face handed out is, so a drawer can tell
// whether it has a glyph and which face to fall back on.
var faceInfo = map[font.Face]faceKey{}

// lastInfo is faceInfo before the cache last started over.
var lastInfo = map[font.Face]faceKey{}

// infoOf is what a face handed out is, if it was handed out lately.
func infoOf(f font.Face) (faceKey, bool) {
	faceMu.Lock()
	defer faceMu.Unlock()
	if k, ok := faceInfo[f]; ok {
		return k, true
	}
	k, ok := lastInfo[f]
	return k, ok
}

// Covers reports whether a face made by Face has a glyph for r.
func Covers(f font.Face, r rune) bool {
	k, ok := infoOf(f)
	if !ok {
		return true
	}
	idx := 2
	if k.mono {
		idx = 0
	}
	if k.bold {
		idx++
	}
	var b sfnt.Buffer
	i, err := fonts[idx].GlyphIndex(&b, r)
	return err == nil && i != 0
}

// MonoOf is the monospace face of the same size and weight, which has the
// arrows the sans faces lack.
func MonoOf(f font.Face) font.Face {
	k, ok := infoOf(f)
	if !ok {
		return f
	}
	return face(true, k.bold, float64(k.px)/10)
}
