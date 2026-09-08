# Design Document: `master` — fix-review-defects (№1–№5 из ревью 2026-09-08)

<!-- Топик: `master` (`.goga/history/2026/master/`). Основа: материализованные
контрактные дельты `arch.md` (rms/xs/analysis + xs-parsing.md), задача
`task.md`, источник `docs/reviews/2026-09-08-full-review.md`. Репро №1–№3
независимо верифицированы 2026-09-08. -->

## Contract Changes

### Changed CODEMANIFEST Files

- `rms/CODEMANIFEST` — `Parse` Algorithm: шаг 5 += «или конца файла»
  (№1); шаг 6 += «неявное закрытие незакрытого вложенного блока →
  Diagnostic severity=warning» (№5).
- `xs/CODEMANIFEST` — `XsParse` Algorithm шаг 2 += мультидекларации
  (№4); `XsFile.Definition` шаг 2 += «…или оператора for с его
  init-декларацией» (№2); `XsFile.VisibleAt` шаг 3 += for-init локаль
  области оператора (№2).
- `analysis/CODEMANIFEST` — `Analyzer.AnalyzeXs` шаг 3 += init-декларация
  for в собранных локалях, консервативно (№2).
- `xs/.usages/xs-parsing.md` — VisibleAt Preconditions += строка о
  for-init локали.

### New Entities

Нет — задача поведенческая, все типы существуют.

### Changed Entities

- `rms.Parse` — поведение (№1, №5), сигнатура неизменна.
- `xs.XsParse` — поведение (№3, №4), сигнатура неизменна.
- `xs.XsFile.VisibleAt` / `Definition` — поведение (№2), сигнатуры
  неизменны.
- `analysis.Analyzer.AnalyzeXs` — поведение (№2), сигнатура неизменна.

### Deleted Entities

Нет.

### Usages and Annotations Changes

- Аннотации перечисленных методов — см. Changed CODEMANIFEST Files.
- Практик в заголовках не добавлено/не удалено.

## Applied Fixes

### Fixed CODEMANIFEST Defects

- `xs/CODEMANIFEST` VisibleAt: `` `for (int i = …)` `` в backticks
  признано линтером неразрешимой ссылкой (`annotation_links_exists`) →
  формулировка без backtick-обёртки (reason: DSL reference rules).
- `analysis/CODEMANIFEST` AnalyzeXs: строгая формулировка «Push перед
  оператором / Pop после» заменена консервативной «входит в собранные
  локали… не маркируется» — трассировка показала: строгая область для
  for несимметрична существующим вложенным блокам (if/{}) и меняет
  диагностическое поведение существующих карт (решение пользователя,
  см. Session Decisions).

## Entity Interaction and Data Flow

### Interaction Diagram

```
rms.Parse ──── RmsFile.XsBlocks ────→ server (didOpen/didChange)
   │  №1: endXsBlock без паники на EOF
   │  №5: closeScopes warning

xs.XsParse ──── XsFile ──┬──→ XsFile.VisibleAt ──→ complete.Completer.XsAt ──→ server.Completion
   │  №3: scanString     │         №2: for-init локаль        (регрессия, kind=local)
   │  №4: parseTypedDecl ├──→ XsFile.Definition ──→ server/навигация
   │                     └──→ analysis.Analyzer.AnalyzeXs ──→ server (диагностика)
   │  №2: parseFor сохраняет init-декларацию        №2: ложные undefined уходят
```

### Data Flows

1. **№1/№5 (rms)**: `Parse(source, name)` → line-split → директивы →
   `endXsBlock(idx)` на директиве/EOF; `closeScopes(closer, stops)` на
   `end_*`. Потребитель: server.didOpen → диагностики + XsBlocks →
   `xs.XsParse(block.Code)`.
2. **№3/№4 (xs)**: `XsParse(source, name)` → `xscanner.next()` →
   `scanString()` (line-учёт); `parseTypedDecl()` (мультидекларации) →
   `file.Decls` → Symbols/Definition/VisibleAt/analysis/complete.
3. **№2 (xs→analysis/complete)**: `parseFor()` сохраняет init-декларацию
   как первый statement тела for → существующие обходы
   (`collectLocals`/`appendLocals` в xs; `collectLocals`/
   `walkStmtsXs` в analysis) подхватывают её автоматически.

### Entity Dependencies

Порядок инициализации не меняется (DI конструкторы без изменений):
`NewAnalyzer(store)`, `NewCompleter(store)`. Новых зависимостей нет.

