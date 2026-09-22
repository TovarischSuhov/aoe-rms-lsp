package format

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/rms"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// ErrParseErrors reports a refused input: the source carries error-severity
// diagnostics, so the AST is not a faithful enough base to re print from —
// recovered text could come out valid-looking and wrong. Classify with
// errors.Is; formatted is empty.
var ErrParseErrors = errors.New("rms source has error-severity diagnostics")

// Kinds of stream element.
const (
	elemSection   = "section"
	elemStatement = "statement"
	elemInclude   = "include"
	elemXsBlock   = "xs-block"
)

// RMS formats an RMS source into the cell's canonical style: indentation,
// brace placement and blank lines are rebuilt from the AST, token text
// comes from the source so string literals keep their quotes. The output
// keeps the input's dominant line ending. Inputs with error-severity
// diagnostics are refused — ErrParseErrors, formatted empty.
func RMS(source string, opts Options) (string, error) {
	// the file name reaches no output — diagnostics are folded into the
	// refusal error below
	file, diags := rms.Parse(source, "")
	if err := refuse(diags); err != nil {
		return "", err
	}

	p := &printer{
		doc:  newSource(source),
		eol:  dominantEOL(source),
		tabs: opts.IndentTabs,
		size: indentWidth(opts.TabSize),
	}
	p.comments = file.Comments
	p.rawClosed = make(map[uint64]bool)
	p.xsMask = xsLineMask(p.doc.blanked, file.XsBlocks)
	p.chainLast = chainLast(p.doc.blanked, p.xsMask)
	p.list(p.elements(file), 0, p.doc.eof(), true)

	return p.result(), nil
}

// refuse turns the first error-severity diagnostic into the refusal error:
// warnings format anyway — recovered input is exactly what the printer is
// for.
func refuse(diags []common.Diagnostic) error {
	for _, d := range diags {
		if d.Severity != common.SeverityError {
			continue
		}

		return fmt.Errorf("%w: %s (line %d)", ErrParseErrors, d.Message, d.Range.Start.Line+1)
	}

	return nil
}

// dominantEOL picks the input's line ending: the more frequent of "\r\n"
// and the bare "\n". A tie — a file without newlines included — prints
// "\n", the cell's canonical ending.
func dominantEOL(source string) string {
	total := strings.Count(source, "\n")
	crlf := strings.Count(source, "\r\n")

	if crlf > total-crlf {
		return "\r\n"
	}

	return "\n"
}

// indentWidth resolves the zero value of Options.TabSize to the default
// width of 4.
func indentWidth(size int) int {
	if size <= 0 {
		return 4
	}

	return size
}

// source is the input text in the coordinates the AST addresses: lines
// split on "\n" with their "\r" tails kept — the raw bytes are what
// verbatim regions re-print — plus the same lines with comment bytes
// blanked out, so structural-word detection reads code only.
type source struct {
	lines   []string
	blanked []string
}

// newSource splits the input and blanks its comments.
func newSource(src string) *source {
	lines := strings.Split(src, "\n")

	return &source{lines: lines, blanked: blankCode(lines)}
}

// eof is the position past the last line: the boundary the top-level
// stream prints up to.
func (s *source) eof() common.Pos {
	last := len(s.lines) - 1

	return common.Pos{Line: uint32(last), Column: uint32(len(s.lines[last]))}
}

// lineEnd is the position just past a line's last byte.
func (s *source) lineEnd(line int) common.Pos {
	if line < 0 || line >= len(s.lines) {
		return s.eof()
	}

	return common.Pos{Line: uint32(line), Column: uint32(len(s.lines[line]))}
}

// text cuts one range out of the raw lines. AST ranges address the
// "\r\n"-normalized text, so they are followed by Line/Column only — the
// offsets are normalized ones and would miss against the raw bytes. A
// zero-column end lies at the previous line's tail, not on an empty line.
func (s *source) text(r common.Range) string {
	return cutLines(s.lines, r)
}

// blankedText cuts one range out of the comment-blanked lines: same
// coordinates, comment bytes replaced by spaces — for reads that must not
// trip over a comment sitting in the middle.
func (s *source) blankedText(r common.Range) string {
	return cutLines(s.blanked, r)
}

