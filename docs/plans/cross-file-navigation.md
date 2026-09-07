# Plan: `cross-file-navigation`

Result of compiling `docs/design/cross-file-navigation.md` (design commit
`7b371d4`; контракты материализованы в `90d725e`, фикc дефекта References —
в `7b371d4`). ralphex-compatible: one task per iteration.

## Purpose

Кросс-файловая навигация в aoe2-lsp: резолв `#include`/`#includeXS`,
include-замыкание с дисковой загрузкой, Definition/References по замыканию
(вкл. обратное направление по открытым документам), missing-include
диагностика, подавление ложных `undefined-symbol` в inline-XS через внешние
декларации. Ячейки: `rms` (+`Include`, `XsIncludes`, `References(name)`),
`xs` (+`References(name)`), `include` (новая — 8 типов), `analysis`
(`AnalyzeXs` + externals), `server` (`DocStore.Text/URIs`, хендлеры через
`Resolver`, конвейер диагностики).

Главные пробелы между контрактом и кодом:

- `#includeXS <file>` отбрасывает имя файла (parse.go:438-440 — только
  режим switch); `Includes` — `[]string` без Range (parse.go:437)
- `include/` не существует; `server` не читает диск, gate открытого
  документа в Definition/References (server.go:327-331)
- `AnalyzeXs(file)` без externals (server.go:559,569) — ложные
  undefined-symbol на символах из внешних `.xs`
- Опоры для переиспользования: word-индекс `rms` (ast.go:52 `words`),
  индекс вхождений `xs` (ast.go:85 `symbols`), `TypeEnv` (types.go:38),
  `shiftDiags/shiftPos` (server.go:573-599 — unexported, в `include`
  нужны локальные зеркала)

Стратегия: лист → корень (rms ∥ xs → include ∥ analysis → server →
интеграция), TDD на каждой задаче, инварианты «editor-state побеждает
диск», «пустой slice ≠ nil», «деградация без ошибок» везде.

## Context

### Contract Surface

**Entity: `Include`** (rms; `location`: `rms/ast.go`)
- Type: Entity (данные); facade: экспорт из `rms`
- Properties: `Path -> string` (как записан, относительный);
  `Range -> Range` (диапазон **аргумента-пути**, не всей директивы)
- Semantic: чистый тип, construct-and-use, без методов

**Properties on `RmsFile`** (rms; `location`: `rms/ast.go`)
- `Includes -> []Include` (было `[]string`) — директивы `#include`
- `XsIncludes -> []Include` — внешние `.xs` (`#includeXS` с аргументом);
  inline-код остаётся в `XsBlocks`

**Method on `RmsFile`** (rms; `location`: `rms/ast.go`)
- `References(name: string) -> ranges: []Range` — все слова-токены, равные
  `name` (имена секций/команд/атрибутов/значения ident/const), по позиции;
  Requirement: `ReferencesAt(pos)` ≡ `References(слово под pos)`

**`Parse` (поправка Algorithm, шаг 5)** (rms; `location`: `rms/parse.go`)
- `#include` с аргументом → `Include` в `Includes` (Range = аргумент);
  `#includeXS` с аргументом → `Include` в `XsIncludes` **и** начало
  `XsBlock`; bare `#includeXS` → только `XsBlock`; без аргумента →
  синтаксическая Diagnostic, `Include` не создаётся

**Method on `XsFile`** (xs; `location`: `xs/ast.go`)
- `References(name: string) -> ranges: []Range` — все вхождения `name` из
  индекса вхождений, включая декларацию, по позиции; Requirement:
  `ReferencesAt(pos)` ≡ `References(SymbolAt(pos))`

**Entity: `Source`** (include; `location`: `include/source.go`)
- Type: Entity (interface); facade: экспорт из `include`
- Methods: `Text(uri: string) -> text: string, found: bool` (editor-state;
  false — «смотреть на диск»); `URIs() -> uris: []string` (все открытые —
  для обратного поиска включающих)

**Entity: `Resolver`** (include; `location`: `include/resolver.go`)
- Type: Entity; ctor `Resolver(source: Source)` (DI)
- Methods (Algorithm из контракта, дословно — см. задачи 6-7):
  - `Closure(ctx: Context, uri: string) -> closure: Closure`
  - `Definition(ctx: Context, uri: string, pos: Pos) -> target: Target, found: bool`
  - `References(ctx: Context, uri: string, pos: Pos) -> refs: []Target`
- Requirements: потокобезопасность (внутренняя синхронизация кэша); ctx
  уважает отмену

**Entity: `Closure`** (include; `location`: `include/closure.go`)
- Properties: `Root -> string`; `Rms -> []RmsEntry`; `Xs -> []XsEntry`;
  `Resolved -> []ResolvedInclude`; `Missing -> []MissingInclude`
- Method: `ExternalDecls(exclude: string) -> decls: []Decl` (DFS-порядок;
  `""` — не исключать)

