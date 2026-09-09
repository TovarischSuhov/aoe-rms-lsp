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

// TestServerDocumentHighlight_XsOccurrences checks the core behavior in a
// pure .xs document: the declaration and every use of the word under the
// cursor come back with exact ranges, all kind Text.
func TestServerDocumentHighlight_XsOccurrences(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	src := "int towerCount = 2;\nvoid bump() {\n\ttowerCount = towerCount + 1;\n}\n"
	s.docs.Put("file:///t.xs", src, 1)

	highlights, err := s.DocumentHighlight(context.Background(), &protocol.DocumentHighlightParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.xs"},
			Position:     protocol.Position{Line: 2, Character: 5},
		},
	})

	require.NoError(t, err)
	require.Len(t, highlights, 3, "declaration plus both uses")

	expected := []protocol.Range{
		{Start: protocol.Position{Line: 0, Character: 4}, End: protocol.Position{Line: 0, Character: 14}},
		{Start: protocol.Position{Line: 2, Character: 1}, End: protocol.Position{Line: 2, Character: 11}},
		{Start: protocol.Position{Line: 2, Character: 14}, End: protocol.Position{Line: 2, Character: 24}},
	}

	for i, h := range highlights {
		require.Equal(t, protocol.DocumentHighlightKindText, h.Kind, "highlight %d", i)
		require.Equal(t, expected[i], h.Range, "highlight %d", i)
	}
}

// TestServerDocumentHighlight_RmsAttributeName checks the plain .rms
// path: same-name attribute tokens across commands highlight together.
func TestServerDocumentHighlight_RmsAttributeName(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	src := "<LAND_GENERATION>\ncreate_land {\n\tland_percent 20\n}\n" +
		"create_land {\n\tland_percent 40\n}\n"
	s.docs.Put("file:///t.rms", src, 1)

	highlights, err := s.DocumentHighlight(context.Background(), &protocol.DocumentHighlightParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.rms"},
			Position:     protocol.Position{Line: 2, Character: 4},
		},
	})

	require.NoError(t, err)
	require.Len(t, highlights, 2)

	expected := []protocol.Range{
		{Start: protocol.Position{Line: 2, Character: 1}, End: protocol.Position{Line: 2, Character: 13}},
		{Start: protocol.Position{Line: 5, Character: 1}, End: protocol.Position{Line: 5, Character: 13}},
	}

	for i, h := range highlights {
		require.Equal(t, protocol.DocumentHighlightKindText, h.Kind, "highlight %d", i)
		require.Equal(t, expected[i], h.Range, "highlight %d", i)
	}
}

// TestServerDocumentHighlight_InlineXsShift checks the inline-XS path: a
// highlight inside an #includeXS block reports document coordinates —
// the declaration on the first block line gains the block's column
// offset, later lines only the line offset.
func TestServerDocumentHighlight_InlineXsShift(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	src := "<PLAYER_SETUP>\n#includeXS\n" +
		"int seed = 0;\nvoid main() {\n\tseed = xsGetMapSeed();\n\tif (seed % 2 == 0) {\n\t\txsSetRiverHeight(3.0);\n\t}\n}\n" +
		"</PLAYER_SETUP>\n"
	s.docs.Put("file:///t.rms", src, 1)

	highlights, err := s.DocumentHighlight(context.Background(), &protocol.DocumentHighlightParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.rms"},
			Position:     protocol.Position{Line: 4, Character: 3},
		},
	})

	require.NoError(t, err)
	require.Len(t, highlights, 3, "declaration, assignment and condition uses")

	expected := []protocol.Range{
		{Start: protocol.Position{Line: 2, Character: 4}, End: protocol.Position{Line: 2, Character: 8}},
		{Start: protocol.Position{Line: 4, Character: 1}, End: protocol.Position{Line: 4, Character: 5}},
		{Start: protocol.Position{Line: 5, Character: 5}, End: protocol.Position{Line: 5, Character: 9}},
	}

	for i, h := range highlights {
		require.Equal(t, protocol.DocumentHighlightKindText, h.Kind, "highlight %d", i)
		require.Equal(t, expected[i], h.Range, "highlight %d is in document coordinates", i)
	}
}
