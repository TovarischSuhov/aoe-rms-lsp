# Architecture Plan: LSP Server for AoE2 RMS + XS

> Topic: `lsp-rms-xs` · Plan: `docs/arch/lsp-rms-xs.md`
> Все ячейки создаются заново (schema пуста; modified — нет).

## Implementation Order

| # | Ячейка | Обоснование порядка |
|---|---|---|
| 1 | `common` | лист: не имеет Imports — нужна всем остальным |
| 2 | `kb` | лист: не имеет Imports; параллельна 3 и 4 |
| 3 | `rms` | лист: Imports только common; параллельна 2 и 4 |
| 4 | `xs` | лист: Imports только common; параллельна 2 и 3 |
| 5 | `analysis` | зависит от common + kb + rms + xs |
| 6 | `server` | корень: зависит от всех; создаётся последней |

## Artifacts

---

### 1. Cell `common` (create)

#### `common/CODEMANIFEST`

```yaml
Usages:
  conventions: .goga/usages/conventions.md

Annotations: |
  Use `conventions` for code writing rules and testing.

  Cell holds pure data types shared by parsers, analysis and server.
  No external dependencies beyond stdlib; no behavior beyond ordering.
  All types are immutable data holders: construct-and-use, no mutation methods.

---

"Pos(line: uint32, column: uint32, offset: int)":
  location: pos.go
  annotations: |
    Позиция в исходном файле: zero-based, координаты LSP.

    `line`: zero-based номер строки
    `column`: zero-based смещение в строке
    `offset`: байтовое смещение от начала файла

    Requirements:
    - сравнение по (line, column); Pos упорядочиваем

    Constraints:
    - чистый тип данных: никаких методов кроме сравнения
  properties:
    "Line -> uint32": |
      Zero-based номер строки.
    "Column -> uint32": |
      Zero-based смещение в строке.
    "Offset -> int": |
      Байтовое смещение от начала файла.

"Range(start: Pos, end: Pos)":
  location: range.go
  annotations: |
    Полуоткрытый промежуток [Start, End).

    `start`: начало, включительно
    `end`: конец, исключительно

    Requirements:
    - End >= Start для корректно построенного диапазона
  properties:
    "Start -> Pos": |
      Начало промежутка (включительно).
    "End -> Pos": |
      Конец промежутка (исключительно).
  methods:
    "Contains(p: Pos) -> contains: bool": |
      Проверка принадлежности позиции промежутку [Start, End).

      `p`: проверяемая позиция

      Algorithm:
      1. Сравнить `p` с нижней и верхней границами промежутка по порядку позиций
      2. Вернуть true, когда `p` лежит внутри [начало, конец)

"Diagnostic(r: Range, severity: int, message: string, code: string)":
  location: diagnostic.go
  annotations: |
    Проблема в файле: единое представление для rms.Parse, xs.Parse и Analyzer.

    `r`: диапазон, к которому относится проблема
    `severity`: 1 error / 2 warning / 3 info / 4 hint
    `message`: человекочитаемый текст
    `code`: стабильный идентификатор правила ("unknown-command", ...)

    Requirements:
    - code стабилен между релизами — на него опираются тесты

    Constraints:
    - значения severity следуют нумерации LSP; новых значений не вводить
  properties:
    "Range -> Range": |
      Диапазон проблемы.
    "Severity -> int": |
      Важность: 1..4 по LSP.
    "Message -> string": |
      Текст проблемы.
    "Code -> string": |
      Стабильный код правила.

---

Author: Goga
CreatedAt: 04/09/26
Description: |
  Общие позиционные типы и диагностика для LSP-сервера aoe2-lsp.
```

#### `common/.usages/positions-and-diagnostics.md`

```markdown
# Positions and Diagnostics — consuming the common cell

Domain: constructing and comparing source positions, and reporting problems as
diagnostics. Target audience: implementers of the rms, xs, analysis and server cells.

## Positions

Positions are zero-based, LSP-aligned: `line` and `column` count from 0.
Order follows (line, column); `offset` is a byte offset kept alongside for O(1) slicing.

```go
start := common.Pos{Line: 3, Column: 0, Offset: 42}
end := common.Pos{Line: 3, Column: 13, Offset: 55}
r := common.Range{Start: start, End: end}

if r.Contains(common.Pos{Line: 3, Column: 7, Offset: 49}) { ... }
```

## Diagnostics

One shape for every producer (rms.Parse, xs.Parse, Analyzer). Construct with a
stable `code` — tests assert on codes, not on message text.

```go
d := common.Diagnostic{
    Range:    r,
    Severity: common.SeverityError, // 1 error, 2 warning, 3 info, 4 hint
    Message:  "unknown command 'create_elefant'",
    Code:     "unknown-command",
}
```

Preconditions:
- `Range.End` >= `Range.Start` for diagnostics you emit; the editor drops invalid ranges.
- Use only severities 1–4 (LSP numbering) — do not invent new values.

Constraints:
- Positions are comparable data; do not mutate them after construction.
```

