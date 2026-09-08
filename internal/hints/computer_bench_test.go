package hints

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/kb"
	"aoe2-lsp/internal/rms"
	"testing"
)

// benchSource builds a map whose first command carries arguments — the
// signature-help target.
func benchSource() (rms.RmsFile, common.Pos) {
	src := "<LAND_GENERATION>\ncreate_elevation 4\n{\n  number_of_clumps 8\n}\n"

	file, _ := rms.Parse(src, "bench.rms")

	return file, common.Pos{Line: 1, Column: 20}
}

// BenchmarkRmsAt measures the signature-help computation for a command
// position (the textDocument/signatureHelp path minus parsing).
func BenchmarkRmsAt(b *testing.B) {
	store, err := kb.NewStore()
	if err != nil {
		b.Fatal(err)
	}

	computer := NewComputer(store)
	file, pos := benchSource()

	b.ReportAllocs()

	for b.Loop() {
		_, _ = computer.RmsAt(file, pos)
	}
}
