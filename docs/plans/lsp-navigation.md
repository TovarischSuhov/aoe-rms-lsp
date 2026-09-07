# Plan: `lsp-navigation`

Result of compiling `docs/design/lsp-navigation.md` (design commit
`2aff170`; контракты материализованы в `5942507`, исправлены в
`9ae6a1b`). ralphex-compatible: one task per iteration.

## Purpose

Однодокументная навигация в aoe2-lsp: textDocument/definition (.xs),
references (.xs/.rms), documentSymbol (.xs/.rms) + capabilities в
Initialize. Ячейки меняются, новых нет: `common` (+`Symbol`),
`xs` (+3 метода `XsFile`), `rms` (+2 метода `RmsFile`, word-индекс, фикс
`</name>`), `server` (+3 хендлера, capabilities).

Главные пробелы между контрактом и кодом:

- `common/symbol.go` не существует; `xs`/`rms`/`server` не имеют
  навигационных методов и хендлеров;
- в `rms` нет word-индекса (нужен внутренний) и корректной обработки
  закрывающих тегов (дефект: `</name>` переоткрывает секцию → фантомные
  пустые секции в outline);
- в `xs` индекс вхождений (`XsFile.symbols` + `record`) УЖЕ есть и
  достаточен — переиспользовать, парсер не менять.

Стратегия: лист → корень (common → xs ∥ rms → server), TDD на каждой
задаче, инвариант `Selection ⊆ Range` и «пустой slice ≠ nil» везде.

## Context

### Contract Surface

**Entity: `Symbol`** (common)
- Type: Entity; `location`: `common/symbol.go`
- Facade obligation: экспорт из пакета `common`
- Properties: `Kind -> string` (словарь у производителя: xs —
  function/variable/rule/event/extern; rms — section/command/xs);
  `Name -> string`; `Range -> Range`; `Selection -> Range` (⊆ Range);
  `Children -> []Symbol`
- Semantic requirements (annotations): Selection ⊆ Range — инвариант
  LSP SelectionRange; диапазоны Children внутри Range родителя; чистый
  тип данных, construct-and-use, без методов
- Imported dependencies: `Range` объявлена в этом же CODEMANIFEST

**Methods on `XsFile`** (xs; `location`: `xs/ast.go`; все — приёмник
`XsFile`, чтение существующего индекса `symbols`)
- `Definition(pos: Pos) -> r: Range, found: bool` — вхождение →
  name-range декларирующего объявления; Algorithm (из контракта,
  дословно):
  1. Найти вхождение идентификатора, содержащее `pos` (как SymbolAt);
     нет вхождения — found=false
  2. Среди деклараций файла, объявляющих это имя, взять самую внутреннюю,
     чья область (тело функции/блока) объемлет позицию вхождения;
     при равной вложенности — ближайшую, предшествующую вхождению
     (параметр/локаль затеняют топ-левел)
  3. Позиция на имени декларации — вернуть её собственный name-range
  4. Локальной декларации нет (builtin/неизвестное) — found=false
  - Requirements: детерминированность
- `ReferencesAt(pos: Pos) -> ranges: []Range` — все вхождения имени,
  включая декларацию, отсортированные; совпадение синтаксическое, по
  имени
- `Symbols() -> symbols: []Symbol` — плоский outline топ-деклараций в
  исходном порядке; Selection=name-range из внутреннего индекса;
  include-декларации ПРОПУСКАЮТСЯ (Requirement контракта); Children пусты

**Methods on `RmsFile`** (rms; `location`: `rms/ast.go`)
- `Symbols() -> symbols: []Symbol` — дерево: секции → команды; блоки
  random/conditional рекурсивно в Children; XsBlock → узел
  kind=xs/Name="#includeXS"/Range=Selection=Range блока в содержащую
  секцию (иначе корень) по позиции; Statements вне секций — узлы корня;
  Selection: секция — имя внутри `<>`, команда — токен имени
- `ReferencesAt(pos: Pos) -> ranges: []Range` — все одноимённые
  слова-токены (имена секций, команд, атрибутов и значения
  ident/const), по позиции; локальных деклараций нет

**Methods on `Server`** (server; `location`: `server/server.go`)
- `Definition(ctx, DefinitionParams) -> DefinitionResult, err` — .xs →
  `XsParse` → `Definition(pos)` → `&Location{URI, Range}`; not found /
  .rms / закрыт → пустая `LocationSlice` (не nil); пустой результат —
  не ошибка; конвертация позиций с учётом positionEncoding
- `References(ctx, ReferenceParams) -> []Location, err` —
  `ReferencesAt` по языку; `IncludeDeclaration=false` и .xs → исключить
  range из `Definition(pos)`; для .rms исключений нет; пустой — пустой
  slice, не nil
- `DocumentSymbol(ctx, DocumentSymbolParams) -> DocumentSymbolResult,
  err` — `Symbols()` по языку → рекурсивно `Symbol` →
  `protocol.DocumentSymbol`; Kind по таблице: xs function/extern→
  Function, variable→Variable, rule/event→Event; rms section→Module,
  command→Function, xs→Namespace; неизвестный → Field; Selection ⊆
  Range сохраняется; вернуть `DocumentSymbolSlice`