---

### 2. Cell `kb` (create)

#### `kb/CODEMANIFEST`

```yaml
Usages:
  conventions: .goga/usages/conventions.md
  kbdata: |
    Embedded knowledge base files under kb/data/ (go:embed, no runtime downloads):
    - xs-functions.json  — adapted from docs/ref/ugc-guide/xs/functions/functions.json
      (categories flattened; fields: name, category, return_type, params[], desc, since_update)
    - xs-constants.json — from constants/constants.json (fields: name, section, value, desc, since_update)
    - rms-commands.json — extracted from docs/ref/zetnus-rms-guide.txt
      (fields: name, section, args[], attributes[], desc, game_versions, since_update)
    since_update sourced from docs/ref/aoe2de-xs-rms-changelog.md; empty string when unknown.
    Names must be unique within each file — validated on load.

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `kbdata` for the embedded JSON file layout and schema.

  Cell is the AoE2 RMS+XS knowledge base: loads embedded JSON once at
  construction and serves read-only lookups. No IO after construction.

---

"Store()":
  location: store.go
  annotations: |
    Загруженная база знаний; единственная точка lookup для всех потребителей.

    Requirements:
    - конструктор NewStore() читает файлы по `kbdata` и валидирует схему:
      неизвестные ключи, дубликаты имён, пустые name — ошибка загрузки
    - после построения Store потокобезопасен для чтения (immutable данные)
  methods:
    "Function(name: string) -> fn: Function, found: bool": |
      Lookup XS-функции по точному имени.

      `name`: точное имя функции (case-sensitive); found=false при отсутствии
    "Functions() -> fns: []Function": |
      Все XS-функции (для completion); порядок — как в файле данных.
    "Constant(name: string) -> c: Constant, found: bool": |
      Lookup XS-константы по точному имени; found=false при отсутствии.
    "Constants(section: string) -> cs: []Constant": |
      Константы одной секции ("" — все секции).
    "Command(name: string) -> cmd: Command, found: bool": |
      Lookup RMS-команды по точному имени; found=false при отсутствии.
    "Commands(section: string) -> cmds: []Command": |
      Команды одной секции RMS ("" — все).
    "Attribute(command: string, attr: string) -> arg: CommandArg, found: bool": |
      Спецификация атрибута команды; found=false если команда или атрибут неизвестен.

      `command`: имя команды; `attr`: имя атрибута

"Function()":
  location: model.go
  annotations: |
    XS-функция из базы (данные).
  properties:
    "Name -> string": |
      Имя функции.
    "ReturnType -> string": |
      Тип возврата.
    "Params -> []Param": |
      Параметры функции.
    "Desc -> string": |
      Описание.
    "SinceUpdate -> string": |
      Апдейт добавления ("" если неизвестен).

"Param()":
  location: model.go
  annotations: |
    Параметр XS-функции (данные). Required=false — есть значение по умолчанию.
  properties:
    "Name -> string": |
      Имя параметра.
    "Type -> string": |
      Тип параметра.
    "Required -> bool": |
      Обязательность.
    "Desc -> string": |
      Описание параметра.

"Constant()":
  location: model.go
  annotations: |
    XS-константа из базы (данные).
  properties:
    "Name -> string": |
      Имя константы.
    "Section -> string": |
      Секция констант.
    "Value -> string": |
      Строковое представление значения.
    "Desc -> string": |
      Описание.
    "SinceUpdate -> string": |
      Апдейт добавления ("" если неизвестен).

"Command()":
  location: model.go
  annotations: |
    RMS-команда из базы (данные).

    Requirements:
    - Args — позиционные аргументы; Attributes — допустимые атрибуты команды
  properties:
    "Name -> string": |
      Имя команды.
    "Section -> string": |
      Секция синтаксиса.
    "Args -> []CommandArg": |
      Позиционные аргументы.
    "Attributes -> []CommandArg": |
      Допустимые атрибуты.
    "Desc -> string": |
      Описание.
    "GameVersions -> string": |
      Версии игры, где работает.
    "SinceUpdate -> string": |
      Апдейт добавления ("" если неизвестен).

"CommandArg()":
  location: model.go
  annotations: |
    Спецификация аргумента/атрибута RMS-команды (данные).
  properties:
    "Name -> string": |
      Имя аргумента/атрибута.
    "Kind -> string": |
      Ожидаемая форма значения: number / percent / const / terrain / object / ...
    "Required -> bool": |
      Обязательность.
    "Desc -> string": |
      Описание.

"ExtractRmsCommands(path: string) -> commands: []Command, err: error":
  location: extract.go
  annotations: |
    Разовое извлечение RMS-команд из текстового экспорта гайда Zetnus
    в rms-commands.json (формат по `kbdata`).

    `path`: путь к docs/ref/zetnus-rms-guide.txt
    `commands`: извлечённые команды; `err`: ошибки чтения/разбора

    Algorithm:
    1. Прочитать файл по `path`
    2. Найти секции Syntax Skeleton и пройтись по блокам команд
    3. Для каждой команды собрать Name, Section, Args, Attributes, Desc, GameVersions
    4. Обогатить SinceUpdate по changelog (docs/ref/aoe2de-xs-rms-changelog.md)
    5. Проверить дубликаты имён; вернуть []Command

    Constraints:
    - утилита сборки данных, не вызывается в рантайме сервера
    - при неоднозначном разборе — пропускать фрагмент и писать WARN в slog,
      не прерывать весь проход

---

Author: Goga
CreatedAt: 04/09/26
Description: |
  База знаний AoE2 RMS+XS: встроенные JSON, lookup API и экстракция из Zetnus.
```

