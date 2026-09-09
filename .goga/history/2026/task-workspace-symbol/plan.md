# Plan: `workspace-symbol`

## Purpose

Реализовать запрос LSP `workspace/symbol` в ячейке `internal/server`:
fuzzy-поиск символов по вселенной «открытые доки + их include-замыкания»,
плоский `SymbolInformation`-список. Контракт уже применён (CODEMANIFEST:
метод `Server.Symbols`, capability `WorkspaceSymbolProvider`); план
закрывает gaps: хендлер отсутствует, fuzzy-скоринг отсутствует, capability
не объявлен в коде. Стратегия: TDD — сначала компактный скорер (без
зависимостей), затем хендлер поверх готовых `Closure`/`symbolKindTable`/
`targetRange`, затем интеграционный stdio-тест.

## Context

### Contract Surface

**Entity: `Server`** (существующая, модифицируется)
- Type: class; Declared `location`: server.go (файл ячейки internal/server)
- Facade: методы доступны через protocol-интерфейс (override
  `protocol.UnimplementedServer`)
- Новый метод: `"Symbols(ctx: Context, params: WorkspaceSymbolParams) ->
  result: WorkspaceSymbolResult, err: error"` — см. CODEMANIFEST
  (Algorithm 5 шагов, Requirements 3, Constraints 3)
- Изменённый метод: `Initialize` — перечень capabilities дополнен
  `WorkspaceSymbolProvider — Boolean(true)`
- Внутренний helper `fuzzyMatch` — вне контракта, fuzzy.go
- Imported dependencies: `Resolver`, `Closure` (include), `RmsFile`
  (rms), `XsFile` (xs), `Symbol` (common); протокольные типы — из
  `lsp-protocol`

### Re-exports
Нет.

### Usages Context

- `conventions` (.goga/usages/conventions.md): Go 1.26+, ctx-first,
  slog, table-driven тесты same-package, doc-комментарии на экспортах.
- `lsp-protocol` (.goga/usages/cooks/lsp-protocol.md), секция
  **Workspace Symbols**: интерфейсный метод — **`Symbols`**; возврат —
  `protocol.SymbolInformationSlice{}` (не WorkspaceSymbolSlice);
  Location может указывать в неоткрытый файл; пустой результат — пустой
  slice, не nil.

### Imported Usages

- `closure` from `internal/include` (`internal/include/.usages/closure.md`):
  `Resolver.Closure(ctx, uri)` — корень замыкания первая запись
  (editor-state парчерез Source.Text); `RmsEntry{URI, File}`,
  `XsEntry{URI, File}`; дисковые парсы кэшированы резолвером.
- `rms-parsing` from `internal/rms` (`internal/rms/.usages/rms-parsing.md`):
  `RmsFile.Symbols()` — дерево kinds: section/command/xs.
- `xs-parsing` from `internal/xs` (`internal/xs/.usages/xs-parsing.md`):
  `XsFile.Symbols()` — плоские топ-декларации
  (function/extern/variable/rule/event).
- `symbols` from `internal/common` (`internal/common/.usages/symbols.md`):
  `Symbol{Name, Kind, Range, Selection, Children}`.

### Local Usages
Ничего нового: `internal/server/.usages/lifecycle.md` уже расширен
(capabilities + секция Workspace symbol search) на этапе apply.

### External Dependencies
- go.lsp.dev/protocol v1.0.1 (закреплён): `WorkspaceSymbolParams.Query`
  (пустая строка = все символы), `SymbolInformation{Name, Kind,
  Location}`, `SymbolInformationSlice`, `WorkspaceSymbolResult`.
- testify (уже в go.mod); новых зависимостей нет.

## Facts

- Интерфейсный метод protocol.Server для workspace/symbol — `Symbols`
  (server.go:78 модуля protocol@v1.0.1); ошибочное имя = not-implemented.
- `Closure` включает корень первой записью; `Resolver.load` берёт текст
  через `Source.Text` (DocStore) — editor-state выигрывает у диска.
- `DocStore.URIs()` возвращает отсортированный []string (docstore.go:72)
  — фундамент детерминированных тай-брейков.
