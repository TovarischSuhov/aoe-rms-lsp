# Plan: `analysis-deepening`

## Purpose

Углубление ячейки `analysis`: две группы семантических проверок на данных
уже загруженных в `kb` — (1) значения аргументов/атрибутов RMS против
`kb.CommandArg.Kind`, (2) типы XS против `Param.Type`/`ReturnType` с
консервативным выводом типов. После реализации пакет `analysis` экспортирует
помимо существующего `Analyzer` четыре новые сущности: `CheckRmsValue`,
`TypeEnv` (+`NewTypeEnv`), `InferType`, `Coerce` — и выдаёт 2 новых кода
диагностик: `bad-argument-value` (error) и `bad-type` (error). Код
`unknown-constant` исключён из скоупа при исполнении (const-имена RMS не
моделируются kb — см. дизайн, Applied Fixes). Контракт уже материализован в `analysis/CODEMANIFEST`
(коммиты 55c378b, ed312bf); провал `goga contract analysis` — текущий разрыв
между контрактом и кодом. Стратегия: TDD-задачи от простых сущностей
(`Coerce`, `TypeEnv`) к сложной интеграции (`Analyzer`), затем
интеграционные регресс-тесты.

## Context

### Contract Surface

Источник: `analysis/CODEMANIFEST` (READ-ONLY для исполнителя).

**Entity: `Analyzer`** *(существующая, меняется реализация, не сигнатуры)*
- Type: class (`analysis/analyzer.go`)
- Facade obligation: импортируемость из пакета `analysis` (уже есть)
- Methods:
  - `AnalyzeRms(file: RmsFile) -> diags: []Diagnostic` — Algorithm расширен
    шагом 4: для каждого позиционного аргумента с имеющейся спецификацией и
    каждого атрибута вызвать `CheckRmsValue` с kind, value и диапазоном
    значения (значения с операндами — выражения и вызовы-хелперы —
    пропускаются); reported=true — добавить Diagnostic к результату
  - `AnalyzeXs(file: XsFile) -> diags: []Diagnostic` — Algorithm:
    1. Построить `TypeEnv` из Decls файла
    2. Собрать объявленные имена (functions, variables, params, rules)
    3. Обойти объявления: при входе в тело функции — Push, объявить
       параметры (`XsParam`) и локальные переменные через Declare; при
       выходе — Pop. ident вне объявленных и вне kb — undefined-symbol
    4. bad-arity (без изменений)
    5. Для каждого аргумента вызова известной функции: тип через
       `InferType`; несовместимость с типом параметра (`Coerce` по
       `xs_coercion`) — code="bad-type", severity=error
    6. Присваивание и return: тип выражения vs объявленный тип
       (`InferType` + `Coerce`) — code="bad-type", severity=error
    7. Отсортировать diags
- Semantic requirements: не дублировать синтаксические diags парсера; не
  изменять входной AST; stateless, без IO; проверки значений RMS
  делегируются `CheckRmsValue`, типизация XS — `TypeEnv`/`InferType`/`Coerce`
- Imported dependencies: `Diagnostic`, `Range` (common), `Store` (kb),
  `RmsFile` (rms), `XsFile`, `XsParam` (xs)

**Routine: `CheckRmsValue`**
- Type: function (`analysis/values.go`, НОВЫЙ файл)
- Signature: `CheckRmsValue(spec: CommandArg, kind: string, value: string, r: Range) -> diag: Diagnostic, reported: bool`
- Go-вид: `func CheckRmsValue(spec kb.CommandArg, kind, value string, r common.Range) (common.Diagnostic, bool)`
- Algorithm:
  1. spec.Kind=percent и kind — number или percent: числовое значение
     `value` вне [0, 100] — Diagnostic с диапазоном `r`, severity=error,
     code="bad-argument-value"
  2. Прочие случаи — reported=false
