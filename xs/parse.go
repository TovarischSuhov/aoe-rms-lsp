package xs

import (
	"fmt"
	"slices"
	"strings"

	"aoe2-lsp/common"
)

// XsParse parses XS source (a .xs file or an inline block from rms.XsBlock)
// into an AST with error recovery: every problem becomes a Diagnostic and
// parsing resynchronizes to the next ';' or '}'. The returned file is never
// nil; diags are sorted by position.
func XsParse(source string, name string) (XsFile, []common.Diagnostic) {
	p := &xparser{file: XsFile{Name: name}}
	p.scan(source)

	for !p.at(xEOF) {
		p.parseDecl()
	}

	slices.SortStableFunc(p.diags, func(a, b common.Diagnostic) int {
		if a.Range.Start.Line != b.Range.Start.Line {
			return int(a.Range.Start.Line) - int(b.Range.Start.Line)
		}

		return int(a.Range.Start.Column) - int(b.Range.Start.Column)
	})

	p.file.symbols = p.syms
	p.file.calls = p.calls
	p.file.noncode = p.noncode

	return p.file, p.diags
}

// xparser holds the incremental state of one XsParse run.
type xparser struct {
	file    XsFile
	diags   []common.Diagnostic
	toks    []xtoken
	pos     int
	syms    []symbol
	calls   []*callRec
	noncode []common.Range
}

// parseDecl parses one top-level declaration, recovering to the next
// declaration boundary on a syntax error.
func (p *xparser) parseDecl() {
	tok := p.peek()

	if tok.kind == xOp && tok.text == ";" {
		p.next()

		return
	}

	if tok.kind != xIdent {
		p.reportf(tok.at, common.SeverityError, "syntax", "unexpected %q at top level", tok.text)
		p.syncDecl()

		return
	}

	switch tok.text {
	case "extern":
		p.parseExtern()
	case "include", "includeDuno":
		p.parseInclude()
	case "rule":
		p.parseRule()
	case "event":
		p.parseEvent()
	default:
		if p.looksLikeDecl() {
			p.parseTypedDecl()
			return
		}

		p.reportf(tok.at, common.SeverityError, "syntax", "unexpected identifier %q at top level", tok.text)
		p.syncDecl()
	}
}

// looksLikeDecl reports whether an identifier starts a typed declaration:
// a type word followed by a name (function or variable).
func (p *xparser) looksLikeDecl() bool {
	if p.pos+1 >= len(p.toks) {
		return false
	}

	if !isTypeWord(p.peek().text) && p.peek().text != "const" {
		return false
	}

	return p.toks[p.pos+1].kind == xIdent
}

// parseTypedDecl parses a variable (`int x = 1;`) or a function
// (`int f(int a) { ... }`) declaration.
func (p *xparser) parseTypedDecl() {
	start := p.peek().at.Start
	typ := p.parseTypeWords()

	name := p.next()

	if name.kind != xIdent {
		p.reportf(name.at, common.SeverityError, "syntax", "expected a name after type %q", typ)
		p.syncDecl()

		return
	}

	p.record(name)

	if p.atOp("(") {
		decl := Decl{Kind: DeclFunction, Name: name.text, Type: typ, Range: common.Range{Start: start, End: name.at.End}}
		decl.Params = p.parseParams()

		if p.atOp("{") {
			decl.Body = p.parseBlock()
			decl.Range.End = p.blockEnd(decl.Range.End, decl.Body)
		} else {
			p.expect(";")
		}

		p.file.Decls = append(p.file.Decls, decl)

		return
	}

	// a comma-separated declaration list yields one Decl per declarator;
	// the first carries the type words in its range, the rest start at
	// their own names
	declStart := start

	for {
		decl := Decl{Kind: DeclVariable, Name: name.text, Type: typ,
			Range: common.Range{Start: declStart, End: name.at.End}}

		if p.atOp("=") {
			p.next()

			if value, ok := p.parseExpr(); ok {
				decl.Body = []Stmt{initStmt(name, value)}
				decl.Range.End = value.Range.End
			}
		}

		end := decl.Range.End
		p.file.Decls = append(p.file.Decls, decl)

		if !p.atOp(",") {
			p.file.Decls[len(p.file.Decls)-1].Range.End = p.expectSemi(end)

			return
		}

		p.next()
		declStart = p.peek().at.Start

		name = p.next()
		if name.kind != xIdent {
			p.reportf(name.at, common.SeverityError, "syntax", "expected a name in declaration")
			p.file.Decls[len(p.file.Decls)-1].Range.End = p.expectSemi(end)

			return
		}

		p.record(name)
	}
}

