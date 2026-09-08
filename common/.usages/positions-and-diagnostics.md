# Positions and Diagnostics — consuming the common cell

Domain: constructing and comparing source positions, and reporting problems as
diagnostics. Target audience: implementers of the rms, xs, analysis and server cells.

## Positions

Positions are zero-based, LSP-aligned: `line` and `column` count from 0.
Order follows (line, column); `offset` is a byte offset kept alongside for O(1) slicing.

```go
start := common.Pos{Line: 3, Column: 0, Offset: 42}
end := common.Pos{Line: 3, Column: 13, Offset: 55}
r := common.Range{Start: start, End: end}

if r.Contains(common.Pos{Line: 3, Column: 7, Offset: 49}) { ... }
```

## Diagnostics

One shape for every producer (rms.Parse, xs.XsParse, Analyzer). Construct with a
stable `code` — tests assert on codes, not on message text.

```go
d := common.Diagnostic{
    Range:    r,
    Severity: common.SeverityError, // 1 error, 2 warning, 3 info, 4 hint
    Message:  "unknown command 'create_elefant'",
    Code:     "unknown-command",
}
```

Preconditions:
- `Range.End` >= `Range.Start` for diagnostics you emit; the editor drops invalid ranges.
- Use only severities 1–4 (LSP numbering) — do not invent new values.

Constraints:
- Positions are comparable data; do not mutate them after construction.
