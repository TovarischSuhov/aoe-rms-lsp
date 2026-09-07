# Architecture Plan: lsp-navigation

Source task: `docs/tasks/lsp-navigation.md`. All cells are **modified** (none
created). Contracts approved 2026-09-07 via brainstorm pipeline.

## Topic

`lsp-navigation` — однодокументная навигация в aoe2-lsp
(definition / references / documentSymbol). План: `docs/arch/lsp-navigation.md`.

## Implementation Order

1. **`common`** — лист (нет Imports); все остальные ячейки зависят от его
   типов; `Symbol` нужен производителям outline.
2. **`xs`** (параллельно с 3) — зависит только от `common`; методы `XsFile`.
3. **`rms`** (параллельно с 2) — зависит только от `common`; методы `RmsFile`.
4. **`server`** — корень; потребляет `Symbol`, `XsFile`, `RmsFile`.

## Artifacts

### Cell `common` — MODIFY

#### CODEMANIFEST (`common/CODEMANIFEST`)

Header (Usages/Annotations) — без изменений. Body — добавить после
`Diagnostic`:

```yaml
"Symbol(kind: string, name: string, r: Range, selection: Range)":
  location: symbol.go
  annotations: |
    Узел outline-дерева документа: единая форма для xs.Symbols и
    rms.Symbols; потребитель (server) мапит в LSP DocumentSymbol.

    `kind`: вид узла — набор значений у ячейки-производителя
    (xs: function/variable/rule/event/extern; rms: section/command/xs)
    `name`: отображаемое имя узла
    `r`: диапазон узла целиком
    `selection`: диапазон имени узла

    Requirements:
    - Selection ⊆ Range — инвариант LSP SelectionRange, нарушение
      клиенты отбрасывают
    - диапазоны Children лежат внутри Range родителя

    Constraints:
    - чистый тип данных: construct-and-use, без методов
  properties:
    "Kind -> string": |
      Вид узла; словарь значений — у ячейки-производителя.
    "Name -> string": |
      Отображаемое имя.
    "Range -> Range": |
      Диапазон узла целиком.
    "Selection -> Range": |
      Диапазон имени (⊆ Range).
    "Children -> []Symbol": |
      Вложенные узлы (пусто для листа).
```

Footer — Description заменить на:

```yaml
Description: |
  Общие позиционные типы, диагностика и outline-узлы для LSP-сервера aoe2-lsp.
```

#### `.usages` — CREATE `common/.usages/symbols.md`

```markdown
# Symbols — consuming the common cell

Domain: building and consuming the document outline tree. Target audience:
implementers of the xs and rms cells (producers) and the server cell
(consumer that maps the tree to LSP DocumentSymbol).

## Outline node

Symbol is a pure data node; producers construct it while walking their AST.
Selection is the identifier range, Range covers the whole construct.

```go
sym := common.Symbol{
    Kind:      "function",
    Name:      "regenerateMap",
    Range:     declRange,  // whole declaration
    Selection: nameRange,  // identifier token; must be inside Range
    Children:  nil,        // leaf
}
```

Preconditions:
- Selection ⊆ Range (LSP rejects DocumentSymbol otherwise); children
  ranges lie inside the parent Range.
- Kind vocabulary belongs to the producer: xs — function/variable/rule/
  event/extern; rms — section/command/xs. Consumers map the string to
  protocol.SymbolKind; unknown kinds fall back to a generic kind.

Constraints:
- Construct-and-use data: do not mutate a built tree.
```
```

### Cell `xs` — MODIFY

#### CODEMANIFEST (`xs/CODEMANIFEST`)

Header — блок Imports из common заменить на:

```yaml
Imports:
  - Types:
      - Pos
      - Range
      - Diagnostic
      - Symbol
    Usages:
      - positions-and-diagnostics
      - symbols
    From: common
```

Глобальные Annotations — добавить строку в конец существующего блока:

```yaml
  Use `symbols` from Imports for building outline nodes in Symbols.
```

Body — у `XsFile` в `methods` после `SymbolAt` добавить:

```yaml
    "Definition(pos: Pos) -> r: Range, found: bool": |
      Определение символа под позицией: вхождение → name-range декларирующего
      объявления (LSP textDocument/definition).

      `pos`: позиция в файле
      `r`: диапазон имени декларации (не всей декларации)
      `found`: локальная декларация существует

      Algorithm:
      1. Найти вхождение идентификатора, содержащее `pos` (как SymbolAt);
         нет вхождения — found=false
      2. Среди деклараций файла, объявляющих это имя, взять самую внутреннюю,
         чья область (тело функции/блока) объемлет позицию вхождения;
         при равной вложенности — ближайшую, предшествующую вхождению
         (параметр/локаль затеняют топ-левел)
      3. Позиция на имени декларации — вернуть её собственный name-range
      4. Локальной декларации нет (builtin/неизвестное) — found=false

      Requirements:
      - детерминированность: одинаковый вход → одинаковый результат

    "ReferencesAt(pos: Pos) -> ranges: []Range": |
      Все вхождения имени под позицией (LSP textDocument/references).

      `pos`: позиция в файле
      `ranges`: все вхождения имени, включая декларацию, по позиции

      Algorithm:
      1. Имя под `pos` (как SymbolAt); нет — пустой список
      2. Собрать диапазоны вхождений имени из индекса вхождений
      3. Отсортировать по позиции

      Requirements:
      - совпадение синтаксическое, по имени: одноимённые символы разных
        областей видимости не различаются

    "Symbols() -> symbols: []Symbol": |
      Outline файла: декларации верхнего уровня (LSP documentSymbol).

      `symbols`: узлы топ-деклараций в исходном порядке

      Algorithm:
      1. Каждый Decl из Decls → `Symbol` по `symbols` из Imports:
         Kind и Name из декларации, Range=Range декларации,
         Selection=name-range из внутреннего индекса, Children пусты

      Requirements:
      - Selection ⊆ Range для каждого узла
      - плоский список: тела деклараций не раскрываются
