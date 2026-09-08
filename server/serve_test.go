package server

import (
	"aoe2-lsp/analysis"
	"aoe2-lsp/complete"
	"aoe2-lsp/hints"
	"aoe2-lsp/kb"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
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

	var sample protocol.CompletionItem

	for _, item := range list.Items {
		if item.Kind == protocol.CompletionItemKindFunction {
			functions++

			if item.Label == "xsGetGoal" {
				sample = item
			}
		}

		assert.Nil(t, item.Documentation, "concise-items: documentation stays out")
	}

	assert.Equal(t, 204, functions, "completion returns all 204 XS functions; the client filters")

	detail, has := sample.Detail.Get()
	require.True(t, has)
	require.Equal(t, "int xsGetGoal(int)", detail)

	sort, hasSort := sample.SortText.Get()
	require.True(t, hasSort)
	assert.Equal(t, "1xsGetGoal", sort, "kb functions sort into group 1")
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

	byLabel := make(map[string]protocol.CompletionItem, len(list.Items))
	for _, item := range list.Items {
		byLabel[item.Label] = item
		assert.Nil(t, item.Documentation, "concise-items: documentation stays out")
	}

	land := byLabel["create_land"]
	assert.Equal(t, protocol.CompletionItemKindFunction, land.Kind,
		"commands map to Function — the outline-table parity, not the old Keyword")
	detail, has := land.Detail.Get()
	require.True(t, has)
	assert.Equal(t, "land_generation", detail)

	sort, has := land.SortText.Get()
	require.True(t, has)
	assert.Equal(t, "1create_land", sort)

	assert.Equal(t, protocol.CompletionItemKindField, byLabel["terrain_type"].Kind,
		"owner attributes complete alongside the section commands")

	attrSort, has := byLabel["terrain_type"].SortText.Get()
	require.True(t, has)
	assert.Equal(t, "0terrain_type", attrSort, "attributes sort before commands")

	_, leaked := byLabel["min_number_of_cliffs"]
	assert.False(t, leaked, "commands of other sections must not leak")
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

	srv := NewServer(
		store,
		analysis.NewAnalyzer(store),
		hints.NewComputer(store),
		complete.NewCompleter(store),
	)

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

// openForSignatureHelp opens a document in the harness and waits for its
// diagnostics batch (the established SignatureHelp test prologue).
func openForSignatureHelp(
	t *testing.T,
	h *lspHarness,
	docURI uri.URI,
	text string,
) {
	t.Helper()

	require.NoError(t, h.disp.DidOpen(context.Background(), &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: docURI, LanguageID: "aoe2rms", Version: 1, Text: text},
	}))
	h.waitDiagnostics(docURI)
}

// TestServe_SignatureHelpXs covers the full XS stack through the real
// stdio harness, including the DI wiring Serve built.
func TestServe_SignatureHelpXs(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	line := "\txsVectorSet(1.0, 2.0, 3.0);"
	docURI := uri.URI("file:///work/script.xs")
	openForSignatureHelp(t, h, docURI, "void test() {\n"+line+"\n}\n")

	help, err := h.disp.SignatureHelp(ctx, &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: uint32(strings.Index(line, "2.0"))},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, help)
	require.Len(t, help.Signatures, 1)
	require.NotNil(t, help.ActiveSignature)
	require.EqualValues(t, 0, *help.ActiveSignature)

	sig := help.Signatures[0]
	assert.Equal(t, "vector xsVectorSet(float x, float y, float z)", sig.Label)
	require.Len(t, sig.Parameters, 3)

	active, ok := sig.ActiveParameter.Get()
	require.True(t, ok, "the second argument is active")
	assert.EqualValues(t, 1, active)
}