**Entities-данные** (include; `location`: `include/closure.go`):
`RmsEntry(uri, file RmsFile)`; `XsEntry(uri, file XsFile)`;
`ResolvedInclude(owner, inc Include, target)`; `MissingInclude(owner,
path, r Range)` (Range — координаты owner); `Target(uri, r Range)`

**Method `Analyzer.AnalyzeXs`** (analysis; `location`: `analysis/analyzer.go`)
- Новая сигнатура: `AnalyzeXs(file: XsFile, externals: []Decl) ->
  diags: []Diagnostic`; Algorithm шаг 1: TypeEnv из Decls файла, затем
  external-декларации в топ-левел **только для незанятых имён**;
  Requirements: externals подавляют undefined-symbol и дают типы InferType;
  локальные приоритетнее; nil/пусто ≡ прежнее поведение; Constraints:
  не мутировать AST и externals

**Methods on `DocStore`** (server; `location`: `server/docstore.go`)
- `Text(uri: string) -> text: string, found: bool` — проекция Get без
  version; `URIs() -> uris: []string` — все открытые (сортировка для
  детерминизма); вместе — структурное удовлетворение `Source`

**Methods on `Server`** (server; `location`: `server/server.go`)
- `Definition`: `Resolver.Definition(ctx, uri, pos)` → `Target` →
  `protocol.Location` (URI может быть чужим); промах → пустая
  `LocationSlice` (не nil); gate открытого документа СНЯТ
- `References`: `Resolver.References` → Locations;
  `IncludeDeclaration=false` → исключить range локальной декларации;
  пустой список не nil
- `DidOpen`/`DidChange`: конвейер = Closure(uri) → Missing(owner==uri) →
  Diagnostic code="missing-include"; inline-блоки:
  `AnalyzeXs(xsFile, ExternalDecls(""))`; `.xs`:
  `AnalyzeXs(file, ExternalDecls(uri))`; одна пачка PublishDiagnostics
- `NewServer` — без изменения сигнатуры: `include.NewResolver(docs)` внутри

**Annotation cascade (глобальные):** `conventions` (DI, ctx-first, doc-
комментарии, table-driven); `lsp-protocol` (Disk-backed Documents,
Cross-file Navigation Results — uri.File/Filename, editor-state приоритет,
Location с чужым URI, пустой slice ≠ nil); parser never fails hard.

### Re-exports

Отсутствуют — не добавлять.

### Usages Context

- `conventions` (.goga/usages/conventions.md): Go 1.23+, DI-конструкторы,
  goimports, testify/require, table-driven `Test<Component>_<Scenario>`,
  early returns, doc-комментарии на экспортируемые
- `lsp-protocol` (.goga/usages/cooks/lsp-protocol.md): секция
  **Disk-backed Documents** — `URI.Filename()` + `filepath.Join(Dir)`,
  editor-state побеждает, деградация без паники; секция **Cross-file
  Navigation Results** — Location с чужим URI, `uri.File(path)`
- `rms_grammar` / `xs_grammar` — семантика директив/деклараций

### Imported Usages

- `positions-and-diagnostics` from `common` —
  `common/.usages/positions-and-diagnostics.md`: zero-based, порядок
  (line, column), Range [Start, End), Offset-поля
- `rms-parsing` + `includes` from `rms` — `rms/.usages/{rms-parsing,
  includes}.md`: Parse/навигация; Includes/XsIncludes + Range hit-test +
  by-name References
- `xs-parsing` from `xs` — `xs/.usages/xs-parsing.md`: XsParse/SymbolAt/
  Definition/References(name)/Symbols (Selection = name-range)
- `lookups` from `kb`, `checks` from `analysis`, `symbols` from `common`
  — server, без изменений
- `closure` from `include` — `include/.usages/closure.md`: wiring
  (DocStore как Source), Closure+Missing, Navigation, ExternalDecls

### Local Usages

Все файлы созданы/обновлены на этапе apply (`90d725e`, `7b371d4`):
`rms/.usages/includes.md`, `include/.usages/closure.md` (new);
`rms-parsing.md`, `xs-parsing.md`, `checks.md`, `lifecycle.md` (extended).
Задач на usage-файлы в плане НЕТ — они потребительская документация,
актуальность подтверждена дизайн-фазой.

### External Dependencies

- `go.lsp.dev/uri` — `uri.File(path)` / `URI.Filename()`; уже в go.sum
  транзитивно; `go mod tidy` переведёт в direct (новой версии нет)
- stdlib `os`, `path/filepath`, `sync` — без изменений
- testify + cmp — тесты

## Facts

- Тесты — только под memory-cap песочницей:
  `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p
  MemorySwapMax=0 bash -c 'go test ./... -count=1'` (CLAUDE.md)
- `rms` word-индекс `words []wordOcc` (ast.go:52) уже пишется при парсинге;
  `xs` индекс `symbols []symbol` (ast.go:85) — переиспользовать, парсеры
  не менять
- `TypeEnv` root-область — `map[string]string` (types.go:38-56): сидинг
  externals = запись в root только незанятых ключей
