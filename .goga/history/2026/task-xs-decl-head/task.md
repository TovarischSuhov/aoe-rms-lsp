# xs: decl-head — дегенерат заголовка декларации печатает хвост верхним уровнем (#162)

Зеркало — GitHub issue #162 (bug/P2/size:S-M/backlog).

## Current State

`void A,#A=0.`: pass 1 печатает `void A;` + хвост `A=0.` отдельным
элементом верхнего уровня (символ `#` теряется), выход первого прохода
не репарсится — `unexpected identifier "A" at top level` (line 3).
Класс открытого перечня XS (stability-invariants.md,
.usages/formatting.md), семейство хвоста деклараторной группы.

## Description

Корень — decl-путь принтера `internal/format/xs.go`: декларация с
группированным хвостом, у которого имя начинается с `#` (или иной
дегенерат), печатает голову как декларацию, а хвост — как statement
верхнего уровня.

## Scope

- `internal/format` (xs.go decl-путь; тесты xs_test.go, сид fuzz_test.go)
- перечни открытых классов (knowledge + .usages) — член удаляется тем же PR
- internal/xs read-only

## Acceptance Criteria

- пин-тест: `void A,#A=0.` сходится/отказывается осознанно, выход репарсится
- сид FuzzXS, зелёный сид-корпус, fuzz ≥2×60s под memory cap
- make check, goga contract internal/format
- corpus-гейт dispatch-ом

Ветвь `task/xs-decl-head`, PR с `Fixes #162`.
