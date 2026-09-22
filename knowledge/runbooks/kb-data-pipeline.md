---
type: Runbook
title: Пайплайн данных базы знаний
description: Регенерация embedded JSON (internal/kb/data) из локальных источников docs/ref/ через kbgen, полудифф-режим kbgen -diff, патч-день (docs/kb-refresh.md, ночной kb-monitor + news), лицензионные следствия GPL-3.0.
sources:
  - resource: internal/kb/.usages/data-pipeline.md
  - resource: docs/kb-refresh.md
  - resource: .github/workflows/kb-monitor.yml
  - resource: README.md
  - resource: internal/news/CODEMANIFEST
generated:
  by: claude-code/glm-5.3
  at: 2026-09-22T07:12:06Z
verified:
  - by: claude-code/glm-5.3
    at: 2026-09-22T07:12:06Z
---

# Пайплайн данных базы знаний

Embedded KB (`internal/kb/data/*.json`) — то, что сервер отдаёт в hover,
completion и signature help. Генерируется из локальных источников, сеть для
данных не нужна.

## Источники → артефакты

| Источник (docs/ref/) | Артефакт (internal/kb/data/) |
|---|---|
| `ugc-guide/xs/functions/functions.json` (204 функции) | `xs-functions.json` |
| `ugc-guide/xs/constants/constants.json` (27 секций) | `xs-constants.json` |
| `zetnus-rms-guide.txt` (Syntax Skeleton) | `rms-commands.json` (via ExtractRmsCommands) |
| `aoe2de-xs-rms-changelog.md` | обогащение `since_update` |
| `attribute-descs.json` | опциональный desc-overlay (файл проекта; пустой — норма, отсутствует → WARN, битый → падение сборки) |

## Регенерация

Единая точка входа — `kb.GenKB` (адаптация, извлечение, обогащение,
детерминированная запись с сортировкой ключей); `cmd/kbgen` — только флаги:

```sh
go run ./cmd/kbgen
```

После регенерации обязательны тесты kb: дубликаты имён в одном файле —
ошибка сборки (разрешать, не пропускать); `NewStore()` должен проходить.
Семантика майнинга описан (glossary → mining → overlay, fill-when-empty,
идемпотентность load-пути) в `internal/kb/.usages/data-pipeline.md` —
при правке пайплайна читать его.

## Обновление на патчах игры

Процесс патч-дня (чеклист источников, поддержка `since_update`, проверка) —
`docs/kb-refresh.md`. Полуавтоматический шаг «что нового»:

```sh
go run ./cmd/kbgen -diff
```

регенерирует в temp-каталог и печатает агрегированный отчёт old→new, не
трогая `internal/kb/data`.

Свежесть мониторит ночной GitHub Action `kb-monitor.yml` (ежедневно,
06:23 UTC): детектор патч-постов (ячейка `internal/news` + `cmd/newscheck`,
RSS ageofempires.com/news/feed/, фильтр заголовков «Age of Empires II +
Update/Hotfix/…») заводит issue с напоминанием обновить KB.

## Лицензии

Проект целиком GPL-3.0, потому что производные данные KB — адаптация
GPL-3.0 UGC Guide. Материал гайда Zetnus используется фактически
(структура команд/атрибутов) с атрибуцией. Корпусные исследования
(`docs/ref/real-map-nuances.md`, `map-scripting-practices.md`) —
собственные, с атрибуцией опубликованных карт. Менять источники данных
без проверки лицензионных следствий нельзя.
