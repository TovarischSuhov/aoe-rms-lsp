# Semantic Checks — consuming the analysis cell

Domain: running semantic diagnostics over parsed RMS/XS files.
Target audience: implementers of the server cell and CLI lint tooling.

## Construct with the knowledge base

```go
analyzer := analysis.NewAnalyzer(store)
```

## Analyze and merge with syntax diagnostics

```go
rmsFile, syntaxDiags := rms.Parse(text, uri)
all := append(syntaxDiags, analyzer.AnalyzeRms(rmsFile)...)
xsFile, xsDiags := xs.XsParse(block.Code, "inline:"+uri)
all = append(all, analyzer.AnalyzeXs(xsFile)...)
// both sorted by position; publish as one batch
```

## Diagnostic codes

| Code | Severity | Meaning |
|---|---|---|
| unknown-command | error | RMS-команда не найдена в kb |
| unknown-section | error | секция не из справочника |
| unknown-attribute | error | атрибут не принадлежит команде |
| bad-argument | error | число/вид аргументов против спецификации |
| bad-argument-value | error | значение аргумента вне диапазона (percent 0..100) |
| deprecated-effect-percent | warning | effect_percent устарел в пользу операторов |
| undefined-symbol | error | ident вне объявлений и kb |
| bad-arity | error | неверное число аргументов вызова |
| bad-type | error | несовместимый тип аргумента/присваивания/return |

Preconditions:
- Input must come from a successful Parse call (partial AST is fine —
  analyzer walks what exists).
- Analyzer does not re-report syntax problems.

Constraints:
- No IO, no mutation of the AST.
