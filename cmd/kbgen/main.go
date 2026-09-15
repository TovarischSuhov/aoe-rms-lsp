// Command kbgen regenerates the embedded knowledge base files under
// internal/kb/data/ from the local sources in docs/ref/ (see internal/kb/.usages/data-pipeline.md).
//
// With -diff it regenerates into a temporary directory instead and prints
// the aggregated old→new report — the "what's new" step of the refresh
// process (docs/kb-refresh.md) — leaving -out untouched.
//
// Usage (from the module root): go run ./cmd/kbgen [-diff]
package main

import (
	"aoe2-lsp/internal/kb"
	"flag"
	"fmt"
	"log/slog"
	"os"
)

func main() {
	refDir := flag.String("ref", "docs/ref", "directory with the source reference files")
	outDir := flag.String("out", "internal/kb/data", "directory for the generated JSON files")
	diff := flag.Bool("diff", false, "print the diff between -out and a fresh regeneration instead of writing -out")
	flag.Parse()

	if *diff {
		if err := runDiff(*refDir, *outDir); err != nil {
			slog.Error("kbgen diff failed", "error", err)
			os.Exit(1)
		}

		return
	}

	if err := kb.GenKB(*refDir, *outDir, slog.Default()); err != nil {
		slog.Error("kbgen failed", "error", err)
		os.Exit(1)
	}
}

// runDiff regenerates the kb into a temporary directory and prints the
// markdown diff against the current data in outDir.
func runDiff(refDir string, outDir string) error {
	tmp, err := os.MkdirTemp("", "kbgen-diff-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			slog.Warn("remove temp dir", "path", tmp, "error", err)
		}
	}()

	if err := kb.GenKB(refDir, tmp, slog.Default()); err != nil {
		return fmt.Errorf("regenerate: %w", err)
	}

	report, err := kb.DiffKB(outDir, tmp)
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}

	fmt.Print(report.Render())

	return nil
}
