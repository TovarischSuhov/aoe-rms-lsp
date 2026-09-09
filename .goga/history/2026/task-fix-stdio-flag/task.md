# fix-stdio-flag: no-op `-stdio` для совместимости с чужими LSP-клиентами (#37)

## Current State

`cmd/aoe2-lsp/main.go` определяет единственный флаг `-debug`; README
фиксирует «No other flags». Issue #37: пользователь Windows запускает
релизный бинарник (`C:\Users\79105\aoe2-lsp\aoe2-lsp.exe`) через клиент,
который по конвенции добавляет `--stdio` (Neovim-style конфиги, generic
LSP-обёртки VS Code) → Go `flag` отвечает `flag provided but not defined:
-stdio`, exit code 2, соединение не поднимается. Собственное расширение
`editors/vscode` передаёт `args: []` и не затронуто — ломается именно
совместимость с внешними клиентами. Принимать и игнорировать `--stdio` —
стандартная практика LSP-серверов.

## Description

Добавить в `cmd/aoe2-lsp/main.go` no-op булев флаг `-stdio`: принят и
молча игнорируется, поскольку сервер всегда говорит LSP over stdio.
Go-парсер флагов не различает `-stdio` и `--stdio` — одной декларацией
покрываются обе формы, какие клиенты ни передавали бы.

## Scope

**In scope:**
- `cmd/aoe2-lsp/main.go`: `flag.Bool("stdio", false, ...)` с описанием
  уровня «accepted and ignored; the server always speaks LSP over stdio
  (client compatibility)»
- README, секция Install & run («No other flags…»): заметка, что `-stdio`
  принят и игнорируется ради совместимости с LSP-клиентами
- Ссылка на issue в коммите (`Fixes #37`) — закроется при попадании в
  master

**Out of scope:**
- другие no-op флаги (`--node-ipc`, `--socket=…` — транспортов, кроме
  stdio, у сервера нет)
- семантика `--version` (остаётся спец-кейсом `os.Args[1]`, без
  `flag.Parse`)
- extension и серверная логика (`internal/*`) — не затрагиваются

## Acceptance Criteria

- `aoe2-lsp -stdio` и `aoe2-lsp --stdio` стартуют без «flag provided but
  not defined» (сервер жив, ждёт протокол на stdin)
- `-debug` и поведение без флагов не изменились
- README отражает новый флаг
- `make check` зелёный; ячейки не затронуты — `goga contract` не требуется

## Stack

- **Frameworks:** Go 1.26+ (stdlib)
- **Libraries:** стандартный пакет `flag` — ничего нового
- **Infrastructure:** —

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| — | — | нет внешних зависимостей |

## Risks and Constraints

- Задача микроскопическая: главный риск — раздуть её границы; всё сверх
  no-op-флага осознанно out of scope
- Поведение флага тривиально (`flag.Bool` + неиспользуемое значение) —
  отдельный тест не требуется по конвенции «тесты покрывают логику, а не
  пересказ сигнатур»; приёмка ручным smoke-запуском

## Scope Estimate

Микро-задача: ~5 строк кода + README, одна ветка `task/fix-stdio-flag`
→ PR. Декомпозиция не требуется.

## Existing Architecture

- `cmd/aoe2-lsp` — тонкий entrypoint вне клеточных контрактов (нет
  CODEMANIFEST); правка не касается `internal/server`
- README — единственное место, документирующее CLI-поверхность бинарника

## Notes

Решения сессии (2026-09-09):

- Вынесена в отдельную задачу из формулировки vscode-autodownload: это
  совместимость сервера с чужими клиентами, к автозагрузке бинарника
  отношения не имеет
- Формулировка утверждена пользователем 2026-09-09 без правок
