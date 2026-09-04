// Package analysis runs semantic checks over parsed RMS and XS sources
// against the embedded knowledge base: unknown names, arity mismatches and
// deprecations. Syntax problems stay with the parsers.
package analysis

import (
	"fmt"
	"slices"

	"aoe2-lsp/common"
	"aoe2-lsp/kb"
	"aoe2-lsp/rms"
	"aoe2-lsp/xs"
)

// Diagnostic codes emitted by the analyzer.
const (
	// CodeUnknownCommand marks an RMS command absent from the knowledge base.
	CodeUnknownCommand = "unknown-command"
	// CodeUnknownSection marks an RMS section with no known commands.
	CodeUnknownSection = "unknown-section"
	// CodeUnknownAttribute marks an attribute a command does not accept.
	CodeUnknownAttribute = "unknown-attribute"
	// CodeBadArgument marks a wrong number of positional arguments.
	CodeBadArgument = "bad-argument"
	// CodeDeprecatedEffectPercent marks the legacy effect_percent command.
	CodeDeprecatedEffectPercent = "deprecated-effect-percent"
	// CodeUndefinedSymbol marks an XS identifier that is neither declared
	// nor known to the knowledge base.
	CodeUndefinedSymbol = "undefined-symbol"
	// CodeBadArity marks a call with a wrong number of arguments.
	CodeBadArity = "bad-arity"
)

// xsBuiltins are language-level identifiers no knowledge base covers.
var xsBuiltins = map[string]bool{
	"true": true, "false": true, "vector": true, "null": true,
}

// Analyzer runs semantic checks over RMS and XS ASTs. It is stateless per
// call: no caching, no IO, the input AST is never mutated.
type Analyzer struct {
	store *kb.Store
}

// NewAnalyzer builds an Analyzer over the given knowledge base.
func NewAnalyzer(store *kb.Store) *Analyzer {
	return &Analyzer{store: store}
}

// AnalyzeRms checks one RMS file: unknown commands, sections and
// attributes, argument-count mismatches and the deprecated effect_percent
// form. The result is sorted by position.
func (a *Analyzer) AnalyzeRms(file rms.RmsFile) []common.Diagnostic {
	var diags []common.Diagnostic

	for i := range file.Sections {
		sec := &file.Sections[i]

		if sec.Name != "global" && len(a.store.Commands(sec.Name)) == 0 {
			diags = append(diags, common.Diagnostic{
				Range:    common.Range{Start: sec.Range.Start, End: sec.Range.Start},
				Severity: common.SeverityError,
				Message:  fmt.Sprintf("unknown section %q", sec.Name),
				Code:     CodeUnknownSection,
			})
		}

		diags = a.walkStmts(sec.Statements, diags)
	}

	sortDiags(diags)

	return diags
}

// walkStmts checks a statement list; bare attribute statements attach to
// the last known command (positional RMS semantics outside braces).
func (a *Analyzer) walkStmts(stmts []rms.Statement, diags []common.Diagnostic) []common.Diagnostic {
	lastKnown := ""

	for i := range stmts {
		stmt := &stmts[i]

		switch stmt.Kind {
		case rms.KindCommand:
			diags = a.checkCommand(stmt, &lastKnown, diags)
		default:
			// structural blocks carry no own semantics; nested commands
			// are checked in place
			diags = a.walkStmts(stmt.Children, diags)
		}
	}

	return diags
}

// checkCommand validates one command statement: name, arguments and
// attributes; bare attribute statements fall back to the last known
// command.
func (a *Analyzer) checkCommand(stmt *rms.Statement, lastKnown *string, diags []common.Diagnostic) []common.Diagnostic {
	if len(stmt.Name) > 0 && stmt.Name[0] == '#' {
		return diags // directives (#const, #include) are not commands
	}

	if stmt.Name == "effect_percent" {
		diags = append(diags, common.Diagnostic{
			Range:    common.Range{Start: stmt.Range.Start, End: stmt.Range.Start},
			Severity: common.SeverityWarning,
			Message:  `effect_percent is deprecated; use effect_amount instead`,
			Code:     CodeDeprecatedEffectPercent,
		})
	}

	cmd, known := a.store.Command(stmt.Name)
	if !known {
		// a bare attribute line outside braces: known attribute of the
		// previous command, not an unknown command
		if *lastKnown != "" {
			if _, ok := a.store.Attribute(*lastKnown, stmt.Name); ok {
				return diags
			}
		}

		diags = append(diags, common.Diagnostic{
			Range:    common.Range{Start: stmt.Range.Start, End: stmt.Range.Start},
			Severity: common.SeverityError,
			Message:  fmt.Sprintf("unknown command %q", stmt.Name),
			Code:     CodeUnknownCommand,
		})

		return diags
	}

	*lastKnown = cmd.Name

	// positional arguments: count against the spec (all are optional in
	// practice; too many is always wrong)
	if len(stmt.Args) > len(cmd.Args) {
		diags = append(diags, common.Diagnostic{
			Range:    argsRange(stmt),
			Severity: common.SeverityError,
			Message:  fmt.Sprintf("command %q takes at most %d argument(s), got %d", cmd.Name, len(cmd.Args), len(stmt.Args)),
			Code:     CodeBadArgument,
		})
	}

	// attributes inside braces belong to the command
	for j := range stmt.Attributes {
		attr := &stmt.Attributes[j]

		if _, ok := a.store.Attribute(cmd.Name, attr.Name); !ok {
			diags = append(diags, common.Diagnostic{
				Range:    attr.Range,
				Severity: common.SeverityError,
				Message:  fmt.Sprintf("unknown attribute %q of %q", attr.Name, cmd.Name),
				Code:     CodeUnknownAttribute,
			})
		}
	}

	return diags
}

