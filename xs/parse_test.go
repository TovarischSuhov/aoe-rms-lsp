package xs

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aoe2-lsp/common"
)

// loadFixture reads a testdata fixture.
func loadFixture(t *testing.T, name string) string {
	t.Helper()

	raw, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)

	return string(raw)
}

// TestXsParse_Fixtures parses every fixture and checks declarations and
// diagnostics.
func TestXsParse_Fixtures(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		declKinds  []string
		diagSubstr []string
	}{
		{
			name:      "functions and locals",
			fixture:   "functions.xs",
			declKinds: []string{DeclFunction, DeclFunction},
		},
		{
			name:      "rules with condition and action",
			fixture:   "rules.xs",
			declKinds: []string{DeclRule, DeclRule},
		},
		{
			name:      "vectors",
			fixture:   "vectors.xs",
			declKinds: []string{DeclVariable, DeclFunction},
		},
		{
			name:      "control flow",
			fixture:   "control.xs",
			declKinds: []string{DeclFunction},
		},
		{
			name:      "broken input recovers",
			fixture:   "broken.xs",
			declKinds: []string{DeclVariable, DeclFunction, DeclFunction},
			diagSubstr: []string{
				`expected a parameter name`,
				`unexpected ";" in expression`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, diags := XsParse(loadFixture(t, tt.fixture), tt.fixture)

			require.Equal(t, tt.fixture, file.Name)

			kinds := make([]string, 0, len(file.Decls))
			for _, d := range file.Decls {
				kinds = append(kinds, d.Kind)
			}

			assert.Equal(t, tt.declKinds, kinds)

			for _, want := range tt.diagSubstr {
				found := false
				for _, d := range diags {
					if strings.Contains(d.Message, want) {
						found = true
						break
					}
				}

				assert.True(t, found, "expected a diagnostic containing %q, got %v", want, messages(diags))
			}

			assertSorted(t, diags)
		})
	}
}

// TestXsParse_FunctionShapes checks params, locals and calls.
func TestXsParse_FunctionShapes(t *testing.T) {
	file, diags := XsParse(loadFixture(t, "functions.xs"), "functions.xs")
	require.Empty(t, diags)

	add := file.Decls[0]
	assert.Equal(t, "add", add.Name)
	assert.Equal(t, "int", add.Type)
	require.Len(t, add.Params, 2)
	assert.Equal(t, Param{Name: "a", Type: "int"}, add.Params[0])
	assert.Equal(t, Param{Name: "b", Type: "int"}, add.Params[1])
	require.Len(t, add.Body, 1)
	assert.Equal(t, StmtReturn, add.Body[0].Kind)

	main := file.Decls[1]
	assert.Equal(t, "main", main.Name)
	assert.Equal(t, "void", main.Type)

	// int total = add(1, 2); — a local declaration with a call initializer
	decl := main.Body[0]
	assert.Equal(t, StmtDecl, decl.Kind)
	require.Len(t, decl.Exprs, 1)

	assign := decl.Exprs[0]
	assert.Equal(t, ExprBinary, assign.Kind)
	assert.Equal(t, "=", assign.Value)
	require.Len(t, assign.Children, 2)
	assert.Equal(t, ExprIdent, assign.Children[0].Kind)
	assert.Equal(t, "total", assign.Children[0].Value)
	assert.Equal(t, ExprCall, assign.Children[1].Kind)
	assert.Equal(t, "add", assign.Children[1].Callee)
	require.Len(t, assign.Children[1].Children, 2)
}

// TestXsParse_RuleBodies checks rule modifiers, condition/action sections
// and the permissive semicolon-less statements inside them.
func TestXsParse_RuleBodies(t *testing.T) {
	file, diags := XsParse(loadFixture(t, "rules.xs"), "rules.xs")
	require.Empty(t, diags)

	rule := file.Decls[0]
	assert.Equal(t, "resource_check", rule.Name)
	require.Len(t, rule.Body, 2)

	condition := rule.Body[0]
	assert.Equal(t, StmtCondition, condition.Kind)
	require.Len(t, condition.Body, 1)

	action := rule.Body[1]
	assert.Equal(t, StmtAction, action.Kind)
	require.Len(t, action.Body, 1)
	assert.Equal(t, StmtExpr, action.Body[0].Kind)
}

