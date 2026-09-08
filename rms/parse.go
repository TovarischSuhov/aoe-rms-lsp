package rms

import (
	"fmt"
	"slices"
	"strings"

	"aoe2-lsp/common"
)

// Parse parses an RMS source into an AST with error recovery: every
// problem becomes a Diagnostic and parsing resynchronizes to the next
// statement or section. The returned file is never nil; diags are sorted
// by position.
func Parse(source string, name string) (RmsFile, []common.Diagnostic) {
	p := newParser(name)
	p.run(source)

	slices.SortStableFunc(p.diags, func(a, b common.Diagnostic) int {
		if a.Range.Start.Line != b.Range.Start.Line {
			return int(a.Range.Start.Line) - int(b.Range.Start.Line)
		}

		return int(a.Range.Start.Column) - int(b.Range.Start.Column)
	})

	return p.file, p.diags
}

// node is the internal statement node; materialized into Statement values
// when a section is closed, so that appends never invalidate pointers.
type node struct {
	kind     string
	name     string
	args     []Expr
	attrs    []Attribute
	children []*node
	start    common.Pos
	end      common.Pos
}

// sect is the internal section under construction.
type sect struct {
	name  string
	nodes []*node
	start common.Pos
}

// parser holds the incremental state of one Parse run.
type parser struct {
	file   RmsFile
	diags  []common.Diagnostic
	lines  []string
	starts []int // byte offset of each line start

	cur         *sect    // section being filled
	scopes      []*node  // open if/elseif/else/start_random/percent_chance blocks
	braceScopes []string // structural nesting inside an attribute block
	owner       *node    // command owning attributes of an open { } block
	lastCmd     *node    // last command statement started (for "{" on the next line)
	openPos     common.Pos

	inXs    bool
	xsStart int
}

// newParser creates a parser for the named file.
func newParser(name string) *parser {
	return &parser{file: RmsFile{Name: name}}
}

// run drives the line-oriented parse.
func (p *parser) run(source string) {
	p.lines = strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	p.starts = make([]int, len(p.lines))

	for i := 1; i < len(p.lines); i++ {
		p.starts[i] = p.starts[i-1] + len(p.lines[i-1]) + 1
	}

	visible, comments := blankComments(p.lines, p.starts)
	p.file.comments = comments
	p.cur = &sect{name: "global", start: common.Pos{}}

	for i := range p.lines {
		line := visible[i]
		trimmed := strings.TrimSpace(line)

		if p.inXs {
			if !xsTerminator(trimmed) {
				continue
			}

			p.endXsBlock(i)
		}

		if trimmed == "" {
			continue
		}

		if name, ok := sectionHeader(trimmed); ok {
			p.closeSection(p.pos(i, 0))
			p.recordSectionWord(name, line, i)
			p.recordExcluded(i)

			if strings.HasPrefix(trimmed, "</") {
				// a closing tag ends the section: following statements
				// are global (rms_grammar), the section is not reopened
				p.cur = &sect{name: "global", start: p.pos(i, 0)}
			} else {
				p.cur = &sect{name: name, start: p.pos(i, 0)}
			}

			continue
		}

		if isDirectiveLine(trimmed) {
			if isExcludedDirective(trimmed) {
				// the whole #include/#includeXS line — path argument
				// included — never owns a statement
				p.recordExcluded(i)
			}

			p.directive(line, i)
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			p.recordExcluded(i) // plain #-comment line

			continue
		}

		p.statementLine(line, i)
	}

	p.closeAll()
}

// recordExcluded adds the whole physical line to the excluded index.
func (p *parser) recordExcluded(idx int) {
	p.file.excluded = append(p.file.excluded,
		common.Range{Start: p.pos(idx, 0), End: p.pos(idx, len(p.lines[idx]))})
}

// isExcludedDirective reports whether the trimmed line is an
// #include/#includeXS directive (the line classes that materialize no
// statement; #const/#define/#include_drs do and stay owned).
func isExcludedDirective(trimmed string) bool {
	word := strings.Fields(trimmed)[0]

	return word == "#include" || word == "#includeXS"
}

