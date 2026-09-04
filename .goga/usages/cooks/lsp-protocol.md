# go.lsp.dev/protocol — LSP Server Implementation

Usage rules for implementing LSP servers with `go.lsp.dev/protocol` + `go.lsp.dev/jsonrpc2`.
API reference: https://pkg.go.dev/go.lsp.dev/protocol (LSP 3.18, types generated from the official meta-model).

## Server Bootstrap (stdio)

`protocol.NewServer` wires the jsonrpc2 connection with the union-aware codec and returns the client dispatcher.

```go
func main() {
	ctx := context.Background()

	stream := jsonrpc2.NewStream(stdio{}) // io.ReadWriteCloser over os.Stdin/os.Stdout
	srv := &Server{docs: NewStore()}

	ctx, conn, client := protocol.NewServer(ctx, srv, stream)
	srv.client = client

	<-conn.Done() // serve until the editor disconnects
}
```

## Handler Implementation

Embed `protocol.UnimplementedServer`; override only implemented methods.
Un-overridden requests return "not implemented"; un-overridden notifications are ignored.

```go
type Server struct {
	protocol.UnimplementedServer
	client protocol.Client
	docs   *Store
}

func (s *Server) Initialize(ctx context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	full := protocol.TextDocumentSyncKindFull

	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: &[]bool{true}[0],
				Change:    &full,
			},
			HoverProvider:      protocol.Boolean(true),
			CompletionProvider: &protocol.CompletionOptions{TriggerCharacters: []string{" ", "<"}},
		},
		ServerInfo: protocol.ServerInfo{Name: "aoe2-lsp"},
	}, nil
}
```

Override `Shutdown` to return `nil` (default returns not-implemented) and terminate on `Exit`.

## Document Sync (Full)

MVP uses full-document sync: on didChange replace the stored text, then push diagnostics.

```go
func (s *Server) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	for _, change := range params.ContentChanges {
		if c, ok := change.(*protocol.TextDocumentContentChangeWholeDocument); ok {
			s.docs.Put(params.TextDocument.URI, c.Text)
		}
	}

	s.publishDiagnostics(ctx, params.TextDocument.URI)

	return nil
}
```

## Diagnostics (push)

Push via the client dispatcher from `NewServer` (or `protocol.ClientFromContext(ctx)`).

```go
func (s *Server) publishDiagnostics(ctx context.Context, doc uri.URI, diags []protocol.Diagnostic) {
	_ = s.client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         doc,
		Diagnostics: diags,
	})
}

func diag(r protocol.Range, sev protocol.DiagnosticSeverity, msg string) protocol.Diagnostic {
	return protocol.Diagnostic{
		Range:    r,
		Severity: sev,
		Source:   protocol.NewOptional("aoe2-lsp"),
		Message:  msg, // plain string is a valid InlayHintTooltip union arm
	}
}
```

## Hover and Completion

`Completion` returns the sealed interface `CompletionResult` — return `*CompletionList` or `CompletionItemSlice`.

```go
func (s *Server) Hover(ctx context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	sym, ok := s.docs.Get(params.TextDocument.URI).SymbolAt(params.Position)
	if !ok {
		return nil, nil // no hover content
	}

	return &protocol.Hover{
		Contents: &protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: sym.Markdown(),
		},
	}, nil
}

func (s *Server) Completion(ctx context.Context, params *protocol.CompletionParams) (protocol.CompletionResult, error) {
	items := s.docs.Get(params.TextDocument.URI).CompletionsAt(params.Position)

	return &protocol.CompletionList{Items: items}, nil
}
```

## Union and Optional Types

- Union ("or") types are sealed interfaces: discriminate with a type switch
  (`TextDocumentContentChangeEvent`, `CompletionResult`, `HoverContents`).
- Boolean capability fields accept `protocol.Boolean(true)` or a pointer-to-options arm.
- Optional fields use `Optional[T]` / `Nullable[T]`: set via `protocol.NewOptional(v)`, read via `.Get()`.

## Logging

The package integrates with `log/slog`:
- `protocol.WithLogger(ctx, logger)` — attach logger to context
- `protocol.LoggerFromContext(ctx)` — retrieve it
- `protocol.LoggingStream(stream, w)` — dump raw protocol messages for debugging

## Constraints

- Go 1.23+ compatible; LSP 3.18 types.
- Positions are zero-based `uint32` line/character. Negotiate `PositionEncodingKind`
  in `Initialize` (default utf-16; prefer utf-8 when the client offers it).
- Custom language IDs (`aoe2rms`, `aoe2xs`) arrive in `TextDocumentItem.LanguageID`;
  the server relies on the editor config mapping file extensions to these IDs.
