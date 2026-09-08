# Plan: `claudemd-compliance-migration` (кодовая и инфраструктурная фаза)

## Purpose

Реализовать кодовую и инфраструктурную часть миграции проекта на правила CLAUDE.md.
Контрактная часть уже материализована (коммит 0e45c4c: kb `GenKB` +
`ExtractRmsCommands(log)`, аннотационные чистки, `.usages`). Этот план закрывает:
перенос генерации данных в kb, точечные дефекты server, рефакторинги вложенности,
`t.Parallel()` в чистых тестах, CI fmt-гейт, актуализацию доков. Стратегия —
behavior-preserving изменения с зелёными тестами после каждого шага.

## Context

### Contract Surface

**Entity: `GenKB`** (kb, Routine)
- Declared `location`: `gen.go` — файла ещё нет
- Facade: `kb.GenKB` импортируем из пакета kb
- Сигнатура: `(refDir: string, dataDir: string, log: Logger) -> err: error`;
  в Go — `func GenKB(refDir, dataDir string, log *slog.Logger) error`
- Поведение (аннотация): прочитать источники по `kbdata` (functions/constants
  JSON ugc-guide; команды — `ExtractRmsCommands`(гайд, `log`)); пополнить
  SinceUpdate по changelog; проверить уникальность имён; записать три JSON в
  `dataDir`. Requirements: детерминированная сериализация. Constraints: утилита
  сборки; `refDir` не изменяется.

**Entity: `ExtractRmsCommands`** (kb, Routine — модификация)
- Declared `location`: `extract.go` — существует
- Новая сигнатура: `(path: string, log: Logger) -> commands: []Command, err: error`;
  Go: `func ExtractRmsCommands(path string, log *slog.Logger) ([]Command, error)`;
  `log == nil → slog.Default()`; WARN при пропуске неоднозначных фрагментов — в `log`.

Прочие контракты не меняются; задачи 2–6 не затрагивают CODEMANIFEST.

### Usages Context

- `conventions` (.goga/usages/conventions.md): код-стиль, тесты, ошибки `%w`,
  doc-комментарии. Приоритет: при противоречии с CLAUDE.md — CLAUDE.md.
- `kbdata` (inline в kb/CODEMANIFEST): layout трёх JSON + правила mining —
  источник путей для `GenKB`.

### External Dependencies

- stdlib: `log/slog`, `encoding/json`, `os`, `path/filepath` — новых модулей нет.

## Facts

- Тесты запускать ТОЛЬКО под memory-cap:
  `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`
- `cmd/kbgen/main.go:118-424` — переносимая логика (`run`, `genFunctions`,
  `genConstants`, `parseUpdates`, `sinceByFunction`, `sinceByConstant`,
  `rawValue`, `writeJSON`, `sortedKeys`); regexp-дубли:
  `updateHeadingRe/backtickRe/wordRe` (kbgen:22-31 ≡ kb/extract.go:25-35).
- server/server.go:350 — задвоенная строка doc-комментария; :661 —
  `_ = client.PublishDiagnostics(...)`.
- Вложенность: analysis/analyzer.go:84 (`walkStmts`), :249 (`collectLocals`);
  xs/ast.go:427 (`collectLocals`), :549 (`appendLocals`); xs/parse.go:275 (`parseEvent`).
- `TestCoerce_Table` — analysis/types_test.go:23.
- CI (.github/workflows/ci.yml): goimports-гейт, `-race`, golangci-action v2.13.2,
  govulncheck. fmt-гейта нет.
- Тексты с «Go 1.23+»: CLAUDE.md:3, conventions.md:19/246/343,
  cooks/lsp-protocol.md:266, README:74; go.mod: `go 1.26.6`.
- docs/plans/build-lsp-rms-xs.md: 38 `[ ]`, 0 `[x]`. docs/tasks/: 5 файлов без статусов.
- CLAUDE.md:53 объявляет `testdata/` — фактически фикстуры в rms/testdata, xs/testdata.

## Gap Analysis

- `kb.GenKB` — отсутствует (implementation: null в goga contract)
- `ExtractRmsCommands` — сигнатура без log (дрейф контракту)
- cmd/kbgen — толстый (~310 строк логики), дублирует regexp-ы kb
- server — `_ =` на ошибке; битый doc-комментарий
- 5 блоков вложенности 4–5 уровней — против стиля conventions
- `t.Parallel()` — 0/25 тест-файлов
- CI не гейтит gofumpt
- Доки отстают от реальности (фичи, layout, версия Go, статусы, чекбоксы)

---

## Tasks

### Task 1: kb — `GenKB`, логгер в `ExtractRmsCommands`, тонкий cmd/kbgen (TDD coding)

