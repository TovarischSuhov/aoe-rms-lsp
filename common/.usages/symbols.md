# Symbols — consuming the common cell

Domain: building and consuming the document outline tree. Target audience:
implementers of the xs and rms cells (producers) and the server cell
(consumer that maps the tree to LSP DocumentSymbol).

## Outline node

Symbol is a pure data node; producers construct it while walking their AST.
Selection is the identifier range, Range covers the whole construct.

```go
sym := common.Symbol{
    Kind:      "function",
    Name:      "regenerateMap",
    Range:     declRange,  // whole declaration
    Selection: nameRange,  // identifier token; must be inside Range
    Children:  nil,        // leaf
}
```

Preconditions:
- Selection ⊆ Range (LSP rejects DocumentSymbol otherwise); children
  ranges lie inside the parent Range.
- Kind vocabulary belongs to the producer: xs — function/variable/rule/
  event/extern; rms — section/command/xs. Consumers map the string to
  protocol.SymbolKind; unknown kinds fall back to a generic kind.

Constraints:
- Construct-and-use data: do not mutate a built tree.
