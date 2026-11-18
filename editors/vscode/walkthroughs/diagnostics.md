# Read the diagnostics

While you edit, the language server checks the file the way the game
does:

- **unknown commands, sections and attributes** — with a did-you-mean
  suggestion when a known name is close;
- **argument counts and value ranges** against the game's command
  reference;
- **XS types** — incompatible call arguments, assignments and return
  values;
- **game-verified nuances** — attributes the engine silently ignores,
  deprecated `effect_percent`, `create_land` with a positional argument,
  bare `terrain_type`/`base_terrain` statements, transitive duplicate
  includes.

Every diagnostic has a stable code; silence one via
`aoe2lsp.diagnostics.severityOverrides` (`"none"` hides a code
entirely). Check this step off once you have seen diagnostics in a
map file.
