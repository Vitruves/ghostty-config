package collection

import "testing"

// BenchmarkInstall refreshes the collection in a themes directory that
// already holds it, as every start does.
func BenchmarkInstall(b *testing.B) {
	dir := b.TempDir()
	if _, _, err := Install(dir); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := Install(dir); err != nil {
			b.Fatal(err)
		}
	}
}
