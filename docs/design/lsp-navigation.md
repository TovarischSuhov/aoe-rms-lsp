# Design Document: `lsp-navigation`

Complete architectural specification for single-document navigation in
aoe2-lsp: definition / references / documentSymbol. Sources:
`docs/arch/lsp-navigation.md` (brainstorm), материализованные контракты
(commits `5942507`, `9ae6a1b`). No implementation code is written at
this stage.

---

## Contract Changes

### Changed CODEMANIFEST Files

- `common/CODEMANIFEST` (modify, материализован):
  - Body: `+Symbol` (Entity, `symbol.go`) после `Diagnostic`
  - Footer: Description дополнен («outline-узлы»)
- `xs/CODEMANIFEST` (modify, материализован):
  - Imports из common: `+Symbol`; `+symbols` usage
  - Annotations: `+Use \`symbols\` …` строка
  - `XsFile`: `+Definition`, `+ReferencesAt`, `+Symbols` (после `SymbolAt`)
- `rms/CODEMANIFEST` (modify, материализован):
  - Imports из common: `+Symbol`; `+symbols`
  - Annotations: `+Use \`symbols\` …` строка
  - `RmsFile`: `+Symbols`, `+ReferencesAt` (после `StatementAt`)
- `server/CODEMANIFEST` (modify, материализован):
  - `+Imports`-блок из common (`Symbol`, `symbols`)
  - Annotations: `+`строка о навигационных хендлерах (пустой slice ≠ nil)
  - `Initialize`: capabilities `+DefinitionProvider/ReferencesProvider/
    DocumentSymbolProvider`
  - `Server`: `+Definition`, `+References`, `+DocumentSymbol` (после
    `Completion`)

### New Entities

- `Symbol` — Entity, `common/symbol.go`: узел outline-дерева (Kind/Name/
  Range/Selection/Children), чистые данные
- Методы-контракты (новые точки входа):
  - `XsFile.Definition(pos) (Range, bool)` — `xs/ast.go`
  - `XsFile.ReferencesAt(pos) []Range` — `xs/ast.go`
  - `XsFile.Symbols() []Symbol` — `xs/ast.go`
  - `RmsFile.Symbols() []Symbol` — `rms/ast.go`
  - `RmsFile.ReferencesAt(pos) []Range` — `rms/ast.go`
  - `Server.Definition/References/DocumentSymbol` — `server/server.go`

### Changed Entities

- `Server.Initialize` — только аннотация (capabilities); сигнатура та же

### Deleted Entities

- нет

### Usages and Annotations Changes

- `common/.usages/symbols.md` — создан (домен: построение/потребление
  outline-узла, инвариант Selection ⊆ Range, словари kind)
- `xs/.usages/xs-parsing.md` — секция Navigation (definition/references/
  outline, include пропускается)
- `rms/.usages/rms-parsing.md` — секция Navigation (outline/references,
  состав слов)
- `server/.usages/lifecycle.md` — секция Advertised capabilities
- Глобальные аннотации xs/rms/server — см. выше

## Applied Fixes

### Fixed CODEMANIFEST Defects

- `xs/CODEMANIFEST` `Symbols`: «Каждый Decl» → «Каждый Decl, кроме
  include» + Requirement «include-декларации пропускаются» (reason:
  include-имя — строковый путь, в индексе вхождений его нет → Selection
  не вычислим; словарь kind контракта include не содержит)
- `rms/CODEMANIFEST` `ReferencesAt`: «(имена констант и команд)» →
  «(имена секций, команд, атрибутов и значения ident/const)» (reason:
  единообразный word-индекс; позиция на имени атрибута иначе даёт пустоту)

### Fixed Implementation Defects (по решению пользователя, 2026-09-07)

- `rms/parse.go`: закрывающий тег `</name>` обрабатывался как открывающий
  — `closeSection` + `sect{name}` с тем же именем → фантомная пустая
  Section материализуется на следующем `<header>` (фикстуры проекта
  пишут только открывающие теги, поэтому тесты не ловили). Фикс: строка
  с префиксом `</` закрывает секцию и переключает `p.cur` на `global`
  (по `rms_grammar`: statements вне секций — глобальные). Закрывается
  тестом `TestParse_ClosingTag`.

## Entity Interaction and Data Flow

### Interaction Diagram

```
 common.Symbol ◄── производство ──┐           ┌── потребление ──► server
                                  │           │
 XsParse(text) ─► XsFile ──Symbols()──► []Symbol ─┐
     │                      │                     │  DocumentSymbol:
     │                      └─Definition(pos)─► Range ──► Location   │
     │                      └─ReferencesAt(pos)─► []Range ─► []Location
     │                                              │        (map kind)
     ▼ (индекс вхождений symbols — уже существует)  │
 RmsParse(text) ─► RmsFile ──Symbols()──► []Symbol ─┘
     │        (word-индекс — новый internal)
     └─ReferencesAt(pos) ─► []Range ─► []Location

 Server.Initialize ─► caps: Definition/References/DocumentSymbolProvider
```

