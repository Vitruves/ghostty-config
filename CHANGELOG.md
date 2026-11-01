# Changelog

All notable changes to ghostty-config are documented here. Versions follow [Semantic Versioning](https://semver.org).

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

[2.0.0]: https://github.com/vitruves/ghostty-config/compare/1.2.0...2.0.0
