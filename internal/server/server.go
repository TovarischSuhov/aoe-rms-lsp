package server

import (
	"aoe2-lsp/internal/analysis"
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/complete"
	"aoe2-lsp/internal/hints"
	"aoe2-lsp/internal/include"
	"aoe2-lsp/internal/kb"
	"aoe2-lsp/internal/rms"
	"aoe2-lsp/internal/xs"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/go-json-experiment/json/jsontext"
)

// serverName identifies the server in Initialize responses and diagnostics.
const serverName = "aoe2-lsp"

// Server is the LSP server: the protocol surface over the kb, rms, xs,
// analysis and hints cells.
type Server struct {
	protocol.UnimplementedServer

	store     *kb.Store
	analyzer  *analysis.Analyzer
	computer  *hints.Computer
	completer *complete.Completer
	docs      *DocStore
	resolver  *include.Resolver
	exit      chan struct{}
	exitOnce  sync.Once

	utf16 atomic.Bool // protocol default: translate byte columns unless utf-8 is negotiated

	clientConfigCap atomic.Bool // the client answers workspace/configuration
	clientWatchCap  atomic.Bool // the client accepts watcher registration
	settings        atomic.Pointer[settingsState]
}

// settingsState is the applied "aoe2lsp" configuration section.
type settingsState struct {
	// severityOverrides maps a diagnostic code to a protocol severity;
	// the zero value suppresses the diagnostic ("none").
	severityOverrides map[string]protocol.DiagnosticSeverity

	// includeRoots are the extra absolute include search roots.
	includeRoots []string
}

// severityFor resolves the effective severity for a code: the override
// when configured (zero = suppress), the diagnostic's own otherwise.
func (st *settingsState) severityFor(code string, own int) (protocol.DiagnosticSeverity, bool) {
	if st == nil {
		return protocol.DiagnosticSeverity(own), true
	}

	sev, ok := st.severityOverrides[code]
	if !ok {
		return protocol.DiagnosticSeverity(own), true
	}

	return sev, sev != 0
}

// settingsJSON mirrors the "aoe2lsp" section schema.
type settingsJSON struct {
	Diagnostics struct {
		SeverityOverrides map[string]string `json:"severityOverrides"`
	} `json:"diagnostics"`
	IncludeRoots []string `json:"includeRoots"`
}

// applySettings parses one raw settings payload (a pull item or the
// didChangeConfiguration settings), stores the result and pushes the
// include roots into the resolver. Unknown severity names are ignored
// with a WARN log (an unknown code simply never matches); diagnostics
// are republished afterwards by the caller-side handlers.
func (s *Server) applySettings(ctx context.Context, raw any) {
	if raw == nil {
		return
	}

	var parsed settingsJSON

	b, err := json.Marshal(raw)
	if err != nil {
		slog.WarnContext(ctx, "settings marshal failed", "err", err)

		return
	}

	if err := json.Unmarshal(b, &parsed); err != nil {
		slog.WarnContext(ctx, "settings parse failed", "err", err)

		return
	}

	overrides := make(map[string]protocol.DiagnosticSeverity, len(parsed.Diagnostics.SeverityOverrides))

	for code, name := range parsed.Diagnostics.SeverityOverrides {
		sev, ok := severityByName(name)
		if !ok {
			slog.WarnContext(ctx, "settings: unknown severity", "code", code, "severity", name)

			continue
		}

		overrides[code] = sev
	}

	state := &settingsState{severityOverrides: overrides, includeRoots: parsed.IncludeRoots}
	s.settings.Store(state)
	s.resolver.SetRoots(state.includeRoots)

	slog.DebugContext(ctx, "settings applied",
		"overrides", len(overrides), "roots", len(state.includeRoots))
}

// severityByName maps a settings severity name to its protocol value;
// "none" maps to zero (suppress), unknown names fail.
func severityByName(name string) (protocol.DiagnosticSeverity, bool) {
	switch name {
	case "error":
		return protocol.DiagnosticSeverityError, true
	case "warning":
		return protocol.DiagnosticSeverityWarning, true
	case "info":
		return protocol.DiagnosticSeverityInformation, true
	case "hint":
		return protocol.DiagnosticSeverityHint, true
	case "none":
		return 0, true
	}

	return 0, false
}

// Initialized runs the post-initialize pulls: the configuration
// section (when the client answers workspace/configuration) and the
// file-watcher registration (when the client accepts dynamic
// registration for didChangeWatchedFiles).
func (s *Server) Initialized(
	ctx context.Context,
	params *protocol.InitializedParams,
) error {
	client, hasClient := protocol.ClientFromContext(ctx)

	if s.clientConfigCap.Load() && hasClient {
		s.pullSettings(ctx, client)
	}

	if s.clientWatchCap.Load() && hasClient {
		s.registerWatchers(ctx, client)
	}

	if s.clientConfigCap.Load() {
		s.republishAll(ctx)
	}

	return nil
}

// pullSettings asks the client for the "aoe2lsp" section and applies
// it through the shared settings path.
func (s *Server) pullSettings(ctx context.Context, client protocol.Client) {
	section := "aoe2lsp"

	items, err := client.Configuration(ctx, &protocol.ConfigurationParams{
		Items: []protocol.ConfigurationItem{{Section: &section}},
	})
	if err != nil {
		slog.WarnContext(ctx, "configuration pull failed", "err", err)

		return
	}

	if len(items) > 0 {
		s.applySettings(ctx, items[0])
	}
}

// registerWatchers asks the client to watch the RMS/XS sources and
// forward change events.
func (s *Server) registerWatchers(ctx context.Context, client protocol.Client) {
	watchAll := protocol.WatchKindCreate | protocol.WatchKindChange | protocol.WatchKindDelete

	options, err := json.Marshal(protocol.DidChangeWatchedFilesRegistrationOptions{
		Watchers: []protocol.FileSystemWatcher{
			{GlobPattern: protocol.Pattern("**/*.rms"), Kind: watchAll},
			{GlobPattern: protocol.Pattern("**/*.xs"), Kind: watchAll},
		},
	})
	if err != nil {
		slog.WarnContext(ctx, "watcher options marshal failed", "err", err)

		return
	}

	err = client.RegisterCapability(ctx, &protocol.RegistrationParams{
		Registrations: []protocol.Registration{{
			ID:              "aoe2lsp/watched-files",
			Method:          "workspace/didChangeWatchedFiles",
			RegisterOptions: jsontext.Value(options),
		}},
	})
	if err != nil {
		slog.WarnContext(ctx, "watcher registration failed", "err", err)

		return
	}

	slog.DebugContext(ctx, "watchers registered")
}

// DidChangeWatchedFiles force-reloads the changed disk files (even
// with an unchanged size/mtime fingerprint) and republishes the
// diagnostics of every open document — the include closure may have
// shifted under the editor.
func (s *Server) DidChangeWatchedFiles(
	ctx context.Context,
	params *protocol.DidChangeWatchedFilesParams,
) (err error) {
	defer recoverNotification(ctx, "DidChangeWatchedFiles", &err)

	paths := make([]string, 0, len(params.Changes))

	for _, change := range params.Changes {
		if change.URI.IsFile() {
			paths = append(paths, change.URI.FsPath())
		}
	}

	s.resolver.Drop(paths)
	s.republishAll(ctx)

	return nil
}

// DidChangeConfiguration applies pushed settings and republishes every
// open document — severity overrides change published diagnostics.
func (s *Server) DidChangeConfiguration(
	ctx context.Context,
	params *protocol.DidChangeConfigurationParams,
) (err error) {
	defer recoverNotification(ctx, "DidChangeConfiguration", &err)

	s.applySettings(ctx, params.Settings)
	s.republishAll(ctx)

	return nil
}