- `shiftPos/shiftDiags` сервера unexported: в `include` — локальные
  зеркала (~15 строк), обратная трансляция doc→block — вычитанием
- Текстовые include в игре — RMS-сниппеты любого расширения: всё кроме
  `.xs` парсится `rms.Parse`; цели `XsIncludes` — `xs.XsParse`
- Диагностика missing-include публикуется только для открытого корня
  (Owner==uri), неоткрытые URI не спамим
- `.xs`-корень: externals = `ExternalDecls(uri)`; standalone-открытие
  `lib.xs` без включающего — вызовы соседних `.xs` остаются undefined
  (задокументированное ограничение, вне сценариев задачи)
- Definition-цель include-перехода: `Range{Pos{}, Pos{}}` (0:0, нулевая
  длина)
- Диапазон аргумента include — из токена лексера (включая кавычки);
  bare-форма (`#include parts/x.inc`) валидна

## Gap Analysis

- Missing entities: весь контракт `include` (8 типов); `rms.Include`,
  `XsIncludes`, `References(name)` (rmx/xs); `DocStore.Text/URIs`
- API mismatches: `RmsFile.Includes []string` → `[]Include`;
  `AnalyzeXs(file)` → `AnalyzeXs(file, externals)` (вызовы в
  server.go:559,569 мигрируют вместе с контрактом; `goga contract
  analysis` потребует обновления сигнатуры)
- Behavioral: `#includeXS` arg丢弃 → запись; Definition/References gate
  открытого документа снимается; конвейер диагностики расширяется
- Reuse: `words`/`symbols` индексы, `TypeEnv`, `sortDiags/toProtocolDiags`,
  stdio-харнесс серверных тестов
- Test coverage gaps: все 16 сценариев дизайн-документа новые

---

## Tasks

> **Package ordering rule**: задачи ячейки закрываются до перехода к
> следующей. Внутри кодовой задачи — contract tests first (TDD).
> Порядок: rms (1-2) ∥ xs (3) → include (4-7) ∥ analysis (8) → server
> (9-11) → интеграция (12).

### Task 1: `rms.Include` + `XsIncludes` + Parse-директивы (TDD coding)

Контракт: `Include` (ast.go), `RmsFile.Includes []Include` +
`XsIncludes []Include`, поправка Algorithm `Parse` (шаг 5). Парсер уже
разбирает директивы в `directive()` (parse.go:421-445): `#include`
извлекает имя из сырой строки без Range; `#includeXS` отбрасывает
аргумент, только переключая inline-режим (`p.inXs=true; p.xsStart=idx+1`).
Лексер строки (`newLexer(line, p.starts[idx], idx)`) даёт токены с
абсолютными позициями — следующий `lex.next()` после токена директивы
даёт токен аргумента с нужным Range.

**Usages relevant to this task:**
- `conventions`: table-driven, `Test<Component>_<Scenario>`
- `rms_grammar` (Directives): `#include <file.rms>`, `#includeXS <file.xs>`,
  bare `#includeXS`; `#const`/`#define`/`#include_drs` — statements
- `positions-and-diagnostics` from Imports: Range [Start, End), конструирование

**CRITICAL: `rms/CODEMANIFEST` — read-only. Несоответствие чинится в коде.**

- [ ] **STEP 0 (DECLARATION)**: работаю по Task 1 плана
      `docs/plans/cross-file-navigation.md`
- [ ] **STEP 1 (CONTRACT TESTS)**: в `rms/parse_test.go` — проверка
      фасада/формы: `file.Includes` имеет тип `[]Include` с полями
      `Path`/`Range`; `file.XsIncludes` — `[]Include`; тест
      `TestParse_IncludeRecordsPathArgumentRange` (quoted + bare формы,
      Range включая кавычки: `Include{Path:"parts/econ.rms",
      Range:[0:9..0:25]}`) и `TestParse_IncludeXSArgumentAndInlineBlock`
      (XsIncludes[0].Path=="lib/helpers.xs" **и** XsBlocks[0] на месте —
      dual-режим). Ожидаемый провал — ок
- [ ] **STEP 2 (IMPLEMENTATION)**: `rms/ast.go` — тип `Include{Path
      string; Range common.Range}` (doc-комментарий: путь как записан +
      диапазон аргумента); `RmsFile.Includes` → `[]Include`; добавить
      `XsIncludes []Include`
- [ ] **STEP 2 (IMPLEMENTATION)**: `rms/parse.go` `directive()`: после
      токена директивы взять токен аргумента `lex.next()`; `Path =
      strings.Trim(argTok.text, "\"")`; пуст → существующая диагностика
      `"#include needs a file name"`, Include не создаётся; `#include` →
      `Includes`; `#includeXS` → `XsIncludes` (если аргумент есть) +
      прежний режим-switch безусловно
- [ ] **STEP 3 (INTERFACE VERIFICATION)**:
      `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p
      MemorySwapMax=0 bash -c 'go test ./rms/... -count=1'` — contract
      tests зелёные
