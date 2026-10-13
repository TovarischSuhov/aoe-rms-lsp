package format

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/xs"
	"slices"
	"strings"
)

// XS formats an XS source into the cell's canonical style: brace
// placement, indentation and blank lines are rebuilt from the AST, token
// text comes from the source so string literals keep their quotes and a
// parameter list rides its verbatim region — default values are parsed
// and discarded by the parser, so only their bytes survive. The output
// keeps the input's dominant line ending. Inputs with error-severity
// diagnostics are refused — ErrParseErrors, formatted empty.
func XS(source string, opts Options) (string, error) {
	// the file name reaches no output — diagnostics are folded into the
	// refusal error below
	file, diags := xs.XsParse(source, "")
	if err := refuse(diags); err != nil {
		return "", err
	}

	p := &xsPrinter{
		src:  source,
		eol:  dominantEOL(source),
		tabs: opts.IndentTabs,
		size: indentWidth(opts.TabSize),
	}
	p.comments = file.Comments
	// a wrong-arity vector is the one warning the parser issues, and its
	// range covers the collapsed region: the statement — or the
	// declaration initializer — carrying one prints as its own bytes, the
	// AST keeps only the first operand
	for _, d := range diags {
		if d.Severity == common.SeverityWarning {
			p.warns = append(p.warns, d.Range)
		}
	}

	p.decls(file.Decls)

	// Whatever is still pending here is unclosed-to-EOF: every flush left
	// it for this anchor, so it prints past every closer synthesized above
	// — the one position its extent cannot swallow on a re parse (the
	// #126 class of divergence, ported from the rms printer).
	for p.pending < len(p.comments) {
		p.line(0, p.commentText(p.comments[p.pending]))
		p.pending++
	}

	return p.result(), nil
}

// xsPrinter walks one XsFile and accumulates the output lines. It lives
// for a single XS call — no state outlives it. XS coordinates are raw
// byte offsets (the scanner never normalizes line endings), so every
// position here addresses the source by Offset alone.
type xsPrinter struct {
	src  string
	eol  string
	tabs bool
	size int

	out      []string
	comments []common.Range
	warns    []common.Range
	pending  int // index of the next comment not yet anchored
	level    int // nesting level of the statement being printed; expression lifts read it
}

// result joins the printed lines with the input's dominant EOL and ends
// the document with exactly one of them.
func (p *xsPrinter) result() string {
	if len(p.out) == 0 {
		return ""
	}

	return strings.Join(p.out, p.eol) + p.eol
}

// at renders one nesting level: a tab each, or TabSize spaces.
func (p *xsPrinter) at(level int) string {
	if p.tabs {
		return strings.Repeat("\t", level)
	}

	return strings.Repeat(" ", level*p.size)
}

// line emits one output line at a nesting level.
func (p *xsPrinter) line(level int, text string) {
	p.out = append(p.out, p.at(level)+text)
}

// separate puts the one blank line the top level keeps between its
// declarations; nothing inside a body is ever padded.
func (p *xsPrinter) separate() {
	if len(p.out) > 0 {
		p.out = append(p.out, "")
	}
}

// cutBytes slices the raw source between byte offsets, guarding the
// inverted span a recovery-shaped range can carry.
func (p *xsPrinter) cutBytes(from, to int) string {
	if from < 0 || to > len(p.src) || to <= from {
		return ""
	}

	return p.src[from:to]
}

// cut slices one AST range out of the raw source.
func (p *xsPrinter) cut(r common.Range) string {
	return p.cutBytes(r.Start.Offset, r.End.Offset)
}

// commentText cuts one comment's bytes for the page, re-lined with the
// page's ending: the scanner is byte-based, so a comment cut from a
// CRLF document carries "\r\n" interiors that would stray into an
// LF-dominant output, and the other way around. Trailing newline bytes
// are the page's own — the EOF-clipped extent carries the file's final
// ones, and letting them ride would grow the document every pass.
func (p *xsPrinter) commentText(r common.Range) string {
	text := strings.TrimRight(strings.ReplaceAll(p.cut(r), "\r\n", "\n"), "\r\n")
	if p.eol == "\r\n" {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}

	return text
}

// unclosedEOF reports whether r is the comment the scanner clipped at the
// file's end: an unterminated /* — or a // on the file's last line —
// whose extent runs to EOF, so every closer printed after its start is
// swallowed by it on the next parse. The extent's bytes are checked too —
// a "*/" inside means the scanner closed this comment and the EOF end
// came from the file simply ending there.
func (p *xsPrinter) unclosedEOF(r common.Range) bool {
	return r.End.Offset >= len(p.src) && !strings.Contains(p.cut(r), "*/")
}

// flushBefore anchors every comment starting before the byte offset, in
// source order, one per line at the given level: the nearest inter-node
// position a comment can take is the gap it already sits in. An
// unclosed-to-EOF comment stops the walk and stays pending — nothing live
// starts past its extent, and anchoring it here would put it ahead of the
// closers synthesized after this point; the pass's final anchor prints it
// past every synthesis.
func (p *xsPrinter) flushBefore(off int, level int) {
	for p.pending < len(p.comments) && p.comments[p.pending].Start.Offset < off {
		if p.unclosedEOF(p.comments[p.pending]) {
			return
		}

		p.line(level, p.commentText(p.comments[p.pending]))
		p.pending++
	}
}

// dropBefore skips the comments starting before the byte offset: their
// bytes travel inside a verbatim cut printed nearby, and anchoring them
// would print them twice.
func (p *xsPrinter) dropBefore(off int) {
	for p.pending < len(p.comments) && p.comments[p.pending].Start.Offset < off {
		p.pending++
	}
}

// isLineComment reports whether the comment at r is a // one: its first
// bytes are the double slash, where a /* opens a block.
func (p *xsPrinter) isLineComment(r common.Range) bool {
	s := r.Start.Offset

	return s+1 < len(p.src) && p.src[s] == '/' && p.src[s+1] == '/'
}

// liftLine takes the pending comments starting inside [from, to) apart
// from the region's bytes and returns those bytes collapsed: a // comment
// lifts to a standalone line at the given level — collapsed into the cut
// it would swallow everything after it on the reparse — while a block
// comment closes itself and keeps its bytes in the region. Comments still
// pending before from anchor above the line first: the merged print leaves
// them no better home, and leaving them pending would block the region's
// own comments behind them. An unclosed-to-EOF comment stops both walks —
// nothing live starts past its extent.
func (p *xsPrinter) liftLine(from, to, level int) string {
	for p.pending < len(p.comments) && p.comments[p.pending].Start.Offset < from {
		if p.unclosedEOF(p.comments[p.pending]) {
			break
		}

		p.line(level, p.commentText(p.comments[p.pending]))
		p.pending++
	}

	var parts []string
	pos := from

	for p.pending < len(p.comments) {
		r := p.comments[p.pending]
		if r.Start.Offset >= to {
			break
		}

		if r.Start.Offset < from {
			// the unclosed-to-EOF comment the pre-region anchor stopped on;
			// the extents are disjoint, so no region comment hides behind it
			break
		}

		if p.isLineComment(r) {
			p.line(level, p.commentText(r))
			parts = append(parts, collapseWS(p.cutBytes(pos, r.Start.Offset)))
			pos = r.End.Offset
		}

		p.pending++
	}

	parts = append(parts, collapseWS(p.cutBytes(pos, to)))

	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			kept = append(kept, part)
		}
	}

	// a part ending on "(" glues to the next without the join space: that
	// space is an artifact of where the source broke the line — `( float f
	// = 1.5)` would canonize the input's newline, not its tokens
	var b strings.Builder

	for i, part := range kept {
		if i > 0 && !strings.HasSuffix(b.String(), "(") {
			b.WriteByte(' ')
		}

		b.WriteString(part)
	}

	return b.String()
}

