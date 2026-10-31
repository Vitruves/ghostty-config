// Command ghostty-config is an interactive editor for Ghostty: themes, the
// palette behind them, fonts, the window and what Ghostty can be extended
// with, on one full-screen interface that takes the terminal's own colours.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/collection"
	"github.com/vitruves/ghostty-config/internal/ghostty"
	"github.com/vitruves/ghostty-config/internal/tui"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "1.2.0"

func main() {
	configPath := flag.String("config", "", "Ghostty config file to edit instead of the default ones")
	themesPath := flag.String("themes", "", "User themes directory (default: ~/.config/ghostty/themes)")
	exportPath := flag.String("export-collection", "", "Write the curated collection to a directory and exit")
	noReload := flag.Bool("no-reload", false, "Never ask the running Ghostty to reload")
	noImages := flag.Bool("no-images", false, "Never draw pictures: the wall of themes is made of cells")
	cellsOnly := flag.Bool("cells", false, "Draw the interface from cells, with pictures only for the cards, instead of as one picture")
	forceImages := flag.Bool("images", false, "Draw pictures through the Kitty graphics protocol even where this terminal is not known to support it")
	plain := flag.Bool("plain", false, "Use only glyphs every terminal and font can draw")
	showVersion := flag.Bool("version", false, "Show version information")
	listPaths := flag.Bool("paths", false, "Print the files this tool would read and write, then exit")
	noUpdateCheck := flag.Bool("no-update-check", false, "Never ask GitHub whether a newer release exists")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `ghostty-config — themes, fonts and window settings for Ghostty, edited live

Usage:
  ghostty-config [options]

Options:
  -config file           Edit this config file instead of Ghostty's defaults
  -themes dir            User themes directory (default ~/.config/ghostty/themes)
  -export-collection dir Write the %d curated themes to a directory and exit
  -no-reload             Never send the reload keystroke to Ghostty
  -cells                 Draw the interface from cells, with pictures only for the cards
  -no-images             Never draw pictures: the wall of themes is made of cells
  -images                Draw pictures even where this terminal is not known to show them
  -plain                 Use only glyphs every terminal and font can draw
  -paths                 Show which files are read and written
  -no-update-check       Never look for a newer release on GitHub
  -version               Show version

What -cells, -no-images, -images, -plain and -no-update-check say can be kept
for every launch in F4, ghostty-config's own settings; a flag given wins.

Ghostty's config files are read in the order Ghostty reads them — config,
config.ghostty, then on macOS the Application Support copies, then every
config-file include — and a setting is rewritten in whichever file holds the
value that wins. Nothing else in those files is touched; every rewrite is
atomic and backed up under ~/.config/ghostty-config/backups.

Keys:
  Tab next section       ←↓↑→ walk the wall of themes   Enter apply, or open
  type to search or run  ←→ change a setting in place    Esc back, then leave
  Ctrl+T try a theme     Delete put a setting back      F2 peek   F1 all keys
`, len(collection.Collection))
	}
	flag.Parse()

	if *showVersion {
		fmt.Printf("ghostty-config %s\n", version)
		return
	}

	if *exportPath != "" {
		written, skipped, err := collection.Install(*exportPath)
		if err != nil {
			fail("Export failed: %v", err)
		}
		fmt.Printf("Wrote %d themes to %s\n", written, *exportPath)
		if skipped > 0 {
			fmt.Printf("Left %d edited file(s) untouched\n", skipped)
		}
		return
	}

	paths := ghostty.DefaultPaths(*configPath, *themesPath)
	tree, err := ghostty.Load(paths)
	if err != nil {
		fail("Could not read the Ghostty configuration: %v", err)
	}

	if *listPaths {
		fmt.Println("Config files, in load order (later wins):")
		for _, d := range tree.Docs {
			fmt.Println("  " + d.Path)
		}
		fmt.Println("New keys are appended to:\n  " + tree.Primary.Path)
		fmt.Println("User themes:\n  " + paths.ThemesDir)
		fmt.Println("Bundled themes:")
		for _, d := range paths.ResourceThemeDirs {
			fmt.Println("  " + d)
		}
		fmt.Println("Favourites, settings and backups:\n  " + paths.StateDir)
		if paths.Binary != "" {
			fmt.Println("Ghostty binary:\n  " + paths.Binary)
		}
		return
	}

	// What only programs can tell is asked now, and answered while the
	// themes are read.
	ghostty.Prefetch(paths.Binary)

	// The curated collection ships inside the binary, so a newer build
	// brings new palettes. Refresh on every start; files the user edited are
	// left alone, and a failure here is not worth stopping for.
	if written, _, err := collection.Install(paths.ThemesDir); err != nil {
		fmt.Fprintf(os.Stderr, "Could not install the curated collection: %v\n", err)
	} else if written > 0 && !ghostty.LoadState(paths).Welcomed {
		fmt.Printf("Installed %d curated themes into %s\n", written, paths.ThemesDir)
	}

	// The flags given win over the preferences kept, which F4 edits.
	given := flags{noReload: *noReload, noImages: *noImages, cells: *cellsOnly, images: *forceImages, plain: *plain, noUpdateCheck: *noUpdateCheck}
	state := ghostty.LoadState(paths)
	opts, cells := merge(given, state, tui.ImagesSupported())
	opts.Version = version

	// The picture interface needs the exact size of a cell, which only the
	// terminal can tell, so it is asked before anything reads the keyboard.
	// The terminal answers while the themes are read; nothing is printed
	// until it has, for the terminal is in raw mode meanwhile.
	type cell struct {
		w, h int
		ok   bool
	}
	var probe chan cell
	if opts.Images && !cells {
		probe = make(chan cell, 1)
		go func() {
			w, h, ok := tui.ProbeCell()
			probe <- cell{w, h, ok}
		}()
	}

	lib := ghostty.LoadLibrary(paths)
	if probe != nil {
		if c := <-probe; c.ok {
			opts.Pixel = true
			opts.PixCell = [2]int{c.w, c.h}
		}
	}
	if len(lib.Themes) == 0 {
		fail("No themes found. Ghostty ships hundreds under its resources directory; is Ghostty installed? (looked in %v)", paths.ResourceThemeDirs)
	}
	model := tui.New(paths, tree, lib, state, opts)
	// The alternate screen and the mouse are asked for by the view itself.
	program := tea.NewProgram(model)
	if _, err := program.Run(); err != nil {
		// However it ended, a theme painted into the terminal for a try must
		// not outlive the editor.
		model.RestoreColours()
		// Interrupted from outside (a signal, a kill): leave quietly, the
		// way a shell expects, writing nothing more.
		if errors.Is(err, tea.ErrInterrupted) || errors.Is(err, tea.ErrProgramKilled) {
			fmt.Print("\x1b_Ga=d,d=A,q=2\x1b\\")
			os.Exit(130)
		}
		fail("%v", err)
	}
	if opts.Images {
		// Free the pictures the terminal still holds.
		fmt.Print("\x1b_Ga=d,d=A,q=2\x1b\\")
	}
	if note := model.ExitNote(); note != "" {
		fmt.Println(note)
	}
}

