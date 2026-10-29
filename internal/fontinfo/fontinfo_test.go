package fontinfo

import (
	"encoding/binary"
	"os"
	"sort"
	"testing"
	"unicode/utf16"
)

// The tests build their fonts by hand, a few tables each, so what the parser
// is asked to read is known to the byte.

type be []byte

func (b be) u16(v int) be    { return binary.BigEndian.AppendUint16(b, uint16(v)) }
func (b be) u32(v uint32) be { return binary.BigEndian.AppendUint32(b, v) }
func (b be) tag(s string) be { return append(b, s...) }

// nameTable holds English Windows names by ID.
func nameTable(names map[int]string) []byte {
	ids := make([]int, 0, len(names))
	for id := range names {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var storage be
	recs := be{}
	for _, id := range ids {
		var s be
		for _, u := range utf16.Encode([]rune(names[id])) {
			s = s.u16(int(u))
		}
		recs = recs.u16(3).u16(1).u16(0x409).u16(id).u16(len(s)).u16(len(storage))
		storage = append(storage, s...)
	}
	t := be{}.u16(0).u16(len(ids)).u16(6 + 12*len(ids))
	return append(append(t, recs...), storage...)
}

// layoutTable is a GSUB or GPOS table whose feature list holds tags, each
// with the name ID of its parameters, 0 for none.
func layoutTable(feats [][2]any) []byte {
	// Header, then the feature list right after it.
	t := be{}.u16(1).u16(0).u16(0).u16(10).u16(0)
	list := be{}.u16(len(feats))
	tables := be{}
	base := 2 + 6*len(feats)
	for _, f := range feats {
		list = list.tag(f[0].(string)).u16(base + len(tables))
		if id := f[1].(int); id != 0 {
			// The parameters follow the feature table: offset, lookup count,
			// then version and name ID.
			tables = tables.u16(4).u16(0).u16(0).u16(id)
		} else {
			tables = tables.u16(0).u16(0)
		}
	}
	return append(append(t, list...), tables...)
}

// fvarTable holds axes: tag, min, default, max, flags, name ID.
func fvarTable(axes [][6]any) []byte {
	t := be{}.u16(1).u16(0).u16(16).u16(2).u16(len(axes)).u16(20).u16(0).u16(0)
	fx := func(v float64) uint32 { return uint32(int32(v * 65536)) }
	for _, a := range axes {
		t = t.tag(a[0].(string)).u32(fx(a[1].(float64))).u32(fx(a[2].(float64))).u32(fx(a[3].(float64))).u16(a[4].(int)).u16(a[5].(int))
	}
	return t
}

// sfntFace lays tables out after a table directory that starts at base.
func sfntFace(tables map[string][]byte, base int) (dir, body []byte) {
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	d := be{}.u32(0x00010000).u16(len(tags)).u16(0).u16(0).u16(0)
	at := base + 12 + 16*len(tags)
	for _, tag := range tags {
		data := tables[tag]
		d = d.tag(tag).u32(0).u32(uint32(at + len(body))).u32(uint32(len(data)))
		body = append(body, data...)
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
	}
	return d, body
}

func sfntFile(tables map[string][]byte) []byte {
	d, body := sfntFace(tables, 0)
	return append(d, body...)
}

// ttcFile is a collection of faces, directories first, tables after.
func ttcFile(faces ...map[string][]byte) []byte {
	head := 12 + 4*len(faces)
	dirSize := func(t map[string][]byte) int { return 12 + 16*len(t) }
	at := head
	offsets := make([]int, len(faces))
	for i, f := range faces {
		offsets[i] = at
		at += dirSize(f)
	}
	out := be{}.tag("ttcf").u16(1).u16(0).u32(uint32(len(faces)))
	for _, o := range offsets {
		out = out.u32(uint32(o))
	}
	var bodies []byte
	for _, f := range faces {
		d, body := sfntFace(f, 0)
		// Rebase the directory's offsets onto where the bodies will sit.
		for i := 0; i < len(f); i++ {
			rec := d[12+16*i:]
			off := binary.BigEndian.Uint32(rec[8:]) - uint32(12+16*len(f))
			binary.BigEndian.PutUint32(rec[8:], off+uint32(at+len(bodies)))
		}
		out = append(out, d...)
		bodies = append(bodies, body...)
	}
	return append(out, bodies...)
}

func TestFeaturesAxesAndNames(t *testing.T) {
	font := sfntFile(map[string][]byte{
		"name": nameTable(map[int]string{6: "Test-Regular", 256: "Round dots", 257: "Weight", 258: "Hidden"}),
		"GSUB": layoutTable([][2]any{{"calt", 0}, {"ss01", 256}, {"ss02", 0}, {"zero", 0}, {"cv07", 0}, {"locl", 0}}),
		"GPOS": layoutTable([][2]any{{"kern", 0}, {"zero", 0}}),
		"fvar": fvarTable([][6]any{{"wght", 100.0, 400.0, 800.0, 0, 257}, {"YAXS", 0.0, 0.0, 1.0, 1, 258}, {"slnt", -12.5, 0.0, 0.0, 0, 0}}),
	})
	info, err := Parse(font, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if info.PostScript != "Test-Regular" {
		t.Errorf("PostScript name %q", info.PostScript)
	}
	var tags []string
	for _, f := range info.Features {
		tags = append(tags, f.Tag)
	}
	if got := join(tags); got != "calt cv07 kern locl ss01 ss02 zero" {
		t.Errorf("features %q: each tag once, sorted, from both tables", got)
	}
	labels := map[string]string{}
	for _, f := range info.Features {
		labels[f.Tag] = f.Label()
	}
	for tag, want := range map[string]string{"ss01": "Round dots", "ss02": "Stylistic set 2", "zero": "Slashed zero", "cv07": "Character variant 7"} {
		if labels[tag] != want {
			t.Errorf("%s is labelled %q, want %q", tag, labels[tag], want)
		}
	}
	if len(info.Axes) != 2 {
		t.Fatalf("axes %+v: the hidden one should be left out", info.Axes)
	}
	if a := info.Axes[0]; a.Tag != "wght" || a.Min != 100 || a.Default != 400 || a.Max != 800 || a.Label() != "Weight" {
		t.Errorf("weight axis %+v", a)
	}
	if a := info.Axes[1]; a.Tag != "slnt" || a.Min != -12.5 || a.Label() != "Slant" {
		t.Errorf("slant axis %+v: an axis without a name takes the registered one", a)
	}
	if info.NerdIcons {
		t.Error("a font without a cmap cannot draw the icons")
	}
}

func TestOptionalLeavesTheLigaturesAndTheMachineryOut(t *testing.T) {
	for _, tag := range []string{"ss01", "ss20", "cv01", "cv99", "zero", "tnum", "onum", "case", "frac"} {
		if !Optional(tag) {
			t.Errorf("%s should be offered", tag)
		}
	}
	for _, tag := range []string{"calt", "liga", "dlig", "kern", "locl", "ccmp", "mark", "ss21", "ss00", "cv100", "aalt"} {
		if Optional(tag) {
			t.Errorf("%s should not be offered", tag)
		}
	}
}

func TestCollectionPicksTheFaceByPostScriptName(t *testing.T) {
	regular := map[string][]byte{
		"name": nameTable(map[int]string{6: "Pair-Regular"}),
		"GSUB": layoutTable([][2]any{{"ss01", 0}}),
	}
	bold := map[string][]byte{
		"name": nameTable(map[int]string{6: "Pair-Bold"}),
		"GSUB": layoutTable([][2]any{{"ss02", 0}, {"zero", 0}}),
	}
	ttc := ttcFile(regular, bold)
	info, err := Parse(ttc, "Pair-Bold", 0)
	if err != nil {
		t.Fatal(err)
	}
	if info.PostScript != "Pair-Bold" || len(info.Features) != 2 || info.Features[0].Tag != "ss02" {
		t.Errorf("by name: %+v", info)
	}
	info, err = Parse(ttc, "", 1)
	if err != nil || info.PostScript != "Pair-Bold" {
		t.Errorf("by index: %+v %v", info, err)
	}
	info, err = Parse(ttc, "Nobody", 0)
	if err != nil || info.PostScript != "Pair-Regular" {
		t.Errorf("an unknown name falls back on the index: %+v %v", info, err)
	}
}

func TestTruncatedFilesAreRefusedNotPanicked(t *testing.T) {
	font := sfntFile(map[string][]byte{
		"name": nameTable(map[int]string{6: "Cut"}),
		"GSUB": layoutTable([][2]any{{"ss01", 256}}),
	})
	for n := 0; n < len(font); n++ {
		_, _ = Parse(font[:n], "", 0)
	}
	if _, err := Parse([]byte("ttcf\x00\x01\x00\x00\x00\x00\x00\x09"), "", 0); err == nil {
		t.Error("a collection of nine faces in twelve bytes should be refused")
	}
}

// The system's fonts, when they are there: SF Mono is variable and names
// its stylistic sets; Menlo is a collection.
func TestSystemFonts(t *testing.T) {
	const sfMono = "/System/Library/Fonts/SFNSMono.ttf"
	if _, err := os.Stat(sfMono); err == nil {
		info, err := Read(sfMono, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(info.Axes) == 0 || info.Axes[0].Tag != "wght" {
			t.Errorf("SF Mono axes %+v", info.Axes)
		}
		named := false
		for _, f := range info.Features {
			if f.Tag == "ss03" && f.Name != "" {
				named = true
			}
		}
		if !named {
			t.Errorf("SF Mono names its stylistic set 3: %+v", info.Features)
		}
	}
	const menlo = "/System/Library/Fonts/Menlo.ttc"
	if _, err := os.Stat(menlo); err == nil {
		info, err := Read(menlo, "Menlo-Bold", 0)
		if err != nil {
			t.Fatal(err)
		}
		if info.PostScript != "Menlo-Bold" || len(info.Axes) != 0 || info.NerdIcons {
			t.Errorf("Menlo Bold %+v", info)
		}
	}
}

func join(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += " "
		}
		out += x
	}
	return out
}
