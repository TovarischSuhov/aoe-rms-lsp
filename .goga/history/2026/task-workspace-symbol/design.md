# Design Document: `workspace-symbol`

## Contract Changes

### Changed CODEMANIFEST Files
- `internal/server/CODEMANIFEST`: глобальная Annotations (+строка про
  WorkspaceSymbol), `Initialize` (+`WorkspaceSymbolProvider Boolean(true)`),
  новый метод `Symbols` у Entity `Server` (после `DocumentSymbol`).

### New Entities
- Нет новых типов ячейки. Новый метод `Server.Symbols` (протокольное имя
  interface-метода go.lsp.dev/protocol для запроса workspace/symbol) —
  `location` сервера не меняется (server.go). Внутренний helper
  `fuzzyMatch` — вне контракта (файл fuzzy.go, same-package тесты).

### Changed Entities
- `Server` — +метод `Symbols`, +capability в `Initialize`.

### Deleted Entities
- Нет.

### Usages and Annotations Changes
- `.goga/usages/cooks/lsp-protocol.md` — секция Workspace Symbols
  (внесена при формулировке; исправлено имя интерфейсного метода).
- `internal/server/.usages/lifecycle.md` — capabilities + секция
  «Workspace symbol search» (применена на apply).

## Applied Fixes

### Fixed CODEMANIFEST Defects
- `internal/server/CODEMANIFEST` + `lsp-protocol.md`: метод назывался
  `WorkspaceSymbol` → исправлен на **`Symbols`** (interface
  go.lsp.dev/protocol: `Symbols(ctx, *WorkspaceSymbolParams)
  (WorkspaceSymbolResult, error)`, server.go:78; аналог прецедента
  `FoldingRanges`). Без переименования оверрайд не регистрируется —
  клиент получил бы not-implemented.

## Entity Interaction and Data Flow

### Interaction Diagram

```
client ──workspace/symbol──> Server.Symbols
                                │
                                ├─ DocStore.URIs() ──── []string (sorted)
                                │        │ на каждый uri
                                │        ▼
                                ├─ Resolver.Closure(ctx, uri)
                                │      ├─ RmsEntry{URI, File RmsFile}   (root = editor-state парс)
                                │      └─ XsEntry{URI, File XsFile}     (диски — из кэша резолвера)
                                │
                                ├─ seen: map[string]bool — дедуп URI
                                ├─ сбор: RMS → sectionsOnly(File.Symbols())
                                │         XS  → File.Symbols() целиком
                                ├─ fuzzyMatch(query, name) → (score, matched)
                                ├─ sort: score↓, uri↑, Selection.Start↑
                                └─ → SymbolInformationSlice{Name, Kind,
                                    Location{URI, targetRange(uri, Selection)}}
```

### Data Flows
Сценарий «query по символу из невключённого открытого дока»: открыты
main.rms и standalone.xs (без include-связи). `URIs()` → оба ури →
`Closure(main.rms)` даёт main+его замыкание (standalone не входит),
`Closure(standalone.xs)` даёт standalone → дедуп → fuzzy находит декларацию
standalone → `SymbolInformation.Location` указывает на неё.

### Entity Dependencies
`Symbols` → `DocStore` (URIs/Get), `Resolver` (Closure), `symbolKindTable`
(общая с DocumentSymbol), `targetRange` (общая с Definition/References),
`fuzzyMatch` (новый). Инициализация не меняется: все зависимости уже в
`Server`.

## Code Stack Trace

### Trace: `Server.Symbols`

#### Chain
1. **Input**: запрос workspace/symbol, `params.Query: string` (пустая
   строка = все символы — подтверждено doc-комментарием
   `WorkspaceSymbolParams.Query`, workspace_features.gen.go:31).
2. `uris := s.docs.URIs()` → отсортированный []string (docstore.go:72) —
   детерминизм тай-брейков. → checkpoint: тип/сортировка ✓
3. `seen := map[string]bool`; для каждого `u`:
   `closure := s.resolver.Closure(ctx, u)` — корень замыкания уже
   спарсен из editor-state (`Resolver.load` идёт через `Source.Text`,
   т.е. DocStore; resolver.go expand → load) → checkpoint: editor-state
   выигрывает ✓, повторный URI отсекается seen ✓