// TestServe_SignatureHelpRms covers the RMS path end-to-end: the full
// kb-ordered list with the mined range, active on the first argument.
func TestServe_SignatureHelpRms(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	line := "create_elevation 3"
	docURI := uri.URI("file:///work/map.rms")
	openForSignatureHelp(t, h, docURI, "<LAND_GENERATION>\n"+line+"\n")

	help, err := h.disp.SignatureHelp(ctx, &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: uint32(strings.Index(line, "3"))},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, help)
	require.Len(t, help.Signatures, 1)

	sig := help.Signatures[0]
	assert.True(t, strings.HasPrefix(sig.Label, "create_elevation("))
	require.NotEmpty(t, sig.Parameters)
	assert.Equal(t, "[MaxHeight: number 1..16]", paramLabel(t, sig, 0))

	active, ok := sig.ActiveParameter.Get()
	require.True(t, ok)
	assert.EqualValues(t, 0, active)
}

// TestServe_SignatureHelpInlineXsBlock covers the inline-block coordinate
// template — the only place block-local translation is exercised
// end-to-end: the call is left unclosed, the cursor on the second
// argument; a wrong translation yields silence or a wrong index.
func TestServe_SignatureHelpInlineXsBlock(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	line := "void b() { xsVectorSet(1.0, 2 }"
	docURI := uri.URI("file:///work/map.rms")
	openForSignatureHelp(t, h, docURI, "#includeXS\n"+line+"\n")

	help, err := h.disp.SignatureHelp(ctx, &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: uint32(strings.Index(line, "2 "))},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, help)
	require.Len(t, help.Signatures, 1)

	sig := help.Signatures[0]
	assert.Equal(t, "vector xsVectorSet(float x, float y, float z)", sig.Label)

	active, ok := sig.ActiveParameter.Get()
	require.True(t, ok, "the active index comes from the translated position")
	assert.EqualValues(t, 1, active)
}

// TestServe_SignatureHelpStateless covers the statelessness requirement:
// the answer depends only on (document, position), never on the trigger
// context.
func TestServe_SignatureHelpStateless(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	line := "\txsVectorSet(1.0, 2.0, 3.0);"
	docURI := uri.URI("file:///work/script.xs")
	openForSignatureHelp(t, h, docURI, "void test() {\n"+line+"\n}\n")

	position := protocol.Position{Line: 1, Character: uint32(strings.Index(line, "2.0"))}

	manual, err := h.disp.SignatureHelp(ctx, &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     position,
		},
		Context: protocol.SignatureHelpContext{
			TriggerKind: protocol.SignatureHelpTriggerKindInvoked,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, manual)

	trigger := "("

	auto, err := h.disp.SignatureHelp(ctx, &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     position,
		},
		Context: protocol.SignatureHelpContext{
			TriggerKind:      protocol.SignatureHelpTriggerKindTriggerCharacter,
			TriggerCharacter: &trigger,
			IsRetrigger:      true,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, auto)

	require.Equal(t, manual, auto, "identical (document, position) — identical answers")
}

// TestServe_SignatureHelpRmsNoActiveParameter covers the RMS kind=none
// path end-to-end: the cursor on the command name renders the full list
// with no active parameter (nil, not 0, not last).
func TestServe_SignatureHelpRmsNoActiveParameter(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	line := "create_elevation 3"
	docURI := uri.URI("file:///work/map.rms")
	openForSignatureHelp(t, h, docURI, "<LAND_GENERATION>\n"+line+"\n")

	help, err := h.disp.SignatureHelp(ctx, &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 1, Character: 0},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, help)
	require.Len(t, help.Signatures, 1)

	sig := help.Signatures[0]
	assert.True(t, strings.HasPrefix(sig.Label, "create_elevation("))

	_, ok := sig.ActiveParameter.Get()
	assert.False(t, ok, "no active parameter on the command name")

	_, ok = help.ActiveParameter.Get()
	assert.False(t, ok, "the result-level field is unset too")
}

// paramLabel extracts the plain-string label of parameter i.
func paramLabel(t *testing.T, sig protocol.SignatureInformation, i int) string {
	t.Helper()
	require.Less(t, i, len(sig.Parameters))

	label, ok := sig.Parameters[i].Label.(protocol.String)
	require.True(t, ok, "parameter labels are plain strings")

	return string(label)
}

// TestServe_CompletionAPIShape verifies the four-argument NewServer
// wiring and the Completer injection.
func TestServe_CompletionAPIShape(t *testing.T) {
	store, err := kb.NewStore()
	require.NoError(t, err)

	srv := NewServer(
		store,
		analysis.NewAnalyzer(store),
		hints.NewComputer(store),
		complete.NewCompleter(store),
	)

	require.NotNil(t, srv)
	require.NotNil(t, srv.completer)
}