// closeAll finalizes the trailing section, XS block and open constructs.
func (p *parser) closeAll() {
	if p.inXs {
		p.endXsBlock(len(p.lines))
	}

	p.syncConstructs("unexpected end of file")

	last := len(p.lines) - 1
	end := common.Pos{Line: uint32(last), Column: uint32(len(p.lines[last])), Offset: p.starts[last] + len(p.lines[last])}

	p.closeSection(end)
}

// statementLine parses one statement, attribute or block-punctuation line.
func (p *parser) statementLine(line string, idx int) {
	lex := newLexer(line, p.starts[idx], idx)
	first := lex.next()

	switch {
	case first.kind == tokLBrace:
		p.openBrace(first)
		return
	case first.kind == tokRBrace:
		p.closeBrace(first)
		return
	case first.kind != tokWord:
		p.reportf(first.at, common.SeverityError, "syntax", "unexpected %q", first.text)
		return
	}

	rest := lex.rest()

	for _, tok := range rest {
		if tok.kind == tokString {
			p.file.strings = append(p.file.strings, tok.at)
		}
	}

	if structuralWords[first.text] {
		p.structural(first, rest)

		return
	}

	if p.owner != nil {
		p.attributeLine(p.owner, first, rest)

		return
	}

	p.commandLine(first, rest)
}

// commandLine builds a command statement. A trailing "{" opens its
// multi-line attribute block; "{ attrs... }" on the same line is parsed
// inline.
func (p *parser) commandLine(first token, rest []token) {
	p.recordWord(first.text, first.at)

	stmt := &node{kind: KindCommand, name: first.text, start: first.at.Start, end: first.at.End}

	args, block, open := splitArgsBlock(rest)
	stmt.args = p.arguments(args, stmt)

	p.add(stmt)

	if open == nil {
		return
	}

	if len(block) == 0 {
		// the block continues on the following lines
		p.owner = stmt
		p.openPos = open.at.Start

		return
	}

	p.inlineAttributes(stmt, block)
}

// splitArgsBlock splits the tokens after a command name into argument
// tokens and (optionally) the tokens of an inline block. open is the "{"
// token when a block starts on this line.
func splitArgsBlock(rest []token) (args []token, block []token, open *token) {
	for i := range rest {
		if rest[i].kind != tokLBrace {
			continue
		}

		brace := rest[i]
		block = rest[i+1:]

		if n := len(block); n > 0 && block[n-1].kind == tokRBrace {
			block = block[:n-1]
		}

		return rest[:i], block, &brace
	}

	return rest, nil, nil
}

// inlineAttributes parses the attributes of a one-line block. Attribute
// names are lower_snake words not followed by "("; everything else
// (ALL-CAPS constants, numbers, expressions) is a value.
func (p *parser) inlineAttributes(stmt *node, toks []token) {
	i := 0

	for i < len(toks) {
		if !startsAttribute(toks, i) {
			p.reportf(toks[i].at, common.SeverityError, "syntax", "unexpected %q", toks[i].text)
			i++

			continue
		}

		j := i + 1
		for j < len(toks) && !startsAttribute(toks, j) {
			j++
		}

		attr := p.buildAttribute(toks[i], toks[i+1:j])
		stmt.attrs = append(stmt.attrs, attr)

		if attr.Range.End.After(stmt.end) {
			stmt.end = attr.Range.End
		}

		i = j
	}
}

// startsAttribute reports whether token i begins a new attribute inside a
// one-line block.
func startsAttribute(toks []token, i int) bool {
	tok := toks[i]
	if tok.kind != tokWord || isConstName(tok.text) {
		return false
	}

	return i+1 >= len(toks) || toks[i+1].kind != tokLParen
}

// attributeLine attaches one attribute line to the owning command.
func (p *parser) attributeLine(owner *node, first token, rest []token) {
	attr := p.buildAttribute(first, rest)

	owner.attrs = append(owner.attrs, attr)

	if attr.Range.End.After(owner.end) {
		owner.end = attr.Range.End
	}
}

