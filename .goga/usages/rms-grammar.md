# RMS Grammar — AoE2 Definitive Edition

Domain: the Random Map Script language structure. Target audience: implementers
of the rms, analysis and kb cells. Sources: docs/ref/zetnus-rms-guide.txt
(canonical), docs/ref/aoe2de-xs-rms-changelog.md (versioning).

## File structure

A .rms file is a sequence of sections and top-level statements:

    <player_setup> ... </player_setup>
    <land_generation> ... </land_generation>
    <elevation_generation> <cliff_generation> <terrain_generation>
    <connection_generation> <objects_generation>

Statements outside sections are global. Section names live in angle brackets.

## Statements

    command arg1 arg2 ...           — e.g. create_elevator 7
    attribute_name value            — belongs to the PRECEDING command (positional semantics)
    start_random ... end_random     — random block:
        percent_chance 25 <statements>
    if <expr> ... [elseif ...] [else ...] endif   — conditionals

Attributes may interleave with a command's arguments only before the next
command starts. Comments: `/* ... */` (block), `//` and `#` line comments.

## Expressions (DE 141935/153015+)

- integers `7`, floats `3.5` (141935+), percents `50%`
- constants/identifiers: `TERRAIN_GRASS`, number_of_objects style names
- binary operators `+ - * /` (153015+): Expr tree, precedence arithmetic
- map-size scaling: `rand_float(a, b)` style helpers are ordinary commands

## Directives

    #include <file.rms>      — text include (rms.File.Includes)
    #includeXS <file.xs>     — inline XS block begins (rms.XsBlock)
    #includeXS               — bare directive switches the rest of file to XS
    #const NAME value        — script constant definition

## DE-era additions (must parse; see changelog for versions)

water_definition, create_object_group, create_connect_land_zones,
land_conformity, generate_mode, spacing_to_specific_terrain,
set_circular_base, require_path, override_map_size, cliff_type,
generate_for_first_land_only, set_facet. `effect_percent` is deprecated
in favor of operators — analysis flags it (warning).

## Testing fixtures

docs/ref corpus: snippets.aoe2map.net (actor areas, error handling),
aoe2map.net community maps. A parser must accept all of them without
false positives.
