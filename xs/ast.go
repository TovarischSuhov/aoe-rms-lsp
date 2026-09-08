// Package xs parses Age of Empires II XS scripts (C-like grammar plus
// rules and events) into a position-carrying AST with error recovery.
package xs

import (
	"math"
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
	// calls are the call contexts recorded by the parser (one per
	// Kind=call expression); CallAt answers from here.
	calls []*callRec
	// noncode are the string and comment spans; positions inside them
	// resolve to no symbol and no call.
	noncode []common.Range
}

// symbol is one identifier occurrence.
type symbol struct {
	name string
	at   common.Range
}

// callRec is one call context recorded by the parser. The span
// [calleeAt, argEnd) deliberately ignores Expr.Range (which stops at the
// last parsed argument): for an unterminated list argEnd is the eofPos
// recovery frontier, so in-progress calls own the cursor positions an
// editor asks about right after the trigger keystroke.
type callRec struct {
	callee   string
	calleeAt common.Pos
	lparen   common.Pos
	argEnd   common.Pos
	commas   []common.Pos
}

// CallSite is the call context under the cursor — data for signature
// help. Construct-and-use data: no mutation.
type CallSite struct {
	// Callee is the callee name of the innermost enclosing call.
	Callee string
	// ArgIndex is the 0-based ordinal of the argument under the cursor
	// (valid when OnArg: right after "(" it is 0; after k top-level
	// commas it is k).
	ArgIndex int
	// OnArg reports whether the cursor is inside the argument list
	// (false — on the callee name).
	OnArg bool
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

	return f.References(name)
}

// CallAt returns the innermost call enclosing pos (signature help):
// among the recorded call contexts whose span contains the position,
// the one with the latest "(" wins — the inner call of a nesting opens
// later, and a postfix chain resolves to its suffix call. In-progress
// (unterminated) calls answer too: their span stretches to the eofPos
// recovery frontier. ArgIndex is never clamped: exceeding any declared
// parameter count is a consumer decision, not navigation's.
func (f XsFile) CallAt(pos common.Pos) (CallSite, bool) {
	// Step 1: positions inside strings and comments resolve to no call —
	// checked before any call lookup.
	for _, r := range f.noncode {
		if r.Contains(pos) {
			return CallSite{}, false
		}
	}

	best := -1

	for i := range f.calls {
		if f.calls[i].contains(pos) && (best < 0 || f.calls[i].lparen.After(f.calls[best].lparen)) {
			best = i
		}
	}

	if best < 0 {
		return CallSite{}, false
	}

	rec := f.calls[best]

	// On the callee name or between the name and "(": callee-side, no
	// active argument.
	if !pos.After(rec.lparen) {
		return CallSite{Callee: rec.callee, ArgIndex: 0, OnArg: false}, true
	}

	// Count this call's top-level commas with end <= pos. A cursor on a
	// comma character still reports the previous argument (the comma's
	// end is one column further).
	k := 0

	for _, end := range rec.commas {
		if !end.After(pos) {
			k++
		}
	}

	return CallSite{Callee: rec.callee, ArgIndex: k, OnArg: true}, true
}

// contains reports whether pos lies within the record span
// [calleeAt, argEnd).
func (r *callRec) contains(pos common.Pos) bool {
	return !pos.Before(r.calleeAt) && pos.Before(r.argEnd)
}

// References returns every occurrence of the name — the declaration
// included — sorted by position, without needing a position in this file
// (cross-file searches over an include closure).
func (f XsFile) References(name string) []common.Range {
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
	if r, ok := f.firstOccurrence(decl.Name, decl.Range, nil); ok {
		return r
	}

	return decl.Range
}

// firstOccurrence returns the first recorded occurrence of name inside
// within, optionally skipping one start position (the declaration's own
// name token when a parameter repeats it).
func (f XsFile) firstOccurrence(
	name string,
	within common.Range,
	skip *common.Pos,
) (common.Range, bool) {
	for i := range f.symbols {
		s := f.symbols[i]

		if s.name != name || !rangeWithin(s.at, within) {
			continue
		}

		if skip != nil && s.at.Start == *skip {
			continue
		}

		return s.at, true
	}

	return common.Range{}, false
}

// rangeWithin reports whether inner lies inside outer (both bounds
// inclusive on the outer side of the half-open spans).
func rangeWithin(inner common.Range, outer common.Range) bool {
	return !inner.Start.Before(outer.Start) && !outer.End.Before(inner.End)
}