// buildAttribute parses one attribute: a name and its (first) value
// expression.
func (p *parser) buildAttribute(first token, rest []token) Attribute {
	p.recordWord(first.text, first.at)

	attr := Attribute{
		Name:   first.text,
		Range:  common.Range{Start: first.at.Start, End: first.at.End},
		nameAt: first.at,
	}

	values := p.expressions(rest)
	if len(values) > 0 {
		attr.Value = values[0]
		attr.Range.End = values[len(values)-1].Range.End
	}

	return attr
}

// structural handles if/elseif/else/endif and start_random/end_random/
// percent_chance lines. Inside an attribute block the nesting is tracked
// without materializing nodes: attributes keep attaching to the owner.
func (p *parser) structural(first token, rest []token) {
	word := first.text

	if p.owner != nil {
		p.structuralInBrace(first)
		return
	}

	switch word {
	case "endif":
		p.closeScopes(first, isConditionalName)
		return
	case "end_random":
		p.closeScopes(first, func(n string) bool { return n == "start_random" })
		return
	case "elseif", "else":
		p.closeScopes(first, isConditionalName)
	case "percent_chance":
		// a sibling percent_chance branch ends silently; the first one has
		// nothing to close
		if n := len(p.scopes); n > 0 && p.scopes[n-1].name == "percent_chance" {
			p.finalizeScope(p.scopes[n-1])
			p.scopes = p.scopes[:n-1]
		}
	}

	kind := KindConditional
	if word == "start_random" || word == "percent_chance" {
		kind = KindRandom
	}

	stmt := &node{kind: kind, name: word, start: first.at.Start, end: first.at.End}
	stmt.args = p.arguments(rest, stmt)

	p.add(stmt)
	p.scopes = append(p.scopes, stmt)
}

// structuralInBrace tracks structural keywords inside an attribute block.
func (p *parser) structuralInBrace(first token) {
	switch first.text {
	case "endif", "end_random":
		if len(p.braceScopes) > 0 {
			p.braceScopes = p.braceScopes[:len(p.braceScopes)-1]
			return
		}

		p.reportf(first.at, common.SeverityError, "syntax", `"%s" without a matching opening block`, first.text)
	case "elseif", "else", "percent_chance":
		if len(p.braceScopes) > 0 {
			p.braceScopes = p.braceScopes[:len(p.braceScopes)-1]
		}

		p.braceScopes = append(p.braceScopes, first.text)
	case "if", "start_random":
		p.braceScopes = append(p.braceScopes, first.text)
	}
}

// closeScopes closes scopes until one matching stops is found; inner
// unterminated scopes are closed alongside (percent_chance closes
// implicitly and never warns).
func (p *parser) closeScopes(closer token, stops func(string) bool) {
	for i, open := range slices.Backward(p.scopes) {
		// A scope the closer does not target is being closed implicitly:
		// warn, except percent_chance which closes silently by design.
		if !stops(open.name) && open.name != "percent_chance" {
			p.reportf(closer.at, common.SeverityWarning, "syntax",
				`"%s" closes an unterminated "%s" block`, closer.text, open.name)
		}

		p.finalizeScope(open)
		p.scopes = p.scopes[:i]

		if stops(open.name) {
			return
		}
	}

	p.reportf(closer.at, common.SeverityError, "syntax", `"%s" without a matching opening block`, closer.text)
}

// isConditionalName reports whether a scope name is part of an if group.
func isConditionalName(name string) bool {
	return name == "if" || name == "elseif" || name == "else"
}

// openBrace starts an attribute block for the last command.
func (p *parser) openBrace(brace token) {
	if p.owner != nil {
		p.reportf(brace.at, common.SeverityError, "syntax", `unexpected "{" inside a block`)

		return
	}

	if p.lastCmd == nil {
		p.reportf(brace.at, common.SeverityError, "syntax", `unexpected "{" without a command`)

		return
	}

	p.owner = p.lastCmd
	p.openPos = brace.at.Start
}

