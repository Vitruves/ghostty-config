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
	f, err := opentype.NewFace(fonts[idx], &opentype.FaceOptions{Size: float64(key.px) / 10, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		panic("kimg: face: " + err.Error())
	}
	faces[key] = f
	return f
}
