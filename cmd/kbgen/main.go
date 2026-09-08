// Command kbgen regenerates the embedded knowledge base files under
// kb/data/ from the local sources in docs/ref/ (see kb/.usages/data-pipeline.md).
//
// Usage (from the module root): go run ./cmd/kbgen
package main

import (
	"aoe2-lsp/kb"
	"flag"
	"log/slog"
	"os"
)

func main() {
	refDir := flag.String("ref", "docs/ref", "directory with the source reference files")
	outDir := flag.String("out", "kb/data", "directory for the generated JSON files")
	flag.Parse()

	if err := kb.GenKB(*refDir, *outDir, slog.Default()); err != nil {
		slog.Error("kbgen failed", "error", err)
		os.Exit(1)
	}
}