// parseExtern parses `extern [const] <type> <name> [= expr];` and the
// function-signature form `extern <type> <name>(params);` (no body).
func (p *xparser) parseExtern() {
	start := p.next().at.Start // extern
	typ := p.parseTypeWords()

	if p.peek().kind != xIdent {
		p.reportf(p.peek().at, common.SeverityError, "syntax", "expected a name in extern declaration")
		p.syncDecl()

		return
	}

	name := p.next()
	p.record(name)

	decl := Decl{Kind: DeclExtern, Name: name.text, Type: typ, Range: common.Range{Start: start, End: name.at.End}}

	if p.atOp("(") {
		decl.Params = p.parseParams()
	}

	if p.atOp("=") {
		p.next()

		if value, ok := p.parseExpr(); ok {
			decl.Body = []Stmt{initStmt(name, value)}
			decl.Range.End = value.Range.End
		}
	}

	decl.Range.End = p.expectSemi(decl.Range.End)
	p.file.Decls = append(p.file.Decls, decl)
}

// initStmt builds the name = value statement stored as a variable
// declaration initializer.
func initStmt(name xtoken, value Expr) Stmt {
	return Stmt{
		Kind: StmtDecl,
		Exprs: []Expr{{
			Kind:     ExprBinary,
			Value:    "=",
			Children: []Expr{{Kind: ExprIdent, Value: name.text, Range: name.at}, value},
			Range:    common.Range{Start: name.at.Start, End: value.Range.End},
		}},
		Range: common.Range{Start: name.at.Start, End: value.Range.End},
	}
}

// parseInclude parses `include "file.xs";` forms.
func (p *xparser) parseInclude() {
	start := p.next().at.Start // include / includeDuno
	path := p.next()

	decl := Decl{Kind: DeclInclude, Range: common.Range{Start: start, End: path.at.End}}

	switch path.kind {
	case xString:
		decl.Name = strings.Trim(path.text, `"`)
	case xIdent:
		decl.Name = path.text
	default:
		p.reportf(path.at, common.SeverityError, "syntax", "expected a file name after include")
	}

	decl.Range.End = p.expectSemi(decl.Range.End)
	p.file.Decls = append(p.file.Decls, decl)
}

// parseRule parses `rule <name> [inactive] [min-interval N] { ... }`,
// skipping modifiers until the body brace.
func (p *xparser) parseRule() {
	start := p.next().at.Start // rule
	decl := Decl{Kind: DeclRule, Range: common.Range{Start: start, End: start}}

	if p.peek().kind == xIdent {
		name := p.next()
		decl.Name = name.text
		p.record(name)
		decl.Range.End = name.at.End
	}

	for !p.at(xEOF) && !p.atOp("{") {
		p.next()
	}

	if p.atOp("{") {
		decl.Body = p.parseBlock()
		decl.Range.End = p.blockEnd(decl.Range.End, decl.Body)
	} else {
		p.reportf(decl.Range, common.SeverityError, "syntax", "rule without a body")
	}

	p.file.Decls = append(p.file.Decls, decl)
}

// parseEvent parses `event(args)` with an optional trailing body or ';'.
func (p *xparser) parseEvent() {
	start := p.next().at.Start // event
	decl := Decl{Kind: DeclEvent, Range: common.Range{Start: start, End: start}}

	if p.atOp("(") {
		if args, ok := p.parseArgs(nil); ok {
			for _, arg := range args {
				if arg.Kind == ExprIdent {
					decl.Name = arg.Value

					break
				}
			}

			if len(args) > 0 {
				decl.Range.End = args[len(args)-1].Range.End
			}
		}
	}

	if p.atOp("{") {
		decl.Body = p.parseBlock()
		decl.Range.End = p.blockEnd(decl.Range.End, decl.Body)
	} else {
		decl.Range.End = p.expectSemi(decl.Range.End)
	}

	p.file.Decls = append(p.file.Decls, decl)
}

// parseTypeWords consumes one or more type words (const int, float, ...)
// and returns them joined.
func (p *xparser) parseTypeWords() string {
	var words []string

	for p.peek().kind == xIdent && (isTypeWord(p.peek().text) || p.peek().text == "const") {
		words = append(words, p.next().text)
	}

	if len(words) == 0 && p.peek().kind == xIdent {
		words = append(words, p.next().text) // permissive: unknown type word
	}

	return strings.Join(words, " ")
}

// parseParams parses a parenthesized parameter list.
func (p *xparser) parseParams() []Param {
	if !p.at(xOp) || p.peek().text != "(" {
		p.reportf(p.peek().at, common.SeverityError, "syntax", "expected (")
		return nil
	}

	p.next() // (

	var out []Param

	for {
		if p.atOp(")") {
			p.next()

			return out
		}

		if p.at(xEOF) {
			p.reportf(p.peek().at, common.SeverityError, "syntax", "unterminated parameter list")

			return out
		}

		typ := p.parseTypeWords()

		if p.peek().kind != xIdent {
			// malformed parameter (e.g. a stray brace): stop the list and
			// let the caller recover from the junk token
			p.reportf(p.peek().at, common.SeverityError, "syntax", "expected a parameter name")

			return out
		}

		name := p.next()
		p.record(name)
		out = append(out, Param{Name: name.text, Type: typ})

		if p.atOp("=") { // default parameter value
			p.next()
			p.parseExpr() // parsed and discarded: signature-only relevance
		}

		if p.atOp(",") {
			p.next()
		}
	}
}