- `symbolKindTable` (server.go:1180) и `targetRange` (server.go:1485)
  существуют и переиспользуются как есть.
- Хендлеры-прототипы: FoldingRanges (парс+Symbols), References
  (Location+targetRange).

## Gap Analysis

- Missing contract entities: `Server.Symbols` (implementation: null по
  `goga contract`), `fuzzyMatch` (вне контракта).
- Capability: `WorkspaceSymbolProvider` не объявлен в `Initialize`-коде.
- Reuse: `symbolKindTable`, `targetRange`, `openDocument`, harness
  `startHarness`/`newNavigationServer`.
- Test coverage gaps: нет тестов workspace/symbol вообще.

## Tasks

> Пакет один (internal/server). Порядок: скорер → хендлер → интеграция.

### Task 1: fuzzy-скорер `fuzzyMatch` (TDD coding)

Контракт поведения (design.md, Algorithm Design → fuzzyMatch): чистая
функция `fuzzyMatch(query, name string) (score int, matched bool)`;
case-insensitive (ToLower); пустой query → (0, true); greedy-subsequence:
+1 за совпадение, +2 за подряд (ni == prev+1), +3 за слово-начало
(ni == 0 или n[ni-1] == '_'), −1 за каждый пропущенный символ между
совпадениями; не подпоследовательность → (0, false). Файл:
`internal/server/fuzzy.go` (неэкспортируемая функция, doc-комментарий по
conventions).

**Usages relevant to this task:**
- `conventions`: table-driven same-package тест; один assert на кейс.

**CRITICAL: `CODEMANIFEST` read-only. Не совпадает — правь код, не контракт.**

- [ ] **Contract tests**: файл `internal/server/fuzzy_test.go`, тест
  `TestFuzzyMatch_Scoring` — таблица кейсов (вычислить точные веса по
  формуле: `{"", "anything", 0, true}`, `{"land",
  "<land_generation>", точный score, true}`, `{"shfn", "sharedFn",
  точный, true}`, `{"SHARED", "sharedFn", точный, true}`, `{"zzz",
  "sharedFn", 0, false}`, `{"x", "", 0, false}`) — сейчас упадёт
  (функции нет)
- [ ] **Code**: создать `internal/server/fuzzy.go` c `fuzzyMatch` по
  алгоритму design.md (побайтовая работа после ToLower)
- [ ] **Interface verification**:
  `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./internal/server/ -run TestFuzzyMatch -count=1'`
- [ ] **Logic tests**: добавить в таблицу edge-кейсы: name короче query
  → (0, false); подряд в начале против разрозненных в середине —
  score первого строго выше; одинаковый вход дважды — одинаковый score
- [ ] **Debugging**: тот же прогон — зелёный; править код, не тесты
- [ ] **Contract re-verification**: сигнатура и видимость (unexported)
  соответствуют design
- [ ] **Lint**: `golangci-lint run ./internal/server/...` (хук
  прогонит fmt; при необходимости `golangci-lint fmt`)

### Task 2: хендлер `Server.Symbols` + capability (TDD coding)

Контракт: CODEMANIFEST internal/server, метод `Symbols` (Algorithm:
вселенная URIs()+Closure с дедупом → сбор RMS sections-only / XS все →
fuzzy-фильтр → сортировка score desc, uri asc, line asc, col asc →
SymbolInformation по lsp-protocol). Трассировка шаг за шагом — design.md
«Code Stack Trace → Server.Symbols» (8 шагов). Переиспользовать
`symbolKindTable`, `targetRange`; Debug-лог «workspace_symbol».
Capability: в `Initialize` добавить `WorkspaceSymbolProvider:
protocol.Boolean(true)` рядом с DocumentHighlightProvider.

**Usages relevant to this task:**
- `lsp-protocol` (Workspace Symbols): метод `Symbols`, возврат
  `SymbolInformationSlice`, пустой slice ≠ nil.
- `closure`: `Closure(ctx, uri)`, entries `Rms`/`Xs` (корень первый,
  editor-state).
