package fontdl

import "strings"

// The families on offer, in two groups. The popular ones come first, in an
// order that puts the safest choices on top; the rest of the Nerd Fonts
// release follows, A to Z, with two families Nerd Fonts does not carry.
//
// Every archive of the Nerd Fonts 3.5.1 release is here except these:
//
//   - FontPatcher is the patching tool, not a font.
//   - NerdFontsSymbolsOnly is the icons alone; Fonts › Icons offers it.
//   - Arimo, Tinos and Ubuntu are proportional: metric twins of Arial and
//     Times, and Ubuntu's interface face. Ubuntu Mono and Ubuntu Sans Mono
//     have lines of their own.
//
// Some archives also carry proportional families beside a monospaced one:
// iA Writer's Duo and Quattro, M+ 1 and 2, Noto Sans and Serif, Literation
// Sans and Serif, Overpass, OpenDyslexic. Only the monospaced family is
// unpacked from those. No archive is bitmap only: Gohu, Terminus,
// ProggyClean and the other pixel designs ship as TrueType, which Ghostty
// loads like any other font.
//
// Outside Nerd Fonts, a family is listed only when its project publishes a
// zip of its own on GitHub at a pinned version, under the OFL. Maple Mono and
// JuliaMono qualify. Fragment Mono does too, but has no bold, which a
// terminal fakes badly; Commit Mono, Monaspace and Recursive are in Nerd
// Fonts already.

// popularDownloads are the well-known families, listed first.
var popularDownloads = []FontDownload{
	{Name: "Google Sans Code", Archive: "GoogleSansCode", Note: "Google's monospace, the font this tool draws its own pictures in",
		URL: "https://github.com/googlefonts/googlesans-code/releases/download/v7.001/GoogleSansCode-v7.001.zip"},
	nerd("JetBrains Mono", "JetBrainsMono", "Tall x-height, made for long reading", "JetBrainsMono", "JetBrainsMonoNL"),
	nerd("Fira Code", "FiraCode", "The one with the programming ligatures"),
	nerd("Hack", "Hack", "Plain, legible, no surprises"),
	nerd("Meslo LG", "Meslo", "Menlo redrawn, the powerline default", "MesloLGS", "MesloLGM", "MesloLGL", "MesloLGSDZ", "MesloLGMDZ", "MesloLGLDZ"),
	nerd("Source Code Pro", "SourceCodePro", "Adobe, conservative and even", "SauceCodePro"),
	nerd("Cascadia Code", "CascadiaCode", "Microsoft, ships with Windows Terminal", "CaskaydiaCove"),
	nerd("Iosevka", "Iosevka", "Narrow, fits more columns per line"),
	nerd("Ubuntu Mono", "UbuntuMono", "Humanist, unusually compact"),
	nerd("DejaVu Sans Mono", "DejaVuSansMono", "Enormous glyph coverage", "DejaVuSansM"),
	nerd("Roboto Mono", "RobotoMono", "Neutral and unfussy"),
	nerd("Inconsolata", "Inconsolata", "A classic, still excellent"),
	nerd("Victor Mono", "VictorMono", "Cursive italics for comments"),
	nerd("Space Mono", "SpaceMono", "Distinctive, with real character"),
	nerd("IBM Plex Mono", "IBMPlexMono", "Corporate in the good sense", "BlexMono"),
	nerd("Anonymous Pro", "AnonymousPro", "Designed for small sizes", "AnonymicePro"),
	nerd("Terminus", "Terminus", "Bitmap heritage, sharp at small sizes", "Terminess"),
}