// parseBlock parses a { ... } block of statements.
func (p *xparser) parseBlock() []Stmt {
	p.next() // {

	var out []Stmt

	for !p.at(xEOF) && (!p.at(xOp) || p.peek().text != "}") {
		before := p.pos
		out = append(out, p.parseStmt())

		if p.pos == before {
			p.next() // guarantee progress on malformed input
		}
	}

	p.expect("}")

	return out
}

// parseStmt parses one statement.
func (p *xparser) parseStmt() Stmt {
	tok := p.peek()

	if tok.kind == xOp && tok.text == "{" {
		body := p.parseBlock()

		return Stmt{Kind: StmtBlock, Body: body, Range: common.Range{Start: tok.at.Start, End: p.blockEnd(tok.at.Start, body)}}
	}

	if tok.kind == xOp && tok.text == ";" { // empty statement
		p.next()

		return Stmt{Kind: StmtExpr, Range: tok.at}
	}

	switch {
	case tok.kind == xIdent:
	case tok.kind == xNumber || tok.kind == xString || tok.kind == xOp && startsExpr(tok.text):
		// expression statement with a non-identifier head
		value, ok := p.parseExpr()
		if !ok {
			p.syncStmt()

			return Stmt{Kind: StmtExpr, Range: tok.at}
		}

		end := p.expectSemi(value.Range.End)

		return Stmt{Kind: StmtExpr, Exprs: []Expr{value}, Range: common.Range{Start: value.Range.Start, End: end}}
	default:
		stmt := Stmt{Kind: StmtExpr, Range: tok.at}
		p.reportf(tok.at, common.SeverityError, "syntax", "unexpected %q", tok.text)
		p.syncStmt()

		return stmt
	}

	switch tok.text {
	case "if":
		return p.parseIf()
	case "while":
		return p.parseWhile()
	case "do":
		return p.parseDo()
	case "for":
		return p.parseFor()
	case "switch":
		return p.parseSwitch()
	case "return":
		return p.parseReturn()
	case "break", "continue":
		p.next()
		end := p.expectSemi(tok.at.End)

		return Stmt{Kind: map[string]string{"break": StmtBreak, "continue": StmtContinue}[tok.text], Range: common.Range{Start: tok.at.Start, End: end}}
	case "condition", "action":
		if next := p.peekAhead(1); next.kind == xOp && (next.text == "{" || next.text == ":") {
			return p.parseRuleSection()
		}
	}

	if isTypeWord(tok.text) && p.peekAhead(1).kind == xIdent {
		return p.parseLocalDecl()
	}

	// expression statement
	value, ok := p.parseExpr()
	if !ok {
		p.syncStmt()

		return Stmt{Kind: StmtExpr, Range: tok.at}
	}

	end := p.expectSemi(value.Range.End)

	return Stmt{Kind: StmtExpr, Exprs: []Expr{value}, Range: common.Range{Start: value.Range.Start, End: end}}
}

// parseIf parses if/else; Body holds the then statement and, when present,
// the else statement.
func (p *xparser) parseIf() Stmt {
	start := p.next().at.Start // if
	stmt := Stmt{Kind: StmtIf, Range: common.Range{Start: start, End: start}}

	if p.atOp("(") {
		p.next()

		if cond, ok := p.parseExpr(); ok {
			stmt.Exprs = []Expr{cond}
			stmt.Range.End = cond.Range.End
		}

		p.expect(")")
	} else {
		p.reportf(p.peek().at, common.SeverityError, "syntax", "expected ( after if")
	}

	stmt.Body = append(stmt.Body, p.parseStmt())

	if p.at(xIdent) && p.peek().text == "else" {
		p.next()
		stmt.Body = append(stmt.Body, p.parseStmt())
	}

	if len(stmt.Body) > 0 {
		stmt.Range.End = stmt.Body[len(stmt.Body)-1].Range.End
	}

	return stmt
}

// parseWhile parses a while loop.
func (p *xparser) parseWhile() Stmt {
	start := p.next().at.Start // while
	stmt := Stmt{Kind: StmtWhile, Range: common.Range{Start: start, End: start}}

	if p.atOp("(") {
		p.next()

		if cond, ok := p.parseExpr(); ok {
			stmt.Exprs = []Expr{cond}
			stmt.Range.End = cond.Range.End
		}

		p.expect(")")
	}

	stmt.Body = []Stmt{p.parseStmt()}
	stmt.Range.End = stmt.Body[0].Range.End

	return stmt
}

// parseDo parses a do/while loop.
func (p *xparser) parseDo() Stmt {
	start := p.next().at.Start // do
	stmt := Stmt{Kind: StmtDo, Range: common.Range{Start: start, End: start}}

	stmt.Body = []Stmt{p.parseStmt()}
	stmt.Range.End = stmt.Body[0].Range.End

	if p.at(xIdent) && p.peek().text == "while" {
		p.next()

		if p.atOp("(") {
			p.next()

			if cond, ok := p.parseExpr(); ok {
				stmt.Exprs = []Expr{cond}
				stmt.Range.End = cond.Range.End
			}

			p.expect(")")
		}
	}

	stmt.Range.End = p.expectSemi(stmt.Range.End)

	return stmt
}

