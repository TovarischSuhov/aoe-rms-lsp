# KB Data Pipeline — regenerating embedded JSON

Domain: rebuilding internal/kb/data/*.json from local sources in docs/ref/.
Target audience: maintainers updating the knowledge base.

## Sources (all local, no network)

- docs/ref/ugc-guide/xs/functions/functions.json → xs-functions.json
- docs/ref/ugc-guide/xs/constants/constants.json → xs-constants.json
- docs/ref/zetnus-rms-guide.txt → rms-commands.json (via ExtractRmsCommands)
- docs/ref/aoe2de-xs-rms-changelog.md → since_update enrichment
- docs/ref/attribute-descs.json → optional desc overlay (project file, not a
  guide mirror): {"attributes": {name: desc}, "command_args": {command:
  [desc, ...]}}. Ships empty — the guide glossary covers every name; a
  missing file only WARNs, a broken one aborts the build.

## Regenerate

Single entry point — kb.GenKB adapts functions/constants JSON, extracts RMS
commands, enriches since_update, applies the optional desc overlay, logs a
desc-coverage summary (INFO) plus a WARN per attribute still without a desc,
validates name uniqueness and writes all three files deterministically
(sorted keys):

```go
err := kb.GenKB("docs/ref", "internal/kb/data", slog.Default())
if err != nil {
    return fmt.Errorf("regenerate knowledge base: %w", err)
}
```

cmd/kbgen wraps exactly this call (flags only, no logic of its own).

Preconditions:
- Duplicate names within one file are a build error — resolve, do not skip.
- NewStore() must pass after regeneration (run kb tests).

## Refresh on game patches

The patch-day process — source checklist, since_update maintenance,
verification — lives in docs/kb-refresh.md. Its semi-automatic "what's new"
step:

```sh
go run ./cmd/kbgen -diff
```

regenerates into a temp dir and prints the aggregated old→new report
(kb.DiffKB) without touching internal/kb/data.

## Mining on regeneration (signature help)

Attribute descs come from the guide's glossary doc blocks, merged into the
skeleton by name, fill-when-empty: the desc is the record's first prose
paragraph verbatim — set_zone_randomly therefore ends mid-sentence at "This
means:", exactly as the guide reads; fixing the source is outside the
pipeline. Positional argument descs are instead the "* Name - desc"
bullets of the command's own Arguments block, matched by index. Signature
lines documented in functional form (rnd(min,max)) are normalized to the
bare name, so their doc block matches too. Where an attribute name has
section-specific variants (6 names), the record whose Example block names
the command's section wins, else the first record in file order. Attribute
Range/Kind come from the first Arguments bullet of the merged record
carrying mineable bounds; an argument mines its own desc. In both cases
the bounds are a heuristic read of the guide's prose (bullet bounds like
"number (0-99)", prose bands like clumping_factor's "Moderate values
(11-40)"), not authoritative limits.

Merge order is glossary → mining → overlay, with the overlay's inserted
descs mined in turn — every desc in the shipped JSON has passed the same
prose pass. Every stage fills only empty fields, and the load path
(NewStore) re-runs the same fill-when-empty mining through the shared
mineCommandArg helper, so JSON and its load representation agree by
construction (idempotency rule). The overlay closes only the gaps the
glossary left — attributes by name, args positionally. Flag attributes
are no longer name-only: their descs come from the glossary, only the
kind stays empty (their prose has no kind word); an empty desc remains a
handled edge of the miner (empty result, no panic).

Ambiguous fragments are skipped with a WARN to the injected logger — check
warnings after a run that changes the guide. After regeneration run kb
tests: they pin specific records and exercise NewStore over the
regenerated JSON, but there is no wholesale loaded-vs-JSON comparison —
the agreement above rests on the shared helper and fill-when-empty, the
tests are spot checks, not a proof.
