# Signature Help for AoE2 RMS + XS

## Current State

**aoe2-lsp** is a Go 1.23+ stdio LSP server (`go.lsp.dev/protocol` +
`go.lsp.dev/jsonrpc2`) split into 7 cells: `common`, `kb`, `rms`, `xs`,
`include`, `analysis`, `server` (top-level directories of the module root).

Today the server provides diagnostics, hover, completion, and navigation
(definition/references/documentSymbol). **No `textDocument/signatureHelp`
handler exists and no `SignatureHelpProvider` capability is advertised** —
the capability this task delivers is entirely absent.

Surfaces the feature must build on:

- `kb` — embedded knowledge base (`Command`/`CommandArg`/`Function`/
  `Param`/`Constant`) with a lookup API and a Zetnus extraction pipeline.
  Parameter **kinds and ranges exist only as `Desc` prose** (e.g.
  `number (0-2)`); `Required` bools are structured (62/325 XS params
  optional; 174/202 RMS args+attributes optional); 34 RMS
  args/attributes have an empty `Kind` (XS params carry `Type`; none
  are empty); kb names are unique (no overloads); `create_object` is
  the largest command with 46 attributes.
- `xs`/`rms` — parsers producing recovery ASTs. Whether recovery preserves
  **in-progress (unbalanced) calls/blocks with usable positions** is
  undocumented in the artifacts — verify at design time (ADR open
  question), extend if not.
- `include` — resolves `#include`/`#includeXS` closures across files;
  declarations are reachable without the defining file ever being opened.
- `analysis` — already establishes "local declarations always take
  precedence over external", which the hint truth model mirrors.

## Description

Deliver the PRD's parameter-hints capability (`prd.md`) exactly as settled
in the ADR (`adr.md`) — a **stateless, kb-mined, source-first**
`textDocument/signatureHelp` provider:

- **Stateless provider, triggers `(` and `,`** — no server-side hint
  state; hint refresh rides on client re-requests plus the editor's manual
  signature-help binding. Capability announced at initialize (R8) so
  native parameter widgets activate in Neovim and VS Code with identical
  behavior. Strictly read-only (R10) and additive (C4).
- **XS hints** — kb functions (R1) and user-declared functions from the
  open document and its include closure (R2): parameter names in
  declaration order, types only when declared (never invented). Hint
  truth: source declarations (document > closure) beat the kb; source
  declarations that disagree with each other → no hints for that name.
  Innermost enclosing call wins for nested calls (R5). Active-parameter
  edges: trailing comma → next index; index beyond the declared
  parameters → signature with **no** active parameter — never clamp.
- **RMS hints** — one kb-ordered list per command: positional `Args`
  first, then `Attributes`. Active argument: by ordinal for positional
  values, by **name-match** for attributes (name is their only truthful
  identity). Cursor off any argument → signature with no active mark.
  The **full list is always rendered** — no truncation or windowing, even
  for the largest kb list (`create_object`, 46 attributes). Optional
  arguments render in square brackets (`[border_fuzziness: percent]`)
  driven by the structured `Required` bools; no default values are
  invented.
- **kb growth** — extraction/load pipeline mines structured kind/range
  from `Desc` prose (`number (0-2)` → kind `number`, range `0..2`), also
  filling the 34 empty-`Kind` RMS args/attributes; unparseable prose →
  raw `Kind`/name-only. Stays inside the existing pipeline and
  provenance (C3/C5).
- **Parser recovery** — extended (`xs`/`rms`) if design-time verification
  shows in-progress calls/blocks are not preserved with usable positions.
  In-progress calls yielding hints is a **hard requirement** (SC1): the
  first keystroke after `(` is the feature's core moment; silence is for
  *broken* code, not code being written.
- **Embedded XS in RMS `XsBlock`s** — in scope, same XS hint behavior as
  `.xs` documents (XsBlock ASTs already exist).
- **Trust rules (R6)** — no hints, silently, when: the called name is
  unknown to the kb and not declared in the closure; the syntax enclosing
  the cursor is unbalanced or broken; the cursor is outside any argument
  context, inside a string literal, or inside a comment; the
  cursor-to-argument mapping is ambiguous — in which case at most a
  signature without an active parameter, never a wrong highlight.

### Target API shape (illustrative — final naming belongs to the architecture stage)

