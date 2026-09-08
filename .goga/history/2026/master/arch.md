# [ARCHITECTURE_PLAN] — Signature Help for AoE2 RMS + XS

## Topic

**Signature Help for AoE2 RMS + XS** — stateless, kb-mined, source-first
`textDocument/signatureHelp` provider for aoe2-lsp.

Plan path: `.goga/history/2026/master/arch.md`

Source reports (all user-approved): PRIMARY_ANALYSIS_REPORT (q1), TYPE_MAP_REPORT
(q2), TYPE_DETAIL_REPORT (q3–q7), CELL_DISTRIBUTION_REPORT (q8),
CONTRACTS_REPORT (q9–q13), CELL_ASSEMBLY_REPORT (q14–q19).

## Implementation Order

Cells ordered leaves → root; a cell is implementable only after everything it
imports exists.

1. **`kb`** — leaf, no Imports. Mining growth (`ValueRange`, `MineKindRange`,
   `CommandArg`/`Store`/`ExtractRmsCommands` deltas) is a prerequisite of
   `hints` rendering (kind/range labels).
2. **`xs`** — leaf (imports `common` only, unchanged). `CallSite` + `XsFile.CallAt`
   is a prerequisite of `hints.XsAt`.
3. **`rms`** — leaf (imports `common` only, unchanged). `ArgSite` + `RmsFile.ArgAt`
   is a prerequisite of `hints.RmsAt`.
4. **`hints`** — **created anew**; imports `common`, `kb`, `xs`, `rms`. All four
   dependencies exist after steps 1–3.
5. **`server`** — root; imports `hints` (new) plus existing cells. Wiring of the
   `SignatureHelp` handler, capability, and DI.

`common`, `include`, `analysis` — consumed as-is, no changes.

Marking per Artifact Resolution: **created** — `hints`; **modified** — `kb`, `xs`,
`rms`, `server`; **unchanged** — `common`, `include`, `analysis`.

---

## Artifacts

### 1. Cell `kb` — MODIFIED

CODEMANIFEST path: `kb/CODEMANIFEST`

Change summary (vs current state):

| Directive | Action | What |
|---|---|---|
| `Usages.kbdata` | change | append «Desc prose mining (signature help)» block to the inline value |
| `Annotations` | change | append mined-data provenance paragraph |
| `Store` | change | Requirements += load-time mining rule |
| `CommandArg` | change | annotations += Kind-after-load rule; properties += `Range -> ValueRange`; `Kind` property annotation extended |
| `ValueRange` | **add** | new Entity (model.go) |
| `MineKindRange` | **add** | new Routine (mine.go) |
| `ExtractRmsCommands` | change | Algorithm step 3 grows the mining pass |
| Footer | unchanged | Author Goga, CreatedAt 04/09/26 |

Impact on dependent cells (analysis, server — unchanged): `CheckRmsValue`
gates on `spec.Kind="percent"` and `commandSignature` renders `Kind`; mining
fills only empty `Kind` (in the current corpus all 34 such entries have
empty `Desc` → no fills) and the mined word is the prose kind («number»),
never overwriting structured values — so the `percent` set and all rendered
output are unchanged (SC8 / no-new-diagnostics).

Complete target state of `kb/CODEMANIFEST`:

```yaml
Usages:
  conventions: .goga/usages/conventions.md
  kbdata: |
    Embedded knowledge base files under kb/data/ (go:embed, no runtime downloads):
    - xs-functions.json  — adapted from docs/ref/ugc-guide/xs/functions/functions.json
      (categories flattened; fields: name, category, return_type, params[], desc, since_update)
    - xs-constants.json — from constants/constants.json (fields: name, section, value, desc, since_update)
    - rms-commands.json — extracted from docs/ref/zetnus-rms-guide.txt
      (fields: name, section, args[], attributes[], desc, game_versions, since_update)
    since_update sourced from docs/ref/aoe2de-xs-rms-changelog.md; empty string when unknown.
    Names must be unique within each file — validated on load.

    Desc prose mining (signature help):
    - Value bounds appear as "(<num>-<num>)" right after the kind word:
      "number (0-99)", "number (1-16)", "number (36-480)"; decimals and negative
      bounds allowed; often followed by unrelated "(default: …)" / "(see: …)"
      parenthesized fragments — only numeric bounds match.
    - The kind word in prose is "number" even for percent-typed args
      (cliff_curliness %: kind=percent, desc="number (0-100)") — mined kind fills
      only empty Kind entries, never overwrites structured ones.
    - Flag attributes (34 entries: set_circular_base, make_indestructible, …) carry
      empty Desc — nothing to mine; they stay name-only by design.

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `kbdata` for the embedded JSON file layout and schema.

  Cell is the AoE2 RMS+XS knowledge base: loads embedded JSON once at
  construction and serves read-only lookups. No IO after construction.

  Mined data (kind/range) follows the provenance rule: structured values from
  extraction win; mining fills only empty fields; unparseable prose silently
  keeps raw values — the load pipeline never fails.

---

"Store()":
  location: store.go
  annotations: |
    Загруженная база знаний; единственная точка lookup для всех потребителей.

    Requirements:
    - конструктор NewStore() читает файлы по `kbdata` и валидирует схему:
      неизвестные ключи, дубликаты имён, пустые name — ошибка загрузки
    - после построения Store потокобезопасен для чтения (immutable данные)
    - при загрузке CommandArg с пустым Range или пустым Kind —
      `MineKindRange`(Desc); идемпотентно с экстракцией (заполнение только
      пустых), непарсимое молча остаётся сырым
  methods:
    "Function(name: string) -> fn: Function, found: bool": |
      Lookup XS-функции по точному имени.

      `name`: точное имя функции (case-sensitive); found=false при отсутствии
    "Functions() -> fns: []Function": |
      Все XS-функции (для completion); порядок — как в файле данных.
    "Constant(name: string) -> c: Constant, found: bool": |
      Lookup XS-константы по точному имени; found=false при отсутствии.
    "Constants(section: string) -> cs: []Constant": |
      Константы одной секции ("" — все секции).
    "Command(name: string) -> cmd: Command, found: bool": |
      Lookup RMS-команды по точному имени; found=false при отсутствии.
    "Commands(section: string) -> cmds: []Command": |
      Команды одной секции RMS ("" — все).
    "Attribute(command: string, attr: string) -> arg: CommandArg, found: bool": |
      Спецификация атрибута команды; found=false если команда или атрибут неизвестен.

      `command`: имя команды; `attr`: имя атрибута

"Function()":
  location: model.go
  annotations: |
    XS-функция из базы (данные).
  properties:
    "Name -> string": |
      Имя функции.
    "ReturnType -> string": |
      Тип возврата.
    "Params -> []Param": |
      Параметры функции.
    "Desc -> string": |
      Описание.
    "SinceUpdate -> string": |
      Апдейт добавления ("" если неизвестен).

"Param()":
  location: model.go
  annotations: |
    Параметр XS-функции (данные). Required=false — есть значение по умолчанию.
  properties:
    "Name -> string": |
      Имя параметра.
    "Type -> string": |
      Тип параметра.
    "Required -> bool": |
      Обязательность.
    "Desc -> string": |
      Описание параметра.

"Constant()":
  location: model.go
  annotations: |
    XS-константа из базы (данные).
  properties:
    "Name -> string": |
      Имя константы.
    "Section -> string": |
      Секция констант.
    "Value -> string": |
      Строковое представление значения.
    "Desc -> string": |
      Описание.
    "SinceUpdate -> string": |
      Апдейт добавления ("" если неизвестен).

"Command()":
  location: model.go
  annotations: |
    RMS-команда из базы (данные).

    Requirements:
    - Args — позиционные аргументы; Attributes — допустимые атрибуты команды
  properties:
    "Name -> string": |
      Имя команды.
    "Section -> string": |
      Секция синтаксиса.
    "Args -> []CommandArg": |
      Позиционные аргументы.
    "Attributes -> []CommandArg": |
      Допустимые атрибуты.
    "Desc -> string": |
      Описание.
    "GameVersions -> string": |
      Версии игры, где работает.
    "SinceUpdate -> string": |
      Апдейт добавления ("" если неизвестен).

"CommandArg()":
  location: model.go
  annotations: |
    Спецификация аргумента/атрибута RMS-команды (данные).

    Requirements:
    - Kind после загрузки: пустой заполняется замайненным (`MineKindRange`),
      непустой (из экстракции) не перезаписывается
  properties:
    "Name -> string": |
      Имя аргумента/атрибута.
    "Kind -> string": |
      Ожидаемая форма значения: number / percent / const / terrain / object / ...
      Пустой после загрузки — только если prose не парсится (флаг-атрибуты).
    "Range -> ValueRange": |
      Замайненные границы значения; пустая — не замайнено.
    "Required -> bool": |
      Обязательность.
    "Desc -> string": |
      Описание.

"ValueRange(min: string, max: string)":
  location: model.go
  annotations: |
    Замайненные границы значения аргумента из Desc-prose (данные).

    `min`: нижняя граница как в prose ("" — не замайнено)
    `max`: верхняя граница ("" — не замайнено)

    Requirements:
    - чистые данные: construct-and-use, без мутации
    - границы — строки: текст prose без преобразования точности
  properties:
    "Min -> string": |
      Нижняя граница ("" — не замайнено).
    "Max -> string": |
      Верхняя граница ("" — не замайнено).

"MineKindRange(desc: string) -> kind: string, r: ValueRange":
  location: mine.go
  annotations: |
    Разбор Desc-prose в структурированные kind и границы значения.

    `desc`: prose-описание аргумента/атрибута из базы
    `kind`: замайненный kind ("" — не найден); `r`: границы (пустая — не найдены)

    Algorithm:
    1. Найти в `desc` первый фрагмент "(<num>-<num>)" — целые или десятичные,
       опциональный минус; нет — вернуть "", пустую `ValueRange`
    2. Слово непосредственно перед фрагментом — замайненный kind
       (в корпусе: number); слова нет — kind=""
    3. Вернуть kind и `ValueRange` с границами как записаны в prose
       (только обрезка окружающих пробелов)

    Requirements:
    - паттерны prose — по `kbdata` (Desc prose mining)
    - детерминированность: одинаковый вход → одинаковый результат

    Constraints:
    - непарсимая prose → пустой результат, не ошибка
    - нечисловые скобочные фрагменты ("(default: …)", "(see: …)") не матчатся

"ExtractRmsCommands(path: string) -> commands: []Command, err: error":
  location: extract.go
  annotations: |
    Разовое извлечение RMS-команд из текстового экспорта гайда Zetnus
    в rms-commands.json (формат по `kbdata`).

    `path`: путь к docs/ref/zetnus-rms-guide.txt
    `commands`: извлечённые команды; `err`: ошибки чтения/разбора

    Algorithm:
    1. Прочитать файл по `path`
    2. Найти секции Syntax Skeleton и пройтись по блокам команд
    3. Для каждой команды собрать Name, Section, Args, Attributes, Desc,
       GameVersions; для каждого CommandArg — `MineKindRange`(Desc) →
       заполнить Range, Kind пуст → замайненный kind
    4. Обогатить SinceUpdate по changelog (docs/ref/aoe2de-xs-rms-changelog.md)
    5. Проверить дубликаты имён; вернуть []Command

    Constraints:
    - утилита сборки данных, не вызывается в рантайме сервера
    - при неоднозначном разборе — пропускать фрагмент и писать WARN в slog,
      не прерывать весь проход

---

Author: Goga
CreatedAt: 04/09/26
Description: |
  База знаний AoE2 RMS+XS: встроенные JSON, lookup API и экстракция из Zetnus.
```

`.usages/` files (existing files preserved, extended):

**File `kb/.usages/data-pipeline.md`** — first repair the pre-existing fence
pairing (the «Regenerate» ```go example has no closing ```; append one), and
reword «per the kbdata schema» to the self-contained «per the JSON schema:
name, section, args[], attributes[], desc, game_versions, since_update»
(usage-file isolation). Then append section:

```md
## Mining on regeneration (signature help)

ExtractRmsCommands now mines structured kind/range from Desc prose via
kb.MineKindRange: bounded entries get Range ("number (0-99)" → 0..99);
empty-Kind entries get the mined kind when prose has one. Flag attributes
(empty Desc, e.g. set_circular_base) stay name-only — nothing to mine.
After regeneration run kb tests: NewStore re-mines fill-when-empty, so
regenerated JSON and load-time mining must agree (idempotent rule).
```

**File `kb/.usages/lookups.md`** — first repair the pre-existing fence
pairing (the «Construct once» ```go example has no closing ```; append one).
Then append section:

```md
## Mined kind/range on CommandArg (signature help rendering)

CommandArg carries structured Range (min/max strings, "" when unmined) and
Kind filled at load when the extractor left it empty. Consumers rendering
argument lists (hints) format an entry as "Name: Kind Min..Max" when Range
is present, "Name: Kind" otherwise, and name-only for flag attributes —
never invent kinds or defaults.
```

---

### 2. Cell `xs` — MODIFIED

CODEMANIFEST path: `xs/CODEMANIFEST`

Change summary:

| Directive | Action | What |
|---|---|---|
| `Annotations` | change | append call-context discrimination paragraph |
| `XsFile` | change | methods += `CallAt` |
| `CallSite` | **add** | new Entity (ast.go) |
| everything else | unchanged | XsParse, Decl, Param, Stmt, Expr, Imports, Usages, Footer |

Complete target state of `xs/CODEMANIFEST`:

