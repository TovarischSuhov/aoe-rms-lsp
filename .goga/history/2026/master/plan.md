# Plan: `master` — fix-review-defects-2 (№6–№11)

<!-- По design.md; контракты материализованы и валидны (goga lint 0).
Задача: task.md. Источник: docs/reviews/2026-09-08-full-review.md. -->

## Purpose

Исправить находки №6–№11 ревью 2026-09-08: utf-16 позиции server,
фантомный XsBlock и строки в blankComments (rms), граница замыкания/
URI-написание/двойной Closure (include). Только код и тесты; контракты
уже материализованы.

## Tasks

> Порядок: rms → include → server (листья → корень). Каждая задача —
> своя ветка task/*, TDD (контракт-тесты первыми), PR в master.

### Task 1: rms — пустой inline-регион #includeXS и строки в blankComments (№7, №11) (TDD)

`rms/parse.go`. №7: `directive` сохраняет аргументность
(`p.xsArg`); `endXsBlock` при `xsArg && end == xsStart` выходит без
блока (bare-директива блок создаёт всегда — регресс-тест Task 1
batch-1 не меняется). №11: `blankComments` — трекер `inString`
(кавычка переключает); внутри строки маркеры комментариев не
распознаются, символы не гасятся. Контракты: Parse шаг 1, шаг 5.

**CRITICAL: `CODEMANIFEST` read-only.**

- [ ] STEP 0: объявить задачу
- [ ] STEP 1 (контракт-тесты, упадут): `TestParse_IncludeXSArgEmptyRegionNoBlock` — аргументная директива + сразу секция → 1 XsIncludes, 0 XsBlocks, 0 diags; `TestParse_StringLiteralKeepsCommentMarkers` — `#include "a//b.rms"` → Path=="a//b.rms", Range.End == позиция закрывающей кавычки
- [ ] STEP 2 (код): xsArg + guard в endXsBlock; inString в blankComments
- [ ] STEP 3: `go test ./rms/ -run 'TestParse_(IncludeXSArgEmptyRegionNoBlock|StringLiteralKeepsCommentMarkers)' -count=1` — зелёные
- [ ] STEP 4 (logic-тесты): guard аргументной директивы с кодом (1 блок — можно расширить существующий dual-mode тест проверкой отсутствия второго пустого блока); строка с `/*` внутри кавычек не гасится
- [ ] STEP 5 (debug): memory-cap `go test ./... -count=1` — весь модуль
- [ ] STEP 6: контракт-реверификация: Parse шаги 1/5; bare-EOF регресс зелёный
- [ ] STEP 7: `goimports -w rms/`; `golangci-lint run rms/...`
- [ ] STEP 8: отметить чекбоксы → REVIEW → APPROVAL → NEXT TASK

### Task 2: include — граница корня, URI запроса, один Closure (№8, №9, №10) (TDD)

`include/resolver.go`. №9: хелпер `resolveTarget(ownerPath, rootDir,
rel) → (target, ok)`: filepath.Rel без `..` + Stat + IsRegular; rootDir
прокинут из Closure() через expand/expandDirectives; definitionInRms
использует тот же хелпер (вне границы found=false). №8: loadDisk
cache-hit возвращает `file{uri: uriArg, …}` поверх кэшированных AST.
№10: References — `cl := r.Closure(ctx, u)` один раз; closureContains →
чистый `closureHas(c, target)`. Контракты: Closure шаг 3/Constraints,
Definition Requirements.

**CRITICAL: `CODEMANIFEST` read-only.**

- [ ] STEP 0: объявить задачу
- [ ] STEP 1 (контракт-тесты, упадут): `TestResolver_EscapeBeyondRootMissing` — tmp-дерево: root в A/, существующий файл за A; `#include ../outside.rms` → 1 Missing, 0 Resolved; `TestResolver_TwoSpellingsOwnTargetURI` — один файл, owners с `x.rms` и `./x.rms` → каждый Definition возвращает свой URI
- [ ] STEP 2 (код): resolveTarget + прокидка rootDir; копия file в loadDisk; closureHas
- [ ] STEP 3: тесты STEP 1 зелёные
- [ ] STEP 4 (logic-тесты): `TestResolver_DirectoryTargetMissing` — цель-директория → Missing; guard — include в поддереве root'а резолвится; References-тесты существующие зелёные (№10)
- [ ] STEP 5 (debug): memory-cap `go test ./... -count=1`
- [ ] STEP 6: контракт-реверификация (Closure/Definition шаги)
- [ ] STEP 7: `goimports -w include/`; `golangci-lint run include/...`
- [ ] STEP 8: отметить чекбоксы → REVIEW → APPROVAL → NEXT TASK

### Task 3: server — byte↔UTF-16 позиции при positionEncoding=utf-16 (№6) (TDD)

`server/server.go`. Server хранит режим (`utf16`, default true;
Initialize: utf-8 → false). Конвертеры-методы над текстом документа:
byte→UTF-16 колонка (префикс строки, суррогатные пары), обратный —
линейный проход; кламп в конец строки за границей. Применение: вход
всех позиционных хендлеров (текст запрошенного документа), выход —
диагностики, DocumentSymbol; Target'ы Definition/References — при
открытом в DocStore документе. utf-8 — прежнее поведение (все
существующие тесты — guard'ы). Контракт: без правок.

**CRITICAL: `CODEMANIFEST` read-only.**

- [ ] STEP 0: объявить задачу
- [ ] STEP 1 (контракт-тесты, упадут): `TestDocumentSymbol_Utf16Columns` — utf-16 клиент, `/* Поколение */ create_elevator 7` → Start.Character == 15; `TestHover_Utf16ClientPosition` — клиент {0,15} → hover по create_elevator
- [ ] STEP 2 (код): поле режима + конвертеры + проводка в хендлерах
- [ ] STEP 3: тесты STEP 1 зелёные
- [ ] STEP 4 (logic-тесты): диагностики utf-16 (диапазон с кириллицей); beyond-end кламп; utf-8 клиент — все существующие тесты зелёные
- [ ] STEP 5 (debug): memory-cap `go test ./... -count=1`
- [ ] STEP 6: контракт-реверификация: Definition Requirements «с учётом positionEncoding» выполнен
- [ ] STEP 7: `goimports -w server/`; `golangci-lint run server/...`
- [ ] STEP 8: отметить чекбоксы → REVIEW → APPROVAL → PLAN COMPLETE

## Validation Commands

- `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`: все тесты
- `goimports -w .`; `golangci-lint run`; `goga lint`;
  `goga contract rms include server` — все exit 0

## Completion Criteria

- [ ] Репро №6–№11 дают корректный результат, каждое — тест
- [ ] Существующие тесты не ломаются (utf-8 клиенты, bare-EOF блок,
      обычные комментарии, include в поддереве)
- [ ] Каждая задача TDD (шаги 0–8); ветки/PR по протоколу
- [ ] CODEMANIFEST не модифицировались
