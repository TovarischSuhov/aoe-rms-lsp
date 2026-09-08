# Миграция проекта на правила CLAUDE.md

## Current State

Три аудита (инфраструктура/доки, Go-код×conventions, CODEMANIFEST×DSL) от 2026-09-09.
База здорова: `goga lint` зелёный (9 ячеек, 0 ошибок), контракты синхронны коду
(`goga contract`), doc-комментарии на всех экспортах, `%w`-обёртки везде,
context/DI/imports чистые, CI гоняет race+lint+govulncheck+goimports.

Отклонения от правил (правило приоритета: **CLAUDE.md > conventions.md > производные**):

**Инфраструктура/доки:**
- CI не гейтит gofumpt (`golangci-lint fmt --diff` отсутствует) — при правиле CLAUDE.md «Формат — golangci-lint fmt»
- README отстаёт: не указаны definition/references/document symbols/signature help; layout-блок без `cmd/`, `hints/`, `complete/`, `include/`; формулировка «release tag pushed by CI» неточна
- «Go 1.23+» в CLAUDE.md/README/conventions.md/cooks/lsp-protocol.md против `go 1.26.6` в go.mod
- CLAUDE.md объявляет корневой `testdata/` — его нет; фикстуры пер-ячеечные (`rms/testdata`, `xs/testdata`)
- Авторитетный план `docs/plans/build-lsp-rms-xs.md`: 38 неотмеченных чекбоксов при сделанной работе; описывает 6 ячеек из 9
- `docs/tasks/*.md` (5 файлов) без статусов завершения, «Current State» местами протух
- release.yml отклоняется от паттерна matrix из cooks/github-actions.md (задокументированное отклонение, cook не благословляет)

**Go-код:**
- `t.Parallel()` отсутствует во всех 25 тест-файлах (~250 функций)
- `cmd/kbgen` толстый: ~310 строк логики генерации + дублирует 3 regexp из `kb` — против «cmd/ — тонкие entrypoints»
- `_ = client.PublishDiagnostics(...)` (server/server.go:661) — отбрасывание ошибки
- глобальный `slog` в ячейке kb (extract.go, 3 вызова) — неинжектированный логгер
- ошибка без контекста (kb/extract.go:99); сравнение текста ошибки в тесте (kb/extract_test.go:66)
- битая строка doc-комментария (server/server.go:350 — задвоение)
- 5 мест вложенности 4–5 уровней (analysis/analyzer.go:84,249; xs/ast.go:427,549; xs/parse.go:275)
- `TestCoerce_Table` — не сценарий в нейминге

**CODEMANIFEST/.usages (DSL/cookbook):**
- дубли аннотаций: параметры конструктора описаны и в type-аннотации, и в property-аннотациях (10 типов: common Pos/Range/Diagnostic/Symbol, kb ValueRange, rms ArgSite/Include, xs CallSite, hints Hint, complete Candidate)
- код-синтаксис в `Algorithm:` шагах: server/CODEMANIFEST (~6 мест), xs/CODEMANIFEST:153-154, analysis/CODEMANIFEST:72-75 (погранично)
- `.usages` битые имена: closure.md `server.NewStore` → `NewDocStore`; positions-and-diagnostics.md `xs.Parse` → `XsParse`
- кросс-ссылки между практиками: kb/lookups.md:9 (`conventions`), server/lifecycle.md:23 (`lsp-protocol`), lifecycle.md:17 («see task README»)
- rms/includes.md: секция by-name references — чужой домен + дублирует rms-parsing.md
- kb/lookups.md:41-47 предписывает рендер ячейки hints; hints/computing.md и complete/completing.md предписывают поведение server — обязательство должно жить в одной месте (server manifest)

## Description

Одна задача-миграция: привести весь проект (код, контракты, практики, CI, доки)
в соответствие с CLAUDE.md как единственным авторитетным сводом правил,
разрешая коллизии правил в пользу CLAUDE.md и правя тексты документов под
реальность (а не наоборот). Результат — один PR в master.

## Scope

