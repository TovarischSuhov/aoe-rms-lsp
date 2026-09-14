# VS Code Extension — aoe2-lsp client wrapper

Usage rules for `editors/vscode/`: the TypeScript extension that wraps
the external `aoe2-lsp` binary in a `vscode-languageclient` session.
No Go, no CODEMANIFEST — this is an editor-side consumer of the
server cell's LSP surface (`internal/server/.usages/lifecycle.md`).

## Client bootstrap (src/extension.ts)

`LanguageClient` over stdio: the server command is whatever
`resolveServer` produced (explicit `aoe2lsp.serverPath`, cached
version, PATH hit or download — next section). The command never
reaches the server.

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
- `activate` is async: it awaits server resolution (next section)
  before constructing and starting the client; `deactivate` stops it.
  Activation is implicit through the `languages` contribution — no
  manual `activationEvents` needed on modern engines.
- A missing binary is a setup problem: `errorHandler.closed →
  CloseAction.DoNotRestart` plus one `showErrorMessage` pointing at
  the setting/download mode. Never crash-loop the extension host.
- The server is external: the extension does not bundle it — it
  downloads it on demand (next section). `aoe2lsp.serverPath` is the
  manual override for a self-built binary
  (`go build ./cmd/aoe2-lsp`).

## Server auto-download (src/install.ts)

`activate` awaits `resolveServer(context, mode)` and starts the client
with the resolved command. Priority order:

1. an explicitly set `aoe2lsp.serverPath` (any settings layer; the
   `aoe2-lsp` default does NOT count as explicit) — wins, zero network;
2. the newest cached version under
   `<globalStorage>/servers/<tag>/aoe2-lsp[.exe]` (tags compared
   numerically per segment: `v0.10.0` > `v0.9.0`);
3. an `aoe2-lsp` binary found by scanning `PATH`;
4. with `aoe2lsp.download.mode: "auto"` (the default): download the
   latest release of `TovarischSuhov/aoe-rms-lsp` from GitHub Releases.

`aoe2lsp.download.mode: "off"` stops after step 3 — no network call
ever leaves the extension.

