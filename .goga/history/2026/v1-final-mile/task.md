# v1-final-mile: хвост эпика v1.0.0 — форматирование XS, публикация, пакетные менеджеры

## Current State

- Эпик v1.0.0 (#50, `2026/v1-lsp-completeness/task.md`): слоты document-link
  (PR #64), selection-range (PR #65), rename-парсеры (PR #67), rename-сервер
  (PR #71), highlighting (PR #66) смержены; format RMS (#45) — в ветке
  `task/format-rms`, близок к завершению
- `internal/format` (по arch-плану #45): `Options`, `RMS(source, opts)` —
  AST-принтер с якорением комментариев; inline-XS (`XsBlock.Code`) вербатим
- `internal/xs`: полный AST (`XsFile.Decls`: rules, функции, директивы,
  statements/exprs с Range); комментарии и строковые литералы — вперемешку в
  неэкспортируемом `noncode []common.Range` (гасят `SymbolAt`/`CallAt`);
  отдельных диапазонов комментариев ячейка не отдаёт
- `internal/server`: все LSP-хендлеры, кроме `textDocument/formatting`;
  capability Formatting не объявлена
- Дистрибуция: `.vsix` собирается и кладётся в GitHub Release (vsix-release-ci,
  PR #69; синк версий extension↔server есть); публикация в Marketplace и Open
  VSX отсутствует; winget и Homebrew tap отсутствуют
- Инварианты #45 на корпусе вскрыли баги rms-парсера (#93–#96) — чинятся в
  своих issue, в этот пакет не входят

## Description

Пакет из пяти слотов, закрывающий #46 (format XS + formatting-хендлер, разрез
на три слота), #47 (публикация расширения) и #49 (пакетные менеджеры). Каждый
слот — своя ветка `task/<name>` → PR → свой issue; независимая ценность и
зелёные проверки на каждом.

- **Слот 1 — server-хендлер Formatting (RMS).** Хендлер
  `textDocument/formatting` над готовым `format.RMS`: capability в
  `initialize`, маппинг LSP FormattingOptions → `format.Options`
  (TabSize/InsertSpaces), отказ при error-диагностиках → ноль edits, успех →
  одно full-document TextEdit
- **Слот 2 — xs: экспорт комментариев + аудит полноты AST.** Разделить
  `noncode` (комментарии ≠ строковые литералы), экспортировать
  `XsFile.Comments -> []Range` (аналог правки rms в #45); аудит: что AST
  теряет из литеральных деталей (числа, строки, вербатимность) — входные
  данные для дизайна слота 3
- **Слот 3 — format.XS: принтер + .xs в хендлере.** Печать XS AST в
  каноническом стиле (отступы, раскладка rules/функций/statements, якорение
  комментариев), golden + инварианты (`parse(format(x)) ≡ parse(x)`,
  идемпотентность, полнота комментариев, EOL); расширение хендлера слота 1 на
  .xs-документы
- **Слот 4 (#47) — публикация расширения.** `vsce publish` (Marketplace) +
  `ovsx` (Open VSX) из release-workflow по тегу `v*`; гейт на секреты
  `VSCE_PAT`/`OVSX_PAT` (без них — сборка без публикации, не красный CI)
- **Слот 5 (#49) — пакетные менеджеры.** Релизные артефакты для установки
  (архив + SHA256, если недостаточно существующих), winget-манифест (PR в
  microsoft/winget-pkgs), Homebrew tap (отдельный репозиторий), cook
  `package-managers`

## Scope

**In scope:**
- слот 1: `internal/server` (хендлер + capability), cook `lsp-protocol`
- слот 2: `internal/xs` (modify: `Comments`), `.usages` xs, аудит-отчёт
- слот 3: `internal/format` (XS-точка входа), `internal/server` (.xs-ветка
  хендлера), golden/инвариант-тесты
- слот 4: `.github/workflows/release.yml`, секреты, cooks `vscode-extension`,
  `github-actions`
- слот 5: `scripts/`/CI-артефакты, winget-манифест, tap-репозиторий, cook
  `package-managers` (create)

**Out of scope:**
- #48 release 1.0.0 (терминальный слот эпика: changelog, тег, README по
  платформам) — формулируется отдельно после этого пакета
- баги rms-парсера #93–#96
- inline-XS внутри RMS: `XsBlock.Code` остаётся вербатим (решение сессии #45
  сохраняется — слот 3 не переформатирует встроенный XS)
- клиенты кроме VS Code; scoop/AUR (кандидаты 1.x, #60)

## Acceptance Criteria

- Слот 1: `textDocument/formatting` отвечает по спецификации для .rms;
  capability объявлена; error-диагностики → пустой список edits (пустой срез,
  не nil); один full-document TextEdit; `make check` зелёный
- Слот 2: `XsFile.Comments` — диапазоны только комментариев (строковые
  литералы исключены), отсортированы, не пересекаются; `SymbolAt`/`CallAt`
  поведение не изменилось; `goga contract xs` зелёный
- Слот 3: `parse(format(x)) ≡ parse(x)` структурно и полнота комментариев на
  golden-фикстурах и .xs из корпуса (пропуск, если `.corpus` не скачан);
  `format(format(x))` байт-идентичен; хендлер форматирует .xs-документы;
  `goga contract format|server` зелёные
- Слот 4: тег `v*` публикует расширение в Marketplace и Open VSX автоматически;
  отсутствие секретов не роняет релизную сборку
- Слот 5: `winget install` и `brew install` ставят бинарник на чистой машине;
  winget-PR отправлен (мерж не блокирует слот)
- Все слоты: `make check` зелёный, CI ветки зелёный

## Stack

- **Frameworks:** Go 1.26+ (stdlib first), go.lsp.dev/protocol (существующая)
- **Libraries:** без новых Go-зависимостей; ячейки `xs` (modify), `format`
  (modify), `server` (modify)
- **Infrastructure:** GitHub Actions (release-workflow), секреты
  `VSCE_PAT`/`OVSX_PAT`, microsoft/winget-pkgs (внешний PR), Homebrew tap
  (отдельный репозиторий)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| go.lsp.dev/protocol — formatting-хендлер | `.goga/usages/cooks/lsp-protocol.md` | update (слоты 1, 3) |
| vsce publish / ovsx | `.goga/usages/cooks/vscode-extension.md` | update (слот 4) |
| release-workflow публикация | `.goga/usages/cooks/github-actions.md` | update (слоты 4, 5) |
| winget + Homebrew tap | `.goga/usages/cooks/package-managers.md` | create (слот 5) |

Секреты `VSCE_PAT`/`OVSX_PAT` — настроить до слота 4 (условие, не cook).

## Risks and Constraints

- Полнота XS AST: литеральные детали чисел/строк — аудит слота 2 до дизайна
  слота 3; инварианты на корпусе — страховка (прецедент: риск №1 #45)
- Разделение `noncode`: новое поле не должно менять существующие поверхности
  xs (`SymbolAt`/`CallAt`/`Definition`) — правка контракта минимальная
- Корпус может не содержать standalone .xs-файлов — тогда инварианты XS на
  golden + fuzz-засев (прецедент `seedFuzzFixtures`)
- Стиль не меняет семантику: порядок decls/statements/exprs неизменен;
  inline-XS вербатим
- winget-pkgs: ревью с задержкой, не блокирует тег релиза; tap — репозиторий
  вне этого repo
- Marketplace/Open VSX индексация с задержкой — проверка публикации ручная,
  в CI только успех шага publish

## Scope Estimate

Мультизадача: 5 слотов, каждый — отдельная ветка `task/<name>` → PR.

| # | Слот | Issue | Ячейки | Зависимости | Объём |
|---|------|-------|--------|-------------|-------|
| 1 | format-handler: textDocument/formatting (RMS) | new | server | мерж #45 | малый |
| 2 | xs-comments: экспорт комментариев + аудит AST | new | xs | — (лист) | малый-средний |
| 3 | format-xs: XS-принтер + .xs в хендлере | new (закрывает #46) | format, server | слоты 1 + 2 | средний |
| 4 | publish-extension: Marketplace + Open VSX | #47 | editors, CI | секреты | малый |
| 5 | package-managers: winget + Homebrew tap | #49 | scripts, CI, внешние | релизные артефакты | малый-средний |

Порядок: слот 2 — параллельно с чем угодно; слот 1 — после мержа #45; слот 3 —
после 1+2; слоты 4–5 независимы. Терминальный слот эпика #48 — вне пакета.

## Existing Architecture

- `internal/server` — новый хендлер + capability; DI по паттерну ячейки
- `internal/xs` — modify: `Comments -> []Range` у `XsFile` (данные уже
  собираются в `noncode`, нужна дифференциация строк/комментариев)
- `internal/format` — modify: XS-точка входа рядом с `RMS`, общие `Options`
- `editors/vscode`, `scripts/release.sh`, `.github/workflows/release.yml` —
  публикация и версионирование
- прецеденты: экспорт комментариев rms (#45), корпус-засев fuzz-тестов,
  протоколо-независимость format

## Notes

Решения сессии (2026-09-22, пользователь):

- Пакет = #46 + #47 + #49; #46 разрезан на три слота (хендлер RMS первым
  после мержа #45 — пользовательская ценность до готовности XS-принтера)
- #45 «почти сделана» — декомпозиции не подлежит; #48 и баги #93–#96 вне
  пакета
- inline-XS внутри RMS вербатим — сохранено решение сессии #45
- Cook-обновления разложены по слотам (прецедент эпика): `lsp-protocol` (1,
  3), `vscode-extension` (4), `github-actions` (4, 5), `package-managers`
  создаётся в слоте 5
- Слоты 1–3 зеркалятся новыми issue со ссылкой на этот task.md; #46
  закрывается слотом 3, #47 и #49 — своими слотами
