package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"aoe2-lsp/analysis"
	"aoe2-lsp/hints"
	"aoe2-lsp/kb"
)

// testTimeout bounds every wait for an asynchronous server reaction.
const testTimeout = 5 * time.Second

// recordingClient is the editor-side client: it records every pushed
// diagnostics batch.
type recordingClient struct {
	protocol.UnimplementedClient

	mu      sync.Mutex
	batches []*protocol.PublishDiagnosticsParams
	notify  chan struct{}
}

// PublishDiagnostics records the batch and wakes waiting tests.
func (c *recordingClient) PublishDiagnostics(
	ctx context.Context,
	params *protocol.PublishDiagnosticsParams,
) error {
	c.mu.Lock()
	c.batches = append(c.batches, params)
	c.mu.Unlock()

	select {
	case c.notify <- struct{}{}:
	default:
	}

	return nil
}

// batchAt returns the recorded batch at index i.
func (c *recordingClient) batchAt(i int) *protocol.PublishDiagnosticsParams {
	c.mu.Lock()
	defer c.mu.Unlock()

	if i >= len(c.batches) {
		return nil
	}

	return c.batches[i]
}

// batchesSnapshot returns a copy of the recorded batches.
func (c *recordingClient) batchesSnapshot() []*protocol.PublishDiagnosticsParams {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]*protocol.PublishDiagnosticsParams(nil), c.batches...)
}

// batchCount returns the number of batches recorded so far.
func (c *recordingClient) batchCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.batches)
}

// duplexPipe joins a read and a write end into one transport.
type duplexPipe struct {
	io.ReadCloser
	io.Writer
}

// Close closes both transport halves.
func (d duplexPipe) Close() error {
	if err := d.ReadCloser.Close(); err != nil {
		return err
	}

	if c, ok := d.Writer.(io.Closer); ok {
		return c.Close()
	}

	return nil
}

// lspHarness drives the real Serve entrypoint over swapped stdio pipes: a
// protocol client talks to the server over an in-process pipe pair.
type lspHarness struct {
	t        *testing.T
	disp     protocol.Server
	client   *recordingClient
	done     chan error
	consumed int
	exitOnce sync.Once
	exitErr  error
}

// awaitExit returns the server result exactly once, however many test and
// cleanup paths wait for it.
func (h *lspHarness) awaitExit() error {
	h.exitOnce.Do(func() {
		select {
		case h.exitErr = <-h.done:
		case <-time.After(testTimeout):
			h.exitErr = fmt.Errorf("server did not exit within %s", testTimeout)
		}
	})

	return h.exitErr
}

// startHarness swaps os.Stdin/os.Stdout for pipes, starts Serve and connects
// a protocol client to the other pipe ends.
func startHarness(t *testing.T) *lspHarness {
	t.Helper()

	reqR, reqW, err := os.Pipe()
	require.NoError(t, err)

	respR, respW, err := os.Pipe()
	require.NoError(t, err)

	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = reqR, respW

	h := &lspHarness{
		t:      t,
		done:   make(chan error, 1),
		client: &recordingClient{notify: make(chan struct{}, 16)},
	}

	go func() { h.done <- Serve(context.Background()) }()

	_, _, disp := protocol.NewClient(
		context.Background(),
		h.client,
		jsonrpc2.NewStream(duplexPipe{ReadCloser: respR, Writer: reqW}),
	)
	h.disp = disp

	t.Cleanup(func() {
		// EOF the server transport if it is still running, then wait before
		// restoring stdio so the server never touches the real files.
		_ = reqW.Close()

		if err := h.awaitExit(); err != nil &&
			!errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
			t.Logf("server exited: %v", err)
		}

		os.Stdin, os.Stdout = oldIn, oldOut
		_ = respR.Close()
	})

	return h
}

// waitDiagnostics waits for the next diagnostics batch of the document
// (skipping batches already seen by earlier calls).
func (h *lspHarness) waitDiagnostics(docURI uri.URI) *protocol.PublishDiagnosticsParams {
	h.t.Helper()

	from := h.consumed
	deadline := time.After(testTimeout)

	for {
		// Advance only past batches we have actually examined, so a batch
		// recorded while we wait is never skipped.
		if b := h.client.batchAt(from); b != nil {
			if b.URI == docURI {
				h.consumed = from + 1

				return b
			}

			from++

			continue
		}

		select {
		case <-h.client.notify:
		case <-deadline:
			uris := make([]string, 0, h.client.batchCount())

			for _, b := range h.client.batchesSnapshot() {
				uris = append(uris, string(b.URI))
			}

			h.t.Fatalf("no diagnostics batch received for %s (recorded %d: %v)",
				docURI, h.client.batchCount(), uris)

			return nil
		}
	}
}

// diagnosticCodes extracts the string codes of a batch.
func diagnosticCodes(t *testing.T, params *protocol.PublishDiagnosticsParams) []string {
	t.Helper()

	codes := make([]string, 0, len(params.Diagnostics))

	for _, d := range params.Diagnostics {
		code, ok := d.Code.(protocol.String)
		require.True(t, ok, "diagnostic code is not a string: %v", d.Code)

		codes = append(codes, string(code))
	}

	return codes
}