// parseFor parses a for loop; Exprs holds init/cond/step in that order
// (missing parts are skipped).
func (p *xparser) parseFor() Stmt {
	start := p.next().at.Start // for
	stmt := &Stmt{Kind: StmtFor, Range: common.Range{Start: start, End: start}}

	if p.atOp("(") {
		p.next()

		// init: a typed declaration or a plain expression
		if p.at(xIdent) && (isTypeWord(p.peek().text) || p.peek().text == "const") {
			init := p.parseLocalDecl() // consumes through ';'
			stmt.Exprs = append(stmt.Exprs, init.Exprs...)
			stmt.Range.End = init.Range.End
		} else {
			stmt.Range.End = p.forPart(stmt, 0)
			p.expect(";")
		}

		// condition and step
		stmt.Range.End = p.forPart(stmt, 1)
		p.expect(";")
		stmt.Range.End = p.forPart(stmt, 2)

		p.expect(")")
	}

	stmt.Body = []Stmt{p.parseStmt()}
	stmt.Range.End = stmt.Body[0].Range.End

	return *stmt
}

// forPart parses one optional for-clause part (0=init when not a declaration,
// 1=condition, 2=step); missing parts are skipped.
func (p *xparser) forPart(stmt *Stmt, part int) common.Pos {
	end := stmt.Range.End

	if p.atOp(";") || (part == 2 && p.atOp(")")) {
		return end
	}

	if e, ok := p.parseExpr(); ok && e.Range.End.After(end) {
		end = e.Range.End
		stmt.Exprs = append(stmt.Exprs, e)
	}

	return end
}

// parseSwitch parses a switch with case/default sub-statements.
func (p *xparser) parseSwitch() Stmt {
	start := p.next().at.Start // switch
	stmt := Stmt{Kind: StmtSwitch, Range: common.Range{Start: start, End: start}}

	if p.atOp("(") {
		p.next()

		if subject, ok := p.parseExpr(); ok {
			stmt.Exprs = []Expr{subject}
			stmt.Range.End = subject.Range.End
		}

		p.expect(")")
	}

	if !p.atOp("{") {
		p.reportf(p.peek().at, common.SeverityError, "syntax", "expected { after switch")

		return stmt
	}

	p.next() // {

	for !p.at(xEOF) && (!p.at(xOp) || p.peek().text != "}") {
		before := p.pos

		if p.at(xIdent) && (p.peek().text == "case" || p.peek().text == "default") {
			stmt.Body = append(stmt.Body, p.parseCase())

			continue
		}

		// statements before the first case label: tolerated permissively
		stmt.Body = append(stmt.Body, p.parseStmt())

		if p.pos == before {
			p.next()
		}
	}

	p.expect("}")

	if len(stmt.Body) > 0 {
		stmt.Range.End = stmt.Body[len(stmt.Body)-1].Range.End
	}

	return stmt
}

// parseCase parses one case/default label with its statements.
func (p *xparser) parseCase() Stmt {
	kw := p.next() // case / default
	stmt := Stmt{Kind: StmtCase, Range: common.Range{Start: kw.at.Start, End: kw.at.End}}

	if kw.text == "case" {
		if label, ok := p.parseExpr(); ok {
			stmt.Exprs = []Expr{label}
			stmt.Range.End = label.Range.End
		}
	}

	p.expect(":")

	for !p.at(xEOF) && (!p.at(xOp) || p.peek().text != "}") {
		if p.at(xIdent) && (p.peek().text == "case" || p.peek().text == "default") {
			break
		}

		before := p.pos
		stmt.Body = append(stmt.Body, p.parseStmt())

		if p.pos == before {
			p.next()
		}
	}

	if len(stmt.Body) > 0 {
		stmt.Range.End = stmt.Body[len(stmt.Body)-1].Range.End
	}

	return stmt
}

// parseReturn parses return with an optional value.
func (p *xparser) parseReturn() Stmt {
	kw := p.next() // return
	stmt := Stmt{Kind: StmtReturn, Range: common.Range{Start: kw.at.Start, End: kw.at.End}}

	if !p.atOp(";") {
		if value, ok := p.parseExpr(); ok {
			stmt.Exprs = []Expr{value}
			stmt.Range.End = value.Range.End
		}
	}

	stmt.Range.End = p.expectSemi(stmt.Range.End)

	return stmt
}

// parseRuleSection parses the permissive `condition { }` / `action { }`
// statement forms used inside rules.
func (p *xparser) parseRuleSection() Stmt {
	kw := p.next() // condition / action
	kind := StmtCondition
	if kw.text == "action" {
		kind = StmtAction
	}

	if p.atOp(":") {
		p.next()
	}

	stmt := Stmt{Kind: kind, Range: common.Range{Start: kw.at.Start, End: kw.at.End}}

	if p.atOp("{") {
		body := p.parseBlock()
		stmt.Body = body
		stmt.Range.End = p.blockEnd(kw.at.Start, body)
	}

	return stmt
}

