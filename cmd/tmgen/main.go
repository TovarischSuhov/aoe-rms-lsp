// Command tmgen regenerates the RMS TextMate grammar at
// editors/vscode/syntaxes/ from the embedded knowledge base (see
// internal/highlight/.usages/grammar-pipeline.md).
//
// Usage (from the module root): go run ./cmd/tmgen
package main

import (
	"aoe2-lsp/internal/highlight"
	"aoe2-lsp/internal/kb"
	"flag"
	"log/slog"
	"os"
)

func main() {
	out := flag.String("out", "editors/vscode/syntaxes/aoe2rms.tmLanguage.json", "path of the generated grammar file")
	flag.Parse()

	store, err := kb.NewStore()
	if err != nil {
		slog.Error("tmgen failed to load the knowledge base", "error", err)
		os.Exit(1)
	}

	if err := highlight.GenTmLanguage(store, *out, slog.Default()); err != nil {
		slog.Error("tmgen failed", "error", err)
		os.Exit(1)
	}
}
