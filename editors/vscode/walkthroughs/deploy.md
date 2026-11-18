# Deploy the map to the game

Run **AoE2 LSP: Deploy map to game** (command palette, or this step's
button) with an `.rms` file open: the map and its local `#include`
dependencies are copied into a local mod's
`resources/_common/random-map-scripts` folder, recreating the relative
structure — the game resolves the includes the same way afterwards.

The destination comes from `aoe2lsp.deploy.targetDir`; left empty, a
single local mod under `%USERPROFILE%\Games\Age of Empires 2 DE` is
detected automatically. Enable the mod in the Mod Manager once — the
folder is read when a match starts.

This step completes when you run the deploy command.
