// Package analysis runs semantic checks over parsed RMS and XS sources
// against the embedded knowledge base: unknown names, arity mismatches and
// deprecations. Syntax problems stay with the parsers.
package analysis

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/kb"
	"aoe2-lsp/internal/rms"
	"aoe2-lsp/internal/xs"
	"fmt"
	"slices"
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
	// CodeBadArgumentValue marks an argument value outside its allowed
	// range (percent specs: 0..100).
	CodeBadArgumentValue = "bad-argument-value"
	// CodeBadType marks an XS value whose inferred type is incompatible
	// with the expected one (call argument, assignment, return).
	CodeBadType = "bad-type"
	// CodeDeprecatedEffectPercent marks the legacy effect_percent command.
	CodeDeprecatedEffectPercent = "deprecated-effect-percent"
	// CodeProbablyIgnored marks a command attribute that is a documented
	// no-op in its position: the game most likely ignores it.
	CodeProbablyIgnored = "probably-ignored"
	// CodeTypoAlias marks a name matching a known typo alias; the message
	// names the correct spelling.
	CodeTypoAlias = "typo-alias"
	// CodeUndefinedSymbol marks an XS identifier that is neither declared
	// nor known to the knowledge base.
	CodeUndefinedSymbol = "undefined-symbol"
	// CodeBadArity marks a call with a wrong number of arguments.
	CodeBadArity = "bad-arity"
	// CodeDuplicateDeclaration marks a name declared twice in the same
	// scope: repeated top-level declarations and same-block locals.
	CodeDuplicateDeclaration = "duplicate-declaration"
	// CodeMissingSection marks a map-shaped RMS file without a section
	// the game needs (only TERRAIN_GENERATION qualifies — the corpus
	// shows every other "required" candidate is a legitimate skip).
	CodeMissingSection = "missing-section"
	// CodeEmptySection marks a section header with no statements before
	// the next header.
	CodeEmptySection = "empty-section"
	// CodeUnusedDeclaration marks a declaration with no references:
	// locals never mentioned again and top-level functions unreachable
	// from the file's engine entry points (rules and events).
	CodeUnusedDeclaration = "unused-declaration"
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
// attributes, argument-count mismatches, nuance-based hints and warnings
// for documented no-op pairs and typos, and data-driven deprecation and
// legacy markers. The result is sorted by position.
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

	diags = checkSections(file, diags)

	sortDiags(diags)

	return diags
}

// minSectionsForMissingCheck separates map-shaped files from include
// fragments: the corpus contains fragments with up to three headers
// (included snippet pieces) while real maps carry at least four. Below
// the threshold the structural checks stay silent.
const minSectionsForMissingCheck = 4

// requiredSection is the only section a playable map cannot go
// without. Everything else the formulation called "required" turned
// out to be a legitimate skip in the corpus (PLAYER_SETUP lives in 29
// maps' fragments, re-opened sections are second generation passes,
// PLAYER_SETUP-without-create_player is a published-map pattern).
// Section names parse lowercased (the kb keys them that way); the
// display spelling stays uppercase.
const requiredSection = "terrain_generation"

// requiredSectionDisplay is the user-facing spelling of requiredSection.
const requiredSectionDisplay = "TERRAIN_GENERATION"