#### `kb/.usages/lookups.md`

```markdown
# KB Lookups — consuming the kb cell

Domain: querying the AoE2 RMS+XS knowledge base. Target audience:
implementers of the analysis and server cells.

## Construct once, share everywhere

NewStore() validates and indexes the embedded JSON. Build one Store per
process and inject it (constructor DI per `conventions`).

```go
store, err := kb.NewStore()
if err != nil {
    return fmt.Errorf("load knowledge base: %w", err)
}
```

## Exact-name lookups (hover, validation)

```go
fn, found := store.Function("xsGetMapSeed")
if !found {
    // emit "unknown function" diagnostic
}
cmd, found := store.Command("create_elevator")
arg, found := store.Attribute("create_elevator", "number_of_objects")
```

## List lookups (completion)

```go
for _, fn := range store.Functions() { /* completion items */ }
for _, c := range store.Constants("") { /* all sections */ }
for _, cmd := range store.Commands("land_generation") { /* section-scoped */ }
```

Preconditions:
- Names are case-sensitive; completion should lowercase-filter client-side.
- SinceUpdate is "" when the version is unknown — treat as "always existed".
```

#### `kb/.usages/data-pipeline.md`

```markdown
# KB Data Pipeline — regenerating embedded JSON

Domain: rebuilding kb/data/*.json from local sources in docs/ref/.
Target audience: maintainers updating the knowledge base.

## Sources (all local, no network)

- docs/ref/ugc-guide/xs/functions/functions.json → xs-functions.json
- docs/ref/ugc-guide/xs/constants/constants.json → xs-constants.json
- docs/ref/zetnus-rms-guide.txt → rms-commands.json (via ExtractRmsCommands)
- docs/ref/aoe2de-xs-rms-changelog.md → since_update enrichment

## Regenerate

```go
cmds, err := kb.ExtractRmsCommands("docs/ref/zetnus-rms-guide.txt")
// marshal into kb/data/rms-commands.json per the kbdata schema
```

Preconditions:
- Duplicate names within one file are a build error — resolve, do not skip.
- NewStore() must pass after regeneration (run kb tests).
```

---

### 3. Cell `rms` (create)

#### `rms/CODEMANIFEST`