// closeBrace ends the attribute block of the owning command.
func (p *parser) closeBrace(brace token) {
	if p.owner == nil {
		p.reportf(brace.at, common.SeverityError, "syntax", `unexpected "}"`)

		return
	}

	if brace.at.End.After(p.owner.end) {
		p.owner.end = brace.at.End
	}

	p.owner = nil
	p.braceScopes = nil
}

// directive handles one #-directive line.
func (p *parser) directive(line string, idx int) {
	lex := newLexer(line, p.starts[idx], idx)
	directive := lex.next()

	args := lex.rest()

	switch directive.text {
	case "#include":
		inc, ok := p.includeArg(directive, line, idx)
		if !ok {
			return
		}

		p.file.Includes = append(p.file.Includes, inc)
	case "#includeXS":
		if inc, ok := p.includeArg(directive, line, idx); ok {
			p.file.XsIncludes = append(p.file.XsIncludes, inc)
		}

		p.inXs = true
		p.xsStart = idx + 1
	default:
		// #const / #define / #include_drs stay visible as statements.
		p.commandLine(directive, args)
	}
}

// includeArg extracts the path argument of an include directive with its
// source range. ok=false reports the missing-argument syntax error (no
// Include is created in that case). Quoted arguments keep the quotes in
// the range; bare arguments span to the end of the trimmed line.
func (p *parser) includeArg(directive token, line string, idx int) (inc Include, ok bool) {
	argLex := newLexer(line, p.starts[idx], idx)
	argLex.next() // the directive token itself
	arg := argLex.next()

	switch arg.kind {
	case tokEOF:
		// A bare #includeXS is legal (inline mode only); #include requires
		// a file name.
		if directive.text == "#include" {
			p.reportf(directive.at, common.SeverityError, "syntax", "%s needs a file name", directive.text)
		}

		return Include{}, false
	case tokString:
		return Include{Path: strings.Trim(arg.text, `"`), Range: arg.at}, true
	default:
		rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), directive.text))
		end := len(strings.TrimRight(line, " \t\r"))
		endPos := common.Pos{Line: uint32(idx), Column: uint32(end), Offset: argLex.base + end}

		return Include{
			Path:  strings.Trim(rest, `"`),
			Range: common.Range{Start: arg.at.Start, End: endPos},
		}, true
	}
}

// endXsBlock finalizes the embedded XS block ending before line idx.
func (p *parser) endXsBlock(idx int) {
	end := min(idx, len(p.lines))

	for end > p.xsStart && strings.TrimSpace(p.lines[end-1]) == "" {
		end--
	}

	code := strings.Join(p.lines[p.xsStart:end], "\n")

	p.file.XsBlocks = append(p.file.XsBlocks, XsBlock{
		Code:  code,
		Range: common.Range{Start: p.lineStartPos(p.xsStart), End: p.lineStartPos(end)},
	})

	p.inXs = false
}

// add attaches a statement to the innermost collecting scope.
func (p *parser) add(stmt *node) {
	if len(p.scopes) > 0 {
		p.scopes[len(p.scopes)-1].children = append(p.scopes[len(p.scopes)-1].children, stmt)
	} else {
		p.cur.nodes = append(p.cur.nodes, stmt)
	}

	if stmt.kind == KindCommand {
		p.lastCmd = stmt
	}
}

// finalizeScope settles a closing block's range over its children.
func (p *parser) finalizeScope(open *node) {
	for _, child := range open.children {
		if child.end.After(open.end) {
			open.end = child.end
		}
	}
}

