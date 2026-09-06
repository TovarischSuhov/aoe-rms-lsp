# Architecture Plan: analysis-deepening

## Topic

`analysis-deepening` — углубление семантических проверок ячейки `analysis`
(значения аргументов RMS + типы XS). План: `docs/arch/analysis-deepening.md`.
Задача-источник: `docs/tasks/analysis-deepening.md` (создана `/goga-propose`).

## Implementation Order

| # | Cell | Действие | Обоснование порядка |
|---|---|---|---|
| 1 | `analysis` | **modify** | Единственная изменяемая ячейка. Зависимости (common, kb, rms, xs) не меняются, поэтому граф свободен — правится одна ячейка. Внутри: сначала `values.go` (`CheckRmsValue`), затем `types.go` (`TypeEnv`/`InferType`/`Coerce`), затем вплетение в `analyzer.go`. |

## Artifacts

### Cell: `analysis` — MODIFY

#### CODEMANIFEST diff

Без удалений. Изменения:

- **Imports (change):** kb: добавить `CommandArg` к `Store`; xs: добавить
  `Expr AS XsExpr`, `Decl`, `Param AS XsParam` к `XsFile` (алиасы
  устраняют коллизии `Expr` и `Param` с kb). Импорт `Expr` из rms
  невозможен: линтер запрещает импорт одного имени типа из двух ячеек
  даже с алиасом — поэтому `CheckRmsValue` принимает примитивные
  `kind`/`value` вместо узла AST (фикс при материализации, `goga lint`).
- **Usages (add):** inline-ключ `xs_coercion` — таблица коерции типов XS.
- **Annotations (change):** добавить строку про `xs_coercion`.
- **Body `Analyzer` (change):** Requirements + делегирование хелперам;
  `AnalyzeRms` Algorithm — новый шаг 4 (вызов `CheckRmsValue`);
  `AnalyzeXs` Algorithm — новый шаг 1 (`TypeEnv`), уточнён шаг 3 (области
  видимости: Push/Pop, объявление параметров `XsParam` и локалов через
  Declare), новые шаги 6–7 (типы аргументов вызовов; присваивание/return →
  `bad-type`).
- **Body (add):** `CheckRmsValue` (location `values.go`), `TypeEnv`,
  `InferType`, `Coerce` (location `types.go`).
- **Footer Description (change):** дописаны «значения аргументов, типы XS».
  `CreatedAt` сохранён (04/09/26 — дата создания манифеста).

#### CODEMANIFEST — целевое содержимое (полностью)

