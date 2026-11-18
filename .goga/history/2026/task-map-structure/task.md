# map-structure: структурные линты секций + quickfix вставки (#112 + #113)

Слоты пачки ux-and-data-quality (#55, разбит 2026-09-22). #113 «builds
on the diagnostics slot» — без диагностик quickfix не к чему
прикрепить, поэтому один PR с двумя атомарными коммитами закрывает
оба issue.

## Current State

Корпусная разведка (100 карт, 2026-11-18) скорректировала formulation
ux-wave-2 — три из четырёх задуманных проверок оказались легитимными
паттернями:

- **Секции называются `*_GENERATION`** (TERRAIN_GENERATION и пр.)
- **Дубликаты секций — норма**: 7 валидных карт переоткрывают секции
  (вторые проходы ELEVATION/TERRAIN/CLIFF/CONNECTION/LAND; стили
  Black Forest, Yucatan, Fortress, KHR WSVG) → проверку duplicate
  **не делаем** (был бы сплошной шум)
- **PLAYER_SETUP необязательна**: 29 карт без неё (28 — фрагменты
  с 0–3 заголовками, 1 — включаемый SNIPP@-кусок с 3 секциями);
  **PLAYER_SETUP без create_player — тоже норма**: 3 опубликованные
  рабочие карты (AM@Islabia, RandomSomethingExtreme, bohemia) →
  проверку players **не делаем**
- **TERRAIN_GENERATION**: отсутствует только у 6 карт — 5 фрагментов
  (0–3 заголовка) + 020 SNIPP@ (3 заголовка). Порог «≥4 заголовков»
  отделяет картообразные файлы от фрагментов: среди ≥4-заголовочных
  карт без TERRAIN — ноль
- **Пустые секции**: 1 случай на корпусе (050, пустая ELEVATION) —
  hint-находка, приемлемая

## Description

**Слот 1 (#112) — диагностики в analysis:**

- `missing-section` (warning): картообразный файл (≥4 заголовков
  секций) без `TERRAIN_GENERATION`; range — первая секция (точка
  вставки для quickfix); обязательный набор — только terrain,
  осознанно
- `empty-section` (hint): секция без стейтментов; range — строка
  заголовка
- Дока кодов — checks.md; severityOverrides — общий механизм

**Слот 2 (#113) — quickfix в server:**

- На `missing-section`: «Insert <TERRAIN_GENERATION> section» —
  zero-width вставка скелета `<TERRAIN_GENERATION>\nbase_terrain
  GRASS\n\n` перед первой секцией (точка из range диагностик;
  Pos.Offset несётся парсером, серверной математики не нужно)

## Acceptance

- make check; corpus: duplicate + missing — ноль, empty — 1 честная
  находка (050), счётчики в лог гейта
- Тесты: missing (карта/фрагмент-эскейп/с terrain), empty; quickfix
  (title, вставка перед первой секцией)

## Result

**Слот 1 (#112) — internal/analysis:**
- `missing-section` (warning): картообразный файл — ≥4 **реальных**
  заголовков (неявная секция `global` парсера не считается); единственный
  required — `terrain_generation` (kb хранит секции в lowercase, display —
  `TERRAIN_GENERATION`); range — первый реальный заголовок
- `empty-section` (hint): заголовок без стейтментов; `global` скипается
- Уточнение из корпуса: duplicate-секции и PLAYER_SETUP-вариации —
  легитимные паттерны (7 переоткрытий, 3 карты без create_player), не
  проверяются; порог ≥4 отсекает фрагменты (макс. 3 заголовка у
  включаемых кусков)

**Слот 2 (#113) — internal/server:**
- `insertSectionAction`: парсинг имени из message, zero-width вставка
  скелета `<TERRAIN_GENERATION>\nbase_terrain GRASS\n\n` перед первой
  секцией (range диагностик указывает туда)

**Приёмка:** corpus-гейт (расширен): duplicate + missing — **0/100**,
empty — 1 честная находка (050, пустая ELEVATION_GENERATION); harness
100/100 ok panic 0; 15/15 пакетов ok, lint 0
**Тесты:** `TestAnalyzeRms_Sections` — 5 кейсов (missing, эскейп
фрагмента, terrain-present, empty в середине, bare headers);
`TestServerCodeAction_MissingSectionFix` — title/скелет/zero-width
**Дока:** codes в checks.md; severityOverrides — общий механизм
