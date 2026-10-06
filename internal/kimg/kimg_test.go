package kimg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