## Code Stack Trace

### Trace: `rms.Parse` — путь №1 (endXsBlock, EOF без `\n`)

#### Chain

1. **Input**: `source = "#includeXS\nvoid main() { int x = 1; }"`
   (нет финального `\n`), `name = "t.rms"`.
2. `Parse` → `p.lines = Split(source, "\n")` → 2 строки;
   `p.starts = [0, 12]` → checkpoint: длины согласованы ✓.
3. Строка 0 — директива `#includeXS` (без аргумента): `p.inXs = true`,
   `p.xsStart = 1` → checkpoint: xsStart < len(lines) ✓.
4. Строка 1 — не директива/секция, `p.inXs` — пропускается как код
   блока (без терминатора) → до конца входа.
5. `closeAll()`: `p.inXs` → `endXsBlock(len(p.lines))` →
   `end = min(2, 2) = 2`; цикл обрезки пустых строк:
   `p.lines[1]` непуста → end остаётся 2 → **дефект**: `p.pos(2, 0)`
   читает `p.starts[2]` — index out of range → panic. Устраняется
   хелпером `lineStartPos` (см. Algorithm Design).
6. **Output** (после фикса): `RmsFile` c 1 `XsBlock{Code: "void main()
   { int x = 1; }", Range: [pos(1,0), конец строки 1]}`; diags пуст;
   паники нет → checkpoint ✓.

#### Checkpoint Summary

- Вектор A (блок с кодом, EOF): panic → фикс `lineStartPos(end)` ✓
- Вектор B (bare `#includeXS` последней строкой): `xsStart = len(lines)`
  → `p.pos(xsStart, 0)` — второй panic-вектор → тот же хелпер для
  Start ✓
- Вектор C (пустые хвостовые строки): цикл обрезки уменьшает end <
  len → старый путь корректен, не регрессирует ✓

### Trace: `rms.Parse` — путь №5 (closeScopes)

#### Chain

1. **Input**: `start_random / if 1 / percent_chance 50 /
   create_terrain GRASS / end_random`.
2. Стек scopes при `end_random`: `[random, if]` (percent_chance уже
   закрыт своим statement-переходом).
3. `closeScopes(closer=end_random, stops=isRandomName)`:
   итерация `slices.Backward` → i=1 (`if`): **дефект** — условие
   `i < len(p.scopes)-1` всегда false (len уже усечён предыдущими
   итерациями/первой проверкой), warning мёртв.
4. **Output** (после фикса): warning ``"end_random" closes an
   unterminated "if" block`` severity=warning code="syntax" ✓; сам
   `random` закрывается без warning (легитимная цель stops) ✓.

#### Checkpoint Summary

- Мёртвое условие → замена на `!stops(open.name) && open.name !=
  "percent_chance"` ✓
- `percent_chance` — не предупреждать (документировано в комментарии
  closeScopes) ✓
- Парные if/endif, start_random/end_random — без warning (регресс-тест) ✓

### Trace: `xs.XsParse` — путь №3 (scanString)

#### Chain

1. **Input**: `string s = "abc\` + `\n` + `DEF";\nint z = 1;`.
2. `next()` → `"` → `scanString()`: escape-ветка `s.pos += 2`
   пропускает `\n` без `s.line++/s.lineStart` → **дефект**: все
   последующие `posAt` дают Line на 1 меньше.
3. **Output** (после фикса): при escape с `\n` → `s.line++;
   s.lineStart = s.pos + 2`; декларация `z` получает Line=2 ✓;
   `noncode`-диапазон литерала корректен (Contains для VisibleAt/CallAt) ✓.

#### Checkpoint Summary

- `skipSpace`-инвариант (line считается только на `\n`; `\r` — обычный
  пробел) сохранён ✓