### Data Flows

1. **Definition (.xs)**: `params.Position` → `fromProtocolPos` →
   `XsParse(text)` → `XsFile.Definition(pos)` → `(common.Range, bool)` →
   `toProtocolRange` → `&protocol.Location{URI, Range}` как
   `DefinitionResult`; not found → `protocol.LocationSlice{}`.
2. **References**: `ReferencesAt(pos)` → `[]common.Range` → (для .xs при
   `IncludeDeclaration=false` вычесть `Definition(pos)`) →
   `[]protocol.Location` (пустой — `[]protocol.Location{}`, не nil).
3. **Outline**: `Symbols()` → `[]common.Symbol` → рекурсивный маппинг
   (kind-таблица) → `protocol.DocumentSymbolSlice` → `DocumentSymbolResult`.
4. **Слово-индекс rms**: лексер/парсер записывает word-токены (имя секции
   внутри `<>`, имя команды, имя атрибута, ident/const-значения) в
   неэкспортированное поле `RmsFile.words` — источник для `ReferencesAt`
   и Selection-диапазонов в `Symbols`.

### Entity Dependencies

- `server` → `common` (Symbol — новое ребро), `xs`, `rms`, `kb`, `analysis`
- `xs`, `rms` → `common` (Pos/Range/Diagnostic/Symbol)
- Порядок инициализации не меняется: `kb.NewStore()` →
  `analysis.NewAnalyzer(store)` → `server.NewServer(store, analyzer)`.
- Все новые методы — чистые чтения по состоянию запроса; новое
  межвызовное состояние не вводится.

## Code Stack Trace

### Trace: `XsFile.Symbols`

#### Chain

1. **Input**: receiver `XsFile` (после `XsParse`).
2. `for _, decl := range f.Decls`; `decl.Kind == DeclInclude` → skip →
   checkpoint: словарь kind контракта (function/variable/rule/event/
   extern) не содержит include — passed.
3. `Symbol{Kind: decl.Kind, Name: decl.Name, Range: decl.Range}`;
   `Selection` = первое вхождение из `f.symbols` с `name == decl.Name` и
   `at` ⊆ `decl.Range` → checkpoint: индекс записывает имена деклараций
   (`parseDecl`/`parseExtern`/`parseRule` вызывают `record`; `parseEvent`
   — через `parsePrimary` в `parseArgs`) раньше любых вхождений в теле
   (однопроходный парсер, порядок = исходный) — passed.
4. `Children = nil` → checkpoint: контракт «плоский список» — passed.
5. **Output**: `[]common.Symbol` в исходном порядке Decls.

#### Checkpoint Summary

- Selection ⊆ Range: name-токен лежит внутри Range декларации (Range
  начинается с type-слов, включает имя) — passed.
- prelude.xs: 882 extern-а → 882 узла, без обхода тел — passed.

### Trace: `XsFile.ReferencesAt`

#### Chain

1. **Input**: `pos common.Pos`.
2. `SymbolAt(pos)` (существующий) → `(name, ok)`; `!ok` → `nil` →
   checkpoint: пустой список = «нет имени» — passed.
3. `for _, s := range f.symbols`: `s.name == name` → append `s.at` →
   checkpoint: индекс содержит ВСЕ вхождения (decl-имена, параметры,
   локалы, Callee через `parsePrimary`, иденты) — passed.
4. Сортировка по Start (индекс уже в исходном порядке; сортировка —
   контрактная гарантия) → **Output**: `[]common.Range`.

#### Checkpoint Summary

- Синтаксическое совпадение по имени: одноимённые символы разных областей
  не различаются — соответствует контракту — passed.

### Trace: `XsFile.Definition`

#### Chain

1. **Input**: `pos common.Pos`.
2. Вхождение `o` из `f.symbols`, где `o.at.Contains(pos)`; нет →
   `(Range{}, false)` → checkpoint: как SymbolAt — passed.
3. Сбор кандидатов-деклараторов имени `o.name` с областями:
   - top-level decl (function/variable/extern/rule/event,
     `decl.Name == o.name`): область `[decl.Range.Start, EOF)`;
     name-range = первое вхождение имени внутри `decl.Range`
   - параметр `p` функции `F` (`p.Name == o.name`): область
     `[name-токен p, F.Range.End)`; name-токен — первое вхождение имени
     внутри `F.Range` (params идут до тела) → checkpoint: `Param` не
     хранит Range, но токен есть в индексе (`parseParams` → `record`) —
     passed.
   - локал: `Stmt{Kind: StmtDecl}` с item `Expr{Kind: ExprIdent,
     Value == o.name}`; область `[item.Range.Start, конец объемлющего
     блока)` → checkpoint: `StmtDecl` items несут Range — passed.