// checkSections runs the structural lints the per-command semantics
// cannot see: a map-shaped file without the required section and
// section headers with nothing inside. The missing-section diagnostic
// points at the first section header — the natural insertion point
// for the insert-section quickfix.
func checkSections(file rms.RmsFile, diags []common.Diagnostic) []common.Diagnostic {
	// the parser buckets statements before the first header into an
	// implicit "global" section — not a real header
	headers := 0

	for i := range file.Sections {
		if file.Sections[i].Name != "global" {
			headers++
		}
	}

	if headers >= minSectionsForMissingCheck {
		first := file.Sections[0]
		if first.Name == "global" && len(file.Sections) > 1 {
			first = file.Sections[1]
		}

		hasRequired := false

		for i := range file.Sections {
			if file.Sections[i].Name == requiredSection {
				hasRequired = true

				break
			}
		}

		if !hasRequired {
			diags = append(diags, common.Diagnostic{
				Range:    first.Range,
				Severity: common.SeverityWarning,
				Message: fmt.Sprintf("missing required section <%s>; without it the map generates no terrain",
					requiredSectionDisplay),
				Code: CodeMissingSection,
			})
		}
	}

	for i := range file.Sections {
		sec := &file.Sections[i]
		if sec.Name == "global" || len(sec.Statements) > 0 {
			continue
		}

		diags = append(diags, common.Diagnostic{
			Range: common.Range{
				Start: sec.Range.Start,
				End: common.Pos{
					Line:   sec.Range.Start.Line,
					Column: sec.Range.Start.Column + uint32(len(sec.Name)+2),
				},
			},
			Severity: common.SeverityHint,
			Message:  fmt.Sprintf("section %q is empty and does nothing", sec.Name),
			Code:     CodeEmptySection,
		})
	}

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
			// are checked in place. percent_chance folds its chance into
			// positional args of the block statement itself.
			diags = a.checkBlockStmt(stmt, diags)
		}
	}

	return diags
}

// nuanceDiag checks an unknown attribute name against the kb nuances: a
// documented no-op pair becomes a probably-ignored hint, a known typo
// alias becomes a warning naming the correct spelling.
func (a *Analyzer) nuanceDiag(owner string, name string, r common.Range) (common.Diagnostic, bool) {
	if a.store.NoopAttribute(owner, name) {
		return common.Diagnostic{
			Range:    r,
			Severity: common.SeverityHint,
			Message:  fmt.Sprintf("attribute %q of %q is probably ignored by the game", name, owner),
			Code:     CodeProbablyIgnored,
		}, true
	}

	if right, ok := a.store.Alias(name); ok {
		return common.Diagnostic{
			Range:    r,
			Severity: common.SeverityWarning,
			Message:  fmt.Sprintf("unknown attribute %q of %q; did you mean %q?", name, owner, right),
			Code:     CodeTypoAlias,
		}, true
	}

	return common.Diagnostic{}, false
}

// checkBlockStmt checks a structural statement: known block commands keep
// their positional args validated, then children are walked in place.
func (a *Analyzer) checkBlockStmt(stmt *rms.Statement, diags []common.Diagnostic) []common.Diagnostic {
	if stmt.Name != "" && stmt.Name[0] != '#' {
		if cmd, known := a.store.Command(stmt.Name); known {
			diags = a.checkArgValues(stmt.Args, cmd.Args, diags)
		}
	}

	return a.walkStmts(stmt.Children, diags)
}

