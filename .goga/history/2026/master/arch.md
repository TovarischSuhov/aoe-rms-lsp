# Architecture Plan: fix-review-defects

## Topic

**fix-review-defects** — исправление дефектов №1–№5 ревью 2026-09-08
(`docs/reviews/2026-09-08-full-review.md`) в существующих ячейках
rms / xs / analysis (+ регрессионный тест в complete).

План: `.goga/history/2026/master/arch.md`
Задача: `.goga/history/2026/master/task.md`

Тип: **modification** — новых ячеек нет; все артефакты существуют,
правки контрактов — точечные дельты аннотаций (поведение начинает
однозначно следовать из контракта). План содержит только артефакты
CODEMANIFEST / .usages; кодовые правки компилируются следующим этапом
(goga-plan) из этих контрактов.

## Implementation Order

Порядок от листьев к корню (контрактная зависимость; кодовые подзадачи
внутри — по плану исполнения):

1. **`rms`** [modify] — зависит только от `common`; контракты нижних
   ячеек не нужны для его правки. Дельты контракта покрывают №1, №5.
2. **`xs`** [modify] — зависит только от `common`; sibling rms.
   Дельты контракта покрывают №2 (видимость), №4 (мультидекларации);
   инвариант №3 уже в контракте. Одна строка в consumer-usage.
3. **`analysis`** [modify] — зависит от `common`, `kb`, `rms`, `xs`:
   правится после уточнения xs (аннотация ссылается на семантику
   for-init из xs). Дельта покрывает №2 (Declare for-init).
4. **`complete`** [без изменений контрактов] — зависит от `common`,
   `kb`, `rms`, `xs`: контракт уже полон (XsAt принимает kind=local из
   VisibleAt); в исполнение идёт только регрессионный тест №2.

## Artifacts

### Cell: `rms` [modify]

**CODEMANIFEST `rms/CODEMANIFEST`** — diff (всё вне фрагментов
байт-в-байт неизменно):

Routine `Parse` annotations, Algorithm (отступы байт-в-байт как в
файле: шаг — 4 пробела, продолжение — 7):

```diff
     5. Директивы: #include с аргументом-путём → Include в Includes
        (Range = аргумент-путь); #includeXS с аргументом → Include в XsIncludes
        и начало XsBlock; bare #includeXS → только XsBlock; собирать строки
-       блока до следующей директивы/секции
+       блока до следующей директивы/секции или конца файла
```

```diff
     6. При ошибке: Diagnostic с диапазоном токена, sync на следующий
-       statement/секцию, продолжить разбор
+       statement/секцию, продолжить разбор; неявное закрытие незакрытого
+       вложенного блока (end_* при открытых внутренних) — Diagnostic
+       severity=warning
```

**.usages**: без изменений (`rms/.usages/rms-parsing.md`,
`rms/.usages/includes.md`).

### Cell: `xs` [modify]

**CODEMANIFEST `xs/CODEMANIFEST`** — diff (всё вне фрагментов
байт-в-байт неизменно):

Routine `XsParse` annotations, Algorithm, шаг 2:

```diff
-    2. Верхний уровень: объявления — functions (с Params), variables, rules
-       (условие+тело), events, include, extern
+    2. Верхний уровень: объявления — functions (с Params), variables
+       (мультидекларация int a = 1, b = 2; — N деклараций, по одной на
+       имя, Range каждой — по своей под-конструкции), rules (условие+тело),
+       events, include, extern
```

`XsFile.Definition` annotations, Algorithm, шаг 2:

```diff
       2. Среди деклараций файла, объявляющих это имя, взять самую внутреннюю,
-         чья область (тело функции/блока) объемлет позицию вхождения;
+         чья область (тело функции/блока или оператора for с его
+         init-декларацией) объемлет позицию вхождения;
          при равной вложенности — ближайшую, предшествующую вхождению
          (параметр/локаль затеняют топ-левел)
```

`XsFile.VisibleAt` annotations, Algorithm, шаг 3:

