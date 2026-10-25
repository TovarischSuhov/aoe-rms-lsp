# Formatting — consuming the format cell

Domain: reformatting RMS and XS sources into the cell's canonical
style. Target audience: implementers of the server cell (the
textDocument/formatting handler) and tooling that rewrites map
scripts.

## Format a document

```go
out, err := format.RMS(text, format.Options{TabSize: 4})
if err != nil {
	// source has error-severity diagnostics — leave the text
	// unchanged and report no edits (classify with errors.Is)
}
// out is the whole formatted document: emit one full-document
// TextEdit, not per-line diffs
```

XS documents go through the same call shape — same options, same
sentinel, same one-edit result:

```go
out, err := format.XS(xsText, format.Options{TabSize: 4})
```

The server picks the printer by URI suffix (.rms → RMS, .xs → XS,
anything else → no edits); either way the answer is one full-document
TextEdit.

## Mapping LSP FormattingOptions (server)

```go
opts := format.Options{
	TabSize:    int(params.Options.TabSize),
	IndentTabs: !params.Options.InsertSpaces,
}
// zero value is valid: TabSize 0 formats with the default
// width of 4; InsertSpaces=false maps to tab indentation
```

Preconditions:
- Refusal is a sentinel error: classify with errors.Is — never a
  panic, never partial output (formatted is empty on refusal).
- Warnings do not refuse: a wrong-arity vector in XS (the only
  warning class the parser issues) prints as its own source bytes
  instead of the canonical form.
- The cell is stateless: call per request, no caching obligations.

## Guarantees to rely on

Both printers, on input they accept:
- format(format(x)) is byte-identical to format(x): formatting an
  already formatted document produces no diff.
- Every input comment reaches the output byte-exact; in XS a comment
  unique in the source appears exactly once — never lost, never
  printed twice.
- The output keeps the input's dominant EOL (CRLF stays CRLF).

RMS additionally:
- parse(format(x)) is structurally equal to parse(x): replacing the
  document with the output never introduces new syntax errors on
  input the parser accepted.
- inline XS blocks pass through verbatim.
- A conditional/random block whose extent crosses section headers
  prints those headers inline in its own child flow, and such
  sections' closing tags defer to the next header or the end of the
  document (each at its opening header's level). The layout follows
  the parse tree, not the author's visual nesting — where an open
  scope disagrees with the section tags around it, the output shows
  the scope's flow, and the tree survives (the #94 interleaving
  model; the former knownDivergent corpus entry is closed by it).
