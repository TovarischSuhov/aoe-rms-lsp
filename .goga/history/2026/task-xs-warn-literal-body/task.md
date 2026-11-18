# xs: warn-литерал в вербатим-теле — вторая } уходит на верхний уровень (#163)

Зеркало — GitHub issue #163 (bug/P2/size:S-M/backlog).

## Current State

`void A(){0.!%{(}}`: вербатим-срез тела warning-стейтмента несёт обе
исходные `}`, а голова функции печатает свою — вторая `}` уходит на
верхний уровень, выход первого прохода не репарсится —
`unexpected "}" at top level`. Класс открытого перечня XS
(stability-invariants.md, .usages/formatting.md), найден
fuzz-бюджетом format-xs.

## Description

Корень — stmt-путь принтера `internal/format/xs.go`: warn-вербатим
(warnVerbatim/warnCutEnd-семейство) берёт срез, чей конец уже держит
закрыватель тела, и синтез головы добавляет второй. Фикс — в
internal/format (stmt-путь warn-срезов); internal/xs read-only.

## Scope

- `internal/format` (xs.go stmt/warn-путь; тесты xs_test.go, сид fuzz_test.go)
- перечни открытых классов (knowledge + .usages) — член удаляется тем же PR
- internal/xs read-only

## Acceptance Criteria

- пин-тест: `void A(){0.!%{(}}` сходится/отказывается осознанно,
  выход репарсится; пограничные ряды (закрытая форма, вложенные тела)
  без регрессии
- сид FuzzXS, зелёный сид-корпус, fuzz ≥2×60s `-parallel=2` под memory
  cap (крашеры открытых классов — сверять с перечнем, удалять,
  перезапускать до двух чистых прогонов)
- make check, goga contract internal/format
- corpus-гейт dispatch-ом

Ветвь `task/xs-warn-literal-body`, PR с `Fixes #163`.
