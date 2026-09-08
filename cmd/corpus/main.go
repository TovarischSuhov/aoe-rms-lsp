// Command corpus runs the aoe2-lsp binary over a directory of RMS/XS
// scripts and reports hard failures (see internal/corpus/.usages/).
package main

import (
	"aoe2-lsp/internal/corpus"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
)

func main() {
	bin := flag.String("bin", "aoe2-lsp", "path to the aoe2-lsp binary")
	dir := flag.String("dir", "", "corpus directory with .rms/.xs files (required)")
	jsonOut := flag.Bool("json", false, "print the machine-readable report instead of the summary")
	flag.Parse()

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	report, err := corpus.NewRunner(*bin).Run(context.Background(), *dir)
	if err != nil {
		slog.Error("corpus run failed", "err", err)

		os.Exit(2)
	}

	if *jsonOut {
		out, err := json.Marshal(report)
		if err != nil {
			slog.Error("encode report", "err", err)

			os.Exit(2)
		}

		fmt.Println(string(out))
	} else {
		fmt.Print(report.Summary())
	}

	if report.HardFailures > 0 {
		os.Exit(1)
	}
}
