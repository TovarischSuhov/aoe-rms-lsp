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
- Convergence has two known escape classes (the knownDivergent list,
  and the flat #-line with an unterminated /* at EOF) — both converge
  from the second pass; the list is closed and pinned.

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
- Convergence has known open escape classes — the list is not
  closed, fresh fuzzing keeps surfacing more (the grouped
  zero-call chain below surfaced in this run's fuzz re-check).
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
  does not reparse, `expected ")"`), the warn-header switch with
  an empty body (`void A(){switch((0,0)){}}` — the body's `{}` pair
  is lost and the output does not reparse, `expected { after
  switch`), the grouped wrong-arity vector with a degenerate
  name (`void A(){((0.(,0));}` — the outer `(` is lost and
  `(0.(,0));` does not reparse, `expected ;`) and the grouped
  zero-call chain (`vector A=(A)()()` — pass 1 prints
  `vector A = (A)();`, one `()` pair short of the input; pass 2
  unwraps the grouping — `vector A = A();`; fixed point from
  pass 3, and neither the input nor any pass output carries a
  diagnostic — a silent byte change, the reparse never breaks).
  An XS divergence outside this list is a finding to report
  just the same: not a bug in your calling code, not something
  to mask with a third pass.

The contracts are pinned by FuzzXS (any accepted input must reparse
and be idempotent; seeds: golden fixtures, the .corpus sample, pinned
reproducers from the fuzzing rounds) and the corpus invariant sweeps —
xs_invariants_test.go for XS (structure, idempotence, comments-once,
EOL dominance) and invariants_test.go for their RMS counterparts.
