package kimg

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi/kitty"
)

func testPal() [16]string {
	return [16]string{"#2c2e30", "#d46c49", "#61d449", "#d4a749", "#6286da", "#d651ad", "#49d4bf", "#c4c7ca", "#52565b", "#dc9e89", "#97dc89", "#dcc189", "#89a2dc", "#dc89c2", "#89dccf", "#eff0f0"}
}

func TestPicturesHaveTheSizeAskedFor(t *testing.T) {
	tile := Tile(18, 6, 20, 42, CardSpec{BG: "#0c1c31", FG: "#d6dae0", Accent: "#e2cb9c", Pal: testPal(), Screen: "#101014"})
	if b := tile.Img.Bounds(); b.Dx() != 360 || b.Dy() != 252 {
		t.Fatalf("tile is %dx%d, want 360x252", b.Dx(), b.Dy())
	}
	if len(tile.PNG()) < 1000 {
		t.Fatal("the PNG should hold a picture")
	}
	// The ring round a selected tile changes the picture; the screen it sits
	// on changes its shadow.
	selected := Tile(18, 6, 20, 42, CardSpec{BG: "#0c1c31", FG: "#d6dae0", Accent: "#e2cb9c", Pal: testPal(), Screen: "#101014", Selected: true, Ring: "#7aa2f7"})
	onLight := Tile(18, 6, 20, 42, CardSpec{BG: "#0c1c31", FG: "#d6dae0", Accent: "#e2cb9c", Pal: testPal(), Screen: "#f4f5f7"})
	if string(selected.PNG()) == string(tile.PNG()) || string(onLight.PNG()) == string(tile.PNG()) {
		t.Fatal("selection and the screen's lightness should both show in the picture")
	}
	win := Window(48, 9, 20, 42, WindowSpec{BG: "#0c1c31", FG: "#d6dae0", Cursor: "#e2cb9c", Pal: testPal(), Screen: "#101014", Behind: [2]string{"#7aa2f7", "#e0af68"}, Titlebar: "tabs", Shadow: true, Opacity: 0.85, Blur: true, PadX: 14, PadY: 12})
	if b := win.Img.Bounds(); b.Dx() != 960 || b.Dy() != 378 {
		t.Fatalf("window is %dx%d, want 960x378", b.Dx(), b.Dy())
	}
}

func TestPlaceholderRowsAreOneCellPerColumn(t *testing.T) {
	row := PlaceholderRow(2, 5)
	if n := strings.Count(row, "\U0010EEEE"); n != 5 {
		t.Fatalf("5 placeholder cells wanted, got %d", n)
	}
}

// The marks name 297 columns; a wider row names the rest by leaving the
// column out, for the terminal to count on from the cell to the left.
func TestPlaceholderRowsWiderThanTheMarks(t *testing.T) {
	if kitty.Diacritic(diacritics-1) == kitty.Diacritic(0) || kitty.Diacritic(diacritics) != kitty.Diacritic(0) {
		t.Fatalf("the library no longer has %d marks", diacritics)
	}
	cells := strings.Split(PlaceholderRow(3, 400), string(kitty.Placeholder))[1:]
	if len(cells) != 400 {
		t.Fatalf("%d cells, want 400", len(cells))
	}
	for c, cell := range cells {
		marks := []rune(cell)
		if len(marks) == 0 || marks[0] != kitty.Diacritic(3) {
			t.Fatalf("cell %d does not name row 3: %q", c, cell)
		}
		switch {
		case c < diacritics && (len(marks) != 2 || marks[1] != kitty.Diacritic(c)):
			t.Fatalf("cell %d does not name its column: %q", c, cell)
		case c >= diacritics && len(marks) != 1:
			t.Fatalf("cell %d names a column the marks cannot: %q", c, cell)
		}
	}
}

// Every size asked for makes a face, and a resize asks for many: the cache
// stays bounded, and a face handed out just before it starts over still
// knows what it is.
func TestFaceCacheIsBounded(t *testing.T) {
	last := Face(false, false, 10)
	for i := 0; i < 4*maxFaces; i++ {
		f := Face(false, false, 10+float64(i)/10)
		if MonoOf(last) == last || Covers(last, '→') {
			t.Fatalf("after %d sizes a face handed out a moment ago forgot what it is", i)
		}
		last = f
	}
	faceMu.Lock()
	n := len(faces)
	faceMu.Unlock()
	if n > maxFaces {
		t.Fatalf("%d faces cached, want at most %d", n, maxFaces)
	}
}

