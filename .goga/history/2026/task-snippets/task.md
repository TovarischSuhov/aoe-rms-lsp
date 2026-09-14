# snippets: VS Code-сниппеты RMS-бойлерплейта (#54)

Слот `snippets` пачки ux-and-data-quality (S), зеркало — issue #54.

## Current State

`editors/vscode/package.json` контрибьютит languages, grammars и
configuration — `contributes.snippets` нет, сниппетов в расширении ноль:
скелет новой карты (секции `<PLAYER_SETUP>`…`<MAP_SIZE>`) и частые блоки
(`create_object`, `start_random`, `base_terrain`) скриптер печатает
руками. Источники уже в репо: `docs/ref/zetnus-rms-guide.txt` (скелет
карты) и `docs/ref/map-scripting-practices.md` (частые блоки).
`.vscodeignore` — exclusion-стиль (`src/**`, `node_modules/**`, …):
новая папка `snippets/` попадает в .vsix автоматически. CI-проверка
contributes-путей ещё не существует — она в scope #69 (vsix-release-ci),
и её текущая формулировка перечисляет только `grammars[].path` и
`languages[].configuration`.

## Description

Декларативные VS Code-сниппеты для языка `aoe2rms`: скелет новой карты
(Zetnus skeleton) и частые блоки из map-scripting-practices. Без
клиентского TS-кода — VS Code резолвит префиксы из JSON. Валидность
скелета прижата Go-автотестом: разворот дефолтов плейсхолдеров →
`rms.Parse` → без ошибок. Границы утверждены 2026-09-14: RMS-only,
автотест (из {RMS-only + ручная приёмка, RMS-only + автотест, RMS + XS}).

## Scope

**In scope:**

- `editors/vscode/snippets/aoe2rms.json` — сниппеты:
  - скелет новой карты: обязательные секции в каноническом порядке,
    минимальный валидный набор;
  - частые блоки: `create_object` с полями, `start_random`/`end_random`
    (+`percent_chance`), `base_terrain`, отдельные секции
- `editors/vscode/package.json` — `contributes.snippets`:
  `{language: "aoe2rms", path: "./snippets/aoe2rms.json"}`
- Go-автотест `editors/vscode/snippets_test.go` (принцип — в образцах
  ниже): читает JSON, разворачивает дефолты плейсхолдеров скелета,
  прогоняет через `rms.Parse`, падает на ошибках
- секция Snippets в `.goga/usages/cooks/vscode-extension.md` (текст
  согласован на формулировке)
- однострочная правка `.goga/history/2026/task-vsix-release-ci/task.md`:
  в перечень contributes-путей CI-проверки добавить `snippets[].path`

**Out of scope:**

- XS-сниппеты (`aoe2xs.json`) — если понадобятся, отдельным слотом
- генерация сниппетов из kb (как tmgen) — сниппеты hand-written
- CI-проверка contributes-путей — принадлежит #69
- клиентский TS-код и правки `.vscodeignore` — не нужны

## Acceptance Criteria

- Автотест зелёный: скелет с подставленными дефолтами плейсхолдеров
  парсится `rms.Parse` без ошибок
- Сниппеты в .vsix: `snippets/aoe2rms.json` упакован (проверяет
  CI-джоба vscode)
- `make check` зелёный; CODEMANIFEST ячеек не менялись (Go-ячейки не
  затронуты)
- Ручная приёмка (за пользователем): префиксы срабатывают в VS Code,
  Tab ходит по плейсхолдерам
- В task.md #69 перечень contributes-путей включает `snippets[].path`

## Stack

- **Frameworks:** VS Code snippets contribution (декларативный JSON)
- **Libraries:** без новых; Go stdlib (`encoding/json`) для автотеста
- **Infrastructure:** без изменений (джоба vscode CI как есть)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| VS Code snippets contribution | `.goga/usages/cooks/vscode-extension.md` | updated (секция Snippets) |

## Risks and Constraints

- npm-реестр недоступен локально: `vsce package` не гоняем — упаковку
  проверяет CI
- скелет обязан оставаться минимально-валидным: любые правки сниппета
  прижаты автотестом
- использовать только конструкции, подтверждённые корпусом/справочником
  (Zetnus guide, map-scripting-practices), не выдумывать синтаксис

## Scope Estimate

Одна задача S, декомпозиция не нужна. Ветка `task/snippets` → PR
(`Fixes #54`).

## Existing Architecture

- `editors/vscode` — не ячейка (cook vscode-extension.md: no Go, no
  CODEMANIFEST), редакторная сторона
- автотест импортирует `internal/rms` (`Parse`) — разрешено внутри
  одного модуля; `Parse(source, name) -> (RmsFile, []Diagnostic)` —
  контракт internal/rms
- меняются: `package.json` (contributes), новый `snippets/`, cook-файл,
  task.md #69; контракты ячеек не затрагиваются

## Notes

Образцы, утверждённые на формулировке (вкладывать смысл, не копировать
дословно):

Тело сниппета (формат VS Code; `language` приходит из ассоциации в
package.json, `scope` не нужен):

```json
"create_object": {
  "prefix": "create_object",
  "body": [
    "create_object ${1:TOWN_CENTER} {",
    "  number_of_objects ${2:1}",
    "  $0",
    "}"
  ],
  "description": "Объект с полями (map-scripting-practices)"
}
```

Принцип автотеста:

1. Прочитать `snippets/aoe2rms.json` (`encoding/json`)
2. Взять `body` сниппета-скелета, подставить дефолты: `${1:default}` →
   `default`, `${1|a,b|}` → первый вариант, `$0` → пустая строка
3. `rms.Parse(развёрнутый текст, "skeleton.rms")`
4. Тест падает, если Parse вернул ошибки
