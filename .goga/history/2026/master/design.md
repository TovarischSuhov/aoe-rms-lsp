# Design Document: `master`

<!-- Topic: Signature Help for AoE2 RMS + XS — implementation design over the
materialized cell contracts (architecture plan: .goga/history/2026/master/arch.md) -->

## Contract Changes

### Changed CODEMANIFEST Files

- `kb/CODEMANIFEST`: `kbdata` inline usage grew a «Desc prose mining» block;
  global Annotations grew the mined-data provenance rule; `Store`
  Requirements grew the load-time mining rule; `CommandArg` gained the
  `Range -> ValueRange` property and the Kind-after-load annotation; new
  entities `ValueRange` (model.go) and `MineKindRange` (mine.go);
  `ExtractRmsCommands` Algorithm step 3 grew the mining pass.
- `xs/CODEMANIFEST`: global Annotations grew the call-context rule (only
  Expr Kind=call creates a call context); `XsFile` gained method
  `CallAt(pos) -> (CallSite, found)`; new entity `CallSite` (ast.go).
- `rms/CODEMANIFEST`: `RmsFile` gained method `ArgAt(pos) -> (ArgSite,
  found)`; new entity `ArgSite` (ast.go).
- `hints/CODEMANIFEST`: **new cell** — `Computer` (computer.go) with
  `XsAt`/`RmsAt`, `Hint` (hint.go); imports common/kb/xs/rms.
