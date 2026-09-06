# Design Document: `analysis-deepening`

Complete architectural specification for deepening the `analysis` cell:
RMS argument value checks + XS type checks. Sources: `docs/arch/analysis-deepening.md`
(materialized contract, commit 55c378b), `docs/tasks/analysis-deepening.md`.
No implementation code is written at this stage.

---

## Contract Changes

### Changed CODEMANIFEST Files

- `analysis/CODEMANIFEST` (modify, уже материализован):
  - Imports: kb `+CommandArg`; xs `+Expr AS XsExpr`, `+Decl`, `+Param AS XsParam`;
    common `+Range`
  - Usages: `+xs_coercion` (inline)
  - Annotations: `+xs_coercion` строка
  - `Analyzer`: Requirements о делегировании; `AnalyzeRms` шаг 4; `AnalyzeXs`
    шаги 1/3/5/6
  - Body: `+CheckRmsValue` (values.go), `+TypeEnv`, `+InferType`, `+Coerce` (types.go)
  - Footer Description дополнен

### New Entities

- `CheckRmsValue` — Routine, `analysis/values.go`: значение одного
  аргумента/атрибута RMS против `kb.CommandArg.Kind`
- `TypeEnv` — Entity, `analysis/types.go`: таблица символов XS (имя → тип,
  стек областей видимости); конструктор `NewTypeEnv(file xs.XsFile) *TypeEnv`
- `InferType` — Routine, `analysis/types.go`: тип XS-выражения ("" — не выведен)
- `Coerce` — Routine, `analysis/types.go`: совместимость типов по `xs_coercion`

### Changed Entities

- `Analyzer` — без изменения сигнатур:
  - `AnalyzeRms`: новый шаг 4 — для позиционных аргументов с имеющейся
    спецификацией и атрибутов вызывается `CheckRmsValue`
  - `AnalyzeXs`: построение `TypeEnv`, области видимости (Push/Pop с
    параметрами `xs.Param` и локалами), проверки типов аргументов вызовов,
    присваиваний и `return` → `bad-type`

### Deleted Entities

- нет

### Usages and Annotations Changes

- `+xs_coercion` (inline): таблица коерции
- `analysis/.usages/checks.md`: таблица 10 кодов (было 7), фикс незакрытой
  кодовой секции, пример inline-XS
- `analysis/.usages/value-and-type-checks.md`: новый файл (гранулярные хелперы
  для CLI-линтера)

## Applied Fixes

### Fixed CODEMANIFEST Defects

- `analysis/CODEMANIFEST`: `Expr AS RmsExpr` (из rms) + `Expr AS XsExpr`
  (из xs) → импорт `Expr` только из xs; `CheckRmsValue` принимает примитивы
  `kind`/`value` (reason: `import_has_not_duplicate` — линтер запрещает
  импорт одного имени типа из двух ячеек даже с алиасами)
- `analysis/CODEMANIFEST`: `` `Decl.Type` `` → `` `Decl` `` (reason:
  `annotation_links_exists` — линтер не резолвит точечные ссылки)
- `analysis/CODEMANIFEST`: `CheckRmsValue(store, spec, kind, value)` →
  `+ r: Range` (reason: трассировка — рутина не могла построить
  позиционированную `common.Diagnostic` без диапазона)

## Entity Interaction and Data Flow

### Interaction Diagram

```
                    AnalyzeRms(file rms.RmsFile)
                    │ walkStmts → checkCommand
                    │   атрибуты: store.Attribute → spec
                    │   позиционные: cmd.Args[i] → spec
                    ▼
             CheckRmsValue(store, spec, kind, value, r)
                    │ store.Constant / парсинг литерала
                    ▼
             []common.Diagnostic ──► сортировка ──► LSP (server, без изменений)

                    AnalyzeXs(file xs.XsFile)
                    │ NewTypeEnv(file) ────► TypeEnv
                    │ per function: Push/Declare(param)/Pop
                    ▼
             walkStmtsXs ──► InferType(store, env, expr) ──► TypeEnv.Lookup
                    │                        │                store.Function
                    ▼                        ▼
             Coerce(expected, actual) ──► bad-type / молчание ("")
```

### Data Flows

1. **RMS-значение**: `rms.Statement.Args[i]`/`Attributes[j].Value` (листовые
   `rms.Expr` без Children) → `(Kind, Value, Range)` → `CheckRmsValue` →
   `common.Diagnostic{bad-argument-value|unknown-constant}` → diags.