- Requirements: фактический набор spec.Kind в данных: number, const,
  percent, float, condition, filename, пусто; проверяется только percent —
  const-имена уровня RMS (terrain/effect types) базой знаний не
  моделируются; единая проверка для позиционных Args и Attributes
- Constraints: чистая функция: без IO, не изменять spec

**Entity: `TypeEnv`**
- Type: class (`analysis/types.go`, НОВЫЙ файл); конструктор
  `NewTypeEnv(file xs.XsFile) *TypeEnv`
- Signature (контракт): `TypeEnv(file: XsFile)`
- Methods: `Push()` — открыть вложенную область видимости (тело
  функции/блока); `Pop()` — закрыть текущую область; `Declare(name: string,
  typ: string)` — объявить символ в самой внутренней области (typ "" —
  неизвестен); `Lookup(name: string) -> typ: string, found: bool` — поиск
  изнутри наружу по стеку областей
- Algorithm (построение): из Decls файла: variables → тип (`Decl`);
  functions и extern — тип возврата; rules/events — имена без типа;
  верхний уровень — исходная область видимости
- Requirements: построение только из `file`, без других источников

**Routine: `InferType`**
- Type: function (`analysis/types.go`)
- Signature: `InferType(store: Store, env: TypeEnv, e: XsExpr) -> typ: string`
- Go-вид: `func InferType(store *kb.Store, env *TypeEnv, e xs.Expr) string`
- Algorithm:
  1. literal — тип по лексеме (int/float/hex→int, bool, string, vector)
  2. ident — `TypeEnv` Lookup; call — Store.Function по `lookups`, иначе
     локальная функция из env
  3. доступ к компоненте vector (x/y/z) — float
  4. unary/binary — операнды рекурсивно; совместимость и результат по
     `xs_coercion`
  5. нераспознанная форма — ""
- Constraints: консервативность: сомнение — вернуть "" (не порождать ложный
  bad-type)

**Routine: `Coerce`**
- Type: function (`analysis/types.go`)
- Signature: `Coerce(expected: string, actual: string) -> ok: bool`
- Algorithm: 1. Типы равны — true; actual=int и expected=float — true;
  2. прочие комбинации — false
- Requirements: actual="" не передаётся: вызывающая сторона пропускает
  проверку

### Re-exports

- нет

### Usages Context

- `conventions` (.goga/usages/conventions.md): Go 1.23+, goimports,
  doc-комментарии на весь экспорт, DI-конструкторы, testify/assert+require,
  cmp, table-driven, `Test<Component>_<Scenario>`, короткие функции с
  early return.
- `rms_grammar` (.goga/usages/rms-grammar.md): формы DE-выражений — числа,
  проценты (`50%`), константы, операторы; `#const NAME value`; секции.
- `xs_grammar` (.goga/usages/xs-grammar.md): типы int/float/bool/string/
  vector/void; vector-литералы `(x,y,z)`; доступ `v.x|v.y|v.z`; rules/
  events — permissive; extern-ы без тела; recovery.
- `xs_coercion` (inline в CODEMANIFEST): одинаковые типы совместимы;
  int→float неявно допустимо; float→int, bool↔числа, string↔прочее,
  vector↔скаляры — несовместимы; тип "" — не ошибка, проверка
  пропускается; арифметика — результат «шире» операндов (int⊕float=float);
  сравнения и логические операторы → bool; vector±vector → vector,
  vector*скаляр → vector.

### Imported Usages

- `lookups` from `kb` — Path: `kb/.usages/lookups.md`. Паттерны
  `store.Function/Constant/Command/Attribute` с `(T, bool)`; имена
  case-sensitive; SinceUpdate "" = «всегда существовал».
- `rms-parsing` from `rms` — Path: `rms/.usages/rms-parsing.md`. Parse
  никогда не падает (частичный AST); StatementAt на атрибуте возвращает
  команду-владельца; XsBlock.Code — позиции относительно блока.
- `xs-parsing` from `xs` — Path: `xs/.usages/xs-parsing.md`. XsParse
  принимает любой C-like вход, включая prelude.xs; SymbolAt на call
  возвращает callee.