4. Фильтр: область содержит `o.at`; выбор: максимум вложенности
   (параметр/локаль глубже топ-левела; внутренний блок глубже внешнего);
   при равной вложенности — ближайший предшествующий декларатор →
   checkpoint: детерминировано контрактом — passed.
5. Позиция на самом имени декларации: `o` входит в собственную область
   (области начинаются с name-токена) → возвращает свой name-range →
   checkpoint: шаг 3 контракта — passed.
6. Кандидатов нет (builtin/неизвестное) → `(Range{}, false)` →
   **Output**: `(common.Range, bool)`.

#### Checkpoint Summary

- Затенение param/local > top-level — passed (глубина области).
- `Decl.Kind == DeclInclude`: Name = путь-строка, идентификаторы с таким
  текстом не записываются — кандидатурой не становится — passed.

### Trace: `RmsFile.Symbols`

#### Chain

1. **Input**: receiver `RmsFile`.
2. `for _, sec := range f.Sections`: `sec.Name == "global"` → её
   Statements становятся узлами корня (секционного узла нет) →
   checkpoint: парсер материализует global-секцию только когда она
   непуста (`closeSection`), контракт говорит «узлы вне секций — узлы
   корня» — passed.
3. Секция → `Symbol{Kind: "section", Name: sec.Name, Range: sec.Range,
   Selection: <имя-токен из word-индекса>}` → checkpoint: `Section`
   хранит только Range (start = колонка 0 строки заголовка), поэтому
   Selection берётся из word-индекса (имя внутри `<>`) — passed.
4. Дети: `sec.Statements` → `stmtSymbol`: `Kind="command"`,
   `Name=stmt.Name`, `Range=stmt.Range` (включая атрибуты — `node.end`
   расширяется), `Selection=<токен имени>`; блоки `random`/`conditional`
   рекурсивно в Children; XsBlock-узлы (`Kind="xs"`, `Name="#includeXS"`,
   `Range=Selection=block.Range`) вставляются в Children содержащей
   секции по позиции (block.Range внутри sec.Range), иначе в корень →
   checkpoint: XsBlocks — плоский список на RmsFile с Range — passed.
5. Инвариант: дети ⊆ Range родителя (statement внутри секции по
   построению парсера; XsBlock — по фильтру вхождения) → **Output**:
   `[]common.Symbol`.

#### Checkpoint Summary

- Фантомные пустые секции от `</name>` устранены фиксом парсера —
  см. Applied Fixes — passed (после фикса).
- Атрибуты узлами не становятся (вложенность — только random/
  conditional и xs) — passed.

### Trace: `RmsFile.ReferencesAt`

#### Chain

1. **Input**: `pos common.Pos`.
2. Word-токен `w` из `f.words`, где `w.at.Contains(pos)`; нет → `nil` →
   checkpoint: числа/проценты/структурные ключевые слова (if/
   percent_chance/…) в индекс не попадают — passed.
3. Все `w2.name == w.name` → append `w2.at`; сортировка по позиции →
   **Output**: `[]common.Range`.

#### Checkpoint Summary

- Word-индекс наполняется в 4 точках парсера: `sectionHeader`-матч в
  `run()` (имя внутри `<>`, открывающий И закрывающий теги), первый
  токен `commandLine` (имя команды), первый токен атрибута в
  `buildAttribute`, ident/const-значения при построении Expr аргументов
  (вкл. имена хелперов `rand_float`) — passed.
- `#const NAME`-строки: парсер трактует `#`-строки как комментарии/directives
  — имя константы в индекс не попадает; вхождения-использования
  (KindConst) попадают. Ограничение зафиксировано (не дефект контракта:
  «локальных деклараций в RMS нет»).

### Trace: `Server.Definition`

#### Chain

1. **Input**: `ctx`, `params *protocol.DefinitionParams`.
2. `openDocument(uri)` → `(text, name, ok)`; `!ok` →
   `protocol.LocationSlice{}` → checkpoint: пустой slice ≠ nil по
   `lsp-protocol`/контракту — passed.
3. `.rms` → `protocol.LocationSlice{}` (контракт: definition только для
   .xs; inline-XS вне скоупа MVP) → passed.
