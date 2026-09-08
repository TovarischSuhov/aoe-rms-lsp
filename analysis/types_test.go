package analysis

import (
	"aoe2-lsp/kb"
	"aoe2-lsp/xs"
	"testing"

	"github.com/stretchr/testify/require"
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
	_ func(store *kb.Store, env *TypeEnv, e xs.Expr) string  = InferType
)

func TestCoerce_ValueShapes(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestInferType_Literals(t *testing.T) {
	t.Parallel()
	env := NewTypeEnv(xs.XsFile{Decls: []xs.Decl{
		{Kind: xs.DeclVariable, Name: "n", Type: "int"},
	}})

	tests := []struct {
		name string
		expr xs.Expr
		want string
	}{
		{name: "decimal int", expr: xs.Expr{Kind: xs.ExprLiteral, Value: "42"}, want: "int"},
		{name: "hex int", expr: xs.Expr{Kind: xs.ExprLiteral, Value: "0x1F"}, want: "int"},
		{name: "float", expr: xs.Expr{Kind: xs.ExprLiteral, Value: "3.5"}, want: "float"},
		{name: "string literal", expr: xs.Expr{Kind: xs.ExprLiteral, Value: `"s"`}, want: "string"},
		{name: "true builtin", expr: xs.Expr{Kind: xs.ExprIdent, Value: "true"}, want: "bool"},
		{name: "false builtin", expr: xs.Expr{Kind: xs.ExprIdent, Value: "false"}, want: "bool"},
		{name: "vector literal", expr: xs.Expr{Kind: xs.ExprVector}, want: "vector"},
		{name: "declared variable", expr: xs.Expr{Kind: xs.ExprIdent, Value: "n"}, want: "int"},
		{name: "null builtin is unknown", expr: xs.Expr{Kind: xs.ExprIdent, Value: "null"}, want: ""},
		{name: "undeclared ident is unknown", expr: xs.Expr{Kind: xs.ExprIdent, Value: "whatever"}, want: ""},
		{name: "unparsable literal", expr: xs.Expr{Kind: xs.ExprLiteral, Value: "12abc"}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, InferType(nil, env, tt.expr))
		})
	}
}

func TestInferType_CallFromKb(t *testing.T) {
	t.Parallel()
	store, err := kb.NewStore()
	require.NoError(t, err)

	env := NewTypeEnv(xs.XsFile{Decls: []xs.Decl{
		{Kind: xs.DeclFunction, Name: "local", Type: "float"},
	}})

	tests := []struct {
		name string
		expr xs.Expr
		want string
	}{
		{name: "kb function return", expr: xs.Expr{Kind: xs.ExprCall, Callee: "xsGetMapSeed"}, want: "int"},
		{name: "local function return", expr: xs.Expr{Kind: xs.ExprCall, Callee: "local"}, want: "float"},
		{name: "unknown callee", expr: xs.Expr{Kind: xs.ExprCall, Callee: "nope"}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, InferType(store, env, tt.expr))
		})
	}
}

func TestInferType_Operators(t *testing.T) {
	t.Parallel()
	store, err := kb.NewStore()
	require.NoError(t, err)

	env := NewTypeEnv(xs.XsFile{})

	lit := func(v string) xs.Expr { return xs.Expr{Kind: xs.ExprLiteral, Value: v} }
	ident := func(v string) xs.Expr { return xs.Expr{Kind: xs.ExprIdent, Value: v} }
	bin := func(op string, l, r xs.Expr) xs.Expr {
		return xs.Expr{Kind: xs.ExprBinary, Value: op, Children: []xs.Expr{l, r}}
	}

	tests := []struct {
		name string
		expr xs.Expr
		want string
	}{
		{name: "int plus float widens", expr: bin("+", lit("1"), lit("2.0")), want: "float"},
		{name: "int plus int stays int", expr: bin("+", lit("1"), lit("2")), want: "int"},
		{name: "comparison yields bool", expr: bin("==", lit("1"), lit("2")), want: "bool"},
		{name: "logical yields bool", expr: bin("&&", ident("true"), ident("false")), want: "bool"},
		{name: "vector member access", expr: bin(".", ident("v"), ident("x")), want: "float"},
		{name: "unknown member stays unknown", expr: bin(".", ident("v"), ident("w")), want: ""},
		{name: "vector over scalar stays unknown", expr: bin("/", xs.Expr{Kind: xs.ExprVector}, lit("2")), want: ""},
		{name: "assignment is untyped", expr: bin("=", ident("a"), lit("1")), want: ""},
		{name: "compound assignment is untyped", expr: bin("+=", ident("a"), lit("1")), want: ""},
		{name: "string arithmetic stays unknown", expr: bin("+", lit(`"a"`), lit(`"b"`)), want: ""},
		{name: "bool arithmetic stays unknown", expr: bin("+", ident("true"), lit("1")), want: ""},
		{name: "vector plus vector", expr: bin("+", xs.Expr{Kind: xs.ExprVector}, xs.Expr{Kind: xs.ExprVector}), want: "vector"},
		{name: "vector times scalar", expr: bin("*", xs.Expr{Kind: xs.ExprVector}, lit("2")), want: "vector"},
		{name: "unknown operand stays unknown", expr: bin("+", ident("lost"), lit("1")), want: ""},
		{name: "binary without operands", expr: xs.Expr{Kind: xs.ExprBinary, Value: "+"}, want: ""},
		{name: "unary not yields bool", expr: xs.Expr{Kind: xs.ExprUnary, Value: "!", Children: []xs.Expr{ident("true")}}, want: "bool"},
		{name: "unary minus keeps int", expr: xs.Expr{Kind: xs.ExprUnary, Value: "-", Children: []xs.Expr{lit("5")}}, want: "int"},
		{name: "unary without operand", expr: xs.Expr{Kind: xs.ExprUnary, Value: "-"}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, InferType(store, env, tt.expr))
		})
	}
}
