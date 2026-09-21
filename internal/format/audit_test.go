package format

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/rms"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// corpusDir is the real-map corpus the audit walks; it is fetched
// separately and a missing directory only skips the audit.
var corpusDir = filepath.Join("..", "..", ".corpus")

// TestAudit is the FR0 corpus audit: it reconstructs every statement and
// attribute line of every .rms file from the AST alone and compares it
// with the source token for token, whitespace aside. What it prints —
// divergence classes, structural words inside command extents, comment
// and EOL statistics — is the input inventory the printer (rms.go) is
// built against; a difference the allow-list does not explain fails the
// audit with a file:line sample.
func TestAudit(t *testing.T) {
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Skipf("corpus not fetched: %v", err)
	}

	a := &audit{eols: map[string]int{}, tailShapes: map[string]int{}}

	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".rms") {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(corpusDir, entry.Name()))
		if err != nil {
			t.Errorf("read %s: %v", entry.Name(), err)

			continue
		}

		a.file(entry.Name(), string(raw))
	}

	a.report(t)
}

// audit accumulates one corpus pass; every counter is aggregated here so
// the test prints one bounded report instead of per-file findings.
type audit struct {
	files  int
	eols   map[string]int
	stray  int // files with a lone \r that is not part of an EOL
	shapes shapeStats

	warnFiles int
	errFiles  int
	errNames  []string

	multiGlobal int // files with more than one synthetic "global" section

	includes   int
	xsIncludes int
	incFiles   []string

	comments commentStats

	braced              int // commands with a structural word inside their { } block
	bracedFiles         []string
	bracelessIf         int // brace-less create_* commands covering a sibling if
	bracelessFiles      []string
	bracelessOther      int // remaining brace-less extents with a structural word
	bracelessOtherFiles []string

	quoted         int // string literals the AST keeps unquoted
	inline         int // command lines truncated at a same-line "{"
	compared       int // statement and attribute lines compared
	exact          int // of those, the token-exact matches
	tailsAttr      int // extra values past an attribute's first
	tailsCmd       int // source tokens past the arguments of a command line
	tailShapes     map[string]int
	tailFiles      []string
	tailSamples    []string
	quotes         int // differences that are only string quotes
	quoteFiles     []string
	parens         int // differences that are only redundant parentheses
	parenFiles     []string
	spacing        int // same characters, whitespace sitting elsewhere
	spacingFiles   []string
	spacingSamples []string
	junk           int // dropped junk tokens in files the parser already rejected
	junkFiles      []string
	newClasses     []string // allow-list failures, each with a file:line sample
}

// shapeStats counts the AST elements the printer walks; the totals bound
// the size of the input it has to survive.
type shapeStats struct {
	sections   int
	statements int
	commands   int
	attributes int
	xsBlocks   int
}

// commentStats buckets every comment extent of the corpus by the position
// the printer has to anchor it at: inside an XS block (it rides along
// with the verbatim Code), inside a statement extent, between the
// statements of a section, or outside every section — plus the inline
// (same line as tokens) and consecutive-run shapes.
type commentStats struct {
	total    int
	inXs     int
	inStmt   int
	inSect   int
	outside  int
	inline   int
	runs     int
	maxRun   int
	unsorted int
}

// fileCtx is the per-file state of one audit pass: the line views the
// comparison works on, the flattened statements and the error flag the
// classifier needs.
type fileCtx struct {
	name    string
	norm    []string // parser coordinate space: lines with their \r tail dropped
	blanked []string // norm with comments blanked to spaces, offsets kept
	stmts   []rms.Statement
	errors  bool
	xs      []span
}