// TestXsParse_Vectors checks vector literals, members and constructors.
func TestXsParse_Vectors(t *testing.T) {
	file, diags := XsParse(loadFixture(t, "vectors.xs"), "vectors.xs")
	require.Empty(t, diags)

	origin := file.Decls[0]
	assert.Equal(t, DeclVariable, origin.Kind)
	assert.Equal(t, "origin", origin.Name)
	assert.Equal(t, "vector", origin.Type)

	// vector(0, 0, 0) — constructor call as initializer
	assert.Equal(t, ExprCall, origin.initValue().Kind)
	assert.Equal(t, "vector", origin.initValue().Callee)
	require.Len(t, origin.initValue().Children, 3)

	place := file.Decls[1]
	require.Len(t, place.Params, 1)
	assert.Equal(t, Param{Name: "pos", Type: "vector"}, place.Params[0])

	// vector offset = (1.0, 2.0, 3.0); — a vector literal
	offset := place.Body[0]
	require.Len(t, offset.Exprs, 1)
	literal := offset.Exprs[0].Children[1]
	assert.Equal(t, ExprVector, literal.Kind)
	require.Len(t, literal.Children, 3)
	assert.Equal(t, "2.0", literal.Children[1].Value)

	// pos.x - origin.x — member access chains
	dx := place.Body[1].Exprs[0].Children[1]
	assert.Equal(t, ExprBinary, dx.Kind)
	assert.Equal(t, "-", dx.Value)
	require.Len(t, dx.Children, 2)
	assert.Equal(t, ".", dx.Children[0].Value)
	assert.Equal(t, "pos", dx.Children[0].Children[0].Value)
}

// TestXsParse_ControlFlow checks for/while/do/switch/case structures.
func TestXsParse_ControlFlow(t *testing.T) {
	file, diags := XsParse(loadFixture(t, "control.xs"), "control.xs")
	require.Empty(t, diags)

	body := file.Decls[0].Body
	require.Len(t, body, 6)

	forStmt := body[0]
	assert.Equal(t, StmtFor, forStmt.Kind)
	require.Len(t, forStmt.Exprs, 3) // init, cond, step
	cond := forStmt.Exprs[1]
	assert.Equal(t, ExprBinary, cond.Kind)
	assert.Equal(t, "<", cond.Value)
	assert.Equal(t, "i", cond.Children[0].Value)
	assert.Equal(t, "10", cond.Children[1].Value)
	// the init declarator names i
	assert.Equal(t, "i", forStmt.Exprs[0].Children[0].Value)

	// the for body is a block wrapping the if with else
	block := forStmt.Body[0]
	assert.Equal(t, StmtBlock, block.Kind)
	require.Len(t, block.Body, 1)
	ifStmt := block.Body[0]
	assert.Equal(t, StmtIf, ifStmt.Kind)
	require.Len(t, ifStmt.Body, 2)

	assert.Equal(t, StmtWhile, body[2].Kind)
	assert.Equal(t, StmtDo, body[3].Kind)

	switchStmt := body[4]
	assert.Equal(t, StmtSwitch, switchStmt.Kind)
	require.Len(t, switchStmt.Body, 4) // case 0, case 1, case 2, default
	assert.Equal(t, StmtCase, switchStmt.Body[0].Kind)
	require.Len(t, switchStmt.Body[0].Exprs, 1)
	assert.Empty(t, switchStmt.Body[1].Body, "case 1 falls through with no body")

	assert.Equal(t, StmtReturn, body[5].Kind)
	assert.Empty(t, body[5].Exprs, "bare return")
}

// TestXsParse_Prelude checks the 5k-line game dump parses with zero false
// errors and all externs are declared.
func TestXsParse_Prelude(t *testing.T) {
	raw, err := os.ReadFile("../docs/ref/ugc-guide/xs/prelude.xs")
	require.NoError(t, err)

	file, diags := XsParse(string(raw), "prelude.xs")

	require.Empty(t, diags, "prelude.xs must parse without false errors: %v", messages(diags))

	externs := 0
	for _, d := range file.Decls {
		if d.Kind == DeclExtern {
			externs++
		}
	}

	assert.Equal(t, 884, externs)

	// a known extern is navigable
	name, found := file.SymbolAt(common.Pos{Line: 7, Column: 19})
	require.True(t, found)
	assert.Equal(t, "infiniteLoopLimit", name)
}

