# Design Document: `master` — fix-review-defects-2 (№6–№11)

<!-- По task.md/arch.md (материализованные контрактные дельты rms/include).
Источник: docs/reviews/2026-09-08-full-review.md №6–№11. Решения ворот
приняты автономно (полномочие пользователя 2026-09-08). -->

## Contract Changes

- `rms/CODEMANIFEST`: Parse шаг 1 += строки при гашении комментариев
  (№11); шаг 5 += пустой inline-регион у #includeXS с аргументом не
  создаёт блок; bare — всегда (№7).
- `include/CODEMANIFEST`: Closure шаг 3 += обычный файл + граница
  корневой директории → иначе Missing (№9); Closure Constraints +=
  граница (№9); Definition Requirements += Target.URI данного запроса
  (№8).
- `include/.usages/closure.md` += 2 precondition.
- `server/CODEMANIFEST`: без правок — Definition Requirements уже
  требует конвертацию по positionEncoding (№6 — разрыв реализации).

## Code Stack Trace (ключевые цепочки)

### №7 — rms.Parse → directive/endXsBlock

1. `directive("#includeXS")` (parse.go:468): `includeArg ok` →
   XsIncludes += …; затем `p.inXs = true; p.xsStart = idx+1`.
   Аргументность нигде не сохраняется.
2. Терминатор (директива/секция/EOF) → `endXsBlock(idx)`: end ==
   xsStart (пустой регион) → Code="", Range нулевой ширины →
   фантомный узел в Symbols (kind=xs).
3. **Fix**: поле `xsArg bool` в parser (ставится в directive);
   endXsBlock: `if p.xsArg && end == p.xsStart { p.inXs=false; return }`.
   Bare-директива: xsArg=false → блок создаётся всегда (включая
   пустой в EOF — регрессия Task 1 сохраняется).

### №11 — rms.Parse → blankComments

1. `blankComments(lines, starts)` (parse.go:~715): по-символьный
   проход, `//` и `/*` гасятся без учёта кавычек.
2. `#include "a//b.rms"`: `//` внутри кавычек → хвост заменён
   пробелами; `includeArg` bare-ветка тянет Range до конца строки,
   Path искажён.
3. **Fix**: трекер `inString` (переключение на `"`), в строке
   маркеры комментариев не распознаются и символы не гасятся.
   Строки RMS однострочные (грамматика), межстрочных нет.

### №9 — include.Resolver → expandDirectives/definitionInRms

1. `expandDirectives` (resolver.go:104): `target := Join(Dir(owner),
   inc.Path)`; только `os.Stat`-проверка: директория проходит,
   `../`-побег читается.
2. **Fix**: общий хелпер `resolveTarget(ownerPath, rootDir, rel) →
   (targetPath string, ok bool)`: (а) `filepath.Rel(rootDir, target)`
   без `..`-префикса; (б) Stat ok и `Mode().IsRegular()`. Иначе —
   MissingInclude. rootDir = `filepath.Dir(корневой путь замыкания)`
   — прокидывается из Closure() через expand (новый параметр).
   Definition: definitionInRms резолвит ту же директиву — тот же
   хелпер (bound консистентен; found=false вне границы).
3. Closure root не на диске (editor-state) — rootDir из
   canonicalPath(uri) корня; пустой путь → bound не применяется
   (недисковый корень, поведение как прежде).

### №8 — include.Resolver → loadDisk/Definition

1. `loadDisk` кэширует `parseFile(uriArg, text)` по каноническому
   пути; cache-hit возвращает `cached.file` с uri ПЕРВОГО
   запросившего. `definitionInRms`/`expand` кладут этот uri в
   Target/Closure-entries → «чужое» написание при двух spelings
   (symlink, `sub/../x.rms` vs `x.rms`).
2. **Fix**: cache-hit возвращает лёгкую копию `file{uri: uriArg,
   text/rms/xs из кэша}` — AST read-only (инвариант), копия
   дешёвая; fresh-путь уже корректен.

### №10 — include.Resolver.References

1. Строки 328-329: `closureContains(ctx, u, uriArg)` (внутри
   Closure(u)) + `r.Closure(ctx, u)` — двойное вычисление замыкания
   каждого открытого includer'а.
2. **Fix**: `cl := r.Closure(ctx, u); if closureHas(cl, uriArg) {…}` —
   closureContains рефакторится в чистый `closureHas(c, target)`.
   Поведение идентично (существующие References-тесты — гарды).

### №6 — server → toProtocolPos/fromProtocolPos

1. `negotiateEncoding` выбирает utf-8, иначе utf-16 default — но
   режим нигде не хранится; конвертеры (server.go:810+) проксируют
   Column как байты. Кириллица: 1 символ = 2 байта = 1 UTF-16 юнит →
   сдвиг колонок правее на строке.
