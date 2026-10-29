package fontdl

import (
	"archive/zip"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The table: every name once, archive names of the shape the release uses,
// the popular group first and the rest A to Z.
func TestTableIsWellFormed(t *testing.T) {
	if len(AvailableDownloads) < 60 {
		t.Fatalf("only %d families on offer", len(AvailableDownloads))
	}
	shape := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+-]*$`)
	names, archives := map[string]bool{}, map[string]bool{}
	popular := 0
	for i, d := range AvailableDownloads {
		if names[strings.ToLower(d.Name)] || archives[d.Archive] {
			t.Errorf("%s (%s) is listed twice", d.Name, d.Archive)
		}
		names[strings.ToLower(d.Name)], archives[d.Archive] = true, true
		if !shape.MatchString(d.Archive) {
			t.Errorf("%s: archive %q is not a release asset name", d.Name, d.Archive)
		}
		if d.Note == "" || strings.HasSuffix(d.Note, ".") {
			t.Errorf("%s: the note %q should be a phrase without a full stop", d.Name, d.Note)
		}
		if d.Popular {
			if i != popular {
				t.Errorf("%s is popular but comes after the others", d.Name)
			}
			popular++
		}
		if d.URL != "" {
			// Not Nerd Fonts: a release of the project itself, at a version.
			if !regexp.MustCompile(`^https://github\.com/[^/]+/[^/]+/releases/download/v?\d[^/]*/[^/]+\.zip$`).MatchString(d.URL) {
				t.Errorf("%s: %s is not a pinned GitHub release zip", d.Name, d.URL)
			}
		}
		if d.Only && len(d.Families) == 0 {
			t.Errorf("%s: Only with no families would unpack nothing", d.Name)
		}
	}
	if popular != 17 {
		t.Errorf("%d popular families, want the 17 well-known ones", popular)
	}
	more := AvailableDownloads[popular:]
	if !sort.SliceIsSorted(more, func(a, b int) bool { return strings.ToLower(more[a].Name) < strings.ToLower(more[b].Name) }) {
		t.Error("the families after the popular ones are not A to Z")
	}
	for _, skipped := range []string{"FontPatcher", "NerdFontsSymbolsOnly", "Arimo", "Tinos", "Ubuntu"} {
		if archives[skipped] {
			t.Errorf("%s is not a monospaced text family", skipped)
		}
	}
	if d, ok := Find("SourceCodePro"); !ok || d.Name != "Source Code Pro" || !d.Popular {
		t.Errorf("Find: %+v %v", d, ok)
	}
}

// installedAs are names the families are installed under, as the 3.5.1
// archives and the projects' own zips write them in their name tables.
var installedAs = map[string][]string{
	"GoogleSansCode":           {"Google Sans Code", "GoogleSansCode NF"},
	"JetBrainsMono":            {"JetBrainsMono NF", "JetBrainsMono NFM", "JetBrainsMonoNL NFP", "JetBrainsMono Nerd Font", "JetBrains Mono"},
	"FiraCode":                 {"FiraCode Nerd Font", "FiraCode Nerd Font Mono", "Fira Code"},
	"Hack":                     {"Hack Nerd Font", "Hack Nerd Font Propo", "Hack"},
	"Meslo":                    {"MesloLGS Nerd Font", "MesloLGM Nerd Font Mono", "MesloLGLDZ Nerd Font"},
	"SourceCodePro":            {"SauceCodePro NF", "SauceCodePro NFM", "Source Code Pro"},
	"CascadiaCode":             {"CaskaydiaCove NF", "CaskaydiaCove Nerd Font Mono", "Cascadia Code"},
	"Iosevka":                  {"Iosevka NF", "Iosevka Nerd Font"},
	"UbuntuMono":               {"UbuntuMono Nerd Font"},
	"DejaVuSansMono":           {"DejaVuSansM Nerd Font", "DejaVu Sans Mono"},
	"RobotoMono":               {"RobotoMono Nerd Font"},
	"Inconsolata":              {"Inconsolata Nerd Font"},
	"VictorMono":               {"VictorMono NF"},
	"SpaceMono":                {"SpaceMono Nerd Font"},
	"IBMPlexMono":              {"BlexMono Nerd Font", "IBM Plex Mono"},
	"AnonymousPro":             {"AnonymicePro Nerd Font"},
	"Terminus":                 {"Terminess Nerd Font"},
	"0xProto":                  {"0xProto Nerd Font"},
	"3270":                     {"3270 Nerd Font"},
	"AdwaitaMono":              {"AdwaitaMono Nerd Font"},
	"Agave":                    {"Agave Nerd Font"},
	"AnnotationMono":           {"AnnotationM NF"},
	"AtkinsonHyperlegibleMono": {"AtkynsonMono NF"},
	"AurulentSansMono":         {"AurulentSansM Nerd Font"},
	"BigBlueTerminal":          {"BigBlueTermPlus Nerd Font", "BigBlueTerm437 Nerd Font"},
	"BitstreamVeraSansMono":    {"BitstromWera Nerd Font"},
	"CascadiaMono":             {"CaskaydiaMono NF", "CaskaydiaMono NFP"},
	"CodeNewRoman":             {"CodeNewRoman Nerd Font"},
	"ComicShannsMono":          {"ComicShannsMono Nerd Font"},
	"CommitMono":               {"CommitMono Nerd Font"},
	"Cousine":                  {"Cousine Nerd Font"},
	"D2Coding":                 {"D2KodingLigature Nerd Font"},
	"DaddyTimeMono":            {"DaddyTimeMono Nerd Font"},
	"DepartureMono":            {"DepartureMono Nerd Font"},
	"DroidSansMono":            {"DroidSansM Nerd Font"},
	"EnvyCodeR":                {"EnvyCodeR Nerd Font"},
	"FantasqueSansMono":        {"FantasqueSansM Nerd Font"},
	"FiraMono":                 {"FiraMono Nerd Font"},
	"GeistMono":                {"GeistMono NF"},
	"Go-Mono":                  {"GoMono Nerd Font"},
	"Gohu":                     {"GohuFont 11 Nerd Font", "GohuFont uni14 Nerd Font Mono"},
	"Hasklig":                  {"Hasklug Nerd Font"},
	"HeavyData":                {"HeavyData Nerd Font"},
	"Hermit":                   {"Hurmit Nerd Font"},
	"iA-Writer":                {"iMWritingMono Nerd Font"},
	"InconsolataGo":            {"InconsolataGo Nerd Font"},
	"InconsolataLGC":           {"Inconsolata LGC Nerd Font"},
	"IntelOneMono":             {"IntoneMono NF"},
	"IosevkaTerm":              {"IosevkaTerm NF"},
	"IosevkaTermSlab":          {"IosevkaTermSlab NF"},
	"JuliaMono":                {"JuliaMono"},
	"Lekton":                   {"Lekton Nerd Font"},
	"LiberationMono":           {"LiterationMono Nerd Font"},
	"Lilex":                    {"Lilex Nerd Font"},
	"MPlus":                    {"M+1Code Nerd Font", "M+CodeLat60 Nerd Font"},
	"MapleMono":                {"Maple Mono NF", "Maple Mono"},
	"MartianMono":              {"MartianMono NF"},
	"Monaspace":                {"MonaspiceNe NF", "MonaspiceXe NFM"},
	"Monofur":                  {"Monofur Nerd Font"},
	"Monoid":                   {"Monoid Nerd Font"},
	"Mononoki":                 {"Mononoki Nerd Font"},
	"Noto":                     {"NotoSansM NF", "NotoMono NF"},
	"OpenDyslexic":             {"OpenDyslexicM Nerd Font"},
	"Overpass":                 {"OverpassM Nerd Font"},
	"ProFont":                  {"ProFont IIx Nerd Font", "ProFontWindows Nerd Font"},
	"ProggyClean":              {"ProggyClean Nerd Font", "ProggyClean CE Nerd Font", "ProggyCleanSZ Nerd Font"},
	"Recursive":                {"RecMonoLinear Nerd Font", "RecMonoSmCasual Nerd Font"},
	"ShareTechMono":            {"ShureTechMono Nerd Font"},
	"UbuntuSans":               {"UbuntuSansMono NF"},
	"ZedMono":                  {"ZedMono NF"},
}