// cutLines slices one range out of the given line view, keeping the lines'
// own bytes: multi-line cuts re-join with "\n", so a "\r" tail travels
// along and a CRLF region comes back byte-exact.
func cutLines(lines []string, r common.Range) string {
	if r.End.Before(r.Start) {
		return ""
	}

	start, end := int(r.Start.Line), int(r.End.Line)
	if start >= len(lines) || end >= len(lines) {
		return ""
	}

	lastCol := min(int(r.End.Column), len(lines[end]))
	if lastCol == 0 && end > start {
		end--
		// the full raw tail travels: exactly one EOL byte is every
		// consumer's own to cut — rawChunk (TrimSuffix) for the verbatim
		// slices, commentText for the comment anchors — and the page
		// ends the line with its own line ending
		lastCol = len(lines[end])
	}

	first := min(int(r.Start.Column), len(lines[start]))
	if start == end {
		// a range retargeted by commentText can end before its start
		// column on one line — the clamp keeps the slice from panicking
		lastCol = max(lastCol, first)

		return lines[start][first:lastCol]
	}

	var b strings.Builder

	b.WriteString(lines[start][first:])

	for i := start + 1; i < end; i++ {
		b.WriteString("\n")
		b.WriteString(lines[i])
	}

	b.WriteString("\n")
	b.WriteString(lines[end][:lastCol])

	return b.String()
}

// blankCode returns the lines with /* */ and // comment bytes replaced by
// spaces: every extent stays valid, and a line's first word is then the
// code's first word even on a fully commented-out line.
func blankCode(lines []string) []string {
	out := make([]string, len(lines))

	inBlock := false

	for i, raw := range lines {
		line := []byte(raw)

		inString := false

		for j := 0; j < len(line); j++ {
			switch {
			case inString:
				if line[j] == '"' {
					inString = false
				}
			case inBlock:
				// quotes inside a block comment are comment bytes, not
				// string delimiters
				if line[j] == '*' && j+1 < len(line) && line[j+1] == '/' {
					line[j], line[j+1] = ' ', ' '
					inBlock = false
					j++
				} else {
					line[j] = ' '
				}
			case line[j] == '/' && j+1 < len(line) && line[j+1] == '*':
				line[j], line[j+1] = ' ', ' '
				inBlock = true
				j++
			case line[j] == '/' && j+1 < len(line) && line[j+1] == '/':
				for k := j; k < len(line); k++ {
					line[k] = ' '
				}

				j = len(line)
			case line[j] == '"':
				inString = true
			}
		}

		out[i] = string(line)
	}

	return out
}

// element is one printable unit of a position-ordered stream: a named
// section with its statements, a single statement of the synthetic
// "global" section, an include directive or an embedded XS block.
type element struct {
	kind    string
	section rms.Section
	stmt    rms.Statement
	include rms.Include
	block   rms.XsBlock
	inner   []element // a section's merged content: statements and directives

	start common.Pos // where the unit begins — the stream's sort key
	end   common.Pos // where its AST extent ends
	rawTo common.Pos // past this the unit's source bytes are printed as-is
}

// elements flattens the file's printable surface: named sections keep
// their statements plus the directives written inside their span, the
// synthetic "global" section dissolves into the stream, and the
// remaining directives and XS blocks join by position — file order is
// the order of range starts.
func (p *printer) elements(file rms.RmsFile) []element {
	directives := p.directives(file)

	var out []element

	for _, sec := range file.Sections {
		if sec.Name == "global" {
			out = append(out, statementElements(sec.Statements)...)
			continue
		}

		el := element{
			kind:    elemSection,
			section: sec,
			start:   sec.Range.Start,
			end:     sec.Range.End,
		}
		el.inner, directives = absorb(sec, directives)
		out = append(out, el)
	}

	out = append(out, directives...)
	slices.SortStableFunc(out, byStart)

	return out
}

// directives builds the include and XS-block units that carry their own
// lines, position-ordered: an include whose directive line opens an XS
// block is left out — its bytes travel inside the block's verbatim print.
// Both kinds start at column 0 of their directive line, not at the AST's
// argument or block position: a comment written on the directive line
// travels inside the line's own verbatim print, and anchoring it as a
// standalone line first would print it twice.
func (p *printer) directives(file rms.RmsFile) []element {
	var out []element

	for _, inc := range file.Includes {
		out = p.appendInclude(out, inc, file.XsBlocks)
	}

	for _, inc := range file.XsIncludes {
		out = p.appendInclude(out, inc, file.XsBlocks)
	}

	for _, block := range file.XsBlocks {
		rawTo := block.Range.End
		if dir := p.doc.lineEnd(dirLine(block)); rawTo.Before(dir) {
			rawTo = dir
		}

		out = append(out, element{
			kind:  elemXsBlock,
			block: block,
			start: common.Pos{Line: uint32(dirLine(block)), Column: 0},
			end:   block.Range.End,
			rawTo: rawTo,
		})
	}

	slices.SortStableFunc(out, byStart)

	return out
}