### Local Usages

- `analysis/.usages/checks.md` — Status: обновлён при материализации
  (таблица 10 кодов с severity); задач по изменению НЕТ.
- `analysis/.usages/value-and-type-checks.md` — Status: создан при
  материализации; задач по изменению НЕТ (пример синхронизирован с
  сигнатурой `r: Range`).

### External Dependencies

- Нет новых: stdlib + существующие testify/cmp из go.mod.

## Facts

- Фактический набор `CommandArg.Kind` в `kb/data/rms-commands.json` (53
  команды): `number` (84), `const` (55), пусто (34), `percent` (21),
  `float` (3), `condition` (3), `filename` (2).
- `rms.Expr.Kind`: `number|percent|const|ident|binary|unary`; константы
  классифицируются лексером как ALL-CAPS `const`, прочие имена — `ident`
  (в т.ч. вызовы-хелперы: их аргументы лежат в `Children`).
- `xs.Stmt` НЕ хранит тип локального объявления (`parseTypeWords` в
  xs/parse.go съедает слова типа): локалы бестиповые → `Declare(name, "")`
  → проверки типов для них молчат.
- `xs.Decl.Type` заполнен для top-level variables и функций (тип возврата);
  `xs.Param.Type` заполнен для параметров.
- kb-константы — только XS (882, `c...`-префикс); const-имена уровня RMS
  (GRASS, DIRT, effect types) в kb ОТСУТСТВУЮТ — поэтому unknown-constant
  исключён из скоупа (6/6 ложных срабатываний на фикстурах). `sqrt`/`abs`
  имеют float-параметр (для bad-type-тестов); `xsSetWorldGravity` в kb НЕТ.
- В `analyzer.go` уже есть `xsBuiltins` (true/false/vector/null) —
  переиспользовать в InferType (true/false → bool, vector → vector,
  null → "").
- Существующий `checkCommand` уже запрашивает `store.Attribute(cmd.Name,
  attr.Name)` в цикле атрибутов — spec доступен без лишних lookup-ов.
- Существующие проверки undefined-symbol (плоская map declared + locals) и
  bad-arity не должны измениться (регресс-тесты охраняют).
- `prelude.xs` — 882 extern-а без тел; фикстура обязательного регресса.
- `go test` запускать ТОЛЬКО в cgroup-песочнице (runaway-тест уже валил
  машину в OOM): `timeout 300 systemd-run --user --scope -p MemoryMax=1500M
  -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`.

## Gap Analysis

- Missing contract entities: `CheckRmsValue` (values.go нет), `TypeEnv`/
  `NewTypeEnv`, `InferType`, `Coerce` (types.go нет) → `goga contract
  analysis` сейчас проваливается.
- Behavioral mismatches: `AnalyzeRms` не проверяет значения; `AnalyzeXs` не
  строит TypeEnv и не проверяет типы; коды `CodeBadArgumentValue`,
  `CodeUnknownConstant`, `CodeBadType` отсутствуют в const-блоке.
- Existing code reuse: обход `walkStmts`/`checkCommand`/`walkStmtsXs`/
  `checkExpr`/`checkCall` — расширять, не переписывать; `sortDiags`,
  `argsRange`, `xsBuiltins` — как есть.
- Test coverage gaps: нет тестов новых проверок; нет базлайна prelude.xs и
  rms-фикстур по новым кодам.

---

## Tasks

> **Package ordering rule**: все задачи — пакет `analysis`; coding-задачи
> выполняются по порядку, в каждой contract-тесты пишутся первыми (TDD).

### Task 1: `Coerce` + `TypeEnv` — базовые типовые примитивы (TDD coding)

Контракт ячейки `analysis` уже материализован и READ-ONLY. Эта задача
создаёт файл `analysis/types.go` с двумя простейшими сущностями: чистой
функцией `Coerce` (кодировка inline-usage `xs_coercion`) и Entity `TypeEnv`
(таблица символов XS со стеком областей видимости, конструктор
`NewTypeEnv`). `InferType` будет добавлен в types.go следующей задачей —
эту задачу он не касается.