```yaml
Imports:
  - Types:
      - Pos
      - Range
      - Diagnostic
    Usages:
      - positions-and-diagnostics
    From: common

Usages:
  conventions: .goga/usages/conventions.md
  rms_grammar: .goga/usages/rms-grammar.md

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `rms_grammar` for the RMS language structure being parsed.
  Use `positions-and-diagnostics` from Imports for building positions and diagnostics.

  Parser never fails hard: every problem becomes a Diagnostic; recovery
  resynchronizes to the next statement/section. Every AST node carries a Range.

---

"Parse(source: string, name: string) -> file: RmsFile, diags: []Diagnostic":
  location: parse.go
  annotations: |
    Разбор RMS-файла в AST с восстановлением после ошибок.

    `source`: текст файла; `name`: имя файла (для сообщений)
    `file`: AST (всегда не-nil, возможно частичный); `diags`: синтаксические проблемы

    Algorithm:
    1. Лексер: токены по `rms_grammar` (слова, числа, проценты, строки,
       комментарии, директивы #include/#includeXS, секции <...>)
    2. Построить список секций; вне секций — глобальные statements
    3. Для каждого statement: команда + позиционные аргументы (Expr),
       последующие атрибуты прикреплять к текущей команде (позиционная семантика)
    4. Блоки start_random/percent_chance и if/elseif/else вкладывать в Children
    5. #includeXS — начать XsBlock; собирать строки до следующей директивы/секции
    6. При ошибке: Diagnostic с диапазоном токена, sync на следующий
       statement/секцию, продолжить разбор

    Requirements:
    - возвращённый File всегда не-nil (пустой при полном провале)
    - diags отсортированы по позиции

    Constraints:
    - не паниковать и не возвращать err для некорректного входа

"RmsFile()":
  location: ast.go
  annotations: |
    Корень AST RMS-файла.

    Requirements:
    - XsBlocks содержат встроенный XS-код, но не разбираются здесь
  properties:
    "Name -> string": |
      Имя файла.
    "Sections -> []Section": |
      Секции скрипта.
    "Includes -> []string": |
      Пути #include.
    "XsBlocks -> []XsBlock": |
      Встроенные XS-блоки (#includeXS).
  methods:
    "SectionAt(pos: Pos) -> section: Section, found: bool": |
      Секция, содержащая позицию (для completion-контекста).

      `pos`: позиция в файле
    "StatementAt(pos: Pos) -> stmt: Statement, found: bool": |
      Ближайший statement, содержащий/предшествующий позиции (для hover).

      `pos`: позиция в файле

      Requirements:
      - для позиции на атрибуте возвращает команду-владельца

"XsBlock()":
  location: ast.go
  annotations: |
    Встроенный XS-код (после #includeXS) с диапазоном исходника.

    Constraints:
    - RMS-парсер не разбирает содержимое; потребитель передаёт Code в xs.Parse
  properties:
    "Code -> string": |
      Исходный текст блока (позиции — относительно блока).
    "Range -> Range": |
      Диапазон блока в RMS-файле.

"Section()":
  location: ast.go
  annotations: |
    Секция скрипта (например land_generation). Name — без угловых скобок.
  properties:
    "Name -> string": |
      Имя секции.
    "Statements -> []Statement": |
      Утверждения секции.
    "Range -> Range": |
      Диапазон секции.

"Statement()":
  location: ast.go
  annotations: |
    Команда или блок.

    Requirements:
    - позиционная семантика: Attributes принадлежат команде, после которой записаны
    - Kind=random — Children вложены; Kind=conditional — if/elseif/else
  properties:
    "Kind -> string": |
      Вид: command / random / conditional.
    "Name -> string": |
      Имя команды.
    "Args -> []Expr": |
      Позиционные аргументы.
    "Attributes -> []Attribute": |
      Атрибуты команды.
    "Children -> []Statement": |
      Вложенные statements (блоки).
    "Range -> Range": |
      Диапазон statement.

"Attribute()":
  location: ast.go
  annotations: |
    Атрибут в коде: имя + одно значение-выражение.
  properties:
    "Name -> string": |
      Имя атрибута.
    "Value -> Expr": |
      Значение атрибута.
    "Range -> Range": |
      Диапазон атрибута.

"Expr()":
  location: ast.go
  annotations: |
    Выражение DE-математики: рекурсивное дерево.

    Requirements:
    - Kind=binary: Value=оператор, Children=операнды; Kind=ident/const: Value=имя
  properties:
    "Kind -> string": |
      Вид: number / percent / const / ident / binary / unary.
    "Value -> string": |
      Литерал, имя или оператор.
    "Children -> []Expr": |
      Операнды.
    "Range -> Range": |
      Диапазон выражения.

---

Author: Goga
CreatedAt: 04/09/26
Description: |
  Парсер Random Map Scripts: лексер, AST с восстановлением, навигация по позициям.
```

#### `rms/.usages/rms-parsing.md`

```markdown
# RMS Parsing — consuming the rms cell

Domain: parsing RMS sources and navigating the resulting AST.
Target audience: implementers of the analysis and server cells.

## Parse and collect diagnostics

Parse never fails: a partial File comes back alongside syntax diagnostics.

```go
file, diags := rms.Parse(text, uri)
// diags: syntax problems, sorted by position — merge with analyzer output
```

## Position navigation (hover, completion context)

```go
if stmt, ok := file.StatementAt(pos); ok {
    // hover: stmt.Name is the command under (or owning) the cursor
}
if sec, ok := file.SectionAt(pos); ok {
    // completion: scope command list to sec.Name via kb.Store.Commands(sec.Name)
}
```

## Inline XS delegation

The rms parser does not parse XS. For every embedded block, hand the code
to the xs parser and merge diagnostics with the block's offset applied:

```go
for _, block := range file.XsBlocks {
    xsFile, xsDiags := xs.XsParse(block.Code, "inline:"+uri)
    // shift xsDiags ranges by block.Range.Start before publishing
}
```

Preconditions:
- StatementAt on an attribute position returns the owning command —
  do not re-walk Attributes yourself.
- XsBlock.Code positions are relative to the block, not the document.
```

---

### 4. Cell `xs` (create)

#### `xs/CODEMANIFEST`

