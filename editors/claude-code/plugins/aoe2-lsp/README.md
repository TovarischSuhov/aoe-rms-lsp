# aoe2-lsp — Claude Code plugin

Wires the `aoe2-lsp` language server into Claude Code so the built-in `LSP`
tool provides code intelligence for AoE2 DE RMS and XS files in `editors/`.
Mirrors `editors/vscode`.

## Supported extensions

`.rms`, `.xs`

## Requirement

The `aoe2-lsp` binary must be on `PATH` (e.g. installed to `/usr/local/bin`):

```bash
go build -o /usr/local/bin/aoe2-lsp ./cmd/aoe2-lsp   # from the repo root
```

## Enable in Claude Code

From this directory's marketplace (`editors/claude-code/`):

```bash
claude plugin marketplace add /path/to/aoe-rms-lsp/editors/claude-code
claude plugin install aoe2-lsp@aoe2-local
```

Then reload: `/reload-plugins --force` (or restart Claude Code).

For a one-off session without registering a marketplace:

```bash
claude --plugin-dir /path/to/aoe-rms-lsp/editors/claude-code/plugins/aoe2-lsp
```

## What you get

`goToDefinition`, `findReferences`, `hover`, `documentSymbol`,
`workspaceSymbol` and diagnostics for `.rms`/`.xs`, resolved across the
`#include`/`#includeXS` closure.
