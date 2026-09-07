// Command aoe2-lsp is the language server binary for Age of Empires II RMS
// and XS scripts; it speaks LSP over stdio (see README for editor setup).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"aoe2-lsp/server"
)

// version is injected at release builds via ldflags "-X main.version=<tag>";
// local builds keep the "dev" default.
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version)

		return
	}

	// stdout carries the protocol; logs must go to stderr only.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if err := server.Serve(context.Background()); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}
