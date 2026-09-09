# Design Document: `doc-highlight`

Спецификация реализации `textDocument/documentHighlight` в ячейке server.
Источники: контракты после apply `e4fee35` (`internal/server/CODEMANIFEST`),
план архитектуры `arch.md` (тот же топик), задача — слот `doc-highlight`
пачки editor-experience. Кода на этом этапе не пишем.

---

## Contract Changes

### Changed CODEMANIFEST Files

- `internal/server/CODEMANIFEST` (modify, материализован `e4fee35`):
  - global Annotations: навигационная строка + `DocumentHighlight`
  - `Server.methods`: **+`DocumentHighlight`** (после `References`):
    Algorithm 4 шага, Requirements (пустой slice), Constraints
    (без `Resolver`/`Closure`; Kind=Text)
  - `Initialize`: **+`DocumentHighlightProvider` — Boolean(true)**
- Imports/Usages/Footer — без изменений

### New / Changed / Deleted Entities

- Новый: метод `DocumentHighlight` на entity `Server` (это единственный
  новый контрактный элемент; отдельных типов нет)
- Изменённые: `Initialize` (список capabilities)
- Удалённые: нет

### Usages and Annotations Changes

- `.goga/usages/cooks/lsp-protocol.md`: подсекция Document Highlight в
  Navigation (применена)
- `internal/server/.usages/lifecycle.md`: capabilities + секция
  In-file highlights (применена)

---

## Traces

### Entry point: `Server.DocumentHighlight(ctx, params)`

Трасса (образец — `References`, server.go:545; `SignatureHelp`
signatureHelpRms — шаблон inline-блока, server.go:339):

```
LSP-клиент → textDocument/documentHighlight
  │
  1. s.openDocument(uri) → (text, name, ok)          [server.go:684]
     ok=false (закрыт док) → text="" → пустой ответ; not an error
  2. pos := s.fromProtocolPos(text, params.Position)  [server.go:899]
     UTF-16→байтовая колонка при согласованном utf-16
  3. switch strings.HasSuffix(name, …):
     ├─ ".rms": file, _ := rms.Parse(text, name)
     │    for block in file.XsBlocks:
     │      block.Range.Contains(pos)?
     │        ├─ да: xsFile := xs.XsParse(block.Code, "inline:"+name)
     │        │      ranges := xsFile.ReferencesAt(unshiftPos(pos, block.Range.Start))
     │        │      ranges := shiftRange(r, block.Range.Start) для каждого r
     │        │      → protocol (шаг 4); выход из цикла (блоки не пересекаются)
     │        └─ нет: продолжить
     │    (ни один блок) ranges := file.ReferencesAt(pos)
     ├─ ".xs": xsFile, _ := xs.XsParse(text, name)
     │         ranges := xsFile.ReferencesAt(pos)
     └─ прочее: ranges = nil → пустой ответ
  4. out := make([]protocol.DocumentHighlight, 0, len(ranges))
     для каждого r: out += {Range: s.toProtocolRange(text, r),       [server.go:910]
                            Kind:  protocol.DocumentHighlightKindText}
  5. slog.DebugContext(ctx, "document_highlight", uri, line, col, count)
  6. return out, nil
```

**Чекпойнты типов** (все пройдены по исходникам):

| Шаг | Данные | Ожидает следующий шаг | ✓ |
|---|---|---|---|
| openDocument | `text, name string` | HasSuffix(name), парсеры | ✓ |
| fromProtocolPos | `common.Pos{Line, Column}` | `Range.Contains(common.Pos)`, `ReferencesAt(common.Pos)` | ✓ |
| rms.Parse / xs.XsParse | `(source, name) → (RmsFile/XsFile, []Diagnostic)` | диагностики игнорируются (stateless-хендлер, прецедент SignatureHelp:328) | ✓ |
| ReferencesAt | `[]common.Range` | `toProtocolRange(text, common.Range)` | ✓ |
| XsBlock.Code / .Range | `string` / `common.Range` | XsParse(source); unshift/shift по `Range.Start` (signatureHelpRms:353–358) | ✓ |
| протокол | `[]protocol.DocumentHighlight` | сериализация jsonrpc (kind=1) | ✓ |

