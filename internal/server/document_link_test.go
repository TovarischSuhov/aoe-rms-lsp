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

// documentLinkFixture writes a map with two resolvable directives —
// #includeXS first, #include second, so document order differs from
// the closure's kind-grouped order — and one missing target.
func documentLinkFixture(t *testing.T) (main, econ, lib uri.URI) {
	t.Helper()

	dir := t.TempDir()
	src := "#includeXS parts/lib.xs\n#include \"parts/econ.rms\"\n#include \"missing.rms\"\n"
	tree := map[string]string{
		"main.rms":       src,
		"parts/econ.rms": "base_terrain GRASS\n",
		"parts/lib.xs":   "void sharedFn(int n) { }\n",
	}

	for name, content := range tree {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	return uri.File(filepath.Join(dir, "main.rms")),
		uri.File(filepath.Join(dir, "parts", "econ.rms")),
		uri.File(filepath.Join(dir, "parts", "lib.xs"))
}

// TestInitialize_AdvertisesDocumentLink pins the capability surface.
func TestInitialize_AdvertisesDocumentLink(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	res, err := s.Initialize(context.Background(), &protocol.InitializeParams{})
	require.NoError(t, err)

	require.NotNil(t, res.Capabilities.DocumentLinkProvider,
		"documentLink must be advertised")
}

// TestServerDocumentLink_APIShape pins the empty-result contract:
// unknown documents and .xs documents answer with an empty slice,
// never nil, never an error.
func TestServerDocumentLink_APIShape(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	links, err := s.DocumentLink(context.Background(), &protocol.DocumentLinkParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///closed.rms"},
	})

	require.NoError(t, err, "an empty result is not an error")
	require.NotNil(t, links)
	require.Empty(t, links)

	s.docs.Put("file:///t.xs", "void f() { }\n", 1)

	links, err = s.DocumentLink(context.Background(), &protocol.DocumentLinkParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.xs"},
	})

	require.NoError(t, err)
	require.NotNil(t, links, ".xs documents get an empty slice, not nil")
	require.Empty(t, links)
}

// TestServerDocumentLink_ResolvedDirectives pins the happy path: every
// resolved directive of the queried document becomes a link whose range
// covers the path argument and whose target is the resolved file, in
// document order (the closure groups #include before #includeXS);
// the missing directive is skipped — its signal is the diagnostic.
func TestServerDocumentLink_ResolvedDirectives(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	main, econ, lib := documentLinkFixture(t)
	src := "#includeXS parts/lib.xs\n#include \"parts/econ.rms\"\n#include \"missing.rms\"\n"
	s.docs.Put(string(main), src, 1)

	links, err := s.DocumentLink(context.Background(), &protocol.DocumentLinkParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: main},
	})

	require.NoError(t, err)
	require.Len(t, links, 2, "two resolved directives; missing.rms is skipped")

	require.NotNil(t, links[0].Target)
	assert.Equal(t, lib, *links[0].Target, "document order: #includeXS on line 0 comes first")
	assert.Equal(t, protocol.Range{
		Start: protocol.Position{Line: 0, Character: 11},
		End:   protocol.Position{Line: 0, Character: 23},
	}, links[0].Range, "the range covers the path argument")

	require.NotNil(t, links[1].Target)
	assert.Equal(t, econ, *links[1].Target)
	assert.Equal(t, protocol.Range{
		Start: protocol.Position{Line: 1, Character: 9},
		End:   protocol.Position{Line: 1, Character: 25},
	}, links[1].Range)
}