// TestXsParse_NeverNil checks the total-garbage path.
func TestXsParse_NeverNil(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "empty", src: ""},
		{name: "comment only", src: "/* nothing */"},
		{name: "garbage", src: "\x00\x01 @@ <<<< \x02"},
		{name: "truncated", src: "void main() {\n\txsGetMapSeed("},
		{name: "lone braces", src: "{{{ }}}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, _ := XsParse(tt.src, "garbage")

			require.NotNil(t, file)
		})
	}
}

// TestXsFile_SymbolAt checks hover navigation: a position on a call
// returns the callee name, on an argument its identifier.
func TestXsFile_SymbolAt(t *testing.T) {
	src := "void main() {\n\tf(seed);\n\tg();\n}"
	file, _ := XsParse(src, "inline")

	// line 1: "\tf(seed);" — f at column 1, seed at column 3
	name, found := file.SymbolAt(common.Pos{Line: 1, Column: 1})
	require.True(t, found)
	assert.Equal(t, "f", name, "position on a call returns the callee")

	name, found = file.SymbolAt(common.Pos{Line: 1, Column: 4})
	require.True(t, found)
	assert.Equal(t, "seed", name)

	name, found = file.SymbolAt(common.Pos{Line: 2, Column: 1})
	require.True(t, found)
	assert.Equal(t, "g", name)

	_, found = file.SymbolAt(common.Pos{Line: 0, Column: 0})
	assert.False(t, found, "position on 'void' keyword is not a symbol")
}

// TestXsParse_RecoveryTruncated checks recovery on input cut mid-call.
func TestXsParse_RecoveryTruncated(t *testing.T) {
	file, diags := XsParse("void main() {\n\txsGetMapSeed(", "truncated")

	require.NotEmpty(t, file.Decls)
	assert.NotEmpty(t, diags)
	assertSorted(t, diags)
}

// initValue returns the initializer expression of a variable or extern
// declaration (empty when absent).
func (d Decl) initValue() Expr {
	if len(d.Body) == 0 || len(d.Body[0].Exprs) == 0 {
		return Expr{}
	}

	e := d.Body[0].Exprs[0]
	if e.Kind == ExprBinary && len(e.Children) == 2 {
		return e.Children[1]
	}

	return e
}

// assertSorted checks that diagnostics are ordered by position.
func assertSorted(t *testing.T, diags []common.Diagnostic) {
	t.Helper()

	for i := 1; i < len(diags); i++ {
		prev, cur := diags[i-1].Range.Start, diags[i].Range.Start

		if prev.Line == cur.Line {
			require.LessOrEqual(t, prev.Column, cur.Column)

			continue
		}

		require.Less(t, prev.Line, cur.Line)
	}
}

// messages extracts diagnostic messages for failure output.
func messages(diags []common.Diagnostic) []string {
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, d.Message)
	}

	return out
}

// TestXsParse_StringEscapeNewlineTracksLines checks that a backslash
// escape followed by a newline inside a string literal advances the
// scanner line counter: positions after the literal stay on their
// physical lines.
func TestXsParse_StringEscapeNewlineTracksLines(t *testing.T) {
	src := "string s = \"abc\\\nDEF\";\nint z = 1;"

	file, diags := XsParse(src, "t.xs")

	require.Empty(t, diags)
	require.Len(t, file.Decls, 2)
	assert.Equal(t, "s", file.Decls[0].Name)
	assert.Equal(t, uint32(0), file.Decls[0].Range.Start.Line)
	assert.Equal(t, "z", file.Decls[1].Name)
	assert.Equal(t, uint32(2), file.Decls[1].Range.Start.Line)
}

// TestXsParse_StringEscapeNonNewlineUnchanged checks that escapes not
// followed by a newline keep the previous line accounting untouched.
func TestXsParse_StringEscapeNonNewlineUnchanged(t *testing.T) {
	src := "string s = \"a\\nb\\\"c\";\nint z = 1;"

	file, diags := XsParse(src, "t.xs")

	require.Empty(t, diags)
	require.Len(t, file.Decls, 2)
	assert.Equal(t, uint32(1), file.Decls[1].Range.Start.Line)
}

// TestXsParse_StringEscapeAtEOFDoesNotPanic checks a trailing backslash
// as the very last byte of a string literal at end of input.
func TestXsParse_StringEscapeAtEOFDoesNotPanic(t *testing.T) {
	file, diags := XsParse("string s = \"abc\\", "t.xs")

	require.NotNil(t, file)
	assert.Empty(t, diags)
	require.Len(t, file.Decls, 1)
	assert.Equal(t, "s", file.Decls[0].Name)
}
