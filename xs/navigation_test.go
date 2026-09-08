package xs

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aoe2-lsp/common"
)

func TestXsSymbols_APIShape(t *testing.T) {
	file, _ := XsParse("void f() {}", "shape.xs")

	syms := file.Symbols()

	require.IsType(t, []common.Symbol{}, syms)
	require.Len(t, syms, 1)
	require.IsType(t, "", syms[0].Kind)
	require.IsType(t, "", syms[0].Name)
	require.IsType(t, common.Range{}, syms[0].Range)
	require.IsType(t, common.Range{}, syms[0].Selection)
	require.IsType(t, []common.Symbol{}, syms[0].Children)
}

func TestXsReferencesAt_APIShape(t *testing.T) {
	file, _ := XsParse("void g() {}", "shape.xs")

	ranges := file.ReferencesAt(common.Pos{Line: 0, Column: 5})

	require.IsType(t, []common.Range{}, ranges)
}

func TestXsSymbols_FlatOutline(t *testing.T) {
	src := "void f() { g(); }\n" +
		"int x = 1;\n" +
		"rule r { condition x }\n" +
		"include \"a.xs\"\n"

	file, _ := XsParse(src, "flat.xs")

	syms := file.Symbols()

	require.Len(t, syms, 3, "include declarations are skipped")

	want := []struct{ kind, name string }{
		{DeclFunction, "f"},
		{DeclVariable, "x"},
		{DeclRule, "r"},
	}

	for i, w := range want {
		require.Equal(t, w.kind, syms[i].Kind, "node %d", i)
		require.Equal(t, w.name, syms[i].Name, "node %d", i)
		require.Nil(t, syms[i].Children, "flat outline: node %d", i)
		require.True(t, rangeWithin(syms[i].Selection, syms[i].Range),
			"Selection must be inside Range: node %d", i)
	}
}

func TestXsReferencesAt_IncludesDeclarationSorted(t *testing.T) {
	src := "void f() { g(); g(); }\nvoid g() {}"

	call1 := strings.Index(src, "g(")
	call2 := call1 + 1 + strings.Index(src[call1+1:], "g(")

	file, _ := XsParse(src, "refs.xs")

	ranges := file.ReferencesAt(common.Pos{Line: 0, Column: uint32(call2)})

	require.Len(t, ranges, 3, "declaration + two calls")

	for i := 1; i < len(ranges); i++ {
		require.True(t, ranges[i-1].Start.Before(ranges[i].Start),
			"ranges must be sorted by position")
	}

	last := ranges[len(ranges)-1]
	require.Equal(t, uint32(1), last.Start.Line, "the declaration is on line 1")
	require.Equal(t, uint32(strings.Index("void g() {}", "g(")), last.Start.Column,
		"the declaration occurrence is the last by position")
}

func TestXsReferencesAt_NoIdentEmpty(t *testing.T) {
	src := "int x = 1;"

	file, _ := XsParse(src, "empty.xs")

	literal := strings.Index(src, "1")

	require.Empty(t, file.ReferencesAt(common.Pos{Line: 0, Column: uint32(literal)}))
}

func TestXsDefinition_APIShape(t *testing.T) {
	file, _ := XsParse("void f() {}", "shape.xs")

	r, found := file.Definition(common.Pos{Line: 0, Column: 5})

	require.True(t, found)
	require.IsType(t, common.Range{}, r)
}

// TestXsCallAt_APIShape pins the call-context contract surface: the
// CallAt method on XsFile returning the exported CallSite.
func TestXsCallAt_APIShape(t *testing.T) {
	file, _ := XsParse("void f() { g(a, b); }", "shape.xs")

	site, found := file.CallAt(common.Pos{Line: 0, Column: uint32(strings.Index("void f() { g(a, b); }", "a"))})

	require.True(t, found)
	require.IsType(t, CallSite{}, site)
	require.IsType(t, "", site.Callee)
	require.IsType(t, 0, site.ArgIndex)
	require.IsType(t, false, site.OnArg)
	require.Equal(t, "g", site.Callee)
}

// col builds a line-0 position at the given column.
func col(c int) common.Pos {
	return common.Pos{Line: 0, Column: uint32(c)}
}

// posOf builds the position of needle's first occurrence in src
// (single-line fixtures).
func posOf(src string, needle string, offset int) common.Pos {
	return common.Pos{Line: 0, Column: uint32(strings.Index(src, needle) + offset)}
}

