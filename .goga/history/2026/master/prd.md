# Signature Help for AoE2 RMS + XS Scripting

Product change for **aoe2-lsp** — the language server for Age of Empires II:
Definitive Edition Random Map Scripts (RMS) and External Subroutines (XS).

## Problem

AoE2 RMS/XS scripters editing scripts in LSP-connected editors (Neovim, VS Code)
spend much of their time inside calls — XS function calls whose parameters are
typed and positional (`kb.Function.Params`, source order), and RMS
commands/attributes whose arguments are positional and kind-constrained
(`number / percent / const / terrain / object / …`).

While typing or editing arguments inside those calls, the author has no
position-aware guidance: nothing tells them which parameter the cursor is on,
what its expected type/kind is, how many arguments remain, or which arguments
are optional. The full signature is only visible by hovering the
function/command name — a static render that does not track the cursor's
position inside the call — and valid argument forms live in external
references (Zetnus guide, UGC docs) the author must context-switch to.

Typing is interrupted by documentation lookups and hover round-trips;
argument-order and argument-type mistakes are made while writing and are only
reported afterwards by diagnostics (`bad-arity`, `bad-type`,
`bad-argument-value`) — or not at all, since an RMS script that runs with
wrong values fails silently in-game, producing a wrong map instead of an
error.

## Users

**Primary user** — AoE2 DE map/script author ("map maker") writing or editing
`.rms`/`.xs` files in an LSP-connected editor (Neovim via lspconfig custom
server, VS Code via generic LSP client).

- *Trying to*: write calls correctly the first time — XS function calls with
  typed positional parameters, RMS commands/attributes with kind-constrained
  positional arguments.
- *When*: mid-editing with the cursor inside an argument list; also when
  revisiting older scripts.
- *What matters*: which parameter the cursor is on, its expected type/kind,
  how many arguments remain — without leaving the editor or breaking typing
  flow.
- *Constraints*: 200+ XS functions and large RMS command surfaces are not
  retainable from memory; the game surfaces no runtime errors for RMS (wrong
  values fail silently); the feature works through whatever parameter-hint
  rendering each editor's client provides.

**Two language subgroups** (same capability, distinct data and hint shapes):

- *XS scripters* — typed parameters in call order.
- *RMS authors* — argument kinds (`number/percent/const/terrain/object/…`),
  ranges, counts.

**Secondary actor** — players of published maps: affected by silent argument
mistakes but never interact with the product; impact rationale only, not a
driver of product behaviour.

## Goals

1. **In-context parameter awareness** — While typing or moving through the
   arguments of a call, an author always sees which parameter the cursor is
   on and what form it expects (type for XS, kind/range for RMS), without
   leaving the editor or breaking typing flow.
2. **Right-first-time argument writing** — Argument-order and argument-form
   mistakes are prevented at the moment of writing, so the author's reliance
   on after-the-fact diagnostics rounds and on silent in-game failures drops.
3. **Equal guidance in both authoring surfaces** — XS scripting and RMS map
   authoring get the same quality of parameter guidance; neither language
   stays a second-class editing experience.
4. **Trustworthy guidance** — Hints track the actual cursor position and stay
   silent rather than mislead when the call context is ambiguous, incomplete,
   or broken.

## User Experience

**Entry point.** The author edits an `.rms`/`.xs` file in their LSP-connected
editor. As the cursor enters an argument context, the editor's native
parameter-hints widget appears; nothing to enable, no mode to enter.

**Primary scenario — XS call.** Typing `xsVectorSet(` shows
`vector xsVectorSet(float x, float y, float z)` with the parameter at the
cursor highlighted; name + type per parameter. Moving the cursor across `,`
moves the highlight; hints dismiss at the closing `)`. The same behaviour
applies to calls to user-defined functions from the open document or any file
of the `#include` closure (parameter names from declarations; types only when
declared in source; no invented types, no knowledge-base-style descriptions).

**Primary scenario — RMS command.** With the cursor inside a command's
arguments (positional statement values or a block's attributes, e.g. within
`create_land { … }`), the widget shows that command's expected arguments —
name plus expected kind/range (`percent (0..100)`, `terrain (const)`, …) —
with the argument at the cursor highlighted; the highlight follows navigation,
hints dismiss when the cursor leaves the command.

