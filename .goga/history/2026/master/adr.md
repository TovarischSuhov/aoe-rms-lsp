# Signature help: stateless, kb-mined, source-first

For the signature-help capability (PRD: `prd.md`) we deliver a stateless LSP
`signatureHelp` provider over the existing kb/include/parser surfaces, with
hint truth coming from source declarations first and the knowledge base
second, and with kb prose mined into structured kind/range data at load time.
These choices keep the feature identical across Neovim/VS Code, silent rather
than wrong in every ambiguous case, and strictly additive (C4/C5).

## Settled decisions

1. **Stateless provider, triggers `(` and `,`.** No server-side hint state;
   RMS hint refresh rides on client re-requests while the widget is open plus
   the manual binding. RMS has no sane trigger character — `space`/`=` would
   fire on nearly every keystroke.
2. **RMS hint is one kb-ordered list** — positional `Args`, then
   `Attributes`. Active argument: by ordinal for positional values, by
   **name-match** for attributes (name is their only truthful identity).
   Cursor off any argument → signature with no active mark.
3. **XS edge positions: never clamp.** Trailing comma activates the next
   index; an index beyond the declared parameters shows the signature with
   **no** active parameter. Clamping to the last parameter would suggest an
   argument belongs to it (SC6 forbids wrong highlights); arity is
   `bad-arity`'s job.
4. **Name collisions: source wins.** Open-document and include-closure
   declarations beat the kb (mirrors the analysis cell's "local declarations
   always take precedence over external"); source declarations that disagree
   with each other → no hints for that name.
5. **Embedded XS in RMS `XsBlock`s is in scope** — same XS hint behavior as
   `.xs` documents (parity, and XsBlock ASTs already exist).
6. **kb extraction mines kind/range from `Desc` prose** (`number (0-2)` →
   `number 0..2`, also filling the 34 empty-`Kind` entries; unparseable → raw
   `Kind`/name-only). Ranges exist only as prose today; R3's promised
   `percent 0..100` guidance is unreachable without this, and it stays within
   the existing pipeline and provenance (C3/C5).
7. **Optional arguments render in square brackets** (`[z: float]`,
   `[border_fuzziness: percent]`) driven by the structured `Required` bools; no
   default values are invented (they are prose in `Desc`).
8. **In-progress (unbalanced) calls must yield hints** — a hard requirement;
   parser recovery is extended if it does not preserve in-progress calls and
   blocks with usable positions. The first keystroke after `(` is the
   feature's core moment (SC1); R6's silence is for *broken* code, not code
   being written.
9. **Helper-calls inside RMS value expressions: no hints this round**
   (silent, deferred). PRD-silent corner, unproven value.
10. **Full kb-ordered list always rendered** — no truncation or windowing,
    even for the largest kb list (`create_object`, 46 attributes).

## Considered options (rejected, non-obvious)

- **Clamp active parameter to the last declared one** (some editors' default
  rendering) — rejected: a wrong highlight is the one failure SC6 names.
- **kb wins over source declarations** — rejected: hints must describe the
  code that will actually run.
- **Structured-fields-only RMS hints (no range mining)** — rejected: drops
  R3's headline capability; the prose patterns in the embedded data are
  regular.
- **RMS trigger characters (`space`, `=`)** — rejected: widget spam on every
  value keystroke.
- **Windowed/truncated argument lists for huge commands** — rejected: invents
  presentation policy and hides information; revisit only on noise
  complaints.

## Consequences

- Completion's trigger chars (`" ", "<"`) share no character with signature
  help's (`"(", ","`) — no double-fire on any keystroke; completion stays
  untouched per C4.
- Decision 6 grows the kb extraction/load pipeline's contract (through the
  cell workflow, C5); decision 8 may grow xs/rms recovery contracts.
- Active-mark tracking quality depends on each client's re-request policy
  while the widget is open; the manual binding is the guaranteed path (R4).

## Open questions (for the cell/architecture stages, not decided here)

- Which cells own hint computation, and where the parser-recovery extension
  lands.
- Verify at design time (undocumented in CODEMANIFEST/usage artifacts):
  whether xs/rms recovery currently preserves in-progress calls/blocks with
  positions; what the include cell does when a closure file fails to parse
  (hints degrade to silence for affected names per C6 either way).