// parseLocalDecl parses a typed declaration inside a body (`int x = 1;`).
// Each declarator becomes one assignment-shaped expression in Exprs.
func (p *xparser) parseLocalDecl() Stmt {
	start := p.peek().at.Start
	end := start

	p.parseTypeWords() // the type words are consumed; the range carries them

	var exprs []Expr

	for {
		name := p.next()
		if name.kind != xIdent {
			p.reportf(name.at, common.SeverityError, "syntax", "expected a name in declaration")
			break
		}

		p.record(name)
		end = name.at.End
		item := Expr{Kind: ExprIdent, Value: name.text, Range: name.at}

		if p.atOp("=") {
			p.next()

			if value, ok := p.parseExpr(); ok {
				end = value.Range.End
				item = Expr{Kind: ExprBinary, Value: "=", Children: []Expr{item, value},
					Range: common.Range{Start: name.at.Start, End: value.Range.End}}
			}
		}

		exprs = append(exprs, item)

		if p.atOp(",") {
			p.next()

			continue
		}

		break
	}

	end = p.expectSemi(end)

	return Stmt{Kind: StmtDecl, Exprs: exprs, Range: common.Range{Start: start, End: end}}
}

// parseExpr parses one expression with precedence climbing.
func (p *xparser) parseExpr() (Expr, bool) {
	return p.parseBinary(1)
}

// parseBinary parses expressions with binding power >= minPrec.
func (p *xparser) parseBinary(minPrec int) (Expr, bool) {
	lhs, ok := p.parseUnary()
	if !ok {
		return Expr{}, false
	}

	for {
		op := p.peek()
		if op.kind != xOp {
			return lhs, true
		}

		prec := binaryPrec(op.text)
		if prec == 0 || prec < minPrec {
			return lhs, true
		}

		p.next()

		if isAssignOp(op.text) {
			rhs, ok := p.parseBinary(prec) // right-associative
			if !ok {
				return lhs, true
			}

			lhs = Expr{Kind: ExprBinary, Value: op.text, Children: []Expr{lhs, rhs},
				Range: common.Range{Start: lhs.Range.Start, End: rhs.Range.End}}

			continue
		}

		rhs, ok := p.parseBinary(prec + 1)
		if !ok {
			return lhs, true
		}

		lhs = Expr{Kind: ExprBinary, Value: op.text, Children: []Expr{lhs, rhs},
			Range: common.Range{Start: lhs.Range.Start, End: rhs.Range.End}}
	}
}

// parseUnary parses a prefix expression or a postfix chain.
func (p *xparser) parseUnary() (Expr, bool) {
	tok := p.peek()

	if tok.kind == xOp && unaryOps[tok.text] {
		p.next()

		if operand, ok := p.parseUnary(); ok {
			return Expr{Kind: ExprUnary, Value: tok.text, Children: []Expr{operand},
				Range: common.Range{Start: tok.at.Start, End: operand.Range.End}}, true
		}

		return Expr{Kind: ExprUnary, Value: tok.text, Range: tok.at}, true
	}

	return p.parsePostfix()
}

// parsePostfix parses a primary expression with call/member/index suffixes.
func (p *xparser) parsePostfix() (Expr, bool) {
	operand, ok := p.parsePrimary()
	if !ok {
		return Expr{}, false
	}

	for {
		op := p.peek()
		if op.kind != xOp {
			return operand, true
		}

		switch op.text {
		case "(":
			// Only this path creates call contexts (the contract's
			// Kind=call rule): vector literals, param lists and grouping
			// parens never record.
			rec := p.openCall(operand, op.at.Start)
			args, _ := p.parseArgs(rec)
			end := operand.Range.End
			if len(args) > 0 && args[len(args)-1].Range.End.After(end) {
				end = args[len(args)-1].Range.End
			}

			operand = Expr{Kind: ExprCall, Callee: identName(operand), Children: args,
				Range: common.Range{Start: operand.Range.Start, End: end}}
		case ".":
			p.next()

			member := p.next()
			end := member.at.End
			if !end.After(operand.Range.End) {
				end = operand.Range.End
			}

			operand = Expr{Kind: ExprBinary, Value: ".", Children: []Expr{operand, {Kind: ExprIdent, Value: member.text, Range: member.at}},
				Range: common.Range{Start: operand.Range.Start, End: end}}
		case "[":
			p.next()

			index, has := p.parseExpr()
			end := index.Range.End
			if !has || !end.After(operand.Range.End) {
				end = operand.Range.End
			}

			p.expect("]")
			operand = Expr{Kind: ExprBinary, Value: "[]", Children: []Expr{operand, index},
				Range: common.Range{Start: operand.Range.Start, End: end}}
		case "++", "--":
			p.next()
			operand = Expr{Kind: ExprUnary, Value: op.text + "post", Children: []Expr{operand},
				Range: common.Range{Start: operand.Range.Start, End: op.at.End}}
		default:
			return operand, true
		}
	}
}