```yaml
Imports:
  - Types:
      - Diagnostic
    From: common
  - Types:
      - Store
      - CommandArg
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
      - Expr AS XsExpr
      - Decl
      - Param AS XsParam
    Usages:
      - xs-parsing
    From: xs

Usages:
  conventions: .goga/usages/conventions.md
  rms_grammar: .goga/usages/rms-grammar.md
  xs_grammar: .goga/usages/xs-grammar.md
  xs_coercion: |
    Таблица коерции типов XS (основа Coerce и типизации операций):
    - одинаковые типы совместимы; int → float допустимо неявно (расширение)
    - float → int, bool ↔ числа, string ↔ прочее, vector ↔ скаляры —
      несовместимы
    - тип "" (не выведен / неизвестен) — не ошибка: вызывающая сторона
      пропускает проверку (консервативность против ложных срабатываний)
    - арифметика: результат — «шире» операндов (int⊕float=float, int⊕int=int);
      сравнения и логические операторы → bool; vector±vector → vector,
      vector*скаляр → vector

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `rms_grammar` and `xs_grammar` for the languages being checked.
  Use `xs_coercion` for type compatibility rules in XS checks.
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
    - проверки значений RMS делегируются `CheckRmsValue`, типизация XS —
      `TypeEnv`/`InferType`/`Coerce`
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
      4. Для каждого позиционного аргумента с имеющейся спецификацией и
         каждого атрибута вызвать `CheckRmsValue` с kind и value выражения;
         reported=true — добавить Diagnostic к результату
      5. effect_percent — code="deprecated-effect-percent", severity=warning
      6. Отсортировать diags по позиции

      Requirements:
      - не дублировать синтаксические diags парсера

      Constraints:
      - не изменять входной AST

    "AnalyzeXs(file: XsFile) -> diags: []Diagnostic": |
      Проверки XS-файла.

      `file`: AST из xs.Parse

      Algorithm:
      1. Построить `TypeEnv` из Decls файла
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
      - не дублировать синтаксические diags парсера

      Constraints:
      - не изменять входной AST

"CheckRmsValue(store: Store, spec: CommandArg, kind: string, value: string) -> diag: Diagnostic, reported: bool":
  location: values.go
  annotations: |
    Проверка значения одного аргумента/атрибута RMS против спецификации Kind.

    `store`: база знаний
    `spec`: спецификация аргумента/атрибута из kb
    `kind`: вид значения из AST (number / percent / const / ident /
    binary / unary)
    `value`: текст значения (литерал или имя)
    `diag`: диагностика (валидна при reported=true)
    `reported`: проверка дала результат

    Algorithm:
    1. spec.Kind=percent и kind — number или percent: числовое значение
       `value` вне [0, 100] — Diagnostic severity=error,
       code="bad-argument-value"
    2. spec.Kind=const и kind — const или ident: lookup Store.Constant
       по `lookups` с `value`; not found — severity=warning,
       code="unknown-constant" (в сообщении оговорка: константа скрипта
       #const — не ошибка)
    3. Прочие случаи — kind binary/unary, spec.Kind number/float/condition/
       filename/пусто — reported=false

    Requirements:
    - фактический набор spec.Kind в данных: number, const, percent, float,
      condition, filename, пусто; проверяются только percent и const
    - единая проверка для позиционных Args и Attributes

    Constraints:
    - чистая функция: без IO, не изменять spec

"TypeEnv(file: XsFile)":
  location: types.go
  annotations: |
    Таблица символов XS-файла: имя → тип, со стеком областей видимости.

    `file`: AST из xs.Parse

    Algorithm:
    1. Из Decls файла: variables → тип (`Decl`); functions и extern —
       тип возврата; rules/events — имена без типа
    2. Верхний уровень — исходная область видимости

    Requirements:
    - построение только из `file`, без других источников
  methods:
    "Push()": |
      Открыть вложенную область видимости (тело функции/блока).
    "Pop()": |
      Закрыть текущую область видимости.
    "Declare(name: string, typ: string)": |
      Объявить символ в самой внутренней области.

      `name`: имя символа; `typ`: тип ("" — неизвестен)
    "Lookup(name: string) -> typ: string, found: bool": |
      Поиск символа изнутри наружу по стеку областей.

      `name`: имя; `typ`: тип ("" — неизвестен); `found`: символ объявлен

"InferType(store: Store, env: TypeEnv, e: XsExpr) -> typ: string":
  location: types.go
  annotations: |
    Тип XS-выражения; "" — не выведен.

    `store`: база знаний; `env`: окружение символов; `e`: выражение

    Algorithm:
    1. literal — тип по лексеме (int/float/hex→int, bool, string, vector)
    2. ident — `TypeEnv` Lookup; call — Store.Function по `lookups`, иначе
       локальная функция из env
    3. доступ к компоненте vector (x/y/z) — float
    4. unary/binary — операнды рекурсивно; совместимость и результат по
       `xs_coercion`
    5. нераспознанная форма — ""

    Constraints:
    - консервативность: сомнение — вернуть "" (не порождать ложный bad-type)

"Coerce(expected: string, actual: string) -> ok: bool":
  location: types.go
  annotations: |
    Совместимость фактического типа с ожидаемым по `xs_coercion`.

    `expected`: ожидаемый тип; `actual`: фактический тип; `ok`: приведение
    допустимо

    Algorithm:
    1. Типы равны — true; actual=int и expected=float — true
    2. Прочие комбинации — false

    Requirements:
    - actual="" не передаётся: вызывающая сторона пропускает проверку

---

Author: Goga
CreatedAt: 04/09/26
Description: |
  Семантический анализ RMS/XS: unknown-символы, арность, значения
  аргументов, типы XS, устаревания.
```