// dirLine is the line an XS block's #includeXS directive sits on. A
// block with content starts on the line after its directive, at column
// 0; the parser's EOF fallback for a directive-only block — bare
// #includeXS as the file's last line, no newline after it — starts on
// the directive line itself, at a column past its text.
func dirLine(block rms.XsBlock) int {
	if block.Range.Start.Column > 0 {
		return int(block.Range.Start.Line)
	}

	return int(block.Range.Start.Line) - 1
}

// absorb splits the directives into those written inside sec's span and
// the rest, merging the inside ones with the section's statements into
// one position-ordered list: a directive between a section's statements
// prints there, not after the closing tag.
func absorb(sec rms.Section, directives []element) (inner, rest []element) {
	inner = statementElements(sec.Statements)
	rest = make([]element, 0, len(directives))

	for _, dir := range directives {
		if sec.Range.Start.Before(dir.start) && dir.start.Before(sec.Range.End) {
			inner = append(inner, dir)
			continue
		}

		rest = append(rest, dir)
	}

	slices.SortStableFunc(inner, byStart)

	return inner, rest
}

// byStart orders stream elements by position: the file's own order.
func byStart(a, b element) int {
	switch {
	case a.start.Before(b.start):
		return -1
	case b.start.Before(a.start):
		return 1
	default:
		return 0
	}
}

// appendInclude adds one directive to the stream — unless its line opens
// an XS block: the directive's bytes travel inside the block's verbatim
// print, so printing the include too would duplicate the line. The
// element starts at column 0 of the directive line: a comment on that
// line rides inside it (directives print as written) instead of
// anchoring as its own line first.
func (p *printer) appendInclude(out []element, inc rms.Include, blocks []rms.XsBlock) []element {
	for _, block := range blocks {
		if dirLine(block) == int(inc.Range.Start.Line) {
			return out
		}
	}

	return append(out, element{
		kind:    elemInclude,
		include: inc,
		start:   common.Pos{Line: inc.Range.Start.Line, Column: 0},
		end:     inc.Range.Start,
		rawTo:   p.doc.lineEnd(int(inc.Range.Start.Line)),
	})
}

// statementElements wraps statements into stream elements.
func statementElements(stmts []rms.Statement) []element {
	out := make([]element, 0, len(stmts))

	for _, stmt := range stmts {
		out = append(out, element{
			kind:  elemStatement,
			stmt:  stmt,
			start: stmt.Range.Start,
			end:   stmt.Range.End,
		})
	}

	return out
}

// printer walks one RmsFile and accumulates the output lines. It lives for
// a single RMS call — no state outlives it.
type printer struct {
	doc  *source
	eol  string
	tabs bool
	size int

	out      []string
	comments []common.Range
	pending  int // index of the next comment not yet anchored
	// rawClosed holds, keyed by the construct's start, every chain-last
	// closer a raw slice already put on the page; closer() spends the
	// matching entry skipping that one synthesis. Addressed, not counted:
	// the replay can close scopes no live element owns — the parser's
	// brace tracking diverges from the line replay on a glued `word{` —
	// and a counted credit would leak into someone else's synthesis.
	rawClosed map[uint64]bool
	chainLast map[uint64]bool // conditionals ending their chain, by lineCol key
	xsMask    []bool          // lines the rms parser never reads: embedded XS
}

// result joins the printed lines with the input's dominant EOL and ends
// the document with exactly one of them.
func (p *printer) result() string {
	if len(p.out) == 0 {
		return ""
	}

	return strings.Join(p.out, p.eol) + p.eol
}

// at renders one nesting level: a tab each, or TabSize spaces.
func (p *printer) at(level int) string {
	if p.tabs {
		return strings.Repeat("\t", level)
	}

	return strings.Repeat(" ", level*p.size)
}

// line emits one output line at a nesting level.
func (p *printer) line(level int, text string) {
	p.out = append(p.out, p.at(level)+text)
}

