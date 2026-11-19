package server

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// recoverStubURI is the document URI the recover tests negotiate: the stub's
// Shutdown pushes its diagnostics batch for it over the live connection.
const recoverStubURI = uri.URI("file:///recover/stub.rms")

// recoverLogEntry is one captured slog record, reduced to the fields the
// recover tests assert on.
type recoverLogEntry struct {
	level      slog.Level
	message    string
	method     string
	panicValue string
}

// recoverLogCapture is the slog handler behind newRecoverLogCapture: it
// buffers every record the default logger emits while a test runs.
type recoverLogCapture struct {
	mu      sync.Mutex
	entries []recoverLogEntry
}

func (c *recoverLogCapture) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (c *recoverLogCapture) Handle(_ context.Context, r slog.Record) error {
	entry := recoverLogEntry{level: r.Level, message: r.Message}

	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "method":
			entry.method = a.Value.String()
		case "panic":
			entry.panicValue = a.Value.String()
		}

		return true
	})

	c.mu.Lock()
	c.entries = append(c.entries, entry)
	c.mu.Unlock()

	return nil
}

func (c *recoverLogCapture) WithAttrs(_ []slog.Attr) slog.Handler { return c }

func (c *recoverLogCapture) WithGroup(_ string) slog.Handler { return c }

// panicRecords returns the ERROR records the request-recover helper logged
// for the given handler method.
func (c *recoverLogCapture) panicRecords(method string) []recoverLogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	var found []recoverLogEntry

	for _, entry := range c.entries {
		if entry.level == slog.LevelError &&
			entry.message == "request panic recovered" &&
			entry.method == method {
			found = append(found, entry)
		}
	}

	return found
}

// newRecoverLogCapture redirects slog.Default into a capture and returns it;
// the previous default is restored on cleanup. slog.Default is process-wide
// state, so tests using this helper must not run in parallel.
func newRecoverLogCapture(t *testing.T) *recoverLogCapture {
	t.Helper()

	capture := &recoverLogCapture{}

	old := slog.Default()
	slog.SetDefault(slog.New(capture))
	t.Cleanup(func() { slog.SetDefault(old) })

	return capture
}

// recoverStubServer is the minimal protocol.Server of the recover tests: it
// carries the production recover lines — Hover and Shutdown hold the same
// `defer recoverRequest(ctx, "<GoName>", &err)` the wrapped request handlers
// of Server hold — while Hover's body panics.
type recoverStubServer struct {
	protocol.UnimplementedServer
}

// Hover mirrors a wrapped request handler and panics with a plain string:
// the "offsets and text drifted" panic class is a value, not an error.
func (s *recoverStubServer) Hover(
	ctx context.Context,
	params *protocol.HoverParams,
) (_ *protocol.Hover, err error) {
	defer recoverRequest(ctx, "Hover", &err)

	panic("boom")
}

// Shutdown mirrors a wrapped request handler that stays healthy: it pushes a
// publishDiagnostics batch over the still-open connection, then answers nil.
func (s *recoverStubServer) Shutdown(ctx context.Context) (err error) {
	defer recoverRequest(ctx, "Shutdown", &err)

	client, ok := protocol.ClientFromContext(ctx)
	if !ok {
		return errors.New("recover stub: no client in context")
	}

	return client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         recoverStubURI,
		Diagnostics: []protocol.Diagnostic{},
	})
}

// recoverTestClient is the minimal editor-side peer of the recover tests: it
// records the pushed diagnostics batches (its own type, not the harness's
// recordingClient).
type recoverTestClient struct {
	protocol.UnimplementedClient

	mu      sync.Mutex
	batches []*protocol.PublishDiagnosticsParams
	pushed  chan struct{}
}

// PublishDiagnostics records the batch and wakes the waiting test.
func (c *recoverTestClient) PublishDiagnostics(
	ctx context.Context,
	params *protocol.PublishDiagnosticsParams,
) error {
	c.mu.Lock()
	c.batches = append(c.batches, params)
	c.mu.Unlock()

	select {
	case c.pushed <- struct{}{}:
	default:
	}

	return nil
}