**Новый хелпер** `shiftRange(r, base common.Range) common.Range` =
`{Start: shiftPos(r.Start, base.Start), End: shiftPos(r.End, base.Start)}`
— зеркален существующему паттерну shiftDiags (server.go:799); две
строки, отдельного контракта не требует (внутренняя функция server.go).

### Ключевые решения трассы

- **`ok` из openDocument игнорируем осознанно** (как `References`:550):
  закрытый документ → text="" → ReferencesAt пуст → пустой slice.
  Отдельной ветки не нужно.
- **Первое вхождение блока выходит из цикла**: XsBlocks не
  пересекаются (контракт rms), `return` внутри цикла — прецедент
  signatureHelpRms.
- **Kind**: `protocol.DocumentHighlightKindText` (=1) — константа
  существует в go.lsp.dev/protocol@v1.0.1 (document_highlight.gen.go:14).
- **Сигнатура метода** обязана совпасть с `protocol.Server`:
  `DocumentHighlight(ctx context.Context, params *protocol.DocumentHighlightParams)
  ([]protocol.DocumentHighlight, error)` — перекрывает заглушку
  `UnimplementedServer`.

### Дефект, найденный трассировкой (в arch.md, не в контракте)

Checklist плана обещал «позиция на слове без пар → пустой список».
По контрактам rms/xs `ReferencesAt` **включает вхождение под позицией**
(xs: «включая декларацию»): уникальное слово → ровно 1 хайлайт (само
слово). Пустой список — только когда под позицией нет слова-токена
(пробел, комментарий, строка, пустая позиция). CODEMANIFEST
безупречен («вхождения имени под позицией»); исправлена формулировка
checklist в arch.md.

---

## Analysis

- **Новых контрактных сущностей нет** — расширяется только поведение
  `Server`; состояние/DI не растут (stateless, read-only).
- **Переиспользование**: маршрутизация по расширению и inline-сдвиг —
  готовые паттерны (Hover/SignatureHelp/Completion); ReferencesAt —
  готовые методы провайдеров; конвертация кодировки — готовые
  `fromProtocolPos`/`toProtocolRange`.
- **Данные**: замыкание не вычисляется → внешний I/O отсутствует,
  хендлер дешевле References (нет Closure DFS).
- **Потоки данных**: params.Position → common.Pos → []common.Range →
  []protocol.DocumentHighlight; обратного потока нет.

## Usages Analysis

| Практика | Что даёт | Где используется | Почему | Как |
|---|---|---|---|---|
| `conventions` | правила Go/тестов | вся ячейка | базовая | DI/error idioms; table-driven |
| `lsp-protocol` (+ новая подсекция) | паттерны go.lsp.dev | `DocumentHighlight`, Initialize | протокольные типы/соглашения | Boolean(true), пустой slice ≠ nil, конвертация позиций |
| `rms-parsing` (Import) | фасад rms | шаг 3 (.rms) | Parse/XsBlock/ReferencesAt | `Parse(text, name)`, `block.Range.Contains` |
| `xs-parsing` (Import) | фасад xs | шаг 3 (.xs, inline) | XsParse/ReferencesAt | `XsParse(source, name)` |
| `lookups`/`checks`/`computing`/`completing`/`symbols`/`closure` | прочие хендлеры | не участвуют | — | не затрагиваются |

Все практики упомянуты в аннотациях (глобальная), lint 0 ошибок.

## Cross-cutting Concerns

- **Errors**: `err` — только протокольные сбои; хендлер не возвращает
  ошибок в норме (прецедент References). Промах — пустой slice.
- **Logging**: одна DEBUG-строка `document_highlight` (uri, line, col,
  count) — по образцу `references`; видна с флагом `-debug`.
- **Caching**: нет — парсинг на запрос, как у всех stateless-хендлеров
  (Hover/SignatureHelp/Completion).
- **Concurrency**: stateless, чтение `s.docs`/`s.utf16` — те же
  гарантии, что у соседей (atomic для utf16).
