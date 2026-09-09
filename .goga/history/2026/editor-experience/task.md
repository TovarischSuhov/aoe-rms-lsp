# Пачка editor experience: 13 слотов в 5 волнах

## Current State

aoe2-lsp v0.3.0: 10 ячеек (common, kb, rms, xs, analysis, complete, hints,
include, server, corpus). LSP-поверхность сегодня: диагностика
(didOpen/didChange, 7+ кодов), hover, completion, signatureHelp,
definition/references (по include-замыканию), documentSymbol. Инфраструктура:
corpus-харнесс на 100 реальных картах (вручную), бенчмарки, debug-логи,
CI (race/lint/govulncheck/coverage-бейдж), release-скрипт.

Отсутствуют: documentHighlight, foldingRange, workspace/symbol, rename,
codeAction, semanticTokens, inlayHints, workspace/configuration,
didChangeWatchedFiles, corpus-гейт в CI, VS Code-extension. Справочник
`docs/ref/real-map-nuances.md` (вердикты ✅/❓/✍️/💬 по корпусу) ещё не
превращён в правила анализа.

## Description

Roadmap-пачка самостоятельных задач: каждый слот берётся отдельно, в ветке
`task/<имя-слота>` → PR. Волны — рекомендуемый порядок, внутри волны порядок
свободный. S-слоты реализуются сразу; M/L-слоты перед реализацией проходят
свой цикл brainstorm → design → plan (контракты могут уточниться).

## Scope

**In scope (слоты):**

| Волна | Слот | Что делаем | Ячейки | Размер |
|---|---|---|---|---|
| 0 | `doc-highlight` | `textDocument/documentHighlight`: вхождения символа в файле на готовом `ReferencesAt` (rms+xs); kind read/write | server | S |
| 0 | `folding` | `textDocument/foldingRange`: секции RMS (start_random, if/elseif, <player_setup>…), тела XS-функций/правил | server | S |
| 1 | `did-you-mean` | unknown-command/attribute/symbol → «а может: X» по edit-distance/prefix к именам kb | analysis, kb | S–M |
| 1 | `quickfix` | `textDocument/codeAction`: вставить `#include` (missing-include), заменить deprecated `effect_percent`, применить did-you-mean | server | M |
| 1 | `nuance-rules` | вердикты real-map-nuances → данные в kb (алиасы/опечатки/no-op списки) + правила: «вероятно игнорируется»=hint, ✍️=warning | analysis, kb | M — **отложено 2026-09-09: требует ручной верификации кейсов** |
| 2 | `config` | `workspace/configuration`: severity-оверрайды, include-корни | server | M |
| 2 | `watched-files` | `didChangeWatchedFiles` → инвалидация кэша замыкания при правках файлов на диске | server, include | M |
| 2 | `corpus-ci` | corpus-прогон как CI-гейт: fetch по pinned SHA (кэш), fail на panic/timeout/exit | corpus, CI | S–M |
| 3 | `vscode-mvp` | extension scaffold (TS), language-config .rms/.xs, запуск бинарника (GitHub Releases/локально), сборка .vsix в CI | `editors/vscode/` (новое) | M |
| 4 | `workspace-symbol` | символы всех открытых доков + замыканий, query-фильтр | server | M |
| 4 | `rename` | `prepareRename`+`rename`: XS-символы, RMS `#define`/`#const`; WorkspaceEdit по замыканию | server, xs, rms | M–L |
| 4 | `semantic-tokens` | легенда known/unknown/deprecated/section/kind; полный файл, инкремент не нужен | server, analysis | M |
| 4 | `inlay-hints` | типы XS-переменных из `InferType` на декларациях | server, analysis | M |

**Out of scope:**

- marketplace-публикация extension (только .vsix)
- HTTP-поверхность (`openapi/`)
- новые Go-зависимости (stdlib + закреплённый go.lsp.dev/protocol)
- инкрементальный парсинг/дебаунс диагностики

## Acceptance Criteria

Общие для каждого слота:

- `make check` зелёный; `goga contract` затронутых ячеек зелёный
- изменение LSP-поверхности покрыто интеграционным stdio-тестом в server
  (как существующие navigation/completion-сценарии)