// pendingBefore reports whether a not-yet-anchored comment starts before
// the byte offset.
func (p *xsPrinter) pendingBefore(off int) bool {
	return p.pending < len(p.comments) && p.comments[p.pending].Start.Offset < off
}

// xsFind walks the source bytes from from to limit, stepping over
// comments (the file's own extents) and string literals with the
// scanner's own discipline — backslash escapes, an unterminated string
// ends at the newline — and answers at each code byte. found reports
// whether the walk stopped early rather than at the limit.
func (p *xsPrinter) xsFind(from, limit int, at func(i int, c byte) bool) (int, bool) {
	src := p.src
	ci := 0 // first comment not fully passed; the extents are sorted and disjoint

	for i := from; i < limit && i < len(src); i++ {
		// a comment covering i is skipped whole; the jump lands one short
		// of its end so the loop's own step resumes past it — the last
		// comment byte must never be tested as code
		jumped := false

		for ci < len(p.comments) && p.comments[ci].Start.Offset <= i {
			if e := p.comments[ci].End.Offset; e > i {
				i = e - 1
				jumped = true
			}

			ci++
		}

		if jumped {
			continue
		}

		if i >= limit || i >= len(src) {
			break
		}

		c := src[i]

		switch {
		case c == '"':
			for i++; i < len(src) && src[i] != '"' && src[i] != '\n'; i++ {
				if src[i] == '\\' {
					i++
				}
			}
		case at(i, c):
			return i, true
		}
	}

	return limit, false
}

// scanFirst returns the offset of the first of the wanted code bytes at or
// after from — comments and string literals transparent.
func (p *xsPrinter) scanFirst(from int, want ...byte) (int, bool) {
	return p.xsFind(from, len(p.src), func(_ int, c byte) bool {
		return slices.Contains(want, c)
	})
}

// openParenBefore finds a call's opening parenthesis: a backward walk from
// the first argument over whitespace and comment extents to the last code
// byte, which is the "(" itself. The callee — an identifier, a member or
// index chain, an inner call — carries no closing parenthesis, so nothing
// else can be the last code byte before an argument.
func (p *xsPrinter) openParenBefore(argStart int) (int, bool) {
	i, ok := p.codeBefore(argStart)
	if ok && p.src[i] == '(' {
		return i, true
	}

	return 0, false
}

// codeBefore finds the offset of the last code byte before the given one,
// stepping back over whitespace, comment extents and the bytes the
// scanner skips silently (xsTokenStart); false past the source's own
// start.
func (p *xsPrinter) codeBefore(off int) (int, bool) {
	i := off - 1

	for i >= 0 {
		if c := p.src[i]; c == ' ' || c == '\t' || c == '\r' || c == '\n' || !xsTokenStart(c) {
			i--
			continue
		}

		inComment := false

		for _, r := range p.comments {
			if r.Start.Offset <= i && i < r.End.Offset {
				i = r.Start.Offset - 1
				inComment = true

				break
			}
		}

		if inComment {
			continue
		}

		return i, true
	}

	return 0, false
}

// xsTokenStart mirrors the scanner's own token beginnings: the
// identifier, number and string starts, the comment slash, and the
// operator set's bytes. Every other byte the scanner skips silently, so
// a walk over code must step over those too.
func xsTokenStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c >= '0' && c <= '9' || c == '"' ||
		strings.IndexByte("+-*/%=<>!&|^~?:;,(){}[].", c) >= 0
}

// calleeStart finds where a callee cut opens: the collapsed extent starts
// inside the groupings the parser drops, so a cut from the extent would
// keep their ")" bytes while losing their "(". Two shapes carry one: a
// cut whose own brackets net negative — every unmatched closer pairs
// with the "(" before the start — and a "(" sitting right before the
// start with its match past it, the group wrapping the operand the
// extent opens on; a degenerate member or argument token can hold the
// openers that cancel the net count. Both walk back one loop each, never
// past a ";" or brace: the hard bound recovery junk cannot cross.
func (p *xsPrinter) calleeStart(start, to int) int {
	depth := 0

	_, _ = p.xsFind(start, to, func(_ int, c byte) bool {
		switch c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		}

		return false
	})

	for {
		b, ok := p.codeBefore(start)
		if !ok {
			return start
		}

		switch p.src[b] {
		case ';', '{', '}':
			return start
		case ')', ']':
			if depth >= 0 {
				return start
			}

			depth--
			start = b
		case '(', '[':
			if depth < 0 {
				depth++
				start = b

				continue
			}

			if p.src[b] != '(' {
				return start
			}

			closer, ok := p.matchBracket(b, '(', ')')
			if !ok || closer <= start {
				return start
			}

			start = b
		default:
			return start
		}
	}
}

// zeroCallParen finds a zero-argument call's opening parenthesis: the
// first code "(" at or after the call's extent end — the extent stops at
// the callee, so the call's own parens lie past it. The callee slice up
// to that "(" must not open more brackets than it closes (comments and
// strings transparent): a positive balance cannot come from an accepted
// call, while a negative one is the grouping a grouped callee carries —
// calleeStart walks the cut's start back over it. The "(" must also
// close: an accepted parse carries its match.
func (p *xsPrinter) zeroCallParen(e xs.Expr) (int, bool) {
	from := e.Range.End.Offset

	open, ok := p.scanFirst(from, '(')
	if !ok {
		return 0, false
	}

	depth := 0
	_, _ = p.xsFind(e.Range.Start.Offset, open, func(_ int, c byte) bool {
		switch c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		}

		return false
	})
	if depth > 0 {
		return 0, false
	}

	if _, ok := p.matchBracket(open, '(', ')'); !ok {
		return 0, false
	}

	return open, true
}

// bodyBlock prints a top-level head line sharing its brace with the block
// at open: contents one level in, the comments ahead of the closer
// anchored inside, then the closer.
func (p *xsPrinter) bodyBlock(head string, open int, body []xs.Stmt) {
	p.line(0, head+" {")
	p.flushBefore(open, 1)
	p.stmts(body, 1)
	if closer, ok := p.matchBracket(open, '{', '}'); ok {
		p.flushBefore(closer, 1)
	}

	p.line(0, "}")
}

// matchBracket returns the offset of the bracket closing the one at from:
// depth counting with comments and strings transparent. The walk starts
// on the opening bracket itself, so the matching closer is the one that
// brings the depth back to zero. A missing closer answers found=false —
// accepted parses always carry theirs.
func (p *xsPrinter) matchBracket(from int, openB, closeB byte) (int, bool) {
	depth := 0

	return p.xsFind(from, len(p.src), func(_ int, c byte) bool {
		switch c {
		case openB:
			depth++
		case closeB:
			depth--
			if depth == 0 {
				return true
			}
		}

		return false
	})
}

