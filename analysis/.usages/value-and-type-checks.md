# Value & Type Helpers — granular checks from the analysis cell

Domain: single-value RMS checks and XS type inference for tooling that needs
more than the batch Analyzer pass (CLI linters, future typed features).
Target audience: implementers of CLI lint tooling and the server cell.

## Check one RMS argument/attribute value

```go
spec, found := store.Attribute("create_land", "percent")
if found {
    v := attr.Value // rms.Expr
    if diag, reported := analysis.CheckRmsValue(spec, v.Kind, v.Value, v.Range); reported {
        // bad-argument-value (error): percent outside 0..100
    }
}
```

## Infer the type of an XS expression

```go
env := analysis.NewTypeEnv(xsFile)
env.Push() // тело функции: параметры и локалы
env.Declare("count", "int")
defer env.Pop()
typ := analysis.InferType(store, env, expr) // "" — не выведен: пропустить
```

## Type compatibility

```go
analysis.Coerce("float", "int") // true: int расширяется до float
analysis.Coerce("int", "float") // false
```

Preconditions:
- Push before declaring params/locals of a function body, Pop after;
  top-level symbols are preloaded from XsFile.Decls.
- InferType "" means "unknown" — treat as no-check, never as error.

Constraints:
- All helpers are pure: no IO, no mutation of inputs.
