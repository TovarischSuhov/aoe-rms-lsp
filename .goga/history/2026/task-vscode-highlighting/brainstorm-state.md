# Brainstorm state — vscode-highlighting (resume)

Сессия прервана 2026-09-10 на Фазе 7 (контракты). Этот файл — точка
восстановления: все утверждённые отчёты пайплайна + что осталось.

Задача: `task.md` (этот каталог), зеркало #51, эпик v1.0.0 (до #47).
Ключевое решение propose: kb-генерация обязательна.

## Position

Пайплайн goga-brainstorm:
- Phase 1 Intake ✓ [INTAKE_REPORT]
- Phase 2 Context ✓ [PROJECT_CONTEXT_REPORT]
- Phase 3 Primary Analysis ✓ утверждён пользователем
- Phase 4 Type Map ✓ утверждён
- Phase 5 Type Detail ✓ утверждён
- Phase 6 Cell Distribution ✓ утверждён
- Phase 7 Contracts — ячейка internal/highlight утверждена
  (usages + аннотации + usage-файл, см. ниже); **осталось собрать
  [CONTRACTS_REPORT]** (одна ячейка, агрегация тривиальна)
- Phase 8 Cell Assembly — не начата
- Phase 9 Plan Assembly (arch.md) — не начата
- Phase 10 Plan Verification — не начата

Нюанс Phase 9: `goga history path -f arch.md` сейчас резолвится в
`2026/master/` — указатель текущего топика стоит в master; арх нужно
класть в ЭТОТ топик (сверить `goga history --help`, возможно есть
команда установки текущего топика; иначе писать по прямому пути
`.goga/history/2026/task-vscode-highlighting/arch.md`).

## [INTAKE_REPORT] (конспективно)

Task file: статическая подсветка RMS/XS через TextMate-грамматики;
keyword-части RMS-грамматики генерируются из internal/kb (решение
зафиксировано); XS — hand-written. Single subsystem.

Acceptance: подсветка без сервера; слои не конфликтуют с semantic
tokens; валидность; генерация из kb + golden «команда из kb ⇒
подсвечивается»; .vsix с грамматиками. Out of scope: инъекционные
грамматики, темы, LSP-изменения.

## [PRIMARY_ANALYSIS_REPORT] (утверждён)

Artifact Resolution:
- генератор → create new cell `internal/highlight`
- источник keyword'ов → existing kb.Store (встроенные данные)
- CLI → new `cmd/tmgen` (flags only, паттерн cmd/kbgen)
- `editors/vscode/syntaxes/aoe2rms.tmLanguage.json` — new, генерируется,
  коммитится
- `editors/vscode/syntaxes/aoe2xs.tmLanguage.json` — new, hand-written
- `editors/vscode/package.json` contributes.grammars — modify
- легенда semantic tokens (internal/server) — reference only
- cook `.goga/usages/cooks/vscode-extension.md` — update (+ секция
  Static highlighting: структура tmLanguage, contributes.grammars,
  scope-нейминг)

Тёмные зоны, закрытые в Фазах 4–5: скелет — структурные типы в коде
ячейки (unexported); 3 keyword-класса с раздельными scope'ами;
case-insensitive matching; константы в RMS-грамматике — все
Constants(""). Остались на дизайн-реализацию: конкретные имена
TextMate-scope'ов, полнота hand-written паттернов (числа/проценты/
строки), необходимость make-target для регенерации (минимум —
документировать в usages).

## [TYPE_MAP_REPORT] + [TYPE_DETAIL_REPORT] (утверждены)

Контракт ячейки — ОДНА экспортированная рутина; модель грамматики
(скелет, scope-константы, keyword-наборы) — unexported, вне манифеста.
Тесты ячейки охраняют оба артефакта: golden «регенерация ≡
закоммиченный aoe2rms.tmLanguage.json» + JSON-валидность обоих файлов
(включая hand-written XS).

| Type | Character | Signature | Mutations |
|---|---|---|---|
| GenTmLanguage | Routine | (store: Store, outPath: string, log: Logger) -> err: error | Imports only |

Именование — по результату, в духе GenKB; Logger-нотация как в
GenKB/ExtractRmsCommands (в Go *slog.Logger, nil → slog.Default()).

## [CELL_DISTRIBUTION_REPORT] (утверждён)

- `internal/highlight` — NEW, листовая: GenTmLanguage
- `internal/kb` — не модифицируется, импортируется
  (Store, Command, Constant + Usages lookups, data-pipeline)
- Циклов нет: highlight → kb однонаправленная

## Контракт ячейки internal/highlight (утверждён, Фаза 7)