// emit adds a chunk that carries its own bytes: verbatim regions keep
// their interior newlines and indentation untouched.
func (p *printer) emit(text string) {
	p.out = append(p.out, text)
}

// separate puts the one blank line the top level keeps between its
// elements; nothing inside a section or block is ever padded.
func (p *printer) separate() {
	if len(p.out) > 0 {
		p.out = append(p.out, "")
	}
}

// eolCut is a raw line's last column in the coordinates the parser
// addresses: the parser folds "\r\n" to "\n" and so eats exactly one "\r"
// before every "\n" — a line's own ending byte is the one column the
// parser can never name.
func eolCut(line string) int {
	if strings.HasSuffix(line, "\r") {
		return len(line) - 1
	}

	return len(line)
}

// commentText cuts one comment's bytes for the page. A comment can end at
// a line boundary — a zero-column end names the previous line's tail —
// and always in the parser's normalized coordinates, so the end moves to
// that line's last column first, its own ending byte included: one short
// of the raw tail is the farthest the cut can reach without growing the
// CR run — a byte a pass, and the document never reaches a fixed point.
// Whatever trails the range on its own line is then a run of "\r" bytes,
// and all but one ride with the comment, the page adding the last one.
// A run broken by code (`/* c */ code`) is not the comment's to take.
func (p *printer) commentText(r common.Range) string {
	if r.End.Column == 0 && r.End.Line > 0 && int(r.End.Line) < len(p.doc.lines) {
		r.End = common.Pos{Line: r.End.Line - 1, Column: uint32(eolCut(p.doc.lines[r.End.Line-1]))}
	}

	text := p.doc.text(r)

	if int(r.End.Line) >= len(p.doc.lines) {
		return text
	}

	line := p.doc.lines[r.End.Line]
	rest := line[min(int(r.End.Column), len(line)):]

	if rest != "" && strings.Trim(rest, "\r") == "" {
		return text + rest[:len(rest)-1]
	}

	return text
}

// flushBefore anchors every comment starting before pos, in source order,
// one per line at the given level: the nearest inter-node position a
// comment can take is the gap it already sits in.
func (p *printer) flushBefore(pos common.Pos, level int) {
	for p.pending < len(p.comments) && p.comments[p.pending].Start.Before(pos) {
		p.line(level, p.commentText(p.comments[p.pending]))
		p.pending++
	}
}

// dropBefore skips the comments starting before pos: their bytes travel
// inside a region printed verbatim, and anchoring them would print them
// twice.
func (p *printer) dropBefore(pos common.Pos) {
	for p.pending < len(p.comments) && p.comments[p.pending].Start.Before(pos) {
		p.pending++
	}
}

// list prints a position-ordered element list at one nesting level. limit
// is where the list's content stops: a verbatim slice never reaches past
// it, and comments trailing the last element anchor before it. sep asks
// for the top-level blank line between elements.
func (p *printer) list(elems []element, level int, limit common.Pos, sep bool) {
	for i := 0; i < len(elems); {
		if sep {
			p.separate()
		}

		el := elems[i]
		p.flushBefore(el.start, level)

		printed, raw := p.element(el, level, p.cut(elems, i, limit))

		i++

		if raw {
			// elements starting inside a raw unit's bytes are already on
			// the page — the brace-less create_* covering a sibling if is
			// the corpus case
			for i < len(elems) && elems[i].start.Before(printed) {
				i++
			}
		}

		p.closer(el, level)
	}

	// a trailing run at the top level keeps the blank line the level's
	// rule gives it: one to the left, none inside a section or block
	if sep && p.pendingBefore(limit) {
		p.separate()
	}

	p.flushBefore(limit, level)
}

// pendingBefore reports whether a not-yet-anchored comment starts before
// pos.
func (p *printer) pendingBefore(pos common.Pos) bool {
	return p.pending < len(p.comments) && p.comments[p.pending].Start.Before(pos)
}

// cut returns where a verbatim slice of elems[i] must stop: the first
// live element start at or past its extent, or the list's limit — the
// slice ends where the next unit begins, never inside the extent itself.
func (p *printer) cut(elems []element, i int, limit common.Pos) common.Pos {
	el := elems[i]

	for j := i + 1; j < len(elems); j++ {
		if !elems[j].start.Before(el.end) {
			return elems[j].start
		}
	}

	if limit.Before(el.end) {
		return el.end
	}

	return limit
}

