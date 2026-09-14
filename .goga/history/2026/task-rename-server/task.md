# rename-сервер: PrepareRename/Rename с кросс-файловой склейкой по замыканию

## Current State

- Парсеры готовы (#43, merged): `xs.XsFile.RenameSites(pos)` →
  `[]RenameSite{Range, Kind, Decl}` — скоуп-aware (резолв как в `Definition`,
  затенение param/local > топ-левел), builtin/неизвестное → `found=false`;
  `rms.RmsFile.RenameSites(pos)` → `[]Range` — только пользовательские
  `#const`/`#define` (декларация + ident-использования), словарь языка
  (команды/атрибуты/секции) не переименовываем; inline-XS делегируется
  xs-парсеру через координаты `XsBlock` (прецедент — semantic tokens)
- `include.Resolver` знает `Closure`/`Definition`/`References` — кросс-файлово
  (прямое замыкание + обратное направление «кто включает меня» по
  `Source.URIs`, трансляция координат inline-блоков, дедуп `(URI, Range)`) —
  но rename-сайтов не знает. Arch-план #43 оставил паттерн склейки:
  `References(name)` → `pos` → `RenameSites(pos)`, фильтр топ-левел
  биндингов по `Kind`
- Сервер: хендлеров `PrepareRename`/`Rename` нет, `RenameProvider` в
  `Initialize` не заявлен
- Cook `lsp-protocol.md`: секции Rename нет (таблица эпика планирует update
  в задаче 4)

## Description

Серверная половина rename (подзадача 4 эпика v1.0.0): ячейка include
склеивает per-file rename-сайты парсеров в кросс-файловые по include-замыканию,
сервер отдаёт `textDocument/prepareRename` (range + placeholder текущего
имени, отказ на непереименовываемой позиции) и `textDocument/rename`
(мультифайловый `WorkspaceEdit` по всем сайтам, включая файлы, не открытые
в редакторе).

## Scope

**In scope:**
- `internal/include` — метод `Resolver`: склейка rename-сайтов по замыканию —
  `RenameSites(pos)` запрошенного файла даёт имя и `Kind`; кросс-файлово
  идут только **топ-левел** биндинги (`param`/`local` файл-локальны — сайты
  только запрошенного файла); корни поиска — как в `References` (замыкание
  запрошенного файла + обратное направление открытых документов); в других
  файлах `References(name)` → на каждое вхождение `RenameSites(pos)` →
  отбор вхождений того же топ-левел биндинга; inline-XS с трансляцией
  координат; дедуп `(URI, Range)`, сортировка (URI, позиция)
- `internal/server` — `PrepareRename` (переименовываемо →
  `PrepareRenamePlaceholder{Range, Placeholder=текущее имя}`; иначе
  `nil, nil`) и `Rename` (`WorkspaceEdit{Changes}` — по одному `TextEdit`
  на сайт; невалидный `NewName` → `ResponseError`); `RenameProvider` с
  `prepareProvider` в `Initialize`
- cook `.goga/usages/cooks/lsp-protocol.md` — секция Rename and Prepare
  Rename (контент утверждён propose-сессией, см. Notes)
- контракты ячеек: `CODEMANIFEST` + `.usages` include/server обновить

**Out of scope:**
- изменения парсеров xs/rms (read-only: per-file API достаточен;
  семантика `ReferencesAt`/`References` не меняется)
- VS Code-клиент; format (подзадачи 5–6 эпика); documentHighlight /
  semantic tokens
- arm `PrepareRenameDefaultBehavior`, change annotations, переименование
  файлов

## Acceptance Criteria

- rename правит все вхождения биндинга по замыканию, включая файлы, не
  открытые в редакторе (критерий эпика)
- prepareRename отказывает (`nil, nil`) на непереименовываемой позиции:
  builtin, ключевое слово, команда/атрибут/секция, строка/комментарий
- `param`/`local`: правки только в файле декларации; одноимённые символы
  других скоупов не затрагиваются
- rms `#const`/`#define`: декларация + ident-использования по всему
  замыканию; вхождение команды-тёзки сайтом не является
- inline-XS в `.rms` — сайты в координатах `.rms`-файла
- `RenameProvider` (prepareProvider) заявлен в `Initialize`
- `make check` зелёный; `goga lint` / `goga contract` по include, server
  зелёные

## Stack

- **Frameworks:** Go 1.26+ (stdlib first), без новых зависимостей
- **Libraries:** существующие ячейки `include`, `server` (modify); xs/rms —
  read-only потребители per-file API; go.lsp.dev/protocol v1.0.1
  (существующая)
- **Infrastructure:** corpus-гейт CI без изменений (регресс-страховка)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| go.lsp.dev/protocol — rename-поверхность (`PrepareRename`, `RenameOptions`, `WorkspaceEdit`) | `.goga/usages/cooks/lsp-protocol.md` | update (секция Rename and Prepare Rename; применяется в ветке задачи) |

## Risks and Constraints

- семантика склейки должна совпасть с per-file `RenameSites`/`Definition` —
  расхождение даст rename≠definition; общий приём: `References(name)` →
  `RenameSites(pos)` с фильтром по `Kind` (якорь arch-плана #43)
- кросс-файловый отбор для xs: вхождение имени в чужом файле без локального
  биндинга — ссылка на топ-левел декларацию; защита от ложных матчей
  (имя совпало, биндинг другой) — per-вхождение `RenameSites(pos)` в том
  файле
- мультифайловые `WorkspaceEdit` в неоткрытых файлах зависят от клиентской
  поддержки (VS Code — ок; поведение задокументировать — риск эпика)
- производительность: на rename-запрос — одно замыкание + проход по файлам
  с существующими индексами; разовые вызовы, кэш резолвера уже есть

## Scope Estimate

Одна задача, средний объём (таблица эпика). Ветка `task/rename-server` →
PR, `Fixes #44`.

## Existing Architecture

- `internal/include` — `Resolver` (+метод склейки), `Closure`/`Target`;
  паттерны References (корни, дедуп, inline-трансляция) — переиспользуются
- `internal/server` — `Server` (+`PrepareRename`/`Rename`, capability в
  `Initialize`), `DocStore` (структурно `Source`)
- потребители per-file API: `xs.RenameSites` (`RenameSite{Range, Kind,
  Decl}`), `rms.RenameSites` (`[]Range`); прецеденты координатного сдвига
  inline-XS — semantic tokens / selectionRange

## Notes

- Решения propose-сессии 2026-09-14: границы подтверждены; `param`/`local`
  файл-локальны (кросс-файлово только топ-левел биндинги); cook-секция
  Rename утверждена, применяется в ветке задачи
- Целевой вызов (якорь для design-этапа, имена НЕ фиксируются):

```go
// include: склейка rename-сайтов по замыканию
sites, ok := resolver.RenameSites(ctx, uri, pos) // []Target-подобных сайтов, ok=false — не переименовываемо

// server: PrepareRename → &protocol.PrepareRenamePlaceholder{Range: сайт под pos, Placeholder: текущее имя}
// server: Rename → &protocol.WorkspaceEdit{Changes: map[uri.URI][]protocol.TextEdit}
```

- Контент cook-секции (утверждён): `### Rename and Prepare Rename` после
  Selection Ranges — capability
  `RenameProvider: &protocol.RenameOptions{PrepareProvider: &[]bool{true}[0]}`;
  хендлеры по интерфейсу (оба названы по запросу); `PrepareRenameResult` —
  sealed interface, arm `PrepareRenamePlaceholder{Range, Placeholder}`
  (3.18, placeholder предзаполняет поле ввода текущим именем), `nil, nil` =
  непереименовываемо — никогда не угаданный диапазон; `Rename` →
  `WorkspaceEdit{Changes}` plain-правки (прецедент Code Actions), ключи —
  в т.ч. неоткрытые файлы (Cross-file Navigation Results); невалидный
  `NewName` → `ResponseError` с сообщением (требование спецификации),
  не пустой edit