4. `.xs` → `xs.XsParse(text, name)` → `Definition(fromProtocolPos(pos))`:
   - found → `&protocol.Location{URI: params.TextDocument.URI,
     Range: toProtocolRange(r)}` — arm `DefinitionResult` → checkpoint:
     `toProtocolRange` существует (server.go) — passed.
   - not found → `protocol.LocationSlice{}`.
5. **Output**: `(protocol.DefinitionResult, nil)` — ошибка не возникает.

### Trace: `Server.References`

#### Chain

1. **Input**: `ctx`, `params *protocol.ReferenceParams`.
2. `openDocument`; `.xs` → `ReferencesAt(pos)`; `.rms` →
   `rms.Parse(text, name)` → `ReferencesAt(pos)` → checkpoint: обе
   сигнатуры возвращают `[]common.Range` — passed.
3. `.xs` и `!params.Context.IncludeDeclaration` → вычесть диапазон
   `Definition(pos)` (если found) из списка; `.rms` — без исключений →
   checkpoint: контракт/`lsp-protocol` (honor IncludeDeclaration) —
   passed.
4. `make([]protocol.Location, 0, len(ranges))` + маппинг →
   **Output**: `([]protocol.Location, nil)`; пустой — пустой slice.

### Trace: `Server.DocumentSymbol`

#### Chain

1. **Input**: `ctx`, `params *protocol.DocumentSymbolParams`.
2. `.xs` → `XsParse` → `Symbols()`; `.rms` → `Parse` → `Symbols()` →
   checkpoint: обе возвращают `[]common.Symbol` — passed.
3. Рекурсивный маппинг `toDocumentSymbol(common.Symbol)`:
   `Name`, `Kind` по таблице, `Range=toProtocolRange(Range)`,
   `SelectionRange=toProtocolRange(Selection)`, `Children`
   рекурсивно → checkpoint: Selection ⊆ Range сохраняется
   (покомпонентный монотонный маппинг) — passed.
4. Kind-таблица (контракт): xs `function`/`extern`→`SymbolFunction`,
   `variable`→`SymbolVariable`, `rule`/`event`→`SymbolEvent`; rms
   `section`→`SymbolModule`, `command`→`SymbolFunction`,
   `xs`→`SymbolNamespace`; неизвестный → `SymbolField` →
   checkpoint: все значения — константы `protocol.SymbolKind` — passed.
5. **Output**: `(protocol.DocumentSymbolSlice, nil)` как
   `DocumentSymbolResult` — иерархическая arm.

### Trace: `Server.Initialize` (изменение)

#### Chain

1. **Input**: `params *protocol.InitializeParams`.
2. Существующие caps (Full+OpenClose, HoverProvider,
   CompletionProvider) не меняются → checkpoint: регресс исключён —
   passed.
3. `+DefinitionProvider: protocol.Boolean(true)`,
   `+ReferencesProvider: protocol.Boolean(true)`,
   `+DocumentSymbolProvider: protocol.Boolean(true)` → checkpoint:
   `protocol.Boolean` — правильная arm для bool-полей по
   `lsp-protocol` — passed.
4. `negotiateEncoding` без изменений → **Output**: `InitializeResult`.

## Algorithm Design

### `Symbol` (common/symbol.go)

**Responsibility**: чистый узел outline-дерева; construct-and-use.

**Algorithm:**
```
1. Структура: Kind, Name string; Range, Selection Range;
   Children []Symbol
2. Инварианты (на совести производителя, проверяются тестами):
   - Selection ⊆ Range
   - Children[*].Range ⊆ Range
```

**Errors:** нет. **Edge Cases:** лист — `Children == nil`.

### Word-индекс rms (internal, rms/parse.go + поле в ast.go)

**Responsibility**: слова-токены файла для ReferencesAt и Selection.

**Algorithm:**
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
Чтение: линейный скан (файлы малы, кэш запрещён контрактом ячейки)
```

**Errors:** нет. **Edge Cases:** структурные ключевые слова, числа,
`#`-строки — не записываются.

### Фикс rms: закрывающие теги (rms/parse.go, run())

**Responsibility**: корректная смена секций.

**Algorithm:**
```
IF строка — sectionHeader И начинается с "</":
  - closeSection(pos(i, 0))
  - p.cur = &sect{name: "global", start: pos(i, 0)}
ELSE (открывающий): как сейчас — closeSection + sect{name}
```

**Errors:** нет новых диагностик. **Edge Cases:** `</x>` без открытой
секции — просто закрывает global (пустой global отбрасывается
существующей проверкой).

### `XsFile.Definition` (xs/ast.go)

**Responsibility**: вхождение → name-range декларирующего объявления.

