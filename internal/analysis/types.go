// XS type model helpers of the analysis cell: the type compatibility rule
// (xs_coercion) and the scoped symbol table behind XS type inference.

package analysis

import (
	"aoe2-lsp/internal/kb"
	"aoe2-lsp/internal/xs"
	"slices"
	"strconv"
	"strings"
)

// Coerce reports whether a value of type actual is usable where expected is
// required. Identical types match, int widens to float implicitly; every
// other combination (float→int, bool↔numbers, string↔rest, vector↔scalars)
// does not. Neither argument may be the unknown type "": callers skip the
// check instead of passing an empty expected or actual.
func Coerce(expected, actual string) bool {
	if expected == actual {
		return true
	}

	return actual == "int" && expected == "float"
}

// TypeEnv is the symbol table of one XS pass: symbol names mapped to their
// types with a stack of scopes. It is built by NewTypeEnv and mutated only
// through Push/Pop/Declare while walking function bodies.
type TypeEnv struct {
	scopes []map[string]string
}

// NewTypeEnv builds the top-level scope from the file declarations:
// variables carry their declared type, functions and externs their return
// type; rules, events and includes are name-only (type "").
func NewTypeEnv(file xs.XsFile) *TypeEnv {
	root := make(map[string]string)

	for i := range file.Decls {
		decl := &file.Decls[i]
		if decl.Name == "" {
			continue
		}

		switch decl.Kind {
		case xs.DeclFunction, xs.DeclExtern, xs.DeclVariable:
			root[decl.Name] = decl.Type
		default:
			root[decl.Name] = ""
		}
	}

	return &TypeEnv{scopes: []map[string]string{root}}
}

// seedExternals declares the include closure's external symbols in the
// root scope, but only names the file itself does not declare anywhere —
// local declarations (including params and locals) always win. Types
// follow the NewTypeEnv rules: variables, functions and externs carry a
// type, other kinds are name-only.
func seedExternals(env *TypeEnv, declared map[string]bool, externals []xs.Decl) {
	for i := range externals {
		decl := &externals[i]
		if decl.Name == "" || declared[decl.Name] {
			continue
		}

		typ := ""
		switch decl.Kind {
		case xs.DeclFunction, xs.DeclExtern, xs.DeclVariable:
			typ = decl.Type
		}

		declared[decl.Name] = true
		env.Declare(decl.Name, typ)
	}
}

// Push opens a nested scope (a function body or a block).
func (env *TypeEnv) Push() {
	env.scopes = append(env.scopes, make(map[string]string))
}

// Pop closes the innermost scope; popping the root scope is a no-op.
func (env *TypeEnv) Pop() {
	if len(env.scopes) > 1 {
		env.scopes = env.scopes[:len(env.scopes)-1]
	}
}

// Declare adds a symbol to the innermost scope. typ "" means the type is
// unknown — the declaration carries no type information.
func (env *TypeEnv) Declare(name, typ string) {
	env.scopes[len(env.scopes)-1][name] = typ
}

// Lookup resolves a symbol from the innermost scope outwards. found is
// false when no scope declares the name; typ is "" for declared symbols
// without type information.
func (env *TypeEnv) Lookup(name string) (string, bool) {
	for _, scope := range slices.Backward(env.scopes) {
		if typ, ok := scope[name]; ok {
			return typ, true
		}
	}

	return "", false
}

// declareLocals declares the names of a typed local declaration statement
// in the innermost scope. Local declarations carry no type in the AST, so
// the type is "". Root-scope declarations are skipped: their names are
// already typed by NewTypeEnv.
func (env *TypeEnv) declareLocals(exprs []xs.Expr) {
	if len(env.scopes) < 2 {
		return
	}

	for i := range exprs {
		e := &exprs[i]

		name := ""

		switch {
		case e.Kind == xs.ExprIdent:
			name = e.Value
		case e.Kind == xs.ExprBinary && e.Value == "=" && len(e.Children) > 0 && e.Children[0].Kind == xs.ExprIdent:
			name = e.Children[0].Value
		}

		if name != "" {
			env.scopes[len(env.scopes)-1][name] = ""
		}
	}
}

