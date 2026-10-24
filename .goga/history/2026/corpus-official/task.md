# corpus-official: локальный корпус официальных карт и XS из установки AoE2DE

Зеркало — GitHub issue #142, PR закрывает `Fixes #142`.

## Current State

Корпус проекта — только пул сообщества: `scripts/corpus-fetch.sh` качает
100 опубликованных RMS/XS по пину `corpus-sources.txt` в gitignored
`.corpus`. Собственных скриптов игры (установленный AoE2DE, Steam app
813780) в обороте нет, хотя это самый репрезентативный материал: 180 карт
`resources/_common/drs/gamedata_x2`, включая крупные DE-карты и сценарные
порты, плюс официальные XS из `resources/_common/xs`. Детект Steam-установки
в проекте существует лишь как бэклог-идея для `includeRoots` (#123).

## Description

Второй, локальный корпус из скриптов самой игры: скрипт-стейджер находит
установку, копирует `.rms`/`.inc`/`.xs` в gitignored-каталог и прогоняет по
ним corpus-харнесс. Контент не коммитится — он Microsoft-copyrighted
(EULA запрещает копирование сверх лицензии и публикацию) и несовместим с
распространением проекта под GPL-3.0.

## Scope

**In scope:**

- `scripts/corpus-official.sh` — детект установки AoE2DE (Steam
  `libraryfolders.vdf` по стандартным путям; переопределение `AOE2DE_DIR`),
  копирование `*.rms`/`*.inc`/`*.xs` в dest (default `.corpus-official`),
  сохранение относительных путей (`includes/…`)
- Makefile-таргет `corpus-official`: `go build -o aoe2-lsp ./cmd/aoe2-lsp`
  → стейдж → `go run ./cmd/corpus -bin ./aoe2-lsp -dir .corpus-official`
- `.gitignore`: `.corpus-official/`
- README (Data sources + Development) и `knowledge/runbooks/verification.md`
  — описание корпуса и лицензионного ограничения (тем же PR)

**Out of scope:**

- резолв `#include_drs` (парсер ведёт его как обычную директиву и в файл
  не резолвит; отдельная задача по ячейке `rms`/`include`)
- прогон `.inc` как самостоятельных документов и изменение `collectScripts`
  харнесса — `.inc` выложены как зеркало установки
- MS Store/Xbox-установка (см. #124)
- изменения Go-кода и CODEMANIFEST ячеек

## Acceptance Criteria

- `make corpus-official` на машине с игрой: стейджится 180 карт + 81 `.inc`
  + 7 `.xs`, харнесс отрабатывает с `HardFailures == 0`
- на машине без игры: понятная ошибка с подсказкой про `AOE2DE_DIR`, exit 2
- `.corpus-official/` невидим для git (`git check-ignore`); в репозитории —
  только скрипт, Makefile, README, knowledge
- `bash -n`/`shellcheck scripts/corpus-official.sh`, `make check` — зелёные;
  CODEMANIFEST ячеек не затронуты

## Scope Estimate

Одна задача S-M, декомпозиция не нужна. Ветвь `task/corpus-official` от
master → PR.

## Existing Architecture

- `internal/corpus` (`Runner`) — прогон каталога `.rms`/`.xs` через полный
  LSP-цикл, жёсткая классификация (panic/exit/timeout/transport)
- `cmd/corpus` — CLI-обвязка харнесса
- `scripts/corpus-fetch.sh` + `corpus-sources.txt` — пул сообщества
- `Makefile` (таргет `corpus`), `.github/workflows/ci.yml` (corpus-джоба)

## Notes

- Первый прогон: 187 файлов, ok 187, hard failures 0 (17 c); диагностик
  1368, лидеры — сценарные карты `BR_FallofRome` (260), `Shipwreck` (248),
  `BR_BattleontheIce` (245) → кандидаты на разбор парсера (вне этой задачи)
- Выложенные `.inc` сейчас на диагностику не влияют (карты подключают их
  через `#include_drs`, не резолвимый парсером) — задел под будущий резолв
- Детект установки частично пересекается с #123 (game-detect для
  `includeRoots`) — логику можно будет переиспользовать