2. **XS-тип вызова**: `xs.Expr{Kind:call}` → для каждого аргумента
   `InferType` → строка типа → `Coerce(fn.Params[i].Type, t)` → diag.
3. **XS-присваивание**: `xs.Expr{binary "="}` → left ident → `TypeEnv.Lookup`
   → ожидаемый тип; right → `InferType` → `Coerce` → diag.
4. **XS-return**: `xs.Stmt{StmtReturn}` → тип возврата текущей функции
   (`xs.Decl.Type`) vs `InferType(expr)` → `Coerce` → diag.

### Entity Dependencies

Порядок инициализации: `kb.NewStore()` → `NewAnalyzer(store)`. Внутри вызова
`AnalyzeXs`: `NewTypeEnv(file)` → Push/Declare → `InferType(store, env, e)` →
`Coerce`. Всё в рамках одного вызова — межвызовного состояния нет.

## Code Stack Trace

### Trace: `Analyzer.AnalyzeRms` (изменение)

#### Chain

1. **Input**: `file rms.RmsFile` (частичный AST допустим).
2. Существующие шаги 1–3 (unknown-section/command/attribute, bad-argument) —
   код `analyzer.go` не меняется → checkpoint: passed (регресс исключён).
3. Новый шаг в `checkCommand` — позиционные аргументы:
   `for j := range stmt.Args` при `j < len(cmd.Args)`:
   spec = `cmd.Args[j]` (kb.CommandArg); skip если
   `len(stmt.Args[j].Children) > 0` (выражение/вызов-хелпер);
   `CheckRmsValue(a.store, spec, e.Kind, e.Value, e.Range)` → checkpoint:
   типы совпадают (`cmd.Args[j]` — kb.CommandArg; Kind/Value/Range — поля
   rms.Expr, string/common.Range) — passed.
4. Новый шаг в `checkCommand` — атрибуты: в существующем цикле после
   `store.Attribute(cmd.Name, attr.Name)` → spec найден (сейчас spec
   отбрасывается — будет использован); leaf-guard тот же;
   `CheckRmsValue(a.store, spec, attr.Value.Kind, attr.Value.Value, attr.Value.Range)`
   → checkpoint: passed.
5. **Output**: diags + новые bad-argument-value/unknown-constant, сортировка
   `sortDiags` — как раньше.

#### Checkpoint Summary

- Типы параметров CheckRmsValue ↔ поля rms.Expr: passed.
- Leaf-guard (Children == 0) исключает ложный unknown-constant на
  вызовах-хелперах (`rand_float(...)`) и DE-выражениях: passed.
- Спецификации атрибутов уже запрашиваются существующим кодом: reuse без
  лишних lookup-ов: passed.

### Trace: `CheckRmsValue`

#### Chain

1. **Input**: `(store, spec kb.CommandArg, kind string, value string, r common.Range)`.
2. `spec.Kind == "percent"` и `kind ∈ {rms.KindNumber, rms.KindPercent}`:
   нормализация литерала — срезать хвостовой `%` (KindPercent), затем
   `strconv.ParseFloat`; ошибка разбора → reported=false (консервативность)
   → checkpoint: passed.
3. Значение `< 0 || > 100` → `common.Diagnostic{Range: r, Severity: error,
   Code: CodeBadArgumentValue, Message: "...percent ... must be within 0..100"}`
   → checkpoint: границы включительно (0 и 100 валидны): passed.