// file audits one corpus file: parse it, compare every statement and
// attribute line with its source, catalogue the shapes.
func (a *audit) file(name string, src string) {
	a.files++
	a.eol(src)

	file, diags := rms.Parse(src, name)

	ctx := &fileCtx{name: name, norm: trimCR(strings.Split(src, "\n"))}
	ctx.stmts = allStmts(file.Sections)
	ctx.blanked = blankComments(ctx.norm)

	for _, block := range file.XsBlocks {
		ctx.xs = append(ctx.xs, span{
			start: atPoint(block.Range.Start.Line, block.Range.Start.Column),
			end:   atPoint(block.Range.End.Line, block.Range.End.Column),
		})
	}

	a.shapes.sections += len(file.Sections)
	a.shapes.statements += len(ctx.stmts)
	a.shapes.xsBlocks += len(file.XsBlocks)

	warns, errs := false, false

	for _, d := range diags {
		switch d.Severity {
		case common.SeverityError:
			errs = true
		case common.SeverityWarning:
			warns = true
		}
	}

	ctx.errors = errs

	if warns {
		a.warnFiles++
	}

	if errs {
		a.errFiles++
		noteFile(&a.errNames, name, "")
	}

	globals := 0

	for _, sec := range file.Sections {
		if sec.Name == "global" {
			globals++
		}
	}

	if globals > 1 {
		a.multiGlobal++
	}

	a.includes += len(file.Includes)
	a.xsIncludes += len(file.XsIncludes)

	if len(file.Includes) > 0 || len(file.XsIncludes) > 0 {
		noteFile(&a.incFiles, name, "")
	}

	for _, stmt := range ctx.stmts {
		if stmt.Kind == rms.KindCommand {
			a.shapes.commands++
		}

		a.shapes.attributes += len(stmt.Attributes)

		a.stmtLine(ctx, stmt)
		a.attrLines(ctx, stmt)

		if stmt.Kind == rms.KindCommand {
			a.structural(ctx, stmt)
		}
	}

	a.commentsOf(ctx, file)
}

// eol classifies the file's line endings: the contract keeps the dominant
// EOL, and ties — or a file without any newline — print "\n".
func (a *audit) eol(src string) {
	crlf := strings.Count(src, "\r\n")
	lf := strings.Count(src, "\n")

	var class string

	switch {
	case lf == 0:
		class = "none"
	case crlf == lf:
		class = "crlf"
	case crlf == 0:
		class = "lf"
	case crlf*2 > lf:
		class = "mixed-crlf"
	default:
		class = "mixed-lf"
	}

	a.eols[class]++

	if strings.Count(src, "\r") > crlf {
		a.stray++
	}
}

// stmtLine compares the command line — name plus positional arguments —
// with its source line. A block opened on the same line is truncated at
// "{": the AST carries no extent for name+args, and the printer emits the
// block's attributes on following lines anyway.
func (a *audit) stmtLine(ctx *fileCtx, stmt rms.Statement) {
	parts := []string{stmt.Name}

	for _, arg := range stmt.Args {
		parts = append(parts, exprText(ctx.norm, arg))
		a.countQuoted(ctx, arg)
	}

	got := lineTokens(ctx.blanked, atPoint(stmt.Range.Start.Line, stmt.Range.Start.Column))

	if i := slices.Index(got, "{"); i >= 0 {
		a.inline++
		got = got[:i]
	}

	a.compare(ctx, "command", int(stmt.Range.Start.Line), strings.Fields(strings.Join(parts, " ")), got)
}

// attrLines compares every attribute line with its source extent. The AST
// keeps only an attribute's first value, so the values past it surface as
// the tail class.
func (a *audit) attrLines(ctx *fileCtx, stmt rms.Statement) {
	for _, attr := range stmt.Attributes {
		a.countQuoted(ctx, attr.Value)

		want := strings.Fields(strings.Join([]string{attr.Name, exprText(ctx.norm, attr.Value)}, " "))
		got := strings.Fields(sliceText(ctx.norm,
			atPoint(attr.Range.Start.Line, attr.Range.Start.Column),
			atPoint(attr.Range.End.Line, attr.Range.End.Column)))

		a.compare(ctx, "attribute", int(attr.Range.Start.Line), want, got)
	}
}

