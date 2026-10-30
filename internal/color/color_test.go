package color

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseHexForms(t *testing.T) {
	for _, in := range []string{"#AABBCC", "aabbcc", "#abc", "ABC"} {
		if _, err := ParseHex(in); err != nil {
			t.Errorf("%q should parse: %v", in, err)
		}
	}
	if _, err := ParseHex("#12345"); err == nil {
		t.Error("five digits must fail")
	}
	if c, err := ParseHex("#1a2B3c"); err != nil || c != (RGB{0x1a, 0x2b, 0x3c}) {
		t.Errorf("#1a2B3c = %v, %v", c, err)
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

func TestNormalizeHue(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0, 0}, {360, 0}, {-360, 0}, {180, 180},
		{720, 0}, {-90, 270}, {450, 90}, {-450, 270},
	}
	for _, c := range cases {
		if got := NormalizeHue(c.in); got != c.want {
			t.Errorf("NormalizeHue(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestHueDistance(t *testing.T) {
	cases := []struct {
		a, b, want float64
	}{
		{0, 0, 0}, {0, 180, 180}, {0, 360, 0},
		{10, 350, 20}, {0, 90, 90}, {180, 170, 10},
		{0, 181, 179}, {359, 1, 2},
	}
	for _, c := range cases {
		if got := HueDistance(c.a, c.b); got != c.want {
			t.Errorf("HueDistance(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestGrade(t *testing.T) {
	cases := []struct {
		ratio float64
		want  string
	}{
		{21, "AAA"}, {7.0, "AAA"}, {6.9, "AA"},
		{4.5, "AA"}, {4.49, "AA+"}, {3.0, "AA+"},
		{2.9, "low"}, {1, "low"},
	}
	for _, c := range cases {
		if got := Grade(c.ratio); got != c.want {
			t.Errorf("Grade(%v) = %q, want %q", c.ratio, got, c.want)
		}
	}
}

func TestIsDark(t *testing.T) {
	cases := map[string]bool{
		"#000000": true, "#ffffff": false, "#282c34": true,
		"#cdd6f4": false, "#808080": true, "garbage": true,
	}
	for hex, want := range cases {
		if got := IsDark(hex); got != want {
			t.Errorf("IsDark(%q) = %v, want %v", hex, got, want)
		}
	}
}

func TestIsHex(t *testing.T) {
	valid := []string{"#000000", "fff", "#abc123", "ABCDEF", "  #123  "}
	invalid := []string{"", "#12345", "gggggg", "#", "xyz", "#-1-1-1", "#ff000g", "+1+1+1", "#f f f ", "-1-", "0x1234"}
	for _, s := range valid {
		if !IsHex(s) {
			t.Errorf("IsHex(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if IsHex(s) {
			t.Errorf("IsHex(%q) = true, want false", s)
		}
	}
}

func TestBlend(t *testing.T) {
	cases := []struct {
		a, b  string
		ratio float64
		want  string
	}{
		{"#000000", "#ffffff", 0.0, "#000000"},
		{"#000000", "#ffffff", 1.0, "#ffffff"},
		{"#000000", "#ffffff", 0.5, "#808080"},
		{"#ff0000", "#00ff00", 0.5, "#808000"},
		{"#ff0000", "#0000ff", 0.25, "#bf0040"},
		{"garbage", "#ffffff", 0.5, "garbage"},
	}
	for _, c := range cases {
		if got := Blend(c.a, c.b, c.ratio); got != c.want {
			t.Errorf("Blend(%q, %q, %v) = %q, want %q", c.a, c.b, c.ratio, got, c.want)
		}
	}
}

func TestEnsureReadable(t *testing.T) {
	t.Run("already readable", func(t *testing.T) {
		if got := EnsureReadable("#ffffff", "#000000"); got != "#ffffff" {
			t.Fatalf("got %q, want %q", got, "#ffffff")
		}
	})
	t.Run("dark bg falls back to white", func(t *testing.T) {
		got := EnsureReadable("#1a1a1a", "#000000")
		if Contrast(got, "#000000") < ContrastAA {
			t.Fatalf("%s does not reach AA", got)
		}
	})
	t.Run("light bg falls back to black", func(t *testing.T) {
		got := EnsureReadable("#e5e5e5", "#ffffff")
		if Contrast(got, "#ffffff") < ContrastAA {
			t.Fatalf("%s does not reach AA", got)
		}
	})
}

func TestHSLHex(t *testing.T) {
	cases := []struct {
		h, s, l float64
		want    string
	}{
		{0, 0, 0, "#000000"},
		{0, 1, 0.5, "#ff0000"},
		{120, 1, 0.5, "#00ff00"},
		{240, 1, 0.5, "#0000ff"},
		{0, 0, 1, "#ffffff"},
	}
	for _, c := range cases {
		if got := HSLHex(c.h, c.s, c.l); got != c.want {
			t.Errorf("HSLHex(%v, %v, %v) = %q, want %q", c.h, c.s, c.l, got, c.want)
		}
	}
}

func TestDescribe(t *testing.T) {
	t.Run("without contrast", func(t *testing.T) {
		got := Describe("#ff0000", "#000000", false)
		if !strings.Contains(got, "#ff0000") || !strings.Contains(got, "rgb(255,0,0)") {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("with contrast", func(t *testing.T) {
		got := Describe("#ff0000", "#000000", true)
		if !strings.Contains(got, "AA") {
			t.Fatalf("red on black should grade at least AA, got %q", got)
		}
	})
	t.Run("invalid hex", func(t *testing.T) {
		if got := Describe("garbage", "#000000", false); got != "garbage" {
			t.Fatalf("got %q", got)
		}
	})
}

// The quick paths say what the slow ones would.
func TestQuickPathsAgree(t *testing.T) {
	for v := -2; v < 260; v++ {
		c := RGB{v, 255 - v, v / 2}
		if got, want := c.Hex(), fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B); got != want {
			t.Fatalf("Hex(%v) = %q, want %q", c, got, want)
		}
		if v >= 0 && v <= 255 && linear[v] != toLinear(v) {
			t.Fatalf("linear[%d] differs", v)
		}
	}
	for _, s := range []string{"#abcdef", "#ABCDEF", "abcdef", " #abcdef", "#abc", "#abcdeg", "#abcdef0", ""} {
		want := "fallback"
		if c, err := ParseHex(s); err == nil {
			want = c.Hex()
		}
		if got := Normalize(s, "fallback"); got != want {
			t.Fatalf("Normalize(%q) = %q, want %q", s, got, want)
		}
	}
}

func BenchmarkNormalize(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Normalize("#1e1e2e", "")
		Normalize("#CDD6F4", "")
		Contrast("#cdd6f4", "#1e1e2e")
	}
}
