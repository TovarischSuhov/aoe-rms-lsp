// Token type vocabulary: the shared Token.Type values. The order of the
// server legend is the server's concern; these strings are the contract.
package analysis

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/rms"
	"aoe2-lsp/internal/xs"
	"slices"
)

// Token types of the semantic-tokens legend.
const (
	TokenKnown      = "known"
	TokenUnknown    = "unknown"
	TokenDeprecated = "deprecated"
	TokenSection    = "section"
	TokenKind       = "kind"
)

// TokensRms classifies the identifiers of one RMS file for
// semanticTokens/full: the diagnostic pass's classification exposed as
// ranges instead of problems. No suggestion thresholds apply. The
// result is sorted by position; spans never overlap; the AST is not
// mutated.
func (a *Analyzer) TokensRms(file rms.RmsFile) []common.Token {
	toks := make([]common.Token, 0)

	for i := range file.Sections {
		sec := &file.Sections[i]

		if r, ok := sectionNameRange(file, sec); ok {
			toks = append(toks, common.Token{Range: r, Type: TokenSection})
		}

		toks = a.tokensStmts(file, sec.Statements, "", toks)
	}

	sortTokens(toks)

	return toks
}

// TokensXs classifies the identifiers of one XS file (with the include
// closure's external declarations) for semanticTokens/full: declaration
// names as kind, identifier occurrences and call callees as
// known/unknown with the same predicate as the undefined-symbol check.
func (a *Analyzer) TokensXs(file xs.XsFile, externals []xs.Decl) []common.Token {
	declared := map[string]bool{}

	for i := range file.Decls {
		decl := &file.Decls[i]
		if decl.Name != "" {
			declared[decl.Name] = true
		}

		for _, param := range decl.Params {
			declared[param.Name] = true
		}

		collectLocals(decl.Body, declared)
	}

	for i := range externals {
		if externals[i].Name != "" {
			declared[externals[i].Name] = true
		}
	}

	toks := make([]common.Token, 0)

	for _, sym := range file.Symbols() {
		toks = append(toks, common.Token{Range: sym.Selection, Type: TokenKind})
	}

	for i := range file.Decls {
		toks = a.tokensStmtsXs(file.Decls[i].Body, declared, toks)
	}

	sortTokens(toks)

	return toks
}

// sectionNameRange returns the span of the section's name inside the
// angle brackets of its header — the first word occurrence on the
// header line (the parser records opening and closing tag names; the
// implicit global section has no header).
func sectionNameRange(file rms.RmsFile, sec *rms.Section) (common.Range, bool) {
	if sec.Name == "global" {
		return common.Range{}, false
	}

	for _, r := range file.References(sec.Name) {
		if r.Start.Line == sec.Range.Start.Line {
			return r, true
		}
	}

	return common.Range{}, false
}

// tokensStmts classifies a statement list; bare attribute statements
// attach to the last known command (positional RMS semantics).
func (a *Analyzer) tokensStmts(
	file rms.RmsFile,
	stmts []rms.Statement,
	lastKnown string,
	toks []common.Token,
) []common.Token {
	for i := range stmts {
		stmt := &stmts[i]

		switch stmt.Kind {
		case rms.KindCommand:
			toks = a.tokenCommand(file, stmt, &lastKnown, toks)
		default:
			toks = a.tokenBlockStmt(file, stmt, toks)
		}
	}

	return toks
}

// tokenCommand classifies one command statement: name, argument leaves
// and attributes; bare attribute lines fall back to the last known
// command. Mirrors checkCommand's control flow.
func (a *Analyzer) tokenCommand(
	file rms.RmsFile,
	stmt *rms.Statement,
	lastKnown *string,
	toks []common.Token,
) []common.Token {
	if len(stmt.Name) > 0 && stmt.Name[0] == '#' {
		return toks // directives (#const, #include) are not commands
	}

	cmd, known := a.store.Command(stmt.Name)

	if !known {
		if *lastKnown != "" {
			if _, ok := a.store.Attribute(*lastKnown, stmt.Name); ok {
				toks = append(toks, common.Token{Range: nameRangeOf(stmt), Type: TokenKnown})

				return a.tokenArgExprs(stmt.Args, toks)
			}
		}

		return append(toks, common.Token{Range: nameRangeOf(stmt), Type: TokenUnknown})
	}

	*lastKnown = cmd.Name

	nameType := TokenKnown
	if stmt.Name == "effect_percent" {
		nameType = TokenDeprecated
	}

	toks = append(toks, common.Token{Range: nameRangeOf(stmt), Type: nameType})
	toks = a.tokenArgExprs(stmt.Args, toks)

	for j := range stmt.Attributes {
		attr := &stmt.Attributes[j]

		_, attrKnown := a.store.Attribute(cmd.Name, attr.Name)

		attrType := TokenKnown
		if !attrKnown {
			attrType = TokenUnknown
		}

		// the deprecated command name carries its deprecation wherever
		// it appears as a statement/attribute name
		if attr.Name == "effect_percent" {
			attrType = TokenDeprecated
		}

		toks = append(toks, common.Token{Range: attrNameRange(file, attr), Type: attrType})
		toks = a.tokenExpr(attr.Value, toks)
	}

	return toks
}