**Algorithm:**
```
1. o := вхождение из индекса, содержащее pos; нет → (zero, false)
2. Кандидаты (name = o.name):
   a) top-level: decl.Name == name, Kind != include
      scope = [decl.Range.Start, EOF); nameRange = первое вхождение
      name внутри decl.Range; глубина 0
   b) param: для каждой DeclFunction с param.Name == name
      scope = [nameRange, decl.Range.End); nameRange = первое
      вхождение name внутри decl.Range; глубина 1
   c) local: обход тел (Stmt.Body рекурсивно); StmtDecl item
      ExprIdent.Value == name → scope = [item.Range.Start,
      конец объемлющего блока); nameRange = item.Range; глубина =
      глубина блока
3. Отфильтровать по scope.Contains(o.at.Start)
4. MAX глубины; ties → последний предшествующий декларатор
5. Вернуть (nameRange, true); пусто → (zero, false)
```

**Errors:** нет. **Edge Cases:** курсор на builtin → false; курсор на
имени декларации → свой name-range (область включает свой токен).

### `XsFile.ReferencesAt` / `XsFile.Symbols`

**Responsibility**: см. трассы выше — прямая фильтрация индекса /
проекция Decls.

**Algorithm:** ReferencesAt: `SymbolAt` → фильтр по имени → sort.
Symbols: Decls (skip include) → Symbol (Selection = первое вхождение
имени в decl.Range), плоско, исходный порядок.

### `RmsFile.Symbols` (rms/ast.go)

**Responsibility**: дерево outline файла.

**Algorithm:**
```
1. roots := []
2. FOR sec IN Sections:
   - sec.Name == "global" → корни из её Statements (без узла секции)
   - ИНАЧЕ узел section (Selection из word-индекса: слово name,
     ближайшее к sec.Range.Start) + дети:
     * stmtSymbol(stmt) рекурсивно: command → узел (Selection — word
       name у stmt.Range.Start); random/conditional → Children
     * XsBlocks: block.Range ⊆ sec.Range → вставить узел xs в Children
       по позиции
3. XsBlocks вне секций → корни по позиции
4. Вернуть roots
```

**Errors:** нет. **Edge Cases:** Selection-word не найден (парсер
восстановился, декларация синтезирована) → Selection = Range (⊆
тривиально).

### `RmsFile.ReferencesAt` (rms/ast.go)

**Responsibility**: все одноимённые слова.

**Algorithm:** см. трассу — word под pos → фильтр по имени → sort.

### `Server.Definition` / `References` / `DocumentSymbol` (server/server.go)

**Responsibility**: протокольная обвязка; разбор языка по расширению
URI (существующий паттерн Hover/Completion).

**Algorithm:**
```
Definition:   openDocument → .xs → XsParse → Definition → Location |
              LocationSlice{}   (.rms/закрыт/not found)
References:   ReferencesAt по языку → [.xs && !IncludeDeclaration →
              вычесть Definition(pos)] → []Location (пустой ≠ nil)
DocumentSymbol: Symbols() → рекурсив toDocumentSymbol (kind-таблица,
              Selection ⊆ Range) → DocumentSymbolSlice
Initialize:   +3 Boolean(true) capabilities
```

**Errors:** ни один хендлер не возвращает error (пустой результат —
не ошибка). **Edge Cases:** закрытый документ → пустые результаты;
неизвестное расширение → пустые результаты.

## Cross-cutting Concerns

- **Error handling**: новые сигнатуры без error; невозможность
  разрешить символ — пустой результат (не ошибка), по контракту и
  `lsp-protocol`.
- **Logging**: отсутствует — хендлеры детерминированы, без IO за
  пределами DocStore (stateless per request).
- **Validation**: инвариант Selection ⊆ Range проверяется тестами
  производителей (xs/rms) и сохраняется маппингом в server; пустые
  протокольные результаты — всегда пустые slice, не nil.
- **Caching**: нет (кэширование запрещено контрактом analysis; для
  навигации — reparse per request, как в существующих Hover/Completion).
- **Concurrency**: новых разделяемых состояний нет; Server обслуживает
  соединение последовательно (jsonrpc2), DocStore уже синхронизирован.

## Usages Analysis

### `conventions`
- **What it provides**: Go 1.23+, DI, goimports, doc-комментарии на
  экспорт, testify/require, table-driven, `Test<Component>_<Scenario>`.
- **Where used**: все новые файлы (symbol.go, методы ast.go/server.go,
  все *_test.go).
- **Why chosen**: базовая практика проекта (base annotations).
- **How exactly**: doc-комментарий на `Symbol`, каждый метод и хендлер;
  тесты — таблицы случаев c `require`.

### `rms_grammar`
- **What it provides**: секции `<...>`/`</...>`, команды, атрибуты,
  позиционная семантика, выражения DE.
