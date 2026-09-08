# Architecture Plan: completion

Topic: **completion** — `textDocument/completion` для AoE2 RMS + XS
(извлечение из server + расширение).

Path: `.goga/history/2026/master/arch.md` (слот топика `master`;
предыдущий артефакт — signature help — сохранён в git-истории).

Источник: brainstorm-конвейер 2026-09-08 (intake → context → primary
analysis → type map → type detail → cell distribution → contracts →
assembly; каждый гейт утверждён пользователем). Вход — `task.md`
топика (сформулирован через `/goga-propose`).

Материализация (2026-09-08, `goga lint` 9/0): коллизии имён с hints
решены переименованиями — тип `Completer` (не `Computer`), файл
`completer.go`, usage `completing.md` (не `computing.md`); аннотации
дополнены бэктиками на импортированные типы (`ArgSite`, `Statement`,
`Attribute`, `Expr`, `Command`, `Symbol`). План ниже отражает
материализованное состояние.

## Implementation Order

1. **`xs`** *(modify)* — лист: импортирует только `common`; метод
   `VisibleAt` не зависит от новых артефактов.
2. **`complete`** *(create)* — зависит от `common`/`kb`/`rms`/`xs`
   (реади-онли импорты; требует `VisibleAt` из шага 1).
3. **`server`** *(modify)* — корень: единственный потребитель
   `complete`; требует ячейку целиком.

Ячейки `common`, `kb`, `rms`, `hints`, `analysis`, `include` — без
изменений (read-only источники / не затрагиваются).

## Artifacts

### Cell: `xs` — modify

**CODEMANIFEST `xs/CODEMANIFEST`** — дельта: в `XsFile.methods`,
после метода `CallAt`, добавить:

```yaml
    "VisibleAt(pos: Pos) -> visible: []Symbol, found: bool": |
      Символы, видимые в точке (completion): топ-левел декларации +
      параметры и локали объемлющей функции.

      `pos`: позиция в файле; `visible`: видимые именованные символы;
      `found`: позиция в коде (false — строка/комментарий)

      Algorithm:
      1. Позиция внутри строки или комментария → found=false
      2. Топ-левел именованные декларации (Kind=function/variable/rule/
         event/extern; include пропускается) → `Symbol` по `symbols`
      3. Внутренняя функция, чьё тело объемлет `pos`: её параметры →
         kind=param; локальные декларации блоков её тела, объемлющих
         `pos` (блоки { } по `xs_grammar`) → kind=local
      4. Порядок: топ-левел по объявлению, затем параметры по списку,
         затем локали по позиции декларации

      Requirements:
      - детерминированность: одинаковый вход → одинаковый результат
      - Selection ⊆ Range для каждого узла (инвариант `symbols`)

      Constraints:
      - параметры/локали функций, не объемлющих `pos`, не возвращаются
      - конфликты имён не резолвятся: одноимённые символы разных областей
        возвращаются оба; приоритизация — решение потребителя
```

Header/Footer xs — без изменений.

**`.usages` `xs/.usages/xs-parsing.md`** — update: новая секция после
«Call-site lookup (signature help)»:

````markdown
## Visible symbols (completion)

VisibleAt answers "which named symbols can be referenced at this
position" — for completion providers. It combines file-scope
declarations with the parameters and locals of the function enclosing
the cursor.

```go
if syms, ok := xsFile.VisibleAt(pos); ok {
    // ok=false → cursor inside a string or comment: render nothing.
    // sym.Kind — "function" | "variable" | "rule" | "event" | "extern"
    //             | "param" | "local"; sym.Name — the symbol name
    // pair with kb functions/constants for the full candidate set;
    // the consumer decides priority when one name appears twice
}
```

Preconditions:
- Parse the document first; visibility answers from the body index
  recorded at parse time.
- Shadowing is not resolved: an outer top-level `int x` and an inner
  `float x` both come back — deduplication policy belongs to the
  consumer.
