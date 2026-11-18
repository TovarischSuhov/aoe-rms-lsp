# Open a map script

Opening any `.rms` or `.xs` file completes this step.

The include paths resolve out of the box: the extension detects a Steam
installation of AoE2 DE and fills `aoe2lsp.includeRoots` with the game's
`resources/_common/random-map-scripts` and `resources/_common/xs`
(definitions, constants, `ailib/*.xs`) — unless you set the setting
yourself. Files `#include`d next to the map resolve relative to the map,
as the game resolves them.

`#include` to a file that does not exist yet is reported as an error
with a quick fix that creates the file.
