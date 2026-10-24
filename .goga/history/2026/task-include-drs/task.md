# include-drs — пропущенные директивы свежего корпуса: резолв #include_drs + распознавание #undefine/#cost

Зеркала — GitHub issues #146 (`#include_drs`), #147 (`#undefine`), #148
(`#cost`); PR закрывает их (`Fixes #146`, `#147`, `#148`).

## Current State

Свежая выгрузка официального корпуса (`.corpus-official`, стейджится из
установленной игры скриптом `scripts/corpus-official.sh`) вскрыла директивы,
которые парсер RMS обрабатывает неверно. Полный свип по трём выгрузкам
(`.corpus`, `.corpus-ext`, `.corpus-official`):

| Директива | official / ext / base | Сейчас | Проблема |
|---|---|---|---|
| `#include_drs` | 2649 / 160 / 68 | роутится в `default` → обычный statement (`internal/rms/parse.go:589-591`) | не попадает в `file.Includes` → нет замыкания, навигации, диагностики |
| `#undefine` | 1 / — / — | не в `directiveWords` (`parse.go:860-866`) → комментарий | строка невидима парсеру |
| `#cost` | — / 2 / — | не в `directiveWords` → комментарий | опечатка `#const` проходит молча |

`#include_drs` — доминирующая форма подключения в DE: официальные карты
используют её 2649 раз и **ни разу** bare `#include` (который гайд вообще
относит к «Non-Functional Syntax»). Код-факты: `internal/rms/parse.go:860-866`
(таблица директив), `parse.go:566-593` (диспетчер: только `#include` и
`#includeXS` кладутся в `Includes`/`XsIncludes`), `internal/include/resolver.go:128-130`
и `353-355` (в замыкание идут только `Includes`/`XsIncludes`),
`internal/format/rms.go:305-309` (форматтер — то же).

## Description

Резолвить `#include_drs` как полноценный include (в замыкание, навигацию,
диагностику отсутствующего файла) — по образцу `#include`; распознать
`#undefine` как директиву-класс строки (семантического действия движка нет);
диагностировать `#cost` как опечатку `#const`. Три директивы — один связный
слой (диспетчер директив RMS), поэтому одна задача/PR.

## Scope

**In scope:**

- `internal/rms/parse.go` — `#include_drs` через `includeArg` в `file.Includes`
  (как `#include`), а не в default-ветку
- `internal/rms` — `#undefine` в `directiveWords`: распознать как директиву
  (без семантического действия), консистентно с `#define`/`#const`
- `internal/rms`/`internal/analysis` — диагностика опечатки `#cost` → `#const`
  (ср. #106, typo aliases → warning с правильным именем)
- тесты: директива в AST, замыкание с `includes/*.inc`, классификация
  `#undefine`, фикстура с `#cost`
- замыкание/навигация/диагностика подхватывают `#include_drs` автоматически
  (существующий механизм)

**Out of scope:**

- game-detect / `includeRoots` (issues #82/#123): резолв `#include_drs`
  относительно gamedata-каталога в произвольной карте — вне этой задачи
- изменение/закрытие issue #98 (там `#include_drs` названа legacy → hint) —
  оставить на ревью
- прочие «Non-Functional» токены гайда (`min_distance`, `set_position`,
  `percent_of_land`, …) — не директивы
- расширение таблицы прочих `#`-директив за пределами найденного корпусом

## Acceptance Criteria

- `#include_drs` попадает в `file.Includes`; замыкание включает цель;
  go-to-definition по пути работает; отсутствующий файл → `MissingInclude`
- `#undefine` классифицируется как директива, а не комментарий
- `#cost` даёт диагностику с предложением `#const`
- новые тесты покрывают все три директивы; существующие не сломаны
- `make check` зелёный; `goga contract internal/rms` (и `internal/analysis`,
  если затронута) зелёный

## Stack

- **Frameworks:** нет (Go 1.26+, module `aoe2-lsp`)
- **Libraries:** только stdlib
- **Infrastructure:** нет; тесты — table-driven, same package, под memory cap
  (`make test`)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| —         | —          | новых внешних зависимостей нет; проектная практика — `.goga/usages/conventions.md` |

## Risks and Constraints

- Пути `#include_drs` в игре относительны gamedata-каталогу, а не файлу:
  резолв относительно includer'а сработает для стейджнутого корпуса (layout
  сохранён `corpus-official.sh` — `includes/…` рядом с картами), но не для
  произвольной карты без `includeRoots`. Не расширять скоуп до game-detect.
- Issue #98 противоречит выводам этой задачи (называет директиву legacy) —
  при фиксе решить её судьбу отдельно.
- `#undefine` в движке — no-op (гайд, «Non-Functional Syntax»): не добавлять
  семантику, которой нет, — только классификация строки.
- `#cost` трактуется как опечатка `#const` (подтверждено: стоит в ряду
  `#const SOIL4 …`), а не как авторская директива.

## Scope Estimate

Одна задача **size:M**. Три директивы — один связный слой диспетчера директив
RMS, один PR. #146 (`#include_drs`) — основной объём; #147/#148 — мелкие.

## Existing Architecture

- `internal/rms` — ячейка парсера RMS: `Parse(source, name) -> RmsFile`;
  `RmsFile` несёт AST и `Includes`/`XsIncludes`; таблицы `structuralWords` и
  `directiveWords`, диспетчер `directive()` (`parse.go:566`)
- `internal/include` — замыкание: `Resolver.Closure` читает
  `Includes`/`XsIncludes`, резолвит относительно директории owner-файла и
  `SetRoots`
- `internal/format` — форматтер RMS (тоже смотрит на `Includes`/`XsIncludes`)
- `internal/analysis` — семантические проверки и диагностики
- `internal/highlight` — грамматики подсветки (уже знают `#include_drs`)

## Notes

- Источник находки — свежая выгрузка официального корпуса (`.corpus-official`),
  лицензионно локальная (не коммитится; см. `scripts/corpus-official.sh`).
- Нарезка (подтверждено): 3 issue, одна задача/PR.
- Issue #98 остаётся открытой: она про hint, эта задача — про резолв.