// republishAll recomputes diagnostics for every open document.
func (s *Server) republishAll(ctx context.Context) {
	for _, uriArg := range s.docs.URIs() {
		s.publishDiagnostics(ctx, uri.URI(uriArg))
	}
}

// NewServer builds the server from its knowledge base, analyzer, hint
// computer and completion completer (DI); the document cache and the
// include resolver over it are created internally (DocStore
// structurally satisfies include.Source).
func NewServer(
	store *kb.Store,
	analyzer *analysis.Analyzer,
	computer *hints.Computer,
	completer *complete.Completer,
) *Server {
	docs := NewDocStore()

	srv := &Server{
		store:     store,
		analyzer:  analyzer,
		computer:  computer,
		completer: completer,
		docs:      docs,
		resolver:  include.NewResolver(docs),
		exit:      make(chan struct{}),
	}

	srv.utf16.Store(true) // the protocol default until utf-8 is negotiated

	return srv
}

// Initialize declares the server capabilities and negotiates the position
// encoding (utf-8 when the client offers it, utf-16 otherwise).
func (s *Server) Initialize(
	ctx context.Context,
	params *protocol.InitializeParams,
) (*protocol.InitializeResult, error) {
	full := protocol.TextDocumentSyncKindFull

	caps := protocol.ServerCapabilities{
		TextDocumentSync: &protocol.TextDocumentSyncOptions{
			OpenClose: &[]bool{true}[0],
			Change:    &full,
		},
		HoverProvider:             protocol.Boolean(true),
		CompletionProvider:        &protocol.CompletionOptions{TriggerCharacters: []string{" ", "<"}},
		DefinitionProvider:        protocol.Boolean(true),
		ReferencesProvider:        protocol.Boolean(true),
		DocumentSymbolProvider:    protocol.Boolean(true),
		DocumentHighlightProvider: protocol.Boolean(true),
		WorkspaceSymbolProvider:   protocol.Boolean(true),
		DocumentLinkProvider:      &protocol.DocumentLinkOptions{},
		SemanticTokensProvider: &protocol.SemanticTokensOptions{
			Legend: protocol.SemanticTokensLegend{
				TokenTypes:     semanticTokenTypes,
				TokenModifiers: []string{},
			},
		},
		FoldingRangeProvider:  protocol.Boolean(true),
		SignatureHelpProvider: &protocol.SignatureHelpOptions{TriggerCharacters: []string{"(", ","}},
		CodeActionProvider: &protocol.CodeActionOptions{
			CodeActionKinds: []protocol.CodeActionKind{protocol.CodeActionKindQuickFix},
		},
	}

	if enc, ok := negotiateEncoding(params); ok {
		caps.PositionEncoding = enc

		if enc == protocol.PositionEncodingKindUTF8 {
			s.utf16.Store(false)
		}
	}

	if ws := params.Capabilities.Workspace; ws != nil {
		if ws.Configuration != nil && *ws.Configuration {
			s.clientConfigCap.Store(true)
		}

		if w := ws.DidChangeWatchedFiles; w != nil && w.DynamicRegistration != nil && *w.DynamicRegistration {
			s.clientWatchCap.Store(true)
		}
	}

	slog.DebugContext(ctx, "initialized", "encoding", s.encodingName())

	return &protocol.InitializeResult{
		Capabilities: caps,
		ServerInfo:   protocol.ServerInfo{Name: serverName},
	}, nil
}

// encodingName reports the negotiated position encoding for debug logs.
func (s *Server) encodingName() string {
	if s.utf16.Load() {
		return "utf-16"
	}

	return "utf-8"
}

// negotiateEncoding picks utf-8 when the client offers it — parser columns
// are byte offsets, so utf-8 keeps positions exact; ok=false keeps the
// utf-16 default.
func negotiateEncoding(
	params *protocol.InitializeParams,
) (protocol.PositionEncodingKind, bool) {
	if params.Capabilities.General == nil {
		return "", false
	}

	if slices.Contains(params.Capabilities.General.PositionEncodings,
		protocol.PositionEncodingKindUTF8) {
		return protocol.PositionEncodingKindUTF8, true
	}

	return "", false
}

// DidOpen stores the document and publishes the first diagnostics batch.
func (s *Server) DidOpen(
	ctx context.Context,
	params *protocol.DidOpenTextDocumentParams,
) (err error) {
	defer recoverNotification(ctx, "DidOpen", &err)

	doc := params.TextDocument
	s.docs.Put(string(doc.URI), doc.Text, int(doc.Version))
	slog.DebugContext(ctx, "did_open", "uri", doc.URI, "version", doc.Version, "bytes", len(doc.Text))
	s.publishDiagnostics(ctx, doc.URI)

	return nil
}

// DidChange replaces the stored text (Full sync) and republishes
// diagnostics; one change batch per notification.
func (s *Server) DidChange(
	ctx context.Context,
	params *protocol.DidChangeTextDocumentParams,
) (err error) {
	defer recoverNotification(ctx, "DidChange", &err)

	docURI := params.TextDocument.URI
	text, _, _ := s.docs.Get(string(docURI))

	for _, change := range params.ContentChanges {
		if c, ok := change.(*protocol.TextDocumentContentChangeWholeDocument); ok {
			text = c.Text
		}
	}

	s.docs.Put(string(docURI), text, int(params.TextDocument.Version))
	slog.DebugContext(ctx, "did_change", "uri", docURI, "version", params.TextDocument.Version, "bytes", len(text))
	s.publishDiagnostics(ctx, docURI)

	return nil
}

// DidClose drops the document and clears its published diagnostics.
func (s *Server) DidClose(
	ctx context.Context,
	params *protocol.DidCloseTextDocumentParams,
) (err error) {
	defer recoverNotification(ctx, "DidClose", &err)

	docURI := params.TextDocument.URI
	s.docs.Remove(string(docURI))
	slog.DebugContext(ctx, "did_close", "uri", docURI)
	s.publish(ctx, docURI, nil)

	return nil
}

// recoverNotification turns a notification-handler panic into a logged,
// degraded success: the editor session survives without the dropped
// notification, the panic lands in stderr (ERROR) where the corpus
// harness and operators find it. Request handlers stay unwrapped — the
// jsonrpc2 layer already answers those with an error response the caller
// sees.
func recoverNotification(ctx context.Context, method string, err *error) {
	if r := recover(); r != nil {
		slog.ErrorContext(ctx, "notification panic recovered",
			"method", method, "panic", r)

		*err = nil
	}
}

// Hover answers with the kb entry under the cursor: the RMS command owning
// the position or the XS symbol. nil, nil when there is nothing to show.
func (s *Server) Hover(
	ctx context.Context,
	params *protocol.HoverParams,
) (*protocol.Hover, error) {
	text, name, ok := s.openDocument(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	pos := s.fromProtocolPos(text, params.Position)

	var markdown string

	switch {
	case strings.HasSuffix(name, ".rms"):
		markdown = s.hoverRms(text, name, pos)
	case strings.HasSuffix(name, ".xs"):
		markdown = s.hoverXs(text, name, pos)
	}

	if markdown == "" {
		slog.DebugContext(ctx, "hover", "uri", params.TextDocument.URI,
			"line", params.Position.Line, "col", params.Position.Character, "hit", false)

		return nil, nil
	}

	slog.DebugContext(ctx, "hover", "uri", params.TextDocument.URI,
		"line", params.Position.Line, "col", params.Position.Character, "hit", true)

	return &protocol.Hover{
		Contents: &protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: markdown,
		},
	}, nil
}