// checkCommand validates one command statement: name, arguments and
// attributes; bare attribute statements fall back to the last known
// command.
func (a *Analyzer) checkCommand(stmt *rms.Statement, lastKnown *string, diags []common.Diagnostic) []common.Diagnostic {
	if len(stmt.Name) > 0 && stmt.Name[0] == '#' {
		if code, message, ok := a.store.Legacy(stmt.Name); ok {
			diags = append(diags, common.Diagnostic{
				Range:    common.Range{Start: stmt.Range.Start, End: stmt.Range.Start},
				Severity: common.SeverityHint,
				Message:  message,
				Code:     code,
			})
		}

		return diags // directives (#const, #include) are not commands
	}

	if code, message, ok := a.store.Deprecated(stmt.Name); ok {
		diags = append(diags, common.Diagnostic{
			Range:    common.Range{Start: stmt.Range.Start, End: stmt.Range.Start},
			Severity: common.SeverityWarning,
			Message:  message,
			Code:     code,
		})
	}

	cmd, known := a.store.Command(stmt.Name)
	if !known {
		// a bare attribute line outside braces: known attribute of the
		// previous command, not an unknown command — its value gets the
		// same checks as an attribute inside braces
		owner := ""
		if *lastKnown != "" {
			if spec, ok := a.store.Attribute(*lastKnown, stmt.Name); ok {
				return a.checkArgValues(stmt.Args, []kb.CommandArg{spec}, diags)
			}

			owner = *lastKnown

			if diag, reported := a.nuanceDiag(owner, stmt.Name, stmt.Range); reported {
				return append(diags, diag)
			}
		}

		if right, ok := a.store.Alias(stmt.Name); ok {
			return append(diags, common.Diagnostic{
				Range:    common.Range{Start: stmt.Range.Start, End: stmt.Range.Start},
				Severity: common.SeverityWarning,
				Message:  fmt.Sprintf("unknown command %q; did you mean %q?", stmt.Name, right),
				Code:     CodeTypoAlias,
			})
		}

		diags = append(diags, common.Diagnostic{
			Range:    common.Range{Start: stmt.Range.Start, End: stmt.Range.Start},
			Severity: common.SeverityError,
			Message:  fmt.Sprintf("unknown command %q", stmt.Name) + suggestSuffix(a.commandNames(), stmt.Name),
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

	// positional argument values: leaf expressions against their specs
	diags = a.checkArgValues(stmt.Args, cmd.Args, diags)

	// attributes inside braces belong to the command
	for j := range stmt.Attributes {
		attr := &stmt.Attributes[j]

		spec, known := a.store.Attribute(cmd.Name, attr.Name)
		if !known {
			if diag, reported := a.nuanceDiag(cmd.Name, attr.Name, attr.Range); reported {
				diags = append(diags, diag)

				continue
			}

			attrNames := make([]string, 0, len(cmd.Attributes))
			for _, attrSpec := range cmd.Attributes {
				attrNames = append(attrNames, attrSpec.Name)
			}

			diags = append(diags, common.Diagnostic{
				Range:    attr.Range,
				Severity: common.SeverityError,
				Message: fmt.Sprintf("unknown attribute %q of %q", attr.Name, cmd.Name) +
					suggestSuffix(attrNames, attr.Name),
				Code: CodeUnknownAttribute,
			})

			continue
		}

		if len(attr.Value.Children) > 0 {
			continue
		}

		if diag, reported := CheckRmsValue(spec, attr.Value.Kind, attr.Value.Value, attr.Value.Range); reported {
			diags = append(diags, diag)
		}
	}

	return diags
}

// AnalyzeXs checks one XS file: identifiers that are neither declared in
// the file nor in externals (the include closure's declarations) nor
// known to the knowledge base, calls with a wrong number of arguments and
// value types incompatible with the expected ones. Local declarations
// always win over externals; nil/empty externals keep the previous
// behavior. The result is sorted by position.
func (a *Analyzer) AnalyzeXs(file xs.XsFile, externals []xs.Decl) []common.Diagnostic {
	declared := map[string]bool{}
	diags := checkDuplicateDecls(file)
	diags = checkUnusedDecls(file, diags)

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

	env := NewTypeEnv(file)
	seedExternals(env, declared, externals)

	for i := range file.Decls {
		decl := &file.Decls[i]

		switch decl.Kind {
		case xs.DeclFunction, xs.DeclRule, xs.DeclEvent:
			// function bodies get their own scope with typed params
			env.Push()

			for _, param := range decl.Params {
				env.Declare(param.Name, param.Type)
			}

			fnType := ""
			if decl.Kind == xs.DeclFunction {
				fnType = decl.Type
			}

			diags = a.walkStmtsXs(decl.Body, declared, env, fnType, diags)
			env.Pop()
		default:
			diags = a.walkStmtsXs(decl.Body, declared, env, "", diags)
		}
	}

	sortDiags(diags)

	return diags
}

// collectLocals gathers local declaration names from a statement tree.
func collectLocals(stmts []xs.Stmt, declared map[string]bool) {
	for i := range stmts {
		stmt := &stmts[i]

		if stmt.Kind == xs.StmtDecl {
			declareItems(stmt.Exprs, declared)
		}

		if stmt.Kind == xs.StmtFor {
			// a two-section XS for keeps its init assignment in Exprs
			// (`for (i = 1; <= size)`) — the loop variable is declared
			// all the same
			declareItems(stmt.Exprs, declared)
		}

		collectLocals(stmt.Body, declared)
	}
}

// checkDuplicateDecls reports same-scope redeclarations: repeated
// top-level declarations and locals declared twice in one block.
// Nested scopes shadow legally (C-like semantics) and stay silent —
// the check is deliberately conservative against false positives.
func checkDuplicateDecls(file xs.XsFile) []common.Diagnostic {
	var diags []common.Diagnostic

	top := map[string]bool{}
	for i := range file.Decls {
		decl := &file.Decls[i]

		// include is a file path, not a symbol; unnamed rules cannot
		// collide
		if decl.Kind == xs.DeclInclude || decl.Name == "" {
			continue
		}

		if top[decl.Name] {
			diags = append(diags, common.Diagnostic{
				Range:    decl.Range,
				Severity: common.SeverityWarning,
				Message: fmt.Sprintf("duplicate declaration of %q; the earlier declaration stays in effect",
					decl.Name),
				Code: CodeDuplicateDeclaration,
			})
		}
		top[decl.Name] = true

		diags = dupBlock(decl.Body, diags)
	}

	return diags
}

// dupBlock walks one block scope: repeated declarators are duplicates,
// each nested construct opens its own scope.
func dupBlock(stmts []xs.Stmt, diags []common.Diagnostic) []common.Diagnostic {
	return dupWalk(stmts, map[string]bool{}, diags)
}

// dupWalk checks the statements of one scope; scope maps names already
// declared at this level.
func dupWalk(stmts []xs.Stmt, scope map[string]bool, diags []common.Diagnostic) []common.Diagnostic {
	for i := range stmts {
		stmt := &stmts[i]

		switch stmt.Kind {
		case xs.StmtDecl:
			// declarations land in the current scope
			diags = dupDeclarators(stmt.Exprs, scope, diags)
			continue
		case xs.StmtIf:
			// Body[0] = then, Body[1] = else: independent scopes
			for j := range stmt.Body {
				diags = dupBlock(stmt.Body[j:j+1], diags)
			}
		case xs.StmtFor:
			// the init section owns the loop's scope; the body nests
			// inside it, so sibling loops may redeclare freely
			loop := map[string]bool{}
			diags = dupDeclarators(stmt.Exprs, loop, diags)
			diags = dupWalk(stmt.Body, loop, diags)
		case xs.StmtSwitch:
			// each case body is its own scope (conservative: shared-scope
			// case declarations stay unflagged)
			for j := range stmt.Body {
				if stmt.Body[j].Kind == xs.StmtCase {
					diags = dupBlock(stmt.Body[j].Body, diags)
				}
			}
		case xs.StmtCondition, xs.StmtAction:
			// permissive rule blocks: independent scopes
			diags = dupBlock(stmt.Body, diags)
		default:
			// block wrappers and bare statements: child scope for the
			// body, loop/while/do bodies included
			diags = dupBlock(stmt.Body, diags)
		}
	}

	return diags
}

// dupDeclarators checks the names one declaration introduces (bare
// declarators and assignment targets) against the current scope.
func dupDeclarators(exprs []xs.Expr, scope map[string]bool, diags []common.Diagnostic) []common.Diagnostic {
	for _, e := range exprs {
		name := ""
		switch {
		case e.Kind == xs.ExprIdent:
			name = e.Value // bare declarator: int x;
		case e.Kind == xs.ExprBinary && e.Value == "=" && len(e.Children) > 0 &&
			e.Children[0].Kind == xs.ExprIdent:
			name = e.Children[0].Value
		}
		if name == "" {
			continue
		}

		if scope[name] {
			diags = append(diags, common.Diagnostic{
				Range:    e.Range,
				Severity: common.SeverityWarning,
				Message: fmt.Sprintf("duplicate declaration of %q; the earlier declaration stays in effect",
					name),
				Code: CodeDuplicateDeclaration,
			})
		}
		scope[name] = true
	}

	return diags
}

// checkUnusedDecls reports declarations with no references. The
// entry-point model: rules and events are called by the engine and are
// the roots of the reachability walk; a file with none of them is a
// library whose top-level functions are exported through include
// closures, so its top level stays unflagged (cross-file callers are
// invisible at file level). Top-level variables are skipped too —
// their initializers may carry load-time side effects. Locals need no
// entry-point reasoning: an initializer with a call keeps the hint
// quiet for the same side-effect reason.
func checkUnusedDecls(file xs.XsFile, diags []common.Diagnostic) []common.Diagnostic {
	functions := map[string]*xs.Decl{}
	scriptShaped := false

	for i := range file.Decls {
		decl := &file.Decls[i]
		if decl.Kind == xs.DeclFunction && decl.Name != "" {
			functions[decl.Name] = decl
		}
		if decl.Kind == xs.DeclRule || decl.Kind == xs.DeclEvent {
			scriptShaped = true
		}
	}

	if scriptShaped {
		// walk the transitive call graph from the engine entry points;
		// names reachable from a root are used
		used := map[string]bool{}
		expanded := map[string]bool{}

		for i := range file.Decls {
			decl := &file.Decls[i]
			if decl.Kind == xs.DeclRule || decl.Kind == xs.DeclEvent {
				expandUses(decl.Body, functions, used, expanded)
			}
		}

		for i := range file.Decls {
			decl := &file.Decls[i]
			if decl.Kind != xs.DeclFunction || decl.Name == "" {
				continue
			}
			if !used[decl.Name] {
				diags = append(diags, common.Diagnostic{
					Range:    decl.Range,
					Severity: common.SeverityHint,
					Message: fmt.Sprintf("function %q is never called from rules or events",
						decl.Name),
					Code: CodeUnusedDeclaration,
				})
			}
		}
	}

	// locals inside executable bodies only: top-level `local` statements
	// are file globals the prelude exports by the dozen, and top-level
	// variables carry load-time side effects
	for i := range file.Decls {
		decl := &file.Decls[i]
		if decl.Kind != xs.DeclFunction && decl.Kind != xs.DeclRule && decl.Kind != xs.DeclEvent {
			continue
		}

		// name-based per enclosing declaration; the declaration itself
		// does not count as a use, everything else does (shadowing
		// collisions are duplicate-declaration's domain instead)
		used := collectIdents(decl.Body, true)

		diags = unusedLocals(decl.Body, used, diags)
	}

	return diags
}

// expandUses adds every name mentioned in body to used and recurses
// into the bodies of the top-level functions it calls.
func expandUses(body []xs.Stmt, functions map[string]*xs.Decl, used map[string]bool, expanded map[string]bool) {
	for ident := range collectIdents(body, false) {
		used[ident] = true

		if _, isLocalFn := functions[ident]; isLocalFn && !expanded[ident] {
			expanded[ident] = true
			expandUses(functions[ident].Body, functions, used, expanded)
		}
	}
}

// collectIdents gathers every identifier mentioned in a statement tree:
// idents, call callees and assignment targets alike. With skipDecls the
// declared names are excluded — the declaration target of a declarator
// does not count as a use of itself — while initializers still do.
func collectIdents(stmts []xs.Stmt, skipDecls bool) map[string]bool {
	used := map[string]bool{}

	var walkExpr func(e xs.Expr)
	walkExpr = func(e xs.Expr) {
		if e.Kind == xs.ExprIdent {
			used[e.Value] = true
		}
		if e.Kind == xs.ExprCall {
			used[e.Callee] = true
		}
		for _, child := range e.Children {
			walkExpr(child)
		}
	}

	// walkDeclarator walks one declarator: a bare `int x;` has no
	// mentions, `x = init` mentions everything except the target x
	walkDeclarator := func(e xs.Expr) {
		if e.Kind == xs.ExprBinary && e.Value == "=" {
			for _, child := range e.Children[min(1, len(e.Children)-1):] {
				walkExpr(child)
			}
		}
	}

	walkExprs := func(exprs []xs.Expr, inDecl bool) {
		for j := range exprs {
			if inDecl && skipDecls {
				walkDeclarator(exprs[j])
			} else {
				walkExpr(exprs[j])
			}
		}
	}

	var walk func(stmts []xs.Stmt)
	walk = func(stmts []xs.Stmt) {
		for i := range stmts {
			stmt := &stmts[i]

			if stmt.Kind == xs.StmtFor && len(stmt.Exprs) > 0 && skipDecls {
				// Exprs[0] is the init section (a declarator or an
				// assignment to an outer variable, whose cond repeats it
				// anyway), the rest reads variables normally
				walkExprs(stmt.Exprs[:1], true)
				walkExprs(stmt.Exprs[1:], false)
			} else {
				walkExprs(stmt.Exprs, stmt.Kind == xs.StmtDecl)
			}

			walk(stmt.Body)
		}
	}

	walk(stmts)

	return used
}

// unusedLocals reports locals whose name is never mentioned in decl's
// body; an initializer with a call may carry side effects and stays
// quiet.
func unusedLocals(stmts []xs.Stmt, used map[string]bool, diags []common.Diagnostic) []common.Diagnostic {
	for i := range stmts {
		stmt := &stmts[i]

		if stmt.Kind == xs.StmtDecl || stmt.Kind == xs.StmtFor {
			for _, e := range stmt.Exprs {
				name := declaratorName(e)
				if name == "" || used[name] {
					continue
				}
				if initializerHasCall(e) {
					continue
				}

				diags = append(diags, common.Diagnostic{
					Range:    e.Range,
					Severity: common.SeverityHint,
					Message:  fmt.Sprintf("local %q is declared but never used", name),
					Code:     CodeUnusedDeclaration,
				})
			}
		}

		diags = unusedLocals(stmt.Body, used, diags)
	}

	return diags
}

// declaratorName returns the declared name of one declarator
// expression: a bare ident or an assignment target.
func declaratorName(e xs.Expr) string {
	switch {
	case e.Kind == xs.ExprIdent:
		return e.Value
	case e.Kind == xs.ExprBinary && e.Value == "=" && len(e.Children) > 0 &&
		e.Children[0].Kind == xs.ExprIdent:
		return e.Children[0].Value
	}

	return ""
}

// initializerHasCall reports whether the declarator's initializer
// subtree contains a call (a side effect the deletion hint must not
// suggest removing).
func initializerHasCall(e xs.Expr) bool {
	if e.Kind == xs.ExprCall {
		return true
	}

	for _, child := range e.Children {
		if initializerHasCall(child) {
			return true
		}
	}

	return false
}

// declareItems marks the names introduced by one declaration's expression
// list: bare identifiers and assignment targets.
func declareItems(exprs []xs.Expr, declared map[string]bool) {
	for _, e := range exprs {
		switch {
		case e.Kind == xs.ExprIdent:
			declared[e.Value] = true // bare declarator: int x;
		case e.Kind == xs.ExprBinary && e.Value == "=" && len(e.Children) > 0:
			if e.Children[0].Kind == xs.ExprIdent {
				declared[e.Children[0].Value] = true
			}
		}
	}
}

// walkStmtsXs checks the expressions of a statement tree; fnType is the
// return type of the enclosing function ("" disables the return check).
func (a *Analyzer) walkStmtsXs(stmts []xs.Stmt, declared map[string]bool, env *TypeEnv, fnType string, diags []common.Diagnostic) []common.Diagnostic {
	for i := range stmts {
		stmt := &stmts[i]

		if stmt.Kind == xs.StmtDecl {
			env.declareLocals(stmt.Exprs)
		}

		if stmt.Kind == xs.StmtFor {
			env.declareLocals(stmt.Exprs)
		}

		if stmt.Kind == xs.StmtReturn && len(stmt.Exprs) > 0 && knownXsType(fnType) {
			diags = a.checkReturnType(stmt.Exprs[0], env, fnType, diags)
		}

		for j := range stmt.Exprs {
			diags = a.checkExpr(stmt.Exprs[j], declared, env, diags)
		}

		diags = a.walkStmtsXs(stmt.Body, declared, env, fnType, diags)
	}

	return diags
}

// checkExpr validates one expression tree.
func (a *Analyzer) checkExpr(e xs.Expr, declared map[string]bool, env *TypeEnv, diags []common.Diagnostic) []common.Diagnostic {
	switch e.Kind {
	case xs.ExprCall:
		diags = a.checkCall(e, declared, env, diags)
	case xs.ExprIdent:
		diags = a.checkIdent(e, declared, diags)
	case xs.ExprBinary:
		if e.Value == "." && len(e.Children) == 2 {
			// vector member access: v.x — only the operand is a symbol
			return a.checkExpr(e.Children[0], declared, env, diags)
		}

		if e.Value == "=" && len(e.Children) == 2 && e.Children[0].Kind == xs.ExprIdent {
			diags = a.checkAssign(e, env, diags)
		}
	}

	for _, child := range e.Children {
		diags = a.checkExpr(child, declared, env, diags)
	}

	return diags
}

// checkAssign reports a typed assignment whose value type is incompatible
// with the declared type of the target identifier.
func (a *Analyzer) checkAssign(e xs.Expr, env *TypeEnv, diags []common.Diagnostic) []common.Diagnostic {
	target, value := e.Children[0], e.Children[1]

	targetType, found := env.Lookup(target.Value)
	if !found || !knownXsType(targetType) {
		return diags
	}

	typ := InferType(a.store, env, value)
	if typ == "" || Coerce(targetType, typ) {
		return diags
	}

	return append(diags, common.Diagnostic{
		Range:    value.Range,
		Severity: common.SeverityError,
		Message:  fmt.Sprintf("cannot assign %s to %s %q", typ, targetType, target.Value),
		Code:     CodeBadType,
	})
}

// checkReturnType reports a return value whose type is incompatible with
// the declared return type of the enclosing function.
func (a *Analyzer) checkReturnType(e xs.Expr, env *TypeEnv, fnType string, diags []common.Diagnostic) []common.Diagnostic {
	typ := InferType(a.store, env, e)
	if typ == "" || Coerce(fnType, typ) {
		return diags
	}

	return append(diags, common.Diagnostic{
		Range:    e.Range,
		Severity: common.SeverityError,
		Message:  fmt.Sprintf("return value of type %s is not compatible with %s", typ, fnType),
		Code:     CodeBadType,
	})
}

// knownXsType reports whether the type name participates in type checks.
func knownXsType(typ string) bool {
	switch typ {
	case "int", "float", "bool", "string", "vector":
		return true
	}

	return false
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
		Message:  fmt.Sprintf("undefined symbol %q", e.Value) + a.symbolSuggestion(declared, e.Value),
		Code:     CodeUndefinedSymbol,
	})
}

// symbolSuggestion renders the undefined-symbol suggestion over kb
// names joined with the file's declared names.
func (a *Analyzer) symbolSuggestion(declared map[string]bool, typo string) string {
	names := append(a.xsSymbolNames(), declaredNames(declared)...)

	return suggestSuffix(names, typo)
}

// checkCall validates the callee name, the argument count and the
// argument types of a call of a knowledge base function.
func (a *Analyzer) checkCall(e xs.Expr, declared map[string]bool, env *TypeEnv, diags []common.Diagnostic) []common.Diagnostic {
	callee := e.Callee

	fn, known := a.store.Function(callee)
	if !known {
		if !declared[callee] && !xsBuiltins[callee] {
			diags = append(diags, common.Diagnostic{
				Range:    e.Range,
				Severity: common.SeverityError,
				Message:  fmt.Sprintf("undefined symbol %q", callee) + a.symbolSuggestion(declared, callee),
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

	for i, arg := range e.Children {
		if i >= len(fn.Params) || !knownXsType(fn.Params[i].Type) {
			continue
		}

		typ := InferType(a.store, env, arg)
		if typ != "" && !Coerce(fn.Params[i].Type, typ) {
			diags = append(diags, common.Diagnostic{
				Range:    arg.Range,
				Severity: common.SeverityError,
				Message:  fmt.Sprintf("argument %d of %s is %s, want %s", i+1, callee, typ, fn.Params[i].Type),
				Code:     CodeBadType,
			})
		}
	}

	return diags
}

// checkArgValues value-checks leaf positional arguments against their
// specs; helper calls and operator expressions stay unchecked.
func (a *Analyzer) checkArgValues(args []rms.Expr, spec []kb.CommandArg, diags []common.Diagnostic) []common.Diagnostic {
	for j := range args {
		if j >= len(spec) || len(args[j].Children) > 0 {
			continue
		}

		if diag, reported := CheckRmsValue(spec[j], args[j].Kind, args[j].Value, args[j].Range); reported {
			diags = append(diags, diag)
		}
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
