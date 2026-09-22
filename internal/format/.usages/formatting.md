# Formatting — consuming the format cell

Domain: reformatting RMS sources into the cell's canonical style.
Target audience: implementers of the server cell (the
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
- The cell is stateless: call per request, no caching obligations.

## Guarantees to rely on

- parse(format(x)) is structurally equal to parse(x): replacing the
  document with the output never introduces new syntax errors on
  input the parser accepted.
- format(format(x)) is byte-identical to format(x): formatting an
  already formatted document produces no diff.
- Every input comment is present in the output (texts byte-exact);
  inline XS blocks pass through verbatim.
- The output keeps the input's dominant EOL (CRLF stays CRLF).