// hoverRms renders the command owning pos, if the kb knows it.
func (s *Server) hoverRms(text string, name string, pos common.Pos) string {
	file, _ := rms.Parse(text, name)

	if markdown, ok := s.hoverAttribute(file, pos); ok {
		return markdown
	}

	stmt, ok := file.StatementAt(pos)
	if !ok {
		return ""
	}

	cmd, found := s.store.Command(stmt.Name)
	if !found {
		return ""
	}

	return commandMarkdown(cmd)
}

// hoverAttribute resolves a hover on an attribute of a command block:
// ArgAt Kind=attr names the owning command and the attribute (name or
// value position). An attribute unknown to the kb answers found=false so
// the caller degrades to command help.
func (s *Server) hoverAttribute(file rms.RmsFile, pos common.Pos) (string, bool) {
	site, ok := file.ArgAt(pos)
	if !ok || site.Kind != rms.KindAttr {
		return "", false
	}

	attr, found := s.store.Attribute(site.Stmt.Name, site.Name)
	if !found {
		return "", false
	}

	return attributeMarkdown(site.Stmt.Name, attr), true
}

// hoverXs renders the function named by the symbol under pos, if the kb
// knows it.
func (s *Server) hoverXs(text string, name string, pos common.Pos) string {
	file, _ := xs.XsParse(text, name)

	symbol, ok := file.SymbolAt(pos)
	if !ok {
		return ""
	}

	fn, found := s.store.Function(symbol)
	if !found {
		return ""
	}

	return functionMarkdown(fn)
}