- `\` + не-`\n` — поведение неизменно ✓; `\` в конце входа — pos += 2
  безопасен (граница цикла) ✓

### Trace: `xs.XsParse` — путь №4 (parseTypedDecl, мультидекларация)

#### Chain

1. **Input**: `int a = 1, b = 2;` (top level).
2. `parseTypedDecl()`: `typ = "int"`, `name = a`, `= 1` → `decl a`
   сформирован; далее `expectSemi` встречает `,` → **дефект**: 2
   синтаксические ошибки, декларация `b` теряется.
3. **Output** (после фикса): цикл деклараторов по образцу
   `parseLocalDecl`: `Decl{a, Range от start (type words)}`, затем
   `Decl{b, Range от имени b, Type: "int", Body: [b = 2]}`; один
   `expectSemi` на весь statement; diags пуст ✓.
4. `p.record(name)` на каждое имя → occurrence-индекс содержит оба →
   Definition/References/VisibleAt/completion работают для `b` ✓.

#### Checkpoint Summary

- Одиночная декларация — путь байт-идентичен текущему (Range от
  type-word start, существующие тесты/outline не меняются) ✓
- Форма функции (`int f() {}`) не затронута: ветка `(` до цикла
  деклараторов ✓

### Trace: `xs.XsFile.VisibleAt` / `Definition` — путь №2

#### Chain

1. **Input**: `void main() { for (int i = 0; i < 10; i++) { int j =
   i + 1; } }`, pos в условии (или теле).
2. `parseFor`: init-декларация парсится `parseLocalDecl()` →
   `Stmt{Kind: StmtDecl, Exprs: [i = 0 binary], Range: "int i = 0"}` →
   **дефект**: `stmt.Exprs = append(stmt.Exprs, init.Exprs...)`
   разворачивает декларацию — маркер StmtDecl теряется.
3. **Output** (после фикса): init сохраняется как `stmt.Body[0]`
   (тело = [init, …statement(s)]); cond/step остаются в `stmt.Exprs`.
4. `VisibleAt(pos)` → `appendLocals(out, decl.Body, decl.Range.End,
   pos)`: рекурсия в for-тело использует `blockEnd = for.Range.End` →
   `i` видима во всём операторе (init/cond/step/body), после — нет ✓.
5. `Definition(pos_i)` → `bestDeclarer` → `collectLocals`:
   `declCandidate{nameRange: i-токен, scopeEnd: for.Range.End,
   depth: глубина}` → покрывает вхождения в заголовке и теле ✓.
6. `analysis.AnalyzeXs`: `collectLocals(decl.Body)` собирает имена из
   StmtDecl-ов рекурсивно → `i` в `declared` до проверок → ложных
   `undefined-symbol` нет; `walkStmtsXs` обходит init как обычную
   декларацию (`env.declareLocals`) — консервативно, без Push/Pop ✓.
7. `complete.Completer.XsAt`: VisibleAt возвращает `i` (kind=local) →
   кандидат `i` группы source ✓.

#### Checkpoint Summary

- Assign-форма `for (i = 0; …)` без type-word — путь `forPart`, не
  создаёт фантомную локаль ✓
- Мульти-init `for (int i = 0, j = 5; …)` — `parseLocalDecl` уже
  поддерживает запятые → обе локали ✓
- Тело без скобок `for (…) x = i;` — Body=[init, exprStmt] ✓
- Вложенный for/shadowing — глубинная декларация побеждает по depth ✓
- Гонка с существующими тестами: `parseFor.Exprs` больше НЕ содержит
  init-выражения → тесты, ожидающие «Exprs = [init, cond, step]»,
  легитимно обновляются (SC-правило; других потребителей Exprs-for нет) ✓

## Algorithm Design

### `rms: lineStartPos` (новый хелпер парсера, unexported)

**Responsibility**: позиция начала строки `i`, устойчивая к `i ==
len(p.lines)` (EOF).

**Algorithm:**
```
1. IF i < len(p.lines) → p.pos(i, 0)
2. ELSE (EOF): last = len(p.lines) - 1 →
   Pos{Line: last, Column: len(p.lines[last]),
       Offset: p.starts[last] + len(p.lines[last])}
   (совпадает с вычислением конца файла в closeAll)
```

**Edge Cases:**
- пустой source (lines = [""]): EOF-ветка → Pos{0,0,0} ✓

### `rms.Parse` — правка `endXsBlock` (№1)

**Algorithm:**
```
1. end = min(idx, len(p.lines))
2. обрезка пустых строк (без изменений)
3. Range = {Start: lineStartPos(p.xsStart), End: lineStartPos(end)}
   — обе границы безопасны при xsStart/end == len(p.lines)
```

**Edge Cases:**
- bare `#includeXS` в EOF → Code "", Range нулевой ширины в EOF
  (блок-мусор в outline — отдельная находка №7 ревью, вне задачи).

### `rms.Parse` — правка `closeScopes` (№5)

**Algorithm:**
```
FOR open В slices.Backward(p.scopes):
  IF !stops(open.name) AND open.name != "percent_chance":
     reportf(closer.at, warning, "syntax",
       `"%" closes an unterminated "%" block`, closer.text, open.name)
  finalizeScope(open); truncate
  IF stops(open.name): RETURN
reportf(..., error, `"%" without a matching opening block`)
```

**Edge Cases:**
- percent_chance закрывается неявно без warning (комментарий
  closeScopes); stops-цель без warning.

### `xs.XsParse` — правка `scanString` (№3)

**Algorithm:**
```
WHILE pos < len(src):
  IF src[pos]=='\\' AND pos+1 < len(src):
     IF src[pos+1]=='\n': line++; lineStart = pos+2
     pos += 2; CONTINUE
  IF src[pos]=='"': pos++; RETURN
  IF src[pos]=='\n': RETURN   // unterminated
  pos++
```

### `xs.XsParse` — правка `parseTypedDecl` (№4)

**Algorithm:**
```
1. typ = parseTypeWords(); первый name; ветка "(" → функция (без изменений)
2. цикл деклараторов (по образцу parseLocalDecl):
   a. name = next(); IF не ident → diag "expected a name in declaration"; BREAK
   b. record(name)
   c. IF atOp("="): parseExpr → Body=[initStmt], Range.End=value.End
   d. append Decl{Kind: variable, Name, Type: typ,
        Range: {Start: (первый декларатор ? start : name.at.Start),
                End: name/value.End}}
   e. IF atOp(","): next(); CONTINUE ELSE BREAK
3. expectSemi(последний End)
```

### `xs.XsParse` — правка `parseFor` (№2)

**Algorithm:**
```
1. init-ветка (type-word/const): init = parseLocalDecl()
2. НЕ разворачивать: сохранить init в локальной переменной
3. cond/step → stmt.Exprs (как сейчас)
4. body = parseStmt()
5. stmt.Body = [init] + body  (только когда init был декларацией)
6. stmt.Range.End = body.End
```

**Constraints:** Exprs у StmtFor = только cond/step; init живёт в Body
как StmtDecl — область видимости (scopeEnd = for.Range.End) и
сбор локалей работают через существующие обходы без их правки.

### `analysis.Analyzer.AnalyzeXs` — без правки кода

`collectLocals`/`walkStmtsXs` уже рекурсивно обходят Body и StmtDecl →
init-декларация for подхватывается автоматически. Консервативная
семантика (использование после цикла не маркируется) — решение
пользователя, зафиксировано в контракте.

### `complete.Completer.XsAt` — без правки кода

`i` приходит из `VisibleAt` как kind=local → рендер и группы
существующие. Только регрессионный тест.

## Cross-cutting Concerns

- **Error handling**: без изменений — парсеры никогда не падают, все
  проблемы как Diagnostic; новые warning-и (№5) в общем конвейере
  сортировки/мерджа.
- **Logging**: отсутствует (существующая архитектура парсеров —
  чистые функции без IO).
- **Validation**: `goga lint` (0 ошибок), `goga contract rms xs
  analysis complete` (exit 0) после контрактных дельт — выполнено;
  тесты строго под memory cap (CLAUDE.md, runaway-прецедент).
- **Caching**: не затрагивается (DocStore/Resolver вне области).
- **Concurrency**: правки в чистых функциях парсера; race-режим
  существующих тестов (`go test -race`) остаётся зелёным.

## Usages Analysis

### `conventions`
- **What**: Go-правила проекта (формат, DI, тесты table-driven).
- **Where**: все изменённые ячейки; тесты по `Test<Component>_<Scenario>`.
- **Why**: базовая практика из `.goga/config.yml`.
- **How**: `goimports`, testify require/assert, memory-cap запуск.

### `rms_grammar` / `xs_grammar`
- **What**: грамматики RMS/XS (директивы, statements, for, recovery).
- **Where**: `Parse` (№1/№5 — inline-режим, end_*-блоки), `XsParse`
  (№3 — строки с escape; №4 — переменные; №2 — for).
- **Why**: единственный источник языковой семантики.
- **How**: inline-XS до конца файла; `end_*`-парность; `\`-escape в
  строках; for(init;cond;step) с typed-decl init.

### Imported Usages
- `xs-parsing` from `xs` — семантика VisibleAt/Definition для analysis
  и complete; обновлена строкой о for-init локали.
  Path: `xs/.usages/xs-parsing.md`.
- `positions-and-diagnostics`, `symbols` from `common` — построение
  Pos/Range/Diagnostic/Symbol; без изменений.

## `.usages/` Update

### Cell: `xs`

#### Existing Files — Consistency
- **`xs-parsing.md`** → `xs/.usages/xs-parsing.md`
  - Status: updated (в этой задаче)
  - Additions: precondition о for-init локали (раздел VisibleAt).
  - Updates needed: нет.

### Cell: `rms`, `analysis`, `complete`

- `rms-parsing.md`, `includes.md`, `checks.md`,
  `value-and-type-checks.md`, `completing.md` — актуальны, правок не
  требуется (диагностические коды не меняются; «Parse never fails»
  становится ещё вернее).

## Test Stack Trace

### General Setup

- Чистые модульные тесты в пакетах `rms`, `xs`, `analysis`,
  `complete` (pattern проекта: table-driven, testify).
- Позиции в тестах — 0-based `common.Pos{Line, Column}`.
- Регрессионная база: репро из
  `docs/reviews/2026-09-08-full-review.md`.

### Source File Registry

- `rms/parse.go` (endXsBlock, closeScopes, +lineStartPos) →
  `rms/parse_test.go`
- `xs/parse.go` (scanString, parseTypedDecl, parseFor) →
  `xs/parse_test.go`, `xs/navigation_test.go`
- `analysis/analyzer.go` (без правки — тесты-регрессии) →
  `analysis/analyzer_test.go`
- `complete/completer.go` (без правки) → `complete/completer_test.go`

---

### Positive Tests

#### `TestParse_UnclosedXsBlockAtEOFWithoutNewline` (rms, №1)

**Setup**: исходник без финального `\n`.

**Input**: `rms.Parse("#includeXS\nvoid main() { int x = 1; }", "t.rms")`

**Trace**:
```
Parse → lines=[#includeXS, void main…] → directive(#includeXS): inXs, xsStart=1
  → closeAll → endXsBlock(2) → lineStartPos(2)=EOF-ветка {1, 25, 37}
  → XsBlock{Code:"void main() { int x = 1; }", Range:{1:0..1:25}}
```

**Assertions**:
```
require.NotPanics; len(file.XsBlocks) == 1
XsBlocks[0].Code == "void main() { int x = 1; }"
XsBlocks[0].Range.Start == {Line:1, Column:0}
XsBlocks[0].Range.End == {Line:1, Column:25}
len(diags) == 0
```

**Sufficiency**: репро №1 — паника всего LSP-сервера на didOpen;
контракт «не паниковать».

#### `TestXsParse_TopLevelMultiDecl` (xs, №4)

**Input**: `xs.XsParse("int a = 1, b = 2;", "t.xs")`

**Trace**: `parseTypedDecl → цикл деклараторов → Decl a, Decl b →
record обоих имён`.

**Assertions**:
```
len(file.Decls) == 2; Decls[0].Name=="a", Decls[1].Name=="b"
оба Kind==variable, Type=="int"
Decls[1].Range.Start == позиция токена "b" (Column 10)
нет синтаксических diags
```

**Sufficiency**: репро №4 — потеря деклараций из навигации/completion.

#### `TestVisibleAt_ForInitVisible` (xs, №2)

**Input**: `void main() { for (int i = 0; i < 10; i++) { int j = i + 1; } }`;
pos в теле цикла (внутри `{ int j`).

**Trace**: `parseFor → Body=[StmtDecl i=0, block] → appendLocals →
локали i, j с blockEnd=for.Range.End`.

**Assertions**:
```
visible содержит {Name:"i", Kind:"local"} и {Name:"j", Kind:"local"}
found == true
```

**Sufficiency**: репро №2 — completion не предлагал переменную цикла.

#### `TestAnalyzeXs_ForLoopVarNoUndefined` (analysis, №2)

**Input**: репро ревью: `void main() { for (int i = 0; i < 10; i++)
{ int j = i + 1; } }`, `AnalyzeXs(file, nil)`.

**Trace**: `collectLocals(decl.Body) → StmtDecl(init) в for-Body →
declared["i"]=true → checkIdent(i) в declared → нет diags`.

**Assertions**:
```
0 диагностик с code="undefined-symbol" (фактически diags пуст)
```

**Sufficiency**: репро №2 — 4 ложных `undefined symbol "i"` на каждый
цикл; главный источник диагностического шума.

#### `TestXsAt_ForInitCandidate` (complete, №2)

**Input**: XsAt(file, pos в теле for, nil).

**Assertions**:
```
кандидат {Label:"i", Kind:"local"} присутствует; Sort.startsWith("0")
```

**Sufficiency**: сквозная регрессия: parse → VisibleAt → Candidate.

#### `TestParse_EndRandomClosesUnterminatedIf` (rms, №5)

**Input**: `start_random\nif 1\npercent_chance 50\ncreate_terrain
GRASS\nend_random`.

**Assertions**:
```
ровно 1 warning: Message `"end_random" closes an unterminated "if"
block"`, Severity=warning, Code="syntax"
```

**Sufficiency**: репро №5 — мёртвый warning, молчаливое закрытие.

#### `TestXsParse_StringEscapeNewlineTracksLines` (xs, №3)

**Input**: `"string s = \"abc\\\nDEF\";\nint z = 1;"` (escape + `\n`
внутри литерала).

**Assertions**:
```
Decl "z".Range.Start.Line == 2
Decl "s".Range.Start.Line == 0
```

**Sufficiency**: репро №3 — сдвиг всех позиций после литерала.

---

### Negative Tests

#### `TestParse_EndRandomWithoutMatch` (rms, guard)

**Input**: `end_random` без открытия → существующий error
`"end_random" without a matching opening block` сохраняется
(severity=error); фикc не превращает его в warning.

#### `TestXsParse_ForAssignFormNoPhantomLocal` (xs, guard №2)

**Input**: `int i;\nvoid main() { for (i = 0; i < 10; i++) {} }`, pos
в теле.

**Assertions**: `i` НЕ kind=local (только топ-левел variable);
assign-форма не декларирует.

**Sufficiency**: защита от ложных деклараций assign-формы — регресс
различения decl/assign.

---

### Edge Case Tests

#### `TestParse_BareIncludeXSAtEOF` (rms, №1-вектор B)

**Input**: `rms.Parse("#includeXS", "t.rms")` → 1 XsBlock, Code "",
Range нулевой ширины в EOF; NotPanics.

#### `TestParse_ClosedNestNoWarning` (rms, №5-guard)

**Input**: полный `start_random … end_random` с парным `if/endif`
внутри → 0 warning-ов о незакрытых блоках.

#### `TestParse_PercentChanceImplicitNoWarn` (rms, №5-guard)

**Input**: `start_random\npercent_chance 50\nend_random` →
percent_chance закрывается неявно БЕЗ warning (документировано).

#### `TestXsParse_MultiVarForInit` (xs, №2-edge)

**Input**: `for (int i = 0, j = 5; …)` → обе локали в VisibleAt.

#### `TestVisibleAt_ForInitNotVisibleAfter` (xs, №2-edge)

**Input**: pos после цикла (в конце тела функции) → локаль `i`
отсутствует; топ-левел символы остаются.

#### `TestDefinition_ForInit` (xs, №2)

**Input**: Definition(pos `i` в условии) → name-range `i` в init;
Definition(pos `i` после цикла) → found=false (нет топ-левел `i`).

#### `TestAnalyzeXs_ForLoopVarUsedAfterNotFlagged` (analysis, консервативность)

**Input**: `void main() { for (int i = 0; i < 3; i++) {} i = 5; }` →
0 undefined-symbol (консервативная семантика, решение пользователя).

#### `TestXsParse_TopLevelSingleDeclUnchanged` (xs, guard №4)

**Input**: `int a = 1;` → 1 Decl, Range от type-word start —
байт-идентично текущему поведению (SC: не менять существующие
ожидания).

#### `TestXsParse_ForBodyWithoutBraces` (xs, №2-edge)

**Input**: `for (int i = 0; i < 3; i++) x = i;` → Body=[init,
exprStmt]; `i` видима.

## Additional Instructions for the Implementation Agent

- Подзадачи и порядок: (1) rms №1+№5; (2) xs №3+№4; (3) xs №2 +
  регрессии analysis/complete. Каждая — отдельная ветка task/*, PR в
  master; №2 последней (зависит от представления parseFor).
- Существующие тесты, ожидающие `StmtFor.Exprs == [init, cond, step]`,
  обновляются легитимно (repr-change зафиксирован дизайном); остальные
  ожидания не трогать (SC-правило из task.md).
- Тесты — только под memory cap:
  `timeout 300 systemd-run --user --scope -p MemoryMax=1500M
  -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`; финально
  `goimports -w .`, `golangci-lint run`, `goga lint`,
  `goga contract rms xs analysis complete`.
- Контракты уже материализованы и валидны (goga lint 0, contract OK) —
  CODEMANIFEST в кодовых задачах не трогать.
- Комментарии в коде — плотность и стиль окружения (короткие
  пояснения неочевидных решений: второй panic-вектор, dead-condition
  история closeScopes, repr-выбор parseFor).
