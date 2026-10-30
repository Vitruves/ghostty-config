package ghostty

import "testing"

// BenchmarkLoadLibrary reads the themes of the Ghostty installed on this
// machine, around a thousand files, the way the editor does at startup.
func BenchmarkLoadLibrary(b *testing.B) {
	paths := DefaultPaths("", "")
	if len(LoadLibrary(paths).Themes) < 100 {
		b.Skip("no Ghostty themes on this machine")
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		LoadLibrary(paths)
	}
}

// BenchmarkDarkDesktop asks the desktop for its appearance, which New does
// before the first frame.
func BenchmarkDarkDesktop(b *testing.B) {
	for i := 0; i < b.N; i++ {
		DarkDesktop()
	}
}
