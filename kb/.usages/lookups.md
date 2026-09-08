# KB Lookups — consuming the kb cell

Domain: querying the AoE2 RMS+XS knowledge base. Target audience:
implementers of the analysis and server cells.

## Construct once, share everywhere

NewStore() validates and indexes the embedded JSON. Build one Store per
process and inject it (constructor DI per `conventions`).

```go
store, err := kb.NewStore()
if err != nil {
    return fmt.Errorf("load knowledge base: %w", err)
}
```

## Exact-name lookups (hover, validation)

```go
fn, found := store.Function("xsGetMapSeed")
if !found {
    // emit "unknown function" diagnostic
}
cmd, found := store.Command("create_elevator")
arg, found := store.Attribute("create_elevator", "number_of_objects")
```

## List lookups (completion)

```go
for _, fn := range store.Functions() { /* completion items */ }
for _, c := range store.Constants("") { /* all sections */ }
for _, cmd := range store.Commands("land_generation") { /* section-scoped */ }
```

Preconditions:
- Names are case-sensitive; completion should lowercase-filter client-side.
- SinceUpdate is "" when the version is unknown — treat as "always existed".

## Mined kind/range on CommandArg (signature help rendering)

CommandArg carries structured Range (min/max strings, "" when unmined) and
Kind filled at load when the extractor left it empty. Consumers rendering
argument lists (hints) format an entry as "Name: Kind Min..Max" when Range
is present, "Name: Kind" otherwise, and name-only for flag attributes —
never invent kinds or defaults.