```yaml
Imports:
  - Types:
      - Pos
      - Range
      - Diagnostic
      - Symbol
    Usages:
      - positions-and-diagnostics
      - symbols
    From: common

Usages:
  conventions: .goga/usages/conventions.md
  xs_grammar: .goga/usages/xs-grammar.md

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `xs_grammar` for the XS language structure being parsed.
  Use `positions-and-diagnostics` from Imports for building positions and diagnostics.
  Use `symbols` from Imports for building outline nodes in Symbols.

  Parser never fails hard: every problem becomes a Diagnostic; recovery
  resynchronizes to the next statement/declaration. Every AST node carries a Range.

  Call-контекст для signature help создаётся только узлами Expr Kind=call:
  векторные литералы «(1,2,3)» по `xs_grammar`, условия if/while/for, списки
  параметров деклараций и скобки группировки вызовом не являются.

---

"XsParse(source: string, name: string) -> file: XsFile, diags: []Diagnostic":
  location: parse.go
  annotations: |
    Разбор XS-кода (C-like + rules/events) в AST с восстановлением.

    `source`: текст (файл .xs или inline-блок из rms.XsBlock)
    `name`: имя для сообщений

    Algorithm:
    1. Лексер по `xs_grammar`: идентификаторы, числа (int/float/hex), строки,
       векторные литералы (x,y,z), операторы, комментарии // и /* */
    2. Верхний уровень: объявления — functions (с Params), variables, rules
       (условие+тело), events, include, extern
    3. Тела: statements (if/else, while, for, do, switch, return, break,
       выражения-стейтменты, блоки) и выражения
    4. При ошибке: Diagnostic, sync на следующий ';' или '}', продолжить

    Requirements:
    - File всегда не-nil; diags отсортированы по позиции

    Constraints:
    - не паниковать; err не возвращается для некорректного входа

"XsFile()":
  location: ast.go
  annotations: |
    Корень XS: Decls верхнего уровня. Имя — как передано в Parse.
  properties:
    "Name -> string": |
      Имя файла/блока.
    "Decls -> []Decl": |
      Объявления верхнего уровня.
  methods:
    "SymbolAt(pos: Pos) -> name: string, found: bool": |
      Идентификатор под позицией (для hover и completion-триггера).

      `pos`: позиция в файле

      Requirements:
      - для позиции на call возвращает имя вызываемой функции

    "Definition(pos: Pos) -> r: Range, found: bool": |
      Определение символа под позицией: вхождение → name-range декларирующего
      объявления (LSP textDocument/definition).

      `pos`: позиция в файле
      `r`: диапазон имени декларации (не всей декларации)
      `found`: локальная декларация существует

      Algorithm:
      1. Найти вхождение идентификатора, содержащее `pos` (как SymbolAt);
         нет вхождения — found=false
      2. Среди деклараций файла, объявляющих это имя, взять самую внутреннюю,
         чья область (тело функции/блока) объемлет позицию вхождения;
         при равной вложенности — ближайшую, предшествующую вхождению
         (параметр/локаль затеняют топ-левел)
      3. Позиция на имени декларации — вернуть её собственный name-range
      4. Локальной декларации нет (builtin/неизвестное) — found=false

      Requirements:
      - детерминированность: одинаковый вход → одинаковый результат

    "ReferencesAt(pos: Pos) -> ranges: []Range": |
      Все вхождения имени под позицией (LSP textDocument/references).

      `pos`: позиция в файле
      `ranges`: все вхождения имени, включая декларацию, по позиции

      Algorithm:
      1. Имя под `pos` (как SymbolAt); нет — пустой список
      2. Собрать диапазоны вхождений имени из индекса вхождений
      3. Отсортировать по позиции

      Requirements:
      - совпадение синтаксическое, по имени: одноимённые символы разных
        областей видимости не различаются

    "References(name: string) -> ranges: []Range": |
      Вхождения имени без позиции (кросс-файловые references: у вызывающей
      стороны нет позиции в чужом файле).

      `name`: имя символа
      `ranges`: все вхождения имени, включая декларацию, по позиции

      Algorithm:
      1. Собрать диапазоны вхождений `name` из индекса вхождений
      2. Отсортировать по позиции

      Requirements:
      - ReferencesAt(pos) эквивалентен References(SymbolAt(pos))
      - отсутствие имени — пустой список

    "Symbols() -> symbols: []Symbol": |
      Outline файла: декларации верхнего уровня (LSP documentSymbol).

      `symbols`: узлы топ-деклараций в исходном порядке

      Algorithm:
      1. Каждый Decl из Decls, кроме include, → `Symbol` по `symbols`
         из Imports: Kind и Name из декларации, Range=Range декларации,
         Selection=name-range из внутреннего индекса, Children пусты

      Requirements:
      - Selection ⊆ Range для каждого узла
      - плоский список: тела деклараций не раскрываются
      - include-декларации пропускаются: словарь kind их не содержит

    "CallAt(pos: Pos) -> call: CallSite, found: bool": |
      Внутренний (innermost) вызов, объемлющий позицию (signature help).

      `pos`: позиция в файле; `call`: контекст вызова; `found`: объемлющий вызов есть

      Algorithm:
      1. Позиция внутри строки или комментария → found=false
      2. Найти внутренний call-узел, чей span — от имени callee до конца списка
         аргументов — содержит `pos`; span списка аргументов тянется до фронта
         восстановления (для незакрытого вызова — до конца доступного ввода),
         не ограничен Range выражения
      3. Объемлющего вызова нет → found=false
      4. `pos` на имени callee или между именем и «(» → CallSite{callee,
         argIndex=0, onArg=false}
      5. Внутри скобок: подсчитать запятые верхнего уровня (не внутри вложенных
         вызовов/скобок/строк) до `pos` → argIndex=k, onArg=true

      Requirements:
      - in-progress (незакрытые) вызовы дают результат — жёсткое требование SC1:
        первый символ после «(» — ключевой момент фичи
      - вложенные вызовы: внутренний побеждает (f(g(x| → g, аргумент 0)
      - только Expr Kind=call создаёт контекст вызова — см. глобальные Annotations
      - детерминированность: одинаковый вход → одинаковый результат

      Constraints:
      - argIndex не клампится: выход за границы списка параметров — решение
        потребителя, не навигации

"CallSite(callee: string, argIndex: int, onArg: bool)":
  location: ast.go
  annotations: |
    Контекст вызова под курсором — данные для signature help.

    `callee`: имя вызываемой функции внутреннего вызова
    `argIndex`: 0-based ординал аргумента под курсором (валиден при onArg=true:
    сразу после «(» — 0; после k запятых верхнего уровня — k)
    `onArg`: курсор внутри списка аргументов (false — на имени callee)

    Requirements:
    - чистые данные: construct-and-use, без мутации
  properties:
    "Callee -> string": |
      Имя вызываемой функции.
    "ArgIndex -> int": |
      0-based ординал аргумента под курсором (при onArg=true).
    "OnArg -> bool": |
      Курсор внутри списка аргументов (false — на имени callee).

"Decl()":
  location: ast.go
  annotations: |
    Объявление.

    Requirements:
    - Kind=function — Params и Body заполнены; Kind=rule — условие в Body[0]
      как Exprs; Kind=extern — только сигнатура (prelude.xs)
  properties:
    "Kind -> string": |
      Вид: function / variable / rule / event / include / extern.
    "Name -> string": |
      Имя объявленного символа.
    "Type -> string": |
      Тип (переменной или возврата функции).
    "Params -> []Param": |
      Параметры функции.
    "Body -> []Stmt": |
      Тело.
    "Range -> Range": |
      Диапазон объявления.

"Param()":
  location: ast.go
  annotations: |
    Параметр XS-функции.
  properties:
    "Name -> string": |
      Имя параметра.
    "Type -> string": |
      Тип (int/float/bool/string/vector/void/...).

"Stmt()":
  location: ast.go
  annotations: |
    Оператор; Kind определяет смысл Exprs (условие) и Body (ветки/тело).
  properties:
    "Kind -> string": |
      Вид оператора.
    "Exprs -> []Expr": |
      Выражения (условие/значение).
    "Body -> []Stmt": |
      Вложенные операторы.
    "Range -> Range": |
      Диапазон оператора.

"Expr()":
  location: ast.go
  annotations: |
    Выражение.

    Requirements:
    - Kind=call: Callee=имя функции, Children=аргументы; Kind=vector:
      Children=3 операнда; Kind=binary: Value=оператор
  properties:
    "Kind -> string": |
      Вид: call / binary / unary / literal / vector / ident.
    "Callee -> string": |
      Имя вызываемой функции (для call).
    "Value -> string": |
      Литерал или оператор.
    "Children -> []Expr": |
      Операнды/аргументы.
    "Range -> Range": |
      Диапазон выражения.

---

Author: Goga
CreatedAt: 04/09/26
Description: |
  Парсер XS (External Subroutines): C-like грамматика с rules/events, AST с восстановлением.
```