// TestCallAt_ClosedCallArgIndex covers baseline comma counting on a
// closed call.
func TestCallAt_ClosedCallArgIndex(t *testing.T) {
	src := "void f() { g(a, b); }"
	file, _ := XsParse(src, "t.xs")

	site, found := file.CallAt(posOf(src, "a", 0))
	require.True(t, found)
	assert.Equal(t, CallSite{Callee: "g", ArgIndex: 0, OnArg: true}, site)

	site, found = file.CallAt(posOf(src, "b", 0))
	require.True(t, found)
	assert.Equal(t, CallSite{Callee: "g", ArgIndex: 1, OnArg: true}, site)
}

// TestCallAt_JustAfterOpenParen covers SC1 — the first character after
// "(" is the feature's key moment: the exact state an editor sends after
// the trigger keystroke, on an unterminated list.
func TestCallAt_JustAfterOpenParen(t *testing.T) {
	src := "void f() { g("
	file, _ := XsParse(src, "t.xs")

	site, found := file.CallAt(posOf(src, "g(", 2))

	require.True(t, found)
	assert.Equal(t, CallSite{Callee: "g", ArgIndex: 0, OnArg: true}, site)
}

// TestCallAt_OnCalleeName covers the cursor on the callee token:
// signature renders with no active argument.
func TestCallAt_OnCalleeName(t *testing.T) {
	src := "void f() { g(a); }"
	file, _ := XsParse(src, "t.xs")

	site, found := file.CallAt(posOf(src, "g(", 0))

	require.True(t, found)
	assert.Equal(t, CallSite{Callee: "g", ArgIndex: 0, OnArg: false}, site)
}

// TestCallAt_NestedInnerWins covers innermost-call selection:
// f(g(x| → g, аргумент 0.
func TestCallAt_NestedInnerWins(t *testing.T) {
	src := "void f() { h(g(x)); }"
	file, _ := XsParse(src, "t.xs")

	site, found := file.CallAt(posOf(src, "x", 0))

	require.True(t, found)
	assert.Equal(t, CallSite{Callee: "g", ArgIndex: 0, OnArg: true}, site)
}

// TestCallAt_UnclosedToEOF covers doubly-nested in-progress calls at the
// exact end of input — the eofPos recovery frontier contains the cursor.
func TestCallAt_UnclosedToEOF(t *testing.T) {
	src := "void f() { g(h("
	file, _ := XsParse(src, "t.xs")

	require.Equal(t, len(src), 15)

	site, found := file.CallAt(col(len(src)))

	require.True(t, found)
	assert.Equal(t, CallSite{Callee: "h", ArgIndex: 0, OnArg: true}, site)
}

// TestCallAt_PositionOnOpenParenChar covers the exact boundary between
// the callee-side and argument-side steps: the "(" character itself is
// callee-side; one column later flips to onArg=true.
func TestCallAt_PositionOnOpenParenChar(t *testing.T) {
	src := "void f() { g(a); }"
	file, _ := XsParse(src, "t.xs")

	site, found := file.CallAt(posOf(src, "g(", 1))

	require.True(t, found)
	assert.Equal(t, CallSite{Callee: "g", ArgIndex: 0, OnArg: false}, site)
}

// TestCallAt_PositionOnCommaChar covers comma-character ownership: the
// previous-argument region (the comma's end is one column further).
func TestCallAt_PositionOnCommaChar(t *testing.T) {
	src := "void f() { g(a, b); }"
	file, _ := XsParse(src, "t.xs")

	site, found := file.CallAt(posOf(src, ",", 0))

	require.True(t, found)
	assert.Equal(t, CallSite{Callee: "g", ArgIndex: 0, OnArg: true}, site)
}

// TestCallAt_ArgIndexNeverClamped covers the never-clamp constraint:
// navigation reports the true ordinal even beyond any declared
// parameter count.
func TestCallAt_ArgIndexNeverClamped(t *testing.T) {
	src := "void f() { g(a, b, c "
	file, _ := XsParse(src, "t.xs")

	site, found := file.CallAt(posOf(src, "c", 0))

	require.True(t, found)
	assert.Equal(t, 2, site.ArgIndex)
	assert.True(t, site.OnArg)
}

// TestCallAt_Deterministic covers the determinism requirement: pure
// index lookup, same question — same answer.
func TestCallAt_Deterministic(t *testing.T) {
	src := "void f() { g(a, b); }"
	file, _ := XsParse(src, "t.xs")

	pos := posOf(src, "b", 0)

	first, found1 := file.CallAt(pos)
	second, found2 := file.CallAt(pos)

	require.True(t, found1)
	require.True(t, found2)
	assert.Equal(t, first, second)
}

// TestCallAt_InString covers string exclusion: positions inside a string
// token never resolve to a call, even inside a call's argument span.
func TestCallAt_InString(t *testing.T) {
	src := `void f() { g("abc"); }`
	file, _ := XsParse(src, "t.xs")

	_, found := file.CallAt(posOf(src, "c", 0))

	assert.False(t, found)
}

