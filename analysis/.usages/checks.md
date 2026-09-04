# Semantic Checks — consuming the analysis cell

Domain: running semantic diagnostics over parsed RMS/XS files.
Target audience: implementers of the server cell and CLI lint tooling.

## Construct with the knowledge base

```go
analyzer := analysis.NewAnalyzer(store)

## Analyze and merge with syntax diagnostics

```go
rmsFile, syntaxDiags := rms.Parse(text, uri)
all := append(syntaxDiags, analyzer.AnalyzeRms(rmsFile)...)
// both sorted by position; publish as one batch

Preconditions:
- Input must come from a successful Parse call (partial AST is fine —
  analyzer walks what exists).
- Analyzer does not re-report syntax problems.
Constraints:
- No IO, no mutation of the AST.
