# v1.0.0 — полный LSP-функционал и дистрибуция

## Current State

aoe2-lsp v0.3.0: 10 ячеек (common, kb, rms, xs, analysis, complete, hints,
include, server, corpus) + `editors/vscode`. LSP-поверхность: full-sync
(utf-8/utf-16 negotiation), диагностика (синтаксис + семантика +
missing-include, severity overrides, watch-reload), hover, completion,
signatureHelp, definition/references по include-замыканию, documentSymbol,
documentHighlight, workspaceSymbol (fuzzy), semanticTokens (full),
foldingRange, codeAction (3 quickfix), конфигурация pull+push
(`aoe2lsp`-секция). Инфраструктура: corpus-гейт в CI (100 карт), бенчмарки,
debug-логи, release-скрипт (changelog + тег + бинарники), VS Code-расширение
in-tree (.vsix — артефакт CI, не публикуется).

Отсутствуют против LSP 3.17/3.18 в применимой части: **rename +
prepareRename**, **documentLink**, **selectionRange**, **formatting**.
Неприменимы/низкая ценность: documentColor, typeHierarchy, moniker,
inlineValue, codeLens, callHierarchy. Дистрибуция: расширение не опубликовано
(Marketplace, Open VSX), версии extension (0.1.0) и server (v0.3) не
синхронизированы, установки через пакетные менеджеры нет, README-инструкций
установки по платформам нет.

## Description

Эпик выпуска v1.0.0 «production-ready редакторский опыт»: добить канонический
набор языковых фич (DocumentLink, SelectionRange, Rename по замыканию,
Formatting), опубликовать VS Code-расширение (Marketplace + Open VSX),
добавить установку через пакетные менеджеры (winget, Homebrew) и инструкции
установки по платформам в README, выпустить релиз v1.0.0.

## Scope

**In scope:**
- documentLink: `#include`/`#includeXS` — кликабельные ссылки на файлы
- selectionRange: расширение выделения по AST rms/xs
- rename: rename-сайты в xs/rms, PrepareRename + Rename, мультифайловые
  WorkspaceEdit по замыканию
- format: новая ячейка `internal/format` (форматтеры RMS и XS, golden-тесты),
  server-хендлер Formatting
- публикация расширения: `vsce publish` + `ovsx` из release-workflow,
  синхронизация версий extension ↔ server в `scripts/release.sh`
- установка: winget-манифест, Homebrew tap, README-инструкции для
  Windows / macOS (Intel + Apple Silicon) / Linux
- release v1.0.0: changelog, тег, релизные заметки

**Out of scope (кандидаты 1.x):**
- InlayHint
- автозагрузка бинарника расширением — сформулирована 2026-09-09:
  `2026/task-vscode-autodownload/task.md`
- автодетект установленной игры в расширении VS Code: при установке /
  активации найти инсталляцию AoE2 DE (Steam — стандартные пути +
  `libraryfolders.vdf`, MS Store / XboxGames) и автоматически пробросить
  корень системных include (напр. `<game>/resources/_common/ai-rms`) в
  `aoe2lsp.includeRoots` — стоковые `#include` резолвятся без ручной
  настройки; перекликается с vscode-autodownload (та же точка входа
  `activate`, но не зависит от неё)
- CallHierarchy, CodeLens, incremental sync, semantic tokens range/delta
- доп. проверки анализа (duplicate declarations, unused)
- scoop, AUR

## Acceptance Criteria

- Хендлеры documentLink / selectionRange / prepareRename / rename /
  formatting отвечают по спецификации, capabilities объявлены в `initialize`;
  пустые результаты — пустые срезы, не nil (`lsp-protocol`)
- rename правит все вхождения по замыканию, включая файлы, не открытые в
  редакторе; prepareRename отказывает на непереименовываемой позиции
- Инварианты форматтера: `parse(format(x)) ≡ parse(x)` и
  `format(format(x)) ≡ format(x)` на golden-фикстурах и корпусе 100 карт
- `.vsix` публикуется в Marketplace и Open VSX автоматически при пуше тега
  `v*`; версии extension и server совпадают
- `winget install` и `brew install` ставят бинарник на чистой машине;
  README описывает установку для всех основных платформ
