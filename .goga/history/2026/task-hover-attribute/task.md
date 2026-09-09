# hover-attribute: справка вложенного поля вместо внешнего объекта

## Current State

Пользовательский отчёт (VS Code, v0.4.0, сервер поднят): при наведении на
вложенное поле (атрибут внутри блока команды, напр. `number_of_objects`
внутри `create_object { … }`) hover показывает справку **внешнего объекта**
(`create_object`), а не самого поля.

Предварительный анализ (глубокий разбор не проводился):

- `RmsFile.StatementAt` (`internal/rms/ast.go:113`) документирован как
  «A position on an attribute inside a command block returns the owning
  command» — владелец-команда возвращается намеренно
- `Server.hoverRms` (`internal/server/server.go:495`) после StatementAt
  спрашивает только `Store.Command(stmt.Name)` — справка по атрибуту
  (`Store.Attribute` с владельцем) в hover не консультируется вовсе

## Description

Hover на позиции вложенного поля должен показывать справку этого поля:
описание атрибута из kb (CommandArg владельца — текст, тип/диапазон
значений), а не справку внешней команды. Справка внешней команды
остаётся корректной для позиций на самой команде и её аргументах.

## Scope

**In scope (предварительно, уточнить при формулировке/design):**
- семантика StatementAt/deepestAt для позиций на строке атрибута
- hoverRms: ветка атрибута — владелец из AST + `Store.Attribute`
- рендер markdown для атрибута (desc, значение/диапазон из kb)
- тесты: фикстура с вложенным блоком, hover на поле ≠ справка владельца

**Out of scope:**
- XS-часть (пользовательский отчёт — про RMS-блоки)
- semantic tokens / highlighting (отдельная задача vscode-highlighting)

## Acceptance Criteria

- hover на `number_of_objects` внутри `create_object` показывает справку
  атрибута; hover на `create_object` — справку команды (не сломано)
- unknown-атрибут → прежнее поведение (молчание/справка владельца —
  решить на design-этапе)
- `make check` зелёный; `goga contract` затронутых ячеек зелёный

## Stack

- **Frameworks:** Go 1.26+ (stdlib)
- **Libraries:** существующие kb/rms/server
- **Infrastructure:** —

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| — | — | нет |

## Risks and Constraints

- Возможна легитимность текущего поведения для позиций «в хвосте» блока —
  нужен аудит позиционной семантики на design-этапе
- kb-данные атрибутов должны содержать достаточно текста для осмысленного
  hover (проверить полноту CommandArg desc)

## Scope Estimate

Микро-задача (одна ветка task/hover-attribute → PR): rms-семантика +
server-ветка + тесты. Формулировка зафиксирована по пользовательскому
отчёту; перед реализацием — короткий дизайн-проход.

## Existing Architecture

- `internal/rms` — StatementAt/deepestAt/Children (random/conditional
  блока; атрибуты команд — отдельная зона AST)
- `internal/kb` — Store.Command/Store.Attribute
- `internal/server` — hoverRms/hoverXs

## Notes

- Зафиксировано по репорту пользователя 2026-09-10 (сессия синхронизации
  манифестов); диагностика остановлена по требованию — план выше
  предварительный
- Порядок: независима от эпика v1.0.0; хорошо ложится рядом с
  selection-range (#2) — та же позиционная семантика AST
