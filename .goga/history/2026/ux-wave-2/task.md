# Пачка «UX волна 2»: 8 слотов редакторского опыта

## Current State

aoe2-lsp v0.6.0: ядро LSP + расширение VS Code (автозагрузка бинарника
#52, подсветка #51, snippets #54); закрыты documentLink/selectionRange/
rename (#41–44), fuzz (#56), duplicate-include (#58), kb-refresh (#57,
PR #77 смержен). В работе: эпик v1.0.0 (#45–50, #69) и остаток пачки
ux-and-data-quality (#53 nuance-rules, #55 map-structure,
#59 kb-attribute-desc). При этом повседневный UX всё ещё дырявый:

- hover на вложенном атрибуте (`number_of_objects` внутри
  `create_object`) показывает справку внешней команды, а не поля —
  осиротевшая формулировка `2026/task-hover-attribute/task.md`, в issues
  не зеркалена;
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
| `hover-attribute` | #78 | Hover на атрибуте внутри блока команды показывает справку атрибута (kb lookup с владельцем), не справку внешней команды; детали — авторитетная формулировка `2026/task-hover-attribute/task.md` (StatementAt, hoverRms); полнота текстов зависит от #59, частично работает и без него | server (+kb lookups) | S–M |
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

- `hover-attribute`: hover на вложенном поле показывает имя атрибута и
  desc; без desc — graceful fallback на владельца; корпус без регрессий
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

Мультизадача: 8 слотов (~2×S, 4×S–M, 2×M), каждый — отдельная ветка
`task/<name>` → PR. Примерно 3 волны:

1. Быстрые победы: `status-bar`, `hover-attribute`, `xs-unused`
2. Серверные M: `auto-include`, `rename-file`
3. Клиентские: `game-detect`, `deploy-to-game`, `walkthrough`

## Existing Architecture

- `hover-attribute` — `internal/server` hoverRms + `kb.Store` lookup
  атрибута с владельцем (детали в `2026/task-hover-attribute/task.md`)
- `xs-unused` — `analysis.Analyzer` (паттерн добавления проверок),
  `xs.XsFile` decls/refs
- `auto-include` — `complete.Completer` (контекстная матрица),
  `include.Resolver` (доступные файлы), quickfix-паттерн `server`
- `rename-file` — `include.Closure` + новый хендлер в `internal/server`
- `game-detect`/`deploy-to-game`/`walkthrough`/`status-bar` —
  `editors/vscode` (package.json contributes, extension.ts)

## Notes

- Пачка собрана 2026-09-16 полным sweep'ом (сервер + клиент + данные)
  по решению пользователя; утверждены все 8 слотов
- settings-schema снят при разборе: описания настроек уже в package.json
- InlayHint, webview-reference, incremental sync, nvim-lspconfig —
  остаются 1.x-кандидатами (решения 2026-09-09/10)
- hover-attribute поглощает осиротевшую формулировку
  `2026/task-hover-attribute/task.md`: её детали (StatementAt,
  hoverRms, kb Attribute lookup) авторитетны для слота
- cook `aoe2-game-paths.md` создаётся первым исполняемым слотом из
  `game-detect`/`deploy-to-game`, общая база для обоих
