# Hints Computing — consuming the hints cell

Domain: computing signature-help hints for RMS and XS positions.
Target audience: implementers of the server cell (the LSP handler).

## Construct once, share everywhere

```go
computer := hints.NewComputer(store) // kb.Store via constructor DI
```

## XS hint at a position

```go
// xsFile from xs.XsParse; external decls from include closure
// (Closure.ExternalDecls(uri)); for inline RMS blocks translate the
// cursor into block-local coordinates first (XsBlock.Range offset).
if hint, ok := computer.XsAt(xsFile, pos, externalDecls); ok {
    // hint.Label / hint.Params / hint.Active — protocol-agnostic
    // hint.Active == -1 → render with no active parameter (never clamp)
}
```

## RMS hint at a position

```go
if hint, ok := computer.RmsAt(rmsFile, pos); ok {
    // full kb-ordered list: positional Args first, then Attributes;
    // optional entries arrive pre-bracketed in the label strings
}
```

Preconditions:
- Parse the document first; hint answers are valid for that parse only.
- Silence (found=false) is the designed answer for unknown names, broken
  syntax, strings/comments, ambiguous mappings — never a guessed hint.
- The cell never imports go.lsp.dev — mapping Hint to
  protocol.SignatureInformation (single signature, ActiveSignature=0,
  ActiveParameter=nil when Active<0) belongs to the server cell.
