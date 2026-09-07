# Architecture Plan: cross-file-navigation

Source task: `docs/tasks/cross-file-navigation.md`. Контракты утверждены
2026-09-07 через brainstorm-пайплайн. Одна ячейка создаётся (`include`),
четыре модифицируются (`rms`, `xs`, `analysis`, `server`).

## Topic

`cross-file-navigation` — кросс-файловая навигация и резолв include-ов в
aoe2-lsp. План: `docs/arch/cross-file-navigation.md`.

## Implementation Order

1. **`rms`** (параллельно с 2) — лист (Imports: только common); тип
   `Include` и поля `Includes`/`XsIncludes` нужны ячейке `include`.
2. **`xs`** (параллельно с 1) — лист (Imports: только common);
   `References(name)` нужен ячейке `include`.
3. **`include`** — CREATE; зависит от common + rms + xs (после 1–2).
4. **`analysis`** (параллельно с 3) — зависит только от xs (`Decl` уже
   импортирован); `AnalyzeXs(externals)` не зависит от `include`.
5. **`server`** — корень; потребляет `include` + обновлённые
   `analysis`/`rms`/`xs` (после всех).

## Artifacts

### Cell `rms` — MODIFY

#### CODEMANIFEST (`rms/CODEMANIFEST`)

Header — Imports/Usages без изменений; в `Annotations` добавить абзац
в конец:

```yaml
  Директивы подключений по `rms_grammar` (Directives): #include → Includes;
  #includeXS с аргументом → XsIncludes + переключение в inline-XS режим
  (блок после директивы — в XsBlocks); bare #includeXS → только XsBlock.
  Путь без аргумента — синтаксическая Diagnostic, Include не создаётся.
```

Body:

1. `Parse` — заменить шаг 5 Algorithm на:

```yaml
    5. Директивы: #include с аргументом-путём → Include в Includes
       (Range = аргумент-путь); #includeXS с аргументом → Include в XsIncludes
       и начало XsBlock; bare #includeXS → только XsBlock; собирать строки
       блока до следующей директивы/секции
```

2. `RmsFile` properties — заменить `"Includes -> []string"` и добавить
   `XsIncludes`:

```yaml
    "Includes -> []Include": |
      Директивы #include (путь + диапазон аргумента).
    "XsIncludes -> []Include": |
      Внешние XS-скрипты (#includeXS с аргументом); inline-код — в XsBlocks.
```

3. `RmsFile` methods — добавить после `ReferencesAt`:

```yaml
    "References(name: string) -> ranges: []Range": |
      Вхождения имени без позиции (кросс-файловые references: у вызывающей
      стороны нет позиции в чужом файле).

      `name`: имя (слово-токен)
      `ranges`: все вхождения имени, по позиции

      Algorithm:
      1. Собрать все слова-токены, равные `name` (имена секций, команд,
         атрибутов и значения ident/const)
      2. Отсортировать по позиции

      Requirements:
      - ReferencesAt(pos) эквивалентен References(слово под pos)
      - отсутствие имени — пустой список
```

4. Добавить тип после `XsBlock`:

```yaml
"Include(path: string, r: Range)":
  location: ast.go
  annotations: |
    Директива-подключение: путь и диапазон аргумента-пути.

    `path`: путь как записан (относительный)
    `r`: диапазон аргумента-пути (не всей директивы — hit-test курсора
    и диагностика точнее)

    Constraints:
    - чистый тип данных: construct-and-use, без методов
  properties:
    "Path -> string": |
      Путь подключения (как записан в исходнике).
    "Range -> Range": |
      Диапазон аргумента-пути.
```

Footer — без изменений.

#### `.usages` — CREATE `rms/.usages/includes.md`

```markdown
# RMS Includes — consuming directive data

Domain: include directives recorded by the rms parser (#include /
#includeXS with a file argument). Target audience: implementers of the
include and server cells.

## Directive records

Parse records every connection directive with the path argument's range
(not the whole directive line):

```go
file, diags := rms.Parse(text, uri)

