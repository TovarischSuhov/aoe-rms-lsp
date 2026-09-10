# vscode-highlighting: TextMate-грамматики для aoe2rms/aoe2xs

## Current State

Расширение `editors/vscode` регистрирует языки только с
language-configuration (комментарии, скобки, индентация — поведение
редактора, не цвета). TextMate-грамматики в `contributes.grammars` нет:
без запущенного сервера файл выглядит простым текстом, а при работающем
сервере semantic tokens красят только идентификаторы
(`known/unknown/deprecated/section/kind`) — комментарии, числа, строки,
`#`-директивы остаются без цветов. Пользовательский репорт: «подсветки
синтаксиса при запуске VS Code не увидел». В планах (эпик v1.0.0, 1.x)
задача отсутствовала — Marketplace-расширение без статической подсветки
даст странное первое впечатление.

## Description

Статическая подсветка обоих языков через TextMate-грамматики
(`syntaxes/aoe2rms.tmLanguage.json`, `syntaxes/aoe2xs.tmLanguage.json`) +
`contributes.grammars` в package.json. Подсветка работает без сервера и
сочетается с semantic tokens (те красят семантику идентификаторов,
грамматика — синтаксис).

## Scope

**In scope:**
- RMS-грамматика: секции `<…>`/`</…>`, `#`-директивы (`#include`,
  `#includeXS`, `#const`, `#define`), блок-комментарии `/* */`,
  команды и атрибуты (ключевые слова), константы (CONIST-style
  UPPER_CASE из kb), числа/проценты, строки в аргументах
- XS-грамматика: C-like — ключевые слова языка, комментарии
  (`//`, `/* */`), строки, числа, типы, вызовы функций
- источник ключевых слов RMS: **генерация из kb** — решение зафиксировано
  (propose-сессия 2026-09-10), не дизайн-выбор: генератор по образцу
  kbgen (расширить `kbgen` или отдельная команда) читает списки
  команд/атрибутов/констант из `internal/kb` и эмитет keyword-части
  RMS-грамматики; tmLanguage-файл коммитится артефактом, регенерация —
  тем же процессом, что и данные kb (сейчас ручной `go run ./cmd/kbgen`
  при обновлении docs/ref, CI-шага регенерации нет). Структура
  грамматики, разбивка generated/hand-written частей и место генератора —
  за design-этапом
- scope-нейминг согласован с semantic tokens легендой (минимизация
  конфликтов раскраски: TextMate — базовый слой, semantic — поверх)
- тест: JSON-валидность грамматик + golden-проверка ключевых слов по kb
  (команда в kb ⇒ подсвечивается); ручная приёмка в VS Code (светлая и
  тёмная темы)

**Out of scope:**
- инъекционные грамматики (inline-XS внутри .rms — опционально позже)
- настройка цветовых тем (пользовательские темы VS Code поверх scope'ов)
- LSP-изменения (semantic tokens не трогаются)

## Acceptance Criteria

- .rms и .xs файлы подсвечиваются в VS Code без запущенного сервера
  (секции, директивы, комментарии, числа, команды)
- с запущенным сервером слои не конфликтуют: semantic tokens уточняют
  идентификаторы, синтаксис от грамматики
- грамматики валидны (`code --status` / загрузка без ошибок в
  developer tools)
- ключевые слова RMS генерируются из kb: артефакт коммитится,
  регенерация — шагом генератора (процесс kbgen); golden-проверка
  «команда из kb ⇒ подсвечивается» зелёная
- `npm run check` зелёный; `.vsix` собирается с грамматиками

## Stack

- **Frameworks:** TextMate Grammar (plist/JSON tmLanguage), VS Code
  contributes.grammars
- **Libraries:** без новых npm-зависимостей; для генерации — Go
  (`internal/kb` данные + генератор по образцу kbgen)
- **Infrastructure:** существующая сборка .vsix в CI

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| TextMate grammars | `.goga/usages/cooks/vscode-extension.md` | done — секция Static highlighting (структура tmLanguage, contributes.grammars, scope-нейминг) |

## Risks and Constraints

- Большой список команд RMS (~100+) в keyword-матчинге — производительность
  TextMate: проверить на реальных картах корпуса; при необходимости
  подсвечивать только структурные элементы, а имена оставить semantic
  tokens
- Генерация из kb добавляет шаг сборки: рассинхрон данных и артефакта
  исключён регенерацией одним процессом, но остаётся риск дрейфа
  генератора при изменении схемы kb — покрыть golden-тестом по данным
- Цвета зависят от темы пользователя — грамматика задаёт только scope'ы

## Scope Estimate

Одна задача, малый-средний объём (2 tmLanguage + package.json +
генерация/процесс + cook). Ветка `task/vscode-highlighting` → PR.
Логично **до** подзадачи #7 эпика (публикация расширения).

## Existing Architecture

- `editors/vscode/` — package.json (contributes), language-configs уже
  есть; сборка esbuild/vsce не меняется
- `internal/kb` — источник списков команд/атрибутов/констант;
  `cmd/kbgen` — образец генератора из данных
- semantic tokens (server) — слой поверх, не затрагивается

## Notes

- Решение сессии 2026-09-10: статическая подсветка признана пробелом
  (репорт пользователя «не увидел подсветки при запуске VS Code»);
  тем же днём включена в эпик v1.0.0 как подзадача #10 (выполнять до
  публикации #7)
- Направление генерации из kb изначально было рекомендацией;
  propose-сессией 2026-09-10 зафиксировано как обязательное решение
  (self-contained данные, уже закоммиченный пайплайн kbgen)
- Целевой артефакт (якорь для design-этапа, не полная грамматика) —
  структура RMS-грамматики; hand-written части — структура и директивы,
  generated — keyword-альтернации из kb:

  ```json
  {
    "scopeName": "source.aoe2rms",
    "patterns": [
      { "include": "#comments" },
      { "include": "#sections" },
      { "include": "#directives" },
      { "include": "#kb-keywords" }
    ],
    "repository": {
      "sections": {
        "name": "entity.name.section.aoe2rms",
        "match": "</?[A-Za-z_]+>"
      },
      "directives": {
        "name": "keyword.control.directive.aoe2rms",
        "match": "#(includeXS?|const|define)\\b"
      },
      "kb-keywords": {
        "name": "keyword.other.command.aoe2rms",
        "match": "(?i)\\b(land_generation|players|create_object|terrain_mask)\\b"
      }
    }
  }
  ```

  Альтернация в `kb-keywords.match` — весь список команд/атрибутов/
  констант, эмитет генератор из `internal/kb` (в примере — 4 имени для
  наглядности). Scope-нейминг `entity.name.section.*` согласован с
  легендой semantic tokens (token type `section`). XS-грамматика
  (`source.aoe2xs`) — целиком hand-written: ключевые слова языка
  фиксированы, kb-генерация не нужна
