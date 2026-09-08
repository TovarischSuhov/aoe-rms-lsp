# KB Lookups — consuming the kb cell

Domain: querying the AoE2 RMS+XS knowledge base. Target audience:
implementers of the analysis and server cells.

## Construct once, share everywhere

NewStore() validates and indexes the embedded JSON. Build one Store per
process and inject it as an explicit constructor parameter.

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

## Mined kind/range on CommandArg

CommandArg carries structured Range (min/max strings, "" when unmined) and
Kind filled at load when the extractor left it empty. Kind/Range are raw
strings from the guide prose — consumers render them per their own contract
(rendering rules live in the hints cell).