- `Initialize` (изменение аннотации): + `DefinitionProvider`,
  `ReferencesProvider`, `DocumentSymbolProvider` — `Boolean(true)`;
  прежние caps не трогать

**Annotation cascade (глобальные, применяются ко всем задачам):**
- xs/rms: `Use \`conventions\`…`, `Use \`rms_grammar\`/\`xs_grammar\`…`,
  `Use \`symbols\` from Imports for building outline nodes in Symbols`;
  parser never fails hard, каждый узел несёт Range
- server: `Use \`lsp-protocol\`…` (Navigation-секция: пустой результат —
  пустой slice, не nil); выбор языка по расширению URI; один
  change = один батч диагностик

### Re-exports

Отсутствуют по контрактам — не добавлять.

### Usages Context

- `conventions` (.goga/usages/conventions.md): Go 1.23+, DI-конструкторы,
  goimports, doc-комментарии на все экспортируемые идентификаторы,
  testify/require, table-driven, `Test<Component>_<Scenario>`, blank line
  между логическими блоками, короткие функции/early return
- `rms_grammar` (.goga/usages/rms-grammar.md): секции `<...>`/`</...>`;
  statements вне секций — глобальные; команды/атрибуты; выражения DE
- `xs_grammar` (.goga/usages/xs-grammar.md): декларации top-level,
  C-like области видимости, extern без тела, include — не идентификатор
- `lsp-protocol` (.goga/usages/cooks/lsp-protocol.md): `DefinitionResult`
  arms (`*Location` | `LocationSlice{}`), References — `[]Location` +
  IncludeDeclaration, `DocumentSymbolSlice` иерархически +
  SelectionRange ⊆ Range (спецификация), Boolean-capabilities

### Imported Usages

- `symbols` from `common` — `common/.usages/symbols.md`: как строить узлы
  (пример конструирования, словари Kind, инварианты, construct-and-use)
- `positions-and-diagnostics` from `common` —
  `common/.usages/positions-and-diagnostics.md`: позиции zero-based,
  порядок (line, column), Range [Start, End)
- `xs-parsing` from `xs` — `xs/.usages/xs-parsing.md` (секция Navigation):
  контракты Definition/ReferencesAt/Symbols для хендлеров, reparse
  before query
- `rms-parsing` from `rms` — `rms/.usages/rms-parsing.md` (секция
  Navigation): outline/references, состав слов, reparse before query

### Local Usages

Все файлы созданы/обновлены при материализации (`5942507`, `9ae6a1b`),
актуальны; в этом плане новых нет:
- `common/.usages/symbols.md` (create — готов)
- `xs/.usages/xs-parsing.md` (+Navigation, include-skip — готов)
- `rms/.usages/rms-parsing.md` (+Navigation, состав слов — готов)
- `server/.usages/lifecycle.md` (+Advertised capabilities — готов)

### External Dependencies

- `go.lsp.dev/protocol` (уже в go.mod): `DefinitionResult`,
  `LocationSlice`, `Location`, `DocumentSymbolResult`,
  `DocumentSymbolSlice`, `DocumentSymbol`, `SymbolKind`-константы,
  `Boolean`, `ReferenceContext.IncludeDeclaration`
- testify (уже в go.mod): require/assert
- stdlib: `sort`/`slices`, `strings`

## Facts

- `XsFile.symbols []symbol{name, at}` — индекс ВСЕХ идентификаторных
  вхождений (decl-имена через `parseDecl`/`parseExtern`/`parseRule`,
  параметры `parseParams`, локалы `StmtDecl`, иденты/Calcee через
  `parsePrimary`), порядок = исходный; `SymbolAt` отвечает из него
- `Param` не хранит Range, но name-токен параметра есть в индексе
- `Decl.Kind == DeclInclude`: Name = путь-строка, в индекс не попадает
- rms: `sect{name, nodes, start}`; `closeSection` материализует секцию;
  пустой global отбрасывается, пустая ИМЕНОВАННАЯ — нет; `</name>`
  сейчас обрабатывается как открывающий (дефект)
- rms: имя команды = первый токел statement-строки (`first.at` — точный
  range); имя секции — внутри `<>` строки заголовка (`p.pos(i, col)`);
  `p.pos(i, column)` строит Pos из line/col
- rms-фикстуры проекта пишут только открывающие теги (тесты не ловили
  дефект закрытия)
- server: существуют `openDocument`, `fromProtocolPos`,
  `toProtocolRange`, `toProtocolDiags`; Hover/Completion — образец
  выбора языка по расширению; DocStore синхронизирован
- Все новые сигнатуры без error; протокол: пустой результат ≠ nil

## Gap Analysis

- Отсутствующие контрактные сущности: `common.Symbol`; `XsFile.Definition/
  ReferencesAt/Symbols`; `RmsFile.Symbols/ReferencesAt`;
  `Server.Definition/References/DocumentSymbol`
