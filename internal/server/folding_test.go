package server

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestInitialize_AdvertisesFoldingRange pins the capability surface: the
// folding provider must be advertised like its document-feature siblings.
func TestInitialize_AdvertisesFoldingRange(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	res, err := s.Initialize(context.Background(), &protocol.InitializeParams{})
	require.NoError(t, err)

	require.Equal(t, protocol.Boolean(true), res.Capabilities.FoldingRangeProvider,
		"foldingRange must be advertised")
}

// TestServerFoldingRanges_APIShape pins the empty-result contract: a
// closed document answers with an empty slice, never nil, never an error.
func TestServerFoldingRanges_APIShape(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	ranges, err := s.FoldingRanges(context.Background(), &protocol.FoldingRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///closed.xs"},
	})

	require.NoError(t, err, "an empty result is not an error")
	require.NotNil(t, ranges)
	require.Empty(t, ranges)
}

// TestServerFoldingRanges_RmsTree pins the .rms regions: the section
// (closing tag included), a multi-line command and an inline-XS block
// fold; a single-line command does not. Traversal order is document
// order.
func TestServerFoldingRanges_RmsTree(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	src := "<LAND_GENERATION>\ncreate_player_lands {\n\tland_percent 32\n}\n" +
		"#includeXS\nvoid h() {\n\tint q = 1;\n}\n</LAND_GENERATION>\ncreate_elevator\n"
	s.docs.Put("file:///t.rms", src, 1)

	ranges, err := s.FoldingRanges(context.Background(), &protocol.FoldingRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.rms"},
	})

	require.NoError(t, err)

	expected := []protocol.FoldingRange{
		{StartLine: 0, EndLine: 8}, // section incl. closing tag
		{StartLine: 1, EndLine: 3}, // create_player_lands block
		{StartLine: 5, EndLine: 8}, // inline XS block
	}

	require.Equal(t, expected, ranges)
}

// TestServerFoldingRanges_XsTopDecls pins the .xs regions: function and
// rule bodies fold up to their last content line (the closing brace
// stays visible); a single-line variable does not fold.
func TestServerFoldingRanges_XsTopDecls(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	src := "int x = 1;\nvoid f() {\n\tint y = 2;\n}\nrule r {\n\tcondition x\n}\n"
	s.docs.Put("file:///t.xs", src, 1)

	ranges, err := s.FoldingRanges(context.Background(), &protocol.FoldingRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.xs"},
	})

	require.NoError(t, err)

	expected := []protocol.FoldingRange{
		{StartLine: 1, EndLine: 2}, // void f() body
		{StartLine: 4, EndLine: 5}, // rule r body
	}

	require.Equal(t, expected, ranges)
}

// TestServerFoldingRanges_Silence pins the silence shape: unknown
// extensions and empty documents answer with an empty slice.
func TestServerFoldingRanges_Silence(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	tests := []struct {
		name string
		uri  string
		src  string
	}{
		{name: "unknown extension", uri: "file:///notes.txt", src: "create_land {\n\tland_percent 5\n}\n"},
		{name: "empty document", uri: "file:///e.rms", src: ""},
		{name: "flat document", uri: "file:///flat.rms", src: strings.Repeat("base_terrain GRASS\n", 3)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s.docs.Put(tt.uri, tt.src, 1)

			ranges, err := s.FoldingRanges(context.Background(), &protocol.FoldingRangeParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(tt.uri)},
			})

			require.NoError(t, err)
			require.NotNil(t, ranges, "empty result must be an empty slice, not nil")
			require.Empty(t, ranges)
		})
	}
}
