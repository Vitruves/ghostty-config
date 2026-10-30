package tui

// guided is the Guided profile: six steps, Colours, Font, Window, Cursor,
// Copy & paste and Done, each a few chosen settings. Until it is written it
// is Atelier under another name, so that a state file naming it opens an
// editor that works.
type guided struct{ atelier }

func newGuided() profile { return &guided{} }