```yaml
Imports:
  - Types:
      - Pos
      - Range
      - Diagnostic
    Usages:
      - positions-and-diagnostics
    From: common

Usages:
  conventions: .goga/usages/conventions.md
  xs_grammar: .goga/usages/xs-grammar.md

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `xs_grammar` for the XS language structure being parsed.
  Use `positions-and-diagnostics` from Imports for building positions and diagnostics.

  Parser never fails hard: every problem becomes a Diagnostic; recovery
  resynchronizes to the next statement/declaration. Every AST node carries a Range.

---

"Parse(source: string, name: string) -> file: RmsFile, diags: []Diagnostic":
  location: parse.go
  annotations: |
    Разбор XS-кода (C-like + rules/events) в AST с восстановлением.

    `source`: текст (файл .xs или inline-блок из rms.XsBlock)
    `name`: имя для сообщений

    Algorithm:
    1. Лексер по `xs_grammar`: идентификаторы, числа (int/float/hex), строки,
       векторные литералы (x,y,z), операторы, комментарии // и /* */
    2. Верхний уровень: объявления — functions (с Params), variables, rules
       (условие+тело), events, include, extern
    3. Тела: statements (if/else, while, for, do, switch, return, break,
       выражения-стейтменты, блоки) и выражения
    4. При ошибке: Diagnostic, sync на следующий ';' или '}', продолжить

    Requirements:
    - File всегда не-nil; diags отсортированы по позиции

    Constraints:
    - не паниковать; err не возвращается для некорректного входа

"XsFile()":
  location: ast.go
  annotations: |
    Корень XS: Decls верхнего уровня. Имя — как передано в Parse.
  properties:
    "Name -> string": |
      Имя файла/блока.
    "Decls -> []Decl": |
      Объявления верхнего уровня.
  methods:
    "SymbolAt(pos: Pos) -> name: string, found: bool": |
      Идентификатор под позицией (для hover и completion-триггера).

      `pos`: позиция в файле

      Requirements:
      - для позиции на call возвращает имя вызываемой функции

"Decl()":
  location: ast.go
  annotations: |
    Объявление.

    Requirements:
    - Kind=function — Params и Body заполнены; Kind=rule — условие в Body[0]
      как Exprs; Kind=extern — только сигнатура (prelude.xs)
  properties:
    "Kind -> string": |
      Вид: function / variable / rule / event / include / extern.
    "Name -> string": |
      Имя объявленного символа.
    "Type -> string": |
      Тип (переменной или возврата функции).
    "Params -> []Param": |
      Параметры функции.
    "Body -> []Stmt": |
      Тело.
    "Range -> Range": |
      Диапазон объявления.

"Param()":
  location: ast.go
  annotations: |
    Параметр XS-функции.
  properties:
    "Name -> string": |
      Имя параметра.
    "Type -> string": |
      Тип (int/float/bool/string/vector/void/...).

"Stmt()":
  location: ast.go
  annotations: |
    Оператор; Kind определяет смысл Exprs (условие) и Body (ветки/тело).
  properties:
    "Kind -> string": |
      Вид оператора.
    "Exprs -> []Expr": |
      Выражения (условие/значение).
    "Body -> []Stmt": |
      Вложенные операторы.
    "Range -> Range": |
      Диапазон оператора.

"Expr()":
  location: ast.go
  annotations: |
    Выражение.

    Requirements:
    - Kind=call: Callee=имя функции, Children=аргументы; Kind=vector:
      Children=3 операнда; Kind=binary: Value=оператор
  properties:
    "Kind -> string": |
      Вид: call / binary / unary / literal / vector / ident.
    "Callee -> string": |
      Имя вызываемой функции (для call).
    "Value -> string": |
      Литерал или оператор.
    "Children -> []Expr": |
      Операнды/аргументы.
    "Range -> Range": |
      Диапазон выражения.

---

Author: Goga
CreatedAt: 04/09/26
Description: |
  Парсер XS (External Subroutines): C-like грамматика с rules/events, AST с восстановлением.
```

#### `xs/.usages/xs-parsing.md`

```markdown
# XS Parsing — consuming the xs cell

Domain: parsing XS sources (standalone .xs files and inline RMS blocks)
and navigating the AST. Target audience: implementers of the analysis
and server cells.

## Parse and collect diagnostics

```go
xsFile, xsDiags := xs.XsParse(text, uri)
// partial AST + syntax diagnostics sorted by position
```

## Symbol lookup (hover, completion)

```go
if name, ok := xsFile.SymbolAt(pos); ok {
    if fn, found := store.Function(name); found {
        // hover: fn signature + desc; completion trigger on "("
    }
}
```

Preconditions:
- Parse accepts any C-like input incl. the 5k-line prelude.xs fixture —
  do not pre-validate the source.
- SymbolAt on a call expression returns the callee name, not the argument.
```

---

### 5. Cell `analysis` (create)

#### `analysis/CODEMANIFEST`