```go
// SignatureHelp answers textDocument/signatureHelp. Stateless, read-only.
func (s *Server) SignatureHelp(ctx context.Context, params *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error) {
	hint, ok := s.docs.Get(params.TextDocument.URI).SignatureAt(params.Position)
	if !ok {
		return nil, nil // R6: silence, never a guessed hint
	}

	return &protocol.SignatureHelp{
		Signatures:      []protocol.SignatureInformation{hint.Sig},
		ActiveSignature: &[]uint32{0}[0],   // single signature per call site
		ActiveParameter: hint.Active,        // *uint32; nil when no argument is active
	}, nil
}
```

### Hint label rendering (pins the ADR's presentation decisions)

- XS, required + optional: `vector xsVectorSet(float x, float y, float z)`
  / `float f(float x, float y, [z: float])`
- RMS, kb-ordered list: `percent_chance(%: percent 0..99)` (required
  positional arg, range mined from `Desc` prose); optional entries
  bracketed — `create_elevation([MaxHeight: number 1..16],
  [number_of_tiles: number], …)`
- Trailing comma / beyond-last index: signature shown, **no** active
  parameter (never clamped)
- `ParameterInformation.Label` is a plain string; `Documentation` omitted —
  extended descriptions remain hover's job (R7)

## Scope

**In scope:**

- XS signature help for knowledge-base functions (R1).
- XS signature help for user-defined functions declared in the open
  document or its `#include` closure (R2).
- RMS command signature help for positional statement arguments and block
  attributes (R3), full kb-ordered list with square-bracket optional
  markers.
- Cursor-to-active-argument tracking and hint lifecycle (R4): active mark
  follows the cursor, hints dismiss on leaving / reappear on re-entry,
  manual trigger works.
- Innermost-call resolution for nested XS calls (R5).
- No-hint trust rules (R6) and concise hint content (R7).
- Capability announcement at connection initialization (R8).
- Graceful degradation when closure data is unavailable (C6) and hints in
  well-formed regions of partially broken documents (R9).
