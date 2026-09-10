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

## Static highlighting

TextMate grammars color both languages without the server:
`syntaxes/aoe2rms.tmLanguage.json` (generated) and
`syntaxes/aoe2xs.tmLanguage.json` (hand-written), registered through
`contributes.grammars` (scopeName `source.aoe2rms` / `source.aoe2xs`).

The RMS grammar is a build artifact: `go run ./cmd/tmgen` regenerates
it from the embedded kb data. Its skeleton (comments `/* */`, `//` and
non-directive `#` lines; sections; `#` directives; strings; numbers)
is hand-written in `internal/highlight`; the three keyword classes
(commands, attributes, constants) are alternations rebuilt from kb.
Never edit the artifact by hand — change the skeleton and regenerate.
When docs/ref changes, regenerate in order: kbgen first, then tmgen
(see `internal/highlight/.usages/grammar-pipeline.md`).

Scope naming follows TextMate conventions and mirrors the server's
semantic-token legend where it can (`entity.name.section.aoe2rms` ↔
token type `section`): with the server running, semantic tokens refine
identifiers on top of the grammar's syntax layer. The XS grammar is
syntax-only — XS constants (cColorBlue and friends) are colored by
semantic tokens, not by the grammar.

## Packaging policy

- The extension version is owned by the release process:
  `scripts/release.sh` sets `version` in `package.json` to the release
  tag and commits it together with the changelog — the committed version
  always equals the last tag, and the packaged `.vsix` carries the
  release version. Never bump it by hand.
- The package must be self-contained: every path referenced from
  `contributes` (`grammars[].path`, `languages[].configuration`) has to
  exist inside the built `.vsix` — VS Code silently ignores dead
  references, so CI verifies them against the archive listing
  (see `github-actions.md`, "packaged-artifact contributes check").
  Keep `.vscodeignore` from excluding anything `contributes` points at
  (including `LICENSE`).
- A copy of the root `LICENSE` lives in `editors/vscode/` and
  `license` points at it as `"SEE LICENSE IN LICENSE"` — vsce resolves
  the license inside the package root without warnings.
- `capabilities`: `virtualWorkspaces: false` (the server is a native
  binary, unusable in vscode.dev/github.dev); `untrustedWorkspaces` —
  the server only parses files, it does not execute them.

## Build & package

- `npm run compile` — esbuild bundles `src/extension.ts` to a single
  CJS `dist/extension.js` (`vscode` external, node platform).
- `npm run check` — `tsc --noEmit` type gate.
- `npx vsce package --no-dependencies` — produces
  `aoe2-lsp-<version>.vsix`; `--no-dependencies` is correct because
  esbuild already inlined `vscode-languageclient` into the bundle.
- CI builds the .vsix (artifact on every push/PR); the release workflow
  attaches it to the GitHub Release next to the platform binaries —
  it lands in `SHA256SUMS` like every other asset. Marketplace
  publishing stays out of scope.

## Manual acceptance

`code --install-extension aoe2-lsp-<version>.vsix`, set
`aoe2lsp.serverPath` if the binary is not on PATH, open a `.rms`
file: hover/completion/diagnostics flow through the language client
(Output channel "aoe2-lsp" shows the session log).