// tokenBlockStmt classifies a structural statement: the block name,
// its positional argument leaves, then children in place (a fresh
// last-known context, mirroring checkBlockStmt → walkStmts).
func (a *Analyzer) tokenBlockStmt(file rms.RmsFile, stmt *rms.Statement, toks []common.Token) []common.Token {
	if stmt.Name != "" && stmt.Name[0] != '#' {
		_, known := a.store.Command(stmt.Name)

		blockType := TokenKnown
		if !known {
			blockType = TokenUnknown
		}

		toks = append(toks, common.Token{Range: nameRangeOf(stmt), Type: blockType})
		toks = a.tokenArgExprs(stmt.Args, toks)
	}

	return a.tokensStmts(file, stmt.Children, "", toks)
}

// attrNameRange returns the attribute's NAME span: Attribute.Range
// spans the whole attribute line (name through value), so the exact
// name token comes from the parse-time word index — falling back to
// the name length when no occurrence matches.
func attrNameRange(file rms.RmsFile, attr *rms.Attribute) common.Range {
	for _, r := range file.References(attr.Name) {
		if r.Start.Line == attr.Range.Start.Line && r.Start.Column == attr.Range.Start.Column {
			return r
		}
	}

	return common.Range{Start: attr.Range.Start, End: posShift(attr.Range.Start, len(attr.Name))}
}

// tokenArgExprs classifies the ident/const leaves of argument
// expressions against the knowledge base constants.
func (a *Analyzer) tokenArgExprs(exprs []rms.Expr, toks []common.Token) []common.Token {
	for _, e := range exprs {
		toks = a.tokenExpr(e, toks)
	}

	return toks
}

// tokenExpr classifies one expression tree's identifier leaves.
func (a *Analyzer) tokenExpr(e rms.Expr, toks []common.Token) []common.Token {
	if e.Kind == rms.KindIdent || e.Kind == rms.KindConst {
		exprType := TokenUnknown
		if _, ok := a.store.Constant(e.Value); ok {
			exprType = TokenKnown
		}

		toks = append(toks, common.Token{Range: e.Range, Type: exprType})
	}

	for _, child := range e.Children {
		toks = a.tokenExpr(child, toks)
	}

	return toks
}

// tokensStmtsXs classifies the expressions of an XS statement tree.
func (a *Analyzer) tokensStmtsXs(stmts []xs.Stmt, declared map[string]bool, toks []common.Token) []common.Token {
	for i := range stmts {
		stmt := &stmts[i]

		for j := range stmt.Exprs {
			toks = a.tokenExprXs(stmt.Exprs[j], declared, toks)
		}

		toks = a.tokensStmtsXs(stmt.Body, declared, toks)
	}

	return toks
}

// tokenExprXs classifies one XS expression tree: call callees (the
// callee is the first token of the call) and identifier occurrences.
// Vector member access classifies only the operand — the member is not
// a symbol (mirrors checkExpr).
func (a *Analyzer) tokenExprXs(e xs.Expr, declared map[string]bool, toks []common.Token) []common.Token {
	switch e.Kind {
	case xs.ExprCall:
		callee := common.Range{Start: e.Range.Start, End: posShift(e.Range.Start, len(e.Callee))}
		toks = append(toks, common.Token{Range: callee, Type: a.xsNameType(e.Callee, declared)})
	case xs.ExprIdent:
		toks = append(toks, common.Token{Range: e.Range, Type: a.xsNameType(e.Value, declared)})
	case xs.ExprBinary:
		if e.Value == "." && len(e.Children) == 2 {
			return a.tokenExprXs(e.Children[0], declared, toks)
		}
	}

	for _, child := range e.Children {
		toks = a.tokenExprXs(child, declared, toks)
	}

	return toks
}

// xsNameType classifies one XS name with the undefined-symbol
// predicate: declared (file, locals, externals), builtin, kb function
// or kb constant.
func (a *Analyzer) xsNameType(name string, declared map[string]bool) string {
	if declared[name] || xsBuiltins[name] {
		return TokenKnown
	}

	if _, ok := a.store.Function(name); ok {
		return TokenKnown
	}

	if _, ok := a.store.Constant(name); ok {
		return TokenKnown
	}

	return TokenUnknown
}

// nameRangeOf returns the statement's name span: the name is the
// first token of the statement (parser guarantee).
func nameRangeOf(stmt *rms.Statement) common.Range {
	return common.Range{Start: stmt.Range.Start, End: posShift(stmt.Range.Start, len(stmt.Name))}
}

// posShift advances a position by n bytes within its line.
func posShift(p common.Pos, n int) common.Pos {
	return common.Pos{Line: p.Line, Column: p.Column + uint32(n), Offset: p.Offset + n}
}

// sortTokens orders tokens by position (stable, so equal starts keep
// insertion order).
func sortTokens(toks []common.Token) {
	slices.SortStableFunc(toks, func(a, b common.Token) int {
		switch {
		case a.Range.Start.Before(b.Range.Start):
			return -1
		case b.Range.Start.Before(a.Range.Start):
			return 1
		default:
			return 0
		}
	})
}
