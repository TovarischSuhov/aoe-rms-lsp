package server

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"aoe2-lsp/analysis"
	"aoe2-lsp/hints"
	"aoe2-lsp/kb"
)

// newNavigationServer builds a server over the real knowledge base.
func newNavigationServer(t *testing.T) *Server {
	t.Helper()

	store, err := kb.NewStore()
	require.NoError(t, err)

	return NewServer(store, analysis.NewAnalyzer(store), hints.NewComputer(store))
}

func TestServerDefinition_APIShape(t *testing.T) {
	s := newNavigationServer(t)

	res, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///closed.xs"},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})

	require.NoError(t, err)
	empty, ok := res.(protocol.LocationSlice)
	require.True(t, ok, "a closed document resolves to an empty LocationSlice")
	require.Empty(t, empty)
}

func TestInitialize_AdvertisesNavigation(t *testing.T) {
	s := newNavigationServer(t)

	res, err := s.Initialize(context.Background(), &protocol.InitializeParams{})
	require.NoError(t, err)

	caps := res.Capabilities
	require.Equal(t, protocol.Boolean(true), caps.DefinitionProvider)
	require.Equal(t, protocol.Boolean(true), caps.ReferencesProvider)
	require.Equal(t, protocol.Boolean(true), caps.DocumentSymbolProvider)

	require.NotNil(t, caps.HoverProvider, "existing capabilities stay intact")
	require.NotNil(t, caps.CompletionProvider)
	require.NotNil(t, caps.TextDocumentSync)
}

func TestServerDefinition_XsLocation(t *testing.T) {
	s := newNavigationServer(t)

	src := "void f() {}\nvoid g() { f(); }"
	s.docs.Put("file:///t.xs", src, 1)

	res, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.xs"},
			Position:     protocol.Position{Line: 1, Character: uint32(strings.Index("void g() { f(); }", "f("))},
		},
	})

	require.NoError(t, err)

	loc, ok := res.(*protocol.Location)
	require.True(t, ok, "a resolved definition is a single Location")

	require.Equal(t, "file:///t.xs", string(loc.URI))
	require.Equal(t, protocol.Position{Line: 0, Character: uint32(len("void "))}, loc.Range.Start)
	require.Equal(t, uint32(len("void f")), loc.Range.End.Character)
}

func TestServerDefinition_EmptyNotNil(t *testing.T) {
	s := newNavigationServer(t)

	tests := []struct {
		name string
		uri  string
		src  string
		pos  protocol.Position
	}{
		{
			name: "rms document",
			uri:  "file:///t.rms",
			src:  "create_land TERRAIN_GRASS\n",
			pos:  protocol.Position{Line: 0, Character: 12},
		},
		{
			name: "closed document",
			uri:  "file:///closed.xs",
			src:  "",
			pos:  protocol.Position{Line: 0, Character: 0},
		},
		{
			name: "builtin callee",
			uri:  "file:///b.xs",
			src:  "void h() { xsSetWorldGravity(1.0); }",
			pos:  protocol.Position{Line: 0, Character: 11},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.src != "" {
				s.docs.Put(tt.uri, tt.src, 1)
			}

			res, err := s.Definition(context.Background(), &protocol.DefinitionParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(tt.uri)},
					Position:     tt.pos,
				},
			})

			require.NoError(t, err, "an empty result is not an error")

			empty, ok := res.(protocol.LocationSlice)
			require.True(t, ok, "empty result must be an empty LocationSlice, not nil")
			require.Empty(t, empty)
		})
	}
}

func TestServerReferences_APIShape(t *testing.T) {
	s := newNavigationServer(t)

	locs, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///closed.xs"},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})

	require.NoError(t, err)
	require.NotNil(t, locs)
	require.Empty(t, locs)
}