// collapseWS flattens whitespace outside string literals to single
// spaces, dropping a trailing run: a verbatim cut stays on its one line
// while its tokens keep their spacing byte-exact (`= -1` in a default
// parameter). A cut whose tokens are always separated by at least one
// whitespace byte loses nothing.
func collapseWS(src string) string {
	var b strings.Builder

	inStr := false
	esc := false
	gap := false

	for i := 0; i < len(src); i++ {
		c := src[i]

		if inStr {
			b.WriteByte(c)

			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}

			continue
		}

		switch c {
		case '"':
			if gap {
				b.WriteByte(' ')
				gap = false
			}

			inStr = true
			b.WriteByte(c)
		case ' ', '\t', '\r', '\n':
			gap = true
		default:
			if gap {
				b.WriteByte(' ')
				gap = false
			}

			b.WriteByte(c)
		}
	}

	return b.String()
}

// blockText cuts a verbatim region that may span lines: horizontal
// whitespace collapses per line, the line breaks survive re-lined with the
// page's ending — a // comment inside the region must keep its newline or
// it swallows the rest of the cut on the reparse. A lone trailing \r is
// the page-ending artifact an unterminated literal carries out of a CRLF
// document — the scanner stops the token at the \n, the \r before it
// rides inside the bytes; the page's own ending closes the line, so the
// artifact goes — an interior one is the literal's own content and stays.
func (p *xsPrinter) blockText(from, to int) string {
	text := strings.TrimRight(strings.ReplaceAll(p.cutBytes(from, to), "\r\n", "\n"), "\r")
	lines := strings.Split(text, "\n")

	for i := range lines {
		lines[i] = collapseWS(lines[i])
	}

	out := strings.Join(lines, "\n")
	if p.eol == "\r\n" {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}

	return out
}

// lineEnd returns the offset of the line terminator ending the line that
// runs through from, or the file's end for the last line; false when from
// sits past the source itself.
func (p *xsPrinter) lineEnd(from int) (int, bool) {
	if from > len(p.src) {
		return 0, false
	}

	if i := strings.IndexAny(p.src[from:], "\r\n"); i >= 0 {
		return from + i, true
	}

	return len(p.src), true
}

// sectionColon reports whether a rule section statement takes the colon
// form: the first code byte past its keyword is the colon, not the block
// brace — the parser only enters the section on one of the two, so the
// bytes settle what the empty-body extents leave open.
func (p *xsPrinter) sectionColon(s xs.Stmt) bool {
	shape, ok := p.scanFirst(s.Range.Start.Offset, '{', ':')

	return ok && p.src[shape] == ':'
}

// bareBlock cuts a bare block expression's bytes whole: the parser
// tolerates the block without materializing its statements, so the region
// is the only carrier of them — and of the comments inside, which the
// global stream must not anchor again. The extent stops at the opening
// brace, so the closer is matched from the bytes. Comments pending before
// the brace anchor above the line first: the slice starts on the brace
// itself, so the drop would bury them with no cut to carry.
func (p *xsPrinter) bareBlock(e xs.Expr) string {
	closer, ok := p.matchBracket(e.Range.Start.Offset, '{', '}')
	if !ok {
		return e.Value
	}

	p.flushBefore(e.Range.Start.Offset, p.level)

	end := closer + 1
	p.dropBefore(end)

	return p.blockText(e.Range.Start.Offset, end)
}

// warnSlice reports where a verbatim print of the statement must open:
// the first parser warning overlapping the statement's extent, clamped to
// the statement's own start. A wrong-arity vector's warning opens on the
// `(` while the collapsed extent starts at the first operand, so the gap
// between them — comments included — belongs to the cut. Declaration
// initializers consult the same warnings through warnInit.
func (p *xsPrinter) warnSlice(s xs.Stmt) (int, bool) {
	if len(p.warns) == 0 || len(s.Exprs) == 0 || s.Range.End.Offset <= s.Range.Start.Offset {
		return 0, false
	}

	for _, w := range p.warns {
		if w.Start.Offset >= s.Range.End.Offset || w.End.Offset <= s.Range.Start.Offset {
			continue
		}

		from := s.Range.Start.Offset
		if w.Start.Offset < from {
			from = w.Start.Offset
		}

		return from, true
	}

	return 0, false
}

// warnCutEnd extends a warning-carrying statement's extent to where its
// verbatim cut must stop: compound statements (if/else chains, while, for,
// switch) end their extent at the last nested statement, so every brace
// the parser closed after it is outside the extent while the statement's
// bytes run through it. The unmatched opens are counted over the cut's own
// code bytes and closed one closer at a time; a source that runs out first
// keeps the extent's end.
func (p *xsPrinter) warnCutEnd(from int, s xs.Stmt) int {
	end := s.Range.End.Offset
	depth := 0

	_, _ = p.xsFind(from, end, func(_ int, c byte) bool {
		switch c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		}

		return false
	})

	for depth > 0 {
		closer, ok := p.scanFirst(end, ')', ']', '}')
		if !ok {
			break
		}

		end = closer + 1
		depth--
	}

	return end
}

// warnVerbatim prints a statement carrying a parser warning as its own
// bytes: a wrong-arity vector collapses to its first operand in the AST,
// so only the bytes keep the lost operands — and the warning range is the
// parser's own record of where the collapsed region sits. The cut opens at
// the warning (warnSlice), closes through the braces the extent leaves out
// (warnCutEnd) and keeps the line breaks: a // comment inside must stay
// line-bound. Comments starting with the cut ride it once — stmt anchors
// against the slice's start, not the extent's. Declarations join the same
// family through warnInit, so every level a warning can reach prints its
// own bytes.
func (p *xsPrinter) warnVerbatim(s xs.Stmt) (string, bool) {
	from, ok := p.warnSlice(s)
	if !ok {
		return "", false
	}

	end := p.warnCutEnd(from, s)
	out := p.blockText(from, end)
	p.dropBefore(end)

	return out, true
}

// warnInit prints a declaration initializer the parser collapsed — a
// wrong-arity vector keeps only its first operand in the AST — as its own
// bytes inside the canonical head: a statement prints whole
// (warnVerbatim), but every declarator of a comma list carries its type
// words itself, so only the initializer region rides the cut — the same
// shape as a bare-block initializer (bareBlock). The region spans the '='
// to the separator closing it — the statement's ';' or the next
// declarator's ',', the first code byte of either at the region's own
// bracket depth — and its comments ride it once.
func (p *xsPrinter) warnInit(d xs.Decl) (string, bool) {
	for _, w := range p.warns {
		if w.Start.Offset >= d.Range.End.Offset || w.End.Offset <= d.Range.Start.Offset {
			continue
		}

		eq, ok := p.scanFirst(d.Range.Start.Offset, '=')
		if !ok || eq >= d.Range.End.Offset {
			// an overlapping warning without an initializer '=' cannot
			// come from an accepted parse; the canonical print is the
			// safe answer
			return "", false
		}

		depth := 0
		// the initializer's own end is the fallback: the collapsed extent
		// stops inside the warned region, the warning's end is its true
		// one — the bytes past it (a trailing unterminated comment runs to
		// the EOF) belong to the global stream, not the cut, or the
		// synthesized ';' would glue into them
		to := max(d.Range.End.Offset, w.End.Offset)

		_, _ = p.xsFind(eq+1, len(p.src), func(i int, c byte) bool {
			switch c {
			case '(', '[', '{':
				depth++
			case ')', ']', '}':
				depth--
			case ',', ';':
				if depth == 0 {
					to = i

					return true
				}
			}

			return false
		})

		// a call's own parens close past the collapsed extent — the audit's
		// callee-stopped extents: an unclosed opener at the cut's end is
		// one of those, and the cut follows its closer out, the same
		// discipline statements take (warnCutEnd)
		balance := 0

		_, _ = p.xsFind(eq+1, to, func(_ int, c byte) bool {
			switch c {
			case '(', '[', '{':
				balance++
			case ')', ']', '}':
				balance--
			}

			return false
		})

		for balance > 0 {
			closer, ok := p.scanFirst(to, ')', ']', '}')
			if !ok {
				break
			}

			to = closer + 1
			balance--
		}

		out := p.blockText(eq+1, to)
		p.dropBefore(to)

		// the cut opens right past the '='; the one space the join
		// already prints is all the head needs
		return strings.TrimLeft(out, " \t"), true
	}

	return "", false
}

