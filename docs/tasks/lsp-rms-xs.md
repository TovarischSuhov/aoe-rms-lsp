# LSP Server for AoE2 RMS + XS

Status: Done — первичная сборка шести ячеек (см. план docs/plans/build-lsp-rms-xs.md); продолжения PR #3–#6

## Current State

Project repository is empty except for `.goga/` config (Go 1.26 image, `conventions` usage mandatory). No code, no cells, `goga schema` returns `[]`.

No LSP tooling exists for AoE2 RMS/XS anywhere:

- **mangudai** (npm, TS) — parser+linter, but grammar is pre-DE (AoC/HD/UP); DE syntax produces false positives
- **T-West.age2-rms** (VS Code, 2026-03) + tree-sitter-aoe2-rms — syntax highlighting only, no LSP features
- **deltaidea.aoe2-rms** (2018) — outdated
- Map scripts can only be verified in-game (no headless generation exists)

Knowledge sources already collected locally in `docs/ref/`:

- `docs/ref/ugc-guide/xs/` — UGC guide repo snapshot (GitHub `Divy1211/AoE2DE_UGC_Guide`, commit 2026-07-24, covers Update 177723):
  - `functions/functions.json` — **204 XS functions**, 15 categories, with return types, params (name/type/required/desc)
  - `constants/constants.json` — **27 constant sections** (incl. new `Tech Attribute`, `Locale`, `Panel`, `Timer Unit`, `Color`)
  - `prelude.xs` — auto-generated dump of game XS externs (2026-07-02): 882 constants + intrinsics with doc comments
  - `programmer.md`, `beginner.md`, `tricks.md` — XS language syntax reference (C-like + rules, vectors, events)
- `docs/ref/aoe2de-xs-rms-changelog.md` — release-notes sweep **2019–2026 complete** (77 update posts scanned, 16 with XS/RMS changes, 404 lines; incl. version metadata for "since update N")
- `docs/ref/zetnus-rms-guide.txt` — Zetnus «Definitive Random Map Scripting Guide» txt export (272KB, 5909 lines), **verified current**: includes all 2025–2026 DE additions (`water_definition`, `create_object_group`, `land_conformity`, `set_circular_base`, `require_path`, `override_map_size`); sections: Syntax Skeleton (PLAYER_SETUP, LAND/ELEVATION/CLIFF/TERRAIN/CONNECTION/OBJECTS_GENERATION, Global Syntax, Random Code = `start_random`/`percent_chance`/`end_random`, Conditionals, Map Sizes, Math Expressions, Walls) + Constant Reference (Terrains, Objects, Effects, Attributes, Resources, Technologies, Classes) + Scripting/Testing

## Description

Build a single Go binary implementing a stdio LSP server (`aoe2-lsp`) for two AoE2 DE languages:

- **RMS** (Random Map Scripts) — section-declarative syntax: `<player_setup>`, `create_*` commands, attributes, constants, `start_random`/`percent_chance` blocks, math expressions (DE 141935+), `#include`, `#includeXS`
- **XS** (External Subroutines) — full C-like syntax (types, functions, expressions, control flow, `rule`/`event` blocks) with error recovery

MVP LSP features: **diagnostics** (syntax errors, unknown commands/attributes, wrong arguments, undefined XS symbols), **hover** (docs from knowledge base), **completion** (RMS commands/attributes/constants, XS functions/constants). Server is editor-agnostic; connection configs for Neovim (lspconfig) and VS Code (via generic LSP extension) shipped as files + README.

## Scope

**In scope:**
1. Knowledge base (KB): unify UGC JSON + Zetnus RMS extraction + release-notes version metadata into versioned JSON in repo, embedded via `go:embed`; lookup API
2. RMS parser: lexer + parser to AST with error recovery, positions preserved
3. XS parser: lexer + parser (C-like + rules/events) to AST with error recovery
4. Analysis layer: semantic diagnostics over ASTs + KB (unknown symbols, arity/type mismatches, DE-version awareness)
5. LSP server on `go.lsp.dev/protocol`: full-document sync, push diagnostics, hover, completion; editor configs + README

**Out of scope:**
- go-to-definition / navigation / references
- Syntax highlighting (covered by T-West extension)
- Headless map generation/validation
- AoC/HD/UP dialect modes (DE only; version metadata is informational, not a dialect switch)
- Own VS Code extension (configs only)
- Formatting

## Acceptance Criteria

- `go test ./...` and `golangci-lint run` pass (per `conventions`)
- RMS parser: parses real DE RMS files (bundled fixtures from shipped maps) without false errors; unknown command/attribute → diagnostic with correct range
- XS parser: parses UGC `prelude.xs` (197KB, 5k lines) and typical RMS-embedded XS without false errors; undefined identifier in XS → diagnostic
- Completion returns all 204 XS functions from `functions.json` (arity + snippet), RMS commands from KB
- Hover shows function signature + description (from JSON `desc`) for XS functions and RMS commands
- Diagnostics pushed on didOpen/didChange within one change round-trip (no debounce required for MVP)
- Server starts via stdio and works with Neovim lspconfig per README instructions (manual check documented)
- All knowledge-base JSON files validated in tests (schema + no duplicate names)

## Stack