**Alternative scenarios.** Nested XS calls: the innermost enclosing call wins,
the outer call on moving out. Include-closure functions show hints even when
the defining file was never opened in the editor. Untyped XS parameters show
names only.

**Failure scenarios (trust).** No hints — silently — when: the
function/command is unknown (not in the knowledge base and not declared in
the closure); the syntax at the cursor is unbalanced or broken; the cursor is
outside any call, in a string literal, or in a comment; the cursor-to-argument
mapping is ambiguous — in which case at most a signature without a highlighted
parameter is shown, never a wrong highlight.

**States and feedback.** Hints visible / not visible; the highlighted
parameter tracks the cursor; re-entering a call re-shows hints; the editor's
manual signature-help binding also works. Rendering is native per editor
(Neovim floating window, VS Code parameter widget). Hints stay concise — deep
descriptions remain hover's job.

**Interruption and consequences.** Pure read-only guidance; no document
modification, no undo surface; syntax errors elsewhere in the file do not
disable hints in well-formed regions (consistent with existing
hover/diagnostics behaviour on broken files).

## Requirements

- **R1 — XS knowledge-base function hints.** When the cursor is inside the
  argument list of a call to an XS function known to the product's knowledge
  base, the product must present that function's signature — name, return
  type, and declared parameters in source order (name + type) — with the
  parameter matching the cursor's argument position marked active, rendered
  through the editor's native parameter-hints widget.
- **R2 — XS user-defined function hints (closure-wide).** The same guidance
  must apply to calls of user-defined XS functions declared in the same
  document or in any file reachable via that document's `#include` closure:
  parameter names in declaration order, types shown only when declared in
  source — never invented.
- **R3 — RMS command hints.** When the cursor is within the arguments of an
  RMS command known to the knowledge base (positional statement values or
  attributes inside a command block), the product must present that command's
  argument list — argument name plus expected value form (kind, and range
  where defined, e.g. `percent 0..100`) — with the argument at the cursor
  marked active.
- **R4 — Active-argument tracking and lifecycle.** The active mark must
  follow the cursor as it moves between arguments; hints must disappear when
  the cursor leaves the call and reappear on re-entry; the same guidance must
  be available on the editor's manual signature-help action wherever
  automatic hints apply.
- **R5 — Nested calls.** For nested XS calls, guidance must address the
  innermost call enclosing the cursor; moving into an outer call's arguments
  switches guidance to that call.
- **R6 — No-hint rule (trust).** No hints — rather than guessed or wrong
  ones — must be shown when: the called name is unknown to the knowledge base
  and not declared in the closure; the syntax enclosing the cursor is
  unbalanced or broken; the cursor is outside any argument context, inside a
  string literal, or inside a comment; or the cursor's argument position
  cannot be determined unambiguously — in which case at most a signature
  without an active parameter may be shown.
- **R7 — Concise content.** Hints must stay concise: signature plus
  per-parameter name/type (XS) or name/kind/range (RMS). Extended
  descriptions and examples remain hover's responsibility and must not be
  forced into hints.
- **R8 — Editor parity and activation.** The capability must work through the
  product's existing editor integrations (Neovim lspconfig custom server, VS
  Code generic LSP client) with identical behaviour, native rendering per
  editor, and must be announced to editors at connection initialization so
  parameter widgets activate without per-editor forks.
- **R9 — Availability under partial breakage.** Hints must remain available
  in well-formed regions of a document that has syntax errors elsewhere,
  consistent with the product's existing hover/diagnostics tolerance of broken
  files.
- **R10 — Read-only.** Hint requests must never modify the document, editor
  state, or any file.

## Constraints

- **C1 — Editor-agnostic delivery.** Guidance must reach users through the
  product's existing standard LSP integration (stdio) in Neovim and VS Code;
  no editor-specific plugin may become a prerequisite.
- **C2 — Offline operation.** Hints must be computable entirely from data
  already local to the product (embedded knowledge base, open documents,
  local include files); no network access may be introduced.
