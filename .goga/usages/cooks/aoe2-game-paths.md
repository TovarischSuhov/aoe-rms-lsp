# AoE2 DE game paths — install layouts and detection

Usage rules for anything in the repository that needs to know where the
game is installed or which of its folders matter (VS Code client
detection today, deploy-to-game later). Facts first, then the consumer
rules.

## Steam layout (verified on a live install, 2026-11-19)

Game root: `<steam-library>/steamapps/common/AoE2DE`.

Steam libraries: standard per-platform roots plus every `"path"` entry in
`<steam-root>/steamapps/libraryfolders.vdf` (modern format; values carry
doubled backslashes on Windows; the pre-2020 direct-number format is not
parsed).

Standard Steam roots:

| Platform | Root |
|---|---|
| Windows | `%ProgramFiles(x86)%\Steam`, `%ProgramFiles%\Steam` |
| macOS | `~/Library/Application Support/Steam` |
| Linux | `~/.steam/steam`, `~/.local/share/Steam` |

Include-worthy folders under the game root (what `aoe2lsp.includeRoots`
should hold — non-recursive search, exact directories):

- `resources/_common/random-map-scripts` — stock maps shipped as loose
  files
- `resources/_common/xs` — `Constants.xs`, `Effects.xs`, `x256tech.xs`,
  `x9tech.xs`, `ailib/*.xs` (`Geometry.xs` and friends for `#includeXS`)

**Spelling warning:** the folder is `resources/_common/...`, NOT
`resources_common/...`. The wrong spelling circulated in older docs
(package.json / README before issue #123) and in
`.goga/history/2026/task-vscode-hygiene/task.md`; the live install has no
`resources_common` directory. History files are left as-is.

## Detection rules (src/gamedetect.ts, extension.ts wiring)

- Candidate libraries: existing standard roots first, then vdf-listed
  paths of each existing root; dedupe case-insensitively (one Windows
  machine lists the same root through both Program Files flavors).
- First existing `<lib>/steamapps/common/AoE2DE` with at least one of the
  two resource folders wins; a partial install (game without the resource
  folders) falls through to the next candidate.
- The extension fills `aoe2lsp.includeRoots` (Global target) only when
  the user has not set the value at any level (`inspect()` global /
  workspace / folder — any explicit value wins, including `[]`).
- A not-found game stays silent: no error, no settings write. Detection
  failures never block activation (includeRoots is a convenience).

<<<<<<< Updated upstream
## MS Store / Xbox (issue #124, not implemented)
=======
## User mods layout (verified on a live install, 2026-11-19)

Per-user mods root: `%USERPROFILE%\Games\Age of Empires 2 DE\<account>\mods\{local,subscribed}\` — one directory per Steam account id (a
machine can hold several, including a bare `0`), each with `local` and
`subscribed`. Mod folders follow the game resources shape:
`<mod>\resources\_common\...` (subscribed mods verified: `drs`,
`info.json`, `thumbnail.jpg`); an RM mod's scripts live in
`<mod>\resources\_common\random-map-scripts`. Local mods appear the
moment their folder structure exists — deploy creates it on demand.

## MS Store / Xbox (issue #124 — deferred indefinitely, 2026-11-19)
>>>>>>> Stashed changes

Deferred by user decision: no live MS Store/Xbox install is available to
verify the layout, and unverified facts do not enter code or data (the
rule above). The packaged install has a different layout (no
`steamapps` nesting; the exact resources layout is unverified) — do not
reuse the Steam facts for it. Resume the slot from a live install when
one becomes available; detection on Steam machines is already complete
(#123).
