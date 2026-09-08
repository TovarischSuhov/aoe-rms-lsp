# Build Plan: aoe2-lsp (LSP server for AoE2 RMS + XS)

> **Status: реализовано** (все чекбоксы отмечены по факту сделанного).
> План описывал стартовые шесть ячеек; проект вырос до девяти —
> продолжения: `docs/tasks/lsp-navigation.md` (навигация),
> `docs/tasks/cross-file-navigation.md` (include-замыкание),
> `docs/tasks/analysis-deepening.md` (значения/типы),
> signature help и completion-фичи (PR #5–#6).

## Overview

- [x] Реализовать LSP-сервер `aoe2-lsp` на Go по контрактам CODEMANIFEST шести
      ячеек: common, kb, rms, xs, analysis, server

## Context

- **Контракты обязательны к соблюдению**: в каждой ячейке лежит `CODEMANIFEST`
  (заголовок/тело/футер) — сигнатуры, свойства, методы и аннотации всех типов.
  Реализация обязана экспортировать имена ровно как в контракте
  (проверяется `goga contract`). Читай CODEMANIFEST каждой ячейки перед её
  реализацией и `.usages/` файлы ячейки (как потреблять соседей).
- **Авторитетный план архитектуры**: `docs/arch/lsp-rms-xs.md` (порядок,
  зависимости, полные контракты, диаграмма).
- **Практики проекта**: `.goga/usages/conventions.md` (обязательные правила
  Go: 1.26+, goimports, DI-конструкторы, context-first, `%w`, slog,
  testify/cmp, table-driven тесты), `.goga/usages/rms-grammar.md`,
  `.goga/usages/xs-grammar.md` (грамматики языков),
  `.goga/usages/cooks/lsp-protocol.md` (паттерны go.lsp.dev для server).
- **Данные (все локально, сеть для данных не нужна)**:
  - `docs/ref/ugc-guide/xs/functions/functions.json` — 204 XS-функции
  - `docs/ref/ugc-guide/xs/constants/constants.json` — 27 секций констант
  - `docs/ref/ugc-guide/xs/prelude.xs` — дамп игры (882 extern), фикстура XS-парсера
  - `docs/ref/zetnus-rms-guide.txt` — RMS-команды (Syntax Skeleton)
  - `docs/ref/aoe2de-xs-rms-changelog.md` — версионность (since_update)
- **Зависимости**: go.mod уже создан, `go.lsp.dev/protocol v1.0.1` закреплён.
  Для скачивания модулей используй `go mod tidy` (НЕ вендорить); env
  GOPROXY=https://goproxy.cn,direct уже задан в среде.
- **Порядок ячеек** (leaves → root): common → kb, rms, xs (независимы) →
  analysis → server. Выполняй задачи строго по порядку.
- Соглашения по валидации (после каждой задачи):
  `go test ./...`, `goimports -w .`, `golangci-lint run`,
  `goga lint`, и `goga contract <путь_ячейки>` для реализованной ячейки.

## Success criteria

- [x] `go test ./...` зелёный, `golangci-lint run` без замечаний
- [x] `goga contract` проходит для всех шести ячеек
- [x] XS-парсер разбирает `docs/ref/ugc-guide/xs/prelude.xs` без ложных ошибок
- [x] RMS-парсер разбирает реальные RMS-фикстуры без ложных ошибок
- [x] Completion возвращает все 204 XS-функции из kb
- [x] Диагностика (unknown-command, unknown-attribute, undefined-symbol,
      bad-arity, deprecated-effect-percent) выдаётся с корректными range
- [x] Бинарник `aoe2-lsp` стартует по stdio и отвечает на initialize/hover/
      completion/didChange-диагностику (проверка LSP-клиентом из теста)

## Task sections

### Task 1: Ячейка common — позиции и диагностика

- [x] Прочитать `common/CODEMANIFEST` и `common/.usages/positions-and-diagnostics.md`
- [x] Реализовать `common/pos.go` (Pos: Line, Column, Offset; упорядоченность),
      `common/range.go` (Range + Contains), `common/diagnostic.go`
      (Diagnostic + константы Severity 1..4)
- [x] Table-driven тесты: упорядоченность Pos, Contains (границы/вне/на концах),
      инвариант severity
- [x] `go test ./common/...`, `goimports -w .`, `golangci-lint run`,
      `goga lint`, `goga contract common` — все зелёные

### Task 2: Ячейка kb — база знаний

- [x] Прочитать `kb/CODEMANIFEST` (включая inline-практику `kbdata`),
      `kb/.usages/lookups.md`, `kb/.usages/data-pipeline.md`
- [x] Реализовать `kb/model.go` (Function, Param, Constant, Command,
      CommandArg) и `kb/store.go` (NewStore: go:embed + валидация схемы,
      дубликатов, пустых имён; 7 lookup-методов)
- [x] Сгенерировать `kb/data/xs-functions.json` (адаптация
      docs/ref/ugc-guide/xs/functions/functions.json: категории развёрнуты,
      поля name/category/return_type/params/desc/since_update; 204 записи)
      и `kb/data/xs-constants.json` (name/section/value/desc/since_update)
- [x] Реализовать `kb/extract.go` — ExtractRmsCommands(path) по Algorithm из
      контракта (Zetnus txt → []Command); сгенерировать
      `kb/data/rms-commands.json` (обогащение since_update из changelog)
- [x] Тесты: NewStore валидация (дубликат/пустое имя → ошибка), все lookup-ы
      (found/не found), счётчики (204 функции; константы 27 секций)
- [x] `go test ./kb/...`, `goimports -w .`, `golangci-lint run`,
      `goga lint`, `goga contract kb` — зелёные

### Task 3: Ячейка rms — парсер RMS

- [x] Прочитать `rms/CODEMANIFEST`, `rms/.usages/rms-parsing.md`,
      `.goga/usages/rms-grammar.md`
- [x] Реализовать `rms/parse.go` (лексер + Parse с recovery по Algorithm из
      контракта) и `rms/ast.go` (RmsFile, Section, Statement, Attribute,
      Expr, XsBlock + SectionAt/StatementAt)
- [x] Фикстуры в `rms/testdata/`: минимум 6 .rms-файлов, покрывающих секции,
      позиционную семантику атрибутов, start_random/percent_chance,
      if/elseif/else, DE-выражения с операторами и float, #include,
      #includeXS-блок, повреждённый вход (recovery)
- [x] Тесты: все фикстуры парсятся с ожидаемым набором диагностик;
      StatementAt возвращает команду-владельца для позиции на атрибуте;
      Parse никогда не возвращает nil-файл
- [x] `go test ./rms/...`, `goimports -w .`, `golangci-lint run`,
      `goga lint`, `goga contract rms` — зелёные

### Task 4: Ячейка xs — парсер XS

- [x] Прочитать `xs/CODEMANIFEST`, `xs/.usages/xs-parsing.md`,
      `.goga/usages/xs-grammar.md`
- [x] Реализовать `xs/parse.go` (XsParse с recovery) и `xs/ast.go`
      (XsFile + SymbolAt, Decl, Param, Stmt, Expr)
- [x] Фикстуры: `docs/ref/ugc-guide/xs/prelude.xs` (тест: 0 ложных ошибок) +
      `xs/testdata/`: функции, правила/события, vector-литералы,
      switch/for/while, обрезанный вход
- [x] Тесты: prelude.xs без ложных ошибок; SymbolAt на call возвращает
      callee; recovery на обрезанном входе
- [x] `go test ./xs/...`, `goimports -w .`, `golangci-lint run`,
      `goga lint`, `goga contract xs` — зелёные

### Task 5: Ячейка analysis — семантические проверки

- [x] Прочитать `analysis/CODEMANIFEST`, `analysis/.usages/checks.md`,
      `kb/.usages/lookups.md`
- [x] Реализовать `analysis/analyzer.go` (NewAnalyzer, AnalyzeRms,
      AnalyzeXs по Algorithm из контракта; коды: unknown-command,
      unknown-section, unknown-attribute, bad-argument,
      deprecated-effect-percent, undefined-symbol, bad-arity)
- [x] Тесты: каждая проверка покрывается позитивным и негативным случаем;
      диангностики отсортированы; AST не мутируется
- [x] `go test ./analysis/...`, `goimports -w .`, `golangci-lint run`,
      `goga lint`, `goga contract analysis` — зелёные

### Task 6: Ячейка server + бинарник + README

- [x] Прочитать `server/CODEMANIFEST`, `server/.usages/lifecycle.md`,
      `.goga/usages/cooks/lsp-protocol.md`
- [x] Реализовать `server/docstore.go`, `server/server.go`
      (protocol.UnimplementedServer + методы Initialize/DidOpen/DidChange/
      DidClose/Hover/Completion/Shutdown/Exit), `server/serve.go` (Serve)
- [x] `cmd/aoe2-lsp/main.go` — точка входа (вызов Serve, slog, exit code)
- [x] Интеграционный тест: LSP-клиент из теста стартует сервер по stdio,
      initialize → didOpen(.rms с unknown-command) → publishDiagnostics
      содержит код unknown-command; hover возвращает сигнатуру xsGetMapSeed;
      completion возвращает 204 функции
- [x] README: секция установки/запуска + конфиги Neovim (lspconfig,
      filetypes aoe2rms/aoe2xs) и VS Code (generic LSP extension)
- [x] `go test ./...`, `goimports -w .`, `golangci-lint run`,
      `goga lint`, `goga contract server` — зелёные; `go build ./cmd/aoe2-lsp`
