// Command aoe2-lsp is the language server binary for Age of Empires II RMS
// and XS scripts; it speaks LSP over stdio (see README for editor setup).
package main

import (
	"aoe2-lsp/internal/server"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
)

// version is injected at release builds via ldflags "-X main.version=<tag>";
// local builds keep the "dev" default.
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version)

		return
	}

	debug := flag.Bool("debug", false, "enable debug logging to stderr")
	flag.Parse()

	// stdout carries the protocol; logs must go to stderr only.
	slog.SetDefault(newLogger(os.Stderr, *debug))

	if err := server.Serve(context.Background()); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}

// newLogger builds the stderr logger; debug flips the level from Info to
// Debug so editors opt into verbose tracing with `aoe2-lsp -debug`.
func newLogger(w io.Writer, debug bool) *slog.Logger {
	level := slog.LevelInfo

	if debug {
		level = slog.LevelDebug
	}

	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}
