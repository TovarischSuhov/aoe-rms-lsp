// Package xs parses Age of Empires II XS scripts (C-like grammar plus
// rules and events) into a position-carrying AST with error recovery.
package xs

import (
	"slices"

	"aoe2-lsp/common"
)

// Declaration kinds.
const (
	// DeclFunction is a function definition with a body.
	DeclFunction = "function"
	// DeclVariable is a top-level variable declaration.
	DeclVariable = "variable"
	// DeclRule is a rule declaration (condition/action body).
	DeclRule = "rule"
	// DeclEvent is an event registration.
	DeclEvent = "event"
	// DeclInclude is an include directive.
	DeclInclude = "include"
	// DeclExtern is an extern declaration (prelude.xs): signature only.
	DeclExtern = "extern"
)

// Statement kinds.
const (
	// StmtBlock is a { } block.
	StmtBlock = "block"
	// StmtIf is an if with optional else in Body[1].
	StmtIf = "if"
	// StmtWhile is a while loop.
	StmtWhile = "while"
	// StmtDo is a do/while loop.
	StmtDo = "do"
	// StmtFor is a for loop; Exprs holds init/cond/step.
	StmtFor = "for"
	// StmtSwitch is a switch; Body holds case statements.
	StmtSwitch = "switch"
	// StmtCase is one case (or default) inside a switch.
	StmtCase = "case"
	// StmtReturn is a return with an optional value.
	StmtReturn = "return"
	// StmtBreak is a break.
	StmtBreak = "break"
	// StmtContinue is a continue.
	StmtContinue = "continue"
	// StmtExpr is an expression statement.
	StmtExpr = "expr"
	// StmtDecl is a typed local declaration; Exprs hold name=init pairs.
	StmtDecl = "decl"
	// StmtCondition is a rule condition block (permissive rule form).
	StmtCondition = "condition"
	// StmtAction is a rule action block (permissive rule form).
	StmtAction = "action"
)

// Expression kinds.
const (
	// ExprCall is a call: Callee names the function, Children are args.
	ExprCall = "call"
	// ExprBinary is an operation: Value is the operator.
	ExprBinary = "binary"
	// ExprUnary is a prefix operation: Value is the operator.
	ExprUnary = "unary"
	// ExprLiteral is a number or string literal.
	ExprLiteral = "literal"
	// ExprVector is a (x, y, z) literal with three operands.
	ExprVector = "vector"
	// ExprIdent is a plain identifier.
	ExprIdent = "ident"
)

// XsFile is the root of the XS AST.
type XsFile struct {
	// Name is the file or block name passed to XsParse.
	Name string
	// Decls are the top-level declarations in source order.
	Decls []Decl

	// symbols are all identifier occurrences (declaration names, callees,
	// plain identifiers) with their ranges; SymbolAt answers from here.
	symbols []symbol
}

// symbol is one identifier occurrence.
type symbol struct {
	name string
	at   common.Range
}

// SymbolAt returns the identifier under pos (for hover and the completion
// trigger); a position on a call returns the callee name.
func (f XsFile) SymbolAt(pos common.Pos) (string, bool) {
	for i := range f.symbols {
		if f.symbols[i].at.Contains(pos) {
			return f.symbols[i].name, true
		}
	}

	return "", false
}

// ReferencesAt returns every occurrence of the name under pos — the
// declaration included — sorted by position (LSP textDocument/references).
// Matching is syntactic, by name: same-name symbols from different
// scopes are not distinguished.
func (f XsFile) ReferencesAt(pos common.Pos) []common.Range {
	name, ok := f.SymbolAt(pos)
	if !ok {
		return nil
	}

	var out []common.Range

	for _, s := range f.symbols {
		if s.name == name {
			out = append(out, s.at)
		}
	}

	slices.SortFunc(out, func(a, b common.Range) int {
		switch {
		case a.Start.Before(b.Start):
			return -1
		case b.Start.Before(a.Start):
			return 1
		default:
			return 0
		}
	})

	return out
}

// Symbols returns the flat outline of the top-level declarations (LSP
// documentSymbol) in source order; include declarations are skipped —
// the kind vocabulary has no entry for them, and their names are string
// paths, not identifiers.
func (f XsFile) Symbols() []common.Symbol {
	out := make([]common.Symbol, 0, len(f.Decls))

	for _, decl := range f.Decls {
		if decl.Kind == DeclInclude {
			continue
		}

		out = append(out, common.Symbol{
			Kind:      decl.Kind,
			Name:      decl.Name,
			Range:     decl.Range,
			Selection: f.declNameRange(decl),
		})
	}

	return out
}

// declNameRange returns the name token range of the declaration: the
// first recorded occurrence of the name inside the declaration span
// (the parser records declaration names before any body occurrence).
// The whole span is the fallback for recovered declarations.
func (f XsFile) declNameRange(decl Decl) common.Range {
	for _, s := range f.symbols {
		if s.name == decl.Name && rangeWithin(s.at, decl.Range) {
			return s.at
		}
	}

	return decl.Range
}

// rangeWithin reports whether inner lies inside outer (both bounds
// inclusive on the outer side of the half-open spans).
func rangeWithin(inner common.Range, outer common.Range) bool {
	return !inner.Start.Before(outer.Start) && !outer.End.Before(inner.End)
}

// Decl is one top-level declaration.
//
// Kind=function: Params and Body are filled. Kind=rule: the condition lives
// in Body as statements (StmtCondition/StmtAction or plain statements).
// Kind=extern: signature only, no body (prelude.xs).
type Decl struct {
	// Kind is one of function / variable / rule / event / include / extern.
	Kind string
	// Name is the declared symbol name (include path without quotes).
	Name string
	// Type is the variable type or the function return type.
	Type string
	// Params are the function parameters.
	Params []Param
	// Body is the declaration body.
	Body []Stmt
	// Range is the span of the declaration.
	Range common.Range
}

// Param is one XS function parameter.
type Param struct {
	// Name is the parameter name.
	Name string
	// Type is the parameter type (int/float/bool/string/vector/void/...).
	Type string
}

// Stmt is one statement; Kind gives the meaning of Exprs (condition/value)
// and Body (branches/body). For Kind=for, Exprs holds init/cond/step.
type Stmt struct {
	// Kind is the statement kind.
	Kind string
	// Exprs are the expressions (condition or value).
	Exprs []Expr
	// Body is the nested statements.
	Body []Stmt
	// Range is the span of the statement.
	Range common.Range
}

// Expr is one expression.
//
// Kind=call: Callee names the function and Children are the arguments.
// Kind=vector: Children are the three operands. Kind=binary/unary: Value is
// the operator.
type Expr struct {
	// Kind is one of call / binary / unary / literal / vector / ident.
	Kind string
	// Callee is the called function name (for call).
	Callee string
	// Value is the literal or the operator.
	Value string
	// Children are the operands or call arguments.
	Children []Expr
	// Range is the span of the expression.
	Range common.Range
}
