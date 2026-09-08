package corpus

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// fakeEnv selects the fake server mode in the re-executed test binary.
const fakeEnv = "AOE2_CORPUS_FAKE"

// TestMain routes the re-executed test binary into the fake LSP server —
// the corpus runner spawns os.Args[0] with fakeEnv set.
func TestMain(m *testing.M) {
	mode := os.Getenv(fakeEnv)

	if mode != "" {
		runFakeServer(mode)
		os.Exit(0)
	}

	os.Exit(m.Run())
}

// corpusFixture builds a corpus directory with two scripts and one
// ignored file.
func corpusFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	require.NoError(t, os.Mkdir(filepath.Join(dir, "deep"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "map.rms"),
		[]byte("<PLAYER_SETUP>\n\n<LAND_GENERATION>\ncreate_land\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deep", "lib.xs"),
		[]byte("void f() {\n  int x = 1;\n}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"),
		[]byte("ignored by the harness\n"), 0o644))

	return dir
}

// runCorpus runs the fixture against the re-executed test binary in the
// given fake mode.
func runCorpus(t *testing.T, mode string) Report {
	t.Helper()

	t.Setenv(fakeEnv, mode)

	dir := corpusFixture(t)

	report, err := NewRunner(os.Args[0]).Run(context.Background(), dir)
	require.NoError(t, err)

	return report
}

func TestRunner_CleanSessionIsOK(t *testing.T) {
	report := runCorpus(t, "ok")

	require.Len(t, report.Files, 2)
	assert.Equal(t, 0, report.HardFailures)

	byPath := map[string]FileResult{}
	for _, f := range report.Files {
		byPath[f.Path] = f
	}

	// Deterministic lexical order: deep/lib.xs sorts before map.rms.
	assert.Equal(t, "deep/lib.xs", report.Files[0].Path)
	assert.Equal(t, "map.rms", report.Files[1].Path)

	for _, f := range report.Files {
		assert.Equal(t, statusOK, f.Status)
		assert.Equal(t, 1, f.Diagnostics)
		assert.Equal(t, 0, f.RequestErrors)
	}

	// map.rms has 5 lines → 5 sampled positions: 5 hover + 5
	// signature_help + 2 completion + 1 document_symbol.
	assert.Equal(t, 13, byPath["map.rms"].Requests)
}

func TestRunner_PanicIsClassified(t *testing.T) {
	// The recovered DidOpen publishes nothing — tighten the diagnostics
	// wait so the panic surfaces via the sampled requests instead.
	oldWait := diagWait
	diagWait = 500 * time.Millisecond

	t.Cleanup(func() { diagWait = oldWait })

	report := runCorpus(t, "panic")

	require.Len(t, report.Files, 2)
	assert.Equal(t, 2, report.HardFailures)

	for _, f := range report.Files {
		assert.Equal(t, statusPanic, f.Status)
		assert.Contains(t, f.Detail, "corpus fake panic")
	}

	assert.Contains(t, report.Summary(), "FAIL")
	assert.Contains(t, report.Summary(), "panic 2")
}

func TestRunner_EarlyExitIsClassified(t *testing.T) {
	report := runCorpus(t, "crash")

	require.Len(t, report.Files, 2)

	for _, f := range report.Files {
		assert.Equal(t, statusExit, f.Status)
		assert.Contains(t, f.Detail, "exit code 7")
	}
}

func TestRunner_HangIsClassified(t *testing.T) {
	oldTimeout := sessionTimeout
	sessionTimeout = 500 * time.Millisecond

	t.Cleanup(func() { sessionTimeout = oldTimeout })

	report := runCorpus(t, "hang")

	require.Len(t, report.Files, 2)

	for _, f := range report.Files {
		assert.Equal(t, statusTimeout, f.Status)
	}
}

func TestRunner_BrokenTransportIsClassified(t *testing.T) {
	report := runCorpus(t, "garbage")

	require.Len(t, report.Files, 2)

	for _, f := range report.Files {
		assert.Equal(t, statusTransport, f.Status)
	}
}

func TestRunner_InfraErrors(t *testing.T) {
	t.Run("missing binary", func(t *testing.T) {
		dir := corpusFixture(t)

		_, err := NewRunner(filepath.Join(t.TempDir(), "no-such-bin")).
			Run(context.Background(), dir)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "no-such-bin")
	})

	t.Run("missing dir", func(t *testing.T) {
		_, err := NewRunner(os.Args[0]).
			Run(context.Background(), filepath.Join(t.TempDir(), "no-such-dir"))

		require.Error(t, err)
	})

	t.Run("dir without scripts", func(t *testing.T) {
		_, err := NewRunner(os.Args[0]).Run(context.Background(), t.TempDir())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "no .rms/.xs files")
	})
}

