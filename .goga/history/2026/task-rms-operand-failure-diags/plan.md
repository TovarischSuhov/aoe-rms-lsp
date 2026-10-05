# Plan: `task-rms-operand-failure-diags`

## Purpose

Закрыть шесть путей молчаливой потери байтов в выраженческом контуре
парсера `internal/rms`: каждый провал парса операнда/токена получает
Diagnostic (error), незакрытая `(` вызова — warning. Публичная поверхность
`Parse(source, name) -> (RmsFile, []Diagnostic)` не меняется; формы AST не
меняются (childless unary и zero-width inner Expr остаются
recovery-формами). Форматтер `internal/format` отказывает на новых
error-диагностиках через существующий `refuse`/`ErrParseErrors` — код
format не меняется, только добавляется интеграционный тест отказа на
воспроизводителе issue #96 (`end_random. ('150% (`).

Стратегия: протяжка sink (`*parser.reportf`) и сбойного токена в
свободные функции `parseExpr`/`parsePrimary`/`parseCall` (сигнатуры
меняются, все вызовы — внутри parse.go), затем шесть эмиттеров по таблице
дизайна, затем интеграционные проверки (формат-отказ, корпус, fuzz-seeds).

## Context

### Contract Surface

**Entity: `Parse(source string, name string) -> file: RmsFile, diags: []Diagnostic`**
- Type: function (Routine), location `parse.go` (package `rms`, `internal/rms`)
- Facade obligation: экспортированная `Parse` пакета `internal/rms` — сигнатура
  неизменна
