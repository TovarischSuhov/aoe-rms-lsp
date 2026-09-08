package hints

import (
	"aoe2-lsp/common"
	"aoe2-lsp/kb"
	"aoe2-lsp/rms"
	"aoe2-lsp/xs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHint_APIShape pins the contract surface of the render result: the
// hints package with the exported Hint entity.
func TestHint_APIShape(t *testing.T) {
	t.Parallel()
	hint := Hint{Label: "l", Params: []string{"p"}, Active: 0}

	require.IsType(t, "", hint.Label)
	require.IsType(t, []string{}, hint.Params)
	require.IsType(t, 0, hint.Active)
}

// TestHint_ConstructAndUse covers the data entity: construction is the
// behavior (the contract's canonical RMS example round-trips).
func TestHint_ConstructAndUse(t *testing.T) {
	t.Parallel()
	hint := Hint{
		Label:  "percent_chance(%: percent 0..99)",
		Params: []string{"%: percent 0..99"},
		Active: 0,
	}

	assert.Equal(t, "percent_chance(%: percent 0..99)", hint.Label)
	assert.Equal(t, []string{"%: percent 0..99"}, hint.Params)
	assert.Equal(t, 0, hint.Active)
}

// TestComputer_APIShape pins the computer contract surface: the
// NewComputer constructor and the two query methods.
func TestComputer_APIShape(t *testing.T) {
	t.Parallel()
	store, err := kb.NewStore()
	require.NoError(t, err)

	computer := NewComputer(store)

	require.IsType(t, &Computer{}, computer)

	file, _ := xs.XsParse("void f() { g(1); }", "t.xs")

	hint, found := computer.XsAt(file, common.Pos{Line: 0, Column: 15}, nil)
	require.IsType(t, Hint{}, hint)
	require.False(t, found) // g is in no pool and no kb

	rmsFile, _ := rms.Parse("create_elevation 3", "t.rms")

	hint, found = computer.RmsAt(rmsFile, common.Pos{Line: 0, Column: 18})
	require.IsType(t, Hint{}, hint)
	require.True(t, found)
}

// xsPos builds the position of needle's first occurrence on line 0
// (offset shifts within the needle).
func xsPos(src string, needle string, offset int) common.Pos {
	return common.Pos{Line: 0, Column: uint32(strings.Index(src, needle) + offset)}
}

// newTestComputer builds a computer over the real embedded store.
func newTestComputer(t *testing.T) *Computer {
	t.Helper()

	store, err := kb.NewStore()
	require.NoError(t, err)

	return NewComputer(store)
}

// TestXsAt_KbFunction covers the contract's canonical XS example
// verbatim: kb fallback path and active-parameter mapping.
func TestXsAt_KbFunction(t *testing.T) {
	t.Parallel()
	src := "void r() { xsVectorSet(1.0, 2.0, 3.0); }"
	file, _ := xs.XsParse(src, "t.xs")

	hint, found := newTestComputer(t).XsAt(file, xsPos(src, "2.0", 0), nil)

	require.True(t, found)
	assert.Equal(t, "vector xsVectorSet(float x, float y, float z)", hint.Label)
	assert.Equal(t, []string{"float x", "float y", "float z"}, hint.Params)
	assert.Equal(t, 1, hint.Active)
}

// TestXsAt_SourceDeclWinsOverKb covers the truth model: source
// declarations beat kb; the unclosed call also proves in-progress calls
// feed the computer (SC1 end-to-end at hints level).
func TestXsAt_SourceDeclWinsOverKb(t *testing.T) {
	t.Parallel()
	src := "int xsVectorSet(int q) { return 0; }\n" +
		"void r() { xsVectorSet(1 "
	file, _ := xs.XsParse(src, "t.xs")

	pos := common.Pos{Line: 1, Column: uint32(strings.Index("void r() { xsVectorSet(1 ", "1 "))}

	hint, found := newTestComputer(t).XsAt(file, pos, nil)

	require.True(t, found)
	assert.Equal(t, "int xsVectorSet(int q)", hint.Label)
	assert.Equal(t, 0, hint.Active)
}

// TestXsAt_ClosureDeclPool covers the closure pool: external
// declarations render with no return type when undeclared.
func TestXsAt_ClosureDeclPool(t *testing.T) {
	t.Parallel()
	src := "void r() { helper(1, "
	file, _ := xs.XsParse(src, "t.xs")

	external := []xs.Decl{{
		Kind:   xs.DeclFunction,
		Name:   "helper",
		Params: []xs.Param{{Name: "a", Type: "int"}, {Name: "b", Type: "int"}},
	}}

	hint, found := newTestComputer(t).XsAt(file, xsPos(src, "1, ", 3), external)

	require.True(t, found)
	assert.Equal(t, "helper(int a, int b)", hint.Label)
	assert.Equal(t, 1, hint.Active)
}

// TestXsAt_OptionalKbParamBracketed covers the bracket form for kb
// optional parameters (Required=false) via the real xsCreateFile entry.
func TestXsAt_OptionalKbParamBracketed(t *testing.T) {
	t.Parallel()
	src := "void r() { xsCreateFile("
	file, _ := xs.XsParse(src, "t.xs")

	hint, found := newTestComputer(t).XsAt(file, xsPos(src, "xsCreateFile(", 14), nil)

	require.True(t, found)
	assert.Equal(t, "bool xsCreateFile([bool append])", hint.Label)
	assert.Equal(t, []string{"[bool append]"}, hint.Params)
	assert.Equal(t, 0, hint.Active)
}

// TestXsAt_UnknownFunction covers silence for unknown names (trust
// rule).
func TestXsAt_UnknownFunction(t *testing.T) {
	t.Parallel()
	src := "void r() { nosuchfn(1 "
	file, _ := xs.XsParse(src, "t.xs")

	_, found := newTestComputer(t).XsAt(file, xsPos(src, "1", 0), nil)

	assert.False(t, found)
}

// TestXsAt_ConflictingSourceDecls covers the conflict rule: a
// wrong-signature hint is worse than none.
func TestXsAt_ConflictingSourceDecls(t *testing.T) {
	t.Parallel()
	src := "void h(int a) {}\n" + "void r() { h("
	file, _ := xs.XsParse(src, "t.xs")

	external := []xs.Decl{{
		Kind:   xs.DeclFunction,
		Name:   "h",
		Params: []xs.Param{{Name: "b", Type: "float"}},
	}}

	pos := common.Pos{Line: 1, Column: uint32(strings.Index("void r() { h(", "h(") + 2)}

	_, found := newTestComputer(t).XsAt(file, pos, external)

	assert.False(t, found)
}

// TestXsAt_NoCallContext covers the call-context requirement:
// identifiers route to hover/completion instead.
func TestXsAt_NoCallContext(t *testing.T) {
	t.Parallel()
	src := "int q = 1;\n" + "void r() { int w = q; }"
	file, _ := xs.XsParse(src, "t.xs")

	pos := common.Pos{Line: 1, Column: uint32(strings.Index("void r() { int w = q; }", "q;"))}

	_, found := newTestComputer(t).XsAt(file, pos, nil)

	assert.False(t, found)
}

// TestXsAt_EqualParamsMerged covers the «единая декларация» rule and the
// deterministic pool-first order (Applied Fixes §1).
func TestXsAt_EqualParamsMerged(t *testing.T) {
	t.Parallel()
	src := "int h(int a) {}\n" + "void r() { h("
	file, _ := xs.XsParse(src, "t.xs")

	external := []xs.Decl{{
		Kind:   xs.DeclFunction,
		Name:   "h",
		Type:   "float",
		Params: []xs.Param{{Name: "a", Type: "int"}},
	}}

	pos := common.Pos{Line: 1, Column: uint32(strings.Index("void r() { h(", "h(") + 2)}

	hint, found := newTestComputer(t).XsAt(file, pos, external)

	require.True(t, found)
	assert.Equal(t, "int h(int a)", hint.Label, "pool-first (document) declaration renders")
	assert.Equal(t, 0, hint.Active)
}

// TestXsAt_ActiveNoneOnCallee covers onArg=false: the hint renders with
// no active parameter.
func TestXsAt_ActiveNoneOnCallee(t *testing.T) {
	t.Parallel()
	src := "void r() { xsVectorSet("
	file, _ := xs.XsParse(src, "t.xs")

	hint, found := newTestComputer(t).XsAt(file, xsPos(src, "xsVectorSet", 2), nil)

	require.True(t, found)
	assert.Equal(t, -1, hint.Active)
}

// TestXsAt_ArgIndexBeyondParams covers never-clamp at the hints level:
// no highlight beats a wrong highlight (xsCreateFile declares one
// optional param; the cursor is on the second argument).
func TestXsAt_ArgIndexBeyondParams(t *testing.T) {
	t.Parallel()
	src := "void r() { xsCreateFile(true, false "
	file, _ := xs.XsParse(src, "t.xs")

	hint, found := newTestComputer(t).XsAt(file, xsPos(src, "false", 0), nil)

	require.True(t, found)
	assert.Equal(t, -1, hint.Active)
}

// TestRmsAt_FullListArgsThenAttrs covers the full-list rendering: mined
// range in labels, flag-attr name-only form, optional bracketing — the
// RMS rendering contract in one test.
func TestRmsAt_FullListArgsThenAttrs(t *testing.T) {
	t.Parallel()
	src := "create_elevation 3"
	file, _ := rms.Parse(src, "t.rms")

	hint, found := newTestComputer(t).RmsAt(file, xsPos(src, "3", 0))

	require.True(t, found)
	require.Len(t, hint.Params, 9, "1 arg + 8 attributes, full list, no truncation")
	assert.Equal(t, "[MaxHeight: number 1..16]", hint.Params[0])
	assert.Equal(t, "[set_scale_by_size]", hint.Params[5])
	assert.True(t, strings.HasPrefix(hint.Label, "create_elevation("))
	assert.Equal(t, 0, hint.Active)
}

// TestRmsAt_PercentChanceMinedRange covers the contract's canonical RMS
// example verbatim: structured kind + mined range compose in one label.
func TestRmsAt_PercentChanceMinedRange(t *testing.T) {
	t.Parallel()
	src := "percent_chance 45"
	file, _ := rms.Parse(src, "t.rms")

	hint, found := newTestComputer(t).RmsAt(file, xsPos(src, "45", 0))

	require.True(t, found)
	assert.Equal(t, "percent_chance(%: percent 0..99)", hint.Label)
	assert.Equal(t, []string{"%: percent 0..99"}, hint.Params)
	assert.Equal(t, 0, hint.Active)
}

// TestRmsAt_ActiveOnAttribute covers the attr active mapping: the
// len(Args) offset lands on the rendered attribute's own slot.
func TestRmsAt_ActiveOnAttribute(t *testing.T) {
	t.Parallel()
	src := "create_elevation 3 {\n  spacing 5\n}\n"
	file, _ := rms.Parse(src, "t.rms")

	hint, found := newTestComputer(t).RmsAt(file, common.Pos{Line: 1, Column: uint32(strings.Index("  spacing 5", "5"))})

	require.True(t, found)
	assert.Equal(t, 7, hint.Active)
	require.Len(t, hint.Params, 9)
	assert.Equal(t, "[spacing: number]", hint.Params[7])
}

// TestRmsAt_UnknownCommand covers RMS hints being kb-gated: no invented
// signatures.
func TestRmsAt_UnknownCommand(t *testing.T) {
	t.Parallel()
	src := "create_elefant 5"
	file, _ := rms.Parse(src, "t.rms")

	_, found := newTestComputer(t).RmsAt(file, xsPos(src, "5", 0))

	assert.False(t, found)
}

// TestRmsAt_ActiveAttrNotInKb covers a document attribute absent from
// the kb list: the full list still renders, the active is unset.
func TestRmsAt_ActiveAttrNotInKb(t *testing.T) {
	t.Parallel()
	src := "create_elevation 3 {\n  number_of_objectz 5\n}\n"
	file, _ := rms.Parse(src, "t.rms")

	hint, found := newTestComputer(t).RmsAt(file, common.Pos{Line: 1, Column: uint32(strings.Index("  number_of_objectz 5", "5"))})

	require.True(t, found)
	assert.Equal(t, -1, hint.Active)
	require.Len(t, hint.Params, 9, "full kb list still renders")
}

// TestRmsAt_ActiveIndexPassesThrough covers RMS never-clamp: an index
// beyond the declared args passes through (the client shows no mark).
func TestRmsAt_ActiveIndexPassesThrough(t *testing.T) {
	t.Parallel()
	src := "create_elevation 1 2 3"
	file, _ := rms.Parse(src, "t.rms")

	hint, found := newTestComputer(t).RmsAt(file, xsPos(src, "3", 0))

	require.True(t, found)
	assert.Equal(t, 2, hint.Active)
}