- kb extraction growth: structured kind/range mined from `Desc` prose
  (ADR #6).
- xs/rms parser-recovery extension where verification shows in-progress
  calls/blocks are not preserved (ADR #8).
- Embedded XS in RMS `XsBlock`s (ADR #5).

**Out of scope:**

- Hover or completion extensions to user-defined/closure names — those
  keep today's kb-only behavior (C4).
- Any new or changed diagnostics (no new checks, codes, analysis changes).
- Deferred analysis items: `since_update` version warnings,
  `unknown-constant`-style constant validation.
- Per-editor plugins, settings UIs, or non-standard integrations beyond
  the existing Neovim lspconfig / VS Code generic-client setups.
- Other LSP features: code actions, quick-fixes, rename, formatting,
  semantic tokens, inlay hints.
- Rich parameter documentation or examples inside hints (stay in hover,
  R7); enumerating valid constant values inside hints (completion's job).
- Documents without file identity (untitled/virtual buffers).
- Helper-calls inside RMS value expressions — silent this round (ADR #9).
- Windowed/truncated argument lists for huge commands (ADR #10).

## Acceptance Criteria

Verified against the PRD's success criteria:

- **SC1** — XS kb hints, position-accurate: cursor on any argument of a
  kb XS function call shows the signature with exactly that parameter
  active (e.g. `xsVectorSet(` cursor on 2nd argument).
- **SC2** — RMS command hints: cursor on any argument/attribute of a
  known command (e.g. inside `create_land { … }`) shows the command's
  argument list — names with expected kind/range — with the argument at
  the cursor active.
- **SC3** — Lifecycle: moving between arguments moves the active mark;
  leaving dismisses; re-entering re-shows; the manual signature-help
  action produces the same guidance.
- **SC4** — Closure-wide user-defined hints: a call to a function
  declared in an `#include`d file never opened in the editor still shows
  correct hints (names in declaration order, types only when declared).
- **SC5** — Language parity: equivalent guidance quality across XS and
  RMS.
- **SC6** — Trust: in every defined no-hint situation the product returns
  no hints rather than wrong ones, and in no observable case marks the
  wrong parameter active.
- **SC7** — Right-first-time writing: an unfamiliar author completes a
  multi-argument XS call and a populated RMS block using only hints +
  existing hover.
- **SC8** — No regression: on the existing fixture corpus, diagnostics,
  hover, completion, and navigation behavior is unchanged.
- In-progress (unbalanced) calls yield hints (ADR #8 hard requirement).
- kb data: mined kinds/ranges present for parseable `Desc` prose,
  including the 34 previously empty-`Kind` RMS args/attributes;
  unparseable entries fall back to raw `Kind`/name-only without failing
  the pipeline.

## Stack

- **Frameworks:** `go.lsp.dev/protocol` v1.0.1 + `go.lsp.dev/jsonrpc2`
  (existing LSP framework; signature help needs only the
  `SignatureHelp` handler + `SignatureHelpOptions` capability).
- **Libraries:** Go 1.23+ standard library; `log/slog` (structured
  logging per conventions); `testify` (`assert`/`require`) and `cmp` for
  tests — all existing.
- **Infrastructure:** none — stdio LSP server, embedded kb JSON; fully
  offline (C2).

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| `go.lsp.dev/protocol` (SignatureHelp: handler, options, trigger chars, `ActiveParameter` semantics) | `.goga/usages/cooks/lsp-protocol.md` | updated (new Signature Help section) |

No new external components. kb / parsers / include are in-project cells.

## Risks and Constraints

- **Parser recovery is unverified for in-progress calls** (ADR open
  question): xs/rms recovery may need contract growth; the extension's
  landing cell is an architecture-stage decision. Hard requirement — SC1
  fails without it.
- **kb prose mining is heuristic**: `Desc` patterns are regular but not
  guaranteed; unparseable entries must degrade to raw `Kind`/name-only,
  never fail the load pipeline (C6 spirit).
- **Ownership of hint computation is undecided** (ADR open question):
  which cell computes hints, and where recovery extension lands, is
  deferred to the cell/architecture stages — this task must not
  pre-empt that.
- **Completion's trigger chars** (`" ", "<"`) share no character with
  signature help's (`"(", ","`) — no double-fire on any keystroke;
  completion stays untouched per C4.
- **Active-mark tracking quality depends on each client's re-request
  policy** while the widget is open; the manual binding is the guaranteed
  path (R4).
- **C1–C6 constraints** bind: editor-agnostic stdio delivery, offline
  operation, kb+user-files provenance only, strictly additive, extend
  existing cell contracts (no boundary redefinition), graceful degradation
  to silence.
- **`create_object`'s 46-attribute list** renders in full by decision —
  presentation-noise risk accepted, revisit only on complaints.

## Scope Estimate

**Single task.** One cohesive capability spanning ~4–5 cells (`server`,
`kb`, `xs`, `rms`; consumes `include`/`common`), 10 settled ADR decisions,
requirements R1–R10. The natural chunks (kb mining, recovery extension,
provider wiring) are prerequisites of one another, not independent
deliverables, and success criteria verify the feature as a whole (SC6/SC8).
Cell-level decomposition belongs to the architecture (brainstorm) stage.

## Existing Architecture

Affected cells (candidates; final ownership is the architecture stage's
per the ADR):

- `server` — new `SignatureHelp` handler, capability advertisement
  (`TriggerCharacters: ["(", ","]`), DocStore/position wiring. Certain.
- `kb` — extraction/load pipeline grows structured kind/range fields
  mined from `Desc` prose (contract growth through the cell workflow,
  C5).
- `xs` — possible parser-recovery contract growth (in-progress calls with
  positions).
- `rms` — possible parser-recovery contract growth (in-progress blocks
  with positions); `XsBlock` hint support.
- `include` — closure declaration lookup consumed as-is (degradation to
  silence per C6 when closure data is unavailable; what the include cell
  does when a closure file fails to parse is a design-time verification
  item).
- `common` — consumed as-is (positions/ranges).

References: `lsp-protocol` cook (`.goga/usages/cooks/lsp-protocol.md`,
Signature Help section) for the wire-level implementation patterns;
`conventions` (`.goga/usages/conventions.md`) for code and test rules.

## Notes

- Input artifacts: ADR `.goga/history/2026/master/adr.md` (10 settled
  decisions with rejected alternatives), PRD
  `.goga/history/2026/master/prd.md` (R1–R10, C1–C6, SC1–SC8).
- Task formulation was validated with the user across five dialog rounds:
  formulation (approved as written), code examples (both included), stack
  (approved, one cook update), cook content (approved), scope estimate
  (single task).
- Design-time verifications carried into the next stage: (1) whether
  xs/rms recovery preserves in-progress calls/blocks with positions;
  (2) include-cell behavior when a closure file fails to parse.