// Each family is recognised under the names it is installed by, and by no
// other entry of the table: Iosevka is not Iosevka Term, Inconsolata not
// Inconsolata Go.
func TestInstalledNamesAreMatched(t *testing.T) {
	for _, d := range AvailableDownloads {
		names, ok := installedAs[d.Archive]
		if !ok {
			t.Errorf("no installed names to test %s with", d.Archive)
			continue
		}
		for _, name := range names {
			if !d.Provides(name) {
				t.Errorf("%s does not recognise %q", d.Name, name)
			}
			if i := Owner(name); i < 0 || AvailableDownloads[i].Archive != d.Archive {
				t.Errorf("Owner(%q) = %d, want %s", name, i, d.Archive)
			}
			for _, other := range AvailableDownloads {
				if other.Archive != d.Archive && other.Provides(name) {
					t.Errorf("%q is taken for %s as well as %s", name, other.Name, d.Name)
				}
			}
		}
	}
	// The proportional families an archive leaves out are not it.
	for archive, names := range map[string][]string{
		"MPlus": {"M+1 Nerd Font", "M+2 Nerd Font"}, "Noto": {"NotoSans NF", "NotoSerif NF"},
		"LiberationMono": {"LiterationSans Nerd Font"}, "Overpass": {"Overpass Nerd Font"},
		"Iosevka": {"Iosevka Term", "IosevkaTerm NF"}, "Hack": {"Hacker"}, "Lekton": {""},
	} {
		d, _ := Find(archive)
		for _, name := range names {
			if d.Provides(name) {
				t.Errorf("%s should not recognise %q", d.Name, name)
			}
		}
	}
}

// An archive that carries proportional families beside the one on offer
// unpacks only that one; others unpack every face.
func TestOnlyTheFamiliesOnOfferAreUnpacked(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "Noto.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for _, name := range []string{"NotoSansMNerdFont-Regular.ttf", "NotoSansMNerdFontMono-Bold.ttf", "NotoMonoNerdFont-Regular.ttf",
		"NotoSansNerdFont-Regular.ttf", "NotoSerifNerdFontPropo-Italic.ttf", "README.md"} {
		e, _ := w.Create("Noto/" + name)
		_, _ = e.Write([]byte("font"))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	noto, _ := Find("Noto")
	out := filepath.Join(dir, "fonts")
	_ = os.MkdirAll(out, 0o755)
	n, err := extractFaces(archive, out, noto.keeps)
	if err != nil || n != 3 {
		t.Fatalf("unpacked %d faces (%v), want the 3 monospaced ones", n, err)
	}
	if _, err := os.Stat(filepath.Join(out, "NotoSansNerdFont-Regular.ttf")); err == nil {
		t.Fatal("a proportional family was unpacked")
	}
	hack, _ := Find("Hack")
	if !hack.keeps("NotoSansNerdFont-Regular.ttf") {
		t.Fatal("an archive not marked Only keeps every face")
	}
}