- v1.0.0 выпущен: changelog, тег, бинарники + SHA256SUMS
- `make check` зелёный, corpus-гейт зелёный

## Stack

- **Frameworks:** Go 1.26+ (stdlib first), go.lsp.dev/protocol (LSP 3.18);
  TypeScript/esbuild для расширения (существующие)
- **Libraries:** без новых Go-зависимостей
- **Infrastructure:** GitHub Actions (release-workflow + публикация),
  секреты `VSCE_PAT` / `OVSX_PAT`, PR в microsoft/winget-pkgs, Homebrew
  tap (отдельный репозиторий)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| go.lsp.dev/protocol — новые хендлеры | `.goga/usages/cooks/lsp-protocol.md` | update (задачи 1–4, 6) |
| vsce publish / ovsx | `.goga/usages/cooks/vscode-extension.md` | update (задача 7) |
| release-workflow публикация | `.goga/usages/cooks/github-actions.md` | update (задачи 7, 9) |
| winget + Homebrew tap | `.goga/usages/cooks/package-managers.md` | create (задача 9) |

## Risks and Constraints

- **rename в RMS**: пользовательских имён в чистом RMS может не оказаться —
  design-этап задачи 3 фиксирует переименовываемые сущности; если их нет,
  rename покрывает только XS (`.xs` + inline-XS `.rms`)
- **Форматтер**: риск изменить семантику реальных карт — обязателен инвариант
  парсинга на корпусе; конкретный стиль — решение design-этапа
- Мультифайловые WorkspaceEdit в неоткрытых файлах зависят от клиентской
  поддержки (VS Code — ок; поведение задокументировать)
- Homebrew tap — отдельный репозиторий вне этого repo; winget — PR в
  microsoft/winget-pkgs с задержкой ревью (не блокирует тег релиза)
- Секреты `VSCE_PAT` / `OVSX_PAT` должны быть настроены до задачи 7

## Scope Estimate

Мультизадача: 9 подзадач, каждая — отдельная ветка `task/<name>` → PR.
Порядок снизу вверх по ячейкам, от быстрых побед к крупным:

| # | Задача | Ячейки | Объём |
|---|--------|--------|-------|
| 1 | document-link — инклуды как ссылки | server (+rms ranges) | малый |
| 2 | selection-range — выделение по AST | server (+rms/xs хелперы) | малый |
| 3 | rename: парсеры — rename-сайты с затенением | xs, rms | средний |
| 4 | rename: сервер — PrepareRename/Rename по замыканию | include, server | средний |
| 5 | format: контракт + bootstrap ячейки + RMS | format (новая) | средний |
| 6 | format: XS + server-хендлер Formatting | format, server | средний |
| 7 | publish extension — версии, vsce/ovsx из CI | editors, scripts, CI | малый |
| 8 | release 1.0.0 — changelog, тег, README-инструкции по платформам | docs | малый |
| 9 | package managers — winget + Homebrew tap | внешняя инфраструктура | малый-средний |

## Existing Architecture

- `internal/server` — все новые хендлеры и capabilities; DI по паттерну ячейки
- `internal/xs`, `internal/rms` — rename-сайты, selectionRange-хелперы
- `internal/include` — замыкание для мультифайловых правок rename
- `internal/format` — новая ячейка: brainstorm → CODEMANIFEST apply →
  реализация (паттерн complete/hints: протоколо-независимая логика)
- `editors/vscode`, `scripts/release.sh`, `.github/workflows/release.yml` —
  публикация и версионирование

## Notes

Решения сессии (2026-09-09):

- Граница «полного функционала» 1.0.0 = ядро (rename, documentLink,
  selectionRange) + форматтер + публикация; InlayHint и автозагрузка
  бинарника — 1.x
- Форматтер — отдельная протоколо-независимая ячейка `internal/format`
- Установка: README-инструкции по платформам + winget и Homebrew tap;
  scoop/AUR — 1.x
- Cook-обновления разложены по задачам: `lsp-protocol.md` (1, 2, 4, 6),
  `vscode-extension.md` (7), `github-actions.md` (7, 9),
  `package-managers.md` создаётся в задаче 9
- Rename-дизайн задачи 3 стартует с аудита переименовываемых сущностей RMS
  (см. риск выше)
