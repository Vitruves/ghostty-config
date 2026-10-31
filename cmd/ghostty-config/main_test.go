package main

import (
	"testing"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// A flag given wins over the preference kept; without one, the preference
// decides.
func TestMergeFlagsOverPrefs(t *testing.T) {
	no := false
	kept := &ghostty.State{Drawing: "text", Plain: true, UpdateCheck: &no}

	opts, cells := merge(flags{}, kept, true)
	if opts.Images || cells || !opts.Plain || !opts.NoUpdateCheck || len(opts.Locked) != 0 {
		t.Fatalf("without flags the kept preferences decide: %+v cells %v", opts, cells)
	}

	opts, cells = merge(flags{cells: true}, kept, true)
	if !opts.Images || !cells || opts.Locked["drawing"].Flag != "-cells" || opts.Locked["drawing"].Value != "cells" {
		t.Fatalf("-cells should win over a kept text only: %+v cells %v", opts, cells)
	}
	opts, _ = merge(flags{images: true}, kept, false)
	if !opts.Images || opts.Locked["drawing"].Value != "images" {
		t.Fatalf("-images should win, even where pictures are not known: %+v", opts)
	}
	opts, _ = merge(flags{noImages: true, images: true}, &ghostty.State{Drawing: "images"}, true)
	if opts.Images || opts.Locked["drawing"].Flag != "-no-images" {
		t.Fatalf("-no-images should win over everything: %+v", opts)
	}

	// The kept drawings, each without a flag.
	for drawing, want := range map[string][2]bool{"": {true, false}, "auto": {true, false}, "images": {true, false}, "cells": {true, true}, "text": {false, false}} {
		opts, cells := merge(flags{}, &ghostty.State{Drawing: drawing}, true)
		if opts.Images != want[0] || cells != want[1] {
			t.Errorf("drawing %q: images %v cells %v, want %v", drawing, opts.Images, cells, want)
		}
	}
	if opts, _ := merge(flags{}, &ghostty.State{Drawing: "images"}, false); !opts.Images {
		t.Error("a kept images should draw pictures where the terminal is not known to")
	}
	if opts, _ := merge(flags{}, &ghostty.State{}, false); opts.Images {
		t.Error("auto should draw no pictures where the terminal is not known to")
	}

	// Glyphs, the update check and reload.
	opts, _ = merge(flags{plain: true, noUpdateCheck: true, noReload: true}, &ghostty.State{}, true)
	if !opts.Plain || !opts.NoUpdateCheck || !opts.NoReload {
		t.Fatalf("the flags should hold: %+v", opts)
	}
	for name, flag := range map[string]string{"glyphs": "-plain", "updatecheck": "-no-update-check", "autoreload": "-no-reload"} {
		if opts.Locked[name].Flag != flag {
			t.Errorf("%s should be locked by %s: %+v", name, flag, opts.Locked[name])
		}
	}
	if opts, _ := merge(flags{}, &ghostty.State{}, true); opts.Plain || opts.NoUpdateCheck {
		t.Fatalf("the defaults: %+v", opts)
	}
}
