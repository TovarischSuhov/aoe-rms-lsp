package xs

import (
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