- [ ] **STEP 4 (LOGIC TESTS)**: негативный `TestParse_IncludeWithoutPath`
      (диагностика syntax, `len(Includes)==0`); совместимость: прежние
      фикстуры `parse_test.go` (Includes-тест на `Team_Islands_lands.rms`
      адаптировать под `[]Include`)
- [ ] **STEP 5 (DEBUGGING)**: все тесты rms зелёные (чинить реализацию,
      не тесты)
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract rms` —
      соответствие Include/Includes/XsIncludes
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run` — чисто
- [ ] **STEP 8 (COMPLETION)**: отметить чекбоксы; коммит
      `feat: rms include — Include-тип, XsIncludes, Range аргумента (Task 1)`

### Task 2: `rms.RmsFile.References(name)` (TDD coding)

Контракт: `References(name: string) -> ranges: []Range` — by-name проекция
word-индекса. Опора: `words []wordOcc` (ast.go:52), существующий
`ReferencesAt` (ast.go:243-279) — тот же фильтр после разрешения имени.

**Usages:** `conventions`; `includes` from Imports (by-name секция).

**CRITICAL: `rms/CODEMANIFEST` — read-only.**

- [ ] **STEP 0 (DECLARATION)**: Task 2
- [ ] **STEP 1 (CONTRACT TESTS)**: в `rms/parse_test.go` (или новый
      `navigation_test.go`): сигнатура `References(name string)
      []common.Range` компилируется и вызывается
- [ ] **STEP 2 (IMPLEMENTATION)**: `rms/ast.go` — метод: фильтр
      `w.name == name` → `w.at`; сортировка `slices.SortFunc` по
      `Start.Before`; рефакторинг: `ReferencesAt` разрешает имя и
      делегирует `References(name)`
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: `go test ./rms/... -count=1`
      (песочница) — зелёное
- [ ] **STEP 4 (LOGIC TESTS)**: `TestReferences_ByName` (rms-ветка):
      `create_elevator` ×2 → 2 range по позициям; эквивалентность
      `ReferencesAt(pos)` ≡ `References(слово под pos)` на каждой
      позиции; отсутствие имени → пустой результат
- [ ] **STEP 5 (DEBUGGING)**: тесты rms зелёные
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract rms`
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
      `feat: rms navigation — References(name) by-name проекция (Task 2)`

### Task 3: `xs.XsFile.References(name)` (TDD coding)

Контракт: `References(name: string) -> ranges: []Range` — из индекса
вхождений `symbols []symbol` (xs/ast.go:85), включая декларацию. Зеркало
`ReferencesAt` (xs/ast.go:110-136) без шага разрешения имени.

**Usages:** `conventions`; `xs_grammar` (индекс вхождений поверх C-like
грамматики — парсер не меняется); `xs-parsing` from Imports (by-name
секция добавлена apply-ем).

**CRITICAL: `xs/CODEMANIFEST` — read-only.**

- [ ] **STEP 0 (DECLARATION)**: Task 3
- [ ] **STEP 1 (CONTRACT TESTS)**: `xs/navigation_test.go`: сигнатура
      компилируется, вызов на распарсенном источнике
- [ ] **STEP 2 (IMPLEMENTATION)**: `xs/ast.go` — фильтр `s.name == name`,
      сортировка по `Start.Before`; `ReferencesAt` делегирует после
      `SymbolAt`
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: `go test ./xs/... -count=1`
      (песочница)
- [ ] **STEP 4 (LOGIC TESTS)**: `TestReferences_ByName` (xs-ветка):
      `void f(){}\nvoid g(){ f(); }` → 3 range (декларация f + call);
      эквивалентность с `ReferencesAt`; prelude-фикстура не регрессирует
- [ ] **STEP 5 (DEBUGGING)**: тесты xs зелёные
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract xs`
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
      `feat: xs navigation — References(name) (Task 3)`

### Task 4: `include.Source` — интерфейс editor-state (TDD coding, bootstrap ячейки)

Первая задача новой ячейки: создать пакет `include` и интерфейс `Source`
(`include/source.go`) — инверсия зависимости от editor-state. Методы:
`Text(uri) (text, found)` и `URIs() []string` (обратный поиск включающих
— фикс дефекта из дизайн-фазы).

**Usages:**
- `conventions`: интерфейс = именованный тип, doc-комментарий
- `closure` from Imports (`include/.usages/closure.md`): секция Wiring

**CRITICAL: `include/CODEMANIFEST` — read-only.**

- [ ] **STEP 0 (DECLARATION)**: Task 4
- [ ] **STEP 1 (CONTRACT TESTS)**: `include/source_test.go`:
      compile-time удовлетворение — hand-written fake
      `type fakeSource map[string]string` с `Text`+`URIs` присваивается
      переменной типа `Source` (`var _ Source = fakeSource{}`)