```

Footer — без изменений.

#### `.usages` — EXTEND `xs/.usages/xs-parsing.md` (секция в конец)

```markdown
## Navigation (definition, references, outline)

Position-based navigation over the parsed file — the LSP server calls
these directly, no extra context required.

```go
// definition: identifier occurrence -> declaring name range (innermost
// enclosing declarer wins: param > local > top-level). Builtins and
// unknown names return found=false.
if r, ok := xsFile.Definition(pos); ok {
    // jump target: Location{URI: uri, Range: toProtocolRange(r)}
}

// references: every occurrence of the name under pos, declaration
// included, sorted by position. Name resolution is syntactic —
// same-name symbols from different scopes are not distinguished.
for _, r := range xsFile.ReferencesAt(pos) { ... }

// outline: top-level declarations as Symbol nodes (flat).
syms := xsFile.Symbols() // []common.Symbol
```

Preconditions:
- Parse the document first; navigation answers from the occurrence index
  the parser recorded — ranges are valid for that parse only.
- Definition on a declaration name returns that declaration itself.
```

### Cell `rms` — MODIFY

#### CODEMANIFEST (`rms/CODEMANIFEST`)

Header — блок Imports из common заменить на:

```yaml
Imports:
  - Types:
      - Pos
      - Range
      - Diagnostic
      - Symbol
    Usages:
      - positions-and-diagnostics
      - symbols
    From: common
```

Глобальные Annotations — добавить строку в конец существующего блока:

```yaml
  Use `symbols` from Imports for building outline nodes in Symbols.
```

Body — у `RmsFile` в `methods` после `StatementAt` добавить:

```yaml
    "Symbols() -> symbols: []Symbol": |
      Outline-дерево файла (LSP textDocument/documentSymbol).

      `symbols`: узлы секций и глобальные узлы вне секций

      Algorithm:
      1. Каждая Section → `Symbol` по `symbols` из Imports: kind=section,
         Name без угловых скобок, Range=Range секции,
         Selection=диапазон токена заголовка
      2. Дети секции — Statements в исходном порядке: kind=command,
         Name=имя команды, Range=statement с атрибутами,
         Selection=токен имени; блоки random/conditional рекурсивно
         в Children
      3. Каждый XsBlock → узел kind=xs, Name="#includeXS",
         Range=Selection=Range блока; поместить в содержащую его
         Section, иначе в корень, по позиции среди детей
      4. Statements вне секций — узлы корня

      Requirements:
      - Selection ⊆ Range для каждого узла; дети внутри Range родителя

    "ReferencesAt(pos: Pos) -> ranges: []Range": |
      Все вхождения имени под позицией (LSP textDocument/references).

      `pos`: позиция в файле
      `ranges`: диапазоны одноимённых слов-токенов, по позиции

      Algorithm:
      1. Слово-токен, содержащий `pos`, из внутреннего индекса токенов;
         нет — пустой список
      2. Собрать все одноимённые слова-токены (имена констант и команд)
      3. Отсортировать по позиции

      Requirements:
      - локальных деклараций в RMS нет: все вхождения равнозначны
        (вызывающая сторона не отбрасывает декларацию)
```

Footer — без изменений.

#### `.usages` — EXTEND `rms/.usages/rms-parsing.md` (секция в конец)

```markdown
## Navigation (outline, references)

```go
// outline: section tree with command children and #includeXS nodes;
// Selection ⊆ Range on every node; nested random/conditional blocks
// become Children.
syms := file.Symbols() // []common.Symbol, kinds: section/command/xs

// references: every word-token matching the name under pos (constants
// and command names), sorted by position. RMS has no local
// declarations — all occurrences are equal.
for _, r := range file.ReferencesAt(pos) { ... }
```

Preconditions:
- Navigation answers from the token/occurrence index recorded at parse
  time — reparse before querying after text changes.
```

### Cell `server` — MODIFY

#### CODEMANIFEST (`server/CODEMANIFEST`)

Header — добавить новый блок Imports (после существующих):

```yaml
  - Types:
      - Symbol
    Usages:
      - symbols
    From: common