func TestServerReferences_ExcludesDeclarationOnFlag(t *testing.T) {
	s := newNavigationServer(t)

	src := "void f() {}\nvoid g() { f(); }"
	s.docs.Put("file:///t.xs", src, 1)

	callPos := protocol.Position{Line: 1, Character: uint32(strings.Index("void g() { f(); }", "f("))}

	withoutDecl, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.xs"},
			Position:     callPos,
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: false},
	})
	require.NoError(t, err)
	require.Len(t, withoutDecl, 1, "only the call remains")
	require.Equal(t, uint32(1), withoutDecl[0].Range.Start.Line)

	withDecl, err := s.References(context.Background(), &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.xs"},
			Position:     callPos,
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})
	require.NoError(t, err)
	require.Len(t, withDecl, 2, "declaration + call")
}

func TestServerDocumentSymbol_APIShape(t *testing.T) {
	s := newNavigationServer(t)

	res, err := s.DocumentSymbol(context.Background(), &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///closed.xs"},
	})
	require.NoError(t, err)

	empty, ok := res.(protocol.DocumentSymbolSlice)
	require.True(t, ok)
	require.Empty(t, empty)
}

func TestServerDocumentSymbol_KindTable(t *testing.T) {
	s := newNavigationServer(t)

	s.docs.Put("file:///t.xs", "void f() {}\nint x = 1;\nrule r { condition x }\n", 1)
	s.docs.Put("file:///t.rms", "<LAND_GENERATION>\ncreate_player_lands {\n	land_percent 32\n}\n#includeXS\nvoid h() {}\n</LAND_GENERATION>\n", 1)

	xsRes, err := s.DocumentSymbol(context.Background(), &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.xs"},
	})
	require.NoError(t, err)

	xsSymbols := xsRes.(protocol.DocumentSymbolSlice)
	require.Len(t, xsSymbols, 3)
	require.Equal(t, protocol.SymbolKindFunction, xsSymbols[0].Kind)
	require.Equal(t, "f", xsSymbols[0].Name)
	require.Equal(t, protocol.SymbolKindVariable, xsSymbols[1].Kind)
	require.Equal(t, protocol.SymbolKindEvent, xsSymbols[2].Kind)

	for _, sym := range xsSymbols {
		require.True(t, symbolRangeContains(sym.Range, sym.SelectionRange),
			"SelectionRange must stay inside Range: %s", sym.Name)
	}

	rmsRes, err := s.DocumentSymbol(context.Background(), &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.rms"},
	})
	require.NoError(t, err)

	rmsSymbols := rmsRes.(protocol.DocumentSymbolSlice)
	require.Len(t, rmsSymbols, 1)

	section := rmsSymbols[0]
	require.Equal(t, protocol.SymbolKindModule, section.Kind)
	require.Len(t, section.Children, 2, "command plus the xs block")

	require.Equal(t, protocol.SymbolKindFunction, section.Children[0].Kind)
	require.Equal(t, "create_player_lands", section.Children[0].Name)

	require.Equal(t, protocol.SymbolKindNamespace, section.Children[1].Kind)
	require.Equal(t, "#includeXS", section.Children[1].Name)

	for _, sym := range append(rmsSymbols, section.Children...) {
		require.True(t, symbolRangeContains(sym.Range, sym.SelectionRange),
			"SelectionRange must stay inside Range: %s", sym.Name)
	}
}

func TestServerDocumentSymbol_EmptyDoc(t *testing.T) {
	s := newNavigationServer(t)

	s.docs.Put("file:///e.rms", "\n", 1)

	res, err := s.DocumentSymbol(context.Background(), &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///e.rms"},
	})
	require.NoError(t, err)

	empty, ok := res.(protocol.DocumentSymbolSlice)
	require.True(t, ok)
	require.Empty(t, empty)
}

// symbolRangeContains reports whether inner stays inside outer.
func symbolRangeContains(outer protocol.Range, inner protocol.Range) bool {
	if inner.Start.Line < outer.Start.Line || inner.Start.Line > outer.End.Line {
		return false
	}

	if inner.Start.Line == outer.Start.Line && inner.Start.Character < outer.Start.Character {
		return false
	}

	if inner.Start.Line == outer.End.Line && inner.Start.Character > outer.End.Character {
		return false
	}

	return true
}