```yaml
Imports:
  - Types:
      - Diagnostic
    From: common
  - Types:
      - Store
    Usages:
      - lookups
    From: kb
  - Types:
      - RmsFile
    Usages:
      - rms-parsing
    From: rms
  - Types:
      - XsFile
    Usages:
      - xs-parsing
    From: xs

Usages:
  conventions: .goga/usages/conventions.md
  rms_grammar: .goga/usages/rms-grammar.md
  xs_grammar: .goga/usages/xs-grammar.md

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `rms_grammar` and `xs_grammar` for the languages being checked.
  Use `lookups` from Imports for kb.Store queries.
  Use `rms-parsing`/`xs-parsing` from Imports for AST shapes and navigation.

  Only semantic checks live here: syntax problems are already reported by
  parsers. Analyzer is stateless per call — no caching, no IO.

---

"Analyzer(store: Store)":
  location: analyzer.go
  annotations: |
    Семантические проверки RMS и XS поверх AST + базы знаний.

    `store`: база знаний (DI)

    Requirements:
    - паттерны lookup строго по `lookups`
  methods:
    "AnalyzeRms(file: RmsFile) -> diags: []Diagnostic": |
      Проверки RMS-файла.

      `file`: AST из rms.Parse

      Algorithm:
      1. Пройти все statement (вкл. Children блоков)
      2. Неизвестная команда/секция — lookup через `lookups` (Store.Command);
         Diagnostic severity=error, code="unknown-command"/"unknown-section"
      3. Для известных команд: неизвестные атрибуты (Store.Attribute) —
         code="unknown-attribute"; несоответствие числа/вида аргументов —
         code="bad-argument"
      4. effect_percent — code="deprecated-effect-percent", severity=warning
      5. Отсортировать diags по позиции

      Requirements:
      - не дублировать синтаксические diags парсера

      Constraints:
      - не изменять входной AST

    "AnalyzeXs(file: XsFile) -> diags: []Diagnostic": |
      Проверки XS-файла.

      `file`: AST из xs.Parse

      Algorithm:
      1. Собрать объявленные имена (functions, variables, params, rules)
      2. Пройти выражения: ident вне объявленных и вне kb (Store.Function,
         Store.Constant) — code="undefined-symbol", severity=error
      3. Для вызовов известных функций: число аргументов vs Params
         (Required=false — можно опускать) — code="bad-arity"
      4. Отсортировать diags

      Requirements:
      - не дублировать синтаксические diags парсера

      Constraints:
      - не изменять входной AST

---

Author: Goga
CreatedAt: 04/09/26
Description: |
  Семантический анализ RMS/XS: unknown-символы, арность, устаревания.
```

#### `analysis/.usages/checks.md`

```markdown
# Semantic Checks — consuming the analysis cell

Domain: running semantic diagnostics over parsed RMS/XS files.
Target audience: implementers of the server cell and CLI lint tooling.

## Construct with the knowledge base

```go
analyzer := analysis.NewAnalyzer(store)
```

## Analyze and merge with syntax diagnostics

```go
rmsFile, syntaxDiags := rms.Parse(text, uri)
all := append(syntaxDiags, analyzer.AnalyzeRms(rmsFile)...)
// both sorted by position; publish as one batch
```

Preconditions:
- Input must come from a successful Parse call (partial AST is fine —
  analyzer walks what exists).
- Analyzer does not re-report syntax problems.
Constraints:
- No IO, no mutation of the AST.
```

---

### 6. Cell `server` (create)

#### `server/CODEMANIFEST`

