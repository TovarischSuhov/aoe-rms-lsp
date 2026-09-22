---
type: Architecture
title: Архитектура aoe2-lsp
description: Слои и порядок зависимостей 13 ячеек internal/, entrypoints в cmd/, путь данных от LSP-запроса до парсеров и базы знаний; подробно не пересказывает планы, а указывает на них.
sources:
  - resource: docs/arch/lsp-rms-xs.md
  - resource: docs/plans/build-lsp-rms-xs.md
  - resource: README.md
  - resource: internal/include/source.go
generated:
  by: claude-code/glm-5.3
  at: 2026-09-22T07:12:06Z
verified:
  - by: claude-code/glm-5.3
    at: 2026-09-22T07:12:06Z
---

# Архитектура aoe2-lsp

LSP-сервер (Go, stdio) для двух языков AoE2 DE: RMS (секционно-декларативный
язык карт) и XS (C-подобный скриптовый). Подробные контракты, диаграмма и
обоснования порядка — в авторитетном плане `docs/arch/lsp-rms-xs.md`;
здесь — карта, чтобы не читать план целиком.

## Слои и порядок зависимостей

Стартовые шесть ячеек строились в порядке leaves → root
(`docs/arch/lsp-rms-xs.md`, раздел Implementation Order):

1. `internal/common` — лист: чистые типы данных (Pos, Range, Diagnostic,
   OutlineSymbol, токены), нужны всем, зависимостей нет.
2. `internal/kb` — лист: embedded JSON (XS-функции, константы, RMS-команды)
   + пайплайн генерации из `docs/ref/`.
3. `internal/rms` — лист: RMS-парсер (лексер → AST с recovery).
4. `internal/xs` — лист: XS-парсер (C-подобная грамматика, rules/events,
   externs из дампа игры `prelude.xs`).
5. `internal/analysis` — семантические проверки над AST + kb
   (unknown-*, bad-arity, типы значений, semantic tokens).
6. `internal/server` — корень: LSP-сервер поверх `go.lsp.dev/protocol`.

Проект вырос до 13 ячеек — добавились протокольно-независимые вычисления
(`hints`, `complete`, `highlight`, `format`), include-замыкание (`include`),
корпусный прогон (`corpus`) и детектор патч-постов (`news`, в рантайме
сервера не участвует). Новые ячейки следуют тому же шаблону: контракт в
`CODEMANIFEST`, потребительские практики в `.usages/`.

Ключевой приём развязки: интерфейс объявляет консюмер. Пример —
`internal/include/source.go`:`Source` инвертирует зависимость include от
кэша документов сервера (реализуется сервером, ячейка о нём не знает).

## Путь данных

- `didOpen`/`didChange` → парсинг (`rms`/`xs`, error recovery, полный файл —
  full-text sync) → `analysis` поверх `kb` → публикация диагностик.
- hover/completion/signature help черпают подписи и описания из `kb`.
- Навигация (definition/references/symbols) идёт по include-замыканию
  (`internal/include/.usages/closure.md`): открытые документы берутся из
  состояния редактора (через `Source`), невзятые — с диска; цели могут жить
  в файлах, не открытых в редакторе.
- Настройки применяются live: смена settings перепубликует диагностику без
  рестарта; файловые вотчеры форс-перезагружают изменённые на диске инклюды.

## Entrypoints (cmd/)

`cmd/` — тонкие точки входа, логики не содержат:

- `aoe2-lsp` — сам сервер (stdio; логи в stderr, `-debug` для трассировки).
- `kbgen` — регенерация embedded KB (см. [пайплайн данных](/runbooks/kb-data-pipeline.md)).
- `corpus` — прогон бинарника сервера по корпусу реальных карт (gate).
- `tmgen` — регенерация TextMate-грамматики RMS для VS Code.
- `newscheck` — детектор патч-постов AoE2 DE (утилита мониторинга KB).

## Редакторы

Сервер редактор-agnostic; в дереве живёт расширение VS Code
(`editors/vscode`): языковые конфигурации, TextMate-грамматики, LSP-клиент,
автозагрузка бинарника из GitHub Releases (SHA256-проверка). Настройки —
общая секция `aoe2lsp` (severityOverrides, includeRoots). Детали интеграции:
`internal/server/.usages/lifecycle.md`.
