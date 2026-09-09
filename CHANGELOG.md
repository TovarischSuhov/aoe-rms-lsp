# Changelog

## v0.3.0 (2026-09-09)

### Docs

- docs: бейджи README + самодостаточный coverage-бейдж в CI

## v0.2.0 (2026-09-09)

### Features

- feat: ZR@-распаковка + XS-источники в корпусе; фиксы XS for/const по расширенному прогону
- feat: бенчмарки горячих путей ячеек (rms, xs, analysis, hints, complete, include, kb)
- feat: corpus-ячейка — прогон реальных карт через LSP + фиксы парсера rms
- feat: флаг -debug — debug-логи жизненного цикла и запросов LSP
- feat: corpus cell — контракт ячейки по архитектурному плану (CODEMANIFEST + .usages)
- feat: скрипт выпуска версии — changelog из conventional commits + тег + пуш
- feat: kb cell — GenKB (перенос генерации из cmd/kbgen), инжектированный логгер в ExtractRmsCommands
- feat: контракты миграции на CLAUDE.md — kb GenKB + ExtractRmsCommands log; дедупликация аннотаций 10 типов; Algorithm без код-синтаксиса; .usages: битые имена, кросс-ссылки, фенсы
- feat: server cell — integration-тесты completion full stack + переписанные MVP-тесты (task 6)
- feat: server cell — Completion делегирует в complete.Completer: роутинг rms/xs/inline-XS, kind-таблица, DI completer; MVP-хелперы wordPrefix/matchesPrefix удалены (task 5)
- feat: complete cell — Completer.XsAt: truth-модель source > kb, external-пул, мини-сигнатуры (task 4)
- feat: complete cell — Completer.RmsAt: контекстная матрица ArgAt/SectionAt + kb (task 3)
- feat: complete cell — bootstrap пакета + Candidate (task 2)
- feat: xs cell — XsFile.VisibleAt: видимые символы для completion, covers-семантика Definition (task 1)
- feat: goga cells — completion contracts (новая ячейка complete + xs VisibleAt + server)
- feat: server cell — integration-тесты signature help full stack + per-signature ActiveParameter (task 9)
- feat: server cell — SignatureHelp handler, capability, computer DI (signature help, task 8)
- feat: hints cell — Computer: truth-модель, конфликт-правило, рендер (signature help, task 7)
- feat: hints cell — bootstrap пакета + Hint (signature help, task 6)
- feat: rms cell — ArgSite + RmsFile.ArgAt с band-owner и записанными спанами (signature help, task 5)
- feat: xs cell — CallSite + XsFile.CallAt с parser-индексами calls/noncode (signature help, task 4)
- feat: kb cell — ExtractRmsCommands mining pass + kbgen range wire (signature help, task 3)
- feat: kb cell — Store load-time mining, fill-when-empty helper (signature help, task 2)
- feat: kb cell — ValueRange, CommandArg.Range, MineKindRange (signature help, task 1)
- feat: goga cells — signature help contracts (новая ячейка hints + kb/xs/rms/server)
- feat: server diagnostics — missing-include + externals конвейер (Task 11)
- feat: server navigation — Definition/References через Resolver (Task 10)
- feat: server docstore — Text/URIs, удовлетворение Source (Task 9)
- feat: analysis externals — AnalyzeXs с декларациями замыкания (Task 8)
- feat: include navigation — Definition/References по замыканию (Task 7)
- feat: include resolver — Closure: DFS, visit-set, лимиты, кэш (Task 6)
- feat: include closure — данные замыкания + ExternalDecls (Task 5)
- feat: include source — интерфейс editor-state (Task 4)
- feat: xs navigation — References(name) (Task 3)
- feat: rms navigation — References(name) by-name проекция (Task 2)
- feat: rms include — Include-тип, XsIncludes, Range аргумента (Task 1)
- feat: CODEMANIFEST cross-file-navigation — контракты пяти ячеек (apply)
- feat: server navigation — References + DocumentSymbol (Task 7)
- feat: server navigation — capabilities + Definition (Task 6)
- feat: rms navigation — Symbols + ReferencesAt (Task 5)
- feat: xs navigation — Definition с затенением областей (Task 3)
- feat: xs navigation — Symbols + ReferencesAt (Task 2)
- feat: common symbol — Symbol outline-узел (Task 1)
- feat: CODEMANIFEST lsp-navigation — контракт навигации (Symbol outline, definition/references)

### Fixes