// openString reports whether the bytes carry an unterminated literal: a
// string the scanner closes on its matching quote alone — an escaped
// newline rides inside one — so a string reaching its line's end swallows
// every byte the printer would synthesize after it, and an unterminated
// literal is the same shape one brace wider: a bare block left open runs
// to the EOF and swallows the same bytes. The canonical print of a
// construct carrying one re-glues on the reparse, so such constructs
// print as their own bytes — the warn family's discipline. Comments step
// over whole: a quote inside one is prose, not a literal.
func (p *xsPrinter) openString(from, to int) bool {
	src := p.src
	ci := 0 // first comment not fully passed; the extents are sorted and disjoint

	for i := from; i < to && i < len(src); i++ {
		jumped := false

		for ci < len(p.comments) && p.comments[ci].Start.Offset <= i {
			if e := p.comments[ci].End.Offset; e > i {
				i = e - 1
				jumped = true
			}

			ci++
		}

		if jumped {
			continue
		}

		switch src[i] {
		case '{':
			// a bare-block literal the scanner skips whole: its matching
			// '}' may sit past this range (a body block's closes beyond
			// the construct's own extent), only a source without one is
			// the unterminated shape
			if closer, ok := p.matchBracket(i, '{', '}'); ok {
				i = closer
			} else {
				return true
			}
		case '"':
			// string literal under the scanner's own discipline: escapes
			// ride two bytes, the matching quote closes, the line's end
			// leaves it open
			for i++; i < to && i < len(src); i++ {
				if src[i] == '\\' && i+1 < len(src) {
					i++

					continue
				}

				if src[i] == '"' || src[i] == '\n' {
					break
				}
			}

			if i >= to || i >= len(src) || src[i] == '\n' {
				return true
			}
		}
	}

	return false
}

// openStringVerbatim prints a declaration line carrying an unterminated
// string literal as its own bytes: the initializer's literal runs to its
// line's end, and the canonical print's synthesized ';' — or the warn
// cut's ';' search, which cannot see through the literal — would glue
// into it on the reparse. The declaration hooks answer whether they took
// the line.
func (p *xsPrinter) openStringVerbatim(from, to int) bool {
	if !p.openString(from, to) {
		return false
	}

	p.declVerbatim(from, to)

	return true
}

// declVerbatim prints a declaration line as its own bytes: the shapes with
// no canonical print that reparses — an unterminated literal
// (openStringVerbatim), a chain whose member name came off a
// non-identifier token (degenerateExprs) — take the whole line, the bytes
// being their own fixed point. Trailing line breaks trim: a degenerate
// name's EOF token ends the extent past the page's own ending.
func (p *xsPrinter) declVerbatim(from, to int) {
	p.line(0, strings.TrimRight(p.blockText(from, to), " \t\r\n"))
	p.dropBefore(to)
}

// declOwnEnd answers where a declaration's own bytes end past its head:
// through the body brace's closer when one follows, through the ';'
// otherwise — the source's own, never a synthesized one.
func (p *xsPrinter) declOwnEnd(from int) int {
	if open, ok := p.scanFirst(from, '{', ';'); ok && p.src[open] == '{' {
		if closer, ok := p.matchBracket(open, '{', '}'); ok {
			return closer + 1
		}
	}

	if semi, ok := p.scanFirst(from, ';'); ok {
		return semi + 1
	}

	return len(p.src)
}

// decls prints the file's declarations, one blank line between groups and
// none inside one.
func (p *xsPrinter) decls(list []xs.Decl) {
	for i := 0; i < len(list); {
		p.separate()

		group := list[i:p.varGroup(list, i)]
		p.flushBefore(group[0].Range.Start.Offset, 0)
		p.level = 0
		p.declGroup(group)

		i += len(group)
	}
}

// varGroup extends a top-level declaration group: consecutive variable
// declarators whose source gap is exactly the separating comma belong to
// one declaration list. The group's canonical print is its own fixed
// point — `int a = 1, b;` re-parses to the same comma-adjacent
// declarators — which is what keeps the pass idempotent where per-node
// printing would drift the split declarators apart.
func (p *xsPrinter) varGroup(list []xs.Decl, i int) int {
	end := i + 1

	for end < len(list) &&
		list[end-1].Kind == xs.DeclVariable && list[end].Kind == xs.DeclVariable &&
		list[end-1].Type == list[end].Type &&
		strings.TrimSpace(p.cutBytes(list[end-1].Range.End.Offset, list[end].Range.Start.Offset)) == "," {
		end++
	}

	return end
}

// declGroup prints one top-level declaration group: a lone declaration of
// any kind, or a comma-adjacent variable list on one line.
func (p *xsPrinter) declGroup(group []xs.Decl) {
	if len(group) == 1 {
		p.decl(group[0])

		return
	}

	// a comment written between the declarators anchors above the line —
	// the nearest inter-node position the merged print leaves it
	p.flushBefore(group[len(group)-1].Range.Start.Offset, 0)

	// an unterminated string in any initializer takes the whole line as
	// its own bytes: the merged print's synthesized ';' would glue into
	// the literal on the reparse
	if p.openStringVerbatim(group[0].Range.Start.Offset, group[len(group)-1].Range.End.Offset) {
		return
	}

	// so does a degenerate member name: its bytes are the line's own
	// fixed point (degenerateExprs)
	for _, d := range group {
		if len(d.Body) > 0 && degenerateExprs(d.Body[0].Exprs) {
			p.declVerbatim(group[0].Range.Start.Offset, group[len(group)-1].Range.End.Offset)

			return
		}
	}

	parts := make([]string, 0, len(group))

	for _, d := range group {
		part := d.Name
		if init := p.initValue(d); init != "" {
			part += " = " + init
		}

		parts = append(parts, part)
	}

	p.line(0, group[0].Type+" "+strings.Join(parts, ", ")+";")
}

// decl prints one top-level declaration.
func (p *xsPrinter) decl(d xs.Decl) {
	switch d.Kind {
	case xs.DeclFunction:
		p.function(d)
	case xs.DeclExtern:
		p.extern(d)
	case xs.DeclVariable:
		p.variable(d)
	case xs.DeclRule:
		p.rule(d)
	case xs.DeclEvent:
		p.event(d)
	case xs.DeclInclude:
		p.include(d)
	default:
		// a kind the printer does not know (a grammar addition) prints its
		// own bytes — the safe default for an unknown shape
		p.line(0, collapseWS(p.cut(d.Range)))
		p.dropBefore(d.Range.End.Offset)
	}
}