// AnalyzeXs checks one XS file: identifiers that are neither declared in
// the file nor known to the knowledge base, and calls with a wrong number
// of arguments. The result is sorted by position.
func (a *Analyzer) AnalyzeXs(file xs.XsFile) []common.Diagnostic {
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

	var diags []common.Diagnostic

	for i := range file.Decls {
		diags = a.walkStmtsXs(file.Decls[i].Body, declared, diags)
	}

	sortDiags(diags)

	return diags
}

// collectLocals gathers local declaration names from a statement tree.
func collectLocals(stmts []xs.Stmt, declared map[string]bool) {
	for i := range stmts {
		stmt := &stmts[i]

		if stmt.Kind == xs.StmtDecl {
			for _, e := range stmt.Exprs {
				if e.Kind == xs.ExprBinary && e.Value == "=" && len(e.Children) > 0 {
					if e.Children[0].Kind == xs.ExprIdent {
						declared[e.Children[0].Value] = true
					}
				}
			}
		}

		collectLocals(stmt.Body, declared)
	}
}

// walkStmtsXs checks the expressions of a statement tree.
func (a *Analyzer) walkStmtsXs(stmts []xs.Stmt, declared map[string]bool, diags []common.Diagnostic) []common.Diagnostic {
	for i := range stmts {
		stmt := &stmts[i]

		for j := range stmt.Exprs {
			diags = a.checkExpr(stmt.Exprs[j], declared, diags)
		}

		diags = a.walkStmtsXs(stmt.Body, declared, diags)
	}

	return diags
}

// checkExpr validates one expression tree.
func (a *Analyzer) checkExpr(e xs.Expr, declared map[string]bool, diags []common.Diagnostic) []common.Diagnostic {
	switch e.Kind {
	case xs.ExprCall:
		diags = a.checkCall(e, declared, diags)
	case xs.ExprIdent:
		diags = a.checkIdent(e, declared, diags)
	case xs.ExprBinary:
		if e.Value == "." && len(e.Children) == 2 {
			// vector member access: v.x — only the operand is a symbol
			return a.checkExpr(e.Children[0], declared, diags)
		}
	}

	for _, child := range e.Children {
		diags = a.checkExpr(child, declared, diags)
	}

	return diags
}

// checkIdent reports one plain identifier when it resolves to nothing.
func (a *Analyzer) checkIdent(e xs.Expr, declared map[string]bool, diags []common.Diagnostic) []common.Diagnostic {
	if declared[e.Value] || xsBuiltins[e.Value] {
		return diags
	}

	if _, ok := a.store.Function(e.Value); ok {
		return diags
	}

	if _, ok := a.store.Constant(e.Value); ok {
		return diags
	}

	return append(diags, common.Diagnostic{
		Range:    e.Range,
		Severity: common.SeverityError,
		Message:  fmt.Sprintf("undefined symbol %q", e.Value),
		Code:     CodeUndefinedSymbol,
	})
}

// checkCall validates the callee name and the argument count of a call.
func (a *Analyzer) checkCall(e xs.Expr, declared map[string]bool, diags []common.Diagnostic) []common.Diagnostic {
	callee := e.Callee

	fn, known := a.store.Function(callee)
	if !known {
		if !declared[callee] && !xsBuiltins[callee] {
			diags = append(diags, common.Diagnostic{
				Range:    e.Range,
				Severity: common.SeverityError,
				Message:  fmt.Sprintf("undefined symbol %q", callee),
				Code:     CodeUndefinedSymbol,
			})
		}

		return diags
	}

	minArgs := 0
	for _, param := range fn.Params {
		if param.Required {
			minArgs++
		}
	}

	switch {
	case len(e.Children) < minArgs:
		diags = append(diags, common.Diagnostic{
			Range:    e.Range,
			Severity: common.SeverityError,
			Message:  fmt.Sprintf("%s expects at least %d argument(s), got %d", callee, minArgs, len(e.Children)),
			Code:     CodeBadArity,
		})
	case len(e.Children) > len(fn.Params):
		diags = append(diags, common.Diagnostic{
			Range:    e.Range,
			Severity: common.SeverityError,
			Message:  fmt.Sprintf("%s takes at most %d argument(s), got %d", callee, len(fn.Params), len(e.Children)),
			Code:     CodeBadArity,
		})
	}

	return diags
}

// argsRange spans a statement's positional arguments (the statement range
// when there are none).
func argsRange(stmt *rms.Statement) common.Range {
	if len(stmt.Args) == 0 {
		return common.Range{Start: stmt.Range.Start, End: stmt.Range.Start}
	}

	return common.Range{Start: stmt.Args[0].Range.Start, End: stmt.Args[len(stmt.Args)-1].Range.End}
}

// sortDiags orders diagnostics by position in place.
func sortDiags(diags []common.Diagnostic) {
	slices.SortFunc(diags, func(a, b common.Diagnostic) int {
		if a.Range.Start.Line != b.Range.Start.Line {
			return int(a.Range.Start.Line) - int(b.Range.Start.Line)
		}

		return int(a.Range.Start.Column) - int(b.Range.Start.Column)
	})
}