**In scope:**
- CI: добавить гейт форматирования `golangci-lint fmt --diff` (enforcement правила CLAUDE.md)
- README: актуализировать фичи (definition, references, document symbols, signature help), layout-блок, формулировку про релизы, версию Go
- Версия Go в текстах: CLAUDE.md, conventions.md, cooks/lsp-protocol.md → «Go 1.26+» (под go.mod 1.26.6)
- conventions.md: добавить строку приоритета «при противоречии с CLAUDE.md приоритет у CLAUDE.md»
- CLAUDE.md: уточнить `testdata/` → пер-ячеечные фикстуры (`<cell>/testdata/`)
- docs/plans/build-lsp-rms-xs.md: отметить выполненные чекбоксы, пометить план реализованным, сослаться на задачи-продолжения (9 ячеек)
- docs/tasks/*.md: проставить статусы завершения (Done + PR-ссылки)
- cooks/github-actions.md: благословить single-job вариант release-воркфлоу (соответствует фактической реализации)
- kb: перенести пайплайн генерации из cmd/kbgen в ячейку (kb/gen.go, экспортная точка вида GenKB), устранить дублирование regexp; cmd/kbgen — тонкий (флаги + вызов)
- kb: `ExtractRmsCommands` — инжектированный `*slog.Logger` вместо глобального slog; обновить CODEMANIFEST и вызовы
- server/server.go:661 — логировать ошибку PublishDiagnostics (WARN), не отбрасывать
- kb/extract.go:99 — контекст в ошибку; kb/extract_test.go:66 — `require.ErrorContains`
- server/server.go:350 — убрать задвоенную строку doc-комментария
- Рефакторинг вложенности: 5 мест → извлечь хелперы (analysis/analyzer.go collectLocals, walkStmts; xs/ast.go collectLocals, appendLocals; xs/parse.go parseEvent)
- Тесты: добавить `t.Parallel()` в тесты без общего состояния (чистые пакеты: common, rms, xs, analysis, kb, complete, hints; не трогать server-harness)
- analysis/types_test.go: `TestCoerce_Table` → имя-сценарий
- CODEMANIFEST: убрать дубли «параметр-в-type-аннотации ↔ property-аннотация» (10 типов, информация не теряется — сливается в property-аннотации)
- CODEMANIFEST: переписать код-шаги `Algorithm:` в язык действий (server, xs, analysis)
- .usages: исправить `server.NewDocStore`, `xs.XsParse`; убрать кросс-ссылки на практики (lookups.md, lifecycle.md ×2); удалить дублирующую секцию из rms/includes.md; триминг чужих обязательств (kb/lookups.md §render, hints/computing.md, complete/completing.md — маппинги nil остаются обязательством server manifest)

**Out of scope:**
- Бенчмарки/integration-теги в CI (требование только conventions.md — по приоритету CLAUDE.md не требуется)
- Переделка release.yml на matrix (отклонение задокументировано, cook благословляется)
- Понижение go.mod до 1.23
- Подключение «осиротевших» практик (data-pipeline, value-and-type-checks, lifecycle) импортами — нет естественных ячеек-потребителей; остаются maintenance-доками
- Переименование ключей `rms_grammar`/`xs_grammar` (правило cookbook — про cell-level usages)
- Любые функциональные изменения поведения LSP-сервера

## Acceptance Criteria

- `golangci-lint fmt --diff` пуст; в CI добавлен соответствующий шаг и он зелёный (локально: синтаксис шага валиден)
- memory-cap `go test ./... -count=1` зелёный; `golangci-lint run` зелёный; `goga lint` 0 ошибок; `goga contract` по затронутым ячейкам без новых расхождений
- В коде нет `_ =` на ошибках (кроме документированных не-error discard) и глобального slog в ячейках
- cmd/kbgen ≤ ~50 строк, без бизнес-логики; логика генерации в kb, regexp не дублируются
- Все чистые тесты отмечены `t.Parallel()`; тесты гоняются зелёно (в т.ч. повторно, без флака)
- В CODEMANIFEST нет дублированных описаний параметров и код-синтаксиса в `Algorithm:`
- `.usages` без кросс-ссылок на практики и битых имён сущностей
- Доки (README, CLAUDE.md, conventions.md, план, задачи) соответствуют реальности: версия Go, фичи, layout, статусы, чекбоксы
- Один PR в master из ветки `task/<имя>`; атомарные коммиты по ячейкам/темам

## Stack

- **Frameworks:** Go 1.26 (stdlib), testify (существующие)
- **Libraries:** go.lsp.dev/{protocol,jsonrpc2,uri} (существующие), log/slog
- **Infrastructure:** GitHub Actions (ci.yml — добавить fmt-гейт), golangci-lint v2 (gofumpt), goga DSL

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| github-actions | `.goga/usages/cooks/github-actions.md` | updated (благословение single-job release) |
| lsp-protocol | `.goga/usages/cooks/lsp-protocol.md` | updated (версия Go) |

Новых внешних зависимостей нет; `go get` не требуется.

## Risks and Constraints

- `t.Parallel()` массово — риск флака при скрытом общем состоянии; митигируется выборочным внесением (только чистые тесты) и повторным прогоном под memory-cap
- Перенос генерации в kb меняет публичную поверхность ячейки → CODEMANIFEST kb обновляется вместе с кодом (контракт и код в одном коммите)
- Рефакторинги вложенности — behavior-preserving, покрываются существующими тестами
- Тесты только под memory-cap (systemd-run MemoryMax=1500M)
- CLAUDE.md правится минимально (версия Go, testdata) — как приведение текста к истине, не смена правил

## Scope Estimate

Одна задача (по требованию пользователя), один PR. Внутри — три связных пласта
(инфра/доки, код, контракты/практики), коммиты атомарны по ячейкам/темам.

## Existing Architecture

Ячейки-мишени контрактов: kb (перенос GenKB + сигнатура ExtractRmsCommands),
server/xs/analysis (аннотации Algorithm), common/rms/hints/complete (дубли
аннотаций). Практики `.usages`: kb/lookups, server/lifecycle, include/closure,
common/positions-and-diagnostics, rms/includes, hints/computing,
complete/completing. Код: server, kb, analysis, xs, cmd/kbgen. Инфра: ci.yml,
README.md, CLAUDE.md, conventions.md, cooks, docs/plans, docs/tasks.

## Notes

- Правило приоритета правил задано пользователем: CLAUDE.md > conventions.md.
  Бенчмарки в CI не делаем (требование только conventions.md).
- Аудиты: 3 фоновых Explore-агента 2026-09-09, полные отчёты в сессии.
- Memory `go-tests-under-memory-cap` восстановлена (CLAUDE.md ссылка валидна).
- Решения по спорным пунктам приняты «рекомендованные» (пользователь недоступен).
