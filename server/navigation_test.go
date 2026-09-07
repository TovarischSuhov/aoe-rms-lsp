package server

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"aoe2-lsp/analysis"
	"aoe2-lsp/kb"
)

// newNavigationServer builds a server over the real knowledge base.
func newNavigationServer(t *testing.T) *Server {
	t.Helper()

	store, err := kb.NewStore()
	require.NoError(t, err)

	return NewServer(store, analysis.NewAnalyzer(store))
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
