# VS Code Extension — aoe2-lsp client wrapper

Usage rules for `editors/vscode/`: the TypeScript extension that wraps
the external `aoe2-lsp` binary in a `vscode-languageclient` session.
No Go, no CODEMANIFEST — this is an editor-side consumer of the
server cell's LSP surface (`internal/server/.usages/lifecycle.md`).

## Client bootstrap (src/extension.ts)

`LanguageClient` over stdio: the server command comes from the
client-side `aoe2lsp.serverPath` setting (bare name → PATH, absolute
path works). The path never reaches the server.

```ts
const serverOptions: ServerOptions = {
  command: serverPath, args: [], transport: "stdio" as const,
};

const clientOptions: LanguageClientOptions = {
  documentSelector: [{ language: "aoe2rms" }, { language: "aoe2xs" }],
  outputChannelName: "aoe2-lsp",
};
```

Rules:
- `activate` starts the client; `deactivate` stops it. Activation is
  implicit through the `languages` contribution — no manual
  `activationEvents` needed on modern engines.
- A missing binary is a setup problem: `errorHandler.closed →
  CloseAction.DoNotRestart` plus one `showErrorMessage` pointing at
  the setting. Never crash-loop the extension host.
- The server is external: the extension does NOT bundle or download
  it (MVP scope). Users build `go build ./cmd/aoe2-lsp` or install a
  release binary and point `aoe2lsp.serverPath` at it.

## Settings pass-through

VS Code answers the server's `workspace/configuration` pull from
contributed properties automatically: the server asks for the
`"aoe2lsp"` section and receives
`{ diagnostics: { severityOverrides: ... }, includeRoots: [...] }` —
exactly the shape the server settings pipeline expects. Contribute
new server settings as `aoe2lsp.*` properties; do not hand-map them
in client code.

## Language configuration

`aoe2rms` (.rms): block comments `/* */` only — `#` lines are
directives, not comments. `aoe2xs` (.xs): C-like (`//`, `/* */`,
brace/paren/bracket pairs, indent rules). Keep the configs as JSON
files referenced from `contributes.languages[].configuration`.

## Build & package

- `npm run compile` — esbuild bundles `src/extension.ts` to a single
  CJS `dist/extension.js` (`vscode` external, node platform).
- `npm run check` — `tsc --noEmit` type gate.
- `npx vsce package --no-dependencies` — produces
  `aoe2-lsp-<version>.vsix`; `--no-dependencies` is correct because
  esbuild already inlined `vscode-languageclient` into the bundle.
- CI builds the .vsix and uploads it as an artifact; marketplace
  publishing stays out of scope.

## Manual acceptance

`code --install-extension aoe2-lsp-<version>.vsix`, set
`aoe2lsp.serverPath` if the binary is not on PATH, open a `.rms`
file: hover/completion/diagnostics flow through the language client
(Output channel "aoe2-lsp" shows the session log).
