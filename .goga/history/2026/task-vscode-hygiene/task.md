# vscode hygiene: includeRoots-дока под реальность DE + allowScripts

## Current State

- Настройка `aoe2lsp.includeRoots` советует несуществующий путь: 
  `markdownDescription` в `editors/vscode/package.json` (строка ~87) —
  «Point it at the game's `ai-rms` folder to resolve stock includes».
  В актуальной Steam-установке AoE2 DE каталога `ai-rms` нет
  (диагностика #68 §5). Реальные полезные корни:
  `resources_common/random-map-scripts` (стоковые карты россыпью;
  основные карты упакованы в DRS) и `resources_common/xs`
  (`Constants.xs`, `Effects.xs`, `x256tech.xs`, `ailib/Geometry.xs` —
  для `#includeXS`).
- Тот же совет с `ai-rms` продублирован в `README.md` дважды:
  строка ~162 (пример настроек VS Code) и ~207 (пример nvim).
- `allowScripts` в коммит-версии `editors/vscode/package.json` пуст/
  отсутствует; одобренные записи (`esbuild@0.24.2`,
  `@vscode/vsce-sign@2.1.0`, `keytar@7.9.0`) существуют только локально
  после `npm approve-scripts` (#68 §6). На свежем окружении npm 11 с
  политикой скриптов молча пропускает их postinstall.
- Прерванный по таймауту `npm install` оставляет битое `node_modules`
  (симптом: `Cannot find module 'es-errors/type'` при запуске vsce);
  повторная установка не лечит — только полный снос каталога. Нигде
  не задокументировано.

## Description

Две мелкие правки гигиены `editors/vscode`, найденные диагностикой #68:

1. **Честная дока `includeRoots`**: переписать `markdownDescription`
   настройки — вместо `ai-rms` указать реальные корни DE
   (`<game>/resources_common/random-map-scripts` для стоковых карт,
   `<game>/resources_common/xs` для `#includeXS`). Те же правки в двух
   местах `README.md`.
2. **Воспроизводимая сборка**: закоммитить заполненный `allowScripts`
   в `editors/vscode/package.json`; дополнить cook
   `.goga/usages/cooks/vscode-extension.md` рецептом восстановления
   битого `node_modules` (только полный снос + чистая установка).

## Scope

**In scope:**
- `editors/vscode/package.json` — `markdownDescription` у
  `aoe2lsp.includeRoots`; `allowScripts`
- `README.md` — два примера с `ai-rms`
- `.goga/usages/cooks/vscode-extension.md` — рецепт лечения
  `node_modules`

**Out of scope:**
- автодетект игровой папки (1.x candidate, #60)
- логика резолва include на сервере (`internal/include`)
- icon/keywords/bugs/homepage (#47, листинг Marketplace)
- что-либо в Go-ячейках

## Acceptance Criteria

- В `package.json` и `README.md` не осталось упоминаний `ai-rms`;
  примеры указывают на `resources_common/random-map-scripts` и
  `resources_common/xs`
- `allowScripts` закоммичен заполненным; CI-джоба `vscode` зелёная
- cook содержит рецепт восстановления `node_modules`
- `make check` зелёный (Go не затронут — контрольный прогон)

## Stack

- **Frameworks:** нет (JSON + Markdown)
- **Libraries:** нет
- **Infrastructure:** npm 11 (`allowScripts`-политика скриптов)

## External Dependencies

| Component    | Usage file                               | Status  |
|--------------|------------------------------------------|---------|
| vsce / npm   | `.goga/usages/cooks/vscode-extension.md` | existing (задача дополнит) |

## Risks and Constraints

- npm-реестр недоступен локально — `allowScripts`-правку нельзя
  прогнать через `npm install` на машине автора; верификация —
  CI-джоба `vscode`
- Формат `allowScripts` взять из локального состояния автора
  (после `npm approve-scripts`) или собрать вручную по списку
  postinstall-пакетов; при реализации сверить с npm 11 документацией
- Пути DE верифицированы на одной машине (#68 §7) — формулировки в
  доке писать без жёсткой привязки к букве диска/Steam-каталогу

## Scope Estimate

Одна крошечная задача: ветка `task/vscode-hygiene` → один PR (Fixes
#70, зеркало задачи). Обе правки из одной диагностики (#68), одна
область, дробить нет смысла.

## Existing Architecture

Ячеек не касается. Слои: `editors/vscode/package.json`, `README.md`,
`.goga/usages/cooks/vscode-extension.md`. Мотивировка и факты —
issue #68 (§5, §6, §7).

## Notes

- Решение (2026-09-10): includeRoots-дока и allowScripts одним PR,
  несмотря на разные аудитории (юзеры/разработчики) — обе правки
  крошечные и из одного источника.
- Автодетект игры — отдельно в 1.x (#60); при его формулировке
  заменить `ai-rms` в тексте на реальные корни (см. комментарий
  в #60).
