// Command ghostty-config is an interactive editor for Ghostty: themes, the
// palette behind them, fonts and the window, all previewed in the terminal
// you are sitting in.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vitruves/ghostty-config/internal/collection"
	"github.com/vitruves/ghostty-config/internal/ghostty"
	"github.com/vitruves/ghostty-config/internal/tui"
)

const version = "1.0.0"

func main() {
	configPath := flag.String("config", "", "Ghostty config file to edit instead of the default ones")
	themesPath := flag.String("themes", "", "User themes directory (default: ~/.config/ghostty/themes)")
	exportPath := flag.String("export-collection", "", "Write the curated collection to a directory and exit")
	noReload := flag.Bool("no-reload", false, "Never ask the running Ghostty to reload")
	plain := flag.Bool("plain", false, "Use only glyphs every terminal and font can draw")
	showVersion := flag.Bool("version", false, "Show version information")
	listPaths := flag.Bool("paths", false, "Print the files this tool would read and write, then exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `ghostty-config — themes, fonts and window settings for Ghostty, edited live

Usage:
  ghostty-config [options]

Options:
  -config file           Edit this config file instead of Ghostty's defaults
  -themes dir            User themes directory (default ~/.config/ghostty/themes)
  -export-collection dir Write the %d curated themes to a directory and exit
  -no-reload             Never send the reload keystroke to Ghostty
  -plain                 Use only glyphs every terminal and font can draw
  -paths                 Show which files are read and written
  -version               Show version

Ghostty's config files are read in the order Ghostty reads them — config,
config.ghostty, then on macOS the Application Support copies, then every
config-file include — and a setting is rewritten in whichever file holds the
value that wins. Nothing else in those files is touched; every rewrite is
atomic and backed up under ~/.config/ghostty-config/backups.

Keys:
  ↑↓ browse (previews live)   a apply   Tab palette   f fonts   p settings
  n create   g random   / search   * favourite   ? all keys   q quit
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

	// The curated collection ships inside the binary, so a newer build
	// brings new palettes. Refresh on every start; files the user edited are
	// left alone, and a failure here is not worth stopping for.
	if written, _, err := collection.Install(paths.ThemesDir); err != nil {
		fmt.Fprintf(os.Stderr, "Could not install the curated collection: %v\n", err)
	} else if written > 0 && !ghostty.LoadState(paths).Welcomed {
		fmt.Printf("Installed %d curated themes into %s\n", written, paths.ThemesDir)
	}

	lib := ghostty.LoadLibrary(paths)
	if len(lib.Themes) == 0 {
		fail("No themes found. Ghostty ships hundreds under its resources directory; is Ghostty installed? (looked in %v)", paths.ResourceThemeDirs)
	}
	state := ghostty.LoadState(paths)

	model := tui.New(paths, tree, lib, state, tui.Options{NoReload: *noReload, Plain: *plain})
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		fail("%v", err)
	}
	if note := model.ExitNote(); note != "" {
		fmt.Println(note)
	}
}

func fail(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "✗ "+format+"\n", a...)
	os.Exit(1)
}