- Отсутствующая внутренняя инфраструктура: word-индекс rms
  (неэкспортированное поле + запись в 4 точках парсера)
- Поведенческие расхождения: `Initialize` без навигационных caps;
  rms `</name>` → фантомные секции (дефект, фикс в Task 4)
- Код для переиспользования: xs `symbols`/`SymbolAt`/`record`;
  rms `deepestAt`-паттерн обхода; server хелперы конвертации
- Пробелы покрытия тестами: все навигационные сценарии (см. задачи);
  инвариантные свипы prelude.xs и rms/testdata

---

## Tasks

> **Package ordering rule**: задачи ячейки завершаются до начала
> следующей (common → xs → rms → server → интеграция). Внутри coding-
> задач — contract tests первыми (TDD).
>
> **Ветка**: вся работа в `task/lsp-navigation` от актуального `master`;
> атомарный коммит на задачу; PR в `master` после Task 8 (мерджит
> пользователь).

### Task 1: `common.Symbol` — узел outline-дерева (TDD coding)

Ячейка `common` (лист). Контрактная сущность: Entity `Symbol`,
`location: common/symbol.go`. Чистый тип данных: construct-and-use, без
методов; поля — только перечисленные в контракте. Инварианты (Selection
⊆ Range, дети ⊆ родитель) — на производителях (задачи 2/5), здесь только
API-форма. Usages: `conventions` (doc-комментарии, testify, table-driven).
`Symbol` опирается на `Range` из этого же пакета.

**Usages relevant to this task:**
- `conventions`: doc-комментарий на тип и каждое поле; testify/require;
  `Test<Component>_<Scenario>`

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [ ] **STEP 0 (DECLARATION)**: работаю по Task 1 плана
  `docs/plans/lsp-navigation.md`
- [ ] **STEP 1 (CONTRACT TESTS)**: создать `common/symbol_test.go` c
  `TestSymbol_FieldsAndShape`: конструирование `common.Symbol{Kind,
  Name, Range, Selection, Children}` всеми полями; `require.Equal`
  значений; проверка типов полей (Kind string, Children []Symbol) —
  до реализации не компилируется (ожидаемо)
- [ ] **STEP 2 (IMPLEMENTATION)**: создать `common/symbol.go`: doc-комментарий
  пакета не трогать (уже есть в pos.go), тип
  `// Symbol is one outline-tree node …` + поля `Kind`, `Name`,
  `Range`, `Selection`, `Children []Symbol` с doc-комментариями
  (семантика из контракта: Selection ⊆ Range, словарь Kind у
  производителя)
- [ ] **STEP 3 (INTERFACE VERIFICATION)**:
  `systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0
  bash -c 'go test ./common/... -count=1'` — зелёный
- [ ] **STEP 4 (LOGIC TESTS)**: дополнить `TestSymbol_FieldsAndShape`
  кейсом-таблицей: лист (Children nil) и узел с ребёнком; значения
  полей читаются без искажения
- [ ] **STEP 5 (DEBUGGING)**: `go test ./common/... -count=1` (в
  cgroup-песочнице) — чинить реализацию, не тесты
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract common` —
  `Symbol` и поля совпадают с манифестом; facade: импортируемость
  `common.Symbol`
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run` — чисто
- [ ] **STEP 8 (COMPLETION)**: отметить чекбоксы; коммит
  `feat: common symbol — Symbol outline-узел (Task 1)`

### Task 2: `xs.Symbols` + `xs.ReferencesAt` — проекция индекса (TDD coding)

Ячейка `xs`. Контрактные сущности: методы `XsFile` `Symbols()` и
`ReferencesAt(pos)`, `location: xs/ast.go` (методы рядом с существующим
`SymbolAt`). Оба читают СУЩЕСТВУЮЩИЙ индекс `XsFile.symbols`
(`symbol{name, at}`); парсер не менять. `Symbol` — из Imports
(`common.Symbol`, practice `symbols`).

Алгоритмы (дословно из контракта):
- Symbols: «Каждый Decl из Decls, кроме include, → `Symbol` по
  `symbols` из Imports: Kind и Name из декларации, Range=Range
  декларации, Selection=name-range из внутреннего индекса, Children
  пусты»; Requirements: Selection ⊆ Range; плоский список; include
  пропускается
- ReferencesAt: «1. Имя под `pos` (как SymbolAt); нет — пустой список
  2. Собрать диапазоны вхождений имени из индекса вхождений
  3. Отсортировать по позиции»

Selection = ПЕРВОЕ вхождение имени внутри `decl.Range` (index в
исходном порядке: имя декларации записывается раньше вхождений в теле).

**Usages relevant to this task:**
- `conventions`: table-driven, require, doc-комментарии на экспорт
- `xs_grammar`: include — путь-строка, не идентификатор
- `symbols` from Imports: конструирование `common.Symbol`,
  словарь kind (function/variable/rule/event/extern), Selection ⊆ Range
