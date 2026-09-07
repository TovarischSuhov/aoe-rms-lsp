# RMS Includes — consuming directive data

Domain: include directives recorded by the rms parser (#include /
#includeXS with a file argument). Target audience: implementers of the
include and server cells.

## Directive records

Parse records every connection directive with the path argument's range
(not the whole directive line):

```go
file, diags := rms.Parse(text, uri)

for _, inc := range file.Includes {     // #include "parts/econ.rms"
    // inc.Path == "parts/econ.rms"; inc.Range covers the path argument
}
for _, inc := range file.XsIncludes {   // #includeXS parts/lib.xs
    // external .xs script; the inline region AFTER the directive is a
    // separate XsBlock (bare #includeXS produces only an XsBlock)
}
```

Preconditions:
- A directive without a path argument yields a syntax Diagnostic and no
  Include record.
- Paths are stored verbatim (relative); resolution against the including
  file's directory is the consumer's job.

## Position hit-testing

inc.Range.Contains(pos) reports whether a cursor sits on the include
path — the trigger for go-to-definition into the target file.

## By-name references

```go
for _, r := range file.References(name) { ... }
// every word-token equal to name (section/command/attribute/ident words),
// sorted by position; ReferencesAt(pos) ≡ References(word under pos)
```
