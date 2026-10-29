package ghostty

// Line diffs. Config files are short, a few hundred lines at the most, so a
// plain longest-common-subsequence table is fast enough and gives the diff a
// person expects: the lines that stayed are as many as they can be. The same
// diff draws the panels that compare two versions of a file and drives the
// three-way merge that undoes one step on a file edited elsewhere since.

// DiffKind says what happened to a line.
type DiffKind int

const (
	DiffSame DiffKind = iota
	DiffDel           // in the old text only
	DiffAdd           // in the new text only
)

// DiffLine is one line of a diff. Old and New are its indexes in the old and
// new text, -1 on the side it is missing from.
type DiffLine struct {
	Kind     DiffKind
	Text     string
	Old, New int
}

// Diff compares two texts line by line. Within a changed run the removed
// lines come before the added ones, the way diffs are usually read.
func Diff(old, new []string) []DiffLine {
	// The lines both ends share cost nothing to match, and in a config edited
	// by a tool they are nearly the whole file.
	pre := 0
	for pre < len(old) && pre < len(new) && old[pre] == new[pre] {
		pre++
	}
	suf := 0
	for suf < len(old)-pre && suf < len(new)-pre && old[len(old)-1-suf] == new[len(new)-1-suf] {
		suf++
	}
	a, b := old[pre:len(old)-suf], new[pre:len(new)-suf]

	// lcs[i][j] is the length of the longest common subsequence of a[i:] and
	// b[j:], kept in one slice.
	w := len(b) + 1
	lcs := make([]int32, (len(a)+1)*w)
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i*w+j] = lcs[(i+1)*w+j+1] + 1
			} else if lcs[(i+1)*w+j] >= lcs[i*w+j+1] {
				lcs[i*w+j] = lcs[(i+1)*w+j]
			} else {
				lcs[i*w+j] = lcs[i*w+j+1]
			}
		}
	}

	out := make([]DiffLine, 0, len(old)+len(new)-pre-suf)
	for i := 0; i < pre; i++ {
		out = append(out, DiffLine{DiffSame, old[i], i, i})
	}
	var dels, adds []DiffLine
	flush := func() {
		out = append(append(out, dels...), adds...)
		dels, adds = dels[:0], adds[:0]
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			flush()
			out = append(out, DiffLine{DiffSame, a[i], pre + i, pre + j})
			i++
			j++
		case j < len(b) && (i == len(a) || lcs[i*w+j+1] >= lcs[(i+1)*w+j]):
			adds = append(adds, DiffLine{DiffAdd, b[j], -1, pre + j})
			j++
		default:
			dels = append(dels, DiffLine{DiffDel, a[i], pre + i, -1})
			i++
		}
	}
	flush()
	for k := 0; k < suf; k++ {
		oi, ni := len(old)-suf+k, len(new)-suf+k
		out = append(out, DiffLine{DiffSame, old[oi], oi, ni})
	}
	return out
}

// DiffCounts is how many lines a diff adds and removes.
func DiffCounts(d []DiffLine) (added, removed int) {
	for _, l := range d {
		switch l.Kind {
		case DiffAdd:
			added++
		case DiffDel:
			removed++
		}
	}
	return added, removed
}

// hunk replaces old[start:end] with lines. An insertion has start == end.
type hunk struct {
	start, end int
	lines      []string
}

// hunks reads a diff as the replacements that turn old into new.
func hunks(old, new []string) []hunk {
	var out []hunk
	var cur *hunk
	pos := 0
	for _, l := range Diff(old, new) {
		switch l.Kind {
		case DiffSame:
			if cur != nil {
				out = append(out, *cur)
				cur = nil
			}
			pos = l.Old + 1
		case DiffDel:
			if cur == nil {
				cur = &hunk{start: l.Old, end: l.Old}
			}
			cur.end = l.Old + 1
			pos = l.Old + 1
		case DiffAdd:
			if cur == nil {
				cur = &hunk{start: pos, end: pos}
			}
			cur.lines = append(cur.lines, l.Text)
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

// touches reports whether a hunk lands on a span of the base already taken.
// Two replacements clash when their ranges overlap; an insertion clashes with
// a range only strictly inside it, and with another insertion at the same
// place, whose order nobody could tell. An insertion right next to a change
// is not a clash: a line appended under a line this tool appended is the
// common case of an edit made elsewhere, and both survive.
func touches(start, end int, h hunk) bool {
	switch {
	case start == end && h.start == h.end:
		return start == h.start
	case h.start == h.end:
		return start < h.start && h.start < end
	case start == end:
		return h.start < start && start < h.end
	}
	return h.start < end && start < h.end
}

// apply rewrites base[start:end] with the hunks that fall in it, in order.
func apply(base []string, start, end int, hs []hunk) []string {
	var out []string
	pos := start
	for _, h := range hs {
		out = append(out, base[pos:h.start]...)
		out = append(out, h.lines...)
		pos = h.end
	}
	return append(out, base[pos:end]...)
}

// merge3 carries the change from base to theirs over onto ours, which also
// started from base. Where both changed the same lines differently, ours is
// kept and the clash counted: the text as it is now wins over a change that
// can no longer be told apart from it.
func merge3(base, ours, theirs []string) ([]string, int) {
	ho, ht := hunks(base, ours), hunks(base, theirs)
	var out []string
	conflicts := 0
	pos, i, j := 0, 0, 0
	for i < len(ho) || j < len(ht) {
		// The cluster starts at the hunk that comes first, and takes in every
		// hunk of either side that lands on it.
		var mine, yours []hunk
		first := func() bool {
			if i == len(ho) {
				return false
			}
			if j == len(ht) {
				return true
			}
			return ho[i].start < ht[j].start || (ho[i].start == ht[j].start && ho[i].end <= ht[j].end)
		}
		var start, end int
		if first() {
			mine = append(mine, ho[i])
			start, end = ho[i].start, ho[i].end
			i++
		} else {
			yours = append(yours, ht[j])
			start, end = ht[j].start, ht[j].end
			j++
		}
		for grew := true; grew; {
			grew = false
			for i < len(ho) && touches(start, end, ho[i]) {
				mine = append(mine, ho[i])
				end = max(end, ho[i].end)
				i, grew = i+1, true
			}
			for j < len(ht) && touches(start, end, ht[j]) {
				yours = append(yours, ht[j])
				end = max(end, ht[j].end)
				j, grew = j+1, true
			}
		}
		out = append(out, base[pos:start]...)
		switch {
		case len(yours) == 0:
			out = append(out, apply(base, start, end, mine)...)
		case len(mine) == 0:
			out = append(out, apply(base, start, end, yours)...)
		default:
			a, b := apply(base, start, end, mine), apply(base, start, end, yours)
			if !sameLines(a, b) {
				conflicts++
			}
			out = append(out, a...)
		}
		pos = end
	}
	return append(out, base[pos:]...), conflicts
}

func sameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