// compare classifies one reconstruction mismatch. The allow-list covers
// the losses the AST is known to take — string quotes, redundant
// parentheses, values past an attribute's first — and dropped junk in
// files the parser already rejected; anything else fails the audit.
func (a *audit) compare(ctx *fileCtx, what string, line int, want, got []string) {
	a.compared++

	class := classify(want, got)
	if class == "" {
		a.exact++

		return
	}

	sample := fmt.Sprintf("%s:%d %s source %q reconstruction %q", ctx.name, line+1, what, got, want)

	switch class {
	case "quotes":
		a.quotes++
		noteFile(&a.quoteFiles, ctx.name, "")
	case "parens":
		a.parens++
		noteFile(&a.parenFiles, ctx.name, "")
	case "spacing":
		a.spacing++
		noteFile(&a.spacingFiles, ctx.name, "")
		note(&a.spacingSamples, sample, 3)
	case "tail":
		if ctx.errors {
			// a file the parser already rejected: extra source content
			// past the reconstruction is dropped junk as much as a tail,
			// and its shapes are of no interest — the printer refuses
			// such input
			a.junk++
			noteFile(&a.junkFiles, ctx.name, "")

			return
		}

		noteFile(&a.tailFiles, ctx.name, "")
		note(&a.tailSamples, sample, 3)

		if what == "attribute" {
			a.tailsAttr++

			// the class is a character-prefix match, so the source may
			// hold fewer tokens than the reconstruction (a glued "3-5"
			// against "3 - 5"): a token tail exists only past its end
			rest := []string(nil)

			if len(got) > len(want) {
				rest = got[len(want):]
			}

			a.tailShapes[tailShape(rest)]++
		} else {
			a.tailsCmd++
		}
	case "other":
		if ctx.errors {
			a.junk++
			noteFile(&a.junkFiles, ctx.name, "")

			return
		}

		note(&a.newClasses, sample, 5)
	}
}

// countQuoted counts the leaves whose source cut is a quoted string: the
// AST stores them unquoted, so every one is a token the printer must
// re-quote or cut from the source to keep byte-exact.
func (a *audit) countQuoted(ctx *fileCtx, e rms.Expr) {
	if e.Kind == "" {
		return
	}

	if len(e.Children) == 0 {
		cut := sliceText(ctx.norm,
			atPoint(e.Range.Start.Line, e.Range.Start.Column),
			atPoint(e.Range.End.Line, e.Range.End.Column))

		if strings.HasPrefix(cut, `"`) {
			a.quoted++
		}

		return
	}

	for _, child := range e.Children {
		a.countQuoted(ctx, child)
	}
}

// structural catalogues the structural keywords living inside a command
// extent [Range.Start, Range.End): inside a { } block the parser tracks
// them without materializing nodes, and a brace-less create_* covers a
// sibling if — both are invisible to the AST, so the printer cannot
// re-emit them without knowing they are there.
func (a *audit) structural(ctx *fileCtx, stmt rms.Statement) {
	foundRandom, foundIf, braced := false, false, false

	start := atPoint(stmt.Range.Start.Line, stmt.Range.Start.Column)
	end := endLine(stmt.Range.End.Line, stmt.Range.End.Column)

	for line := start.line; line <= end && line < len(ctx.blanked); line++ {
		fields := strings.Fields(ctx.blanked[line])

		if slices.Contains(fields, "{") {
			braced = true
		}

		if len(fields) == 0 || !structuralWords[fields[0]] {
			continue
		}

		if isConditionalWord(fields[0]) {
			foundIf = true
		} else {
			foundRandom = true
		}
	}

	switch {
	case !foundIf && !foundRandom:
		return
	case braced:
		a.braced++
		noteFile(&a.bracedFiles, ctx.name, "("+stmt.Name+")")
	case strings.HasPrefix(stmt.Name, "create_") && foundIf:
		a.bracelessIf++
		noteFile(&a.bracelessFiles, ctx.name, "("+stmt.Name+")")
	default:
		a.bracelessOther++
		noteFile(&a.bracelessOtherFiles, ctx.name, "("+stmt.Name+")")
	}
}

// commentsOf buckets the comment extents of one file and counts the
// consecutive-comment runs. The extents are the parser's own Comments —
// the surface the printer anchors from, plain #-lines included, so the
// catalogue covers what the printer will actually see — and the buckets
// say where those positions are; the XS ones must not be anchored at all,
// they ride along with the verbatim Code.
func (a *audit) commentsOf(ctx *fileCtx, file rms.RmsFile) {
	comments := make([]span, 0, len(file.Comments))

	for _, r := range file.Comments {
		comments = append(comments, span{
			start: atPoint(r.Start.Line, r.Start.Column),
			end:   atPoint(r.End.Line, r.End.Column),
		})
	}

	a.comments.total += len(comments)

	// the runs walk needs position order: counted, not assumed — an
	// unsorted Comments is exactly what the audit is here to notice
	for i := 1; i < len(comments); i++ {
		if comments[i].start.Before(comments[i-1].start) {
			a.comments.unsorted++
		}
	}

	for _, c := range comments {
		switch {
		case spanAny(ctx.xs, c.start):
			a.comments.inXs++
		case stmtAny(ctx.stmts, c.start):
			a.comments.inStmt++
		case sectAny(file.Sections, c.start):
			a.comments.inSect++
		default:
			a.comments.outside++
		}

		if line := c.start.line; line < len(ctx.blanked) &&
			strings.TrimSpace(ctx.blanked[line][:min(c.start.col, len(ctx.blanked[line]))]) != "" {
			a.comments.inline++
		}
	}

	if len(comments) > 0 {
		runs, cur := 1, 1

		for i := 1; i < len(comments); i++ {
			if blanksBetween(ctx.blanked, comments[i-1], comments[i]) {
				cur++
			} else {
				runs++
				cur = 1
			}

			a.comments.maxRun = max(a.comments.maxRun, cur)
		}

		a.comments.runs += runs
	}
}

