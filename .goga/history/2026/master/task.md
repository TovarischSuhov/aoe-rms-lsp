# Ячейки под internal/ (соответствие шаблону new-go-project)

## Current State

Девять goga-ячеек лежали в корне модуля — осознанное отклонение от шаблона
new-go-project (`internal/` — вся логика), зафиксированное в CLAUDE.md.
Пользователь решил привести структуру к шаблону.

## Description

Перенести ячейки в `internal/<cell>`, обновив импорты, `From:` манифестов,
пути kb/data в контрактах/практиках, дефолты kbgen, относительные пути
тестов, CLAUDE.md/README. Семантика контрактов не меняется — только пути.

## Scope

**In scope:** git mv девяти ячеек; импорты `aoe2-lsp/internal/<cell>`;
`From: internal/<cell>`; kbdata-текст и data-pipeline.md; kbgen default
out; `"../docs` → `"../../docs` в тестах; CLAUDE.md Structure (флаг
отклонения снимается); README layout.

**Out of scope:** исторические доки (docs/tasks, docs/arch — snapshot
прошлого); release.yml/ci.yml (пути не ссылаются на ячейки).

## Acceptance Criteria

- `make check` зелёный; `goga lint` 9/0; `goga contract internal/kb` —
  сигнатуры без изменений (только ключи ячеек с префиксом internal/)
- `go run ./cmd/kbgen` регенерирует в internal/kb/data (пути в дефолтах)
- CLAUDE.md/README описывают новую структуру; отклонение internal/ снято
- Один PR, атомарные коммиты

## Risks and Constraints

Механический рефакторинг; goga поддерживает вложенные пути ячеек
(проверено lint'ом до массовых правок).

## Scope Estimate

Одна задача, один PR.

## Existing Architecture

Граф ячеек не меняется; потребители — cmd/ и тесты.