- [ ] **STEP 2 (IMPLEMENTATION)**: `include/source.go`: package doc
      (include-замыкание документов), `Source` interface{ Text(uri
      string) (text string, found bool); URIs() (uris []string) } с
      doc-комментариями семантики каждого метода (found=false — сигнал
      «смотреть на диск»)
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: `go build ./include/...` и
      `go test ./include/... -count=1` (песочница)
- [ ] **STEP 4 (LOGIC TESTS)**: fake `Text`/`URIs` — карта открытых
      документов: found/не found, перечисление
- [ ] **STEP 5 (DEBUGGING)**: зелёное
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga lint` (include в
      схеме) + `goga contract include`
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
      `feat: include source — интерфейс editor-state (Task 4)`

### Task 5: `include` данные замыкания — `Closure` + entries + `Target` (TDD coding)

Контракт (`include/closure.go`): `Closure{Root, Rms []RmsEntry, Xs
[]XsEntry, Resolved []ResolvedInclude, Missing []MissingInclude}` +
`ExternalDecls(exclude) []Decl`; данные `RmsEntry{URI, File}`,
`XsEntry{URI, File}`, `ResolvedInclude{Owner, Inc, Target}`,
`MissingInclude{Owner, Path, Range}` (координаты owner), `Target{URI,
Range}`. Всё — construct-and-use данные с doc-комментариями.

**Usages:** `conventions`; `xs-parsing` from Imports (Decls); `rms-parsing`
(RmsFile).

**CRITICAL: `include/CODEMANIFEST` — read-only.**

- [ ] **STEP 0 (DECLARATION)**: Task 5
- [ ] **STEP 1 (CONTRACT TESTS)**: `include/closure_test.go`: все типы
      компилируются и конструируются с контрактными полями;
      `Closure.ExternalDecls` сигнатура
- [ ] **STEP 2 (IMPLEMENTATION)**: `include/closure.go` — 7 типов +
      метод `ExternalDecls`: `for e in Xs: if e.URI != exclude: out =
      append(out, e.File.Decls...)`, DFS-порядок сохраняется, `""` —
      ничего не исключать
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: `go test ./include/... -count=1`
- [ ] **STEP 4 (LOGIC TESTS)**: `ExternalDecls` — пустое Xs → пусто;
      два XsEntry → конкатенация Decls в порядке; exclude отфильтровывает
      ровно один URI
- [ ] **STEP 5 (DEBUGGING)**: зелёное
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract include`
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
      `feat: include closure — данные замыкания + ExternalDecls (Task 5)`

### Task 6: `include.Resolver` — конструктор + `Closure(ctx, uri)` (TDD coding)

Ядро ячейки. Algorithm (из дизайна/контракта, дословно):
```
Closure(ctx, uri):
1. text := load(uri)          # Source.Text → os.ReadFile(Filename());
                               # оба промаха (вкл. non-file URI) →
                               # Closure{Root: uri} с пустыми списками
2. entry := parse(uri, text)  # .xs → xs.XsParse; иначе → rms.Parse
                               # (diags отбрасываются)
3. queue := [{uri, entry, 0}]; visited := {canon(uri)}
4. WHILE queue: e := pop-front
   IF e — RmsEntry: FOR inc IN Includes + XsIncludes:
     target := canon(filepath.Join(dir(path(e.URI)), inc.Path))
     stat ok → Resolved += {e.URI, inc, uri.File(target)};
       target ∉ visited AND depth+1 ≤ 64 AND len(files) < 1024 →
       загрузить (дисковый кэш по stat size+modtime), visited +=,
       queue += {target, depth+1}
     stat/read неудача → Missing += {e.URI, inc.Path, inc.Range}
5. RETURN Closure{Root, Rms, Xs, Resolved, Missing}  # DFS-порядок директив
```
Кэш: `map[string]cachedFile` + `sync.Mutex`; записи из `Source.Text`
кэшем не накрываются (парсятся на каждый вызов — editor-state всегда
свежий); дисковые — по (size, modtime).

**Usages:**
- `lsp-protocol` (Disk-backed Documents): Filename+Join, editor-state
  побеждает, деградация без паники
- `includes` from Imports: Includes/XsIncludes с Range
- `rms-parsing`/`xs-parsing` from Imports: сигнатуры Parse/XsParse
- `closure` from Imports: секция Closure and missing includes

**CRITICAL: `include/CODEMANIFEST` — read-only.**

- [ ] **STEP 0 (DECLARATION)**: Task 6
- [ ] **STEP 1 (CONTRACT TESTS)**: `include/resolver_test.go`:
      `NewResolver(source)` компилируется, возвращает `*Resolver`;
      `Closure(ctx, uri)` сигнатура; тип результата — `Closure`
