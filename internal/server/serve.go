package server

import (
	"aoe2-lsp/internal/analysis"
	"aoe2-lsp/internal/complete"
	"aoe2-lsp/internal/hints"
	"aoe2-lsp/internal/kb"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// Serve wires the dependencies and serves LSP over stdio until the editor
// disconnects or calls exit. The returned error is the termination reason:
// nil on transport EOF or a clean shutdown-then-exit, non-nil when the
// client exits without shutdown (main turns it into exit code 1).
func Serve(ctx context.Context, version string) error {
	return serve(ctx, stdio{}, version)
}

// serve runs the bootstrap over an arbitrary transport: stdio in
// production, pipes in tests. The returned error is the termination
// reason with the same encoding as Serve.
func serve(ctx context.Context, rwc io.ReadWriteCloser, version string) error {
	store, err := kb.NewStore()
	if err != nil {
		return fmt.Errorf("load knowledge base: %w", err)
	}

	srv := NewServer(
		store,
		analysis.NewAnalyzer(store),
		hints.NewComputer(store),
		complete.NewCompleter(store),
		version,
	)

	// The client dispatcher reaches handlers via the request context
	// (protocol.ClientFromContext), so nothing races the serving start.
	slog.DebugContext(ctx, "serve started")

	ctx = protocol.WithLogger(ctx, slog.Default())
	_, conn, _ := protocol.NewServer(ctx, srv, jsonrpc2.NewStream(rwc))

	var exited bool
	select {
	case <-conn.Done():
	case <-srv.exit:
		exited = true
	}

	if err := conn.Err(); err != nil {
		slog.ErrorContext(ctx, "connection failed", "err", err)
	}

	// Exit without shutdown breaks the LSP lifecycle: serve fails (main
	// exits 1) even though the close below still runs — its own error
	// loses to the exit reason when both fire. A transport EOF stays nil
	// regardless of the flag.
	var exitErr error
	if exited && !srv.shutDown.Load() {
		exitErr = errors.New("exit without shutdown")
	}

	if err := conn.Close(); err != nil && exitErr == nil {
		return fmt.Errorf("close connection: %w", err)
	}

	return exitErr
}

// stdio adapts the process stdin/stdout pair to a jsonrpc2 stream.
type stdio struct{}

// Read reads protocol bytes from stdin.
func (stdio) Read(p []byte) (int, error) {
	return os.Stdin.Read(p)
}

// Write writes protocol bytes to stdout.
func (stdio) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}

// Close closes both transport files.
func (stdio) Close() error {
	if err := os.Stdin.Close(); err != nil {
		return fmt.Errorf("close stdin: %w", err)
	}

	if err := os.Stdout.Close(); err != nil {
		return fmt.Errorf("close stdout: %w", err)
	}

	return nil
}