`.usages/` files (existing file preserved, extended):

**File `xs/.usages/xs-parsing.md`** — first repair the pre-existing fence
pairing (the «Parse and collect diagnostics» ```go example has no closing
```; the «Navigation» example's closer then pairs the wrong block — append
one). Then append section:

````md
## Call-site lookup (signature help)

CallAt answers "which call encloses the cursor, and which argument am I
on" — for hint providers. It works on in-progress (unbalanced) calls: a
cursor just after "(" maps to argument 0, after a comma to the next index.

```go
if cs, ok := xsFile.CallAt(pos); ok {
    // cs.Callee — innermost callee name (nested calls: inner wins)
    // cs.ArgIndex — 0-based ordinal, valid when cs.OnArg is true
    // cs.OnArg — false when the cursor sits on the callee name:
    //            render the signature with no active parameter
}
```

Preconditions:
- Only call expressions create a call context — vector literals "(1,2,3)",
  if/while/for conditions, declaration parameter lists and grouping parens
  are not calls (found=false unless an enclosing call contains the position).
- Positions inside strings/comments never resolve to a call.
- ArgIndex may exceed the declared parameter count — the consumer decides
  (signature-help policy: never clamp, show no active parameter instead).
````

---

### 3. Cell `rms` — MODIFIED

CODEMANIFEST path: `rms/CODEMANIFEST`

Change summary:

| Directive | Action | What |
|---|---|---|
| `RmsFile` | change | methods += `ArgAt` |
| `ArgSite` | **add** | new Entity (ast.go) |
| everything else | unchanged | Parse, XsBlock, Include, Section, Statement, Attribute, Expr, Imports, Usages, Annotations, Footer |

Complete target state of `rms/CODEMANIFEST`:

```yaml
Imports:
  - Types:
      - Pos
      - Range
      - Diagnostic
      - Symbol
    Usages:
      - positions-and-diagnostics
      - symbols
    From: common

Usages:
  conventions: .goga/usages/conventions.md
  rms_grammar: .goga/usages/rms-grammar.md

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `rms_grammar` for the RMS language structure being parsed.
  Use `positions-and-diagnostics` from Imports for building positions and diagnostics.
  Use `symbols` from Imports for building outline nodes in Symbols.

  Parser never fails hard: every problem becomes a Diagnostic; recovery
  resynchronizes to the next statement/section. Every AST node carries a Range.

  Директивы подключений по `rms_grammar` (Directives): #include → Includes;
  #includeXS с аргументом → XsIncludes + переключение в inline-XS режим
  (блок после директивы — в XsBlocks); bare #includeXS → только XsBlock.
  Путь без аргумента — синтаксическая Diagnostic, Include не создаётся.

  Канонические значения Kind (Statement, Expr) экспортированы как
  Go-константы Kind* (например KindCommand, KindBinary).

---

"Parse(source: string, name: string) -> file: RmsFile, diags: []Diagnostic":
  location: parse.go
  annotations: |
    Разбор RMS-файла в AST с восстановлением после ошибок.

    `source`: текст файла; `name`: имя файла (для сообщений)
    `file`: AST (всегда не-nil, возможно частичный); `diags`: синтаксические проблемы

    Algorithm:
    1. Лексер: токены по `rms_grammar` (слова, числа, проценты, строки,
       комментарии, директивы #include/#includeXS, секции <...>)
    2. Построить список секций; вне секций — глобальные statements
    3. Для каждого statement: команда + позиционные аргументы (Expr),
       последующие атрибуты прикреплять к текущей команде (позиционная семантика)
    4. Блоки start_random/percent_chance и if/elseif/else вкладывать в Children
    5. Директивы: #include с аргументом-путём → Include в Includes
       (Range = аргумент-путь); #includeXS с аргументом → Include в XsIncludes
       и начало XsBlock; bare #includeXS → только XsBlock; собирать строки
       блока до следующей директивы/секции
    6. При ошибке: Diagnostic с диапазоном токена, sync на следующий
       statement/секцию, продолжить разбор

    Requirements:
    - возвращённый File всегда не-nil (пустой при полном провале)
    - diags отсортированы по позиции

    Constraints:
    - не паниковать и не возвращать err для некорректного входа

"RmsFile()":
  location: ast.go
  annotations: |
    Корень AST RMS-файла.

    Requirements:
    - XsBlocks содержат встроенный XS-код, но не разбираются здесь
  properties:
    "Name -> string": |
      Имя файла.
    "Sections -> []Section": |
      Секции скрипта. Statements вне именованных секций собираются
      в синтетическую секцию "global" (материализуется только если непуста);
      в грамматике RMS такой секции нет.
    "Includes -> []Include": |
      Директивы #include (путь + диапазон аргумента).
    "XsIncludes -> []Include": |
      Внешние XS-скрипты (#includeXS с аргументом); inline-код — в XsBlocks.
    "XsBlocks -> []XsBlock": |
      Встроенные XS-блоки (#includeXS).
  methods:
    "SectionAt(pos: Pos) -> section: Section, found: bool": |
      Секция, содержащая позицию (для completion-контекста).
      Для позиции вне именованных секций возвращает синтетическую
      "global" (если она материализована), иначе found=false.

      `pos`: позиция в файле

    "StatementAt(pos: Pos) -> stmt: Statement, found: bool": |
      Ближайший statement, содержащий/предшествующий позиции (для hover).

      `pos`: позиция в файле

      Requirements:
      - для позиции на атрибуте возвращает команду-владельца

    "Symbols() -> symbols: []Symbol": |
      Outline-дерево файла (LSP textDocument/documentSymbol).

      `symbols`: узлы секций и глобальные узлы вне секций

      Algorithm:
      1. Каждая Section → `Symbol` по `symbols` из Imports: kind=section,
         Name без угловых скобок, Range=Range секции,
         Selection=диапазон токена заголовка
      2. Дети секции — Statements в исходном порядке: kind=command,
         Name=имя команды, Range=statement с атрибутами,
         Selection=токен имени; блоки random/conditional рекурсивно
         в Children
      3. Каждый XsBlock → узел kind=xs, Name="#includeXS",
         Range=Selection=Range блока; поместить в содержащую его
         Section, иначе в корень, по позиции среди детей
      4. Statements вне секций — узлы корня

      Requirements:
      - Selection ⊆ Range для каждого узла; дети внутри Range родителя

    "ReferencesAt(pos: Pos) -> ranges: []Range": |
      Все вхождения имени под позицией (LSP textDocument/references).

      `pos`: позиция в файле
      `ranges`: диапазоны одноимённых слов-токенов, по позиции

      Algorithm:
      1. Слово-токен, содержащий `pos`, из внутреннего индекса токенов;
         нет — пустой список
      2. Собрать все одноимённые слова-токены (имена секций, команд,
         атрибутов и значения ident/const)
      3. Отсортировать по позиции

      Requirements:
      - локальных деклараций в RMS нет: все вхождения равнозначны
        (вызывающая сторона не отбрасывает декларацию)

    "References(name: string) -> ranges: []Range": |
      Вхождения имени без позиции (кросс-файловые references: у вызывающей
      стороны нет позиции в чужом файле).

      `name`: имя (слово-токен)
      `ranges`: все вхождения имени, по позиции

      Algorithm:
      1. Собрать все слова-токены, равные `name` (имена секций, команд,
         атрибутов и значения ident/const)
      2. Отсортировать по позиции

      Requirements:
      - ReferencesAt(pos) эквивалентен References(слово под pos)
      - отсутствие имени — пустой список

    "ArgAt(pos: Pos) -> site: ArgSite, found: bool": |
      Дискриминация аргумента под курсором: команда-владелец + активный
      элемент (signature help).

      `pos`: позиция в файле; `site`: аргументный контекст; `found`: владелец есть

      Algorithm:
      1. Ближайший statement, объемлющий/предшествующий `pos` — семантика
         StatementAt, включая хвостовые позиции и незакрытые блоки; нет
         (директивы #const/#include, заголовки секций, вне всего) → found=false
      2. `pos` внутри строки или комментария → found=false
      3. `pos` на токене имени команды → kind=none
      4. `pos` внутри значения N-го позиционного аргумента (Args[N]) →
         kind=arg, index=N
      5. `pos` на имени атрибута или его значении (Attribute.Name/Value) →
         kind=attr, name=имя атрибута (name-match — имя атрибута — его
         единственная truthful-идентичность)
      6. Неоднозначный маппинг (странные однострочные сплиты атрибутов,
         braceless-слово как позиционный аргумент при неясности) или
         позиция вне всех значений и имён (пробел между аргументами,
         «{»/«}», хвост команды) → kind=none

      Requirements:
      - вложенные блоки (random/conditional): владеет самый внутренний statement
      - атрибуты в «{ }» после команды — обычный случай позиционной семантики
      - детерминированность: одинаковый вход → одинаковый результат

      Constraints:
      - kind=none при неоднозначности — никогда неправильная активная метка (SC6);
        это решение навигации, а не потребителя

"ArgSite(stmt: Statement, kind: string, index: int, name: string)":
  location: ast.go
  annotations: |
    Аргументный контекст под курсором — данные для signature help.

    `stmt`: команда-владелец (позиционная семантика: атрибуты принадлежат
    предшествующей команде; вложенные блоки — своему statement)
    `kind`: arg (курсор на позиционном аргументе) / attr (на атрибуте — имени
    или значении) / none (на имени команды или неоднозначно)
    `index`: 0-based номер позиционного аргумента (kind=arg)
    `name`: имя атрибута (kind=attr)

    Requirements:
    - чистые данные: construct-and-use, без мутации
    - kind-дискриминатор — стиль Statement.Kind/Expr.Kind
  properties:
    "Stmt -> Statement": |
      Команда-владелец.
    "Kind -> string": |
      arg / attr / none.
    "Index -> int": |
      0-based номер позиционного аргумента (kind=arg).
    "Name -> string": |
      Имя атрибута (kind=attr).

"XsBlock()":
  location: ast.go
  annotations: |
    Встроенный XS-код (после #includeXS) с диапазоном исходника.

    Constraints:
    - RMS-парсер не разбирает содержимое; потребитель передаёт Code в xs.Parse
  properties:
    "Code -> string": |
      Исходный текст блока (позиции — относительно блока).
    "Range -> Range": |
      Диапазон блока в RMS-файле.

"Include(path: string, r: Range)":
  location: ast.go
  annotations: |
    Директива-подключение: путь и диапазон аргумента-пути.

    `path`: путь как записан (относительный)
    `r`: диапазон аргумента-пути (не всей директивы — hit-test курсора
    и диагностика точнее)

    Constraints:
    - чистый тип данных: construct-and-use, без методов
  properties:
    "Path -> string": |
      Путь подключения (как записан в исходнике).
    "Range -> Range": |
      Диапазон аргумента-пути.

"Section()":
  location: ast.go
  annotations: |
    Секция скрипта (например land_generation). Name — без угловых скобок.
  properties:
    "Name -> string": |
      Имя секции.
    "Statements -> []Statement": |
      Утверждения секции.
    "Range -> Range": |
      Диапазон секции.

"Statement()":
  location: ast.go
  annotations: |
    Команда или блок.

    Requirements:
    - позиционная семантика: Attributes принадлежат команде, после которой записаны
    - Kind=random — Children вложены; Kind=conditional — if/elseif/else
  properties:
    "Kind -> string": |
      Вид: command / random / conditional.
    "Name -> string": |
      Имя команды.
    "Args -> []Expr": |
      Позиционные аргументы.
    "Attributes -> []Attribute": |
      Атрибуты команды.
    "Children -> []Statement": |
      Вложенные statements (блоки).
    "Range -> Range": |
      Диапазон statement.

"Attribute()":
  location: ast.go
  annotations: |
    Атрибут в коде: имя + одно значение-выражение.
  properties:
    "Name -> string": |
      Имя атрибута.
    "Value -> Expr": |
      Значение атрибута.
    "Range -> Range": |
      Диапазон атрибута.

"Expr()":
  location: ast.go
  annotations: |
    Выражение DE-математики: рекурсивное дерево.

    Requirements:
    - Kind=binary: Value=оператор, Children=операнды; Kind=ident/const: Value=имя
  properties:
    "Kind -> string": |
      Вид: number / percent / const / ident / binary / unary.
    "Value -> string": |
      Литерал, имя или оператор.
    "Children -> []Expr": |
      Операнды; для ident-вызова хелпера — аргументы вызова.
    "Range -> Range": |
      Диапазон выражения.

---

Author: Goga
CreatedAt: 04/09/26
Description: |
  Парсер Random Map Scripts: лексер, AST с восстановлением, навигация по позициям.
```

`.usages/` files (existing file preserved, extended):

**File `rms/.usages/rms-parsing.md`** — append section:

````md
## Argument lookup (signature help)

ArgAt answers "which command owns the cursor, and which argument or
attribute am I on" — for hint providers. Owner resolution follows
StatementAt semantics (works on trailing positions and unclosed blocks);
the added value is argument discrimination.

```go
if site, ok := file.ArgAt(pos); ok {
    // site.Stmt — owning command statement (attrs attach to the preceding
    //             command; nested blocks own their innermost statement)
    // site.Kind — "arg" | "attr" | "none"
    // site.Index — 0-based positional ordinal (Kind="arg")
    // site.Name — attribute name (Kind="attr"), active by name-match
}
```

Preconditions:
- Positions inside strings/comments and on directives/section headers
  return found=false — render no hint.
- Ambiguous mappings return Kind="none" — render the signature with no
  active mark rather than a guessed one.
- The consumer (hints) maps a positional ordinal to the kb Args list and
  an attribute name to its index in kb Attributes — RMS has no local
  declarations to consult.
````

---

### 4. Cell `hints` — CREATED

CODEMANIFEST path: `hints/CODEMANIFEST` (new file)

Complete content:

```yaml
Imports:
  - Types:
      - Pos
    Usages:
      - positions-and-diagnostics
    From: common
  - Types:
      - Store
      - Function
      - Command
      - CommandArg
      - ValueRange
    Usages:
      - lookups
    From: kb
  - Types:
      - XsFile
      - CallSite
      - Decl
    Usages:
      - xs-parsing
    From: xs
  - Types:
      - RmsFile
      - ArgSite
      - Statement
    Usages:
      - rms-parsing
    From: rms

Usages:
  conventions: .goga/usages/conventions.md

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `lookups` from Imports для kb-lookup API (Store.Function / Store.Command).
  Use `xs-parsing` from Imports для семантики CallAt call-контекста.
  Use `rms-parsing` from Imports для семантики ArgAt аргументов.
  Use `positions-and-diagnostics` from Imports для Pos.

  Ячейка вычисляет хинты signature help над готовыми AST и kb: stateless —
  результат зависит только от аргументов метода; протоколо-независима.
  Truth-модель XS: source-декларации (документ и замыкание — один пул)
  побеждают kb; конфликтующие source-декларации одного имени — молчание. Опциональные
  параметры — в квадратных скобках по структурным Required; default-значения
  не выдумываются (они — prose в Desc). RMS-список рендерится полностью,
  без усечения. Активный параметр никогда не клампится.

---

"Computer(store: Store)":
  location: computer.go
  annotations: |
    Вычислитель хинтов: truth-модель + правило конфликтов + рендер.

    `store`: база знаний (DI; NewComputer)

    Requirements:
    - stateless: без мутабельного состояния между вызовами
    - Documentation в Hint не входит — расширенные описания остаются в hover
  methods:
    "XsAt(file: XsFile, pos: Pos, external: []Decl) -> hint: Hint, found: bool": |
      Хинт для позиции в XS-файле (документ или inline-блок в block-local
      координатах — трансляцию выполняет вызывающая сторона).

      `file`: разобранный XS; `pos`: позиция в координатах файла;
      `external`: декларации include-замыкания;
      `hint`: рендер; `found`: контекст и сигнатура найдены

      Algorithm:
      1. Call-контекст: метод CallAt файла `XsFile` по `xs-parsing` →
         `CallSite`; found=false → молчание
      2. Truth-модель по callee: source-декларации (Kind=function) из
         file.Decls + `external`; несколько с разными параметрами → молчание;
         совпадающие (последовательности пар Type+Name равны) → единая
         декларация; нет source → lookup `Function`
         в `Store`; нет и там → молчание
      3. Рендер: label = «<ret> <name>(<метки>)», ret — только если объявлен
         (source: Decl.Type; kb: ReturnType); метки: kb — «<Type> <Name>»,
         опциональные (Required=false) в «[…]»; source — «<Type> <Name>»,
         без типа — «<Name>»; source-параметры все обязательные
      4. active: onArg=false → -1; иначе argIndex < len(params) → argIndex,
         иначе -1 (никогда не клампить)

      Requirements:
      - имена в порядке объявления; типы только объявленные — не выдумываются

    "RmsAt(file: RmsFile, pos: Pos) -> hint: Hint, found: bool": |
      Хинт для позиции в RMS-файле: kb-упорядоченный список команды.

      `file`: разобранный RMS; `pos`: позиция в файле;
      `hint`: рендер; `found`: контекст найден

      Algorithm:
      1. Аргументный контекст: метод ArgAt файла `RmsFile` по `rms-parsing` →
         `ArgSite`; found=false → молчание
      2. Lookup `Command` в `Store` по имени владельца (`Statement` из site);
         нет → молчание
      3. Рендер: label = «<name>(<args…>, <attrs…>)» — сначала Args, затем
         Attributes; метка элемента `CommandArg` — «Name: Kind» + « Min..Max»
         при непустом `ValueRange`; опциональные (Required=false) в «[…]»;
         флаг-атрибуты без Kind — name-only; полный список всегда, без усечения
      4. active: kind=arg → index; kind=attr → len(Args) + позиция Name в
         Command.Attributes (имени нет в kb-списке → -1); kind=none → -1

      Requirements:
      - качество подсказок XS ↔ RMS паритетно (SC5)

"Hint(label: string, params: []string, active: int)":
  location: hint.go
  annotations: |
    Рендер готового хинта для одного call-site (данные, протоколо-независимые).

    `label`: полная сигнатура одной строкой
    (например «vector xsVectorSet(float x, float y, float z)»,
    «percent_chance(%: percent 0..99)»)
    `params`: метки параметров в порядке списка; опциональные — в «[…]»
    (например «[z: float]», «[MaxHeight: number 1..16]», «[set_circular_base]»)
    `active`: индекс активного параметра в params; -1 — активного нет

    Requirements:
    - чистые данные: construct-and-use, без мутации
    - мапинг в protocol.SignatureInformation — ответственность server
  properties:
    "Label -> string": |
      Полная сигнатура одной строкой.
    "Params -> []string": |
      Метки параметров в порядке списка; опциональные — в «[…]».
    "Active -> int": |
      Индекс активного параметра; -1 — активного нет.

---

Author: Goga
CreatedAt: 08/09/26
Description: |
  Вычисление хинтов signature help для RMS и XS: truth-модель (source > kb),
  правило конфликтов, рендер меток; протоколо-независимая ячейка над kb/xs/rms.
```

`.usages/` files (new):

**File `hints/.usages/computing.md`** (new):

````md
# Hints Computing — consuming the hints cell

Domain: computing signature-help hints for RMS and XS positions.
Target audience: implementers of the server cell (the LSP handler).

## Construct once, share everywhere

```go
computer := hints.NewComputer(store) // kb.Store via constructor DI
```

## XS hint at a position

```go
// xsFile from xs.XsParse; external decls from include closure
// (Closure.ExternalDecls(uri)); for inline RMS blocks translate the
// cursor into block-local coordinates first (XsBlock.Range offset).
if hint, ok := computer.XsAt(xsFile, pos, externalDecls); ok {
    // hint.Label / hint.Params / hint.Active — protocol-agnostic
    // hint.Active == -1 → render with no active parameter (never clamp)
}
```

## RMS hint at a position

```go
if hint, ok := computer.RmsAt(rmsFile, pos); ok {
    // full kb-ordered list: positional Args first, then Attributes;
    // optional entries arrive pre-bracketed in the label strings
}
```

Preconditions:
- Parse the document first; hint answers are valid for that parse only.
- Silence (found=false) is the designed answer for unknown names, broken
  syntax, strings/comments, ambiguous mappings — map it to nil, nil in the
  protocol handler, never to a guessed hint.
- The cell never imports go.lsp.dev — mapping Hint to
  protocol.SignatureInformation (single signature, ActiveSignature=0,
  ActiveParameter=nil when Active<0) belongs to the server cell.
````

---

### 5. Cell `server` — MODIFIED

CODEMANIFEST path: `server/CODEMANIFEST`

Change summary:

| Directive | Action | What |
|---|---|---|
| `Imports` | **add** | new block: Types `Computer`, `Hint`; Usages `computing`; From `hints` |
| `Annotations` | change | Usages-строка += `computing`; append signature-help paragraph |
| `Server` | change | signature += `computer: Computer`; annotations += computer param; `Initialize` += SignatureHelpProvider; methods += `SignatureHelp` |
| `Serve` | change | Algorithm step 1 += NewComputer(store) |
| Footer `Description` | change | «над kb/rms/xs/analysis/include/hints» |
| everything else | unchanged | DocStore and its methods, all other Server methods, Usages, CreatedAt |

Complete target state of `server/CODEMANIFEST`:

```yaml
Imports:
  - Types:
      - Store
    Usages:
      - lookups
    From: kb
  - Types:
      - Parse
      - RmsFile
      - XsBlock
    Usages:
      - rms-parsing
    From: rms
  - Types:
      - XsParse
      - XsFile
      - Decl
    Usages:
      - xs-parsing
    From: xs
  - Types:
      - Analyzer
    Usages:
      - checks
    From: analysis
  - Types:
      - Computer
      - Hint
    Usages:
      - computing
    From: hints
  - Types:
      - Symbol
    Usages:
      - symbols
    From: common
  - Types:
      - Source
      - Resolver
      - Closure
      - Target
      - MissingInclude
    Usages:
      - closure
    From: include

Usages:
  conventions: .goga/usages/conventions.md
  lsp-protocol: .goga/usages/cooks/lsp-protocol.md

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `lsp-protocol` for go.lsp.dev patterns (NewServer bootstrap, Full sync,
  PublishDiagnostics, Hover/Completion result shapes, Boolean/Optional arms).
  Use `lookups`, `rms-parsing`, `xs-parsing`, `checks`, `closure`, `computing`
  from Imports for the provider APIs wired into handlers.

  Language selection by URI extension: .rms → `Parse` → `RmsFile`
  (+ inline `XsBlock` → `XsParse` with range shift), .xs → `XsParse` → `XsFile`.
  One change = one diagnostics batch.
  Protocol types (InitializeParams, Hover, CompletionList, ...) belong to
  go.lsp.dev/protocol — follow `lsp-protocol` for their construction.

  Навигационные хендлеры (Definition/References/DocumentSymbol) следуют
  секции Navigation `lsp-protocol`: пустой результат — пустой slice, не nil.
  Definition/References резолвятся через `Resolver` по include-замыканию
  (Cross-file Navigation Results): цели могут жить в неоткрытых файлах.

  `Resolver` создаётся в конструкторе `Server` над собственным `DocStore`
  (NewResolver(docs): DocStore структурно удовлетворяет `Source`).
  Внешние декларации (`Decl` из ExternalDecls по `closure`) передаются в
  AnalyzeXs — тип `Decl` импортирован из xs.

  Use `computing` from Imports для вычисления хинтов signature help:
  молчание found=false → nil, nil (конвенция Hover); триггеры «(» и «,»
  не пересекаются с completion'ыми « » и «<».

---

"Server(store: Store, analyzer: Analyzer, computer: Computer)":
  location: server.go
  annotations: |
    LSP-сервер: протокол поверх kb/rms/xs/analysis/hints.

    `store`: база знаний; `analyzer`: семантические проверки;
    `computer`: вычислитель хинтов (DI)

    Requirements:
    - паттерны протокола строго по `lsp-protocol` (UnimplementedServer, Full sync)
    - Resolver над собственным DocStore (по `closure`)
  methods:
    "Initialize(ctx: Context, params: InitializeParams) -> result: InitializeResult, err: error": |
      Заявить возможности: TextDocumentSync Full+OpenClose, HoverProvider
      Boolean(true), CompletionProvider с триггерами; DefinitionProvider,
      ReferencesProvider, DocumentSymbolProvider — Boolean(true);
      SignatureHelpProvider с триггерами «(» и «,»;
      согласовать positionEncoding (utf-8 при поддержке клиентом).
    "DidOpen(ctx: Context, params: DidOpenTextDocumentParams) -> err: error": |
      Algorithm:
      1. DocStore.Put(uri, text, version)
      2. Пересчитать диагностики (общая функция с DidChange):
         вычислить `Closure` для uri; записи `MissingInclude` с Owner==uri →
         Diagnostic (code="missing-include", range директивы); RMS:
         синтаксис + AnalyzeRms; inline XsBlock: XsParse + AnalyzeXs(xsFile,
         ExternalDecls("")); .xs: AnalyzeXs(file, ExternalDecls(uri))
      3. PublishDiagnostics по `lsp-protocol` (одна пачка)
    "DidChange(ctx: Context, params: DidChangeTextDocumentParams) -> err: error": |
      Algorithm:
      1. Взять весь текст из ContentChanges (WholeDocument)
      2. DocStore.Put; пересчитать (как в DidOpen); PublishDiagnostics
    "DidClose(ctx: Context, params: DidCloseTextDocumentParams) -> err: error": |
      DocStore.Remove(uri); опубликовать пустой список диагностик.
    "Hover(ctx: Context, params: HoverParams) -> hover: Hover, err: error": |
      Algorithm:
      1. Для .rms: StatementAt → Store.Command → Markdown (сигнатура+desc+
         GameVersions/SinceUpdate); для .xs: SymbolAt → Store.Function
      2. nil, nil когда под курсором ничего нет
    "Completion(ctx: Context, params: CompletionParams) -> result: CompletionResult, err: error": |
      Algorithm:
      1. .rms: SectionAt → Store.Commands(section) + константы;
         .xs: префикс слова → Store.Functions + Store.Constants
      2. CompletionList: label, kind, detail=сигнатура, documentation=desc
    "SignatureHelp(ctx: Context, params: SignatureHelpParams) -> help: SignatureHelp, err: error": |
      Параметрические хинты для позиции в документе (stateless, read-only).

      `params`: позиция и контекст запроса; `help`: результат или nil (молчание)

      Algorithm:
      1. Язык по расширению URI (общий шаблон с Hover)
      2. .xs: XsParse(text) + Closure(uri).ExternalDecls(uri) →
         computer.XsAt(file, pos, decls)
      3. .rms: Parse(text); external = Closure(uri).ExternalDecls("")
         (как DidOpen для inline-блоков); позиция внутри XsBlock
         (Range.Contains) →
         трансляция в block-local координаты (шаблон include.Definition) +
         XsParse(block.Code) → computer.XsAt(xsFile, blockPos, external);
         иначе computer.RmsAt(file, pos)
      4. `Hint` → protocol.SignatureInformation по `lsp-protocol` (Signature Help):
         Label=hint.Label; Parameters — метки строкой; Documentation опускается;
         ровно одна сигнатура, ActiveSignature=0; ActiveParameter=uint32(active)
         при active>=0, иначе nil
      5. Молчание: found=false → nil, nil

      Requirements:
      - stateless: ответ зависит только от (document, position);
        TriggerKind/TriggerCharacter/IsRetrigger на результат не влияют
      - read-only (R10), аддитивно к существующим хендлерам (C4/SC8)
      - C6: недоступное замыкание → ExternalDecls даёт что может,
        unknown-имена молча падают до kb → silence
    "Definition(ctx: Context, params: DefinitionParams) -> result: DefinitionResult, err: error": |
      Кросс-файловый переход к определению.

      Algorithm:
      1. Resolver.Definition(ctx, uri, pos) по `closure`
      2. Найденный `Target` → protocol.Location (URI может отличаться
         от запрошенного — Cross-file Navigation Results в `lsp-protocol`)
      3. Промах → пустая LocationSlice (не nil); ошибкой не считать

      Requirements:
      - конвертация позиций — с учётом согласованного positionEncoding

    "References(ctx: Context, params: ReferenceParams) -> locations: []Location, err: error": |
      Ссылки на символ по include-замыканию.

      Algorithm:
      1. Resolver.References(ctx, uri, pos) по `closure`; каждый `Target` →
         Location
      2. IncludeDeclaration=false → исключить range локальной декларации
         (range Definition в запрошенном файле)
      3. Пустой результат — пустой список, не nil

    "DocumentSymbol(ctx: Context, params: DocumentSymbolParams) -> result: DocumentSymbolResult, err: error": |
      Outline документа.

      Algorithm:
      1. Symbols() по языку (`xs-parsing` / `rms-parsing`)
      2. Рекурсивно `Symbol` → protocol.DocumentSymbol по `symbols`:
         Kind по таблице (xs: function/extern→Function, variable→Variable,
         rule/event→Event; rms: section→Module, command→Function,
         xs→Namespace; неизвестный kind → Field);
         Range/Selection напрямую, Children рекурсивно
      3. Вернуть DocumentSymbolSlice (иерархическая форма)

      Requirements:
      - Selection ⊆ Range сохраняется при маппинге (инвариант `symbols`)
    "Shutdown(ctx: Context) -> err: error": |
      Вернуть nil (default Unimplemented возвращает not-implemented).
    "Exit(ctx: Context) -> err: error": |
      Завершить соединение/процесс.

"DocStore()":
  location: docstore.go
  annotations: |
    Кэш открытых документов uri → text + version.

    Requirements:
    - Put со stale version (меньше текущей) игнорируется
    - структурно удовлетворяет Source из Imports (метод Text)
  methods:
    "Put(uri: string, text: string, version: int)": |
      Сохранить текст документа с версией.
    "Get(uri: string) -> text: string, version: int, found: bool": |
      Текст и версия документа; found=false если документ не открыт.
    "Text(uri: string) -> text: string, found: bool": |
      Editor-state текст без версии (проекция Get для `Source`).

      `uri`: идентификатор документа; `found`: документ открыт
    "URIs() -> uris: []string": |
      URI всех открытых документов (часть удовлетворения `Source`).
    "Remove(uri: string)": |
      Удалить документ из кэша.

"Serve(ctx: Context) -> err: error":
  location: serve.go
  annotations: |
    stdio-бутстрап по `lsp-protocol`.

    `ctx`: контекст процесса; `err`: причина завершения

    Algorithm:
    1. Собрать зависимости: NewStore, NewAnalyzer, NewComputer(store),
       Server(store, analyzer, computer), DocStore
    2. protocol.NewServer(ctx, srv, stream); ждать conn.Done()

---

Author: Goga
CreatedAt: 04/09/26
Description: |
  Корневая ячейка: LSP-сервер (stdio) над kb/rms/xs/analysis/include/hints
  для редакторов.
```

