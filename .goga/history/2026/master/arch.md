# Architecture Plan: fix-review-defects-2 (№6–№11)

## Topic

**fix-review-defects-2** — находки №6–№11 ревью 2026-09-08:
utf-16 позиции (server), фантомный XsBlock и строки в blankComments
(rms), URI кэша/границы замыкания/двойной Closure (include).

План: `.goga/history/2026/master/arch.md`; задача: `task.md`.
Тип: **modification** — 0 новых ячеек/типов; 2 CODEMANIFEST с
минимальными аннотационными дельтами (rms, include); server — без
контрактных правок (требование уже в контракте).

Решения ворот (автономно, полномочие пользователя 2026-09-08):
№6 — внутренние хелперы server; №7 — skip только для директивы с
аргументом; №9 — граница = директория корневого документа замыкания.

## Implementation Order

1. **`rms`** [modify] — лист (deps: common). №7 + №11.
2. **`include`** [modify] — deps: common, rms, xs. №8 + №9 + №10.
3. **`server`** [modify] — корень. №6 (контракт не меняется).

## Artifacts

### Cell: `rms` [modify]

**CODEMANIFEST `rms/CODEMANIFEST`** — diff (вне фрагментов байт-в-байт;
отступы: шаг — 4 пробела, продолжение — 7):

Routine `Parse`, Algorithm, шаг 1 (№11):

```diff
     1. Лексер: токены по `rms_grammar` (слова, числа, проценты, строки,
-       комментарии, директивы #include/#includeXS, секции <...>)
+       комментарии, директивы #include/#includeXS, секции <...>);
+       гашение комментариев не затрагивает содержимое строковых
+       литералов (кавычки учитываются)
```

Routine `Parse`, Algorithm, шаг 5 (№7):

```diff
     5. Директивы: #include с аргументом-путём → Include в Includes
        (Range = аргумент-путь); #includeXS с аргументом → Include в XsIncludes
-       и начало XsBlock; bare #includeXS → только XsBlock; собирать строки
-       блока до следующей директивы/секции или конца файла
+       и начало XsBlock; bare #includeXS → только XsBlock; собирать строки
+       блока до следующей директивы/секции или конца файла; пустой
+       inline-регион (терминатор на следующей же строке) у #includeXS
+       с аргументом XsBlock не создаёт; bare #includeXS создаёт блок
+       всегда (включая пустой в конце файла)
```

**.usages**: без изменений.

### Cell: `include` [modify]

**CODEMANIFEST `include/CODEMANIFEST`** — diff (вне фрагментов
байт-в-байт; отступы метод-Algorithm: шаг — 6, продолжение — 9):

`Resolver.Closure` Algorithm, шаг 3 (№9):

```diff
      3. Для каждого RmsEntry: Includes и XsIncludes по `includes` — резолв
         относительно директории owner-файла по `lsp-protocol`; файл
-        существует → ResolvedInclude и target в очередь; нет →
-        MissingInclude
+        существует, является обычным файлом и лежит внутри директории
+        корневого документа → ResolvedInclude и target в очередь;
+        иначе (нет файла, директория, выход за границу) → MissingInclude
```

`Resolver.Closure` Constraints (№9) — добавить пункт:

```diff
     Constraints:
     - inline XsBlocks в замыкание не входят (часть owner-парза)
     - XS include-декларации не разворачиваются
+    - резолв не покидает директорию корневого документа (защита от
+      ../-побега за пределы рабочей области карты)
```

`Resolver.Definition` Requirements (№8) — добавить пункт:

```diff
     Requirements:
     - детерминированность: одинаковый вход → одинаковый результат
+    - Target.URI — написание цели из резолва данного запроса;
+      канонический путь — только ключ кэша/visit-set
```

№10 (двойной Closure в References) — контракт не меняется:
Algorithm уже описывает один обход; правка реализации.

**.usages `include/.usages/closure.md`** — diff: добавить в конец
блока Preconditions (или Notes по фактической структуре файла):

```diff
+- Resolution stays inside the root document's directory; escapes
+  (`../`) and directory targets become MissingInclude entries.
+- Target.URI spells the path as resolved for the current query; the
+  canonical path is only the cache key.
```

### Cell: `server` [modify]

**CODEMANIFEST**: без правок — Definition Requirements «конвертация
позиций — с учётом согласованного positionEncoding» уже покрывает №6.
**.usages**: без изменений. Правка: внутренние хелперы
byte↔UTF-16 конвертации колонок при utf-16-режиме.

## Dependency Map

```
common ──> rms [modify №7№11] ──> include [modify №8№9№10] ──> server [modify №6]
common ──> xs ──────────────────────> include, server
```

Новых рёбер Imports нет; циклов нет.

## Verification Checklist

1. `goga lint` — 0 ошибок по 9 ячейкам после диффов
2. `goga contract rms include server` — exit 0
3. `git diff` по CODEMANIFEST содержит только фрагменты плана
4. После кодового этапа: репро №6–№11 зелёные (регрессионные тесты),
   memory-cap `go test ./...`, goimports, golangci-lint
