# KB Data Pipeline — regenerating embedded JSON

Domain: rebuilding kb/data/*.json from local sources in docs/ref/.
Target audience: maintainers updating the knowledge base.

## Sources (all local, no network)

- docs/ref/ugc-guide/xs/functions/functions.json → xs-functions.json
- docs/ref/ugc-guide/xs/constants/constants.json → xs-constants.json
- docs/ref/zetnus-rms-guide.txt → rms-commands.json (via ExtractRmsCommands)
- docs/ref/aoe2de-xs-rms-changelog.md → since_update enrichment

## Regenerate

```go
cmds, err := kb.ExtractRmsCommands("docs/ref/zetnus-rms-guide.txt")
// marshal into kb/data/rms-commands.json per the kbdata schema

Preconditions:
- Duplicate names within one file are a build error — resolve, do not skip.
- NewStore() must pass after regeneration (run kb tests).
