# format-rms: ячейка internal/format + экспорт комментариев rms — RMS-форматтер

## Current State

- Ячеек 12 (`common`, `kb`, `rms`, `xs`, `analysis`, `complete`, `hints`,
  `include`, `server`, `corpus`, `highlight`, `news`); `internal/format` нет.
  Форматирование — последняя крупная LSP-поверхность эпика v1.0.0 (задача 5
  из 10); XS-форматтер и server-хендлер — следующая подзадача #46
- `rms.Parse` даёт AST с Range на каждом узле; statements несут всё
  содержимое файла: команды + позиционные аргументы + атрибуты, блоки
  random/conditional, `#const`/`#define`/`#include_drs` (statements),
  `#include`/`#includeXS` (Includes/XsIncludes), inline-XS (`XsBlock.Code`
  вербатим). **Комментарии в AST не входят**: лексер гасит `/* */`, `//` и
  `#`-строки, диапазоны хранятся в неэкспортируемом `RmsFile.comments`
  (сегодня используются только `ArgAt`)
- Инвариант `parse(format(x)) ≡ parse(x)` сравнивает AST: пока комментариев
  в AST нет, их потеря инвариантом не ловится — поэтому экспорт
  комментариев входит в задачу
- Прецеденты: бутстрап новой ячейки — `hints` (brainstorm → CODEMANIFEST →
  реализация, протоколо-независимая); засев тестов из корпуса — fuzz-тесты
  rms/xs читают `.corpus`, когда скачан (CI-гейт качает 100 карт);
  corpus-раннер — чёрный ящик по LSP-циклу, для инвариантов этой задачи не
  подходит (хендлера Formatting ещё нет — #46)

## Description

Новая протоколо-независимая ячейка `internal/format` (паттерн
`complete`/`hints`) и минимальная правка контракта `rms`: RMS-форматтер —
AST-принтер.

- **rms**: экспорт `Comments -> []Range` в `RmsFile` (те же диапазоны, что
  сегодня в неэкспортируемом поле); тексты комментариев форматтер читает
  из исходника по диапазонам
- **format**: контракт ячейки через brainstorm → CODEMANIFEST → bootstrap;
  форматтер печатает файл заново из AST, вставляя комментарии; вход с
  error-диагностиками → отказ без изменения текста (warning'и —
  форматируем); опции протоколо-независимые (tabSize, табы/пробелы) —
  сервер (#46) проксирует из LSP FormattingOptions; EOL — доминирующий
  перевод строк входа сохраняется (CRLF-безопасность); inline-XS
  (`XsBlock.Code`) — вербатим
- Инварианты: `parse(format(x)) ≡ parse(x)` — структурная эквивалентность
  AST без Range, включая полноту комментариев; `format(format(x)) ≡
  format(x)` — байт-в-байт. Прогон на golden-фикстурах и корпусе 100 карт
  (пропуск теста, когда `.corpus` не скачан)

## Scope

**In scope:**
- brainstorm + CODEMANIFEST + `.usages/` ячейки `internal/format`
- rms: экспорт `Comments -> []Range` (минимальная правка контракта)
- RMS-форматтер: отступы, пустые строки, раскладка
  секций/команд/атрибутов (конкретный стиль — решение design-этапа)
- опции форматирования в API ячейки (протоколо-независимые)
- golden-тесты (`testdata/`, `-update`) + инвариант-тесты на `.corpus`

**Out of scope:**
- XS-форматтер и содержимое inline-XS блоков (#46)
- server-хендлер `textDocument/formatting`, capability, маппинг LSP
  FormattingOptions (#46)
- клиент VS Code
- прочие изменения поверхностей rms (references, semantic tokens,
  diagnostics не меняются)

## Acceptance Criteria

- `parse(format(x)) ≡ parse(x)` структурно (без Range) на golden-фикстурах
  и корпусе 100 карт; все комментарии входа присутствуют в выходе
- `format(format(x))` байт-идентичен `format(x)`
- вход с error-диагностикой → форматтер отказывает, текст не меняется;
  вход с warning'ами — форматируется
- CRLF-вход остаётся CRLF (доминирующий EOL сохраняется)
- `make check` зелёный; `goga lint`, `goga contract rms|format` зелёные

## Stack

- **Frameworks:** Go 1.26+ (stdlib first), без новых зависимостей
- **Libraries:** ячейки `common` (Pos/Range, read-only), `rms` (modify:
  Comments), новая `format`
- **Infrastructure:** corpus-гейт CI (без изменений — источник `.corpus`
  для инвариант-тестов)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| — | — | нет: LSP-поверхность (Formatting) появляется в #46, туда же update `lsp-protocol.md` из таблицы эпика |

## Risks and Constraints

- AST может не нести отдельные литеральные детали (пишет ли `Expr`
  проценты/строки как в исходнике) — первый шаг design-этапа: аудит
  полноты AST против корпуса; инварианты на корпусе — страховка
- якорение комментариев в перестроенном тексте (перенос, слипание с
  командами) — решение design-этапа
- правка стабильного контракта rms — держать минимальной (одно свойство,
  без изменения существующих)
- стиль не должен менять семантику позиционных атрибутов: порядок
  аргументов и атрибутов неизменен; `effect_percent` и прочее — как есть
- производительность: форматирование — разовый вызов на команду клиента,
  без новых индексов

## Scope Estimate

Одна задача, средний объём (по таблице эпика). Ветка `task/format-rms` →
PR, `Fixes #45`.

## Existing Architecture

- `internal/rms` — `Parse` → `RmsFile` (Sections/Statements/Expr/Includes/
  XsIncludes/XsBlocks, Range на узлах), внутренние `comments
  []common.Range` — экспортируемая поверхность будущей правки
- `internal/format` — новая ячейка: brainstorm → CODEMANIFEST apply →
  реализация (паттерн complete/hints: протоколо-независимая логика)
- `internal/server` — будущий потребитель (#46): хендлер Formatting +
  маппинг опций
- прецедент засева из корпуса: `seedFuzzFixtures` в fuzz-тестах rms/xs

## Notes

- Решения propose-сессии 2026-09-21 (пользователь): механизм — AST-принтер
  с экспортом комментариев из rms (не токен-уровень, не line-based);
  error-диагностики → отказ; опции в API уже сейчас; EOL сохраняется;
  inline-XS вербатим; конкретный стиль — design-этап
- Целевой вызов (якорь для design-этапа, имена НЕ фиксируются):

```go
out, err := format.RMS(source, format.Options{TabSize: 4, IndentTabs: false})
// err != nil — вход с error-диагностиками: потребитель текст не меняет
// вариант с передачей уже разобранного *rms.RmsFile — решение design-этапа
```