- **Where used**: rms Symbols/ReferencesAt/word-индекс, фикс
  закрывающих тегов.
- **Why chosen**: источник структуры outline и словаря слов.
- **How exactly**: `</name>` — закрытие; statements вне секций —
  глобальные; команды/атрибуты — слова.

### `xs_grammar`
- **What it provides**: виды деклараций, C-like области видимости,
  prelude-толерантность.
- **Where used**: xs Definition (правила областей), Symbols.
- **Why chosen**: семантика затенения param/local/top-level.
- **How exactly**: extern без тела; include — не идентификатор.

### `lsp-protocol`
- **What it provides**: Navigation-секция — arms `DefinitionResult`
  (`*Location` | `LocationSlice`), `[]Location` для References,
  `DocumentSymbolSlice` иерархически, Boolean-capabilities,
  IncludeDeclaration.
- **Where used**: server Definition/References/DocumentSymbol/Initialize.
- **Why chosen**: единственная точка правды о go.lsp.dev формах.
- **How exactly**: пустой результат — `protocol.LocationSlice{}` /
  `[]protocol.Location{}`; `SelectionRange ⊆ Range` — требование
  спецификации.

### `xs_coercion`
- Не используется этим изменением ( домен analysis) — упомянут для
  полноты: новых связей нет.

### Imported Usages

- `symbols` from `common` — Path: `common/.usages/symbols.md`. Как
  строить узлы (Kind-словари, Selection ⊆ Range, construct-and-use).
  Используют: `xs.Symbols`, `rms.Symbols`, `server.DocumentSymbol`.
- `positions-and-diagnostics` from `common` — Path:
  `common/.usages/positions-and-diagnostics.md`. Позиции zero-based,
  сравнение (line, column). Используют: все Range-вычисления.
- `xs-parsing` from `xs` — Path: `xs/.usages/xs-parsing.md`. Секция
  Navigation: контракты Definition/ReferencesAt/Symbols для хендлеров.
- `rms-parsing` from `rms` — Path: `rms/.usages/rms-parsing.md`. Секция
  Navigation: outline/references, reparse-before-query.

## `.usages/` Update

### Cell: `common`

#### New Files
- **`symbols`** → `common/.usages/symbols.md`
  - Reason: домен «outline-узлы» для потребителей (xs/rms/server)
  - Related entities: `Symbol`
  - Статус: создан при материализации, актуален

### Cell: `xs`

#### Existing Files — Consistency
- **`xs-parsing`** → `xs/.usages/xs-parsing.md`
  - Status: актуален (секция Navigation добавлена при материализации,
    include-skip отражён)

### Cell: `rms`

#### Existing Files — Consistency
- **`rms-parsing`** → `rms/.usages/rms-parsing.md`
  - Status: актуален (секция Navigation, состав слов)

### Cell: `server`

#### Existing Files — Consistency
- **`lifecycle`** → `server/.usages/lifecycle.md`
  - Status: актуален (Advertised capabilities)

Новых файлов сверх материализованных не требуется.

## Test Stack Trace

### General Setup

- Парсеры напрямую: `xs.XsParse(src, "test.xs")`, `rms.Parse(src,
  "test.rms")` — syntax-диагностики в навигационных тестах игнорируются.
- Server: `NewServer(store, analyzer)` + `s.docs.Put(uri, text, 1)`
  (без stdio); интеграционные — по существующему паттерну serve_test.go.
- Регресс-фикстуры: `docs/ref/ugc-guide/xs/prelude.xs`,
  `rms/testdata/*.rms`.

### Source File Registry

- `common/symbol.go` → инварианты проверяются тестами производителей
  (отдельный symbol_test.go не нужен — тип без поведения)
- `xs/ast.go` → `xs/navigation_test.go`
- `rms/ast.go`, `rms/parse.go` → `rms/navigation_test.go` +
  `rms/parse_test.go` (фикс закрывающих тегов)
- `server/server.go` → `server/navigation_test.go` + `server/serve_test.go`

---

### Positive Tests

#### `TestXsSymbols_FlatOutline`

**Setup**: src = `void f() { g(); }\nint x = 1;\nrule r { condition x }`.

**Input**: `XsParse(src).Symbols()`.

**Trace**: Symbols → Decls[f, x, r] → 3 узла; Selection f = range
токена `f` (первое вхождение внутри decl.Range); Children nil.

**Assertions**: len=3; Kind/Name по порядку
(`function/f, variable/x, rule/r`); каждый Selection ⊆ Range; Children
пусты.

**Sufficiency**: ядро outline; инвариант Selection ⊆ Range.

#### `TestXsReferencesAt_IncludesDeclarationSorted`