- контракты CODEMANIFEST меняются до кода (для слотов с колонкой «Ячейки»)

Слотовые критерии:

- `doc-highlight`: hover-подобная позиция на вхождении → все вхождения
  файла с корректными kind
- `folding`: фикстура с секциями/функциями → ожидаемый набор регионов
- `did-you-mean`: `creat_object` → подсказка `create_object`; без кандидатов
  — без подсказки
- `quickfix`: missing-include diagnostic → codeAction вставляет include
  строку в правильное место
- `nuance-rules`: находки из справочника дают hint/warning на картах
  корпуса; ✅-конструкции не шумят
- `config`: изменение severity через didChangeConfiguration применяется без
  рестарта
- `watched-files`: правка включённого файла на диске обновляет диагностику
  includer'а
- `corpus-ci`: PR с паникой парсера краснит CI
- `vscode-mvp`: `code --install-extension aoe2-lsp-*.vsix` → hover/completion
  работают в .rms-файле
- `workspace-symbol`: query по символу из невключённого открытого дока
- `rename`: локальная XS-переменная и #define в замыкании переименовываются
  во всех файлах
- `semantic-tokens`/`inlay-hints`: golden-тест легенды/позиций на фикстуре

## Stack

- **Frameworks:** Go 1.26+ (stdlib first), go.lsp.dev/protocol v1.0.1
  (закреплён); для `vscode-mvp` — TypeScript + vscode-languageclient +
  esbuild + @vscode/vsce
- **Libraries:** новых Go-зависимостей нет; edit-distance для
  `did-you-mean` — свой компактный (Левенштейн ~30 строк), без библиотеки
- **Infrastructure:** GitHub Actions (расширение существующих ci.yml /
  release-артефактов), shields-бейджи не затрагиваются

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| VS Code Extension API | `.goga/usages/cooks/vscode-extension.md` | создать в слоте `vscode-mvp` |
| LSP-паттерны go.lsp.dev | `.goga/usages/cooks/lsp-protocol.md` | существует; дополнить секциями codeAction/semanticTokens по мере слотов |

## Risks and Constraints

- `rename` — самый крупный слот: переименование по замыканию требует
  консенсуса по строкам правки (WorkspaceEdit с version- overriding);
  сначала brainstorm.
- `semantic-tokens`: клиентские легенды различаются — держать полный
  легенд-набор на сервере, не настраиваемый.
- `corpus-ci`: скачивание 100 карт (network) — нужен кэш по SHA и
  бюджет времени джобы; возможен nightly вместо per-PR.
- `nuance-rules`: данные из справочника ручные — риск шума на ✅-картах;
  прогонять корпус до merge как приёмку.
- `vscode-mvp`: запуск бинарника из Releases привязывает версии
  extension↔server; в MVP — настройка пути + fallback на bundled.

## Scope Estimate

13 слотов, 5 волн; каждый слот — независимая задача со своей веткой и PR.
Рекомендуемые первые шаги: `doc-highlight` (разогрев) либо `config`
(фундамент для vscode-mvp).

## Existing Architecture

- Волна 0/4 опираются на готовые `ReferencesAt`/`Symbols` (rms, xs) и
  `Closure` (include).
- Волна 1 опирается на `kb.Store` (lookup-и, mining) и коды диагностики
  analysis.
- Волна 2 опирается на `include.Resolver`/`Closure` (кэш на includer'а) и
  существующий docstore.
- `vscode-mvp` — новая директория `editors/vscode/` вне internal/, без
  контрактов Go-ячеек (отдельный стек, сборка в CI отдельной джобой).

## Notes

- Формулировка утверждена пользователем 2026-09-09: все 4 направления
  (навигация, quickfix-UX, операционка, обогащение) + VS Code-плагин
  в статусе «MVP + .vsix в CI».
- Слоты не зависят друг от друга, кроме: `config` до `vscode-mvp`
  (extension передаёт настройки), `did-you-mean` до `quickfix`
  (переиспользование выдачи).
- Каждый слот при подборе: свежая ветка от master → если M/L, свой
  brainstorm/design/plan в топике слота → реализация → PR.
