# XS Grammar — AoE2 Definitive Edition

Domain: the XS scripting language structure. Target audience: implementers
of the xs, analysis and server cells. Sources: docs/ref/ugc-guide/xs/
(programmer.md, functions.json), docs/ref/ugc-guide/xs/prelude.xs (externs).

## Program

C-like. Top level declarations:

    void main() { ... }                       — entry point
    int/float/bool/string/vector <name>;      — variables
    <type> <name>(<params>) { ... }           — functions
    rule <name> [inactive] [min-interval X] { condition ... action ... }
    event(...)
    extern <decl>;                            — extern declaration (prelude.xs)
    include "file.xs" / includeDuno "..."     — includes

## Types

int, float, bool, string, void, vector. Vector literals: `(1.0, 2.0, 3.0)`.
Numbers: decimal int, float, hex `0x1F`. Strings: double-quoted, escapes.

## Statements

`{ }` blocks; `if/else`, `while`, `do/while`, `for(init; cond; step)`,
`switch/case/default/break`, `return [expr]`, `break`, `continue`,
expression statements. Semicolons terminate simple statements.

## Expressions

calls (`callee(args)` — Expr.Kind=call), binary ops `+ - * / % == != < <= > >=
&& || & | ^ << >>` and assignment forms, unary `! - ~ ++ --`, literals,
identifiers, vector member access `v.x|v.y|v.z`, vector constructors.

## Rules and events

`rule` bodies use special statement forms (`condition`, `action`,
`xsSetRule...` runtime calls). Parse permissively: unknown statements inside
rules must not break recovery. Events use `event(name, handler)` form.

## Externs (prelude.xs)

`extern` declarations carry doc comments — parse signature only, no body.

## Recovery

On error: emit Diagnostic, resynchronize to next `;` or `}` (statement level)
or next top-level keyword (declaration level). Never panic, never fail hard.

## Testing fixtures

docs/ref/ugc-guide/xs/prelude.xs (5k lines, 882 externs) must parse with zero
false positives; typical inline XS from RMS maps likewise.