**Setup**: src = `void f() { g(); g(); }\nvoid g() {}`.

**Input**: pos на втором `g` (в теле f).

**Trace**: SymbolAt → "g" → фильтр индекса → [decl g, call g, call g]
→ sort.

**Assertions**: 3 диапазона, возрастают по Start, включают name-токен
декларации `g`.

**Sufficiency**: контракт references (вкл. декларацию, сортировка).

#### `TestXsDefinition_ParamShadowsTopLevel`

**Setup**: `int x = 1;\nvoid f(float x) { x = 2; }`.

**Input**: pos на `x` в `x = 2`.

**Trace**: occurrence x → кандидаты: top-level (глубина 0), param
(глубина 1) → param wins → name-токен параметра.

**Assertions**: found; Range == диапазон токена `x` в `(float x)`.

**Sufficiency**: главное правило затенения.

#### `TestXsDefinition_OnDeclarationReturnsItself`

**Input**: pos на имени `f` в `void f()`.

**Assertions**: found; Range == токен `f` декларации.

**Sufficiency**: шаг 3 контракта (курсор на декларации).

#### `TestXsDefinition_TopLevelVariable`

**Setup**: `int x = 1;\nvoid f() { x = 2; }`; pos на `x` в теле.

**Assertions**: found; Range == токен `x` декларации (топ-левел).

**Sufficiency**: случай без затенения.

#### `TestRmsSymbols_SectionTreeWithNestedBlocks`

**Setup**: `<LAND_GENERATION>\ncreate_player_lands {\n terrain_type DIRT\n}\nstart_random\n percent_chance 50\n create_land TERRAIN_GRASS\nend_random\n</LAND_GENERATION>`.

**Input**: `Parse(src).Symbols()`.

**Trace**: узел section LAND_GENERATION; дети: create_player_lands
(command, Selection=токен имени), start_random → Children=[create_land];
Selection каждого ⊆ Range.

**Assertions**: дерево (1 секция → 2 команды, у второй 1 ребёнок);
Selection секции = имя внутри `<>`; атрибуты узлами не являются.

**Sufficiency**: структура outline rms + вложенность random.

#### `TestRmsSymbols_GlobalStatementsAtRoot`

**Setup**: команда до первой секции + секция.

**Assertions**: корень = [command, section]; узла "global" нет.

**Sufficiency**: flatten global-секции.

#### `TestRmsSymbols_XsBlockPlacement`

**Setup**: `#includeXS\nvoid f() {}\n` внутри секции + блок вне секций.

**Assertions**: узел `xs`/`#includeXS` — в Children содержащей секции /
в корне соответственно; Range=Selection=Range блока.

**Sufficiency**: шаг 3 контракта Symbols.

#### `TestRmsReferencesAt_AllWordKinds`

**Setup**: секция + команда с одноимённой константой в аргументе и
атрибутом.

**Input**: pos на имени команды.

**Assertions**: диапазоны всех одноимённых слов (имя команды,
закрывающий/открывающий тег при совпадении имени, значения аргументов),
отсортированы.

**Sufficiency**: word-индекс по 4 точкам записи.

#### `TestParse_ClosingTag_NoPhantomSection`

**Setup**: `<PLAYER_SETUP>\nrandom_placement\n</PLAYER_SETUP>\n<LAND_GENERATION>\nbase_terrain GRASS\n</LAND_GENERATION>`.

**Trace**: `</PLAYER_SETUP>` → closeSection + global; `<LAND_GENERATION>`
→ материализует player_setup (непустая) + открывает land_generation;
global пуст → отброшен.

**Assertions**: Sections == [player_setup, land_generation]; обе
непустые.

**Sufficiency**: фикс Applied Fixes — регресс против фантомов.

#### `TestParse_ClosingTag_PostCloseGlobal`

**Setup**: `</A>` затем команда без секции.

**Assertions**: команда в global-секции (Sections содержит global).

**Sufficiency**: семантика «после закрытия — глобальные».

#### `TestServerDefinition_XsLocation`

**Setup**: docs.Put("file:///t.xs", `void f() {}\nvoid g() { f(); }`);
pos на `f` в теле g.

**Trace**: Definition → XsParse → XsFile.Definition → Range токена f →
Location.

**Assertions**: result — `*protocol.Location`; URI совпадает; Range =
токен `f` декларации.

**Sufficiency**: сквозной сценарий definition.

#### `TestServerReferences_ExcludesDeclarationOnFlag`

**Setup**: как выше; `IncludeDeclaration=false`.

**Assertions**: len == 1 (только вызов); `=true` → 2.

**Sufficiency**: IncludeDeclaration-семантика.

#### `TestServerDocumentSymbol_KindTable`