- [ ] **STEP 2 (IMPLEMENTATION)**: `include/resolver.go` — конструктор
      (source + мьютекс + кэш), `Closure` по Algorithm; хелперы
      load/parse/canon/expand как приватные функции; лимиты —
      именованные константы (`maxDepth = 64`, `maxFiles = 1024`)
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: `go test ./include/... -count=1`
- [ ] **STEP 4 (LOGIC TESTS)**: из дизайн-документа (tempdir +
      fake Source):
      - `TestResolver_ClosureDFSAndMissing`: дерево main→{a→b, lib.xs,
        missing} → Rms=3 (DFS), Xs=1, Resolved=3, Missing=1 с Owner и
        Range аргумента
      - `TestResolver_EditorStateWinsOverDisk`: Source-версия a.rms
        (new.rms) побеждает дисковую (old.rms); диск не читается
      - `TestResolver_RootUnavailable`: пустое Closure{Root}, без паники
      - `TestResolver_CycleTerminates`: a→b→a — 2 Rms, тест не виснет
      - `TestResolver_DepthAndCountLimits`: цепочка 70 файлов —
        65 Rms, Missing пуст
- [ ] **STEP 5 (DEBUGGING)**: `go test ./include/... -count=1` (песочница)
      зелёное
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract include`
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
      `feat: include resolver — Closure: DFS, visit-set, лимиты, кэш (Task 6)`

### Task 7: `include.Resolver` — `Definition` + `References` (TDD coding)

Algorithm (дословно из дизайна/контракта):
```
Definition(ctx, uri, pos):
1. entry := load-and-parse(uri)
2. FOR inc IN Includes+XsIncludes: inc.Range.Contains(pos)
   → resolve(inc) → RETURN Target{targetURI, Range{Pos{}, Pos{}}}, true
3. IF .rms AND pos ∈ block.Range: bpos = pos − block.Range.Start;
   XsParse(block.Code).Definition(bpos) → hit → Target{uri, shift(r)}
4. IF .xs: entry.Definition(pos) → hit → Target{uri, r}
5. name := SymbolAt (xs / inline-translated);
   FOR e IN closure.Xs (DFS): FOR sym IN e.File.Symbols():
     sym.Name == name → RETURN Target{e.URI, sym.Selection}
6. RETURN zero, false

References(ctx, uri, pos):
1. closure := Closure(uri); name := .xs→SymbolAt | .rms→
   (ReferencesAt(pos) → имя = text[first range по Offset])
2. roots := {uri} ∪ {u ∈ Source.URIs(): u≠uri, Closure(u) ∋ uri}
3. hits := []; FOR root: FOR f IN Closure(root).files:
     hits += {f.URI, r ∈ f.References(name)};
   FOR root: FOR block: hits += {root.URI, shift(r) ∈
     XsParse(block.Code).References(name)}
4. RETURN dedup(URI,Range), сортировка (URI, Start)
```
Трансляция координат: локальные зеркала `shiftPos` (зеркало
server.go:587-599: line+base.Line, column только при line 0, offset+
base.Offset) и обратное вычитание.

**Usages:**
- `lsp-protocol` (Cross-file Navigation Results): Target с чужим URI
- `xs-parsing` from Imports: SymbolAt/Definition/References(name)/Symbols
- `closure` from Imports: секция Navigation (корни поиска по Source.URIs,
  дедуп, found=false для builtin)

**CRITICAL: `include/CODEMANIFEST` — read-only.**

- [ ] **STEP 0 (DECLARATION)**: Task 7
- [ ] **STEP 1 (CONTRACT TESTS)**: сигнатуры `Definition(ctx, uri,
      common.Pos) (Target, bool)` и `References(ctx, uri, common.Pos)
      []Target` компилируются
- [ ] **STEP 2 (IMPLEMENTATION)**: методы в `include/resolver.go` по
      Algorithm; хелперы shift/unshift как приватные
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: `go test ./include/... -count=1`
- [ ] **STEP 4 (LOGIC TESTS)**: из дизайн-документа:
      - `TestResolver_DefinitionIncludeDirective`: курсор на `a.rms` в
        include-строке → Target{uri(a.rms), Range 0:0 нулевой длины}
      - `TestResolver_DefinitionExternalDecl`: sharedFn в inline →
        Target{uri(lib.xs), name-range из Symbols().Selection}
      - `TestResolver_ReferencesReverseOverOpenDocs`: из lib.xs (main
        открыт) → вхождения в main (inline, трансляция) + lib
      - `TestResolver_ReferencesDedupAndSort`: двойное включение — нет
        дублей (URI,Range), сортировка (URI, Start)
      - негатив: builtin-имя → found=false; пустой результат — не nil
- [ ] **STEP 5 (DEBUGGING)**: зелёное (песочница)
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract include`
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
      `feat: include navigation — Definition/References по замыканию (Task 7)`

### Task 8: `analysis.AnalyzeXs` — externals (TDD coding)

Контракт: `AnalyzeXs(file: XsFile, externals []Decl) -> diags`.
Трасса: `env := NewTypeEnv(file)` без изменений; затем сидинг
`for d in externals: if d.Name != "" && root не содержит d.Name:
root[d.Name] = тип по Kind` (DeclFunction/DeclExtern/DeclVariable →
d.Type; прочие → ""). Затенение параметрами/локалями уже работает —
они кладутся во вложенные области. nil/пусто ≡ прежнее поведение.

