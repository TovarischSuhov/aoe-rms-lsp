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

## Completion Items

`Completion` returns the sealed interface `CompletionResult` — return
`*protocol.CompletionList` (or `protocol.CompletionItemSlice{}`).
Empty candidates → `&protocol.CompletionList{}` with empty items, never nil:
completion is not an error case, editors handle an empty list gracefully.

```go
func (s *Server) Completion(ctx context.Context, params *protocol.CompletionParams) (protocol.CompletionResult, error) {
	items := s.computer.Items(doc, params.Position)

	return &protocol.CompletionList{IsIncomplete: false, Items: items}, nil
}
```

Item rules:
- **`Label`** is the candidate text; omit `InsertText` when insertion equals the
  label (the common case).
- **`InsertTextFormat`**: plain text only — no snippet syntax (`$0`, `${...}`);
  snippet infrastructure is out of scope.
- **`Kind`** maps the candidate's nature to `protocol.CompletionItemKind`
  constants (`Function`, `Constant`, `Variable`, `Field`, `Keyword`, `Value`).
- **`Detail`** is one short line (arg range "0..100", param summary); extended
  prose stays out of completion — concise-items rule, mirroring concise-hints.
- **`SortText`** defines stable server-side ordering when alphabetical is not
  desired (context-appropriate candidates first); clients ignoring it fall
  back to label sorting.
- The provider is **stateless**: items depend only on (document, position),
  never on `params.Context` trigger info or previous responses — same rule as
  `SignatureHelp`.
- Filtering by prefix is the client's job; the server returns the full
  context-appropriate candidate set (bounded by context, not by typed prefix).

## Signature Help

Advertise in `Initialize`: `SignatureHelpProvider: &protocol.SignatureHelpOptions{TriggerCharacters: []string{"(", ","}}`.
Trigger characters are `(` and `,` only — RMS value keystrokes must not spam the widget.

Override `SignatureHelp(ctx, params *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error)`.
For no-hint situations return `nil, nil` (nullable result) — same silence convention as `Hover`.

```go
func (s *Server) SignatureHelp(ctx context.Context, params *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error) {
	hint, ok := s.docs.Get(params.TextDocument.URI).SignatureAt(params.Position)
	if !ok {
		return nil, nil // trust rule: silence, never a guessed hint
	}

	return &protocol.SignatureHelp{
		Signatures:      []protocol.SignatureInformation{hint.Sig}, // exactly one
		ActiveSignature: &[]uint32{0}[0],
		ActiveParameter: hint.Active, // *uint32; nil when no argument is active
	}, nil
}
```

Rules:
- **Single signature per call site**: `Signatures` has exactly one element; `ActiveSignature` is always 0.
- **`ActiveParameter` is `*uint32`** — set it to the cursor's argument index; leave nil when the cursor
  is off any argument or the index is beyond the declared parameters. **Never clamp** to the last
  parameter — a wrong highlight is worse than none.
- **`ParameterInformation.Label` is a plain string** (`"float x"`, `"%: percent 0..99"`);
  optional parameters render in square brackets (`"[float z]"`). Omit `Documentation` — extended
  descriptions stay in hover (concise-hints rule).
- `params.Context` (`TriggerKind`, `TriggerCharacter`, `IsRetrigger`, `ActiveSignatureHelp`) describes
  why the request fired. The provider is **stateless**: the answer must depend only on
  (document, position), never on the trigger context or previous answers.

## Navigation (Definition / References / DocumentSymbol)

Advertise in `Initialize`: `DefinitionProvider`, `ReferencesProvider`,
`DocumentSymbolProvider` — each `protocol.Boolean(true)`.

`Definition` returns the sealed interface `DefinitionResult` — return
`*protocol.Location` (single site) or `protocol.LocationSlice{}` when nothing
resolves (empty slice, not nil).

```go
func (s *Server) Definition(ctx context.Context, params *protocol.DefinitionParams) (protocol.DefinitionResult, error) {
	loc, ok := s.definitionAt(params.TextDocument.URI, params.Position)
	if !ok {
		return protocol.LocationSlice{}, nil
	}
	return &loc, nil
}
```

`References` returns plain `[]protocol.Location`; honor
`params.Context.IncludeDeclaration` — prepend the declaration range when true.

`DocumentSymbol` returns `DocumentSymbolResult` — arms: `DocumentSymbolSlice`
(hierarchical, preferred) or `SymbolInformationSlice` (flat). With hierarchical
`DocumentSymbol`:
- `Range` encloses the whole construct (decl header + body);
  `SelectionRange` points at the identifier name and **must be contained
  in `Range`** (spec requirement, clients reject otherwise);
- `Children` nests (RMS: section → statements; XS: nested where available);
- map kinds via `protocol.SymbolKind` constants (`Function`, `Constant`,
  `Variable`, ...).

## Cross-file Navigation Results

Definition/References may return locations in files other than the queried
document. Build `protocol.Location` with the target file's URI — the editor
opens the file at the given range on demand; the target is NOT required to be
an open document.

```go
loc := protocol.Location{
	URI:   uri.File(targetPath), // filesystem path → URI (go.lsp.dev/uri)
	Range: r,
}
```

## Disk-backed Documents (include resolution)

Documents referenced by the open file (e.g. `#include` closures) are read from
disk on demand and cached by URI:

- Convert the including document's URI via `URI.FsPath()` (guard non-file
  schemes with `URI.IsFile()`); resolve relative include paths with
  `filepath.Join(filepath.Dir(...), ...)`.
- **Editor state wins**: if the URI is open in the doc store, its text is
  authoritative — the disk copy never overrides it.
- Missing or unreadable include files surface as publishDiagnostics errors
  covering the directive's range; degrade without panicking.
- Files changed outside the editor are not tracked (no file watcher); cache
  refresh happens when the file itself changes via didChange.

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

- Go 1.26+ compatible; LSP 3.18 types.
- Positions are zero-based `uint32` line/character. Negotiate `PositionEncodingKind`
  in `Initialize` (default utf-16; prefer utf-8 when the client offers it).
- Custom language IDs (`aoe2rms`, `aoe2xs`) arrive in `TextDocumentItem.LanguageID`;
  the server relies on the editor config mapping file extensions to these IDs.