```diff
       3. Внутренняя функция, чьё тело объемлет `pos`: её параметры →
          kind=param; локальные декларации блоков её тела, объемлющих
-         `pos` (блоки { } по `xs_grammar`) → kind=local
+         `pos` (блоки { } по `xs_grammar`) → kind=local; init-декларация
+         оператора for (`for (int i = …)`) — локаль с областью всего
+         оператора for: видима в init, условии, шаге и теле; после
+         оператора не видна → kind=local
```

**.usages `xs/.usages/xs-parsing.md`** — diff: в разделе
«Visible symbols (completion)», блок Preconditions, после пункта о
shadowing добавить:

```diff
 - Shadowing is not resolved: an outer top-level `int x` and an inner
   `float x` both come back — deduplication policy belongs to the
   consumer.
+- For-loop init declarations (`for (int i = ...)`) are locals scoped to the
+  for statement: visible in the initializer, condition, step and body,
+  not after the statement.
 - `include` declarations are skipped (not name-bearing for completion).
```

### Cell: `analysis` [modify]

**CODEMANIFEST `analysis/CODEMANIFEST`** — diff (всё вне фрагментов
байт-в-байт неизменно):

`Analyzer.AnalyzeXs` annotations, Algorithm, шаг 3:

```diff
       3. Обойти объявления: при входе в тело функции — Push, объявить
          параметры (`XsParam`) и локальные переменные через Declare; при
-         выходе — Pop. ident вне объявленных и вне kb (Store.Function,
+         выходе — Pop; init-декларация оператора for — Declare в области
+         оператора for: Push перед разбором оператора, Pop после него.
+         ident вне объявленных и вне kb (Store.Function,
          Store.Constant) — code="undefined-symbol", severity=error
```

**.usages**: без изменений (`analysis/.usages/checks.md`,
`analysis/.usages/value-and-type-checks.md`).

### Cell: `complete` [без изменений]

**CODEMANIFEST `complete/CODEMANIFEST`**: без изменений — XsAt шаг 2
уже потребляет kind=local из VisibleAt; словарь Candidate полон.
**.usages**: без изменений (`complete/.usages/completing.md`).
В исполнение (не в этот план) — только регрессионный тест №2 в
`complete/completer_test.go`.

## Dependency Map

```
common ──(Pos, Range, Diagnostic, Symbol + usages)──> rms [modify]
common ──(Pos, Range, Diagnostic, Symbol + usages)──> xs  [modify]
kb ──────(Store, CommandArg + lookups)──────────────> analysis [modify]
rms ─────(RmsFile + rms-parsing)────────────────────> analysis [modify]
xs ──────(XsFile, Decl, Expr, Param + xs-parsing)───> analysis [modify]
kb ──────(Store, Function, … + lookups)─────────────> complete [no Δ]
xs ──────(XsFile, Decl + xs-parsing)────────────────> complete [no Δ]
rms ─────(RmsFile, ArgSite, … + rms-parsing)────────> complete [no Δ]
common ──(Pos, Symbol + usages)─────────────────────> complete [no Δ]
```

Новые рёбра Imports: **нет** (сигнатуры неизменны). Циклов нет.

## Verification Checklist

После применения артефактов этого плана:

1. **DSL-валидность**: `goga lint` — 0 ошибок по всем 9 ячейкам
2. **Соответствие контрактов реализации** (после кодовых задач
   следующего этапа): `goga contract rms`, `goga contract xs`,
   `goga contract analysis`, `goga contract complete` — все exit 0
3. **Точечность правок**: `git diff` по трём CODEMANIFEST и одному
   usage-файлу содержит только фрагменты из этого плана (сигнатуры,
   properties, методы, Imports/Usages-заголовки — нетронуты)
4. **Referential integrity**: новые backtick-ссылки в аннотациях
   (`xs_grammar`, `XsParam` и др.) разрешаются в контексте своих
   документов; goga lint это проверяет
5. После кодового этапа: репро №1–№5 из
   `docs/reviews/2026-09-08-full-review.md` дают корректный результат
   (регрессионные тесты зелёные под memory cap)
