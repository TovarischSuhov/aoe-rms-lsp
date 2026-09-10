# XS Parsing — consuming the xs cell

Domain: parsing XS sources (standalone .xs files and inline RMS blocks)
and navigating the AST. Target audience: implementers of the analysis
and server cells.

## Parse and collect diagnostics

```go
xsFile, xsDiags := xs.XsParse(text, uri)
// partial AST + syntax diagnostics sorted by position
```

## Symbol lookup (hover, completion)

```go
if name, ok := xsFile.SymbolAt(pos); ok {
    if fn, found := store.Function(name); found {
        // hover: fn signature + desc; completion trigger on "("
    }
}
```

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

// by-name references: for files where no position is known (cross-file
// searches over an include closure). Includes the declaration occurrence;
// ReferencesAt(pos) ≡ References(name under pos).
for _, r := range xsFile.References(name) { ... }

// cross-file definition lookup: Symbols() carries each top-level decl's
// name range in Selection — match by Name to find the jump target when
// the declaring file is not the queried one.
for _, sym := range xsFile.Symbols() {
    if sym.Name == name { /* target: sym.Selection */ }
}

// outline: top-level declarations as Symbol nodes (flat); include
// directives are skipped — the kind vocabulary has no entry for them.
syms := xsFile.Symbols() // []common.Symbol
```

Preconditions:
- Parse the document first; navigation answers from the occurrence index
  the parser recorded — ranges are valid for that parse only.
- Definition on a declaration name returns that declaration itself.

## Rename sites (rename, prepareRename)

RenameSites answers "which occurrences belong to the binding under the
cursor" — scope-aware, unlike the syntactic ReferencesAt. Binding
resolution matches Definition (innermost enclosing declarer wins:
param > local > top-level; for-init declarations are scoped to the
statement).

```go
if sites, ok := xsFile.RenameSites(pos); ok {
    // sites[].Range — name range of each occurrence (declaration
    //                  included), sorted by position
    // sites[].Kind — the binding's kind: function | variable | rule |
    //                event | extern (top-level) | param | local
    // sites[].Decl — true on declaring occurrences
}
// ok=false → not renameable: cursor off an identifier, or the name is
// builtin / unknown (no declaring binding)
```

Preconditions:
- Parse the document first; sites answer from the occurrence index
  recorded at parse time.
- All sites of one call share the same Kind — it describes the binding,
  not the occurrence. Cross-file merging over the include closure
  belongs to the server: top-level kinds merge across files, param/local
  stay file-local. A same-name occurrence resolving to a different
  (shadowing) binding is NOT part of the result.

## Call-site lookup (signature help)

CallAt answers "which call encloses the cursor, and which argument am I
on" — for hint providers. It works on in-progress (unbalanced) calls: a
cursor just after "(" maps to argument 0, after a comma to the next index.

```go
if cs, ok := xsFile.CallAt(pos); ok {
    // cs.Callee — innermost callee name (nested calls: inner wins)
    // cs.ArgIndex — 0-based ordinal, valid when cs.OnArg is true
    // cs.OnArg — false when the cursor sits on the callee name:
    //            render the signature with no active parameter
}
```

Preconditions:
- Only call expressions create a call context — vector literals "(1,2,3)",
  if/while/for conditions, declaration parameter lists and grouping parens
  are not calls (found=false unless an enclosing call contains the position).
- Positions inside strings/comments never resolve to a call.
- ArgIndex may exceed the declared parameter count — the consumer decides
  (signature-help policy: never clamp, show no active parameter instead).

## Visible symbols (completion)

VisibleAt answers "which named symbols can be referenced at this
position" — for completion providers. It combines file-scope
declarations with the parameters and locals of the function enclosing
the cursor.

```go
if syms, ok := xsFile.VisibleAt(pos); ok {
    // ok=false → cursor inside a string or comment: render nothing.
    // sym.Kind — "function" | "variable" | "rule" | "event" | "extern"
    //             | "param" | "local"; sym.Name — the symbol name
    // pair with kb functions/constants for the full candidate set;
    // the consumer decides priority when one name appears twice
}
```

Preconditions:
- Parse the document first; visibility answers from the body index
  recorded at parse time.
- Shadowing is not resolved: an outer top-level `int x` and an inner
  `float x` both come back — deduplication policy belongs to the
  consumer.
- For-loop init declarations (`for (int i = ...)`) are locals scoped to the
  for statement: visible in the initializer, condition, step and body,
  not after the statement.
- `include` declarations are skipped (not name-bearing for completion).