### Usages header

- Base: `conventions: .goga/usages/conventions.md`
- Imports из internal/kb: Types Store, Command, Constant;
  Usages `lookups`, `data-pipeline`
- External: `vscode-extension: .goga/usages/cooks/vscode-extension.md`
  (в задаче запланирован update — секция Static highlighting)
- Usages keys: `conventions`, `lookups`, `data-pipeline`,
  `vscode-extension`; External Usage Gaps: нет

### Annotations

Global:
```
Use `conventions` for code writing rules and testing.
Use `vscode-extension` for the artifact home and tmLanguage structure
(scope naming, contributes.grammars).
Use `data-pipeline` for the regeneration order: kb data (kbgen) first,
then grammar (tmgen) — artifact must never drift from committed kb data.

Ячейка — оффлайн-генерация статической подсветки: читает загруженную
базу знаний kb и эмитет RMS-грамматику; утилита сборки, в рантайме
сервера не участвует. Модель грамматики (скелет, scope-константы,
keyword-наборы) — внутренняя реализация, вне публичной поверхности.
```

Type-level `GenTmLanguage`:
```
Генерация TextMate-грамматики aoe2rms.tmLanguage.json: hand-written
скелет + сгенерированные keyword-альтернации из базы знаний.

`store`: загруженная база знаний (kb.NewStore); read-only
`outPath`: путь к целевому файлу
(editors/vscode/syntaxes/aoe2rms.tmLanguage.json)
`log`: инжектированный логгер сборки (в Go — *slog.Logger;
nil → slog.Default())
`err`: ошибки записи; обёрнуты с путём файла

Use `lookups` from Imports for keyword collection via Store.

Algorithm:
1. Собрать keyword-наборы: имена команд Commands(""), имена атрибутов
   из Attributes каждой команды, имена констант Constants("")
2. Построить модель грамматики: скелет (секции, #-директивы,
   блок-комментарии, числа/проценты, строки) + keyword-альтернации
   трёх классов (команды/атрибуты/константы); имена экранированы,
   порядок стабилен
3. Сериализовать в JSON детерминированно (стабильный порядок ключей)
4. Записать файл в outPath

Requirements:
- повторный запуск на тех же данных → побайтово идентичный файл
- RMS-имена матчатся case-insensitively; границы слов учитывают
  подчёркивания и цифры в именах
- три keyword-класса с раздельными scope'ами; нейминг согласован
  с легендой semantic tokens сервера (по `vscode-extension`)
- пустой keyword-набор → соответствующий паттерн опускается,
  не ошибка

Constraints:
- утилита сборки, не вызывается в рантайме сервера
- Store не модифицируется
- скелет грамматики определён в коде ячейки, не читается из
  внешних файлов
```

### Usage-файл

`internal/highlight/.usages/grammar-pipeline.md` — полный текст
написан и утверждён (регенерация через cmd/tmgen, порядок
kbgen → tmgen, no-drift golden, компаньоны: aoe2xs.tmLanguage.json
hand-written + contributes.grammars). Текст см. в сессии или
пересобрать по аннотациям; ключевые пункты:
- entry point highlight.GenTmLanguage(store, outPath, slog.Default())
- cmd/tmgen: flags only, default out =
  editors/vscode/syntaxes/aoe2rms.tmLanguage.json, флаг -out
- порядок: kbgen (данные) → tmgen (грамматика); артефакт коммитится
- golden: регенерация в temp dir ≡ закоммиченный файл; ручные правки
  артефакта запрещены (править скелет в internal/highlight)
- aoe2xs.tmLanguage.json — hand-written; константы в XS красит
  semantic tokens, не грамматика

## Дальше (порядок возобновления)

1. Собрать [CONTRACTS_REPORT] из утверждённого контракта выше (одна
   ячейка; Base Compliance таблица + External Usage Gaps: нет)
2. Phase 8: goga-brainstorm-cell-assembly → CODEMANIFEST (сборка из
   контракта) + .usages; WAIT-гейт на ячейку и финал
3. Phase 9: goga-brainstorm-plan-assembly → arch.md (в этот топик,
   см. нюанс выше); WAIT-гейт
4. Phase 10: goga-brainstorm-plan-verification → VERIFICATION_REPORT
5. После верификации: apply → реализация (ветка task/vscode-highlighting,
   PR, Fixes #51), cook vscode-extension.md update, task.md External
   Dependencies — исполнить

Готчи (из прошлого опыта): бэктики в аннотациях — только
imports/usages/entities; имена методов без бэктиков; пустые результаты
— пустые срезы; `make check` перед завершением; тесты — под memory cap.
