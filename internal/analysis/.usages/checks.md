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
all = append(all, analyzer.AnalyzeXs(xsFile, nil)...)
// both sorted by position; publish as one batch
```

## XS with an include closure

```go
// XS with an include closure behind it: pass the closure's declarations —
// names declared in included .xs files no longer fire undefined-symbol.
externals := closure.ExternalDecls(uri) // exclude the analyzed file itself
diags := analyzer.AnalyzeXs(xsFile, externals)
// nil externals — same behavior as before the parameter existed
```

## Diagnostic codes

| Code | Severity | Meaning |
|---|---|---|
| unknown-command | error | RMS-команда не найдена в kb |
| unknown-section | error | секция не из справочника |
| unknown-attribute | error | атрибут не принадлежит команде |
| bad-argument | error | число/вид аргументов против спецификации |
| bad-argument-value | error | значение аргумента вне диапазона (percent 0..100) |
| deprecated-effect-percent | warning | effect_percent устарел в пользу операторов (из kb nuances.json; будущие deprecated-имена получают свои коды из тех же данных) |
| typo-alias | warning | имя — известная опечатка; сообщение называет правильное написание (данные kb nuances.json) |
| probably-ignored | hint | атрибут — задокументированный no-op в этой позиции, игра его игнорирует (данные kb nuances.json) |
| legacy-… | hint | #-директива устарела механически, но работает (код и текст из kb nuances.json) |
| undefined-symbol | error | ident вне объявлений и kb |
| bad-arity | error | неверное число аргументов вызова |
| bad-type | error | несовместимый тип аргумента/присваивания/return |

## Nuance ladder for unknown names

Before reporting unknown-attribute / unknown-command, the analyzer checks
the kb nuance data — soft diagnoses replace the error when the name is a
documented mistake rather than a real unknown:

```go
// no-op pair → hint, the game ignores the attribute anyway
// attribute "border_fuzziness" of "create_terrain" is probably ignored by the game
// typo alias → warning naming the fix
// unknown attribute "group_varience" of "create_object"; did you mean "group_variance"?
```

- no-op pairs are scoped to their command: the same attribute under
  another command stays plain unknown-attribute
- bare attribute lines outside braces attach to the previous command and
  run the same ladder
- deprecated/legacy markers are data entries (nuances.json): the code and
  message come verbatim from the data, so new markers never need analyzer
  changes
- suppression is per code via the server's severityOverrides ("none"
  hides the diagnostic) — no analyzer involvement

## Did-you-mean suggestions

unknown-command, unknown-attribute and undefined-symbol append the
closest known name to the message when one is close enough:

```go
// typo'd command → message suggests the fix
// unknown command "creat_object"; did you mean "create_object"?
```

- candidates: unknown-command — every kb command; unknown-attribute —
  the known command's attributes; undefined-symbol — kb function and
  constant names plus the file's declared names (a typo'd local
  suggests the local)
- closeness: case-insensitive Levenshtein distance ≤ max(1, len/4);
  ties resolve to the lexicographically smaller name — suggestions
  are deterministic
- nothing within the threshold → plain message, no suffix
- unknown-section never carries a suggestion

Preconditions:
- Input must come from a successful Parse call (partial AST is fine —
  analyzer walks what exists).
- Analyzer does not re-report syntax problems.

Constraints:
- No IO, no mutation of the AST.

## Semantic tokens

TokensRms / TokensXs classify identifiers for semanticTokens/full — the
diagnostic pass's classification exposed as ranges instead of problems.

```go
for _, tok := range analyzer.TokensRms(rmsFile) {
    // tok.Type: known / unknown / deprecated / section / kind
    // tok.Range: exact source span of the identifier
}
```

- known/unknown use the same lookups as the diagnostics (commands,
  attributes, XS idents); deprecated marks effect_percent; section
  covers RMS section names (the span inside the angle brackets); kind
  covers XS declaration names (overriding known on that span).
- No did-you-mean thresholds apply — tokenization never suggests.
- The result is sorted by position; spans never overlap.

Preconditions: same as the diagnostic passes — parse first, partial
ASTs are fine, no IO, the AST is not mutated.
