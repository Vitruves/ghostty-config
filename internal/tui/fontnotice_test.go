package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/fontdl"
	"github.com/vitruves/ghostty-config/internal/fontinfo"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// fakeInstall stands in for the download, recording what was fetched.
func fakeInstall(t *testing.T) *[]string {
	var fetched []string
	real := installFont
	installFont = func(d fontdl.FontDownload, _ func(string)) error {
		fetched = append(fetched, d.Archive)
		return nil
	}
	t.Cleanup(func() { installFont = real })
	return &fetched
}

// finishDownload ends a download started by cmd, then lists the families
// again with these added, as the machine would.
func (h *harness) finishDownload(cmd tea.Cmd, added ...string) {
	h.t.Helper()
	done, ok := cmd().(installDoneMsg)
	if !ok {
		h.t.Fatal("the command was not a download")
	}
	h.m.Update(done)
	families := append([]ghostty.Family(nil), h.m.families...)
	for _, name := range added {
		families = append(families, ghostty.Family{Name: name, Styles: []string{"Regular", "Bold"}, Mono: true})
	}
	_, after := h.m.Update(fontsLoadedMsg{families})
	if after != nil {
		after()
	}
}

func (h *harness) fixRow() *option {
	for i := range h.m.results {
		if o := &h.m.results[i]; o.cmd != nil && o.cmd.name == "fixfont" {
			return o
		}
	}
	return nil
}

// The configured family is not installed, and the table has it: the status
// line says so once, the Fonts section opens with the line that installs
// it, and one Enter there fetches it. The config is not touched: once the
// family is there, the name it gives works.
func TestAMissingFontIsInstalledInOneEnter(t *testing.T) {
	fetched := fakeInstall(t)
	const config = "theme = Dracula\nfont-family = JetBrainsMono Nerd Font\n"
	h := newHarness(t, config)
	h.key("tab")
	if !strings.Contains(h.m.status, "Your font, JetBrainsMono Nerd Font, is not") || !strings.Contains(h.m.status, "Install JetBrains Mono") {
		t.Fatalf("the status line should say the font is missing: %q", h.m.status)
	}
	// Once per run: another message is not pushed off by it again.
	h.m.info("something else")
	h.key("down", "up")
	if h.m.status != "something else" {
		t.Fatalf("the hint came back: %q", h.m.status)
	}

	h.m.jumpToGroup("Fonts")
	o := h.fixRow()
	if o == nil || o.label != "Install JetBrains Mono" || o.detail != "not "+thisMachine() || &h.m.results[0] != o {
		t.Fatalf("the Fonts section should open with the line that installs the font: %+v", h.m.results[0])
	}
	if v := h.view(); !strings.Contains(v, "Install JetBrains Mono") || !strings.Contains(v, "is not on this") {
		t.Fatalf("the section should show the line and say what is wrong:\n%s", v)
	}
	h.m.sel.cursor = 0
	_, cmd := h.m.Update(keyPress("enter"))
	if h.m.overlay != overlayBusy || cmd == nil {
		t.Fatalf("one Enter should start the download, overlay %v", h.m.overlay)
	}
	h.finishDownload(cmd, "JetBrainsMono Nerd Font")
	if len(*fetched) != 1 || (*fetched)[0] != "JetBrainsMono" {
		t.Fatalf("fetched %v", *fetched)
	}
	if h.config() != config || !strings.Contains(h.m.status, "the config names it already") {
		t.Fatalf("the config should be left as it is (%q):\n%s", h.m.status, h.config())
	}
	if h.m.fontTrouble() != nil || h.fixRow() != nil {
		t.Fatal("the line should go once the font is there")
	}
}

// The font list opens on the same line, marked as the font in use.
func TestTheFontListOpensOnAMissingFont(t *testing.T) {
	fetched := fakeInstall(t)
	const config = "theme = Dracula\nfont-family = SauceCodePro NF\n"
	h := newHarness(t, config)
	h.typeText("font ")
	o := h.m.selected()
	if o == nil || o.label != "SauceCodePro NF" || o.value != downloadPrefix+"SourceCodePro" || !o.current {
		t.Fatalf("the list should open on the missing font, on %+v", o)
	}
	if v := h.view(); !strings.Contains(v, "Your font") || !strings.Contains(v, "Your config names it") {
		t.Fatalf("the list and its panel should say the font is missing:\n%s", v)
	}
	_, cmd := h.m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("Enter should start the download")
	}
	h.finishDownload(cmd, "SauceCodePro NF", "SauceCodePro NFM")
	if len(*fetched) != 1 || (*fetched)[0] != "SourceCodePro" || h.config() != config {
		t.Fatalf("fetched %v:\n%s", *fetched, h.config())
	}
}

