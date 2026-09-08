package server

import (
	"aoe2-lsp/analysis"
	"aoe2-lsp/complete"
	"aoe2-lsp/hints"
	"aoe2-lsp/kb"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// Serve wires the dependencies and serves LSP over stdio until the editor
// disconnects or calls exit. The returned error is the shutdown reason.
func Serve(ctx context.Context) error {
	return serve(ctx, stdio{})
}

// serve runs the bootstrap over an arbitrary transport: stdio in
// production, pipes in tests.
func serve(ctx context.Context, rwc io.ReadWriteCloser) error {
	store, err := kb.NewStore()
	if err != nil {
		return fmt.Errorf("load knowledge base: %w", err)
	}

	srv := NewServer(
		store,
		analysis.NewAnalyzer(store),
		hints.NewComputer(store),
		complete.NewCompleter(store),
	)

	// The client dispatcher reaches handlers via the request context
	// (protocol.ClientFromContext), so nothing races the serving start.
	ctx = protocol.WithLogger(ctx, slog.Default())
	_, conn, _ := protocol.NewServer(ctx, srv, jsonrpc2.NewStream(rwc))

	select {
	case <-conn.Done():
	case <-srv.exit:
	}

	if err := conn.Close(); err != nil {
		return fmt.Errorf("close connection: %w", err)
	}

	return nil
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
