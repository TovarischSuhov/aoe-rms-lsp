---
type: Contract
title: Ячеечные контракты CODEMANIFEST
description: goga-устройство репозитория — CODEMANIFEST как публичная поверхность ячейки (read-only в кодовых задачах, проверяется goga contract), .usages/ потребительские практики, приоритет CLAUDE.md над conventions.md, интерфейсы объявляет консюмер.
sources:
  - resource: docs/plans/build-lsp-rms-xs.md
  - resource: .goga/config.yml
  - resource: .goga/usages/conventions.md
  - resource: internal/news/CODEMANIFEST
  - resource: internal/include/source.go
generated:
  by: claude-code/glm-5.3
  at: 2026-09-22T07:12:06Z
verified:
  - by: claude-code/glm-5.3
    at: 2026-09-22T07:12:06Z
---

# Ячеечные контракты CODEMANIFEST

Репозиторий следует [goga](https://pypi.org/project/goga/) CODEMANIFEST-воркфлоу:
ячейка = `internal/<pkg>`, контракт = `CODEMANIFEST` в её корне.

## CODEMANIFEST

- Формат: заголовок (`Usages`, `Annotations`), тело — блоки по типам
  `"Имя(параметры)"` с `location`, `annotations`, `properties`, `methods`
  (в аннотациях — Requirements/Constraints/Algorithm), футер
  Author/CreatedAt/Description.
- **Публичная поверхность ячейки**: реализация обязана экспортировать имена
  ровно как в контракте; проверяется `goga contract <cell>`.
- **Read-only в кодовых задачах**: менять CODEMANIFEST как попутное следствие
  кодовой правки нельзя — изменение контракта это отдельное решение
  (своя задача/PR).

## .usages/

- `<cell>/.usages/` — как потреблять ячейку (пишет потребитель, не автор
  ячейки). Перед работой с ячейкой читать её CODEMANIFEST + .usages.
- `.goga/usages/` — проектные практики: `conventions.md` (обязательные
  правила Go: 1.26+, goimports, DI-конструкторы, context-first, `%w`,
  slog, testify/cmp, table-driven), `rms-grammar.md` / `xs-grammar.md`
  (грамматики языков), `cooks/lsp-protocol.md` (паттерны go.lsp.dev).

## Приоритет правил

При конфликте **CLAUDE.md важнее `conventions.md`** (зафиксировано в самой
conventions.md, раздел Priority).

## Интерфейсы

**Интерфейсы объявляет консюмер**, не поставщик. Действующий пример —
`internal/include/source.go`: `Source` (текст открытых документов) объявлен
в include и реализован сервером. Когда интерфейсов станет много — конвенция
`<cell>/requirements.go`, там же `go generate` для API и моков. Принимай
интерфейсы (`Source`), возвращай структуры.

## Планы и задачи

- Корневой план сборки — `docs/plans/build-lsp-rms-xs.md` (реализован);
  продолжения — `docs/tasks/*.md` и планы `docs/plans/lsp-navigation.md`,
  `cross-file-navigation.md`, `analysis-deepening.md`.
- Сформулированная задача живёт в `.goga/history/<год>/<топик>/task.md`
  и зеркалится в GitHub issue; мерж PR закрывает issue (`Fixes #N`).
- Проверки воркфлоу: `goga lint` (в `make check`) и `goga contract` для
  затронутых ячеек — см. [проверку изменений](/runbooks/verification.md).