```yaml
Imports:
  - Types:
      - Store
    Usages:
      - lookups
    From: kb
  - Types:
      - Parse
      - RmsFile
      - XsBlock
    Usages:
      - rms-parsing
    From: rms
  - Types:
      - XsParse
      - XsFile
    Usages:
      - xs-parsing
    From: xs
  - Types:
      - Analyzer
    Usages:
      - checks
    From: analysis

Usages:
  conventions: .goga/usages/conventions.md
  lsp-protocol: .goga/usages/cooks/lsp-protocol.md

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `lsp-protocol` for go.lsp.dev patterns (NewServer bootstrap, Full sync,
  PublishDiagnostics, Hover/Completion result shapes, Boolean/Optional arms).
  Use `lookups`, `rms-parsing`, `xs-parsing`, `checks` from Imports for the
  provider APIs wired into handlers.

  Language selection by URI extension: .rms → `Parse` → `RmsFile`
  (+ inline `XsBlock` → `XsParse` with range shift), .xs → `XsParse` →
  `XsFile`. One change = one diagnostics batch.
  Protocol types (InitializeParams, Hover, CompletionList, ...) belong to
  go.lsp.dev/protocol — follow `lsp-protocol` for their construction.

---

"Server(store: Store, analyzer: Analyzer)":
  location: server.go
  annotations: |
    LSP-сервер: протокол поверх kb/rms/xs/analysis.

    `store`: база знаний; `analyzer`: семантические проверки (DI)

    Requirements:
    - паттерны протокола строго по `lsp-protocol` (UnimplementedServer, Full sync)
  methods:
    "Initialize(ctx: Context, params: InitializeParams) -> result: InitializeResult, err: error": |
      Заявить возможности: TextDocumentSync Full+OpenClose, HoverProvider
      Boolean(true), CompletionProvider с триггерами; согласовать
      positionEncoding (utf-8 при поддержке клиентом).
    "DidOpen(ctx: Context, params: DidOpenTextDocumentParams) -> err: error": |
      Algorithm:
      1. DocStore.Put(uri, text, version)
      2. Пересчитать диагностики (общая функция с DidChange)
      3. PublishDiagnostics по `lsp-protocol`
    "DidChange(ctx: Context, params: DidChangeTextDocumentParams) -> err: error": |
      Algorithm:
      1. Взять весь текст из ContentChanges (WholeDocument)
      2. DocStore.Put; пересчитать; PublishDiagnostics
    "DidClose(ctx: Context, params: DidCloseTextDocumentParams) -> err: error": |
      DocStore.Remove(uri); опубликовать пустой список диагностик.
    "Hover(ctx: Context, params: HoverParams) -> hover: Hover, err: error": |
      Algorithm:
      1. Для .rms: StatementAt → Store.Command → Markdown (сигнатура+desc+
         GameVersions/SinceUpdate); для .xs: SymbolAt → Store.Function
      2. nil, nil когда под курсором ничего нет
    "Completion(ctx: Context, params: CompletionParams) -> result: CompletionResult, err: error": |
      Algorithm:
      1. .rms: SectionAt → Store.Commands(section) + константы;
         .xs: префикс слова → Store.Functions + Store.Constants
      2. CompletionList: label, kind, detail=сигнатура, documentation=desc
    "Shutdown(ctx: Context) -> err: error": |
      Вернуть nil (default Unimplemented возвращает not-implemented).
    "Exit(ctx: Context) -> err: error": |
      Завершить соединение/процесс.

"DocStore()":
  location: docstore.go
  annotations: |
    Кэш открытых документов uri → text + version.

    Requirements:
    - Put со stale version (меньше текущей) игнорируется
  methods:
    "Put(uri: string, text: string, version: int)": |
      Сохранить текст документа с версией.
    "Get(uri: string) -> text: string, version: int, found: bool": |
      Текст и версия документа; found=false если документ не открыт.
    "Remove(uri: string)": |
      Удалить документ из кэша.

"Serve(ctx: Context) -> err: error":
  location: serve.go
  annotations: |
    stdio-бутстрап по `lsp-protocol`.

    `ctx`: контекст процесса; `err`: причина завершения

    Algorithm:
    1. Собрать зависимости: NewStore, NewAnalyzer, Server, DocStore
    2. protocol.NewServer(ctx, srv, stream); ждать conn.Done()

---

Author: Goga
CreatedAt: 04/09/26
Description: |
  Корневая ячейка: LSP-сервер (stdio) над kb/rms/xs/analysis для редакторов.
```

#### `server/.usages/lifecycle.md`

```markdown
# Server Lifecycle — consuming the server cell

Domain: binary entrypoint wiring and editor connection. Target audience:
cmd/aoe2-lsp maintainers and editor-config authors.

## Entrypoint

```go
func main() {
    if err := server.Serve(context.Background()); err != nil {
        slog.Error("server exited", "err", err)
        os.Exit(1)
    }
}
```

## Editor configs (see task README)
- Neovim: lspconfig, cmd = aoe2-lsp binary, filetypes = { "aoe2rms", "aoe2xs" }
- VS Code: generic LSP extension launching the binary over stdio

Preconditions:
- Single binary, no flags required for MVP; positionEncoding negotiated
  in Initialize (prefer utf-8 when offered, per `lsp-protocol`).
```

---

### 7. Project-level usages (create)

#### `.goga/usages/rms-grammar.md`

```markdown
# RMS Grammar — AoE2 Definitive Edition

Domain: the Random Map Script language structure. Target audience: implementers
of the rms, analysis and kb cells. Sources: docs/ref/zetnus-rms-guide.txt
(canonical), docs/ref/aoe2de-xs-rms-changelog.md (versioning).

## File structure

A .rms file is a sequence of sections and top-level statements:

    <player_setup> ... </player_setup>
    <land_generation> ... </land_generation>
    <elevation_generation> <cliff_generation> <terrain_generation>
    <connection_generation> <objects_generation>

Statements outside sections are global. Section names live in angle brackets.

## Statements

    command arg1 arg2 ...           — e.g. create_elevator 7
    attribute_name value            — belongs to the PRECEDING command (positional semantics)
    start_random ... end_random     — random block:
        percent_chance 25 <statements>
    if <expr> ... [elseif ...] [else ...] endif   — conditionals

Attributes may interleave with a command's arguments only before the next
command starts. Comments: `/* ... */` (block), `//` and `#` line comments.

## Expressions (DE 141935/153015+)

- integers `7`, floats `3.5` (141935+), percents `50%`
- constants/identifiers: `TERRAIN_GRASS`, number_of_objects style names
- binary operators `+ - * /` (153015+): Expr tree, precedence arithmetic
- map-size scaling: `rand_float(a, b)` style helpers are ordinary commands

## Directives

    #include <file.rms>      — text include (rms.File.Includes)
    #includeXS <file.xs>     — inline XS block begins (rms.XsBlock)
    #includeXS               — bare directive switches the rest of file to XS
    #const NAME value        — script constant definition

## DE-era additions (must parse; see changelog for versions)