**Usages:** `conventions`; `checks` from Imports (externals-паттерн);
`xs_coercion` — типизация не меняется.

**CRITICAL: `analysis/CODEMANIFEST` — read-only.**

- [ ] **STEP 0 (DECLARATION)**: Task 8
- [ ] **STEP 1 (CONTRACT TESTS)**: сигнатура с двумя параметрами; вызовы
      в существующих тестах analysis обновить на `AnalyzeXs(file, nil)`
- [ ] **STEP 2 (IMPLEMENTATION)**: `analysis/analyzer.go` — параметр +
      сидинг в root-область после локальных (приватный хелпер
      seedExternals); типы по правилам NewTypeEnv
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: `go test ./analysis/... -count=1`
- [ ] **STEP 4 (LOGIC TESTS)**:
      `TestAnalyzeXs_ExternalsSuppressUndefinedAndLocalsWin`:
      с externals — 0 undefined-symbol; с nil — 1; file-декларация
      приоритетнее одноимённого external; external с пустым Name —
      пропуск; bad-type работает с типом из external
- [ ] **STEP 5 (DEBUGGING)**: зелёное (песочница)
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract analysis`
      (сигнатура сверена с контрактом)
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
      `feat: analysis externals — AnalyzeXs с декларациями замыкания (Task 8)`

### Task 9: `server.DocStore` — `Text` + `URIs` (TDD coding)

Контракт: `Text(uri) (string, bool)` — проекция Get без version;
`URIs() []string` — ключи под мьютексом, сортировка (детерминизм).
Вместе — структурное удовлетворение `include.Source`.

**Usages:** `conventions`; `closure` from Imports (Wiring: DocStore
satisfies Source).

**CRITICAL: `server/CODEMANIFEST` — read-only.**

- [ ] **STEP 0 (DECLARATION)**: Task 9
- [ ] **STEP 1 (CONTRACT TESTS)**: `server/docstore_test.go`:
      `var _ include.Source = (*DocStore)(nil)` — compile-time
      удовлетворение
- [ ] **STEP 2 (IMPLEMENTATION)**: `server/docstore.go` — оба метода под
      существующим мьютексом
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: `go test ./server/... -count=1`
- [ ] **STEP 4 (LOGIC TESTS)**: `TestDocStore_TextAndURIs`: Put a,b →
      Text(a)=("text-a",true), Text(c)=("",false), URIs()=
      ["file:///a","file:///b"] отсортировано; Remove убирает из URIs
- [ ] **STEP 5 (DEBUGGING)**: зелёное (песочница)
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract server`
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
      `feat: server docstore — Text/URIs, удовлетворение Source (Task 9)`

### Task 10: `server` — Resolver wiring + Definition/References (TDD coding)

Контракт: `NewServer` создаёт `include.NewResolver(docs)` (сигнатура не
меняется); `Definition` → `Resolver.Definition` → `Target` →
`protocol.Location{URI: uri.URI(t.URI), Range: toProtocolRange(t.Range)}`;
промах → `protocol.LocationSlice{}`; `References` → `Resolver.References`
→ Locations, `IncludeDeclaration=false` → исключить локальную декларацию
(range Definition в запрошенном файле — обобщение существующего
`excludeDeclaration`, server.go:389). **Gate открытого документа в
Definition/References снимается** — резолвер сам fallback-ится на диск.

**Usages:**
- `lsp-protocol` (Cross-file Navigation Results): пустой slice ≠ nil,
  Location с чужим URI
- `closure` from Imports: секция Navigation
- `conventions`: DI
- `lookups` from `kb`, `symbols` from `common` — hover/completion/
  documentSymbol не меняются; их практики действуют как прежде

**CRITICAL: `server/CODEMANIFEST` — read-only.**

- [ ] **STEP 0 (DECLARATION)**: Task 10
- [ ] **STEP 1 (CONTRACT TESTS)**: хендлеры возвращают Location с URI
      ≠ запрошенному (cross-file) — форма результата
- [ ] **STEP 2 (IMPLEMENTATION)**: `server/server.go` — поле `resolver
      *include.Resolver` в `NewServer`; `Definition`/`References`
      переписаны по Algorithm; `excludeDeclaration` обобщить на Target
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: `go test ./server/... -count=1`
- [ ] **STEP 4 (LOGIC TESTS)**: юнит-тесты хендлеров через tempdir
      фикстуру (definition на include → URI целевого файла; references
      IncludeDeclaration=false исключает декларацию; закрытый документ
      отвечает из диска)
