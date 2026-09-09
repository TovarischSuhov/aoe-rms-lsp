package server

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestInitialize_AdvertisesWorkspaceSymbol pins the capability surface:
// the workspace symbol provider must be advertised like its siblings.
func TestInitialize_AdvertisesWorkspaceSymbol(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	res, err := s.Initialize(context.Background(), &protocol.InitializeParams{})
	require.NoError(t, err)

	require.Equal(t, protocol.Boolean(true), res.Capabilities.WorkspaceSymbolProvider,
		"workspace/symbol must be advertised")
}

// TestServerSymbols_APIShape pins the empty-result contract: no open
// documents answer with an empty SymbolInformationSlice — never nil,
// never an error.
func TestServerSymbols_APIShape(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	res, err := s.Symbols(context.Background(), &protocol.WorkspaceSymbolParams{Query: "any"})

	require.NoError(t, err, "an empty universe is not an error")

	list, ok := res.(protocol.SymbolInformationSlice)
	require.True(t, ok, "result must be the SymbolInformationSlice arm")
	require.NotNil(t, list, "empty result must be an empty slice, not nil")
	require.Empty(t, list)
}

// TestServerSymbols_NonIncludedOpenDoc is the slot's acceptance
// criterion: a symbol of an open document that no other open document
// includes is still found — every open doc seeds the universe.
func TestServerSymbols_NonIncludedOpenDoc(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///main.rms",
		"<LAND_GENERATION>\nbase_terrain GRASS\n</LAND_GENERATION>\n", 1)
	s.docs.Put("file:///standalone.xs", "void isolatedHelper() { }\n", 1)

	res, err := s.Symbols(context.Background(), &protocol.WorkspaceSymbolParams{Query: "isolated"})
	require.NoError(t, err)

	list, ok := res.(protocol.SymbolInformationSlice)
	require.True(t, ok)

	require.Len(t, list, 1, "exactly the isolatedHelper declaration")
	require.Equal(t, "isolatedHelper", list[0].Name)
	require.Equal(t, protocol.SymbolKindFunction, list[0].Kind)
	require.Equal(t, uri.URI("file:///standalone.xs"), list[0].Location.URI)
	require.Equal(t, uint32(0), list[0].Location.Range.Start.Line)
}

// TestServerSymbols_ClosureSectionFromDisk pins the closure half of the
// universe: a section of a disk-backed include (not open in the editor)
// is found through the opener's closure.
func TestServerSymbols_ClosureSectionFromDisk(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	dir := t.TempDir()
	mainPath := dir + "/main.rms"
	econPath := dir + "/econ.rms"

	require.NoError(t, os.WriteFile(mainPath, []byte("#include \"econ.rms\"\n"), 0o644))
	require.NoError(t, os.WriteFile(econPath,
		[]byte("<ECONOMY_GENERATION>\ncreate_gold_mine\n</ECONOMY_GENERATION>\n"), 0o644))

	mainURI := uri.File(mainPath)
	s.docs.Put(string(mainURI), "#include \"econ.rms\"\n", 1)

	res, err := s.Symbols(context.Background(), &protocol.WorkspaceSymbolParams{Query: "econ"})
	require.NoError(t, err)

	list, ok := res.(protocol.SymbolInformationSlice)
	require.True(t, ok)

	var found bool

	for _, si := range list {
		if si.Name == "economy_generation" && si.Location.URI == uri.File(econPath) {
			found = true
		}
	}

	require.True(t, found, "the disk-backed econ section must be found via the closure (rms lowercases section names)")
}

// TestServerSymbols_RmsCommandsExcluded pins the noise constraint: RMS
// command statements never enter the result — only sections and XS
// declarations do.
func TestServerSymbols_RmsCommandsExcluded(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///t.rms",
		"<LAND_GENERATION>\ncreate_land {\n\tland_percent 5\n}\n</LAND_GENERATION>\n", 1)
	s.docs.Put("file:///lib.xs", "void createWidget() { }\n", 1)

	res, err := s.Symbols(context.Background(), &protocol.WorkspaceSymbolParams{Query: "create"})
	require.NoError(t, err)

	list, ok := res.(protocol.SymbolInformationSlice)
	require.True(t, ok)

	require.Len(t, list, 1, "only createWidget; create_land stays out")
	require.Equal(t, "createWidget", list[0].Name)
	require.Equal(t, uri.URI("file:///lib.xs"), list[0].Location.URI)
}