// function prints a function declaration: the canonical head from the
// AST, the parameter list verbatim (defaults are parser-discarded, only
// the bytes carry them) and the body canonically under a brace shared
// with the head line. The shape after the head words — `(`, `{` or `;` —
// is read from the bytes, because an empty body and an empty parameter
// list both parse to nil and the AST extents stop short of their tokens.
func (p *xsPrinter) function(d xs.Decl) {
	head := strings.TrimSpace(d.Type + " " + d.Name)

	shape, ok := p.scanFirst(d.Range.Start.Offset, '(', '{', ';')
	if !ok || p.src[shape] == ';' || p.src[shape] == '{' {
		// `;` a prototype; `{` cannot come from an accepted parse before
		// the parameter list — either way the head line is all there is
		p.line(0, head+";")

		return
	}

	rparen, ok := p.matchBracket(shape, '(', ')')
	if !ok {
		p.line(0, head+";")

		return
	}

	// an unterminated literal in the parameter bytes prints the whole
	// declaration as its own bytes: the head line would end inside the
	// literal, and the ";" or " {" synthesized after it would glue into
	// it on the reparse
	if p.openString(shape, rparen+1) {
		p.declVerbatim(d.Range.Start.Offset, p.declOwnEnd(rparen+1))

		return
	}

	// comments ahead of the parameter list anchor above the head line; the
	// // ones inside it lift above it too — collapsed into the cut they
	// would swallow the rest of the head on the reparse
	p.flushBefore(shape, 0)
	head += p.liftLine(shape, rparen+1, 0)

	if open, ok := p.scanFirst(rparen+1, '{', ';'); ok && p.src[open] == '{' {
		p.bodyBlock(head, open, d.Body)

		return
	}

	p.line(0, head+";")
}

// extern prints an extern declaration: a prototype with its verbatim
// parameter list, or the variable form with its initializer from the AST.
// The shape test reads the bytes for the same reason function's does —
// an empty parameter list parses to nil.
func (p *xsPrinter) extern(d xs.Decl) {
	head := strings.TrimSpace("extern " + d.Type + " " + d.Name)

	shape, ok := p.scanFirst(d.Range.Start.Offset, '(', ';', '=')
	if !ok || p.src[shape] == ';' {
		p.line(0, head+";")

		return
	}

	if p.src[shape] == '=' {
		if p.openStringVerbatim(d.Range.Start.Offset, d.Range.End.Offset) {
			return
		}

		if len(d.Body) > 0 && degenerateExprs(d.Body[0].Exprs) {
			p.declVerbatim(d.Range.Start.Offset, d.Range.End.Offset)

			return
		}

		if init := p.initValue(d); init != "" {
			p.line(0, head+" = "+init+";")

			return
		}

		p.line(0, head+";")

		return
	}

	rparen, ok := p.matchBracket(shape, '(', ')')
	if !ok {
		p.line(0, head+";")

		return
	}

	p.flushBefore(shape, 0)
	head += p.liftLine(shape, rparen+1, 0)
	p.line(0, head+";")
}

// variable prints one top-level variable declarator: the type words and
// the name from the AST, the initializer canonically. A multi-declarator
// source line arrives as one declaration per node and prints one per
// line — the grouping back into a single line is the hybrid-slice pass's
// to refine.
func (p *xsPrinter) variable(d xs.Decl) {
	if p.openStringVerbatim(d.Range.Start.Offset, d.Range.End.Offset) {
		return
	}

	if len(d.Body) > 0 && degenerateExprs(d.Body[0].Exprs) {
		p.declVerbatim(d.Range.Start.Offset, d.Range.End.Offset)

		return
	}

	head := strings.TrimSpace(d.Type + " " + d.Name)

	if init := p.initValue(d); init != "" {
		head += " = " + init
	}

	p.line(0, head+";")
}

// initValue reads a variable or extern declaration's initializer
// expression: the value side of the name = value pair the parser stores
// as the declaration's single statement. An initializer the parser
// collapsed prints as its own bytes (warnInit).
func (p *xsPrinter) initValue(d xs.Decl) string {
	if len(d.Body) == 0 || len(d.Body[0].Exprs) == 0 {
		return ""
	}

	if out, ok := p.warnInit(d); ok {
		return out
	}

	init := d.Body[0].Exprs[0]
	if init.Kind == xs.ExprBinary && init.Value == "=" && len(init.Children) == 2 {
		return p.expr(init.Children[1], 1)
	}

	return p.expr(init, 1)
}

// rule prints a rule declaration: the header words verbatim up to the
// body brace — the parser skips the modifiers (inactive, min-interval)
// without materializing them, so only the bytes carry them — then the
// body canonically. The brace is searched past the AST extent: an empty
// body ends the extent at the rule name.
func (p *xsPrinter) rule(d xs.Decl) {
	startOff := d.Range.Start.Offset

	open, ok := p.scanFirst(startOff, '{')
	if !ok {
		// no body brace anywhere: the whole declaration prints as its own
		// bytes
		p.line(0, collapseWS(p.cut(d.Range)))
		p.dropBefore(d.Range.End.Offset)

		return
	}

	// an unterminated literal in the header bytes prints the whole rule as
	// its own bytes: the header line would end inside the literal, and the
	// " {" synthesized after it would glue into it on the reparse
	if p.openString(startOff, open) {
		if closer, ok := p.matchBracket(open, '{', '}'); ok {
			p.declVerbatim(startOff, closer+1)

			return
		}
	}

	p.line(0, p.liftLine(startOff, open, 0)+" {")
	p.stmts(d.Body, 1)
	if closer, ok := p.matchBracket(open, '{', '}'); ok {
		p.flushBefore(closer, 1)
	}

	p.line(0, "}")
}

// event prints an event registration: the argument list verbatim, an
// optional body canonically or the closing semicolon — the body brace is
// searched past the AST extent for the same empty-body reason rule's is.
func (p *xsPrinter) event(d xs.Decl) {
	head := "event"

	shape, ok := p.scanFirst(d.Range.Start.Offset, '(', ';')
	if !ok || p.src[shape] == ';' {
		p.line(0, head+";")

		return
	}

	rparen, ok := p.matchBracket(shape, '(', ')')
	if !ok {
		p.line(0, head+";")

		return
	}

	// an unterminated literal in the argument bytes is function's header
	// shape: the whole declaration prints as its own bytes
	if p.openString(shape, rparen+1) {
		p.declVerbatim(d.Range.Start.Offset, p.declOwnEnd(rparen+1))

		return
	}

	// comments ahead of the argument list anchor above the head line; the
	// // ones inside it lift above it — the reason event's shape read
	// flushes first, like function's does
	p.flushBefore(shape, 0)
	head += p.liftLine(shape, rparen+1, 0)

	if open, ok := p.scanFirst(rparen+1, '{', ';'); ok && p.src[open] == '{' {
		p.bodyBlock(head, open, d.Body)

		return
	}

	p.line(0, head+";")
}