// SignatureHelp answers textDocument/signatureHelp with the parameter
// hints for the position: .xs documents and inline XS blocks route to the
// XS path (with the include closure), plain .rms positions to the RMS
// path. The handler is stateless and read-only: the answer depends only
// on (document, position) — the trigger context is ignored. Silence —
// nil, nil — when no hint resolves (trust rule, the Hover convention).
func (s *Server) SignatureHelp(
	ctx context.Context,
	params *protocol.SignatureHelpParams,
) (*protocol.SignatureHelp, error) {
	text, name, ok := s.openDocument(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	docURI := string(params.TextDocument.URI)
	pos := s.fromProtocolPos(text, params.Position)

	var hint hints.Hint

	found := false

	switch {
	case strings.HasSuffix(name, ".rms"):
		hint, found = s.signatureHelpRms(ctx, text, name, pos, docURI)
	case strings.HasSuffix(name, ".xs"):
		hint, found = s.signatureHelpXs(ctx, text, name, pos, docURI)
	}

	slog.DebugContext(ctx, "signature_help", "uri", params.TextDocument.URI,
		"line", params.Position.Line, "col", params.Position.Character, "hit", found)

	if !found {
		return nil, nil
	}

	return toSignatureHelp(hint), nil
}

// signatureHelpXs parses the .xs document and computes the hint with the
// closure's external declarations (the analyzed file is excluded from its
// own externals — the analyzeXs pattern).
func (s *Server) signatureHelpXs(
	ctx context.Context,
	text string,
	name string,
	pos common.Pos,
	docURI string,
) (hints.Hint, bool) {
	file, _ := xs.XsParse(text, name)
	closure := s.resolver.Closure(ctx, docURI)

	return s.computer.XsAt(file, pos, closure.ExternalDecls(docURI))
}

// signatureHelpRms parses the .rms document: a position inside an inline
// XS block routes to the XS path in block-local coordinates (the
// include-Definition template; inline blocks are not closure members, so
// the whole closure serves as externals); any other position takes the
// RMS command path.
func (s *Server) signatureHelpRms(
	ctx context.Context,
	text string,
	name string,
	pos common.Pos,
	docURI string,
) (hints.Hint, bool) {
	file, _ := rms.Parse(text, name)

	for _, block := range file.XsBlocks {
		if !block.Range.Contains(pos) {
			continue
		}

		xsFile, _ := xs.XsParse(block.Code, "inline:"+name)
		closure := s.resolver.Closure(ctx, docURI)

		return s.computer.XsAt(
			xsFile,
			unshiftPos(pos, block.Range.Start),
			closure.ExternalDecls(""),
		)
	}

	return s.computer.RmsAt(file, pos)
}

// toSignatureHelp maps one rendered hint to the protocol shape per
// lsp-protocol: exactly one signature, ActiveSignature 0, plain-string
// parameter labels, documentation omitted (concise-hints rule);
// ActiveParameter is unset when no argument is active — never clamped to
// the last parameter. It is set on both the result and the signature
// (the per-signature field takes precedence since 3.16).
func toSignatureHelp(hint hints.Hint) *protocol.SignatureHelp {
	sig := protocol.SignatureInformation{Label: hint.Label}

	for _, label := range hint.Params {
		sig.Parameters = append(sig.Parameters, protocol.ParameterInformation{
			Label: protocol.String(label),
		})
	}

	var active protocol.Nullable[uint32]

	if hint.Active >= 0 {
		active = protocol.NewNullable(uint32(hint.Active))
		sig.ActiveParameter = active
	}

	return &protocol.SignatureHelp{
		Signatures:      []protocol.SignatureInformation{sig},
		ActiveSignature: &[]uint32{0}[0],
		ActiveParameter: active,
	}
}

// Completion answers the candidates in scope: .rms positions complete
// by the RMS context matrix, .xs and inline XS blocks by the visible
// symbol pool over the kb functions and constants. The provider is
// stateless — the answer depends only on (document, position); prefix
// filtering belongs to the client. An empty candidate list is the
// designed silence: an empty CompletionList, never an error.
func (s *Server) Completion(
	ctx context.Context,
	params *protocol.CompletionParams,
) (protocol.CompletionResult, error) {
	text, name, ok := s.openDocument(params.TextDocument.URI)
	if !ok {
		return &protocol.CompletionList{Items: []protocol.CompletionItem{}}, nil
	}

	docURI := string(params.TextDocument.URI)
	pos := s.fromProtocolPos(text, params.Position)

	var cands []complete.Candidate

	switch {
	case strings.HasSuffix(name, ".rms"):
		cands = s.completionRms(ctx, text, name, pos, docURI)
	case strings.HasSuffix(name, ".xs"):
		cands = s.completionXs(ctx, text, name, pos, docURI)
	}

	slog.DebugContext(ctx, "completion", "uri", params.TextDocument.URI,
		"line", params.Position.Line, "col", params.Position.Character, "items", len(cands))

	return &protocol.CompletionList{IsIncomplete: false, Items: toCompletionItems(cands)}, nil
}

// completionRms parses the .rms document: a position inside an inline
// XS block routes to the XS path in block-local coordinates (the
// include-Definition template; inline blocks are not closure members,
// so the whole closure serves as externals); any other position takes
// the RMS context matrix.
func (s *Server) completionRms(
	ctx context.Context,
	text string,
	name string,
	pos common.Pos,
	docURI string,
) []complete.Candidate {
	file, _ := rms.Parse(text, name)

	for _, block := range file.XsBlocks {
		if !block.Range.Contains(pos) {
			continue
		}

		xsFile, _ := xs.XsParse(block.Code, "inline:"+name)
		closure := s.resolver.Closure(ctx, docURI)

		return s.completer.XsAt(
			xsFile,
			unshiftPos(pos, block.Range.Start),
			closure.ExternalDecls(""),
		)
	}

	return s.completer.RmsAt(file, pos)
}

// completionXs parses the .xs document and computes the candidates with
// the closure's external declarations (the analyzed file is excluded
// from its own externals — the analyzeXs pattern).
func (s *Server) completionXs(
	ctx context.Context,
	text string,
	name string,
	pos common.Pos,
	docURI string,
) []complete.Candidate {
	file, _ := xs.XsParse(text, name)
	closure := s.resolver.Closure(ctx, docURI)

	return s.completer.XsAt(file, pos, closure.ExternalDecls(docURI))
}

// completionKinds maps the complete cell's candidate kinds to protocol
// completion kinds; the table is parallel to the DocumentSymbol mapping
// (command → Function, like the outline).
var completionKinds = map[string]protocol.CompletionItemKind{
	complete.KindCommand:   protocol.CompletionItemKindFunction,
	complete.KindAttribute: protocol.CompletionItemKindField,
	complete.KindConstant:  protocol.CompletionItemKindConstant,
	complete.KindFunction:  protocol.CompletionItemKindFunction,
	complete.KindVariable:  protocol.CompletionItemKindVariable,
	complete.KindParam:     protocol.CompletionItemKindVariable,
	complete.KindLocal:     protocol.CompletionItemKindVariable,
}

// toCompletionItems renders candidates per lsp-protocol (Completion
// Items): Label, the kind-table Kind, one-line Detail, SortText; the
// insert text equals the label and stays unset, documentation stays
// out of completion (concise-items rule).
func toCompletionItems(cands []complete.Candidate) []protocol.CompletionItem {
	items := make([]protocol.CompletionItem, 0, len(cands))

	for _, c := range cands {
		item := protocol.CompletionItem{
			Label:    c.Label,
			Kind:     completionKinds[c.Kind],
			SortText: protocol.NewOptional(c.Sort),
		}

		if c.Detail != "" {
			item.Detail = protocol.NewOptional(c.Detail)
		}

		items = append(items, item)
	}

	return items
}

// Definition answers textDocument/definition across the document's
// include closure: include paths jump into the target file, XS names
// resolve locally then through the closure. Not-found positions resolve
// to an empty LocationSlice (not nil); an empty result is not an error.
func (s *Server) Definition(
	ctx context.Context,
	params *protocol.DefinitionParams,
) (protocol.DefinitionResult, error) {
	text, _, _ := s.openDocument(params.TextDocument.URI)

	target, found := s.resolver.Definition(
		ctx,
		string(params.TextDocument.URI),
		s.fromProtocolPos(text, params.Position),
	)
	slog.DebugContext(ctx, "definition", "uri", params.TextDocument.URI,
		"line", params.Position.Line, "col", params.Position.Character, "hit", found)

	if !found {
		return protocol.LocationSlice{}, nil
	}

	return &protocol.Location{
		URI:   uri.URI(target.URI),
		Range: s.targetRange(target.URI, target.Range),
	}, nil
}

// References answers textDocument/references over the include closure
// (plus open documents that include the queried one). The local
// declaration is dropped unless the client includes it. Empty results
// are empty slices, not nil.
func (s *Server) References(
	ctx context.Context,
	params *protocol.ReferenceParams,
) ([]protocol.Location, error) {
	docURI := string(params.TextDocument.URI)
	text, _, _ := s.openDocument(params.TextDocument.URI)
	pos := s.fromProtocolPos(text, params.Position)

	targets := s.resolver.References(ctx, docURI, pos)

	if !params.Context.IncludeDeclaration {
		targets = s.excludeLocalDeclaration(ctx, docURI, pos, targets)
	}

	out := make([]protocol.Location, 0, len(targets))

	for _, t := range targets {
		out = append(out, protocol.Location{
			URI:   uri.URI(t.URI),
			Range: s.targetRange(t.URI, t.Range),
		})
	}

	slog.DebugContext(ctx, "references", "uri", params.TextDocument.URI,
		"line", params.Position.Line, "col", params.Position.Character, "count", len(out))

	return out, nil
}

// excludeLocalDeclaration drops the declaration range of the symbol
// under pos in the queried document (includeDeclaration=false).
func (s *Server) excludeLocalDeclaration(
	ctx context.Context,
	docURI string,
	pos common.Pos,
	targets []include.Target,
) []include.Target {
	decl, found := s.resolver.Definition(ctx, docURI, pos)
	if !found || decl.URI != docURI {
		return targets
	}

	out := make([]include.Target, 0, len(targets))

	for _, t := range targets {
		if t.URI != decl.URI || t.Range != decl.Range {
			out = append(out, t)
		}
	}

	return out
}

// DocumentHighlight answers textDocument/documentHighlight: every
// in-file occurrence of the word under the cursor, all kind Text —
// occurrences are syntactic name matches (ReferencesAt), the server
// does not split reads from writes. Unlike Definition/References no
// include closure is computed: highlights never cross file boundaries;
// inline-XS blocks shift their block-local hits back into document
// coordinates. Empty results are empty slices, not nil.
func (s *Server) DocumentHighlight(
	ctx context.Context,
	params *protocol.DocumentHighlightParams,
) ([]protocol.DocumentHighlight, error) {
	docURI := params.TextDocument.URI
	text, name, _ := s.openDocument(docURI)
	pos := s.fromProtocolPos(text, params.Position)

	ranges := s.occurrenceRanges(text, name, pos)

	out := make([]protocol.DocumentHighlight, 0, len(ranges))

	for _, r := range ranges {
		out = append(out, protocol.DocumentHighlight{
			Range: s.toProtocolRange(text, r),
			Kind:  protocol.DocumentHighlightKindText,
		})
	}

	slog.DebugContext(ctx, "document_highlight", "uri", docURI,
		"line", params.Position.Line, "col", params.Position.Character, "count", len(out))

	return out, nil
}

// occurrenceRanges routes a position through the document language and
// returns every same-name occurrence in document coordinates — the
// shared word machinery of documentHighlight and quickfix edits.
func (s *Server) occurrenceRanges(text string, name string, pos common.Pos) []common.Range {
	switch {
	case strings.HasSuffix(name, ".rms"):
		return s.highlightRms(text, name, pos)
	case strings.HasSuffix(name, ".xs"):
		file, _ := xs.XsParse(text, name)
		return file.ReferencesAt(pos)
	}

	return nil
}

// highlightRms collects highlight ranges for a .rms document: a position
// inside an inline XS block routes to the XS word index in block-local
// coordinates and shifts the hits back (the signatureHelpRms template);
// any other position takes the RMS word index.
func (s *Server) highlightRms(text string, name string, pos common.Pos) []common.Range {
	file, _ := rms.Parse(text, name)

	for _, block := range file.XsBlocks {
		if !block.Range.Contains(pos) {
			continue
		}

		xsFile, _ := xs.XsParse(block.Code, "inline:"+name)

		return shiftRanges(xsFile.ReferencesAt(unshiftPos(pos, block.Range.Start)), block.Range.Start)
	}

	return file.ReferencesAt(pos)
}

// FoldingRanges answers textDocument/foldingRange with the foldable
// regions of the document: every outline node spanning more than one
// line, in document order. Only StartLine/EndLine are set — characters
// and kind stay to the client defaults. Empty results are empty
// slices, not nil.
func (s *Server) FoldingRanges(
	ctx context.Context,
	params *protocol.FoldingRangeParams,
) ([]protocol.FoldingRange, error) {
	docURI := params.TextDocument.URI
	text, name, _ := s.openDocument(docURI)

	var syms []common.Symbol

	switch {
	case strings.HasSuffix(name, ".rms"):
		file, _ := rms.Parse(text, name)
		syms = file.Symbols()
	case strings.HasSuffix(name, ".xs"):
		file, _ := xs.XsParse(text, name)
		syms = file.Symbols()
	}

	out := make([]protocol.FoldingRange, 0, len(syms))
	collectFoldables(syms, &out)

	slog.DebugContext(ctx, "folding_ranges", "uri", docURI, "count", len(out))

	return out, nil
}

// collectFoldables walks the outline tree depth-first, appending a
// region for every node whose range spans more than one line — the
// traversal order keeps the result in document order.
func collectFoldables(syms []common.Symbol, out *[]protocol.FoldingRange) {
	for _, sym := range syms {
		if sym.Range.End.Line > sym.Range.Start.Line {
			*out = append(*out, protocol.FoldingRange{
				StartLine: sym.Range.Start.Line,
				EndLine:   sym.Range.End.Line,
			})
		}

		collectFoldables(sym.Children, out)
	}
}

// codeMissingInclude mirrors the analyzer-side missing-include code.
const codeMissingInclude = "missing-include"

// CodeAction answers textDocument/codeAction with quickfixes built
// from the diagnostics the client echoes in the request context:
// did-you-mean renames, the effect_percent replacement and
// missing-include file creation. Stateless — everything derives from
// params; the filesystem is neither read nor written (the create-file
// fix is an edit the client applies). Empty results are empty slices,
// not nil.
func (s *Server) CodeAction(
	ctx context.Context,
	params *protocol.CodeActionParams,
) ([]protocol.CommandOrCodeAction, error) {
	if !onlyAllowsQuickFix(params.Context.Only) {
		return []protocol.CommandOrCodeAction{}, nil
	}

	out := make([]protocol.CommandOrCodeAction, 0, len(params.Context.Diagnostics))

	for i := range params.Context.Diagnostics {
		diag := &params.Context.Diagnostics[i]

		if !rangesOverlap(diag.Range, params.Range) {
			continue
		}

		switch fmt.Sprint(diag.Code) {
		case analysis.CodeUnknownCommand, analysis.CodeUnknownAttribute, analysis.CodeUndefinedSymbol:
			out = appendAction(out, s.renameAction(params, diag))
		case analysis.CodeDeprecatedEffectPercent:
			out = appendAction(out, s.replaceEffectPercentAction(params, diag))
		case codeMissingInclude:
			out = appendAction(out, s.createIncludeAction(params, diag))
		}
	}

	slog.DebugContext(ctx, "code_action", "uri", params.TextDocument.URI, "count", len(out))

	return out, nil
}

// appendAction adds a built action when present.
func appendAction(out []protocol.CommandOrCodeAction, action *protocol.CodeAction) []protocol.CommandOrCodeAction {
	if action == nil {
		return out
	}

	return append(out, action)
}

// renameAction builds the did-you-mean fix: replace the word under the
// diagnostic start with the suggested name from the message suffix.
func (s *Server) renameAction(
	params *protocol.CodeActionParams,
	diag *protocol.Diagnostic,
) *protocol.CodeAction {
	name, ok := suggestedName(messageText(diag.Message))
	if !ok {
		return nil
	}

	return s.replaceWordAction(params, diag, "Change to '"+name+"'", name)
}

// replaceEffectPercentAction renames the deprecated command token; the
// percent-vs-absolute argument semantics stay with the author.
func (s *Server) replaceEffectPercentAction(
	params *protocol.CodeActionParams,
	diag *protocol.Diagnostic,
) *protocol.CodeAction {
	return s.replaceWordAction(params, diag, "Replace with effect_amount", "effect_amount")
}

// replaceWordAction builds a quickfix replacing the word at the
// diagnostic start with newText; nil when the document has no word
// token under the position.
func (s *Server) replaceWordAction(
	params *protocol.CodeActionParams,
	diag *protocol.Diagnostic,
	title string,
	newText string,
) *protocol.CodeAction {
	text, name, _ := s.openDocument(params.TextDocument.URI)
	pos := s.fromProtocolPos(text, diag.Range.Start)

	var word common.Range

	found := false

	for _, r := range s.occurrenceRanges(text, name, pos) {
		if r.Contains(pos) {
			word, found = r, true

			break
		}
	}

	if !found {
		return nil
	}

	kind := protocol.CodeActionKindQuickFix

	return &protocol.CodeAction{
		Title:       title,
		Kind:        &kind,
		Diagnostics: []protocol.Diagnostic{*diag},
		Edit: &protocol.WorkspaceEdit{Changes: map[uri.URI][]protocol.TextEdit{
			params.TextDocument.URI: {{Range: s.toProtocolRange(text, word), NewText: newText}},
		}},
	}
}

// createIncludeAction builds the missing-include fix: create the target
// file next to the including document (idempotent).
func (s *Server) createIncludeAction(
	params *protocol.CodeActionParams,
	diag *protocol.Diagnostic,
) *protocol.CodeAction {
	path, ok := strings.CutPrefix(messageText(diag.Message), "include not found: ")
	if !ok || path == "" {
		return nil
	}

	dir := filepath.Dir(params.TextDocument.URI.FsPath())
	kind := protocol.CodeActionKindQuickFix

	return &protocol.CodeAction{
		Title:       "Create '" + path + "'",
		Kind:        &kind,
		Diagnostics: []protocol.Diagnostic{*diag},
		Edit: &protocol.WorkspaceEdit{DocumentChanges: []protocol.DocumentChange{
			&protocol.CreateFile{
				Kind:    "create",
				URI:     uri.File(filepath.Join(dir, path)),
				Options: &protocol.CreateFileOptions{IgnoreIfExists: &[]bool{true}[0]},
			},
		}},
	}
}

// messageText projects the diagnostic message union onto its plain
// string arm (the only arm the server itself ever sends).
func messageText(m protocol.InlayHintTooltip) string {
	if s, ok := m.(protocol.String); ok {
		return string(s)
	}

	return fmt.Sprint(m)
}

// suggestedName extracts X from a message carrying the documented
// did-you-mean suffix; false when the message has none.
func suggestedName(msg string) (string, bool) {
	_, rest, ok := strings.Cut(msg, `; did you mean "`)
	if !ok {
		return "", false
	}

	name, _, ok := strings.Cut(rest, `"?`)

	return name, ok
}

// onlyAllowsQuickFix reports whether the Only filter permits quickfix
// actions (an empty filter permits everything).
func onlyAllowsQuickFix(only []protocol.CodeActionKind) bool {
	if len(only) == 0 {
		return true
	}

	return slices.Contains(only, protocol.CodeActionKindQuickFix)
}

// rangesOverlap reports whether two protocol ranges share at least one
// position (inclusive start, exclusive end).
func rangesOverlap(a, b protocol.Range) bool {
	return !posBefore(a.End, b.Start) && !posBefore(b.End, a.Start)
}

// posBefore orders protocol positions by line, then character.
func posBefore(a, b protocol.Position) bool {
	if a.Line != b.Line {
		return a.Line < b.Line
	}

	return a.Character < b.Character
}

// DocumentSymbol answers textDocument/documentSymbol with the
// hierarchical outline of the document.
func (s *Server) DocumentSymbol(
	ctx context.Context,
	params *protocol.DocumentSymbolParams,
) (protocol.DocumentSymbolResult, error) {
	text, name, ok := s.openDocument(params.TextDocument.URI)
	if !ok {
		return protocol.DocumentSymbolSlice{}, nil
	}

	var syms []common.Symbol

	switch {
	case strings.HasSuffix(name, ".rms"):
		file, _ := rms.Parse(text, name)
		syms = file.Symbols()
	case strings.HasSuffix(name, ".xs"):
		file, _ := xs.XsParse(text, name)
		syms = file.Symbols()
	}

	out := make(protocol.DocumentSymbolSlice, 0, len(syms))

	for _, sym := range syms {
		out = append(out, s.toDocumentSymbol(text, sym))
	}

	slog.DebugContext(ctx, "document_symbol", "uri", params.TextDocument.URI, "symbols", len(out))

	return out, nil
}

// workspaceCandidate is one fuzzy-matched symbol awaiting ordering.
type workspaceCandidate struct {
	score int
	sym   common.Symbol
	uri   string
}

// Symbols answers workspace/symbol: a fuzzy query over the universe of
// every open document plus its include closure (the closure root is the
// open document itself, parsed from editor state). RMS files contribute
// sections only — command statements are noise; XS files contribute all
// top-level declarations. Results carry SymbolInformation (flat arm);
// locations may point into files that are not open. Empty results are
// empty slices, not nil.
func (s *Server) Symbols(
	ctx context.Context,
	params *protocol.WorkspaceSymbolParams,
) (protocol.WorkspaceSymbolResult, error) {
	query := params.Query

	seen := make(map[string]bool)

	var candidates []workspaceCandidate

	for _, u := range s.docs.URIs() {
		closure := s.resolver.Closure(ctx, u)

		for _, entry := range closure.Rms {
			if seen[entry.URI] {
				continue
			}

			seen[entry.URI] = true

			candidates = s.collectSymbols(candidates, query, entry.URI, sectionsOnly(entry.File.Symbols()))
		}

		for _, entry := range closure.Xs {
			if seen[entry.URI] {
				continue
			}

			seen[entry.URI] = true

			candidates = s.collectSymbols(candidates, query, entry.URI, entry.File.Symbols())
		}
	}

	slices.SortStableFunc(candidates, compareCandidates)

	out := make(protocol.SymbolInformationSlice, 0, len(candidates))

	for _, c := range candidates {
		kind, ok := symbolKindTable[c.sym.Kind]
		if !ok {
			kind = protocol.SymbolKindField
		}

		out = append(out, protocol.SymbolInformation{
			BaseSymbolInformation: protocol.BaseSymbolInformation{
				Name: c.sym.Name,
				Kind: kind,
			},
			Location: protocol.Location{
				URI:   uri.URI(c.uri),
				Range: s.targetRange(c.uri, c.sym.Selection),
			},
		})
	}

	slog.DebugContext(ctx, "workspace_symbol", "query", query, "count", len(out))

	return out, nil
}

// collectSymbols filters one file's symbols through the fuzzy matcher.
func (s *Server) collectSymbols(
	out []workspaceCandidate,
	query string,
	uriArg string,
	syms []common.Symbol,
) []workspaceCandidate {
	for _, sym := range syms {
		score, matched := fuzzyMatch(query, sym.Name)
		if !matched {
			continue
		}

		out = append(out, workspaceCandidate{score: score, sym: sym, uri: uriArg})
	}

	return out
}

// compareCandidates orders the workspace result: score descending, then
// URI, then position — a total order, so the answer is deterministic.
func compareCandidates(a, b workspaceCandidate) int {
	if a.score != b.score {
		return b.score - a.score
	}

	if a.uri != b.uri {
		return strings.Compare(a.uri, b.uri)
	}

	if a.sym.Selection.Start.Line != b.sym.Selection.Start.Line {
		return int(a.sym.Selection.Start.Line) - int(b.sym.Selection.Start.Line)
	}

	return int(a.sym.Selection.Start.Column) - int(b.sym.Selection.Start.Column)
}

// sectionsOnly keeps the section nodes of an RMS outline tree — nested
// sections included, command and inline-XS nodes dropped.
func sectionsOnly(syms []common.Symbol) []common.Symbol {
	out := make([]common.Symbol, 0, len(syms))

	for _, sym := range syms {
		if sym.Kind != "section" {
			continue
		}

		if len(sym.Children) > 0 {
			sym.Children = sectionsOnly(sym.Children)
		}

		out = append(out, sym)
	}

	return out
}

// semanticTokenTypes is the fixed server-side legend: a token type's
// slice index is its protocol token-type index. The legend is not
// client-negotiable — editors map the types to their own highlight
// groups.
var semanticTokenTypes = []string{
	"known",
	"unknown",
	"deprecated",
	"section",
	"kind",
}

// SemanticTokensFull answers textDocument/semanticTokens/full with the
// classified identifiers of one document: RMS names and values plus
// inline XS blocks (shifted into file coordinates) for .rms, XS
// declarations and identifiers (with the include closure's externals)
// for .xs. Data is delta-encoded quintuples; full-document only, no
// range requests, no deltas. Empty documents answer empty Data — not
// nil, not an error.
func (s *Server) SemanticTokensFull(
	ctx context.Context,
	params *protocol.SemanticTokensParams,
) (*protocol.SemanticTokens, error) {
	docURI := params.TextDocument.URI
	text, name, _ := s.openDocument(docURI)

	var toks []common.Token

	switch {
	case strings.HasSuffix(name, ".rms"):
		file, _ := rms.Parse(text, name)

		toks = s.analyzer.TokensRms(file)

		for _, block := range file.XsBlocks {
			xsFile, _ := xs.XsParse(block.Code, "inline:"+name)

			for _, tok := range s.analyzer.TokensXs(xsFile, nil) {
				tok.Range = common.Range{
					Start: shiftPos(tok.Range.Start, block.Range.Start),
					End:   shiftPos(tok.Range.End, block.Range.Start),
				}

				toks = append(toks, tok)
			}
		}
	case strings.HasSuffix(name, ".xs"):
		xsFile, _ := xs.XsParse(text, name)

		externals := s.resolver.Closure(ctx, string(docURI)).ExternalDecls(string(docURI))
		toks = s.analyzer.TokensXs(xsFile, externals)
	}

	data := s.encodeSemanticTokens(text, toks)

	slog.DebugContext(ctx, "semantic_tokens", "uri", docURI, "tokens", len(toks))

	return &protocol.SemanticTokens{Data: data}, nil
}

// encodeSemanticTokens converts classified tokens into delta-encoded
// quintuples [Δline, Δstart, length, typeIdx, modifiers] in the
// negotiated position encoding; the input is position-sorted with no
// overlaps (analysis guarantee).
func (s *Server) encodeSemanticTokens(text string, toks []common.Token) []uint32 {
	data := make([]uint32, 0, len(toks)*5)

	prevLine, prevStart := uint32(0), uint32(0)

	for _, tok := range toks {
		typeIdx := slices.Index(semanticTokenTypes, tok.Type)
		if typeIdx < 0 {
			continue
		}

		start := s.toProtocolPos(text, tok.Range.Start)
		end := s.toProtocolPos(text, tok.Range.End)

		dLine := start.Line - prevLine

		dStart := start.Character
		if dLine == 0 {
			dStart -= prevStart
		}

		data = append(data, dLine, dStart, end.Character-start.Character, uint32(typeIdx), 0)

		prevLine, prevStart = start.Line, start.Character
	}

	return data
}

// symbolKindTable maps the producer kind vocabularies to protocol
// symbol kinds; unknown kinds fall back to Field.
var symbolKindTable = map[string]protocol.SymbolKind{
	"function": protocol.SymbolKindFunction,
	"extern":   protocol.SymbolKindFunction,
	"variable": protocol.SymbolKindVariable,
	"rule":     protocol.SymbolKindEvent,
	"event":    protocol.SymbolKindEvent,
	"section":  protocol.SymbolKindModule,
	"command":  protocol.SymbolKindFunction,
	"xs":       protocol.SymbolKindNamespace,
}

// toDocumentSymbol converts one outline node recursively, preserving
// the Selection ⊆ Range invariant of the source tree.
func (s *Server) toDocumentSymbol(text string, sym common.Symbol) protocol.DocumentSymbol {
	kind, ok := symbolKindTable[sym.Kind]
	if !ok {
		kind = protocol.SymbolKindField
	}

	out := protocol.DocumentSymbol{
		Name:           sym.Name,
		Kind:           kind,
		Range:          s.toProtocolRange(text, sym.Range),
		SelectionRange: s.toProtocolRange(text, sym.Selection),
	}

	if len(sym.Children) > 0 {
		out.Children = make([]protocol.DocumentSymbol, 0, len(sym.Children))

		for _, child := range sym.Children {
			out.Children = append(out.Children, s.toDocumentSymbol(text, child))
		}
	}

	return out
}

// DocumentLink answers textDocument/documentLink with one link per
// resolved #include / #includeXS directive of the queried .rms
// document: the range covers the path argument, the target is the
// resolved file (it need not be open). Unresolved directives are
// skipped — the missing-include diagnostic is their signal. Empty
// results are empty slices, not nil.
func (s *Server) DocumentLink(
	ctx context.Context,
	params *protocol.DocumentLinkParams,
) ([]protocol.DocumentLink, error) {
	docURI := string(params.TextDocument.URI)
	text, name, _ := s.openDocument(params.TextDocument.URI)

	if !strings.HasSuffix(name, ".rms") {
		return []protocol.DocumentLink{}, nil
	}

	closure := s.resolver.Closure(ctx, docURI)

	resolved := make([]include.ResolvedInclude, 0, len(closure.Resolved))
	for _, r := range closure.Resolved {
		if r.Owner == docURI {
			resolved = append(resolved, r)
		}
	}

	// The closure groups directives by kind (#include before
	// #includeXS); links are reported in document order instead.
	slices.SortStableFunc(resolved, func(a, b include.ResolvedInclude) int {
		return a.Inc.Range.Start.Offset - b.Inc.Range.Start.Offset
	})

	out := make([]protocol.DocumentLink, 0, len(resolved))
	for _, r := range resolved {
		target := uri.URI(r.Target)
		out = append(out, protocol.DocumentLink{
			Range:  s.toProtocolRange(text, r.Inc.Range),
			Target: &target,
		})
	}

	slog.DebugContext(ctx, "document_link", "uri", docURI, "count", len(out))

	return out, nil
}

// Shutdown acknowledges a clean shutdown request.
func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

// Exit terminates the connection; Serve returns once the exit channel
// closes.
func (s *Server) Exit(ctx context.Context) error {
	s.exitOnce.Do(func() { close(s.exit) })

	return nil
}

// openDocument returns the text and name of an open document.
func (s *Server) openDocument(docURI uri.URI) (text string, name string, ok bool) {
	text, _, ok = s.docs.Get(string(docURI))

	return text, string(docURI), ok
}

// publishDiagnostics reparses and analyzes the stored document version and
// pushes one diagnostics batch.
func (s *Server) publishDiagnostics(ctx context.Context, docURI uri.URI) {
	text, _, ok := s.docs.Get(string(docURI))
	if !ok {
		return
	}

	uriArg := string(docURI)
	closure := s.resolver.Closure(ctx, uriArg)

	diags := s.analyze(uriArg, text, closure)
	slog.DebugContext(ctx, "diagnostics", "uri", uriArg, "count", len(diags))

	s.publish(ctx, docURI, diags)
}

// publish pushes a diagnostics batch for docURI via the client dispatcher
// embedded in the handler context.
func (s *Server) publish(
	ctx context.Context,
	docURI uri.URI,
	diags []protocol.Diagnostic,
) {
	client, ok := protocol.ClientFromContext(ctx)
	if !ok {
		return
	}

	// A failed push must not break the session — the next change republishes
	// the batch; record it as a WARN for the operator.
	if err := client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         docURI,
		Diagnostics: diags,
	}); err != nil {
		slog.WarnContext(ctx, "publish diagnostics", "uri", docURI, "err", err)
	}
}