**Usages relevant to this task:**
- `conventions`: doc-комментарии на экспорт; testify/require; table-driven;
  `Test<Component>_<Scenario>`; конструктор `NewTypeEnv` (DI-стиль).
- `xs_coercion`: равные типы — ok; int→float — ok; float→int, bool↔числа,
  string↔прочее, vector↔скаляры — не ok.
- `xs_grammar`: типы XS; верхний уровень — variables/functions/extern/
  rules/events.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [x] **STEP 0 (DECLARATION)**: объявить задачу Task 1 — Coerce + TypeEnv
- [x] **Contract tests** (упадут — ожидаемо): в `analysis/types_test.go`:
  проверки фасада/формы — `analysis.Coerce` существует с сигнатурой
  `func(string, string) bool`; `analysis.NewTypeEnv(xs.XsFile) *TypeEnv`;
  методы `(*TypeEnv) Push()`, `Pop()`, `Declare(string, string)`,
  `Lookup(string) (string, bool)` — компиляция теста и есть контракт-тест
- [x] **Code**: создать `analysis/types.go` (package analysis) с
  doc-комментарием файла
- [x] **Code**: `Coerce` — точная кодировка Algorithm: равны → true;
  actual=="int" && expected=="float" → true; прочее → false
- [x] **Code**: `TypeEnv` — Algorithm построения из `xs.XsFile.Decls`:
  variable → Declare(name, Decl.Type); function/extern → Declare(name,
  Decl.Type) (тип возврата); rule/event → Declare(name, ""); верхний
  уровень — исходная область. Внутреннее хранилище — стек map
  `[]map[string]string` (деталь реализации, не контракт)
- [x] **Code**: методы `Push` (append пустой map), `Pop` (IF len>1 →
  усечь; на корне — no-op), `Declare` (запись в самую внутреннюю),
  `Lookup` (изнутри наружу, первый found → (typ, true); иначе ("", false))
- [x] **Interface verification**: `systemd-run --user --scope -p
  MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./analysis/...
  -count=1 -run "TestCoerce|TestTypeEnv"'` — контракт-тесты проходят
- [x] **Logic tests**: `TestCoerce_Table` — таблица пар (expected, actual):
  `(int,int)→t, (float,int)→t, (int,float)→f, (bool,int)→f, (int,bool)→f,
  (vector,vector)→t, (vector,float)→f, (string,string)→t, (string,int)→f`;
  `TestTypeEnv_ScopeShadowing` — NewTypeEnv(файл с `int x;` + функция с
  параметром `float x`): в теле f Lookup("x")=="float"; после Pop — "int";
  `TestTypeEnv_PopRootNoOp` — Pop() на корне, Lookup("x") → found=false
- [x] **Debugging**: тот же sandbox-запуск всех тестов пакета — чинить
  реализацию (НЕ тесты) до зелёного
- [x] **Contract re-verification**: фасад — `analysis.Coerce`,
  `analysis.NewTypeEnv`, `analysis.TypeEnv` импортируемы; сигнатуры
  соответствуют CODEMANIFEST (Go-вид)
- [x] **Lint**: `goimports -w . && golangci-lint run && goga lint`
- [x] **STEP 8 (COMPLETION)**: отметить чекбоксы

### Task 2: `InferType` — консервативный вывод типа XS-выражения (TDD coding)

Задача добавляет в `analysis/types.go` рутину `InferType(store, env, e)` —
единственный «мозг» типизации. Опирается на Task 1 (`TypeEnv.Lookup`,
`Coerce` — только для бинарных операций). Контракт: возвращаемая строка —
имя типа или `""` (не выведен); `""` НИКОГДА не должен превращаться в
диагностику (консервативность против ложных bad-type на prelude.xs).

