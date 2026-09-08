# Кросс-файловая навигация и резолв include-ов (cross-file navigation)

Status: Done — PR #4 (task/cross-file-navigation)

## Current State

Навигация `aoe2-lsp` (PR #3, `docs/tasks/lsp-navigation.md`) — строго
однодокументная: Definition в XS разрешается внутри одного файла, References —
вхождения в том же файле, DocumentSymbol — outline одного документа.

Кросс-файловая часть отсутствует целиком:

- `rms.RmsFile.Includes []string` — пути `#include` собираются парсером
  (`rms/parse.go`), но никем не резолвятся: сервер не читает диск, включённые
  файлы не парсятся, переход внутрь include невозможен
- `#includeXS <file.xs>` — парсер переключается в inline-XS режим и
  **отбрасывает имя файла**; внешний `.xs` не загружается. Следствие: вызов в
  inline-XS функции из подключённого `.xs` даёт пустой Definition
- `server.DocStore` — кэш только открытых в редакторе документов
  (text+version); слоя «дочитать с диска по требованию» нет
- Диагностики missing-include не существует

## Description

Сделать include-замыкание документа видимым для навигации и загрузки:

1. **Резолв `#include`** — путь относительно директории включающего документа
   (`URI.Filename()` + `filepath`); чтение с диска по требованию, кэш по URI,
   приоритет editor-state (открытый документ авторитетнее дисковой копии)
2. **Диагностика** — ненайденный/нечитаемый include → диагностика с range
   директивы
3. **Definition через файлы**:
   - курсор на include-директиве → Location во включённый файл
   - курсор на имя в inline-XS, объявлённое во внешнем `.xs` → Definition
     в тот файл
4. **`#includeXS <file.xs>`** — внешний файл загружается как реальный
   XS-источник (сейчас имя игнорируется); bare `#includeXS` (inline-режим на
   остаток файла) сохраняет текущее поведение
5. **References по замыканию** — поиск вхождений по include-замыканию
   текущего файла

Ложные `undefined-symbol` в inline-XS уходят как следствие: подключённые
`.xs`-источники видны анализу (отдельная подзадача на analysis не выделяется).

## Scope

**In scope:**
- Контракт `rms`: `Includes` из `[]string` → тип с Range директивы; аргумент
  `#includeXS <file>` сохраняется в AST
- Компонент резолва include-замыкания: URI→path, относительные пути,
  дисковая загрузка с кэшем, visit-set против циклов, missing → ошибка
- Server: Definition/References по замыканию, missing-include диагностика
- LSP-сценарии как приёмочные тесты (см. Notes)

**Out of scope:**
- `workspace/symbol`, rename
- `#include_drs` (бинарный игровой архив, локально не резолвится)
- file-watcher (`didChangeWatchedFiles`): disk-файлы, изменённые вне
  редактора, перечитываются по правилам кэша (ограничение фиксируется
  в документации)
- Изменение protocol-поверхности: capabilities уже заявлены, меняется только
  наполнение результатов

## Acceptance Criteria

- Definition на include-директиве возвращает Location во включённый файл
  (range — начало файла/первой секции)
- Definition на имени из внешнего `.xs` возвращает Location в этом файле
- References возвращает вхождения из include-замыкания (файлы на диске,
  не только открытые)
- didOpen с ненайденным include публикует missing-include диагностику
  с range директивы
- Циклические include (A→B→A) не зацикливают резолв (visit-set/лимит глубины)
- Открытый в редакторе документ-цель отдаёт свою (редакторную) версию, а не
  дисковую копию
- Интеграционный тест по stdio покрывает сценарии из Notes на многофайловой
  фикстуре в `testdata/`
- `go test ./...`, `goimports -w .`, `golangci-lint run`, `goga lint`,
  `goga contract <ячейка>` — зелёные для затронутых ячеек

## Stack

- **Language:** Go (существующий модуль, новых модулей нет)
- **LSP:** `go.lsp.dev/protocol` v1.0.1 (закреплён; кросс-файловость не меняет
  protocol-поверхность)
- **Диск:** stdlib `os`/`path/filepath` + `go.lsp.dev/uri` (транзитивно;
  `uri.File`/`URI.Filename()` для конверсий); rootUri не нужен
- **Testing:** `testify` + `cmp` по `conventions`; многофайловые фикстуры
- **Infrastructure:** нет

## External Dependencies

| Component             | Usage file                            | Status                              |
|-----------------------|---------------------------------------|-------------------------------------|
| `go.lsp.dev/protocol` | `.goga/usages/cooks/lsp-protocol.md`  | updated (секции Cross-file Navigation Results, Disk-backed Documents) |
| `testify`, `cmp`      | `.goga/usages/conventions.md`         | existing (covered by conventions)   |

## Risks and Constraints

- **Циклы include** (A→B→A, транзитивные) — visit-set + лимит глубины
- **Кэш-инвалидация** — disk-файлы, изменённые вне редактора, не
  отслеживаются (file-watcher вне скоупа); перечитывание на didChange самого
  файла, внешние изменения видны по правилам кэша — задокументировать как
  ограничение
- **Позиции во внешних `.xs`** — парсятся напрямую, без range-сдвига как у
  inline `XsBlock`
- **Контрактные изменения** `rms` + `server` (возможно новая ячейка для
  include-графа) — порядок leaves → root по `goga-cookbook`; размещение
  include-резолвера решает brainstorm
- **Кроссплатформенность путей** — только `filepath` + `uri.File()`
  (CI-задача целится в win/mac/linux)

## Scope Estimate

Одна задача, 3 подзадачи (порядок: 1 → 2 → 3):

1. **rms-include-positions** — контракт `rms`: Include с Range директивы +
   сохранение аргумента `#includeXS`; тесты. Ценность: AST несёт позиции
2. **include-graph** — резолвер замыкания (пути, диск, кэш, циклы,
   missing). Ценность: изолированно тестируемый домен
3. **server-wiring** — хендлеры Definition/References по замыканию,
   missing-include диагностика, интеграционные LSP-сценарии. Ценность:
   работающая фича

## Existing Architecture

Утверждено brainstorm'ом 2026-09-07 (план: `docs/arch/cross-file-navigation.md`;
верификация VERIFIED):

