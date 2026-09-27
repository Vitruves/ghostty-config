package ghostty

import "testing"

func TestStylesFromFaces(t *testing.T) {
	styles := stylesFromFaces("JetBrainsMono Nerd Font", []string{"JetBrainsMono Nerd Font Bold Italic", "JetBrainsMono Nerd Font", "Other Face"})
	if len(styles) != 3 || styles[0] != "Bold Italic" || styles[1] != "Regular" || styles[2] != "Other Face" {
		t.Fatalf("got %v", styles)
	}
}

func TestFormatSize(t *testing.T) {
	for in, want := range map[float64]string{13: "13", 14.5: "14.5", 12.25: "12.25"} {
		if got := FormatSize(in); got != want {
			t.Errorf("%v → %q, want %q", in, got, want)
		}
	}
}

func TestReloadKeystrokeFallsBackToDefault(t *testing.T) {
	key, mods := reloadKeystroke("")
	if key != "," || mods != "command down, shift down" {
		t.Fatalf("got %q %q", key, mods)
	}
}