- `rms-parsing`/`xs-parsing`/`symbols`: `Symbols()` и поля Symbol.
- `conventions`: ctx-first, slog Debug, doc-комментарий на экспортном
  методе.

**CRITICAL: `CODEMANIFEST` read-only.**

- [ ] **Contract tests**: файл `internal/server/workspace_symbol_test.go`:
  `TestInitialize_AdvertisesWorkspaceSymbol` (capability ==
  Boolean(true)); `TestServerSymbols_APIShape` (нет открытых доков →
  NotNil+Empty slice, NoError) — упадут
- [ ] **Code**: `Initialize` — добавить WorkspaceSymbolProvider
  Boolean(true)
- [ ] **Code**: `Server.Symbols` в server.go (после DocumentSymbol):
  вселенная → сбор → фильтр → сортировка (slices.SortStableFunc,
  составной компаратор) → SymbolInformation{Name, Kind:
  symbolKindTable[...] fallback Field, Location{uri.URI(u),
  targetRange(u, sym.Selection)}}
- [ ] **Interface verification**:
  `go test ./internal/server/ -run 'TestInitialize_AdvertisesWorkspaceSymbol|TestServerSymbols_APIShape' -count=1` (под memory cap)
- [ ] **Logic tests** (сценарии design.md Test Stack Trace):
  `TestServerSymbols_NonIncludedOpenDoc` (главный критерий приёмки:
  символ невключённого открытого дока находится; ровно 1 запись,
  Name/Kind/Location); `TestServerSymbols_ClosureSectionFromDisk`
  (секция из дискового econ.rms); `TestServerSymbols_RmsCommandsExcluded`
  (create_land не в выдаче, createWidget — да);
  `TestServerSymbols_EmptyQueryAllDeterministic` (все символы, порядок
  uri→позиция, идентичность повторного вызова);
  `TestServerSymbols_UnknownExtensionSkipped` (txt-док молча пропущен)
- [ ] **Debugging**: полный пакет под memory cap — зелёный
- [ ] **Contract re-verification**:
  `goga contract internal/server` — Symbols implementation не null,
  расхождений нет
- [ ] **Lint**: `golangci-lint run ./internal/server/...`

### Task 3: интеграционный stdio-тест (integration tests)

Сценарий design.md `TestServerWorkspaceSymbol_IntegrationStdio`: реальный
stdio (startHarness), tmp-дерево: main.rms (`#includeXS lib.xs`) открыт,
lib.xs на диске (sharedFn), standalone.xs открыт и никем не включён.

**Usages relevant to this task:**
- `lsp-protocol`: Dispatcher вызов `h.disp.Symbols(ctx,
  &protocol.WorkspaceSymbolParams{Query: ...})`.

- [ ] Создать/дополнить тест в `internal/server/workspace_symbol_test.go`
  c harness-паттерном integration_crossfile_test.go
- [ ] Сценарий: query «shared» → `sharedFn` из lib.xs (диск, не открыт)
- [ ] Сценарий: query «isolated» (или аналог) → декларация
  standalone.xs (невключённый открытый док)
- [ ] Run:
  `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./internal/server/ -run TestServerWorkspaceSymbol_IntegrationStdio -count=1'`

## Validation Commands

- `make check`: вся предзадачная проверка (fmt → build → memory-cap
  test → lint → goga lint)
- `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p
  MemorySwapMax=0 bash -c 'go test ./... -count=1'`: все тесты под
  memory cap
- `goga contract internal/server`: контракт↔код ячейки
- `golangci-lint run`: линт

## Completion Criteria

- [ ] `Server.Symbols` реализован в server.go по Algorithm манифеста
- [ ] `WorkspaceSymbolProvider` объявлен в Initialize
- [ ] fuzzyMatch без внешних зависимостей, тесты таблицей
- [ ] Пустой результат — пустой slice, не nil; порядок детерминирован
- [ ] Интеграционный stdio-тест зелёный (критерии слота)
- [ ] Каждый coding-таск прошёл TDD (contract tests → code →
  verification → logic tests → debugging → re-verification → lint)
- [ ] `CODEMANIFEST` не менялся при реализации
- [ ] Все Validation Commands зелёные
