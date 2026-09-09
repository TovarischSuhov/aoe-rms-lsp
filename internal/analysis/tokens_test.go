package analysis

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/kb"
	"aoe2-lsp/internal/rms"
	"aoe2-lsp/internal/xs"
	"testing"

	"github.com/stretchr/testify/require"
)

// tokenShape strips the parser-internal Offset so table expectations
// stay (line, column, endLine, endColumn, type).
type tokenShape [5]any

func shapes(toks []common.Token) []tokenShape {
	out := make([]tokenShape, 0, len(toks))

	for _, tok := range toks {
		out = append(out, tokenShape{
			tok.Range.Start.Line, tok.Range.Start.Column,
			tok.Range.End.Line, tok.Range.End.Column,
			tok.Type,
		})
	}

	return out
}

func newTestAnalyzer(t *testing.T) *Analyzer {
	t.Helper()

	store, err := kb.NewStore()
	require.NoError(t, err)

	return NewAnalyzer(store)
}

// TestAnalyzerTokens_Rms pins the RMS classification: section name span,
// known/unknown command names, effect_percent deprecation, attribute
// names and ident argument values — sorted, no suggestion machinery.
func TestAnalyzerTokens_Rms(t *testing.T) {
	t.Parallel()

	a := newTestAnalyzer(t)

	src := "<LAND_GENERATION>\n" +
		"create_land {\n" +
		"\tland_percent MY_CONST\n" +
		"}\n" +
		"creat_object 5\n" +
		"effect_percent 50\n"
	file, _ := rms.Parse(src, "t.rms")

	toks := a.TokensRms(file)

	expected := []tokenShape{
		{uint32(0), uint32(1), uint32(0), uint32(16), TokenSection},    // <LAND_GENERATION>
		{uint32(1), uint32(0), uint32(1), uint32(11), TokenKnown},      // create_land
		{uint32(2), uint32(1), uint32(2), uint32(13), TokenKnown},      // land_percent
		{uint32(2), uint32(14), uint32(2), uint32(22), TokenUnknown},   // MY_CONST
		{uint32(4), uint32(0), uint32(4), uint32(12), TokenUnknown},    // creat_object
		{uint32(5), uint32(0), uint32(5), uint32(14), TokenDeprecated}, // effect_percent
	}

	require.Equal(t, expected, shapes(toks))
}

// TestAnalyzerTokens_RmsDirectivesSkipped pins that #-directives carry
// no tokens and that bare attribute lines classify under the last
// known command.
func TestAnalyzerTokens_RmsDirectivesSkipped(t *testing.T) {
	t.Parallel()

	a := newTestAnalyzer(t)

	src := "#const CLIFFS 3\nbase_terrain GRASS\n"
	file, _ := rms.Parse(src, "t.rms")

	toks := a.TokensRms(file)

	expected := []tokenShape{
		{uint32(1), uint32(0), uint32(1), uint32(12), TokenKnown},    // base_terrain
		{uint32(1), uint32(13), uint32(1), uint32(18), TokenUnknown}, // GRASS (not an xs constant)
	}

	require.Equal(t, expected, shapes(toks))
}

// TestAnalyzerTokens_Xs pins the XS classification: declaration names
// as kind, idents known (params, locals, externals, kb) or unknown,
// callee names of calls.
func TestAnalyzerTokens_Xs(t *testing.T) {
	t.Parallel()

	a := newTestAnalyzer(t)

	src := "void myFn(int n) {\n" +
		"\tint q = undefinedThing;\n" +
		"\tunknownFn(n);\n" +
		"}\n"
	file, _ := xs.XsParse(src, "t.xs")

	externals := []xs.Decl{{Kind: xs.DeclFunction, Name: "extFn"}}

	toks := a.TokensXs(file, externals)

	expected := []tokenShape{
		{uint32(0), uint32(5), uint32(0), uint32(9), TokenKind},     // myFn
		{uint32(1), uint32(5), uint32(1), uint32(6), TokenKnown},    // q
		{uint32(1), uint32(9), uint32(1), uint32(23), TokenUnknown}, // undefinedThing
		{uint32(2), uint32(1), uint32(2), uint32(10), TokenUnknown}, // unknownFn
		{uint32(2), uint32(11), uint32(2), uint32(12), TokenKnown},  // n
	}

	require.Equal(t, expected, shapes(toks))
}

// TestAnalyzerTokens_XsExternalsKnown pins that externals suppress
// unknown on calls.
func TestAnalyzerTokens_XsExternalsKnown(t *testing.T) {
	t.Parallel()

	a := newTestAnalyzer(t)

	src := "void caller() {\n\textFn();\n}\n"
	file, _ := xs.XsParse(src, "t.xs")

	externals := []xs.Decl{{Kind: xs.DeclFunction, Name: "extFn"}}

	toks := a.TokensXs(file, externals)

	expected := []tokenShape{
		{uint32(0), uint32(5), uint32(0), uint32(11), TokenKind}, // caller
		{uint32(1), uint32(1), uint32(1), uint32(6), TokenKnown}, // extFn
	}

	require.Equal(t, expected, shapes(toks))
}

// TestAnalyzerTokens_Empty pins the empty-input shape: empty sections
// and no declarations answer nil-free empty results.
func TestAnalyzerTokens_Empty(t *testing.T) {
	t.Parallel()

	a := newTestAnalyzer(t)

	rmsFile, _ := rms.Parse("", "t.rms")
	require.NotNil(t, a.TokensRms(rmsFile))

	xsFile, _ := xs.XsParse("", "t.xs")
	require.NotNil(t, a.TokensXs(xsFile, nil))
}
