package server

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestInitialize_AdvertisesDocumentHighlight pins the capability surface:
// the highlight provider must be advertised like its navigation siblings.
func TestInitialize_AdvertisesDocumentHighlight(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	res, err := s.Initialize(context.Background(), &protocol.InitializeParams{})
	require.NoError(t, err)

	require.Equal(t, protocol.Boolean(true), res.Capabilities.DocumentHighlightProvider,
		"documentHighlight must be advertised")
}

// TestServerDocumentHighlight_APIShape pins the empty-result contract: a
// closed document answers with an empty slice, never nil, never an error.
func TestServerDocumentHighlight_APIShape(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	highlights, err := s.DocumentHighlight(context.Background(), &protocol.DocumentHighlightParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///closed.xs"},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})

	require.NoError(t, err, "an empty result is not an error")
	require.NotNil(t, highlights)
	require.Empty(t, highlights)
}

// TestServerDocumentHighlight_EmptyResults pins the silence shape: a
// position outside any word token, an unknown extension and an empty
// document all answer with an empty slice, never nil, never an error.
func TestServerDocumentHighlight_EmptyResults(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	tests := []struct {
		name string
		uri  string
		src  string
		pos  protocol.Position
	}{
		{
			name: "position outside a word token",
			uri:  "file:///t.xs",
			src:  "void f() {}\n\nvoid g() { f(); }",
			pos:  protocol.Position{Line: 1, Character: 0},
		},
		{
			name: "unknown extension",
			uri:  "file:///notes.txt",
			src:  "create_land TERRAIN_GRASS\n",
			pos:  protocol.Position{Line: 0, Character: 5},
		},
		{
			name: "empty document",
			uri:  "file:///e.rms",
			src:  "",
			pos:  protocol.Position{Line: 0, Character: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s.docs.Put(tt.uri, tt.src, 1)

			highlights, err := s.DocumentHighlight(context.Background(), &protocol.DocumentHighlightParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(tt.uri)},
					Position:     tt.pos,
				},
			})

			require.NoError(t, err)
			require.NotNil(t, highlights, "empty result must be an empty slice, not nil")
			require.Empty(t, highlights)
		})
	}
}

// TestServerDocumentHighlight_UniqueWord pins the ReferencesAt
// semantics: the occurrence under the cursor is included, so a word
// with no siblings highlights exactly itself with kind Text.
func TestServerDocumentHighlight_UniqueWord(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	src := "void h() { xsSetWorldGravity(1.0); }"
	s.docs.Put("file:///u.xs", src, 1)

	highlights, err := s.DocumentHighlight(context.Background(), &protocol.DocumentHighlightParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///u.xs"},
			Position:     protocol.Position{Line: 0, Character: uint32(strings.Index(src, "xsSetWorldGravity"))},
		},
	})

	require.NoError(t, err)
	require.Len(t, highlights, 1, "the occurrence itself is highlighted")
	require.Equal(t, protocol.DocumentHighlightKindText, highlights[0].Kind)
	require.Equal(t, uint32(0), highlights[0].Range.Start.Line)
	require.Equal(t, uint32(len("void h() { ")), highlights[0].Range.Start.Character)
}