water_definition, create_object_group, create_connect_land_zones,
land_conformity, generate_mode, spacing_to_specific_terrain,
set_circular_base, require_path, override_map_size, cliff_type,
generate_for_first_land_only, set_facet. `effect_percent` is deprecated
in favor of operators — analysis flags it (warning).

## Testing fixtures

docs/ref corpus: snippets.aoe2map.net (actor areas, error handling),
aoe2map.net community maps. A parser must accept all of them without
false positives.
```

#### `.goga/usages/xs-grammar.md`

```markdown
# XS Grammar — AoE2 Definitive Edition

Domain: the XS scripting language structure. Target audience: implementers
of the xs, analysis and server cells. Sources: docs/ref/ugc-guide/xs/
(programmer.md, functions.json), docs/ref/ugc-guide/xs/prelude.xs (externs).

## Program

C-like. Top level declarations:

    void main() { ... }                       — entry point
    int/float/bool/string/vector <name>;      — variables
    <type> <name>(<params>) { ... }           — functions
    rule <name> [inactive] [min-interval X] { condition ... action ... }
    event(...)
    extern <decl>;                            — extern declaration (prelude.xs)
    include "file.xs" / includeDuno "..."     — includes

## Types

int, float, bool, string, void, vector. Vector literals: `(1.0, 2.0, 3.0)`.
Numbers: decimal int, float, hex `0x1F`. Strings: double-quoted, escapes.

## Statements

`{ }` blocks; `if/else`, `while`, `do/while`, `for(init; cond; step)`,
`switch/case/default/break`, `return [expr]`, `break`, `continue`,
expression statements. Semicolons terminate simple statements.

## Expressions

calls (`callee(args)` — Expr.Kind=call), binary ops `+ - * / % == != < <= > >=
&& || & | ^ << >>` and assignment forms, unary `! - ~ ++ --`, literals,
identifiers, vector member access `v.x|v.y|v.z`, vector constructors.

## Rules and events

`rule` bodies use special statement forms (`condition`, `action`,
`xsSetRule...` runtime calls). Parse permissively: unknown statements inside
rules must not break recovery. Events use `event(name, handler)` form.

## Externs (prelude.xs)

`extern` declarations carry doc comments — parse signature only, no body.

## Recovery

On error: emit Diagnostic, resynchronize to next `;` or `}` (statement level)
or next top-level keyword (declaration level). Never panic, never fail hard.

## Testing fixtures

docs/ref/ugc-guide/xs/prelude.xs (5k lines, 882 externs) must parse with zero
false positives; typical inline XS from RMS maps likewise.
```

---

## Dependency Map

```
                    ┌──────────────────────────────────────────────────┐
                    │                    server (корень)               │
                    │  Serve · Server · DocStore      [lsp-protocol]   │
                    └──────┬────────┬────────┬──────────┬──────────────┘
                           │        │        │          │
                    Store,lookups  RmsFile,Parse,XsBlock  XsFile,XsParse  Analyzer,checks
                           │        │        │          │
                    ┌──────▼──┐ ┌───▼───┐ ┌──▼───┐ ┌───▼─────┐
                    │   kb    │ │  rms  │ │  xs  │ │ analysis│
                    └─────────┘ └───┬───┘ └──┬───┘ └───┬─────┘
                     rms, xs: Pos/Range/Diagnostic + positions-and-diagnostics
                     analysis: Diagnostic
                                    └────┬──────┬──────┘
                                    ┌────▼──────▼────┐
                                    │    common      │
                                    └────────────────┘
Порядок: common → kb ∥ rms ∥ xs → analysis → server (ациклично;
server из common не импортирует — протокольные типы из go.lsp.dev)
```

## Verification Checklist

| После артефакта | Проверка |
|---|---|
| `common/CODEMANIFEST` + usages | `goga lint` чисто; экспортируемые имена = контракту; `go test ./common/...` зелёный |
| `.goga/usages/rms-grammar.md`, `xs-grammar.md` | покрывают все Kind'ы AST из contracts rms/xs; сверены с docs/ref |
| `kb/CODEMANIFEST` + usages + data JSON | `goga lint`; NewStore валидирует (дубликаты/пустые имена); 204 функции в xs-functions.json; rms-commands.json покрывает Syntax Skeleton |
| `rms/CODEMANIFEST` + usages | фикстуры aoe2map/snippets — 0 false positives; неизвестная команда → Diagnostic с корректным range |
| `xs/CODEMANIFEST` + usages | prelude.xs — 0 false positives; recovery: обрезанный вход не паникует |
| `analysis/CODEMANIFEST` + usages | unknown-command/attribute/undefined-symbol/bad-arity коды в тестах; нет дублей синтаксических diags |
| `server/CODEMANIFEST` + usages | `goga lint`; e2e: Neovim открывает .rms/.xs, hover/completion/diagnostics работают |
| Весь проект | `go test ./...`, `golangci-lint run`, `goga contract` по всем ячейкам |
