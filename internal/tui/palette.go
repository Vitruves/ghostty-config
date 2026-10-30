package tui

// palette is the Palette profile: one search line over every command, theme
// and font, answered in one list. Until it is written it is Atelier under
// another name, so that a state file naming it opens an editor that works.
type palette struct{ atelier }

func newPalette() profile { return &palette{} }
