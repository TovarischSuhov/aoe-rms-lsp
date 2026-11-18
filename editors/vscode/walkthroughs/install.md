# The server installs itself

On activation the extension resolves the `aoe2-lsp` binary in this order:

1. an explicit `aoe2lsp.serverPath` setting (for a self-built binary),
2. the newest cached version in the extension's global storage,
3. a hit on `PATH`,
4. — with `aoe2lsp.download.mode: "auto"` (the default) — the latest
   GitHub Release, SHA256-verified against `SHA256SUMS` and extracted
   with the system `tar` into a per-version cache.

Watch it happen: click the status-bar item (the output channel shows the
resolution origin: `setting` / `cache` / `path` / `download`). Set
`aoe2lsp.download.mode` to `"off"` for fully offline use.

The status bar also shows the server state (starting / running /
stopped) and the server version once it is up.
