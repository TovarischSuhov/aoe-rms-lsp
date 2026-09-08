package kb

import "testing"

// BenchmarkNewStore measures the startup cost: loading the embedded JSON
// knowledge base and building the lookup indices.
func BenchmarkNewStore(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := NewStore(); err != nil {
			b.Fatal(err)
		}
	}
}