for _, inc := range file.Includes {     // #include "parts/econ.rms"
    // inc.Path == "parts/econ.rms"; inc.Range covers the path argument
}
for _, inc := range file.XsIncludes {   // #includeXS parts/lib.xs
    // external .xs script; the inline region AFTER the directive is a
    // separate XsBlock (bare #includeXS produces only an XsBlock)
}
```

Preconditions:
- A directive without a path argument yields a syntax Diagnostic and no
  Include record.
- Paths are stored verbatim (relative); resolution against the including
  file's directory is the consumer's job.

## Position hit-testing

inc.Range.Contains(pos) reports whether a cursor sits on the include
path — the trigger for go-to-definition into the target file.

## By-name references

```go
for _, r := range file.References(name) { ... }
// every word-token equal to name (section/command/attribute/ident words),
// sorted by position; ReferencesAt(pos) ≡ References(word under pos)
```
```

#### `.usages` — EXTEND `rms/.usages/rms-parsing.md`

В секцию Navigation добавить после примера `ReferencesAt`:

```go
// by-name form for cross-file searches: same occurrences without a
// position in this file.
for _, r := range file.References(name) { ... }
```

### Cell `xs` — MODIFY

#### CODEMANIFEST (`xs/CODEMANIFEST`)

Header/Footer — без изменений. Body — `XsFile` methods, добавить после
`ReferencesAt`:

```yaml
    "References(name: string) -> ranges: []Range": |
      Вхождения имени без позиции (кросс-файловые references: у вызывающей
      стороны нет позиции в чужом файле).

      `name`: имя символа
      `ranges`: все вхождения имени, включая декларацию, по позиции

      Algorithm:
      1. Собрать диапазоны вхождений `name` из индекса вхождений
      2. Отсортировать по позиции

      Requirements:
      - ReferencesAt(pos) эквивалентен References(SymbolAt(pos))
      - отсутствие имени — пустой список
```

#### `.usages` — EXTEND `xs/.usages/xs-parsing.md`

В секцию Navigation добавить:

````markdown
```go
// by-name references: for files where no position is known (cross-file
// searches over an include closure). Includes the declaration occurrence;
// ReferencesAt(pos) ≡ References(name under pos).
for _, r := range xsFile.References(name) { ... }

// cross-file definition lookup: Symbols() carries each top-level decl's
// name range in Selection — match by Name to find the jump target when
// the declaring file is not the queried one.
for _, sym := range xsFile.Symbols() {
    if sym.Name == name { /* target: sym.Selection */ }
}
```
````

### Cell `include` — CREATE

#### CODEMANIFEST (`include/CODEMANIFEST`)

