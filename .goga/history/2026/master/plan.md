# Plan: `master` — Signature Help for AoE2 RMS + XS

<!-- Topic: `master` (`.goga/history/2026/master/`) — compiled from the
reviewed design `.goga/history/2026/master/design.md` over the materialized
CODEMANIFEST contracts (kb, xs, rms, hints [new], server). -->

## Purpose

Implement LSP Signature Help (`textDocument/signatureHelp`) for the AoE2
RMS + XS language server, exactly as specified by the already-materialized
contracts and the reviewed design:

- **kb** — mine value bounds (`ValueRange`) and kind words from `CommandArg`
  Desc prose at load and extraction time (fill-when-empty, idempotent).
- **xs** — `XsFile.CallAt(pos)`: innermost in-progress-or-closed call
  enclosing the cursor, with argument ordinal (parser-recorded `calls` +
  `noncode` indices).
- **rms** — `RmsFile.ArgAt(pos)`: owning command + argument/attribute
  discrimination (parser-recorded `strings`/`comments`/`excluded` indices,
  `Attribute.nameAt`, band-based `ownerAt`).
- **hints** (new cell) — `Computer.XsAt`/`RmsAt`: truth-model (source > kb),
  conflict rule, protocol-agnostic `Hint` rendering.
- **server** — `SignatureHelp` handler with `.xs` / inline-XS-block /
  `.rms` routing, `unshiftPos` coordinate translation, capability
  advertisement (triggers `(` and `,`), `NewComputer` wiring.

Overall strategy: dependency-correct order **kb → xs → rms → hints →
server**, TDD per task (contract tests first), silence-never-guess
everywhere, zero behavioral drift for existing consumers (SC8).

## Context

### Contract Surface

**Entity: `kb.ValueRange(min: string, max: string)`** (NEW)
- Type: data entity
- Declared `location`: `kb/model.go`
- Facade obligation: importable from package `kb`
- Properties: `Min -> string`, `Max -> string` ("" — not mined)
- Semantic requirements: pure data (construct-and-use, no mutation); bounds
  are strings — prose text without precision conversion; carries
  `json:"min"`/`json:"max"` tags so one type serves model and wire.
- Annotation context: mined value bounds from Desc-prose.

**Entity: `kb.MineKindRange(desc: string) -> kind: string, r: ValueRange`** (NEW)
- Type: routine (function)
- Declared `location`: `kb/mine.go` (new file)
- Facade obligation: importable from package `kb`
- Semantic requirements: pure, deterministic; unparseable prose → empty
  result, never an error; only numeric-bounds parentheticals match.
- Annotation context: Algorithm 1–3 + Requirements/Constraints per
  `kb/CODEMANIFEST` (see Task 1).

**Entity: `kb.Store()`** (CHANGED)
- Type: entity (struct + factory `NewStore`)
- Declared `location`: `kb/store.go`
- Change: load-time mining Requirement — on load, `CommandArg` with empty
  `Range` or empty `Kind` → `MineKindRange(Desc)`; idempotent with
  extraction (fill only empty); unparseable prose silently stays raw.
- Existing methods (`Function`, `Functions`, `Constant`, `Constants`,
  `Command`, `Commands`, `Attribute`) are unchanged and serve mined data.

**Entity: `kb.CommandArg()`** (CHANGED)
- Type: data entity
- Declared `location`: `kb/model.go`
- Change: new property `Range -> ValueRange` (mined bounds; empty — not
  mined); Kind semantics after load (empty filled by mining, structured
  never overwritten).

**Entity: `kb.ExtractRmsCommands(path: string) -> commands: []Command, err: error`** (CHANGED)
- Type: routine (function)
- Declared `location`: `kb/extract.go`
- Change: Algorithm step 3 gains the mining pass — per built `CommandArg`,
  `MineKindRange(Desc)` → fill `Range`, empty `Kind` → mined kind.
- Imported dependencies: none (leaf cell).

**Entity: `xs.XsFile()`** (CHANGED)
- Type: entity (struct, value methods)
- Declared `location`: `xs/ast.go`
- Change: new method `CallAt(pos: Pos) -> call: CallSite, found: bool`.
- Imported dependencies: `common` (`Pos`, `Range`, `Diagnostic`, `Symbol`).

**Entity: `xs.CallSite(callee: string, argIndex: int, onArg: bool)`** (NEW)
- Type: data entity
- Declared `location`: `xs/ast.go`
- Facade obligation: importable from package `xs`
- Properties: `Callee -> string`, `ArgIndex -> int`, `OnArg -> bool`
- Semantic requirements: pure data; `argIndex` valid when `onArg=true`;
  never clamped.

**Entity: `rms.RmsFile()`** (CHANGED)
- Type: entity (struct, value methods)
- Declared `location`: `rms/ast.go`
- Change: new method `ArgAt(pos: Pos) -> site: ArgSite, found: bool`.
- Imported dependencies: `common`.

**Entity: `rms.ArgSite(stmt: Statement, kind: string, index: int, name: string)`** (NEW)
- Type: data entity
- Declared `location`: `rms/ast.go`
- Facade obligation: importable from package `rms`
- Properties: `Stmt -> Statement`, `Kind -> string` (arg/attr/none),
  `Index -> int`, `Name -> string`
- Semantic requirements: pure data; kind constants exported as
  `KindArg = "arg"`, `KindAttr = "attr"`, `KindNone = "none"` (manifest
  `Kind*` Go-constant convention).

**Entity: `hints.Computer(store: Store)`** (NEW — new cell `hints`)
- Type: entity (struct + factory `NewComputer`)
- Declared `location`: `hints/computer.go` (new file)
- Facade obligation: importable from package `hints`
- Methods: `XsAt(file: XsFile, pos: Pos, external: []Decl) -> hint: Hint, found: bool`;
  `RmsAt(file: RmsFile, pos: Pos) -> hint: Hint, found: bool`
