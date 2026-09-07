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

## Editor configs (see task README)
- Neovim: lspconfig, cmd = aoe2-lsp binary, filetypes = { "aoe2rms", "aoe2xs" }
- VS Code: generic LSP extension launching the binary over stdio

Preconditions:
- Single binary, no flags required for MVP; positionEncoding negotiated
  in Initialize (prefer utf-8 when offered, per `lsp-protocol`).

## Advertised capabilities

Initialize advertises: diagnostics (Full sync + OpenClose), hover,
completion, and navigation — definition, references, documentSymbol.
Editor configs need no extra flags; positionEncoding is negotiated
(prefer utf-8 when the client offers it).

## Cross-file navigation

Definition and references resolve across the document's include closure:
targets may live in files that are not open in the editor — the server
loads them from disk on demand (editor state always wins for open files).
Missing #include / #includeXS targets surface as "missing-include"
diagnostics on the directive's path range.

Preconditions:
- Include paths resolve relative to the including file's directory; the
  game's installation root is not searched.
- Files changed outside the editor without a size/mtime change are not
  reloaded (no file watcher).

