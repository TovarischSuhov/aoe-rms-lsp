# Plan: `master` — fix-review-defects (№1–№5 ревью 2026-09-08)

<!-- По design.md (be275ff) и материализованным контрактам. Источник:
docs/reviews/2026-09-08-full-review.md. -->

## Purpose

Исправить пять дефектов, найденных полным ревью 2026-09-08: паника
rms-парсера (№1), невидимость переменной for-цикла (№2), сдвиг строк
после escape в строковом литерале (№3), непарсящиеся top-level
мультидекларации XS (№4), мёртвый warning о неявном закрытии блока
(№5). Контракты уже материализованы и валидны (goga lint 0 ошибок);
план — только код и тесты. Стратегия: минимальный радиус поражения —
№2 решается одной правкой parseFor (init-декларация как первый
statement тела for), analysis/complete кода не меняют.

## Context

### Contract Surface

**Routine: `Parse(source: string, name: string) -> file: RmsFile, diags: []Diagnostic`**
- Declared `location`: `rms/parse.go`; фасад — пакет `rms`
- Изменения поведения (контракт be275ff): Algorithm шаг 5 += «…или
  конца файла» (№1); шаг 6 += «неявное закрытие незакрытого вложенного
  блока (end_* при открытых внутренних) — Diagnostic severity=warning»
  (№5)
- Constraints: не паниковать и не возвращать err для некорректного
  входа; File всегда не-nil; diags отсортированы по позиции
- Imported dependencies: `Pos`, `Range`, `Diagnostic`, `Symbol` из
  `common` (+ usages `positions-and-diagnostics`, `symbols`)

**Routine: `XsParse(source: string, name: string) -> file: XsFile, diags: []Diagnostic`**
- Declared `location`: `xs/parse.go`; фасад — пакет `xs`
- Изменения поведения: Algorithm шаг 2 += «variables (мультидекларация
  int a = 1, b = 2; — N деклараций, по одной на имя, Range каждой —
  по своей под-конструкции)» (№4); сканер корректно считает строки
  (№3 — инвариант «Every AST node carries a Range»); parseFor
  сохраняет init-декларацию (№2 — представление)
- Constraints: не паниковать; err не возвращается

**Entity: `XsFile`** (методы в `xs/ast.go`, поведение строится парсером)
- `VisibleAt(pos: Pos) -> visible: []Symbol, found: bool`: Algorithm
  шаг 3 += for-init локаль области оператора for (№2)
- `Definition(pos: Pos) -> r: Range, found: bool`: Algorithm шаг 2 +=
  «…или оператора for с его init-декларацией» (№2)

**Entity: `Analyzer`** (`analysis/analyzer.go`)
- `AnalyzeXs(file: XsFile, externals: []Decl) -> diags: []Diagnostic`:
  шаг 3 += init-декларация for в собранных локалях, консервативно.
  Кодовая правка НЕ требуется (обходы уже рекурсивны) — только тесты.

**Entity: `Completer`** (`complete/completer.go`)
- `XsAt(file: XsFile, pos: Pos, external: []Decl) -> candidates:
  []Candidate`: кодовая правка НЕ требуется — только регрессионный
  тест (for-var → kind=local через VisibleAt).

### Re-exports

Нет.

### Usages Context

- `conventions` (.goga/usages/conventions.md): Go 1.23+, testify,
  table-driven, `Test<Component>_<Scenario>`, goimports/golangci-lint
- `rms_grammar` (.goga/usages/rms-grammar.md): inline-XS после
  #includeXS; end_*-парность блоков; recovery
- `xs_grammar` (.goga/usages/xs-grammar.md): for(init;cond;step);
  строки с escape; мультидекларации переменных; recovery
- `xs_coercion` (inline, analysis): типизация — не меняется

### Imported Usages

- `xs-parsing` из `xs` (`xs/.usages/xs-parsing.md`) — семантика
  VisibleAt/Definition; обновлён for-init precondition (be275ff)
- `positions-and-diagnostics`, `symbols` из `common` — Pos/Range/
  Diagnostic/Symbol

### Local Usages

Правок .usages в задачах нет (выполнено в be275ff).

### External Dependencies

Нет новых: std + testify (go.mod).

## Facts

- Репро №1–№3 независимо верифицированы 2026-09-08 (раздел «Репро»
  ревью-документа)
- №1 имеет ДВА panic-вектора: End (`p.pos(end,0)` при
  end==len(lines)) и Start (`p.pos(p.xsStart,0)` при bare #includeXS
  в EOF → xsStart==len(lines))
