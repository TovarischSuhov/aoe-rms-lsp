# nuance-rules: микропачка — вердикты real-map-nuances в правила анализа

## Current State

Корпусная мудрость собрана, но не работает на пользователя:
`docs/ref/real-map-nuances.md` (прогон 100 карт 09.09.2026) содержит вердикты
✅/❓/✍️/💬 по конструкциям вне референса, однако правила анализа их не знают.
Парсер уже поддерживает ✅-синтаксис раздела 2 (фиксы PR #22, #24);
`analysis.Analyzer` умеет unknown-диагностики с did-you-mean (#28), но
«вероятно игнорируется», опечатки-алиасы, deprecated и XS-мусор отдельными
правилами не выражены. Слот #53 пачки ux-and-data-quality (M) ни разу не
брался — декомпозирован в эту микропачку (2026-09-22).

## Description

Микропачка из 15 слотов: один фундамент (схема kb-данных + механизмы правил +
корпус-калибровка) и 14 микрозадач — по одной на паттерн справочника.
Зернистость — по паттернам (решение пользователя 2026-09-22): строки таблиц
разделов 3–4 с общим механизмом проверки сгруппированы; внутри микрозадачи
кураторство каждой пары команда×атрибут идёт построчно.

Каждая микрозадача: кураторство (подтвердить/опровергнуть по источникам вне
Зетнуса — форум World's Edge, тесты в игре; либо добавить атрибут в kb-данные
для команды, либо пометить no-op) → данные → включение правила →
фикстура/корпус-прогон.

## Scope

**In scope (слоты):**

| # | Слот | Issue | Паттерн (строки справочника) | Вердикт → severity |
|---|------|-------|------------------------------|--------------------|
| 0 | `nuance-engine` | #97 | фундамент: схема kb-данных нюансов (no-op списки команда×атрибут, алиасы, deprecated/легаси-метки) + механизмы правил «вероятно игнорируется»=hint, ✍️=warning, deprecated=warning; severityOverrides; корпус-калибровка — ✅-конструкции раздела 2 молчат | — |
| 1 | `nuance-include-drs` | #98 | `#include_drs` — легаси-директива, 78× / 45 карт (разд. 1) | ❓ hint |
| 2 | `nuance-object-scaling` | #99 | `set_scale_by_size`/`set_scale_by_groups` на create_object, 104× / 4 (разд. 3, 2 строки) | ❓ hint |
| 3 | `nuance-land-attrs-on-terrain` | #100 | create_land-атрибуты на create_terrain: `border_fuzziness`, `other_zone_avoidance_distance`, `left/right/top/bottom_border`+`base_size`, `number` (5 строк) | ❓ hint |
| 4 | `nuance-elevation-attrs` | #101 | `clumping_factor`, `spacing_to_other_terrain_types` на create_elevation (2 строки) | ❓ hint |
| 5 | `nuance-object-attrs` | #102 | `set_avoid_player_start_areas`, `terrain_to_build_on`, `temp_min_distance_to_players`, `base_terrain` на create_object (4 строки) | ❓ hint |
| 6 | `nuance-terrain-cross-attrs` | #103 | `distance_to_other_terrain_types`, `min/max_distance_to_players` на create_terrain (2 строки) | ❓ hint |
| 7 | `nuance-nested-create-terrain` | #104 | вложенный `create_terrain` (1 строка) | ❓ hint |
| 8 | `nuance-land-positional` | #105 | `create_land N` с позиционным аргументом (гайд-скелет — без аргументов) | ❓ hint |
| 9 | `nuance-typo-aliases` | #106 | опечатки ✍️: cliff-семейство (`min_length_cliff` и др., 2 строки), `group_varience`, `clumbing_factor`, `set_avoid_play_start_areas` — точные алиасы «правильно: X» | ✍️ warning |
| 10 | `nuance-effect-percent` | #107 | `effect_percent` deprecated с DE, 36× / 3 карты — «use effect_amount instead» | warning |
| 11 | `nuance-bare-terrain-cmds` | #108 | `terrain_type`/`base_terrain` как команды вне create-контекста (в start_random-ветках), 50× — мёртвый код | ❓/💬 hint |
| 12 | `nuance-xs-const-local` | #109 | `const int x = …` внутри тела функции — поддержка движком не проверена | ❓ hint |
| 13 | `nuance-xs-types` | #110 | `long`/`double`/`char` — не работают (UGC Guide bugged-кейс) | 💬 warning |
| 14 | `nuance-xs-goto` | #111 | `goto` — отсутствует в XS (UGC Guide bugged-кейс) | 💬 warning |

**Out of scope:**

- 💬 французский текст вне сломанного комментария (разд. 4) — находка
  парсера-диагностики, не правило нюансов
- ✅-конструкции раздела 2 — уже поддержаны парсером; для них только
  отрицательная калибровка (молчание) в `nuance-engine`
- webview/nvim-lspconfig и прочие 1.x-кандидаты

## Acceptance Criteria

Общие для каждого слота:

- `make check` зелёный; `goga contract` затронутых ячеек зелёный
- severityOverrides (`none`) подавляет каждую новую диагностику
- корпус-прогон: находки справочника дают заданный severity на картах-носителях;
  ✅-конструкции не шумят

`nuance-engine` дополнительно: схемы данных и правила покрыты тестами на
синтетических фикстурах; последующие слоты не меняют механизмы, только данные.

## Stack

- **Frameworks:** Go 1.26+ (stdlib)
- **Libraries:** без новых зависимостей; ячейки `kb` (modify: данные нюансов),
  `analysis` (modify: правила), `xs`/`rms` — read-only (AST уже несёт нужное)
- **Infrastructure:** корпус-гейт CI (как приёмка analysis-слотов, без изменений)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| — | — | нет: внешние источники (форум World's Edge, UGC Guide) — объекты кураторства, не инструменты |

## Risks and Constraints

- Кураторство ❓-вердиктов внешними источниками может не дать однозначного
  ответа — тогда решение «no-op hint с формулировкой не документировано»
  фиксируется как есть, с пометкой источника в данных
- Шум на корпусе — главный риск severity; пороги решает `nuance-engine`,
  микрозадачи наследуют
- `nuance-typo-aliases` пересекается с did-you-mean (#28): алиас даёт точное
  соответствие, did-you-mean остаётся фолбэком — механика разделения на
  design-этапе слота
- Правила не должны менять парсеры: только данные kb и проверки analysis

## Scope Estimate

Мультизадача: 15 слотов (1 фундамент M-ish + 14 микрозадач S), каждая —
отдельная ветка `task/<имя-слота>` → PR → свой issue.

Порядок: `nuance-engine` первым; микрозадачи 1–14 — параллельно после него,
внутри — по интересу (приоритет по частоте в корпусе: object-scaling 104×,
include-drs 78×/45 карт, bare-terrain-cmds 50×). #53 закрывается последним
смерженным слотом.

## Existing Architecture

- `analysis.Analyzer` — паттерн добавления проверок (прецеденты:
  duplicate-include, unknown-* + did-you-mean)
- `kb.Store` — данные: команды/атрибуты из GenKB; сюда ложатся no-op списки,
  алиасы, deprecated/легаси-метки
- корпус-прогон (`cmd/corpus` + CI-гейт) — приёмка без шума
- `docs/ref/real-map-nuances.md` — источник вердиктов; `zetnus-rms-guide.txt`
  и UGC Guide — референсы для кураторства

## Notes

Решения сессии (2026-09-22, пользователь):

- #53 декомпозирован в микропачку; зернистость — по паттернам (строгий
  построчный вариант ~27 задач отклонён как раз).
- Вне пачки: 💬-мусор вне сломанного комментария, ✅-синтаксис (уже в парсере)
- Каждый слот зеркалится issue со ссылкой на этот task.md; родитель #53
  закрывается последним слотом
- Перенос из пачки ux-and-data-quality отмечен в её task.md
