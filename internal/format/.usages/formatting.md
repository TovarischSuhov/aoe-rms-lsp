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
- Convergence has one documented open escape class — a doubled
  synthesized `;` in warnInit (an unpaired bracket inside an
  initializer block literal keeps the balance positive, the
  separator search sees no `;` at zero depth and the cut swallows
  the synthesized semicolon; repro `vector A=(0,0)%{(0}`). The
  output grows pass over pass — no later pass converges, unlike
  the RMS escapes. An XS divergence outside the open class is a
  finding to report: not a bug in your calling code, not something
  to mask with a third pass. Inputs hitting the open class never
  enter the FuzzXS seed corpus — the seed corpus must stay green
  (a diverging corpus .xs is removed from the sample, not pinned
  as a seed). The include-CR escape (#150) is closed in the same
  #120 change-set that introduced this printer: p.include trims
  the lone trailing \r an unterminated include-path literal
  carries out of its line — blockText's page-ending rule — pinned
  by the TestXS_IncludeTrailingCR rows.

The contracts are pinned by FuzzXS (any accepted input must reparse
and be idempotent; seeds: golden fixtures, the .corpus sample, pinned
reproducers from the fuzzing rounds) and the corpus invariant sweeps —
xs_invariants_test.go for XS (structure, idempotence, comments-once,
EOL dominance) and invariants_test.go for their RMS counterparts.
