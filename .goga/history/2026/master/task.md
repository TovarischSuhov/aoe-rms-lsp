# Сверка CLAUDE.md с шаблоном new-go-project + Makefile + релизный скрипт

## Current State

CLAUDE.md был смерджен с шаблоном new-go-project в `6606dcd`: скелет секций
взят, но Conventions местами потеряны («результат — одним assert'ом», golden
files/benchstat, расширенная формулировка про тривиальные тесты), Commands
шаблона живут вокруг Makefile, которого в проекте нет. Релизы: тег v* вручную,
release.yml собирает и публикует с --generate-notes; curated-ченджлога нет.

## Description

Добрать применимые правила шаблона в CLAUDE.md; завести Makefile (зеркало
шаблона, адаптация: memory-cap тесты, goga); добавить скрипт выпуска версии
с генерацией/публикацией CHANGELOG.md и тегом, который подхватывает release.yml.

## Scope

**In scope:**
- Makefile: build / test (memory-cap, без -race — race в CI) / lint / fmt /
  check (fmt+build+test+lint+goga-lint) / clean / release
- scripts/release.sh: чистота дерева, синхрон с origin/master, версия
  (major|minor|patch|явный vX.Y.Z), changelog из conventional commits с
  прошлого тега, CHANGELOG.md + коммит + тег + пуш; --dry-run
- CLAUDE.md: недостающие правила Conventions (один assert, golden files +
  benchstat, расширенные тривиальные тесты), make-команды в Commands,
  scripts/ в Structure
- README: раздел Releases (процесс выпуска через make release)
- cooks/github-actions.md: примечание о скрипте в паттерне release build

**Out of scope:**
- internal/pkg из шаблона (goga-ячейки — осознанное отклонение, зафиксировано)
- requirements.go/go-generate/oapi-codegen (не goga-модель)
- Изменение release.yml (скрипт лишь ставит тег; сборка остаётся за CI)

## Acceptance Criteria

- `make check` зелёный; `make test` работает под memory-cap без ручного
  копирования systemd-run
- `scripts/release.sh --dry-run patch` печатает версию и changelog, ничего
  не меняя; `bash -n` чист
- CLAUDE.md не противоречит шаблону в применимых пунктах; отклонения
  (internal/, requirements.go) — осознанные
- CHANGELOG.md не создаётся до первого выпуска
- Один PR из task/*, атомарные коммиты

## Stack

Go 1.26, bash (set -euo pipefail), git/gh — существующие; новых зависимостей нет.

## Risks and Constraints

- Makefile test обязан сохранять memory-cap — иначе runaway-риск
- Скрипт пушит master+тег — только по явному запуску (не в check);
  --dry-run по умолчанию консервативен

## Scope Estimate

Одна задача, один PR.

## Existing Architecture

Вне ячеек: корень репозитория (Makefile, scripts/, CLAUDE.md, README.md,
cooks/github-actions.md). Контракты ячеек не затрагиваются.
