package tui

import "strings"

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
	haystack, needle = strings.ToLower(haystack), strings.ToLower(needle)
	if needle == "" || strings.Contains(haystack, needle) {
		return true
	}
	i := 0
	runes := []rune(needle)
	for _, r := range haystack {
		if i < len(runes) && runes[i] == r {
			i++
		}
	}
	return i == len(runes)
}

// fuzzyScore ranks matches: exact substring beats prefix-of-words beats
// scattered letters, so the best hit sits at the top of a filtered list.
func fuzzyScore(haystack, needle string) int {
	h, n := strings.ToLower(haystack), strings.ToLower(needle)
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
