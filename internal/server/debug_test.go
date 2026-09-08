package server

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// captureLogger swaps slog.Default for a handler at the given level and
// returns its buffer; the previous logger is restored on cleanup.
func captureLogger(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(old) })

	return &buf
}

// openDebugDoc opens one .rms document in a fresh harness and returns the
// server dispatcher and the opened URI.
func openDebugDoc(t *testing.T, text string) (protocol.Server, uri.URI) {
	t.Helper()

	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	docURI := uri.File("debug.rms")
	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:  docURI,
			Text: text,
		},
	}))

	h.waitDiagnostics(docURI)

	return h.disp, docURI
}

func TestDebug_EventsAtDebugLevel(t *testing.T) {
	buf := captureLogger(t, slog.LevelDebug)

	const marker = "UNIQUE_PAYLOAD_MARKER_9137"
	disp, docURI := openDebugDoc(t, marker+"\n<PLAYER_SETUP>\n")

	_, err := disp.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 0},
		},
	})
	require.NoError(t, err)

	logs := buf.String()

	for _, event := range []string{
		"serve started",
		"initialized",
		"did_open",
		"diagnostics",
		"completion",
	} {
		assert.Contains(t, logs, event)
	}
}

func TestDebug_NoDocumentPayloadInLogs(t *testing.T) {
	buf := captureLogger(t, slog.LevelDebug)

	const marker = "UNIQUE_PAYLOAD_MARKER_9137"

	_, _ = openDebugDoc(t, marker+"\n<PLAYER_SETUP>\n")

	assert.NotContains(t, buf.String(), marker)
}

func TestDebug_SilentAtInfoLevel(t *testing.T) {
	buf := captureLogger(t, slog.LevelInfo)

	_, _ = openDebugDoc(t, "<PLAYER_SETUP>\n")

	assert.Empty(t, buf.String())
}
