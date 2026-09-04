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