// analyze runs the diagnostics pipeline for one document version: parse,
// semantic checks, missing includes and inline XS blocks (for .rms) with
// the closure's external declarations, merged and sorted.
func (s *Server) analyze(uriArg string, text string, closure include.Closure) []protocol.Diagnostic {
	var diags []common.Diagnostic

	switch {
	case strings.HasSuffix(uriArg, ".rms"):
		diags = s.analyzeRms(uriArg, text, closure)
	case strings.HasSuffix(uriArg, ".xs"):
		diags = s.analyzeXs(uriArg, text, closure)
	}

	sortDiags(diags)

	return s.toProtocolDiags(text, diags)
}

// analyzeRms parses the RMS source, runs semantic checks, reports the
// missing includes owned by the document and delegates each inline XS
// block to the XS pipeline with ranges shifted into document coordinates.
// External declarations come from the whole closure (inline blocks are
// not closure members, nothing to exclude).
func (s *Server) analyzeRms(uriArg string, text string, closure include.Closure) []common.Diagnostic {
	file, syntax := rms.Parse(text, uriArg)

	diags := append(syntax, s.analyzer.AnalyzeRms(file)...)
	diags = append(diags, missingDiags(uriArg, closure)...)

	for _, block := range file.XsBlocks {
		xsFile, xsSyntax := xs.XsParse(block.Code, "inline:"+uriArg)
		base := block.Range.Start

		diags = append(diags, shiftDiags(xsSyntax, base)...)
		diags = append(diags, shiftDiags(s.analyzer.AnalyzeXs(xsFile, closure.ExternalDecls("")), base)...)
	}

	return diags
}