- fix: xs — const-локальные декларации внутри тел функций
- fix: server cell — логировать ошибку PublishDiagnostics (WARN, не роняя сессию), убрать задвоенную строку doc-комментария Completion
- fix: server cell — byte↔UTF-16 конвертация колонок при positionEncoding=utf-16: атомарный режим + текст-зависимые конвертеры во всех хендлерах (batch-2, №6)
- fix: include cell — граница корневой директории и обычный файл в резолве (№9); URI написания запроса из кэша (№8); один Closure на includer'а (№10)
- fix: rms cell — пустой inline-регион #includeXS с аргументом не создаёт XsBlock (№7); blankComments не гасит комментарии внутри строк (№11)
- fix: восстановлена закрывающая скобка TestXsParse_MultiDeclNavigation, потерянная в merge-конфликте с master (PR #10 CI)
- fix: xs cell — parseFor хранит init-декларацию первым statement тела: переменная цикла видима в init/cond/step/body, не видна после (task 5, №2)
- fix: xs cell — top-level мультидекларации int a = 1, b = 2; — цикл деклараторов в parseTypedDecl (task 4, №4)
- fix: xs cell — scanString считает строку при escape \ + \n в литерале: line++/lineStart (task 3, №3)
- fix: rms cell — воскрешённый warning о неявном закрытии блока в closeScopes (task 2, №5)
- fix: rms cell — endXsBlock без паники на незакрытом XS-блоке в EOF: lineStartPos-хелпер покрывает оба вектора (End и bare #includeXS Start) (task 1, №1)
- fix: xs cell — VisibleAt берёт params/locals только из function-тел; инициализатор top-level переменной не локаль
- fix: rms parse — закрывающие теги секций + word-индекс (Task 4)
- fix: review Task 1 — value-семантика и доступ к Children вместо тавтологии
- fix: contract lsp-navigation — skip include в xs.Symbols, все слова в rms.ReferencesAt

### Performance

- perf: xs-сканер на string-источнике — −37% аллокаций XsParse; bench-отчёт

### Refactoring

- refactor: ячейки под internal/ — соответствие шаблону new-go-project
- refactor: analysis/xs — извлечение хелперов для вложенности 4-5 уровней
- refactor: xs cell — чистка линт-хинтов: skipBlock без неиспользуемого параметра, indexByte через strings.IndexByte

### Tests

- test: t.Parallel() для 209 тестов без общего состояния; TestCoerce_ValueShapes (бывш. TestCoerce_Table)
- test: analysis + complete — регрессии №2 full stack: 0 ложных undefined-symbol на for-цикле, консервативность после цикла, кандидат i kind=local в completion (task 6)
- test: cross-file navigation — stdio-сценарии 1-5 (Task 12)
- test: navigation — интеграционные свипы и stdio-приёмка (Task 8)

### CI

- ci: гейт gofumpt (golangci-lint fmt --diff), пин v2.13.2 идентичен lint-action

### Docs

- docs: справочник незадокументированных нюансов реальных карт (с вердиктами и ссылками)
- docs: архитектурный план operational-readiness (corpus-ячейка, debug, bench)
- docs: задача — debug-логи, бенчмарки, корпус-прогон 100 карт (goga history)
- docs: структура internal/ в CLAUDE.md и README; задача в goga history
- docs: CLAUDE.md — условные правила из шаблона: requirements.go при росте интерфейсов, openapi/ при появлении HTTP
- docs: CLAUDE.md — сверка с шаблоном new-go-project; README Releases; cook
- docs: goga history — design.md и plan.md фазы миграции (исполнительный план кода/инфра)
- docs: актуализация под реальность — Go 1.26+, правило приоритета CLAUDE.md, фичи и layout README, статусы задач, чекбоксы плана
- docs: ревью 2026-09-08 — статус-таблица: все 11 находок исправлены (PR #7–#15)
- docs: контракты и план fix-review-defects-2 — находки №6–№11 ревью 2026-09-08
- docs: план fix-review-defects — 6 задач rms→xs→analysis/complete, TDD, ralphex-формат (по design be275ff)
- docs: контракты и дизайн fix-review-defects — №1–№5 по ревью 2026-09-08
- docs: находки полного ревью 2026-09-08 — паника rms-парсера, for-loop локали, сдвиг строк, utf-16
- docs: план completion — 6 задач xs→complete→server, TDD, ralphex-формат (по design 72301b5)
- docs: design completion — трассировки, алгоритмы, тест-сценарии (по контрактам 8c77336)
- docs: brainstorm completion — arch.md (ячейка complete, xs VisibleAt, server) + правка task.md
- docs: задача completion (task.md) + секция Completion Items в cooks/lsp-protocol.md
- docs: goga plan — environment gate актуален (локальный toolchain go1.26.6, сборка/тесты локально)
- docs: plan cross-file-navigation — ralphex-план из дизайн-документа
- docs: design cross-file-navigation — спецификация по материализованному контракту
- docs: arch cross-file-navigation — контракты пяти ячеек (brainstorm)
- docs: propose cross-file-navigation — формулировка задачи + cook-паттерны
- docs: plan lsp-navigation — ralphex-план из дизайн-документа
- docs: design lsp-navigation — спецификация навигации по материализованному контракту
- docs: arch lsp-navigation — контракты навигации для common/xs/rms/server (brainstorm)
- docs: task lsp-navigation — однодокументная навигация (definition/references/documentSymbol)

### Chore

- build: Makefile — зеркало шаблона new-go-project с адаптацией под проект
- Fix
- chore: golangci v2 по шаблону new-go-project (.golangci.yml: standard + copyloopvar/misspell/unconvert + gofumpt) + CLAUDE.md смердж с шаблоном + gofumpt-форматирование
- Fi
- Fix
- FIx
- chore: goga env — claude через Z.ai (BASE_URL + glm-модели) для pipeline и build
- chore: goga config — build-pipeline fields per 1.3 docs schema
- chore: goga 1.3.0 — image :1.3 + pipeline section in .goga/config.yml
- Fix

