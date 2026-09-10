package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lsp.dev/protocol"
)

// TestInitialize_AdvertisesSelectionRange pins the capability surface.
func TestInitialize_AdvertisesSelectionRange(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	res, err := s.Initialize(context.Background(), &protocol.InitializeParams{})
	require.NoError(t, err)

	require.Equal(t, protocol.Boolean(true), res.Capabilities.SelectionRangeProvider,
		"selectionRange must be advertised")
}

// TestServerSelectionRange_InlineXsChain pins the .rms chain through an
// inline-XS block: the XS levels in file coordinates come first, then
// the block, then the RMS section — one nesting chain, innermost first.
func TestServerSelectionRange_InlineXsChain(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	src := "<LAND_GENERATION>\n" +
		"#includeXS\n" +
		"void f() {\n" +
		"int a = 1 + b;\n" +
		"}\n" +
		"</LAND_GENERATION>\n"
	s.docs.Put("file:///t.rms", src, 1)

	out, err := s.SelectionRange(context.Background(), &protocol.SelectionRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.rms"},
		Positions: []protocol.Position{
			{Line: 3, Character: 12}, // on b
		},
	})

	require.NoError(t, err)
	require.Len(t, out, 1)

	got := []protocol.Range{}
	for node := &out[0]; node != nil; node = node.Parent {
		got = append(got, node.Range)
	}

	assert.Equal(t, []protocol.Range{
		{Start: protocol.Position{Line: 3, Character: 12}, End: protocol.Position{Line: 3, Character: 13}}, // b
		{Start: protocol.Position{Line: 3, Character: 8}, End: protocol.Position{Line: 3, Character: 13}},  // 1 + b
		{Start: protocol.Position{Line: 3, Character: 4}, End: protocol.Position{Line: 3, Character: 13}},  // a = 1 + b
		{Start: protocol.Position{Line: 3, Character: 0}, End: protocol.Position{Line: 3, Character: 14}},  // int decl
		{Start: protocol.Position{Line: 2, Character: 0}, End: protocol.Position{Line: 3, Character: 14}},  // void f
		{Start: protocol.Position{Line: 2, Character: 0}, End: protocol.Position{Line: 5, Character: 0}},   // XS block
		{Start: protocol.Position{Line: 0, Character: 0}, End: protocol.Position{Line: 5, Character: 0}},   // section
	}, got)
}

// TestServerSelectionRange_FallbackAndAlignment pins the empty-chain
// fallback (a whole line per position) and the one-entry-per-position
// alignment: a gap in .xs and an unknown extension both answer the
// line range alone.
func TestServerSelectionRange_FallbackAndAlignment(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///t.xs", "void f() { g(); }\n\nvoid h() { }\n", 1)
	s.docs.Put("file:///t.txt", "hello\n", 1)

	out, err := s.SelectionRange(context.Background(), &protocol.SelectionRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.xs"},
		Positions: []protocol.Position{
			{Line: 1, Character: 0},  // the empty line between declarations
			{Line: 0, Character: 11}, // on g
		},
	})

	require.NoError(t, err)
	require.Len(t, out, 2, "one entry per position, in order")

	assert.Equal(t, protocol.Range{
		Start: protocol.Position{Line: 1, Character: 0},
		End:   protocol.Position{Line: 1, Character: 0},
	}, out[0].Range, "a gap position falls back to its whole line")
	assert.Nil(t, out[0].Parent, "the line fallback has no parent")

	assert.Equal(t, protocol.Range{
		Start: protocol.Position{Line: 0, Character: 11},
		End:   protocol.Position{Line: 0, Character: 12},
	}, out[1].Range, "a code position starts at the innermost name")
	require.NotNil(t, out[1].Parent, "the chain nests outward")

	txt, err := s.SelectionRange(context.Background(), &protocol.SelectionRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.txt"},
		Positions:    []protocol.Position{{Line: 0, Character: 2}},
	})

	require.NoError(t, err)
	require.Len(t, txt, 1)
	assert.Equal(t, protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End:   protocol.Position{Line: 0, Character: 5},
	}, txt[0].Range, "unknown extensions answer the line too")
}