func TestSamplePositions_Deterministic(t *testing.T) {
	text := "line one\n  indented\n\nlast\n"

	first := samplePositions(text, 4)
	second := samplePositions(text, 4)

	require.NotEmpty(t, first)
	assert.Equal(t, first, second)

	for _, pos := range first {
		assert.LessOrEqual(t, pos.Line, uint32(4))
	}
}

// fakeServer is the editor-facing server used by the re-executed test
// binary; mode selects the failure being simulated.
type fakeServer struct {
	protocol.UnimplementedServer

	mode string
}

// runFakeServer serves the fake LSP server over the process stdio until
// the connection dies.
func runFakeServer(mode string) {
	if mode == "garbage" {
		// Speak broken bytes, then hold the process open until stdin
		// closes: the client sees a dead connection with a live process.
		_, _ = os.Stdout.WriteString("this is not lsp\r\n\r\n")
		_, _ = io.Copy(io.Discard, os.Stdin)

		return
	}

	_, conn, _ := protocol.NewServer(
		context.Background(),
		&fakeServer{mode: mode},
		jsonrpc2.NewStream(osStdio{}),
	)

	<-conn.Done()
}

// osStdio adapts the process stdin/stdout pair to a jsonrpc2 stream.
type osStdio struct{}

// Read reads protocol bytes from stdin.
func (osStdio) Read(p []byte) (int, error) { return os.Stdin.Read(p) }

// Write writes protocol bytes to stdout.
func (osStdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }

// Close closes both transport files.
func (osStdio) Close() error {
	if err := os.Stdin.Close(); err != nil {
		return err
	}

	return os.Stdout.Close()
}

// Initialize declares a minimal capability set.
func (s *fakeServer) Initialize(
	ctx context.Context,
	params *protocol.InitializeParams,
) (*protocol.InitializeResult, error) {
	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{},
		ServerInfo:   protocol.ServerInfo{Name: "corpus-fake"},
	}, nil
}

// DidOpen simulates the selected mode or publishes one diagnostic.
func (s *fakeServer) DidOpen(
	ctx context.Context,
	params *protocol.DidOpenTextDocumentParams,
) (err error) {
	// The real server recovers notification panics into a logged,
	// degraded success; the fake mirrors that so the panic path flows
	// through the follow-up requests.
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "notification panic recovered method=DidOpen panic=%v\n", r)
			err = nil
		}
	}()

	switch s.mode {
	case "panic":
		panic("corpus fake panic")
	case "crash":
		os.Exit(7)
	case "hang":
		time.Sleep(time.Hour)

		return nil
	}

	client, ok := protocol.ClientFromContext(ctx)
	if !ok {
		return nil
	}

	return client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI: params.TextDocument.URI,
		Diagnostics: []protocol.Diagnostic{{
			Message: protocol.String("fake"),
			Range:   protocol.Range{},
		}},
	})
}

// Hover answers nothing — the silence convention; in panic mode it
// reproduces the same parser panic the notification recovered from.
func (s *fakeServer) Hover(
	ctx context.Context,
	params *protocol.HoverParams,
) (*protocol.Hover, error) {
	if s.mode == "panic" {
		panic("corpus fake panic")
	}

	return nil, nil
}

// SignatureHelp answers nothing — the silence convention.
func (s *fakeServer) SignatureHelp(
	ctx context.Context,
	params *protocol.SignatureHelpParams,
) (*protocol.SignatureHelp, error) {
	return nil, nil
}

// Completion answers an empty list — empty is not an error.
func (s *fakeServer) Completion(
	ctx context.Context,
	params *protocol.CompletionParams,
) (protocol.CompletionResult, error) {
	return &protocol.CompletionList{Items: []protocol.CompletionItem{}}, nil
}

// DocumentSymbol answers an empty outline.
func (s *fakeServer) DocumentSymbol(
	ctx context.Context,
	params *protocol.DocumentSymbolParams,
) (protocol.DocumentSymbolResult, error) {
	return protocol.DocumentSymbolSlice{}, nil
}

// Shutdown acknowledges the shutdown request.
func (s *fakeServer) Shutdown(ctx context.Context) error {
	return nil
}

// Exit terminates the process — the real server exits the same way.
func (s *fakeServer) Exit(ctx context.Context) error {
	os.Exit(0)

	return nil
}