// TestCallAt_InComment covers line-comment exclusion.
func TestCallAt_InComment(t *testing.T) {
	src := "// g(a, b)\nvoid f() { g(a, b); }"
	file, _ := XsParse(src, "t.xs")

	_, found := file.CallAt(common.Pos{Line: 0, Column: 4})

	assert.False(t, found)
}

// TestCallAt_InBlockComment covers multi-line block-comment spans — the
// only comment form whose span crosses lines (from "/*" past "*/").
func TestCallAt_InBlockComment(t *testing.T) {
	src := "void f() { /* open\nstill comment */ g(a); }"
	file, _ := XsParse(src, "t.xs")

	_, found := file.CallAt(common.Pos{Line: 1, Column: uint32(strings.Index("still comment */ g(a); }", "still"))})

	assert.False(t, found)
}

// TestCallAt_VectorLiteralNotCall covers the Kind=call rule: vector
// literals are not call contexts.
func TestCallAt_VectorLiteralNotCall(t *testing.T) {
	src := "void f() { vector v = (1, 2, 3); }"
	file, _ := XsParse(src, "t.xs")

	_, found := file.CallAt(posOf(src, "3", 0))

	assert.False(t, found)
}

// TestCallAt_ParamListNotCall covers the Kind=call rule: declaration
// parameter lists are not call contexts.
func TestCallAt_ParamListNotCall(t *testing.T) {
	src := "int f(int a, int b) { return 0; }"
	file, _ := XsParse(src, "t.xs")

	_, found := file.CallAt(posOf(src, "b)", 0))

	assert.False(t, found)
}

func TestXsDefinition_ParamShadowsTopLevel(t *testing.T) {
	line := "void f(float x) { x = 2; }"

	file, _ := XsParse("int x = 1;\n"+line, "shadow.xs")

	r, found := file.Definition(common.Pos{Line: 1, Column: uint32(strings.Index(line, "x = 2"))})

	require.True(t, found)
	require.Equal(t, uint32(1), r.Start.Line)
	require.Equal(t, uint32(strings.Index(line, "x)")), r.Start.Column,
		"the parameter token wins over the top-level variable")
}

func TestXsDefinition_OnDeclarationReturnsItself(t *testing.T) {
	src := "void f() {}"

	file, _ := XsParse(src, "self.xs")

	r, found := file.Definition(common.Pos{Line: 0, Column: uint32(strings.Index(src, "f("))})

	require.True(t, found)
	require.Equal(t, uint32(strings.Index(src, "f(")), r.Start.Column)
	require.Equal(t, uint32(len("f")), r.End.Column-r.Start.Column)
}

func TestXsDefinition_TopLevelVariable(t *testing.T) {
	line := "void f() { x = 2; }"

	file, _ := XsParse("int x = 1;\n"+line, "top.xs")

	r, found := file.Definition(common.Pos{Line: 1, Column: uint32(strings.Index(line, "x = 2"))})

	require.True(t, found)
	require.Equal(t, uint32(0), r.Start.Line)
	require.Equal(t, uint32(strings.Index("int x = 1;", "x")), r.Start.Column)
}

func TestXsDefinition_BuiltinNotFound(t *testing.T) {
	line := "void f() { xsSetWorldGravity(1.0); }"

	file, _ := XsParse(line, "builtin.xs")

	_, found := file.Definition(common.Pos{Line: 0, Column: uint32(strings.Index(line, "xsSetWorldGravity("))})

	require.False(t, found, "builtins have no local declaration")
}

func TestXsDefinition_LocalShadowsOuterLocal(t *testing.T) {
	src := "void f() {\n" +
		"\tint x = 1;\n" +
		"\tif (1) {\n" +
		"\t\tint x = 2;\n" +
		"\t\tx = 3;\n" +
		"\t}\n" +
		"}\n"

	lines := strings.Split(src, "\n")
	file, _ := XsParse(src, "locals.xs")

	r, found := file.Definition(common.Pos{
		Line:   4,
		Column: uint32(strings.Index(lines[4], "x")),
	})

	require.True(t, found)
	require.Equal(t, uint32(3), r.Start.Line, "the inner block local wins")
	require.Equal(t, uint32(strings.Index(lines[3], "x")), r.Start.Column)
}

func TestXsSymbols_PreludeInvariantSweep(t *testing.T) {
	raw, err := os.ReadFile("../docs/ref/ugc-guide/xs/prelude.xs")
	require.NoError(t, err)

	file, parseDiags := XsParse(string(raw), "prelude.xs")
	require.Empty(t, parseDiags, "prelude.xs must parse without false errors")

	syms := file.Symbols()

	externs := 0

	for _, sym := range syms {
		if sym.Kind == DeclExtern {
			externs++
		}

		require.True(t, rangeWithin(sym.Selection, sym.Range),
			"Selection must stay inside Range: %s", sym.Name)
		require.Nil(t, sym.Children, "flat outline")
	}

	require.Equal(t, 884, externs, "the prelude declares 884 externs")
	require.Len(t, syms, len(file.Decls), "extern and function declarations alike")
}

