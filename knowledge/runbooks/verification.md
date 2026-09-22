---
type: Runbook
title: Проверка изменений
description: Предзадачная последовательность make check, тесты только под memory cap, goga contract затронутых ячеек, corpus gate на 100 картах, fuzz-цели, состав CI и рабочий процесс задача → ветка → PR.
sources:
  - resource: Makefile
  - resource: .github/workflows/ci.yml
  - resource: CLAUDE.md
  - resource: scripts/corpus-fetch.sh
generated:
  by: claude-code/glm-5.3
  at: 2026-09-22T07:12:06Z
verified:
  - by: claude-code/glm-5.3
    at: 2026-09-22T07:12:06Z
---

# Проверка изменений

## Стандартная последовательность перед завершением задачи

```sh
make check    # fmt → build → test → lint → goga lint
goga contract <путь затронутой ячейки>   # для каждой изменённой ячейки
```

Дальше — зелёный CI ветки; готовность к ревью заявляется только после него
(транзиентное падение инфраструктуры — rerun, не правка кода).

## Тесты — только под memory cap

`go test` локально запускать только в cgroup-песочнице (runaway-тест уже
валил машину в OOM); `make test` уже оборачивает:

```sh
timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 \
    bash -c 'go test ./... -count=1'
```

Race-прогон — в CI (`go test -race ./...`), не локально.

## Corpus gate

`make corpus`: собирает сервер и гонит его по детерминированной выборке из
100 опубликованных RMS/XS-скриптов. Выборка закреплена SHA в
`scripts/corpus-sources.txt`; `scripts/corpus-fetch.sh` качает её при первом
запуске, CI кэширует `.corpus` по хэшу источников. Регрессия парсера на
реальных скриптах валит гейт. В CI гейт не гоняется на PR (фetch — сетевой
раунд-трип), только master-push и ручной запуск.

## Fuzz

CI крутит короткие сидированные прогоны: `FuzzParse` (internal/rms) и
`FuzzXsParse` (internal/xs), по 30 секунд; на тёплом кэше корпуса фаззер
подхватывает 100 реальных карт, при промахе деградирует до фикстур репо.

## CI (ci.yml)

- `checks` — goimports, gofumpt (`golangci-lint fmt --diff`), `go test -race`,
  golangci-lint v2, govulncheck.
- `vscode` — type-check, тесты, упаковка .vsix и проверка, что каждый путь
  из `contributes` реально лежит в архиве (VS Code молча роняет такие entry).
- `fuzz`, `corpus` — см. выше.
- `coverage` — бейдж через JSON на orphan-ветку `badges` (без стороннего
  сервиса).
- Релизы собирает отдельный workflow по тегу `v*`; выпуск —
  `make release BUMP=patch|minor|major` (changelog из conventional commits,
  коммит, тег, пуш).

## Рабочий процесс

Задача (из `docs/tasks/` или `.goga/history/`) → ветка `task/<имя>` от
актуального `master` → атомарные коммиты → пуш → PR в `master` c `Fixes #N`.
PR не мержится агентом; прямые коммиты в master — только docs и
инфраструктура вне кодовых задач. Голден-файлы обновляются
`go test -update`; заявление «стало быстрее» — бенчмарк до/после через
benchstat.
