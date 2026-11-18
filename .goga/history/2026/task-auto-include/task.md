# auto-include: completion из доступных include + quickfix (#116 + #117, зонтик #80)

Слоты пачки ux-wave-2 (#80, разбит 2026-09-22). #117 «builds on the
completion slot» — один PR, два атомарных коммита (паттерн
map-structure PR #193).

## Current State

- `Completer.XsAt(file, pos, external)` предлагает только видимые
  символы + декларации **уже подключённого** замыкания + kb. Символ из
  доступного, но не подключённого файла не предлагается — `#include`
  пишется руками после unknown-symbol
- `Resolver` знает roots (`SetRoots`, из `aoe2lsp.includeRoots`) и
  держит дисковый кэш с stat-fingerprint; `parseFile` строит AST по
  суффиксу; скан-списка доступных файлов нет
- Quickfix-механика server: CodeAction строится из диагностики в
  контексте (`renameAction` парсит имя из message через `suggestedName`)
- `CompletionItem.AdditionalTextEdits` — LSP-механизм автоприменения
  правок при принятии кандидата (вставка include без ручных шагов)

## Description

**include** — `Resolver.Available(ctx, uriArg) []AvailableEntry`:
скан директории документа + roots на `*.xs` (case-insensitive),
исключая сам документ и членов текущего замыкания; декларации — через
существующий `load()` (editor-state приоритетен, диск кэширован по
stat-fingerprint — readdir дёшев, parse кэширован). `AvailableEntry`
несёт URI, include-аргумент (путь относительно директории документа,
иначе как есть) и верхнеуровневые декларации.

**complete (#116)** — `Candidate.Include` (путь источника для вставки
директивы; пусто у обычных кандидатов); `XsAt` принимает
`[]AvailableFile{Path, Decls}` — кандидаты из них с пометкой источника
(detail — сигнатура + файл), Sort-группа после source-символов.

**server** — wiring: completion .xs и inline-XS прокидывают available;
`toCompletionItems` с `Include != ""` ставит `AdditionalTextEdits` —
вставка `#include "path"` (standalone .xs) / `#includeXS "path"`
(inline-XS) в начало документа. **(#117)**: CodeAction на
`undefined-symbol` — имя из message, available-скан: ровно один
файл-владелец декларации → «Add include 'path'» (та же вставка);
0 или ≥2 кандидатов — молчание.

## Acceptance

- make check; тесты: include.Available (скан/исключения/кэш-инвалидация
  через Drop?), complete-кандидаты с Include, server completion с
  AdditionalTextEdits, quickfix 1-кандидат/мульти-тишина
- Верификация в ручном сценарии не выполняется (машина без VS Code
  отладки); корректность — юнит-тестами протокола

## Result

**Слот 1 — internal/include + internal/complete (#116):**
- `Resolver.Available(ctx, uri)` — скан директории документа + roots на
  `*.xs` (dedupe по canonical path), исключая документ и членов
  замыкания; `AvailableEntry{URI, Path, Decls}` — путь
  релативизован от документа (`..`-эскейп → абсолют); декларации через
  существующий `load()` (editor-state приоритетен, диск кэширован);
  namedDecls — function/variable/extern
- `Candidate.Include` (путь источника) + `AvailableFile{Path, Decls}` в
  `Completer.XsAt` (4-й параметр); Sort-группа available — «1» (между
  source «0» и kb-функциями «2»; kb-константы «3») — пины сдвинуты
  планово; in-scope имена выигрывают у available; rules/events не
  адресуемы

**Слот 2 — internal/server (#116 + #117):**
- completion .xs → `#include`, inline-XS в .rms → `#includeXS` —
  AdditionalTextEdits, zero-width вставка в начало документа
- `addIncludeAction` на `undefined-symbol`: имя из message
  (`undefinedSymbolName`), available-скан — **ровно один** владелец →
  «Add include 'path'»; 0/≥2 — тишина (двух-владельцевый тест)

**Проверки:** 15/15 пакетов ok, lint 0, corpus harness 100/100 ok
panic 0. **Тесты:** TestResolver_Available (6 кейсов: относительные
пути, замыкание-исключение, roots, декларации, editor-state,
non-file URI), TestXsAt_AvailableFiles, TestToCompletionItems_AutoInclude,
TestServerCodeAction_AddIncludeFix (single/two-owners)
**Находка:** `codeActionAt`-диагностика с Start>End падает в
rangesOverlap — тестовые диагностики несут корректный End