4. Для каждой `RmsEntry`: `sectionsOnly(entry.File.Symbols())` —
   рекурсивный обход, узлы `kind=section` собираются (включая
   вложенные секции), `command`/`xs` пропускаются; для каждой
   `XsEntry`: весь `entry.File.Symbols()` (топ-декларации, плоско) →
   checkpoint: словари kinds соответствуют symbolKindTable
   (rms: section/command/xs; xs: function/extern/variable/rule/event) ✓
5. `fuzzyMatch(query, sym.Name)` → `(score, matched)`; `!matched` →
   пропуск. → checkpoint: чистая функция, детерминизм ✓
6. Сортировка кандидатов: score desc → uri asc → Selection.Start.Line asc
   → Selection.Start.Column asc (`slices.SortStableFunc` по составному
   ключу). → checkpoint: URIs() отсортирован, обход детерминирован ✓
7. Конвертация: `protocol.SymbolInformation{Name, Kind:
   symbolKindTable[sym.Kind] (fallback Field), Location:
   protocol.Location{URI: uri.URI(u), Range: s.targetRange(u,
   sym.Selection)}}` — `targetRange` конвертирует с текстом для
   открытых доков, байтовые колонки для дисковых (server.go:1485) →
   checkpoint: переиспользование существующих конвертеров ✓
8. **Output**: `protocol.SymbolInformationSlice(out)`; пусто —
   `make(..., 0)` (не nil); Debug-лог «workspace_symbol».

#### Checkpoint Summary
- Все чекпоинты прошли; единственный дефект (имя метода) найден и
  исправлен до трассировки шага 7 (см. Applied Fixes).

### Trace: `fuzzyMatch`

#### Chain
1. **Input**: `query`, `name` (строки; `query` пустой → `(0, true)`).
2. Оба приводятся к нижнему регистру (`strings.ToLower`) — один раз на
   вызов. → checkpoint: case-insensitive ✓
3. Greedy-проход одним указателем по `name`: для очередного символа
   query ищется следующее вхождение в name; нет — `(0, false)`.
4. Скоринг на лету: +1 за совпавший символ; +2 за подряд (позиция
   совпадения = предыдущая+1); +3 за слово-начало (позиция 0 или
   предыдущий символ `_`); −1 за каждый пропущенный символ между
   совпадениями. → checkpoint: целые, детерминированные ✓
5. **Output**: `(score, matched)`.

## Algorithm Design

### `Server.Symbols`

**Responsibility**: ответ на workspace/symbol — плоский список
`SymbolInformation` по вселенной открытых доков и их замыканий.

**Algorithm:**
```
1. uris := DocStore.URIs(); seen := {}
2. FOR u IN uris:
     closure := Resolver.Closure(ctx, u)
     FOR entry IN closure.Rms:          # root первый (editor-state парс)
       IF seen[entry.URI]: CONTINUE
       seen[entry.URI] = true
       collect(entry.URI, sectionsOnly(entry.File.Symbols()))
     FOR entry IN closure.Xs:
       IF seen[entry.URI]: CONTINUE
       seen[entry.URI] = true
       collect(entry.URI, entry.File.Symbols())
3. collect(uri, syms):
     FOR sym IN syms:
       score, matched := fuzzyMatch(query, sym.Name)
       IF !matched: CONTINUE
       candidates = append({score, uri, sym})
4. SORT candidates: score desc, uri asc, line asc, col asc
5. RETURN SymbolInformationSlice{...}
```

**Errors:**
- Протокольные сбои только; парсерные ошибки деградируют (парсеры
  толерантны, error не возвращается хендлером).

**Edge Cases:**
- Нет открытых доков → пустой slice (не nil, не ошибка).
- Открытый док с неизвестным расширением: `Closure` не найдёт entry
  (canonicalPath/расширение) → молча пропускается.
- Один файл включён несколькими открытыми доками → seen-дедуп, один
  набор символов.
