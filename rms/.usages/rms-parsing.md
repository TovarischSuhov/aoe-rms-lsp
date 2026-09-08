# RMS Parsing — consuming the rms cell

Domain: parsing RMS sources and navigating the resulting AST.
Target audience: implementers of the analysis and server cells.

## Parse and collect diagnostics

Parse never fails: a partial File comes back alongside syntax diagnostics.

```go
file, diags := rms.Parse(text, uri)
// diags: syntax problems, sorted by position — merge with analyzer output
```

## Position navigation (hover, completion context)

```go
if stmt, ok := file.StatementAt(pos); ok {
    // hover: stmt.Name is the command under (or owning) the cursor
}
if sec, ok := file.SectionAt(pos); ok {
    // completion: scope command list to sec.Name via kb.Store.Commands(sec.Name)
}

## Inline XS delegation

The rms parser does not parse XS. For every embedded block, hand the code
to the xs parser and merge diagnostics with the block's offset applied:

```go
for _, block := range file.XsBlocks {
    xsFile, xsDiags := xs.XsParse(block.Code, "inline:"+uri)
    // shift xsDiags ranges by block.Range.Start before publishing
}

Preconditions:
- StatementAt on an attribute position returns the owning command —
  do not re-walk Attributes yourself.
- XsBlock.Code positions are relative to the block, not the document.

## Navigation (outline, references)

```go
// outline: section tree with command children and #includeXS nodes;
// Selection ⊆ Range on every node; nested random/conditional blocks
// become Children.
syms := file.Symbols() // []common.Symbol, kinds: section/command/xs

// references: every word-token matching the name under pos (section,
// command, attribute and identifier words), sorted by position. RMS has
// no local declarations — all occurrences are equal.
for _, r := range file.ReferencesAt(pos) { ... }

// by-name form for cross-file searches: same occurrences without a
// position in this file.
for _, r := range file.References(name) { ... }
```

Preconditions:
- Navigation answers from the token/occurrence index recorded at parse
  time — reparse before querying after text changes.

## Argument lookup (signature help)

ArgAt answers "which command owns the cursor, and which argument or
attribute am I on" — for hint providers. Owner resolution follows
StatementAt semantics (works on trailing positions and unclosed blocks);
the added value is argument discrimination.

```go
if site, ok := file.ArgAt(pos); ok {
    // site.Stmt — owning command statement (attrs attach to the preceding
    //             command; nested blocks own their innermost statement)
    // site.Kind — "arg" | "attr" | "none"
    // site.Index — 0-based positional ordinal (Kind="arg")
    // site.Name — attribute name (Kind="attr"), active by name-match
}
```

Preconditions:
- Positions inside strings/comments and on directives/section headers
  return found=false — render no hint.
- Ambiguous mappings return Kind="none" — render the signature with no
  active mark rather than a guessed one.
- The consumer (hints) maps a positional ordinal to the kb Args list and
  an attribute name to its index in kb Attributes — RMS has no local
  declarations to consult.