- `include` declarations are skipped (not name-bearing for completion).
````

### Cell: `complete` — create

**CODEMANIFEST `complete/CODEMANIFEST`** — полный файл:

```yaml
Imports:
  - Types:
      - Pos
      - Symbol
    Usages:
      - positions-and-diagnostics
      - symbols
    From: common
  - Types:
      - Store
      - Function
      - Command
      - CommandArg
      - Constant
      - ValueRange
    Usages:
      - lookups
    From: kb
  - Types:
      - XsFile
      - Decl
    Usages:
      - xs-parsing
    From: xs
  - Types:
      - RmsFile
      - ArgSite
      - Statement
      - Attribute
      - Expr
    Usages:
      - rms-parsing
    From: rms

Usages:
  conventions: .goga/usages/conventions.md

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `lookups` from Imports для kb-lookup API (Store.Functions /
  Store.Constants / Store.Commands / Store.Command).
  Use `xs-parsing` from Imports для семантики VisibleAt.
  Use `rms-parsing` from Imports для семантики ArgAt/SectionAt.
  Use `positions-and-diagnostics` from Imports для Pos.
  Use `symbols` from Imports для `Symbol` (visible-набор).

  Ячейка вычисляет кандидатов completion над готовыми AST и kb:
  stateless — результат зависит только от аргументов метода;
  протоколо-независима. Пустой список — штатное молчание (не nil,
  не ошибка, без found-флага). Truth-модель XS паритетно hints:
  source-декларации побеждают kb; данные не выдумываются.
  Качество подсказок RMS ↔ XS паритетно.

---

"Completer(store: Store)":
  location: completer.go
  annotations: |
    Вычислитель кандидатов completion: контекстная матрица RMS +
    пул видимых символов XS + рендер.

    `store`: база знаний (DI; NewCompleter)

    Requirements:
    - stateless: без мутабельного состояния между вызовами
  methods:
    "RmsAt(file: RmsFile, pos: Pos) -> candidates: []Candidate": |
      Кандидаты для позиции в RMS-файле по контекстной матрице.

      `file`: разобранный RMS; `pos`: позиция в файле;
      `candidates`: кандидаты (пустой — штатное молчание)

      Algorithm:
      1. ArgAt по `rms-parsing` не нашёл владельца → SectionAt: секция
         есть → команды секции (Store.Commands; синтетическая "global" →
         Commands("") — все); нет → пусто
      2. `ArgSite`.Kind=none (имя команды/хвост/неоднозначно) → команды
         секции (как шаг 1) + атрибуты команды-владельца: `Command`
         lookup в Store по имени владельца (`Statement` из
         `ArgSite`.Stmt) → Attributes; нет в kb → без атрибутов
      3. `ArgSite`.Kind=arg → константы: Store.Constants("")
      4. `ArgSite`.Kind=attr → по диапазонам атрибута: `pos` в
         `Attribute`.Value (`Expr`).Range → константы; иначе (на
         имени) → атрибуты владельца (как шаг 2)
      5. Рендер: Kind = command/attribute/constant; Detail: команда →
         `Command`.Section; атрибут (`CommandArg`) → «Kind Min..Max» при
         непустом `ValueRange`, иначе «Kind», флаги (пустой Kind) →
         пусто; константа → Constant.Value; Sort = группа (атрибуты 0,
         команды 1, константы 2) + Label

      Requirements:
      - детерминированность: одинаковый вход → одинаковый результат
      - кандидаты не выдумываются: только имена из kb

    "XsAt(file: XsFile, pos: Pos, external: []Decl) -> candidates: []Candidate": |
      Кандидаты для позиции в XS (документ или inline-блок в
      block-local координатах — трансляция у вызывающей стороны).

      `file`: разобранный XS; `pos`: позиция в координатах файла;
      `external`: декларации include-замыкания;
      `candidates`: кандидаты (пустой — штатное молчание)

      Algorithm:
      1. `XsFile`.VisibleAt: строка/комментарий → пусто
      2. Пул source: visible-символы (function/variable/param/local;
         extern → function) + `external` (Kind=function/variable;
         include пропускается); имя из пула source вытесняет
         одноимённые kb-записи; одноимённые source разных видов — оба
      3. kb: `Function` + `Constant` (Store.Functions() +
         Store.Constants(""))
      4. Рендер: Kind = function/constant/variable/param/local;
         Detail: function → мини-сигнатура «<ret> <name>(<типы>)»
         (source: Decl.Type + Params; kb: ReturnType + Params;
         без типа — без «<ret> »); constant → Constant.Value;
         variable/param/local → пусто (тип в Symbol недоступен —
         не выдумывается); Sort = группа (source 0, kb-функции 1,
         kb-константы 2) + Label

      Requirements:
      - детерминированность; паритет качества с RmsAt

"Candidate(label: string, kind: string, detail: string, sort: string)":
  location: candidate.go
  annotations: |
    Протоколо-независимый кандидат completion (чистые данные, стиль Hint).

    `label`: текст кандидата; `kind`: вид из словаря command /
    attribute / constant / function / variable / param / local;
    `detail`: одна строка (секция / границы / мини-сигнатура / значение;
    пустая — когда данные недоступны); `sort`: основа SortText —
    детерминированный порядок групп контекста

    Requirements:
    - чистые данные: construct-and-use, без мутации
    - маппинг kind в CompletionItemKind — ответственность server
  properties:
    "Label -> string": |
      Текст кандидата.
    "Kind -> string": |
      Вид: command / attribute / constant / function / variable /
      param / local.
    "Detail -> string": |
      Одна строка описания; пустая — когда данные недоступны.
    "Sort -> string": |
      Основа SortText: группа контекста + Label.

---

Author: Goga
CreatedAt: 08/09/26
Description: |
  Вычисление кандидатов completion для RMS и XS: контекстная матрица,
  truth-модель (source > kb), рендер; протоколо-независимая ячейка
  над kb/xs/rms.
```