func TestServe_InitializeNegotiatesUTF8(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	res, err := h.disp.Initialize(ctx, &protocol.InitializeParams{
		Capabilities: protocol.ClientCapabilities{
			General: &protocol.GeneralClientCapabilities{
				PositionEncodings: []protocol.PositionEncodingKind{
					protocol.PositionEncodingKindUTF8,
				},
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "aoe2-lsp", res.ServerInfo.Name)
	assert.Equal(t, protocol.PositionEncodingKindUTF8, res.Capabilities.PositionEncoding)

	sync, ok := res.Capabilities.TextDocumentSync.(*protocol.TextDocumentSyncOptions)
	require.True(t, ok)
	require.NotNil(t, sync.OpenClose)
	assert.True(t, *sync.OpenClose)
	require.NotNil(t, sync.Change)
	assert.Equal(t, protocol.TextDocumentSyncKindFull, *sync.Change)
}

func TestServe_InitializeDefaultsToUTF16(t *testing.T) {
	h := startHarness(t)

	res, err := h.disp.Initialize(context.Background(), &protocol.InitializeParams{})

	require.NoError(t, err)
	assert.Equal(t, protocol.PositionEncodingKind(""), res.Capabilities.PositionEncoding)
}

func TestServe_RmsDiagnosticsAndClear(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	docURI := uri.URI("file:///work/map.rms")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        docURI,
			LanguageID: "aoe2rms",
			Version:    1,
			Text:       "<PLAYER_SETUP>\nnonexistent_command 1 2\n",
		},
	}))

	batch := h.waitDiagnostics(docURI)

	require.NotEmpty(t, batch.Diagnostics, "unknown command must be reported")
	assert.Contains(t, diagnosticCodes(t, batch), "unknown-command")

	for _, d := range batch.Diagnostics {
		source, ok := d.Source.Get()
		require.True(t, ok)
		assert.Equal(t, "aoe2-lsp", source)
		assert.Contains(t, string(d.Message.(protocol.String)), "nonexistent_command")
	}

	// Fixing the document must clear the diagnostic.
	require.NoError(t, h.disp.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: docURI},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			&protocol.TextDocumentContentChangeWholeDocument{
				Text: "<PLAYER_SETUP>\nrandom_placement\n",
			},
		},
	}))

	batch = h.waitDiagnostics(docURI)

	for _, code := range diagnosticCodes(t, batch) {
		assert.NotEqual(t, "unknown-command", code)
	}
}

func TestServe_DidCloseClearsDiagnostics(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	docURI := uri.URI("file:///work/close.rms")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: docURI, LanguageID: "aoe2rms", Version: 1,
			Text: "<LAND_GENERATION>\nnonexistent_command\n",
		},
	}))
	h.waitDiagnostics(docURI)

	require.NoError(t, h.disp.DidClose(ctx, &protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	}))

	batch := h.waitDiagnostics(docURI)

	assert.Empty(t, batch.Diagnostics)
}

func TestServe_HoverXsFunction(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	docURI := uri.URI("file:///work/script.xs")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: docURI, LanguageID: "aoe2xs", Version: 1,
			Text: "void test() {\n\tint seed = xsGetMapSeed();\n}\n",
		},
	}))
	h.waitDiagnostics(docURI)

	hover, err := h.disp.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 15},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, hover, "hover over xsGetMapSeed must return content")

	contents, ok := hover.Contents.(*protocol.MarkupContent)
	require.True(t, ok)
	assert.Contains(t, contents.Value, "xsGetMapSeed")
	assert.Contains(t, contents.Value, "(")
}

func TestServe_HoverNothingUnderCursor(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	docURI := uri.URI("file:///work/empty.rms")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: docURI, LanguageID: "aoe2rms", Version: 1, Text: "\n\n",
		},
	}))
	h.waitDiagnostics(docURI)

	hover, err := h.disp.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})

	require.NoError(t, err)
	assert.Nil(t, hover)
}

func TestServe_CompletionAllXsFunctions(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	docURI := uri.URI("file:///work/comp.xs")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: docURI, LanguageID: "aoe2xs", Version: 1, Text: "\n",
		},
	}))
	h.waitDiagnostics(docURI)

	res, err := h.disp.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})

	require.NoError(t, err)

	list, ok := res.(*protocol.CompletionList)
	require.True(t, ok)

	functions := 0

	for _, item := range list.Items {
		if item.Kind == protocol.CompletionItemKindFunction {
			functions++
		}
	}

	assert.Equal(t, 204, functions, "completion must return all 204 XS functions")
}

func TestServe_CompletionRmsScopedToSection(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	docURI := uri.URI("file:///work/sections.rms")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: docURI, LanguageID: "aoe2rms", Version: 1,
			Text: "<LAND_GENERATION>\ncreate_land\n",
		},
	}))
	h.waitDiagnostics(docURI)

	res, err := h.disp.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 3},
		},
	})

	require.NoError(t, err)

	list, ok := res.(*protocol.CompletionList)
	require.True(t, ok)

	labels := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		labels = append(labels, item.Label)
	}

	assert.Contains(t, labels, "create_land")
	assert.NotContains(t, labels, "min_number_of_cliffs", "commands of other sections must not leak")
}