Download pipeline (`installFromRelease`): pick the platform asset
(`aoe2-lsp-<goos>-<goarch>[.zip|.tar.gz]`; Node's `x64` arch maps to
Go's `amd64` naming), stream it into
`<storage>/servers/.tmp-<rand>/` (same filesystem as the final
location), verify SHA256 against the release's `SHA256SUMS`
(`<hex>␣␣<filename>` lines), extract with the system `tar -xf`
(bsdtar ships with Windows 10 1803+ and reads `.zip` as well),
`chmod 0o755` on unix, publish under `servers/<tag>/aoe2-lsp[.exe]`.
Failures are classified as `InstallError.kind`: `rate-limit` |
`network` | `unsupported-platform` | `checksum` | `archive` | `spawn`.

Contracts and failure policy:

- One network check per activation (`releases/latest`, ~10s
  `AbortSignal` timeout): its answer decides cache-hit vs download.
- The archive member name is an external contract of `release.yml`:
  members are `aoe2-lsp-<goos>-<goarch>[.exe]` at the archive root
  and get renamed to the suffix-free `aoe2-lsp[.exe]` on publish.
  Change the release naming and this rename breaks.
- Atomicity/races: work happens in `.tmp-*`, publish is a single
  rename. An existing `<tag>` binary means another window won the
  race — success, not an error; a `<tag>` dir without a binary is
  stale and gets replaced. The winner's prune may also delete a
  loser's in-flight `.tmp-*`: on a mid-install failure the loser
  re-checks the published binary and adopts it instead of erroring.
  A failed install leaves nothing behind (temp is cleaned in
  `finally`).
- Prune: after a successful resolve or install only the winning tag
  stays — older tags and stale `.tmp-*` dirs are removed.
- Rate limit (403/429 — 60 req/h unauthenticated): degrade silently to
  the newest cache, no error message. Every other failure kind
  degrades the same way (cache → PATH → bare command) with exactly one
  `showErrorMessage` per activation. Activation never throws.
- Progress: the download runs under `window.withProgress`
  (ProgressLocation.Window, non-cancellable — cancelling a
  half-written install buys nothing; cancellation is deliberately not
  implemented). `install.ts` reports absolute fractions 0→1 (download
  0→0.7, hash 0.7→0.8, extract 0.8→0.95), `extension.ts` converts
  them to `withProgress` increments.
- Proxy: downloads go through the extension host's patched `fetch`, so
  `http.proxySupport` applies. Worst case a hostile proxy environment
  breaks the GitHub API call — it degrades like any other `network`
  failure (one message, PATH fallback). Escape hatches: point
  `aoe2lsp.serverPath` at a manually installed binary, or set
  `aoe2lsp.download.mode: "off"`.
- The `vscode` import in `src/install.ts` is type-only on purpose: all
  VS Code surface (settings, progress UI, messages, output channel) is
  injected through the `wireEnv` seam, which is what lets the unit
  tests (`test/install.test.ts`, node:test) run the module under plain
  Node type stripping without an extension host. Keep it that way.

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

## Snippets

RMS snippets are a declarative contribution: `snippets/aoe2rms.json`
referenced from `contributes.snippets` for language `aoe2rms`. No client
code — VS Code resolves prefixes from the JSON alone.

- Content sources: the new-map skeleton (Zetnus guide,
  `docs/ref/zetnus-rms-guide.txt`) and frequent blocks from
  `docs/ref/map-scripting-practices.md` (create_object with fields,
  start_random, base_terrain, sections).
- Bodies use placeholders (`${1:default}`, choice `${1|a,b|}`); `$0` is
  the final cursor position.
- The skeleton snippet is guarded by a Go test
  (`editors/vscode/snippets_test.go`): expand placeholder defaults, feed
  the result through `rms.Parse` — no parse errors allowed. Change the
  snippet, the test keeps it valid.
- `.vscodeignore` is exclusion-style — a new `snippets/` directory is
  packaged automatically; the packaged-artifact contributes check (#69)
  must list `snippets[].path` alongside grammars and languages.

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

## Build & package

- `npm run compile` — esbuild bundles `src/extension.ts` to a single
  CJS `dist/extension.js` (`vscode` external, node platform).
- `npm run check` — `tsc --noEmit` type gate.
- `npx vsce package --no-dependencies` — produces
  `aoe2-lsp-<version>.vsix`; `--no-dependencies` is correct because
  esbuild already inlined `vscode-languageclient` into the bundle.
- CI builds the .vsix and uploads it as a workflow artifact
  (`aoe2-lsp-vsix`) on every push/PR. The Release workflow uploads
  only `dist/*` — the four platform archives plus `SHA256SUMS`; the
  `.vsix` is not a release asset and never lands in `SHA256SUMS`.
  Marketplace publishing stays out of scope.

### npm install hygiene

- `package.json` carries a filled `allowScripts` (pinned `pkg@version`
  entries for `esbuild`, `@vscode/vsce-sign`, `keytar`). npm 11 warns
  about — and npm 12 blocks — install scripts not covered by the
  policy: on a fresh environment without the field the postinstalls
  are silently skipped. When bumping one of these packages, refresh
  its pin (`npm approve-scripts <pkg>` rewrites it to the installed
  version).
- No lockfile is committed (the npm registry is unreachable from some
  dev machines), so drift of the pinned transitive versions is caught
  by the CI `vscode` job, not locally.
- A timed-out `npm install` leaves a broken `node_modules` — the
  telltale symptom is `Cannot find module 'es-errors/type'` from vsce
  on the next run. Re-running `npm install` does not repair it;
  the only recovery is a full removal and a clean install:
  `rm -rf node_modules && npm install`.

## Manual acceptance

`code --install-extension aoe2-lsp-<version>.vsix`, open a `.rms`
file, and check the scenario matrix (the Output channel "aoe2-lsp"
logs the resolved command and its origin):

- Clean machine — no `aoe2lsp.serverPath`, no binary on PATH, mode
  `auto`: the download progress shows, the binary lands in
  `<globalStorage>/servers/<tag>/aoe2-lsp`, hover/completion work.
- Second activation with a warm cache: no download (one latest-check
  per activation); a newer published release updates the cache and
  prunes the old `<tag>` dir.
- Explicit `aoe2lsp.serverPath`: that exact binary runs, network
  stays idle.
- `aoe2lsp.download.mode: "off"`: zero network — cache, then PATH,
  then the bare `aoe2-lsp` command.
- Broken network: exactly one error message, PATH/bare-command
  fallback, activation completes.

Windows specifics (bsdtar reading `.zip`, the `.exe` suffix, spawn
failure messages without tar) are a PR checklist item — CI and the
unit tests run Linux only.