- Поведенческие требования (CODEMANIFEST, применены в коммите 9e9bf1f —
  read-only для имплементатора):
  - шаг 6 алгоритма: «провал разбора операнда/токена в выражении —
    Diagnostic severity=error; незакрытая «(» вызова при EOF —
    severity=warning»
  - Requirements: «каждый потреблённый токен либо представлен в AST, либо
    покрыт Diagnostic (аудит-критерий); потеря байтов при провале разбора
    операнда — error (форматтер отказывает), незавершённая, но полная в
    AST конструкция — warning»
- Существующие неизменные требования: File всегда не-nil; diags
  отсортированы по позиции (parse.go:19-24, SortStableFunc — новые
  эмиттеры участвуют автоматически); не паниковать на некорректном входе
- Imported dependencies: `common.Diagnostic`, `common.Range`,
  `common.SeverityError`, `common.SeverityWarning` из `internal/common`

Внутренние (не-контрактные) единицы, меняемые задачами — все в `parse.go`,
тестами напрямую не зовутся:
- `expressions(toks []token) []Expr` (parse.go:1311) — драйвер токен-рана
- `parseExpr(r *tokenReader, minPrec int) (Expr, bool)` (parse.go:1410)
- `parsePrimary(r *tokenReader) (Expr, bool)` (parse.go:1440)
- `parseCall(r *tokenReader, name token, kind string) (Expr, bool)`
  (parse.go:1495)
- `reportf(r Range, severity int, code string, format string, args ...any)`
  (parse.go:778) — единственный sink, без изменений

### Re-exports
- нет.

### Usages Context
- `conventions` (`.goga/usages/conventions.md`): правила кода и тестов —
  короткие функции, early returns, table-driven, testify require/assert,
  doc-комментарии на экспортах, `t.Parallel()` для новых тестов без
  общего состояния.
- `rms_grammar` (`.goga/usages/rms-grammar.md`): грамматика НЕ меняется —
  классификация токенов (числа/проценты/константы/binary `+ - * /`)
  соответствует секции Expressions; диагносцируются только её нарушения.

### Imported Usages
- `positions-and-diagnostics` из `internal/common`
  (`internal/common/.usages/positions-and-diagnostics.md`): форма
  Diagnostic; severity 1–4; Range.End >= Start (все span/zero-width
  диапазоны соблюдают); стабильный code для ассертов — тесты ассертят
  коды, не тексты.
- `symbols` из `internal/common`
  (`internal/common/.usages/symbols.md`): outline — не затрагивается
  (Symbols строится по statements; zero-width Expr узлов не создаёт).

### Local Usages
- `internal/rms/.usages/rms-parsing.md` — current, правок не требует
  («Parse never fails: a partial File comes back alongside syntax
  diagnostics» остаётся точным). Задач на обновление нет.

### External Dependencies
- нет новых; `testify` уже в go.mod.

## Facts

- `Parse` стабиль-сортирует diags по позиции на выходе (parse.go:19-24).
- `parsePrimary` потребляет токен ДО классификации (`tok := r.next()`,
  parse.go:1441) — провал `parseExpr` всегда означает ≥1 потреблённый
  токен, кроме EOF (`r.next()` на EOF не двигает позицию, parse.go:1400-1406).
- Тесты `internal/rms` не зовут `parseExpr`/`parsePrimary`/`parseCall`
  напрямую — смена сигнатур внутренняя для parse.go.
- 7 мест вызова: 1325 (`expressions`), 1411 (parsePrimary из parseExpr),
  1425 (binary rhs), 1460 (parseCall из parsePrimary), 1469 (unary
  operand), 1481 (paren inner), 1507 (call arg).
- Существующие `reportf`-вызовы используют `code: "syntax"`, стилистику
  `%q` для токенов; тесты ассертят `Code: "syntax"` (parse_test.go:644).
- `format.RMS` отказывает на первом error-severity diag: `refuse`
  (internal/format/rms.go:66), sentinel `ErrParseErrors` (rms.go:17),
  warnings форматируются.
- Инвариант-тесты форматтера: `TestRMS_IdempotentCorpus`
  (internal/format/invariants_test.go:20), `TestRMS_StructureInvariant`
  (internal/format/rms_test.go:316); отказ-тест `TestRMS_RefusalOnError`
  (rms_test.go:220).
- `fuzz_test.go` существует в `internal/rms`; seed-корпус пополняется.
- Тесты гонять ТОЛЬКО под memory cap (systemd-run, CLAUDE.md).
- Латентный off-by-one сегодня: `parseCall` при `!ok` делает `r.next()`
  (parse.go:1513), хотя провалившийся токен уже потреблён
  `parsePrimary` — skip съедает ЛИШНИЙ токен (устраняется задачей 1).

## Gap Analysis

- Missing contract behavior: 6 путей эмиттеров (№1–№6 таблицы ниже) —
  сегодня молчат.
- Missing plumbing: `*parser`-sink и сбойный токен не доходят до
  обрабатывающих контекстов (сигнатуры без них).
- Behavioral mismatches: `expressions` делает `break` при `!ok`
  (потеря остатка рана); `parseCall` off-by-one skip; оба EOF-выхода
  `parseCall` без предупреждения о незакрытой `(`.
- Existing code to reuse: `reportf`, тип `token`, лексер, существующие
  таблицы parse_test.go, форматтерский `refuse`.
- Test coverage gaps: нет тестов на 6 путей; нет формат-теста отказа на
  воспроизводителе #96; fuzz-seed не содержит патологических входов.

---

## Tasks

> **Package ordering rule**: coding tasks `internal/rms` до интеграционных
> задач. Внутри coding-задачи контрактные тесты пишутся первыми (TDD).

### Task 1: Диагностика провалов операндов в parse.go (TDD coding)

Ячейка `internal/rms`, файл `internal/rms/parse.go`. Покрывает контрактное
поведение `Parse` (шаг 6 + Requirements аудита CODEMANIFEST). Протянуть
sink и сбойный токен в свободные функции и добавить шесть эмиттеров.
`CODEMANIFEST` — read-only (правка уже в коммите 9e9bf1f): если
реализация расходится с контрактом, править реализацию, не контракт.

Новые сигнатуры (все внутренние, 7 мест вызова обновить: 1325, 1411, 1425,
1460, 1469, 1481, 1507):

```go
func parseExpr(p *parser, r *tokenReader, minPrec int) (Expr, token, bool)
func parsePrimary(p *parser, r *tokenReader) (Expr, token, bool)
func parseCall(p *parser, r *tokenReader, name token, kind string) (Expr, bool)
```

Таблица эмиттеров (из design.md, дословно; все `code: "syntax"`,
сообщения — точные строки, тесты ассертят текст):

| № | Сайт | Sev | Диапазон | Сообщение | Восстановление |
|---|------|-----|----------|-----------|----------------|
| 1 | `parseExpr`, binary-цикл, rhs `!ok` | error | span `op.at.Start` … `bad.at.End` (bad на EOF — EOF-позиция ридера) | `` `missing operand after %q` (op.text) `` | `return lhs, token{}, true`; сбойный потреблён, покрыт span |
| 2 | `parsePrimary`, tokOp `-`/`+`, operand `!ok` | error | `bad.at` | `` `missing operand after unary %q` (tok.text) `` | childless unary (Range = op-токен), `token{}, true` |
| 3 | `parsePrimary`, tokLParen, inner `!ok` | error | span `tok.at.Start` … `peek().at.End` (peek на EOF — EOF-позиция) | `` `"(" without a valid expression` `` | `return inner, token{}, true` (zero-width inner — как сегодня); `)` потребляется как прежде |
| 4 | `parseCall`, arg `!ok` и bad != tokEOF | error | `bad.at` | `` `unexpected %q in call arguments` (bad.text) `` | `continue` БЕЗ повторного `r.next()` — bad уже потреблён (устраняет off-by-one) |
| 5 | `parseCall`, любой выход без потреблённого `)`: bad == tokEOF или peek == tokEOF | warning | `open.at` (токен `(`, захвачен на входе) | `` `unclosed "(" in call to %q` (name.text) `` | `return call, true` |
| 6 | `expressions`, `parseExpr` `!ok` | error | `bad.at` | `` `unexpected %q` (bad.text) `` | continue цикла (вместо `break`); bad уже потреблён — НЕ потреблять повторно; гард `lex.pos == before` сохранить |

Правило якорей: один сбойный токен — ровно одна диагностика от ближайшего
обрабатывающего контекста; каскад `ok=false` вверх не дублирует
(передающие уровни не репортят). Допустимы 2 диагностики на конструкцию,
если якоря на РАЗНЫХ токенах (пример `1 + )`: span `+`…`)` и `)` от
expressions). `parsePrimary` сам НЕ репортит — возвращает `tok` наверх.

**Usages relevant to this task:**
- `positions-and-diagnostics`: Range.End >= Start для всех span; severity
  только 1–4; стабильный `code: "syntax"` (тесты ассертят код).
- `conventions`: короткие функции; сообщения lowercase в стиле
  существующих `reportf`-вызовов; doc-комментарии обновить у трёх функций.
- `rms_grammar`: грамматика не меняется — классификация токенов в
  `parsePrimary` (числа/проценты/константы/операторы `+ - * /`) остаётся
  по секции Expressions; диагностируются только её нарушения.
- `symbols`: outline не затрагивается — `Symbols()` строится по
  statements; zero-width Expr узлов не создаёт, проверять не нужно.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [ ] **Contract tests**: в `internal/rms/parse_test.go` новая table-driven
  `TestParse_OperandFailureDiagnostics` (t.Parallel, testify require) —
  кейсы T2–T7 с точными входами и ожиданиями (severity, code, сообщение,
  якорь-диапазон, AST-проверка): T2 №1 `create_land 1 +` → 1 error
  `missing operand after "+"`, Args=[1]; T3 №2 `create_land -` → 1 error
  `missing operand after unary "-"` на `-`, Args=[childless unary];
  T4 №3 `create_land ( )` → 1 error `"(" without a valid expression`,
  span `(`…`)`; T5 №4 `create_land f(1, , 2)` → 1 error
  `unexpected "," in call arguments` на второй `,`, Children=[1,2]; T6 №5
  `create_land f(1, 2` → 1 warning `unclosed "(" in call to "f"` на `(`,
  Children=[1,2]; T7 №6 `create_land , 5` → 1 error `unexpected ","` на
  `,`, **Args=[5] — арность сохранена** (expected fail на этом шаге)
- [ ] **Code**: поменять сигнатуры трёх функций (`*parser` первым
  аргументом; `parseExpr`/`parsePrimary` возвращают `(Expr, token, bool)`),
  обновить все 7 мест вызова; `parseCall` захватывает токен `(` при входе
  (`open := r.next()`)
- [ ] **Code**: №6 — в `expressions` при `!ok`: reportf + continue вместо
  break (bad уже потреблён; гард `lex.pos == before` остаётся)
- [ ] **Code**: №1 — span-диагностика в binary-цикле; №2 — репорт в
  tokOp-ветке; №3 — репорт в tokLParen-ветке со span группы; №4/№5 —
  репорты в `parseCall` (без повторного скипа; warning на обоих EOF-выходах)
- [ ] **Interface verification**: `timeout 300 systemd-run --user --scope
  -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./internal/rms/
  -run TestParse_OperandFailureDiagnostics -count=1'` — все кейсы зелёные
- [ ] **Logic tests**: в `parse_test.go`: T9
  `TestParse_OperandFailure_NoDuplicateAnchors` — `create_land 1 + )`:
  ровно 2 diagnostics, якоря на разных токенах (разные Start), порядок по
  позиции; `TestParse_EmptyCallIsValid` — `create_land f()`: 0 diagnostics,
  call с 0 Children, Range покрывает `f()`; регресс существующих таблиц
  parse_test.go (включая rename/comments/navigation) без правок ожиданий
- [ ] **Debugging**: `timeout 300 systemd-run --user --scope
  -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./internal/rms/
  -count=1'` — чинить реализацию (не тесты) до зелёного
- [ ] **Contract re-verification**: `goga contract internal/rms` — exit 0;
  публичная поверхность `Parse`/`RmsFile`/`Expr` не изменилась; формы AST
  прежние (childless unary, zero-width inner)
- [ ] **Lint**: `golangci-lint run ./internal/rms/... && golangci-lint fmt`
  — ноль замечаний

### Task 2: Интеграционные проверки — отказ форматтера, корпус, fuzz (integration tests)

Кросс-ячеечный сценарий `internal/rms` → `internal/format`: новые
error-диагностики отказывают форматтер на воспроизводителе issue #96;
корпус 78 файлов и fuzz не регрессируют. Код ячеек не меняется — только
тесты. `CODEMANIFEST` — read-only.

**Usages relevant to this task:**
- `conventions`: тесты same-package `internal/format`; sentinel-классификация
  `errors.Is`; один assert на кейс.
- `positions-and-diagnostics`: ассерты по severity/code, не по текстам.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [ ] В `internal/format/rms_test.go` добавить T1
  `TestRMS_RefusesReproducer96` (t.Parallel): вход `end_random. ('150% (`
  → `format.RMS(src, Options{})` → `require.Error`,
  `require.True(errors.Is(err, ErrParseErrors))`, `formatted == ""`;
  companion-проверка: `rms.Parse(src, "t.rms")` содержит ≥1
  error-severity diag
- [ ] Fuzz-семена: в `internal/rms/fuzz_test.go` seed-корпус пополнить
  шестью входами из таблицы Task 1 (T2–T7);
  `go test ./internal/rms/ -run FuzzParse -count=1` — без паник
- [ ] Корпусный регресс: `timeout 300 systemd-run --user --scope
  -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./internal/format/
  -count=1'` — `TestRMS_IdempotentCorpus`, `TestRMS_StructureInvariant`,
  golden-тесты зелёные БЕЗ правок ожиданий (новых knownDivergent нет)
- [ ] Полный прогон: `make check` — зелёный

---

## Validation Commands

- `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`: все тесты (только под memory cap)
- `goga contract internal/rms`: контракт фасада internal/rms (exit 0)
- `goga lint`: DSL-линтер контрактов (0 ошибок)
- `golangci-lint run && golangci-lint fmt`: линтер/формат
- `make check`: предзадачная проверка одним вызовом (fmt → build → memory-cap test → lint → goga lint)

## Completion Criteria

- [ ] Каждый из шести путей даёт Diagnostic заявленной severity (№1–4, №6 — error; №5 — warning), якорь покрывает потерянный/сломанный регион
- [ ] `end_random. ('150% (` → error-диагностика из `Parse`; `format.RMS` → err (`errors.Is(err, ErrParseErrors)`), formatted пуст
- [ ] Корпус 78 файлов: `TestRMS_IdempotentCorpus`/`TestRMS_StructureInvariant` зелёные, новых knownDivergent нет
- [ ] Чистые входы без новых диагностик: существующие тесты `internal/rms` и `internal/format` зелёные без правок ожиданий
- [ ] Публичная поверхность `internal/rms` и формы AST не изменились; `internal/format` код не менялся (только тесты)
- [ ] `CODEMANIFEST` не модифицировался имплементатором (правка 9e9bf1f — часть дизайна)
- [ ] Каждый coding-task прошёл TDD-цикл (contract tests → code → verification → logic tests → debugging → re-verification → lint)
- [ ] Все Validation Commands зелёные
- [ ] Каждая Usages-запись упомянута минимум в одной задаче (conventions, rms_grammar, positions-and-diagnostics, symbols)
