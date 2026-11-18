# nuance-in-game-facts: реализация 4 правил по in-game верификации (#99 #104 #105 #108)

Слоты nuance-конвейера, подтверждённые фактами 2026-11-18 (карты в
DE, итоги — комментарии в issues + `docs/ref/real-map-nuances.md`).
Один PR, коммит на issue (паттерн map-structure/auto-include).

## Current State

Механизм nuance-лестницы реализован: noop_attributes → probably-ignored
hint (данные), aliases → typo-alias warning, deprecated/legacy →
маркеры; corpus-гейт `TestCorpus_NoDuplicateDecls` — постоянная
приёмка analysis-слотов. Фикстуры — `nuance-examples/task.md` (все
проверены парсерами + in-game).

AST-разведка (probe): вложенный create_terrain парсится как
**Attribute** владельца create_terrain → правится данными; bare
terrain_type/base_terrain — Statement → код; create_land N —
Statement с Args → код.

## Description

- **#99 (данные)**: noop-пары `create_object×set_scale_by_size`,
  `create_object×set_scale_by_groups` → probably-ignored hint
- **#104 (данные)**: noop-пара `create_terrain×create_terrain` (вложенная
  форма — атрибут-паттерн в AST)
- **#108 (код)**: Statement с именем `terrain_type`/`base_terrain` →
  hint `misplaced-attribute` («атрибут, а не команда; перенесите внутрь
  create_terrain/create_elevation»); атрибутные вхождения не задеваются
  (другая ветка AST)
- **#105 (код)**: `create_land` с непустыми Args → hint
  `create-land-positional` («форма ломает создание ленда, in-game
  факт; конфигурация — внутрь блока») — только create_land
  (create_elevation N валиден, гайд:293)

## Result

(заполняется по завершении)