- Циклы включений → visit-set резолвера (гарантия include-контракта).

### `fuzzyMatch`

**Responsibility**: компактный детерминированный fuzzy-фильтр имени по
query (subsequence + бонусы), без внешних зависимостей.

**Algorithm:**
```
1. IF query == "": RETURN (0, true)
2. q, n := ToLower(query), ToLower(name)
3. score := 0; prev := -2; qi := 0
4. FOR ni := 0..len(n)-1 WHILE qi < len(q):
     IF n[ni] == q[qi]:
       score += 1
       IF ni == prev+1: score += 2          # подряд
       IF ni == 0 || n[ni-1] == '_': score += 3   # слово-начало
       prev = ni; qi++
     ELSE IF qi > 0:
       score -= 1                            # разрыв (gap)
5. IF qi < len(q): RETURN (0, false)         # не подпоследовательность
6. RETURN (score, true)
```
(Побайтовая работа после ToLower корректна: UTF-8 lower не меняет длину
этих ASCII-границ; не-ASCII имена просто сравниваются побайтово —
детерминизм сохраняется.)

**Errors:** нет — чистая функция.

**Edge Cases:** пустое имя при пустом query → (0, true); пустое имя при
непустом query → (0, false); name короче query → (0, false).

## Cross-cutting Concerns

- **Error handling**: хендлер не возвращает ошибок (stateless read-only);
  молчание при пустой вселенной.
- **Logging**: одна строка `slog.DebugContext(ctx, "workspace_symbol",
  "query", params.Query, "count", len(out))` — как folding/document_symbol.
- **Validation**: неизвестные расширения/языки молча пропускаются;
  невалидный UTF-8 в имени — поведение парсеров (толерантно).
- **Caching**: none сверх существующего — `Resolver` кэширует дисковые
  парсы (stat-keyed); открытые доки парсится на запрос заново (stateless,
  тот же паттерн, что Hover/Completion).
- **Concurrency**: без новой; DocStore/Resolver синхронизированы своими
  мьютексами (контракты ячеек), хендлер однопоточно читает.

## Usages Analysis

### `lsp-protocol`
- **What it provides**: паттерны go.lsp.dev/protocol — в т.ч. новая секция
  Workspace Symbols (форма `SymbolInformationSlice`, silent-пустота,
  правило «не открыт в редакторе» для Location).
- **Where used**: `Server.Symbols` (шаги 7–8 трассировки), `Initialize`.
- **Why chosen**: закреплённая зависимость проекта; протокольные типы.
- **How exactly**: `protocol.WorkspaceSymbolParams.Query`; возврат
  `protocol.SymbolInformationSlice{}` (не `WorkspaceSymbolSlice` — resolve
  не заявляем); `protocol.Location{URI, Range}`.

### `conventions`
- Go 1.26+, ctx-first, slog, gofumpt, doc-комментарии на экспортах
  (метод Symbols — экспортный оверрайд: doc-комментарий обязателен).

### Imported Usages
- `closure` from `internal/include` — вселенная замыканий: `Closure`,
  `RmsEntry`, `XsEntry`; Path: `internal/include/.usages/closure.md`.
- `rms-parsing` from `internal/rms` — `Parse`, `RmsFile.Symbols()`
  (kind=section в дереве); Path: `internal/rms/.usages/rms-parsing.md`.
- `xs-parsing` from `internal/xs` — `XsParse`, `XsFile.Symbols()`
  (плоские топ-декларации); Path: `internal/xs/.usages/xs-parsing.md`.
- `symbols` from `internal/common` — `Symbol{Name, Kind, Range,
  Selection, Children}`; Path: `internal/common/.usages/symbols.md`.

## `.usages/` Update

### Cell: `internal/server`

#### Existing Files — Consistency
- **`lifecycle`** → `internal/server/.usages/lifecycle.md`
  - Status: current (обновлён на apply: capabilities + секция Workspace
    symbol search)
  - Additions needed: нет
  - Updates needed: нет

#### New Files (if any)
- Нет: домен «потребление сервера редактором» уже покрыт lifecycle.md;
  отдельный файл для одного запроса — «too fine».