func TestCompletionKinds_FullDictionary(t *testing.T) {
	expected := map[string]protocol.CompletionItemKind{
		complete.KindCommand:   protocol.CompletionItemKindFunction,
		complete.KindAttribute: protocol.CompletionItemKindField,
		complete.KindConstant:  protocol.CompletionItemKindConstant,
		complete.KindFunction:  protocol.CompletionItemKindFunction,
		complete.KindVariable:  protocol.CompletionItemKindVariable,
		complete.KindParam:     protocol.CompletionItemKindVariable,
		complete.KindLocal:     protocol.CompletionItemKindVariable,
	}

	require.Len(t, completionKinds, len(expected), "every candidate kind maps")

	for kind, want := range expected {
		require.Equal(t, want, completionKinds[kind], "candidate kind %s", kind)
	}
}

func TestToCompletionItems_Render(t *testing.T) {
	items := toCompletionItems([]complete.Candidate{
		{
			Label: "create_land", Kind: complete.KindCommand,
			Detail: "land_generation", Sort: "1create_land",
		},
		{
			Label: "set_circular_base", Kind: complete.KindAttribute,
			Detail: "", Sort: "0set_circular_base",
		},
	})

	require.Len(t, items, 2)

	require.Equal(t, "create_land", items[0].Label)
	require.Equal(t, protocol.CompletionItemKindFunction, items[0].Kind)

	detail, has := items[0].Detail.Get()
	require.True(t, has)
	require.Equal(t, "land_generation", detail)

	sort, has := items[0].SortText.Get()
	require.True(t, has)
	require.Equal(t, "1create_land", sort)

	require.True(t, items[0].InsertText.IsZero(), "insert text equals the label — unset")
	require.Nil(t, items[0].Documentation, "concise-items: documentation stays out")

	require.True(t, items[1].Detail.IsZero(), "empty detail stays unset")
}

func TestToCompletionItems_EmptyNilSafe(t *testing.T) {
	items := toCompletionItems(nil)

	require.NotNil(t, items)
	require.Empty(t, items)
}

func TestCompletion_UnknownLanguage_EmptyList(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	docURI := uri.URI("file:///work/notes.txt")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: docURI, LanguageID: "plaintext", Version: 1, Text: "hello",
		},
	}))

	res, err := h.disp.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 1},
		},
	})

	require.NoError(t, err)

	list, ok := res.(*protocol.CompletionList)
	require.True(t, ok)
	require.NotNil(t, list.Items, "empty list, not a nil result")
	assert.Empty(t, list.Items)
}

func TestCompletion_RmsIntegration(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	docURI := uri.URI("file:///work/int.rms")

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
			Position:     protocol.Position{Line: 1, Character: 6},
		},
	})

	require.NoError(t, err)

	list, ok := res.(*protocol.CompletionList)
	require.True(t, ok)
	assert.False(t, list.IsIncomplete)

	var land protocol.CompletionItem

	for _, item := range list.Items {
		if item.Label == "create_land" {
			land = item
		}
	}

	require.Equal(t, protocol.CompletionItemKindFunction, land.Kind)
	require.True(t, land.InsertText.IsZero(), "insert text equals the label — unset")
}

func TestCompletion_InlineXsBlock(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	docURI := uri.URI("file:///work/inline.rms")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: docURI, LanguageID: "aoe2rms", Version: 1,
			Text: "<LAND_GENERATION>\n#includeXS\nvoid inline_fn() { }\n",
		},
	}))
	h.waitDiagnostics(docURI)

	res, err := h.disp.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 2, Character: 17},
		},
	})

	require.NoError(t, err)

	list, ok := res.(*protocol.CompletionList)
	require.True(t, ok)

	byLabel := make(map[string]protocol.CompletionItem, len(list.Items))

	for _, item := range list.Items {
		byLabel[item.Label] = item
	}

	local := byLabel["inline_fn"]
	require.Equal(t, protocol.CompletionItemKindFunction, local.Kind,
		"the inline-block function completes through unshiftPos")

	sort, has := local.SortText.Get()
	require.True(t, has)
	require.Equal(t, "0inline_fn", sort, "source symbols sort into group 0")

	require.Contains(t, byLabel, "xsGetGoal", "kb functions complete inside inline blocks")
}