// moreDownloads are the rest, A to Z by name.
var moreDownloads = []FontDownload{
	nerd("0xProto", "0xProto", "Wide letters, each one distinct, made for source code"),
	nerd("3270", "3270", "The IBM 3270 terminal face, by way of x3270"),
	nerd("Adwaita Mono", "AdwaitaMono", "GNOME's monospace, drawn from Iosevka"),
	nerd("Agave", "Agave", "Small, simple and geometrically regular"),
	nerd("Annotation Mono", "AnnotationMono", "Idealised handwriting, still easy to read", "AnnotationM"),
	nerd("Atkinson Hyperlegible Mono", "AtkinsonHyperlegibleMono", "Drawn for low vision, every letter distinct", "AtkynsonMono"),
	nerd("Aurulent Sans Mono", "AurulentSansMono", "A soft sans serif, a single weight", "AurulentSansM"),
	nerd("BigBlue Terminal", "BigBlueTerminal", "IBM's EGA and VGA text mode, pixel for pixel", "BigBlueTermPlus", "BigBlueTerm437"),
	nerd("Bitstream Vera Sans Mono", "BitstreamVeraSansMono", "The ancestor of DejaVu and Hack", "BitstromWera"),
	nerd("Cascadia Mono", "CascadiaMono", "Cascadia Code without the ligatures", "CaskaydiaMono"),
	nerd("Code New Roman", "CodeNewRoman", "Compact, with a slashed zero"),
	nerd("Comic Shanns Mono", "ComicShannsMono", "Comic Sans redrawn for code, on purpose"),
	nerd("Commit Mono", "CommitMono", "Neutral, with smart kerning that evens the gaps"),
	nerd("Cousine", "Cousine", "Courier New's metrics, far easier to read"),
	nerd("D2Coding", "D2Coding", "Naver's coding font, Korean included", "D2KodingLigature"),
	nerd("DaddyTimeMono", "DaddyTimeMono", "Rounded and quirky, a single weight"),
	nerd("Departure Mono", "DepartureMono", "A pixel font with a lo-fi, techy feel"),
	nerd("Droid Sans Mono", "DroidSansMono", "Android's first monospace, good at small sizes", "DroidSansM"),
	nerd("Envy Code R", "EnvyCodeR", "Drawn for code and command prompts"),
	nerd("Fantasque Sans Mono", "FantasqueSansMono", "A handwritten wobble, still tidy", "FantasqueSansM"),
	nerd("Fira Mono", "FiraMono", "Mozilla's monospace, Fira Code without ligatures"),
	nerd("Geist Mono", "GeistMono", "Vercel's monospace, clean and geometric"),
	nerd("Go Mono", "Go-Mono", "Made for the Go project, with small serifs", "GoMono"),
	nerd("Gohu", "Gohu", "A bitmap design, crisp at 11 and 14 pixels", "GohuFont 11", "GohuFont 14", "GohuFont uni11", "GohuFont uni14"),
	nerd("Hasklig", "Hasklig", "Source Code Pro with ligatures", "Hasklug"),
	nerd("Heavy Data", "HeavyData", "Bold and blocky, one weight only"),
	nerd("Hermit", "Hermit", "Symbols that stand out from the letters", "Hurmit"),
	only(nerd("iA Writer Mono", "iA-Writer", "From the iA Writer app, built on IBM Plex", "iMWritingMono")),
	nerd("Inconsolata Go", "InconsolataGo", "Inconsolata with straight quotes"),
	nerd("Inconsolata LGC", "InconsolataLGC", "Inconsolata with Greek and Cyrillic", "Inconsolata LGC"),
	nerd("Intel One Mono", "IntelOneMono", "Intel's, shaped with low-vision developers", "IntoneMono"),
	nerd("Iosevka Term", "IosevkaTerm", "Iosevka with narrow arrows, for terminals"),
	nerd("Iosevka Term Slab", "IosevkaTermSlab", "Iosevka Term with slab serifs"),
	{Name: "JuliaMono", Archive: "JuliaMono", Note: "Vast Unicode coverage, from the Julia community",
		URL: "https://github.com/cormullion/juliamono/releases/download/v0.63.2/JuliaMono-ttf.zip"},
	nerd("Lekton", "Lekton", "Light and thin; 0 and O look alike"),
	only(nerd("Liberation Mono", "LiberationMono", "Courier New's metrics, from Red Hat", "LiterationMono")),
	nerd("Lilex", "Lilex", "IBM Plex Mono extended with ligatures"),
	only(nerd("M+ Code", "MPlus", "Japanese included, Latin in two widths", "M+1Code", "M+CodeLat50", "M+CodeLat60")),
	{Name: "Maple Mono", Archive: "MapleMono", Note: "Rounded, cursive italics, Nerd icons built in",
		URL: "https://github.com/subframe7536/maple-font/releases/download/v7.9/MapleMono-NF.zip", Families: []string{"Maple Mono NF"}},
	nerd("Martian Mono", "MartianMono", "Evil Martians' wide, technical monospace"),
	nerd("Monaspace", "Monaspace", "GitHub's five matching faces, with texture healing", "MonaspiceAr", "MonaspiceKr", "MonaspiceNe", "MonaspiceRn", "MonaspiceXe"),
	nerd("Monofur", "Monofur", "Curvy and compact, an acquired taste"),
	nerd("Monoid", "Monoid", "Narrow, with large operators and ligatures"),
	nerd("Mononoki", "Mononoki", "Made so that look-alike letters never blur"),
	only(nerd("Noto Sans Mono", "Noto", "Google's Noto, monospaced; a 600 MB archive", "NotoSansM", "NotoMono")),
	only(nerd("OpenDyslexic Mono", "OpenDyslexic", "Weighted shapes meant to help dyslexic readers", "OpenDyslexicM")),
	only(nerd("Overpass Mono", "Overpass", "Inspired by American highway signs", "OverpassM")),
	nerd("ProFont", "ProFont", "A tiny-size classic with squared corners", "ProFont IIx", "ProFontWindows"),
	nerd("ProggyClean", "ProggyClean", "Made for very small sizes", "ProggyClean", "ProggyClean CE", "ProggyCleanSZ"),
	nerd("Recursive Mono", "Recursive", "Linear or casual, four cuts of Recursive", "RecMonoLinear", "RecMonoCasual", "RecMonoSmCasual", "RecMonoDuotone"),
	nerd("Share Tech Mono", "ShareTechMono", "Squarish and technical, one weight", "ShureTechMono"),
	only(nerd("Ubuntu Sans Mono", "UbuntuSans", "The newer Ubuntu monospace", "UbuntuSansMono")),
	nerd("Zed Mono", "ZedMono", "From the Zed editor, a rounder Iosevka"),
}