## Test Stack Trace

### General Setup
- Unit: `newNavigationServer(t)` (navigation_test.go:20) — Server над
  реальным kb.Store; `s.docs.Put(uri, src, 1)`.
- Integration: `startHarness(t)` (serve_test.go:146) — реальный stdio
  Serve + protocol.NewClient dispatcher (`h.disp`), `crossFileFixture`-
  стиль tmp-дерева.
- Memory cap: все прогоны — под systemd-run (CLAUDE.md).

### Source File Registry
- `internal/server/fuzzy.go` (новый) — fuzzyMatch.
- `internal/server/fuzzy_test.go` (новый) — table-driven скоринг.
- `internal/server/workspace_symbol_test.go` (новый) — хендлер-тесты.
- `internal/server/server.go` — Symbols + capability (Initialize).
- `internal/server/serve_test.go` или workspace_symbol_test.go —
  интеграционный stdio-сценарий.

---

### Positive Tests

#### `TestInitialize_AdvertisesWorkspaceSymbol`

**Setup**: `newNavigationServer(t)`.

**Input**: `Initialize(ctx, &protocol.InitializeParams{})`.

**Trace**:
```
s.Initialize(params)
  → capabilities assembled
  returns: InitializeResult
→ assert WorkspaceSymbolProvider == Boolean(true)
```

**Assertions**: `res.Capabilities.WorkspaceSymbolProvider ==
protocol.Boolean(true)`.

**Sufficiency**: capability забыли → клиент не шлёт запрос вовсе;
пиновка поверхности (паттерн folding_test.go:15).

---

#### `TestFuzzyMatch_Scoring` (table-driven)

**Setup**: таблица кейсов {query, name, wantScore, wantMatched}:
- `{"", "anything", 0, true}` — пустой query
- `{"land", "<LAND_GENERATION>", 4+3+2+2+1, true}` — prefix+streaks
  (точные веса считаются от модели; в таблице — конкретные вычисленные
  значения, например land→<land_generation>: l(+1+3 word-start) a(+1+2
  подряд) n(+1+2) d(+1+2) −1 за '<'... веса фиксируются в тесте по
  формуле, регрессия на изменение весов — сознательная)
- `{"shfn", "sharedFn", …, true}` — подпоследовательность по слову
- `{"SHARED", "sharedFn", …, true}` — case-insensitive
- `{"zzz", "sharedFn", 0, false}` — не подпоследовательность
- `{"x", "", 0, false}` — пустое имя

**Input**: пары (query, name).

**Trace**: `fuzzyMatch(q, n)` → (score, matched) → require.Equal по
кейсам.

**Assertions**: точные (score, matched) на кейс.

**Sufficiency**: скоринг — сердце сортировки; таблица пинит и модель
бонусов, и case-insensitive, и отказ.

---

#### `TestServerSymbols_NonIncludedOpenDoc` (критерий приёмки)

**Setup**: `newNavigationServer(t)`; открыты без include-связи:
- `file:///main.rms`: `"<LAND_GENERATION>\nbase_terrain GRASS\n</LAND_GENERATION>\n"`
- `file:///standalone.xs`: `"void isolatedHelper() { }\n"`

**Input**: `Symbols(ctx, &protocol.WorkspaceSymbolParams{Query: "isolated"})`.

**Trace**:
```
Symbols(params)
  → URIs() = [main.rms, standalone.xs]
  → Closure(main.rms): Rms=[main]; Closure(standalone.xs): Xs=[standalone]
  → fuzzyMatch("isolated", "isolatedHelper") matched
  → SymbolInformation{Name:"isolatedHelper", Kind:Function,
     Location{URI: standalone.xs, Range: Selection}}
```

**Assertions**: ровно 1 элемент; Name/Kind; Location.URI ==
standalone.xs; Range = Selection декларации (line 0).

**Sufficiency**: главный критерий приёмки слота — символ из
невключённого открытого дока находится.

---

#### `TestServerSymbols_ClosureSectionFromDisk`

**Setup**: tmp-дерево: main.rms `#include "econ.rms"`, econ.rms на диске
с секцией `<ECONOMY_GENERATION>`; открыт только main.rms.

