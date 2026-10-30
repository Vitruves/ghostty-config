package tui

import (
	"bytes"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// newChooserHarness opens the editor on a state with no layout chosen: a
// new user's, or, when returning, that of someone who used the tool before
// layouts were offered.
func newChooserHarness(t *testing.T, returning bool, opts Options) *harness {
	t.Helper()
	was := harnessProfile
	harnessProfile = ""
	defer func() { harnessProfile = was }()
	h := newHarnessWith(t, "theme = Dracula\n", opts)
	h.m.state.Welcomed = returning
	return h
}

func TestChooserIsAskedUntilALayoutIsChosen(t *testing.T) {
	h := newChooserHarness(t, false, Options{NoReload: true})
	if h.m.overlay != overlayLayout {
		t.Fatalf("a new user did not see the chooser: overlay %d", h.m.overlay)
	}
	v := h.view()
	for _, want := range []string{"How do you want to work?", "Three layouts", "Atelier", "Palette", "Guided", "default", "Tool → Interface → Layout"} {
		if !strings.Contains(v, want) {
			t.Errorf("the chooser does not say %q:\n%s", want, v)
		}
	}
	if profileKinds[h.m.layoutAt].name != ghostty.ProfileAtelier {
		t.Fatalf("Atelier is not preselected: %d", h.m.layoutAt)
	}
	h.dump("chooser-new")
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	lines := strings.Split(h.view(), "\n")
	if len(lines) != 24 || !strings.Contains(h.view(), "Continue") || !strings.Contains(h.view(), "How do you want to work?") {
		t.Fatalf("the chooser does not fit 80×24:\n%s", h.view())
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 80 {
			t.Fatalf("row %d is %d wide at 80×24", i, w)
		}
	}
	h.dump("chooser-new-80x24")

	// Nothing but a choice goes on: not Esc, not q, not a stray letter.
	for _, k := range []string{"esc", "q", "x", "f2", "f3", "ctrl+z"} {
		h.key(k)
		if h.m.overlay != overlayLayout {
			t.Fatalf("%s skipped the chooser", k)
		}
	}
	if h.m.state.Profile != "" {
		t.Fatalf("a layout was kept without Enter: %q", h.m.state.Profile)
	}

	// A new user choosing Atelier goes on to its welcome.
	h.key("right", "left", "enter")
	if h.m.state.Profile != ghostty.ProfileAtelier {
		t.Fatalf("Enter did not keep Atelier: %q", h.m.state.Profile)
	}
	if got := ghostty.LoadState(h.paths).Profile; got != ghostty.ProfileAtelier {
		t.Fatalf("the layout was not saved: %q", got)
	}
	if h.m.overlay != overlayWelcome {
		t.Fatalf("the welcome did not follow the chooser: overlay %d", h.m.overlay)
	}
}

func TestChooserForSomeoneReturning(t *testing.T) {
	h := newChooserHarness(t, true, Options{NoReload: true, Version: "v1.5.0"})
	if h.m.overlay != overlayLayout {
		t.Fatal("someone returning without a layout did not see the chooser")
	}
	v := h.view()
	for _, want := range []string{"two new ways", "Atelier is the one you had", "current", "updated to v1.5.0"} {
		if !strings.Contains(v, want) {
			t.Errorf("the chooser does not say %q:\n%s", want, v)
		}
	}
	h.dump("chooser-returning")
	h.key("right", "enter")
	if h.m.state.Profile != ghostty.ProfilePalette {
		t.Fatalf("→ Enter did not keep Palette: %q", h.m.state.Profile)
	}
	if h.m.overlay != overlayNone {
		t.Fatalf("someone returning was shown another screen: overlay %d", h.m.overlay)
	}
}

func TestChooserIsNotAskedOnceChosen(t *testing.T) {
	for _, name := range []string{ghostty.ProfileAtelier, ghostty.ProfilePalette, ghostty.ProfileGuided} {
		was := harnessProfile
		harnessProfile = name
		h := newHarness(t, "theme = Dracula\n")
		harnessProfile = was
		if h.m.overlay != overlayNone {
			t.Fatalf("%s: the chooser or another screen opened: overlay %d", name, h.m.overlay)
		}
	}
	// A word this build does not know is no choice.
	s := &ghostty.State{Profile: "fichier"}
	if s.ProfileChosen() || s.ActiveProfile() != ghostty.ProfileAtelier {
		t.Fatal("an unknown layout counts as chosen")
	}
}

// Ctrl+C leaves from the chooser as from anywhere.
func TestChooserLetsCtrlCQuit(t *testing.T) {
	h := newChooserHarness(t, false, Options{NoReload: true})
	h.key("ctrl+c")
	if !h.m.quitting {
		t.Fatal("Ctrl+C did not leave the chooser")
	}
}

// A click picks a layout, a second click on it goes on.
func TestChooserAnswersTheMouse(t *testing.T) {
	h := newChooserHarness(t, true, Options{NoReload: true})
	h.view()
	r, ok := h.find(hitButton, 2, "layout-2")
	if !ok {
		t.Fatal("the Guided column is not clickable")
	}
	h.click(r.x+1, r.y+1)
	if h.m.layoutAt != 2 || h.m.overlay != overlayLayout {
		t.Fatalf("a click did not pick Guided: at %d, overlay %d", h.m.layoutAt, h.m.overlay)
	}
	h.view()
	h.click(r.x+1, r.y+1)
	if h.m.state.Profile != ghostty.ProfileGuided {
		t.Fatalf("a second click did not keep Guided: %q", h.m.state.Profile)
	}
}

// Tool → Interface → Layout switches at any time, and only Enter does.
func TestLayoutSwitches(t *testing.T) {
	h := newHarness(t, "theme = Dracula\n")
	h.m.jumpToGroup("Tool")
	h.highlightCommand("layout")
	h.do("layout ", false)
	h.key("down")
	if h.m.state.Profile != ghostty.ProfileAtelier {
		t.Fatalf("walking the layouts switched to %q", h.m.state.Profile)
	}
	h.do("layout guided", true)
	if h.m.state.Profile != ghostty.ProfileGuided || ghostty.LoadState(h.paths).Profile != ghostty.ProfileGuided {
		t.Fatalf("Layout guided did not switch and save: %q", h.m.state.Profile)
	}
	if _, ok := h.m.profile().(*guided); !ok {
		t.Fatalf("the profile in effect is %T", h.m.profile())
	}
	h.do("layout atelier", true)
	if _, ok := h.m.profile().(atelier); !ok || h.m.state.Profile != ghostty.ProfileAtelier {
		t.Fatalf("back to Atelier: %T, %q", h.m.profile(), h.m.state.Profile)
	}
	var labels []string
	for _, o := range h.command("layout").options(h.m, "") {
		labels = append(labels, o.label)
	}
	if strings.Join(labels, " ") != "Atelier Palette Guided" {
		t.Fatalf("the layouts are listed as %v", labels)
	}
}

// Switching layouts throws away every picture the terminal holds and sends
// the whole screen again, once the screen is ready for it.
func TestLayoutSwitchSendsTheScreenAgain(t *testing.T) {
	var out bytes.Buffer
	h := newHarnessWith(t, pixConfig, Options{NoReload: true, Images: true, ImgOut: &out, Pixel: true, PixCell: [2]int{17, 38}})
	h.m.View()
	first := strings.Count(out.String(), "a=T")
	if first == 0 {
		t.Fatal("the first frame sent no picture")
	}
	h.m.px.pics["x"] = nil
	out.Reset()
	h.m.switchProfile(ghostty.ProfilePalette)
	if h.m.imgReady {
		t.Fatal("pictures would be sent before the screen is sent again")
	}
	if len(h.m.px.pics) != 0 {
		t.Fatal("the pictures drawn for the last layout were kept")
	}
	h.m.View()
	if strings.Contains(out.String(), "a=T") {
		t.Fatal("a picture was sent while the screen was being replaced")
	}
	h.send(imgReadyMsg{h.m.imgSeq})
	h.m.View()
	if !strings.Contains(out.String(), "a=d") {
		t.Fatal("the pictures of the last layout were not freed")
	}
	// Every row of the new layout's frame, as many as that frame sent fresh.
	n := strings.Count(out.String(), "a=T")
	out.Reset()
	h.m.imgs.reset()
	h.m.View()
	if all := strings.Count(out.String(), "a=T"); n != all {
		t.Fatalf("after a switch %d pictures were sent, want all %d of the frame", n, all)
	}
}

// Until Palette and Guided are written they stand on Atelier: any layout a
// state file names gives an editor that draws, answers keys and the mouse,
// in both interfaces.
func TestEveryProfileWorks(t *testing.T) {
	for _, k := range profileKinds {
		for _, pixel := range []bool{false, true} {
			was := harnessProfile
			harnessProfile = k.name
			opts := Options{NoReload: true}
			if pixel {
				opts = Options{NoReload: true, Images: true, ImgOut: io.Discard, Pixel: true, PixCell: [2]int{17, 38}}
			}
			h := newHarnessWith(t, pixConfig, opts)
			harnessProfile = was
			for _, size := range [][2]int{{120, 34}, {80, 24}} {
				h.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				h.send(imgReadyMsg{h.m.imgSeq})
				for _, key := range []string{"down", "right", "tab", "f3", "esc", "ctrl+z"} {
					if key == "esc" && h.m.overlay == overlayNone {
						// F3 found no page here: Esc would leave.
						continue
					}
					h.key(key)
					if pixel {
						h.shot("profile-" + k.name)
					} else if h.view() == "" {
						t.Fatalf("%s: nothing drawn after %s", k.name, key)
					}
				}
				h.click(10, 10)
				h.send(tea.MouseWheelMsg{X: 10, Y: 10, Button: tea.MouseWheelDown})
			}
		}
	}
}
