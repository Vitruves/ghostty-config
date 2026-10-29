package ghostty

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in   string
		want Version
	}{
		{"Ghostty 1.4.0\n\nVersion\n  - version: 1.4.0\n", Version{1, 4, 0, true}},
		{"Ghostty 1.3.1\n", Version{1, 3, 1, true}},
		{"Ghostty 1.5.0-dev+0123abc\n", Version{1, 5, 0, true}},
		{"Ghostty 2.0\n", Version{2, 0, 0, true}},
		{"", Version{}},
		{"error: unknown flag\n", Version{}},
	}
	for _, c := range cases {
		if got := ParseVersion(c.in); got != c.want {
			t.Errorf("ParseVersion(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		v            Version
		major, minor int
		want         bool
	}{
		{Version{1, 4, 0, true}, 1, 4, true},
		{Version{1, 3, 9, true}, 1, 4, false},
		{Version{1, 5, 0, true}, 1, 4, true},
		{Version{2, 0, 0, true}, 1, 4, true},
		{Version{0, 9, 0, true}, 1, 4, false},
		{Version{}, 1, 4, true},
	}
	for _, c := range cases {
		if got := c.v.AtLeast(c.major, c.minor); got != c.want {
			t.Errorf("%+v.AtLeast(%d, %d) = %v, want %v", c.v, c.major, c.minor, got, c.want)
		}
	}
	if s := (Version{}).String(); s != "unknown" {
		t.Errorf("unknown version prints %q", s)
	}
	if s := (Version{1, 4, 0, true}).String(); s != "1.4.0" {
		t.Errorf("1.4.0 prints %q", s)
	}
}

func TestBinaryVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a shell script")
	}
	bin := filepath.Join(t.TempDir(), "ghostty")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 'Ghostty 1.3.1'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if v := BinaryVersion(bin); v != (Version{1, 3, 1, true}) {
		t.Fatalf("BinaryVersion = %+v", v)
	}
	// The answer is kept: a changed binary is not asked again.
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 'Ghostty 1.4.0'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if v := BinaryVersion(bin); v.Minor != 3 {
		t.Fatalf("BinaryVersion asked again: %+v", v)
	}
	if v := BinaryVersion(""); v.Known || !v.AtLeast(1, 4) {
		t.Fatalf("no binary: %+v", v)
	}
	if v := BinaryVersion(filepath.Join(t.TempDir(), "missing")); v.Known {
		t.Fatalf("missing binary: %+v", v)
	}
}