Контракт kb (CODEMANIFEST — read-only): `GenKB(refDir, dataDir, log) -> err`
в `gen.go`; `ExtractRmsCommands(path, log)` в `extract.go`. Источник логики —
`cmd/kbgen/main.go` (перенос, не копирование: regexp-ы уже в kb/extract.go).
CLI-контракт kbgen (флаги) сохранить.

**Usages:** `conventions` (%w, doc-комментарии, table-driven тесты);
`kbdata` (пути источников и формат выходных JSON).

**CRITICAL: `CODEMANIFEST` — read-only. Несоответствие чинится в коде.**

- [ ] **Contract tests**: `kb/gen_test.go` — table-driven `TestGenKB_*`:
  мини-источники в `t.TempDir()` → три JSON в `dataDir`; проверка имён файлов,
  уникальности (дубль имени → error), детерминированности (второй запуск —
  идентичные байты); `t.Parallel()` (ожидаемо падают — gen.go нет)
- [ ] **Code**: `kb/extract.go` — `ExtractRmsCommands(path string, log *slog.Logger)`;
  nil→`slog.Default()`; log-threading в `parseSkeleton/placeholderKind/readChangelog`;
  контекст в ошибку «heading not found» (бывш. :99)
- [ ] **Code**: `kb/gen.go` — перенос пайплайна из cmd/kbgen (`run/genFunctions/
  genConstants/parseUpdates/sinceBy*/rawValue/writeJSON/sortedKeys`), экспорт
  `GenKB`; переиспользование regexp-ов extract.go; doc-комментарий
- [ ] **Code**: `cmd/kbgen/main.go` — только флаги + slog + `kb.GenKB(...)` (≤~50 строк)
- [ ] **Interface verification**: `go build ./...`; `goga contract kb` —
  `GenKB.implementation` != null; `ExtractRmsCommands` пара `(path, log)`
- [ ] **Logic tests**: ошибочные пути (нет functions.json → error с путём;
  пустой changelog → since_update=""), позитив (мини-фикстуры)
- [ ] **Debugging**: memory-cap `go test ./... -count=1` — зелёный
- [ ] **Contract re-verification**: сигнатуры/фасад соответствуют CODEMANIFEST
- [ ] **Lint**: `golangci-lint fmt && golangci-lint run`
- [ ] Коммит: `feat: kb cell — GenKB (перенос генерации из cmd/kbgen), логгер в ExtractRmsCommands`

### Task 2: server — ошибка PublishDiagnostics и doc-комментарий (coding)

server/server.go: два точечных дефекта из аудита. Контракт не меняется
(методы/поведение прежние) — контрактная проверка: сборка + существующие тесты.

**Usages:** `conventions` (каждая ошибка обработана; doc-комментарий корректен).

**CRITICAL: `CODEMANIFEST` — read-only.**

- [ ] **Code**: server.go:350 — удалить задвоенную строку doc-комментария `Completion`
- [ ] **Code**: server.go:661 — `if err := client.PublishDiagnostics(ctx, ...); err != nil { slog.WarnContext(ctx, "publish diagnostics", "uri", uri, "err", err) }`
- [ ] **Interface verification**: `go build ./...`; `go vet ./server`
- [ ] **Debugging**: memory-cap `go test ./... -count=1`
- [ ] **Lint**: `golangci-lint fmt && golangci-lint run`
- [ ] Коммит: `fix: server cell — логировать ошибку PublishDiagnostics, убрать задвоенный doc-комментарий`

### Task 3: Рефакторинг вложенности — analysis, xs (refactor)

Behavior-preserving извлечения хелперов; контракты не затронуты (unexported).

**Usages:** `conventions` (вложенность ≤2 уровней → именованные хелперы).

- [ ] **Code**: analysis/analyzer.go:249 `collectLocals` → `declareItem(item Expr)` (или доменное имя)
- [ ] **Code**: analysis/analyzer.go:84 `walkStmts` → guard-хелпер для `checkArgValues`
- [ ] **Code**: xs/ast.go:427 `collectLocals` → пер-элементный хелпер
- [ ] **Code**: xs/ast.go:549 `appendLocals` → выровнять ≤3 уровней
- [ ] **Code**: xs/parse.go:275 `parseEvent` → `eventArgsName(args)` хелпер
- [ ] **Debugging**: memory-cap `go test ./... -count=1` — поведение идентично
- [ ] **Lint**: `golangci-lint fmt && golangci-lint run`
- [ ] Коммит: `refactor: analysis/xs — извлечение хелперов для вложенности 4-5 уровней`

### Task 4: Тесты — t.Parallel в чистых пакетах, нейминг (tests)

CLAUDE.md: `t.Parallel()` для тестов без общего состояния. Границы: чистые пакеты
(common, rms, xs, analysis, kb, complete, hints) + docstore/resolver; НЕ трогаем
server serve-харнессы.

**Usages:** `conventions` (test naming `Test<Component>_<Scenario>`).