func TestCompletion_Stateless_SameAnswerTwice(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	docURI := uri.URI("file:///work/stateless.xs")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: docURI, LanguageID: "aoe2xs", Version: 1, Text: "void f() { }",
		},
	}))
	h.waitDiagnostics(docURI)

	ask := func(trigger protocol.CompletionTriggerKind) []protocol.CompletionItem {
		res, err := h.disp.Completion(ctx, &protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
				Position:     protocol.Position{Line: 0, Character: 11},
			},
			Context: protocol.CompletionContext{TriggerKind: trigger},
		})
		require.NoError(t, err)

		list, ok := res.(*protocol.CompletionList)
		require.True(t, ok)

		return list.Items
	}

	assert.Equal(t,
		ask(protocol.CompletionTriggerKindInvoked),
		ask(protocol.CompletionTriggerKindTriggerForIncompleteCompletions),
		"the answer depends only on (document, position)")
}

func TestCompletion_EmptyCandidates_EmptyListNotError(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	docURI := uri.URI("file:///work/silent.rms")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: docURI, LanguageID: "aoe2rms", Version: 1, Text: "#include \"a.rms\"\n",
		},
	}))
	h.waitDiagnostics(docURI)

	res, err := h.disp.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 2},
		},
	})

	require.NoError(t, err, "silence is not an error")

	list, ok := res.(*protocol.CompletionList)
	require.True(t, ok)
	require.NotNil(t, list.Items, "empty items, not a nil list")
	require.Empty(t, list.Items)
}

// TestServer_Utf16DocumentSymbolColumns checks column translation for
// clients that did not negotiate utf-8: parser byte columns become
// UTF-16 code units in the outline (Cyrillic comment on the line).
func TestServer_Utf16DocumentSymbolColumns(t *testing.T) {
	srv, docURI := utf16ServerFixture(t, "/* Поколение */ random_placement\n")

	res, err := srv.DocumentSymbol(context.Background(), &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	})
	require.NoError(t, err)

	tree, ok := res.(protocol.DocumentSymbolSlice)
	require.True(t, ok)
	require.NotEmpty(t, tree)

	// the command node: a root child, or under the synthetic global section
	cmd := tree[0]

	if cmd.Name != "create_elevator" && len(cmd.Children) > 0 {
		cmd = cmd.Children[0]
	}

	require.Equal(t, "random_placement", cmd.Name)
	assert.Equal(t, uint32(16), cmd.Range.Start.Character, "UTF-16 units, not bytes (25)")
}

// TestServer_Utf16HoverPosition checks the inbound direction: the
// client's UTF-16 column translates into the parser's byte column.
func TestServer_Utf16HoverPosition(t *testing.T) {
	srv, docURI := utf16ServerFixture(t, "/* Поколение */ random_placement\n")

	hover, err := srv.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 0, Character: 16},
		},
	})
	require.NoError(t, err)

	require.NotNil(t, hover, "UTF-16 column 16 → byte 25 lands on random_placement")

	md, ok := hover.Contents.(*protocol.MarkupContent)
	require.True(t, ok)
	assert.Contains(t, md.Value, "random_placement")
}

// utf16ServerFixture builds a server with the utf-16 default (no
// Initialize call) and opens the document.
func utf16ServerFixture(t *testing.T, text string) (*Server, uri.URI) {
	t.Helper()

	store, err := kb.NewStore()
	require.NoError(t, err)

	srv := NewServer(
		store,
		analysis.NewAnalyzer(store),
		hints.NewComputer(store),
		complete.NewCompleter(store),
	)

	docURI := uri.URI("file:///work/ru.rms")

	require.NoError(t, srv.DidOpen(context.Background(), &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: docURI, LanguageID: "aoe2rms", Version: 1, Text: text,
		},
	}))

	return srv, docURI
}