func TestServe_ShutdownExit(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	require.NoError(t, h.disp.Shutdown(ctx))
	require.NoError(t, h.disp.Exit(ctx))

	require.NoError(t, h.awaitExit())
}

func TestServerNavigation_IntegrationStdio(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	initRes, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	require.Equal(t, protocol.Boolean(true), initRes.Capabilities.DefinitionProvider)
	require.Equal(t, protocol.Boolean(true), initRes.Capabilities.ReferencesProvider)
	require.Equal(t, protocol.Boolean(true), initRes.Capabilities.DocumentSymbolProvider)

	xsURI := uri.URI("file:///work/nav.xs")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        xsURI,
			LanguageID: "aoe2xs",
			Version:    1,
			Text:       "void f() {}\nvoid g() { f(); }\n",
		},
	}))
	h.waitDiagnostics(xsURI)

	callPos := protocol.Position{Line: 1, Character: uint32(len("void g() { "))}

	defRes, err := h.disp.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: xsURI},
			Position:     callPos,
		},
	})
	require.NoError(t, err)

	loc, ok := defRes.(*protocol.Location)
	require.True(t, ok, "definition resolves to a single Location")
	require.Equal(t, xsURI, loc.URI)
	require.Equal(t, uint32(0), loc.Range.Start.Line)
	require.Equal(t, uint32(len("void ")), loc.Range.Start.Character)

	refRes, err := h.disp.References(ctx, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: xsURI},
			Position:     callPos,
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})
	require.NoError(t, err)
	require.Len(t, refRes, 2, "declaration plus call")

	rmsURI := uri.URI("file:///work/nav.rms")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        rmsURI,
			LanguageID: "aoe2rms",
			Version:    1,
			Text:       "<LAND_GENERATION>\ncreate_player_lands {\n	land_percent 32\n}\n</LAND_GENERATION>\n",
		},
	}))
	h.waitDiagnostics(rmsURI)

	symRes, err := h.disp.DocumentSymbol(ctx, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: rmsURI},
	})
	require.NoError(t, err)

	tree, ok := symRes.(protocol.DocumentSymbolSlice)
	require.True(t, ok, "hierarchical documentSymbol arm")
	require.Len(t, tree, 1)
	require.Equal(t, "land_generation", tree[0].Name)
	require.Equal(t, protocol.SymbolKindModule, tree[0].Kind)
	require.Len(t, tree[0].Children, 1)
	require.Equal(t, "create_player_lands", tree[0].Children[0].Name)
}

// TestServe_SignatureHelpAPIShape pins the handler contract surface: the
// three-argument NewServer wiring and the SignatureHelp method shape.
func TestServe_SignatureHelpAPIShape(t *testing.T) {
	store, err := kb.NewStore()
	require.NoError(t, err)

	srv := NewServer(store, analysis.NewAnalyzer(store), hints.NewComputer(store))

	require.NotNil(t, srv)

	h := startHarness(t)
	ctx := context.Background()

	_, err = h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	docURI := uri.URI("file:///work/blank.rms")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: docURI, LanguageID: "aoe2rms", Version: 1, Text: "\n\n"},
	}))
	h.waitDiagnostics(docURI)

	help, err := h.disp.SignatureHelp(ctx, &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})

	require.NoError(t, err)
	assert.Nil(t, help, "blank document: silence is a nil result")
}

// TestServe_InitializeAdvertisesSignatureHelp covers the capability
// contract: triggers "(" and "," only — disjoint from completion's
// " " and "<"; existing capabilities unchanged.
func TestServe_InitializeAdvertisesSignatureHelp(t *testing.T) {
	h := startHarness(t)

	res, err := h.disp.Initialize(context.Background(), &protocol.InitializeParams{})
	require.NoError(t, err)

	caps := res.Capabilities
	require.NotNil(t, caps.SignatureHelpProvider)
	require.Equal(t, []string{"(", ","}, caps.SignatureHelpProvider.TriggerCharacters)

	require.NotNil(t, caps.HoverProvider, "existing capabilities stay intact")
	require.NotNil(t, caps.CompletionProvider)
	require.NotNil(t, caps.DefinitionProvider)
	require.NotNil(t, caps.ReferencesProvider)
	require.NotNil(t, caps.DocumentSymbolProvider)
	require.NotNil(t, caps.TextDocumentSync)
}

// TestServe_SignatureHelpSilence covers the nil,nil silence convention
// through the protocol layer: a nullable result, not an error and not an
// empty SignatureHelp.
func TestServe_SignatureHelpSilence(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	docURI := uri.URI("file:///work/empty.rms")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: docURI, LanguageID: "aoe2rms", Version: 1, Text: "\n\n"},
	}))
	h.waitDiagnostics(docURI)

	help, err := h.disp.SignatureHelp(ctx, &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})

	require.NoError(t, err)
	require.Nil(t, help)
}