- `rms` (leaf) — `Includes []string` → `[]Include` (Path+Range аргумента);
  +`XsIncludes`; +`RmsFile.References(name)`
- `xs` (leaf) — +`XsFile.References(name)`
- `include` — **новая ячейка**: `Source`/`Resolver`/`Closure`/`RmsEntry`/
  `XsEntry`/`ResolvedInclude`/`MissingInclude`/`Target`; инверсия
  editor-state через `Source`
- `analysis` — `AnalyzeXs(file, externals []Decl)`: декларации замыкания
  сеются в TypeEnv, локальные приоритетнее (ложные undefined-symbol уходят)
- `server` (root) — `DocStore.Text` (удовлетворяет `Source`);
  Definition/References через `Resolver`; missing-include диагностика;
  inline-XS анализ с externals
- `common`, `kb` — без изменений

Порядок: rms ∥ xs → include ∥ analysis → server.

## Notes

**LSP-сценарии** (зафиксированы как ожидаемое поведение; станут основой
приёмочных тестов подзадачи 3). Раскладка фикстуры:

```
testdata/
  maps/main.rms            # #include "parts/econ.rms", #includeXS parts/lib.xs, inline-XS вызов sharedFn
  maps/parts/econ.rms      # секции + команды
  maps/parts/lib.xs        # void sharedFn(int n) { ... }
  maps/broken.rms          # #include "missing.rms" (файла нет)
```

Ожидания:

| # | Действие | Результат |
|---|----------|-----------|
| 1 | Definition на `parts/econ.rms` в include-директиве `main.rms` | Location в `econ.rms` |
| 2 | Definition на `sharedFn` в inline-XS `main.rms` | Location объявления `sharedFn` в `lib.xs` |
| 3 | References на `sharedFn` (в `lib.xs`; `main.rms` открыт) | вхождение в `main.rms` + объявление/вызовы в `lib.xs` (обратное направление — по открытым документам, чьё замыкание содержит запрошенный файл) |
| 4 | didOpen `broken.rms` | диагностика missing-include с range директивы |
| 5 | didOpen `main.rms` при уже открытой вкладке `econ.rms` (изменённой) | Definition/References используют редакторную версию `econ.rms` |

Решения, принятые при формулировке (2026-09-07):

- Диагностика ложных `undefined-symbol` не выделена отдельной подзадачей —
  уходит следствием загрузки внешних `.xs`
- Примеры в задаче — LSP-сценарии (не код API): конкретные интерфейсы
  появятся на этапе brainstorm контрактов
- Обновление куки `lsp-protocol.md` (кросс-файловые Location + дисковая
  загрузка) утверждено вместе с задачей
