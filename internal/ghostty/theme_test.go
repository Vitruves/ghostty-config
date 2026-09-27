package ghostty

import (
	"strings"
	"testing"
)

const catppuccin = `palette = 0=#45475a
palette = 1=#f38ba8
palette = 15=#bac2de
palette = 16=#123456
background = #1e1e2e
foreground = #cdd6f4
cursor-color = #f5e0dc
cursor-text = #1e1e2e
selection-background = #585b70
selection-foreground = #cdd6f4
bold-color = #ffffff
`

func TestParseThemeKeepsWhatItDoesNotManage(t *testing.T) {
	th := ParseThemeText(catppuccin)
	if th.Colors["palette.0"] != "#45475a" || th.Colors["background"] != "#1e1e2e" {
		t.Fatalf("colours: %v", th.Colors)
	}
	if len(th.Passthrough) != 2 {
		t.Fatalf("passthrough = %v", th.Passthrough)
	}
	th.Name = "test"
	out := th.Render(OwnedHeader)
	if !strings.HasPrefix(out, OwnedHeader+" test\n") {
		t.Fatalf("header missing: %q", out)
	}
	for _, want := range []string{"palette = 16=#123456", "bold-color = #ffffff", "palette = 0=#45475a", "selection-foreground = #cdd6f4"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered theme lacks %q:\n%s", want, out)
		}
	}
	again := ParseThemeText(out)
	if !again.Equal(th) {
		t.Fatal("render/parse is not a round trip")
	}
}

func TestThemeSettingPair(t *testing.T) {
	s := ParseThemeSetting("light:Rose Pine Dawn, dark:Rose Pine")
	if !s.Pair || s.Light != "Rose Pine Dawn" || s.Dark != "Rose Pine" {
		t.Fatalf("%+v", s)
	}
	if got := s.Active(true); got != "Rose Pine" {
		t.Fatalf("active dark = %q", got)
	}
	if got := s.With("Nord", true); got != "light:Rose Pine Dawn,dark:Nord" {
		t.Fatalf("with = %q", got)
	}
	plain := ParseThemeSetting("Dracula")
	if plain.Pair || plain.With("Nord", false) != "Nord" {
		t.Fatal("plain setting should be replaced outright")
	}
}

func TestLabelsAndSections(t *testing.T) {
	if Label("palette.9") != "red" || Label("cursor-color") != "color" || Label("background") != "background" {
		t.Fatal("labels")
	}
	n := 0
	for _, s := range Sections {
		n += len(s.Keys)
	}
	if n != len(ColorKeys) {
		t.Fatalf("sections cover %d keys, ColorKeys has %d", n, len(ColorKeys))
	}
}

func TestSanitizeName(t *testing.T) {
	if got := SanitizeName("  ../My Theme!.txt "); got != "My Theme.txt" {
		t.Fatalf("got %q", got)
	}
}
