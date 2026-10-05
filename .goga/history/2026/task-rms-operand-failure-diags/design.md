# Design Document: `task-rms-operand-failure-diags`

Задача: диагностика провалов парса операндов в `internal/rms` (issue #96).
Источник — task.md топика; контрактная правка применена и провалидирована
(`goga lint` 0 ошибок, `goga contract internal/rms` exit 0).

## Contract Changes

### Changed CODEMANIFEST Files
- `internal/rms/CODEMANIFEST`: тип `Parse(...)` — шаг 6 алгоритма дополнен
  severity-политикой (провал операнда/токена выражения — error; незакрытая
  «(» вызова при EOF — warning); в Requirements добавлен аудит-критерий
  «каждый потреблённый токен либо представлен в AST, либо покрыт
  Diagnostic». Формулировки — см. diff в git истории ветки.

### New Entities
- нет. Публичная поверхность `internal/rms` не меняется: `Parse(source,
  name) -> (RmsFile, []Diagnostic)` остаётся единственной точкой входа,
  сигнатуры экспортированных типов не трогаются.

### Changed Entities
- `Parse` — только поведенчески: на шести путях молчаливой потери байтов
  появляются Diagnostics (таблица эмиттеров ниже). Формы AST не меняются:
  childless unary (№2) и zero-width inner Expr (№3) остаются
  recovery-формами; их потребление форматтером отсекается отказом по
  error-severity.

### Deleted Entities
- нет.

### Usages and Annotations Changes
- нет: `conventions` и `rms_grammar` не меняются; импортируемые
  `positions-and-diagnostics`/`symbols` используются как прежде.

## Applied Fixes

### Fixed CODEMANIFEST Defects
- не обнаружено. Аудит консистентности (4 измерения) и трассировка дефектов
  контракта не вскрыли; единственная правка — плановая (выше).

## Entity Interaction and Data Flow

### Interaction Diagram

```
format.RMS ──► rms.Parse ──► parser.run ──► expressions (драйвер токен-рана)
                                              │ reportf (sink: *parser)
                                              ▼
                                          parseExpr ◄──┐ рекурсия
                                              │        │ (unary/paren/binary)
                                        parsePrimary ──┤
                                              │        │
                                          parseCall ───┘
                                              
потребители diags: format (refuse по error), server (публикация),
analysis/hints/complete/include (без правок)
```

Sink: `reportf` (parse.go:778) — единственный канал эмиттера, все новые
диагностики идут через него с `code: "syntax"`.

### Data Flows

**Сценарий A (воспроизводитель #96)**: `end_random. ('150% (` →
`expressions` → `parseExpr` → `parsePrimary`: tokLParen-ветка, inner-парс
проваливается → NEW: error-диагностика группы → AST содержит zero-width
Expr + error diag → `format.RMS` отказывает (`refuse`, ErrParseErrors),
INV1 не нарушается (форматтер не печатает).

**Сценарий B (тихий break сегодня)**: `create_land , 5` → `parseExpr`
проваливается на `,` → сегодня: break, `5` теряется молча → NEW:
error `unexpected ","`, `,` уже потреблён parsePrimary, цикл продолжается →
`5` парсится, арность сохранена (Args=[5]).

### Entity Dependencies

`expressions`/`parseExpr`/`parsePrimary`/`parseCall` — внутренние функции
parse.go, получают `*parser` первым аргументом (см. Algorithm Design);
инициализация — только из `Parse` → `parser.run`. Внешних зависимостей нет.

## Code Stack Trace

### Trace: `Parse(source, name) -> (RmsFile, []Diagnostic)`

#### Chain
1. **Input**: текст .rms-файла + имя; вызывается форматтером (rms.go:35),
   сервером, ячейками analysis/include.
2. **Step**: `parser.run` — построчный проход, лексер, секции/statements →
   checkpoint: неизменно, не трогается.
3. **Step**: `expressions(toks)` — драйвер выраженческого контура →
   checkpoint: сегодня `break` при `!ok` — молчаливая потеря (№6); дизайн
   заменяет на report+continue.
4. **Step**: `parseExpr` (precedence climbing, parse.go:1410) → binary
   цикл потребляет оператор, rhs `!ok` → checkpoint: сегодня оператор
   исчезает молча (№1); дизайн: error со span-диапазоном.
5. **Step**: `parsePrimary` (parse.go:1440) — токен потребляется первым
   делом (`r.next()`, parse.go:1441); провал = потреблённый токен без
   AST-представления → checkpoint: вызывающий обязан получить токен назад
   для репорта (сигнатура +token).
6. **Step**: `parseCall` (parse.go:1495) — skip-ветка (№4) и выход без
   `)` (№5) → checkpoint: сегодня skip съедает ЛИШНИЙ токен (провалившийся
   уже потреблён parsePrimary, parse.go:1441, а skip делает второй
   `r.next()`) — латентный off-by-one, дизайн устраняет.
7. **Output**: (File, diags); diags стабиль-сортируются по позиции
   (parse.go:19-24) — новые эмиттеры участвуют автоматически → checkpoint
   passed.

#### Checkpoint Summary
- сортировка diags: passed (существующий SortStableFunc покрывает новые).
- «потреблённый токен всегда доступен репортящему»: passed через
  сигнатуру `(Expr, token, bool)` у `parsePrimary`/`parseExpr`.
- форматтерский контракт: passed — отказ приходит через существующий
  `refuse` (rms.go:66), правок format нет.
- consumers ArgAt/EnclosingRanges на zero-width Expr: passed-с-оговоркой —
  поведение как сегодня, но теперь помечено error-диагностикой (формат
  отказывает, LSP-навигация деградирует только на уже сломанном входе).

## Algorithm Design

### `Parse`
**Responsibility**: без изменений (контракт CODEMANIFEST).

### `expressions` (parse.go:1311)
**Responsibility**: драйвер токен-рана → список Expr; репорт токенов,
не начинающих выражение.

**Algorithm:**
```
1. tokUnknown-ветка — как сегодня: report error `unexpected %q`,
   consume, continue
2. parseExpr(lex, 1) → (e, bad, ok)
   IF !ok:
     - reportf(bad.at, error, "syntax", `unexpected %q`, bad.text)   // №6
     - bad уже потреблён parsePrimary — НЕ потреблять повторно
     - continue цикла (вместо break)
3. IF lex.pos == before { break } — прогресс-гард остаётся
4. хвост цикла (comma-скип, recordValueWords/recordExprWords) — без
   изменений
```

**Errors:** каждый невалидный старт-токен — один error; потерянных байтов
нет (все токен-раны либо в AST, либо под diag).

**Edge Cases:** `, 5` в начале рана → error на `,`, затем 5 парсится
(арность сохранена); EOF-токен один — синтетический за концом слайса:
`next()`/`peek()` выдают его с нулевым диапазоном и не продвигают
позицию; гард `pos == before` защищает от зацикливания на нём.

### `parseExpr` (parse.go:1410)
**Responsibility**: precedence climbing; репорт потерянного оператора.

**Algorithm:**
```
сигнатура: parseExpr(p *parser, r *tokenReader, minPrec int)
           -> (Expr, token, bool)          // token — сбойный, для репорта

1. lhs, bad, ok := parsePrimary(p, r); IF !ok → return Expr{}, bad, false
2. цикл операторов:
   prec == 0 || prec < minPrec → return lhs, token{}, true
   r.next()  // оператор потреблён
   rhs, bad, ok := parseExpr(r, prec+1)
   IF !ok:
     - span := Range{Start: op.at.Start, End: bad.at.End}  // op+сбойный
       (bad на EOF — синтетический, без диапазона: End деградирует
       до consumedEnd(), конца последнего потреблённого токена)
     - reportf(span, error, "syntax", `missing operand after %q`, op.text)  // №1
     - return lhs, token{}, true       // сбойный потреблён, покрыт span
   ...
```

**Errors:** №1 — error, span покрывает и оператор, и сбойный токен (оба
потреблены, оба не представлены в AST — lhs возвращается).

**Edge Cases:** `1 + )` на верхнем уровне → 1 диагностика: span №1 `+`…`)`
покрывает и оператор, и потреблённый `)`; возврат ok=true — expressions
второго репорта не видит, а второй якорь на том же `)` запрещён правилом
якорей; тест T9 фиксирует счётчик.

### `parsePrimary` (parse.go:1440)
**Responsibility**: первичное выражение; возвращает сбойный токен наверх,
сам не репортит (репорт — у обрабатывающего контекста).

**Algorithm:**
```
сигнатура: parsePrimary(p *parser, r *tokenReader) (Expr, token, bool)

1. tok := r.next()  — как сегодня (потребление до классификации)
2. tokNumber/tokString/tokWord — без изменений, ok=true, token{}
3. tokOp `-`/`+`:
   operand, bad, ok := parseExpr(r, 3)
   IF !ok:
     - reportf(bad.at, error, "syntax", `missing operand after unary %q`,
       tok.text)                                   // №2, якорь — сбойный
     - return childless unary (Range = op-токен), token{}, true
4. tokOp прочие / default: return Expr{}, tok, false  // без репорта
5. tokLParen:
   open := токен `(` (уже в tok)
   reported := len(p.diags)
   inner, bad, ok := parseExpr(r, 1)
   IF !ok:
     - span := Range{Start: tok.at.Start, End: peek().at.End};
       peek на EOF (синтетический, без диапазона) — End деградирует
       до consumedEnd(), конца последнего потреблённого токена
     - reportf(span, error, "syntax", `"(" without a valid expression`)  // №3
     - return inner, token{}, true   // zero-width inner — recovery-форма;
                                     // ошибка видна через diag
   IF r.peek().kind == tokRParen { r.next() }   // ДО группового репорта:
                                                 // его спан покрывает и `)`
   IF len(p.diags) > reported:
     - span := Range{Start: tok.at.Start, End: r.consumedEnd()}
     - reportf(span, error, "syntax", `"(" without a valid expression`)  // №3
   ELSE IF `)` не потреблена (EOF/другой токен):
     - reportf(tok.at, warning, "syntax", `unclosed "("`)   // ветка №3:
       чистый inner без закрывающей — потреблённая `(` не представлена
       в AST (Range inner начинается с внутреннего токена) и не покрыта
       диагностикой; warning, не error — конструкция полная в AST, лишь
       незавершённая (консистентно с №5); аудит финального ревью R2
   return inner, token{}, true
```

**Errors:** №2 error (якорь = сбойный токен; op-токен представлен Range
childless unary — печатается харденингом #45); №3 — три ветки:
(а) error — провал внутреннего парса (!ok): span `(`…peek (на EOF —
consumedEnd: синтетический EOF диапазона не имеет); (б) error —
внутренний recovery с диагностиками
(ok=true, но p.diags вырос) — групповой репорт: внутренний Expr —
recovery-форма, чей Range не покрывает внешнюю `(`; (в) warning — чистый
inner без закрывающей `)` (аудит финального ревью R2, 2026-10-06):
`unclosed "("` на токене `(` — severity по политике «незавершённая, но
полная в AST конструкция — warning», консистентно с №5.
Инвариант спана: групповой репорт покрывает всё,
потреблённое группой, включая её закрывающую `)`, когда группа её
потребила; на пути без закрывающей — до consumedEnd (конец последнего
потреблённого токена).

### `parseCall` (parse.go:1495)
**Responsibility**: `name(arg, ...)`; репорт мусора в аргументах и
незакрытой скобки.

**Algorithm:**
```
сигнатура: parseCall(p *parser, r *tokenReader, name token, kind string)

1. open := r.next()  //consume "(" — ТОКЕН ЗАПОМИНАЕТСЯ (для №5)
2. цикл:
   peek == tokRParen → consume, closed=true, return call,true
   arg, bad, ok := parseExpr(r, 1)
   IF !ok:
     IF bad.kind == tokEOF:
       - reportf(open.at, warning, "syntax", `unclosed "(" in call to %q`,
         name.text)                                // №5
       - return call,true
     - reportf(bad.at, error, "syntax", `unexpected %q in call arguments`,
       bad.text)                                   // №4
     - continue   // БЕЗ повторного r.next(): bad уже потреблён parsePrimary
                  // (устраняет латентный off-by-one скипа)
   ...append/comma — без изменений
3. выход по EOF в peek: peek == tokEOF → №5-warning, return call,true
   (сегодня оба EOF-выхода молчат — оба проходят через №5)
```

**Errors:** №4 error (якорь = сбойный токен, он же потреблён); №5 warning
(якорь = `(`; все байты вызова в AST — только сигнализация незавершённости,
форматтер НЕ отказывает).

**Edge Cases:** `f(,)` — сегодня: `,` проваливает parseExpr, skip съедает
ещё и `)`, вызов «закрывается» EOF-путём без диагностик; после: error на
`,`, `)` закрывает вызов штатно (1 child потерянных — нет). `f()` — валид,
ноль детей, без diag.

### `reportf` (parse.go:778)
**Responsibility**: без изменений — единственный sink; новые вызовы
передают `p *parser` (свободные функции получают его первым аргументом).

**Проверка поверхности:** тесты `internal/rms` не зовут `parseExpr`/
`parsePrimary`/`parseCall` напрямую (grep по *_test.go пуст) — смена
сигнатур внутренняя для parse.go; 7 мест вызова (1325, 1411, 1425, 1460,
1469, 1481, 1507).

## Cross-cutting Concerns

- **Error handling**: парсер никогда не падает и не возвращает err —
  неизменно; новые diagnostics — тот же канал, что существующие
  (`code: "syntax"`, severity по политике). Форматтер: отказ через
  существующий `refuse`/`ErrParseErrors` — код format не меняется.
- **Logging**: нет (библиотечная ячейка, как сегодня).
- **Validation**: аудит-критерий зафиксирован в Requirements `Parse`
  (CODEMANIFEST, применено).
- **Caching**: нет.
- **Concurrency**: `Parse` — чистая функция; sink передаётся явно, общего
  мутируемого состояния нет; race-free по построению (CI гоняет `-race`).
- **Determinism**: эмиттеры в порядке обхода; финальная стабильная
  сортировка по позиции существует (parse.go:19-24).

## Usages Analysis

### `conventions`
- **What it provides**: правила кода и тестирования (table-driven, testify,
  doc-комментарии).
- **Where used**: все изменения parse.go + новые тесты.
- **Why chosen**: базовая practice ячейки.
- **How exactly**: короткие функции/early returns; тесты same-package,
  один assert на кейс.

### `rms_grammar`
- **What it provides**: структура языка (выражения DE: числа/проценты/
  константы/binary `+ - * /`).
- **Where used**: `parseExpr`/`parsePrimary`/`parseCall`.
- **Why chosen**: справочник грамматики; изменения НЕ трогают грамматику —
  только диагностику её нарушений.
- **How exactly**: классификация токенов соответствует секции Expressions.

### Imported Usages
- `positions-and-diagnostics` из `internal/common` — форма Diagnostic,
  severity 1–4, требование Range.End >= Start (все span/zero-width
  диапазоны соблюдают), стабильный code для ассертов.
  - Path: `internal/common/.usages/positions-and-diagnostics.md`
- `symbols` из `internal/common` — outline; не затрагивается (Symbols
  строится по statements, zero-width Expr не создаёт узлов).
  - Path: `internal/common/.usages/symbols.md`

## `.usages/` Update

### Cell: `internal/rms`

#### Existing Files — Consistency
- **rms-parsing** → `internal/rms/.usages/rms-parsing.md`
  - Status: current
  - Additions needed: нет («Parse never fails: a partial File comes back
    alongside syntax diagnostics» остаётся точным; потребители уже
    документировано мержат diags)
  - Updates needed: нет

#### New Files (if any)
- нет: потребительский API не изменился, новой функциональной домены нет.

## Test Stack Trace

### General Setup
- Тесты same-package (`internal/rms`, `internal/format`), testify
  require/assert, table-driven; `Parse(source, "test.rms")` без файловой
  системы; прогон — под memory cap (systemd-run, CLAUDE.md).

### Source File Registry
- `internal/rms/parse_test.go` — новые кейсы T2–T7, T9 (таблица
  `TestParse_OperandFailureDiagnostics` + точечные)
- `internal/rms/fuzz_test.go` — seed-корпус: 6 патологических входов
- `internal/format/rms_test.go` — T1 (новый кейс в `TestRMS_RefusalOnError`
  или рядом)
- `internal/format/invariants_test.go` — корпусный регресс
  (`TestRMS_IdempotentCorpus`, `TestRMS_StructureInvariant`) — без правок,
  должен остаться зелёным

---

### Negative Tests

#### `TestParse_OperandFailureDiagnostics/…` (таблица, T2–T7)

**Setup**: без фикстур; вход — строка, имя "test.rms".

**Input / Assertions** (по путям):

| Кейс | Input | Ожидаемые diags (severity, сообщение, якорь) | AST-проверка |
|------|-------|----------------------------------------------|--------------|
| T2 №1 | `create_land 1 +` | 1 error `missing operand after "+"`, span от `+` до EOF | Args=[1] |
| T3 №2 | `create_land -` | 1 error `missing operand after unary "-"` на `-` | Args=[childless unary], печать не паникует |
| T4 №3 | `create_land ( )` | 1 error `"(" without a valid expression`, span `(`…`)` | zero-width Expr в Args |
| T5 №4 | `create_land f(1, , 2)` | 1 error `unexpected "," in call arguments` на второй `,`; вызов закрыт `)` штатно | Children=[1,2] |
| T6 №5 | `create_land f(1, 2` (EOF) | 1 warning `unclosed "(" in call to "f"` на `(` | Children=[1,2] |
| T7 №6 | `create_land , 5` | 1 error `unexpected ","` на `,` | **Args=[5] — арность сохранена** |

**Trace** (на примере T7):
```
Parse → run → expressions
  → parseExpr → parsePrimary: next=`,` → default → (Expr{}, `,`, false)
  → expressions: reportf(`,`, error, `unexpected ","`)   // №6
  → continue → parseExpr → parsePrimary: `5` → Expr{number}
→ assert: len(diags)==1, Args==[Expr{Kind:number,Value:"5"}]
```

**Sufficiency**: по кейсу на каждый путь инвентаря — регресс «молчаливой
потери»; T7 дополнительно фиксирует главный выигрыш №6 (арность не плавит
analysis/арность-проверки).

**Единый assert-стиль**: один `require` на кейс — слайс
`(severity, code, message, range)` диагностик + арность AST.

#### `TestRMS_RefusesReproducer96` (T1, format)

**Setup**: константа-строка reproducer.

**Input**: `end_random. ('150% (` → `format.RMS(src, Options{})`.

**Trace**:
```
format.RMS → rms.Parse → expressions → parsePrimary(tokLParen)
  → inner fail → reportf(error, `"(" without a valid expression`)  // №3
  → refuse(diags) → ErrParseErrors
```

**Assertions**: `err != nil`, `errors.Is(err, ErrParseErrors)`,
`formatted == ""`; доп. кейс: `rms.Parse(src)` содержит ≥1 error-severity.

**Sufficiency**: закрывает сценарий issue #96 end-to-end: форматтер
отказывается там, где сегодня молча печатает потерю.

### Edge Case Tests

#### `TestParse_OperandFailure_NoDuplicateAnchors` (T9)

**Setup/Input**: `create_land , 1 + )` (два сбоя на разных токенах).

**Trace**: ведущая `,` — №6 от expressions; далее binary-loop: rhs fail на
`)` → span-диагностика `+`…`)`, возврат lhs с ok=true — потреблённый `)`
покрыт span, expressions его повторно не репортит.

**Assertions**: ровно 2 diagnostics; якоря на разных токенах (`,` от №6 и
span.Start №1 на `+`); порядок по позиции (существующая сортировка).
Одиночный `create_land 1 + )` — ровно 1 диагностика: span покрывает
потреблённый `)` (кейс в таблице `TestParse_OperandFailureDiagnostics`).

**Sufficiency**: правило «один якорь на токен» — защита от решенного
в-design риска дублирования при каскаде ok=false.

#### `TestParse_EmptyCallIsValid` 

**Input**: `create_land f()`.

**Assertions**: 0 diagnostics; call с 0 Children, Range покрывает `f()`.

**Sufficiency**: №4/№5 не должны срабатывать на валидном пустом вызове.

#### fuzz seed corpus

**Input**: 6 патологических входов из таблицы (T2–T7) добавляются в seed
`fuzz_test.go`.

**Assertions**: `go test -run FuzzParse` зелёный — ни паник, ни зависаний.

**Sufficiency**: новые пути эмиттера исполняются на adversarial-входе.

### Positive / Regression

- корпус: `TestRMS_IdempotentCorpus` + `TestRMS_StructureInvariant` +
  golden-тесты format — зелёные БЕЗ правок ожиданий (валидные файлы не
  проходят через пути эмиттеров);
- существующие тесты rms (включая `rename_test`, `comments_*`,
  `navigation_*`) — зелёные без правок;
- `make check` + `goga contract internal/rms` + CI ветки.

## Additional Instructions for the Implementation Agent

- Сигнатуры: `parseExpr(p *parser, r *tokenReader, minPrec int) (Expr,
  token, bool)`, `parsePrimary(p *parser, r *tokenReader) (Expr, token,
  bool)`, `parseCall(p *parser, r *tokenReader, name token, kind string)`;
  обновить все 7 мест вызова; в тестах внутренности не зовутся.
- Сообщения диагностик — точные строки из таблицы тестов (тесты ассертят
  текст); `code: "syntax"` везде; стилистика %q — как в существующих
  `reportf`-вызовах.
- Ни одного изменения: публичных сигнатур, форм AST, файлов вне
  `internal/rms/parse.go`, `internal/rms/CODEMANIFEST` (уже изменён),
  тестов.
- EOF-токен один — синтетический за концом слайса: нулевой диапазон,
  `next()`/`peek()` его выдают и не продвигают позицию. Span End на EOF
  деградирует до consumedEnd() (конец последнего потреблённого токена),
  гард `pos == before` сохранить.
- Тесты — под memory cap (`systemd-run ... MemoryMax=1500M`); коммиты
  атомарные, `docs:`/`fix:` стиль; PR `Fixes #96`; ветка
  `task/rms-operand-failure-diags`.
- Гоча: lint-хук может молча блокировать коммит — проверять `git status`
  после коммита.
- После реализации: кандидат в `knowledge/` — аудит-критерий как
  stability-инвариант парсера (решить по итогам, тот же PR или следом).