// analyzeXs parses the XS source and runs semantic checks with the
// closure's external declarations; the analyzed file itself is excluded.
func (s *Server) analyzeXs(uriArg string, text string, closure include.Closure) []common.Diagnostic {
	file, syntax := xs.XsParse(text, uriArg)

	return append(syntax, s.analyzer.AnalyzeXs(file, closure.ExternalDecls(uriArg))...)
}

// missingDiags converts the closure's missing includes owned by the
// document into diagnostics on the path argument's range.
func missingDiags(uriArg string, closure include.Closure) []common.Diagnostic {
	var out []common.Diagnostic

	for _, m := range closure.Missing {
		if m.Owner != uriArg {
			continue
		}

		out = append(out, common.Diagnostic{
			Range:    m.Range,
			Severity: common.SeverityError,
			Message:  "include not found: " + m.Path,
			Code:     "missing-include",
		})
	}

	return out
}

// shiftDiags moves block-relative diagnostics into document coordinates.
func shiftDiags(diags []common.Diagnostic, base common.Pos) []common.Diagnostic {
	shifted := make([]common.Diagnostic, len(diags))

	for i, d := range diags {
		shifted[i] = d
		shifted[i].Range.Start = shiftPos(d.Range.Start, base)
		shifted[i].Range.End = shiftPos(d.Range.End, base)
	}

	return shifted
}

