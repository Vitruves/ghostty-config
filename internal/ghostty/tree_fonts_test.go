package ghostty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const bellDefaults = "no-system,no-audio,attention,title,no-border"

func TestWithFlagChangesOneWordAndKeepsTheRest(t *testing.T) {
	for _, c := range []struct {
		value, flag string
		on          bool
		want        string
	}{
		{"", "audio", true, "audio"},
		{"audio", "audio", false, ""},
		{"", "title", false, "no-title"},
		{"no-title", "title", true, ""},
		// The word stays where it was; words this tool does not know stay.
		{"border, future-thing,no-attention", "attention", true, "border,future-thing"},
		{"border,future-thing,attention", "attention", false, "border,future-thing,no-attention"},
		{"system,no-system,audio", "system", true, "system,audio"},
		// A bare boolean stands for every flag.
		{"true", "border", false, "system,audio,attention,title"},
		{"false", "title", true, "no-system,no-audio,no-attention,no-border"},
	} {
		if got := WithFlag(c.value, c.flag, c.on, bellDefaults); got != c.want {
			t.Errorf("WithFlag(%q, %s, %v) = %q, want %q", c.value, c.flag, c.on, got, c.want)
		}
	}
	f := Flags("audio,no-title,future-thing", bellDefaults)
	if !f["audio"] || f["title"] || !f["attention"] || f["border"] || !f["future-thing"] {
		t.Errorf("Flags read %v", f)
	}
	if f := Flags("TRUE", bellDefaults); !f["system"] || !f["border"] {
		t.Errorf("true switches every flag on: %v", f)
	}
}

func loadText(t *testing.T, text string) (*Tree, string) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "config")
	if err := os.WriteFile(root, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	tree, err := Load(Paths{Roots: []string{root}, StateDir: filepath.Join(dir, "state")})
	if err != nil {
		t.Fatal(err)
	}
	return tree, root
}

func live(tree *Tree, key string) []string {
	var out []string
	for _, e := range tree.All(key) {
		out = append(out, e.Value)
	}
	return out
}

func TestSetFlagRewritesTheLineInEffect(t *testing.T) {
	tree, _ := loadText(t, "bell-features = system\nfont-size = 13\nbell-features = border,future-thing\n")
	tree.SetFlag("bell-features", "audio", true, bellDefaults)
	if got := live(tree, "bell-features"); len(got) != 2 || got[1] != "border,future-thing,audio" {
		t.Fatalf("the last line wins and takes the flag: %v", got)
	}
	tree.SetFlag("bell-features", "border", false, bellDefaults)
	tree.SetFlag("bell-features", "audio", false, bellDefaults)
	tree.SetFlag("bell-features", "future-thing", false, bellDefaults)
	if got := live(tree, "bell-features"); len(got) != 0 {
		t.Fatalf("a list back to the defaults is switched off, the lines it outranked with it: %v", got)
	}
}

func TestFontFeaturesLiveBesideTheLigatures(t *testing.T) {
	tree, _ := loadText(t, "font-family = Iosevka\nfont-feature = cv01, -ss02\n")
	tree.SetLigatures(false)
	tree.SetFontFeature("ss01", true)
	tree.SetFontFeature("ss02", true)
	if !tree.FontFeatureOn("ss01") || !tree.FontFeatureOn("ss02") || !tree.FontFeatureOn("cv01") {
		t.Fatalf("features: %v", tree.FontFeatures())
	}
	if !tree.LigaturesDisabled() {
		t.Fatal("turning features on must leave the ligatures off")
	}
	if got := live(tree, "font-feature"); strings.Join(got, " | ") != "cv01 | -calt, -liga, -dlig | ss01 | ss02" {
		t.Fatalf("each feature on a line of its own, the user's line keeping its other word: %v", got)
	}
	tree.SetLigatures(true)
	if !tree.FontFeatureOn("ss01") || tree.LigaturesDisabled() {
		t.Fatalf("ligatures back on must keep the features: %v", tree.FontFeatures())
	}
	tree.SetFontFeature("cv01", false)
	tree.SetFontFeature("ss01", false)
	if got := live(tree, "font-feature"); strings.Join(got, " | ") != "ss02" {
		t.Fatalf("off takes every word about the feature away: %v", got)
	}
	// Twice is the same as once: a replay may run an edit again.
	before := tree.Primary.Text()
	tree.SetFontFeature("ss02", true)
	tree.SetFontFeature("cv01", false)
	if tree.Primary.Text() != before {
		t.Fatalf("an edit already in place changed the file:\n%s", tree.Primary.Text())
	}
}

