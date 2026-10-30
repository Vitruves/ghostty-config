package tui

import (
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// newGuidedHarness opens the editor in the Guided layout, in the picture
// interface when pixel is set, on a config of the test's own.
func newGuidedHarness(t *testing.T, config string, pixel bool, iface string) *harness {
	t.Helper()
	was := harnessProfile
	harnessProfile = ghostty.ProfileGuided
	defer func() { harnessProfile = was }()
	opts := Options{NoReload: true}
	if pixel {
		opts = Options{NoReload: true, Images: true, ImgOut: io.Discard, Pixel: true, PixCell: [2]int{17, 38}}
	}
	h := newHarnessWith(t, config, opts)
	if iface != "" {
		h.m.state.Interface = iface
	}
	h.send(tea.WindowSizeMsg{Width: 120, Height: 34})
	h.send(imgReadyMsg{h.m.imgSeq})
	return h
}

func (h *harness) guide() *guided {
	h.t.Helper()
	g, ok := h.m.profile().(*guided)
	if !ok {
		h.t.Fatalf("the profile in effect is %T", h.m.profile())
	}
	return g
}

// guidedShots draws every page of the guide, in a dark and a light scheme,
// at a usual window's size and at 80×24.
func guidedShots(t *testing.T) {
	for _, iface := range []string{"midnight", "paper"} {
		for _, size := range [][2]int{{120, 34}, {80, 24}} {
			h := newGuidedHarness(t, pixConfig, true, iface)
			h.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			suffix := ""
			if size[0] == 80 {
				suffix = "-80x24"
			}
			if iface == "paper" {
				suffix += "-light"
			}
			g := h.guide()
			for step := 0; step < gSteps; step++ {
				pages := g.pages(h.m, step)
				for page := range pages {
					g.goTo(h.m, step, page)
					h.shot("24-guided-" + itoa(step+1) + string(rune('a'+page)) + suffix)
				}
			}
		}
	}
}

// expect fails unless the config has every line.
func (h *harness) expect(what string, lines ...string) {
	h.t.Helper()
	cfg := h.config()
	for _, l := range lines {
		if !strings.Contains(cfg, l+"\n") {
			h.t.Fatalf("%s: the config does not say %q:\n%s", what, l, cfg)
		}
	}
}

func (h *harness) at(step, page int) {
	h.t.Helper()
	if g := h.guide(); g.step != step || g.page != page || g.away {
		h.t.Fatalf("the guide is at step %d page %d (away %v), want %d %d", g.step, g.page, g.away, step, page)
	}
}

// guideText is the page in words, whichever interface draws it.
func (h *harness) guideText() string {
	h.t.Helper()
	text := ansi.Strip(h.guide().viewCells(h.m))
	h.m.regions = h.m.regions[:0]
	return text
}

// walkGuide answers every question of the guide with something other than
// what the config says, and checks each page writes what it said it would.
func walkGuide(t *testing.T, h *harness) {
	t.Helper()
	h.at(gColours, 0)
	if !strings.Contains(h.guideText(), "Light or dark?") || !strings.Contains(h.guideText(), "Step 1 of 6") {
		t.Fatalf("the guide does not open on its first question:\n%s", h.guideText())
	}
	h.key("down", "up", "enter")
	h.at(gColours, 1)
	h.key("enter")
	h.at(gColours, 2)
	before := h.config()
	h.key("down")
	g := h.guide()
	pg, _ := g.at(h.m)
	chosen := pg.qs[0].choices[1].theme
	if h.m.cur.Name != chosen.Name || h.config() != before {
		t.Fatalf("choosing a card did not only show it: shown %s, config\n%s", h.m.cur.Name, h.config())
	}
	if !strings.Contains(h.guideText(), "Use "+chosen.Name+" →") {
		t.Fatalf("the button does not offer %s:\n%s", chosen.Name, h.guideText())
	}
	h.key("enter")
	h.expect("the theme", "theme = "+chosen.Name)
	h.at(gFont, 0)

	h.key("down", "enter")
	h.expect("the family", "font-family = Menlo")
	h.at(gFont, 1)
	h.key("down", "tab", "down")
	pg, _ = g.at(h.m)
	if v := g.willWrite(h.m, pg); v != "Will write font-size = 15 · font-feature = -calt, -liga, -dlig" {
		t.Fatalf("the page says %q", v)
	}
	if !strings.Contains(h.guideText(), "Will write font-size = 15") {
		t.Fatalf("the page does not say what it will write:\n%s", h.guideText())
	}
	h.key("enter")
	h.expect("size and ligatures", "font-size = 15", "font-feature = -calt, -liga, -dlig")
	h.at(gWindow, 0)

	h.key("down", "tab", "down", "enter")
	h.expect("the window", "background-opacity = 0.85", "window-padding-x = 12", "window-padding-y = 12")
	if strings.Contains(h.config(), "background-blur") {
		t.Fatalf("translucent wrote a blur:\n%s", h.config())
	}
	if runtime.GOOS == "darwin" {
		h.at(gWindow, 1)
		h.key("down", "enter")
		h.expect("the title bar", "macos-titlebar-style = native")
	}
	h.at(gCursor, 0)

	h.key("down", "tab", "up", "enter")
	h.expect("the cursor", "cursor-style = bar", "cursor-style-blink = false")
	h.at(gCopy, 0)

	h.key("down", "tab", "down", "enter")
	h.expect("copying", "copy-on-select = clipboard", "clipboard-trim-trailing-spaces = false")
	h.key("down", "enter")
	h.expect("pasting", "clipboard-paste-protection = false")
	h.key("down", "tab", "down", "enter")
	h.expect("the clipboard", "clipboard-read = allow", "clipboard-write = ask")
	h.at(gDone, 0)
	v := h.guideText()
	for _, want := range []string{"Changed this session", "cursor-style = bar", "font-family = Menlo", "Fine-tune everything"} {
		if !strings.Contains(v, want) {
			t.Fatalf("Done does not say %q:\n%s", want, v)
		}
	}
}

func TestGuidedWalksEveryStep(t *testing.T) {
	h := newGuidedHarness(t, "theme = Dracula\n", false, "")
	walkGuide(t, h)

	// Every line the guide wrote is one Ghostty takes, when it is installed.
	bin, err := exec.LookPath("ghostty")
	if err != nil {
		bin = "/Applications/Ghostty.app/Contents/MacOS/ghostty"
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skip("ghostty is not installed")
	}
	if !ghostty.BinaryVersion(bin).AtLeast(1, 4) {
		t.Skip("the installed ghostty is older than 1.4")
	}
	p := h.paths
	p.Binary = bin
	problems, err := ghostty.Validate(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ghostty +validate-config: %d problems, theme ones allowed: %+v", len(problems), problems)
	for _, pr := range problems {
		// The themes are the test's own, in a directory Ghostty does not
		// search: only that one may be missing.
		if pr.Key != "theme" && !strings.Contains(pr.Message, "theme") {
			t.Errorf("ghostty +validate-config: %+v\n%s", pr, h.config())
		}
	}
}

func TestGuidedWalksInThePicture(t *testing.T) {
	h := newGuidedHarness(t, "theme = Dracula\n", true, "paper")
	walkGuide(t, h)
}

// Back goes to the page before, Skip leaves a step as it is, Esc goes back
// a step and leaves from the first.
func TestGuidedBackSkipAndEsc(t *testing.T) {
	h := newGuidedHarness(t, "theme = Dracula\n", false, "")
	h.key("enter", "enter", "down")
	h.at(gColours, 2)
	shown := h.m.cur.Name
	h.key("s")
	h.at(gFont, 0)
	if h.config() != "theme = Dracula\n" || h.m.cur.Name != "Dracula" {
		t.Fatalf("skipping wrote or kept a theme only looked at (%s, was %s):\n%s", h.m.cur.Name, shown, h.config())
	}
	h.key("left")
	h.at(gColours, 2)
	h.key("backspace", "backspace")
	h.at(gColours, 0)
	h.key("left")
	h.at(gColours, 0)
	h.key("3")
	h.at(gWindow, 0)
	h.key("down", "s")
	h.at(gCursor, 0)
	if h.config() != "theme = Dracula\n" {
		t.Fatalf("skipping the window wrote:\n%s", h.config())
	}
	h.key("esc")
	h.at(gWindow, 0)
	h.key("esc", "esc")
	h.at(gColours, 0)
	if h.m.quitting {
		t.Fatal("Esc left before the first step")
	}
	h.key("esc")
	if !h.m.quitting {
		t.Fatal("Esc on the first step did not leave")
	}
}

// Ctrl+Z takes back a page in one step, and the page shows the config
// again.
func TestGuidedUndoTakesBackAPage(t *testing.T) {
	h := newGuidedHarness(t, "theme = Dracula\n", false, "")
	h.key("3", "down", "tab", "down", "enter")
	h.expect("the window", "background-opacity = 0.85", "window-padding-x = 12")
	h.key("ctrl+z")
	if cfg := h.config(); strings.Contains(cfg, "background-opacity = 0.85") || strings.Contains(cfg, "window-padding-x = 12") {
		t.Fatalf("Ctrl+Z did not take the page back:\n%s", cfg)
	}
	h.key("left")
	h.at(gWindow, 0)
	if !strings.Contains(h.view(), "Nothing to write") {
		t.Fatalf("after the undo the page does not show the config:\n%s", h.view())
	}
}

// Done hands over to the other layouts, and the guide is where it was left
// when it comes back.
func TestGuidedDoneSwitchesLayout(t *testing.T) {
	for i, want := range []string{ghostty.ProfileAtelier, ghostty.ProfilePalette} {
		h := newGuidedHarness(t, "theme = Dracula\n", false, "")
		h.key("6")
		h.at(gDone, 0)
		for j := 0; j < i; j++ {
			h.key("down")
		}
		h.key("enter")
		if h.m.state.Profile != want || ghostty.LoadState(h.paths).Profile != want {
			t.Fatalf("Done did not switch to %s: %q", want, h.m.state.Profile)
		}
		h.m.switchProfile(ghostty.ProfileGuided)
		h.at(gDone, 0)
	}
}

// See all and All … settings step aside into Atelier; Esc comes back.
func TestGuidedStepsAside(t *testing.T) {
	h := newGuidedHarness(t, "theme = Dracula\n", false, "")
	h.key("enter", "enter")
	h.key("/")
	if !h.guide().away || h.m.cmd == nil || h.m.cmd.name != "theme" {
		t.Fatal("See all did not open the wall of themes")
	}
	h.key("esc")
	h.at(gColours, 2)
	h.key("3", "a")
	if !h.guide().away || groups[h.m.group] != "Window" {
		t.Fatalf("All window settings did not open the Window section: %v", groups[h.m.group])
	}
	h.highlightCommand("opacity")
	if name, _ := h.m.profile().explainTarget(h.m); name != "background-opacity" {
		t.Fatalf("F3 reads %q beside the settings", name)
	}
	h.key("esc")
	h.at(gWindow, 0)
	if name, _ := h.m.profile().explainTarget(h.m); name != "background-opacity" {
		t.Fatalf("F3 reads %q on the window question", name)
	}
}

// The mouse picks an answer, goes on with the button, and jumps to a step.
func TestGuidedAnswersTheMouse(t *testing.T) {
	for _, pixel := range []bool{false, true} {
		h := newGuidedHarness(t, "theme = Dracula\n", pixel, "")
		r, ok := h.find(hitButton, 2, "g-step")
		if !ok {
			t.Fatal("the steps are not clickable")
		}
		h.click(r.x, r.y)
		h.at(gWindow, 0)
		r, ok = h.find(hitButton, 1, "g-q-0")
		if !ok {
			t.Fatal("the answers are not clickable")
		}
		h.click(r.x+1, r.y)
		r, ok = h.find(hitButton, 0, "g-next")
		if !ok {
			t.Fatal("the way on is not clickable")
		}
		h.click(r.x+1, r.y)
		h.expect("a click", "background-opacity = 0.85")
	}
}

// Every page draws, fills the window and fits it, in both interfaces, at
// a usual size and at 80×24.
func TestGuidedFitsEveryWindow(t *testing.T) {
	for _, pixel := range []bool{false, true} {
		for _, iface := range []string{"midnight", "paper"} {
			h := newGuidedHarness(t, pixConfig, pixel, iface)
			for _, size := range [][2]int{{120, 34}, {80, 24}} {
				h.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				h.send(imgReadyMsg{h.m.imgSeq})
				g := h.guide()
				for step := 0; step < gSteps; step++ {
					for page := range g.pages(h.m, step) {
						g.goTo(h.m, step, page)
						if pixel {
							h.shot("guided")
							continue
						}
						v := h.view()
						lines := strings.Split(v, "\n")
						if len(lines) != size[1] {
							t.Fatalf("step %d page %d: %d rows at %v", step, page, len(lines), size)
						}
						for i, l := range lines {
							if w := ansi.StringWidth(l); w > size[0] {
								t.Fatalf("step %d page %d: row %d is %d wide at %v", step, page, i, w, size)
							}
						}
						if pg, pages := g.at(h.m); !strings.Contains(v, strings.TrimSuffix(g.primary(h.m, pg, pages), " →")) {
							t.Fatalf("step %d page %d at %v: the way on is cut off:\n%s", step, page, size, v)
						}
					}
				}
			}
		}
	}
}