// element prints one stream unit and reports how far it consumed. Raw
// units report their rawTo — everything starting inside those bytes is
// printed by the unit itself.
func (p *printer) element(el element, level int, limit common.Pos) (common.Pos, bool) {
	switch el.kind {
	case elemSection:
		p.section(el, level, limit)
	case elemStatement:
		if p.verbatim(el.stmt) {
			p.emit(p.at(level) + rawChunk(trimBlankLines(p.doc.text(ranges(el.stmt.Range.Start, limit)))))
			p.dropBefore(limit)
			maps.Copy(p.rawClosed, p.rawClosers(el.stmt.Range.Start, limit))

			return limit, true
		}

		p.statement(el.stmt, level, limit)
	case elemInclude:
		p.line(level, p.directiveLine(int(el.include.Range.Start.Line)))
		p.dropBefore(el.rawTo)

		return el.rawTo, true
	case elemXsBlock:
		p.xsBlock(el.block, level)
		p.dropBefore(el.rawTo)

		return el.rawTo, true
	}

	return common.Pos{}, false
}

// ranges builds a source range from start to end.
func ranges(start, end common.Pos) common.Range {
	return common.Range{Start: start, End: end}
}

// section prints a named section: header, its merged inner stream one
// level in, closing tag. Its own range runs to the next header, so it
// bounds the content — a caller's tighter limit wins.
func (p *printer) section(el element, level int, limit common.Pos) {
	sec := el.section
	p.line(level, "<"+sec.Name+">")

	inner := sec.Range.End
	if limit.Before(inner) {
		inner = limit
	}

	p.list(el.inner, level+1, inner, false)
	p.line(level, "</"+sec.Name+">")
}

// statement prints one statement canonically: the header line with its
// positional arguments, attributes one per line inside braces that
// always print (create_* accepts brace-less blocks, every other command
// loses its attributes without them on a re parse), nested
// random/conditional blocks recursively with their closers rebuilt.
func (p *printer) statement(stmt rms.Statement, level int, limit common.Pos) {
	head := stmt.Name

	for _, arg := range stmt.Args {
		head += " " + p.expr(arg, 1)
	}

	if len(stmt.Attributes) > 0 {
		head += " {"
	}

	p.line(level, head)

	for _, attr := range stmt.Attributes {
		// a comment written in the gap before an attribute line anchors
		// there — after the command line, or between two attributes
		p.flushBefore(attr.Range.Start, level+1)
		p.line(level+1, p.attribute(attr))
	}

	if len(stmt.Attributes) > 0 {
		// a trailing comment belongs inside the braces, before the closer
		p.flushBefore(stmt.Range.End, level+1)
		p.line(level, "}")
	}

	if len(stmt.Children) > 0 {
		p.list(statementElements(stmt.Children), level+1, stmt.Range.End, false)
	}

	// childless blocks keep their header-line comment
	p.flushBefore(stmt.Range.End, level+1)
}

// closer rebuilds the block closer the AST does not carry: the parser
// drops end_random/endif lines, so every start_random gets an end_random
// and the last element of an if chain gets one endif. Which conditional
// is a chain's last one cannot be read off the statement list — chains
// cross section headers — so the answer comes from the chainLast replay.
// A closer a raw slice has already put on the page — a { } block reaching
// past its chain's endif, a warning end_random finishing an if together
// with its start_random — is skipped: the construct's start key is spent
// from rawClosed, and an unsuppressed synthesis always prints.
func (p *printer) closer(el element, level int) {
	last := el.stmt.Name == "start_random" ||
		(el.stmt.Kind == rms.KindConditional && p.chainLast[lineCol(el.stmt.Range.Start)])
	if !last {
		return
	}

	at := lineCol(el.stmt.Range.Start)

	if p.rawClosed[at] {
		delete(p.rawClosed, at)

		return
	}

	if el.stmt.Name == "start_random" {
		p.line(level, "end_random")

		return
	}

	p.line(level, "endif")
}

// chainLast returns, keyed by start position, every conditional statement
// that ends its chain and every start_random — the elements whose dropped
// closer the printer must rebuild. The answer cannot be read off the
// statement lists, least of all for chains that cross section headers, so
// it comes from replaying the whole file's structural lines with the
// parser's scope discipline. percent_chance branches never carry one.
func chainLast(blanked []string, mask []bool) map[uint64]bool {
	res := replayChains(blanked, 0, len(blanked)-1, func(i int) bool { return mask[i] }, replaySeed{})

	for _, at := range res.open {
		res.ended[at] = true // never closed — warning inputs close anyway
	}

	return res.ended
}

