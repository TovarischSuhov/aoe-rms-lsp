# aoe2-lsp — Language Server for AoE2 RMS + XS

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)

Language Server Protocol implementation (Go, stdio) for two Age of Empires II:
Definitive Edition languages:

- **RMS** — Random Map Scripts (section-declarative map generation language)
- **XS** — External Subroutines (C-like scripting used inside RMS and scenarios)

MVP feature set: **diagnostics** (syntax + semantic), **hover** (signatures +
descriptions from the knowledge base), **completion** (RMS
commands/attributes, XS functions/constants, visible params/locals),
**signature help** (argument hints on `(` and `,`), **navigation** —
go-to-definition, references and document symbols across the include
closure. Editor-agnostic: connection configs for Neovim
(lspconfig) and VS Code (generic LSP extension) — see
[`server/.usages/lifecycle.md`](server/.usages/lifecycle.md).

No LSP tooling existed for these languages: the only RMS linter (mangudai) has
a pre-DE grammar, and fresh editor extensions provide highlighting only.

## Project layout

```
cmd/       thin entrypoints: aoe2-lsp (server binary), kbgen (KB regeneration)
common/    positions, ranges, diagnostics, outline symbols (shared data types)
kb/        knowledge base: embedded JSON (XS functions, constants, RMS commands)
           + data pipeline (GenKB extraction from the reference docs)
rms/       RMS parser (lexer → AST with error recovery)
xs/        XS parser (C-like grammar, rules/events, externs)
analysis/  semantic checks (unknown symbols, arity, value types)
hints/     signature-help computation (protocol-agnostic)
complete/  completion candidates (protocol-agnostic)
include/   #include/#includeXS closure, cross-file navigation
server/    LSP server over go.lsp.dev/protocol (stdio)
docs/
  tasks/     task definitions & acceptance criteria
  plans/     execution plans (build-lsp-rms-xs.md is the root one)
  arch/      architecture plans (cells, CODEMANIFEST contracts)
  design/    design documents per feature
  reviews/   review notes
  ref/       local copies of all data sources (see below)
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
| **Official AoE2 DE release notes** (World's Edge) | version metadata ("since update N") for KB entries | quoted factually | [ageofempires.com/news](https://www.ageofempires.com/news/) |
| **aoe2map.net / snippets** (siegeengineers) | real-world RMS corpus for parser fixtures | community content | [aoe2map.net](https://aoe2map.net/) · [snippets.aoe2map.net](https://snippets.aoe2map.net/) |

Derived knowledge-base files (`kb/data/xs-functions.json`,
`kb/data/xs-constants.json`) are adapted from the UGC Guide and are covered by
GPL-3.0 accordingly.

## License

**GPL-3.0** — see [LICENSE](LICENSE).

The project is licensed under GPL-3.0 as a whole because its embedded
knowledge-base data derives from the GPL-3.0 AoE2DE UGC Guide. Zetnus guide
material is used factually (command/attribute structure) with attribution.

Age of Empires II: Definitive Edition is a product of World's Edge / Xbox Game
Studios; this project is not affiliated with or endorsed by them.

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
hover, completion, signature help, definition, references and document
symbols. Position encoding is negotiated per client (utf-8 preferred).

## Editor setup

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

### VS Code (generic LSP extension)

Install a generic LSP client extension (e.g.
[`vscode-glsl-linter`-style adapters or `lsp-vscode`](https://marketplace.visualstudio.com/items?itemName=llllvvuu.lsp-vscode))
and point it at the binary over stdio. With
[lsp-vscode](https://marketplace.visualstudio.com/items?itemName=llllvvuu.lsp-vscode),
in `settings.json`:

```jsonc
{
  "lsp-vscode": {
    "languages": ["aoe2rms", "aoe2xs"],
    "servers": {
      "aoe2-lsp": {
        "module": "/path/to/aoe2-lsp",
        "args": [],
        "transport": "stdio"
      }
    }
  }
}
```

Associate the extensions in `files.associations`:

```jsonc
{
  "files.associations": {
    "*.rms": "aoe2rms",
    "*.xs": "aoe2xs"
  }
}
```