**Usages relevant to this task:**
- `xs_grammar`: типы int/float/bool/string/vector; hex `0x1F`; строки в
  кавычках; vector-литерал `(x,y,z)`; доступ компоненты `v.x|v.y|v.z`;
  операторы `+ - * / % == != < <= > >= && || & | ^ << >>`, unary `! - ~ ++ --`.
- `xs_coercion`: результат бинарных операций — widening (int⊕float=float,
  int⊕int=int), сравнения/логика → bool, vector±vector → vector,
  vector*скаляр → vector; операнд "" → результат "".
- `lookups` from Imports: `store.Function(name) (Function, bool)` →
  `.ReturnType`.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [x] **STEP 0 (DECLARATION)**: объявить задачу Task 2 — InferType
- [x] **Contract tests**: `analysis.InferType(*kb.Store, *TypeEnv, xs.Expr)
  string` компилируется и вызывается (facade/shape)
- [x] **Code**: реализовать в `analysis/types.go` Algorithm:
  1. literal `e.Value`: начинается с `"` → "string";
     `strconv.ParseInt(v, 0, 64)` ok (base 0 покрывает 0x-hex) → "int";
     иначе `strconv.ParseFloat` ok → "float"; иначе ""
  2. ident `e.Value`: true/false → "bool"; vector → "vector"; null → "";
     иначе `env.Lookup` → typ (found=false → ""); kb-константы НЕ
     типизированы (у `kb.Constant` нет поля типа) → "" — переиспользовать
     `xsBuiltins` из analyzer.go
  3. call `e.Callee`: `store.Function(callee)` → `.ReturnType`; miss →
     `env.Lookup(callee)` (локальная функция); иначе ""
  4. binary op "." c `children[1].Value ∈ {x,y,z}` → "float"
  5. binary: op ∈ {==,!=,<,<=,>,>=,&&,||} → "bool"; op ∈
     {+,-,*,/,%,&,|,^,<<,>>} → рекурсивно операнды: оба "int" → "int";
     любой "float" (при известном втором) → "float"; любой "vector" при op
     ∈ {+,-,*} → "vector"; операнд "" → ""; unary: `!` → "bool"; `- ~ ++ --
     ` → тип операнда
  6. прочее (в т.ч. vector-литерал как типизация аргумента — kind vector →
     "vector") → по лексеме/детям; нераспознанное → ""
- [x] **Interface verification**: sandbox-запуск
  `go test ./analysis/... -count=1 -run "TestInferType"` — контракт-тесты
  проходят
- [x] **Logic tests**: `TestInferType_Literals` — "42"→"int"; "3.5"→
  "float"; "0x1F"→"int"; `"\"s\""`→"string"; ident true→"bool";
  vector-литерал→"vector"; `TestInferType_CallFromKb` — call
  xsGetMapSeed → "int"; доп. таблица: unknown ident → "", бинарное
  1+2.0 → "float", `1 == 2` → "bool", unary !true → "bool", доступ v.x →
  "float", пустые Children у binary → ""
- [x] **Debugging**: sandbox-запуск всех тестов пакета; чинить реализацию
  (НЕ тесты) до зелёного
- [x] **Contract re-verification**: сигнатура и консервативность (""
  вместо ошибки) соответствуют CODEMANIFEST
- [x] **Lint**: `goimports -w . && golangci-lint run && goga lint`
- [x] **STEP 8 (COMPLETION)**: отметить чекбоксы

### Task 3: `CheckRmsValue` — проверка значения RMS-аргумента (TDD coding)

Задача создаёт файл `analysis/values.go` с рутиной `CheckRmsValue(spec,
kind, value, r)` — проверка одного значения против `kb.CommandArg.Kind`
(только percent; см. Facts про const-имена RMS). Функция чистая, без
позиционной логики: Range приходит параметром `r`.

**Usages relevant to this task:**
- `rms_grammar`: percent-литерал `50%` и число `25` — обе формы допустимы
  для percent-Kind; выражения с операторами (153015+) не проверяются.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract.**

