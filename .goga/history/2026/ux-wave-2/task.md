# Пачка «UX волна 2»: 7 слотов редакторского опыта

## Current State

aoe2-lsp v0.6.0: ядро LSP + расширение VS Code (автозагрузка бинарника
#52, подсветка #51, snippets #54); закрыты documentLink/selectionRange/
rename (#41–44), fuzz (#56), duplicate-include (#58), kb-refresh (#57,
PR #77 смержен). В работе: эпик v1.0.0 (#45–50, #69) и остаток пачки
ux-and-data-quality (#53 nuance-rules, #55 map-structure,
#59 kb-attribute-desc). При этом повседневный UX всё ещё дырявый:

- XS не диагностирует unused/duplicate declarations (выпало из эпика
  v1.0.0 в out-of-scope и не было подхвачено);
- символ из доступного, но не подключённого include-файла не
  предлагается — `#include` пишется руками по ошибке unknown-symbol;
- переименование .rms/.xs файла руками влечёт правку всех `#include`
  у ссылающихся;
- расширение не знает, где установлена игра: `aoe2lsp.includeRoots`
  настраивается вручную (автодетект — 1.x-кандидат из эпика);
- нет команды «проверить карту в игре» — копирование в мод-папку руками;
- нет welcome-обучения (walkthrough) и индикатора состояния сервера.

## Description

Roadmap-пачка самостоятельных задач улучшения UX по образцу
`2026/task-ux-and-data-quality` — вторая волна, собранная полным
sweep'ом (сервер + VS Code клиент + данные). Каждый слот берётся
отдельно, в ветке `task/<имя-слота>` → PR; S-слоты реализуются сразу,
M — с коротким дизайн-проходом. Не блокирует эпик v1.0.0 и не
блокируется им.

## Scope

**In scope (слоты):**

| Слот | Issue | Что делаем | Ячейки/области | Размер |
|---|---|---|---|---|
| `xs-unused` | #79 | Диагностика unused/duplicate declarations в XS: локальные переменные/функции без использований; правила severity-override работают как обычно | analysis, xs | S–M |
| `auto-include` | #80 | Completion предлагает символы из доступных (не подключённых) include-файлов с пометкой источника; выбор вставляет `#include`/`#includeXS`; quickfix для unknown-symbol с однозначным кандидатом | complete, server, include | M |
| `rename-file` | #81 | Хендлер `workspace/willRenameFiles`: переименование/перемещение .rms/.xs обновляет `#include` во всех ссылающихся файлах замыкания (WorkspaceEdit) | server (+include) | M |
| `game-detect` | #82 | Автодетект установки AoE2 DE (Steam — стандартные пути + `libraryfolders.vdf`, MS Store/Xbox) → автозаполнение `aoe2lsp.includeRoots`; явная ручная настройка выигрывает; перенос 1.x-кандидата из эпика | editors/vscode | M |
| `deploy-to-game` | #83 | Команда «деплой карты»: копирование активной .rms (и её include-зависимостей — решает design) в мод-папку игры для теста | editors/vscode | S–M |
| `walkthrough` | #84 | Welcome walkthrough в VS Code: установка → первый скрипт → диагностика/hover/completion | editors/vscode | S–M |
| `status-bar` | #85 | Статус-бар: состояние сервера (running/stopped), версия kb | editors/vscode | S |

**Out of scope (остаются 1.x-кандидатами, решения 2026-09-09/10 не
пересматривались):**

- InlayHint; command-reference webview; incremental sync + semantic
  tokens range/delta; контрибуция в nvim-lspconfig; scoop/AUR

Снят при разборе: settings-schema — `markdownDescription` настроек уже
в `editors/vscode/package.json`, тултипы в Settings UI работают.

## Acceptance Criteria

Общие для каждого слота:

- `make check` зелёный; `goga contract` затронутых ячеек зелёный
- новые диагностики: коды задокументированы, работают
  severityOverrides (`none` подавляет), не шумят на ✅-конструкциях
  корпуса (прогон корпуса как приёмка для analysis-слотов)

Слотовые критерии:

- `xs-unused`: unused-переменная → диагностика с кодом; rules/events
  (вызываются движком) не помечаются unused; duplicates — warning
- `auto-include`: completion предлагает символ из неподключённого
  include-файла, выбор вставляет include; quickfix на unknown-symbol с
  единственным кандидатом
- `rename-file`: rename .rms в workspace → `#include` обновлены во всех
  ссылающихся файлах замыкания, включая неоткрытые
- `game-detect`: на машине со Steam-версией includeRoots заполняются без
  ручной настройки; игра не найдена — тишина, не ошибка
- `deploy-to-game`: команда копирует карту в мод-папку, путь
  настраивается, задокументирован
- `walkthrough`: walkthrough виден в Getting Started, шаги совпадают с
  реальным UX
- `status-bar`: индикатор отражает состояние сервера и версию kb

## Stack

- **Frameworks:** Go 1.26+ (stdlib), go.lsp.dev/protocol (существующая),
  TypeScript/esbuild для расширения (существующие)
- **Libraries:** без новых зависимостей
- **Infrastructure:** GitHub Actions (без изменений)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| go.lsp.dev/protocol — willRenameFiles | `.goga/usages/cooks/lsp-protocol.md` | update в слоте `rename-file` |
| VS Code API: commands/walkthrough/statusBar | `.goga/usages/cooks/vscode-extension.md` | update в слотах `deploy-to-game`, `walkthrough`, `status-bar` |
| Раскладка AoE2 DE: инсталляции + мод-папки | `.goga/usages/cooks/aoe2-game-paths.md` | create в первом исполняемом из `game-detect`/`deploy-to-game` |

## Risks and Constraints

- `xs-unused`: главный риск — ложные срабатывания: rules/events
  вызываются движком, точка входа внешняя; нужна модель entry-points,
  пороги severity — на design-этапе, приёмка корпусом обязательна
- `auto-include`: поиск по всем доступным include-файлам может быть
  дорогим и шумным — ограничение числа кандидатов на design-этапе
- `rename-file`: willRenameFiles зависит от клиентской поддержки
  (VS Code — ок); поведение задокументировать
- `game-detect`: MS Store/Xbox-пути нестабильны между машинами; явное
  `aoe2lsp.includeRoots` всегда выигрывает над автодетектом
- `deploy-to-game`: мод-папка может требовать перезапуска игры —
  сверить с UGC Guide на design-этапе
- `walkthrough`/`deploy-to-game` логичнее после публикации расширения
  (#47), но жёстко не блокируются
- Пачка не блокирует и не блокируется эпиком v1.0.0 и остатком
  ux-and-data-quality

## Scope Estimate

Мультизадача: 7 слотов (1×S, 3×S–M, 3×M), каждый — отдельная ветка
`task/<name>` → PR. Примерно 3 волны:

1. Быстрые победы: `status-bar`, `xs-unused`
2. Серверные M: `auto-include`, `rename-file`
3. Клиентские: `game-detect`, `deploy-to-game`, `walkthrough`

## Existing Architecture

- `xs-unused` — `analysis.Analyzer` (паттерн добавления проверок),
  `xs.XsFile` decls/refs
- `auto-include` — `complete.Completer` (контекстная матрица),
  `include.Resolver` (доступные файлы), quickfix-паттерн `server`
- `rename-file` — `include.Closure` + новый хендлер в `internal/server`
- `game-detect`/`deploy-to-game`/`walkthrough`/`status-bar` —
  `editors/vscode` (package.json contributes, extension.ts)

## Notes

- Пачка собрана 2026-09-16 полным sweep'ом (сервер + клиент + данные)
  по решению пользователя; в слейт входили 8 кандидатов, после сверки
  с кодом hover-attribute снят — реализован PR #40 (2026-09-09,
  `ArgAt Kind=attr` + `Store.Attribute`); issue #78 закрыт как дубль,
  остаточная боль (полнота desc) — в #59
- settings-schema снят при разборе: описания настроек уже в package.json
- InlayHint, webview-reference, incremental sync, nvim-lspconfig —
  остаются 1.x-кандидатами (решения 2026-09-09/10)
- cook `aoe2-game-paths.md` создаётся первым исполняемым слотом из
  `game-detect`/`deploy-to-game`, общая база для обоих
