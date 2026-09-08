# KB Data Pipeline — regenerating embedded JSON

Domain: rebuilding kb/data/*.json from local sources in docs/ref/.
Target audience: maintainers updating the knowledge base.

## Sources (all local, no network)

- docs/ref/ugc-guide/xs/functions/functions.json → xs-functions.json
- docs/ref/ugc-guide/xs/constants/constants.json → xs-constants.json
- docs/ref/zetnus-rms-guide.txt → rms-commands.json (via ExtractRmsCommands)
- docs/ref/aoe2de-xs-rms-changelog.md → since_update enrichment

## Regenerate

Single entry point — kb.GenKB adapts functions/constants JSON, extracts RMS
commands, enriches since_update, validates name uniqueness and writes all
three files deterministically (sorted keys):

```go
err := kb.GenKB("docs/ref", "kb/data", slog.Default())
if err != nil {
    return fmt.Errorf("regenerate knowledge base: %w", err)
}
```

cmd/kbgen wraps exactly this call (flags only, no logic of its own).

Preconditions:
- Duplicate names within one file are a build error — resolve, do not skip.
- NewStore() must pass after regeneration (run kb tests).

## Mining on regeneration (signature help)

Extraction mines structured kind/range from Desc prose via kb.MineKindRange:
bounded entries get Range ("number (0-99)" → 0..99); empty-Kind entries get
the mined kind when prose has one. Flag attributes (empty Desc, e.g.
set_circular_base) stay name-only — nothing to mine. Ambiguous fragments are
skipped with a WARN to the injected logger — check warnings after a run that
changes the guide. After regeneration run kb tests: NewStore re-mines
fill-when-empty, so regenerated JSON and load-time mining must agree
(idempotent rule).