// TestServerDefinition_CrossFileInclude checks the handler wiring: a
// Definition on an include path returns a Location in another file.
func TestServerDefinition_CrossFileInclude(t *testing.T) {
	s := newNavigationServer(t)

	dir := t.TempDir()
	econPath := dir + "/econ.rms"
	require.NoError(t, os.WriteFile(econPath, []byte("base_terrain GRASS\n"), 0o644))

	main := "#include \"econ.rms\"\n"
	s.docs.Put(uri.File(dir+"/main.rms").String(), main, 1)

	res, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.File(dir + "/main.rms")},
			Position:     protocol.Position{Line: 0, Character: 13},
		},
	})
	require.NoError(t, err)

	loc, ok := res.(*protocol.Location)
	require.True(t, ok, "an include target is a single Location")
	require.Equal(t, uri.File(econPath), loc.URI)
	require.Equal(t, protocol.Position{Line: 0, Character: 0}, loc.Range.Start)
}

// TestServerDefinition_ClosedDocFromDisk checks that a closed document
// with a disk copy still answers (the open-document gate is gone).
func TestServerDefinition_ClosedDocFromDisk(t *testing.T) {
	s := newNavigationServer(t)

	dir := t.TempDir()
	xsPath := dir + "/lib.xs"
	require.NoError(t, os.WriteFile(xsPath, []byte("void f() {}\n"), 0o644))

	res, err := s.Definition(context.Background(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.File(xsPath)},
			Position:     protocol.Position{Line: 0, Character: 5},
		},
	})
	require.NoError(t, err)

	loc, ok := res.(*protocol.Location)
	require.True(t, ok)
	require.Equal(t, uri.File(xsPath), loc.URI)
}

// TestAnalyze_MissingIncludeDiagnostic checks that a missing include
// surfaces as one missing-include diagnostic on the path argument.
func TestAnalyze_MissingIncludeDiagnostic(t *testing.T) {
	s := newNavigationServer(t)

	dir := t.TempDir()
	mainURI := uri.File(dir + "/main.rms").String()

	s.docs.Put(mainURI, "#include \"nope.rms\"\n", 1)

	closure := s.resolver.Closure(context.Background(), mainURI)
	diags := s.analyze(mainURI, "#include \"nope.rms\"\n", closure)

	require.Len(t, diags, 1)
	require.Equal(t, protocol.String("missing-include"), diags[0].Code)
	require.Equal(t, uint32(0), diags[0].Range.Start.Line)
}

// TestAnalyze_InlineSeesClosureDeclarations checks that an inline XS call
// resolves through the closure: no undefined-symbol for sharedFn.
func TestAnalyze_InlineSeesClosureDeclarations(t *testing.T) {
	s := newNavigationServer(t)

	dir := t.TempDir()
	libPath := dir + "/lib.xs"
	require.NoError(t, os.WriteFile(libPath, []byte("void sharedFn(int n) { }\n"), 0o644))

	mainURI := uri.File(dir + "/main.rms").String()
	main := "#includeXS lib.xs\nvoid main() { sharedFn(1); }\n"
	s.docs.Put(mainURI, main, 1)

	closure := s.resolver.Closure(context.Background(), mainURI)
	diags := s.analyze(mainURI, main, closure)

	for _, d := range diags {
		require.NotEqual(t, "undefined-symbol", d.Code,
			"closure declarations must suppress undefined-symbol, got: %v", diags)
	}
}

// TestAnalyze_XsRootExcludesItself checks the .xs pipeline: analyzing the
// included file itself does not seed its own declarations as externals.
func TestAnalyze_XsRootExcludesItself(t *testing.T) {
	s := newNavigationServer(t)

	libURI := "file:///lib.xs"
	lib := "void sharedFn(int n) { }\nvoid bad() { ghost(); }\n"
	s.docs.Put(libURI, lib, 1)

	closure := s.resolver.Closure(context.Background(), libURI)
	diags := s.analyze(libURI, lib, closure)

	codes := make([]string, 0, len(diags))
	for _, d := range diags {
		codes = append(codes, fmt.Sprint(d.Code))
	}

	require.Contains(t, codes, "undefined-symbol", "ghost stays unknown")
	require.NotContains(t, codes, "missing-include")
}