// report prints the aggregated catalogue: one bounded summary per
// catalogue item, the allow-list classes with their samples and every
// allow-list failure as an error.
func (a *audit) report(t *testing.T) {
	t.Logf("files: %d", a.files)
	t.Logf("EOL: %v; files with a lone CR: %d", a.eols, a.stray)
	t.Logf("shapes: %+v", a.shapes)
	t.Logf("diagnostics: %d files with warnings, %d with errors; error files: %s",
		a.warnFiles, a.errFiles, strings.Join(a.errNames, ", "))
	t.Logf("synthetic \"global\" sections: files with more than one: %d", a.multiGlobal)
	t.Logf("directives: %d #include, %d #includeXS; files: %s",
		a.includes, a.xsIncludes, strings.Join(a.incFiles, ", "))
	t.Logf("comments (RmsFile.Comments): %+v", a.comments)
	t.Logf("structural words inside command extents: braced %d %v, braceless create_* sibling-if %d %v, other %d %v",
		a.braced, a.bracedFiles, a.bracelessIf, a.bracelessFiles, a.bracelessOther, a.bracelessOtherFiles)
	t.Logf("token comparison: %d lines compared, %d exact, %d truncated at a same-line \"{\", %d quoted literals",
		a.compared, a.exact, a.inline, a.quoted)
	t.Logf("attribute tails: %d multi-value, %d past command args, shapes %v, files %s",
		a.tailsAttr, a.tailsCmd, a.tailShapes, strings.Join(a.tailFiles, ", "))
	t.Logf("tail samples: %s", strings.Join(a.tailSamples, " | "))
	t.Logf("spacing samples: %s", strings.Join(a.spacingSamples, " | "))
	t.Logf("allow-list: %d quote-only %v, %d paren-only %v, %d spacing-only, %d junk %v",
		a.quotes, a.quoteFiles, a.parens, a.parenFiles, a.spacing, a.junk, a.junkFiles)

	for _, sample := range a.newClasses {
		t.Errorf("divergence the allow-list does not explain: %s", sample)
	}
}

// classify names the allow-list class of a reconstruction mismatch: ""
// on a token-exact match, then the known losses in the order they are
// normalized away, and "other" for anything the allow-list does not
// explain.
//
// Spacing and tail are compared whitespace-blind, over the joined
// characters: the corpus writes punctuation and negative signs
// inconsistently (rnd(-302,+50) beside rnd (-302,+50), and "-300" in
// argument position parses as one glued token), so no AST-faithful
// reconstruction can match the source token for token. What the classes
// still guard is content — spacing means the very same characters, tail
// means the source carries extra content past the reconstructed prefix.
func classify(want, got []string) string {
	switch {
	case slices.Equal(want, got):
		return ""
	case slices.Equal(dequote(want), dequote(got)):
		return "quotes"
	case slices.Equal(dropParens(want), dropParens(got)):
		return "parens"
	case strings.Join(want, "") == strings.Join(got, ""):
		return "spacing"
	case strings.HasPrefix(strings.Join(got, ""), strings.Join(want, "")):
		return "tail"
	default:
		return "other"
	}
}

// dequote strips the string delimiters off every token: a difference that
// is only quotes is the AST's known string-literal loss.
func dequote(toks []string) []string {
	out := make([]string, len(toks))

	for i, tok := range toks {
		out[i] = strings.Trim(tok, `"`)
	}

	return out
}

// dropParens removes the parenthesis tokens: the expression parser drops
// them, so a difference that is only parentheses is the known loss.
func dropParens(toks []string) []string {
	out := make([]string, 0, len(toks))

	for _, tok := range toks {
		if tok != "(" && tok != ")" {
			out = append(out, tok)
		}
	}

	return out
}