- №5: условие `i < len(p.scopes)-1` в closeScopes всегда false (срез
  усечён) — warning мёртв
- №3: scanString `s.pos += 2` на escape пропускает `\n` без
  line++/lineStart; skipSpace считает строки только на `\n`
- №4: parseLocalDecl запятые уже поддерживает — parseTypedDecl нет
- №2: существующие обходы (xs collectLocals/appendLocals, analysis
  collectLocals/walkStmtsXs) рекурсивно обходят Body и StmtDecl —
  представление init как Body[0] делает их корректными без правок
- Тесты — только под memory cap (CLAUDE.md)

## Gap Analysis

- Поведенческие разрывы: №1 паника (нарушение constraint rms), №2
  4×ложных undefined-symbol + Definition/VisibleAt/completion слепы к
  for-var, №3 сдвиг Line, №4 потеря деклараций, №5 недиагностирование
- Существующий код для переиспользования: parseLocalDecl (№4 образец
  цикла деклараторов), closeAll-вычисление EOF-позиции (образец для
  lineStartPos), slices.Backward-обход closeScopes
- Тест-пробелы: 17 сценариев design.md не покрыты
- CODEMANIFEST read-only — контракты уже соответствуют (be275ff)

---

## Tasks

> Правило порядка: задачи ячейки rms завершаются до xs; №2 (Task 5)
> после №3/№4 (одно место парсера); каждая кодовая задача — TDD:
> контракт-тесты первыми.

### Task 1: rms — `Parse`: незакрытый XsBlock в EOF без паники (№1) (TDD)

`rms/parse.go`: `endXsBlock(idx)` паникует, когда блок не закрыт и
файл не заканчивается `\n`: `p.pos(end, 0)` читает `p.starts[end]` при
`end == len(p.lines)` (вектор A: `#includeXS\nvoid main() { int x =
1; }`). Второй вектор B: bare `#includeXS` последней строкой →
`p.xsStart == len(p.lines)` → паника в Start-позиции. Решение — хелпер
`lineStartPos(i int) common.Pos`: `i < len(lines)` → `p.pos(i, 0)`;
иначе EOF-ветка `Pos{last, len(lines[last]), starts[last]+len}` (по
образцу closeAll, rms/parse.go:163-164). `endXsBlock` использует
хелпер для обеих границ Range. Контракт: Parse Algorithm шаг 5 «…до
следующей директивы/секции или конца файла», constraint «не
паниковать».

**Usages relevant to this task:**
- `rms_grammar`: inline-XS после #includeXS — блок до терминатора или EOF
- `conventions`: table-driven, `Test<Component>_<Scenario>`
- `positions-and-diagnostics` из Imports: Pos{Line, Column, Offset}

**CRITICAL: `CODEMANIFEST` files — read-only. Не соответствует —
чинить реализацию, не контракт.**

- [ ] STEP 0: объявить задачу
- [ ] STEP 1 (контракт-тесты, упадут): `TestParse_UnclosedXsBlockAtEOFWithoutNewline` — `rms.Parse("#includeXS\nvoid main() { int x = 1; }", "t.rms")`: NotPanics, 1 XsBlock, Code=="void main() { int x = 1; }", Range {Start:{1,0}, End:{1,25}}, 0 diags; `TestParse_BareIncludeXSAtEOF` — `rms.Parse("#includeXS", "t.rms")`: NotPanics, 1 XsBlock, Code=="", Range нулевой ширины в EOF
- [ ] STEP 2 (код): добавить `lineStartPos` (unexported, c doc-комментарием: устойчив к i==len(lines)); в `endXsBlock` заменить `p.pos(p.xsStart, 0)` и `p.pos(end, 0)` на `p.lineStartPos(...)`
- [ ] STEP 3: `go test ./rms/ -run 'TestParse_(UnclosedXsBlockAtEOFWithoutNewline|BareIncludeXSAtEOF)' -count=1` (memory cap) — зелёные
- [ ] STEP 4 (logic-тесты): `TestParse_XsBlockTerminatedByDirective` (guard: блок, закрытый следующей директивой — Range/Code без изменений); `TestParse_XsBlockTrailingBlankLines` (guard: обрезка пустых строк сохранена — Range.End на последней непустой)
- [ ] STEP 5 (debug): `go test ./rms/ -count=1` (memory cap) — править только реализацию
- [ ] STEP 6: контракт-реверификация: Parse не паникует, File не-nil, diags сортированы; фасад `rms.Parse`/`RmsFile`/`XsBlock` не изменён
- [ ] STEP 7: `goimports -w rms/`; `golangci-lint run rms/...`
- [ ] STEP 8: отметить чекбоксы
- [ ] → REVIEW → APPROVAL → NEXT TASK

