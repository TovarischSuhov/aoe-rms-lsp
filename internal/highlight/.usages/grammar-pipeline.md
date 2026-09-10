# Grammar Pipeline — regenerating the RMS TextMate grammar

Domain: rebuilding editors/vscode/syntaxes/aoe2rms.tmLanguage.json from the
committed kb data. Target audience: maintainers updating the knowledge base
or the grammar skeleton.

## Regenerate

Single entry point — highlight.GenTmLanguage emits the grammar from a
loaded kb Store: the hand-written skeleton (sections, # directives, block
comments, numbers/percent, strings) plus keyword alternations generated
from Commands(""), command Attributes and Constants(""):

```go
store, err := kb.NewStore()
if err != nil {
    return fmt.Errorf("load knowledge base: %w", err)
}
err = highlight.GenTmLanguage(store,
    "editors/vscode/syntaxes/aoe2rms.tmLanguage.json", slog.Default())
```

cmd/tmgen wraps exactly this call (flags only, no logic of its own);
its default output is editors/vscode/syntaxes/aoe2rms.tmLanguage.json,
overridable with -out.

## Order: kbgen first, then tmgen

Keyword content comes from the committed kb data. When docs/ref changes,
regenerate in order: kb data first (cmd/kbgen), then the grammar
(cmd/tmgen). The grammar artifact is committed and must never drift from
the committed kb data.

## No-drift golden

Regeneration is deterministic — the same data yields a byte-identical
file. The cell's golden test regenerates into a temp dir and compares
against the committed artifact. Never edit the committed
aoe2rms.tmLanguage.json by hand: change the skeleton in internal/highlight
and regenerate.

## Hand-written companions

- aoe2xs.tmLanguage.json is hand-written (fixed XS language keywords,
  no generation). Constants in XS are colored by the server's semantic
  tokens, not by the grammar. JSON validity of both grammar files is
  guarded by tests.
- contributes.grammars in editors/vscode/package.json registers both
  grammars (scopeName source.aoe2rms / source.aoe2xs).