- `positions-and-diagnostics` from Imports: сравнение позиций (line,
  column) для сортировки

**CRITICAL: `CODEMANIFEST` files — read-only. Mismatch → fix implementation, never the contract.**

- [ ] **STEP 0 (DECLARATION)**: Task 2, `docs/plans/lsp-navigation.md`
- [ ] **STEP 1 (CONTRACT TESTS)**: в новом `xs/navigation_test.go` —
  `TestXsSymbols_APIShape` (возвращает `[]common.Symbol`, поля
  Kind/Name/Range/Selection/Children) и `TestXsReferencesAt_APIShape`
  (возвращает `[]common.Range`); до реализации — compile fail (ожидаемо)
- [ ] **STEP 2 (IMPLEMENTATION)**: в `xs/ast.go` реализовать:
  `Symbols() []common.Symbol` — цикл по Decls, skip `DeclInclude`,
  Selection = первое `symbol` с совпадающим именем внутри `decl.Range`
  (иначе Selection = Range — синтезированные декларации), Children nil
- [ ] **STEP 2 (IMPLEMENTATION)**: `ReferencesAt(pos common.Pos)
  []common.Range` — `SymbolAt`-подобный поиск имени под pos → фильтр
  индекса по имени → сортировка по Start (`slices.SortFunc`)
- [ ] **STEP 3 (INTERFACE VERIFICATION)**:
  `go test ./xs/... -count=1` (cgroup-песочница) — контрактные тесты
  зелёные
- [ ] **STEP 4 (LOGIC TESTS)**:
  `TestXsSymbols_FlatOutline` (src `void f() { g(); }` / `int x = 1;` /
  `rule r { condition x }`: 3 узла в порядке, Kind/Name, каждый
  Selection ⊆ Range, Children пусты; include-строка — 4-я декларация
  `include "a.xs"` — НЕ попадает в outline);
  `TestXsReferencesAt_IncludesDeclarationSorted`
  (`void f() { g(); g(); } void g() {}`, pos на втором `g`: 3 диапазра
  возрастают, включают name-токен декларации `g`);
  `TestXsReferencesAt_NoIdentEmpty` (pos на литерале/операторе — пусто)
- [ ] **STEP 5 (DEBUGGING)**: все тесты xs зелёные (чинить реализацию)
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract xs` —
  методы на месте, сигнатуры совпадают; регресс `SymbolAt` не тронут
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
  `feat: xs navigation — Symbols + ReferencesAt (Task 2)`

### Task 3: `xs.Definition` — разрешение декларации с затенением (TDD coding)

Ячейка `xs`. Контрактная сущность: метод `XsFile.Definition(pos) (Range,
bool)`, `location: xs/ast.go`. Самый сложный метод: области видимости.
Парсер и индекс не менять; допустимы внутренние хелперы в ast.go
(обход тел, структура кандидата).

Контрактный Algorithm (дословно): 1) вхождение, содержащее pos (как
SymbolAt); нет — found=false 2) среди деклараций, объявляющих имя, —
самая внутренняя область, объемлющая вхождение; равная вложенность —
ближайшая предшествующая (параметр/локаль затеняют топ-левел)
3) позиция на имени декларации → её name-range 4) нет локальной
декларации (builtin/неизвестное) → found=false. Requirements:
детерминированность.

Детализация дизайна (проверено трассировкой по коду):
- top-level (function/variable/extern/rule/event, `decl.Name == name`,
  Kind ≠ include): scope `[decl.Range.Start, EOF)`, nameRange = первое
  вхождение имени внутри `decl.Range`, глубина 0
- параметр `p` функции `F`: scope `[nameRange(p), F.Range.End)`,
  nameRange = первое вхождение имени внутри `F.Range` (params до тела),
  глубина 1
- локал: `Stmt{Kind: StmtDecl}` c item `Expr{Kind: ExprIdent,
  Value == name}`: scope `[item.Range.Start, конец объемлющего блока)`,
  nameRange = item.Range, глубина = глубина блока
- выбор: максимум глубины среди содержащих `o.at`; ties → последний
  предшествующий декларатор

**Usages relevant to this task:**
- `conventions`: декомпозиция на именованные хелперы вместо вложенности
- `xs_grammar`: C-like области; extern без тела
- `positions-and-diagnostics` from Imports: Range.Contains для областей

**CRITICAL: `CODEMANIFEST` files — read-only.**

- [ ] **STEP 0 (DECLARATION)**: Task 3
- [ ] **STEP 1 (CONTRACT TESTS)**: в `xs/navigation_test.go` —
  `TestXsDefinition_APIShape`: вызов `Definition(pos)` возвращает
  `(common.Range, bool)` на минимальном src; compile fail до реализации
- [ ] **STEP 2 (IMPLEMENTATION)**: `Definition` в `xs/ast.go` по
  алгоритму выше; внутренние хелперы: сбор top-level кандидатов, обход
  тел функций (params/locals, рекурсивно по `Stmt.Body`), выбор по
  глубине/близости
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: `go test ./xs/... -count=1`
- [ ] **STEP 4 (LOGIC TESTS)**:
  `TestXsDefinition_ParamShadowsTopLevel` (`int x = 1; void f(float x)
  { x = 2; }`, pos на `x` в теле → name-токен параметра);
  `TestXsDefinition_OnDeclarationReturnsItself` (pos на `f` в `void f()`
  → сам токен); `TestXsDefinition_TopLevelVariable` (`int x = 1; void
  f() { x = 2; }` → токен декларации); `TestXsDefinition_BuiltinNotFound`
  (`xsSetWorldGravity` в теле → found=false);
  `TestXsDefinition_LocalShadowsOuterLocal` (локал `x` в блоке if
  глубже внешнего `x` той же функции)
- [ ] **STEP 5 (DEBUGGING)**: `go test ./xs/... -count=1` зелёный
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract xs`;
  детерминированность (повторный вызов — тот же результат)
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
  `feat: xs navigation — Definition с затенением областей (Task 3)`