- `server/CODEMANIFEST`: Imports grew `Computer`, `Hint`, `computing` from
  `hints`; global Annotations grew the hints wiring paragraph; `Server`
  constructor gained the `computer` parameter; `Initialize` gained
  SignatureHelpProvider with triggers «(» and «,»; new method
  `SignatureHelp`; `Serve` Algorithm wired `NewComputer(store)`;
  Description updated.

### New Entities

- `kb.ValueRange(min: string, max: string)` — mined value bounds (model.go).
- `kb.MineKindRange(desc: string) -> kind: string, r: ValueRange` — Desc
  prose parser (mine.go).
- `xs.CallSite(callee: string, argIndex: int, onArg: bool)` — call context
  under the cursor (ast.go).
- `rms.ArgSite(stmt: Statement, kind: string, index: int, name: string)` —
  argument context under the cursor (ast.go).
- `hints.Computer(store: Store)` — hint computer (computer.go).
- `hints.Hint(label: string, params: []string, active: int)` — rendered
  protocol-agnostic hint (hint.go).

### Changed Entities

- `kb.Store` — load-time mining requirement (fill-when-empty, idempotent
  with extraction).
- `kb.CommandArg` — `Range -> ValueRange` property; Kind semantics after
  load.
- `kb.ExtractRmsCommands` — mining pass per CommandArg.
- `xs.XsFile` — `CallAt` method.
- `rms.RmsFile` — `ArgAt` method.
- `server.Server` — third constructor dependency `computer: Computer`;
  `SignatureHelp` method; capability delta in `Initialize`.
- `server.Serve` — dependency assembly includes `NewComputer(store)`.

### Deleted Entities

- None.

### Usages and Annotations Changes

- `kb` `Usages.kbdata` (inline) — appended «Desc prose mining (signature
  help)» block: bounds shape «(<num>-<num>)», decimals/negatives, only
  numeric bounds match, percent-typed args keep structured kind, flag
  attributes stay name-only.
- `xs` global Annotations — call-context rule: only Expr Kind=call;
  vector literals, if/while/for conditions, declaration parameter lists
  and grouping parens are not calls.
- `hints` (new manifest) — header Imports (Pos; Store/Function/Command/
  CommandArg/ValueRange; XsFile/CallSite/Decl; RmsFile/ArgSite/Statement),
  `conventions` usage, global annotations with the truth-model statement.
- `server` — `computing` import usage; hints wiring paragraph in global
  Annotations.
- Cell-level `.usages/` files already updated by the apply stage (see
  «`.usages/` Update» below — verified current, no further edits).

## Applied Fixes

### Fixed CODEMANIFEST Defects

- Phase 3 audit (DSL syntax, key casing, `location` values, import
  closure acyclicity, four consistency dimensions) found no defects;
  `goga lint` reported 8 cells / 0 errors. The apply stage had already
  repaired the pre-existing code-fence pairing in the three kb/xs usage
  files it scoped.
- **Design review** found and fixed one contradiction: `hints` `Hint`
  example «[z: float]» (Name-colon-Type) vs `XsAt` step 3's «<Type>
  <Name>» — resolved Type-first; the example is now «[float z]» in
  `hints/CODEMANIFEST` (and the matching `lsp-protocol` example). See
  Applied Fix 5 below.

**Ambiguities resolved by deterministic design decisions (no contract
edit required — recorded here for the implementer):**

1. `hints.Computer.XsAt` step 2 — «совпадающие (последовательности пар
   Type+Name равны) → единая декларация» does not say which declaration
   renders when equal-parameter duplicates differ in return type.
   Decision: the **first declaration in pool order** (document `Decls` in
   source order, then `external` in closure DFS order) supplies the label.
   Pool order is deterministic and matches the «документ и замыкание —
   один пул» truth model.
2. `xs.XsFile.CallAt` — the innermost call among nested spans is selected
   by **latest `(` position** among the candidate call records whose span
   contains `pos`. For nested calls the inner call opens later; for
   postfix chains (`f(1)(…)`) the later `(` is the suffix call the cursor
   is inside. Ties cannot occur between distinct `(` positions.
3. `rms.RmsFile.ArgAt` step 1 — «семантика StatementAt, включая хвостовые
   позиции» is implemented as a dedicated band-based owner lookup
   (`ownerAt`), not a call to the existing `StatementAt`: the existing
   fallback (`endsBefore`, strictly earlier line) misses same-line
   trailing positions (cursor past the last argument — the key
   signature-help moment) and can return an *earlier* statement. `ownerAt`
   fixes this without touching `StatementAt` (hover behavior unchanged,
   SC8).
4. Directive-styled statements (`#const`, `#define`, `#include_drs` — the
   parser materializes them as command statements) are filtered inside
   `ArgAt` (`found=false` when the owner name starts with `#`), matching
   the contract text «директивы #const/#include … → found=false». The
   end-to-end result equals silence either way (`Store.Command("#const")
   → not found`), but the navigation-level contract is honored exactly.
   The `#`-prefix filter alone is not sufficient: `#include`/
   `#includeXS` lines and section headers materialize **no** statement,
   so under band semantics a preceding command would own them — the
   review added an `excluded` span index (header lines, include-directive
   lines incl. path arguments, plain `#`-comment lines) so these
   positions answer `found=false` categorically, as the contract's
   exclusion list and `rms-parsing` consumer doc state.
5. kb optional-param label form — the hints contract contained a
   contradiction: `XsAt` step 3 prescribes «<Type> <Name>» (Type-first)
   while the `Hint` example showed «[z: float]» (Name-colon-Type, the
   RMS style). Resolved as **Type-first everywhere for XS kb params**
   («[float z]», «[bool append]»): the canonical label example
   «vector xsVectorSet(float x, float y, float z)» is Type-first, and
   the existing `functionSignature` renderer (hover/completion detail)
   is Type-first. The «[z: float]» fragments in `hints/CODEMANIFEST`
   and `lsp-protocol` were typos mirroring the RMS colon form — both
   fixed by the design review. RMS labels («Name: Kind Min..Max») are
   unaffected.

## Entity Interaction and Data Flow

### Interaction Diagram

```
                 LSP client (editor)
                        |  textDocument/signatureHelp (pos, trigger "(" / ",")
                        v
  +---------------------------------------------------------------------+
  | server.Server.SignatureHelp                                          |
  |  .xs:  xs.XsParse(text) ------------------------------------+        |
  |        include.Resolver.Closure(uri).ExternalDecls(uri) --+  |        |
  |                                                           v  v        |
  |  .rms: rms.Parse(text) --+   XsBlock hit: unshift pos,   hints.Computer|
  |        |                 |   xs.XsParse(block.Code) ---->  .XsAt(file, |
  |        +-- no block -----+----------------------------------> pos,     |
  |                                                          ext) []xs.Decl|
  |                                     else: hints.Computer.RmsAt(file,pos)|
  |  found=false -> nil, nil (silence)                                     |
  |  Hint -> protocol.SignatureHelp (1 sig, ActiveSignature=0,             |
  |           ActiveParameter=*uint32(active) or nil)                      |
  +---------------------------------------------------------------------+
             |                    |                      |
             v                    v                      v
     xs.XsFile.CallAt     rms.RmsFile.ArgAt      kb.Store (lookups)
     -> xs.CallSite       -> rms.ArgSite          .Function / .Command
             |                    |               (CommandArg.Range/Kind
             v                    v                mined at load/extract)
     [parser-recorded       [parser-recorded
      call index +            attr name tokens +
      non-code spans]         string/comment spans]
```

### Data Flows

1. **XS document hint**: editor request → `Server.SignatureHelp` →
   `openDocument` (text) → `xs.XsParse` → `XsFile.CallAt(pos)` →
   `CallSite{callee, argIndex, onArg}` → truth model over
   `file.Decls ++ closure.ExternalDecls(uri)` (Kind=function, name match,
   conflict rule) → fallback `kb.Store.Function(callee)` → label/params
   rendering → `Hint` → `protocol.SignatureHelp` → client.
2. **Inline XS block hint (inside .rms)**: same entry → `rms.Parse` →
   innermost `XsBlock` with `Range.Contains(pos)` → `unshiftPos(pos,
   block.Range.Start)` → `xs.XsParse(block.Code)` →
   `Closure(uri).ExternalDecls("")` (inline blocks are not closure
   members) → `Computer.XsAt(xsFile, blockPos, external)` → `Hint` →
   protocol (document coordinates are implicit: the client position is
   unchanged; only the lookup coordinates are block-local).
3. **RMS command hint**: same entry → `rms.Parse` → `RmsFile.ArgAt(pos)`
   → `ArgSite{stmt, kind, index, name}` → `kb.Store.Command(stmt.Name)`
   → full kb-ordered rendering (Args then Attributes, mined kind/range in
   labels, optionals bracketed) → active mapping (arg → index; attr →
   len(Args)+attribute position; none → −1) → `Hint` → protocol.
4. **Mining at load (kb)**: `NewStore` → `indexCommands` → per
   `CommandArg` with empty `Range` or empty `Kind` → `MineKindRange(Desc)`
   → fill empty fields only → immutable store.
5. **Mining at extraction (kb)**: `cmd/kbgen` → `ExtractRmsCommands` →
   per built `CommandArg` → same fill-when-empty helper → marshaled JSON
   (wire gains `range`).

### Entity Dependencies

Initialization order (leaves → root, matching the architecture plan):

1. `kb` (no imports) — `NewStore()` (+ `MineKindRange` used by extraction).
2. `xs`, `rms` (import `common` only).
3. `hints.NewComputer(store)` — needs kb + xs + rms types.
4. `server.NewServer(store, analyzer, computer)`; `server.Serve` wires
   `NewStore → NewAnalyzer → NewComputer → NewServer → DocStore/Resolver`.

Runtime dependency direction: `server → hints → {kb, xs, rms} → common`;
no cycles (verified by `goga schema`).

## Code Stack Trace

### Trace: `kb.MineKindRange(desc)`

#### Chain

1. **Input**: caller passes the raw `CommandArg.Desc` prose string
   (e.g. `"number (0-99) (default: 12)"`).
2. **Step**: single regexp match over `desc`:
   `([A-Za-z_][A-Za-z0-9_]*)?\s*\((-?\d+(?:\.\d+)?)-(-?\d+(?:\.\d+)?)\)`
   — first (leftmost) occurrence only → checkpoint: numeric-bounds shape
   only; `"(default: 5)"`, `"(see: …)"` cannot match because the
   parenthesized content must be exactly `<num>-<num>`; decimals and an
   optional leading minus on either bound are accepted. The optional
   leading word group is the mined kind («number» in the corpus).
   Verified against the live corpus: 8 bounded entries, all mine
   kind `"number"`, ranges `0..2`, `36..480`, `0..53`, `0..7` (×2),
   `1..16`, `0..100`, `0..99`.
3. **Step**: no match → return `("", ValueRange{})` → checkpoint:
   unparseable prose yields an empty result, never an error (contract
   Constraint).
4. **Step**: match → return `(kind, ValueRange{Min, Max})` with groups as
   written (no numeric conversion — precision-preserving strings, contract
   Requirement) → checkpoint: deterministic pure function (same input →
   same output).
5. **Output**: `(kind string, r ValueRange)`; consumed by the load-time
   fill in `indexCommands` and by `buildCommand` in extraction.

#### Checkpoint Summary

- Bounds-shape correctness: passed (corpus-verified, non-numeric
  parentheticals rejected by construction).
- Determinism / no-error contracts: passed.

### Trace: `kb.NewStore` (load-time mining path)

#### Chain

1. **Input**: embedded `kb/data/rms-commands.json` bytes.
2. **Step**: `loadCommands → indexCommands(raw)` → `decodeData`
   (DisallowUnknownFields) into `[]commandWire` → checkpoint: wire type
   gains `Range ValueRange json:"range"` so regenerated JSON decodes; the
   committed JSON (no `range` key) decodes to the zero `ValueRange` —
   missing keys are not unknown keys.
3. **Step**: per wire record → `Command{Name, Section, Args, Attributes,
   …}`; per arg/attribute conversion `CommandArg(wa)` still compiles
   because both structs now share the identical field set (incl. `Range`)
   → checkpoint: type-flow OK.
4. **Step**: apply the shared fill-when-empty helper (same helper as
   extraction) to every arg and attribute: if `Range` is the zero
   `ValueRange` **or** `Kind == ""` → `kind, r := MineKindRange(Desc)`;
   fill `Range` only when empty, fill `Kind` only when empty →
   checkpoints: (a) structured values from extraction are never
   overwritten (provenance rule); (b) idempotent with extraction because
   both call sites use the identical helper semantics; (c) the 34
   empty-Kind entries in the current corpus all have empty `Desc` → no
   Kind fills (SC8 — `analysis` percent-gating and rendered signatures
   unchanged).
5. **Step**: index into `commandByName`, `commandsBySect`, `attrByKey`
   **after** mining so lookups serve mined data → checkpoint:
   `Store.Command`/`Store.Attribute` return mined `Range`/`Kind`.
6. **Output**: immutable `*Store`; load errors (unknown keys, duplicate or
   empty names) unchanged — mining itself never errors.

#### Checkpoint Summary

- Interface ↔ Type (`Range -> ValueRange` on the served `CommandArg`):
  passed.
- Provenance/idempotency: passed (single shared helper).
- SC8 (no behavioral drift for existing consumers): passed.

### Trace: `kb.ExtractRmsCommands(path)` (mining step)

#### Chain

1. **Input**: path to `docs/ref/zetnus-rms-guide.txt` (build-time tool
   `cmd/kbgen`; never called at runtime).
2. **Steps 1–2** (unchanged): read file; parse Syntax Skeleton sections.
3. **Step 3** (changed): `buildCommand` assembles `CommandArg{Name, Kind:
   kindOf(token), Required, Desc}` per positional arg and attribute, then
   applies the same fill-when-empty mining helper → checkpoint: freshly
   built args have an empty `Range`, so mining fills `Range` whenever the
   Desc has bounds; `Kind` keeps the structured skeleton kind
   (`percent` for `%`), falling back to the mined word only when the
   skeleton kind is empty → `cliff_curliness %` keeps `Kind="percent"`
   while gaining `Range 0..100`.
4. **Steps 4–5** (unchanged): changelog enrichment; duplicate check.
5. **Output**: `[]Command` with mined `Range`/`Kind`; `cmd/kbgen`
   marshals via its own `commandJSON/argJSON` mirror, which **must gain
   the `range` field** (`kb.ValueRange` with `json:"range"` tags) or the
   regenerated JSON silently drops the mined bounds. `cmd/` lives outside
   the cell system — covered under Additional Instructions.

#### Checkpoint Summary

- Extraction ↔ load idempotency: passed (shared helper; the JSON written
  by kbgen re-mines to itself on load).
- Wire schema symmetry (`argWire`/`argJSON`): design-mandated, checkpoint
  for the implementer.

### Trace: `xs.XsParse` → `XsFile.CallAt(pos)`

#### Chain (parser side — index recording)

1. **Input**: XS source (standalone `.xs` or inline `rms.XsBlock.Code`).
2. **Step**: the scanner already skips `//` and `/* */` comments and
   tokenizes strings; extend the scan loop to append every comment span
   (`skipLineComment`/`skipBlockComment` extents) and every `xString`
   token range to a `noncode []common.Range` index → checkpoint: positions
   inside strings/comments become testable without changing the token
   stream consumed by the parser.
3. **Step**: in `parsePostfix`'s `"("` case, open a `callRec{callee:
   identName(operand), calleeAt: operand.Range.Start, lparen: "("-token
   Start}` and let `parseArgs` record into it: every top-level `,`
   consumed **by this invocation** appends the comma token end position
   (nested calls record into their own records via recursion — their
   commas are not this call's); termination sets `argEnd` = `)`-token
   `End` (closed call) or the `eofPos` sentinel (unterminated list — the
   recovery frontier; `parseArgs` exits only on `)` or EOF, junk tokens
   are skipped in place; `eofPos` from `ast.go` compares after every
   real position, so a cursor at the exact end of input — the canonical
   SC1 state, and the norm for inline blocks whose `Code` has no
   trailing newline — is contained by the span `[calleeAt, eofPos)`) →
   checkpoints: (a) the recorded span
   `[calleeAt, argEnd)` is **not** bounded by `Expr.Range` (which stops
   at the last parsed argument) — contract Requirement; (b) in-progress
   calls (`f(`, `f(a,`) produce records — SC1; (c) vector literals
   (`parseParenExpr`), declaration parameter lists (`parseParams`) and
   grouping parens never create records — only Expr Kind=call does.
4. **Step**: `XsParse` attaches `calls` and `noncode` to the returned
   `XsFile` (unexported fields, same pattern as the existing `symbols`
   index) → checkpoint: value-copy safety — `XsFile` is returned by
   value and already carries unexported slice fields; append-only
   construction, no mutation after return.

#### Chain (query side — CallAt)

1. **Input**: `pos` in file coordinates (block-local for inline blocks —
   translation is the caller's job).
2. **Step**: `pos` inside any `noncode` span → `found=false` (contract
   step 1) → checkpoint: strings/comments never resolve to a call.
3. **Step**: select the record with the **latest `lparen`** among records
   whose span `[calleeAt, argEnd)` contains `pos` → checkpoints:
   innermost-wins (`f(g(x|` → `g`), postfix chains resolve to the suffix
   call, enclosing call wins when the cursor precedes an inner callee.
4. **Step**: none contains `pos` → `found=false` (contract step 3).
5. **Step**: `pos` on/before the `(` start (`!pos.After(lparen)`) →
   `CallSite{callee, ArgIndex: 0, OnArg: false}` (on the callee name or
   between name and `(`) → checkpoint: contract step 4.
6. **Step**: otherwise count this record's commas whose stored end
   position is `<= pos` → `argIndex = k`, `OnArg = true` → checkpoints:
   cursor right after `(` → k=0 (SC1 key moment); after the k-th
   top-level comma → k; a cursor **on** a comma character still reports
   the previous argument (the comma's end is `pos+1`); commas inside
   nested calls/parens/strings belong to their own records/spans and are
   never counted — contract step 5; no clamping — argIndex may exceed any
   declared parameter count (contract Constraint).
7. **Output**: `(CallSite, bool)`.

#### Checkpoint Summary

- Span/frontier semantics: passed (record-based, not `Expr.Range`-based).
- Innermost selection determinism: passed (distinct `(` positions).
- Never-clamp: passed at navigation level; consumer policy is separate.

### Trace: `rms.Parse` → `RmsFile.ArgAt(pos)`

#### Chain (parser side — index recording)

1. **Input**: RMS source text.
2. **Step**: `blankComments` already walks every character and knows the
   exact `//` and `/* */` extents — extend it to emit those extents as
   `common.Range` spans (multi-line block comments span across the
   recorded lines; `p.starts` supplies offsets) → `RmsFile.comments`.
3. **Step**: `statementLine` already holds the full token list of each
   statement/structural line (`rest`); record every `tokString` token
   range into `RmsFile.strings` → checkpoint: string values (attribute
   values) are testable; quoted include-path strings are covered by the
   excluded-band index below (the whole directive line is excluded, so
   they need no separate string entry).
4. **Step**: record an `excluded []common.Range` index in `run()` for
   the line classes that never own a statement — section-header lines
   (`<NAME>` / `</NAME>`), `#include`/`#includeXS` directive lines
   (including their path arguments, quoted or bare), and plain
   `#`-comment lines (`rms_grammar` comments; `#const`/`#define`/
   `#include_drs` are NOT excluded — they materialize statements and
   are handled by the `#`-prefix owner filter) → checkpoint: a cursor
   on these lines answers `found=false` even when a preceding command
   exists (the band owner would otherwise be that command — a wrong
   hint); the spans follow the existing `words` index pattern.
5. **Step**: `buildAttribute` records the attribute name token range into
   an unexported `nameAt common.Range` field on `Attribute` (set at
   construction, preserved by the value-copying `materialize`) →
   checkpoint: precise name-or-value discrimination for kind=attr without
   relying on word-index heuristics; flag attributes have a zero `Value`
   whose empty range never contains any position.

#### Chain (query side — ArgAt)

1. **Input**: `pos` in file coordinates.
2. **Step**: `pos` inside a recorded string or comment span, or inside
   an `excluded` span (section-header line, `#include`/`#includeXS`
   directive line incl. its path argument, plain `#`-comment line) →
   `found=false` (contract steps 1–2) → checkpoint: these positions
   never fall through to the band owner, so a preceding command's hint
   cannot leak onto a header/include/comment cursor.
3. **Step**: owner lookup `ownerAt(pos)` — band semantics: walk sections
   top-down; at each statement list pick the **last** statement whose
   `Range.Start <= pos` (positional order) and recurse into its children;
   the statement reached when no child qualifies is the owner; no
   candidate at the top level → `found=false` → checkpoints: (a)
   containing positions behave exactly like `StatementAt`'s deepest-match
   (positional containment implies the band picks the same statement);
   (b) same-line trailing positions (cursor past the last argument —
   missed by `StatementAt`'s strict `End.Line < pos.Line` fallback) are
   owned by their own statement, not an earlier one; (c) positions in
   gaps/blank lines fall to the preceding statement («предшествующий»
   semantics); (d) unclosed blocks own their trailing region (children
   simply run out); (e) section headers, `#include`/`#includeXS`
   directive lines and plain `#`-comment lines are excluded spans →
   `found=false` **even when a preceding command exists** (the band
   alone would pick that command as owner — the excluded index is what
   honors the contract's «директивы #include, заголовки секций →
   found=false» in the general case).
4. **Step**: owner name starts with `#` (directive-styled statement:
   `#const`, `#define`, `#include_drs`) → `found=false` (see Applied
   Fixes §4).
5. **Step**: `pos` inside `Stmt.Args[i].Range` → `kind=arg, index=i`
   (top-level argument spans are disjoint — the expression splitter
   breaks on top-level commas).
6. **Step**: `pos` inside an attribute's `nameAt` or its
   `Value.Range` → `kind=attr, name=Attribute.Name` (name-match — the
   attribute's only truthful identity; works for flag attributes via
   `nameAt`).
7. **Step**: otherwise (command name token, whitespace between tokens,
   `{`/`}`, command tail, ambiguous one-line block splits) →
   `kind=none` with the owner statement — never a guessed label (SC6).
8. **Output**: `(ArgSite{Stmt, Kind, Index, Name}, bool)`; `Kind` uses
   exported constants `KindArg = "arg"`, `KindAttr = "attr"`,
   `KindNone = "none"` (the manifest's `Kind*` Go-constant convention).

#### Checkpoint Summary

- Owner semantics incl. trailing/unclosed: passed (band model).
- String/comment exclusion: passed (recorded spans).
- none-on-ambiguity: passed (exact token-range membership only).

### Trace: `hints.NewComputer(store)` + `Computer.XsAt(file, pos, external)`

#### Chain

1. **Input**: `NewComputer(store *kb.Store)` (constructor DI per
   `conventions`); `XsAt(file xs.XsFile, pos common.Pos, external
   []xs.Decl)`.
2. **Step**: `file.CallAt(pos)` → `CallSite`; `found=false` → silence
   (`Hint{}, false`) → checkpoint: type flow `xs.CallSite` consumed
   field-wise (`Callee`, `ArgIndex`, `OnArg`).
3. **Step**: truth-model pool — collect `Kind == "function"`
   declarations named `Callee` from `file.Decls` (source order) then from
   `external` (closure DFS order) → checkpoint: document and closure form
   one pool; the analyzed file's own decls come first (pool order is the
   deterministic tie-break, Applied Fixes §1).
4. **Step**: pool non-empty → conflict check: every other pool member
   must have an equal parameter sequence (length + pairwise
   `(Type, Name)` equality); any mismatch → silence → checkpoint:
   contract «несколько с разными параметрами → молчание»; equal
   duplicates collapse onto the pool-first declaration («единая
   декларация»).
5. **Step**: pool empty → `store.Function(Callee)`; `found=false` →
   silence → checkpoint: kb lookup API matches `lookups` usage.
6. **Step**: render — `label = "<ret> <name>(<labels>)"` with `ret`
   omitted when empty (source: `Decl.Type`; kb: `Function.ReturnType`);
   per-parameter label `"<Type> <Name>"`, falling back to `"<Name>"` when
   the type is unset (permissive source decls); kb optional params
   (`Required=false`) wrapped in `[…]`; source params are all required
   (no brackets — `xs.Param` carries no `Required`) → checkpoints:
   examples from the contract reproduce exactly:
   `vector xsVectorSet(float x, float y, float z)` (live kb data),
   `bool xsCreateFile([bool append])`; types/names are never invented.
7. **Step**: `active`: `OnArg=false → -1`; else `ArgIndex < len(params) →
   ArgIndex`; else `-1` → checkpoint: never clamped (SC2 policy).
8. **Output**: `(Hint{Label, Params, Active}, true)`; `Hint` carries no
   documentation (concise-hints rule — extended descriptions stay in
   hover).

#### Checkpoint Summary

- Interface ↔ Interface (server passes `[]xs.Decl` from
  `Closure.ExternalDecls`) : passed — same concrete type.
- Truth model + conflict rule determinism: passed.
- Rendering parity with contract examples: passed (verified against live
  kb JSON).

### Trace: `hints.Computer.RmsAt(file, pos)`

#### Chain

1. **Input**: `RmsAt(file rms.RmsFile, pos common.Pos)`.
2. **Step**: `file.ArgAt(pos)` → `ArgSite`; `found=false` → silence.
3. **Step**: `store.Command(site.Stmt.Name)`; not found → silence
   (unknown command, block keywords like `start_random`, filtered
   directives) → checkpoint: `Statement.Name` is the lookup key;
   name-match only.
4. **Step**: render the **full** kb-ordered list — `Args` first, then
   `Attributes`, no truncation: per `CommandArg` label = `Name` when
   `Kind == ""` (flag attributes, name-only), else `"Name: Kind"` plus
   `" Min..Max"` when `Range` is non-empty (mined bounds, string
   concatenation with `".."`); `Required=false` wraps the whole label in
   `[…]`; `label = "<name>(<labels joined by ", ">)"` → checkpoints:
   contract examples reproduce from live data:
   `percent_chance(%: percent 0..99)` (arg `%` kind `percent`, mined
   `0..99`); `create_elevation([MaxHeight: number 1..16], …,
   [set_scale_by_size], …)`; kinds/defaults never invented.
5. **Step**: `active`: `kind=arg → Index` (pass-through, no clamp);
   `kind=attr → len(Args) + index of Name in Command.Attributes` (absent
   → −1); `kind=none → −1` → checkpoint: SC6/never-clamp.
6. **Output**: `(Hint, true)`.

#### Checkpoint Summary

- Mined `ValueRange` consumption: passed (kb side provides; hints render).
- Active mapping for attrs: passed (`len(Args)` offset puts attribute
  labels at their `Params` positions).

### Trace: `server.Server.SignatureHelp(ctx, params)`

#### Chain

1. **Input**: `*protocol.SignatureHelpParams` (URI, position, trigger
   context).
2. **Step**: `openDocument(uri)` → text; not open → `nil, nil` →
   checkpoint: same silence convention as `Hover`.
3. **Step**: language by URI suffix (shared template with Hover/Completion):
   `.xs` → `xs.XsParse(text)` + `s.resolver.Closure(ctx, uriArg)` +
   `computer.XsAt(file, pos, closure.ExternalDecls(uriArg))` →
   checkpoints: the analyzed file is excluded from externals (same
   pattern as `analyzeXs`); C6 — a partially unavailable closure simply
   yields fewer externals; unknown names fall through to kb, then to
   silence.
4. **Step**: `.rms` → `rms.Parse(text)`; find the innermost `XsBlock`
   with `Range.Contains(pos)` → hit: `xs.XsParse(block.Code,
   "inline:"+name)`, translate `pos` block-ward with `unshiftPos(pos,
   block.Range.Start)` (mirror of the existing `shiftPos`: subtract base
   line/offset; subtract base column only on the first block line — same
   math as `include.resolver`'s helper), `computer.XsAt(xsFile, blockPos,
   closure.ExternalDecls(""))` (inline blocks are not closure members —
   same argument as `analyzeRms`) → no block:
   `computer.RmsAt(file, pos)` → checkpoints: document-coordinate answer
   needs no re-shifting (the client position is echoed, only lookup
   coordinates are local); the inline path reuses the established
   Definition/include template.
5. **Step**: `found=false` → `nil, nil` (silence — never a guessed hint).
6. **Step**: `Hint → protocol.SignatureHelp` per `lsp-protocol`
   (Signature Help section): exactly one `SignatureInformation` with
   `Label = hint.Label`; `Parameters` = one `ParameterInformation` per
   `hint.Params` entry with a plain-string label; `Documentation`
   omitted; `ActiveSignature = &[]uint32{0}[0]`; `ActiveParameter` =
   `*uint32(active)` when `active >= 0`, else `nil` → checkpoints:
   single-signature rule; never-clamp rule (`ActiveParameter` nil, not
   last-parameter); statelessness — the answer depends only on
   (document, position); `params.Context` (TriggerKind/
   TriggerCharacter/IsRetrigger) is ignored.
7. **Output**: `(*protocol.SignatureHelp, error)` — `nil, nil` on silence,
   `nil` error otherwise (read-only handler, R10).

#### Checkpoint Summary

- Protocol mapping ↔ `lsp-protocol` usage: passed.
- Inline-block coordinate template: passed (mirrors shift/unshift pair).
- Silence convention: passed.

### Trace: `server.Server.Initialize` (capability delta)

1. **Input**: `*protocol.InitializeParams`.
2. **Step**: existing capability set plus
   `SignatureHelpProvider: &protocol.SignatureHelpOptions{
   TriggerCharacters: []string{"(", ","}}` → checkpoint: trigger
   characters `(` and `,` only — they do not intersect the completion
   triggers ` ` and `<`; RMS value keystrokes must not spam the widget.
3. **Output**: unchanged negotiation (utf-8 when offered), `ServerInfo`
   unchanged.

### Trace: `server.Serve` (wiring delta)

1. **Input**: process context + stdio stream.
2. **Step**: dependency assembly grows one step after `NewAnalyzer`:
   `computer := hints.NewComputer(store)` → `NewServer(store, analyzer,
   computer)` → checkpoint: constructor DI per `conventions`; `DocStore`
   and `Resolver` remain internal to `NewServer`; the call sites of
   `NewServer` (`serve.go`, the test helper in `navigation_test.go`)
   are updated in the same change.

## Algorithm Design

### `kb.MineKindRange` (mine.go)

**Responsibility**: parse Desc prose into a structured kind word and
value bounds; pure, deterministic, never errors.

```
1. compile once: mineBoundsRe =
   ([A-Za-z_][A-Za-z0-9_]*)? \s* \( (-?\d+(\.\d+)?) - (-?\d+(\.\d+)?) \)
2. m := first submatch in desc
3. IF m == nil:
   - return "", ValueRange{}            # unparseable → empty, not an error
4. return m[1], ValueRange{Min: m[2], Max: m[3]}   # bounds as written
```

**Errors**: none — the empty result is the error channel.

**Edge cases**: decimals (`0.5-1.5`) and negative bounds (`-5-10`) parse;
`(default: 5)`, `(see: …)` never match (content must be exactly
`num-num`); bounds without a preceding word → `kind=""`; first bounds
fragment wins when several exist.

### `kb` fill-when-empty helper (shared by store + extract)

```
1. IF arg.Range == zero OR arg.Kind == "":
2.   kind, r := MineKindRange(arg.Desc)
3.   IF arg.Range == zero: arg.Range = r
4.   IF arg.Kind == "":    arg.Kind = kind
```

Applied to every `CommandArg` of every `Command` — in `indexCommands`
(before map indexing) and in `buildCommand` (after assembly).

### `xs.XsFile.CallAt` (ast.go)

**Responsibility**: innermost in-progress-or-closed call enclosing the
cursor, with argument ordinal; navigation only, no rendering policy.

```
1. IF pos inside any noncode span (string/comment): return false
2. best := index of the callRec with the latest lparen among records
   whose [calleeAt, argEnd) contains pos
3. IF none: return false
4. IF NOT pos.After(best.lparen):                     # on callee name / before "("
   - return CallSite{best.callee, 0, false}
5. k := count of best.commas with stored end <= pos
6. return CallSite{best.callee, k, true}              # never clamped
```

Parser-side recording: `callRec{callee, calleeAt, lparen, argEnd,
commas}` per Expr Kind=call; `argEnd` = `)`-token end (closed call) or
the `eofPos` sentinel (`ast.go` — compares after every real position)
for the unterminated recovery frontier, so the half-open `[calleeAt,
argEnd)` contains a cursor at the exact end of input; `noncode` =
string-token + comment spans.

**Errors**: none (found=false is the only failure).

**Edge cases**: unclosed calls span to the `eofPos` sentinel (SC1 — a
cursor at the exact end of input, including inline-block last lines, is
contained); nested inner wins; postfix chains report the suffix call
(callee may be `""` — the computer silences); vector literals / param
lists / grouping parens are not calls; cursor on a comma character
counts the previous argument.

### `rms.RmsFile.ArgAt` (ast.go)

**Responsibility**: owning command + argument/attribute discrimination
under the cursor.

```
1. IF pos inside a recorded string/comment span OR an excluded span
   (section-header line, #include/#includeXS line, #-comment line):
   return false
2. stmt := ownerAt(pos)          # band model, see below; none → false
3. IF stmt.Name starts with "#": return false        # directive-styled
4. FOR i, arg IN stmt.Args:
   - IF arg.Range contains pos: return ArgSite{stmt, KindArg, i, ""}
5. FOR a IN stmt.Attributes:
   - IF a.nameAt contains pos OR a.Value.Range contains pos:
       return ArgSite{stmt, KindAttr, 0, a.Name}
6. return ArgSite{stmt, KindNone, 0, ""}              # name token, gaps,
                                                      # braces, tail, ambiguity
```

`ownerAt(pos)`: from each section's statements (and the synthetic
`global` section) pick the last statement with `Range.Start <= pos`,
recurse into its children, and return the statement where no child
qualifies; no candidate → not found.

Parser-side recording: `Attribute.nameAt` (name token range);
`RmsFile.strings` (statement-line string tokens); `RmsFile.comments`
(blankComments extents); `RmsFile.excluded` (section-header lines,
`#include`/`#includeXS` directive lines incl. path arguments, plain
`#`-comment lines — recorded in `run()`, `words` index pattern).

**Errors**: none (found=false / kind=none are the only failures).

**Edge cases**: same-line trailing cursor owned by its own statement;
blank-line cursor owned by the preceding statement; `{`/`}` and inter-token
spaces → kind=none; nested random/conditional blocks — innermost owns;
flag attributes match on name only; section-header, include-directive
and `#`-comment line cursors → found=false even after a command.

### `hints.Computer.XsAt` (computer.go)

**Responsibility**: XS hint at a position — truth model, conflict rule,
label rendering.

```
1. cs, ok := file.CallAt(pos); IF NOT ok: silence
2. pool := [d FOR d IN file.Decls      IF d.Kind==function AND d.Name==cs.Callee]
        ++ [d FOR d IN external        IF d.Kind==function AND d.Name==cs.Callee]
3. IF pool non-empty:
   - FOR each other IN pool[1:]:
       IF NOT paramsEqual(other.Params, pool[0].Params): silence      # conflict
   - render from pool[0]                        # single merged declaration
   ELSE:
   - fn, ok := store.Function(cs.Callee); IF NOT ok: silence
   - render from fn
4. labels:
   - kb:      per p IN fn.Params:  s = (p.Type != "" ? p.Type+" "+p.Name : p.Name)
              IF NOT p.Required: s = "["+s+"]"
   - source:  per p IN decl.Params: s = (p.Type != "" ? p.Type+" "+p.Name : p.Name)
   - label = (ret != "" ? ret+" " : "") + name + "(" + join(labels, ", ") + ")"
5. active := NOT cs.OnArg ? -1 : (cs.ArgIndex < len(params) ? cs.ArgIndex : -1)
6. return Hint{label, labels, active}
```

`paramsEqual`: same length and pairwise `(Type, Name)` equality.

**Errors**: none — silence is the failure mode.

**Edge cases**: equal-parameter duplicates merge (pool-first renders);
callee `""` (postfix chain) never matches → silence; argIndex beyond the
parameter list → active −1 (never clamp).

### `hints.Computer.RmsAt` (computer.go)

**Responsibility**: RMS hint — kb-ordered full list with active mapping.

```
1. site, ok := file.ArgAt(pos); IF NOT ok: silence
2. cmd, ok := store.Command(site.Stmt.Name); IF NOT ok: silence
3. labels := []
   FOR a IN cmd.Args ++ cmd.Attributes:
   - s := a.Name
   - IF a.Kind != "": s = a.Name + ": " + a.Kind
     IF a.Range non-empty: s = s + " " + a.Range.Min + ".." + a.Range.Max
   - IF NOT a.Required: s = "[" + s + "]"
   - append labels, s                                # full list, no truncation
4. label := cmd.Name + "(" + join(labels, ", ") + ")"
5. active :=
     kind==KindArg  → site.Index                       # pass-through
     kind==KindAttr → len(cmd.Args) + indexOf(cmd.Attributes, site.Name) or -1
     otherwise      → -1
6. return Hint{label, labels, active}
```

**Errors**: none — silence on unknown owner.

**Edge cases**: flag attributes name-only; attr not in the kb list → −1;
index beyond the rendered list passes through (client renders no active
mark).

### `server.Server.SignatureHelp` (server.go)

**Responsibility**: protocol surface — language routing, coordinate
translation, protocol mapping, silence.

```
1. text := openDocument(uri); IF not open: return nil, nil
2. IF ".xs":
   - file := xs.XsParse(text, name)
   - closure := resolver.Closure(ctx, uri)
   - hint := computer.XsAt(file, pos, closure.ExternalDecls(uri))
3. IF ".rms":
   - file := rms.Parse(text, name)
   - IF innermost block IN file.XsBlocks with block.Range.Contains(pos):
       xsFile := xs.XsParse(block.Code, "inline:"+name)
       closure := resolver.Closure(ctx, uri)
       hint := computer.XsAt(xsFile, unshiftPos(pos, block.Range.Start),
                             closure.ExternalDecls(""))
   - ELSE: hint := computer.RmsAt(file, pos)
4. IF NOT found: return nil, nil
5. return toSignatureHelp(hint):                      # per lsp-protocol
   - Signatures: [ {Label: hint.Label,
                    Parameters: [{Label: p} FOR p IN hint.Params]} ]   # exactly one
   - ActiveSignature: 0 (pointer)
   - ActiveParameter: *uint32(active) IF active >= 0 ELSE nil          # never clamp
   - Documentation omitted
```

`unshiftPos(p, base)`: `Line −= base.Line`, `Offset −= base.Offset`,
`Column −= base.Column` only when `p.Line == base.Line` (inverse of the
existing `shiftPos`).

**Errors**: none returned — handler is read-only; unparseable input is
silence by construction (parsers never fail hard).

**Edge cases**: trigger context ignored (stateless, R-equivalent answers
for manual and auto requests); non-.rms/.xs URIs fall through both switch
arms → silence.

### `server.Server.Initialize` + `server.Serve` (deltas)

- Capability: add `SignatureHelpProvider` with triggers `"("`, `","`.
- Wiring: `hints.NewComputer(store)` between `NewAnalyzer` and
  `NewServer`; `NewServer(store, analyzer, computer)` gains the third
  parameter (update `serve.go` and the test helper).

## Cross-cutting Concerns

- **Error handling**: signature help has **no error path** — every
  failure is silence (`nil, nil`), mirroring the Hover convention and
  the trust rule («silence, never a guessed hint»). `MineKindRange`
  returns empty values instead of errors. Parsers keep their
  diagnostics-only recovery (unchanged).
- **Logging**: none added. The handler is read-only and silent by
  design; parse/diagnostic logging behavior is untouched (SC8).
- **Validation**: contract-level only — no new input validation. The
  server trusts the document store; `Computer` trusts its arguments.
  `MineKindRange` validates prose implicitly (no match → empty).
- **Caching**: none. Each request reparses the current document text
  (same pattern as `Hover`/`Completion`); answers are valid for the
  parsed snapshot only (`xs-parsing`/`rms-parsing` preconditions). The
  `Computer` holds only the immutable `*kb.Store`.
- **Concurrency**: `Computer` is stateless (constructor-held immutable
  store) — safe for concurrent handler use, matching the existing
  server's per-request reparsing model. No goroutines, channels, or
  shared mutable state are introduced.
- **Performance**: request cost = one parse + one index lookup + one
  linear scan of the call/trivia indices (already the navigation cost
  model of `SymbolAt`/`References`); no hot-path concern.

## Usages Analysis

### `conventions` (all five cells)

- **What it provides**: Go coding/testing rules — DI, error wrapping,
  table-driven tests, naming (`Test<Component>_<Scenario>`), blank-line
  formatting, doc comments.
- **Where used**: every new file (`mine.go`, `computer.go`, `hint.go`,
  handler, tests); `NewComputer` constructor DI; `fmt.Errorf` wrapping in
  unchanged error paths.
- **Why chosen**: project-wide mandatory baseline.
- **How exactly**: constructor injection for `Computer`; `require`/
  `assert` table tests mirroring `*_test.go` next to sources.

### `kbdata` (kb, inline)

- **What it provides**: embedded JSON schemas + the Desc-prose mining
  patterns («(<num>-<num>)» after the kind word; percent args keep
  structured kind; flag attributes name-only).
- **Where used**: `MineKindRange` (pattern source), `argWire`/`argJSON`
  (schema), `indexCommands` (fill rule).
- **Why chosen**: single authoritative data-shape reference.
- **How exactly**: regexp mirrors the documented bounds shape; wire tags
  `range/min/max`.

### `xs_grammar` (xs, file `.goga/usages/xs-grammar.md`)

- **What it provides**: XS lexical/structural rules — vector literals
  `(1.0, 2.0, 3.0)`, call syntax `callee(args)`.
- **Where used**: `CallAt` call-context discrimination (vector literals
  and grouping parens are not calls).
- **Why chosen**: the global annotation binds call-context creation to
  the grammar's expression forms.
- **How exactly**: only the `parsePostfix` `"("` branch records call
  records.

### `rms_grammar` (rms, file `.goga/usages/rms-grammar.md`)

- **What it provides**: RMS line-oriented structure, directives,
  positional attribute semantics.
- **Where used**: `ArgAt` owner/discrimination rules (attrs attach to the
  preceding command; blocks nest).
- **Why chosen**: same binding role as `xs_grammar` for RMS.
- **How exactly**: band owner model over the existing statement tree;
  `Attribute.nameAt` recorded at parse time.

### `positions-and-diagnostics` (common, imported by xs/rms/hints)

- **What it provides**: `common.Pos`/`common.Range` construction and
  half-open containment semantics.
- **Where used**: every recorded span and every containment test in
  `CallAt`/`ArgAt`; `Pos` parameters in hints.
- **Why chosen**: shared positional vocabulary.
- **How exactly**: `Range.Contains(pos)`; immutable comparable
  positions.

### `lookups` (kb, imported by hints)

- **What it provides**: `NewStore`, exact-name lookups
  (`Function`, `Command`, `Attribute`), mined kind/range rendering
  contract for consumers.
- **Where used**: `Computer.XsAt` (`store.Function`), `Computer.RmsAt`
  (`store.Command`).
- **Why chosen**: single lookup point mandated by the kb cell.
- **How exactly**: `(value, found)` pairs; silence on `found=false`.

### `xs-parsing` (xs, imported by hints/server)

- **What it provides**: `XsParse`, `CallAt` consumer semantics (in-
  progress calls, innermost wins, ArgIndex may exceed params).
- **Where used**: `Computer.XsAt`; server `.xs` and inline-block paths.
- **Why chosen**: the parser cell's consumer documentation.
- **How exactly**: parse-then-query per request; block-local coordinate
  translation for inline blocks.

### `rms-parsing` (rms, imported by hints/server)

- **What it provides**: `Parse`, `ArgAt` consumer semantics (owner
  resolution, kind vocabulary, ambiguity → none).
- **Where used**: `Computer.RmsAt`; server `.rms` path.
- **Why chosen / how**: symmetric with `xs-parsing`.

### `computing` (hints, imported by server)

- **What it provides**: `NewComputer`, `XsAt`/`RmsAt` call patterns,
  silence mapping (`found=false → nil, nil`), protocol-mapping boundary
  (`Hint` → `protocol.SignatureInformation` belongs to server).
- **Where used**: `Server.SignatureHelp`, `Serve` wiring.
- **Why chosen**: the hints cell's consumer documentation.
- **How exactly**: one `Computer` per process, injected into `NewServer`.

### `lsp-protocol` (server, file `.goga/usages/cooks/lsp-protocol.md`)

- **What it provides**: go.lsp.dev construction patterns — capability
  advertisement (`SignatureHelpOptions{TriggerCharacters: []string{"(",
  ","}}`), `SignatureHelp` result shape (single signature,
  `ActiveSignature` 0, `ActiveParameter *uint32` nil-when-none, plain-
  string parameter labels, omitted documentation), statelessness rule.
- **Where used**: `Initialize`, `SignatureHelp`, `toSignatureHelp`.
- **Why chosen**: binding protocol reference for the server cell.
- **How exactly**: nullable result `nil, nil` for silence; union/optional
  helpers per the usage's conventions (same library idioms the file
  already uses for `CompletionItem`).

### `closure`, `checks`, `symbols` (imported by server — unchanged paths)

- Read for context; the signature-help paths consume `Closure.
  ExternalDecls` exactly as `analyzeRms`/`analyzeXs` do (`closure`).
  `checks` and `symbols` are untouched by this change (SC8).

## `.usages/` Update

### Cell: `kb`

- **`lookups.md`** → current — the «Mined kind/range on CommandArg»
  section matches the implemented `Range`/`Kind` fill semantics; no
  edits.
- **`data-pipeline.md`** → current — the «Mining on regeneration»
  section matches `ExtractRmsCommands` mining and the idempotency rule;
  no edits.

### Cell: `xs`

- **`xs-parsing.md`** → current — the «Call-site lookup» section matches
  `CallAt` semantics (in-progress calls, innermost wins, never clamp);
  no edits.

### Cell: `rms`

- **`rms-parsing.md`** → current — the «Argument lookup» section matches
  `ArgAt` semantics; no edits. (All code fences balanced — review
  corrected an earlier note that mis-attributed an unclosed fence to
  this file.)

### Cell: `hints`

- **`computing.md`** → current — construct/XsAt/RmsAt patterns and the
  silence/protocol-boundary preconditions match this design; no edits.

### Cell: `server`

- **`lifecycle.md`** → current — the advertised-capabilities paragraph
  includes signature help with triggers and the widget-refresh note; no
  edits. (Pre-existing unclosed code fence in the older «Entrypoint»
  section — outside this change's scope; noted by the design review.)

No new `.usages/` files: every change falls inside an existing functional
domain of an existing file (cookbook decision rule), and `hints`
already ships `computing.md` covering its single domain.

## Test Stack Trace

### General Setup

- Go table-driven tests with `testify` `require`/`assert`, mirroring
  source files (`mine.go → mine_test.go`, …); navigation-method tests
  extend the existing `xs/navigation_test.go` / `rms/navigation_test.go`.
- Positions are computed from fixtures via `strings.Index`/explicit
  `common.Pos{Line, Column}` (established style in `xs/navigation_test.go`).
- kb load-path tests use the existing `indexCommands`/`indexFunctions`
  seams with hand-crafted JSON payloads (style of
  `TestIndexCommands_InvalidPayload`).
- hints tests build a real `kb.NewStore()` (embedded data) plus
  hand-built `xs.XsFile`/`rms.RmsFile` from real parses.
- server tests use the existing `startHarness` stdio harness
  (`h.disp.Initialize → h.disp.DidOpen → h.waitDiagnostics →
  h.disp.SignatureHelp`).

### Source File Registry

- `kb/mine.go` (+ `kb/mine_test.go`) — new.
- `kb/model.go`, `kb/store.go`, `kb/extract.go` (+ existing test files
  extended) — modified.
- `xs/parse.go`, `xs/ast.go` (+ `xs/navigation_test.go` extended) —
  modified.
- `rms/parse.go`, `rms/ast.go` (+ `rms/navigation_test.go` extended) —
  modified.
- `hints/computer.go`, `hints/hint.go` (+ `hints/computer_test.go`) — new.
- `server/server.go`, `server/serve.go` (+ `server/serve_test.go`
  extended, `server/navigation_test.go` helper updated) — modified.
- `cmd/kbgen/main.go` — wire mirror gains `range` (no dedicated test;
  covered by the kb idempotency test).

---

### Positive Tests

#### `TestMineKindRange_BoundedNumber`

**Setup**: none (pure function).

**Input**: `desc = "number (0-99) (default: 12)"`.

**Trace**:
```
MineKindRange("number (0-99) (default: 12)")
  → mineBoundsRe.FindStringSubmatch   # first match at "number (0-99)"
    returns: ["number (0-99)", "number", "0", "99"]
  → return ("number", ValueRange{"0","99"})
```

**Assertions**:
```
kind == "number"; r.Min == "0"; r.Max == "99"
```

**Sufficiency**: the canonical corpus pattern (8 of 8 bounded entries);
pins the «only numeric bounds match» rule against the trailing
`(default: …)` fragment.

#### `TestMineKindRange_DecimalAndNegativeBounds`

**Setup**: none.

**Input**: `desc = "float (-1.5-2.5) range"`.

**Trace**:
```
MineKindRange("float (-1.5-2.5) range")
  → first match "float (-1.5-2.5)"
  → return ("float", ValueRange{"-1.5", "2.5"})
```

**Assertions**: `kind == "float"; r.Min == "-1.5"; r.Max == "2.5"`.

**Sufficiency**: decimals and optional minus are contract Requirements;
prevents a future refactor from dropping the sign or the fraction.

#### `TestStore_LoadMinesRangeAndEmptyKind`

**Setup**: `indexCommands` seam with one command: arg
`{"name":"N","kind":"","required":true,"desc":"number (0-53)"}` and **no**
`range` key (the committed-JSON shape).

**Input**: the JSON payload above.

**Trace**:
```
indexCommands(raw)
  → decode → CommandArg{Name:"N", Kind:"", Desc:"number (0-53)"}
  → fill-when-empty: Range empty OR Kind empty → MineKindRange(Desc)
    returns: ("number", {0,53})
  → Range = {0,53}; Kind = "number"
  → commandByName["set_gaia_civilization"] holds the mined arg
```

**Assertions**: loaded `Command.Args[0].Kind == "number"`;
`Range == ValueRange{"0","53"}`; `Store.Command(...).Args[0].Range.Min == "0"`.

**Sufficiency**: proves load-time mining works on the committed (unmined)
JSON — the shipping path for the existing data file.

#### `TestStore_LoadKeepsStructuredKindOverMined`

**Setup**: `indexCommands` seam: arg
`{"name":"%","kind":"percent","required":true,"desc":"number (0-100)"}`.

**Input**: the JSON payload above.

**Trace**:
```
indexCommands → CommandArg{Kind:"percent", Desc:"number (0-100)"}
  → fill-when-empty: Range empty → mine → Range={0,100}
  → Kind non-empty → NOT overwritten (stays "percent")
```

**Assertions**: `Kind == "percent"`; `Range == ValueRange{"0","100"}`.

**Sufficiency**: the provenance rule (structured wins); guards the
`cliff_curliness`/`percent_chance` percent set that `analysis` gates on
(SC8 — no percent arg may silently become «number»).

#### `TestExtractRmsCommands_MinesRange` (extension of `TestExtractRmsCommands_RealGuide`)

**Setup**: real `docs/ref/zetnus-rms-guide.txt` fixture (existing test
already reads it).

**Input**: `ExtractRmsCommands("docs/ref/zetnus-rms-guide.txt")`.

**Trace**:
```
ExtractRmsCommands → buildCommand(create_elevation)
  → arg MaxHeight {Kind:"number" (skeleton), Desc:"number (1-16) (default: …)"}
  → fill-when-empty → Range = {1,16}, Kind stays "number"
```

**Assertions**: `create_elevation.Args[0].Range == ValueRange{"1","16"}`;
`cliff_curliness` arg `%`: `Kind == "percent"`, `Range ==
ValueRange{"0","100"}`.

**Sufficiency**: the extraction pipeline (kbgen path) mines as the
data-pipeline usage documents; guards regeneration correctness.

#### `TestStore_MiningIdempotentWithExtraction`

**Setup**: extract from the real guide; marshal args to the wire shape;
decode through `indexCommands`.

**Input**: extraction output of the real guide.

**Trace**:
```
ExtractRmsCommands(guide) → []Command (mined at build)
  → marshal args/attrs to JSON (incl. range)
  → indexCommands(json)
  → fill-when-empty: Range already set → no re-mining effect
```

**Assertions**: for every command, every arg/attribute: loaded
`Kind == extracted Kind` and loaded `Range == extracted Range`.

**Sufficiency**: the data-pipeline idempotency rule («regenerated JSON
and load-time mining must agree»); prevents double-mining drift.

#### `TestCallAt_ClosedCallArgIndex`

**Setup**: `src := "void f() { g(a, b); }"`; `file, _ := XsParse(src, "t.xs")`.

**Input**: `pos` on `a` (index of "a") and `pos` on `b` (index of "b").

**Trace**:
```
XsParse(src) → callRec{callee:"g", commas:[end of ","]}
CallAt(posA) → posA inside [g, ")"-end) → onArg branch → commas ending <= posA: 0
CallAt(posB) → commas ending <= posB: 1
```

**Assertions**: `CallAt(posA)` → `{Callee:"g", ArgIndex:0, OnArg:true}`;
`CallAt(posB)` → `{Callee:"g", ArgIndex:1, OnArg:true}`.

**Sufficiency**: baseline comma counting on a closed call.

#### `TestCallAt_JustAfterOpenParen` (SC1)

**Setup**: `src := "void f() { g("`.

**Input**: `pos` = column right after `(` (index of "(" + 1), line 0.

**Trace**:
```
XsParse → callRec{callee:"g", argEnd: EOF} (unterminated list)
CallAt(pos) → span contains pos; pos.After(lparen) → true
  → commas: none → argIndex 0, onArg true
```

**Assertions**: `{Callee:"g", ArgIndex:0, OnArg:true}`, found.

**Sufficiency**: SC1 — the first character after «(» is the feature's
key moment; the exact state an editor sends after the trigger keystroke.

#### `TestCallAt_OnCalleeName`

**Setup**: `src := "void f() { g(a); }"`; pos on the `g` token.

**Input**: `pos` = index of "g(".

**Trace**:
```
CallAt(pos) → span starts at callee start, contains pos
  → NOT pos.After(lparen) → onArg=false, argIndex=0
```

**Assertions**: `{Callee:"g", ArgIndex:0, OnArg:false}`.

**Sufficiency**: contract step 4 — cursor on the callee renders the
signature with no active parameter (distinct from argument 0).

#### `TestCallAt_NestedInnerWins`

**Setup**: `src := "void f() { h(g(x)); }"`; pos on `x`.

**Input**: `pos` = index of "x".

**Trace**:
```
records: h (lparen L1) and g (lparen L2 > L1); both spans contain pos
  → latest lparen wins → g → commas of g before pos: 0
```

**Assertions**: `{Callee:"g", ArgIndex:0, OnArg:true}`.

**Sufficiency**: contract «f(g(x| → g, аргумент 0» — innermost-call
selection.

#### `TestArgAt_OnPositionalArg`

**Setup**: `src := "create_elevator PLAYER_1 5"`; `file, _ := rms.Parse(src, "t.rms")`.

**Input**: `pos` on `PLAYER_1` (line 0, its column).

**Trace**:
```
Parse → Statement{command, Args:[PLAYER_1, 5]}
ArgAt(pos) → ownerAt → the command; Args[0].Range contains pos
  → ArgSite{Kind:"arg", Index:0}
```

**Assertions**: found; `site.Kind == KindArg`; `site.Index == 0`;
`site.Stmt.Name == "create_elevator"`.

**Sufficiency**: baseline positional-argument discrimination.

#### `TestArgAt_OnAttributeValueAndName`

**Setup**:
```
src := "create_elevator A {\n  number_of_objects 5\n}\n"
```
**Input**: pos on `5` (line 1) and pos on `number_of_objects` (line 1).

**Trace**:
```
Parse → owner command with Attributes[number_of_objects{Value:5}]
ArgAt(posOn5)   → Value.Range contains pos → attr, name number_of_objects
ArgAt(posOnName) → nameAt contains pos   → attr, name number_of_objects
```

**Assertions**: both → `Kind == KindAttr`, `Name == "number_of_objects"`,
`Stmt.Name == "create_elevator"`.

**Sufficiency**: attribute discrimination on name **and** value
(name-match identity; flag attributes work through the name branch).

#### `TestArgAt_TrailingSameLine`

**Setup**: `src := "create_elevation 7 "`; pos past the end of the line
(column of len(line), line 0).

**Input**: `pos = {0, len("create_elevation 7 ")}`.

**Trace**:
```
ArgAt(pos) → ownerAt: band picks the statement whose start <= pos
  (raw StatementAt would fall back to an *earlier* statement — the quirk)
  → pos in no Args range, no attrs → kind=none
```

**Assertions**: found; `Kind == KindNone`; `Stmt.Name ==
"create_elevation"` (not a preceding statement's name).

**Sufficiency**: the band-owner fix — typing the next argument of a
command is the primary RMS hint moment; prevents the wrong command's
signature from appearing.

#### `TestXsAt_KbFunction` (hints)

**Setup**: `store, _ := kb.NewStore()`; `src := "void r() { xsVectorSet(1.0, 2.0, 3.0); }"`;
`file, _ := xs.XsParse(src, "t.xs")`; `computer := NewComputer(store)`.

**Input**: `computer.XsAt(file, posOnSecondArg, nil)` — pos on `2.0`.

**Trace**:
```
XsAt → CallAt → {callee:"xsVectorSet", argIndex:1, onArg:true}
  → pool: no source decls named xsVectorSet, external empty
  → store.Function("xsVectorSet") → kb entry (vector / x,y,z float, all required)
  → label "vector xsVectorSet(float x, float y, float z)"
  → active: 1 < 3 → 1
```

**Assertions**: found; `hint.Label == "vector xsVectorSet(float x, float
y, float z)"`; `hint.Params == ["float x","float y","float z"]`;
`hint.Active == 1`.

**Sufficiency**: the contract's canonical XS example verbatim; kb
fallback path and active-parameter mapping.

#### `TestXsAt_SourceDeclWinsOverKb`

**Setup**: same store; `src := "int xsVectorSet(int q) { return 0; }\n"+
"void r() { xsVectorSet(1 }"` (unclosed call, cursor on the argument);
external = nil.

**Input**: `computer.XsAt(file, posOnArg, nil)`.

**Trace**:
```
CallAt → {callee:"xsVectorSet", argIndex:0, onArg:true}
  → pool: file.Decls has Kind=function, Name match → 1 entry
  → kb NOT consulted
  → label "int xsVectorSet(int q)"; params ["int q"]; active 0
```

**Assertions**: `Label == "int xsVectorSet(int q)"`; `Active == 0`.

**Sufficiency**: truth model — source declarations beat kb; also proves
in-progress calls feed the computer (SC1 end-to-end at the hints level).

#### `TestXsAt_ClosureDeclPool`

**Setup**: same store; `src := "void r() { helper(1,| }"`; external =
`[]xs.Decl{{Kind:"function", Name:"helper", Params: []xs.Param{{Name:"a",
Type:"int"},{Name:"b", Type:"int"}}}}`.

**Input**: `computer.XsAt(file, posAfterComma, external)`.

**Trace**:
```
CallAt → {callee:"helper", argIndex:1, onArg:true}
  → pool: external[0] (document has none)
  → label "helper(int a, int b)" (no ret: Type empty)
  → active 1
```

**Assertions**: `Label == "helper(int a, int b)"`; `Active == 1`.

**Sufficiency**: closure pool (external decls) and the no-return-type
label rule («ret — только если объявлен»).

#### `TestXsAt_OptionalKbParamBracketed`

**Setup**: `indexFunctions` seam — craft a function wire with params
`x` (required) and `z` (optional); build the store via the seam (or use
the real `kb` `xsCreateFile` — label `bool xsCreateFile([bool append])`).

**Input**: a call site on the crafted function; `computer.XsAt(...)`.

**Trace**:
```
kb params: x required, z optional
  → labels: "int x", "[float z]"
  → active on the optional slot → its index
```

**Assertions**: `hint.Params` contains `"[float z]"` (exact bracket
form); `Label` ends with `"(int x, [float z])"`.

**Sufficiency**: the contract's bracket example form; distinguishes the
kb Required flag from source decls (no brackets there).

#### `TestRmsAt_FullListArgsThenAttrs` (hints)

**Setup**: real store; `src := "create_elevation 3"`; `file, _ :=
rms.Parse(src, "t.rms")`; pos on `3`.

**Input**: `computer.RmsAt(file, posOnArg)`.

**Trace**:
```
ArgAt → {stmt create_elevation, kind arg, index 0}
  → store.Command → kb entry (1 arg + 8 attrs, all optional)
  → labels: "[MaxHeight: number 1..16]" (mined range), "[base_terrain:
    const]", …, "[set_scale_by_size]" (empty Kind → name-only), …
  → label "create_elevation([MaxHeight: number 1..16], …)"
  → active: 0
```

**Assertions**: `hint.Params` has `1 + 8 == 9` entries (full list, no
truncation); `hint.Params[0] == "[MaxHeight: number 1..16]"`;
`hint.Params[5] == "[set_scale_by_size]"`; `hint.Active == 0`.

**Sufficiency**: full-list rendering, mined range in labels, flag-attr
name-only form, optional bracketing — the RMS rendering contract in one
test.

#### `TestRmsAt_PercentChanceMinedRange`

**Setup**: real store; `src := "percent_chance 45"`; pos on `45`.

**Input**: `computer.RmsAt(file, pos)`.

**Trace**:
```
ArgAt → arg 0 → store.Command("percent_chance")
  → arg "%" kind percent (structured), mined Range 0..99
  → label "percent_chance(%: percent 0..99)"; active 0
```

**Assertions**: `Label == "percent_chance(%: percent 0..99)"`;
`Params == ["%: percent 0..99"]`; `Active == 0`.

**Sufficiency**: the contract's canonical RMS example verbatim; proves
structured-kind + mined-range compose in one label.

#### `TestRmsAt_ActiveOnAttribute`

**Setup**: real store; attributes attach to the command inside its block:
```
src := "create_elevation 3 {\n  spacing 5\n}\n"
```
pos on `5` (line 1).

**Input**: `computer.RmsAt(file, pos)`.

**Trace**:
```
ArgAt → {kind attr, name "spacing"}
  → active = len(Args)=1 + index of spacing in Attributes(6) = 7
```

**Assertions**: `hint.Active == 7`; `hint.Params[7] == "[spacing:
number]"`.

**Sufficiency**: the attr active mapping (`len(Args)` offset) lands on
the rendered attribute's own slot.

#### `TestServe_InitializeAdvertisesSignatureHelp`

**Setup**: `startHarness`; `Initialize(&protocol.InitializeParams{})`.

**Input**: the initialize request.

**Trace**:
```
Initialize → caps.SignatureHelpProvider = &SignatureHelpOptions{
  TriggerCharacters: ["(", ","]}
```

**Assertions**: provider non-nil; trigger characters exactly `["(", ","]`
(order included); existing capabilities unchanged (Hover/Completion/
Definition/References/DocumentSymbol, Full+OpenClose sync).

**Sufficiency**: the capability contract; the trigger set must not
intersect completion's `" "`/`"<"`.

#### `TestServe_SignatureHelpXs`

**Setup**: harness; open `file:///work/script.xs` with text
`"void test() {\n\txsVectorSet(1.0, 2.0, 3.0);\n}\n"`; wait diagnostics.

**Input**: `SignatureHelp` at `Position{Line:1, Character: index of
"2.0"}`.

**Trace**:
```
SignatureHelp → .xs → XsParse → Closure(uri).ExternalDecls(uri)
  → computer.XsAt → Hint{active:1}
  → toSignatureHelp → 1 signature, ActiveSignature 0,
    ActiveParameter *1, label "vector xsVectorSet(float x, float y, float z)"
```

**Assertions**: result non-nil; `len(Signatures) == 1`;
`*ActiveSignature == 0`; `*ActiveParameter == 1`; `Signatures[0].Label`
equals the expected string; `len(Parameters) == 3`.

**Sufficiency**: the full XS stack through the real stdio harness,
including DI wiring (`Serve` built the computer).

#### `TestServe_SignatureHelpRms`

**Setup**: harness; open `file:///work/map.rms` with
`"<LAND_GENERATION>\ncreate_elevation 3\n"`; wait diagnostics.

**Input**: `SignatureHelp` at the position of `3`.

**Trace**:
```
.rms → Parse → no XsBlock contains pos → computer.RmsAt
  → ArgAt arg 0 → full list render, active 0
```

**Assertions**: one signature; label starts with
`"create_elevation("`; `*ActiveParameter == 0`; params include
`"[MaxHeight: number 1..16]"`.

**Sufficiency**: the RMS path end-to-end.

#### `TestServe_SignatureHelpInlineXsBlock`

**Setup**: harness; open `file:///work/map.rms` with (the call left
unclosed, cursor marked by the requested position):
```
#includeXS
void b() { xsVectorSet(1.0, 2 }
```
wait diagnostics.

**Input**: `SignatureHelp` at the position of `2` (line 1).

**Trace**:
```
.rms → Parse → XsBlock.Range contains pos
  → unshiftPos → block-local pos → XsParse(block.Code)
  → Closure(uri).ExternalDecls("") → computer.XsAt(xsFile, blockPos, ext)
  → Hint active 1 → protocol result
```

**Assertions**: one signature; label
`"vector xsVectorSet(float x, float y, float z)"`; `*ActiveParameter ==
1` — computed from the **translated** position (a wrong translation
yields silence or a wrong index, so this pins the shift math).

**Sufficiency**: the inline-block coordinate template — the only place
block-local translation is exercised end-to-end.

#### `TestServe_SignatureHelpStateless`

**Setup**: same document/position as the XS test.

**Input**: two requests — first with
`Context: {TriggerKind: Invoked}` (manual), then with
`{TriggerKind: TriggerCharacter, TriggerCharacter: "(", IsRetrigger:
true}`.

**Trace**:
```
both requests → identical (parse, closure, XsAt) path; Context unused
```

**Assertions**: the two results deep-equal (label, params, active).

**Sufficiency**: the statelessness Requirement — the answer must depend
only on (document, position), never on the trigger context.

---

### Negative Tests

#### `TestMineKindRange_NoBounds`

**Setup**: none (pure function).

**Input**: `desc = "plain description without bounds"`.

**Trace**:
```
MineKindRange(desc) → mineBoundsRe.FindStringSubmatch → nil
  → return ("", ValueRange{})
```

**Assertions**: `kind == ""`; `r == ValueRange{}` (both Min and Max
empty).

**Sufficiency**: unparseable prose → empty result, not an error
(contract Constraint — the empty result is the error channel).

#### `TestMineKindRange_DefaultAndSeeFragmentsIgnored`

**Setup**: none (pure function).

**Input**: `"(default: 5)"`; `"(see: create_elevation)"`;
`"number (default: 0 - not elevated)"`.

**Trace**:
```
each input → the regex requires the paren content to be exactly
  <num>-<num>; "default: 5", "see: …", "default: 0 - not elevated"
  start with a non-digit right after "(" → no match anywhere
  → ("", ValueRange{})
```

**Assertions**: all three → `("", ValueRange{})`.

**Sufficiency**: contract Constraint — non-numeric parentheticals never
match; the third input is a real corpus string (create_elevation's Desc)
whose `"0 - not elevated"` tail must not be mined.

#### `TestCallAt_InString`

**Setup**: `src := 'void f() { g("ab|c"); }'`; `file, _ := XsParse(src,
"t.xs")`.

**Input**: `pos` inside the string token (on `c`).

**Trace**:
```
XsParse → noncode holds the xString token span; callRec g spans to
  ")"-end (contains pos)
CallAt(pos) → step 1: pos inside a noncode span → found=false
  (checked before any call lookup)
```

**Assertions**: `found == false`.

**Sufficiency**: contract step 1 — strings never resolve to a call
(even though the string sits inside `g`'s argument span).

#### `TestCallAt_InComment`

**Setup**: `src := "// g(a, b)\nvoid f() { g(a, b); }"`; `file, _ :=
XsParse(src, "t.xs")`.

**Input**: `pos` inside the line comment (line 0, column 4).

**Trace**:
```
XsParse → noncode holds the // span [start of "//", end of line 0)
CallAt(pos) → noncode contains pos → found=false; the line-1 call
  record's span starts after the comment and never contains pos
```

**Assertions**: `found == false`.

**Sufficiency**: comment spans are recorded trivia; the call on line 1
must not capture the comment cursor.

#### `TestCallAt_InBlockComment`

**Setup**:
```
src := "void f() { /* open\nstill comment */ g(a); }"
```
`file, _ := XsParse(src, "t.xs")`.

**Input**: `pos` on line 1 inside the block comment (on `still`).

**Trace**:
```
XsParse → skipBlockComment consumes both lines; noncode holds one span
  from "/*" on line 0 past the "*/" on line 1
CallAt(pos) → step 1: noncode contains pos → found=false (the g record
  starts after the comment, so no call span contains pos either —
  both checks agree; the noncode check is what makes the order
  contract step 1, not an accident of this fixture)
```

**Assertions**: `found == false`.

**Sufficiency**: multi-line block-comment spans — the only comment form
whose span crosses lines; guards the span-extent recording (start at
`/*`, end past `*/`).

#### `TestCallAt_VectorLiteralNotCall`

**Setup**: `src := "void f() { vector v = (1, 2, 3); }"`; `file, _ :=
XsParse(src, "t.xs")`.

**Input**: `pos` on `3` (the third vector component).

**Trace**:
```
XsParse → parseParenExpr builds the vector literal; only parsePostfix's
  "(" case records callRecs → no record exists
CallAt(pos) → no record's span contains pos → found=false
```

**Assertions**: `found == false`.

**Sufficiency**: the global-annotation rule — vector literals are not
calls (a hint for `(1,2,3)` would be nonsense).

#### `TestCallAt_ParamListNotCall`

**Setup**: `src := "int f(int a, int b) { return 0; }"`; `file, _ :=
XsParse(src, "t.xs")`.

**Input**: `pos` on `b` in the parameter list (line 0, its column).

**Trace**:
```
XsParse → parseParams consumes the "(int a, int b)" list without
  creating a callRec (only parsePostfix's "(" case records)
CallAt(pos) → no containing span → found=false
```

**Assertions**: `found == false`.

**Sufficiency**: declaration parameter lists are not call contexts.

#### `TestArgAt_InComment`

**Setup**: `src := "create_elevation 3 /* hill */"`; `file, _ :=
rms.Parse(src, "t.rms")`.

**Input**: `pos` inside the block comment (on `hill`).

**Trace**:
```
Parse → blankComments emits the /* */ extent into comments; the
  command's Args[0] is `3` (the comment is blanked)
ArgAt(pos) → comment span contains pos → found=false (checked before
  owner lookup)
```

**Assertions**: `found == false`.

**Sufficiency**: RMS comment exclusion (blankComments-recorded spans).

#### `TestArgAt_InString`

**Setup**:
```
src := "create_object GOLF_BALL {\n  object_name \"grass|land\"\n}\n"
```
`file, _ := rms.Parse(src, "t.rms")` — an attribute with a quoted
string value on a statement line.

**Input**: `pos` inside the quotes (on `land`, line 1).

**Trace**:
```
Parse → the attribute line's rest tokens include the tokString
  "grassland"; its range is recorded into strings
ArgAt(pos) → string span contains pos → found=false (step 2 precedes
  the attribute name/value checks)
```

**Assertions**: `found == false`.

**Sufficiency**: string exclusion on the RMS side — editing a quoted
attribute value renders no hint rather than the attribute's own.

#### `TestArgAt_OnDirectiveAndSectionHeader`

**Setup**:
```
src := "#include other.rms\n" +
	"<LAND_GENERATION>\n" +
	"#const TERRAIN 7\n" +
	"create_elevation 3\n"
```
**Input**: pos on `other.rms` (include-argument line); pos on the
section header line; pos on `7` in `#const TERRAIN 7`.
**Trace**: include/header lines are excluded spans (found=false even
though no statement precedes here); `#const` statements are filtered by
the `#`-prefix rule.
**Assertions**: all three → `found == false`.
**Sufficiency**: contract step 1's exclusions, including the
directive-styled statements the parser materializes. (The
preceding-command configuration is covered by
`TestArgAt_NonStatementLinesAfterCommand`.)

#### `TestXsAt_UnknownFunction` (hints)

**Setup**: `store, _ := kb.NewStore()`; `src := "void r() { nosuchfn(1
}"`; `file, _ := xs.XsParse(src, "t.xs")`; `computer :=
NewComputer(store)`.

**Input**: `computer.XsAt(file, posOnArg, nil)`.

**Trace**:
```
CallAt → {callee:"nosuchfn", argIndex:0, onArg:true}
  → pool: no source decls named nosuchfn; external empty
  → store.Function("nosuchfn") → found=false → silence
```

**Assertions**: `found == false`.

**Sufficiency**: silence for unknown names (trust rule).

#### `TestXsAt_ConflictingSourceDecls`

**Setup**: document declares `void h(int a)`; external contains
`void h(float b)`; call `h(` with cursor on the argument.

**Input**: `computer.XsAt(file, posOnArg, external)` — `external =
[]xs.Decl{{Kind:"function", Name:"h", Params: []xs.Param{{Name:"b",
Type:"float"}}}}`.

**Trace**:
```
CallAt → {callee:"h", argIndex:0, onArg:true}
  → pool: document decl (int,a) + external decl (float,b) — 2 entries
  → paramsEqual(pool[1], pool[0]): (float,b) ≠ (int,a) → silence
```

**Assertions**: `found == false`.

**Sufficiency**: the conflict rule — a wrong-signature hint is worse
than none.

#### `TestXsAt_NoCallContext`

**Setup**: `src := "int q = 1;\nvoid r() { int w = q; }"`; `file, _ :=
xs.XsParse(src, "t.xs")`; real store.

**Input**: `computer.XsAt(file, posOnQ, nil)` — pos on the identifier
`q` on line 1.

**Trace**:
```
CallAt(pos) → no callRec span contains pos (no call anywhere)
  → found=false → silence
```

**Assertions**: `found == false`.

**Sufficiency**: hints require a call context; identifiers route to
hover/completion instead.

#### `TestRmsAt_UnknownCommand`

**Setup**: `src := "create_elefant 5"` (typo) — pos on `5`.

**Input**: `computer.RmsAt(file, posOn5)`.

**Trace**: `ArgAt` succeeds (arg 0); `store.Command("create_elefant")`
misses → silence.

**Assertions**: `found == false`.

**Sufficiency**: RMS hints are kb-gated — no invented signatures.

#### `TestServe_SignatureHelpSilence`

**Setup**: harness; open a `.rms` document with `"\n\n"`.

**Input**: `SignatureHelp` at `{0,0}`.

**Trace**:
```
openDocument → text "\n\n" → .rms → Parse → no XsBlock contains pos
  → RmsAt → ArgAt({0,0}): no excluded/string/comment hit; ownerAt finds
  no statement (blank lines skipped, empty global not materialized)
  → found=false → nil, nil
```

**Assertions**: `help == nil && err == nil`.

**Sufficiency**: the nil,nil silence convention through the protocol
layer (nullable result, not an error, not an empty SignatureHelp).

---

### Edge Case Tests

#### `TestMineKindRange_EmptyDesc`

**Setup**: none (pure function).

**Input**: `desc = ""`.

**Trace**:
```
MineKindRange("") → regex finds no submatch → ("", ValueRange{})
```

**Assertions**: `kind == ""`; `r == ValueRange{}`; no panic.
**Sufficiency**: flag attributes (34 corpus entries) — empty Desc, no
panic, no fill.

#### `TestMineKindRange_BoundsWithoutKindWord`

**Setup**: none (pure function).

**Input**: `desc = "(0-5) picks randomly"`.

**Trace**:
```
regex: the optional word group matches empty (no word precedes "(")
  → ("", ValueRange{"0","5"})
```

**Assertions**: `kind == ""`; `r == ValueRange{"0","5"}`.
**Sufficiency**: kind word absent → kind "" while bounds still mine
(the fragments are independent).

#### `TestMineKindRange_FirstBoundsFragmentWins`

**Setup**: none (pure function).

**Input**: `desc = "number (0-5) or (10-20)"`.

**Trace**:
```
FindStringSubmatch returns the leftmost match → "number (0-5)"
  → ("number", ValueRange{"0","5"})   # the second fragment is ignored
```

**Assertions**: `kind == "number"`; `r == ValueRange{"0","5"}`.
**Sufficiency**: the Algorithm's first-match edge case — a refactor to
last-match (FindAll + [-1]) or a re-anchored pattern would flip this.

#### `TestStore_FlagAttributesStayNameOnly`

**Setup**: `indexCommands` seam with attribute
`{"name":"set_scale_by_size","kind":"","required":false,"desc":""}`.

**Input**: the JSON payload above.

**Trace**:
```
indexCommands → CommandArg{Kind:"", Desc:""}
  → fill-when-empty: MineKindRange("") → no match → nothing filled
```

**Assertions**: after load `Kind == ""`, `Range == ValueRange{}`.
**Sufficiency**: the name-only flag form survives the load pipeline
(no invented kinds).

#### `TestCallAt_UnclosedToEOF`

**Setup**: `src := "void f() { g(h("`.

**Input**: `pos` = the EOF position `{0, 15}`.

**Trace**: both records' spans end at the `eofPos` sentinel (which
compares after every real position, so the exact-EOF cursor is
contained); latest lparen = `h` → arg 0.
**Assertions**: `{Callee:"h", ArgIndex:0, OnArg:true}`.
**Sufficiency**: recovery-frontier spans — doubly-nested in-progress
calls at the exact end of input.

#### `TestCallAt_PositionOnOpenParenChar`

**Setup**: `src := "void f() { g(a); }"`; `file, _ := XsParse(src,
"t.xs")`.

**Input**: `pos` exactly on the `(` character (its column).

**Trace**:
```
CallAt(pos) → span contains pos; NOT pos.After(lparen) (pos == lparen)
  → callee-side branch → {Callee:"g", ArgIndex:0, OnArg:false}
```

**Assertions**: `{Callee:"g", ArgIndex:0, OnArg:false}`, found.
**Sufficiency**: the exact boundary between contract steps 4 and 5 —
the `(` character itself is callee-side (no active parameter), one
column later flips to SC1's `onArg=true`; guards an off-by-one in the
`pos.After(lparen)` check.

#### `TestCallAt_PositionOnCommaChar`

**Setup**: `src := "void f() { g(a, b); }"`.
**Input**: `pos` exactly on the `,` character (its column).
**Trace**: the comma's end (pos+1) is not `<= pos` → k stays 0.
**Assertions**: `{ArgIndex:0, OnArg:true}`.
**Sufficiency**: comma-character ownership — the previous argument
region; the after-comma case (k=1) is already covered by the closed-call
test.

#### `TestCallAt_ArgIndexNeverClamped`

**Setup**: `src := "void f() { g(a, b, c" ` (three args typed; `g`'s kb
existence is irrelevant to navigation).

**Input**: `pos` after the second comma.

**Trace**:
```
CallAt(pos) → span [g, eofPos) contains pos; two recorded comma ends
  <= pos → k=2, onArg=true (no comparison against any parameter count)
```

**Assertions**: `ArgIndex == 2` (navigation reports the true ordinal
even beyond any declared parameter count).
**Sufficiency**: contract Constraint — clamping is a consumer decision,
never the navigation's.

#### `TestCallAt_Deterministic`

**Setup**: any fixture; parse once.
**Input**: the same `pos` queried twice.
**Trace**: both queries walk the same immutable `calls`/`noncode`
indices — no state changes between calls.
**Assertions**: equal results.
**Sufficiency**: determinism Requirement (pure index lookup).

#### `TestArgAt_BraceAndGapPositions`

**Setup**:
```
src := "create_elevation 3 {\n  spacing 5\n}\n"
```
**Input**: pos on `{` (line 0), pos on `}` (line 2), pos on the space
between `create_elevation` and `3`.
**Trace**:
```
ArgAt(posBrace)  → owner = create_elevation (band); pos in no Args
                   range (Args=[3]); `{` line has no attrs → kind=none
ArgAt(posRBrace) → owner = create_elevation (its Range grew over the
                   block via closeBrace) → kind=none
ArgAt(posGap)    → owner = create_elevation; between-token gap is in
                   no Args range → kind=none
```
**Assertions**: all → found, `Kind == KindNone`, `Stmt.Name ==
"create_elevation"` (hint renders with no active mark).
**Sufficiency**: contract step 6 — braces/gaps never guess a label (SC6).

#### `TestArgAt_NonStatementLinesAfterCommand`

**Setup**:
```
src := "create_elevation 3\n" +
	"<LAND_GENERATION>\n" +
	"#include \"other.rms\"\n" +
	"# a plain comment\n"
```
`file, _ := rms.Parse(src, "t.rms")`.

**Input**: four positions — on `3` (line 0), on the section-header
line (line 1), inside the quoted include path (line 2, inside
`"other.rms"`), and on the `#`-comment line (line 3).

**Trace**:
```
Parse → global section holds create_elevation; header/include/#-lines
  record no statement but DO record excluded spans (lines 1, 2, 3)
ArgAt(posOn3)      → no excluded span; band owner = create_elevation;
                     Args[0] contains pos → arg 0
ArgAt(posHeader)   → excluded span (line 1) → found=false (the band
                     alone would pick create_elevation as owner)
ArgAt(posInclude)  → excluded span (line 2, incl. the path string) →
                     found=false
ArgAt(posComment)  → excluded span (line 3) → found=false
```

**Assertions**: `ArgAt(posOn3)` → found, `Kind == KindArg`,
`Index == 0`; each of the other three positions → `found == false`
(no hint — not `create_elevation` with `kind=none`).

**Sufficiency**: pins the contract's categorical exclusion («директивы
#include, заголовки секций → found=false») in the only configuration
that exposes it — a preceding command exists. Prevents the band-owner
model from leaking the previous command's signature onto header /
include-path / comment cursors (trust rule: silence, never a guessed
hint); also covers quoted include strings (contract step 2) and
`#`-comment lines, which `blankComments` does not blank.

#### `TestArgAt_NestedBlockInnermost`

**Setup**:
```
src := "start_random\n  percent_chance 50\n    create_elevation 3\n  end_random\nend_random\n"
```
**Input**: pos on `3`.
**Trace**: band recursion descends start_random → percent_chance →
create_elevation (innermost owning statement).
**Assertions**: `Stmt.Name == "create_elevation"`, `Kind == KindArg`,
`Index == 0`.
**Sufficiency**: «владеет самый внутренний statement» — nested blocks.

#### `TestArgAt_Deterministic`

**Setup**: any fixture; parse once.
**Input**: the same `pos` queried twice.
**Trace**: both queries walk the same immutable statement tree and
recorded spans.
**Assertions**: equal results.
**Sufficiency**: determinism Requirement.

#### `TestXsAt_EqualParamsMerged`

**Setup**: document declares `int h(int a)`; external has `int h(int a)`
(identical params); call `h(` on its argument.
**Trace**: pool of 2, sequences equal → merged → render from pool-first.
**Assertions**: found; `Label == "int h(int a)"`; `Active == 0`.
**Sufficiency**: the «единая декларация» rule and the deterministic
pool-order decision (Applied Fixes §1).

#### `TestXsAt_ActiveNoneOnCallee`

**Setup**: real store; `src := "void r() { xsVectorSet("`.
**Input**: `computer.XsAt(file, posOnCallee, nil)` — pos on the callee
name.
**Trace**: `CallAt` → `OnArg=false` → `active = -1`; the kb render
proceeds as usual.
**Assertions**: found; `hint.Active == -1`.
**Sufficiency**: onArg=false → no active parameter (server maps to nil).

#### `TestXsAt_ArgIndexBeyondParams`

**Setup**: kb function with 1 param (crafted `indexFunctions` seam);
call with two typed args `fn(a, b|`.
**Input**: `computer.XsAt(file, posAfterB, nil)`.
**Trace**: `CallAt` → `argIndex=1`; `1 < len(params)=1` is false →
`active = -1`.
**Assertions**: `hint.Active == -1` (argIndex 1 >= len(params) 1).
**Sufficiency**: never-clamp at the hints level — no highlight beats a
wrong highlight.

#### `TestRmsAt_ActiveAttrNotInKb`

**Setup**: a document attribute name absent from the kb entry
(e.g. `number_of_objectz` typo) with pos on it.
**Input**: `computer.RmsAt(file, posOnTypo)`.
**Trace**: `ArgAt` → `kind=attr, name="number_of_objectz"`;
`indexOf(cmd.Attributes, "number_of_objectz")` misses → `active = -1`;
the full kb list still renders.
**Assertions**: `hint.Active == -1`; label still the full kb list.
**Sufficiency**: name-match failure → −1 (never a guessed position).

#### `TestRmsAt_ActiveIndexPassesThrough`

**Setup**: `create_elevation 1 2 3` (more args than the kb declares).
**Input**: `computer.RmsAt(file, posOnThird)` — pos on `3` → index 2.
**Trace**: `ArgAt` → `kind=arg, index=2` → `active = 2` (no comparison
against `len(cmd.Args)=1`).
**Assertions**: `hint.Active == 2` (pass-through, beyond the single
declared arg's slot 0; the client shows no active mark).
**Sufficiency**: RMS never-clamp (SC2-family rule).

#### `TestServe_SignatureHelpRmsNoActiveParameter`

**Setup**: harness; open `file:///work/map.rms` with
`"<LAND_GENERATION>\ncreate_elevation 3\n"`; wait diagnostics.

**Input**: `SignatureHelp` at the position of the command name
(`create_elevation`, line 1).

**Trace**:
```
.rms → Parse → no XsBlock contains pos → computer.RmsAt
  → ArgAt → kind=none → full list renders, active = -1
  → toSignatureHelp → ActiveParameter = nil (not 0, not last)
```

**Assertions**: one signature; `Signatures[0].ActiveParameter == nil`;
label starts with `"create_elevation("`.
**Sufficiency**: the RMS `kind=none` path end-to-end — the nil
`ActiveParameter` mapping is otherwise server-tested only for XS.

#### `TestServe_ExistingBehaviorUnchanged` (SC8 sweep)

**Setup**: run the full existing test suite (`go test ./...`).
**Assertions**: all pre-existing tests pass unmodified (diagnostics,
hover, completion, navigation, store, parsers); the only edited existing
test is the `NewServer` call-site helper (signature change).
**Sufficiency**: additivity (C4/SC8) — the new handler and mining must
not alter any published behavior; the percent-kind set and rendered
signatures are provably unchanged (34 empty-Kind entries all have empty
Desc → no Kind fills in this corpus).

---

## Additional Instructions for the Implementation Agent

- **Implementation order** (dependency-correct): kb → xs → rms → hints →
  server; within kb: `model.go` (`ValueRange`, `CommandArg.Range`) →
  `mine.go` → `store.go` (wire + fill helper) → `extract.go` →
  `cmd/kbgen/main.go` (wire mirror).
- **Wire schema**: `argWire` and `cmd/kbgen`'s `argJSON` both gain
  `Range kb.ValueRange `json:"range"``; `ValueRange` carries
  `json:"min"`/`json:"max"` tags so one type serves model and wire. Do
  **not** regenerate `kb/data/rms-commands.json` in this change —
  load-time mining covers the committed data (verified: 34 empty-Kind
  entries all have empty Desc → no Kind fills; 8 bounded entries mine at
  load). Regeneration stays a `cmd/kbgen` operator action; the
  idempotency test proves extraction/load agreement without touching the
  committed file.
- **Parser index additions are unexported and append-only** (`calls`,
  `noncode` on `XsFile`; `strings`, `comments`, `excluded` on `RmsFile`;
  `nameAt` on `Attribute`): follow the existing `symbols`/`words` index
  pattern; never mutate after parse returns.
- **Do not modify `StatementAt`** — `ArgAt` uses its own `ownerAt`
  (band semantics); hover behavior is frozen (SC8).
- **Protocol construction**: follow `.goga/usages/cooks/lsp-protocol.md`
  (Signature Help section) as the binding reference for
  `SignatureHelp`/`SignatureInformation`/`ParameterInformation` shapes;
  if `ParameterInformation.Label` is one of the library's union fields,
  wrap with `protocol.String` exactly as `server.go` does for
  `CompletionItem.Documentation`.
- **Update the `NewServer` call sites** in `server/serve.go` and the
  `server/navigation_test.go` helper in the same commit; no other
  callers exist.
- **Naming**: exported kind constants in rms follow the manifest's
  `Kind*` convention (`KindArg`, `KindAttr`, `KindNone`).
- **Environment note**: the design was produced in an environment where
  the Go toolchain could not build (`go.mod` requires go ≥ 1.26.6, local
  toolchain 1.26.4, toolchain download blocked). All code references
  were read from source; run `go build ./... && go test ./...` in a
  capable environment (CI) — the SC8 sweep and the full test stack above
  are the acceptance gate.
