# Plan: `master` — Completion (textDocument/completion)

<!-- Компилирован из design.md (`72301b5`) по контрактам `8c77336`
(arch.md). Прецедент структуры — план signature help (PR #5).
Выполняется в ветке task/completion; CODEMANIFEST — read-only. -->

## Purpose

Реализовать `textDocument/completion` для AoE2 RMS + XS: новая
протоколо-независимая ячейка `complete` (`Completer`/`Candidate`),
навигационный метод `xs.XsFile.VisibleAt` (полный scope: top-level +
params/locals), интеграция в `server` (DI, роутинг `.rms`/`.xs`/
inline-XS, рендер `Candidate` → `protocol.CompletionItem`).

Главные разрывы контракт↔код: ячейка `complete` не существует;
`VisibleAt` не реализован; `Server.Completion` — старый MVP (прямые
Store-запросы, серверная префикс-фильтрация, Keyword-kind,
Documentation в items) — заменяется делегированием в `Completer`.

Стратегия: зависимостно-корректный порядок **xs → complete → server**,
TDD в каждой задаче (контракт-тесты → код → логика), silence-never-guess,
нулевые изменения поведения не-completion фич (SC8).

## Context

### Contract Surface

**Entity: `xs.XsFile` (+метод `VisibleAt`)**
- Type: существующий Entity (modify), метод в навигационном семействе
- Declared `location`: `xs/ast.go`
- Facade: метод экспортирован у `xs.XsFile`
- Method: `VisibleAt(pos: Pos) -> visible: []Symbol, found: bool`
- Семантика (из CODEMANIFEST + design):
  - `found=false` ТОЛЬКО для позиций в строках/комментариях (noncode);
  - состав: топ-левел именованные декларации (Kind=function/variable/
    rule/event/extern; include пропускается) → параметры объемлющей
    функции (kind=param) → локали объемлющих блоков, объявленные до
    `pos` (kind=local; covers-семантика `declCandidate` из Definition);
  - порядок: top по объявлению → параметры по списку → локали по
    позиции декларации;
  - Selection ⊆ Range для каждого узла;
  - конфликты имён не резолвятся (одноимённые — оба);
  - детерминированность.
- Внутренняя база (существует): `f.noncode`, `declNameRange`,
  `paramNameRange`, `collectLocals`/`declaredLocal`, `declCandidate.covers`.
- Annotation context: глобальные xs (parser never fails; kinds —
  Go-константы Kind*), `symbols` (common), `xs_grammar`.

**Entity: `complete.Completer`**
- Type: Entity (new)
- Declared `location`: `complete/completer.go`
- Facade: `complete.NewCompleter(store *kb.Store) *Completer`
- Properties: store (DI, внутреннее поле)
- Methods:
  - `RmsAt(file rms.RmsFile, pos common.Pos) []Candidate` —
    контекстная матрица: ArgAt found=false → SectionAt (global → все);
    Kind=none → команды секции + атрибуты владельца; Kind=arg →
    константы; Kind=attr → имя-vs-значение по `Attribute.Value.Range`
    (значение → константы, имя → атрибуты владельца).
  - `XsAt(file xs.XsFile, pos common.Pos, external []xs.Decl) []Candidate` —
    VisibleAt found=false → пусто; пул source (visible + external;
    extern→function; rule/event/include skip) вытесняет kb; kb
    Functions+Constants.
- Requirements: stateless; кандидаты не выдумываются; пустой список —
  штатное молчание; паритет качества RMS ↔ XS.

**Entity: `complete.Candidate`**
- Type: Entity (new, чистые данные — стиль hints.Hint)
- Declared `location`: `complete/candidate.go`
- Facade: `complete.Candidate` (struct, construct-and-use)
- Properties: `Label string`, `Kind string` (словарь: command /
  attribute / constant / function / variable / param / local),
  `Detail string` (одна строка; "" — данные недоступны), `Sort string`
  (группа контекста + Label).

**Entity: `server.Server` (+DI, `Completion` переписан)**
- Type: существующий Entity (modify)
- Declared `location`: `server/server.go`
- Signature: `NewServer(store, analyzer, computer, completer)` — +DI
- Method `Completion(ctx, *protocol.CompletionParams) (protocol.CompletionResult, error)`:
  роутинг `.xs` (XsParse + ExternalDecls(uri) → XsAt) / `.rms` (Parse;
  pos ∈ XsBlock → unshiftPos + XsParse(Code) + ExternalDecls("") →
  XsAt; иначе RmsAt) → рендер → `*CompletionList{IsIncomplete: false}`;
  пусто → пустой Items (не nil, не ошибка); stateless (params.Context
  игнорируется); read-only.
- `Serve` (serve.go): сборка + `complete.NewCompleter(store)`.

### Re-exports

Нет.

### Usages Context

- `conventions` (.goga/usages/conventions.md): DI-конструкторы,
  `(result, error)`, table-driven testify-тесты, goimports, гейты.
- `lsp-protocol` (.goga/usages/cooks/lsp-protocol.md): секции «Hover
  and Completion» + «Completion Items» — result shape (`*CompletionList`),
  stateless, Label/Kind/Detail/SortText, InsertText опущен, plain text,
  клиентская фильтрация, IsIncomplete: false.
- `rms_grammar`, `xs_grammar`: секции/позиционная семантика атрибутов;
  топ-левел decls, блоки `{ }`.

### Imported Usages

- `lookups` из `kb` (`kb/.usages/lookups.md`): `Functions()`,
  `Constants("")`, `Commands(sec)`, `Command(name)`; имена
  case-sensitive; формат «Kind Min..Max» при Range.
- `rms-parsing` из `rms` (`rms/.usages/rms-parsing.md`): `ArgAt`
  (kind arg/attr/none; строки/комментарии/directives → found=false),
  `SectionAt` (синтетическая "global" только если материализована).
- `xs-parsing` из `xs` (`xs/.usages/xs-parsing.md`): новая секция
  «Visible symbols (completion)» — found-семантика, словарь kinds,
  shadowing не резолвится.
- `positions-and-diagnostics`, `symbols` из `common`: Pos; Symbol
  (Selection ⊆ Range; словарь kind у производителя).
- `completing` из `complete` (`complete/.usages/completing.md`):
  consumer-контракт Completer (для server).

### Local Usages

- `complete/.usages/completing.md` — создан на материализации; после
  реализации сверить примеры с фактическим API. Связан с Task 3–4.

### External Dependencies

- `go.lsp.dev/protocol` + `jsonrpc2` + `uri` (go.mod) — protocol.CompletionItem,
  CompletionItemKind, CompletionList.
- `testify` — тесты.
- Новых зависимостей нет.

## Facts

- Внутри xs существует вся scope-машинера: `noncode`, `declNameRange`,
  `paramNameRange`, `collectLocals`+`declaredLocal`, `declCandidate.covers`
  (`!at.Before(nameRange.Start) && at.Before(scopeEnd)`), eofPos.
  `VisibleAt` = инверсия `bestDeclarer` (перечислить покрывающих
  candidates вместо выбора лучшего для одного имени).
- `rms.Attribute` экспортирует `Name`, `Value Expr`, `Range`; `Expr.Range`
  экспортирован; атрибут без значения → zero-Range → Contains=false →
  дефолт «на имени».
- Аргументные kind-значения — Go-константы `rms.KindArg/KindAttr/KindNone`.
- server: `unshiftPos(pos, block.Range.Start)` существует (паттерн
  SignatureHelp); `openDocument`, `fromProtocolPos`, Closure-доступ —
  существующие хелперы.
- Старый MVP: `completionsRms`/`completionsXs` + хелперы `wordPrefix`,
  `matchesPrefix`, `commandSignature`, `functionSignature` — после
  замены мёртвые (hover их не использует — проверено grep).
- Существующие completion-тесты server:
  `TestServe_CompletionAllXsFunctions`, `TestServe_CompletionRmsScopedToSection`
  (+ capability-тест `caps.CompletionProvider`) — первые два
  переписываются под новый контракт, третий остаётся.
- `kb/data/rms-commands.json` не перегенерировать (by design).
- Тесты — только под memory cap (systemd-run, см. CLAUDE.md).

## Gap Analysis

- Missing contract entities: `complete.Completer`, `complete.Candidate`
  (пакет не существует); `xs.XsFile.VisibleAt`.
- API mismatches: `Server.Completion` реализован как MVP — не
  соответствует переписанному контракту (роутинг inline-XS, kindMap,
  concise-items, клиентская фильтрация); `NewServer` без completer;
  `Serve` без сборки.
- Reuse: scope-машинера xs (выше); unshiftPos/openDocument/Closure в
  server; kb list-lookups; kind-таблица DocumentSymbol как прецедент.
- Test coverage gaps: нет тестов VisibleAt/Completer/нового Completion;
  два MVP-теста конфликтуют с новым контрактом.
- Visibility: `goga contract xs|server` сейчас красный (contracts-first,
  ожидаемо) — позеленеет по ходу задач; `goga contract complete`
  потребует существования пакета.

---

## Tasks

> **Package ordering rule**: задачи ячейки завершаются до перехода к
> следующей: **xs → complete → server**. Внутри код-задачи — TDD
> (контракт-тесты первыми).

### Task 1: xs — `XsFile.VisibleAt` (TDD)

Ячейка xs (modify). Новый метод навигации для completion: символы,
видимые в точке. Реализация в `xs/ast.go` рядом с `Definition`;
переиспользовать существующую scope-машинеру — НЕ копировать covers-
логику. CODEMANIFEST read-only.

Scope-модель (verbatim из design): covers-семантика `declCandidate` —
локаль видима, если `!pos.Before(nameRange.Start) && pos.Before(scopeEnd)`
(объявлена до курсора в объемлющем блоке); параметры видны во всём
диапазоне тела функции; топ-левел — везде. Это та же модель, что у
`Definition` (затенение), никакой второй интерпретации.

**Usages relevant to this task:**
- `xs-parsing` (локальная секция «Visible symbols (completion)» —
  consumer-пример с found-семантикой; словарь kinds top-level
  function/variable/rule/event/extern + param/local; include skip).
- `symbols` из common (`common/.usages/symbols.md`): Selection ⊆ Range.
- `conventions`: table-driven testify.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [ ] **STEP 0 (DECLARATION)**: работаем над Task 1 — xs.XsFile.VisibleAt
- [ ] **Contract tests** (упадут): в `xs/navigation_test.go` —
  `TestVisibleAt_Contract`: метод существует, сигнатура
  `VisibleAt(common.Pos) ([]common.Symbol, bool)`; на фикстуре с
  decls возвращает found=true; тестируем через пакет xs (фасад).
- [ ] **Code**: в `xs/ast.go` реализовать `VisibleAt` по Algorithm
  (design §`xs.XsFile.VisibleAt`): (1) noncode → (nil, false);
  (2) топ-левел `Kind != DeclInclude` → Symbol{Kind: decl.Kind, Name,
  Range: decl.Range, Selection: declNameRange(decl)};
  (3) для function-Decl с `decl.Range` ∋ pos — параметры:
  `paramNameRange(decl, p.Name)` (fallback r=decl.Range при !ok) →
  Symbol{Kind: "param", Range: r, Selection: r};
  (4) локали: обобщить `collectLocals` → внутренний
  `collectVisibleLocals(stmts, pos, blockEnd, out)` без фильтра по
  имени (использовать обобщённый `declaredLocalName(item)` поверх
  `declaredLocal`), включать только covers(pos) →
  Symbol{Kind: "local", Range: r, Selection: r};
  (5) порядок: top → params → locals; return (out, true).
- [ ] **Interface verification**: `go test ./xs/ -run TestVisibleAt_Contract` — pass
- [ ] **Logic tests** (design, Test Stack Trace):
  `TestVisibleAt_TopLevelDecls` (g=variable, f=function, e=extern,
  a=param; include отсутствует; Selection ⊆ Range);
  `TestVisibleAt_ParamAndLocalScopes` ([f, p, a1]; НЕ b1 — блок
  закрылся до pos; НЕ a2 — объявлена после pos);
  `TestVisibleAt_InStringAndComment` (две позиции → found=false);
  edge: пустой файл → (empty, true); одноимённые top+local → оба.
- [ ] **Debugging**: memory-cap `go test ./xs/ -count=1` — чинить
  реализацию (не тесты) до зелёного
- [ ] **Contract re-verification**: сигнатура/фасад/виды kinds — по
  контракту; `goga contract xs` — exit 0
- [ ] **Lint**: `goimports -w xs/`; `golangci-lint run`; `goga lint`
- [ ] **STEP 8 (COMPLETION)**: отметить чекбоксы
- [ ] → REVIEW → APPROVAL → NEXT TASK

### Task 2: complete — bootstrap пакета + `Candidate` (TDD)

Ячейка complete (create). Пакет `complete` (doc-комментарий пакета по
conventions), файл `complete/candidate.go`: чистые данные-кандидат,
стиль `hints.Hint` (construct-and-use, без мутации). Манифест уже
материализован (`complete/CODEMANIFEST`), read-only.

**Usages relevant to this task:**
- `conventions`: doc-комментарии на экспортированное; testify.
- `completing` (локальная, `complete/.usages/completing.md`): после
  реализации сверить, что consumer-примеры соответствуют API.

**CRITICAL: `CODEMANIFEST` files — read-only. Fix implementation, never the contract.**

- [ ] **STEP 0 (DECLARATION)**: Task 2 — bootstrap complete + Candidate
- [ ] **Contract tests** (упадут): `complete/candidate_test.go` —
  `TestCandidate_Contract`: тип экспортирован из `complete`, поля
  `Label/Kind/Detail/Sort` — string, конструкция
  `complete.Candidate{...}` компилируется (фасад).
- [ ] **Code**: создать `complete/candidate.go` —
  `type Candidate struct { Label, Kind, Detail, Sort string }` с
  doc-комментариями свойств из контракта; пакетный doc-комментарий.
- [ ] **Interface verification**: `go test ./complete/ -run TestCandidate_Contract` — pass
- [ ] **Logic tests**: `TestCandidate_ConstructAndUse` — конструкция
  со всеми полями, чтение полей (чистые данные; мутаций нет).
- [ ] **Debugging**: memory-cap `go test ./complete/ -count=1` — зелёный
- [ ] **Contract re-verification**: `goga contract complete` — сигнатура
  Candidate совпала (место: candidate.go)
- [ ] **Lint**: `goimports -w complete/`; `golangci-lint run`; `goga lint`
- [ ] **STEP 8 (COMPLETION)**: отметить чекбоксы
- [ ] → REVIEW → APPROVAL → NEXT TASK

### Task 3: complete — `Completer.RmsAt` (TDD)

Ячейка complete. Вычислитель RMS-кандидатов: контекстная матрица по
`ArgAt`/`SectionAt` + kb list-lookups; рендер в Candidate с
детерминированным Sort («0»-атрибуты / «1»-команды / «2»-константы +
Label). Файл `complete/completer.go`. kb — настоящий `NewStore()`
(embedded), без моков. Read-only контракт.

Контекстная матрица (verbatim из design):
```
site, ok := file.ArgAt(pos)
!ok → SectionAt: "global"→Commands(""); секция→Commands(sec); нет→nil
Kind=none  → sectionCommands(pos) ∪ ownerAttributes(site.Stmt)
Kind=arg   → constants()
Kind=attr  → последний a ∈ site.Stmt.Attributes: a.Name==site.Name &&
             a.Range ∋ pos; a.Value.Range ∋ pos → constants();
             иначе → ownerAttributes(site.Stmt)
ownerAttributes: Store.Command(stmt.Name); !ok→nil; cmd.Attributes →
             Candidate{Label, "attribute", attrDetail(a), "0"+Name}
sectionCommands: Store.Commands(sec) → {Name, "command", c.Section, "1"+Name}
constants: Store.Constants("") → {Name, "constant", c.Value, "2"+Name}
attrDetail: Min&&Max → "Kind Min..Max"; Kind≠"" → "Kind"; иначе "" (флаг)
```

**Usages relevant to this task:**
- `lookups` из kb: `Commands(section)` ("" — все), `Command(name)` →
  (Command, bool), `Constants("")`, CommandArg.Kind/Range (ValueRange
  Min/Max — строки, "" не замайнено); формат «Kind Min..Max» паритетен
  hints-меткам.
- `rms-parsing` из rms: ArgAt-семантика (kind-значения; found=false на
  директивах/заголовках/строках), SectionAt ("global" материализуется
  только при непустых глобальных statements).
- `conventions`: table-driven.

**CRITICAL: `CODEMANIFEST` files — read-only. Fix implementation, never the contract.**

- [ ] **STEP 0 (DECLARATION)**: Task 3 — Completer + RmsAt
- [ ] **Contract tests** (упадут): `complete/completer_test.go` —
  `TestCompleter_Contract`: `complete.NewCompleter(store *kb.Store)`
  возвращает *Completer; метод `RmsAt(rms.RmsFile, common.Pos) []complete.Candidate`.
- [ ] **Code**: `complete/completer.go` — `Completer{store}`,
  `NewCompleter(store *kb.Store) *Completer`, метод `RmsAt` по
  матрице (выше); приватные хелперы sectionCommands/ownerAttributes/
  constants/attrDetail.
- [ ] **Interface verification**: `go test ./complete/ -run TestCompleter_Contract` — pass
- [ ] **Logic tests** (design):
  `TestRmsAt_CommandNamePosition` (команды секции Sort "1…"+ атрибуты
  владельца Sort "0…"; Kind ∈ {command, attribute}; констант нет);
  `TestRmsAt_ArgValuePosition_Constants` (kind=constant, Detail=Value,
  Sort "2…"); `TestRmsAt_AttrNameVsValue` (имя → атрибуты, флаг
  set_scaling_by_map Detail=""; значение → константы);
  `TestRmsAt_GlobalSection_AllCommands`; `TestRmsAt_NoContext_Empty`
  (директива вне секций → len 0); `TestRmsAt_UnknownOwner_NoAttributes`;
  `TestRmsAt_AttributeWithoutValue_DefaultsToName`.
- [ ] **Debugging**: memory-cap `go test ./complete/ -count=1` — зелёный
- [ ] **Contract re-verification**: `goga contract complete` — RmsAt
  сигнатура совпала (ресивер Completer, completer.go)
- [ ] **Lint**: `goimports -w complete/`; `golangci-lint run`; `goga lint`
- [ ] **STEP 8 (COMPLETION)**: отметить чекбоксы
- [ ] → REVIEW → APPROVAL → NEXT TASK

### Task 4: complete — `Completer.XsAt` (TDD)

Ячейка complete. XS-кандидаты: VisibleAt + external + kb, truth-модель
(source > kb), Sort-группы («0» source / «1» kb-функции / «2»
kb-константы). Тот же файл `complete/completer.go`. Read-only контракт.

Алгоритм (verbatim из design):
```
syms, ok := file.VisibleAt(pos); !ok → nil
FOR sym IN syms: kind := sym.Kind;
  function/variable/param/local → как есть; extern → "function";
  rule/event → SKIP (не адресуемы по имени)
  name ∉ seen → out += {Name, kind, detailFor(kind), "0"+Name}; seen+=
FOR d IN external: extern→"function"; function→"function";
  variable→"variable"; else SKIP; name ∉ seen → "0"+d.Name (fnDetail)
FOR fn IN Store.Functions() WHERE Name ∉ seen: {"function", fnDetail(fn), "1"}
FOR c IN Store.Constants("")  WHERE Name ∉ seen: {"constant", c.Value, "2"}
fnDetail: source Decl → "[Type ]Name(T1, T2)"; kb → "[ReturnType ]Name(…)";
  пустой тип — без префикса; variable/param/local → Detail ""
```

**Usages relevant to this task:**
- `xs-parsing` из xs: секция «Visible symbols (completion)» —
  found-семантика; shadowing не резолвится (потребитель решает).
- `lookups` из kb: `Functions()` (порядок файла данных), `Constants("")`;
  `Function.ReturnType/Params`, `Param.Type`.
- `conventions`: table-driven.

**CRITICAL: `CODEMANIFEST` files — read-only. Fix implementation, never the contract.**

- [ ] **STEP 0 (DECLARATION)**: Task 4 — Completer.XsAt
- [ ] **Contract tests**: `TestCompleter_XsAt_Contract` — сигнатура
  `XsAt(xs.XsFile, common.Pos, []xs.Decl) []Candidate`.
- [ ] **Code**: метод `XsAt` по алгоритму (выше); хелпер fnDetail
  (две ветви: xs.Decl / kb.Function); пул seen (map или сортированный
  срез — по conventions, без преждевременной оптимизации).
- [ ] **Interface verification**: `go test ./complete/ -run TestCompleter_XsAt_Contract` — pass
- [ ] **Logic tests** (design):
  `TestXsAt_SourceShadowsKb` (ровно один trQuestVarGet, Detail из
  source, Sort "0…"); `TestXsAt_ExternalDeclsMerged` (helper function
  "0…"; include-dекларация отсутствует; kb "1…"/"2…" присутствуют);
  `TestXsAt_VisibleAtFalse_Empty` (pos в строке); edge:
  SameNameDifferentKinds_BothKept (function+variable, оба "0…");
  KbFunctionNoReturnType_DetailWithoutRet (source `foo() {}` →
  Detail "foo()"); rule/event из VisibleAt исключены; param/local с
  Detail "".
- [ ] **Debugging**: memory-cap `go test ./complete/ -count=1` — зелёный
- [ ] **Contract re-verification**: `goga contract complete` — обе
  сигнатуры Completer совпали
- [ ] **Lint**: `goimports -w complete/`; `golangci-lint run`; `goga lint`
- [ ] **STEP 8 (COMPLETION)**: отметить чекбоксы
- [ ] → REVIEW → APPROVAL → NEXT TASK

### Task 5: server — DI completer + `Completion` из `complete` (TDD)

Ячейка server (modify). Замена MVP: `NewServer` + поле completer,
`Server.Completion` делегирует в `complete.Completer`, рендер по kind-
таблице, удаление мёртвых хелперов, сборка в `Serve`. Read-only контракт.

Kind-таблица (verbatim, паритет DocumentSymbol):
command→`protocol.CompletionItemKindFunction`, attribute→Field,
constant→Constant, function→Function, variable→Variable,
param→Variable, local→Variable. Рендер: `Label`; `Detail` —
`protocol.NewOptional(c.Detail)` только при Detail≠""; `SortText` =
Optional(c.Sort); `InsertText` опущен; `InsertTextFormat` — plain
(zero value); `Documentation` — nil (concise-items). Result — всегда
`*protocol.CompletionList{IsIncomplete: false, Items: items}`; пусто →
`Items: []protocol.CompletionItem{}` (не nil).

Роутинг (verbatim из design):
```
".xs":  file := XsParse(text); ext := Closure.ExternalDecls(uri)
        cands := completer.XsAt(file, pos, ext)
".rms": file := Parse(text)
        IF ∃ block IN file.XsBlocks WHERE block.Range ∋ pos:
          bp := unshiftPos(pos, block.Range.Start)
          cands := completer.XsAt(XsParse(block.Code), bp, ExternalDecls(""))
        ELSE cands := completer.RmsAt(file, pos)
```

**Usages relevant to this task:**
- `lsp-protocol`: секция «Completion Items» (result shape, stateless,
  клиентская фильтрация, InsertText-правила) + «Hover and Completion».
- `completing` из complete: consumer-контракт (конструкция
  NewCompleter, пустой slice = молчание → пустой CompletionList).
- `conventions`: DI, errors.

**CRITICAL: `CODEMANIFEST` files — read-only. Fix implementation, never the contract.**

- [ ] **STEP 0 (DECLARATION)**: Task 5 — server DI + Completion
- [ ] **Contract tests** (упадут): в `server/serve_test.go` —
  `TestServer_Completion_Contract`: NewServer принимает completer
  (компиляция с `complete.NewCompleter(store)`); `Completion`
  возвращает `*protocol.CompletionList` (type-assert).
- [ ] **Code**: `server.go` — struct Server += `completer *complete.Completer`;
  NewServer + параметр; `Completion` переписать по роутингу+рендеру
  (выше); приватный `renderCandidates([]complete.Candidate) []protocol.CompletionItem`
  + kindMap. `serve.go` — Serve: `completer := complete.NewCompleter(store)`.
- [ ] **Code (удаление мёртвого)**: удалить `completionsRms`,
  `completionsXs`, `wordPrefix`, `matchesPrefix`, `commandSignature`,
  `functionSignature` (перед удалением grep — убедиться, что вне
  completion не используются; hover рендерит Markdown отдельно).
- [ ] **Interface verification**: `go test ./server/ -run TestServer_Completion_Contract` — pass
- [ ] **Logic tests**: `TestCompletion_UnknownLanguage_EmptyList`
  (.txt → Items len 0, err nil); документ не открыт → пустой Items;
  kindMap покрывает весь словарь Candidate (тест-таблица по 7 kinds).
- [ ] **Debugging**: memory-cap `go test ./server/ -count=1` — чинить
  реализацию (не тесты); существующие НЕ-completion тесты не менялись
  (SC8) — если падают, чинить реализацию
- [ ] **Contract re-verification**: `goga contract server` — exit 0
- [ ] **Lint**: `goimports -w server/`; `golangci-lint run`; `goga lint`
- [ ] **STEP 8 (COMPLETION)**: отметить чекбоксы
- [ ] → REVIEW → APPROVAL → NEXT TASK

### Task 6: server — integration-тесты completion full stack

Ячейка server. Сквозные сценарии через dispatcher (паттерн
serve_test.go: harness + `h.disp.Completion`). Единственная задача,
меняющая существующие тесты: переписать два MVP-теста под новый
контракт (задокументировано в design; остальные тесты — SC8 не трогать).

**Usages relevant to this task:**
- `lsp-protocol`: full-stack Completion через jsonrpc2-dispatcher.
- `completing` из complete + `xs-parsing` (inline-блок координаты).

**CRITICAL: `CODEMANIFEST` files — read-only.**

- [ ] Переписать `TestServe_CompletionAllXsFunctions` → новый контракт:
  .xs документ, пустой префикс не фильтруется сервером (полный набор),
  kb-функции с Kind=Function и SortText "1…", Documentation nil.
- [ ] Переписать `TestServe_CompletionRmsScopedToSection` → команды
  секции (Kind=Function, не Keyword), Detail=Section, атрибуты
  владельца SortText "0…" при kind=none.
- [ ] `TestCompletion_RmsIntegration`: .rms команда → рендер (Label,
  Kind=Function, SortText, InsertText nil).
- [ ] `TestCompletion_InlineXsBlock`: курсор в inline-XS → кандидаты
  через unshiftPos (локальная функция + kb-функции).
- [ ] `TestCompletion_Stateless_SameAnswerTwice`: два запроса с разными
  params.Context → require.Equal(items).
- [ ] `TestCompletion_EmptyCandidates_EmptyListNotError`: позиция без
  кандидатов → Items != nil, len 0, err nil.
- [ ] Run validation: memory-cap `go test ./... -count=1` — весь
  модуль зелёный; `goimports -w .`; `golangci-lint run`; `goga lint`;
  `goga contract xs complete server` — все exit 0
- [ ] → REVIEW → APPROVAL → PLAN COMPLETE

---

## Validation Commands

- `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`: все тесты (только под memory cap — CLAUDE.md)
- `goimports -w .`: форматирование
- `golangci-lint run`: линтер (0 issues)
- `goga lint`: DSL-контракты (0 ошибок)
- `goga contract xs` / `goga contract complete` / `goga contract server`: соответствие реализации контрактам (exit 0)

---

## Completion Criteria

- [ ] Контрактные сущности реализованы в верных `location`
  (xs/ast.go; complete/completer.go, complete/candidate.go; server)
- [ ] Фасад: `VisibleAt`, `NewCompleter`, `RmsAt`, `XsAt`, `Candidate`
  доступны из пакетов xs / complete
- [ ] Сигнатуры и поведение соответствуют CODEMANIFEST (design —
  эталон алгоритмов)
- [ ] Stateles-правило, пустой-список-не-nil, kind-таблица — соблюдены
- [ ] TDD-протокол пройден в каждой код-задаче (шаги 0–8)
- [ ] Контракт- и логика-тесты покрывают фасад/API/поведение; 23
  сценария design имплементированы (+2 переписанных integration)
- [ ] SC8: не-completion тесты и поведение не изменились; заменены
  только два MVP-completion-теста server
- [ ] CODEMANIFEST не модифицировались (read-only)
- [ ] Все Validation Commands зелёные
- [ ] Каждая Usages-запись задействована хотя бы в одной задаче
  (conventions — все; lsp-protocol — 5,6; lookups — 3,4; rms-parsing — 3;
  xs-parsing — 1,4,6; symbols/positions — 1; completing — 3,4,5,6)