### Task 4: rms-парсер — фикс `</name>` + word-индекс (TDD coding)

Ячейка `rms`. Контрактных сущностей НЕТ (внутренняя инфраструктура для
Task 5 + дефект-фикс из Applied Fixes дизайна). Файлы:
`rms/parse.go` (запись индекса, закрытие секций), `rms/ast.go`
(неэкспортированное поле `words`).

Дефект (fix, одобрен пользователем): закрывающий тег `</name>`
обрабатывался как открывающий → фантомная пустая Section
материализуется на следующем `<header>`. Фикс: строка с префиксом `</`
→ `closeSection(pos(i, 0))` + `p.cur = &sect{name: "global",
start: pos(i, 0)}` (по `rms_grammar`: statements вне секций —
глобальные).

Word-индекс (алгоритм дизайна, дословно):
```
type wordOcc struct { name string; at common.Range }
RmsFile.words []wordOcc — неэкспортированное поле
Запись (в порядке разбора):
1. run(): матч sectionHeader → колонка '<' + (2 если "</", иначе 1)
   → Range длины len(name)
2. commandLine(): первый токен (имя команды)
3. buildAttribute(): первый токен (имя атрибута)
4. построение Expr аргументов: токены, становящиеся KindConst/KindIdent
   значениями (вкл. имена хелперов вида rand_float)
```
Числа, проценты, структурные ключевые слова (if/percent_chance/…),
`#`-строки — НЕ записываются.

**Usages relevant to this task:**
- `conventions`: table-driven тесты парсера
- `rms_grammar`: секции и их закрытие; команды/атрибуты/выражения

**CRITICAL: `CODEMANIFEST` files — read-only. Контракт rms не меняется — только внутренности парсера.**

- [ ] **STEP 0 (DECLARATION)**: Task 4
- [ ] **STEP 1 (CONTRACT TESTS)**: не применимо (контрактных сущностей
  нет) — вместо них red-тесты дефекта: в `rms/parse_test.go` добавить
  `TestParse_ClosingTag_NoPhantomSection` (src `<PLAYER_SETUP>
  random_placement </PLAYER_SETUP> <LAND_GENERATION> base_terrain
  GRASS </LAND_GENERATION>` → Sections == [player_setup,
  land_generation], обе непустые — СЕЙЧАС падает: фантом) и
  `TestParse_ClosingTag_PostCloseGlobal` (команда после `</A>` — в
  global-секции)
- [ ] **STEP 2 (IMPLEMENTATION)**: фикс закрытия тегов в `run()`
  (см. выше); проверить, что пустой global по-прежнему отбрасывается
- [ ] **STEP 2 (IMPLEMENTATION)**: `wordOcc` + поле `words` в
  `rms/ast.go`; запись в 4 точках `parse.go` (sectionHeader-матч в
  `run()` — вычисление колонки имени внутри `<>`; `commandLine`
  первый токен; `buildAttribute` первый токен; ident/const-токены при
  построении Expr аргументов)
- [ ] **STEP 3 (INTERFACE VERIFICATION)**:
  `go test ./rms/... -count=1` — дефект-тесты зелёные, регресс
  существующих parse-тестов зелёный
- [ ] **STEP 4 (LOGIC TESTS)**: `TestParse_WordIndexRecordsAllKinds`
  (внутренний тест пакета: после Parse — `words` содержит имя секции
  (открывающий И закрывающий теги), имя команды, имя атрибута,
  ident/const-значение; числа в `words` отсутствуют)
