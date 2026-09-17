# kb-attribute-desc: полнота desc атрибутов RMS-команд в kb

Слот пачки ux-and-data-quality (issue #59, размер S–M, ячейка kb — данные).
Предусловие hover-attribute (PR #40) выполнено — консюмеры уже рендерят
`desc`, задача чисто про данные и пайплайн.

## Current State

`internal/kb/data/rms-commands.json` (53 команды, генерируется
`cmd/kbgen` из `docs/ref/zetnus-rms-guide.txt`):

- `args`: 60 экземпляров, пустых `desc` — 2 (`rnd: arg1, arg2`; в гайде
  не задокументированы);
- `attributes`: 142 экземпляра, пустых `desc` — все 142 → 89 уникальных
  имён; у некогда «намайненных» диапазонов `range` тоже пусто.

Расклад 89 уникальных пустых имён по источникам:

- **61** — имеют запись в глоссарии Zetnus-гайда: standalone-записи вида
  `land_percent  %` + `Game versions:` + prose-абзац (часто с
  `Mutually exclusive with:`, `Arguments: * % - number (0-100)`);
- **28** — флаги (34 экземпляра: `set_tight_grouping`,
  `make_indestructible`, `find_closest*`, …) — глоссарных записей нет;
  аннотация `kbdata` в CODEMANIFEST фиксирует «name-only by design».

`ExtractRmsCommands` парсит только скелеты команд и
`* Name - description`-буллеты аргументов — глоссарий не читает.
Attribute hover, completion Detail и signature help рендерят `desc`,
когда он есть: правок вне kb не требуется.

## Description

Заполнить `desc` (и доступные диапазоны) атрибутов и аргументов
RMS-команд из двух источников, с ручным overlay для нехватки:

1. **Глоссарий гайда** — расширить экстракцию `internal/kb`: парсить
   глоссарные записи Zetnus-гайда и мержить `desc` в атрибуты команд по
   имени. При нескольких записях одного имени (варианты
   `number_of_tiles (elevation)` vs `(terrain)`) предпочесть запись,
   чей раздел (по Example-блоку записи) совпадает с разделом команды;
   иначе — первая запись имени.
2. **Overlay** — проектный (не зеркальный) hand-maintained файл
   `docs/ref/attribute-descs.json` для того, чего нет в гайде: 28
   флагов и 2 аргумента `rnd`. Формат:

   ```json
   {
     "attributes": {
       "set_tight_grouping": "Place group members with minimal spacing between them.",
       "make_indestructible": "…"
     },
     "command_args": {
       "rnd": ["<desc arg1>", "<desc arg2>"]
     }
   }
   ```

3. **Отчёт по пробелам** — детерминированный coverage-вывод `cmd/kbgen`
   после регенерации: сколько заполнено, что осталось пустым и почему
   (нет источника), WARN в лог при непокрытых именах.

## Scope

**In scope:**

- парсер глоссарных записей в `internal/kb` + мерж в атрибуты/аргументы
  на этапе `GenKB` (диапазоны — через существующий `MineKindRange`);
- overlay-файл `docs/ref/attribute-descs.json` (контент: 28 флагов +
  `rnd`) и его чтение в `GenKB`;
- правило мержа **fill-when-empty**: глоссарий авторитетен, overlay
  заполняет остаток и никогда не переопределяет глоссарий;
- регенерация `internal/kb/data/rms-commands.json`;
- обновление контракт-документации ячейки: аннотация `kbdata` в
  CODEMANIFEST (флаги больше не «name-only by design»),
  `internal/kb/.usages/data-pipeline.md`, таблица источников в
  `docs/kb-refresh.md` (строка overlay: проектный, не зеркало);
- coverage-отчёт/лог в `cmd/kbgen`.

**Out of scope:**

- правки консюмеров (`internal/hints`, `internal/complete`,
  `internal/server`) — они уже рендерят `desc`;
- `desc` в `xs-functions.json` / `xs-constants.json` (другие источники,
  полнота регулируется UGC Guide);
- новые внешние источники помимо Zetnus-гайда;
- override глоссарных текстов через overlay.

## Acceptance Criteria

- `make check` зелёный; `goga contract internal/kb` зелёный.
- В `rms-commands.json` пустых `desc` среди 89 имён глоссарного покрытия
  не остаётся; флаги и `rnd`-арги закрыты overlay; итог — 0 пустых `desc`
  либо каждый остаток перечислен в отчёте с причиной.
- Повторная регенерация байт-в-байт идемпотентна (тот же вывод diff и
  coverage-отчёта).
- `kb.DiffKB`-отчёт регенерации показывает изменения только вида
  `attributes[x].desc` / `args[x].desc` (+ `range`, где появился).
- Аннотация `kbdata` и `.usages/data-pipeline.md` не противоречат новым
  данным (про «name-only by design» текст убран/обновлён).
- Corpus-прогон: расхождений с master, кроме смысла диффа (desc), нет.

## Stack

- **Frameworks:** Go 1.26+ (stdlib)
- **Libraries:** без новых зависимостей (JSON-writer с сортировкой ключей
  и `MineKindRange` уже в ячейке)
- **Infrastructure:** — 

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| Zetnus-гайд (локальный источник) | — | существующий, `docs/ref/zetnus-rms-guide.txt` |
| attribute-descs overlay | — | новый проектный файл, не внешний компонент |

Новых внешних зависимостей нет — cooks-файлы не требуются.

## Risks and Constraints

- Формат глоссарных записей вариативен (редкие формы без
  `Game versions:`/`Arguments:`) — регекспы калечатся; страховка:
  тесты экстракции на реальном гайде + WARN на непропарсенном.
- Идемпотентность: JSON-вывод детерминирован (сортировка ключей),
  повторный прогон не должен менять данные; правило мержа
  fill-when-empty — и на этапе генерации, и при загрузке (NewStore)
  должно сходиться.
- Overlay обязан переживать kb-refresh (переэкспорт гайда): живёт в
  `docs/ref/` рядом с источниками, применяется после экстракции.
- Пробелы после двух источников (если всплывут) не блокируют приёмку —
  фиксируются отчётом.

## Scope Estimate

Одна задача, S–M. Декомпозиция не нужна: парсер + overlay + регенерация
+ документация — один атомарный слот ячейки kb.

## Existing Architecture

- Ячейка `internal/kb`: `extract.go` (ExtractRmsCommands), `gen.go`
  (GenKB — детерминированная запись), `mine.go` (MineKindRange),
  `model.go` (CommandArg.Desc); пайплайн описан в
  `internal/kb/.usages/data-pipeline.md`, патчевый процесс — в
  `docs/kb-refresh.md`.
- Консюмеры `CommandArg` (hints/complete/server) не затрагиваются:
  публичная поверхность CODEMANIFEST не меняется, меняется только
  наполнение данных и внутренняя логика генерации.

## Notes

- Решение (согласовано на формулировке): подход B — глоссарий +
  overlay, а не «только глоссарий с отчётом по флагам».
- Overlay кладём в `docs/ref/`, потому что там уже живут поддерживаемые
  нами файлы (aoe2de-xs-rms-changelog.md), а не только зеркала; строка в
  таблице источников `docs/kb-refresh.md` пометит его как проектный.
- Текущие цифры (145 пустых из 255 с учётом args) зафиксированы на
  2026-09-17; пере-прогон jq перед началом работы — дешёвая страховка.
- После сохранения task.md обновить зеркало: тело issue #59 ссылается
  на формулировку пачки — заменить ссылку на этот файл.
