package tui

import (
	"strings"
	"unicode/utf8"
)

// scroller keeps a cursor inside a window of rows, the way every list in the
// editor scrolls.
type scroller struct {
	cursor, offset int
}

// clamp keeps the cursor within count rows and the window within height.
func (s *scroller) clamp(count, height int) {
	if count == 0 {
		s.cursor, s.offset = 0, 0
		return
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor >= count {
		s.cursor = count - 1
	}
	if height < 1 {
		height = 1
	}
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+height {
		s.offset = s.cursor - height + 1
	}
	if s.offset > count-height {
		s.offset = count - height
	}
	if s.offset < 0 {
		s.offset = 0
	}
}

// move steps the cursor, clamped.
func (s *scroller) move(delta, count, height int) {
	s.cursor += delta
	s.clamp(count, height)
}

// window returns the rows visible, given a renderer for one row.
func (s *scroller) window(count, height int, render func(i int, selected bool) string) []string {
	s.clamp(count, height)
	var out []string
	for i := s.offset; i < count && i < s.offset+height; i++ {
		out = append(out, render(i, i == s.cursor))
	}
	return out
}

// fuzzyMatch reports whether every rune of needle appears in order in
// haystack, so "ctpmoc" finds "catppuccin-mocha" without an exact substring.
func fuzzyMatch(haystack, needle string) bool {
	return fuzzyMatchLower(strings.ToLower(haystack), strings.ToLower(needle))
}

// fuzzyMatchLower is fuzzyMatch of a haystack and a needle already in lower
// case.
func fuzzyMatchLower(haystack, needle string) bool {
	if needle == "" || strings.Contains(haystack, needle) {
		return true
	}
	for _, r := range haystack {
		if needle == "" {
			break
		}
		if n, size := utf8.DecodeRuneInString(needle); n == r {
			needle = needle[size:]
		}
	}
	return needle == ""
}

// fuzzyScore ranks matches: exact substring beats prefix-of-words beats
// scattered letters, so the best hit sits at the top of a filtered list.
func fuzzyScore(haystack, needle string) int {
	return fuzzyScoreLower(strings.ToLower(haystack), strings.ToLower(needle))
}

// fuzzyScoreLower is fuzzyScore of a haystack and a needle already in lower
// case.
func fuzzyScoreLower(h, n string) int {
	switch {
	case n == "":
		return 0
	case h == n:
		return 0
	case strings.HasPrefix(h, n):
		return 1
	case strings.Contains(h, n):
		return 2
	}
	return 3
}