// shiftPos maps a block-relative position into the document by adding the
// block start; only the first block line also gains the start column.
func shiftPos(p common.Pos, base common.Pos) common.Pos {
	shifted := common.Pos{
		Line:   p.Line + base.Line,
		Column: p.Column,
		Offset: p.Offset + base.Offset,
	}

	if p.Line == 0 {
		shifted.Column += base.Column
	}

	return shifted
}

// unshiftPos maps a document position into block-relative coordinates by
// subtracting the block start — the exact inverse of shiftPos; only the
// first block line also loses the start column. Lookup coordinates become
// block-local; the document-coordinate answer needs no re-shifting.
func unshiftPos(p common.Pos, base common.Pos) common.Pos {
	unshifted := common.Pos{
		Line:   p.Line - base.Line,
		Column: p.Column,
		Offset: p.Offset - base.Offset,
	}

	if p.Line == base.Line {
		unshifted.Column -= base.Column
	}

	return unshifted
}

// shiftRanges moves block-relative highlight ranges into document
// coordinates — the range counterpart of shiftDiags.
func shiftRanges(ranges []common.Range, base common.Pos) []common.Range {
	out := make([]common.Range, 0, len(ranges))

	for _, r := range ranges {
		out = append(out, common.Range{
			Start: shiftPos(r.Start, base),
			End:   shiftPos(r.End, base),
		})
	}

	return out
}

