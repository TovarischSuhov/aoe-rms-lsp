package xs

import (
	"os"
	"strings"
	"testing"

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