**`.usages` `complete/.usages/completing.md`** — create:

````markdown
# Complete Computing — consuming the complete cell

Domain: computing completion candidates for RMS and XS positions.
Target audience: implementers of the server cell (the LSP handler).

## Construct once, share everywhere

completer := complete.NewCompleter(store) // kb.Store via constructor DI

## RMS candidates at a position

// Context matrix: command-name/tail → section commands + owner
// attributes; attribute name → owner attributes; argument/attribute
// value → constants; no section → empty slice.
items := completer.RmsAt(rmsFile, pos)

## XS candidates at a position

// visible, found := xsFile.VisibleAt(pos); !found → inside string or
// comment, render nothing. external decls from Closure.ExternalDecls(uri);
// for inline RMS blocks translate the cursor into block-local coordinates
// first (XsBlock.Range offset) — same as signature help.
items := completer.XsAt(xsFile, pos, externalDecls)

Preconditions:
- Parse the document first; candidates are valid for that parse only.
- Empty slice is the designed silence — map it to an empty
  CompletionList, never to an error.
- Source declarations win over same-name kb entries (truth model);
  same-name source symbols of different kinds both come back.
- The cell never imports go.lsp.dev — mapping Candidate to
  protocol.CompletionItem (Label/Kind/Detail/SortText) belongs to the
  server cell.
````

### Cell: `server` — modify

**CODEMANIFEST `server/CODEMANIFEST`** — дельты:

1. Imports — добавить после hints-блока:

```yaml
  - Types:
      - Completer
      - Candidate
    Usages:
      - completing
    From: complete
```

2. Global Annotations — заменить последний абзац на:

```yaml
  Use `computing` from Imports для вычисления хинтов signature help:
  молчание found=false → nil, nil (конвенция Hover); триггеры «(» и «,»
  не пересекаются с completion'ыми « » и «<».

  Use `completing` from Imports (complete) для вычисления кандидатов
  completion: пустой `Candidate`-список → CompletionList с пустыми
  items, не nil и не ошибка. Kind-маппинг `Candidate`.Kind → CompletionItemKind
  (таблица паритетна DocumentSymbol): command→Function,
  attribute→Field, constant→Constant, function→Function,
  variable→Variable, param→Variable, local→Variable. Существующий
  MVP-алгоритм Completion (прямые Store-запросы в server) упраздняется —
  вычисление уходит в complete.
```

3. Body — сигнатура Server:

```yaml
"Server(store: Store, analyzer: Analyzer, computer: Computer, completer: Completer)":
```

   аннотация типа: добавить `completer`: вычислитель кандидатов
   completion (DI).

4. Метод `Completion` — заменить аннотацию на:

```yaml
      Кандидаты для позиции в документе (stateless, read-only).

      `params`: позиция и контекст запроса; `result`: список кандидатов
      (возможно пустой — не ошибка)

      Algorithm:
      1. Язык по расширению URI (общий шаблон Hover/SignatureHelp)
      2. .xs: XsParse(text) + Closure(uri).ExternalDecls(uri) →
         completer.XsAt(file, pos, decls)
      3. .rms: Parse(text); позиция внутри XsBlock (Range.Contains) →
         трансляция в block-local координаты + XsParse(block.Code) +
         ExternalDecls("") → XsAt; иначе completer.RmsAt(file, pos)
      4. `Candidate`-список → []CompletionItem по `lsp-protocol`
         (Completion Items): Label; Kind по таблице; Detail; SortText=Sort;
         InsertText опущен; InsertTextFormat plain text
      5. Пусто → CompletionList с пустыми items (не nil, не ошибка)

      Requirements:
      - stateless: TriggerKind/TriggerCharacter/IsRetrigger не влияют
      - read-only (R10); существующие хендлеры не меняются (SC8)
```

5. Метод `Serve` — Algorithm шаг 1 дополнить сборкой
   `NewCompleter(store)` из complete рядом с `NewComputer(store)`
   из hints.

`Initialize` — без изменений (capability `CompletionProvider`
с триггерами « » и «<» уже заявлена).

**`.usages`**: `server/.usages/lifecycle.md` — без изменений.

## Dependency Map

```
common ─┬─(Pos, Symbol, positions-and-diagnostics, symbols)──> complete
kb ─────┤ (Store, Function, Command, CommandArg, Constant,
        │  ValueRange, lookups)
rms ────┤ (RmsFile, ArgSite, Statement, Attribute, Expr, rms-parsing)
xs ─────┘ (XsFile, Decl, xs-parsing)          [xs += VisibleAt]

complete ──(Completer, Candidate, completing)──────────────> server
hints ────(Computer, Hint, computing)───────────────────────┘
```

Циклов нет (DSL-запрет кросс-импортов соблюдён: complete ↔ hints —
сиблинги без взаимных импортов).

## Verification Checklist

После материализации каждого артефакта:

- [ ] `goga lint` — 0 ошибок по всем ячейкам
- [ ] `goga contract xs` — exit 0 (VisibleAt сигнатура совпадает:
      `(pos: Pos) -> (visible: []Symbol, found: bool)`)
- [ ] `goga contract complete` — exit 0 (`NewCompleter(store)`,
      `RmsAt`/`XsAt`-ресиверы, `Candidate`-структ с 4 полями)
- [ ] `goga contract server` — exit 0 (DI-параметр completer,
      пере-аннотированный `Completion`)
- [ ] Существующие тесты не менялись (SC8); `go test ./...` (memory
      cap) зелёный после каждого шага реализации
- [ ] `goimports -w .`, `golangci-lint run` — чисто
- [ ] Acceptance-критерии `task.md` покрыты (см. проверку в сборке:
      все 9 пунктов — покрыто)
