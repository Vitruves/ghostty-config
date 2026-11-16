# Changelog

All notable changes to ghostty-config are documented here. Versions follow [Semantic Versioning](https://semver.org).

## [Unreleased]

## [2.4.0] - 2026-11-16

### Added

- **About a hundred more Ghostty options**, 212 in all on macOS with Ghostty 1.4 (116 before), each with the control its shape calls for and hidden where the platform or the Ghostty version does not have it:
  - Fonts: bold and italic families and styles, synthetic styles, bold colour, a Metrics part (baseline, underline, strikethrough, overline, box drawing), icon height.
  - Window: title and its font, window buttons, full screen, maximise, new-tab position, split fill, divider colour, search colours, generated palette, shader animation; the GTK title bar, toolbar and tab settings on Linux.
  - Input: right click, scroll speed for wheel and trackpad, click interval, mouse reporting, Shift capture, cursor thickness and height, a Selection part (word characters, clear on copy), link previews, bracketed-paste safety.
  - Session: a Startup part (command, first command, working directory, first window, wait after command), what new tabs and splits inherit, undo timeout, resize overlay position and time, scroll to bottom, notification delay and actions, progress style, image memory; Updates and a macOS part (icon, Dock drop, secure input, shortcuts, AppleScript); a Linux part (single instance, class, cgroups); an Advanced part (TERM, ENQ answer, colour report format, checksum reports, grapheme width, alpha blending, vsync, FreeType hinting).
  - Text and colour options take what you type, checked before it is written (a colour must be hex, a directory must exist), under suggestions: the theme's colours, your shells, installed families.
  - Left for the file itself: `env`, `input`, `key-remap`, `command-palette-entry`, `clipboard-codepoint-map`, `gtk-custom-css` (lists of lines), and `font-variation-bold`/`-italic`/`-bold-italic`.
- **Extensions → Programs:** command-line tools that put Ghostty to use, installed in one Enter after a yes, and started with your shell.
  - Shell: starship, zoxide, fzf, atuin, carapace, and lig (IDE-style completion; it sets itself up with `lig install`, and keeps Ghostty's shell integration since 1.2.2).
  - Images in the terminal, through the Kitty graphics protocol: yazi, superfile, presenterm, fastfetch, kitten (`kitten icat`), chafa, viu, tdf (PDFs).
  - Clickable links (OSC 8): ripgrep (`rg` alias with `--hyperlink-format=default`), eza (`ll` alias with `--hyperlink`), fd (`fd` alias with `--hyperlink=auto`), delta (`GIT_PAGER='delta --hyperlinks'`).
  - Tools: bat, glow, lazygit.
  - System: btop, bottom, dust, duf, procs, hyperfine.
  - Search: television, jq, fx, tealdeer, navi (Ctrl+G widget, with fzf).
  - Editors: neovim (Kitty keyboard, OSC 52, OSC 8 and Ghostty's progress bar since 0.12), helix, micro. neovim and helix come from Homebrew, their releases holding runtime files beside the program.
  - Sessions: zmx, built on libghostty-vt; zellij.
  - The list shows one group at a time under a row of tabs (Shell, Images, Links, Tools, System, Search, Editors, Sessions): `←` `→` or a click turns them, the row scrolls to keep the open one in view, and typing searches every group. Acting on a program opens its group, so it stays highlighted after Enter. Palette lists every group, as it searches what it lists.
  - Where a project publishes no build for the system (eza and btop on macOS, delta on Intel Macs, chafa and tdf anywhere), the program comes from Homebrew, and the list says "needs Homebrew" when there is none.
  - GitHub's limit of 60 API questions an hour without an account is said in words, with the time it lifts.
  - Installed with Homebrew when it is there and the program is in homebrew/core; otherwise from the project's latest GitHub release into `~/.local/bin`, refused unless the file matches the SHA-256 GitHub gives for it. Never as root.
  - The line that starts a program goes in a marked block of `~/.zshrc`, `~/.bashrc` (`~/.bash_profile` on macOS) or `conf.d/ghostty-config.fish`, backed up before its first change and placed above lig's closing line. Switching a program off takes its line out and leaves the program installed; a line of your own that already starts it is left alone.
  - `Ctrl+X` uninstalls a program after a yes, in Atelier and Palette: its line leaves the block, then `brew uninstall`, or its executables are deleted from `~/.local/bin`; lig runs `lig uninstall` first, while it is still there. A program installed by anything else (apt, cargo, its own installer) is left alone, and a line of your own that would start a missing program is pointed out.
- **Shaders:** cursor_tail, cursor_warp, cursor_sweep, ripple_cursor and sonic_boom_cursor from sahaj-b/cursor-shaders, wisp-cursor from hced/ghostty-cursor-trails, blackhole from s0xDk/ghostty-blackhole (all MIT).
- **Theme packs** not shipped with Ghostty: cyberdream, onedarkpro, nightfly, vscode, oasis (16 dark palettes), all MIT.

### Changed

- **Settings, reordered.** The parts of each section come in the order people reach for them, and the settings of a part in a fixed order of their own: Window opens on Appearance (opacity, blur) before the title bar and padding; Input has Cursor (style, blink, opacity) after Keyboard, and "Click to move the cursor" sits with the mouse; Session goes Windows, Scrollback, Notifications, Bell, Quick terminal, Shell integration, Programs. An open setting lists its neighbours by name, in the same order and groups, instead of by command word.
- **Settings answer the pointer** in the picture interface. What the pointer is over lights up (lines, pills, switches, sections, the column on the left). Sliders are pulled: the value and the window preview follow the pointer, and the config is written once, when the button is let go. The arrows beside a value too long for pills are clickable. The mark of a changed setting becomes an arrow that puts Ghostty's default back. "Changed" in the left column walks the changed settings, and a dot marks the sections that hold some.
- An open number has the whole scale to pull above its steps.
- The window preview shows the cursor in its style and opacity, and comes with the cursor settings too.
- **Tool tab and F4 settings:** ghostty-config's own settings show real controls (pills, switches, colour dots for the accent), set with a click or `←` `→`, with values in display case and a panel that says what each value does.
- **Actions say what they do.** A line of the menu that opens something (Undo, Changes, Check, Restore, Preset, Keys, Shader…) carries a short description under its name; opened, its full description sits under the title on two lines instead of being cut, and the title, the description and the lines share one left edge.
- **Faster window preview.** The picture of the window beside a Window setting is redrawn in about 2 ms instead of about 85 ms when a value changes (blurred layers are kept, fills and blurs run across cores), so stepping opacity or padding, or pulling a slider, no longer stutters.
- **Help (F1)** lists each section with its parts by name, in place of two hundred command words that no longer fitted.
- The tab settings on Linux sit together under Window → Tabs.
- Long whole numbers read in groups of three (50 000 000). The config path beside a setting is cut at its start, not its end. Changed settings read a tone brighter than the ones left to Ghostty.
- The catppuccin and rose-pine theme packs are gone: Ghostty ships both families itself (Catppuccin Latte, Frappe, Macchiato, Mocha; Rose Pine, Moon, Dawn). Themes already installed from them stay where they are.
- 0xhckr/ghostty-shaders is now hackrdotsh/ghostty-shaders; it still states no licence, as the panel says.
- Shaders and themes are fetched from each repository's default branch, whatever it is called.

## [2.1.0] - 2026-11-06

### Added

- **`pip install ghostty-config`:** the same checksum-built binaries, published on [PyPI](https://pypi.org/project/ghostty-config/) as one wheel per system (macOS 12+ Intel and ARM, Linux x86_64 and aarch64, glibc and musl). pip, pipx and uv put `ghostty-config` straight on the PATH, with no Python in between. The release workflow publishes them with each tag, by trusted publishing.
- **Website** in `docs/`, served by GitHub Pages; `make site-shots` draws its pictures.

### Changed

- README and website pictures redrawn.

## [2.0.0] - 2026-11-01

### Added

- **Two new layouts, Palette and Guided**, beside Atelier (the interface as it was). A chooser asks for one at first launch and once after this update (Atelier preselected, `Enter` keeps it); change it any time in `F4` or Tool → Interface → Layout.
  - **Palette:** one search line over themes, fonts, settings, shortcuts and commands, with filter chips and one list grouped by kind. The highlighted setting opens in place with its values; moving onto a theme previews it in a strip drawn in its colours.
  - **Guided:** six steps of plain questions (Colours, Font, Window, Cursor, Copy & paste, Done), one page at a time, with large previews, Back / Skip / Next and links to all the settings of a step.
- **`F4`, ghostty-config's own settings from anywhere:** layout, look, reload after each change, update check, drawing mode and simple glyphs, kept in `state.json`. Command-line flags win over saved preferences and are shown as such.
- **Interface look:** accent colour (the scheme's own, mono, or one of six hues), high contrast, and in the image interface its size and the width of theme cards.
- **`F3`** opens Ghostty 1.4's own documentation (`+explain-config`) of the highlighted setting or keybind action.

### Changed: Ghostty 1.4

- Settings brought up to Ghostty 1.4 while keeping 1.3 working: the installed version is detected and keys or values only 1.4 reads are offered only to 1.4.
- New settings: `middle-click-action`, `link-osc8`, `drag-handle`, `abnormal-command-exit-runtime`, `vt-window-resize-allowed`, `clipboard-write-limit-bytes`, `scrollback-limit-bytes`, `scrollback-limit-lines`, `scrollback-compression`, `clipboard-read` / `clipboard-write`, and on Linux `gtk-tabs-inverted`, `gtk-horizontal-tab-scroll`, `quick-terminal-screen`.
- `copy-on-select` and `window-padding-balance` take their 1.4 values; presets map them per version.
- New shortcut actions `move_tab_to_new_window` and `prompt_window_title`; key data regenerated from 1.4.
- "SSH hosts" in Session → Shell integration lists the `ghostty +ssh-cache` entries, to forget, prune or clear them.
- 1071 themes: the 252 of the curated collection and the 819 Ghostty 1.4 ships.
- Reload: on macOS, Ghostty 1.3+ reloads over AppleScript (keystroke kept as fallback); on Linux, over D-Bus.

### Performance

- Startup to first frame: 103 → 42 ms (themes read in parallel, `ghostty --version` and appearance queried ahead, unchanged files skipped).
- Image interface, 120×34 cells: an unchanged frame 11.3 → 1.7 ms, a move along the themes wall 11.5 → 5.3 ms, a first frame 29 → 10 ms; typing a letter and back 21.4 → 9.0 ms. Theme cards are memoised and drawn on several cores, shapes composited in one pass, rows hashed and compressed in parallel.
- Memory: live heap after 100 keys on the wall 33 → 25 MB (120×34), 94 → 75 MB (240×68); far fewer allocations per frame.
- Cell interface: wall cards kept by colour (a move 3.1 → 1.7 ms); Palette draws only visible rows and searches from a per-library index.
- Images held by the terminal are now bounded by bytes (160 MB) as well as count, so large 5K/6K windows no longer lose rows to the terminal's own eviction.

### Fixed

- Footer: `F1`, `F3` and `F4` keep their place on every screen, narrow ones included; Guided shows Help and Settings.

[2.1.0]: https://github.com/vitruves/ghostty-config/compare/2.0.0...2.1.0
[2.0.0]: https://github.com/vitruves/ghostty-config/compare/1.2.0...2.0.0