// tailShape names the shape of the values an attribute carries past its
// first: the printer has to know whether it would be dropping numbers or
// names. An empty tail is its own shape — the reconstruction already
// covers the whole extent.
func tailShape(toks []string) string {
	if len(toks) == 0 {
		return "none"
	}

	numeric, names := 0, 0

	for _, tok := range toks {
		if isNumericToken(tok) {
			numeric++
		} else {
			names++
		}
	}

	switch {
	case names == 0:
		return "numeric"
	case numeric == 0:
		return "names"
	default:
		return "mixed"
	}
}

// isNumericToken reports whether a token is a number literal: digits and
// dots with an optional leading sign and trailing percent.
func isNumericToken(tok string) bool {
	tok = strings.TrimSuffix(strings.TrimPrefix(tok, "-"), "%")
	if tok == "" {
		return false
	}

	for i := 0; i < len(tok); i++ {
		if (tok[i] < '0' || tok[i] > '9') && tok[i] != '.' {
			return false
		}
	}

	return true
}

// exprText rebuilds one expression from the AST alone: a leaf is the
// exact source cut (the AST keeps string literals unquoted, so the cut —
// not Value — is what survives the round trip), binary operators print
// infix with single spaces, unary operators attach without a space and
// calls print as name(arg, ...).
func exprText(lines []string, e rms.Expr) string {
	switch e.Kind {
	case "":
		return ""
	case rms.KindBinary:
		return exprText(lines, e.Children[0]) + " " + e.Value + " " + exprText(lines, e.Children[1])
	case rms.KindUnary:
		return e.Value + exprText(lines, e.Children[0])
	}

	if len(e.Children) > 0 {
		parts := make([]string, 0, len(e.Children))

		for _, child := range e.Children {
			parts = append(parts, exprText(lines, child))
		}

		return e.Value + "(" + strings.Join(parts, ", ") + ")"
	}

	return sliceText(lines,
		atPoint(e.Range.Start.Line, e.Range.Start.Column),
		atPoint(e.Range.End.Line, e.Range.End.Column))
}

// lineTokens returns the whitespace tokens of the blanked line at pos and
// past it; positions outside the file answer nothing.
func lineTokens(blanked []string, pos point) []string {
	if pos.line < 0 || pos.line >= len(blanked) {
		return nil
	}

	line := blanked[pos.line]

	return strings.Fields(line[min(pos.col, len(line)):])
}

// sliceText cuts an extent out of the original lines by Line/Column. The
// AST offsets are "\n"-coordinates and must not be used against the raw
// bytes; the "\r" tails of the original lines lie past every token and
// never enter a cut.
func sliceText(lines []string, start, end point) string {
	if start.line < 0 || start.line >= len(lines) || end.line >= len(lines) || end.Before(start) {
		return ""
	}

	if start.line == end.line {
		if start.col > len(lines[start.line]) {
			return ""
		}

		return lines[start.line][start.col:min(end.col, len(lines[start.line]))]
	}

	var b strings.Builder

	b.WriteString(lines[start.line][min(start.col, len(lines[start.line])):])

	for i := start.line + 1; i < end.line; i++ {
		b.WriteString("\n")
		b.WriteString(lines[i])
	}

	b.WriteString("\n")
	b.WriteString(lines[end.line][:min(end.col, len(lines[end.line]))])

	return b.String()
}

// point is a (line, column) position: the AST carries the same
// coordinates as a uint32 pair, so the audit compares them numerically
// without importing the positional type.
type point struct {
	line int
	col  int
}

// Before reports whether p lies strictly before other in (line, column)
// order — the same ordering the AST positions follow.
func (p point) Before(other point) bool {
	if p.line != other.line {
		return p.line < other.line
	}

	return p.col < other.col
}

// span is a half-open [start, end) extent of (line, column) positions.
type span struct {
	start point
	end   point
}

// atPoint converts an AST position pair into the audit's coordinates.
func atPoint(line, col uint32) point {
	return point{line: int(line), col: int(col)}
}

// endLine is the last line of an extent: Range.End is exclusive, so a
// zero-column end sits on the previous line.
func endLine(line, col uint32) int {
	if col == 0 {
		return int(line) - 1
	}

	return int(line)
}

