package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestInitialize_AdvertisesSemanticTokens pins the capability surface:
// the fixed server-side legend, empty modifiers, full-only support.
func TestInitialize_AdvertisesSemanticTokens(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	res, err := s.Initialize(context.Background(), &protocol.InitializeParams{})
	require.NoError(t, err)

	provider, ok := res.Capabilities.SemanticTokensProvider.(*protocol.SemanticTokensOptions)
	require.True(t, ok, "semantic tokens must be advertised with the legend")

	require.Equal(t, []string{"known", "unknown", "deprecated", "section", "kind"},
		provider.Legend.TokenTypes)
	require.Empty(t, provider.Legend.TokenModifiers)
}

// TestServerSemanticTokens_APIShape pins the empty-result contract: an
// unknown extension answers empty Data — not nil, not an error.
func TestServerSemanticTokens_APIShape(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///notes.txt", "create_land {\n}\n", 1)

	res, err := s.SemanticTokensFull(context.Background(), &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI("file:///notes.txt")},
	})

	require.NoError(t, err, "an empty result is not an error")
	require.NotNil(t, res.Data)
	require.Empty(t, res.Data)
}

// TestServerSemanticTokens_GoldenRms is the slot's golden test: the
// exact delta-encoded quintuples over an .rms fixture with a section,
// known/unknown names, a deprecated form and an ident value.
func TestServerSemanticTokens_GoldenRms(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///t.rms",
		"<LAND_GENERATION>\ncreate_land {\n\tland_percent MY_CONST\n}\ncreat_object 5\neffect_percent 50\n", 1)

	res, err := s.SemanticTokensFull(context.Background(), &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI("file:///t.rms")},
	})
	require.NoError(t, err)

	expected := []uint32{
		0, 1, 15, 3, 0, // land_generation: section
		1, 0, 11, 0, 0, // create_land: known
		1, 1, 12, 0, 0, // land_percent: known
		0, 13, 8, 1, 0, // MY_CONST: unknown
		2, 0, 12, 1, 0, // creat_object: unknown
		1, 0, 14, 2, 0, // effect_percent: deprecated
	}

	require.Equal(t, expected, res.Data)
}

// TestServerSemanticTokens_GoldenXs pins the .xs golden: declaration
// kind, known params/locals, unknown idents and callees.
func TestServerSemanticTokens_GoldenXs(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///t.xs",
		"void myFn(int n) {\n\tint q = undefinedThing;\n\tunknownFn(n);\n}\n", 1)

	res, err := s.SemanticTokensFull(context.Background(), &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI("file:///t.xs")},
	})
	require.NoError(t, err)

	expected := []uint32{
		0, 5, 4, 4, 0, // myFn: kind
		1, 5, 1, 0, 0, // q: known
		0, 4, 14, 1, 0, // undefinedThing: unknown
		1, 1, 9, 1, 0, // unknownFn: unknown callee
		0, 10, 1, 0, 0, // n: known
	}

	require.Equal(t, expected, res.Data)
}

// TestServerSemanticTokens_InlineXs pins the coordinate shift: tokens
// of an inline XS block arrive in the outer .rms file's coordinates.
func TestServerSemanticTokens_InlineXs(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///t.rms", "#includeXS\nvoid h() {}\n", 1)

	res, err := s.SemanticTokensFull(context.Background(), &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI("file:///t.rms")},
	})
	require.NoError(t, err)

	expected := []uint32{
		1, 5, 1, 4, 0, // h on line 1 (block base): kind
	}

	require.Equal(t, expected, res.Data)
}

// TestServerSemanticTokens_IntegrationStdio runs the token pipeline
// over the real stdio entrypoint: the advertised legend agrees with the
// emitted token-type indices, and an .rms document with an unknown
// command plus an inline XS block classifies end to end.
func TestServerSemanticTokens_IntegrationStdio(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	init, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	provider, ok := init.Capabilities.SemanticTokensProvider.(*protocol.SemanticTokensOptions)
	require.True(t, ok)
	legend := provider.Legend.TokenTypes

	docURI := uri.File(t.TempDir() + "/map.rms")

	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: docURI, LanguageID: "aoe2rms", Version: 1,
			Text: "creat_object 5\n#includeXS\nvoid h() {}\n",
		},
	}))
	h.waitDiagnostics(docURI)

	res, err := h.disp.SemanticTokensFull(ctx, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Data)

	// every fifth element is a token-type index — all must address the
	// advertised legend
	types := map[uint32]bool{}

	for i := 3; i < len(res.Data); i += 5 {
		require.Less(t, res.Data[i], uint32(len(legend)), "token type within legend")
		types[res.Data[i]] = true
	}

	// the unknown command and the inline declaration are both present
	unknownIdx := uint32(0)

	for i, name := range legend {
		if name == "unknown" {
			unknownIdx = uint32(i)
		}
	}

	require.True(t, types[unknownIdx], "creat_object classified unknown")
	require.True(t, types[uint32(len(legend))-1], "h classified kind")
}