// TestServerSymbols_EmptyQueryAllDeterministic pins the empty-query
// semantics (the whole universe) and the deterministic order: score,
// then URI, then position; an identical request answers identically.
func TestServerSymbols_EmptyQueryAllDeterministic(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///b.rms", "<LAND_GENERATION>\n</LAND_GENERATION>\n", 1)
	s.docs.Put("file:///a.xs", "void alphaFn() { }\nint betaVar = 1;\n", 1)

	res, err := s.Symbols(context.Background(), &protocol.WorkspaceSymbolParams{Query: ""})
	require.NoError(t, err)

	first, ok := res.(protocol.SymbolInformationSlice)
	require.True(t, ok)
	require.Len(t, first, 3, "two XS decls + one RMS section")

	names := []string{first[0].Name, first[1].Name, first[2].Name}
	require.Equal(t, []string{"alphaFn", "betaVar", "land_generation"}, names,
		"URI order first (a.xs before b.rms), then position; rms lowercases section names")

	again, err := s.Symbols(context.Background(), &protocol.WorkspaceSymbolParams{Query: ""})
	require.NoError(t, err)
	require.Equal(t, first, again.(protocol.SymbolInformationSlice), "identical request, identical answer")
}

// TestServerSymbols_UnknownExtensionSkipped pins the silent skip: an
// open document with an unknown extension contributes nothing and
// breaks nothing.
func TestServerSymbols_UnknownExtensionSkipped(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///notes.txt", "create_land {\n\tland_percent 5\n}\n", 1)

	res, err := s.Symbols(context.Background(), &protocol.WorkspaceSymbolParams{Query: "create"})

	require.NoError(t, err)
	require.NotNil(t, res)

	list, ok := res.(protocol.SymbolInformationSlice)
	require.True(t, ok)
	require.Empty(t, list)
}

// TestServerWorkspaceSymbol_IntegrationStdio runs the slot's acceptance
// scenarios over the real stdio entrypoint: a disk-backed closure
// declaration and a non-included open document are both found by a
// fuzzy query.
func TestServerWorkspaceSymbol_IntegrationStdio(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	dir := t.TempDir()
	libPath := dir + "/lib.xs"

	require.NoError(t, os.WriteFile(libPath,
		[]byte("void sharedFn(int n) { }\n"), 0o644))

	mainURI := uri.File(dir + "/main.rms")
	standaloneURI := uri.File(dir + "/standalone.xs")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: mainURI, LanguageID: "aoe2rms", Version: 1,
			Text: "#includeXS lib.xs\n<LAND_GENERATION>\nbase_terrain GRASS\n</LAND_GENERATION>\n",
		},
	}))
	h.waitDiagnostics(mainURI)

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: standaloneURI, LanguageID: "aoe2xs", Version: 1,
			Text: "void isolatedHelper() { }\n",
		},
	}))
	h.waitDiagnostics(standaloneURI)

	// A disk-backed closure declaration: lib.xs is not open anywhere.
	res, err := h.disp.Symbols(ctx, &protocol.WorkspaceSymbolParams{Query: "shfn"})
	require.NoError(t, err)

	list, ok := res.(protocol.SymbolInformationSlice)
	require.True(t, ok, "SymbolInformationSlice arm over stdio")
	require.Len(t, list, 1, "exactly sharedFn from the disk-backed lib.xs")
	require.Equal(t, "sharedFn", list[0].Name)
	require.Equal(t, uri.File(libPath), list[0].Location.URI)

	// The slot's headline criterion: a symbol of an open document that
	// no other open document includes.
	res, err = h.disp.Symbols(ctx, &protocol.WorkspaceSymbolParams{Query: "isolated"})
	require.NoError(t, err)

	list, ok = res.(protocol.SymbolInformationSlice)
	require.True(t, ok)
	require.Len(t, list, 1, "exactly isolatedHelper from the standalone open doc")
	require.Equal(t, "isolatedHelper", list[0].Name)
	require.Equal(t, standaloneURI, list[0].Location.URI)
}