// syncConstructs reports constructs left open at a section header or EOF.
func (p *parser) syncConstructs(context string) {
	if p.owner != nil {
		p.diags = append(p.diags, common.Diagnostic{
			Range:    common.Range{Start: p.openPos, End: p.openPos},
			Severity: common.SeverityWarning,
			Message:  fmt.Sprintf("unclosed \"{\" block (%s)", context),
			Code:     "syntax",
		})
		p.owner = nil
	}

	p.braceScopes = nil

	for _, open := range p.scopes {
		p.finalizeScope(open)
	}

	if len(p.scopes) > 0 {
		names := make([]string, 0, len(p.scopes))
		for _, open := range p.scopes {
			names = append(names, open.name)
		}

		p.diags = append(p.diags, common.Diagnostic{
			Range:    common.Range{Start: p.scopes[0].start, End: p.scopes[0].end},
			Severity: common.SeverityWarning,
			Message:  fmt.Sprintf("unterminated block(s): %s", strings.Join(names, ", ")),
			Code:     "syntax",
		})

		p.scopes = nil
	}
}

// closeSection materializes the current section into the file.
func (p *parser) closeSection(end common.Pos) {
	p.syncConstructs("section change")
	p.lastCmd = nil

	if p.cur.name == "global" && len(p.cur.nodes) == 0 {
		return
	}

	out := Section{
		Name:       p.cur.name,
		Statements: make([]Statement, 0, len(p.cur.nodes)),
		Range:      common.Range{Start: p.cur.start, End: end},
	}

	for _, n := range p.cur.nodes {
		out.Statements = append(out.Statements, materialize(n))
	}

	p.file.Sections = append(p.file.Sections, out)
}

// materialize converts the internal node tree into contract Statement
// values.
func materialize(n *node) Statement {
	out := Statement{
		Kind:       n.kind,
		Name:       n.name,
		Args:       n.args,
		Attributes: n.attrs,
		Range:      common.Range{Start: n.start, End: n.end},
	}

	if len(n.children) > 0 {
		out.Children = make([]Statement, 0, len(n.children))
		for _, child := range n.children {
			out.Children = append(out.Children, materialize(child))
		}
	}

	return out
}

// arguments converts command tokens to positional argument expressions
// and grows the statement range.
func (p *parser) arguments(toks []token, stmt *node) []Expr {
	args := p.expressions(toks)
	if len(args) > 0 && args[len(args)-1].Range.End.After(stmt.end) {
		stmt.end = args[len(args)-1].Range.End
	}

	return args
}

// pos builds the absolute position of (line, column).
func (p *parser) pos(line int, column int) common.Pos {
	return common.Pos{Line: uint32(line), Column: uint32(column), Offset: p.starts[line] + column}
}

// lineStartPos builds the start position of line, tolerating line ==
// len(p.lines) (end of file): the position of the last line's end.
func (p *parser) lineStartPos(line int) common.Pos {
	if line < len(p.lines) {
		return p.pos(line, 0)
	}

	last := len(p.lines) - 1

	return common.Pos{
		Line:   uint32(last),
		Column: uint32(len(p.lines[last])),
		Offset: p.starts[last] + len(p.lines[last]),
	}
}

// reportf appends a syntax diagnostic.
func (p *parser) reportf(r common.Range, severity int, code string, format string, args ...any) {
	p.diags = append(p.diags, common.Diagnostic{
		Range:    r,
		Severity: severity,
		Message:  fmt.Sprintf(format, args...),
		Code:     code,
	})
}

// structuralWords are the keywords that open or close nesting.
var structuralWords = map[string]bool{
	"if":             true,
	"elseif":         true,
	"else":           true,
	"endif":          true,
	"start_random":   true,
	"end_random":     true,
	"percent_chance": true,
}

// directiveWords are the #-directives; any other #-line is a comment.
var directiveWords = map[string]bool{
	"#include":     true,
	"#includeXS":   true,
	"#include_drs": true,
	"#const":       true,
	"#define":      true,
}

// sectionHeader recognizes "<name>" and "</name>" lines.
func sectionHeader(trimmed string) (string, bool) {
	if len(trimmed) < 3 || trimmed[0] != '<' {
		return "", false
	}

	name := strings.Trim(trimmed, "<>/")
	if name == "" || strings.ContainsAny(name, " \t<>") {
		return "", false
	}

	return strings.ToLower(name), true
}

// isDirectiveLine reports whether the trimmed line is a #-directive.
func isDirectiveLine(trimmed string) bool {
	fields := strings.Fields(trimmed)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "#") {
		return false
	}

	return directiveWords[fields[0]]
}