// replayChains walks blanked[from..to] with the parser's own scope
// discipline (internal/rms/parse.go): braces shield their contents, a new
// percent_chance closes its sibling silently, and endif/end_random — or an
// elseif/else branch — close every inner scope up to the construct of
// their own kind, exactly like closeScopes does. seed plants the scopes
// and brace depth standing where the walk begins. ended collects the
// scopes a closer or a branch switch finished; open holds the ones still
// standing at the walk's end (percent_chance aside).
func replayChains(blanked []string, from, to int, skip func(int) bool, seed replaySeed) (res replayResult) {
	w := replay{ended: make(map[uint64]bool), closed: make(map[uint64]bool), seed: len(seed.stack)}
	w.stack = append(w.stack, seed.stack...)
	depth := seed.depth

	for i := from; i <= to && i < len(blanked); i++ {
		if skip != nil && skip(i) {
			continue
		}

		fields := strings.Fields(blanked[i])
		opened, closed := braceDelta(fields)

		// the leading word is cut by the lexer's rules (rms.FirstWord):
		// `if}` and `else{` are the words "if" and "else" to the parser,
		// while a Fields split would see one glued token and lose the
		// scope line
		word := rms.FirstWord(blanked[i])

		if word != "" && depth == 0 {
			key := lineCol(common.Pos{Line: uint32(i), Column: uint32(fieldStart(blanked[i]))})

			switch word {
			case "if":
				w.push(chainFrame{at: key, kind: 'c'})
			case "start_random":
				w.push(chainFrame{at: key, kind: 'r'})
			case "percent_chance":
				if len(w.stack) > 0 && w.stack[len(w.stack)-1].kind == 'p' {
					w.pop() // a sibling branch ends silently — no closer, no mark
				}

				w.push(chainFrame{at: key, kind: 'p'})
			case "elseif", "else":
				// the branch closes the inner scopes; the conditional it
				// continues stays open — its chain goes on unmarked
				w.closeScopes('c', false)
				w.push(chainFrame{at: key, kind: 'c'})
			case "endif", "end_random":
				stop := byte('c')
				if word == "end_random" {
					stop = 'r'
				}

				w.closeScopes(stop, true)
			}
		}

		depth = max(depth+opened-closed, 0)
	}

	res = replayResult{
		ended:      w.ended,
		stack:      w.stack,
		depth:      depth,
		closedSeed: w.closed,
	}

	for _, frame := range w.stack {
		if frame.kind != 'p' {
			res.open = append(res.open, frame.at)
		}
	}

	return res
}

// replaySeed plants a replay where the source left it off: the scopes
// still standing and the brace depth around them. A zero seed starts
// clean.
type replaySeed struct {
	stack []chainFrame
	depth int
}

// replayResult is what one structural replay learned about the scopes.
type replayResult struct {
	ended      map[uint64]bool // scopes a closer or a branch switch finished
	open       []uint64        // scopes never closed by the walk, percent_chance aside
	stack      []chainFrame    // scopes still standing at the walk's end
	depth      int             // brace depth at the walk's end
	closedSeed map[uint64]bool // planted scopes the walk finished, percent aside, keyed by start
}

// replay is the scope state of one structural walk.
type replay struct {
	stack  []chainFrame
	seed   int             // frames planted where the walk began, at the stack's bottom
	closed map[uint64]bool // planted scopes the walk finished, percent aside, keyed by start
	ended  map[uint64]bool // scopes a closer or a branch switch finished
}

// push opens one scope inside the walk — above every planted frame.
func (w *replay) push(frame chainFrame) {
	w.stack = append(w.stack, frame)
}

// pop drops the top scope, keeping the planted count true: it shrinks
// with the stack while the bottom plants pop, so a frame the walk itself
// opened never reads as planted.
func (w *replay) pop() (frame chainFrame, planted bool) {
	planted = len(w.stack) <= w.seed
	if planted {
		w.seed--
	}

	frame = w.stack[len(w.stack)-1]
	w.stack = w.stack[:len(w.stack)-1]

	return frame, planted
}

