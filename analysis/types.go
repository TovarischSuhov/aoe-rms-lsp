// XS type model helpers of the analysis cell: the type compatibility rule
// (xs_coercion) and the scoped symbol table behind XS type inference.

package analysis

import (
	"slices"

	"aoe2-lsp/xs"
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