### Task 2: rms — `closeScopes`: warning о неявном закрытии (№5) (TDD)

`rms/parse.go:396-412`: условие `i < len(p.scopes)-1` всегда false
(срез `p.scopes` уже усечён до `i+1` предыдущей итерацией, строка 404)
— warning `"..." closes an unterminated "..." block` мёртв.
Репро: `start_random / if 1 / percent_chance 50 / create_terrain
GRASS / end_random` → end_random молча закрывает незакрытый if.
Решение: заменить условие на `!stops(open.name) && open.name !=
"percent_chance"` (percent_chance закрывается неявно без warning —
документировано комментарием closeScopes). Контракт: Parse Algorithm
шаг 6 «неявное закрытие незакрытого вложенного блока (end_* при
открытых внутренних) — Diagnostic severity=warning».

**Usages relevant to this task:**
- `rms_grammar`: парность start_random/end_random, if/endif
- `conventions`: именование тестов

**CRITICAL: `CODEMANIFEST` files — read-only.**

- [ ] STEP 0: объявить задачу
- [ ] STEP 1 (контракт-тесты, упадут): `TestParse_EndRandomClosesUnterminatedIf` — вход выше: ровно 1 diag, Severity==warning, Code=="syntax", Message==`"end_random" closes an unterminated "if" block`
- [ ] STEP 2 (код): в closeScopes условие → `!stops(open.name) && open.name != "percent_chance"`; комментарий об истории мёртвого условия (усечение среза) — кратко, в стиле окружения
- [ ] STEP 3: `go test ./rms/ -run TestParse_EndRandomClosesUnterminatedIf -count=1` — зелёный
- [ ] STEP 4 (logic-тесты): `TestParse_EndRandomClosedNestNoWarning` (парный if/endif внутри — 0 warning); `TestParse_PercentChanceImplicitNoWarn` (percent_chance закрыт неявно — 0 warning); `TestParse_EndRandomWithoutMatch` (guard: одиночный end_random — существующий error `"end_random" without a matching opening block` сохраняется)
- [ ] STEP 5 (debug): `go test ./rms/ ./server/ -count=1` (memory cap; server потребляет диагностики)
- [ ] STEP 6: контракт-реверификация: severity/error-маппинг диагностик не изменён
- [ ] STEP 7: `goimports -w rms/`; `golangci-lint run rms/...`
- [ ] STEP 8: отметить чекбоксы
- [ ] → REVIEW → APPROVAL → NEXT TASK

### Task 3: xs — `scanString`: escape `\`+`\n` считает строки (№3) (TDD)

`xs/parse.go:1437-1459`: escape-ветка `s.pos += 2` пропускает `\n` без
`s.line++`/`s.lineStart` — все позиции после литерала со сдвигом.
Репро: `string s = "abc\` + `\n` + `DEF";\nint z = 1;` → `z` получает
Line=1 вместо 2. Инвариант сканера: строки считаются только на `\n`
(skipSpace); `\r` — обычный пробел. Решение: в escape-ветке
`if s.src[s.pos+1] == '\n' { s.line++; s.lineStart = s.pos + 2 }`
перед `s.pos += 2`. Контракт: инвариант позиций XsParse («Every AST
node carries a Range»).

**Usages relevant to this task:**
- `xs_grammar`: строки double-quoted с escape
- `conventions`

**CRITICAL: `CODEMANIFEST` files — read-only.**