// xsTerminator reports whether a line ends an embedded XS block: a
// section header or a #-directive.
func xsTerminator(trimmed string) bool {
	if _, ok := sectionHeader(trimmed); ok {
		return true
	}

	return isDirectiveLine(trimmed)
}

// blankComments returns a copy of lines with /* */ and // comments
// replaced by spaces (preserving all offsets) together with the exact
// comment extents: multi-line block comments span from "/*" past "*/"
// across the recorded lines; line comments span to the end of their line.
func blankComments(lines []string, starts []int) ([]string, []common.Range) {
	out := make([]string, len(lines))
	copy(out, lines)

	var comments []common.Range

	inBlock := false
	var blockStart common.Pos

	at := func(i int, col int) common.Pos {
		return common.Pos{Line: uint32(i), Column: uint32(col), Offset: starts[i] + col}
	}

	for i := range out {
		line := out[i]

		for j := 0; j < len(line); j++ {
			switch {
			case inBlock:
				if line[j] == '*' && j+1 < len(line) && line[j+1] == '/' {
					line = line[:j] + "  " + line[j+2:]
					comments = append(comments, common.Range{Start: blockStart, End: at(i, j+2)})
					inBlock = false
					j++
				} else if line[j] != '\n' {
					line = line[:j] + " " + line[j+1:]
				}
			case line[j] == '/' && j+1 < len(line) && line[j+1] == '*':
				line = line[:j] + "  " + line[j+2:]
				blockStart = at(i, j)
				inBlock = true
				j++
			case line[j] == '/' && j+1 < len(line) && line[j+1] == '/':
				line = line[:j] + strings.Repeat(" ", len(line)-j)
				comments = append(comments, common.Range{Start: at(i, j), End: at(i, len(line))})
				j = len(line)
			}
		}

		out[i] = line
	}

	if inBlock {
		last := len(out) - 1
		comments = append(comments, common.Range{Start: blockStart, End: at(last, len(out[last]))})
	}

	return out, comments
}

// tokenKind enumerates the lexer token kinds.
type tokenKind int

// Token kinds.
const (
	tokWord tokenKind = iota
	tokNumber
	tokString
	tokLParen
	tokRParen
	tokLBrace
	tokRBrace
	tokComma
	tokOp
	tokUnknown
	tokEOF
)

// token is one lexical token with its source range.
type token struct {
	kind      tokenKind
	text      string
	isPercent bool
	at        common.Range
}

// lexer scans one physical line.
type lexer struct {
	line  string
	base  int // byte offset of the line start
	idx   int // line index
	pos   int // cursor within the line
	eofAt common.Range
}

// newLexer creates a lexer over one line.
func newLexer(line string, base int, idx int) *lexer {
	return &lexer{
		line: line,
		base: base,
		idx:  idx,
		eofAt: common.Range{
			Start: common.Pos{Line: uint32(idx), Column: uint32(len(line)), Offset: base + len(line)},
			End:   common.Pos{Line: uint32(idx), Column: uint32(len(line)), Offset: base + len(line)},
		},
	}
}