// closeScopes pops inner scopes until one of kind stop pops too, exactly
// like the parser's own closeScopes (internal/rms/parse.go): every popped
// non-percent scope lands in ended — markStop says whether the stopped
// one does (a closer ends its construct, a branch word continues it) —
// and a planted one also counts into closed: the walk just finished a
// scope that was standing when it began.
func (w *replay) closeScopes(stop byte, markStop bool) {
	for len(w.stack) > 0 {
		frame, planted := w.pop()

		if frame.kind == stop {
			if markStop {
				w.finish(frame, planted)
			}

			return
		}

		w.finish(frame, planted)
	}
}

// finish records one popped scope: chain-last in ended, and keyed into
// closed when planted. percent_chance branches carry no closer of their
// own and never count.
func (w *replay) finish(frame chainFrame, planted bool) {
	if frame.kind == 'p' {
		return
	}

	w.ended[frame.at] = true

	if planted {
		w.closed[frame.at] = true
	}
}

// chainFrame is one nesting scope of the structural replay.
type chainFrame struct {
	at   uint64
	kind byte // 'c' conditional, 'r' start_random, 'p' percent_chance
}

// lineCol keys a position by its line and column alone: AST positions
// also carry offsets the replay cannot know, and a struct key would
// compare them too.
func lineCol(pos common.Pos) uint64 {
	return uint64(pos.Line)<<32 | uint64(pos.Column)
}