```yaml
Imports:
  - Types:
      - Pos
      - Range
    Usages:
      - positions-and-diagnostics
    From: common
  - Types:
      - Parse
      - RmsFile
      - Include
    Usages:
      - rms-parsing
      - includes
    From: rms
  - Types:
      - XsParse
      - XsFile
      - Decl
    Usages:
      - xs-parsing
    From: xs

Usages:
  conventions: .goga/usages/conventions.md
  lsp-protocol: .goga/usages/cooks/lsp-protocol.md

Annotations: |
  Use `conventions` for code writing rules and testing (DI-конструктор,
  ctx-first, %w).
  Use `lsp-protocol` (Disk-backed Documents) для дисковых документов,
  editor-state приоритета и uri.File/Filename конверсий.
  Use `positions-and-diagnostics` from Imports для Pos/Range.
  Use `rms-parsing`, `includes`, `xs-parsing` from Imports для парсов и
  навигационных API провайдеров.

  Ячейка потокобезопасна (мьютекс кэша; запросы interleaved). Зависимость
  от editor-state инвертирована через Source; про server/protocol ячейка
  не знает. IO — stdlib + go.lsp.dev/uri.

---

"Source()":
  location: source.go
  annotations: |
    Поставщик editor-state текста документов; инверсия зависимости —
    ячейка не знает про DocStore сервера.
  methods:
    "Text(uri: string) -> text: string, found: bool": |
      Текст открытого в редакторе документа.

      `uri`: идентификатор документа
      `text`: текст editor-state
      `found`: документ открыт (false — сигнал «смотреть на диск»)

"Resolver(source: Source)":
  location: resolver.go
  annotations: |
    Include-замыкание документа и кросс-файловая навигация над ним.

    `source`: поставщик editor-state текста (DI)

    Requirements:
    - потокобезопасность: внутренняя синхронизация кэша
    - ctx уважает отмену
  methods:
    "Closure(ctx: Context, uri: string) -> closure: Closure": |
      Замыкание #include/#includeXS от корня.

      `ctx`: контекст отмены; `uri`: корневой документ

      Algorithm:
      1. Текст корня: Text из `Source`; found=false → диск по `lsp-protocol`
         (Filename + чтение); недоступен → Closure с Root=uri и пустыми
         списками
      2. Расширение → парс: .rms → Parse по `rms-parsing`, .xs → XsParse
         по `xs-parsing`; entry в visit-set по каноническому пути
      3. Для каждого RmsEntry: Includes и XsIncludes по `includes` — резолв
         относительно директории owner-файла по `lsp-protocol`; файл
         существует → ResolvedInclude и target в очередь; нет →
         MissingInclude
      4. Повторный канонический путь не разворачивается (циклы A→B→A
         безопасны); лимит глубины 64 и лимит файлов 1024: при превышении
         разворот останавливается, ошибкой не считать
      5. Вернуть Closure

      Requirements:
      - DFS-порядок файлов детерминирован (порядок директив)
      - каждая ненайденная директива — своя MissingInclude (повторы путей
        не дедуплицировать)

      Constraints:
      - inline XsBlocks в замыкание не входят (часть owner-парза)
      - XS include-декларации не разворачиваются
    "Definition(ctx: Context, uri: string, pos: Pos) -> target: Target, found: bool": |
      Кросс-файловое определение символа/директивы под позицией.

      `uri`: документ; `pos`: позиция; `target`: точка перехода;
      `found`: цель найдена

      Algorithm:
      1. Include.Range.Contains(pos) в Includes/XsIncludes запрошенного
         файла → резолв → Target{URI цели, Range 0:0 нулевой длины};
         не резолвится — found=false
      2. Позиция в inline XsBlock — транслировать в координаты блока,
         локальная Definition по `xs-parsing`, результат транслировать
         обратно
      3. .xs: локальная Definition(pos)
      4. Имя под позицией → поиск Symbols() XS-файлов замыкания
         (Selection = name-range), первый в DFS-порядке → Target
      5. Не найдено (builtin/неизвестное) — found=false

      Requirements:
      - детерминированность: одинаковый вход → одинаковый результат
    "References(ctx: Context, uri: string, pos: Pos) -> refs: []Target": |
      Вхождения имени под позицией по всему замыканию.

      `uri`: документ; `pos`: позиция; `refs`: вхождения (декларация
      включается; фильтрация — вызывающая сторона)

      Algorithm:
      1. Closure(uri)
      2. Имя под pos: .xs — SymbolAt по `xs-parsing`; .rms —
         ReferencesAt(pos) запрошенного файла, имя = текст первого range;
         пусто — пустой результат
      3. По всем файлам замыкания References(name) (.rms и .xs) →
         Target{URI, Range}
      4. Inline-блоки корня: XsParse(block.Code) + вхождения с
         трансляцией координат блока
      5. Сортировка: URI, затем позиция

      Requirements:
      - запрошенный файл входит в результат

"Closure(root: string)":
  location: closure.go
  annotations: |
    Результат замыкания: файлы с AST, резолвы, пропуски.
  properties:
    "Root -> string": |
      URI корневого документа запроса.
    "Rms -> []RmsEntry": |
      RMS-файлы замыкания (DFS-порядок).
    "Xs -> []XsEntry": |
      XS-файлы замыкания (DFS-порядок).
    "Resolved -> []ResolvedInclude": |
      Успешно резолвленные директивы.
    "Missing -> []MissingInclude": |
      Ненайденные подключения.
  methods:
    "ExternalDecls(exclude: string) -> decls: []Decl": |
      Декларации XS-файлов замыкания для анализа.

      `exclude`: URI исключаемого файла ("" — не исключать)
      `decls`: топ-декларации, DFS-порядок

"RmsEntry(uri: string, file: RmsFile)":
  location: closure.go
  annotations: |
    RMS-файл замыкания.
  properties:
    "URI -> string": |
      URI файла.
    "File -> RmsFile": |
      AST файла.

"XsEntry(uri: string, file: XsFile)":
  location: closure.go
  annotations: |
    XS-файл замыкания.
  properties:
    "URI -> string": |
      URI файла.
    "File -> XsFile": |
      AST файла.

"ResolvedInclude(owner: string, inc: Include, target: string)":
  location: closure.go
  annotations: |
    Успешно резолвленная директива-подключение.
  properties:
    "Owner -> string": |
      URI файла, содержащего директиву.
    "Inc -> Include": |
      Директива.
    "Target -> string": |
      URI цели.

"MissingInclude(owner: string, path: string, r: Range)":
  location: closure.go
  annotations: |
    Ненайденное подключение.

    Requirements:
    - Range — координаты owner-файла
  properties:
    "Owner -> string": |
      URI файла, содержащего директиву.
    "Path -> string": |
      Путь как записан.
    "Range -> Range": |
      Диапазон аргумента-пути.

"Target(uri: string, r: Range)":
  location: closure.go
  annotations: |
    Точка кросс-файловой навигации.
  properties:
    "URI -> string": |
      URI файла-цели.
    "Range -> Range": |
      Диапазон в целевом файле.

---

Author: Goga
CreatedAt: 07/09/26
Description: |
  Include-замыкание документов: резолв #include/#includeXS, дисковая загрузка,
  кросс-файловая навигация.
```