**Setup**: .xs с f/x/rule; .rms с секцией+командой+xs-блоком.

**Assertions**: Kind соответственно
Function/Variable/Event/Module/Function/Namespace; Children вложены;
SelectionRange ⊆ Range.

**Sufficiency**: kind-таблица + иерархия.

#### `TestInitialize_AdvertisesNavigation`

**Setup**: `s.Initialize(ctx, &protocol.InitializeParams{})`.

**Assertions**: DefinitionProvider/ReferencesProvider/
DocumentSymbolProvider — `protocol.Boolean(true)`; прежние caps не
изменены.

**Sufficiency**: capabilities-регресс.

### Negative Tests

#### `TestXsDefinition_BuiltinNotFound`

**Setup**: `void f() { xsSetWorldGravity(1.0); }`; pos на callee.

**Assertions**: found=false (kb-имя не декларировано локально).

**Sufficiency**: шаг 4 контракта.

#### `TestServerDefinition_EmptyNotNil`

**Setup**: .rms-документ; закрытый документ; .xs с builtin.

**Assertions**: три случая → `protocol.LocationSlice{}` (не nil, len 0),
err nil.

**Sufficiency**: «пустой slice, не nil» — протокольное требование.

#### `TestXsReferencesAt_NoIdentEmpty`

**Input**: pos на литерале/операторе.

**Assertions**: пустой (nil) список.

**Sufficiency**: консервативность.

#### `TestRmsReferencesAt_NumberEmpty`

**Input**: pos на числе/проценте.

**Assertions**: пустой список.

**Sufficiency**: числа не индексируются.

### Edge Case Tests

#### `TestXsDefinition_LocalShadowsOuterLocal`

**Setup**: локал `x` в блоке if внутри функции с внешним `x`.

**Assertions**: inner block local wins.

**Sufficiency**: вложенность блоков.

#### `TestXsSymbols_PreludeInvariantSweep`

**Input**: `docs/ref/ugc-guide/xs/prelude.xs`.

**Assertions**: Symbols() не паникует; каждый узел Selection ⊆ Range;
882 extern-узла.

**Sufficiency**: приёмка на реальном корпусе.

#### `TestRmsSymbols_FixturesInvariantSweep`

**Input**: все `rms/testdata/*.rms`.

**Assertions**: для каждого узла дерева Selection ⊆ Range и дети ⊆
родитель; узлов "global" нет.

**Sufficiency**: инвариант на корпусе; критерий приёмки arch-плана.

#### `TestServerDocumentSymbol_EmptyDoc`

**Setup**: пустой текст .rms/.xs.

**Assertions**: пустой DocumentSymbolSlice (не nil), err nil.

**Sufficiency**: пустой документ.

#### `TestServerNavigation_IntegrationStdio`

**Setup**: существующий stdio-харнесс serve_test.go.

**Trace**: initialize → capabilities содержат навигацию → didOpen .xs →
textDocument/definition → Location; documentSymbol .rms → дерево;
references → []Location.

**Assertions**: ответы соответствуют юнит-ожиданиям.

**Sufficiency**: сквозная приёмка arch-плана (чек-лист).

## Additional Instructions for the Implementation Agent

- Читать материализованные `CODEMANIFEST` (commits `5942507`,
  `9ae6a1b`) — они авторитетны; данный документ — детализация.
- Порядок работ внутри ячеек: common → xs ∥ rms → server (arch-план);
  фикс закрывающих тегов rms выполнить ДО полагающихся на него
  Symbols-тестов.
- `rms`: word-индекс — неэкспортированное поле + запись в 4 точках
  парсера (см. Algorithm Design); методы контракта — в `rms/ast.go`
  (location RmsFile).
- `xs`: новые методы — в `xs/ast.go` (location XsFile); переиспользовать
  существующий индекс `symbols`/`SymbolAt`, НЕ менять поведение парсера.
- `server`: хендлеры — в `server.go` (location Server); разбор языка по
  расширению — существующий паттерн (Hover/Completion); переиспользовать
  `toProtocolRange`/`openDocument`.
- Экспортируемые имена ровно по контрактам: `common.Symbol` (+поля),
  методы `XsFile`/`RmsFile`/`Server` — `goga contract <cell>` обязан
  пройти для всех 4 ячеек.
- Валидация: `go test ./...` ТОЛЬКО в cgroup-песочнице (`timeout 300
  systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0
  bash -c 'go test ./... -count=1'`), `goimports -w .`,
  `golangci-lint run`, `goga lint`, `goga contract common/xs/rms/server`.
- Гонки не ожидаемы (stateless per request); `go test -race ./server/...`
  полезен при удвоении запусков.