- **Validation**: неизвестное расширение → пустой список; позиция вне
  слов → пустой список (не ошибка).

## Test Scenarios

Интеграционные stdio-сценарии (харнесс navigation_test.go; тесты — под
memory cap).

### T1 `TestDocumentHighlight_XsOccurrences` (positive)

- **Setup**: didOpen `map.xs`: `int towerCount = 2;` … `towerCount =
  towerCount + 1;` (декларация + 2 использования).
- **Input**: documentHighlight на строке с первым использованием
  `towerCount`.
- **Trace**: openDocument → fromProtocolPos → XsParse →
  ReferencesAt → [декларация, исп.1, исп.2] → toProtocolRange ×3 →
  Kind=Text.
- **Assertions**: len=3; все Kind==1; range'и указывают на вхождения
  `towerCount` (проверка start.line/column по фикстуре).
- **Sufficiency**: ядро фичи — все вхождения файла с kind; регрессия
  на потерю декларации или сдвиг range.

### T2 `TestDocumentHighlight_RmsAttributeName` (positive)

- **Setup**: didOpen `map.rms` c `land_percent` в двух секциях
  (шаблон sections.rms).
- **Input**: documentHighlight на имени атрибута в первой секции.
- **Trace**: Parse → вне XsBlocks → RmsFile.ReferencesAt → 2 range →
  protocol.
- **Assertions**: len=2; Kind==1; обе позиции — имена атрибутов.
- **Sufficiency**: RMS-путь без inline-блока; регрессия на фильтрацию
  атрибутных имён.

### T3 `TestDocumentHighlight_InlineXsShift` (positive, edge)

- **Setup**: didOpen `map.rms` c `#includeXS`-блоком (шаблон
  includes.rms: `int seed = 0;` … `seed = xsGetMapSeed();` …
  `if (seed % 2 == 0)`).
- **Input**: documentHighlight на `seed` в строке `seed =
  xsGetMapSeed();` (не первая строка блока).
- **Trace**: Parse → block.Range.Contains → XsParse(block.Code) →
  ReferencesAt(unshiftPos) → shiftRange каждого → toProtocolRange.
- **Assertions**: len=3; все Kind==1; **все range в координатах
  внешнего .rms** (в т.ч. первая строка блока получила сдвиг колонки).
- **Sufficiency**: единственный тест сдвига координат — ловит
  классическую ошибку «забыл сдвинуть назад».

### T4 `TestDocumentHighlight_NoToken` (negative)

- **Setup**: didOpen `map.xs` из T1.
- **Input**: позиция на пустой строке.
- **Trace**: ReferencesAt → пусто → пустой slice.
- **Assertions**: len==0; err==nil.
- **Sufficiency**: контракт «пустой slice, не nil» — клиенты падают на
  null.

### T5 `TestDocumentHighlight_UnknownExtension` (negative)

- **Setup**: didOpen `notes.txt` (unknown language).
- **Input**: documentHighlight на любом слове.
- **Assertions**: len==0; err==nil.
- **Sufficiency**: маршрутизация по расширению не падает на
  постороннем файле.

### T6 `TestDocumentHighlight_UniqueWord` (edge)

- **Setup**: didOpen `map.xs`: слово `xsGetMapSeed()` встречается один
  раз.
- **Input**: documentHighlight на этом слове.
- **Assertions**: len==1 (само вхождение); Kind==1.
- **Sufficiency**: фиксирует семантику «ReferencesAt включает
  вхождение под позицией» — защита от «пустого ответа на уникальное
  слово».

## Usages / `.usages/` Consistency

`internal/server/.usages/lifecycle.md` — обновлён при apply (capabilities
+ In-file highlights); новых доменов нет, файл актуален. Другие ячейки
не менялись — их `.usages/` не затронуты.

## Verification (для плана)

- `make check` зелёный; `goga contract internal/server` зелёный
  (метод материализован — extractor увидит его на Server)
- T1–T6 зелёные (`go test ./internal/server/...` под memory cap)
- Capability видна клиенту: initialize-ответ содержит
  documentHighlightProvider=true (покрыт T-сценариями харнесса)