// A config that names the original family gets it back under the name
// Nerd Fonts gives it, and font-family follows: the old name would still
// find nothing.
func TestAMissingOriginalBecomesItsNerdFont(t *testing.T) {
	fakeInstall(t)
	h := newHarness(t, "theme = Dracula\nfont-family = Fira Code\n")
	h.m.jumpToGroup("Fonts")
	h.m.sel.cursor = 0
	_, cmd := h.m.Update(keyPress("enter"))
	h.finishDownload(cmd, "FiraCode Nerd Font Propo", "FiraCode Nerd Font", "FiraCode Nerd Font Mono")
	if !strings.Contains(h.config(), "font-family = FiraCode Nerd Font\n") || !strings.Contains(h.m.status, "font-family now names it") {
		t.Fatalf("font-family should name what was installed (%q):\n%s", h.m.status, h.config())
	}
}

// A proportional family, or a missing one the table does not have: a few
// monospaced families are proposed, the installed ones first.
func TestAProportionalFontGetsMonospaceSuggestions(t *testing.T) {
	h := newHarness(t, "theme = Dracula\nfont-family = Helvetica\n")
	h.key("tab")
	if !strings.Contains(h.m.status, "Helvetica, is proportional") {
		t.Fatalf("the status line should say the font is proportional: %q", h.m.status)
	}
	h.m.jumpToGroup("Fonts")
	o := h.fixRow()
	if o == nil || o.label != "Choose a monospace font" || o.detail != "proportional" {
		t.Fatalf("the Fonts section should offer to choose another: %+v", o)
	}
	h.m.sel.cursor = 0
	h.key("enter")
	if h.m.cmd == nil || h.m.cmd.name != "font" {
		t.Fatalf("Enter should open the font list, on %v", h.m.prompt.Value())
	}
	var got []string
	for _, r := range h.m.results {
		if r.header && r.label == here() {
			break
		}
		if !r.header {
			got = append(got, r.label+" "+r.detail)
		}
	}
	want := []string{"Hack monospace · installed", "Google Sans Code download", "JetBrains Mono download"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("suggestions %q, want %q", got, want)
	}
	if s := h.m.selected(); s == nil || s.label != "Hack" {
		t.Fatalf("the list should open on the first suggestion, on %+v", s)
	}
	if h.config() != "theme = Dracula\nfont-family = Helvetica\n" {
		t.Fatalf("opening the list writes nothing:\n%s", h.config())
	}

	// Missing and not in the table: the same suggestions.
	h = newHarness(t, "theme = Dracula\nfont-family = Berkeley Mono\n")
	h.typeText("font ")
	if s := h.m.selected(); s == nil || s.label != "Hack" || !strings.Contains(h.view(), "Berkeley Mono is not") {
		t.Fatalf("a missing font the table lacks should get suggestions, on %+v:\n%s", s, h.view())
	}
}

// Nothing is said when nothing is wrong, nor for the family Ghostty
// carries inside itself.
func TestNoNoticeWhenTheFontIsFine(t *testing.T) {
	for _, config := range []string{"theme = Dracula\n", "theme = Dracula\nfont-family = Menlo\n", "theme = Dracula\nfont-family = JetBrains Mono\n"} {
		h := newHarness(t, config)
		h.key("tab")
		h.m.jumpToGroup("Fonts")
		if h.m.fontTrouble() != nil || h.fixRow() != nil || strings.Contains(h.m.status, "Your font") {
			t.Fatalf("%q: a notice for a font that is fine", config)
		}
		for _, c := range h.m.commands() {
			if c.name == "fixfont" {
				t.Fatalf("%q: the fix command outlived the trouble", config)
			}
		}
	}
}

// The panel says when the highlighted family has no Nerd Font icons, and
// knows a Nerd Font by its name.
func TestTheFontPanelSaysWhenIconsAreMissing(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.m.families = append(h.m.families, ghostty.Family{Name: "Hack Nerd Font", Styles: []string{"Regular"}, Mono: true})
	if has, known := h.m.nerdIconsOf(h.m.families[len(h.m.families)-1]); !has || !known {
		t.Fatal("a Nerd Font has its icons")
	}
	fakeFont(h, "Hack-Regular.ttf", &fontinfo.Info{})
	if note := h.m.iconsNote(h.m.families[0]); !strings.Contains(note, "No Nerd Font icons of its own") || !strings.Contains(note, "Symbols Nerd Font it carries") {
		t.Fatalf("a font with no icons should say where they come from: %q", note)
	}
}