4. `spec.Kind == "const"` и `kind ∈ {rms.KindConst, rms.KindIdent}`:
   `_, found := store.Constant(value)`; not found → Diagnostic{Range: r,
   Severity: warning, Code: CodeUnknownConstant, Message с оговоркой про
   #const} → checkpoint: соответствует `lookups` (точное имя, case-sensitive):
   passed.
5. Прочее → `(Diagnostic{}, false)`.
6. **Output**: `(diag, reported)`.

#### Checkpoint Summary

- Диапазон диагностики приходит параметром `r`: passed (дефект исправлен).
- ParseFloat-отказ ≠ ошибка значения: passed (молчание).

### Trace: `NewTypeEnv` + `TypeEnv`

#### Chain

1. **Input**: `file xs.XsFile`.
2. Верхний scope из `file.Decls`: `DeclVariable` → Declare(name, decl.Type);
   `DeclFunction`/`DeclExtern` → Declare(name, decl.Type) (тип возврата);
   `DeclRule`/`DeclEvent` → Declare(name, "") → checkpoint: поля Decl
   существуют (Kind/Name/Type): passed.
3. `Push()` — append пустой map; `Pop()` — pop (не ниже корневого scope —
   defensive); `Declare` — запись в верхнюю map; `Lookup` — изнутри наружу,
   первый found → checkpoint: контракт методов соответствует: passed.
4. **Output**: окружение для `InferType` и обхода тел функций.

#### Checkpoint Summary

- Локалы: `xs.Stmt` не хранит тип объявления (`parseTypeWords` в xs/parse.go
  съедает слова типа) → локалы объявляются с typ="" → проверки типов для них
  молчат — зафиксировано как ограничение AST, не дефект контракта: passed.

### Trace: `InferType`

#### Chain

1. **Input**: `(store *kb.Store, env *TypeEnv, e xs.Expr)`.
2. `ExprLiteral`: `e.Value` начинается с `"` → "string"; `strconv.ParseInt(
   base 0)` (покрывает 0x-hex) → "int"; иначе `ParseFloat` → "float"; иначе ""
   → checkpoint: passed.
3. `ExprIdent`: `true|false` → "bool"; `vector` → "vector"; `null` → "";
   иначе `env.Lookup` → typ ("" — если найден без типа); не найден → ""
   (включая kb-константы — у `kb.Constant` нет поля типа) → checkpoint:
   xsBuiltins из analyzer.go переиспользуются: passed.
4. `ExprCall`: `store.Function(e.Callee).ReturnType`; miss → `env.Lookup(callee)`
   (локальная функция); иначе "" → checkpoint: kb.Function.ReturnType string:
   passed.
5. Vector-доступ: `ExprBinary` op "." c `children[1].Value ∈ {x,y,z}` →
   "float" → checkpoint: согласовано со специальным случаем "." в checkExpr:
   passed.
6. `ExprBinary` прочие: операнды рекурсивно; оператор ∈ {==,!=,<,<=,>,>=,&&,||}
   → "bool"; ∈ {+,-,*,/,%,&,|,^,<<,>>} → расширение (int⊕int=int, int/float
   ⊕ → float); vector±vector → vector; vector*скаляр → vector; любой операнд
   "" → "" → checkpoint: таблица `xs_coercion`: passed.
7. `ExprUnary`: `!` → "bool"; `-`/`~`/`++`/`--` → тип операнда.
8. **Output**: строка типа (может быть "").

#### Checkpoint Summary

- Неизвестное всегда "" (не ложный bad-type): passed.
- Оператор "=" в InferType не участвует (присваивания обрабатывает AnalyzeXs
  напрямую): passed.

### Trace: `Coerce`

#### Chain

1. **Input**: `(expected string, actual string)`.
2. `expected == actual` → true; `actual == "int" && expected == "float"` →
   true; прочее → false → checkpoint: чистая функция, детерминирована:
   passed.
3. **Output**: bool. Вызывающий гарантирует `actual != ""` и
   `expected ∈ {int,float,bool,string,vector}` (пустые/экзотические типы kb
   пропускаются до вызова).

### Trace: `Analyzer.AnalyzeXs` (изменение)

#### Chain

1. **Input**: `file xs.XsFile`.
2. Существующее построение плоского `declared map[string]bool` (undefined-symbol)
   сохраняется без изменений → checkpoint: регресс исключён: passed.
3. `env := NewTypeEnv(file)`.
4. Обход `file.Decls`: для `DeclFunction` — `env.Push()`, `Declare(param.Name,
   param.Type)` для каждого `xs.Param`, обход тела, `env.Pop()`; правило/событие —
   тело без параметров (Push/Pop для симметрии) → checkpoint: шаг 3 контракта:
   passed.
5. В `walkStmtsXs` (передаётся текущий контекст: env + тип возврата функции):
   - `ExprCall` с известной kb-функцией: для `i < min(len(children),
     len(params))` при `params[i].Type` известного вида: `t := InferType`;
     `t != "" && !Coerce(params[i].Type, t)` → bad-type (Range =
     children[i].Range) → checkpoint: bad-arity остаётся отдельным кодом:
     passed.
   - `ExprBinary` op "=" с `children[0].Kind == ExprIdent`: `typ, found :=
     env.Lookup(children[0].Value)`; `found && typ != ""` → правый операнд
     `InferType` → `Coerce(typ, t)` → bad-type (Range = children[1].Range):
     passed.
   - `StmtReturn` с Exprs[0] и текущим типом возврата функции `decl.Type`
     (только для DeclFunction): `InferType` → `Coerce` → bad-type: passed.
   - Локалы (`StmtDecl`): `Declare(name, "")` — бестиповые, проверок нет:
     passed.
6. **Output**: diags (включая новые), `sortDiags`.

#### Checkpoint Summary

- Существующие проверки (undefined-symbol, bad-arity) не затронуты: passed.
- prelude.xs: 882 extern-а — extern-ы дают только Declare(name, ReturnType);
  тела нет; ложных bad-type нет: passed (закрепляется регресс-тестом).

## Algorithm Design

### `CheckRmsValue` (values.go)

**Responsibility**: чистая проверка одного значения против Kind-спецификации.

**Algorithm:**
```
1. IF spec.Kind == "percent" AND kind ∈ {number, percent}:
   - s := strings.TrimSuffix(value, "%")
   - n, err := strconv.ParseFloat(s, 64)
   - IF err != nil OR n < 0 OR n > 100:
     - IF err != nil → reported=false (молчание)
     - ELSE → Diagnostic{r, error, bad-argument-value}
2. ELSE IF spec.Kind == "const" AND kind ∈ {const, ident}:
   - _, found := store.Constant(value)
   - IF !found → Diagnostic{r, warning, unknown-constant,
     сообщение упоминает #const-оговорку}
3. ELSE → reported=false
```

**Errors:** нет (сигнатура без error).

**Edge Cases:**
- `"50%"` (percent-литерал) — срез `%` перед ParseFloat
- `"0"`, `"100"` — валидны (границы включительно)
- непарсимый литерал — молчание, не диагностика

### `TypeEnv` (types.go)

**Responsibility**: символы с типами + стек областей; один экземпляр на
проход AnalyzeXs.

**Algorithm:**
```
NewTypeEnv(file):
1. scopes = [map]; для каждого Decl:
   - variable/function/extern → корень[name] = Decl.Type
   - rule/event → корень[name] = ""
Push: scopes = append(scopes, map)
Pop:  IF len(scopes) > 1 → scopes = scopes[:len-1]
Declare(name, typ): scopes[len-1][name] = typ
Lookup(name): от последней области к первой; первый found → (typ, true);
  иначе ("", false)
```

**Errors:** нет.

**Edge Cases:**
- повторное Declare в той же области — перезапись (внутренние области
  затеняют внешние — это и требуется)
- Pop на корне — no-op (защита)

### `InferType` (types.go)

**Responsibility**: консервативный вывод типа выражения; "" = неизвестно.

**Algorithm:**
```
1. literal → лексема: `"…` → string; ParseInt(base 0) → int;
   ParseFloat → float; иначе ""
2. ident → true/false → bool; vector → vector; null → ""; Lookup → typ|""; 
   kb-константы не типизированы → ""
3. call → store.Function(Callee).ReturnType | Lookup(Callee) | ""
4. binary "." с member ∈ {x,y,z} → float
5. binary сравнение/логика → bool; арифметика → расширение операндов;
   vector±vector, vector*скаляр → vector; операнд "" → ""
6. unary: ! → bool; - ~ ++ -- → тип операнда
7. прочее → ""
```

**Errors:** нет. **Edge Cases:** пустые Children у binary — "".

### `Coerce` (types.go)

**Responsibility**: чистая таблица коерции (см. `xs_coercion`).

**Algorithm:**
```
expected == actual → true
actual == "int" && expected == "float" → true
иначе → false
```

**Errors:** нет. **Edge Cases:** вызов с actual="" запрещён контрактом;
защитно → true (молчание), не диагностика.

### `Analyzer` (интеграция, analyzer.go)

**Responsibility**: встраивание новых проверок в существующие обходы.

**Algorithm:**
```
checkCommand (расширение):
1. позиционные: for j, arg := range stmt.Args, j < len(cmd.Args),
   len(arg.Children) == 0 → CheckRmsValue(store, cmd.Args[j], arg.Kind,
   arg.Value, arg.Range) → reported → append
2. атрибуты: в существующем цикле после store.Attribute(...) → spec;
   len(attr.Value.Children) == 0 → CheckRmsValue(...) → append
AnalyzeXs (расширение):
1. env := NewTypeEnv(file)
2. для каждой DeclFunction: Push; Declare(param); walkBody(fnType); Pop
3. walkBody: вызовы (типы аргументов), "=" (Lookup left + InferType right),
   StmtReturn (InferType vs fnType); StmtDecl → Declare(name, "")
```

**Errors:** нет. **Edge Cases:** функция без типа возврата ("") → return
не проверяется; kb Param.Type экзотического вида → пропуск до Coerce.

## Cross-cutting Concerns

- **Error handling**: ни одна новая сигнатура не возвращает error (чистые
  функции); невозможность разобрать литерал — молчание, не ошибка.
- **Logging**: отсутствует (stateless, без IO — по контракту ячейки).
- **Validation**: leaf-guard (Children == 0) в AnalyzeRms; guard `expected`
  известного вида перед Coerce; defensive Pop.
- **Caching**: нет (запрещено контрактом).
- **Concurrency**: Analyzer без изменений stateless; TypeEnv — локальный
  экземпляр на вызов AnalyzeXs, между горутинами не разделяется.

## Usages Analysis

### `conventions`
- **What it provides**: обязательные правила Go 1.23+ (DI, goimports,
  doc-комментарии на экспорт, testify/require, table-driven,
  `Test<Component>_<Scenario>`).
- **Where used**: все новые файлы (values.go, types.go, analyzer.go, *_test.go).
- **Why chosen**: базовая практика проекта.
- **How exactly**: `NewTypeEnv` — конструктор; doc-комментарии на все
  экспортируемые идентификаторы; tests: `require.Equal`, таблицы случаев.

### `rms_grammar`
- **What it provides**: формы DE-выражений (числа/проценты/константы/
  операторы), `#const`.
- **Where used**: `CheckRmsValue`, leaf-guard в `AnalyzeRms`.
- **Why chosen**: определяет, какие значения литеральны и проверяемы.
- **How exactly**: percent-литерал `50%` и число `25` оба допустимы для
  percent-Kind; выражения с операторами пропускаются.

### `xs_grammar`
- **What it provides**: типы XS, формы выражений (вкл. vector-доступ `v.x`),
  правила/events, толерантность prelude.xs.
- **Where used**: `TypeEnv`, `InferType`, обход тел в `AnalyzeXs`.
- **Why chosen**: источник множества типов и спецформ.
- **How exactly**: типы {int,float,bool,string,vector,void}; vector-литерал
  `(x,y,z)`; extern-ы без тела.

### `xs_coercion` (inline)
- **What it provides**: таблица совместимости int/float/bool/string/vector +
  правила результата бинарных операций; `""` = молчание.
- **Where used**: `Coerce`, `InferType` (шаг 5), шаги AnalyzeXs.
- **Why chosen**: единственная точка правды о коерции; inline — короткая,
  локальная для ячейки.
- **How exactly**: `Coerce` — прямая кодировка первых двух правил; результат
  операций — widening/bool/vector-правила.

### Imported Usages

- `lookups` from `kb` — паттерны Store-запросов: `Function`/`Constant`/
  `Command`/`Attribute` с `(T, bool)`; используются в CheckRmsValue,
  InferType, существующем checkCommand. Path: `kb/.usages/lookups.md`.
- `rms-parsing` from `rms` — навигация AST, позиционная семантика атрибутов
  (Attributes принадлежат команде). Path: `rms/.usages/rms-parsing.md`.
- `xs-parsing` from `xs` — SymbolAt, допустимость частичного AST,
  prelude-толерантность. Path: `xs/.usages/xs-parsing.md`.

## `.usages/` Update

### Cell: `analysis`

#### Existing Files — Consistency

- **`checks.md`** → `analysis/.usages/checks.md`
  - Status: обновлён при материализации (таблица 10 кодов + severity,
    пример inline-XS, фикс незакрытой секции) — текущий, дополнений не нужно
- (новых файлов сверх материализованного `value-and-type-checks.md` не требуется;
  пример в нём синхронизирован с сигнатурой `r: Range`)

#### New Files (if any)

- **`value-and-type-checks`** → `analysis/.usages/value-and-type-checks.md`
  - Reason: отдельный домен «гранулярные проверки для CLI-линтера»
  - Related entities: `CheckRmsValue`, `TypeEnv`, `InferType`, `Coerce`
  - Статус: создан при материализации, актуален

## Test Stack Trace

### General Setup

- `store, err := kb.NewStore()` (реальная база, `require.NoError`);
  `a := NewAnalyzer(store)`.
- RMS-вход: `rms.Parse(text, "test.rms")` → AST + syntax diags (в тестах
  проверок анализа syntax-диагностики фильтруются по Code).
- XS-вход: `xs.XsParse(text, "test.xs")`.
- Регресс-фикстуры: `docs/ref/ugc-guide/xs/prelude.xs`,
  `rms/testdata/*.rms` (базлайны — существующие наборы диагностик).

### Source File Registry

- `analysis/values.go` → `analysis/values_test.go`
- `analysis/types.go` → `analysis/types_test.go`
- `analysis/analyzer.go` → `analysis/analyzer_test.go` (расширение)

---

### Positive Tests

#### `TestCheckRmsValue_PercentOutOfRange`

**Setup**: store (kb.NewStore); spec = kb.CommandArg{Kind: "percent"}.

**Input**: `(store, spec, rms.KindPercent, "150", r)`; отдельно
`(store, spec, rms.KindNumber, "-1", r)`.

**Trace**:
```
CheckRmsValue(store, spec, "percent", "150", r)
  → TrimSuffix("150","%") = "150" → ParseFloat = 150.0
  → 150 > 100 → Diagnostic{r, error, "bad-argument-value"} → reported=true
```

**Assertions**: reported true; diag.Code == "bad-argument-value";
diag.Severity == common.SeverityError; diag.Range == r.

**Sufficiency**: ядро новой RMS-проверки — диапазон percent.

#### `TestCheckRmsValue_KnownConst`

**Setup**: store; spec = kb.CommandArg{Kind: "const"}.

**Input**: `(store, spec, rms.KindConst, "TERRAIN_GRASS", r)`.

**Trace**: `store.Constant("TERRAIN_GRASS")` → found → reported=false.

**Assertions**: reported false. **Sufficiency**: известные константы не
дают шума (анти-ложное-срабатывание).

#### `TestCoerce_Table`

**Setup**: —. **Input**: пары (expected, actual):
`(int,int)→t, (float,int)→t, (int,float)→f, (bool,int)→f, (int,bool)→f,
(vector,vector)→t, (vector,float)→f, (string,string)→t, (string,int)→f`.

**Trace**: table-driven, каждый вызов — чистая функция.

**Assertions**: require.Equal по таблице. **Sufficiency**: кодирование
inline-usage `xs_coercion`; регресс против случайного int→bool.

#### `TestInferType_Literals`

**Input** (через env из мини-файла): literal "42"→"int"; "3.5"→"float";
"0x1F"→"int"; `"\"s\""`→"string"; ident "true"→"bool"; vector-литерал→"vector".

**Assertions**: require.Equal типам. **Sufficiency**: основа вывода типов.

#### `TestInferType_CallFromKb`

**Input**: call xsGetMapSeed (ReturnType "int" в kb).

**Assertions**: тип "int". **Sufficiency**: связка kb → типизация.

#### `TestAnalyzeRms_BadArgumentValue_Attribute`

**Input**: `<land_generation>` + `create_land` блок с `percent = 150`…

**Trace**: Parse → checkCommand → attrs loop → store.Attribute →
CheckRmsValue → diag.

**Assertions**: диагностика bad-argument-value присутствует, Range на
значении; отсортировано. **Sufficiency**: интеграция в реальный AST.

#### `TestAnalyzeXs_BadType_CallArgument`

**Input**: `void f() { xsSetWorldGravity("fast"); }` (параметр float).

**Trace**: XsParse → NewTypeEnv → walkBody → call → InferType("fast")="string"
→ Coerce("float","string")=false → bad-type.

**Assertions**: bad-type, severity error, Range на аргументе "fast".
**Sufficiency**: главный XS-сценарий из задачи.

#### `TestAnalyzeXs_BadType_AssignmentTopLevel`

**Input**: `int x = 1.5; void f() {}`.

**Trace**: DeclVariable x:"int" → env; binary "=" → Lookup("x")="int";
InferType(1.5)="float" → Coerce("int","float")=false → bad-type.

**Sufficiency**: присваивание top-level переменной (тип известен).

---

### Negative Tests

#### `TestCheckRmsValue_UnknownConst`

**Input**: `(store, spec{const}, rms.KindConst, "NOT_A_TERRAIN", r)`.

**Assertions**: reported true; Code "unknown-constant"; Severity ==
common.SeverityWarning; сообщение содержит "#const".

**Sufficiency**: warning-семантика с #const-оговоркой.

#### `TestCheckRmsValue_ExpressionSkipped`

**Input**: `(store, spec{percent}, rms.KindBinary, "1 + 2", r)` и
`(store, spec{const}, rms.KindNumber, "7", r)`.

**Assertions**: reported false (оба). **Sufficiency**: консервативность —
не-литералы и number-в-const не проверяются.

#### `TestAnalyzeRms_HelperCallNotFlagged`

**Input**: percent-атрибут со значением `rand_float(10, 20)`.

**Assertions**: unknown-constant отсутствует. **Sufficiency**: leaf-guard —
вызовы-хелперы не проходят в CheckRmsValue.

#### `TestAnalyzeXs_UnknownInferTypeSilent`

**Input**: вызов локальной функции с аргументом-локалом бестипового
объявления (`g(x)`, где `int`-тип x потерян AST).

**Assertions**: bad-type отсутствует. **Sufficiency**: консервативность ""
(защита от ложных срабатываний на реальных картах).

---

### Edge Case Tests

#### `TestCheckRmsValue_PercentBoundaries`

**Input**: `"0"`, `"100"`, `"0%"`, `"100%"`.

**Assertions**: reported false (границы включительны).

**Sufficiency**: защита от off-by-one в диапазоне.

#### `TestTypeEnv_ScopeShadowing`

**Input**: NewTypeEnv(файл с `int x; void f(float x) {}`) → в теле f:
Lookup("x") == "float"; после Pop: "int".

**Sufficiency**: изнутри наружу, затенение, Pop.

#### `TestTypeEnv_PopRootNoOp`

**Input**: env := NewTypeEnv(пустой XsFile); Pop(); Lookup("x") → found=false.

**Sufficiency**: defensive Pop.

#### `TestAnalyzeXs_PreludeNoFalsePositives`

**Input**: `docs/ref/ugc-guide/xs/prelude.xs`.

**Assertions**: набор диагностик строго равен базлайну ДО изменения
(undefined-symbol/bad-arity как раньше; bad-type — 0).

**Sufficiency**: критерий приёмки задачи — 0 ложных на 882 extern.

#### `TestAnalyzeRms_FixturesRegression`

**Input**: все `rms/testdata/*.rms`.

**Assertions**: новые диагностики только там, где значение действительно
нарушает percent/const; baseline остальных кодов не изменён.

**Sufficiency**: критерий приёмки — 0 новых ложных ошибок на фикстурах.

#### `TestAnalyzeXs_ReturnMismatch`

**Input**: `int f() { return 1.5; }`.

**Assertions**: bad-type на return. **Sufficiency**: третья точка проверки
(аргументы/присваивание/return).

## Additional Instructions for the Implementation Agent

- Читать `analysis/CODEMANIFEST` (уже материализован) — он авторитетен;
  данный документ — детализация, не замена.
- Существующий код `analyzer.go` менять минимально: undefined-symbol и
  bad-arity пути не трогать (регресс-тесты охраняют).
- Новые коды — константы `CodeBadArgumentValue`, `CodeUnknownConstant`,
  `CodeBadType` рядом с существующими в `analyzer.go`.
- `goga contract analysis` обязан пройти: экспортируемые имена ровно
  `CheckRmsValue`, `NewTypeEnv`+`TypeEnv`+методы, `InferType`, `Coerce`
  (плюс существующие).
- Валидация после реализации: `go test ./...` (только в cgroup-песочнице:
  `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p
  MemorySwapMax=0 bash -c 'go test ./... -count=1'`), `goimports -w .`,
  `golangci-lint run`, `goga lint`, `goga contract analysis`.
- Гонки не ожидаемы (stateless), но `go test -race ./analysis/...` полезен
  при удвоении запусков.