// fieldStart returns the column of a line's first non-blank byte — the
// position the parser records as the statement's start.
func fieldStart(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

// xsLineMask marks the lines the rms parser never reads: embedded XS
// code, whose C braces and ifs would corrupt the replay.
func xsLineMask(lines []string, blocks []rms.XsBlock) []bool {
	mask := make([]bool, len(lines))

	for _, b := range blocks {
		for i := int(b.Range.Start.Line); i <= int(b.Range.End.Line) && i < len(mask); i++ {
			mask[i] = true
		}
	}

	return mask
}

// rawClosers names, by their start, the enclosing chain-last constructs a
// raw slice's closers finish: the slice prints source bytes, and an
// endif/end_random inside them can close the constructs wrapping the raw
// statement — a warning input's end_random finishes an inner if and its
// start_random at once. The scopes standing around the statement come
// from replaying the file before it; every planted scope the slice
// finishes is one closer() synthesis the page already carries, and the
// key makes the credit addressed — a scope no live element owns (the
// replay's line view diverges from the parser's brace tracking on a
// glued `word{`) can never spend another construct's synthesis.
// percent_chance branches carry no closer and never count. Frames are
// named, not closer lines counted: one line may finish several
// constructs, and every one of them stands covered.
func (p *printer) rawClosers(from, to common.Pos) map[uint64]bool {
	last := int(to.Line)
	if to.Column == 0 && last > int(from.Line) {
		last--
	}

	skip := func(i int) bool { return p.xsMask[i] }
	around := replayChains(p.doc.blanked, 0, int(from.Line)-1, skip, replaySeed{})

	return replayChains(p.doc.blanked, int(from.Line), last, skip,
		replaySeed{stack: around.stack, depth: around.depth}).closedSeed
}

// braceDelta counts a line's "{" and "}" tokens.
func braceDelta(fields []string) (opened, closed int) {
	for _, f := range fields {
		switch f {
		case "{":
			opened++
		case "}":
			closed++
		}
	}

	return opened, closed
}

// verbatim reports whether a command prints as its source bytes: its
// extent holds a line whose first word is a structural keyword. Inside a
// { } block the parser tracks those without materializing nodes, and a
// brace-less create_* covers a sibling if — both are invisible to the AST,
// so the only faithful print is the source itself. The keyword set is the
// parser's own (rms.IsStructural) — a private copy here would drift the
// day the grammar grows.
func (p *printer) verbatim(stmt rms.Statement) bool {
	if stmt.Kind != rms.KindCommand {
		return false
	}

	last := int(stmt.Range.End.Line)
	if stmt.Range.End.Column == 0 {
		last-- // a zero-column end sits on the previous line
	}

	for i := int(stmt.Range.Start.Line); i <= last && i < len(p.doc.blanked); i++ {
		// the word is cut by the lexer's rules: `if}` opens a scope the
		// parser sees, and a Fields split would miss it
		if rms.IsStructural(rms.FirstWord(p.doc.blanked[i])) {
			return true
		}
	}

	return false
}

// xsBlock prints an embedded XS block: the #includeXS directive line as
// written — the block's range starts on the line after it — and the code
// verbatim. The rms parser does not read XS, so the bytes are not the
// printer's to normalize.
func (p *printer) xsBlock(block rms.XsBlock, level int) {
	p.line(level, p.directiveLine(dirLine(block)))

	if code := rawChunk(p.doc.text(block.Range)); code != "" {
		p.emit(code)
	}
}

// rawChunk prepares one verbatim slice for the page: its extent ends at
// column 0 of a following line, which cuts the whole previous raw line —
// "\r" tail included — and the page adds its own line ending, so the one
// trailing "\r" is dropped to keep CRLF files free of stray bytes.
func rawChunk(text string) string {
	return strings.TrimSuffix(text, "\r")
}

// directiveLine cuts one whole source line: include directives print as
// written, quotes and all — the AST keeps only the argument range, and
// re-quoting it would guess at the original spelling.
func (p *printer) directiveLine(line int) string {
	if line < 0 || line >= len(p.doc.lines) {
		return ""
	}

	return strings.TrimSpace(p.doc.lines[line])
}

// attribute prints one attribute line: name and first value from the AST,
// the values past it cut from the source — the AST keeps only the first,
// and the rest are data the printer must not lose. A name starting with
// '#' is the one spelling that cannot round-trip: only the single-line
// `{ #x 1 }` form lexes into an attribute, and the printed multi-line
// form re-parses as a plain #-comment (issue #95) — RMS attribute names
// never start with '#'.
func (p *printer) attribute(attr rms.Attribute) string {
	out := attr.Name

	if attr.Value.Kind != "" {
		out += " " + p.expr(attr.Value, 1)

		if tail := p.tail(attr); tail != "" {
			out += " " + tail
		}
	}

	return out
}

// tail reads the values an attribute carries past its first: the source
// cut between the first value's end and the attribute's end, comments
// blanked out, whitespace collapsed to single spaces. A leading ")" is
// dropped with it — the AST cut a parenthesized first value out of its
// parentheses, so the closing one has nothing left to close.
func (p *printer) tail(attr rms.Attribute) string {
	if !attr.Value.Range.End.Before(attr.Range.End) {
		return ""
	}

	fields := strings.Fields(p.doc.blankedText(ranges(attr.Value.Range.End, attr.Range.End)))
	kept := fields[:0]

	for _, field := range fields {
		field = strings.TrimLeft(field, ")")
		if field != "" && field != "," {
			kept = append(kept, field)
		}
	}

	return strings.Join(kept, " ")
}

// bindPower is the binding power of a binary operator, the parser's own
// ordering: additive, then multiplicative.
func bindPower(op string) int {
	switch op {
	case "+", "-":
		return 1
	case "*", "/":
		return 2
	default:
		return 1
	}
}

// expr prints one expression: leaves are source cuts — string literals
// keep their quotes, percents their sign — operators print canonically,
// and a child binding weaker than its context is parenthesized, so
// (1 + 2) * 3 keeps the parentheses the AST dropped.
func (p *printer) expr(e rms.Expr, minPrec int) string {
	switch e.Kind {
	case rms.KindBinary:
		prec := bindPower(e.Value)

		out := p.expr(e.Children[0], prec) + " " + e.Value + " " + p.expr(e.Children[1], prec+1)
		if prec < minPrec {
			return "(" + out + ")"
		}

		return out
	case rms.KindUnary:
		// a failed operand parse leaves the unary without children and
		// without an error diagnostic — the operator token is all the AST
		// ever knew, and its source bytes are the faithful print
		if len(e.Children) == 0 {
			return p.doc.text(e.Range)
		}

		// the operand binds tighter than any binary operator, mirroring
		// the parser's own minimum
		return e.Value + p.expr(e.Children[0], 3)
	}

	if len(e.Children) > 0 {
		parts := make([]string, 0, len(e.Children))

		for _, child := range e.Children {
			parts = append(parts, p.expr(child, 1))
		}

		return e.Value + "(" + strings.Join(parts, ", ") + ")"
	}

	// a leaf's bytes are its source cut. The zero-width leaves of the
	// parser's error recovery (a failed operand swallowed without a
	// diagnostic, issue #96) print as nothing — their bytes were never
	// recorded anywhere the AST reaches
	return p.doc.text(e.Range)
}

// trimBlankLines drops a raw chunk's trailing blank lines: the slice
// between two statements often spans the gap around the next one, and the
// canonical style owns blank lines.
func trimBlankLines(text string) string {
	lines := strings.Split(text, "\n")

	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n")
}