- [x] **STEP 0 (DECLARATION)**: объявить задачу Task 3 — CheckRmsValue
- [x] **Contract tests**: `analysis.CheckRmsValue(kb.CommandArg, string,
  string, common.Range) (common.Diagnostic, bool)` компилируется и
  вызывается (facade/shape)
- [x] **Code**: создать `analysis/values.go`; константу кода добавить в
  существующий const-блок `analysis/analyzer.go`:
  `CodeBadArgumentValue = "bad-argument-value"` (рядом с
  CodeUnknownCommand)
- [x] **Code**: Algorithm:
  1. `spec.Kind == "percent"` && `kind ∈ {rms.KindNumber, rms.KindPercent}`:
     `s := strings.TrimSuffix(value, "%")`; `n, err :=
     strconv.ParseFloat(s, 64)`; err → reported=false (молчание);
     `n < 0 || n > 100` → Diagnostic{Range: r, Severity:
     common.SeverityError, Code: CodeBadArgumentValue, Message про
     диапазон 0..100}
  2. прочее (spec.Kind const/number/float/condition/filename/пусто; kind
     binary/unary/const/ident) → (Diagnostic{}, false)
- [x] **Interface verification**: sandbox-запуск
  `go test ./analysis/... -count=1 -run "TestCheckRmsValue"`
- [x] **Logic tests**: позитив — `TestCheckRmsValue_PercentOutOfRange`
  (KindPercent "150" → error bad-argument-value; KindNumber "-1" → то же);
  негатив — `TestCheckRmsValue_ExpressionSkipped` (KindBinary "1 + 2" →
  false; spec{const} + KindConst "GRASS" → false; spec{number} +
  KindNumber "7" → false); edge — `TestCheckRmsValue_PercentBoundaries`
  ("0", "100", "0%", "100%" → reported false; границы включительно);
  непарсимый литерал "12x" → false
- [x] **Debugging**: sandbox-запуск всех тестов пакета; чинить реализацию
  (НЕ тесты)
- [x] **Contract re-verification**: чистая функция, без IO, spec не
  мутируется; сигнатура соответствует CODEMANIFEST
- [x] **Lint**: `goimports -w . && golangci-lint run && goga lint`
- [x] **STEP 8 (COMPLETION)**: отметить чекбоксы

### Task 4: Интеграция в `Analyzer` — новые шаги AnalyzeRms/AnalyzeXs (TDD coding)

Задача вплетает готовые примитивы (Task 1–3) в `analysis/analyzer.go`:
новый код `CodeBadType`, шаг значений в `checkCommand`, типизация в обходе
XS. Существующие пути (unknown-*, bad-argument, undefined-symbol, bad-arity,
effect_percent, `declared`-map, `collectLocals`) НЕ переписывать — только
расширять; после задачи `goga contract analysis` обязан пройти целиком.

**Usages relevant to this task:**
- `rms-parsing` from Imports: позиционная семантика — Attributes
  принадлежат команде; Args — позиционные.
- `xs-parsing` from Imports: SymbolAt на call возвращает callee; Parse
  принимает частичный AST.
- `lookups` from Imports: `store.Function(name).Params[i].Type`;
  `store.Attribute(cmd, attr)` — spec для значений атрибутов.
- `xs_coercion`: Coerce — единственная точка решения о совместимости.
- `conventions`: сортировка diags сохраняется (`sortDiags`), AST не
  мутируется.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions. Do NOT modify them. If implementation does not match the contract, fix the implementation — never fix the contract.**

- [x] **STEP 0 (DECLARATION)**: объявить задачу Task 4 — Analyzer
- [x] **Contract tests**: `goga contract analysis` — весь контракт ячейки
  (Analyzer + 4 новых типа) разрешается (это и есть фасад-тест); запуск
  фиксируется как проверка в задаче
- [x] **Code**: const-блок analyzer.go: добавить `CodeBadType =
  "bad-type"` (CodeBadArgumentValue уже из Task 3)