- Semantic requirements: stateless (no mutable state between calls);
  documentation stays out of `Hint` (hover's job); truth model
  source-decls > kb; conflicting source decls → silence; active parameter
  never clamped.
- Imported dependencies: `Pos` (common); `Store`, `Function`, `Command`,
  `CommandArg`, `ValueRange` (kb); `XsFile`, `CallSite`, `Decl` (xs);
  `RmsFile`, `ArgSite`, `Statement` (rms).

**Entity: `hints.Hint(label: string, params: []string, active: int)`** (NEW)
- Type: data entity
- Declared `location`: `hints/hint.go` (new file)
- Facade obligation: importable from package `hints`
- Properties: `Label -> string`, `Params -> []string`, `Active -> int`
- Semantic requirements: pure data; mapping to
  `protocol.SignatureInformation` is the server's responsibility.

**Entity: `server.Server(store: Store, analyzer: Analyzer, computer: Computer)`** (CHANGED)
- Type: entity (struct + factory `NewServer`)
- Declared `location`: `server/server.go`
- Changes: constructor gains the third DI parameter `computer: Computer`;
  `Initialize` advertises `SignatureHelpProvider` with triggers `(` and `,`;
  new method `SignatureHelp(ctx, params) -> help: SignatureHelp, err: error`
  (stateless, read-only, additive).
- Imported dependencies: existing set + `Computer`, `Hint` from `hints`.

**Entity: `server.Serve(ctx: Context) -> err: error`** (CHANGED)
- Type: routine (function)
- Declared `location`: `server/serve.go`
- Change: dependency assembly grows `NewComputer(store)` between
  `NewAnalyzer` and `NewServer`; both `NewServer` call sites
  (`serve.go:31`, `server/navigation_test.go:25`) updated in the same change.

### Entity Interaction and Data Flow

Interaction diagram (design, verbatim):

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

Data flows (design, verbatim):

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

Entity dependencies (design, verbatim — initialization order, leaves →
root, matching the task ordering of this plan):

1. `kb` (no imports) — `NewStore()` (+ `MineKindRange` used by extraction).
2. `xs`, `rms` (import `common` only).
3. `hints.NewComputer(store)` — needs kb + xs + rms types.
4. `server.NewServer(store, analyzer, computer)`; `server.Serve` wires
   `NewStore → NewAnalyzer → NewComputer → NewServer → DocStore/Resolver`.

Runtime dependency direction: `server → hints → {kb, xs, rms} → common`;
no cycles (verified by `goga schema`).

### Re-exports

None — no `->Name: {}` blocks exist in any of the five manifests.

### Usages Context

- `conventions` (`.goga/usages/conventions.md`, all five cells): Go coding
  and testing rules — constructor DI, table-driven tests with testify
  `require`/`assert`, naming `Test<Component>_<Scenario>`, `goimports`
  formatting, doc comments. Mandatory baseline for every task.
- `kbdata` (kb, inline in `kb/CODEMANIFEST`): embedded JSON schemas plus
  the «Desc prose mining» block — bounds shape `(<num>-<num>)` after the
  kind word; decimals/negatives allowed; only numeric bounds match;
  percent-typed args keep structured kind; flag attributes (34 entries)
  carry empty Desc and stay name-only.

### Imported Usages

- `positions-and-diagnostics` — from `common`, `common/.usages/` (cell
  level, imported by xs/rms/hints): `common.Pos`/`common.Range`
  construction, half-open `Range.Contains(pos)` semantics. Used by every
  recorded span and containment test in Tasks 4–7.
- `lookups` — from `kb`, `kb/.usages/lookups.md` (imported by hints):
  `NewStore`, exact-name lookups (`Function`, `Command`, `Attribute`),
  mined kind/range rendering contract for consumers; `(value, found)`
  pairs, silence on `found=false`. Used in Task 7.
- `xs-parsing` — from `xs`, `xs/.usages/xs-parsing.md` (imported by
  hints/server): `XsParse`, `CallAt` consumer semantics (in-progress calls,
  innermost wins, ArgIndex may exceed params). Used in Tasks 7–8.
- `rms-parsing` — from `rms`, `rms/.usages/rms-parsing.md` (imported by
  hints/server): `Parse`, `ArgAt` consumer semantics (owner resolution,
  kind vocabulary, ambiguity → none; directives/section headers →
  found=false). Used in Tasks 7–8.
- `computing` — from `hints`, `hints/.usages/computing.md` (imported by
  server): `NewComputer`, `XsAt`/`RmsAt` call patterns, silence mapping
  (`found=false → nil, nil`), protocol-mapping boundary (`Hint` →
  `protocol.SignatureInformation` belongs to server). Used in Task 8.
- `closure` — from `include` (imported by server, unchanged): signature
  help consumes `Closure.ExternalDecls` exactly as `analyzeXs`/`analyzeRms`
  do. Read for context in Task 8.
- `checks`, `symbols` — from `analysis`/`common` (imported by server,
  unchanged): untouched by this change (SC8). Read for context only.
- `lsp-protocol` — `.goga/usages/cooks/lsp-protocol.md` (server local
  usage): go.lsp.dev construction patterns; the Signature Help section is
  the binding reference for `SignatureHelpOptions`, single-signature
  result shape, `ActiveSignature` 0, `ActiveParameter *uint32`
  nil-when-none, plain-string parameter labels, omitted documentation,
  statelessness. Used in Tasks 8–9.

### Local Usages

No new `.usages/` files and no edits to existing ones. The design's
`.usages/` Update section verified every consumer file current against
this design:

- `kb/.usages/lookups.md`, `kb/.usages/data-pipeline.md` — current.
- `xs/.usages/xs-parsing.md` — current («Call-site lookup» section).
- `rms/.usages/rms-parsing.md` — current («Argument lookup» section).
- `hints/.usages/computing.md` — current (shipped by the apply stage with
  the new cell).
- `server/.usages/lifecycle.md` — current (capability paragraph already
  includes signature help; a pre-existing unclosed code fence in the older
  «Entrypoint» section is out of scope, per user decision in design
  review).

Status: extends-existing = none; creation task reference = N/A.

### External Dependencies

- `go.lsp.dev/protocol` (already vendored in the module) —
  `SignatureHelpOptions`, `SignatureHelp`, `SignatureInformation`,
  `ParameterInformation`; union/optional helpers (`protocol.String`) per
  `lsp-protocol`.
- `github.com/stretchr/testify` — `require`/`assert` (existing test style).
- No new module dependencies. `cmd/kbgen` (outside the cell system) needs
  only the mirrored `range` wire field.

## Facts

- All five CODEMANIFESTs (including the new `hints/CODEMANIFEST`) are
  materialized and reviewed; `goga lint` reports 8 cells / 0 errors.
  **They are read-only for the implementation agent.**
- The `hints/` directory contains only `CODEMANIFEST` and
  `.usages/computing.md` — no Go sources yet; the Go package facade is
  created by the first hints task.
- Corpus facts (live `kb/data/rms-commands.json`): 34 empty-Kind entries
  (flag attributes) all have empty `Desc` → load-time mining fills no Kind
  (SC8); 8 bounded entries all mine kind `number`, ranges `0..2`, `36..480`,
  `0..53`, `0..7` (×2), `1..16`, `0..100`, `0..99`.
- `xs/ast.go` already defines the `eofPos` sentinel (`xs/ast.go:211`) that
  compares after every real position — reuse it as `argEnd` for
  unterminated calls.
- `xs/ast.go` `XsFile` already carries the unexported append-only `symbols`
  index — the new `calls`/`noncode` fields follow the same pattern;
  value-copy safety is established (returned by value).
- `rms/ast.go` `RmsFile` carries the unexported `words` index — the new
  `strings`/`comments`/`excluded` fields follow the same pattern.
- `rms` `StatementAt` fallback (`endsBefore`, strictly earlier line,
  `rms/ast.go:118`) misses same-line trailing positions — that is why
  `ArgAt` gets its own band-based `ownerAt`; **do not modify
  `StatementAt`** (hover frozen, SC8).
- `server.NewServer` currently takes `(store, analyzer)`; exactly two call
  sites exist: `server/serve.go:31` and `server/navigation_test.go:25`.
- `server/server.go` already contains the `shiftPos` helper (include
  template); `unshiftPos` is its exact inverse.
- Test seams available: `indexCommands`/`indexFunctions` (kb, unexported —
  exercised via hand-crafted JSON payloads in `store_test.go`),
  `startHarness` stdio harness + `h.waitDiagnostics` (server,
  `serve_test.go:126`), fixture-driven position computation via
  `strings.Index` (xs/rms navigation tests).
- `docs/ref/zetnus-rms-guide.txt` exists and is already read by
  `TestExtractRmsCommands_RealGuide`.
- **Environment**: `go.mod` requires go ≥ 1.26.6; the sandbox toolchain is
  1.26.4 with toolchain download blocked — `go build`/`go test` cannot run
  locally. All Go validation commands below run in a capable environment
  (CI). The SC8 sweep (`go test ./...`) is the acceptance gate.
- Test-runner safety: `go test ./...` must run under the memory cap
  prescribed by `CLAUDE.md` (a runaway test previously OOM'd the machine).

## Gap Analysis

- **Missing contract entities (kb)**: `ValueRange`, `MineKindRange`,
  `CommandArg.Range` property — `kb/model.go` has none of them;
  `kb/mine.go` does not exist.
- **Missing behavioral change (kb Store)**: `argWire`
  (`kb/store.go:325`) has no `range` field; `indexCommands` performs no
  mining.
- **Missing behavioral change (kb extract)**: `buildCommand`
  (`kb/extract.go:458`) applies no fill-when-empty helper;
  `cmd/kbgen`'s `argJSON` (`cmd/kbgen/main.go:93`) has no `range` field —
  without it, regenerated JSON silently drops mined bounds.
- **Missing contract entities (xs)**: no `CallSite` type, no
  `XsFile.CallAt`, no `calls`/`noncode` parser indices; scanner has
  `skipLineComment`/`skipBlockComment` (`xs/parse.go:1337`, `:1344`) but
  records nothing; `parsePostfix`'s `(`-case (`xs/parse.go:808`) and
  `parseArgs` (`xs/parse.go:931`) create no records.
- **Missing contract entities (rms)**: no `ArgSite` type, no
  `RmsFile.ArgAt`, no `ownerAt`, no `strings`/`comments`/`excluded`
  indices, no `Attribute.nameAt`; `blankComments` (`rms/parse.go:681`)
  blanks but does not emit extents; `statementLine`/`buildAttribute`
  record nothing extra.
- **Missing facade exposure (hints)**: the entire `hints` Go package —
  `computer.go`, `hint.go`, `NewComputer`, `Computer`, `Hint`.
- **API mismatches (server)**: `NewServer(store, analyzer)` lacks
  `computer`; no `SignatureHelp` handler; `Initialize` does not advertise
  `SignatureHelpProvider`; `Serve` does not build the computer; no
  `unshiftPos`/`toSignatureHelp` helpers.
- **Existing code reused as-is**: `eofPos` sentinel, `symbols`/`words`
  index patterns, `kindOf`, `blankComments`, `statementLine`,
  `buildAttribute`, `shiftPos`, `openDocument` language routing,
  `startHarness`, kb test seams, the whole existing test suite (SC8
  baseline).
- **Test coverage gaps**: all 60 design test scenarios below are new; the
  only pre-existing test edited is the `NewServer` call-site helper
  (`server/navigation_test.go:25`, signature change).
- **Workspace visibility**: everything lives in the current repository;
  `hints/` is untracked-new; no git operations are required by the plan
  beyond the project's own commit policy.

---

## Tasks

> **Package ordering rule**: coding tasks for each package are completed before starting the next. Within each coding task, contract tests are written first (TDD workflow).

<!-- ================================================================ -->
<!-- PACKAGE: kb                                                      -->
<!-- ================================================================ -->

### Task 1: kb — `ValueRange` model, `CommandArg.Range`, `MineKindRange` prose parser

This task creates the mined-data foundation of the kb cell: the
`ValueRange` data entity and the `Range` property on `CommandArg` (both in
`kb/model.go`), and the `MineKindRange` Desc-prose parser (new file
`kb/mine.go`). It is pure data + a pure function — no store, no wire
changes (those are Tasks 2–3). Contract entities covered:
`kb.ValueRange` (new), `kb.CommandArg.Range` property (new),
`kb.MineKindRange` (new). Locations: `kb/model.go`, `kb/mine.go`,
tests in `kb/mine_test.go` (new).

**Usages relevant to this task:**
- `conventions`: doc comments on every exported identifier; table-driven
  tests with testify; `Test<Component>_<Scenario>` naming.
- `kbdata` (from kb `Usages`, inline): the «Desc prose mining» block —
  bounds appear as `(<num>-<num>)` right after the kind word
  (`"number (0-99)"`); decimals and negative bounds allowed; unrelated
  `"(default: …)"` / `"(see: …)"` fragments never match; the prose kind
  word is `number` even for percent-typed args; flag attributes carry
  empty Desc.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

Contract semantics (from `kb/CODEMANIFEST`, binding):

`MineKindRange(desc: string) -> kind: string, r: ValueRange`:
1. Find in `desc` the first fragment `(<num>-<num>)` — integer or decimal,
   optional minus; none → return `""`, empty `ValueRange`.
2. The word immediately before the fragment is the mined kind (corpus:
   `number`); no word → `kind=""`.
3. Return kind and `ValueRange` with bounds exactly as written in prose
   (only surrounding-whitespace trimming).

Requirements: prose patterns per `kbdata`; determinism (same input → same
output). Constraints: unparseable prose → empty result, **not an error**;
non-numeric parentheticals (`"(default: …)"`, `"(see: …)"`) never match.

Algorithm (design, verbatim):

```
1. compile once: mineBoundsRe =
   ([A-Za-z_][A-Za-z0-9_]*)? \s* \( (-?\d+(\.\d+)?) - (-?\d+(\.\d+)?) \)
2. m := first submatch in desc
3. IF m == nil:
   - return "", ValueRange{}            # unparseable → empty, not an error
4. return m[1], ValueRange{Min: m[2], Max: m[3]}   # bounds as written
```

Verified corpus checkpoint: 8 bounded entries, all mine kind `"number"`,
ranges `0..2`, `36..480`, `0..53`, `0..7` (×2), `1..16`, `0..100`, `0..99`.

- [ ] **STEP 0 (DECLARATION)**: declare Task 1 (kb model + mining routine) as the task being executed
- [ ] **STEP 1 (CONTRACT TESTS)**: create `kb/mine_test.go` with API-shape contract tests (name the test `TestMineKindRange_APIShape`): `MineKindRange` is exported from `kb` with signature `func MineKindRange(desc string) (string, ValueRange)`; `ValueRange` is exported with fields `Min string`, `Max string` (json tags `min`/`max`); `CommandArg` has field `Range ValueRange` (expected to fail at this stage — none of these exist)
- [ ] **STEP 2 (IMPLEMENTATION)**: in `kb/model.go` add `ValueRange` (doc comment per the manifest: mined value bounds, "" — not mined; requirements: pure data construct-and-use, bounds are strings without precision conversion; `json:"min"` / `json:"max"` tags so one type serves model and wire)
- [ ] **STEP 2 (IMPLEMENTATION)**: in `kb/model.go` add the `Range ValueRange` field to `CommandArg` (doc: mined value bounds; empty — not mined), placed after `Kind`
- [ ] **STEP 2 (IMPLEMENTATION)**: create `kb/mine.go` — package `kb`, the compiled-once `mineBoundsRe` regexp above, and `MineKindRange` implementing the 4-step algorithm verbatim; single leftmost `FindStringSubmatch`; no numeric conversion; never returns an error
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: run the STEP 1 contract tests — `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./kb -count=1 -run TestMineKindRange'` — all must pass
- [ ] **STEP 4 (LOGIC TESTS)**: extend `kb/mine_test.go` with table-driven tests (verbatim scenarios from the design):
  - `TestMineKindRange_BoundedNumber` — `"number (0-99) (default: 12)"` → `("number", {"0","99"})`; pins «only numeric bounds match» against the trailing `(default: …)`
  - `TestMineKindRange_DecimalAndNegativeBounds` — `"float (-1.5-2.5) range"` → `("float", {"-1.5","2.5"})`
  - `TestMineKindRange_NoBounds` — `"plain description without bounds"` → `("", ValueRange{})`
  - `TestMineKindRange_DefaultAndSeeFragmentsIgnored` — `"(default: 5)"`, `"(see: create_elevation)"`, `"number (default: 0 - not elevated)"` (real corpus string) → all `("", ValueRange{})`
  - `TestMineKindRange_EmptyDesc` — `""` → `("", ValueRange{})`, no panic (flag attributes: 34 corpus entries)
  - `TestMineKindRange_BoundsWithoutKindWord` — `"(0-5) picks randomly"` → `("", ValueRange{"0","5"})`
  - `TestMineKindRange_FirstBoundsFragmentWins` — `"number (0-5) or (10-20)"` → `("number", ValueRange{"0","5"})` (leftmost match; a refactor to last-match would flip this)
- [ ] **STEP 5 (DEBUGGING)**: run `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./kb -count=1'` — fix implementation code until all tests pass (do NOT fix test code)
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: verify contract obligations — `ValueRange`/`MineKindRange` importable from `kb`; `CommandArg` field set matches the manifest (Name/Kind/Range/Required/Desc); determinism and no-error constraints hold
- [ ] **STEP 7 (LINT)**: `goimports -w kb` and `golangci-lint run kb/...` (or `gofmt -l kb` when golangci-lint is unavailable) — fix formatting; decompose if necessary
- [ ] **STEP 8 (COMPLETION)**: mark all checkboxes of Task 1 complete

### Task 2: kb — `Store` load-time mining (wire field + fill-when-empty helper)

This task implements the `Store` Requirement added by the contract: at
load time, every `CommandArg` with an empty `Range` or empty `Kind` is
mined via `MineKindRange(Desc)`, fill-empty-only, before map indexing —
so `Store.Command`/`Store.Attribute` serve mined data. Changes are
confined to `kb/store.go` (`argWire` + `indexCommands` + one shared
unexported helper) and tests in `kb/store_test.go`. Contract entity
covered: `kb.Store` (Requirements change). Task 1 must be complete.

**Usages relevant to this task:**
- `conventions`: table-driven tests; reuse the `indexCommands` seam style
  of `TestIndexCommands_InvalidPayload` (`store_test.go:188`) with
  hand-crafted JSON payloads.
- `kbdata`: wire schema — the committed `rms-commands.json` has no
  `range` key; missing keys are not unknown keys, so the committed JSON
  decodes to the zero `ValueRange` and `DisallowUnknownFields` stays
  satisfied. Mined-data provenance rule: structured values from
  extraction win; mining fills only empty fields; the load pipeline never
  fails.

Shared fill-when-empty helper (design, verbatim — one unexported helper
used by BOTH this task and Task 3; extraction↔load idempotency depends on
the two call sites using identical semantics):

```
1. IF arg.Range == zero OR arg.Kind == "":
2.   kind, r := MineKindRange(arg.Desc)
3.   IF arg.Range == zero: arg.Range = r
4.   IF arg.Kind == "":    arg.Kind = kind
```

Applied to every `CommandArg` of every `Command` — in `indexCommands`
(before map indexing into `commandByName`/`commandsBySect`/`attrByKey`)
and (Task 3) in `buildCommand` (after assembly).

Load-path chain (design, verbatim checkpoints):
1. `loadCommands → indexCommands(raw)` → `decodeData`
   (DisallowUnknownFields) into `[]commandWire` — wire type gains
   `Range ValueRange json:"range"` so regenerated JSON decodes; the
   committed JSON (no `range` key) decodes to the zero `ValueRange`.
2. Per wire record → `Command{...}`; per arg/attribute conversion
   `CommandArg(wa)` still compiles because both structs now share the
   identical field set (incl. `Range`).
3. Apply the fill-when-empty helper to every arg and attribute:
   (a) structured values are never overwritten (provenance rule);
   (b) idempotent with extraction (identical helper);
   (c) SC8 — the 34 empty-Kind entries in the corpus all have empty
   `Desc` → no Kind fills (`analysis` percent-gating and rendered
   signatures unchanged).
4. Index into the maps **after** mining so lookups serve mined data.
5. Load errors (unknown keys, duplicate or empty names) unchanged —
   mining itself never errors.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [ ] **STEP 0 (DECLARATION)**: declare Task 2 (kb Store load-time mining) as the task being executed
- [ ] **STEP 1 (CONTRACT TESTS)**: add to `kb/store_test.go` a contract test (name it `TestStore_LoadMines_APIShape`) asserting the served API shape: after `indexCommands` on a payload whose arg has `"desc":"number (0-53)"` and no `range` key, `Store.Command(name)` returns `Args[0].Kind == "number"` and `Args[0].Range == ValueRange{"0","53"}` (expected to fail — no mining exists yet)
- [ ] **STEP 2 (IMPLEMENTATION)**: in `kb/store.go` add `Range ValueRange \`json:"range"\`` to `argWire` (comment: regenerated JSON carries mined bounds; committed JSON decodes to the zero value)
- [ ] **STEP 2 (IMPLEMENTATION)**: add the shared unexported fill-when-empty helper (4-step algorithm verbatim; e.g. `mineCommandArg(arg *CommandArg)`) placed near `indexCommands`; it is the single helper also reused by extraction in Task 3
- [ ] **STEP 2 (IMPLEMENTATION)**: in `indexCommands`, after building each `Command` (args + attributes) and **before** map indexing, apply the helper to every arg and attribute; make sure the `CommandArg(wa)`-style struct conversion path carries `Range` through
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: run the STEP 1 contract test — `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./kb -count=1 -run TestStore_Load'` — must pass
- [ ] **STEP 4 (LOGIC TESTS)**: extend `kb/store_test.go` (verbatim scenarios; `indexCommands` seam, hand-crafted JSON payloads):
  - `TestStore_LoadMinesRangeAndEmptyKind` — arg `{"name":"N","kind":"","required":true,"desc":"number (0-53)"}`, **no** `range` key (committed-JSON shape) → loaded `Kind == "number"`, `Range == ValueRange{"0","53"}`; proves load-time mining works on the committed unmined JSON (the shipping path)
  - `TestStore_LoadKeepsStructuredKindOverMined` — arg `{"name":"%","kind":"percent","required":true,"desc":"number (0-100)"}` → `Kind == "percent"` (NOT overwritten), `Range == ValueRange{"0","100"}`; provenance rule — guards the percent set `analysis` gates on (SC8)
  - `TestStore_FlagAttributesStayNameOnly` — attribute `{"name":"set_scale_by_size","kind":"","required":false,"desc":""}` → after load `Kind == ""`, `Range == ValueRange{}`; the name-only flag form survives (no invented kinds)
- [ ] **STEP 5 (DEBUGGING)**: run `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./kb -count=1'` — fix implementation until green (do NOT fix tests); confirm the pre-existing `TestIndexCommands_InvalidPayload` and `TestNewStore_Success` still pass unchanged (load errors unchanged, real embedded data unaffected)
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: verify — lookups serve mined data (indexing happens after mining); provenance rule (fill-empty-only); SC8 (real corpus: no Kind fills, percent kinds intact); immutable store construction unchanged
- [ ] **STEP 7 (LINT)**: `goimports -w kb` and `golangci-lint run kb/...` — fix formatting
- [ ] **STEP 8 (COMPLETION)**: mark all checkboxes of Task 2 complete

### Task 3: kb — `ExtractRmsCommands` mining pass + `cmd/kbgen` wire mirror

This task adds the mining pass to the build-time extraction pipeline:
`buildCommand` (`kb/extract.go:458`) applies the shared fill-when-empty
helper (from Task 2) to every freshly built `CommandArg`, and
`cmd/kbgen/main.go`'s `argJSON` mirror (`cmd/kbgen/main.go:93`) gains the
`range` field so regenerated JSON does not silently drop mined bounds.
`cmd/` lives outside the cell system — the wire mirror is mandated by the
design under Additional Instructions. Contract entity covered:
`kb.ExtractRmsCommands` (Algorithm step 3 change). Tasks 1–2 must be
complete.

**Usages relevant to this task:**
- `conventions`: extend the existing `TestExtractRmsCommands_RealGuide`
  fixture test rather than adding new fixtures.
- `kbdata`: extraction writes the documented schema; «Mining on
  regeneration» — regenerated JSON and load-time mining must agree
  (idempotency).

Extraction chain (design, verbatim): steps 1–2 unchanged (read file; parse
Syntax Skeleton sections). Step 3 (changed): `buildCommand` assembles
`CommandArg{Name, Kind: kindOf(token), Required, Desc}` per positional
arg and attribute, then applies the same fill-when-empty mining helper →
freshly built args have an empty `Range`, so mining fills `Range` whenever
the Desc has bounds; `Kind` keeps the structured skeleton kind
(`percent` for `%`), falling back to the mined word only when the skeleton
kind is empty → `cliff_curliness %` keeps `Kind="percent"` while gaining
`Range 0..100`. Steps 4–5 unchanged (changelog enrichment; duplicate
check). `cmd/kbgen` marshals via its own `commandJSON/argJSON` mirror,
which **must gain the `range` field** (`kb.ValueRange` with
`json:"range"` tags) or the regenerated JSON silently drops the mined
bounds.

**IMPORTANT**: do NOT regenerate `kb/data/rms-commands.json` in this
change — load-time mining covers the committed data (verified: 34
empty-Kind entries all have empty Desc → no Kind fills; 8 bounded entries
mine at load). Regeneration stays a `cmd/kbgen` operator action; the
idempotency test proves extraction/load agreement without touching the
committed file.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [ ] **STEP 0 (DECLARATION)**: declare Task 3 (kb extraction mining + kbgen mirror) as the task being executed
- [ ] **STEP 1 (CONTRACT TESTS)**: add to `kb/extract_test.go` a contract test (name it `TestExtractRmsCommands_APIShape`) asserting `ExtractRmsCommands("docs/ref/zetnus-rms-guide.txt")` returns `create_elevation.Args[0].Range == ValueRange{"1","16"}` (expected to fail — extraction does not mine yet)
- [ ] **STEP 2 (IMPLEMENTATION)**: in `kb/extract.go` `buildCommand`: after assembling each `CommandArg` (positional args and attributes), apply the shared fill-when-empty helper from Task 2 (the identical unexported function — do not duplicate its logic)
- [ ] **STEP 2 (IMPLEMENTATION)**: in `cmd/kbgen/main.go`: add `Range kb.ValueRange \`json:"range"\`` to `argJSON` and populate it in `argsToJSON` (no dedicated test — covered by the kb idempotency test below)
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: run the STEP 1 contract test — `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./kb -count=1 -run TestExtractRmsCommands'` — must pass
- [ ] **STEP 4 (LOGIC TESTS)**: extend `kb/extract_test.go` (verbatim scenarios):
  - `TestExtractRmsCommands_MinesRange` (extension of `TestExtractRmsCommands_RealGuide`) — real guide fixture: `create_elevation.Args[0].Range == ValueRange{"1","16"}`; `cliff_curliness` arg `%`: `Kind == "percent"`, `Range == ValueRange{"0","100"}` (structured kind + mined range compose)
  - `TestStore_MiningIdempotentWithExtraction` — extract from the real guide; marshal args/attrs to the wire shape (incl. `range`); decode through `indexCommands`; assert for every command, every arg/attribute: loaded `Kind == extracted Kind` and loaded `Range == extracted Range` (the data-pipeline idempotency rule — prevents double-mining drift)
- [ ] **STEP 5 (DEBUGGING)**: run `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./kb ./cmd/... -count=1'` — fix implementation until green (do NOT fix tests); `kb/data/rms-commands.json` must show no diff
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: verify — extraction Algorithm steps 1–2/4–5 untouched; WARN-and-skip constraint intact; `cmd/kbgen` output shape mirrors `argWire` field-for-field (`range` included)
- [ ] **STEP 7 (LINT)**: `goimports -w kb cmd` and `golangci-lint run kb/... cmd/...` — fix formatting
- [ ] **STEP 8 (COMPLETION)**: mark all checkboxes of Task 3 complete

<!-- ================================================================ -->
<!-- PACKAGE: xs                                                      -->
<!-- ================================================================ -->

### Task 4: xs — `CallSite` + `XsFile.CallAt` with parser-recorded call/noncode indices

This task adds call-context navigation to the xs cell: the `CallSite`
data entity and the `XsFile.CallAt(pos)` query method (both in
`xs/ast.go`), backed by parser-side recording in `xs/parse.go` — a
`noncode []common.Range` index (string/comment spans) and per-call
`callRec` records created only in `parsePostfix`'s `(`-case and filled by
`parseArgs`. Contract entities covered: `xs.CallSite` (new),
`xs.XsFile.CallAt` (new method). Locations: `xs/ast.go`, `xs/parse.go`,
tests extend `xs/navigation_test.go`. Tasks 1–3 (kb) must be complete
(package ordering; xs itself depends only on common).

**Usages relevant to this task:**
- `conventions`: positions computed from fixtures via
  `strings.Index`/explicit `common.Pos{Line, Column}` (established
  `xs/navigation_test.go` style).
- `xs_grammar` (`.goga/usages/xs-grammar.md`): XS lexical/structural
  rules — vector literals `(1.0, 2.0, 3.0)`, call syntax `callee(args)`.
  The global annotation binds call-context creation to the grammar's
  expression forms: **only Expr Kind=call creates a call context** —
  vector literals, if/while/for conditions, declaration parameter lists
  and grouping parens are not calls.
- `positions-and-diagnostics` (from common, Imports): `common.Pos`/
  `common.Range` construction; half-open containment via
  `Range.Contains(pos)`.

Query algorithm — `CallAt` (design, verbatim):

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

Parser-side recording (design, verbatim):

- `callRec{callee, calleeAt, lparen, argEnd, commas}` per Expr Kind=call;
  `argEnd` = `)`-token end (closed call) or the **`eofPos` sentinel**
  (`xs/ast.go:211` — compares after every real position) for the
  unterminated recovery frontier, so the half-open `[calleeAt, argEnd)`
  contains a cursor at the exact end of input (SC1 — the canonical state
  after the trigger keystroke, and the norm for inline blocks whose
  `Code` has no trailing newline); `noncode` = string-token + comment
  spans.
- Scanner: the scan loop already skips `//` (`skipLineComment`,
  `xs/parse.go:1337`) and `/* */` (`skipBlockComment`, `:1344`) comments
  and tokenizes strings — extend it to append every comment span and
  every `xString` token range to `noncode` (positions inside
  strings/comments become testable without changing the token stream
  consumed by the parser).
- In `parsePostfix`'s `"("` case (`xs/parse.go:808`): open a
  `callRec{callee: identName(operand), calleeAt: operand.Range.Start,
  lparen: "("-token Start}` and let `parseArgs` (`xs/parse.go:931`)
  record into it: every top-level `,` consumed **by this invocation**
  appends the comma token end position (nested calls record into their
  own records via recursion — their commas are not this call's);
  termination sets `argEnd` = `)`-token `End` or the `eofPos` sentinel
  (`parseArgs` exits only on `)` or EOF, junk tokens skipped in place).
- `XsParse` attaches `calls` and `noncode` to the returned `XsFile`
  (unexported fields, same pattern as the existing `symbols` index);
  value-copy safe (append-only construction, no mutation after return).

Binding checkpoints (design, verbatim):
- the recorded span `[calleeAt, argEnd)` is **not** bounded by
  `Expr.Range` (which stops at the last parsed argument) — contract
  Requirement;
- in-progress calls (`f(`, `f(a,`) produce records — SC1;
- vector literals (`parseParenExpr`), declaration parameter lists
  (`parseParams`) and grouping parens never create records — only Expr
  Kind=call does;
- innermost selection determinism: latest `lparen` among containing
  records — distinct `(` positions, ties impossible;
- postfix chains (`f(1)(…)`) resolve to the suffix call (callee may be
  `""` — the computer silences);
- no clamping — argIndex may exceed any declared parameter count
  (contract Constraint: clamping is a consumer decision).

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [ ] **STEP 0 (DECLARATION)**: declare Task 4 (xs CallSite + CallAt) as the task being executed
- [ ] **STEP 1 (CONTRACT TESTS)**: add to `xs/navigation_test.go` an API-shape contract test (name it `TestXsCallAt_APIShape`): `CallAt` method exists on `XsFile` with signature `func (f XsFile) CallAt(pos common.Pos) (CallSite, bool)`; `CallSite` is exported with fields `Callee string`, `ArgIndex int`, `OnArg bool` (expected to fail — none exist)
- [ ] **STEP 2 (IMPLEMENTATION)**: in `xs/ast.go` add the exported `CallSite` struct (doc comment per manifest: callee name; 0-based argument ordinal valid when `OnArg=true` — right after `(` → 0, after k top-level commas → k; `OnArg` false — on the callee name; requirements: pure data, construct-and-use)
- [ ] **STEP 2 (IMPLEMENTATION)**: in `xs/parse.go` scanner: record comment spans (`skipLineComment`/`skipBlockComment` extents — block comments span from `/*` past `*/` across lines) and `xString` token ranges into the `noncode []common.Range` index
- [ ] **STEP 2 (IMPLEMENTATION)**: in `xs/parse.go` `parsePostfix`'s `"("` case: open the `callRec` (callee from the operand identifier, `calleeAt` = operand start, `lparen` = `(`-token start); have `parseArgs` append each top-level comma's token end to the active record and set `argEnd` = `)`-token end or the `eofPos` sentinel for unterminated lists; only this code path creates records (vector literals / param lists / grouping parens never do)
- [ ] **STEP 2 (IMPLEMENTATION)**: attach `calls []callRec` and `noncode []common.Range` to `XsFile` in `XsParse` (unexported, append-only, `symbols`-index pattern); implement `XsFile.CallAt` in `xs/ast.go` following the 6-step query algorithm verbatim (noncode check first; latest-lparen selection; `!pos.After(lparen)` → callee-side `{callee, 0, false}`; comma-end `<= pos` counting)
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: run the STEP 1 contract test — `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./xs -count=1 -run TestXsCallAt_APIShape'` — must pass
- [ ] **STEP 4 (LOGIC TESTS)**: extend `xs/navigation_test.go` with the design scenarios verbatim (positive): `TestCallAt_ClosedCallArgIndex` (`"void f() { g(a, b); }"`, pos on `a` → `{g,0,true}`, pos on `b` → `{g,1,true}`); `TestCallAt_JustAfterOpenParen` (SC1: `"void f() { g("`, pos right after `(` → `{g,0,true}` — the exact state an editor sends after the trigger keystroke); `TestCallAt_OnCalleeName` (pos on `g` → `{g,0,false}`); `TestCallAt_NestedInnerWins` (`"void f() { h(g(x)); }"`, pos on `x` → `{g,0,true}`)
- [ ] **STEP 4 (LOGIC TESTS)**: (edge) `TestCallAt_UnclosedToEOF` (`"void f() { g(h("`, pos = exact EOF `{0,15}` → `{h,0,true}` — doubly-nested in-progress calls at the eofPos frontier); `TestCallAt_PositionOnOpenParenChar` (pos exactly on `(` → `{g,0,false}` — boundary between steps 4/5, guards the `pos.After` off-by-one); `TestCallAt_PositionOnCommaChar` (pos on `,` → `{ArgIndex:0,OnArg:true}` — the comma's end is `pos+1`, previous-argument region); `TestCallAt_ArgIndexNeverClamped` (`"void f() { g(a, b, c"` after second comma → `ArgIndex == 2`); `TestCallAt_Deterministic` (same pos queried twice → equal results)
- [ ] **STEP 4 (LOGIC TESTS)**: (negative) `TestCallAt_InString` (`'void f() { g("ab|c"); }'` pos inside the string → found=false, checked before any call lookup); `TestCallAt_InComment` (`"// g(a, b)\nvoid f() { g(a, b); }"` pos in line comment → found=false); `TestCallAt_InBlockComment` (`"void f() { /* open\nstill comment */ g(a); }"` pos on `still` (line 1) → found=false — multi-line span from `/*` past `*/`); `TestCallAt_VectorLiteralNotCall` (`"void f() { vector v = (1, 2, 3); }"` pos on `3` → found=false); `TestCallAt_ParamListNotCall` (`"int f(int a, int b) { return 0; }"` pos on `b` in the param list → found=false)
- [ ] **STEP 5 (DEBUGGING)**: run `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./xs -count=1'` — fix implementation until green (do NOT fix tests); the existing suite (SymbolAt/Definition/References/Symbols) must pass unchanged (SC8: token stream and AST untouched)
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: verify — span semantics record-based (not `Expr.Range`-based); innermost determinism; never-clamp; in-progress calls produce records (SC1); only Expr Kind=call creates contexts (global annotation)
- [ ] **STEP 7 (LINT)**: `goimports -w xs` and `golangci-lint run xs/...` — fix formatting; decompose if necessary
- [ ] **STEP 8 (COMPLETION)**: mark all checkboxes of Task 4 complete

<!-- ================================================================ -->
<!-- PACKAGE: rms                                                     -->
<!-- ================================================================ -->

### Task 5: rms — `ArgSite` + `RmsFile.ArgAt` with band owner and recorded spans

This task adds argument-context navigation to the rms cell: the `ArgSite`
data entity and the `RmsFile.ArgAt(pos)` query method (both in
`rms/ast.go`), backed by parser-side recording in `rms/parse.go` —
`RmsFile.strings` (statement-line string tokens), `RmsFile.comments`
(`blankComments` extents), `RmsFile.excluded` (section-header lines,
`#include`/`#includeXS` directive lines incl. path arguments, plain
`#`-comment lines), and `Attribute.nameAt` (name token range). The owner
lookup is a dedicated band-based `ownerAt` — **`StatementAt` is NOT
modified** (hover behavior frozen, SC8). Contract entities covered:
`rms.ArgSite` (new), `rms.RmsFile.ArgAt` (new method). Locations:
`rms/ast.go`, `rms/parse.go`, tests extend `rms/navigation_test.go`.
Task 4 (xs) must be complete (package ordering).

**Usages relevant to this task:**
- `conventions`: fixture positions via `strings.Index`/explicit
  `common.Pos` (`rms/navigation_test.go` style).
- `rms_grammar` (`.goga/usages/rms-grammar.md`): RMS line-oriented
  structure, directives, positional attribute semantics (attrs attach to
  the preceding command; blocks nest) — the owner/discrimination rules.
- `positions-and-diagnostics` (from common, Imports): `Range.Contains`,
  span construction.

Resolved design decisions binding this task (design «Applied Fixes»
§3–§4, verbatim essentials):
1. `ArgAt` step 1 «семантика StatementAt, включая хвостовые позиции» is
   implemented as a dedicated band-based owner lookup (`ownerAt`), **not**
   a call to `StatementAt`: the existing fallback (`endsBefore`, strictly
   earlier line, `rms/ast.go:118`) misses same-line trailing positions
   (cursor past the last argument — the key signature-help moment) and can
   return an *earlier* statement. `ownerAt` fixes this without touching
   `StatementAt`.
2. Directive-styled statements (`#const`, `#define`, `#include_drs` —
   materialized as command statements) are filtered inside `ArgAt`
   (`found=false` when the owner name starts with `#`). The `#`-prefix
   filter alone is not sufficient: `#include`/`#includeXS` lines and
   section headers materialize **no** statement, so under band semantics
   a preceding command would own them — the `excluded` span index makes
   these positions answer `found=false` categorically.

Query algorithm — `ArgAt` (design, verbatim):

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

Band-owner checkpoints (design, verbatim):
- containing positions behave exactly like `StatementAt`'s deepest-match
  (positional containment implies the band picks the same statement);
- same-line trailing positions are owned by their own statement, not an
  earlier one;
- positions in gaps/blank lines fall to the preceding statement
  («предшествующий» semantics);
- unclosed blocks own their trailing region (children simply run out);
- section headers / `#include`/`#includeXS` directive lines / plain
  `#`-comment lines are excluded spans → `found=false` **even when a
  preceding command exists** (the band alone would pick that command as
  owner — the excluded index is what honors the contract's categorical
  exclusion list and `rms-parsing`).

Parser-side recording (design, verbatim):
- `blankComments` (`rms/parse.go:681`) already walks every character and
  knows the exact `//` and `/* */` extents — extend it to emit those
  extents as `common.Range` spans (multi-line block comments span across
  the recorded lines; `p.starts` supplies offsets) → `RmsFile.comments`.
- `statementLine` (`rms/parse.go:145`) holds the full token list of each
  statement/structural line (`rest`); record every `tokString` token
  range into `RmsFile.strings` (quoted include-path strings are covered
  by the excluded-band index — the whole directive line is excluded, so
  they need no separate string entry).
- Record `excluded []common.Range` in `run()` (`rms/parse.go:73`) for the
  line classes that never own a statement — section-header lines
  (`<NAME>` / `</NAME>`), `#include`/`#includeXS` directive lines
  (including their path arguments, quoted or bare), and plain
  `#`-comment lines (`rms_grammar` comments; `#const`/`#define`/
  `#include_drs` are NOT excluded — they materialize statements and are
  handled by the `#`-prefix owner filter); spans follow the existing
  `words` index pattern.
- `buildAttribute` (`rms/parse.go:282`) records the attribute name token
  range into an unexported `nameAt common.Range` field on `Attribute`
  (set at construction, preserved by the value-copying `materialize`) —
  precise name-or-value discrimination for kind=attr without word-index
  heuristics; flag attributes have a zero `Value` whose empty range never
  contains any position.
- Kind constants exported: `KindArg = "arg"`, `KindAttr = "attr"`,
  `KindNone = "none"` (manifest `Kind*` convention).

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [ ] **STEP 0 (DECLARATION)**: declare Task 5 (rms ArgSite + ArgAt) as the task being executed
- [ ] **STEP 1 (CONTRACT TESTS)**: add to `rms/navigation_test.go` an API-shape contract test (name it `TestRmsArgAt_APIShape`): `ArgAt` method exists on `RmsFile` with signature `func (f RmsFile) ArgAt(pos common.Pos) (ArgSite, bool)`; `ArgSite` exported with fields `Stmt Statement`, `Kind string`, `Index int`, `Name string`; constants `KindArg`/`KindAttr`/`KindNone` exported (expected to fail — none exist)
- [ ] **STEP 2 (IMPLEMENTATION)**: in `rms/ast.go` add exported `ArgSite` (doc comment per manifest: owner command — positional semantics, attrs belong to the preceding command, nested blocks to their own statement; kind arg/attr/none; index 0-based positional ordinal; name — attribute name; requirements: pure data, kind-discriminator in the `Statement.Kind`/`Expr.Kind` style) plus the three `Kind*` constants
- [ ] **STEP 2 (IMPLEMENTATION)**: in `rms/parse.go`: extend `blankComments` to emit comment extents → `RmsFile.comments`; record `tokString` token ranges from statement lines → `RmsFile.strings`; record `excluded` spans in `run()` for section-header / `#include`/`#includeXS` directive (incl. path args) / plain `#`-comment lines; record `Attribute.nameAt` in `buildAttribute`; attach the unexported indices to the returned `RmsFile` (`words`-index pattern, append-only)
- [ ] **STEP 2 (IMPLEMENTATION)**: in `rms/ast.go` implement the unexported `ownerAt(pos)` (band model verbatim: last statement with `Range.Start <= pos`, recurse into children, innermost wins; walk sections top-down incl. the synthetic `global`) and `RmsFile.ArgAt` following the 6-step query algorithm verbatim (string/comment/excluded check first; `#`-prefix owner filter; `Args[i].Range` containment; `nameAt`/`Value.Range` attribute discrimination; `KindNone` fallback)
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: run the STEP 1 contract test — `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./rms -count=1 -run TestRmsArgAt_APIShape'` — must pass
- [ ] **STEP 4 (LOGIC TESTS)**: extend `rms/navigation_test.go` with the design scenarios verbatim (positive): `TestArgAt_OnPositionalArg` (`"create_elevator PLAYER_1 5"`, pos on `PLAYER_1` → found, `KindArg`, `Index 0`, `Stmt.Name == "create_elevator"`); `TestArgAt_OnAttributeValueAndName` (`"create_elevator A {\n  number_of_objects 5\n}\n"` — pos on `5` and on `number_of_objects` → both `KindAttr`, `Name == "number_of_objects"`); `TestArgAt_TrailingSameLine` (`"create_elevation 7 "` pos past line end → found, `KindNone`, owner `create_elevation` — NOT a preceding statement; the band-owner fix, the primary RMS hint moment)
- [ ] **STEP 4 (LOGIC TESTS)**: (edge) `TestArgAt_BraceAndGapPositions` (`"create_elevation 3 {\n  spacing 5\n}\n"` — pos on `{`, on `}`, on the inter-token space → all found, `KindNone`, owner `create_elevation`); `TestArgAt_NonStatementLinesAfterCommand` (`"create_elevation 3\n<LAND_GENERATION>\n#include \"other.rms\"\n# a plain comment\n"` — pos on `3` → found/`KindArg`/`Index 0`; pos on the header line, inside the quoted include path, and on the `#`-comment line → all `found=false` — NOT `create_elevation` with kind=none; pins the categorical exclusion in the only configuration that exposes it); `TestArgAt_NestedBlockInnermost` (nested `start_random`/`percent_chance` fixture, pos on `3` → owner `create_elevation`); `TestArgAt_Deterministic` (same pos twice → equal results)
- [ ] **STEP 4 (LOGIC TESTS)**: (negative) `TestArgAt_OnDirectiveAndSectionHeader` (`"#include other.rms\n<LAND_GENERATION>\n#const TERRAIN 7\ncreate_elevation 3\n"` — pos on the include path, on the header, on `7` in `#const` → all `found=false`; `#const` via the `#`-prefix owner filter); `TestArgAt_InComment` (`"create_elevation 3 /* hill */"` pos on `hill` → found=false); `TestArgAt_InString` (`"create_object GOLF_BALL {\n  object_name \"grass|land\"\n}\n"` pos on `land` → found=false)
- [ ] **STEP 5 (DEBUGGING)**: run `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./rms -count=1'` — fix implementation until green (do NOT fix tests); the existing suite (StatementAt/Symbols/References) must pass **unmodified** — `StatementAt` itself is untouched (SC8)
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: verify — owner semantics incl. same-line trailing/unclosed blocks (band model); string/comment/excluded exclusion checked before owner lookup; none-on-ambiguity (exact token-range membership only, SC6); determinism; `Kind*` constants exported
- [ ] **STEP 7 (LINT)**: `goimports -w rms` and `golangci-lint run rms/...` — fix formatting; decompose if necessary
- [ ] **STEP 8 (COMPLETION)**: mark all checkboxes of Task 5 complete

<!-- ================================================================ -->
<!-- PACKAGE: hints                                                   -->
<!-- ================================================================ -->

### Task 6: hints — package bootstrap + `Hint` entity

This task creates the `hints` Go package (the new cell's facade — a Go
package's exported identifiers are the public API) and its first contract
entity: `Hint` in `hints/hint.go` — the protocol-agnostic render result
for one call site. The cell directory already contains `CODEMANIFEST` and
`.usages/computing.md` (read-only artifacts shipped by the apply stage).
Contract entity covered: `hints.Hint` (new). Location: `hints/hint.go`
(new); tests in `hints/computer_test.go` (new — the design's registry
puts all hints tests there). Task 5 (rms) must be complete (package
ordering).

**Usages relevant to this task:**
- `conventions`: doc comments on exported identifiers; table-driven
  testify tests.
- `positions-and-diagnostics` (from common, Imports): referenced by the
  manifest header; supplies the `common.Pos` vocabulary used by the
  entity's consumers (Task 7 methods).

`Hint` semantics (from `hints/CODEMANIFEST`, binding):
- `label`: full one-line signature, e.g.
  `vector xsVectorSet(float x, float y, float z)`,
  `percent_chance(%: percent 0..99)`.
- `params`: parameter labels in list order; optional ones bracketed —
  `[float z]`, `[MaxHeight: number 1..16]`, `[set_circular_base]`
  (**Type-first** for XS kb params — the design review resolved the
  Name-colon-Type form as a typo; RMS labels `Name: Kind Min..Max` are
  unaffected).
- `active`: index of the active parameter in `params`; −1 — none.
- Requirements: pure data (construct-and-use, no mutation); mapping to
  `protocol.SignatureInformation` is the server's responsibility.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [ ] **STEP 0 (DECLARATION)**: declare Task 6 (hints package + Hint entity) as the task being executed
- [ ] **STEP 1 (CONTRACT TESTS)**: create `hints/computer_test.go` with an API-shape contract test (name it `TestHint_APIShape`): package `hints` compiles and `Hint` is exported with fields `Label string`, `Params []string`, `Active int` (expected to fail — the package has no Go sources)
- [ ] **STEP 2 (IMPLEMENTATION)**: create `hints/hint.go` — `package hints`, the `Hint` struct with the three exported fields and doc comments per the manifest (full one-line signature; parameter labels in order, optional bracketed; active index, −1 — none; requirements: pure data, protocol mapping belongs to server)
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: run the STEP 1 contract test — `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./hints -count=1 -run TestHint'` — must pass
- [ ] **STEP 4 (LOGIC TESTS)**: extend `hints/computer_test.go` with `TestHint_ConstructAndUse` — construct `Hint{Label: "percent_chance(%: percent 0..99)", Params: []string{"%: percent 0..99"}, Active: 0}` and assert field round-trip (data entity: construction is the behavior)
- [ ] **STEP 5 (DEBUGGING)**: run `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./hints -count=1'` — fix implementation until green (do NOT fix tests)
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: verify — `Hint` importable from `hints` (facade = package); field set/types match the manifest properties `Label`/`Params`/`Active`; no methods added (pure data)
- [ ] **STEP 7 (LINT)**: `goimports -w hints` and `golangci-lint run hints/...` (or `gofmt -l hints`) — fix formatting
- [ ] **STEP 8 (COMPLETION)**: mark all checkboxes of Task 6 complete

### Task 7: hints — `Computer` with `XsAt` / `RmsAt` (truth model, conflict rule, rendering)

This task implements the heart of the new cell: `Computer` in
`hints/computer.go` — factory `NewComputer(store *kb.Store)` (constructor
DI per `conventions`), stateless, with the two query methods. Contract
entity covered: `hints.Computer` (new). Location: `hints/computer.go`
(new); tests extend `hints/computer_test.go`. Task 6 must be complete;
consumes kb (`Store.Function`/`Store.Command` via `lookups`), xs
(`XsFile.CallAt` via `xs-parsing`), rms (`RmsFile.ArgAt` via
`rms-parsing`).

**Usages relevant to this task:**
- `conventions`: constructor DI; table-driven tests; hints tests build a
  real `kb.NewStore()` (embedded data) plus hand-built
  `xs.XsFile`/`rms.RmsFile` from real parses.
- `lookups` (from kb, Imports — `kb/.usages/lookups.md`): `NewStore`,
  exact-name lookups `Function`/`Command`, `(value, found)` pairs,
  silence on `found=false`; the mined kind/range rendering contract
  (consume `CommandArg.Range`/`Kind` as served).
- `xs-parsing` (from xs, Imports): `XsParse`, `CallAt` consumer
  semantics — in-progress calls, innermost wins, ArgIndex may exceed
  params.
- `rms-parsing` (from rms, Imports): `Parse`, `ArgAt` consumer
  semantics — owner resolution, kind vocabulary, ambiguity → none.
- `positions-and-diagnostics` (from common, Imports): `common.Pos`
  parameters.

`Computer.XsAt` algorithm (design, verbatim):

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
Resolved ambiguity (design «Applied Fixes» §1, binding): when
equal-parameter duplicates differ in return type, the **first
declaration in pool order** (document `Decls` in source order, then
`external` in closure DFS order) supplies the label — pool order is the
deterministic tie-break and matches the «документ и замыкание — один
пул» truth model.

`Computer.RmsAt` algorithm (design, verbatim):

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

Binding checkpoints (design): rendering examples reproduce exactly from
live kb data — `vector xsVectorSet(float x, float y, float z)`,
`bool xsCreateFile([bool append])`, `percent_chance(%: percent 0..99)`,
`create_elevation([MaxHeight: number 1..16], …, [set_scale_by_size], …)`;
types/names/kinds/defaults are never invented; `Hint` carries no
documentation (concise-hints rule — extended descriptions stay in
hover); errors: none — silence is the failure mode.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [ ] **STEP 0 (DECLARATION)**: declare Task 7 (hints Computer) as the task being executed
- [ ] **STEP 1 (CONTRACT TESTS)**: add to `hints/computer_test.go` an API-shape contract test (name it `TestComputer_APIShape`): `NewComputer(store *kb.Store) *Computer` exists; methods `func (c *Computer) XsAt(file xs.XsFile, pos common.Pos, external []xs.Decl) (Hint, bool)` and `func (c *Computer) RmsAt(file rms.RmsFile, pos common.Pos) (Hint, bool)` exist (expected to fail — `Computer` does not exist)
- [ ] **STEP 2 (IMPLEMENTATION)**: create `hints/computer.go` — `package hints`; `Computer` struct holding only the immutable `*kb.Store`; `NewComputer` constructor (DI); implement `XsAt` following the 6-step algorithm verbatim (pool assembly document-then-external; `paramsEqual` conflict check; kb fallback; label rendering with `ret` omitted when empty, `"<Type> <Name>"` falling back to `"<Name>"`, kb optional params bracketed `[…]`, source params never bracketed; active mapping never clamped)
- [ ] **STEP 2 (IMPLEMENTATION)**: implement `RmsAt` following the 6-step algorithm verbatim (kb-ordered full list Args then Attributes, no truncation; `"Name"` when `Kind == ""` else `"Name: Kind"` + `" Min..Max"` when `Range` non-empty; `Required=false` wraps the whole label in `[…]`; active mapping arg → pass-through index, attr → `len(Args)` + attribute position, absent → −1, none → −1)
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: run the STEP 1 contract test — `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./hints -count=1 -run TestComputer'` — must pass
- [ ] **STEP 4 (LOGIC TESTS)**: extend `hints/computer_test.go` with the design scenarios verbatim. XsAt positive/edge: `TestXsAt_KbFunction` (real store; `"void r() { xsVectorSet(1.0, 2.0, 3.0); }"` pos on `2.0` → `Label == "vector xsVectorSet(float x, float y, float z)"`, `Params == ["float x","float y","float z"]`, `Active == 1` — the contract's canonical XS example verbatim); `TestXsAt_SourceDeclWinsOverKb` (document declares `int xsVectorSet(int q)`, unclosed call, cursor on argument, external=nil → `Label == "int xsVectorSet(int q)"`, `Active == 0`; kb NOT consulted — also proves in-progress calls feed the computer, SC1 end-to-end at hints level); `TestXsAt_ClosureDeclPool` (external `[]xs.Decl{{Kind:"function",Name:"helper",Params:[a int, b int]}}`, pos after comma → `Label == "helper(int a, int b)"` (no ret — Type empty), `Active == 1`); `TestXsAt_OptionalKbParamBracketed` (crafted `indexFunctions` seam — params `x` required, `z` optional, or the real `xsCreateFile`: `bool xsCreateFile([bool append])`; assert `Params` contains `"[float z]"` exact bracket form / `Label` ends `"(int x, [float z])"`)
- [ ] **STEP 4 (LOGIC TESTS)**: XsAt negative/edge: `TestXsAt_UnknownFunction` (`nosuchfn(` → found=false — silence for unknown names, trust rule); `TestXsAt_ConflictingSourceDecls` (document `void h(int a)` + external `void h(float b)` → found=false — a wrong-signature hint is worse than none); `TestXsAt_NoCallContext` (`"int q = 1;\nvoid r() { int w = q; }"` pos on `q` → found=false — identifiers route to hover/completion); `TestXsAt_EqualParamsMerged` (document `int h(int a)` + external identical → pool of 2, sequences equal → merged, render from pool-first: `Label == "int h(int a)"`, `Active == 0` — «единая декларация» + deterministic pool order); `TestXsAt_ActiveNoneOnCallee` (real store, `"void r() { xsVectorSet("` pos on callee name → found, `Active == -1`); `TestXsAt_ArgIndexBeyondParams` (crafted 1-param function, call `fn(a, b|` → `Active == -1` — never clamp at hints level)
- [ ] **STEP 4 (LOGIC TESTS)**: RmsAt positive: `TestRmsAt_FullListArgsThenAttrs` (real store; `"create_elevation 3"` pos on `3` → `Params` has `1 + 8 == 9` entries (full list, no truncation), `Params[0] == "[MaxHeight: number 1..16]"`, `Params[5] == "[set_scale_by_size]"`, `Active == 0`); `TestRmsAt_PercentChanceMinedRange` (`"percent_chance 45"` pos on `45` → `Label == "percent_chance(%: percent 0..99)"`, `Params == ["%: percent 0..99"]`, `Active == 0` — structured kind + mined range compose; the contract's canonical RMS example verbatim); `TestRmsAt_ActiveOnAttribute` (`"create_elevation 3 {\n  spacing 5\n}\n"` pos on `5` → `Active == 7` (`len(Args)=1` + attribute position 6), `Params[7] == "[spacing: number]"`)
- [ ] **STEP 4 (LOGIC TESTS)**: RmsAt negative/edge: `TestRmsAt_UnknownCommand` (`"create_elefant 5"` typo → found=false — RMS hints are kb-gated, no invented signatures); `TestRmsAt_ActiveAttrNotInKb` (document attribute `number_of_objectz` absent from kb → `Active == -1`, full kb list still renders); `TestRmsAt_ActiveIndexPassesThrough` (`"create_elevation 1 2 3"` pos on `3` → `Active == 2` — beyond the single declared arg's slot 0, RMS never-clamp)
- [ ] **STEP 5 (DEBUGGING)**: run `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./hints -count=1'` — fix implementation until green (do NOT fix tests)
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: verify — stateless (`Computer` holds only the immutable store); truth model (source > kb, document+closure one pool); conflict rule (any parameter mismatch → silence); rendering parity with the contract examples above; `Hint` carries no documentation; active never clamped; XS↔RMS hint quality parity (SC5)
- [ ] **STEP 7 (LINT)**: `goimports -w hints` and `golangci-lint run hints/...` — fix formatting; decompose if necessary
- [ ] **STEP 8 (COMPLETION)**: mark all checkboxes of Task 7 complete

<!-- ================================================================ -->
<!-- PACKAGE: server                                                  -->
<!-- ================================================================ -->

### Task 8: server — `computer` DI, `SignatureHelp` handler, capability delta, `Serve` wiring

This task wires the feature into the protocol surface: `Server` gains the
third constructor dependency `computer *hints.Computer` and the
`SignatureHelp` handler (`server/server.go`); `Initialize` advertises
`SignatureHelpProvider` with triggers `(` and `,`; `Serve`
(`server/serve.go`) builds `NewComputer(store)`; **both `NewServer` call
sites** (`server/serve.go:31` and the test helper at
`server/navigation_test.go:25`) are updated in the same change. Contract
entities covered: `server.Server` (constructor + `Initialize` +
`SignatureHelp`), `server.Serve` (wiring delta). Locations:
`server/server.go`, `server/serve.go`, `server/navigation_test.go`
(helper signature only). Task 7 must be complete.

**Usages relevant to this task:**
- `conventions`: constructor DI (`NewServer(store, analyzer, computer)`);
  handler tests via the existing `startHarness` stdio harness
  (`serve_test.go:126`): `h.disp.Initialize → h.disp.DidOpen →
  h.waitDiagnostics → h.disp.SignatureHelp`.
- `computing` (from hints, Imports — `hints/.usages/computing.md`):
  `NewComputer`, `XsAt`/`RmsAt` call patterns, silence mapping
  (`found=false → nil, nil` — the Hover convention), protocol-mapping
  boundary (`Hint` → `protocol.SignatureInformation` belongs to server);
  one `Computer` per process, injected into `NewServer`.
- `xs-parsing` / `rms-parsing` (from xs/rms, Imports): parse-then-query
  per request; block-local coordinate translation for inline blocks.
- `lsp-protocol` (`.goga/usages/cooks/lsp-protocol.md`, Signature Help
  section — binding reference): advertise
  `SignatureHelpProvider: &protocol.SignatureHelpOptions{TriggerCharacters: []string{"(", ","}}`;
  result = exactly one `SignatureInformation` (`Label = hint.Label`;
  `Parameters` = one `ParameterInformation` per `hint.Params` entry with
  a plain-string label — if `ParameterInformation.Label` is one of the
  library's union fields, wrap with `protocol.String` exactly as
  `server.go` does for `CompletionItem.Documentation`; `Documentation`
  omitted); `ActiveSignature = &[]uint32{0}[0]`; `ActiveParameter` =
  `*uint32(active)` when `active >= 0`, else `nil` (never clamp — nil,
  not last-parameter); nullable result `nil, nil` for silence;
  statelessness (TriggerKind/TriggerCharacter/IsRetrigger ignored).
- `closure` (from include, Imports — unchanged): `resolver.Closure(ctx,
  uri)` consumed exactly as `analyzeXs`/`analyzeRms` do — the analyzed
  file is excluded from its own externals; a partially unavailable
  closure simply yields fewer externals (C6).

`SignatureHelp` algorithm (design, verbatim):

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
`Column −= base.Column` only when `p.Line == base.Line` — the exact
inverse of the existing `shiftPos` (same math as `include.resolver`'s
helper; document-coordinate answers need no re-shifting — the client
position is echoed, only lookup coordinates are block-local). Inline
blocks are not closure members → `ExternalDecls("")` (same argument as
`analyzeRms`). Non-`.rms`/`.xs` URIs fall through both switch arms →
silence. `Serve` wiring: `computer := hints.NewComputer(store)` between
`NewAnalyzer` and `NewServer`; `DocStore` and `Resolver` remain internal
to `NewServer`.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [ ] **STEP 0 (DECLARATION)**: declare Task 8 (server SignatureHelp + wiring) as the task being executed
- [ ] **STEP 1 (CONTRACT TESTS)**: add to `server/serve_test.go` a contract test (name it `TestServe_SignatureHelpAPIShape`) asserting the API shape: `NewServer` accepts three arguments `(store, analyzer, computer)`; `Server` implements `SignatureHelp(ctx context.Context, params *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error)` (expected to fail — the method and third parameter do not exist); update the `navigation_test.go:25` helper call site in the same edit
- [ ] **STEP 2 (IMPLEMENTATION)**: in `server/server.go`: add the `computer *hints.Computer` field; change `NewServer(store *kb.Store, analyzer *analysis.Analyzer, computer *hints.Computer)` (doc comment: computer — the signature-help hint engine, DI); implement the unexported `unshiftPos` helper (exact `shiftPos` inverse, per the math above)
- [ ] **STEP 2 (IMPLEMENTATION)**: implement `Server.SignatureHelp` following the 5-step algorithm verbatim (language routing shared with Hover; `.xs` closure path excluding the analyzed file; `.rms` innermost-`XsBlock` path with `unshiftPos` + `ExternalDecls("")`, else `RmsAt`; silence `nil, nil`; mapping via a `toSignatureHelp(hint)` helper per `lsp-protocol` — one signature, `ActiveSignature` 0-pointer, `ActiveParameter` `*uint32` or nil, plain-string parameter labels wrapped with `protocol.String` if the field is a union, documentation omitted)
- [ ] **STEP 2 (IMPLEMENTATION)**: in `Initialize` add `SignatureHelpProvider: &protocol.SignatureHelpOptions{TriggerCharacters: []string{"(", ","}}` — triggers must not intersect completion's ` ` and `<` (RMS value keystrokes must not spam the widget); existing capabilities and negotiation unchanged
- [ ] **STEP 2 (IMPLEMENTATION)**: in `server/serve.go` insert `computer := hints.NewComputer(store)` after `NewAnalyzer` and pass it to `NewServer(store, analyzer, computer)`; update the `server/navigation_test.go:25` helper identically (no other callers exist)
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: run the STEP 1 contract test — `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./server -count=1 -run TestServe_SignatureHelpAPIShape'` — must pass
- [ ] **STEP 4 (LOGIC TESTS)**: extend `server/serve_test.go` with the design scenarios verbatim: `TestServe_InitializeAdvertisesSignatureHelp` (via `startHarness` + `Initialize(&protocol.InitializeParams{})` — provider non-nil; trigger characters exactly `["(", ","]` order included; existing capabilities unchanged: Hover/Completion/Definition/References/DocumentSymbol, Full+OpenClose sync); `TestServe_SignatureHelpSilence` (open a `.rms` document `"\n\n"`, request at `{0,0}` → `help == nil && err == nil` — the nil,nil silence convention through the protocol layer, nullable result not an error not an empty SignatureHelp)
- [ ] **STEP 5 (DEBUGGING)**: run `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./server -count=1'` — fix implementation until green (do NOT fix tests); the only edited existing test is the `NewServer` call-site helper
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: verify — constructor DI per `conventions`; handler stateless/read-only (R10) and additive (C4/SC8: existing handlers untouched); trigger set disjoint from completion's; silence convention identical to Hover; capability negotiation and `ServerInfo` unchanged
- [ ] **STEP 7 (LINT)**: `goimports -w server` and `golangci-lint run server/...` — fix formatting; decompose if necessary
- [ ] **STEP 8 (COMPLETION)**: mark all checkboxes of Task 8 complete

### Task 9: Integration tests — signature help full stack (XS, RMS, inline block, statelessness, SC8 sweep)

Cross-entity integration: the complete request path LSP client →
`Server.SignatureHelp` → parse → closure → `Computer` → protocol result,
through the real stdio harness (the DI wiring built by `Serve` is what
the harness exercises). This task adds no production code — only tests
in `server/serve_test.go`. Tasks 1–8 must all be complete.

**Usages relevant to this task:**
- `conventions`: `startHarness` stdio harness pattern —
  `h.disp.Initialize → h.disp.DidOpen → h.waitDiagnostics →
  h.disp.SignatureHelp`.
- `lsp-protocol`: result-shape assertions (single signature,
  `ActiveSignature` 0, `ActiveParameter` nil-when-none) and the
  statelessness rule (identical answers for manual and auto requests).
- `computing` (from hints): end-to-end silence and protocol-boundary
  preconditions.
- `xs-parsing` / `rms-parsing`: block-local coordinate translation for
  inline blocks — the only place it is exercised end-to-end.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [ ] Add to `server/serve_test.go` the design's end-to-end scenarios verbatim:
  - `TestServe_SignatureHelpXs` — open `file:///work/script.xs` with `"void test() {\n\txsVectorSet(1.0, 2.0, 3.0);\n}\n"`, wait diagnostics, request at the position of `2.0` → result non-nil; `len(Signatures) == 1`; `*ActiveSignature == 0`; `*ActiveParameter == 1`; `Signatures[0].Label == "vector xsVectorSet(float x, float y, float z)"`; `len(Parameters) == 3` — the full XS stack through the real harness including DI wiring
  - `TestServe_SignatureHelpRms` — open `file:///work/map.rms` with `"<LAND_GENERATION>\ncreate_elevation 3\n"`, request at the position of `3` → one signature; label starts with `"create_elevation("`; `*ActiveParameter == 0`; params include `"[MaxHeight: number 1..16]"` — the RMS path end-to-end
  - `TestServe_SignatureHelpInlineXsBlock` — open `file:///work/map.rms` with `"#includeXS\nvoid b() { xsVectorSet(1.0, 2 }\n"` (call left unclosed), request at the position of `2` (line 1) → one signature; label `"vector xsVectorSet(float x, float y, float z)"`; `*ActiveParameter == 1` — computed from the **translated** position (a wrong translation yields silence or a wrong index, so this pins the shift math; the inline-block coordinate template)
  - `TestServe_SignatureHelpStateless` — same document/position as the XS test; two requests, first with `Context: {TriggerKind: Invoked}`, then `{TriggerKind: TriggerCharacter, TriggerCharacter: "(", IsRetrigger: true}` → the two results deep-equal (label, params, active) — the answer depends only on (document, position)
  - `TestServe_SignatureHelpRmsNoActiveParameter` — the RMS fixture above, request at the position of the command name `create_elevation` (line 1) → one signature; `Signatures[0].ActiveParameter == nil` (not 0, not last); label starts with `"create_elevation("` — the RMS `kind=none` path end-to-end
- [ ] `TestServe_ExistingBehaviorUnchanged` (SC8 sweep): run the full pre-existing suite — `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go build ./... && go test ./... -count=1'` — all pre-existing tests pass unmodified (diagnostics, hover, completion, navigation, store, parsers); the only edited existing test is the `NewServer` call-site helper; percent-kind set and rendered signatures provably unchanged (34 empty-Kind entries all have empty Desc → no Kind fills)
- [ ] Run full validation: `go build ./...` clean; memory-capped `go test ./... -count=1` green; `goimports -w .`; `golangci-lint run`; `goga lint` (8 cells / 0 errors); `goga contract kb`, `goga contract xs`, `goga contract rms`, `goga contract hints`, `goga contract server` all pass
- [ ] Verify no unintended diffs: `kb/data/rms-commands.json` untouched; `StatementAt` untouched; `.usages/` files untouched; `CODEMANIFEST` files untouched (read-only)

---

## Validation Commands

> **Environment gate**: `go.mod` requires go ≥ 1.26.6; the sandbox
> toolchain is 1.26.4 with toolchain download blocked. Run all Go
> commands below in a capable environment (CI). `go test ./...` must run
> under the memory cap (a runaway test previously OOM'd the machine).

- `go build ./...`: compile every package — facade accessibility of all exported contract identifiers
- `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`: run all tests (SC8 sweep included)
- `goimports -w .`: import/format normalization (includes `gofmt`)
- `golangci-lint run`: lint check
- `goga lint`: cell-manifest lint (expect 8 cells / 0 errors)
- `goga contract kb` / `goga contract xs` / `goga contract rms` / `goga contract hints` / `goga contract server`: per-cell contract conformance between CODEMANIFEST and implementation

---

## Completion Criteria

- [ ] Every contract entity is implemented in the correct `location` (`kb/model.go`, `kb/mine.go`, `kb/store.go`, `kb/extract.go`, `xs/ast.go`+`xs/parse.go`, `rms/ast.go`+`rms/parse.go`, `hints/computer.go`, `hints/hint.go`, `server/server.go`, `server/serve.go`)
- [ ] Every contract entity is accessible from its facade (Go package exports: `kb.ValueRange`, `kb.MineKindRange`, `xs.CallSite`, `XsFile.CallAt`, `rms.ArgSite`+`KindArg`/`KindAttr`/`KindNone`, `RmsFile.ArgAt`, `hints.Computer`+`NewComputer`, `hints.Hint`, `server.NewServer(…, computer)`, `Server.SignatureHelp`)
- [ ] Properties and methods match the declared API
- [ ] Descriptions are reflected in behavior (truth model, conflict rule, silence conventions, never-clamp, fill-when-empty provenance, band owner, categorical exclusions)
- [ ] Contract dependencies are met (hints imports common/kb/xs/rms; server imports hints)
- [ ] Re-exports: none declared — none required
- [ ] Every coding task followed the TDD workflow (contract tests → code → verification → logic tests → debugging → re-verification → lint)
- [ ] Contract tests and logic tests cover facade, API, and behavior within each coding task
- [ ] Integration tests exist for the cross-entity full-stack scenarios (Task 9)
- [ ] No package boundary was expanded (`cmd/kbgen` wire mirror only, as mandated by the design's Additional Instructions; no new cells beyond the contracted `hints`)
- [ ] `CODEMANIFEST` files were not modified (contract is read-only); `.usages/` files not modified (verified current by the design); `kb/data/rms-commands.json` not regenerated
- [ ] All validation commands pass (in a capable environment: `go build ./...`, memory-capped `go test ./... -count=1`, `goimports`, `golangci-lint run`, `goga lint`, `goga contract <cell>` × 5)
- [ ] Every Usages entry is mentioned in at least one task (`conventions` T1–T9, `kbdata` T1–T3, `xs_grammar` T4, `rms_grammar` T5, `positions-and-diagnostics` T4–T7, `lookups` T7, `xs-parsing` T7–T9, `rms-parsing` T7–T9, `computing` T8–T9, `lsp-protocol` T8–T9, `closure` T8)