// waitPush waits for the next diagnostics push and returns it.
func (c *recoverTestClient) waitPush(t *testing.T) *protocol.PublishDiagnosticsParams {
	t.Helper()

	select {
	case <-c.pushed:
	case <-time.After(testTimeout):
		t.Fatal("no publishDiagnostics push arrived")

		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.batches[len(c.batches)-1]
}

// TestRecoverRequest_UnitPanicBecomesInternalError covers spec S3 (R1, R2,
// R3): a panic under the recoverRequest defer stays inside the function, is
// answered as *jsonrpc2.Error InternalError naming the handler and the panic
// value, and lands as an ERROR log entry with both fields.
func TestRecoverRequest_UnitPanicBecomesInternalError(t *testing.T) {
	logs := newRecoverLogCapture(t)
	ctx := context.Background()

	err := func() (err error) {
		defer recoverRequest(ctx, "Test", &err)

		panic("boom")
	}()

	require.Error(t, err, "the panic must not leave the wrapped function")

	var rpcErr *jsonrpc2.Error
	require.ErrorAs(t, err, &rpcErr,
		"the recover path answers *jsonrpc2.Error — a plain error reaches the wire with code 0")
	assert.Equal(t, jsonrpc2.InternalError, rpcErr.Code,
		"the client sees the same -32603 the jsonrpc2 layer itself would answer")
	assert.Contains(t, rpcErr.Message, "Test panicked")
	assert.Contains(t, rpcErr.Message, "boom")

	records := logs.panicRecords("Test")
	require.Len(t, records, 1, "exactly one recovered-panic log entry")
	assert.Contains(t, records[0].panicValue, "boom")
}

// TestRecoverRequest_UnitNoPanicKeepsNil covers the second leg of spec S3
// (R3): the recover path is inert without a panic — nil stays nil, an
// explicit handler error passes through untouched, nothing is logged.
func TestRecoverRequest_UnitNoPanicKeepsNil(t *testing.T) {
	logs := newRecoverLogCapture(t)
	ctx := context.Background()

	err := func() (err error) {
		defer recoverRequest(ctx, "Test", &err)

		return nil
	}()

	require.NoError(t, err, "a healthy wrapped handler answers nil")
	assert.Empty(t, logs.panicRecords("Test"), "a healthy handler logs nothing")

	sentinel := errors.New("explicit handler error")

	err = func() (err error) {
		defer recoverRequest(ctx, "Test", &err)

		return sentinel
	}()

	require.ErrorIs(t, err, sentinel,
		"recoverRequest never touches the error of a handler that returns normally")
	assert.Empty(t, logs.panicRecords("Test"), "a clean error return logs no panic record")
}

// TestRecover_PanicSurvivalOverProtocol covers spec S1 and S2 (AC1–AC4):
// with the production recover line in a request handler, a panicking
// textDocument/hover over a real protocol.NewServer/jsonrpc2 connection
// answers -32603, two panics in a row each get their own answer, the
// connection keeps serving requests, and a push notification is delivered
// without re-initialization.
func TestRecover_PanicSurvivalOverProtocol(t *testing.T) {
	logs := newRecoverLogCapture(t)
	ctx := context.Background()

	// The transport: one os.Pipe pair per direction, joined by the harness's
	// duplexPipe (the startHarness pattern minus the stdio swap — this test
	// mounts a stub server directly, not Serve).
	clientToServerR, clientToServerW, err := os.Pipe()
	require.NoError(t, err)

	serverToClientR, serverToClientW, err := os.Pipe()
	require.NoError(t, err)

	client := &recoverTestClient{pushed: make(chan struct{}, 16)}

	_, serverConn, _ := protocol.NewServer(ctx, &recoverStubServer{},
		jsonrpc2.NewStream(duplexPipe{ReadCloser: clientToServerR, Writer: serverToClientW}))

	_, clientConn, disp := protocol.NewClient(ctx, client,
		jsonrpc2.NewStream(duplexPipe{ReadCloser: serverToClientR, Writer: clientToServerW}))

	t.Cleanup(func() {
		// EOF both read loops first so the conns reach idle, then tear the
		// connections down under a timeout guard — cleanup must never hang.
		_ = clientToServerW.Close()
		_ = serverToClientW.Close()

		done := make(chan struct{})

		go func() {
			_ = clientConn.Close()
			_ = serverConn.Close()

			close(done)
		}()

		select {
		case <-done:
		case <-time.After(testTimeout):
			t.Log("connection teardown timed out")
		}

		_ = clientToServerR.Close()
		_ = serverToClientR.Close()
	})

	hoverAt := func() error {
		_, err := disp.Hover(ctx, &protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: recoverStubURI},
				Position:     protocol.Position{Line: 0, Character: 0},
			},
		})

		return err
	}

	// spec S1: the panicking hover answers -32603 naming the handler.
	hoverErr := hoverAt()

	require.Error(t, hoverErr, "the panicking hover must answer with an error, not hang")

	var rpcErr *jsonrpc2.Error
	require.ErrorAs(t, hoverErr, &rpcErr,
		"the wire error reaches the client as *jsonrpc2.Error")
	assert.Equal(t, jsonrpc2.InternalError, rpcErr.Code)
	assert.Contains(t, rpcErr.Message, "Hover panicked")
	assert.Contains(t, rpcErr.Message, "boom")

	// spec S2: a second panicking request gets its own -32603 — the
	// connection survived the first panic.
	secondErr := hoverAt()

	require.Error(t, secondErr, "the second panicking hover must answer too")

	var secondRpcErr *jsonrpc2.Error
	require.ErrorAs(t, secondErr, &secondRpcErr)
	assert.Equal(t, jsonrpc2.InternalError, secondRpcErr.Code)
	assert.Contains(t, secondRpcErr.Message, "Hover panicked")

	// AC3: after both panics the connection still serves requests and still
	// pushes notifications — shutdown succeeds and the stub's publish
	// arrives at the client without re-initialization.
	require.NoError(t, disp.Shutdown(ctx), "the session survives two handler panics")

	batch := client.waitPush(t)
	require.NotNil(t, batch)
	assert.Equal(t, recoverStubURI, batch.URI)

	// AC2: each recovered panic landed in the log with the handler's name.
	records := logs.panicRecords("Hover")
	require.Len(t, records, 2, "both panics logged")

	for _, record := range records {
		assert.Contains(t, record.panicValue, "boom")
	}
}
