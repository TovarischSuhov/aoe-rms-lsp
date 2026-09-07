package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// crossFileFixture writes the multi-file tree from the task document:
// main.rms includes parts/econ.rms and parts/lib.xs; the inline block
// calls sharedFn.
func crossFileFixture(t *testing.T) map[string]uri.URI {
	t.Helper()

	dir := t.TempDir()
	tree := map[string]string{
		"maps/main.rms":       "#include \"parts/econ.rms\"\n#includeXS parts/lib.xs\nvoid main() { sharedFn(1); }\n",
		"maps/parts/econ.rms": "base_terrain GRASS\n",
		"maps/parts/lib.xs":   "void sharedFn(int n) { }\nvoid caller() { sharedFn(2); }\n",
		"maps/broken.rms":     "#include \"missing.rms\"\n",
	}

	uris := make(map[string]uri.URI, len(tree))
	for name, content := range tree {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
		uris[name] = uri.File(path)
	}

	return uris
}

// TestServerCrossFile_IntegrationStdio runs the acceptance scenarios 1-5
// of docs/tasks/cross-file-navigation.md over the real stdio entrypoint.
func TestServerCrossFile_IntegrationStdio(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	f := crossFileFixture(t)

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: f["maps/main.rms"], LanguageID: "aoe2rms", Version: 1,
			Text: "#include \"parts/econ.rms\"\n#includeXS parts/lib.xs\nvoid main() { sharedFn(1); }\n",
		},
	}))
	h.waitDiagnostics(f["maps/main.rms"])

	// Scenario 2 depends on lib being open only for scenario 3; open it now.
	libText := "void sharedFn(int n) { }\nvoid caller() { sharedFn(2); }\n"
	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: f["maps/parts/lib.xs"], LanguageID: "aoe2xs", Version: 1, Text: libText,
		},
	}))
	h.waitDiagnostics(f["maps/parts/lib.xs"])

	// Scenario 1: definition on the include path jumps into the target.
	defRes, err := h.disp.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: f["maps/main.rms"]},
			Position:     protocol.Position{Line: 0, Character: 13},
		},
	})
	require.NoError(t, err)

	loc, ok := defRes.(*protocol.Location)
	require.True(t, ok, "scenario 1: a single Location into the target file")
	assert.Equal(t, f["maps/parts/econ.rms"], loc.URI)
	assert.Equal(t, protocol.Position{Line: 0, Character: 0}, loc.Range.Start)

	// Scenario 2: definition on the inline sharedFn call jumps into lib.xs.
	defRes, err = h.disp.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: f["maps/main.rms"]},
			Position:     protocol.Position{Line: 2, Character: 18},
		},
	})
	require.NoError(t, err)

	loc, ok = defRes.(*protocol.Location)
	require.True(t, ok, "scenario 2: a single Location into lib.xs")
	assert.Equal(t, f["maps/parts/lib.xs"], loc.URI)
	assert.Equal(t, uint32(0), loc.Range.Start.Line)
	assert.Equal(t, uint32(len("void ")), loc.Range.Start.Character)
	assert.Equal(t, uint32(len("void sharedFn")), loc.Range.End.Character)

	// Scenario 3: references from lib.xs find the main.rms inline call too.
	refRes, err := h.disp.References(ctx, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: f["maps/parts/lib.xs"]},
			Position:     protocol.Position{Line: 0, Character: 8},
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})
	require.NoError(t, err)

	byURI := map[string]int{}
	for _, l := range refRes {
		byURI[string(l.URI)]++
	}
	assert.Equal(t, 2, byURI[string(f["maps/parts/lib.xs"])], "declaration plus call in lib.xs")
	assert.Equal(t, 1, byURI[string(f["maps/main.rms"])], "the inline call in main.rms")

	// Scenario 4: a missing include surfaces as a missing-include diagnostic.
	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: f["maps/broken.rms"], LanguageID: "aoe2rms", Version: 1,
			Text: "#include \"missing.rms\"\n",
		},
	}))
	batch := h.waitDiagnostics(f["maps/broken.rms"])

	assert.Contains(t, diagnosticCodes(t, batch), "missing-include")
	for _, d := range batch.Diagnostics {
		if code, _ := d.Code.(protocol.String); code == "missing-include" {
			assert.Equal(t, uint32(0), d.Range.Start.Line)
		}
	}
}

// TestServerCrossFile_EditorStateWins is scenario 5: navigation through
// an open document uses its editor version, not the disk copy.
func TestServerCrossFile_EditorStateWins(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.rms")
	econPath := filepath.Join(dir, "econ.rms")

	require.NoError(t, os.WriteFile(mainPath,
		[]byte("#include \"econ.rms\"\nbase_terrain GRASS\n"), 0o644))
	require.NoError(t, os.WriteFile(econPath,
		[]byte("base_terrain GRASS\n"), 0o644)) // one occurrence on disk

	mainURI := uri.File(mainPath)
	econURI := uri.File(econPath)

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: mainURI, LanguageID: "aoe2rms", Version: 1,
			Text: "#include \"econ.rms\"\nbase_terrain GRASS\n",
		},
	}))
	h.waitDiagnostics(mainURI)

	// The editor's econ version has TWO grass lines; navigation must see
	// this version (1 in main + 2 in econ), not the disk copy (1 + 1).
	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: econURI, LanguageID: "aoe2rms", Version: 1,
			Text: "base_terrain GRASS\nbase_terrain GRASS\n",
		},
	}))
	h.waitDiagnostics(econURI)

	refRes, err := h.disp.References(ctx, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: mainURI},
			Position:     protocol.Position{Line: 1, Character: 17},
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})
	require.NoError(t, err)

	byURI := map[string]int{}
	for _, l := range refRes {
		byURI[string(l.URI)]++
	}

	assert.Equal(t, 1, byURI[string(mainURI)], "the queried occurrence in main")
	assert.Equal(t, 2, byURI[string(econURI)], "the EDITOR version of econ (2 lines), not the disk copy (1)")
}