// next returns the next token.
func (l *lexer) next() token {
	for l.pos < len(l.line) && (l.line[l.pos] == ' ' || l.line[l.pos] == '\t' || l.line[l.pos] == '\r') {
		l.pos++
	}

	if l.pos >= len(l.line) {
		return token{kind: tokEOF, at: l.eofAt}
	}

	start := l.pos
	ch := l.line[l.pos]

	isWord := ch == '_' || ch == '#' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')

	switch {
	case isWord:
		// '#' starts a word (directives like #const) and also continues one,
		// so the scan always advances past the first character.
		for l.pos < len(l.line) {
			c := l.line[l.pos]
			if c == '_' || c == '#' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
				l.pos++
				continue
			}

			break
		}

		if l.pos == start {
			l.pos++ // never return a zero-width token: callers loop on next()
		}

		return l.token(tokWord, start)
	case ch >= '0' && ch <= '9':
		for l.pos < len(l.line) {
			c := l.line[l.pos]
			if (c >= '0' && c <= '9') || c == '.' {
				l.pos++
				continue
			}

			break
		}

		tok := l.token(tokNumber, start)
		if l.pos < len(l.line) && l.line[l.pos] == '%' {
			l.pos++
			tok.isPercent = true
			tok.text = l.line[start:l.pos]
			tok.at.End = l.spanEnd()
		}

		return tok
	case ch == '"':
		l.pos++

		for l.pos < len(l.line) && l.line[l.pos] != '"' {
			l.pos++
		}

		if l.pos < len(l.line) {
			l.pos++
		}

		return l.token(tokString, start)
	case ch == '(':
		l.pos++

		return l.token(tokLParen, start)
	case ch == ')':
		l.pos++

		return l.token(tokRParen, start)
	case ch == '{':
		l.pos++

		return l.token(tokLBrace, start)
	case ch == '}':
		l.pos++

		return l.token(tokRBrace, start)
	case ch == ',':
		l.pos++

		return l.token(tokComma, start)
	case ch == '+' || ch == '-' || ch == '*' || ch == '/':
		l.pos++

		return l.token(tokOp, start)
	default:
		l.pos++

		return l.token(tokUnknown, start)
	}
}

// rest consumes and returns all remaining tokens.
func (l *lexer) rest() []token {
	var toks []token

	for {
		tok := l.next()
		if tok.kind == tokEOF {
			return toks
		}

		toks = append(toks, tok)
	}
}

// token builds a token for the consumed span [start, l.pos).
func (l *lexer) token(kind tokenKind, start int) token {
	return token{
		kind: kind,
		text: l.line[start:l.pos],
		at:   common.Range{Start: l.spanStart(start), End: l.spanEnd()},
	}
}

// spanStart builds the absolute position of a column on this line.
func (l *lexer) spanStart(col int) common.Pos {
	return common.Pos{Line: uint32(l.idx), Column: uint32(col), Offset: l.base + col}
}

// spanEnd builds the absolute position of the cursor on this line.
func (l *lexer) spanEnd() common.Pos {
	return l.spanStart(l.pos)
}

// operatorPrecedence returns the binding power of a binary operator
// token; 0 when the token is not a binary operator.
func operatorPrecedence(tok token) int {
	switch tok.text {
	case "+", "-":
		return 1
	case "*", "/":
		return 2
	default:
		return 0
	}
}

// expressions parses a token run into a list of value expressions;
// unknown characters are reported and skipped (recovery).
func (p *parser) expressions(toks []token) []Expr {
	lex := &tokenReader{tokens: toks}

	var out []Expr

	for lex.peek().kind != tokEOF {
		if lex.peek().kind == tokUnknown {
			bad := lex.next()
			p.reportf(bad.at, common.SeverityError, "syntax", "unexpected %q", bad.text)

			continue
		}

		before := lex.pos
		e, ok := parseExpr(lex, 1)
		if !ok {
			break
		}

		out = append(out, e)

		if lex.peek().kind == tokComma {
			lex.next()
		}

		if lex.pos == before {
			break
		}
	}

	p.recordExprWords(out)

	return out
}

// recordWord appends one word occurrence to the index.
func (p *parser) recordWord(name string, at common.Range) {
	p.file.words = append(p.file.words, wordOcc{name: name, at: at})
}

// recordExprWords collects the identifier/constant leaves of built
// expressions (helper-call names included) into the word index.
func (p *parser) recordExprWords(exprs []Expr) {
	for _, e := range exprs {
		switch e.Kind {
		case KindConst, KindIdent:
			p.recordWord(e.Value, e.Range)
		}

		p.recordExprWords(e.Children)
	}
}

// recordSectionWord records the section name token inside the angle
// brackets of a header line (opening or closing).
func (p *parser) recordSectionWord(name string, line string, idx int) {
	lead := len(line) - len(strings.TrimLeft(line, " \t"))
	col := lead + 1

	if strings.HasPrefix(strings.TrimSpace(line), "</") {
		col = lead + 2
	}

	start := p.pos(idx, col)
	end := p.pos(idx, col+len(name))

	p.recordWord(name, common.Range{Start: start, End: end})
}

