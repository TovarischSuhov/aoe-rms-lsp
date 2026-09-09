# Server Lifecycle — consuming the server cell

Domain: binary entrypoint wiring and editor connection. Target audience:
cmd/aoe2-lsp maintainers and editor-config authors.

## Entrypoint

```go
func main() {
    if err := server.Serve(context.Background()); err != nil {
        slog.Error("server exited", "err", err)
        os.Exit(1)
    }
}
```

## Editor configs
- Neovim: lspconfig, cmd = aoe2-lsp binary, filetypes = { "aoe2rms", "aoe2xs" }
- VS Code: generic LSP extension launching the binary over stdio

Preconditions:
- Single binary, no flags required for MVP; positionEncoding negotiated
  in Initialize (prefer utf-8 when offered).

## Advertised capabilities

Initialize advertises: diagnostics (Full sync + OpenClose), hover,
completion, navigation — definition, references, documentSymbol,
documentHighlight — folding ranges, quick fixes (did-you-mean renames,
effect_percent replacement, missing-include file creation), and
signature help (TriggerCharacters "(" and ","). Editor configs need no
extra flags; positionEncoding is negotiated (prefer utf-8 when the
client offers it). Signature-help widgets refresh on client re-requests
while open; the manual signature-help binding is the guaranteed path.

## Settings

The `"aoe2lsp"` configuration section, delivered both by pull (the
server requests it on `initialized` when the client supports
`workspace/configuration` — VS Code) and push
(`workspace/didChangeConfiguration` — Neovim lspconfig `settings`):

```json
{
  "aoe2lsp": {
    "diagnostics": {
      "severityOverrides": {
        "undefined-symbol": "hint",
        "deprecated-effect-percent": "none"
      }
    },
    "includeRoots": ["/abs/path/to/ai-rms"]
  }
}
```

- `severityOverrides` maps a diagnostic code (see analysis checks) to
  `error` / `warning` / `info` / `hint` / `none`; `none` suppresses the
  diagnostic. Unknown severity names are ignored with a WARN log
  (an unknown code simply never matches). Applied without a server
  restart: every open document's diagnostics are republished after a
  settings change.
- `includeRoots` adds absolute directories searched after the
  including file's own directory (in order) when resolving
  `#include` / `#includeXS` — point it at the game's `ai-rms` folder
  to resolve stock includes. Each call replaces the whole set.

Defaults: no overrides, no extra roots.

## Cross-file navigation

Definition and references resolve across the document's include closure:
targets may live in files that are not open in the editor — the server
loads them from disk on demand (editor state always wins for open files).
Missing #include / #includeXS targets surface as "missing-include"
diagnostics on the directive's path range.

Preconditions:
- Include paths resolve relative to the including file's directory;
  additionally the configured `includeRoots` settings are searched in
  order.
- Disk files are cached by size and modtime. When the client supports
  dynamic registration, the server registers watchers for `**/*.rms`
  and `**/*.xs` on `initialized`: `workspace/didChangeWatchedFiles`
  events force-reload the changed files (even with an unchanged
  size/mtime fingerprint) and republish the diagnostics of every open
  document.

## In-file highlights

documentHighlight returns every occurrence of the word under the cursor
within the current file only — RMS word tokens (section/command/attribute
names, ident/const values) or XS names, including inline-XS regions of
.rms files. All highlights carry kind Text: occurrences are syntactic
name matches, the server does not distinguish reads from writes.
Inline regions report ranges in the outer .rms file's coordinates, so
the editor highlights the source text as written.

