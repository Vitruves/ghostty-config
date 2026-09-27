package color

import "testing"

func TestParseHexForms(t *testing.T) {
	for _, in := range []string{"#AABBCC", "aabbcc", "#abc", "ABC"} {
		if _, err := ParseHex(in); err != nil {
			t.Errorf("%q should parse: %v", in, err)
		}
	}
	if _, err := ParseHex("#12345"); err == nil {
		t.Error("five digits must fail")
	}
	if Normalize("#ABCDEF", "") != "#abcdef" {
		t.Error("normalize should lower-case")
	}
}

func TestContrastKnownValues(t *testing.T) {
	if r := Contrast("#000000", "#ffffff"); r < 20.9 || r > 21.1 {
		t.Fatalf("black/white = %v", r)
	}
	if r := Contrast("#777777", "#ffffff"); r < 4.4 || r > 4.6 {
		t.Fatalf("grey/white = %v", r)
	}
}

func TestHSLRoundTrip(t *testing.T) {
	for _, hex := range []string{"#ff0000", "#00ff00", "#0000ff", "#123456", "#fedcba", "#808080"} {
		c, _ := ParseHex(hex)
		if got := c.ToHSL().Hex(); got != hex {
			t.Errorf("%s → %s", hex, got)
		}
	}
}

func TestEnsureContrastReaches(t *testing.T) {
	out := EnsureContrast("#333333", "#101010", ContrastAA)
	if Contrast(out, "#101010") < ContrastAA {
		t.Fatalf("did not reach AA: %s", out)
	}
}

func TestGeneratedSchemesAreReadable(t *testing.T) {
	for i := 0; i < 200; i++ {
		var s *Scheme
		switch i % 3 {
		case 0:
			s = Random(i%2 == 0, 0, false)
		default:
			s = Harmonious(i%2 == 0, Harmony(i%6), 0, false)
		}
		if s.Contrast() < ContrastAAA {
			t.Errorf("%s: text %.1f:1 below AAA", s.Name, s.Contrast())
		}
		for j := 1; j < 7; j++ {
			if r := Contrast(s.Normal[j], s.Background); r < MinContrastRatio {
				t.Errorf("%s: normal %s %.1f:1", s.Name, ANSI[j], r)
			}
			if r := Contrast(s.Bright[j], s.Background); r < MinContrastRatio {
				t.Errorf("%s: bright %s %.1f:1", s.Name, ANSI[j], r)
			}
		}
		if Contrast(s.SelectText, s.Selection) < ContrastLow {
			t.Errorf("%s: selection text unreadable", s.Name)
		}
	}
}

func TestSemanticHuesStayRecognisable(t *testing.T) {
	for i := 0; i < 100; i++ {
		// Monochromatic deliberately paints every slot in one hue.
		s := Harmonious(true, Harmony(i%5), float64(i*37%360), true)
		if s.Name == "" {
			t.Fatal("name")
		}
		red, _ := ParseHex(s.Normal[1])
		if h := red.ToHSL().H; HueDistance(h, 2) > 25 {
			t.Errorf("%s: red drifted to %.0f°", s.Name, h)
		}
		green, _ := ParseHex(s.Normal[2])
		if h := green.ToHSL().H; HueDistance(h, 128) > 32 {
			t.Errorf("%s: green drifted to %.0f°", s.Name, h)
		}
	}
}

func TestAdjustSteps(t *testing.T) {
	if Adjust("#808080", AdjustBrightness, true) != "#888888" {
		t.Error("brightness")
	}
	c, _ := ParseHex(Adjust("#ff0000", AdjustHue, true))
	if h := c.ToHSL().H; h < 7 || h > 9 {
		t.Errorf("hue step = %v", h)
	}
}