// include prints an include directive as its own source line: the '#'
// prefix (the scanner skips it as an unknown byte, so the extent starts
// past it), the path's original quoting and a trailing // comment are the
// line's own bytes — the AST keeps none of them.
func (p *xsPrinter) include(d xs.Decl) {
	from := d.Range.Start.Offset
	if from > 0 && p.src[from-1] == '#' {
		from--
	}

	end, ok := p.lineEnd(d.Range.End.Offset)
	if !ok {
		end = d.Range.End.Offset
	}

	// the bytes past the extent are the line's trailing comment's — until
	// the next declaration's own code starts: a line carrying two includes
	// has no newline to stop the walk, and the companion's bytes would
	// ride the cut and print again from their own declaration
	if code, ok := p.xsFind(d.Range.End.Offset, end, func(_ int, _ byte) bool {
		return true
	}); ok {
		end = code
	}

	// a comment spilling past the line — a multi-line block opened here —
	// keeps its tail out of the cut; the global stream anchors it after.
	// Comments fully inside the line ride the cut, so the spiller may sit
	// behind them: the walk takes the first whose extent crosses the end.
	// An unclosed block ends on the EOF the line's own end already is, so
	// the crossing test cannot see it — yet riding the cut would glue the
	// page's synthesized newline into its extent on the reparse and
	// re-anchor it, so it spills like any other; a // comment clipped at
	// that same EOF rides fine, its extent stops short of the newline
	for i := p.pending; i < len(p.comments) && p.comments[i].Start.Offset < end; i++ {
		r := p.comments[i]
		if r.End.Offset > end || (!p.isLineComment(r) && p.unclosedEOF(r)) {
			end = r.Start.Offset

			break
		}
	}

	p.line(0, collapseWS(p.cutBytes(from, end)))
	p.dropBefore(end)
}

// stmts prints a statement list, anchoring each statement's preceding
// comments at the list's own level. The statement after a colon-form rule
// section hangs one level in — the label reads as the head of what follows
// — while the sections themselves always print at the list's own level.
func (p *xsPrinter) stmts(list []xs.Stmt, level int) {
	hang := false

	for _, s := range list {
		at := level
		if hang && s.Kind != xs.StmtCondition && s.Kind != xs.StmtAction {
			at = level + 1
		}

		p.stmt(s, at)
		hang = (s.Kind == xs.StmtCondition || s.Kind == xs.StmtAction) && p.sectionColon(s)
	}
}

// stmtVerbatim prints a whole statement as its own bytes at the level:
// the degenerate shapes — a literal the source left unterminated, a
// header the parser took without the parentheses its canonical print
// would have to synthesize — have no canonical print that reparses, so
// the bytes are the fixed point. The cut closes through the braces the
// extent leaves out (warnCutEnd), and through an empty body block's pair
// when the extent stops right on its opener — blockEnd has no statement
// to reach past it, and nothing but the brace may follow an accepted
// statement's end.
func (p *xsPrinter) stmtVerbatim(s xs.Stmt, level int) {
	from := s.Range.Start.Offset
	end := p.warnCutEnd(from, s)

	if open, ok := p.xsFind(end, len(p.src), func(_ int, _ byte) bool {
		return true
	}); ok && p.src[open] == '{' {
		if closer, ok := p.matchBracket(open, '{', '}'); ok {
			end = closer + 1
		}
	}

	p.line(level, strings.TrimRight(p.blockText(from, end), " \t\r\n"))
	p.dropBefore(end)
}

// stmt prints one statement at a nesting level.
func (p *xsPrinter) stmt(s xs.Stmt, level int) {
	// a warning-carrying statement prints verbatim from the warning: the
	// comments in the warning-to-extent gap ride that cut, so the anchor
	// point here is the slice's own start, not the extent's
	if from, ok := p.warnSlice(s); ok {
		p.flushBefore(from, level)
	} else {
		p.flushBefore(s.Range.Start.Offset, level)
	}

	p.level = level

	if out, ok := p.warnVerbatim(s); ok {
		p.line(level, out)

		return
	}

	// an unterminated string literal anywhere in the statement prints it
	// as its own bytes too: the canonical print would end inside the
	// literal, and every byte synthesized after it glues into the string
	// on the reparse
	if p.openString(s.Range.Start.Offset, s.Range.End.Offset) {
		p.stmtVerbatim(s, level)

		return
	}

	// a chain with a degenerate member name takes the whole statement:
	// the name's own bytes are what keeps the reparse from feeding the
	// synthesized ";" back as the name (degenerateExprs). A for loop's
	// typed init prints inside the header, so its expressions count too
	if degenerateExprs(s.Exprs) ||
		(s.Kind == xs.StmtFor && len(s.Body) > 0 && degenerateExprs(s.Body[0].Exprs)) {
		p.stmtVerbatim(s, level)

		return
	}

	switch s.Kind {
	case xs.StmtIf:
		p.stmtIf(s, level, "")
	case xs.StmtWhile:
		// a while the parser took without its parentheses has no
		// canonical print: the empty condition would have to synthesize
		// "()", which the parser itself cannot reparse
		if len(s.Exprs) == 0 {
			p.stmtVerbatim(s, level)

			return
		}

		p.tail("while ("+p.expr(s.Exprs[0], 1)+")", s, level)
	case xs.StmtDo:
		p.stmtDo(s, level)
	case xs.StmtFor:
		p.stmtFor(s, level)
	case xs.StmtSwitch:
		// a switch the parser took without its subject's parentheses is
		// the while shape: "switch ()" would not reparse
		if len(s.Exprs) == 0 {
			p.stmtVerbatim(s, level)

			return
		}

		p.stmtSwitch(s, level)
	case xs.StmtCase:
		p.stmtCase(s, level)
	case xs.StmtCondition, xs.StmtAction:
		kw := "condition"
		if s.Kind == xs.StmtAction {
			kw = "action"
		}

		// colon form: the keyword line is a label — the parser keeps the
		// statements after it as siblings of the section, and stmts hangs
		// the next one off the label line
		if p.sectionColon(s) {
			if colon, ok := p.scanFirst(s.Range.Start.Offset, ':'); ok {
				p.line(level, p.liftLine(s.Range.Start.Offset, colon, level)+":")
			} else {
				p.line(level, kw+":")
			}

			return
		}

		// the brace is searched past the AST extent: an empty section body
		// ends the extent at the keyword
		if open, ok := p.scanFirst(s.Range.Start.Offset, '{'); ok {
			p.line(level, kw+" {")
			p.stmts(s.Body, level+1)
			if closer, ok := p.matchBracket(open, '{', '}'); ok {
				p.flushBefore(closer, level+1)
			}

			p.line(level, "}")

			return
		}

		p.line(level, kw)
	case xs.StmtBlock:
		p.line(level, "{")
		p.stmts(s.Body, level+1)
		if closer, ok := p.matchBracket(s.Range.Start.Offset, '{', '}'); ok {
			p.flushBefore(closer, level+1)
		}

		p.line(level, "}")
	case xs.StmtReturn:
		out := "return"
		if len(s.Exprs) > 0 {
			out += " " + p.expr(s.Exprs[0], 1)
		}

		p.line(level, out+";")
	case xs.StmtBreak:
		p.line(level, "break;")
	case xs.StmtContinue:
		p.line(level, "continue;")
	case xs.StmtDecl:
		if len(s.Exprs) == 0 {
			p.line(level, ";")

			return
		}

		p.line(level, p.declText(s, level)+";")
	default:
		// StmtExpr and recovery shapes
		if len(s.Exprs) == 0 {
			p.line(level, ";")

			return
		}

		p.line(level, p.expr(s.Exprs[0], 1)+";")
	}
}

