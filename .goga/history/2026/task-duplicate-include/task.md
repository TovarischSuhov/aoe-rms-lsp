# duplicate-include: warning на повторное подключение файла в замыкании корня

Слот пачки ux-and-data-quality; зеркало — GitHub issue #58, PR закрывает
`Fixes #58`.

## Current State

Server в `DidOpen`/`DidChange` уже вычисляет `include.Resolver.Closure`
и мапит записи `MissingInclude` с `Owner==uri` в диагностики корневого
документа (шаг пересчёта диагностики, `internal/server/CODEMANIFEST`).
`Closure.Resolved` несёт все директивы замыкания (Owner, `Include`
с Range, Target) — ячейка `include` повторы директив не выбрасывает,
данные для проверки готовы. Движок игры повторный include выполняет
повторно (дублирование эффектов — реальная боль RMS), но диагностики
на это нет.

## Description

Warning `duplicate-include` на повторную директиву `#include`/
`#includeXS` того же файла в замыкании одного корневого документа.
Область — только директивы корневого документа (`Owner==root uri`,
паттерн missing-include); транс-файловые дубли — отдельная задача.

## Scope

**In scope:**

- Семантическая сверка до кода (риск пачки): по корпусу и справочнику
  подтвердить, что двойной include дублирует эффекты и в корпусе нет
  легитимных ✅-паттернов повторного включения; находки → решение
  (hint вместо warning / отмена чека) фиксируется в Notes этой задачи
- Чек рядом с missing-include: группировка `Closure.Resolved` по
  каноническому target; target с ≥2 директивами `Owner==root` →
  Diagnostic (code="duplicate-include", severity warning, range
  директивы) на каждой повторной директиве — первая не помечается;
  смешанные `#include`+`#includeXS` одного target — тоже дубль
- severityOverrides работают через общий механизм (`none` подавляет)
- Тесты table-driven (same package): двойной `#include`, повтор
  `#includeXS`, смешанный случай, единственное подключение — пусто,
  транзитивный дубль в зависимом файле — пусто
- Дока кода: `internal/server/.usages/lifecycle.md` +
  `docs/ref/map-scripting-practices.md` (по паттерну missing-include)
- Косметика манифеста: упоминание чека в аннотации DidOpen/DidChange
  server (публичная поверхность — типы/методы — не меняется)

**Out of scope:**

- транзитивные дубли (root→A→std, root→B→std): диагностика в
  файлах-владельцах директив требует издания диагностики для
  неоткрытых файлов и расширения алгоритма DidOpen/DidChange —
  отдельная задача-кандидат
- quickfix удаления директивы (удаляем руками)
- `#include_drs`
- изменение семантики разворота Closure (visit-set как есть)

## Acceptance Criteria

- `make check` зелёный; `goga contract` internal/server, internal/include
  зелёный
- фикстура с двойным include одного файла в корневом документе → ровно
  один warning `duplicate-include` на повторной директиве;
  severityOverride `none` убирает его
- единственное подключение и подключения разных файлов — без диагностики
- прогон корпуса: новых срабатываний на ✅-картах нет (или каждое
  обосновано сверкой семантики из In scope)

## Stack

- **Frameworks:** Go 1.26+ (stdlib)
- **Libraries:** без новых зависимостей
- **Infrastructure:** —

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| — | — | внешних зависимостей нет |

## Risks and Constraints

- главный риск — ложные срабатывания на картах, где повторный include
  легитимен; закрывается сверкой семантики до кода
- ключ группировки — канонический путь target (резолв уже даёт
  канонический Target); написание файла в сообщении — как в директиве
- CODEMANIFEST ячеек read-only по публичной поверхности: новый чек
  входит в существующий шаг пересчёта диагностики, контракт не меняется

## Scope Estimate

Одна задача S, декомпозиция не нужна. Ветвь `task/duplicate-include`
от master → PR, `Fixes #58`; после мерджа отметить слот в issue #50
не требуется (слот из пачки, не из эпика).

## Existing Architecture

- `internal/include` — только чтение: `Resolver.Closure`,
  `Resolved []ResolvedInclude` (Owner/Inc/Target) уже содержат повторы
- `internal/server` — шаг пересчёта диагностики DidOpen/DidChange,
  серверный const кода по образцу `codeMissingInclude`
  (`server.go`), общий механизм severityOverrides
- корпус — приёмка на шум (прогон диагностики по корпусу)

## Notes

- Слот S из пачки ux-and-data-quality — реализуется сразу, без
  дизайн-прохода; семантическая сверка до кода обязательна (риск
  пачки)
- Решение пользователя (формулировка 2026-09-14): ловим только
  директивы корневого документа; транзитивные дубли — осознанно
  отдельной задачей
- Issue #58 уже открыта (зеркало слота), новая не создаётся