- [ ] **Code**: func-level `t.Parallel()` во всех тест-функциях: common/*_test.go,
  rms/parse_test.go, rms/navigation_test.go, xs/parse_test.go, xs/navigation_test.go,
  analysis/{analyzer,types,values}_test.go, kb/{mine,store,extract,gen}_test.go,
  complete/{candidate,completer}_test.go, hints/computer_test.go,
  server/docstore_test.go, include/resolver_test.go
- [ ] **Code**: в устойчивых таблицах — `t.Parallel()` и в `t.Run`-замыканиях
  (семантика переменных цикла Go 1.22+); при малейшем сомнении — только func-level
- [ ] **Code**: `TestCoerce_Table` → `TestCoerce_ValueShapes`
- [ ] **Debugging**: memory-cap `go test ./... -count=1` дважды (проверка на флак)
- [ ] **Lint**: `golangci-lint fmt && golangci-lint run`
- [ ] Коммит: `test: t.Parallel для тестов без общего состояния; TestCoerce_ValueShapes`

### Task 5: CI — gofumpt fmt-гейт (infrastructure)

CLAUDE.md: «Формат — golangci-lint fmt (gofumpt)». CI сейчас гейтит только goimports.

**Usages:** `cooks/github-actions` (пины, permissions — не менять).

- [ ] **Code**: .github/workflows/ci.yml — шаг после goimports:
  `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2` +
  `test -z "$(golangci-lint fmt --diff .)"`
- [ ] **Verify**: `golangci-lint fmt --diff .` локально пуст (exit 0)
- [ ] **Verify**: YAML валиден (`python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml'))"`)
- [ ] Коммит: `ci: гейт gofumpt (golangci-lint fmt --diff)`

### Task 6: Доки — реальность (infrastructure)

Синхронизация текстов с фактом: версия Go, фичи README, layout, статусы, чекбоксы.
Приоритет правил фиксируется в conventions.md.

- [ ] **Docs**: CLAUDE.md — «Go 1.23+»→«Go 1.26+»; Structure: «`testdata/`» →
  «`<cell>/testdata/` — фикстуры ячеек (rms/testdata, xs/testdata)»
- [ ] **Docs**: conventions.md — версия → 1.26+ (3 места); в шапку строку
  «При противоречии с CLAUDE.md приоритет у CLAUDE.md.»
- [ ] **Docs**: cooks/lsp-protocol.md:266 — «Go 1.26+»
- [ ] **Docs**: cooks/github-actions.md — примечание к Matrix cross-compile:
  single-job sequential вариант благословлён для малых модулей (как release.yml)
- [ ] **Docs**: README.md — фичи (+definition, references, document symbols,
  signature help), layout-блок (+cmd/, hints/, complete/, include/), релизная
  формулировка, «Go 1.26+»
- [ ] **Docs**: docs/plans/build-lsp-rms-xs.md — отметить выполненные чекбоксы `[x]`,
  шапка: статус реализовано + ссылка на задачи-продолжения
- [ ] **Docs**: docs/tasks/*.md — `Status: Done (PR #N)` по git-истории (5 файлов)
- [ ] **Verify**: grep — не осталось «1.23» в правилах; `[ ]` в плане = 0
- [ ] Коммит: `docs: актуализация под реальность — Go 1.26+, фичи README, статусы задач, чекбоксы плана`

### Task 7: Финальная валидация и PR (integration)

- [ ] memory-cap `go test ./... -count=1` — зелёный (повторно)
- [ ] `golangci-lint fmt` (без диффа) → `golangci-lint run` → `goga lint` →
  `goga contract kb common rms xs analysis hints complete include server` — без новых расхождений
- [ ] Acceptance criteria task.md — все 9 пунктов выполнены
- [ ] Push ветки, PR в master (не мерджить), тело PR — сводка миграции

---

## Validation Commands

- `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`: все тесты (memory-cap обязателен)
- `golangci-lint run`: линт
- `golangci-lint fmt --diff .`: gofumpt-формат
- `goga lint`: DSL-контракты
- `goga contract <cell>`: соответствие контракту

## Completion Criteria

- [ ] `kb.GenKB` реализован в gen.go, cmd/kbgen тонкий, regexp без дублей
- [ ] `ExtractRmsCommands` принимает логгер; WARN-ы не в глобальный slog
- [ ] server: ошибка PublishDiagnostics логируется; doc-комментарий цел
- [ ] Вложенность ≤3 уровней в 5 отмеченных местах
- [ ] Чистые тесты с `t.Parallel()`; двойной прогон без флака
- [ ] CI содержит fmt-гейт; локально `fmt --diff` пуст
- [ ] Доки синхронны с реальностью; правило приоритета CLAUDE.md зафиксировано
- [ ] Все validation commands зелёные; CODEMANIFEST не менялись после 0e45c4c
- [ ] Один PR из task/claudemd-compliance-migration
