# XS Parsing — consuming the xs cell

Domain: parsing XS sources (standalone .xs files and inline RMS blocks)
and navigating the AST. Target audience: implementers of the analysis
and server cells.

## Parse and collect diagnostics

```go
xsFile, xsDiags := xs.XsParse(text, uri)
// partial AST + syntax diagnostics sorted by position

## Symbol lookup (hover, completion)

```go
if name, ok := xsFile.SymbolAt(pos); ok {
    if fn, found := store.Function(name); found {
        // hover: fn signature + desc; completion trigger on "("
    }
}

Preconditions:
- Parse accepts any C-like input incl. the 5k-line prelude.xs fixture —
  do not pre-validate the source.
- SymbolAt on a call expression returns the callee name, not the argument.

## Navigation (definition, references, outline)

Position-based navigation over the parsed file — the LSP server calls
these directly, no extra context required.

```go
// definition: identifier occurrence -> declaring name range (innermost
// enclosing declarer wins: param > local > top-level). Builtins and
// unknown names return found=false.
if r, ok := xsFile.Definition(pos); ok {
    // jump target: Location{URI: uri, Range: toProtocolRange(r)}
}

// references: every occurrence of the name under pos, declaration
// included, sorted by position. Name resolution is syntactic —
// same-name symbols from different scopes are not distinguished.
for _, r := range xsFile.ReferencesAt(pos) { ... }

// outline: top-level declarations as Symbol nodes (flat); include
// directives are skipped — the kind vocabulary has no entry for them.
syms := xsFile.Symbols() // []common.Symbol
```

Preconditions:
- Parse the document first; navigation answers from the occurrence index
  the parser recorded — ranges are valid for that parse only.
- Definition on a declaration name returns that declaration itself.