// blockOpen names the brace a header line takes when its body is a block;
// a brace-less single-statement body keeps the structure the parser saw —
// synthesizing braces would change the tree.
func blockOpen(b xs.Stmt) string {
	if b.Kind == xs.StmtBlock {
		return " {"
	}

	return ""
}

// tail prints a header whose body follows as a branch (while, and the
// rule-section forms): a block body shares the head line and owes the
// closer, a single-statement body stays brace-less one level in.
func (p *xsPrinter) tail(head string, s xs.Stmt, level int) {
	body := xs.Stmt{Kind: xs.StmtExpr}

	if len(s.Body) > 0 {
		body = s.Body[0]
	}

	p.line(level, head+blockOpen(body))
	if p.branch(body, level) {
		p.line(level, "}")
	}
}

// branch prints one branch body and answers whether a "}" closer is owed
// on the level line: a block prints its contents at level+1 with the
// comments ahead of its closer anchored inside, any other statement
// prints brace-less.
func (p *xsPrinter) branch(b xs.Stmt, level int) bool {
	if b.Kind != xs.StmtBlock {
		p.stmt(b, level+1)

		return false
	}

	p.stmts(b.Body, level+1)
	if closer, ok := p.matchBracket(b.Range.Start.Offset, '{', '}'); ok {
		p.flushBefore(closer, level+1)
	}

	return true
}

// stmtIf prints an if with its optional else, the chain flattened:
// `} else {` shares the closer line with the else head, and an else-if
// continues the same line.
func (p *xsPrinter) stmtIf(s xs.Stmt, level int, prefix string) {
	cond := ""
	if len(s.Exprs) > 0 {
		cond = p.expr(s.Exprs[0], 1)
	}

	if len(s.Body) == 0 {
		p.line(level, prefix+"if ("+cond+");")

		return
	}

	thenB := s.Body[0]
	p.line(level, prefix+"if ("+cond+")"+blockOpen(thenB))
	closed := p.branch(thenB, level)

	if len(s.Body) < 2 {
		if closed {
			p.line(level, "}")
		}

		return
	}

	el := s.Body[1]
	// a comment written before the else keyword anchors where it sits:
	// inside the braces when the then branch closes one, at the level
	// line otherwise
	if p.pendingBefore(el.Range.Start.Offset) {
		at := level
		if closed {
			at = level + 1
		}

		p.flushBefore(el.Range.Start.Offset, at)
	}

	kw := "else"
	if closed {
		kw = "} else"
	}

	switch el.Kind {
	case xs.StmtBlock:
		p.line(level, kw+" {")
		p.branch(el, level)
		p.line(level, "}")
	case xs.StmtIf:
		p.stmtIf(el, level, kw+" ")
	default:
		p.line(level, kw)
		p.stmt(el, level+1)
	}
}

// stmtDo prints a do/while: the body under the do line, the condition on
// the closer line. A do the parser took without its while's parentheses —
// or without the while at all — prints as its own bytes: the closer line
// would have to synthesize a "while ()" the parser cannot reparse.
func (p *xsPrinter) stmtDo(s xs.Stmt, level int) {
	if len(s.Exprs) == 0 {
		p.stmtVerbatim(s, level)

		return
	}

	body := xs.Stmt{Kind: xs.StmtExpr}

	if len(s.Body) > 0 {
		body = s.Body[0]
	}

	cond := ""
	if len(s.Exprs) > 0 {
		cond = p.expr(s.Exprs[0], 1)
	}

	p.line(level, "do"+blockOpen(body))
	closer := ""
	if p.branch(body, level) {
		closer = "}"
	}

	p.line(level, closer+" while ("+cond+");")
}

// stmtFor prints a for loop, rebuilding the header's three sections.
func (p *xsPrinter) stmtFor(s xs.Stmt, level int) {
	init, rest := p.forInit(s, level)

	cond, step := "", ""
	if len(rest) > 0 {
		cond = p.expr(rest[0], 1)
	}

	if len(rest) > 1 {
		step = p.expr(rest[1], 1)
	}

	head := "for (" + init + ";" + condPart(cond) + ";" + stepPart(step) + ")"

	body := xs.Stmt{Kind: xs.StmtExpr}
	if len(s.Body) > 0 {
		body = s.Body[len(s.Body)-1]
	}

	p.line(level, head+blockOpen(body))
	if p.branch(body, level) {
		p.line(level, "}")
	}
}

// condPart renders a for header's middle section, keeping the separator
// spacing of the C canon: empty sections print bare.
func condPart(cond string) string {
	if cond == "" {
		return ""
	}

	return " " + cond
}

// stepPart renders a for header's last section.
func stepPart(step string) string {
	return condPart(step)
}

// forInit names the init clause and the trailing expressions of a for
// header. A typed init is the declaration statement the parser stores as
// the body's first statement; otherwise the XS idiom's expression init
// (`for (i = 1; <= size)`) is the first expression — a bare condition
// cannot be told apart and reads as the init, the shape the real-world
// XS for has.
func (p *xsPrinter) forInit(s xs.Stmt, level int) (init string, rest []xs.Expr) {
	rest = s.Exprs

	if len(s.Body) == 2 && s.Body[0].Kind == xs.StmtDecl {
		return p.declText(s.Body[0], level), rest
	}

	if len(s.Body) == 1 && len(s.Exprs) > 0 {
		return p.expr(s.Exprs[0], 1), s.Exprs[1:]
	}

	return "", rest
}

// stmtSwitch prints a switch: the subject on the head line, the case
// statements one level in.
func (p *xsPrinter) stmtSwitch(s xs.Stmt, level int) {
	subj := ""
	if len(s.Exprs) > 0 {
		subj = p.expr(s.Exprs[0], 1)
	}

	p.line(level, "switch ("+subj+") {")
	p.stmts(s.Body, level+1)
	// the body brace is searched from past the subject: an empty body ends
	// the AST extent before its own brace
	searchFrom := s.Range.Start.Offset
	if len(s.Exprs) > 0 {
		searchFrom = s.Exprs[0].Range.End.Offset + 1
	}

	if open, ok := p.scanFirst(searchFrom, '{'); ok {
		if closer, ok := p.matchBracket(open, '{', '}'); ok {
			p.flushBefore(closer, level+1)
		}
	}

	p.line(level, "}")
}

// stmtCase prints one case or default label with its statements. The
// label prints as its own bytes through the colon: a block comment written
// inside it (`case /* c */ 1:`) is not the printer's to move, and a // one
// lifts above the label — collapsed into the line it would swallow the
// colon on the reparse.
func (p *xsPrinter) stmtCase(s xs.Stmt, level int) {
	colon, ok := p.scanFirst(s.Range.Start.Offset, ':')
	if !ok {
		// an accepted parse always carries the label colon
		p.line(level, "default:")
		p.stmts(s.Body, level+1)

		return
	}

	p.line(level, p.liftLine(s.Range.Start.Offset, colon, level)+":")
	p.stmts(s.Body, level+1)
}

