# Бейджи README (CI, coverage, релиз, версия Go)

## Current State

В README один бейдж — License. CI (`ci.yml`) гоняет fmt / `go test -race` /
lint / govulncheck, но покрытие не считает; внешних badge-сервисов проект не
использует. Go Report Card закрыт (sunset) — его бейдж недоступен.

## Description

Собрать в README строку бейджей: CI-статус (GitHub Actions), Release,
Coverage, Go version (из go.mod), Downloads, License. Coverage — без сторонних
SaaS (Codecov/Coveralls): джоба в ci.yml считает покрытие
(`go test -race -coverprofile`), конвертирует тотал в shields-endpoint JSON и
force-push'ит его в орфан-ветку `badges`; бейдж читает JSON через
`img.shields.io/endpoint`.

## Scope

**In scope:**
- `README.md` — блок бейджей
- `.github/workflows/ci.yml` — джоба `coverage` (permissions
  `contents: write` только у неё; публикация только на push в master)
- посев ветки `badges` из локального прогона, чтобы бейдж жил сразу

**Out of scope:**
- сторонние сервисы покрытия (осознанно, self-contained)
- гейты/пороги покрытия в CI
- другие workflows и кодовые ячейки

## Acceptance Criteria

- README: 6 бейджей, все URL валидны и рендерятся
- coverage-джоба публикует бейдж только на push в master; на PR —
  прогон без публикации
- ветка `badges` создана до пуша README
- `make check` зелёный; прямой пуш в master (по явному указанию
  пользователя, docs/infra вне кодовых задач)

## Stack

- **Frameworks:** —
- **Libraries:** — (без новых go-зависимостей)
- **Infrastructure:** GitHub Actions (существующий `ci.yml`), shields.io
  endpoint-бейджи

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| shields.io | — | используется как публичный рендерер бейджей, usage-файл не требуется |

## Risks and Constraints

- Бейдж coverage показывает «invalid», пока ветки `badges` нет → сею ветку
  локально до пуша README.
- shields кэширует endpoint (~5 мин) — бейдж обновляется с задержкой после
  CI.
- actionlint локально нет — валидность workflow проверяет сам CI-прогон.

## Scope Estimate

Одна задача (README + CI, кодовые ячейки не затрагиваются), один коммит в
master.

## Existing Architecture

Ячейки не затрагиваются; README и workflows лежат вне клеточных контрактов.

## Notes

- Go Report Card исключён: сервис закрыт (проверено запросом к API).
- Решение о самодостаточном coverage (без Codecov) — по философии проекта
  (self-contained, минимум внешних связей), принято автономно по делегированию.
