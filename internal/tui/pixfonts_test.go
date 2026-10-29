package tui

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/fontinfo"
	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// newPixHarnessWith is the picture interface over a config of the test's own.
func newPixHarnessWith(t *testing.T, iface, config string) *harness {
	t.Helper()
	h := newHarnessWith(t, config, Options{NoReload: true, Images: true, ImgOut: io.Discard, Pixel: true, PixCell: [2]int{17, 38}})
	h.m.state.Interface = iface
	h.send(tea.WindowSizeMsg{Width: 120, Height: 34})
	return h
}

// highlightOption puts the highlight on the line with this label.
func (h *harness) highlightOption(label string) {
	h.t.Helper()
	for i, o := range h.m.results {
		if !o.header && o.label == label {
			h.m.sel.cursor = i
			h.m.afterMove()
			return
		}
	}
	h.t.Fatalf("no line %q in the list", label)
}

// The pictures of the font list, the notices and the icons download, for
// looking at: GHOSTTY_CONFIG_PXSHOTS.
func TestPixelFontScreens(t *testing.T) {
	fakeInstall(t)
	dir := filepath.Join("..", "kimg", "fonts")
	specimen := map[string]ghostty.FontFile{
		"Regular": {Path: filepath.Join(dir, "sans-regular.ttf")},
		"Bold":    {Path: filepath.Join(dir, "sans-bold.ttf")},
	}

	// The long list: the families on this machine, then the popular ones to
	// install, then the rest A to Z.
	h := newPixHarness(t, "midnight")
	h.m.setPromptText("Font ")
	h.highlightOption("Terminus")
	h.shot("40-font-list-popular")
	h.highlightOption("Maple Mono")
	h.shot("41-font-list-more")
	h.m.setPromptText("Font mono")
	h.shot("42-font-list-typed")

	// A family with no icons of its own, in its specimen.
	h.m.families[0].Files = specimen
	h.m.fx.infos = map[string]fontInfo{fileKey(specimen["Regular"]): {info: &fontinfo.Info{}}}
	h.m.setPromptText("Font ")
	h.highlightOption("Hack")
	if load := h.m.wantSpecimen(); load != nil {
		h.send(load())
	}
	h.shot("43-font-specimen-no-icons")

	// The configured family is missing and the table has it.
	h = newPixHarnessWith(t, "midnight", pixConfig+"font-family = JetBrainsMono Nerd Font\n")
	h.key("tab")
	if !strings.Contains(h.m.status, "Your font") {
		t.Fatalf("no hint on the status line: %q", h.m.status)
	}
	h.shot("44-status-missing-font")
	h.m.jumpToGroup("Fonts")
	h.shot("45-fonts-missing")
	h.m.setPromptText("Font ")
	h.shot("46-font-list-missing")

	// The configured family is proportional.
	h = newPixHarnessWith(t, "midnight", pixConfig+"font-family = Helvetica\n")
	h.m.families[0].Files = specimen
	h.m.jumpToGroup("Fonts")
	h.shot("47-fonts-proportional")
	h.m.sel.cursor = 0
	h.key("enter")
	if load := h.m.wantSpecimen(); load != nil {
		h.send(load())
	}
	h.shot("48-font-list-suggestions")

	// The icons: on downloads the family, behind the progress line.
	h = newPixHarness(t, "midnight")
	h.m.setPromptText("Icons ")
	h.highlightOption("on")
	h.shot("49-icons-on-missing")
	h.m.Update(keyPress("enter"))
	if h.m.overlay != overlayBusy {
		t.Fatal("on did not start the download")
	}
	h.shot("50-icons-downloading")

	light := newPixHarnessWith(t, "paper", pixConfig+"font-family = Fira Code\n")
	light.m.jumpToGroup("Fonts")
	light.shot("51-fonts-missing-light")
	light.m.setPromptText("Font ")
	light.highlightOption("Gohu")
	light.shot("52-font-list-light")
}