**Input**: `Query: "econ"`.

**Trace**: Closure(main.rms) → Rms=[main(root, editor), econ(диск)] →
sectionsOnly: секция econ.rms собрана → matched.

**Assertions**: результат содержит `ECONOMY_GENERATION` с
Location.URI = econ.rms (файл НЕ открыт — Cross-file Navigation).

**Sufficiency**: disk-backed замыкания в выдаче; критерий «секция
замыкания находится».

---

### Negative Tests

#### `TestServerSymbols_APIShape`

**Setup**: `newNavigationServer(t)` без открытых доков.

**Input**: `Query: "any"`.

**Assertions**: `require.NoError`; результат — `SymbolInformationSlice`
`NotNil` + `Empty`.

**Sufficiency**: пустой slice ≠ nil ≠ ошибка (требование манифеста и
lsp-protocol).

---

#### `TestServerSymbols_RmsCommandsExcluded`

**Setup**: открыт `file:///t.rms`:
`"<LAND_GENERATION>\ncreate_land {\n\tland_percent 5\n}\n</LAND_GENERATION>\n"`
+ открытый `file:///lib.xs`: `"void createWidget() { }\n"`.

**Input**: `Query: "create"`.

**Assertions**: выдача содержит `createWidget` (Function, xs) и НЕ
содержит ни `create_land`, ни `LAND_GENERATION`-командных узлов; из
RMS — ничего (в файле нет секции c "create" в имени).

**Sufficiency**: критерий «команды RMS не шумят» (Constraint манифеста).

---

### Edge Case Tests

#### `TestServerSymbols_EmptyQueryAllDeterministic`

**Setup**: открыты два дока с известными символами (main.rms секция +
lib.xs функция).

**Input**: `Query: ""`.

**Assertions**: все символы вселенной (2 записи); порядок: uri asc →
позиция; повторный вызов — идентичный результат.

**Sufficiency**: пустой query = все; детерминированность порядка
(Requirement).

---

#### `TestServerSymbols_UnknownExtensionSkipped`

**Setup**: открыт `file:///notes.txt` с текстом-командой.

**Input**: `Query: ""`.

**Assertions**: пустой не-nil slice; txt-док не дал записей и не уронил
хендлер.

**Sufficiency**: молчаливый пропуск незнакомых расширений.

---

#### `TestServerWorkspaceSymbol_IntegrationStdio`

**Setup**: `startHarness(t)`; Initialize; DidOpen main.rms
(`#includeXS lib.xs` + inline) и standalone.xs (не включён никем);
tmp-дерево с lib.xs на диске.

**Input**: `h.disp.Symbols(ctx, &protocol.WorkspaceSymbolParams{Query:
"shared"})`, затем `Query: "isolated"`.

**Trace**:
```
disp.Symbols(params) → jsonrpc2 → Serve → Server.Symbols
  → вселенная из двух открытых + замыканий
  → SymbolInformationSlice
```

**Assertions**: «shared» находит `sharedFn` из lib.xs (диск,
невключённый-открытым); «isolated» находит декларацию standalone.xs;
no error.

**Sufficiency**: end-to-end через stdio — приёмочный критерий пачки
(«изменение LSP-поверхности покрыто интеграционным stdio-тестом»).

## Additional Instructions for the Implementation Agent

- Метод называется `Symbols` (оверрайд interface), НЕ WorkspaceSymbol.
- Переиспользовать `symbolKindTable`, `targetRange`, `slog.DebugContext`
  — не дублировать.
- `fuzzyMatch` — неэкспортируемый, отдельный файл fuzzy.go, без
  зависимостей; тесты — same package (fuzzy_test.go).
- Возврат — `protocol.SymbolInformationSlice` (не WorkspaceSymbolSlice).
- Порядок сортировки реализовать одним `slices.SortStableFunc` с
  составным компаратором (score↓, uri↑, line↑, col↑).
- Не трогать соседние хендлеры; Initialize — только добавить capability.
- Прогоны: `make check` (memory cap), `goga contract internal/server`.