// AvailableDownloads is every family on offer, the popular ones first.
var AvailableDownloads = func() []FontDownload {
	out := make([]FontDownload, 0, len(popularDownloads)+len(moreDownloads))
	for _, d := range popularDownloads {
		d.Popular = true
		out = append(out, d)
	}
	return append(out, moreDownloads...)
}()

// nerd is a family of the Nerd Fonts release. families are the names its
// faces are installed under, before " Nerd Font"; none means the archive's
// own name.
func nerd(name, archive, note string, families ...string) FontDownload {
	return FontDownload{Name: name, Archive: archive, Note: note, Families: families}
}

// only marks an archive that also carries families not on offer.
func only(d FontDownload) FontDownload {
	d.Only = true
	return d
}

// compact is a family name as it is compared: no spaces, any case.
func compact(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", ""))
}

// names are the families the download installs: for Nerd Fonts, the names
// before " Nerd Font".
func (d FontDownload) names() []string {
	if len(d.Families) > 0 {
		return d.Families
	}
	return []string{d.Archive}
}

// Provides reports whether an installed family is one this download brings,
// or the original it was made from. "JetBrains Mono" counts, and so do
// "JetBrainsMono Nerd Font" and its Mono and Propo cuts. Nerd Fonts writes
// "NF", "NFM" and "NFP" instead where the full name would not fit the font's
// name table, which is why "JetBrainsMono NF" counts too.
//
// Names are matched by prefix, so the families of one archive must not start
// with those of another: "Iosevka Nerd Font" is not "IosevkaTerm Nerd Font".
func (d FontDownload) Provides(family string) bool {
	return d.matcher().matches(compact(family))
}

// matcher is what Provides compares a family's compacted name with.
type matcher struct {
	exact    string
	prefixes []string
}

func (d FontDownload) matcher() matcher {
	m := matcher{exact: compact(d.Name)}
	for _, name := range d.names() {
		c := compact(name)
		if d.URL != "" {
			m.prefixes = append(m.prefixes, c)
			continue
		}
		m.prefixes = append(m.prefixes, c+"nerdfont", c+"nf")
	}
	return m
}

func (m matcher) matches(family string) bool {
	if family == "" {
		return false
	}
	if family == m.exact {
		return true
	}
	for _, p := range m.prefixes {
		if strings.HasPrefix(family, p) {
			return true
		}
	}
	return false
}

// matchers are those of AvailableDownloads, worked out once: a machine has
// hundreds of families, and the font list asks on every keystroke.
var matchers = func() []matcher {
	out := make([]matcher, len(AvailableDownloads))
	for i, d := range AvailableDownloads {
		out[i] = d.matcher()
	}
	return out
}()

// Owner is the index in AvailableDownloads of the family on offer that an
// installed family comes from, -1 when none does.
func Owner(family string) int {
	f := compact(family)
	for i, m := range matchers {
		if m.matches(f) {
			return i
		}
	}
	return -1
}

// keeps reports whether a file of the archive is unpacked: every font file,
// or for an archive marked Only, those of the families on offer. Nerd Fonts
// names its files after the family, "SauceCodeProNerdFontMono-Bold.ttf".
func (d FontDownload) keeps(file string) bool {
	if !d.Only {
		return true
	}
	base := compact(file)
	for _, name := range d.names() {
		if strings.HasPrefix(base, compact(name)+"nerdfont") {
			return true
		}
	}
	return false
}

// Find returns the family on offer with this archive name.
func Find(archive string) (FontDownload, bool) {
	for _, d := range AvailableDownloads {
		if d.Archive == archive {
			return d, true
		}
	}
	return FontDownload{}, false
}

// HasIcons reports whether the family comes with the Nerd Font icons: every
// one of the Nerd Fonts release, and a project's own "NF" build.
func (d FontDownload) HasIcons() bool {
	if d.URL == "" {
		return true
	}
	for _, name := range d.Families {
		if strings.HasSuffix(name, " NF") {
			return true
		}
	}
	return false
}

// Source is where the archive comes from, as "owner/repository".
func (d FontDownload) Source() string {
	if d.URL == "" {
		return "ryanoasis/nerd-fonts"
	}
	source := strings.TrimPrefix(d.URL, "https://")
	if parts := strings.SplitN(source, "/", 4); len(parts) >= 3 {
		return parts[1] + "/" + parts[2]
	}
	return source
}
