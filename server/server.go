package server

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"aoe2-lsp/analysis"
	"aoe2-lsp/common"
	"aoe2-lsp/kb"
	"aoe2-lsp/rms"
	"aoe2-lsp/xs"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// serverName identifies the server in Initialize responses and diagnostics.
const serverName = "aoe2-lsp"

// Server is the LSP server: the protocol surface over the kb, rms, xs and
// analysis cells.
type Server struct {
	protocol.UnimplementedServer

	store    *kb.Store
	analyzer *analysis.Analyzer
	docs     *DocStore
	exit     chan struct{}
	exitOnce sync.Once
}

// NewServer builds the server from its knowledge base and analyzer (DI);
// the document cache is created internally.
func NewServer(store *kb.Store, analyzer *analysis.Analyzer) *Server {
	return &Server{
		store:    store,
		analyzer: analyzer,
		docs:     NewDocStore(),
		exit:     make(chan struct{}),
	}
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
		HoverProvider:          protocol.Boolean(true),
		CompletionProvider:     &protocol.CompletionOptions{TriggerCharacters: []string{" ", "<"}},
		DefinitionProvider:     protocol.Boolean(true),
		ReferencesProvider:     protocol.Boolean(true),
		DocumentSymbolProvider: protocol.Boolean(true),
	}

	if enc, ok := negotiateEncoding(params); ok {
		caps.PositionEncoding = enc
	}

	return &protocol.InitializeResult{
		Capabilities: caps,
		ServerInfo:   protocol.ServerInfo{Name: serverName},
	}, nil
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
) error {
	doc := params.TextDocument
	s.docs.Put(string(doc.URI), doc.Text, int(doc.Version))
	s.publishDiagnostics(ctx, doc.URI)

	return nil
}

// DidChange replaces the stored text (Full sync) and republishes
// diagnostics; one change batch per notification.
func (s *Server) DidChange(
	ctx context.Context,
	params *protocol.DidChangeTextDocumentParams,
) error {
	docURI := params.TextDocument.URI
	text, _, _ := s.docs.Get(string(docURI))

	for _, change := range params.ContentChanges {
		if c, ok := change.(*protocol.TextDocumentContentChangeWholeDocument); ok {
			text = c.Text
		}
	}

	s.docs.Put(string(docURI), text, int(params.TextDocument.Version))
	s.publishDiagnostics(ctx, docURI)

	return nil
}

// DidClose drops the document and clears its published diagnostics.
func (s *Server) DidClose(
	ctx context.Context,
	params *protocol.DidCloseTextDocumentParams,
) error {
	docURI := params.TextDocument.URI
	s.docs.Remove(string(docURI))
	s.publish(ctx, docURI, nil)

	return nil
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

	pos := fromProtocolPos(params.Position)

	var markdown string

	switch {
	case strings.HasSuffix(name, ".rms"):
		markdown = s.hoverRms(text, name, pos)
	case strings.HasSuffix(name, ".xs"):
		markdown = s.hoverXs(text, name, pos)
	}

	if markdown == "" {
		return nil, nil
	}

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

// Completion answers with the kb entries in scope: for .rms the commands of
// the section at pos plus all constants, for .xs the functions and
// constants matching the word prefix at pos.
func (s *Server) Completion(
	ctx context.Context,
	params *protocol.CompletionParams,
) (protocol.CompletionResult, error) {
	text, name, ok := s.openDocument(params.TextDocument.URI)
	if !ok {
		return &protocol.CompletionList{Items: []protocol.CompletionItem{}}, nil
	}

	pos := fromProtocolPos(params.Position)

	var items []protocol.CompletionItem

	switch {
	case strings.HasSuffix(name, ".rms"):
		items = s.completionsRms(text, name, pos)
	case strings.HasSuffix(name, ".xs"):
		items = s.completionsXs(text, pos)
	}

	return &protocol.CompletionList{Items: items}, nil
}

// completionsRms lists the commands of the section containing pos plus all
// constants.
func (s *Server) completionsRms(
	text string,
	name string,
	pos common.Pos,
) []protocol.CompletionItem {
	file, _ := rms.Parse(text, name)

	sec, ok := file.SectionAt(pos)
	if !ok {
		return nil
	}

	cmds := s.store.Commands(sec.Name)
	consts := s.store.Constants("")

	items := make([]protocol.CompletionItem, 0, len(cmds)+len(consts))

	for _, cmd := range cmds {
		items = append(items, protocol.CompletionItem{
			Label:         cmd.Name,
			Kind:          protocol.CompletionItemKindKeyword,
			Detail:        protocol.NewOptional(commandSignature(cmd)),
			Documentation: protocol.String(cmd.Desc),
		})
	}

	for _, c := range consts {
		items = append(items, protocol.CompletionItem{
			Label:         c.Name,
			Kind:          protocol.CompletionItemKindConstant,
			Detail:        protocol.NewOptional(c.Value),
			Documentation: protocol.String(c.Desc),
		})
	}

	return items
}

// completionsXs lists functions and constants matching (case-insensitively)
// the identifier prefix ending at pos; an empty prefix matches everything.
func (s *Server) completionsXs(text string, pos common.Pos) []protocol.CompletionItem {
	prefix := strings.ToLower(wordPrefix(text, pos))

	fns := s.store.Functions()
	consts := s.store.Constants("")

	items := make([]protocol.CompletionItem, 0, len(fns)+len(consts))

	for _, fn := range fns {
		if !matchesPrefix(fn.Name, prefix) {
			continue
		}

		items = append(items, protocol.CompletionItem{
			Label:         fn.Name,
			Kind:          protocol.CompletionItemKindFunction,
			Detail:        protocol.NewOptional(functionSignature(fn)),
			Documentation: protocol.String(fn.Desc),
		})
	}

	for _, c := range consts {
		if !matchesPrefix(c.Name, prefix) {
			continue
		}

		items = append(items, protocol.CompletionItem{
			Label:         c.Name,
			Kind:          protocol.CompletionItemKindConstant,
			Detail:        protocol.NewOptional(c.Value),
			Documentation: protocol.String(c.Desc),
		})
	}

	return items
}

// matchesPrefix reports whether name starts with the lowercase prefix,
// comparing case-insensitively.
func matchesPrefix(name string, prefix string) bool {
	return strings.HasPrefix(strings.ToLower(name), prefix)
}

// Definition answers textDocument/definition: the declaration of the
// symbol under the cursor in an .xs document. Not-found positions, .rms
// documents and closed documents resolve to an empty LocationSlice
// (not nil); an empty result is not an error.
func (s *Server) Definition(
	ctx context.Context,
	params *protocol.DefinitionParams,
) (protocol.DefinitionResult, error) {
	text, name, ok := s.openDocument(params.TextDocument.URI)
	if !ok || !strings.HasSuffix(name, ".xs") {
		return protocol.LocationSlice{}, nil
	}

	file, _ := xs.XsParse(text, name)

	r, found := file.Definition(fromProtocolPos(params.Position))
	if !found {
		return protocol.LocationSlice{}, nil
	}

	return &protocol.Location{
		URI:   params.TextDocument.URI,
		Range: toProtocolRange(r),
	}, nil
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

	s.publish(ctx, docURI, s.analyze(string(docURI), text))
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

	_ = client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         docURI,
		Diagnostics: diags,
	})
}