// inferBoolOps yield a bool result; inferArithOps widen their operands.
var (
	inferBoolOps = map[string]bool{
		"==": true, "!=": true, "<": true, "<=": true, ">": true, ">=": true,
		"&&": true, "||": true,
	}
	inferArithOps = map[string]bool{
		"+": true, "-": true, "*": true, "/": true, "%": true,
		"&": true, "|": true, "^": true, "<<": true, ">>": true,
	}
)

// InferType returns the XS type of the expression, or "" when the type
// cannot be determined. Conservative by contract: doubt means "" so the
// caller skips the check instead of reporting a false bad-type. A nil
// store is tolerated as long as the tree holds no call expressions.
func InferType(store *kb.Store, env *TypeEnv, e xs.Expr) string {
	switch e.Kind {
	case xs.ExprVector:
		return "vector"
	case xs.ExprLiteral:
		return literalType(e.Value)
	case xs.ExprIdent:
		return identType(env, e.Value)
	case xs.ExprCall:
		if store != nil {
			if fn, ok := store.Function(e.Callee); ok {
				return fn.ReturnType
			}
		}

		typ, found := env.Lookup(e.Callee)
		if found {
			return typ
		}

		return ""
	case xs.ExprUnary:
		return unaryType(store, env, e)
	case xs.ExprBinary:
		return binaryType(store, env, e)
	}

	return ""
}

// literalType classifies a literal lexeme: quoted → string, decimal/hex
// integer → int, anything strconv reads as float → float.
func literalType(value string) string {
	if strings.HasPrefix(value, `"`) {
		return "string"
	}

	if _, err := strconv.ParseInt(value, 0, 64); err == nil {
		return "int"
	}

	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return "float"
	}

	return ""
}

// identType resolves a plain identifier: the boolean and vector builtins
// are typed, null stays unknown, everything else comes from the symbol
// table (missing → "").
func identType(env *TypeEnv, name string) string {
	switch name {
	case "true", "false":
		return "bool"
	case "vector":
		return "vector"
	case "null":
		return ""
	}

	typ, found := env.Lookup(name)
	if !found {
		return ""
	}

	return typ
}

// unaryType: ! always yields bool; the numeric prefix operators keep the
// operand type.
func unaryType(store *kb.Store, env *TypeEnv, e xs.Expr) string {
	if e.Value == "!" {
		return "bool"
	}

	if len(e.Children) == 0 {
		return ""
	}

	return InferType(store, env, e.Children[0])
}

// binaryType types a binary operation: vector member access → float,
// comparisons and logic → bool, arithmetic widens its operands (vector
// stays vector for + - *); assignments and unrecognized operators stay
// unknown.
func binaryType(store *kb.Store, env *TypeEnv, e xs.Expr) string {
	if len(e.Children) != 2 {
		return ""
	}

	if e.Value == "." && e.Children[1].Kind == xs.ExprIdent {
		switch e.Children[1].Value {
		case "x", "y", "z":
			return "float"
		}

		return ""
	}

	if e.Value == "=" {
		return "" // assignments are checked by the analyzer, not typed
	}

	if inferBoolOps[e.Value] {
		return "bool"
	}

	if !inferArithOps[e.Value] {
		return ""
	}

	left := InferType(store, env, e.Children[0])
	right := InferType(store, env, e.Children[1])
	if left == "" || right == "" {
		return ""
	}

	if left == "vector" || right == "vector" {
		if e.Value == "+" || e.Value == "-" || e.Value == "*" {
			return "vector"
		}

		return ""
	}

	if left == "float" || right == "float" {
		return "float"
	}

	if left == "int" && right == "int" {
		return "int"
	}

	return ""
}