func TestTransmitChunksAndEnds(t *testing.T) {
	seq := Transmit(7, make([]byte, 20000), 10, 4)
	if !strings.HasPrefix(seq, "\x1b_Ga=T,f=100,U=1,i=7,c=10,r=4,q=2,m=1;") {
		t.Fatalf("bad start: %q", seq[:60])
	}
	if !strings.Contains(seq, "\x1b_Gm=0;") {
		t.Fatal("the last chunk must say m=0")
	}
}

// KIMG_OUT=dir writes sample pictures for a look.
func TestWritePreviews(t *testing.T) {
	dir := os.Getenv("KIMG_OUT")
	if dir == "" {
		t.Skip("set KIMG_OUT to write previews")
	}
	os.MkdirAll(dir, 0o755)
	p := testPal()
	for name, spec := range map[string]CardSpec{
		"tile":          {BG: "#0c1c31", FG: "#d6dae0", Accent: "#e2cb9c", Pal: p, Screen: "#101014"},
		"tile-selected": {BG: "#170a29", FG: "#dad6e0", Accent: "#f04cac", Pal: p, Screen: "#101014", Selected: true, Ring: "#7aa2f7"},
		"tile-on-light": {BG: "#fafafa", FG: "#212121", Accent: "#d41125", Pal: p, Screen: "#f4f5f7"},
	} {
		os.WriteFile(filepath.Join(dir, name+".png"), Tile(18, 6, 20, 42, spec).PNG(), 0o644)
	}
	for _, bar := range []string{"native", "transparent", "tabs", "hidden", "none"} {
		win := Window(48, 9, 20, 42, WindowSpec{BG: "#0c1c31", FG: "#d6dae0", Cursor: "#e2cb9c", Pal: p, Screen: "#101014", Behind: [2]string{"#7aa2f7", "#e0af68"}, Titlebar: bar, Shadow: true, Opacity: 0.85, Blur: true, PadX: 14, PadY: 12})
		os.WriteFile(filepath.Join(dir, "window-"+bar+".png"), win.PNG(), 0o644)
	}
}

// TestTransmitEncodedChunks wants the escapes the protocol has always been
// sent in: the control data with the first chunk, 4096 bytes a chunk, m=1
// on every chunk but the last.
func TestTransmitEncodedChunks(t *testing.T) {
	want := func(id uint32, enc string, w, h, cols, rows int) string {
		ctrl := fmt.Sprintf("a=T,f=32,s=%d,v=%d,U=1,i=%d,c=%d,r=%d,q=2", w, h, id, cols, rows)
		var b strings.Builder
		for first := true; len(enc) > 0; first = false {
			n := min(chunk, len(enc))
			part := enc[:n]
			enc = enc[n:]
			more := 0
			if len(enc) > 0 {
				more = 1
			}
			if first {
				fmt.Fprintf(&b, "\x1b_G%s,o=z,m=%d;%s\x1b\\", ctrl, more, part)
			} else {
				fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, part)
			}
		}
		return b.String()
	}
	for _, n := range []int{0, 1, chunk - 1, chunk, chunk + 1, 3*chunk + 17} {
		enc := strings.Repeat("A", n)
		if got, w := TransmitEncoded(77, enc, 2040, 38, 120, 1, ""), want(77, enc, 2040, 38, 120, 1); got != w {
			t.Fatalf("%d bytes: got %q..., want %q...", n, got[:min(len(got), 80)], w[:min(len(w), 80)])
		}
	}
	file := TransmitEncoded(5, "", 4, 2, 1, 1, "/tmp/x")
	if file != "\x1b_Ga=T,f=32,s=4,v=2,U=1,i=5,c=1,r=1,q=2,t=t;"+base64.StdEncoding.EncodeToString([]byte("/tmp/x"))+"\x1b\\" {
		t.Fatalf("by file: %q", file)
	}
}
