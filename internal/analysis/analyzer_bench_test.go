package analysis

import (
	"aoe2-lsp/internal/kb"
	"aoe2-lsp/internal/rms"
	"aoe2-lsp/internal/xs"
	"fmt"
	"strings"
	"testing"
)

// BenchmarkAnalyzeRms measures semantic checks over an already parsed map
// (the didOpen/didChange diagnostics path reuses the same analyzer).
func BenchmarkAnalyzeRms(b *testing.B) {
	store, err := kb.NewStore()
	if err != nil {
		b.Fatal(err)
	}

	analyzer := NewAnalyzer(store)

	src := benchRmsSource(500)
	file, _ := rms.Parse(src, "bench.rms")

	b.ReportAllocs()

	for b.Loop() {
		_ = analyzer.AnalyzeRms(file)
	}
}

// BenchmarkAnalyzeXs measures semantic checks over an already parsed XS
// document without externals (the inline-block path).
func BenchmarkAnalyzeXs(b *testing.B) {
	store, err := kb.NewStore()
	if err != nil {
		b.Fatal(err)
	}

	analyzer := NewAnalyzer(store)

	src := benchXsSource(200)
	file, _ := xs.XsParse(src, "bench.xs")

	b.ReportAllocs()

	for b.Loop() {
		_ = analyzer.AnalyzeXs(file, nil)
	}
}

// benchRmsSource mirrors the rms cell's generated benchmark document: the
// shapes analysis sees in real maps.
func benchRmsSource(commands int) string {
	var b strings.Builder

	fmt.Fprintf(&b, "<LAND_GENERATION>\nbase_terrain GRASS\n")
	fmt.Fprintf(&b, "create_player_lands {\n  terrain_type GRASS\n  land_percent 70\n}\n")

	for i := range commands {
		switch i % 3 {
		case 0:
			fmt.Fprintf(&b, "create_land {\n  terrain_type DESERT\n  land_percent %d\n}\n", i%90+5)
		case 1:
			fmt.Fprintf(&b, "create_elevation %d {\n  number_of_clumps %d\n}\n", i%7, i%9+1)
		case 2:
			fmt.Fprintf(&b, "create_object SHEEP {\n  number_of_objects %d\n  set_gaia_object_only\n}\n", i%20+1)
		}
	}

	return b.String()
}

// benchXsSource mirrors the xs cell's generated benchmark document.
func benchXsSource(functions int) string {
	var b strings.Builder

	for i := range functions {
		fmt.Fprintf(&b, "void fn_%d(int a) {\n  int local = a + 1;\n  acc = xsGetGoal(%d) + local;\n}\n", i, i%8)
	}

	return b.String()
}