// parsePrimary parses a literal, identifier, parenthesized expression or a
// vector literal.
func (p *xparser) parsePrimary() (Expr, bool) {
	tok := p.next()

	switch tok.kind {
	case xNumber, xString:
		return Expr{Kind: ExprLiteral, Value: tok.text, Range: tok.at}, true
	case xIdent:
		p.record(tok)

		return Expr{Kind: ExprIdent, Value: tok.text, Range: tok.at}, true
	case xOp:
		switch tok.text {
		case "(":
			return p.parseParenExpr(tok)
		case "{":
			// bare block as expression: tolerated, skip to matching }
			p.skipBlock(tok)

			return Expr{Kind: ExprLiteral, Value: "{}", Range: tok.at}, true
		}
	}

	p.reportf(tok.at, common.SeverityError, "syntax", "unexpected %q in expression", tok.text)

	return Expr{}, false
}

// parseParenExpr parses a parenthesized expression or a (x, y, z) vector
// literal; the opening ( is already consumed.
func (p *xparser) parseParenExpr(open xtoken) (Expr, bool) {
	first, ok := p.parseExpr()
	if !ok {
		p.expect(")")

		return Expr{}, false
	}

	if !p.atOp(",") {
		p.expect(")")

		return first, true
	}

	operands := []Expr{first}

	for p.atOp(",") {
		p.next()

		if e, ok := p.parseExpr(); ok {
			operands = append(operands, e)
		}
	}

	closeTok := p.expect(")")

	if len(operands) == 3 {
		return Expr{Kind: ExprVector, Children: operands,
			Range: common.Range{Start: open.at.Start, End: closeTok}}, true
	}

	p.reportf(common.Range{Start: open.at.Start, End: closeTok}, common.SeverityWarning, "syntax",
		"expected 3 components in a vector literal, got %d", len(operands))

	return first, true
}

// parseArgs parses a call argument list: ( expr, ... ). When rec is
// non-nil, every top-level comma consumed by this invocation appends its
// token end to the record and termination pins argEnd (the ) end, or the
// eofPos frontier for an unterminated list); nested calls record into
// their own records via recursion.
func (p *xparser) parseArgs(rec *callRec) ([]Expr, bool) {
	if !p.at(xOp) || p.peek().text != "(" {
		return nil, false
	}

	p.next() // (

	var out []Expr

	for {
		if p.atOp(")") {
			closer := p.next()

			if rec != nil {
				rec.argEnd = closer.at.End
			}

			return out, true
		}

		if p.at(xEOF) {
			p.reportf(p.peek().at, common.SeverityError, "syntax", "unterminated argument list")

			if rec != nil {
				rec.argEnd = eofPos
			}

			return out, false
		}

		if arg, ok := p.parseExpr(); ok {
			out = append(out, arg)
		} else if p.at(xOp) && p.peek().text != "," && p.peek().text != ")" {
			p.next() // skip the bad token and keep going

			continue
		}

		if p.atOp(",") {
			comma := p.next()

			if rec != nil {
				rec.commas = append(rec.commas, comma.at.End)
			}
		} else if !p.atOp(")") {
			p.reportf(p.peek().at, common.SeverityError, "syntax", "expected , or ) in argument list")
			p.next()

			continue
		}
	}
}

// openCall starts one call-context record for a postfix call on operand
// (lparen is the "(" token start); parseArgs fills in the rest.
func (p *xparser) openCall(operand Expr, lparen common.Pos) *callRec {
	rec := &callRec{
		callee:   identName(operand),
		calleeAt: operand.Range.Start,
		lparen:   lparen,
	}
	p.calls = append(p.calls, rec)

	return rec
}

// skipBlock consumes a { ... } region (used for permissive recovery).
func (p *xparser) skipBlock(open xtoken) {
	depth := 1

	for depth > 0 && !p.at(xEOF) {
		tok := p.next()
		switch {
		case tok.kind == xOp && tok.text == "{":
			depth++
		case tok.kind == xOp && tok.text == "}":
			depth--
		}
	}
}

// syncStmt resynchronizes after a bad statement: to just after the next ';'
// or up to the next '}' (leaving it for the caller).
func (p *xparser) syncStmt() {
	for !p.at(xEOF) {
		tok := p.next()
		if tok.kind != xOp {
			continue
		}

		if tok.text == ";" {
			return
		}

		if tok.text == "{" { // unbalanced block: skip it whole
			p.skipBlock(tok)

			return
		}

		if tok.text == "}" {
			p.pos--

			return
		}
	}
}

// syncDecl resynchronizes after a bad top-level declaration.
func (p *xparser) syncDecl() {
	for !p.at(xEOF) {
		tok := p.next()
		if tok.kind != xOp {
			continue
		}

		if tok.text == ";" {
			return
		}

		if tok.text == "{" {
			p.skipBlock(tok)

			return
		}

		if tok.text == "}" {
			return
		}
	}
}

// atOp reports whether the current token is the given operator.
func (p *xparser) atOp(text string) bool {
	return p.at(xOp) && p.peek().text == text
}

// at reports whether the current token has the given kind.
func (p *xparser) at(kind xtokKind) bool {
	return p.peek().kind == kind
}

