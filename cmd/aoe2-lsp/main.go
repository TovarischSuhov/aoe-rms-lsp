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
	"strings"
)

// version is injected at release builds via ldflags "-X main.version=<tag>";
// local builds keep the "dev" default.
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version)

		return
	}

	fs := flag.NewFlagSet("aoe2-lsp", flag.ContinueOnError)
	// Flag problems are reported through the logger below; the flag
	// package's own printing would only duplicate them on stderr.
	fs.SetOutput(io.Discard)
	debug := fs.Bool("debug", false, "enable debug logging to stderr")

	// A mismatched client must not lose the server: unknown flag-looking
	// arguments (e.g. the widespread --stdio convention) are logged and
	// dropped instead of aborting startup.
	known, unknown := splitUnknownArgs(fs, os.Args[1:])
	parseErr := fs.Parse(known)

	// stdout carries the protocol; logs must go to stderr only.
	slog.SetDefault(newLogger(os.Stderr, *debug))

	for _, arg := range unknown {
		slog.Warn("ignoring unknown flag", "arg", arg)
	}

	if parseErr != nil {
		slog.Error("ignoring flag parse error", "err", parseErr)
	}

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

// splitUnknownArgs partitions args into flag arguments `fs` recognizes
// (kept verbatim, `-name=value` included) and unknown flag-looking ones
// (to be logged and dropped by the caller). Positional arguments and
// everything after a bare `--` are kept as-is; every registered flag is
// boolean, so a kept argument never consumes the one following it.
func splitUnknownArgs(fs *flag.FlagSet, args []string) (known, unknown []string) {
	for i, arg := range args {
		if arg == "--" {
			known = append(known, args[i:]...)

			break
		}

		if len(arg) < 2 || arg[0] != '-' {
			known = append(known, arg)

			continue
		}

		name := strings.TrimLeft(arg, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
		}

		if fs.Lookup(name) == nil {
			unknown = append(unknown, arg)

			continue
		}

		known = append(known, arg)
	}

	return known, unknown
}