`.usages/` files (existing file preserved, one section replaced):

**File `server/.usages/lifecycle.md`** — replace the «Advertised capabilities»
section body with:

```md
Initialize advertises: diagnostics (Full sync + OpenClose), hover,
completion, navigation — definition, references, documentSymbol — and
signature help (TriggerCharacters "(" and ","). Editor configs need no
extra flags; positionEncoding is negotiated (prefer utf-8 when the
client offers it). Signature-help widgets refresh on client re-requests
while open; the manual signature-help binding is the guaranteed path.
```

---

## Dependency Map

```
                    ┌──────────┐
                    │  common  │ unchanged (Pos)
                    └────┬─────┘
         ┌───────────┬───┴────────┬─────────────┐
         ▼           ▼            ▼             │
      ┌─────┐    ┌─────┐     ┌─────┐           │
      │ kb  │    │  xs │     │ rms │  all modified (growth only)
      └──┬──┘    └──┬──┘     └──┬──┘           │
         │          │            │              │
         └──────────┼────────────┘              │
                    ▼                           │
              ┌──────────┐                      │
              │  hints   │  CREATED             │
              └────┬─────┘                      │
                   │                            │
    include ───────┼──────────────┐  unchanged  │
    analysis ──────┼─────────┐    │  unchanged  │
                   ▼         ▼    ▼             ▼
              ┌─────────────────────────────┐
              │           server            │  modified
              └─────────────────────────────┘
```

Imported types per new connection:

- `common` → `hints`: `Pos` (+ usage `positions-and-diagnostics`)
- `kb` → `hints`: `Store`, `Function`, `Command`, `CommandArg`, `ValueRange`
  (+ usage `lookups`)
- `xs` → `hints`: `XsFile`, `CallSite`, `Decl` (+ usage `xs-parsing`)
- `rms` → `hints`: `RmsFile`, `ArgSite`, `Statement` (+ usage `rms-parsing`)
- `hints` → `server`: `Computer`, `Hint` (+ usage `computing`)

No cycles; strictly downward.

## Verification Checklist

After implementing each artifact, verify:

**kb**
- `goga lint` passes for `kb/CODEMANIFEST` (YAML structure, casing, locations)
- `MineKindRange` unit tests: `number (0-99)` → kind number, 0..99;
  `number (36-480)` → 36..480; `(default: use size set in lobby)` → no match;
  empty prose → empty result, no error
- `NewStore` load: 34 flag attributes stay name-only (Kind "", empty Range);
  8 bounded entries get Range; `percent_chance.%` keeps Kind `percent` with
  Range 0..99 (provenance rule — no overwrite)
- Extraction idempotence: re-running `ExtractRmsCommands` then `NewStore`
  yields the same visible CommandArg state as load-time mining alone
- `kb/.usages/` files carry the appended sections; existing content intact

**xs**
- `goga lint` passes for `xs/CODEMANIFEST`
- `CallAt` tests: `f(` → {f, 0, true}; `f(a,` → {f, 1, true}; `f(g(x), y`
  with cursor in `x` → {g, 0, true}; cursor on `f` name → {f, 0, false};
  vector literal `(1,2,3)` with cursor on `2` → found=false; declaration
  param list `function f(int a, int b` → found=false; string/comment → found=false
- No regression: `SymbolAt`/`Definition`/`ReferencesAt`/`Symbols` tests unchanged
- `xs/.usages/xs-parsing.md` carries the appended section

**rms**
- `goga lint` passes for `rms/CODEMANIFEST`
- `ArgAt` tests: cursor on positional value → {stmt, arg, N}; on attribute name
  and on attribute value → {stmt, attr, name}; on command name → {stmt, none};
  cursor between arguments (whitespace), on «{»/«}» → {stmt, none};
  inside string/comment, on `#const`/section header → found=false; unclosed
  block still resolves to the owning command
- No regression: `SectionAt`/`StatementAt`/`Symbols`/`References` tests unchanged
- `rms/.usages/rms-parsing.md` carries the appended section

**hints**
- `goga lint` passes for `hints/CODEMANIFEST`; no import of go.lsp.dev in the cell
- `XsAt` tests: kb function with optional param renders `[z: float]`; source
  decl (typed and untyped) wins over kb; conflicting source decls → silence;
  argIndex beyond declared params → Active=-1 (never clamp); trailing comma →
  next index
- `RmsAt` tests: `percent_chance` → label `percent_chance(%: percent 0..99)`;
  attribute active by name-match at correct combined index (len(Args)+attr
  position); `create_object` renders all 46 attributes; unknown attribute name
  → Active=-1; unknown command → silence
- `hints/.usages/computing.md` exists with the approved content

**server**
- `goga lint` passes for `server/CODEMANIFEST`; `goga contract` stays green
  for the whole project
- `Initialize` advertises SignatureHelpProvider with TriggerCharacters
  `["(", ","]` — disjoint from completion's `[" ", "<"]`
- `SignatureHelp` handler tests: silence → nil, nil; single signature,
  ActiveSignature=0; ActiveParameter nil when Active<0; embedded XsBlock
  position translated before `XsAt`
- SC8 sweep: fixture corpus diagnostics/hover/completion/navigation unchanged
- `server/.usages/lifecycle.md` section replaced as specified
