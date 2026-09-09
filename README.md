# aoe2-lsp — Language Server for AoE2 RMS + XS

[![CI](https://github.com/TovarischSuhov/aoe-rms-lsp/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/TovarischSuhov/aoe-rms-lsp/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/TovarischSuhov/aoe-rms-lsp)](https://github.com/TovarischSuhov/aoe-rms-lsp/releases/latest)
[![Coverage](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2FTovarischSuhov%2Faoe-rms-lsp%2Fbadges%2Fcoverage.json)](https://github.com/TovarischSuhov/aoe-rms-lsp/actions/workflows/ci.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/TovarischSuhov/aoe-rms-lsp)](go.mod)
[![Downloads](https://img.shields.io/github/downloads/TovarischSuhov/aoe-rms-lsp/total)](https://github.com/TovarischSuhov/aoe-rms-lsp/releases)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)

Language Server Protocol implementation (Go, stdio) for two Age of Empires II:
Definitive Edition languages:

- **RMS** — Random Map Scripts (section-declarative map generation language)
- **XS** — External Subroutines (C-like scripting used inside RMS and scenarios)

The server is editor-agnostic; a VS Code extension ships in-tree
([`editors/vscode`](editors/vscode)) and Neovim wiring is documented below.
Editor-integration details (capabilities, settings, cross-file specifics):
[`internal/server/.usages/lifecycle.md`](internal/server/.usages/lifecycle.md).

No LSP tooling existed for these languages: the only RMS linter (mangudai) has
a pre-DE grammar, and fresh editor extensions provide highlighting only.

## Features

- **Diagnostics** — syntax (error-recovery parsers) and semantic (unknown
  commands/attributes/symbols, arity, value types); missing
  `#include`/`#includeXS` targets are reported on the directive's path.
  Per-code severity overrides via settings (`none` suppresses a diagnostic).
- **Quick fixes** — "Change to X" renames suggested from the diagnostic
  message for unknown identifiers, `effect_percent` → `effect_amount`
  replacement, stub creation for missing includes.
- **Hover** — signatures and descriptions from the embedded knowledge base.
- **Completion** — RMS commands and attributes (context-aware: section,
  argument position), XS functions/constants, visible params and locals of the
  enclosing scope.
- **Signature help** — argument hints on `(` and `,`.
- **Navigation** — go-to-definition, references and document symbols across
  the include closure (targets may live in files not open in the editor);
  in-file document highlight; fuzzy workspace symbol search over open
  documents and their closures.
- **Semantic tokens** — full-document highlighting with a server-side legend
  `known, unknown, deprecated, section, kind` for RMS
  command/attribute/constant names and XS identifiers.
- **Folding ranges** — every outline node spanning more than one line (RMS
  sections, XS declarations).
- **Live config** — settings changes re-publish diagnostics without a
  restart; file watchers force-reload changed on-disk includes.

## Project layout

```
cmd/            thin entrypoints: aoe2-lsp (server), kbgen (KB regeneration),
                corpus (corpus-gate runner)
internal/
  common/       positions, ranges, diagnostics, outline symbols, tokens
                (shared data types)
  kb/           knowledge base: embedded JSON (XS functions, constants, RMS
                commands) + data pipeline (GenKB extraction from docs/ref)
  rms/          RMS parser (lexer → AST with error recovery)
  xs/           XS parser (C-like grammar, rules/events, externs)
  analysis/     semantic checks (unknown symbols, arity, value types,
                semantic tokens)
  hints/        signature-help computation (protocol-agnostic)
  complete/     completion candidates (protocol-agnostic)
  include/      #include/#includeXS closure, cross-file navigation
  corpus/       running the LSP binary over a sample of real maps
  server/       LSP server over go.lsp.dev/protocol (stdio)
editors/vscode/ VS Code extension (language configs + LSP client wiring)
scripts/        release.sh (version + changelog + tag), corpus-fetch.sh
                (deterministic corpus sample, pinned in corpus-sources.txt)
docs/
  tasks/        task definitions & acceptance criteria
  plans/        execution plans (build-lsp-rms-xs.md is the root one)
  arch/         architecture plans (cells, CODEMANIFEST contracts)
  design/       design documents per feature
  reviews/      review notes (incl. corpus runs)
  ref/          local copies of all data sources (see below)
```

The repository follows the [goga](https://pypi.org/project/goga/) CODEMANIFEST
workflow: each package is a cell with a `CODEMANIFEST` contract and `.usages/`
consumer docs; `.goga/usages/` holds project-wide practices (Go conventions,
RMS/XS grammars, go.lsp.dev patterns).

## Data sources

All sources are vendored locally under `docs/ref/` (no network at build or
runtime).

| Source | What it provides | License | Link |
|---|---|---|---|
| **AoE2DE UGC Guide** (Divy1211 et al.) | `xs-functions.json` (204 XS functions), `xs-constants.json` (27 sections), `prelude.xs` dump, XS language docs | **GPL-3.0** | [github.com/Divy1211/AoE2DE_UGC_Guide](https://github.com/Divy1211/AoE2DE_UGC_Guide) · [ugc.aoe2.rocks](https://ugc.aoe2.rocks/general/xs/) |
| **Zetnus — Definitive Random Map Scripting Guide** | RMS commands/attributes reference (Syntax Skeleton, Constant Reference) | no explicit license; used factually with attribution | [Google Doc](https://docs.google.com/document/d/1jnhZXoeL9mkRUJxcGlKnO98fIwFKStP_OBozpr0CHXo/edit) · [forum thread](https://forums.ageofempires.com/t/definitive-random-map-scripting-guide/104902) |
| **Official AoE2 DE release notes** (World's Edge) | version metadata ("since update N") for KB entries (`docs/ref/aoe2de-xs-rms-changelog.md`) | quoted factually | [ageofempires.com/news](https://www.ageofempires.com/news/) |
| **aoe2map.net / snippets** (siegeengineers) | real-world RMS/XS corpus: parser fixtures, the corpus gate sample, research notes | community content | [aoe2map.net](https://aoe2map.net/) · [snippets.aoe2map.net](https://snippets.aoe2map.net/) |

Derived knowledge-base files (`internal/kb/data/xs-functions.json`,
`internal/kb/data/xs-constants.json`) are adapted from the UGC Guide and are
covered by GPL-3.0 accordingly. Corpus-derived research (undocumented
real-map nuances, authoring practices — `docs/ref/real-map-nuances.md`,
`docs/ref/map-scripting-practices.md`) is the project's own, collected from
published maps with attribution.

## Install & run

Prebuilt binaries (Windows, Linux, macOS Intel/Apple Silicon) are attached to
[GitHub Releases](https://github.com/TovarischSuhov/aoe-rms-lsp/releases) —
archives plus `SHA256SUMS`; releases are built and published by CI when a
`v*` tag is pushed. Check the version with `aoe2-lsp --version`.

Cutting a release (from a clean, synced master): `make release BUMP=patch`
(or `VERSION=vX.Y.Z`) — the script prepends the changelog built from the
conventional commits since the last tag to `CHANGELOG.md`, commits, tags
and pushes; CI does the rest. `scripts/release.sh <spec> --dry-run` shows
what would be released without changing anything.

Or build from source; requires Go 1.26+.

```sh
go build -o aoe2-lsp ./cmd/aoe2-lsp
./aoe2-lsp   # speaks LSP over stdio; logs go to stderr
```

No other flags: capabilities are declared in `initialize` — full-text sync,
hover, completion, signature help, definition, references, document and
workspace symbols, document highlight, semantic tokens, folding ranges and
quick fixes. Position encoding is negotiated per client (utf-8 preferred).

## Debugging

Launch the server with `-debug` to get verbose tracing on stderr (stdout
stays reserved for the protocol):

```sh
./aoe2-lsp -debug 2>lsp.log
```

Debug events cover the lifecycle (`serve started`, `initialized`,
`settings applied`, `watchers registered`) and every request (`did_open`,
`did_change`, `did_close`, `diagnostics`, `hover`, `completion`,
`signature_help`, `definition`, `references`, `document_highlight`,
`document_symbol`, `workspace_symbol`, `semantic_tokens`, `folding_ranges`,
`code_action`) with metadata only — uri, version, sizes, counts; document
contents never enter the logs. Without the flag the server stays silent
apart from errors.

## Editor setup

Both editors talk to the same binary over stdio and share the `aoe2lsp`
settings section:

```jsonc
{
  "aoe2lsp": {
    "diagnostics": {
      "severityOverrides": { "undefined-symbol": "hint" }  // code → error|warning|info|hint|none
    },
    "includeRoots": ["/abs/path/to/ai-rms"]  // extra #include search dirs, e.g. the game folder
  }
}
```

`none` suppresses a diagnostic code entirely; settings apply without a
restart. VS Code reads this from its settings; Neovim delivers it via the
lspconfig `settings` table.

### VS Code (bundled extension)

The extension in `editors/vscode/` registers the `aoe2rms`/`aoe2xs` languages
(brackets, comments), launches the binary and forwards the settings. It is
not on the Marketplace yet — package and install it locally:

```sh
cd editors/vscode
npm install
npm run package            # dist/extension.js + aoe2-lsp-0.1.0.vsix
code --install-extension aoe2-lsp-0.1.0.vsix
```

A prebuilt `.vsix` is also attached to every CI run (artifact
`aoe2-lsp-vsix`). The server binary itself must be installed separately and
reachable via `aoe2lsp.serverPath` (default: `aoe2-lsp` on `PATH`).

### Neovim (nvim-lspconfig)

Register the server manually (custom server, not shipped with lspconfig) —
e.g. in `init.lua`:

```lua
local lspconfig = require('lspconfig')
local configs = require('lspconfig.configs')

if not configs.aoe2 then
  configs.aoe2 = {
    default_config = {
      cmd = { '/path/to/aoe2-lsp' },
      filetypes = { 'aoe2rms', 'aoe2xs' },
      root_dir = function(fname)
        return lspconfig.util.find_git_ancestor(fname)
      end,
      settings = {
        aoe2lsp = {
          includeRoots = { '/path/to/ai-rms' },
        },
      },
    },
  }
end

lspconfig.aoe2.setup({})
```

Map the file extensions to the filetypes (e.g. in `filetype.lua` or via
`vim.filetype.add`):

```lua
vim.filetype.add({
  extension = {
    rms = 'aoe2rms',
    xs = 'aoe2xs',
  },
})
```

## Development

Go 1.26+, stdlib first. The standard pre-completion check sequence:

```sh
make check    # fmt → build → test → lint → goga lint
```

Local `go test` runs must stay under a memory cap (a runaway test once
OOM'd a dev machine) — `make test` wraps `go test ./...` in a
`systemd-run` cgroup sandbox; race detector runs live in CI.

Two gates beyond unit tests:

- `make corpus` — builds the server and runs it over a deterministic sample
  of 100 published RMS/XS scripts (`scripts/corpus-fetch.sh` downloads the
  pinned pool on first use; CI caches it). A parser regression on
  real-world scripts fails the gate.
- CI (`.github/workflows/ci.yml`) — goimports/gofumpt, `go test -race`,
  golangci-lint, govulncheck, VS Code extension type-check + `.vsix`
  packaging, and the corpus gate on master pushes.

The embedded knowledge base is regenerated from `docs/ref/` with
`go run ./cmd/kbgen` (see `internal/kb/.usages/data-pipeline.md`).

## License

**GPL-3.0** — see [LICENSE](LICENSE).

The project is licensed under GPL-3.0 as a whole because its embedded
knowledge-base data derives from the GPL-3.0 AoE2DE UGC Guide. Zetnus guide
material is used factually (command/attribute structure) with attribution.

Age of Empires II: Definitive Edition is a product of World's Edge / Xbox Game
Studios; this project is not affiliated with or endorsed by them.