- [ ] **STEP 5 (DEBUGGING)**: `go test ./rms/... -count=1` зелёный;
  полный прогон `analysis`/`server` не сломан (`go test ./...` в
  песочнице)
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract rms` —
  без изменений (зелёный); AST-контракт (Sections/Statements/XsBlocks)
  не расширен экспортно
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
  `fix: rms parse — закрывающие теги секций + word-индекс (Task 4)`

### Task 5: `rms.Symbols` + `rms.ReferencesAt` — outline-дерево (TDD coding)

Ячейка `rms`. Контрактные сущности: методы `RmsFile` `Symbols()` и
`ReferencesAt(pos)`, `location: rms/ast.go`. Опираются на word-индекс
(Task 4) и материализованные Sections.

Контрактный Algorithm Symbols (дословно):
1. Каждая Section → `Symbol` по `symbols`: kind=section, Name без
   угловых скобок, Range=Range секции, Selection=диапазон токена
   заголовка
2. Дети секции — Statements в исходном порядке: kind=command,
   Name=имя команды, Range=statement с атрибутами,
   Selection=токен имени; блоки random/conditional рекурсивно в Children
3. Каждый XsBlock → узел kind=xs, Name="#includeXS",
   Range=Selection=Range блока; поместить в содержащую его Section,
   иначе в корень, по позиции среди детей
4. Statements вне секций — узлы корня
Requirements: Selection ⊆ Range; дети внутри Range родителя.

Детализация дизайна: секция `Name == "global"` НЕ эмитится как узел —
её Statements идут в корень; Selection секции/команды — word из
`words`, ближайший к `Range.Start` (fallback: Selection = Range);
ReferencesAt — word под pos → фильтр по имени → sort.

**Usages relevant to this task:**
- `symbols` from Imports: словарь kind (section/command/xs),
  Selection ⊆ Range, construct-and-use
- `rms_grammar`: вложенность random/conditional; `#includeXS`
- `positions-and-diagnostics` from Imports: Range.Contains
- `conventions`: table-driven

**CRITICAL: `CODEMANIFEST` files — read-only.**

- [ ] **STEP 0 (DECLARATION)**: Task 5
- [ ] **STEP 1 (CONTRACT TESTS)**: в новом `rms/navigation_test.go` —
  `TestRmsSymbols_APIShape` (`[]common.Symbol`), 
  `TestRmsReferencesAt_APIShape` (`[]common.Range`); compile fail
  ожидаем
- [ ] **STEP 2 (IMPLEMENTATION)**: `Symbols()` в `rms/ast.go`: сбор
  корней (global-flatten), `stmtSymbol` рекурсивно (command → узел,
  random/conditional → Children), XsBlock-узлы по вхождению Range в
  секцию (иначе корень), позиционная вставка среди детей
- [ ] **STEP 2 (IMPLEMENTATION)**: `ReferencesAt(pos)` — word под pos →
  одноимённые → sort
- [ ] **STEP 3 (INTERFACE VERIFICATION)**: `go test ./rms/... -count=1`
- [ ] **STEP 4 (LOGIC TESTS)**:
  `TestRmsSymbols_SectionTreeWithNestedBlocks` (секция + команда с
  атрибутом + start_random/percent_chance/create_land/end_random:
  дерево 1→2, у второго 1 ребёнок; Selection секции = имя внутри `<>`;
  атрибуты узлами не являются);
  `TestRmsSymbols_GlobalStatementsAtRoot` (команда до первой секции:
  корень = [command, section], узла "global" нет);
  `TestRmsSymbols_XsBlockPlacement` (блок в секции → Children; вне —
  корень; Range=Selection=Range блока);
  `TestRmsReferencesAt_AllWordKinds` (одноимённые команда/константа/
  атрибут: все диапазоны, отсортированы);
  `TestRmsReferencesAt_NumberEmpty` (pos на числе — пусто)