- Convergence is unconditional: the former escape class (a flat
  #-line with an unterminated /* EOF clips at the line's end) is
  closed in the parser (#135) — mergeComments cuts the #-extent at
  the comment's start for the EOF-clipped tail too, so pass one
  already prints the split form.

XS additionally:
- The formatted output reparses without error diagnostics, and its
  tree is structurally equal to the input's.
- The include-CR escape (#150) is closed in the same #120 change-set
  that introduced this printer: p.include trims the lone trailing \r
  an unterminated include-path literal carries out of its line —
  blockText's page-ending rule — pinned by the
  TestXS_IncludeTrailingCR rows.
- The grouped wrong-arity vector (#152) is closed on the branch
  task/format-xs (the format-xs-fixes run, 2026-10-23/24): a warn
  statement's verbatim cut opened on the warning's own start and
  lost the dropped groupings' `(` while keeping their `)` —
  warnVerbatim now walks back to the groupings' own `(` after
  cutArgsEnd (calleeStart) and takes it only while the widened cut
  keeps its brackets net non-positive. Pins:
  TestXS_WarnGroupedVectorStmt (four rows), the FuzzXS seeds
  `void A(){((0,0));}` / `void A(){(((0,0)));}` and the pinned
  seed crasher 1777e76aae25ce1c (testdata/fuzz).
- The zero-extent sibling (#153) is closed on both print paths on
  the same branch and run: statements — emptyBodyEnd spends a `{}`
  pair only on the statement's own zero-width block (a StmtBlock
  of its body chain starting exactly at the extent's end), never
  on the sibling block a bare `while;` ends its extent beside;
  declarations — a rule header's verbatim cut clamps to declLimit,
  the boundary decls sets at the next declaration's start, and its
  closer hunt gates on the same boundary (closer+1 past it breaks
  the loop). Pins: TestXS_ZeroExtentSiblingOnce (crasher
  6481cc9454be3769, the warn-chain and do rows; the accepted
  `void A(){do;{}}` shape refuses with ErrParseErrors),
  TestXS_DeclAdjacentOnce (crasher df7bb1183f5bb08, adj5, partial),
  the FuzzXS seeds `int A(){{while;{}}}` / `while;{}` and the two
  decl crashers.
- The split-declarator head (#162) is closed in the decl print paths:
  the scanner skips an unknown byte silently, so a declaration list's
  tail declarator starts its extent at the bare name and the gap around
  the byte is not a bare comma. varGroup's gap test now reduces the
  gap's visible bytes — a gap of a comma plus scanner-invisible junk
  still groups the pair (the merged line is its own byte-exact fixed
  point), and a split declarator that must print standalone (a comment
  anchors in the gap) gets the type words synthesized ahead of its
  verbatim bytes (declHead). Pins: TestXS_DeclSplitHeadOnce (nine rows)
  and the FuzzXS seed `void A,#A=0.`. A residual stays open: a split
  declarator whose NAME is a type word and whose gap carries a comment
  (`void A,/*c*/void=1.`) has no reparseable standalone print — the
  parser reads `void void` as two type words on any reparse — and only
  comma re-attachment could print it, which the comment anchoring
  forbids.
- The warn-literal statement body (#163) is closed on the statement
  path: warnCutEnd's closer hunt pairs by opener kind — a closer pops
  the opener of its own kind, and a closer nothing opened of its kind
  (the parser tolerates an unclosed `(` inside a block literal) closes
  the opener the depth counted, so `void A(){0.!%{(}}`'s cut stops on
  the literal's own `}` instead of spending it on the `(` and eating
  the enclosing body's brace. The declaration-level cut (warnInit)
  keeps its own terminator discipline — `vector A=0.!%{(}` was never
  bitten. Pins: TestXS_WarnLiteralBodyOnce (six rows) and the FuzzXS
  seed `void A(){0.!%{(}}`.
- The unterminated-string chain (#176) is closed on the same hunt: the
  parser clips the unclosed literal at the line end, so the statement's
  extent starts inside the dropped grouping — and the orphan `)` closing
  the grouping's own `(` canceled the unit of the call's `(` in the net
  count (`rule{("\n)("")}` printed `("\n)(""`, the reparse died inside
  the argument list). The depth is the residual the cut leaves
  (xsResidualKinds): junk closers leave the stack alone, the hunt spends
  the call's own `)`, and the walk back restores the grouping's opener —
  the cut carries the whole chain with its line break. Pins:
  TestXS_UnterminatedStringChainOnce (two rows) and the FuzzXS seed
  `rule{("\n)("")}`.
- The include tail-comment drift (#164) is closed: unclosedEOF's
  byte check reads a closing `*/` only where a closer can be — the
  suffix, with its `*` past the opener's own (the opener holds the
  first two bytes, so `/*/` and `/*/0` both end short of a real
  closer) — and the include line's cut stops before the unterminated
  tail instead of gluing it to the directive line, so the comment
  anchors on its own line from the first pass. A closed block at EOF
  still rides the line as written. Pins:
  TestXS_IncludeTrailingCommentOnce (six rows) and the FuzzXS seeds
  `include"";include"";/*/0` / `include""/*/`.
- The zero-call chain (#165) is closed on the expression path: the
  parser folds every zero-argument call suffix into its callee — one
  Call node whose extent stops at the callee — and the canonical print
  spent at most one "(" pair past it, so a second suffix was lost
  silently (`vector A=(A)()()` printed `(A)()`, `vector A=A()()` printed
  `A()()`, byte drift with no diagnostic) and a truncation that landed
  inside an argument list printed an unbalanced callee slice
  (`vector A=0(0())()` printed `0(0();`, the output stopped parsing).
  zeroCallChain now recovers the chain's own bytes as one cut: the
  grouping prefixes walk back, the extent's unclosed opens close one
  closer at a time (typed pairing), and every postfix "(" pair that
  immediately follows is consumed — the walk reads one code token at a
  time and ends at any token that cannot continue the chain, so a ";"
  boundary is never crossed (crossing it duplicated the next statement
  on every pass — the case41 fixture caught it). The cut prints through
  the same comment discipline as the callee cut: a // inside it lifts
  to its own line — collapsed into the cut, the chain's tail re-read as
  part of the comment and the shape drifted on the next pass (#177),
  while a block comment rides. The single-suffix and
  argument-capped shapes stay canonical. Pins:
  TestXS_ZeroCallChainOnce (ten rows) and the FuzzXS seeds
  `vector A=0(0())()` / `void A(){get(1)();(1, 2, 3)();}` /
  `void A(){00//\n()()}`.
- Convergence has known open escape classes — the list is not
  closed, fresh fuzzing keeps surfacing more.
  Every class below is pre-existing — each reproduces, byte for
  byte, on the task/format-xs tree before this run's delta and on
  the final tree (rechecked 2026-10-24) — and none of them enters
  the FuzzXS seed corpus: the seed corpus must stay green (a
  diverging corpus .xs is removed from the sample, not pinned as
  a seed). The fuzzer's pre-existing classes:
  the degenerate declarator tail
  `vector A000000;extern A00 A000000=(0);int A0000000000000000000;int A00000000=(0),A="`
  (the first pass's output does not reparse — `unexpected
  identifier "A" at top level`), the unterminated include-path
  literal with a trailing backslash (`include"\` — the first two
  passes grow the document by a byte, fixed point from pass 3),
  the warn-collapsed initializer with a second declarator
  (`vector A=(0.!),A` — output does not reparse, `expected ;`), the
  degenerate chain in a rule body (`rule { (""."") }` — output
  does not reparse, `expected ")"`), the same chain in a function
  body behind a bare block (`int A(A A){\n{\nwhile(0)\n\n0=0+0.}
  }A000}` — the bare block's `}` rides the degenerate statement's
  cut and the output does not reparse, `expected ;` — appended
  2026-10-25 from this run's battery, rechecked byte-for-byte on
  master), the warn-header switch with
  an empty body (`void A(){switch((0,0)){}}` — the body's `{}` pair
  is lost and the output does not reparse, `expected { after
  switch`), the grouped wrong-arity vector with a degenerate
  name (`void A(){((0.(,0));}` — the outer `(` is lost and
  `(0.(,0));` does not reparse, `expected ;`), the grouped
  zero-call chain (`vector A=(A)()()` — pass 1 prints
  `vector A = (A)();`, one `()` pair short of the input; pass 2
  unwraps the grouping — `vector A = A();`; fixed point from
  pass 3, and neither the input nor any pass output carries a
  diagnostic — a silent byte change, the reparse never breaks) and
  the paren-dropping zero-call chain (`vector A=0(0())()` —
  pass 1 prints `vector A = 0(0();`, one `)` short, and the output
  does not reparse, `expected , or ) in argument list`; the two
  remaining members appended 2026-10-25 from the format-xs run's
  backlog, each rechecked byte-for-byte on master. The split
  type-word-name residual above joins them as the batch's third
  `(0.(,0));` does not reparse, `expected ;`), the comment drift
  behind a doubled include (`include"";include"";/*/0` — pass 1
  glues `/*/0` to the second include line, pass 2 moves it to its
  own line; fixed point from pass 2, the reparse stays whole; the
  remaining member appended 2026-10-25 from the format-xs run's
  backlog, rechecked byte-for-byte on master. The split
  type-word-name residual above joins it as the batch's second
  member — narrowed out of the closed #162 class on
  task/xs-decl-head).
  An XS divergence outside this list is a finding to report
  just the same: not a bug in your calling code, not something
  to mask with a third pass.

The contracts are pinned by FuzzXS (any accepted input must reparse
and be idempotent; seeds: golden fixtures, the .corpus sample, pinned
reproducers from the fuzzing rounds) and the corpus invariant sweeps —
xs_invariants_test.go for XS (structure, idempotence, comments-once,
EOL dominance) and invariants_test.go for their RMS counterparts.
