package tui

import (
	"strings"
	"sync"

	"github.com/vitruves/ghostty-config/internal/ghostty"
	"github.com/vitruves/ghostty-config/internal/kimg"
)

// Palette searches every theme on every key and Guided sorts them by warmth;
// both draw a theme's colours and grade on every frame. A theme of the
// library never changes once read (the library is read again whole after a
// save or a delete), so what is worked out of one is kept until the library
// is another: the wall's order, a theme's names in lower case, its colours as
// a card has them, how its text reads.

// themeFacts is what is kept of one theme.
type themeFacts struct {
	// lowerName and lowerFind are its name, and its family and note, in lower
	// case, as the search reads them.
	lowerName, lowerFind string
	dark                 bool
	spec                 kimg.CardSpec
	// ratio is its text on its ground; weak and weakName its least readable
	// colour of the first eight, as palContrast says them.
	ratio, weak float64
	weakName    string
	// palMeta is its kind and grade, as Palette's row says them; listed the
	// same as the theme command's list says them.
	palMeta, listed string
}

// themeIndex is what is kept of one library.
type themeIndex struct {
	lib   *ghostty.Library
	n     int
	order []*ghostty.Theme
	// facts has a key for every theme of the library, nil until asked for.
	facts map[*ghostty.Theme]*themeFacts
}

// themeIdx is the index of the library last asked about. Several models may
// draw at once in tests, so it is behind a lock.
var themeIdx struct {
	sync.Mutex
	themeIndex
}

// indexFor is the index of the model's library, made afresh when the library
// is another. The caller holds the lock.
func (m *Model) indexFor() *themeIndex {
	ix := &themeIdx.themeIndex
	if ix.lib != m.lib || ix.n != len(m.lib.Themes) {
		*ix = themeIndex{lib: m.lib, n: len(m.lib.Themes), facts: make(map[*ghostty.Theme]*themeFacts, len(m.lib.Themes))}
		for _, t := range m.lib.Themes {
			ix.facts[t] = nil
		}
	}
	return ix
}

// themeOrder is every theme of the library in the order of the wall,
// themeOptions("") without its titles. The slice is shared: it is not to be
// changed.
func (m *Model) themeOrder() []*ghostty.Theme {
	themeIdx.Lock()
	order := m.indexFor().order
	themeIdx.Unlock()
	if order != nil {
		return order
	}
	// Made without the lock, which themeOptions takes for each theme.
	order = make([]*ghostty.Theme, 0, len(m.lib.Themes))
	for _, o := range m.themeOptions("") {
		if !o.header && o.theme != nil {
			order = append(order, o.theme)
		}
	}
	themeIdx.Lock()
	if ix := m.indexFor(); ix.order == nil {
		ix.order = order
	}
	themeIdx.Unlock()
	return order
}

// factsOf is what is kept of a theme: worked out once for a theme of the
// library, afresh for any other, such as the one being edited.
func (m *Model) factsOf(t *ghostty.Theme) *themeFacts {
	if f := m.libFacts(t); f != nil {
		return f
	}
	return m.newFacts(t)
}

// libFacts is what is kept of a theme of the library, nil for any other.
func (m *Model) libFacts(t *ghostty.Theme) *themeFacts {
	themeIdx.Lock()
	defer themeIdx.Unlock()
	ix := m.indexFor()
	f, in := ix.facts[t]
	if !in {
		return nil
	}
	if f == nil {
		f = m.newFacts(t)
		ix.facts[t] = f
	}
	return f
}

func (m *Model) newFacts(t *ghostty.Theme) *themeFacts {
	f := &themeFacts{
		lowerName: strings.ToLower(t.Name),
		lowerFind: strings.ToLower(themeFamily(t) + " " + t.Note),
		dark:      t.IsDark(),
		spec:      m.themeSpec(t),
		palMeta:   palThemeMeta(t),
	}
	f.ratio, f.weak, f.weakName = palContrast(t)
	f.listed = themeListed(t)
	return f
}
