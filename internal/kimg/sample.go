package kimg

// Tok is a run of sample text and which colour it takes: a palette index, or
// one of the negative named colours.
type Tok struct {
	Idx  int
	Text string
}

// Named colours for Tok.Idx.
const (
	Fg     = -1
	Dim    = -2
	Cursor = -3
)

// Sample is the four lines of sample output a card shows. They vary from
// theme to theme (a shell, a diff, Python, a log) so a wall of cards reads
// as different things in different colours, not one picture repeated. The
// cells-only gallery draws the same lines, so the two never disagree.
func Sample(name string) [4][]Tok {
	sum := 0
	for _, r := range name {
		sum += int(r)
	}
	switch sum % 4 {
	case 1:
		return [4][]Tok{
			{{6, " @@ -12,6 +12,7 @@"}, {Fg, " render"}},
			{{1, " - old := load()"}},
			{{2, " + cur := load(ctx)"}},
			{{2, " ok "}, {Fg, "12 passed "}, {1, "1 failed "}, {Cursor, " "}},
		}
	case 2:
		return [4][]Tok{
			{{5, " def "}, {12, "render"}, {Fg, "(n: "}, {6, "int"}, {Fg, "):"}},
			{{Dim, "     # readable?"}},
			{{Fg, "     "}, {5, "return "}, {2, "f\"{n}\""}, {Fg, " + "}, {3, "1"}},
			{{2, " >>> "}, {Fg, "render(42)"}, {Cursor, " "}},
		}
	case 3:
		return [4][]Tok{
			{{Fg, " { "}, {4, "\"ok\""}, {Fg, ": "}, {5, "true"}, {Fg, ", "}, {4, "\"n\""}, {Fg, ": "}, {3, "42"}, {Fg, " }"}},
			{{6, " INFO  "}, {Fg, "listening on "}, {4, ":8080"}},
			{{3, " WARN  "}, {Fg, "cache is cold"}},
			{{1, " ERROR "}, {Fg, "timeout "}, {Cursor, " "}},
		}
	}
	return [4][]Tok{
		{{2, " > "}, {Fg, "ls "}, {4, "src/ "}, {6, "main.go "}, {Dim, "README"}},
		{{5, " func "}, {12, "main"}, {Fg, "() { "}, {Dim, "// hello"}},
		{{Fg, "   fmt."}, {4, "Println"}, {Fg, "("}, {2, "\"ok\""}, {Fg, ", "}, {3, "42"}, {Fg, ")"}},
		{{1, " error"}, {Fg, ": "}, {3, "warn "}, {2, "pass "}, {Cursor, " "}},
	}
}
