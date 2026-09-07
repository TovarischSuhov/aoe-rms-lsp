# Include Closure — consuming the include cell

Domain: resolving the #include/#includeXS closure of a document and
answering cross-file navigation over it. Target audience: implementers of
the server cell.

## Wiring

Provide editor-state text via Source; DocStore satisfies it structurally
through its Text method:

```go
docs := server.NewStore()               // has Text(uri) (string, bool)
resolver := include.NewResolver(docs)   // include.Source is satisfied
```

Preconditions:
- Source must reflect the CURRENT editor state — the resolver never
  overrides an open document with its disk copy (editor state wins).
- Cache invalidation: disk files are reloaded when size/mtime change;
  files changed outside the editor without a stat change are not tracked.

## Closure and missing includes

```go
closure := resolver.Closure(ctx, uri)
// closure.Rms / closure.Xs — parsed files in DFS directive order
// closure.Resolved — every resolved directive (owner, Include, target URI)
for _, m := range closure.Missing {
    // Diagnostic{Range: m.Range, Severity: error, Code: "missing-include",
    //            Message: "include not found: " + m.Path} — publish for m.Owner
}
```

## Navigation (definition / references)

```go
if t, ok := resolver.Definition(ctx, uri, pos); ok {
    // protocol.Location{URI: t.URI, Range: toProtocol(t.Range)}
}
for _, t := range resolver.References(ctx, uri, pos) { /* []Location */ }
```

Preconditions:
- References search the closure of the queried file PLUS the closures of
  open documents whose closure contains it (reverse direction: who
  includes me — Source.URIs). Occurrences are deduplicated by
  (URI, Range).
- References include the declaration occurrence — filter the local
  declaration range yourself when the client sends
  includeDeclaration=false.
- Definition returns found=false for builtins and unknown names — an
  empty LSP result, not an error.

## External declarations for analysis

```go
externals := closure.ExternalDecls("") // all closure XS declarations
diags := analyzer.AnalyzeXs(xsFile, externals)
// undefined-symbol no longer fires for names declared in included .xs files
```