- **C3 — Data provenance.** Parameter data may come only from the existing
  knowledge base and the user's own files; no new external data sources
  without license review (the product stays GPL-3.0 compatible).
- **C4 — No regression.** Existing diagnostics, hover, completion, and
  navigation behaviour must remain unchanged; the new capability is strictly
  additive.
- **C5 — Established component boundaries.** The solution must extend the
  existing component contracts (parsers, knowledge base, include resolution)
  rather than redefine them; affected contracts are updated through the
  project's cell workflow.
- **C6 — Graceful degradation.** When closure data is unavailable (missing or
  unresolvable include, unparsable file), hints degrade to no-hints for the
  affected names — no errors or prompts enter the editing flow (missing
  includes are already reported by diagnostics).

## Scope

### In Scope

1. XS signature help for knowledge-base functions (R1).
2. XS signature help for user-defined functions declared in the open document
   or its `#include` closure (R2).
3. RMS command signature help for positional statement arguments and block
   attributes (R3).
4. Cursor-to-active-argument tracking, hint lifecycle (dismiss on leaving,
   reappear on re-entry, manual trigger) (R4).
5. Innermost-call resolution for nested XS calls (R5).
6. No-hint trust rules: unknown names, broken syntax, outside/string/comment
   positions, ambiguous argument mapping (R6).
7. Capability announcement at connection initialization so native editor
   widgets activate (R8).
8. Graceful degradation when closure data is unavailable (C6).
9. Availability in well-formed regions of partially broken documents (R9).

### Out of Scope

1. Extending **hover or completion** to user-defined functions or closure
   names — those features keep today's knowledge-base-only behaviour (C4).
2. New or changed **diagnostics** — no new checks, codes, or analysis changes
   of any kind.
3. Deferred analysis items from previous work: `since_update` version
   warnings, `unknown-constant`-style constant validation.
4. Per-editor plugins, settings UIs, or non-standard integrations beyond the
   existing Neovim/VS Code setups.
5. Other LSP features: code actions, quick-fixes, rename, formatting,
   semantic tokens, inlay hints.
6. Rich parameter documentation or examples inside hints (stay in hover,
   per R7).
7. Enumerating valid constant values inside hints (already completion's job).
8. Documents without file identity (untitled/virtual buffers) — guidance for
   open `.rms`/`.xs` documents only, consistent with existing behaviour.

## Success Criteria

- **SC1 — XS knowledge-base hints, position-accurate.** In a live editor
  session over the product's standard connection, placing the cursor on any
  argument of a call to a knowledge-base XS function shows the function's
  signature in the editor's parameter widget with exactly the parameter under
  the cursor marked active (verifiable e.g. on `xsVectorSet(` with the
  cursor on the 2nd argument).
- **SC2 — RMS command hints.** With the cursor on any argument/attribute of a
  known RMS command (e.g. inside `create_land { … }`), the widget shows that
  command's argument list — names with expected kind/range — with the
  argument at the cursor active.
- **SC3 — Lifecycle.** Moving the cursor between arguments moves the active
  mark; leaving the call dismisses hints; re-entering re-shows them; the
  editor's manual signature-help action produces the same guidance.
- **SC4 — Closure-wide user-defined hints.** A call to a user-defined XS
  function declared in an `#include`d file that was never opened in the
  editor still shows correct parameter hints (names in declaration order,
  types only when declared).
- **SC5 — Language parity.** The guidance quality is equivalent across XS and
  RMS: both subgroups get signature plus per-parameter hints with
  active-argument tracking.
- **SC6 — Trust.** In every defined no-hint situation (unknown name, broken
  syntax, outside a call, string, comment, unresolvable closure data,
  ambiguous position) the product returns no hints rather than wrong ones —
  and in no observable case does it mark the wrong parameter active.
- **SC7 — Right-first-time writing.** An author unfamiliar with a given API
  can write a complete multi-argument XS call and a populated RMS block
  command using only hints plus existing hover, without consulting external
  documentation.
- **SC8 — No regression.** On the project's existing fixture corpus,
  diagnostics, hover, completion, and navigation behaviour is unchanged after
  the feature ships.

No quantitative targets are established; none were justified by the product
context.