// eofPos closes top-level scopes: it compares after every real position.
var eofPos = common.Pos{
	Line:   ^uint32(0),
	Column: ^uint32(0),
	Offset: math.MaxInt,
}

// Definition returns the declaration of the symbol under pos: the name
// range of the innermost enclosing declarer (LSP textDocument/definition).
// Parameters and locals shadow top-level declarations; builtins and
// unknown names report found=false.
func (f XsFile) Definition(pos common.Pos) (common.Range, bool) {
	occ, ok := f.occurrenceAt(pos)
	if !ok {
		return common.Range{}, false
	}

	return f.bestDeclarer(occ)
}

// occurrenceAt returns the identifier occurrence containing pos.
func (f XsFile) occurrenceAt(pos common.Pos) (symbol, bool) {
	for i := range f.symbols {
		if f.symbols[i].at.Contains(pos) {
			return f.symbols[i], true
		}
	}

	return symbol{}, false
}

// declCandidate is one declaration site of a name together with its
// scope: [nameRange.Start, scopeEnd).
type declCandidate struct {
	nameRange common.Range
	scopeEnd  common.Pos
	depth     int
}

// covers reports whether the candidate scope contains at.
func (c declCandidate) covers(at common.Pos) bool {
	return !at.Before(c.nameRange.Start) && at.Before(c.scopeEnd)
}

// better reports whether c wins over other: the deeper scope first,
// then the nearest preceding declarer.
func (c declCandidate) better(other declCandidate) bool {
	if c.depth != other.depth {
		return c.depth > other.depth
	}

	return c.nameRange.Start.After(other.nameRange.Start)
}

// bestDeclarer picks the winning declaration of the occurrence name:
// the innermost scope covering the occurrence, ties broken by the
// nearest preceding declarer.
func (f XsFile) bestDeclarer(occ symbol) (common.Range, bool) {
	var best declCandidate
	found := false

	consider := func(cand declCandidate) {
		if cand.covers(occ.at.Start) && (!found || cand.better(best)) {
			best = cand
			found = true
		}
	}

	for _, decl := range f.Decls {
		if decl.Kind != DeclInclude && decl.Name == occ.name {
			consider(declCandidate{
				nameRange: f.declNameRange(decl),
				scopeEnd:  eofPos,
			})
		}

		for _, p := range decl.Params {
			if p.Name != occ.name {
				continue
			}

			if r, ok := f.paramNameRange(decl, occ.name); ok {
				consider(declCandidate{nameRange: r, scopeEnd: decl.Range.End, depth: 1})
			}
		}

		var locals []declCandidate

		f.collectLocals(occ.name, decl.Body, decl.Range.End, 1, &locals)
		for _, cand := range locals {
			consider(cand)
		}
	}

	if !found {
		return common.Range{}, false
	}

	return best.nameRange, true
}

// paramNameRange returns the parameter-list token of name: the first
// occurrence inside the declaration span, skipping the function's own
// name token when a parameter repeats it.
func (f XsFile) paramNameRange(decl Decl, name string) (common.Range, bool) {
	var skip *common.Pos

	if decl.Name == name {
		if r, ok := f.firstOccurrence(decl.Name, decl.Range, nil); ok {
			start := r.Start
			skip = &start
		}
	}

	return f.firstOccurrence(name, decl.Range, skip)
}

// collectLocals appends local-declaration candidates of name found in
// stmts, recursing into nested blocks; blockEnd closes the block scope.
func (f XsFile) collectLocals(
	name string,
	stmts []Stmt,
	blockEnd common.Pos,
	depth int,
	out *[]declCandidate,
) {
	for i := range stmts {
		if stmts[i].Kind == StmtDecl {
			for _, item := range stmts[i].Exprs {
				if r, ok := declaredLocal(item, name); ok {
					*out = append(*out, declCandidate{
						nameRange: r,
						scopeEnd:  blockEnd,
						depth:     depth,
					})
				}
			}
		}

		f.collectLocals(name, stmts[i].Body, stmts[i].Range.End, depth+1, out)
	}
}

// declaredLocal extracts the name range of a declared local when the
// declaration item matches name: items are bare identifiers or
// name = initializer binary nodes.
func declaredLocal(item Expr, name string) (common.Range, bool) {
	if item.Kind == ExprIdent && item.Value == name {
		return item.Range, true
	}

	if item.Kind == ExprBinary && item.Value == "=" &&
		len(item.Children) > 0 && item.Children[0].Kind == ExprIdent &&
		item.Children[0].Value == name {
		return item.Children[0].Range, true
	}

	return common.Range{}, false
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