// declText renders a local declaration without its semicolon: the type
// words cut from the source ahead of the first declarator (the AST keeps
// only the consumed words, the cut keeps their spelling; // comments
// inside the cut lift to the given level — collapsed into the line they
// would swallow the declarators on the reparse), then the declarators
// canonically.
func (p *xsPrinter) declText(s xs.Stmt, level int) string {
	if len(s.Exprs) == 0 {
		return ""
	}

	typ := p.liftLine(s.Range.Start.Offset, s.Exprs[0].Range.Start.Offset, level)
	parts := make([]string, 0, len(s.Exprs))

	for _, item := range s.Exprs {
		if item.Kind == xs.ExprIdent {
			parts = append(parts, item.Value)

			continue
		}

		if item.Kind == xs.ExprBinary && item.Value == "=" && len(item.Children) == 2 {
			parts = append(parts, p.expr(item.Children[0], 1)+" = "+p.expr(item.Children[1], 1))

			continue
		}

		parts = append(parts, p.expr(item, 1))
	}

	return typ + " " + strings.Join(parts, ", ")
}

// memberPrec is the binding power above every operator: member and index
// chains, unary operands and the postfix ride print bare under anything.
const memberPrec = 12

// xsPrec is the binding power of a binary operator, the parser's own
// ordering (internal/xs parse.go binaryPrec — a private mirror: the
// parser's table is unexported, and the printer must parenthesize exactly
// where the parser needed parentheses); 0 when the token is not one.
func xsPrec(op string) int {
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

// isXsAssign reports whether an operator is an assignment form:
// right-associative in the parser, so its operand minima swap.
func isXsAssign(op string) bool {
	switch op {
	case "=", "+=", "-=", "*=", "/=", "%=":
		return true
	}

	return false
}

// identText reports whether text is an identifier's spelling — the
// scanner's own rule, so a member name the parser took from a
// non-identifier token (an operator, the EOF's empty text) reads as the
// degenerate shape it is.
func identText(text string) bool {
	if text == "" {
		return false
	}

	for i := 0; i < len(text); i++ {
		c := text[i]

		switch {
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
		case i > 0 && c >= '0' && c <= '9':
		default:
			return false
		}
	}

	return true
}

// degenerateExprs reports whether any expression of the list carries a
// chain whose member name came off a non-identifier token: the parser
// takes any token after ".", and the canonical join would glue those
// bytes — a ";", a number — into whatever the page prints next, while a
// cut without the name would feed the synthesized ";" back as the name
// and eat the head of what follows. The construct prints as its own
// bytes instead (stmtVerbatim, declVerbatim).
func degenerateExprs(list []xs.Expr) bool {
	for _, e := range list {
		if e.Kind == xs.ExprBinary && e.Value == "." && len(e.Children) == 2 &&
			e.Children[1].Kind == xs.ExprIdent && !identText(e.Children[1].Value) {
			return true
		}

		if degenerateExprs(e.Children) {
			return true
		}
	}

	return false
}

// paren wraps out when its binding power is below the context minimum —
// the parentheses the AST dropped come back exactly there.
func paren(out string, minPrec, prec int) string {
	if prec < minPrec {
		return "(" + out + ")"
	}

	return out
}

// expr prints one expression: leaves are token values (string literals
// keep their quotes, hex literals their spelling), operators print
// canonically, and a child binding weaker than its context is
// parenthesized.
func (p *xsPrinter) expr(e xs.Expr, minPrec int) string {
	switch e.Kind {
	case xs.ExprLiteral, xs.ExprIdent:
		if e.Value == "{}" {
			// the bare block is the one literal the AST holds as a marker:
			// its statements were tolerated away, the bytes kept them
			return p.bareBlock(e)
		}

		return e.Value
	case xs.ExprUnary:
		return p.unary(e, minPrec)
	case xs.ExprCall:
		return p.call(e)
	case xs.ExprVector:
		parts := make([]string, 0, len(e.Children))

		for _, op := range e.Children {
			parts = append(parts, p.expr(op, 1))
		}

		return "(" + strings.Join(parts, ", ") + ")"
	}

	return p.binary(e, minPrec)
}

// unary prints a prefix operator or a postfix increment/decrement (the
// parser marks those with a "post" suffix on the operator).
func (p *xsPrinter) unary(e xs.Expr, minPrec int) string {
	op := e.Value

	if op == "++post" || op == "--post" {
		if len(e.Children) == 0 {
			return p.cut(e.Range)
		}

		return p.expr(e.Children[0], memberPrec) + op[:2]
	}

	if len(e.Children) == 0 {
		// an operator without an operand cannot come from an accepted
		// parse; its own bytes are the safe print
		return p.cut(e.Range)
	}

	operand := p.expr(e.Children[0], memberPrec)
	// two prefix operators of the +/- family in a row need a separating
	// space: `--a` re-reads as the decrement
	if (op == "+" || op == "-" || op == "++" || op == "--") && operand != "" &&
		(operand[0] == '+' || operand[0] == '-') {
		return op + " " + operand
	}

	return op + operand
}

// call prints a call: the callee name from the AST when it is a plain
// identifier, cut from the source otherwise — the Call node keeps only
// the name, its Children are the arguments alone. The cut ends at the
// call's "(" — found by walking back from the first argument, or ahead
// of the extent when there are none — so it never doubles the
// parenthesis a re-collapse would keep; a grouped callee opens the cut at
// its own "(" (groupedCalleeStart), or the group's ")" would ride the
// cut while its opener went missing.
func (p *xsPrinter) call(e xs.Expr) string {
	callee := e.Callee
	if callee == "" {
		open, ok := 0, false
		if len(e.Children) > 0 {
			open, ok = p.openParenBefore(e.Children[0].Range.Start.Offset)
		} else {
			open, ok = p.zeroCallParen(e)
		}

		if !ok {
			callee = collapseWS(p.cut(e.Range))
		} else {
			callee = p.liftLine(p.calleeStart(e.Range.Start.Offset, open), open, p.level)
		}
	}

	parts := make([]string, 0, len(e.Children))

	for _, arg := range e.Children {
		parts = append(parts, p.expr(arg, 1))
	}

	return callee + "(" + strings.Join(parts, ", ") + ")"
}

// binary prints a binary operation: member and index access spelled
// without spaces, everything else through the precedence table — left
// child at its own power, right child one above, the assignment forms
// mirrored for their right associativity.
func (p *xsPrinter) binary(e xs.Expr, minPrec int) string {
	op := e.Value

	switch op {
	case ".":
		if len(e.Children) == 2 {
			out := p.expr(e.Children[0], memberPrec) + "." + p.expr(e.Children[1], memberPrec)
			return paren(out, minPrec, memberPrec)
		}
	case "[]":
		if len(e.Children) == 2 {
			out := p.expr(e.Children[0], memberPrec) + "[" + p.expr(e.Children[1], 1) + "]"
			return paren(out, minPrec, memberPrec)
		}
	}

	prec := xsPrec(op)
	if prec == 0 || len(e.Children) < 2 {
		// an operator the table does not know cannot come from an accepted
		// parse; its own bytes are the safe print
		return collapseWS(p.cut(e.Range))
	}

	leftMin, rightMin := prec, prec+1
	if isXsAssign(op) {
		leftMin, rightMin = prec+1, prec
	}

	out := p.expr(e.Children[0], leftMin) + " " + op + " " + p.expr(e.Children[1], rightMin)

	return paren(out, minPrec, prec)
}