```

Глобальные Annotations — добавить строку в конец существующего блока:

```yaml
  Навигационные хендлеры (Definition/References/DocumentSymbol) следуют
  секции Navigation `lsp-protocol`: пустой результат — пустой slice, не nil.
```

Body — у `Server`:

аннотацию метода `Initialize` заменить на:

```yaml
    "Initialize(ctx: Context, params: InitializeParams) -> result: InitializeResult, err: error": |
      Заявить возможности: TextDocumentSync Full+OpenClose, HoverProvider
      Boolean(true), CompletionProvider с триггерами; DefinitionProvider,
      ReferencesProvider, DocumentSymbolProvider — Boolean(true);
      согласовать positionEncoding (utf-8 при поддержке клиентом).
```

после `Completion` добавить методы:

```yaml
    "Definition(ctx: Context, params: DefinitionParams) -> result: DefinitionResult, err: error": |
      Переход к определению символа под позицией.

      Algorithm:
      1. Открыть документ; .xs → `XsParse` → Definition(pos) по
         `xs-parsing`: found → Location{URI, Range}
      2. Не найдено (builtin/неизвестное), .rms или документ закрыт —
         пустая LocationSlice (не nil) по `lsp-protocol`
      3. Пустой результат ошибкой не считать

      Requirements:
      - конвертация позиций — с учётом согласованного positionEncoding

    "References(ctx: Context, params: ReferenceParams) -> locations: []Location, err: error": |
      Ссылки на символ под позицией.

      Algorithm:
      1. ReferencesAt(pos) по языку (`xs-parsing` / `rms-parsing`)
      2. Каждый Range → Location
      3. IncludeDeclaration=false и язык .xs → исключить range из
         Definition(pos) (декларация); для .rms исключений нет —
         локальных деклараций нет
      4. Пустой результат — пустой список, не nil

    "DocumentSymbol(ctx: Context, params: DocumentSymbolParams) -> result: DocumentSymbolResult, err: error": |
      Outline документа.

      Algorithm:
      1. Symbols() по языку (`xs-parsing` / `rms-parsing`)
      2. Рекурсивно `Symbol` → protocol.DocumentSymbol по `symbols`:
         Kind по таблице (xs: function/extern→Function, variable→Variable,
         rule/event→Event; rms: section→Module, command→Function,
         xs→Namespace; неизвестный kind → Field);
         Range/Selection напрямую, Children рекурсивно
      3. Вернуть DocumentSymbolSlice (иерархическая форма)

      Requirements:
      - Selection ⊆ Range сохраняется при маппинге (инвариант `symbols`)
```

`DocStore`, `Serve` и Footer — без изменений.

#### `.usages` — EXTEND `server/.usages/lifecycle.md` (секция в конец)

```markdown
## Advertised capabilities

Initialize advertises: diagnostics (Full sync + OpenClose), hover,
completion, and navigation — definition, references, documentSymbol.
Editor configs need no extra flags; positionEncoding is negotiated
(prefer utf-8 when the client offers it).
```

## Dependency Map

```
                    ┌─(Pos,Range,Diagnostic,Symbol)──→ xs
 common ◄── лист ───┼─(Pos,Range,Diagnostic,Symbol)──→ rms
                    ├─(Pos,Range,Diagnostic)─────────→ analysis   (не меняется)
                    └─(Symbol)───────────────────────→ server    (новое ребро)

 kb ─────────────────────────────────┐
 xs ──(XsFile, XsParse)──────────────┤
 rms ─(RmsFile, Parse, XsBlock)──────┼──→ server
 analysis ─(Analyzer)────────────────┘
```

Циклов нет (cross-import prohibition соблюдена: `analysis`→`xs` существует,
поэтому навигация XS живёт в `xs`, без обратного импорта).

## Verification Checklist

После каждой ячейки:

- [ ] `goga lint` — 0 ошибок; `goga contract <cell>` — зелёный
- [ ] `go test ./<cell>/...` — зелёный (table-driven по `conventions`)
- [ ] `goimports -w .`, `golangci-lint run` — чисто

Специфика:

- [ ] `common`: `Symbol` — чистые данные; тест-инвариант Selection ⊆ Range
- [ ] `xs`: Definition — затенение (параметр/локаль vs топ-левел), курсор
      на декларации → сама, builtin → found=false; ReferencesAt — вкл.
      декларацию, сортировка; Symbols — плоский, Selection ⊆ Range
- [ ] `rms`: Symbols — вложенность random/conditional, XsBlock в
      содержащей секции; ReferencesAt — слова-токены
- [ ] `server`: capabilities в Initialize; Definition на builtin/.rms →
      пустая LocationSlice (не nil); References — исключение декларации
      по IncludeDeclaration; DocumentSymbol — kind-таблица, Selection ⊆ Range
- [ ] Интеграционный stdio-тест: initialize → definition (.xs) → Location;
      documentSymbol (.rms) → дерево; references → []Location
- [ ] Финал: `go test ./...` (в cgroup-песочнице), `goga lint`,
      `goga contract` для всех 4 ячеек