func TestFeatureWordForms(t *testing.T) {
	for w, want := range map[string]bool{"ss01": true, "+ss01": true, "ss01 on": true, "ss01=1": true, "ss01 = 2": true, `"ss01"`: true,
		"-ss01": false, "ss01 off": false, "ss01=0": false} {
		tag, on, ok := featureWord(w)
		if !ok || tag != "ss01" || on != want {
			t.Errorf("%q read as %s %v %v", w, tag, on, ok)
		}
	}
}

func TestFontVariationAndCodepointMap(t *testing.T) {
	tree, _ := loadText(t, "font-variation = wght=300\nfont-variation = wdth=90\nfont-variation = wght = 500\n")
	if v, ok := tree.FontVariation("font-variation", "wght"); !ok || v != "500" {
		t.Fatalf("the last line about an axis wins: %q", v)
	}
	tree.SetFontVariation("font-variation", "wght", "350")
	if got := live(tree, "font-variation"); strings.Join(got, " | ") != "wdth=90 | wght=350" {
		t.Fatalf("one line says the axis: %v", got)
	}
	tree.SetFontVariation("font-variation", "opsz", "14")
	tree.SetFontVariation("font-variation", "wght", "")
	if got := live(tree, "font-variation"); strings.Join(got, " | ") != "wdth=90 | opsz=14" {
		t.Fatalf("an axis back to its default has no line: %v", got)
	}

	tree.SetCodepointMap("U+E000-U+E00A", "Symbols Nerd Font Mono", true)
	tree.SetCodepointMap("U+F000-U+F2E0", "Symbols Nerd Font Mono", true)
	if got := live(tree, "font-codepoint-map"); len(got) != 1 || got[0] != "U+E000-U+E00A=Symbols Nerd Font Mono" {
		t.Fatalf("a mapping already there is kept: %v", got)
	}
	tree.SetCodepointMap("", "Symbols Nerd Font Mono", false)
	if tree.CodepointMapped("Symbols Nerd Font Mono") {
		t.Fatal("the mapping should be gone")
	}
}

// Edits to the repeatable keys made while someone else edits the file land
// on the file as they find it: each is replayed as what it means.
func TestFontAndFlagEditsSurviveAnOutsideEdit(t *testing.T) {
	tree, root := loadText(t, "font-feature = -calt, -liga, -dlig\nbell-features = no-title\n")
	tree.SetFontFeature("zero", true)
	tree.SetFontVariation("font-variation", "wght", "450")
	tree.SetCodepointMap("U+E000-U+E00A", "Symbols Nerd Font Mono", true)
	tree.SetFlag("bell-features", "border", true, bellDefaults)

	// Meanwhile, in an editor: lines above, a feature of their own, another
	// bell flag on the line.
	os.WriteFile(root, []byte("# mine\nfont-size = 15\nfont-feature = ss03\nfont-feature = -calt, -liga, -dlig\nbell-features = no-title,system\n"), 0o644)
	if _, err := tree.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(root)
	text := string(data)
	for _, want := range []string{"# mine\n", "font-size = 15\n", "font-feature = ss03\n", "font-feature = -calt, -liga, -dlig\n",
		"bell-features = no-title,system,border\n", "font-feature = zero\n", "font-variation = wght=450\n", "font-codepoint-map = U+E000-U+E00A=Symbols Nerd Font Mono\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q after the replay:\n%s", want, text)
		}
	}
	if strings.Count(text, "font-feature = zero") != 1 {
		t.Errorf("zero written more than once:\n%s", text)
	}
}
