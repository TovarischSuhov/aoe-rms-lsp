# Углубление analysis: значения аргументов RMS и типы XS

Status: Done — влито в master; регрессии — PR #12

## Current State

Ячейка `analysis` реализована по базовому плану (`docs/plans/build-lsp-rms-xs.md`,
Task 5): один `Analyzer(store)` с методами `AnalyzeRms` / `AnalyzeXs`, семь кодов
проверок — уровень имён и арности:

| Область | Коды | Уровень |
|---|---|---|
| RMS | unknown-command, unknown-section, unknown-attribute, bad-argument, deprecated-effect-percent | имя + число/вид аргументов |
| XS | undefined-symbol, bad-arity | имена + число аргументов |

При этом `kb` уже содержит данные, которые анализ не использует:

- `CommandArg.Kind` (`number / percent / const / terrain / object / ...`) —
  ожидаемая форма значения аргумента/атрибута RMS-команды;
- `Param.Type` / `ReturnType` XS-функций (`int / float / bool / string / vector`).

То есть проверяется «правильное ли имя и сколько аргументов», но не «правильное
ли значение/тип».

## Description

Углубить семантические проверки `analysis` двумя группами на уже загруженных
данных `kb` — без новых источников данных и без изменения парсеров:

1. **Значения аргументов RMS** (по `CommandArg.Kind`): диапазоны для percent,
   резолв `const`-значений в `kb.Constants`, несоответствие литерала ожидаемой
   форме (например, число там, где ожидается именованная константа).
2. **Типы XS** (по `Param.Type` / `ReturnType`): сверка типов аргументов при
   вызовах XS-функций, проверка присваиваний и `return`; простой вывод типа
   выражения из литералов и объявленных типов переменных.

Примеры «вход → диагностика» (ориентировочные, финализируются при дизайне
контракта):

```text
RMS: percent = 150                  → bad-argument-value   (percent вне 0..100)
RMS: base_terrain = NOT_A_TERRAIN   → unknown-constant     (const не резолвится в kb)
XS:  xsSetWorldGravity("fast")      → bad-type             (string вместо float)
XS:  int x = 1.5;                   → bad-type             (float-литерал → int)
XS:  vector v = xsGetMapSeed();     → bad-type             (int → vector)
```

## Scope

**In scope:**

- Новые коды диагностик в `AnalyzeRms` (значения аргументов) и `AnalyzeXs`
  (типы); точный перечень кодов — на этапе дизайна контракта.
- Обновление `analysis/CODEMANIFEST`: новые шаги Algorithm, inline-usage с
  таблицей коерции типов XS (`int→float` допустимо, `float→int` — ошибка,
  правила для `vector` и т.п.).
- Table-driven тесты: позитивный и негативный случай на каждую проверку.
- Проверка фактического перечня `Kind` в `kb/data/rms-commands.json` перед
  финализацией кодов (как шаг дизайна).

**Out of scope:**

- Предупреждения по `since_update` / версионность игры.
- Перекрёстные ссылки RMS↔XS между файлами (workspace/DocStore — отдельная
  задача).
- Изменения ячеек `common`, `kb`, `rms`, `xs`, `server` (новые диагностики
  попадают в `server` автоматически через существующий контракт).
- Полноценная система вывода типов XS (generics, перегрузки, межпроцедурный
  анализ).

## Acceptance Criteria

- `go test ./...` зелёный; `golangci-lint run`, `goga lint`,
  `goga contract analysis` — без замечаний.
- Каждая новая проверка покрыта позитивным и негативным тестом.
- Инварианты ячейки сохранены: диагностики отсортированы по позиции, входной
  AST не мутируется, нет дублей синтаксических диагностик парсеров, нет IO.
- Без ложных срабатываний на существующих фикстурах: `prelude.xs`
  (882 extern) и `.rms`-фикстурах из `rms/testdata/`.

## Stack

- **Frameworks:** нет — Go 1.23+, stdlib.
- **Libraries:** testify/cmp для тестов (по `conventions`); новых библиотек нет.
- **Infrastructure:** нет.

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| — | — | внешних зависимостей нет |

Существующие usages покрывают задачу: `conventions`, `rms-grammar`,
`xs-grammar`. Правила коерции типов XS — inline-usage в `analysis/CODEMANIFEST`
(решение утверждено при формулировке).

## Risks and Constraints

- **Ложные срабатывания на prelude.xs**: таблица коерции типов должна быть
  консервативной — при неизвестном/сложном типе выражения диагностика не
  выдаётся.
- **Разнородный `CommandArg.Kind`**: значения вида «number / percent / const /
  terrain / object / ...» — реальный перечень надо снять с
  `kb/data/rms-commands.json` до фиксации кодов проверок.
- **DE-выражения в значениях атрибутов**: диапазоны проверяются только у
  литеральных значений; выражения с операторами пропускаются.
- Анализатор остаётся stateless (без кеша и IO) — по контракту ячейки.

## Scope Estimate

Одна задача. Одна затронутая ячейка (`analysis`), две группы проверок, ~5–8
новых кодов диагностик. Один проход `goga-brainstorm` → план → реализация;
разбивка на две задачи отклонена (две правки одного CODEMANIFEST дороже одной).

## Existing Architecture

- `analysis` импортирует `common.Diagnostic`, `kb.Store` (+ usage `lookups`),
  `rms.RmsFile` (+ `rms-parsing`), `xs.XsFile` (+ `xs-parsing`) — граф
  зависимостей не меняется.
- Потребитель — `server` (через `analysis/.usages/checks.md`): новые
  диагностики доезжают до LSP-клиента без изменений в `server`.

## Notes

- Примеры «вход → диагностика» включены по решению пользователя.
- Имена кодов в примерах (`bad-argument-value`, `unknown-constant`, `bad-type`)
  — рабочие; финальные имена фиксируются в CODEMANIFEST при дизайне.
- В `analysis/.usages/checks.md` замечена незакрытая кодовая секция (```)
  после примера `NewAnalyzer` — поправить попутно при обновлении usage.