- [ ] **STEP 5 (DEBUGGING)**: `go test ./rms/... -count=1` зелёный
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract rms`;
  `SectionAt`/`StatementAt` без регресса
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
  `feat: rms navigation — Symbols + ReferencesAt (Task 5)`

### Task 6: server — capabilities + `Definition` (TDD coding)

Ячейка `server` (корень). Контрактные сущности: `Initialize` (change:
+3 Boolean-capabilities) и метод `Server.Definition`,
`location: server/server.go`. Протокольные формы — строго
`lsp-protocol` (Navigation-секция).

Контрактные Algorithm (дословно):
- Initialize: «Заявить возможности: … DefinitionProvider,
  ReferencesProvider, DocumentSymbolProvider — Boolean(true);
  согласовать positionEncoding (utf-8 при поддержке клиентом)»
- Definition: «1. Открыть документ; .xs → `XsParse` → Definition(pos)
  по `xs-parsing`: found → Location{URI, Range} 2. Не найдено
  (builtin/неизвестное), .rms или документ закрыт — пустая
  LocationSlice (не nil) по `lsp-protocol` 3. Пустой результат ошибкой
  не считать»; Requirements: конвертация позиций — с учётом
  согласованного positionEncoding

Паттерн: разбор языка по расширению URI (как Hover/Completion),
`openDocument`, `fromProtocolPos`, `toProtocolRange` — существуют.

**Usages relevant to this task:**
- `lsp-protocol`: `DefinitionResult` arms — `*protocol.Location` (один
  сайт) | `protocol.LocationSlice{}` (пусто, не nil);
  Boolean-capabilities
- `xs-parsing` from Imports: контракт `Definition` (innermost
  declarer, builtin → found=false, reparse before query)
- `conventions`: хелперы вместо вложенности

**CRITICAL: `CODEMANIFEST` files — read-only.**

- [ ] **STEP 0 (DECLARATION)**: Task 6
- [ ] **STEP 1 (CONTRACT TESTS)**: в новом `server/navigation_test.go`
  — `TestServerDefinition_APIShape` (метод существует, сигнатура
  совместима с `protocol.Server`: вызов с nil-safe params на закрытом
  документе возвращает `protocol.LocationSlice{}`, err nil);
  `TestInitialize_APIShape` (три поля capabilities установлены)
- [ ] **STEP 2 (IMPLEMENTATION)**: `Initialize`: добавить
  `DefinitionProvider/ReferencesProvider/DocumentSymbolProvider:
  protocol.Boolean(true)` (прежде не трогать)
- [ ] **STEP 2 (IMPLEMENTATION)**: `Definition` в `server.go`:
  openDocument → .xs → XsParse → Definition → `&protocol.Location{URI,
  Range: toProtocolRange(r)}` | `protocol.LocationSlice{}`
- [ ] **STEP 3 (INTERFACE VERIFICATION)**:
  `go test ./server/... -count=1`
- [ ] **STEP 4 (LOGIC TESTS)**:
  `TestInitialize_AdvertisesNavigation` (3 Boolean(true), прежние caps
  не изменены); `TestServerDefinition_XsLocation`
  (`docs.Put("file:///t.xs", "void f() {}\nvoid g() { f(); }")`, pos на
  `f` в теле g → `*protocol.Location`, URI совпадает, Range = токен `f`
  декларации); `TestServerDefinition_EmptyNotNil` (три случая: .rms,
  закрытый документ, builtin → `LocationSlice{}` len 0, err nil)
- [ ] **STEP 5 (DEBUGGING)**: `go test ./server/... -count=1` зелёный
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract server` —
  Definition на месте, Initialize-обязательства выполнены
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
  `feat: server navigation — capabilities + Definition (Task 6)`

### Task 7: server — `References` + `DocumentSymbol` (TDD coding)

Ячейка `server`. Контрактные сущности: методы `Server.References` и
`Server.DocumentSymbol`, `location: server/server.go`.

Контрактные Algorithm (дословно):
- References: «1. ReferencesAt(pos) по языку (`xs-parsing` /
  `rms-parsing`) 2. Каждый Range → Location 3. IncludeDeclaration=false
  и язык .xs → исключить range из Definition(pos) (декларация); для
  .rms исключений нет — локальных деклараций нет 4. Пустой результат —
  пустой список, не nil»
- DocumentSymbol: «1. Symbols() по языку 2. Рекурсивно `Symbol` →
  protocol.DocumentSymbol по `symbols`: Kind по таблице (xs:
  function/extern→Function, variable→Variable, rule/event→Event; rms:
  section→Module, command→Function, xs→Namespace; неизвестный kind →
  Field); Range/Selection напрямую, Children рекурсивно 3. Вернуть
  DocumentSymbolSlice (иерархическая форма)»; Requirements:
  Selection ⊆ Range сохраняется при маппинге

**Usages relevant to this task:**
- `lsp-protocol`: References — `[]protocol.Location` + honor
  `params.Context.IncludeDeclaration`; DocumentSymbol —
  `DocumentSymbolSlice` (hierarchical arm), `protocol.SymbolKind`
  константы; `protocol.NewOptional` не нужен для этих полей
- `xs-parsing`/`rms-parsing` from Imports: контракты ReferencesAt/
  Symbols обоих языков
- `symbols` from Imports: kind-словари производителей

**CRITICAL: `CODEMANIFEST` files — read-only.**

- [ ] **STEP 0 (DECLARATION)**: Task 7
- [ ] **STEP 1 (CONTRACT TESTS)**: в `server/navigation_test.go` —
  `TestServerReferences_APIShape` + `TestServerDocumentSymbol_APIShape`
  (сигнатуры совместимы с `protocol.Server`; пустой документ → пустые
  результаты, err nil)
- [ ] **STEP 2 (IMPLEMENTATION)**: `References`: язык → parse →
  ReferencesAt; `.xs && !params.Context.IncludeDeclaration` → вычесть
  `Definition(pos)`-диапазон; `make([]protocol.Location, 0, n)` + маппинг
- [ ] **STEP 2 (IMPLEMENTATION)**: `DocumentSymbol`: Symbols() →
  рекурсивный `toDocumentSymbol(sym)` (kind-таблица контракта;
  Range/SelectionRange через `toProtocolRange`; Children рекурсивно) →
  `protocol.DocumentSymbolSlice`
- [ ] **STEP 3 (INTERFACE VERIFICATION)**:
  `go test ./server/... -count=1`