#### `.usages` — CREATE `include/.usages/closure.md`

```markdown
# Include Closure — consuming the include cell

Domain: resolving the #include/#includeXS closure of a document and
answering cross-file navigation over it. Target audience: implementers of
the server cell.

## Wiring

Provide editor-state text via Source; DocStore satisfies it structurally
through its Text method:

```go
docs := server.NewStore()               // has Text(uri) (string, bool)
resolver := include.NewResolver(docs)   // include.Source is satisfied
```

Preconditions:
- Source must reflect the CURRENT editor state — the resolver never
  overrides an open document with its disk copy (editor state wins).
- Cache invalidation: disk files are reloaded when size/mtime change;
  files changed outside the editor without a stat change are not tracked.

## Closure and missing includes

```go
closure := resolver.Closure(ctx, uri)
// closure.Rms / closure.Xs — parsed files in DFS directive order
// closure.Resolved — every resolved directive (owner, Include, target URI)
for _, m := range closure.Missing {
    // Diagnostic{Range: m.Range, Severity: error, Code: "missing-include",
    //            Message: "include not found: " + m.Path} — publish for m.Owner
}
```

## Navigation (definition / references)

```go
if t, ok := resolver.Definition(ctx, uri, pos); ok {
    // protocol.Location{URI: t.URI, Range: toProtocol(t.Range)}
}
for _, t := range resolver.References(ctx, uri, pos) { /* []Location */ }
```

Preconditions:
- References include the declaration occurrence — filter the local
  declaration range yourself when the client sends
  includeDeclaration=false.
- Definition returns found=false for builtins and unknown names — an
  empty LSP result, not an error.

## External declarations for analysis

```go
externals := closure.ExternalDecls("") // all closure XS declarations
diags := analyzer.AnalyzeXs(xsFile, externals)
// undefined-symbol no longer fires for names declared in included .xs files
```
```

### Cell `analysis` — MODIFY

#### CODEMANIFEST (`analysis/CODEMANIFEST`)

Header/Footer и остальные типы — без изменений. Body — метод
`Analyzer.AnalyzeXs` заменить целиком на:

```yaml
    "AnalyzeXs(file: XsFile, externals: []Decl) -> diags: []Diagnostic": |
      Проверки XS-файла с учётом внешних деклараций замыкания.

      `file`: анализируемый AST
      `externals`: декларации включённых .xs-источников (например,
      Closure.ExternalDecls); nil/пусто — поведение прежнее

      Algorithm:
      1. Построить `TypeEnv` из Decls файла; затем каждую external-декларацию
         объявить в топ-левел области (Declare), только если имя ещё не
         объявлено файлом
      2. Собрать объявленные имена (functions, variables, params, rules)
      3. Обойти объявления: при входе в тело функции — Push, объявить
         параметры (`XsParam`) и локальные переменные через Declare; при
         выходе — Pop. ident вне объявленных и вне kb (Store.Function,
         Store.Constant) — code="undefined-symbol", severity=error
      4. Для вызовов известных функций: число аргументов vs Params
         (Required=false — можно опускать) — code="bad-arity"
      5. Для каждого аргумента вызова известной функции: тип через
         `InferType`; несовместимость с типом параметра (`Coerce` по
         `xs_coercion`) — code="bad-type", severity=error
      6. Присваивание и return: тип выражения vs объявленный тип (`InferType`
         + `Coerce`) — code="bad-type", severity=error
      7. Отсортировать diags

      Requirements:
      - externals подавляют undefined-symbol и предоставляют типы для
        InferType; локальные декларации file всегда приоритетнее внешних
      - не дублировать синтаксические diags парсера

      Constraints:
      - не изменять входной AST и externals
```

#### `.usages` — EXTEND `analysis/.usages/checks.md`

Добавить в домен XS-проверок:

```go
// XS with an include closure behind it: pass the closure's declarations —
// names declared in included .xs files no longer fire undefined-symbol.
externals := closure.ExternalDecls(uri) // exclude the analyzed file itself
diags := analyzer.AnalyzeXs(xsFile, externals)
// nil externals — same behavior as before the parameter existed
```

### Cell `server` — MODIFY

#### CODEMANIFEST (`server/CODEMANIFEST`)

Header:

1. Imports — в блок `From: xs` добавить тип `Decl`; добавить новый блок:

```yaml
  - Types:
      - Source
      - Resolver
      - Closure
      - Target
      - MissingInclude
    Usages:
      - closure
    From: include
```

2. Annotations — добавить в конец:

```yaml
  Definition/References резолвятся через `Resolver` по include-замыканию
  (Cross-file Navigation Results): цели могут жить в неоткрытых файлах.

  `Resolver` создаётся в конструкторе `Server` над собственным `DocStore`
  (`NewResolver(docs)`: DocStore структурно удовлетворяет `Source`).
  Внешние декларации (`Decl` из ExternalDecls по `closure`) передаются в
  AnalyzeXs — тип `Decl` импортирован из xs.
```

Body:

1. `Server` annotations Requirements — добавить пункт:
   `- Resolver над собственным DocStore (по closure)`
2. `DidOpen` — Algorithm заменить на:

```yaml
      Algorithm:
      1. DocStore.Put(uri, text, version)
      2. Пересчитать диагностики (общая функция с DidChange):
         Closure(uri) → Missing с Owner==uri → Diagnostic
         (code="missing-include", range директивы); RMS: синтаксис +
         AnalyzeRms; inline XsBlock: XsParse + AnalyzeXs(xsFile,
         ExternalDecls("")); .xs: AnalyzeXs(file, ExternalDecls(uri))
      3. PublishDiagnostics по `lsp-protocol` (одна пачка)
```

3. `DidChange` — Algorithm заменить на:

```yaml
      Algorithm:
      1. Взять весь текст из ContentChanges (WholeDocument)
      2. DocStore.Put; пересчитать (как в DidOpen); PublishDiagnostics
```

4. `Definition` — заменить целиком:

```yaml
    "Definition(ctx: Context, params: DefinitionParams) -> result: DefinitionResult, err: error": |
      Кросс-файловый переход к определению.

      Algorithm:
      1. Resolver.Definition(ctx, uri, pos) по `closure`
      2. Target → protocol.Location (URI может отличаться от запрошенного —
         Cross-file Navigation Results в `lsp-protocol`)
      3. Промах → пустая LocationSlice (не nil); ошибкой не считать

      Requirements:
      - конвертация позиций — с учётом согласованного positionEncoding
```

5. `References` — заменить целиком:

```yaml
    "References(ctx: Context, params: ReferenceParams) -> locations: []Location, err: error": |
      Ссылки на символ по include-замыканию.

      Algorithm:
      1. Resolver.References(ctx, uri, pos) по `closure` → []Target →
         Locations
      2. IncludeDeclaration=false → исключить range локальной декларации
         (range Definition в запрошенном файле)
      3. Пустой результат — пустой список, не nil
```

6. `DocStore` — в Requirements добавить:
   `- структурно удовлетворяет Source из Imports (метод Text)`;
   в methods добавить после `Get`:

```yaml
    "Text(uri: string) -> text: string, found: bool": |
      Editor-state текст без версии (проекция Get для `Source`).

      `uri`: идентификатор документа; `found`: документ открыт
```

Footer — Description заменить на:

```yaml
Description: |
  Корневая ячейка: LSP-сервер (stdio) над kb/rms/xs/analysis/include
  для редакторов.
```

#### `.usages` — EXTEND `server/.usages/lifecycle.md`

Добавить секцию после Advertised capabilities:

```markdown
## Cross-file navigation

Definition and references resolve across the document's include closure:
targets may live in files that are not open in the editor — the server
loads them from disk on demand (editor state always wins for open files).
Missing #include / #includeXS targets surface as "missing-include"
diagnostics on the directive's path range.

Preconditions:
- Include paths resolve relative to the including file's directory; the
  game's installation root is not searched.
- Files changed outside the editor without a size/mtime change are not
  reloaded (no file watcher).
```

## Dependency Map

```
common ──(Pos, Range)──────────────────────> include
rms ────(Parse, RmsFile, Include)──────────> include
xs ─────(XsParse, XsFile, Decl)────────────> include
include ─(Source, Resolver, Closure,
          Target, MissingInclude)──────────> server
common/kb/rms/xs/analysis ──(существующие)> server
```

Циклов нет. `DocStore` (server) реализует `Source` (include) структурно —
инверсия зависимости без обратного импорта.

## Verification Checklist

После материализации артефактов (до реализации):

- [ ] `goga lint` — 7 ячеек, 0 ошибок (новая `include` в схеме)
- [ ] `goga schema` — `include` видна; dependencies без циклов
- [ ] `goga contract <cell>` для каждой затронутой ячейки после её
      реализации (порядок Implementation Order)

После реализации каждой ячейки:

- [ ] `rms`: тесты Include/XsIncludes (Range = аргумент-пути),
      References(name) ≡ ReferencesAt(word), malformed-директива →
      Diagnostic без Include; `go test ./rms/...`
- [ ] `xs`: тесты References(name) (вкл. декларацию, пустой список);
      `go test ./xs/...`
- [ ] `include`: tempdir-фикстуры — замыкание DFS, missing, цикл A→B→A
      завершается, editor-state из Source побеждает диск, лимиты 64/1024,
      Definition (include-hit → 0:0; Symbols-fallback), References
      (by-name + inline-трансляция), ExternalDecls(exclude);
      `go test ./include/...`
- [ ] `analysis`: externals подавляют undefined-symbol, локальные
      приоритетнее, nil-externals = прежнее поведение; `go test ./analysis/...`
- [ ] `server`: интеграционные LSP-сценарии 1–5 из задачи
      (`docs/tasks/cross-file-navigation.md`, Notes) по stdio на
      многофайловой tempdir-фикстуре; `go test ./...` зелёный
- [ ] `goimports -w .`, `golangci-lint run`, `goga lint` — чисто
```