// analyze runs the diagnostics pipeline for one document version: parse,
// semantic checks and inline XS blocks (for .rms), merged and sorted.
func (s *Server) analyze(name string, text string) []protocol.Diagnostic {
	var diags []common.Diagnostic

	switch {
	case strings.HasSuffix(name, ".rms"):
		diags = s.analyzeRms(name, text)
	case strings.HasSuffix(name, ".xs"):
		diags = s.analyzeXs(name, text)
	}

	sortDiags(diags)

	return toProtocolDiags(diags)
}

// analyzeRms parses the RMS source, runs semantic checks and delegates each
// inline XS block to the XS pipeline with ranges shifted into document
// coordinates.
func (s *Server) analyzeRms(name string, text string) []common.Diagnostic {
	file, syntax := rms.Parse(text, name)

	diags := append(syntax, s.analyzer.AnalyzeRms(file)...)

	for _, block := range file.XsBlocks {
		xsFile, xsSyntax := xs.XsParse(block.Code, "inline:"+name)
		base := block.Range.Start

		diags = append(diags, shiftDiags(xsSyntax, base)...)
		diags = append(diags, shiftDiags(s.analyzer.AnalyzeXs(xsFile), base)...)
	}

	return diags
}

// analyzeXs parses the XS source and runs semantic checks.
func (s *Server) analyzeXs(name string, text string) []common.Diagnostic {
	file, syntax := xs.XsParse(text, name)

	return append(syntax, s.analyzer.AnalyzeXs(file)...)
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

// toProtocolDiags converts shared diagnostics to the protocol shape.
func toProtocolDiags(diags []common.Diagnostic) []protocol.Diagnostic {
	out := make([]protocol.Diagnostic, 0, len(diags))

	for _, d := range diags {
		out = append(out, protocol.Diagnostic{
			Range:    toProtocolRange(d.Range),
			Severity: protocol.DiagnosticSeverity(d.Severity),
			Code:     protocol.String(d.Code),
			Source:   protocol.NewOptional(serverName),
			Message:  protocol.String(d.Message),
		})
	}

	return out
}

// toProtocolPos converts a shared position to the protocol shape (Offset is
// protocol-external and dropped).
func toProtocolPos(p common.Pos) protocol.Position {
	return protocol.Position{Line: p.Line, Character: p.Column}
}

// fromProtocolPos converts a protocol position to the shared shape; the
// byte offset is recomputed on demand by the parsers.
func fromProtocolPos(p protocol.Position) common.Pos {
	return common.Pos{Line: p.Line, Column: p.Character}
}

// toProtocolRange converts a shared range to the protocol shape.
func toProtocolRange(r common.Range) protocol.Range {
	return protocol.Range{Start: toProtocolPos(r.Start), End: toProtocolPos(r.End)}
}

// wordPrefix returns the identifier prefix ending at pos in text.
func wordPrefix(text string, pos common.Pos) string {
	lines := strings.Split(text, "\n")
	if int(pos.Line) >= len(lines) {
		return ""
	}

	line := lines[pos.Line]

	end := min(int(pos.Column), len(line))

	start := end

	for start > 0 && isIdentByte(line[start-1]) {
		start--
	}

	return line[start:end]
}

// isIdentByte reports whether b can appear in an XS identifier.
func isIdentByte(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
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