- **Language:** Go 1.23+ (per `conventions`), single module
- **LSP:** `go.lsp.dev/protocol` + `go.lsp.dev/jsonrpc2` (LSP 3.18 types)
- **Parsers:** hand-written on stdlib (custom lexers; `text/scanner` optional) — no existing DE-grade grammar exists to reuse
- **Data:** JSON files in repo + `go:embed`; no runtime downloads
- **Testing:** `testify` + `cmp` (AST deep-compare), per `conventions`
- **Quality:** `goimports`, `golangci-lint`, `go test ./...`

## External Dependencies

| Component                     | Usage file                            | Status                            |
|-------------------------------|---------------------------------------|-----------------------------------|
| `go.lsp.dev/protocol` (+jsonrpc2) | `.goga/usages/cooks/lsp-protocol.md` | created (2026-09-04)              |
| `testify`, `cmp`              | `.goga/usages/conventions.md`         | existing (covered by conventions) |

## Risks and Constraints

- Zetnus guide extraction: txt export is prose-with-skeleton (not machine-readable) — RMS command/attribute KB requires a manual/scripted extraction pass within subtask 1; structure follows the guide's Syntax Skeleton sections
- XS grammar corner cases (vector literals, `rule` reactivation params, string escapes) — mitigate with `prelude.xs` + real map fixtures as parser tests
- RMS semantics are position-dependent (attributes belong to preceding command) — AST must model statement context, not flat token stream
- `go.lsp.dev` sees infrequent releases; API is stable (LSP 3.18), verified against pkg.go.dev on 2026-09-04 — see cook
- Corporate network: docs.google.com and some ugc.aoe2.rocks paths unreachable/blocked (Kaspersky) from the dev machine — use MCP webReader for such fetches; GitHub works

## Scope Estimate

Single task, 5 subtasks (order: 1 → (2 ∥ 3) → 4 → 5):

1. **KB** — convert UGC JSON (as-is or adapted), extract RMS commands from Zetnus, merge version metadata from release-notes sweep; define Go types + `go:embed` + validation tests. Independent value: queryable data layer.
2. **XS parser** — lexer/parser/AST/error-recovery + fixtures (prelude.xs, sample .xs). Independent value: `xs.Parse` usable as a library.
3. **RMS parser** — lexer/parser/AST/error-recovery + fixtures (shipped DE maps). Independent value: `rms.Parse` usable as a library.
4. **Analysis** — diagnostics over 2+3 backed by 1. Independent value: CLI-lintable checks.
5. **LSP server** — wire 4 into protocol handlers, completion/hover from KB; editor configs, README. Final integrator.

## Existing Architecture

None — task creates architecture from scratch. Cell/CODEMANIFEST design is deferred to the `goga-brainstorm` stage (bottom-up: parsers and KB are leaf candidates).

## Notes

- MVP feature set agreed 2026-09-04: parser + diagnostics + hover + completion; navigation excluded
- Client: server + editor connection configs (Neovim lspconfig, VS Code generic LSP); no own extension
- Target API examples (Go, approved to include):

```go
// Knowledge base (subtask 1)
type Store struct{ /* embedded JSON */ }
func Load() (*Store, error)                       // from go:embed
func (s *Store) Function(name string) *Function   // nil if unknown
func (s *Store) Constants(prefix string) []Constant
func (s *Store) Command(name string) *Command     // RMS command + attributes + args

// Parsers (subtasks 2, 3)
func rms.Parse(src []byte, name string) (*rms.File, []Diagnostic)
func xs.Parse(src []byte, name string) (*xs.File, []Diagnostic)

// Analysis (subtask 4)
func Analyze(f File, kb *kb.Store) []Diagnostic    // union type over rms.File / xs.File

// LSP (subtask 5)
type Server struct {
    protocol.UnimplementedServer
    client protocol.Client
    docs   *Store // open-document cache: uri -> parsed File
}
```

- Data sources map: UGC JSON → XS functions/constants; `prelude.xs` → completeness cross-check; Zetnus → RMS commands/attributes; release-notes changelog → "since update N" metadata for hover
- Fixtures corpus for parsers: `aoe2map.net` (community RMS maps) + `snippets.aoe2map.net` (RMS snippets incl. actor areas, error handling, version checks). Game is NOT installed on the dev machine — shipped DE maps unavailable locally; UGC `docs/rms/` section is empty (verified 2026-09-04: index.md = 2 outbound links, basics.md = 0 bytes)
- **RMS language grew far beyond classic AoC grammar in 2025–2026** (per changelog): arithmetic operators and float constants (141935/153015), `water_definition`, `create_object_group`, `create_connect_land_zones`, attributes `land_conformity`, `generate_mode`, `set_circular_base` (153015), `require_path` (141935), `cliff_type` (95810), `generate_for_first_land_only`, `override_map_size` (81058). The RMS parser and KB must cover these DE-era extensions; `effect_percent` is deprecated in favor of operators (141935)
- XS timeline anchor points for version metadata: XS introduced in update **42848** (2020-11); Effects.xs / resource-33 XS calls (153015); unit manipulation functions `xsCreateUnit`/`xsSetUnitPosition`/`xsRemoveUnit` (169123); big function batch incl. math/bitwise (177723)
- Update 177723 (2026-06-02) added ~30 XS functions (e.g. `xsGetMapSeed`, `xsGetTechAttribute`, math/bitwise) — UGC JSON already covers them; KB version metadata should mark them `since: 177723`
- Custom language IDs: `aoe2rms`, `aoe2xs` (see `lsp-protocol` cook)