// TestReferences_ByName checks the by-name form: every occurrence of the
// name including the declaration, without needing a position in this file.
func TestReferences_ByName(t *testing.T) {
	src := "void f() {}\nvoid g() { f(); }\n"
	file, diags := XsParse(src, "t.xs")
	require.Empty(t, diags)

	ranges := file.References("f")
	require.Len(t, ranges, 2) // declaration + call
	assert.Equal(t, uint32(0), ranges[0].Start.Line)
	assert.Equal(t, uint32(1), ranges[1].Start.Line)

	// Equivalence with ReferencesAt at each occurrence.
	for _, r := range ranges {
		assert.Equal(t, ranges, file.ReferencesAt(r.Start))
	}

	assert.Empty(t, file.References("missing"))
}

func TestVisibleAt_APIShape(t *testing.T) {
	file, _ := XsParse("void f() {}", "shape.xs")

	syms, found := file.VisibleAt(common.Pos{Line: 0, Column: 6})

	require.True(t, found, "code position must resolve")
	require.IsType(t, []common.Symbol{}, syms)
	require.IsType(t, "", syms[0].Kind)
	require.IsType(t, "", syms[0].Name)
	require.IsType(t, common.Range{}, syms[0].Range)
	require.IsType(t, common.Range{}, syms[0].Selection)
}

func TestVisibleAt_TopLevelDecls(t *testing.T) {
	src := "int g = 1;\n" +
		"void f(float a) { a = 2; }\n" +
		"extern int e();\n" +
		"include \"x.xs\"\n"

	file, _ := XsParse(src, "top.xs")

	syms, found := file.VisibleAt(common.Pos{Line: 1, Column: 18})

	require.True(t, found)

	kinds := make(map[string]string)

	for _, s := range syms {
		kinds[s.Name] = s.Kind
		require.True(t, rangeWithin(s.Selection, s.Range),
			"Selection must be inside Range: %s", s.Name)
		require.NotEqual(t, "x.xs", s.Name, "include is not name-bearing")
	}

	require.Equal(t, DeclVariable, kinds["g"])
	require.Equal(t, DeclFunction, kinds["f"])
	require.Equal(t, DeclExtern, kinds["e"])
	require.Equal(t, KindParam, kinds["a"])
}

func TestVisibleAt_ParamAndLocalScopes(t *testing.T) {
	src := "void f(int p) {\n" +
		"  int a1 = 1;\n" +
		"  {\n" +
		"    int b1 = 2;\n" +
		"  }\n" +
		"  int a2;\n" +
		"}\n"

	file, _ := XsParse(src, "scopes.xs")

	syms, found := file.VisibleAt(common.Pos{Line: 5, Column: 2})

	require.True(t, found)

	names := make([]string, 0, len(syms))

	for _, s := range syms {
		names = append(names, s.Name)
	}

	// Top-level first, then parameters, then locals declared at or
	// before pos; b1's block closed before pos, a2 is declared after.
	require.Equal(t, []string{"f", "p", "a1"}, names)
}

func TestVisibleAt_InStringAndComment(t *testing.T) {
	src := "void f() {\n" +
		"  string s = \"abcd\";\n" +
		"}\n" +
		"// tail\n"

	file, _ := XsParse(src, "noncode.xs")

	for _, pos := range []common.Pos{
		{Line: 1, Column: 15},
		{Line: 3, Column: 3},
	} {
		_, found := file.VisibleAt(pos)

		require.False(t, found, "position %v must not resolve", pos)
	}
}

func TestVisibleAt_EmptyFile_TrueEmpty(t *testing.T) {
	file, _ := XsParse("", "empty.xs")

	syms, found := file.VisibleAt(common.Pos{Line: 0, Column: 0})

	require.True(t, found, "empty file is still code")
	require.Empty(t, syms)
}

func TestVisibleAt_ShadowingBothKept(t *testing.T) {
	src := "int x = 1;\n" +
		"void f() {\n" +
		"  float x = 2;\n" +
		"  x = 3;\n" +
		"}\n"

	file, _ := XsParse(src, "shadow.xs")

	syms, found := file.VisibleAt(common.Pos{Line: 3, Column: 2})

	require.True(t, found)

	kinds := make(map[string]int)

	for _, s := range syms {
		kinds[s.Kind]++
	}

	require.Equal(t, 1, kinds[DeclVariable], "top-level x kept")
	require.Equal(t, 1, kinds[KindLocal], "local x kept")
}