// flags are what the command line said of what the state file keeps too.
type flags struct {
	noReload, noImages, cells, images, plain, noUpdateCheck bool
}

// merge lays the flags given over the preferences kept: a flag wins for the
// session, and the editor is told which so that F4 shows it as the flag set
// it; without one, what was kept decides. supported is whether the terminal
// is known to show pictures; cells is whether the interface is drawn from
// cells even where it could be one picture.
func merge(f flags, s *ghostty.State, supported bool) (opts tui.Options, cells bool) {
	locked := map[string]tui.Lock{}
	lock := func(name, flag, value string) { locked[name] = tui.Lock{Flag: flag, Value: value} }
	noImages, force := f.noImages, f.images
	cells = f.cells
	switch {
	case f.noImages:
		lock("drawing", "-no-images", "text")
	case f.cells:
		lock("drawing", "-cells", "cells")
	case f.images:
		lock("drawing", "-images", "images")
	default:
		switch s.Drawing {
		case "images":
			force = true
		case "cells":
			cells = true
		case "text":
			noImages = true
		}
	}
	if f.plain {
		lock("glyphs", "-plain", "on")
	}
	if f.noUpdateCheck {
		lock("updatecheck", "-no-update-check", "off")
	}
	if f.noReload {
		lock("autoreload", "-no-reload", "off")
	}
	opts = tui.Options{
		NoReload: f.noReload, Plain: f.plain || s.Plain,
		Images:        !noImages && (force || supported),
		NoUpdateCheck: f.noUpdateCheck || !s.UpdateCheckEnabled(),
		Locked:        locked,
	}
	return opts, cells
}

func fail(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "✗ "+format+"\n", a...)
	os.Exit(1)
}