- [ ] STEP 0: объявить задачу
- [ ] STEP 1 (контракт-тесты, упадут): `TestXsParse_StringEscapeNewlineTracksLines` — вход выше: Decl "s" Line==0, Decl "z" Line==2
- [ ] STEP 2 (код): escape-ветка scanString += line-учёт (`s.line++; s.lineStart = s.pos + 2` только для `\n`)
- [ ] STEP 3: `go test ./xs/ -run TestXsParse_StringEscapeNewlineTracksLines -count=1` — зелёный
- [ ] STEP 4 (logic-тесты): `TestXsParse_StringEscapeNonNewlineUnchanged` (guard: `\n`, `\"`, `\\` — позиции и токены без изменений); `TestXsParse_StringEscapeAtEOFDoesNotPanic` (edge: `\` последний байт файла — NotPanics)
- [ ] STEP 5 (debug): `go test ./xs/ -count=1` (memory cap)
- [ ] STEP 6: контракт-реверификация: noncode-диапазоны литералов корректны (VisibleAt/CallAt Contains)
- [ ] STEP 7: `goimports -w xs/`; `golangci-lint run xs/...`
- [ ] STEP 8: отметить чекбоксы
- [ ] → REVIEW → APPROVAL → NEXT TASK

### Task 4: xs — `parseTypedDecl`: top-level мультидекларации (№4) (TDD)

`xs/parse.go:103-147`: после первого декларатора `expectSemi` встречает
`,` → 2 ошибки, декларации теряются. Репро: `int a = 1, b = 2;` →
expected `;` + unexpected "," at top level; `b` вне навигации.
Решение (образец — parseLocalDecl, xs/parse.go:701-744): цикл
деклараторов: name → record → optional `= parseExpr` (Body=[initStmt])
→ append `Decl{Kind: DeclVariable, Type: typ, Range: первый — от
start (type words), последующие — от своего имени}`; `,` → continue;
один `expectSemi` в конце. Ветка функции (`(`) — до цикла, не
трогать. Контракт: XsParse Algorithm шаг 2 «мультидекларация — N
деклараций, по одной на имя, Range каждой — по своей
под-конструкции».

**Usages relevant to this task:**
- `xs_grammar`: переменные top level
- `conventions`

**CRITICAL: `CODEMANIFEST` files — read-only.**

- [ ] STEP 0: объявить задачу
- [ ] STEP 1 (контракт-тесты, упадут): `TestXsParse_TopLevelMultiDecl` — `int a = 1, b = 2;`: 2 Decls (a, b), оба Kind==variable Type=="int", Decls[1].Range.Start == позиция токена `b`, Body[0] обоих — initStmt, 0 синтаксических diags
- [ ] STEP 2 (код): цикл деклараторов в parseTypedDecl по образцу parseLocalDecl; `p.record(name)` на каждое имя
- [ ] STEP 3: `go test ./xs/ -run TestXsParse_TopLevelMultiDecl -count=1` — зелёный
- [ ] STEP 4 (logic-тесты): `TestXsParse_TopLevelSingleDeclUnchanged` (guard: `int a = 1;` — 1 Decl, Range от type-word start, байт-идентично прежнему); `TestXsParse_MultiDeclNavigation` (Definition/VisibleAt находят обе `a` и `b`)
- [ ] STEP 5 (debug): `go test ./xs/ -count=1` (memory cap)
- [ ] STEP 6: контракт-реверификация: occurrence-индекс содержит оба имени; Symbols/outline корректны
- [ ] STEP 7: `goimports -w xs/`; `golangci-lint run xs/...`
- [ ] STEP 8: отметить чекбоксы
- [ ] → REVIEW → APPROVAL → NEXT TASK

### Task 5: xs — `parseFor`: init-декларация видима в операторе for (№2) (TDD)

`xs/parse.go:524-555`: parseFor разворачивает init-декларацию в
`stmt.Exprs` (`init.Exprs...`), маркер StmtDecl теряется → переменная
цикла нигде не видима: 4×`undefined symbol "i"` (analysis), Definition
found=false, VisibleAt/completion без `i`. Решение: НЕ разворачивать —
сохранить init как первый statement тела: `body := p.parseStmt();
stmt.Body = append([]Stmt{init}, body)` (только когда init —
декларация). Тогда существующие обходы дают контрактную семантику без
правок: xs `collectLocals`/`appendLocals` — `scopeEnd =
stmts[i].Range.End` = конец for (видимость init/cond/step/body, не
видна после); analysis `collectLocals` собирает имя в `declared`
(ложные undefined уходят; консервативно, без Push/Pop — решение
пользователя, зафиксировано в контракте analysis). Exprs у StmtFor =
только cond/step.

**Usages relevant to this task:**
- `xs_grammar`: for(init;cond;step)
- `xs-parsing` из Imports: обновлённый precondition — for-init локаль
  области оператора

**CRITICAL: `CODEMANIFEST` files — read-only.**

- [ ] STEP 0: объявить задачу
- [ ] STEP 1 (контракт-тесты, упадут): `TestVisibleAt_ForInitVisible` — `void main() { for (int i = 0; i < 10; i++) { int j = i + 1; } }`, pos в теле: visible содержит {i, local} и {j, local}, found; `TestDefinition_ForInit` — Definition(i в условии) == name-range i в init
- [ ] STEP 2 (код): parseFor — init в Body[0]; Range.End — как сейчас
- [ ] STEP 3: `go test ./xs/ -run 'Test(VisibleAt|Definition)_ForInit' -count=1` — зелёные
- [ ] STEP 4 (logic-тесты): `TestVisibleAt_ForInitNotVisibleAfter` (после цикла — локали i нет); `TestXsParse_ForAssignFormNoPhantomLocal` (guard: `for (i = 0; …)` при топ-левел i — НЕ local); `TestXsParse_MultiVarForInit` (edge: `for (int i = 0, j = 5; …)` — обе локали); `TestXsParse_ForBodyWithoutBraces` (edge: тело-выражение — Body=[init, exprStmt]); обновить существующие тесты, ожидавшие Exprs==[init, cond, step] (repr-изменение зафиксировано design)
- [ ] STEP 5 (debug): `go test ./xs/ ./analysis/ ./complete/ ./server/ -count=1` (memory cap)
- [ ] STEP 6: контракт-реверификация: VisibleAt/Definition шаги контракта (be275ff) соответствуют поведению; symbols-инвариант Selection ⊆ Range
- [ ] STEP 7: `goimports -w xs/`; `golangci-lint run xs/...`
- [ ] STEP 8: отметить чекбоксы
- [ ] → REVIEW → APPROVAL → NEXT TASK

### Task 6: analysis + complete — регрессии №2: for-var в диагностике и completion (integration)

Код analysis/complete не меняется (Task 5 даёт корректный вход).
Сквозные проверки: analyzer не выдаёт ложных undefined на for-var
(репро №2: 4×undefined → 0), консервативность (использование после
цикла не маркируется), complete предлагает for-var (kind=local) —
сквозная цепочка parse → VisibleAt → Candidate.

**Usages relevant to this task:**
- `xs-parsing` из Imports: for-init precondition
- `conventions`: integration-тесты между пакетами

**CRITICAL: `CODEMANIFEST` files — read-only.**

- [ ] `TestAnalyzeXs_ForLoopVarNoUndefined` в `analysis/analyzer_test.go`: репро ревью `void main() { for (int i = 0; i < 10; i++) { int j = i + 1; } }`, AnalyzeXs(file, nil) → 0 диагностик
- [ ] `TestAnalyzeXs_ForLoopVarUsedAfterNotFlagged` (консервативность): `… { for (int i = 0; i < 3; i++) {} i = 5; }` → 0 undefined-symbol
- [ ] `TestAnalyzeXs_MultiDeclNoUndefined` (синергия №4): `int a = 1, b = 2;` + использование обеих → 0 undefined
- [ ] `TestXsAt_ForInitCandidate` в `complete/completer_test.go`: XsAt(file, pos в теле for, nil) → кандидат {Label:"i", Kind:"local"}, Sort начинается с "0"
- [ ] Run validation: memory-cap `go test ./... -count=1` — весь модуль зелёный; `goimports -w .`; `golangci-lint run`; `goga lint`; `goga contract rms xs analysis complete` — все exit 0
- [ ] → REVIEW → APPROVAL → PLAN COMPLETE

---

## Validation Commands

- `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`: все тесты (только под memory cap — CLAUDE.md)
- `goimports -w .`: форматирование
- `golangci-lint run`: линтер (0 issues)
- `goga lint`: DSL-контракты (0 ошибок)
- `goga contract rms` / `goga contract xs` / `goga contract analysis` / `goga contract complete`: соответствие контрактам (exit 0)

---

## Completion Criteria

- [ ] Репро №1–№5 из `docs/reviews/2026-09-08-full-review.md` дают корректный результат (регрессионные тесты Tasks 1–6 зелёные)
- [ ] Контрактные шаги (be275ff) реализованы верно: EOF-блок (rms Parse шаг 5), warning неявного закрытия (шаг 6), мультидекларации (xs XsParse шаг 2), for-init видимость (VisibleAt шаг 3, Definition шаг 2, AnalyzeXs шаг 3)
- [ ] Существующие тесты не ломаются (обновлены только напрямую связанные с изменённым поведением: Exprs-for, warning-ожидания)
- [ ] Каждая кодовая задача прошла TDD-протокол (шаги 0–8)
- [ ] Все Validation Commands зелёные
- [ ] CODEMANIFEST не модифицировались (read-only)
- [ ] Задачи исполняются по одной на ветке task/* от master, атомарные коммиты, PR в master (CLAUDE.md)