// sortDiags orders diagnostics by start position, then message, so every
// published batch is stable.
func sortDiags(diags []common.Diagnostic) {
	slices.SortStableFunc(diags, func(a, b common.Diagnostic) int {
		if c := comparePos(a.Range.Start, b.Range.Start); c != 0 {
			return c
		}

		return strings.Compare(a.Message, b.Message)
	})
}

// comparePos orders two positions by (Line, Column).
func comparePos(a, b common.Pos) int {
	if a.Line != b.Line {
		return int(a.Line) - int(b.Line)
	}

	return int(a.Column) - int(b.Column)
}

// toProtocolDiags converts shared diagnostics to the protocol shape,
// translating columns for the negotiated encoding and applying the
// configured severity overrides (none suppresses the diagnostic).
func (s *Server) toProtocolDiags(text string, diags []common.Diagnostic) []protocol.Diagnostic {
	out := make([]protocol.Diagnostic, 0, len(diags))

	for _, d := range diags {
		severity, keep := s.settings.Load().severityFor(d.Code, d.Severity)
		if !keep {
			continue
		}

		out = append(out, protocol.Diagnostic{
			Range:    s.toProtocolRange(text, d.Range),
			Severity: severity,
			Code:     protocol.String(d.Code),
			Source:   protocol.NewOptional(serverName),
			Message:  protocol.String(d.Message),
		})
	}

	return out
}

// toProtocolPos converts a shared position to the protocol shape,
// translating the byte column into UTF-16 code units unless the client
// negotiated utf-8 (parser columns are byte offsets; Offset itself is
// protocol-external and dropped).
func (s *Server) toProtocolPos(text string, p common.Pos) protocol.Position {
	if !s.utf16.Load() {
		return protocol.Position{Line: p.Line, Character: p.Column}
	}

	return protocol.Position{Line: p.Line, Character: byteToUTF16(lineOf(text, p.Line), p.Column)}
}

// fromProtocolPos converts a protocol position to the shared shape,
// translating UTF-16 code units back into the byte column the parsers
// expect; the offset is recomputed on demand by the parsers.
func (s *Server) fromProtocolPos(text string, p protocol.Position) common.Pos {
	// no text at hand (a closed document queried from disk): keep the
	// raw column — there is nothing to translate against
	if !s.utf16.Load() || text == "" {
		return common.Pos{Line: p.Line, Column: p.Character}
	}

	return common.Pos{Line: p.Line, Column: utf16ToByte(lineOf(text, p.Line), p.Character)}
}

// toProtocolRange converts a shared range with the document text.
func (s *Server) toProtocolRange(text string, r common.Range) protocol.Range {
	return protocol.Range{Start: s.toProtocolPos(text, r.Start), End: s.toProtocolPos(text, r.End)}
}

// targetRange converts a cross-file target range: open documents convert
// with their text; unopened files keep byte columns (their text is not
// at hand — same-line non-ASCII before the target stays shifted).
func (s *Server) targetRange(uriArg string, r common.Range) protocol.Range {
	if text, _, ok := s.docs.Get(uriArg); ok {
		return s.toProtocolRange(text, r)
	}

	return protocol.Range{
		Start: protocol.Position{Line: r.Start.Line, Character: r.Start.Column},
		End:   protocol.Position{Line: r.End.Line, Character: r.End.Column},
	}
}

// lineOf returns the n-th line of text ("" beyond the end).
func lineOf(text string, n uint32) string {
	for ; n > 0; n-- {
		i := strings.IndexByte(text, '\n')
		if i < 0 {
			return ""
		}

		text = text[i+1:]
	}

	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}

	return text
}

// byteToUTF16 converts a byte column into UTF-16 code units within line,
// clamping past-the-end columns.
func byteToUTF16(line string, col uint32) uint32 {
	if int(col) > len(line) {
		col = uint32(len(line))
	}

	units := uint32(0)

	for _, r := range line[:col] {
		if r > 0xFFFF {
			units += 2 // surrogate pair
		} else {
			units++
		}
	}

	return units
}

// utf16ToByte converts a UTF-16 unit count into a byte column; a
// position inside a surrogate pair or past the end clamps to the line.
func utf16ToByte(line string, units uint32) uint32 {
	seen := uint32(0)

	for i, r := range line {
		w := uint32(1)
		if r > 0xFFFF {
			w = 2
		}

		if seen+w > units {
			return uint32(i)
		}

		seen += w
	}

	return uint32(len(line))
}

// commandSignature renders the call signature of an RMS command, e.g.
// "create_elevator(number of_objects, ...)".
func commandSignature(cmd kb.Command) string {
	var b strings.Builder

	b.WriteString(cmd.Name)

	if len(cmd.Args) > 0 {
		b.WriteString("(")

		for i, arg := range cmd.Args {
			if i > 0 {
				b.WriteString(", ")
			}

			fmt.Fprintf(&b, "%s %s", arg.Kind, arg.Name)
		}

		b.WriteString(")")
	}

	return b.String()
}

// functionSignature renders the signature of an XS function, e.g.
// "int xsGetMapSeed()".
func functionSignature(fn kb.Function) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s %s(", fn.ReturnType, fn.Name)

	for i, p := range fn.Params {
		if i > 0 {
			b.WriteString(", ")
		}

		fmt.Fprintf(&b, "%s %s", p.Type, p.Name)
	}

	b.WriteString(")")

	return b.String()
}

// commandMarkdown renders an RMS command for hover: signature, description,
// game versions and introducing update.
func commandMarkdown(cmd kb.Command) string {
	var b strings.Builder

	fmt.Fprintf(&b, "**%s**\n\n%s", commandSignature(cmd), cmd.Desc)

	if cmd.GameVersions != "" {
		fmt.Fprintf(&b, "\n\nGame versions: %s.", cmd.GameVersions)
	}

	if cmd.SinceUpdate != "" {
		fmt.Fprintf(&b, "\n\nSince update %s.", cmd.SinceUpdate)
	}

	return b.String()
}

// attributeMarkdown renders a command-attribute hover: the owning command
// for context, then description, value shape and mined bounds (same
// vocabulary as signature-help labels).
func attributeMarkdown(owner string, attr kb.CommandArg) string {
	var b strings.Builder

	fmt.Fprintf(&b, "**%s** — attribute of `%s`", attr.Name, owner)

	if attr.Desc != "" {
		fmt.Fprintf(&b, "\n\n%s", attr.Desc)
	}

	if attr.Kind != "" {
		fmt.Fprintf(&b, "\n\nValue: `%s`", attr.Kind)

		if attr.Range != (kb.ValueRange{}) {
			fmt.Fprintf(&b, " (%s..%s)", attr.Range.Min, attr.Range.Max)
		}

		b.WriteString(".")
	}

	return b.String()
}

// functionMarkdown renders an XS function for hover: signature,
// description and introducing update.
func functionMarkdown(fn kb.Function) string {
	var b strings.Builder

	fmt.Fprintf(&b, "**%s**\n\n%s", functionSignature(fn), fn.Desc)

	if fn.SinceUpdate != "" {
		fmt.Fprintf(&b, "\n\nSince update %s.", fn.SinceUpdate)
	}

	return b.String()
}