// peek returns the current token without consuming it.
func (p *xparser) peek() xtoken {
	if p.pos >= len(p.toks) {
		return p.toks[len(p.toks)-1] // EOF sentinel
	}

	return p.toks[p.pos]
}

// peekAhead looks n tokens ahead without consuming.
func (p *xparser) peekAhead(n int) xtoken {
	if p.pos+n >= len(p.toks) {
		return p.toks[len(p.toks)-1]
	}

	return p.toks[p.pos+n]
}

// next consumes and returns the current token.
func (p *xparser) next() xtoken {
	tok := p.peek()
	if p.pos < len(p.toks)-1 {
		p.pos++
	}

	return tok
}

// expect consumes the given punctuation or reports a diagnostic.
func (p *xparser) expect(text string) common.Pos {
	if p.at(xOp) && p.peek().text == text {
		return p.next().at.End
	}

	p.reportf(p.peek().at, common.SeverityError, "syntax", "expected %q", text)

	return p.peek().at.End
}

// expectSemi consumes a ';' if present and returns the end position.
func (p *xparser) expectSemi(fallback common.Pos) common.Pos {
	if p.atOp(";") {
		return p.next().at.End
	}

	if !p.at(xEOF) && !p.atOp("}") {
		p.reportf(p.peek().at, common.SeverityError, "syntax", "expected ;")
	}

	return fallback
}

// record remembers an identifier occurrence for SymbolAt.
func (p *xparser) record(name xtoken) {
	p.syms = append(p.syms, symbol{name: name.text, at: name.at})
}

// reportf appends a syntax diagnostic.
func (p *xparser) reportf(r common.Range, severity int, code string, format string, args ...any) {
	p.diags = append(p.diags, common.Diagnostic{
		Range:    r,
		Severity: severity,
		Message:  fmt.Sprintf(format, args...),
		Code:     code,
	})
}

// blockEnd computes the end of a block that began at start.
func (p *xparser) blockEnd(start common.Pos, body []Stmt) common.Pos {
	end := start

	for i := range body {
		if body[i].Range.End.After(end) {
			end = body[i].Range.End
		}
	}

	return end
}

// identName returns the callee spelling of an expression.
func identName(e Expr) string {
	if e.Kind == ExprIdent {
		return e.Value
	}

	return ""
}

// startsExpr reports whether an operator token can begin an expression.
func startsExpr(op string) bool {
	switch op {
	case "(", "-", "+", "!", "~", "++", "--":
		return true
	}

	return false
}

// isTypeWord reports whether a word is an XS type keyword.
func isTypeWord(word string) bool {
	switch word {
	case "int", "float", "bool", "string", "vector", "void":
		return true
	}

	return false
}

// binaryPrec returns the binding power of a binary operator; 0 when the
// token is not one. Assignments bind loosest and associate right.
func binaryPrec(op string) int {
	switch op {
	case "=", "+=", "-=", "*=", "/=", "%=":
		return 1
	case "||":
		return 2
	case "&&":
		return 3
	case "|":
		return 4
	case "^":
		return 5
	case "&":
		return 6
	case "==", "!=":
		return 7
	case "<", ">", "<=", ">=":
		return 8
	case "<<", ">>":
		return 9
	case "+", "-":
		return 10
	case "*", "/", "%":
		return 11
	}

	return 0
}

// unaryOps are the prefix operators.
var unaryOps = map[string]bool{
	"!": true, "-": true, "+": true, "~": true, "++": true, "--": true,
}

// isAssignOp reports whether an operator is an assignment form.
func isAssignOp(op string) bool {
	switch op {
	case "=", "+=", "-=", "*=", "/=", "%=":
		return true
	}

	return false
}

// xtokKind enumerates the lexer token kinds.
type xtokKind int

// Token kinds.
const (
	xIdent xtokKind = iota
	xNumber
	xString
	xOp
	xEOF
)

// xtoken is one lexical token with its source range.
type xtoken struct {
	kind xtokKind
	text string
	at   common.Range
}

// multiCharOps are the two- and three-character operators, longest first.
var multiCharOps = []string{
	"<<", ">>", "<=", ">=", "==", "!=", "&&", "||",
	"+=", "-=", "*=", "/=", "%=", "++", "--",
}

// singleOps are the one-character operators and punctuation.
const singleOps = "+-*/%=<>!&|^~?:;,(){}[]."

// scan tokenizes the whole source into p.toks with an EOF sentinel and
// collects the non-code spans (strings, comments) into p.noncode.
func (p *xparser) scan(source string) {
	s := &xscanner{src: []byte(source)}

	for {
		tok := s.next()
		p.toks = append(p.toks, tok)
		if tok.kind == xEOF {
			p.noncode = s.noncode

			return
		}
	}
}

// xscanner walks the source producing tokens.
type xscanner struct {
	src       []byte
	pos       int
	line      uint32
	lineStart int
	noncode   []common.Range
}

// posAt builds the position of byte offset i.
func (s *xscanner) posAt(i int) common.Pos {
	return common.Pos{Line: s.line, Column: uint32(i - s.lineStart), Offset: i}
}