- [ ] **STEP 5 (DEBUGGING)**: зелёное (песочница)
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract server`
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
      `feat: server navigation — Definition/References через Resolver (Task 10)`

### Task 11: `server` — конвейер диагностики: missing-include + externals (TDD coding)

Контракт `DidOpen`/`DidChange` Algorithm: `publishDiagnostics` →
`closure := resolver.Closure(ctx, uri)`; `for m in closure.Missing:
m.Owner == uri → common.Diagnostic{Range: m.Range, Severity: error,
Code: "missing-include", Message: "include not found: " + m.Path}`;
`.rms`: прежний конвейер + inline-блоки с `AnalyzeXs(xsFile,
closure.ExternalDecls(""))` + `shiftDiags`; `.xs`: `AnalyzeXs(file,
closure.ExternalDecls(uri))`. Хелперы `analyze*` получают `ctx` и `uri`.
Диагностики — только для открытого корня.

**Usages:** `lsp-protocol` (PublishDiagnostics одной пачкой); `checks`
from Imports (externals-паттерн); `closure` from Imports (missing →
Diagnostic).

**CRITICAL: `server/CODEMANIFEST` — read-only.**

- [ ] **STEP 0 (DECLARATION)**: Task 11
- [ ] **STEP 1 (CONTRACT TESTS)**: сигнатуры внутренних хелперов не
      экспортируются — контрактный тест: didOpen `.rms` с ненайденным
      include публикует diagnostic с code="missing-include" и range
      аргумента (fake client или существующий stdio-харнесс)
- [ ] **STEP 2 (IMPLEMENTATION)**: `server/server.go` — расширение
      `publishDiagnostics/analyze*` по Algorithm; missing конвертится
      `toProtocolDiags`
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: `go test ./server/... -count=1`
- [ ] **STEP 4 (LOGIC TESTS)**: inline-XS вызов sharedFn из lib.xs —
      0 undefined-symbol (externals); отсутствующий include — ровно одна
      missing-include диагностика на директиву; `.xs`-документ:
      ExternalDecls(uri) исключает сам файл; одна пачка, сортировка
      сохранена
- [ ] **STEP 5 (DEBUGGING)**: зелёное (песочница)
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract server`
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
      `feat: server diagnostics — missing-include + externals конвейер (Task 11)`

### Task 12: Интеграция — stdio-сценарии 1–5 (integration tests)

Сквозная приёмка задачи `docs/tasks/cross-file-navigation.md` по stdio
на tempdir-фикстуре (существующий харнесс LSP-клиента из теста):
`maps/main.rms` (#include parts/econ.rms, #includeXS parts/lib.xs,
inline вызов sharedFn), `maps/parts/econ.rms`, `maps/parts/lib.xs`
(void sharedFn), `maps/broken.rms` (#include missing.rms).

**Usages:** `lsp-protocol` (полный стек хендлеров); `closure` from
Imports.

- [ ] Создать `server/integration_crossfile_test.go` + tempdir-фикстуру
      (helper: дерево файлов из задачи)
- [ ] Сценарий 1: Definition на include-строке main.rms → Location
      econ.rms (0:0)
- [ ] Сценарий 2: Definition на sharedFn (inline) → Location в lib.xs
      (name-range)
- [ ] Сценарий 3: References на sharedFn (lib.xs открыт, main открыт) →
      вхождения main + lib
- [ ] Сценарий 4: didOpen broken.rms → publishDiagnostics содержит
      code=missing-include с range директивы
- [ ] Сценарий 5: didOpen main при открытой изменённой вкладке econ →
      навигация видит редакторную версию econ
- [ ] Run validation: полная команда из Validation Commands — все тесты
      проекта зелёные
- [ ] Коммит: `test: cross-file navigation — stdio-сценарии 1-5 (Task 12)`

---

## Validation Commands

- `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`: все тесты (только в песочнице — CLAUDE.md)
- `goimports -w .`: форматирование
- `golangci-lint run`: линтер
- `goga lint`: контракты (7 ячеек, 0 ошибок)
- `goga contract <cell>`: соответствие контракт-коду ячейки (rms, xs, include, analysis, server)
- `go build ./cmd/aoe2-lsp`: бинарник собирается
- `go mod tidy && git diff --exit-code go.mod go.sum || true`: go.lsp.dev/uri перешёл в direct без смены версии

## Completion Criteria

- [ ] Все контрактные сущности реализованы в верных `location`
- [ ] Фасады доступности: `Include`/`References` (rms), `References` (xs),
      все 8 типов `include`, `AnalyzeXs(externals)`, `DocStore.Text/URIs`
      — `goga contract` зелёный по пяти ячейкам
- [ ] Поведение соответствует описаниям: эквивалентность
      ReferencesAt≡References; editor-state побеждает диск; циклы
      терминируются; лимиты 64/1024; externals подавляют undefined-symbol
      с приоритетом локальных
- [ ] Каждая кодовая задача прошла TDD (contract → code → verify →
      logic → debug → re-verify → lint)
- [ ] Интеграционные stdio-сценарии 1–5 зелёные
- [ ] Границы ячеек не расширены; `CODEMANIFEST` не изменялись
- [ ] Все Validation Commands проходят
- [ ] Каждая Usages-практика упомянута минимум в одной задаче
