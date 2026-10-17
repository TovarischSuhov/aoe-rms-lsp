# RMS Includes — consuming directive data

Domain: include directives recorded by the rms parser (#include,
#include_drs / #includeXS with a file argument). Target audience:
implementers of the include and server cells.

## Directive records

Parse records every connection directive with the path argument's range
(not the whole directive line):

```go
file, diags := rms.Parse(text, uri)

for _, inc := range file.Includes {     // #include "parts/econ.rms"
    // also #include_drs includes/scaling.inc 54201 (DE form)
    // inc.Path == "parts/econ.rms"; inc.Range covers the path argument
}
for _, inc := range file.XsIncludes {   // #includeXS parts/lib.xs
    // external .xs script; the inline region AFTER the directive is a
    // separate XsBlock (bare #includeXS produces only an XsBlock)
}
```

Preconditions:
- A directive without a path argument yields a syntax Diagnostic and no
  Include record. `#include` and `#include_drs` require a file name; a
  bare `#includeXS` is legal (it only opens the inline region).
- `#include_drs` shares `Includes` with `#include`: `Path` is the first
  whitespace-delimited token, and a trailing legacy numeric id
  (`#include_drs Arabia.rms 54201`) is part of neither `Path` nor `Range`.
  Resolution stays includer-relative (same as `#include`): targets staged
  beside the map resolve, an arbitrary layout may need `includeRoots`.
- Paths are stored verbatim (relative); resolution against the including
  file's directory is the consumer's job.

## Other #-directive lines

`#undefine` and `#cost` are directives, not comments: both stay visible as
statements, and `#cost` (a `#const` typo) additionally reports an
`unknown-directive` warning with a did-you-mean hint. Do not classify those
lines as comments.

## Position hit-testing

inc.Range.Contains(pos) reports whether a cursor sits on the include
path — the trigger for go-to-definition into the target file.