// spanAny reports whether any extent contains pos.
func spanAny(spans []span, pos point) bool {
	for _, s := range spans {
		if s.contains(pos) {
			return true
		}
	}

	return false
}

// contains reports whether pos lies within the half-open extent.
func (s span) contains(pos point) bool {
	return !pos.Before(s.start) && pos.Before(s.end)
}

// stmtAny reports whether any statement extent contains pos.
func stmtAny(stmts []rms.Statement, pos point) bool {
	for _, stmt := range stmts {
		if spanContains(stmt.Range, pos) || stmtAny(stmt.Children, pos) {
			return true
		}
	}

	return false
}

// sectAny reports whether any section extent contains pos.
func sectAny(sections []rms.Section, pos point) bool {
	for _, sec := range sections {
		if spanContains(sec.Range, pos) {
			return true
		}
	}

	return false
}

// spanContains adapts an AST range to the audit's containment test.
func spanContains(r common.Range, pos point) bool {
	return !pos.Before(atPoint(r.Start.Line, r.Start.Column)) &&
		pos.Before(atPoint(r.End.Line, r.End.Column))
}

// allStmts flattens the file's statements depth-first: every audit pass
// walks the same list.
func allStmts(sections []rms.Section) []rms.Statement {
	var out []rms.Statement

	for _, sec := range sections {
		appendStmts(&out, sec.Statements)
	}

	return out
}

// appendStmts appends stmts and their nested children to out.
func appendStmts(out *[]rms.Statement, stmts []rms.Statement) {
	for _, stmt := range stmts {
		*out = append(*out, stmt)
		appendStmts(out, stmt.Children)
	}
}

// trimCR drops one trailing "\r" per line: the parser normalizes "\r\n"
// before building positions, so AST extents only line up with the split
// lines once their tail is gone.
func trimCR(lines []string) []string {
	out := make([]string, len(lines))

	for i, line := range lines {
		out[i] = strings.TrimSuffix(line, "\r")
	}

	return out
}

// blankComments blanks /* */ and // comment bytes out of the lines,
// leaving the comparison lines free of comment words: the production
// blankCode of the printer, which the audit examines, does exactly this —
// the same function keeps the two from drifting apart. The comment
// catalogue itself is not built here: it comes from the parser's
// RmsFile.Comments, which is the surface the printer anchors from.
func blankComments(lines []string) []string {
	return blankCode(lines)
}

// blanksBetween reports whether nothing but blanks separates two
// consecutive comment extents — the shape of a run of comments the
// printer keeps in order.
func blanksBetween(blanked []string, prev, next span) bool {
	if next.start.line < prev.end.line || next.start.line > prev.end.line+1 {
		return false
	}

	var b strings.Builder

	if prev.end.line < len(blanked) {
		b.WriteString(blanked[prev.end.line][min(prev.end.col, len(blanked[prev.end.line])):])
	}

	for i := prev.end.line + 1; i < next.start.line && i < len(blanked); i++ {
		b.WriteString(blanked[i])
	}

	if next.start.line < len(blanked) {
		b.WriteString(blanked[next.start.line][:min(next.start.col, len(blanked[next.start.line]))])
	}

	return strings.TrimSpace(b.String()) == ""
}

// note appends s to dst while it has fewer than limit entries: the report
// prints examples, never per-occurrence spam.
func note(dst *[]string, s string, limit int) {
	if len(*dst) < limit {
		*dst = append(*dst, s)
	}
}

// noteFile records a file example once per file: the report lists files,
// and a bucket one file dominates would otherwise fill every slot with
// the same name.
func noteFile(dst *[]string, file, label string) {
	for _, s := range *dst {
		if s == file || strings.HasPrefix(s, file+" ") {
			return
		}
	}

	if label != "" {
		file += " " + label
	}

	note(dst, file, 3)
}

// structuralWords mirrors the parser's nesting keywords
// (internal/rms/parse.go): the audit cannot reach the unexported map.
var structuralWords = map[string]bool{
	"if":             true,
	"elseif":         true,
	"else":           true,
	"endif":          true,
	"start_random":   true,
	"end_random":     true,
	"percent_chance": true,
}

// isConditionalWord reports whether a structural keyword belongs to an
// if group — the sibling if a brace-less create_* covers.
func isConditionalWord(word string) bool {
	return word == "if" || word == "elseif" || word == "else" || word == "endif"
}