2. **Fix**: Server хранит режим (`utf16 bool`, default true;
   Initialize: utf-8 → false). Конвертеры становятся методами с
   текстом документа: byte→UTF-16 — подсчёт юнитов по префиксу
   строки (`utf16.RuneCountInString` семантика: пары сурогатов),
   UTF-16→byte — обратный линейный проход. Применение:
   - вход: fromProtocol для Hover/Completion/SignatureHelp/
     Definition/References — текст запрошенного документа;
   - выход: диагностики DidOpen/DidChange, DocumentSymbol —
     запрошенный документ; Definition/References Target'ы —
     текст из DocStore при открытом документе, иначе колонки
     как есть (байты; закодированное ограничение — кириллица на
     строке цели в неоткрытом файле; см. Risks).
3. utf-8 режим — прежнее поведение (без конвертации), все
   существующие тесты utf-8-клиента не меняются.

## Algorithm Design (сводно)

- `rms.parseDirective`: `p.xsArg = ok` (аргументная директива).
- `rms.endXsBlock`: пустой регион + xsArg → выход без блока.
- `rms.blankComments`: `inString` на кавычке; в строке — только
  поиск закрывающей кавычки.
- `include.resolveTarget(ownerPath, rootDir, rel) -> (string, bool)`:
  Join + Rel-bound + Stat + IsRegular.
- `include.loadDisk` cache-hit: копия file с uri данного запроса.
- `include.References`: замыкание includer'а один раз
  (`closureHas`).
- `server`: `utf16 atomic.Bool`; `s.toProtocolPos(text, pos)`,
  `s.fromProtocolPos(text, pos)` (+Range-обёртки) c линейной
  конвертацией колонки по строке `text`.

## Cross-cutting Concerns

- Ошибки: парсеры/резолвер — прежние инварианты (никаких panic,
  Missing как данные). №6: конвертация за границей строки —
  клампится в конец строки (защитно).
- Логирование: отсутствует (чистые преобразования).
- Concurrency: №6 — atomic.Bool (Initialize до запросов); resolver —
  существующий мьютекс; копия file в №8 под мьютексом чтения.
- Производительность: №10 — минус одно замыкание на includer'а;
  №6 — O(длина строки) на позицию, только utf-16 клиенты.

## Test Scenarios

- `TestParse_IncludeXSArgEmptyRegionNoBlock` (rms №7):
  `#includeXS lib/helpers.xs\n<land_generation>\ncreate_elevator 7\n`
  → 1 XsIncludes, 0 XsBlocks, 0 diags.
- Guard'ы №7: `TestParse_IncludeXSArgWithCodeKeepsBlock` (код после
  аргументной директивы → 1 блок, существующий dual-mode тест);
  bare-EOF тест Task 1 не меняется.
- `TestParse_StringLiteralKeepsCommentMarkers` (rms №11):
  `#include "a//b.rms"\n` → Path=="a//b.rms", Range End на закрывающей
  кавычке (не конец строки); строка с `/*` внутри. Guard:
  обычные `//`/`/* */` комментарии гасятся (существующие тесты).
- `TestResolver_EscapeBeyondRootMissing` (include №9): tmp-дерево
  `A/main.rms` + внешний существующий файл за пределами A;
  `#include ../outside.rms` → 1 Missing, 0 Resolved.
- `TestResolver_DirectoryTargetMissing` (№9): `#include sub`
  (директория) → Missing (не Resolved).
- Guard №9: include внутри поддерева root'а резолвится (существующие
  тесты).
- `TestResolver_TwoSpellingsOwnTargetURI` (№8): один файл, два
  написания (`x.rms` и `./x.rms` в разных owner-документах):
  Definition в каждом → Target.URI своего написания.
- №10: без новых тестов — рефакторинг, существующие References-тесты
  как гарды (в задаче плана — явная проверка запуска).
- `TestDocumentSymbol_Utf16Columns` (server №6): utf-16 клиент
  (General caps без utf-8): `/* Поколение */ create_elevator 7` →
  символ command: Start.Character == 15 (UTF-16), не 23 (байты).
- `TestHover_Utf16ClientPosition` (№6): клиент шлёт {0,15} →
  hover находит create_elevator (UTF-16→byte 23).
- Guard №6: utf-8 клиент — все позиции как прежде (существующие
  тесты).

## Risks

- №6 cross-file: колонки целей в неоткрытых файлах остаются
  байтовыми при utf-16 (маловероятный случай: не-ASCII на строке
  цели); зафиксировано как ограничение, отдельной задачей —
  проброс текста цели через Resolver, если проявится.
- №9: легитимные `../`-включения общих библиотек карт за пределами
  корня становятся Missing (принято: локальный инструмент,
  защита по умолчанию, контракт зафиксировал).

## Additional Instructions

- Подзадачи: (1) rms №7+№11; (2) include №8+№9+№10; (3) server №6.
- Тесты под memory cap; goimports/golangci-lint/goga lint/goga
  contract rms include server в каждой задаче.
- CODEMANIFEST read-only (дельты уже материализованы и валидны).