- [x] **Code** (AnalyzeRms, шаг 4 контракта): в `checkCommand` —
  позиционные: `for j := range stmt.Args` при `j < len(cmd.Args)`, skip
  если `len(stmt.Args[j].Children) > 0` (leaf-guard: выражения и
  вызовы-хелперы вроде rand_float не проверяются); `CheckRmsValue(a.store,
  cmd.Args[j], e.Kind, e.Value, e.Range)` → reported → append
- [x] **Code** (AnalyzeRms, шаг 4 контракта): атрибуты — в существующем
  цикле после `store.Attribute(cmd.Name, attr.Name)` сохранить spec; тот же
  leaf-guard по `attr.Value.Children`; `CheckRmsValue(a.store, spec,
  attr.Value.Kind, attr.Value.Value, attr.Value.Range)` → reported → append
- [x] **Code** (AnalyzeXs, шаги 1/3/5/6 контракта): `env := NewTypeEnv
  (file)`; существующую плоскую `declared`-map НЕ трогать; обход decls:
  для DeclFunction — `env.Push()`, `Declare(param.Name, param.Type)` для
  каждого параметра, обход тела с контекстом (env + тип возврата
  decl.Type), `env.Pop()`; в обходе тел: (a) StmtDecl → дляExprs-пар
  имя=init → `Declare(name, "")` (бестиповые локалы); (b) ExprCall с
  известной kb-функцией → для `i < min(len(children), len(params))` при
  `params[i].Type` известного вида {int,float,bool,string,vector}:
  `t := InferType(a.store, env, children[i])`; `t != "" &&
  !Coerce(params[i].Type, t)` → bad-type (Range = children[i].Range,
  severity error); (c) ExprBinary "=" с `children[0].Kind == ExprIdent`:
  `typ, found := env.Lookup(children[0].Value)`; found && typ != "" →
  `t := InferType(children[1])`; `t != "" && !Coerce(typ, t)` → bad-type
  (Range = children[1].Range); (d) StmtReturn с Exprs[0] в теле
  DeclFunction при `decl.Type` известного вида: `InferType` + `Coerce` →
  bad-type (Range = Exprs[0].Range)
- [x] **Interface verification**: `goga contract analysis` — pass;
  sandbox-запуск `go test ./analysis/... -count=1`
- [x] **Logic tests** (в `analysis/analyzer_test.go`, расширение):
  `TestAnalyzeRms_BadArgumentValue_Attribute` — `<land_generation>` +
  create_land с percent = 150 → bad-argument-value, Range на значении,
  diags отсортированы; `TestAnalyzeRms_HelperCallNotFlagged` — percent =
  rand_float(10, 20) → unknown-constant отсутствует;
  `TestAnalyzeXs_BadType_CallArgument` — `void f() {
  sqrt("fast"); }` → bad-type на "fast" (параметр float; xsSetWorldGravity
  в kb отсутствует — см. Facts);
  `TestAnalyzeXs_BadType_AssignmentTopLevel` — `int x = 1.5;` → bad-type;
  `TestAnalyzeXs_ReturnMismatch` — `int f() { return 1.5; }` → bad-type;
  `TestAnalyzeXs_UnknownInferTypeSilent` — вызов с аргументом-бестиповым
  локалом → bad-type отсутствует
- [x] **Debugging**: sandbox-запуск `go test ./... -count=1` — чинить
  реализацию (НЕ тесты); существующие тесты (включая server) обязаны
  остаться зелёными
- [x] **Contract re-verification**: `goga contract analysis` pass; AST не
  мутируется; нет дублей синтаксических диагностик; diags отсортированы
- [x] **Lint**: `goimports -w . && golangci-lint run && goga lint`
- [x] **STEP 8 (COMPLETION)**: отметить чекбоксы

### Task 5: Интеграционные регресс-тесты (integration tests)

