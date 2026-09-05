// Command aoe2-lsp is the language server binary for Age of Empires II RMS
// and XS scripts; it speaks LSP over stdio (see README for editor setup).
package main

import (
	"context"
	"log/slog"
	"os"

	"aoe2-lsp/server"
)

func main() {
	// stdout carries the protocol; logs must go to stderr only.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if err := server.Serve(context.Background()); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}
