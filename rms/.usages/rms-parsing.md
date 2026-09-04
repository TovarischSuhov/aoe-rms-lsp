# RMS Parsing — consuming the rms cell

Domain: parsing RMS sources and navigating the resulting AST.
Target audience: implementers of the analysis and server cells.

## Parse and collect diagnostics

Parse never fails: a partial File comes back alongside syntax diagnostics.

```go
file, diags := rms.Parse(text, uri)
// diags: syntax problems, sorted by position — merge with analyzer output

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