// tokenReader iterates a fixed token slice.
type tokenReader struct {
	tokens []token
	pos    int
}

// next consumes one token.
func (r *tokenReader) next() token {
	if r.pos >= len(r.tokens) {
		return token{kind: tokEOF}
	}

	tok := r.tokens[r.pos]
	r.pos++

	return tok
}

// peek looks at the next token without consuming it.
func (r *tokenReader) peek() token {
	if r.pos >= len(r.tokens) {
		return token{kind: tokEOF}
	}

	return r.tokens[r.pos]
}

// parseExpr parses one expression with precedence climbing; minPrec is
// the minimum binding power to continue with.
func parseExpr(r *tokenReader, minPrec int) (Expr, bool) {
	lhs, ok := parsePrimary(r)
	if !ok {
		return Expr{}, false
	}

	for {
		op := r.peek()
		prec := operatorPrecedence(op)
		if prec == 0 || prec < minPrec {
			return lhs, true
		}

		r.next()

		rhs, ok := parseExpr(r, prec+1)
		if !ok {
			return lhs, true
		}

		lhs = Expr{
			Kind:     KindBinary,
			Value:    op.text,
			Children: []Expr{lhs, rhs},
			Range:    common.Range{Start: lhs.Range.Start, End: rhs.Range.End},
		}
	}
}

// parsePrimary parses a literal, identifier, call or unary expression.
func parsePrimary(r *tokenReader) (Expr, bool) {
	tok := r.next()

	switch tok.kind {
	case tokNumber:
		kind := KindNumber
		if tok.isPercent {
			kind = KindPercent
		}

		return Expr{Kind: kind, Value: tok.text, Range: tok.at}, true
	case tokString:
		return Expr{Kind: KindConst, Value: strings.Trim(tok.text, `"`), Range: tok.at}, true
	case tokWord:
		kind := KindIdent
		if isConstName(tok.text) {
			kind = KindConst
		}

		if r.peek().kind == tokLParen {
			return parseCall(r, tok, kind)
		}

		return Expr{Kind: kind, Value: tok.text, Range: tok.at}, true
	case tokOp:
		if tok.text != "-" && tok.text != "+" {
			return Expr{}, false
		}

		operand, ok := parseExpr(r, 3)
		if !ok {
			return Expr{Kind: KindUnary, Value: tok.text, Range: tok.at}, true
		}

		return Expr{
			Kind:     KindUnary,
			Value:    tok.text,
			Children: []Expr{operand},
			Range:    common.Range{Start: tok.at.Start, End: operand.Range.End},
		}, true
	case tokLParen:
		inner, _ := parseExpr(r, 1)

		if r.peek().kind == tokRParen {
			r.next()
		}

		return inner, true
	default:
		return Expr{}, false
	}
}

// parseCall parses name(arg, ...) helper calls, keeping arguments as
// Children.
func parseCall(r *tokenReader, name token, kind string) (Expr, bool) {
	r.next() // consume "("

	call := Expr{Kind: kind, Value: name.text, Range: name.at}

	for {
		if r.peek().kind == tokRParen {
			call.Range.End = r.next().at.End

			return call, true
		}

		arg, ok := parseExpr(r, 1)
		if !ok {
			if r.peek().kind == tokEOF {
				return call, true
			}

			r.next() // skip the unexpected token and keep going

			continue
		}

		call.Children = append(call.Children, arg)
		call.Range.End = arg.Range.End

		if r.peek().kind == tokComma {
			r.next()
		}
	}
}

// isConstName reports whether an identifier looks like an ALL-CAPS
// constant (GRASS, TINY_MAP, CT_GRANITE).
func isConstName(word string) bool {
	for i := 0; i < len(word); i++ {
		c := word[i]
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			continue
		}

		return false
	}

	return len(word) > 0
}
