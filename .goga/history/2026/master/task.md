# Fix parse/analysis defects from 2026-09-08 review (№1–№5)

## Current State

Полное ревью кода (`docs/reviews/2026-09-08-full-review.md`, master 09161d8)
зафиксировало 11 дефектов. Автопроверки зелёные (go test, -race, vet,
golangci-lint, goimports, govulncheck, goga lint, goga contract × 9) —
дефекты семантические, контракты ячеек их уже запрещают (например,
«не паниковать» в rms). Находки №1–№3 воспроизведены независимо
репро-скриптами (репро включены в ревью-документ).

Неисправленное состояние:

- `rms.Parse("#includeXS\nvoid main() { int x = 1; }")` — паника
  index out of range (падение всего LSP-сервера на didOpen/didChange).
- `for (int i = 0; …)` — 4 ложных `undefined symbol "i"` на каждый цикл;
  Definition/VisibleAt/completion не видят переменную цикла.
- `\` + `\n` в строковом литерале сдвигает все последующие позиции
  на строку вверх.
- `int a = 1, b = 2;` на top level — 2 ошибки, `b` теряется.
- Warning о неявном закрытии незакрытого блока в closeScopes —
  недостижим (мёртвый код).

## Description

Исправить дефекты №1–№5 из ревью 2026-09-08 в рамках существующих
контрактов ячеек. CODEMANIFEST не меняются: все исправления —
приведение реализации к уже задекларированному поведению.

- **№1** `rms/parse.go` `endXsBlock`: обрабатывать `end == len(p.lines)`
  (конечная позиция = конец последней строки, без индексации `starts`).
- **№2** `xs` + `analysis`: переменная `for (int i = …)` собирается
  в локали; семантика области — как в C/XS: видима в init/cond/step/body,
  не видима после цикла. Убирает ложные `undefined`, чинит
  `Definition`/`VisibleAt`/кандидаты complete.
- **№3** `xs/parse.go` `scanString`: `\` + `\n` инкрементирует счётчик
  строк (`s.line++`, `lineStart`), позиции после литерала корректны.
- **№4** `xs/parse.go` `parseTypedDecl`: top-level мультидекларации
  `int a = 1, b = 2;` парсятся симметрично `parseLocalDecl`.
- **№5** `rms/parse.go` `closeScopes`: сравнение с исходной длиной
  среза до усечения — warning реально выдаётся при неявном закрытии.

## Scope

**In scope:**

- №1 паника endXsBlock — rms
- №2 for-loop локали — xs, analysis (+ регрессионный тест в complete)
- №3 сдвиг строк scanString — xs
- №4 top-level мультидекларации — xs
- №5 мёртвый warning closeScopes — rms
- Регрессионные тесты на каждую находку: репро из ревью → table-driven
  тесты по конвенции проекта (`Test<Component>_<Scenario>`)

**Out of scope:**

- Находки №6–№11 ревью (utf-16 конвертация, фантомный XsBlock,
  include-resolver, blankComments)
- Изменения CODEMANIFEST (read-only)
- Рефакторинг и новые фичи

## Acceptance Criteria

- Все пять репро из ревью дают корректный результат (нет паники,
  0 ложных undefined, корректные Line, обе декларации, warning выдан)
- Репро оформлены регрессионными тестами в соответствующих пакетах
- Существующие тесты не ломаются (SC-правило: менять только ожидания,
  прямо связанные с исправляемым поведением)
- `timeout 300 systemd-run --user --scope -p MemoryMax=1500M
  -p MemorySwapMax=0 bash -c 'go test ./... -count=1'` — зелёный
- `goimports -w .`, `golangci-lint run`, `goga lint`,
  `goga contract rms xs analysis complete` — все exit 0

## Stack

- **Frameworks:** Go 1.23+ (стандартная библиотека)
- **Libraries:** testify (assert/require) — уже в go.mod
- **Infrastructure:** нет

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| — | — | новых внешних зависимостей нет |

Файлы практик (`cooks`) не требуются: правки внутри существующих
ячеек, новых библиотек/инструментов не появляется.

## Risks and Constraints

- №2 затрагивает сразу три ячейки (xs, analysis, complete):
  изменение сбора локалей может сместить ожидания существующих тестов
  — менять только те, что прямо связаны с областью видимости for.
- №5 включает ранее мёртвый warning: реальные .rms с неаккуратными
  блоками начнут получать диагностику — проверить на тестовых
  fixture проекта, чтобы не устроить «диагностический шум».
- №1: тщательно покрыть граничные случаи (пустой файл, только
  `#includeXS`, блок в конце/начале файла) — парсер обязан
  обрабатывать произвольный вход без паники (контракт rms).
- Тесты — только под memory cap (CLAUDE.md).

## Scope Estimate

Разбиение на 3 подзадачи по ячейкам, каждая — самостоятельная ценность,
ветка `task/fix-parse-bugs` (или три ветки по подзадаче — по протоколу
task → branch → PR):

1. **rms**: №1 (паника) + №5 (мёртвый warning)
2. **xs**: №3 (scanString) + №4 (мультидекларации)
3. **xs + analysis + регрессия complete**: №2 (for-loop локали)

## Existing Architecture

Затрагиваемые ячейки и контракты (все — корректировка реализации
внутри контракта, без правки CODEMANIFEST):

- `rms` — `Parse`, RmsFile/XsBlock (№1, №5)
- `xs` — `XsParse`, XsFile.VisibleAt/collectLocals, scanString,
  parseTypedDecl (№2, №3, №4)
- `analysis` — Analyzer.AnalyzeXs/collectLocals (№2)
- `complete` — только регрессионный тест: кандидаты содержат
  переменную цикла (№2, без правок кода, если VisibleAt починен в xs)

Направление зависимостей: complete → xs/analysis → common; правки
не меняют сигнатур, только поведение.

## Notes

- Источник задачи: `/goga-propose docs/reviews/2026-09-08-full-review.md`
- Пользователь утвердил: объём №1–№5, разбиение на 3 подзадачи,
  стек без новых зависимостей.
- Репро №1–№3 уже проверены независимо (раздел «Репро» в ревью) —
  использовать их как заготовки тестов.
- Приоритет мерджа: подзадача 1 (паника) — первая.