Кросс-сущностные сценарии приёмки из задачи: 0 ложных срабатываний на
реальном корпусе + полный конвейер «парсер → анализатор» в том виде, как
это потребляет `server` (merge syntax + semantic в один батч).

**Usages relevant to this task:**
- `xs-parsing` from Imports: prelude.xs — обязательная фикстура (5k строк,
  882 extern-а), Parse без предвалидации.
- `rms-parsing` from Imports: rms/testdata — реальные RMS-фикстуры.
- `conventions`: интеграционные тесты — те же sandbox-правила запуска.

**CRITICAL: `CODEMANIFEST` files — read-only contract definitions.**

- [x] Создать `analysis/integration_test.go`
- [x] `TestAnalyzeXs_PreludeNoFalsePositives`: XsParse(
  docs/ref/ugc-guide/xs/prelude.xs) → AnalyzeXs — набор диагностик
  содержит 0 bad-type (undefined-symbol/bad-arity — как в базлайне до
  изменения; если базлайн-тест уже существует — расширить его сравнением
  по Code)
- [x] `TestAnalyzeRms_FixturesRegression`: все `rms/testdata/*.rms` →
  Parse → AnalyzeRms — новые bad-argument-value только на реально
  нарушающих значениях; прочие коды не изменились против базлайна
- [x] `TestPipeline_MergedDiagnostics`: для .rms с unknown-command +
  percent=150 и inline-XS блока с sqrt("fast") — собрать
  полный батч как в server (syntax diags парсера + AnalyzeRms +
  XsParse+AnalyzeXs со сдвигом диапазонов XsBlock.Range.Start) — батч
  содержит все 3 кода (unknown-command, bad-argument-value, bad-type),
  отсортирован по позиции
- [x] Run validation: sandbox-запуск `go test ./analysis/... -count=1`;
  затем полный `go test ./... -count=1` (sandbox)
- [x] Lint: `goimports -w . && golangci-lint run && goga lint`

---

## Validation Commands

- `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`: все тесты (ОБЯЗАТЕЛЬНО в cgroup-песочнице — runaway-тест уже валил машину в OOM)
- `goimports -w .`: форматирование
- `golangci-lint run`: линтер
- `goga lint`: DSL/usage-целостность ячеек
- `goga contract analysis`: контракт ячейки analysis — все экспортируемые имена совпадают с CODEMANIFEST (фасад-проверка)
- `go build ./cmd/aoe2-lsp`: бинарник собирается (потребитель server не задет)

---

## Completion Criteria

- [ ] Every contract entity is implemented in the correct `location`
      (`analyzer.go`, `values.go`, `types.go`)
- [ ] Every contract entity is accessible from the facade
      (`analysis.CheckRmsValue`, `analysis.NewTypeEnv`/`TypeEnv`,
      `analysis.InferType`, `analysis.Coerce`, `analysis.NewAnalyzer`)
- [ ] Properties and methods match the declared API (`goga contract
      analysis` pass)
- [ ] Descriptions are reflected in behavior (severity/коды/сообщения
      соответствуют контракту: bad-argument-value=error, bad-type=error)
- [ ] Contract dependencies are met (только импорт из common/kb/rms/xs)
- [ ] Re-exports: отсутствуют по контракту — не добавлять
- [ ] Every coding task followed the TDD workflow (contract tests → code →
      verification → logic tests → debugging → re-verification → lint)
- [ ] Contract tests and logic tests cover facade, API, and behavior within
      each coding task
- [ ] Integration tests exist (prelude.xs, rms-фикстуры, merged pipeline)
- [ ] No package boundary was expanded (новых ячеек/пакетов нет)
- [ ] `CODEMANIFEST` files were not modified (contract is read-only)
- [ ] All validation commands pass
- [ ] Every Usages entry is mentioned in at least one task (conventions —
      все; rms_grammar — 3,4; xs_grammar — 1,2,4; xs_coercion — 1,2,4;
      lookups — 2,3,4; rms-parsing — 4,5; xs-parsing — 2,4,5)
