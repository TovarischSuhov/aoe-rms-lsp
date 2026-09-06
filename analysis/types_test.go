package analysis

import (
	"testing"

	"github.com/stretchr/testify/require"

	"aoe2-lsp/xs"
)

// Compile-time contract checks: the exported surface must match
// analysis/CODEMANIFEST exactly.
var (
	_ func(expected, actual string) bool                     = Coerce
	_ func(file xs.XsFile) *TypeEnv                          = NewTypeEnv
	_ func(e *TypeEnv)                                       = (*TypeEnv).Push
	_ func(e *TypeEnv)                                       = (*TypeEnv).Pop
	_ func(e *TypeEnv, name, typ string)                     = (*TypeEnv).Declare
	_ func(e *TypeEnv, name string) (typ string, found bool) = (*TypeEnv).Lookup
)

func TestCoerce_Table(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		ok       bool
	}{
		{name: "same int", expected: "int", actual: "int", ok: true},
		{name: "same float", expected: "float", actual: "float", ok: true},
		{name: "same vector", expected: "vector", actual: "vector", ok: true},
		{name: "int widens to float", expected: "float", actual: "int", ok: true},
		{name: "float narrows to int", expected: "int", actual: "float", ok: false},
		{name: "bool to int", expected: "int", actual: "bool", ok: false},
		{name: "int to bool", expected: "bool", actual: "int", ok: false},
		{name: "vector to float", expected: "float", actual: "vector", ok: false},
		{name: "same string", expected: "string", actual: "string", ok: true},
		{name: "string to int", expected: "int", actual: "string", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.ok, Coerce(tt.expected, tt.actual))
		})
	}
}

func TestTypeEnv_ScopeShadowing(t *testing.T) {
	file := xs.XsFile{Decls: []xs.Decl{
		{Kind: xs.DeclVariable, Name: "x", Type: "int"},
		{Kind: xs.DeclFunction, Name: "f", Type: "void", Params: []xs.Param{{Name: "x", Type: "float"}}},
		{Kind: xs.DeclExtern, Name: "ext", Type: "int"},
		{Kind: xs.DeclRule, Name: "r"},
		{Name: ""}, // recovery artifact: nameless declarations are skipped
	}}

	env := NewTypeEnv(file)

	// a parameter shadows the top-level variable inside the function body
	env.Push()
	env.Declare("x", "float")

	// a nested block shadows the parameter again
	env.Push()
	env.Declare("x", "vector")

	typ, found := env.Lookup("x")
	require.True(t, found)
	require.Equal(t, "vector", typ)

	env.Pop()

	typ, found = env.Lookup("x")
	require.True(t, found)
	require.Equal(t, "float", typ)

	// rules are declared without a type
	typ, found = env.Lookup("r")
	require.True(t, found)
	require.Equal(t, "", typ)

	// externs expose their return type
	typ, found = env.Lookup("ext")
	require.True(t, found)
	require.Equal(t, "int", typ)

	// after Pop the top-level symbol is visible again
	env.Pop()

	typ, found = env.Lookup("x")
	require.True(t, found)
	require.Equal(t, "int", typ)

	// functions expose their return type
	typ, found = env.Lookup("f")
	require.True(t, found)
	require.Equal(t, "void", typ)
}

func TestTypeEnv_PopRootNoOp(t *testing.T) {
	env := NewTypeEnv(xs.XsFile{Decls: []xs.Decl{
		{Kind: xs.DeclVariable, Name: "x", Type: "int"},
	}})

	env.Pop()
	env.Pop() // the root scope must survive

	typ, found := env.Lookup("x")
	require.True(t, found)
	require.Equal(t, "int", typ)
}

func TestTypeEnv_Undeclared(t *testing.T) {
	env := NewTypeEnv(xs.XsFile{})

	env.Declare("tmp", "int")

	typ, found := env.Lookup("missing")
	require.False(t, found)
	require.Equal(t, "", typ)

	// Declare is live: the symbol resolves immediately
	typ, found = env.Lookup("tmp")
	require.True(t, found)
	require.Equal(t, "int", typ)
}
