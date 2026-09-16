# status-bar: индикатор состояния сервера и его версии

## Current State

Расширение активируется на .rms/.xs и поднимает `LanguageClient`, но не
даёт никакой видимости своего состояния: пользователь не видит, запущен
ли сервер (падение/рестарт проходят молча), и какая версия работает.
`client.onDidChangeState` (Starting/Running/Stopped) в
`editors/vscode/src/extension.ts` не используется. Серверная сторона:
`InitializeResult.ServerInfo` (`internal/server/server.go:358`)
заполняет только `Name` — `main.version` (ldflags, флаг `--version`) до
клиента не доходит. Сущности «версия kb» в проекте нет: kb зашита в
бинарник, её свежесть = версия бинарника (до появления kb-live-update
#62).

## Description

Индикатор в статус-баре VS Code: состояние сервера (starting/running/
stopped) + версия сервера. Состояние — из клиентского state-machine,
версия — из `initializeResult.serverInfo.version`, куда её начинает
отдавать сервер (прокидка `main.version` в ячейку `server`
DI-параметром, `ServerInfo{Name, Version}`). Клик по индикатору
открывает output channel сервера.

Решение сессии (2026-09-16): слот обещал «версию kb», но такой
сущности нет — показываем версию сервера (пользователь утвердил);
реальная версия данных появится с #62 kb-live-update.

## Scope

**In scope:**

- `internal/server`: версия в `ServerInfo` initialize-ответа —
  прокидка из `cmd/aoe2-lsp` (DI-параметр `Serve`/`Server`); CODEMANIFEST
  server отражает изменение контракта
- `editors/vscode/src/extension.ts`: `createStatusBarItem` (Left) —
  метки по состоянию: `$(sync)` Starting, `$(zap) AoE2 LSP vN.N.N`
  Running, `$(circle-slash)` Stopped; тултип с деталями; команда клика —
  показать output channel
- маппинг состояние→метка — чистая функция с node-тестом
- тест сервера: initialize отвечает `ServerInfo.Version` (расширение
  существующего initialize-теста)
- cooks: `vscode-extension.md` (паттерн statusbar),
  `lsp-protocol.md` (ServerInfo.Version в initialize-паттерне)

**Out of scope:**

- версия данных kb (появится с #62 kb-live-update; пока = версия
  бинарника)
- настройка вкл/выкл индикатора (YAGNI)
- серверные метрики/health-checkи, прогресс прогрессов анализа

## Acceptance Criteria

- при открытии .rms/.xs индикатор появляется: Starting → Running с
  версией сервера из `serverInfo.version`
- остановка/падение сервера переводит индикатор в Stopped (не молчит)
- клик открывает output channel
- `initialize` отвечает `ServerInfo{Name: …, Version: …}` — проверено
  тестом сервера
- маппинг состояние→метка покрыт node-тестом
- `make check` зелёный; `goga contract server` зелёный; vscode-джоба CI
  зелёная

## Stack

- **Frameworks:** TypeScript + VS Code API (`createStatusBarItem`,
  `onDidChangeState`, codicons); Go 1.26+ (server)
- **Libraries:** без новых (vscode-languageclient уже используется)
- **Infrastructure:** —

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| VS Code status bar API | `.goga/usages/cooks/vscode-extension.md` | update (в задаче) |
| ServerInfo.Version в initialize | `.goga/usages/cooks/lsp-protocol.md` | update (в задаче) |

## Risks and Constraints

- состояние клиента ≠ состояние процесса сервера точно до кадра —
  индикатор отражает state-machine клиента (достаточно для UX)
- `serverInfo.version` пуст в dev-сборках без ldflags (`dev`) —
  показывать как есть
- version-прокидка меняет публичный контракт ячейки server —
  CODEMANIFEST правится в той же задаче (единственная ячейка)

## Scope Estimate

Одна задача: ветка `task/status-bar` → один PR (Fixes #85). Объём S
(~0.5 дня): серверная прокидка + индикатор + тесты + два cook-абзаца.

## Existing Architecture

- `editors/vscode/src/extension.ts` — activate/deactivate, создание
  `LanguageClient` (extension.ts:89); output channel — из клиентского
  логгера
- `internal/server` — `Serve`/`Server`, initialize-ответ
  `server.go:358`; `cmd/aoe2-lsp/main.go` — `main.version` (ldflags)
- паттерн статуса в cook `vscode-extension.md` — по существующим секциям

## Notes

- Решение пользователя (2026-09-16): показываем версию сервера, не
  выдумываем версию kb
- Слот пачки ux-wave-2 (`2026/ux-wave-2/task.md`), зеркало — issue #85
- Версия до клиента: единственный источник — `initializeResult` (не
  `--version`-запуск бинарника расширением)