// span builds the range [start, s.pos).
func (s *xscanner) span(start int) common.Range {
	return common.Range{Start: s.posAt(start), End: s.posAt(s.pos)}
}

// token builds a token for [start, s.pos).
func (s *xscanner) token(kind xtokKind, start int) xtoken {
	return xtoken{kind: kind, text: string(s.src[start:s.pos]), at: s.span(start)}
}

// next returns the next token, skipping whitespace and comments.
func (s *xscanner) next() xtoken {
	for {
		s.skipSpace()

		if s.pos >= len(s.src) {
			at := common.Range{Start: s.posAt(s.pos), End: s.posAt(s.pos)}
			return xtoken{kind: xEOF, at: at}
		}

		start := s.pos
		c := s.src[s.pos]

		if c == '/' && s.pos+1 < len(s.src) {
			if s.src[s.pos+1] == '/' {
				s.skipLineComment()
				s.noncode = append(s.noncode, s.span(start))

				continue
			}

			if s.src[s.pos+1] == '*' {
				s.skipBlockComment()
				s.noncode = append(s.noncode, s.span(start))

				continue
			}
		}

		switch {
		case isIdentStart(c):
			for s.pos < len(s.src) && isIdentPart(s.src[s.pos]) {
				s.pos++
			}

			return s.token(xIdent, start)
		case c >= '0' && c <= '9':
			s.scanNumber()

			return s.token(xNumber, start)
		case c == '"':
			s.scanString()
			tok := s.token(xString, start)
			s.noncode = append(s.noncode, tok.at)

			return tok
		default:
			if tok, ok := s.scanOp(); ok {
				return tok
			}

			s.pos++ // unknown byte: skip it (recovery)

			continue
		}
	}
}

// skipSpace consumes spaces, tabs and newlines, tracking the line counter.
func (s *xscanner) skipSpace() {
	for s.pos < len(s.src) {
		switch s.src[s.pos] {
		case ' ', '\t', '\r':
			s.pos++
		case '\n':
			s.pos++
			s.line++
			s.lineStart = s.pos
		default:
			return
		}
	}
}

// skipLineComment consumes a // comment.
func (s *xscanner) skipLineComment() {
	for s.pos < len(s.src) && s.src[s.pos] != '\n' {
		s.pos++
	}
}

// skipBlockComment consumes a /* */ comment.
func (s *xscanner) skipBlockComment() {
	s.pos += 2

	for s.pos < len(s.src) {
		if s.src[s.pos] == '\n' {
			s.line++
			s.lineStart = s.pos + 1
		}

		if s.src[s.pos] == '*' && s.pos+1 < len(s.src) && s.src[s.pos+1] == '/' {
			s.pos += 2
			return
		}

		s.pos++
	}
}

// scanNumber consumes a decimal int, a float or a hex literal.
func (s *xscanner) scanNumber() {
	if s.src[s.pos] == '0' && s.pos+1 < len(s.src) &&
		(s.src[s.pos+1] == 'x' || s.src[s.pos+1] == 'X') {
		s.pos += 2

		for s.pos < len(s.src) && isHexDigit(s.src[s.pos]) {
			s.pos++
		}

		return
	}

	for s.pos < len(s.src) && s.src[s.pos] >= '0' && s.src[s.pos] <= '9' {
		s.pos++
	}

	if s.pos < len(s.src) && s.src[s.pos] == '.' && s.pos+1 < len(s.src) &&
		s.src[s.pos+1] >= '0' && s.src[s.pos+1] <= '9' {
		s.pos++

		for s.pos < len(s.src) && s.src[s.pos] >= '0' && s.src[s.pos] <= '9' {
			s.pos++
		}
	}
}

// scanString consumes a double-quoted string with escapes.
func (s *xscanner) scanString() {
	s.pos++ // opening quote

	for s.pos < len(s.src) {
		if s.src[s.pos] == '\\' && s.pos+1 < len(s.src) {
			s.pos += 2

			continue
		}

		if s.src[s.pos] == '"' {
			s.pos++

			return
		}

		if s.src[s.pos] == '\n' {
			return // unterminated: stop at end of line
		}

		s.pos++
	}
}

// scanOp consumes a multi- or single-character operator.
func (s *xscanner) scanOp() (xtoken, bool) {
	rest := s.src[s.pos:]

	for _, op := range multiCharOps {
		if len(rest) >= len(op) && string(rest[:len(op)]) == op {
			start := s.pos
			s.pos += len(op)

			return s.token(xOp, start), true
		}
	}

	if s.indexByte(s.src[s.pos]) {
		start := s.pos
		s.pos++

		return s.token(xOp, start), true
	}

	return xtoken{}, false
}

// indexByte reports whether c is a single-character operator.
func (s *xscanner) indexByte(c byte) bool {
	for i := 0; i < len(singleOps); i++ {
		if singleOps[i] == c {
			return true
		}
	}

	return false
}

// isIdentStart reports whether c can start an identifier.
func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isIdentPart reports whether c can continue an identifier.
func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// isHexDigit reports whether c is a hex digit.
func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
