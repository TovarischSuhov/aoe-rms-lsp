# Design Document: `master` — Completion (textDocument/completion)

<!-- Топик: `master` (`.goga/history/2026/master/`). Основа: материализованные
контракты `8c77336` (ячейка complete + xs VisibleAt + server), план
`arch.md`, задача `task.md`. Прецедент архитектуры — signature help
(design/plan 2026-09, PR #5). -->

## Contract Changes

### Changed CODEMANIFEST Files

- `complete/CODEMANIFEST` — **новая ячейка**: `Completer(store)` с
  `RmsAt`/`XsAt`, `Candidate(label, kind, detail, sort)`; usage
  `completing.md`.
- `xs/CODEMANIFEST` — `XsFile` += метод `VisibleAt(pos) -> (visible
  []Symbol, found bool)`; kind-словарь outline расширен `param`/`local`.
- `server/CODEMANIFEST` — импорт `complete` (`Completer`, `Candidate`,
  `completing`); `Server(store, analyzer, computer, completer)`;
  переписан `Completion`; `Serve` собирает `NewCompleter(store)`;
  глобальные аннотации += kind-таблица и правило пустого списка.

### New Entities

- `complete.Completer` — вычислитель кандидатов (completer.go)
- `complete.Candidate` — протоколо-независимый кандидат (candidate.go)

### Changed Entities

- `xs.XsFile` — +`VisibleAt` (навигационное семейство SymbolAt/Definition/CallAt)
- `server.Server` — DI `completer`; `Completion` делегирует в complete

### Deleted Entities

- `server.completionsRms`, `server.completionsXs` (реализация, не контракт) —
  MVP-вычислители упраздняются вместе с хелперами `wordPrefix`,
  `matchesPrefix`, `commandSignature`, `functionSignature` (используются
  только старым completion; hover рендерит Markdown отдельно).

### Usages and Annotations Changes

- `xs/.usages/xs-parsing.md` += секция «Visible symbols (completion)».
- `complete/.usages/completing.md` — создан.
- `server` глобальные аннотации: `completing` (complete), kind-таблица
  `Candidate.Kind → CompletionItemKind`, правило «пустой список — не nil».
- `.goga/usages/cooks/lsp-protocol.md` += «Completion Items» (сделано на
  propose-этапе, `e864192`).

## Applied Fixes

### Fixed CODEMANIFEST Defects

- Дефектов Phase 3 не обнаружено (`goga lint` 9/0; интерфейсные
  чекпойнты трассировки — все passed, см. Code Stack Trace).
- Материализационные переименования (по требованию линтера, внесены до
  design-фазы): `Computer`→`Completer`, `computing.md`→`completing.md`
  (коллизия с hints), бэктики на импортированные типы.

## Entity Interaction and Data Flow

### Interaction Diagram

```
                    ┌──────────────────────── server (LSP) ────────────────────────┐
 textDocument/      │  Completion(ctx, params)                                     │
 completion ───────►│    │ язык по расширению URI                                  │
                    │    ├─ .xs ──► XsParse(text) ──────────┐                       │
                    │    │        Closure.ExternalDecls(uri)│                      │
                    │    │                                  ▼                      │
                    │    │                        Completer.XsAt(file,pos,ext)     │
                    │    ├─ .rms ─► Parse(text) ─┐           ▲                      │
                    │    │   pos ∈ XsBlock ──────┼─ unshift ┘                       │
                    │    │        └──────────────► XsParse(block.Code)+Ext("")     │
                    │    │                  else ─► Completer.RmsAt(file,pos)       │
                    │    │                              ▼                          │
                    │    │                       []Candidate                       │
                    │    └──────────────► render → []protocol.CompletionItem ──────┼──► CompletionList
                    └──────────────────────────────────────────────────────────────┘
                                             internal complete:
  RmsAt ─► rms.RmsFile.ArgAt / SectionAt ─► kb.Store.Commands/Command/Constants
  XsAt  ─► xs.XsFile.VisibleAt ──────────► kb.Store.Functions/Constants
                                           + external []xs.Decl (передан server'ом)
```

### Data Flows

1. **RMS-команда**: editor → Server.Completion → rms.Parse → RmsAt:
   ArgAt(pos) → (ArgSite{Stmt,Kind}) → Store.Command(stmt.Name) /
   SectionAt → Store.Commands(sec) → []Candidate → CompletionItem[].
2. **RMS-значение**: ArgSite.Kind=arg|attr(+Value.Range) →
   Store.Constants("") → []Candidate(kind=constant, Detail=Value).
3. **XS**: VisibleAt(pos) → []common.Symbol (top+param+local) ∪
   external []Decl → вытеснение kb ∪ Store.Functions()+Constants("")
   → []Candidate.
4. **inline-XS**: pos∈XsBlock.Range → unshiftPos → XsParse(Code) →
   поток 3 с ExternalDecls("").

### Entity Dependencies

Инициализация (Serve): `kb.NewStore()` → `analysis.NewAnalyzer(store)` →
`hints.NewComputer(store)` → `complete.NewCompleter(store)` →
`NewServer(store, analyzer, computer, completer)` → `NewDocStore()`.
Все зависимости immutable после построения; порядок не влияет.

## Code Stack Trace

### Trace: `xs.XsFile.VisibleAt(pos)`

#### Chain

1. **Input**: `pos common.Pos` в координатах файла (или block-local —
   трансляция у вызывающей стороны).
2. Проверка `noncode` (строки/комментарии, индекс с парс-времени —
   тот же, что у `CallAt`) → contains → **return (nil, false)**.
   → checkpoint: типы ✓ (существующий `[]common.Range`-индекс).
3. Топ-левел: для каждого `Decls[i]` с `Kind != DeclInclude` →
   `common.Symbol{Kind: decl.Kind, Name: decl.Name, Range: decl.Range,
   Selection: declNameRange(decl)}` (существующий хелпер `Symbols()`
   использует тот же рендер). → checkpoint: Selection ⊆ Range ✓
   (declNameRange — первое вхождение имени внутри Range).
4. Параметры: для каждого function-Decl, чей `decl.Range` объемлет
   `pos` (внутренний при вложенности невозможна — топ-левел плоский),
   каждый `Params[j]` → `Symbol{Kind: "param", Name, Range: r,
   Selection: r}`, где `r = paramNameRange(decl, p.Name)` (существующий;
   skip-логика для имени функции = имени параметра уже учтена).
   → checkpoint: ✓ переиспользование существующих хелперов.
5. Локали: обход `collectVisibleLocals(decl.Body, pos, depth=1)` —
   вариант существующего `collectLocals` без фильтра по имени:
   `StmtDecl` → каждый item `declaredLocal(item, "")`-обобщённый →
   candidate{nameRange, scopeEnd=blockEnd, depth}; включаются только
   те, чей scope покрывает `pos` (`!pos.Before(nameRange.Start) &&
   pos.Before(scopeEnd)` — семантика `declCandidate.covers`).
   → `Symbol{Kind: "local", Name, Range: r, Selection: r}`.
   → checkpoint: ✓ модель scope идентична `bestDeclarer` (Definition) —
   никакой второй интерпретации видимости.
6. **Output**: `(symbols, true)`; порядок: топ-левел по объявлению →
   параметры по списку → локали по позиции декларации.

#### Checkpoint Summary

- noncode-индекс существует (`f.noncode`) — passed.
- Параметры/локали уже имеют machinery (`paramNameRange`,
  `collectLocals`, `declaredLocal`, `declCandidate.covers`) — passed:
  VisibleAt = инверсия bestDeclarer (перечислить всех candidates,
  покрывающих pos, вместо выбора лучшего для одного имени).
- Контракт «локали — блоки, объемлющие pos» уточнён: локаль видима,
  если её декларация **предшествует или совпадает** с pos в объемлющем
  блоке (семантика covers). Локаль, объявленная после курсора, не
  видима — согласовано с Definition (затенение). Дефектов нет.

### Trace: `complete.NewCompleter(store)` / `Completer`

1. **Input**: `store *kb.Store` (DI).
2. `Completer{store: store}` — единственное поле; без горутин, без
   кэшей (Store immutable). → checkpoint: ✓ conventions (constructor DI).

### Trace: `Completer.RmsAt(file, pos)`

#### Chain

1. **Input**: `file rms.RmsFile` (свежий Parse), `pos common.Pos`.
2. `file.ArgAt(pos)` → `(ArgSite, bool)`.
   - **found=false** → `file.SectionAt(pos)`:
     - found → `sectionCommands(sec.Name)` (гл. 4 ниже); "global" →
       `store.Commands("")` (все); → checkpoint: ✓ `SectionAt`
       возвращает синтетическую "global" только если материализована.
     - не found → **return nil**.
3. **ArgSite.Kind** (значения «arg»/«attr»/«none» — канонические
   константы rms.KindArg/KindAttr/KindNone):
   - `none` → `sectionCommands(...)` ∪ `ownerAttributes(site.Stmt)`.
   - `arg` → `store.Constants("")` → candidates kind=constant.
   - `attr` → поиск код-атрибута: среди `site.Stmt.Attributes` взять
     последний с `Name == site.Name` и `Range.Contains(pos)` →
     `attrCode`; тогда `attrCode.Value.Range.Contains(pos)` →
     **value-позиция** → константы; иначе (имя/без значения) →
     `ownerAttributes(site.Stmt)`.
     → checkpoint: ✓ Value — экспортированное поле `rms.Attribute`,
     `Expr.Range` экспортирован; атрибут без значения → zero-Range →
     Contains всегда false → корректный дефолт «на имени».
4. `sectionCommands(section)`: `store.Commands(section)` →
   `Candidate{Label: cmd.Name, Kind: "command", Detail: cmd.Section,
   Sort: "1"+cmd.Name}`.
5. `ownerAttributes(stmt)`: `store.Command(stmt.Name)`; found → для
   каждого `Command.Attributes[i]` → `Candidate{Label: a.Name, Kind:
   "attribute", Detail: attrDetail(a), Sort: "0"+a.Name}`, где
   `attrDetail`: Range непуста (Min≠"" && Max≠"") → «Kind Min..Max»;
   иначе Kind≠"" → «Kind»; иначе "" (флаг).
   → checkpoint: ✓ формат паритетен меткам hints (`lookups`:
   «Name: Kind Min..Max»; здесь без Name — Label уже рядом).
6. Константы: `Candidate{Label: c.Name, Kind: "constant", Detail:
   c.Value, Sort: "2"+c.Name}`.
7. **Output**: `[]Candidate` (порядок: атрибуты(Sort 0) → команды(1) →
   константы(2); внутри группы — Label, т.к. Sort = группа+Label).
   Дедупликация не требуется (источники не пересекаются по Label).

#### Checkpoint Summary

- Все kb-вызовы сигнатурно совпадают (`Commands(string) []Command`,
  `Command(string) (Command, bool)`, `Constants(string) []Constant`,
  `CommandArg.Kind/Range`) — passed.
- ArgAt/SectionAt — экспортированы, семантика в `rms-parsing` — passed.
- Дефектов нет.

### Trace: `Completer.XsAt(file, pos, external)`

#### Chain

1. **Input**: `file xs.XsFile`, `pos`, `external []xs.Decl`
   (include-замыкание; для inline-блоков — ExternalDecls("")).
2. `file.VisibleAt(pos)` → `(syms, ok)`; `!ok` → **return nil**.
   → checkpoint: ✓ строка/комментарий отсечены здесь (единственный
   in-string-гейт для XS-completion).
3. **Пул source** (Sort "0"): map `seen[name]struct{}`:
   - из `syms`: kind function/variable/param/local → как есть; kind
     extern → kind "function"; kind rule/event → как есть (словарь
     VisibleAt их возвращает; кандидатный kind — "function"? нет:
     rule/event не вызываются по имени — **исключаются**, фиксируется
     ниже); каждому — Sort "0"+Label.
   - из `external`: Kind=function → "function"; Kind=variable →
     "variable"; Kind=extern → "function"; rule/event/include — skip.
   - имя в пуле → вытесняет kb (запись в `seen`).
   - Detail: для function — мини-сигнатура (см. шаг 5); для прочих "".
     → checkpoint: !! контракт говорит «visible-символы
     (function/variable/param/local; extern → function)» — про
     rule/event из VisibleAt не сказано. Решение дизайн-уровня:
     **исключать** rule/event (не адресуемы по имени в выражениях).
     Согласовано с контрактом (перечислен исчерпывающий словарь).
4. **kb** (не в `seen`): `store.Functions()` → kind "function",
   Sort "1"+Name; `store.Constants("")` → kind "constant",
   Sort "2"+Name.
5. **Мини-сигнатура** `functionDetail`: source → `Decl.Type` +
   `Decl.Params` (типы через ", "); kb → `Function.ReturnType` +
   `Function.Params`; рендер `«[ret ]name(t1, t2)»`; пустой Type →
   без «ret ». → checkpoint: ✓ поля существуют (`Param.Type`,
   `ReturnType`); паритет рендера с hints (Label-часть).
6. **Output**: `[]Candidate`.

#### Checkpoint Summary

- VisibleAt ↔ XsAt: `[]common.Symbol` ↔ потребление Name+Kind — passed.
- external `[]Decl` ↔ `hints.XsAt`-паттерн — passed.
- Открытка: rule/event из VisibleAt исключаются (дизайн-решение в
  рамках контрактного словаря) — дефектом не является.

### Trace: `server.Server.Completion(ctx, params)`

#### Chain

1. **Input**: `params.TextDocument.URI`, `params.Position`;
   `params.Context` игнорируется (stateless).
2. `openDocument(uri)` → (text, name); не открыт → пустой
   `CompletionList{Items: []}` (не nil).
3. Роутинг (общий шаблон SignatureHelp):
   - `.xs`: `XsParse(text)` + `closure.ExternalDecls(uri)` →
     `completer.XsAt(file, pos, ext)`.
   - `.rms`: `Parse(text)`; найти `XsBlock` с `Range.Contains(pos)` →
     `unshiftPos(pos, block.Range.Start)` + `XsParse(block.Code)` +
     `closure.ExternalDecls("")` → `XsAt`; иначе `RmsAt(file, pos)`.
   → checkpoint: ✓ unshiftPos существует (server.go:746, паттерн
   SignatureHelp task 8); Closure-доступ — как в существующих хендлерах.
4. Рендер: для каждого `Candidate` → `protocol.CompletionItem{Label:
   c.Label, Kind: kindMap[c.Kind], Detail: protocol.NewOptional(c.Detail)
   при c.Detail != "", SortText: Optional(c.Sort)}`; `InsertText`
   опускается; `InsertTextFormat` — plain (zero value).
   kindMap: command→Function, attribute→Field, constant→Constant,
   function→Function, variable→Variable, param→Variable, local→Variable.
   → checkpoint: ✓ таблица = глобальная аннотация server; поведение
   отличается от MVP (command был Keyword, Documentation заполнялся,
   фильтрация была серверной) — задокументировано как замена MVP.
5. **Output**: `&protocol.CompletionList{IsIncomplete: false, Items:
   items}`; пустой items — `[]protocol.CompletionItem{}` (не nil).

#### Checkpoint Summary

- Открытый документ/парсинг — существующие хелперы — passed.
- Замена MVP: тесты `TestServe_CompletionAllXsFunctions`,
  `TestServe_CompletionRmsScopedToSection` переписываются под новый
  контракт (единственное изменение существующих тестов в задаче;
  остальные фичи — SC8, не трогаются). Capability-тест (строка 615)
  остаётся валидным.

### Trace: `server.Serve` (дельта)

1. `store := kb.NewStore()` (существ.)
2. `+ completer := complete.NewCompleter(store)`
3. `NewServer(store, analyzer, computer, completer)` — расширенный
   конструктор; struct Server += поле `completer`.
   → checkpoint: ✓ DI-конструктор по conventions; алиасинг пакетов
   не нужен (hints.Computer / complete.Completer — разные имена).

## Algorithm Design

### `xs.XsFile.VisibleAt`

**Responsibility**: видимый в точке набор именованных символов для
completion-провайдеров; scope-модель = `Definition` (covers-семантика).

**Algorithm:**
```
1. IF pos ∈ noncode → return (nil, false)
2. out := []
3. FOR decl IN Decls WHERE Kind != include:
     out += Symbol{Kind: decl.Kind, Name, Range: decl.Range,
                   Selection: declNameRange(decl)}
4. FOR decl IN Decls WHERE Kind == function AND decl.Range ∋ pos:
     FOR p IN decl.Params (по списку):
       r := paramNameRange(decl, p.Name); fallback r = decl.Range
       out += Symbol{Kind: "param", Name: p.Name, Range: r, Selection: r}
5. FOR decl IN Decls WHERE Kind == function AND decl.Range ∋ pos:
     walk(decl.Body, blockEnd = decl.Range.End, depth = 1):
       FOR stmt WHERE Kind == StmtDecl:
         FOR item IN stmt.Exprs:
           (name, r) := declaredLocalName(item)   // обобщение declaredLocal
           IF r.Start <= pos < blockEnd:           // covers
             out += Symbol{Kind: "local", Name: name, Range: r, Selection: r}
       RECURSE stmt.Body (blockEnd = stmt.Range.End, depth+1)
6. return (out, true)
```

**Errors**: не возвращаются (парсер не падает; навигация детерминирована).

**Edge Cases**:
- пустой файл → `(empty, true)` (found отличает «нет символов» от «не код»).
- локаль объявлена после pos → не видима (covers-семантика = Definition).
- параметр = имени функции → paramNameRange skip-логика возвращает токен
  параметра, не имени функции.
- одноимённые топ-левел и локаль → оба в выдаче (приоритизация — потребитель).

### `complete.Completer.RmsAt`

**Responsibility**: контекстная матрица RMS → кандидаты.

**Algorithm:**
```
1. site, ok := file.ArgAt(pos)
2. IF !ok:
     sec, ok2 := file.SectionAt(pos)
     IF !ok2 → return nil
     IF sec.Name == "global" → return sectionCommands("")
     return sectionCommands(sec.Name)
3. SWITCH site.Kind:
   case none:  return append(sectionCommands(sectionOf(pos)),
                             ownerAttributes(site.Stmt)...)
   case arg:   return constants()
   case attr:  attrCode := последний a IN site.Stmt.Attributes
                          WHERE a.Name == site.Name AND a.Range ∋ pos
               IF attrCode != nil AND attrCode.Value.Range ∋ pos:
                 return constants()
               return ownerAttributes(site.Stmt)
4. sectionCommands(sec): Store.Commands(sec) →
     Candidate{Label: c.Name, Kind: "command", Detail: c.Section,
               Sort: "1" + c.Name}
   ownerAttributes(stmt): (cmd, ok) := Store.Command(stmt.Name); !ok → nil
     FOR a IN cmd.Attributes:
       Candidate{Label: a.Name, Kind: "attribute", Detail: attrDetail(a),
                 Sort: "0" + a.Name}
   constants(): Store.Constants("") →
     Candidate{Label: c.Name, Kind: "constant", Detail: c.Value,
               Sort: "2" + c.Name}
   attrDetail(a): Min!=""&&Max!="" → "Kind Min..Max";
                   Kind!="" → "Kind"; else ""
```

**Errors**: нет; пустой список = молчание.

**Edge Cases**:
- владелец не в kb → только команды секции (без атрибутов).
- атрибут без значения (`set_scaling_by_map ␣`) → zero Value.Range →
  Contains=false → имена атрибутов (правильный дефолт).
- секция "global" → все команды (`Commands("")`).
- позиция на заголовке секции/директиве при ArgAt=false → SectionAt
  fallback даёт команды секции (шум отфильтрует клиент; молчание здесь
  хуже — пользователь начинает вводить команду).

### `complete.Completer.XsAt`

**Responsibility**: пул source (visible + external) + kb → кандидаты.

**Algorithm:**
```
1. syms, ok := file.VisibleAt(pos); !ok → return nil
2. seen := set(); out := []
3. FOR sym IN syms:                       // Sort "0"
     kind := sym.Kind
     SWITCH kind:
       function, variable, param, local → как есть
       extern   → "function"
       rule, event, section → SKIP        // не адресуемы в выражениях
     IF sym.Name ∉ seen: out += cand(sym.Name, kind, detailFor(kind), "0"+Name)
     seen += sym.Name
4. FOR d IN external:                     // Sort "0", тот же приоритет
     kind := d.Kind == extern → "function"; function → "function";
            variable → "variable"; else SKIP
     IF d.Name ∉ seen: out += cand(d.Name, kind, fnDetail(d), "0"+d.Name)
     seen += d.Name
5. FOR fn IN Store.Functions() WHERE fn.Name ∉ seen:   // Sort "1"
     out += Candidate{fn.Name, "function", fnDetail(fn), "1"+fn.Name}
6. FOR c IN Store.Constants("") WHERE c.Name ∉ seen:   // Sort "2"
     out += Candidate{c.Name, "constant", c.Value, "2"+c.Name}
```

`fnDetail`: source Decl → `[Type +] Name(T1, T2)`; kb Function →
`[ReturnType +] Name(P1.Type, P2.Type)`; пустой тип — без префикса.

**Errors**: нет.

**Edge Cases**:
- source-функция = имя kb-функции → только source (truth-модель).
- function foo + variable foo в source → оба (разные kinds).
- Detail у variable/param/local — "" (тип в Symbol недоступен; не
  выдумывать) — клиент покажет просто имя.

### `server.Server.Completion` (переписываемый)

**Responsibility**: роутинг + рендер; stateless.

**Algorithm:**
```
1. text, name, ok := openDocument(uri); !ok → пустой CompletionList
2. pos := fromProtocolPos(params.Position)
3. SWITCH расширение name:
   ".xs":  file := XsParse(text); ext := Closure.ExternalDecls(uri)
           cands := completer.XsAt(file, pos, ext)
   ".rms": file := Parse(text)
           IF ∃ block IN file.XsBlocks WHERE block.Range ∋ pos:
             bp := unshiftPos(pos, block.Range.Start)
             xsFile := XsParse(block.Code)
             cands := completer.XsAt(xsFile, bp, Closure.ExternalDecls(""))
           ELSE cands := completer.RmsAt(file, pos)
4. items := render(cands)      // kindMap; Detail Optional при != "";
                                // SortText=Sort; InsertText опущен
5. return &CompletionList{IsIncomplete: false, Items: items}
```

**Errors**: err всегда nil (молчание = пустой список; «не открыт» и
«нет кандидатов» — не ошибки протокола).

**Edge Cases**:
- неизвестное расширение/язык → пустой items (существующее поведение
  switch без ветки).
- Closure недоступен (C6) → ExternalDecls отдаёт что может; unknown
  имена молча падают до kb.

## Cross-cutting Concerns

- **Error handling**: полином nil-error — ни один новый путь не
  возвращает err (парсеры не падают; kb-lookup — `(T, bool)`; пустой
  список = штатное молчание). Согласовано с контрактами (без `err` в
  сигнатурах Completer/VisibleAt).
- **Logging**: отсутствует (чистые вычисления; паритет с hints —
  там тоже нет логов).
- **Validation**: детерминированность всех методов (одинаковый вход →
  одинаковый результат — Requirements контрактов); сортировка
  стабильна через Sort = группа+Label.
- **Caching**: нет новых кэшей. Parse выполняется на каждый запрос —
  паритет с SignatureHelp/Hover (per-request parse); DocStore кэширует
  только текст.
- **Concurrency**: все структуры immutable после построения; Completer
  разделяётся горутиной соединения безопасно (как Computer/Analyzer).

## Usages Analysis

### `conventions`
- **What**: обязательные правила Go-кода/тестов (DI, errors, table-driven).
- **Where**: все три ячейки (глобальные аннотации).
- **Why**: базовая практика проекта.
- **How**: NewCompleter-конструктор; методы без err; testify-таблицы.

### `lsp-protocol` (server)
- **What**: go.lsp.dev-паттерны: result shapes, Completion Items.
- **Where**: `Server.Completion` (рендер, empty-list), `Serve`.
- **Why**: единый источник протокольных конвенций.
- **How**: `*CompletionList` (не slice-арм), Optional-обёртки,
  stateless-правило, клиентская фильтрация.

### Imported Usages

- `lookups` из `kb` — list-lookups API (`Functions`/`Constants`/
  `Commands`/`Command`); путь `kb/.usages/lookups.md`. Использован
  Completer'ом; регистрозависимость имён учтена (сервер не фильтрует).
- `rms-parsing` из `rms` — `ArgAt`/`SectionAt`-семантика; путь
  `rms/.usages/rms-parsing.md`. Использован RmsAt.
- `xs-parsing` из `xs` — `VisibleAt` (+`CallAt`-семантика noncode);
  путь `xs/.usages/xs-parsing.md`. Использован XsAt.
- `positions-and-diagnostics`, `symbols` из `common` — Pos, Symbol
  (Selection ⊆ Range); пути `common/.usages/*.md`.
- `computing` из `hints` — unchanged (SignatureHelp).
- `completing` из `complete` — consumer-контракт нового Completer;
  путь `complete/.usages/completing.md`.

## `.usages/` Update

### Cell: `xs`
- **xs-parsing.md** → статус current (секция Visible symbols добавлена
  на материализации); дополнений не требуется.

### Cell: `complete`
- **completing.md** → создан на материализации; после реализации
  проверяется соответствие примеров фактическому API (NewCompleter).

### Cell: `server`
- **lifecycle.md** → not affected (Completion — не lifecycle);
  изменений нет.

## Test Stack Trace

### General Setup

- `kb`: настоящий `NewStore()` (embedded JSON; lookups.md) — без моков.
- `rms`/`xs`: Parse/XsParse реальных фикстур; позиции — байтовые offset
  из фикстуры (patтерн navigation_test.go).
- `complete`: table-driven на (фикстура, pos) → []Candidate; сравнение
  полных срезов (Label, Kind, Detail, Sort) через require.Equal.
- `server`: integration через dispatcher (паттерн serve_test.go:
  harness h.disp.Completion); язык определяется URI (.rms/.xs).

### Source File Registry

- `xs/ast.go` (+`visible.go` при необходимости) — VisibleAt + хелперы.
- `complete/completer.go`, `complete/candidate.go` (+ тесты).
- `server/server.go` — Completion, render, DI; `server/serve.go` — сборка.
- Тесты: `xs/navigation_test.go` (доп.), `complete/completer_test.go`,
  `complete/candidate_test.go`, `server/serve_test.go` (замена
  completion-тестов).

---

### Positive Tests

#### `TestVisibleAt_TopLevelDecls`

**Setup**: `XsParse("int g = 1;\nvoid f(float a) { a = 2; }\nextern int e();\ninclude \"x.xs\";", "t.xs")`

**Input**: pos на строке 2 (внутри `void f` заголовка), например offset
начала `float a`.

**Trace**:
```
VisibleAt(pos)
  → noncode: пусто → ok
  → топ-левел: g(variable), f(function), e(extern) → Symbol
  → f.Volume ∋ pos? Range f покрывает → params: a → kind=param
  → locals тела: нет до pos
  → (out, true)
```

**Assertions**: names/подмножество kinds: g=variable, f=function,
e=extern, a=param; `include` отсутствует; для каждого Selection ⊆ Range;
found=true.

**Sufficiency**: фиксирует словарь top-level + skip include — регрессия
на «completion предлагает пути include».

#### `TestVisibleAt_ParamAndLocalScopes`

**Setup**: `void f(int p) { int a1 = 1; { int b1 = 2; } pos_mark int a2; }`
(курсор — на `pos_mark`, перед декларацией a2)

**Trace**:
```
VisibleAt(pos)
  → top: f
  → params: p
  → locals: a1 (r.Start <= pos < blockEnd) ✓; b1 — inner block r.Start
    <= pos? b1 объявлен до pos, его блок объемлет pos? блок { int b1 }
    закрылся до pos → scopeEnd < pos → НЕ видима; a2 объявлена после
    pos → не видима
  → [f, p, a1]
```

**Assertions**: contains f(function), p(param), a1(local); NOT contains
b1, a2; порядок: f → p → a1.

**Sufficiency**: covers-семантика (Declaration-before-use) — ядро
scope-модели; синхронно с Definition.

#### `TestRmsAt_CommandNamePosition`

**Setup**: rms-фикстура `<land_generation>\ncreate_land\n</land_generation>`;
kb.Store; pos на токене `create_land` (ArgAt → kind=none).

**Trace**:
```
RmsAt(file, pos)
  → ArgAt → {Stmt: create_land, Kind: none}
  → sectionCommands("land_generation") → команды секции, Sort "1…"
  → ownerAttributes(create_land) → Store.Command("create_land").Attributes,
    Sort "0…"
```

**Assertions**: содержит create_land (kind=command, Detail="land_generation",
Sort="1create_land"); содержит атрибуты create_land с Sort="0"+имя;
все Kind ∈ {command, attribute}; констант нет.

**Sufficiency**: главная UX-ветка; атрибуты опережают команды в SortText.

#### `TestRmsAt_ArgValuePosition_Constants`

**Setup**: `set_up_lands 5`? — аргумент number: kb create_elevator;
фикстура `create_elevator 7`; pos на `7` (kind=arg).

**Assertions**: только константы (kind=constant, Detail=c.Value,
Sort="2"+Name); пусто от команд/атрибутов.

**Sufficiency**: value-контекст даёт константы — не команды.

#### `TestRmsAt_AttrNameVsValue`

**Setup**: `create_elevator 7 { set_scaling_by_map 50 }`; pos1 — на
`set_scaling_by_map` (имя), pos2 — на `50` (значение).

**Assertions**: pos1 → атрибуты владельца (kind=attribute; Detail по
Kind/Range флаг-атрибута set_scaling_by_map — без Kind → Detail="");
pos2 → константы.

**Sufficiency**: дискриминация имя-vs-значение через Value.Range.

#### `TestRmsAt_GlobalSection_AllCommands`

**Setup**: фиксtура с глобальным statement (вне секций), pos на хвосте.

**Assertions**: кандидаты = Commands("") (все секции), kind=command.

**Sufficiency**: синтетическая global → все команды.

#### `TestXsAt_SourceShadowsKb`

**Setup**: XsParse с `int xsVectorSet() { return 1; }`? — имя kb-функции;
проще: `void main() {}` + локальная `int trQuestVarGet = 1;`? — берём
функцию с именем из kb (`trQuestVarGet`), external=nil.

**Assertions**: ровно один `trQuestVarGet` (kind=function, Detail из
source-Decl.Type+Params, Sort="0…"); Detail НЕ из kb.

**Sufficiency**: truth-модель source > kb.

#### `TestXsAt_ExternalDeclsMerged`

**Setup**: XsParse(`void main() {}`), external = []Decl{{Kind: function,
Name: "helper"}, {Kind: include, Name: "z.xs"}}.

**Assertions**: helper присутствует (kind=function, Sort "0…");
include-декларация отсутствует; kb-функции/константы присутствуют
(Sort "1…"/"2…").

**Sufficiency**: include-замыкание — единый пул source.

#### `TestCompletion_RmsIntegration` (server)

**Setup**: harness serve_test; didOpen `t.rms` c секцией; запрос.

**Trace**: disp.Completion → Server.Completion → Parse → RmsAt → render.

**Assertions**: `*CompletionList`; items непусты; для command-кандидата:
Kind=CompletionItemKindFunction (не Keyword — смена MVP), InsertText
nil, SortText == Candidate.Sort; Documentation nil (concise-items).

**Sufficiency**: полная стопка + kind-таблица + concise-items.

#### `TestCompletion_InlineXsBlock` (server)

**Setup**: didOpen `t.rms` c `#includeXS\nvoid f() { |` (курсор в
блоке); запрос на позицию внутри блока.

**Assertions**: items содержат f (function, Sort "0…") и kb-функции;
координаты корректны (unshiftPos) — например, main-функция из kb.

**Sufficiency**: inline-роутинг повторяет SignatureHelp-паттерн.

### Negative Tests

#### `TestVisibleAt_InStringAndComment`

**Setup**: `void f() { string s = "abc|def"; } // tail|` (две позиции).

**Assertions**: обе → (nil/empty, false).

**Sufficiency**: in-string completion — главный «мусорный» кейс.

#### `TestRmsAt_NoContext_Empty`

**Setup**: rms-текст только из директив/вне секций (`#include "a.rms"`);
pos на директиве вне секций.

**Assertions**: RmsAt → пустой слайс (len 0), не nil-ошибка.

**Sufficiency**: молчание как пустой список.

#### `TestXsAt_VisibleAtFalse_Empty`

**Setup**: XsParse с pos в строке; external=nil.

**Assertions**: пустой слайс.

**Sufficiency**: единый in-string-гейт.

#### `TestCompletion_UnknownLanguage_EmptyList`

**Setup**: didOpen `t.txt`; запрос.

**Assertions**: `*CompletionList`, Items len 0; err nil.

**Sufficiency**: не-ошибка для неизвестного языка.

### Edge Case Tests

#### `TestRmsAt_UnknownOwner_NoAttributes`

**Setup**: `not_a_command 1` (нет в kb), pos на имени (kind=none).

**Assertions**: команды секции есть; атрибутов нет; паник нет.

**Sufficiency**: деградация без краха.

#### `TestRmsAt_AttributeWithoutValue_DefaultsToName`

**Setup**: `create_elevator 7 { set_scaling_by_map }` (без значения),
pos на имени атрибута.

**Assertions**: атрибуты владельца (не константы) — zero Value.Range.

**Sufficiency**: дефолт name-позиции.

#### `TestXsAt_SameNameDifferentKinds_BothKept`

**Setup**: source `void foo() {}` + `int foo;`... — XS не допустит;
берём top-level variable `int foo` + external function foo.

**Assertions**: оба кандидата (function и variable, оба Sort "0").

**Sufficiency**: словарь kinds не схлопывает одноимённые.

#### `TestXsAt_KbFunctionNoReturnType_DetailWithoutRet`

**Setup**: kb-функция с пустым ReturnType (есть в данных, напр. с void?
— берём любую с "" при наличии, иначе фикструем через source Decl без
Type: `foo() {}`).

**Assertions**: Detail == "foo()" (без «void »-подобного префикса).

**Sufficiency**: не выдумываем типы.

#### `TestCompletion_Stateless_SameAnswerTwice` (server)

**Setup**: два запроса с разными params.Context (TriggerCharacter
различен, эмуляция ретриггера).

**Assertions**: items идентичны (require.Equal).

**Sufficiency**: контракт stateless (AC task.md).

#### `TestCompletion_EmptyCandidates_EmptyListNotError` (server)

**Setup**: `.rms` документ, позиция вне контекста (даёт пустых
кандидатов), например директива вне секций.

**Assertions**: result — *CompletionList; Items != nil (len 0); err nil.

**Sufficiency**: AC «пустой список — не nil, не ошибка».

## Additional Instructions for the Implementation Agent

- Порядок: xs (VisibleAt) → complete (Candidate → Completer) → server
  (DI → Completion → Serve) — совпадает с arch.md; каждая задача —
  зелёные гейты (memory-cap `go test ./... -count=1`, goimports,
  golangci-lint, `goga lint`, `goga contract <cell>`).
- Существующие тесты не модифицируются, КРОМЕ completion-тестов server
  (`TestServe_CompletionAllXsFunctions`, `TestServe_CompletionRmsScopedToSection`)
  — они проверяют заменяемый MVP; переписать под новый контракт
  (kind-таблица, concise-items, клиентская фильтрация).
- Удалить ставшие мёртвыми: `completionsRms`, `completionsXs`,
  `wordPrefix`, `matchesPrefix`, `commandSignature`, `functionSignature`
  (перед удалением убедиться grep'ом, что hover их не использует).
- `VisibleAt` реализуется в xs/ast.go рядом с Definition; переиспользовать
  `declNameRange`, `paramNameRange`, обобщить `collectLocals` →
  collect-без-фильтра-по-имени (не копировать логику covers).
- В complete НЕ импортировать go.lsp.dev (протоколо-независимость,
  проверяется goga contract + ревью).
- Kind-словарь Candidate и kindMap server — точные таблицы из
  контрактов; никаких новых значений без правки CODEMANIFEST.
- `kb/data/rms-commands.json` не перегенерировать (by design, паритет
  с signature help).