- [ ] **STEP 4 (LOGIC TESTS)**:
  `TestServerReferences_ExcludesDeclarationOnFlag` (src Task 6;
  IncludeDeclaration=false → 1 локация, true → 2);
  `TestServerDocumentSymbol_KindTable` (.xs f/x/rule →
  Function/Variable/Event; .rms секция+команда+xs-блок →
  Module/Function/Namespace; Children вложены; SelectionRange ⊆ Range);
  `TestServerDocumentSymbol_EmptyDoc` (пустой .rms/.xs → пустой
  DocumentSymbolSlice, err nil)
- [ ] **STEP 5 (DEBUGGING)**: `go test ./server/... -count=1` зелёный
- [ ] **STEP 6 (CONTRACT RE-VERIFICATION)**: `goga contract server`;
  Hover/Completion/диагностика без регресса
- [ ] **STEP 7 (LINT)**: `goimports -w .` + `golangci-lint run`
- [ ] **STEP 8 (COMPLETION)**: чекбоксы; коммит
  `feat: server navigation — References + DocumentSymbol (Task 7)`

### Task 8: Интеграционные регрессы и финальная валидация (integration tests)

Сквозные сценарии поверх всех ячеек: инвариантные свипы на корпусе и
stdio-интеграция (чек-лист `docs/arch/lsp-navigation.md`). Файлы:
`xs/navigation_test.go` (свип), `rms/navigation_test.go` (свип),
`server/serve_test.go` (расширение существующего харнесса).

**Usages relevant to this task:**
- `xs-parsing`/`rms-parsing` from Imports: reparse-before-query
- `lsp-protocol`: stdio-bootstrap и Navigation-формы ответов
- `conventions`: fixtures через testdata, без моков чистой логики

**CRITICAL: `CODEMANIFEST` files — read-only.**

- [ ] Создать `TestXsSymbols_PreludeInvariantSweep`: parse
  `docs/ref/ugc-guide/xs/prelude.xs` → Symbols() не паникует; каждый
  узел Selection ⊆ Range; 882 extern-узла
- [ ] Создать `TestRmsSymbols_FixturesInvariantSweep`: все
  `rms/testdata/*.rms` → для каждого узла Selection ⊆ Range, дети ⊆
  Range родителя, узлов "global" нет
- [ ] Расширить `server/serve_test.go` интеграционным
  `TestServerNavigation_IntegrationStdio`: initialize → capabilities
  содержат definition/references/documentSymbol → didOpen .xs →
  textDocument/definition → Location; documentSymbol .rms → дерево;
  references → []Location
- [ ] Прогнать валидацию: `timeout 300 systemd-run --user --scope -p
  MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`
  — зелёный; `go test -race ./server/... -count=1` — зелёный
- [ ] `goimports -w .`, `golangci-lint run`, `goga lint` — чисто
- [ ] `goga contract common` / `xs` / `rms` / `server` — все зелёные
- [ ] Запушить ветку `task/lsp-navigation`, открыть PR в `master`
  (мердж — за пользователем)

---

## Validation Commands

- `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`: Run all tests (только в cgroup-песочнице — runaway-тесты валили машину в OOM)
- `goimports -w .`: Форматирование кода и импортов
- `golangci-lint run`: Линт (вкл. staticcheck)
- `goga lint`: Валидация CODEMANIFEST-файлов
- `goga contract common` / `goga contract xs` / `goga contract rms` / `goga contract server`: Соответствие контракту (facade-доступность всех сущностей)
- `go test -race ./server/... -count=1`: Гонки в server (stateless per request — ожидаемо чисто)

---

## Completion Criteria

- [ ] Every contract entity is implemented in the correct `location`
      (`common/symbol.go`; методы в `xs/ast.go`, `rms/ast.go`,
      `server/server.go`)
- [ ] Every contract entity is accessible from the facade
      (`common.Symbol`, `XsFile.Definition/ReferencesAt/Symbols`,
      `RmsFile.Symbols/ReferencesAt`,
      `Server.Definition/References/DocumentSymbol`)
- [ ] Properties and methods match the declared API (`goga contract`
      зелёный для всех 4 ячеек)
- [ ] Descriptions are reflected in behavior (Selection ⊆ Range;
      include пропускается; пустой slice ≠ nil; kind-таблица;
      IncludeDeclaration)
- [ ] Contract dependencies are met (импорты только common/kb/rms/xs/
      analysis по схеме)
- [ ] Re-exports: отсутствуют по контракту — не добавлены
- [ ] Every coding task followed the TDD workflow (contract tests →
      code → verification → logic tests → debugging →
      re-verification → lint)
- [ ] Contract tests and logic tests cover facade, API, and behavior
      within each coding task
- [ ] Integration tests exist (prelude/fixtures свипы + stdio)
- [ ] No package boundary was expanded (word-индекс неэкспортирован;
      новых ячеек нет)
- [ ] `CODEMANIFEST` files were not modified (contract is read-only)
- [ ] All validation commands pass
- [ ] Дефект `</name>` устранён (TestParse_ClosingTag_*)
- [ ] Ветка `task/lsp-navigation` запушена, PR открыт
