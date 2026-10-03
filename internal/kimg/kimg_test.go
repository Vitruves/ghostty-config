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

func TestCardHasTheSizeAskedFor(t *testing.T) {
	c := Card(34, 9, 20, 42, CardSpec{Name: "ukiyo-wave", Family: "Movements", BG: "#0c1c31", FG: "#d6dae0", Accent: "#e2cb9c", Pal: testPal(), Ratio: 12.5, Grade: "AAA", Dark: true})
	if b := c.Img.Bounds(); b.Dx() != 680 || b.Dy() != 378 {
		t.Fatalf("card is %dx%d, want 680x378", b.Dx(), b.Dy())
	}
	if len(c.PNG()) < 1000 {
		t.Fatal("the PNG should hold a picture")
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
		"card":          {Name: "ukiyo-wave", Family: "Movements", BG: "#0c1c31", FG: "#d6dae0", Accent: "#e2cb9c", Pal: p, Ratio: 12.5, Grade: "AAA", Applied: true, Fav: true},
		"card-selected": {Name: "dusk-synth", Family: "Movements", BG: "#170a29", FG: "#dad6e0", Accent: "#f04cac", Pal: p, Ratio: 13.2, Grade: "AAA", Selected: true},
		"card-hot":      {Name: "swiss-grid", Family: "Modernism", BG: "#fafafa", FG: "#212121", Accent: "#d41125", Pal: p, Ratio: 15.4, Grade: "AAA", Hot: true, HotColour: "#6fd3e8"},
		"card-diff":     {Name: "elegant-reserve", Family: "Image Scale", BG: "#14121a", FG: "#d6d0dc", Accent: "#9a80b8", Pal: p, Ratio: 12.3, Grade: "AAA"},
	} {
		c := Card(34, 9, 20, 42, spec)
		os.WriteFile(filepath.Join(dir, name+".png"), c.PNG(), 0o644)
	}
}
