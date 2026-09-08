package complete

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/kb"
	"aoe2-lsp/internal/rms"
	"testing"
)

// benchDoc builds a parsed map with sections and brace-less attribute
// context — the two completion contexts of real maps.
func benchDoc() rms.RmsFile {
	src := `<LAND_GENERATION>
base_terrain GRASS
create_land
  terrain_type DESERT

<OBJECTS_GENERATION>
create_object SHEEP
  number_of_objects 5
`
	file, _ := rms.Parse(src, "bench.rms")

	return file
}

// BenchmarkRmsAt_Section measures completion at a section command
// position — the heaviest candidate set.
func BenchmarkRmsAt_Section(b *testing.B) {
	store, err := kb.NewStore()
	if err != nil {
		b.Fatal(err)
	}

	completer := NewCompleter(store)
	file := benchDoc()

	pos := common.Pos{Line: 1, Column: 0}

	b.ReportAllocs()

	for b.Loop() {
		_ = completer.RmsAt(file, pos)
	}
}

// BenchmarkRmsAt_Attribute measures completion inside a brace-less
// attribute context.
func BenchmarkRmsAt_Attribute(b *testing.B) {
	store, err := kb.NewStore()
	if err != nil {
		b.Fatal(err)
	}

	completer := NewCompleter(store)
	file := benchDoc()

	pos := common.Pos{Line: 3, Column: 2}

	b.ReportAllocs()

	for b.Loop() {
		_ = completer.RmsAt(file, pos)
	}
}
