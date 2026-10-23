---
type: Game-behavior
title: Резолюция #include_drs в AoE2 DE
description: Как движок DE резолвит #include_drs — по имени из resources/_common/drs/gamedata_x2/ и по относительным путям рядом с картой; числовой id — легаси-адресация DRS-архива. Опора для include-замыкания и будущих game-detect includeRoots (#82/#123).
sources:
  - resource: docs/ref/real-map-nuances.md
  - resource: docs/ref/nuance-maps/nuance-98-include-drs.rms
  - resource: internal/rms/parse.go
  - resource: internal/include/resolver.go
generated:
  by: claude-code/sonnet-4-6
  at: 2026-10-23T23:50:00Z
---

# Резолюция #include_drs в AoE2 DE

`#include_drs <файл> <id>` — не легаси и не no-op: в DE это основная форма
подключения RMS-фрагментов. Верифицировано первушкой на карте пользователя
(23.10.2026, слот nuance-rules #98, issue #98).

## Что известно

- **Цели лежат развёрнутыми файлами** в установке DE:
  `resources/_common/drs/gamedata_x2/` — 307 файлов, включая
  `random_map.def` и `std_resources.inc`; каталог называется `drs`, но
  это не DRS-архив, а каталог с распакованными включениями.
- **Резолюция по имени файла** (следствие: файлы развёрнуты, id без
  пары в каталоге). Числовой id — адресация DRS-архива AoC/HD;
  игнорируется ли он движком DE — проверяется картой
  [nuance-98-include-drs.rms](/docs/ref/nuance-maps/nuance-98-include-drs.rms)
  (шаг 2, id → 99999), на 23.10.2026 не проведена.
- **Относительные пути рядом с картой тоже резолвятся**: официальная
  Oasis.rms подключает `#include_drs includes/scaling.inc` и т.п.
- **Масштаб**: в официальном корпусе (`.corpus-official`) — 2649
  использований, bare `#include` — 0; в публичном корпусе (`.corpus`) —
  78 использований в 45 из 100 карт.
- **Содержимое значимо**: `std_resources.inc` — create_object-блоки
  (ратуша, жители, скаут, реликвии) спецресурсов Gold Rush / Crater Lake;
  `random_map.def` — библиотека `#const` (имена карт и прочее).

## Что из этого следует LSP

- Парсер: `#include_drs` кладётся в `file.Includes` наравне с `#include`
  (`internal/rms/parse.go`, PR #149) — замыкание, навигация и
  `MissingInclude` работают без различения форм.
- Резолвер (`internal/include/resolver.go`) ищет цели в workspace;
  авторезолв из каталога установки — задача game-detect `includeRoots`
  (issues #82/#123): путь `resources/_common/drs/gamedata_x2/` — первый
  кандидат в корни включений, рядом с `includes/` самой карты.
- Правило-нюанс (hint «легаси») не заводилось: опровергнуто; при
  подтверждении игнорирования id шагом 2 карты можно рассмотреть
  аргументный hint на id — но маркеры kb ключуются по имени, а не по
  аргументу (см. `internal/kb/store.go`, `indexNuances`).