#### `.usages/checks.md` — MODIFY (целевое содержимое полностью)

```md
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
xsFile, xsDiags := xs.XsParse(block.Code, "inline:"+uri)
all = append(all, analyzer.AnalyzeXs(xsFile)...)
// both sorted by position; publish as one batch
```

## Diagnostic codes

| Code | Severity | Meaning |
|---|---|---|
| unknown-command | error | RMS-команда не найдена в kb |
| unknown-section | error | секция не из справочника |
| unknown-attribute | error | атрибут не принадлежит команде |
| bad-argument | error | число/вид аргументов против спецификации |
| bad-argument-value | error | значение аргумента вне диапазона (percent 0..100) |
| unknown-constant | warning | const не резолвится в kb (возможно #const скрипта) |
| deprecated-effect-percent | warning | effect_percent устарел в пользу операторов |
| undefined-symbol | error | ident вне объявлений и kb |
| bad-arity | error | неверное число аргументов вызова |
| bad-type | error | несовместимый тип аргумента/присваивания/return |

Preconditions:
- Input must come from a successful Parse call (partial AST is fine —
  analyzer walks what exists).
- Analyzer does not re-report syntax problems.

Constraints:
- No IO, no mutation of the AST.
```

#### `.usages/value-and-type-checks.md` — CREATE (содержимое полностью)

```md
# Value & Type Helpers — granular checks from the analysis cell

Domain: single-value RMS checks and XS type inference for tooling that needs
more than the batch Analyzer pass (CLI linters, future typed features).
Target audience: implementers of CLI lint tooling and the server cell.

## Check one RMS argument/attribute value

```go
spec, found := store.Attribute("create_land", "percent")
if found {
    v := attr.Value // rms.Expr
    if diag, reported := analysis.CheckRmsValue(store, spec, v.Kind, v.Value); reported {
        // bad-argument-value (error) / unknown-constant (warning)
    }
}
```

## Infer the type of an XS expression

```go
env := analysis.NewTypeEnv(xsFile)
env.Push() // тело функции: параметры и локалы
env.Declare("count", "int")
defer env.Pop()
typ := analysis.InferType(store, env, expr) // "" — не выведен: пропустить
```

## Type compatibility

```go
analysis.Coerce("float", "int") // true: int расширяется до float
analysis.Coerce("int", "float") // false
```

Preconditions:
- Push before declaring params/locals of a function body, Pop after;
  top-level symbols are preloaded from XsFile.Decls.
- InferType "" means "unknown" — treat as no-check, never as error.

Constraints:
- All helpers are pure: no IO, no mutation of inputs.
```

## Dependency Map

```
common ──(Diagnostic)─────────────────┐
kb ──(Store, CommandArg★, lookups)────┼──> analysis ──(Analyzer, checks)──> server
rms ──(RmsFile, rms-parsing)──────────│
xs ──(XsFile, XsExpr★, XsParam★, Decl★, xs-parsing)┘

★ — новые импорты этого плана. Циклов нет; server/kb/rms/xs/common
не меняются: новые диагностики доезжают до LSP-клиента автоматически.
```

## Verification Checklist

После применения артефактов ячейки `analysis`:

- [ ] `goga lint` — без ошибок (включая новые usage-файлы)
- [ ] `goga contract analysis` — все экспортируемые имена совпадают
- [ ] `go test ./...` (в cgroup-песочнице, memory cap), `golangci-lint run`
- [ ] Тесты: позитив + негатив на каждый новый код
      (`bad-argument-value`, `unknown-constant`, `bad-type`)
- [ ] Регресс: `prelude.xs` — 0 новых диагностик; фикстуры `rms/testdata/`
      — 0 новых ложных ошибок
- [ ] Инварианты: diags отсортированы; AST не мутируется; нет дублей
      синтаксических диагностик; нет IO
- [ ] `server` не пересобирается по контракту — интеграционный тест LSP
      проходит без изменений
