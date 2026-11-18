# xs-unused-decls: unused-диагностика + entry-point model (#115)

Слот пачки ux-wave-2 (#79, разбивка), пара к смерженному
xs-duplicate-decls (#114, PR #191). Спека: unused locals/functions;
rules/events — engine-called entry points, не помечаются; **entry-point
model — дизайн-решение слота**; corpus acceptance.

## Current State

Анализ одноклеточный (`AnalyzeXs(file, externals)`): данных о том, кто
Ссылается на файл, нет (обратный индекс включений — отдельный слот
#121 rename-file). Наивный reachability от rules/events дал бы ложные
срабатывания на библиотеках.

**Корпусная проверка эвристики** (.corpus): `013__vectorStringDict.xs`
— 0 rules, 18 функций (чистая библиотека, вызывается из других .xs);
`014__codex_teleport_barracks.xs` — 1 rule + 1 функция (скрипт).
Форма файла (есть ли engine-entry points) предсказывает «экспортируемость».

## Description

Код `unused-declaration` (**hint** — совет, не ошибка; severityOverrides
подавляют):

1. **Entry-point model**: файл с ≥1 rule/event — самостоятельный
   скрипт: roots = rules/events, топ-левел функции, недостижимые из
   roots по графу вызовов файла (транзитивно) → hint на `Decl.Range`.
   Файл без rules/events — библиотека (функции экспортируются через
   include-замыкание) — топ-левел не помечается. Cross-kind ссылки
   (ident на имя) считаются использованием — консервативно
2. **Локалы**: декларатор без единой ссылки по всему телу объемлющей
   декларации → hint на `Expr.Range`. Инициализатор с вызовом — скип
   (side effect удаления непредсказуем); top-level переменные — скип
   (side-effect инициализация + static-семантика + cross-file)
3. Параметры не помечаются (вне спеки; rule-хендлеры с пустыми
   параметрами — норма)
4. Дока кода — `checks.md`; корпусная приёмка — гейт
   `TestCorpus_NoDuplicateDecls` расширяется до analysis-набора

## Acceptance

- make check; corpus: duplicate — ноль (как было), unused — прогон с
  числами в PR (истинные находки на «диком» коде допустимы, ложные —
  нет: библиотечные функции корпуса должны остаться тихими)
- тесты: script-shaped reachability (транзитивные вызовы, рекурсия,
  мёртвая пара), library-skip, locals с/без side-effect инициализатора,
  затенение, rules/events не помечаются

## Result

- Код `unused-declaration` (**hint**) в internal/analysis:
  - **Entry-point model**: rules/events — engine-roots; в
    script-shaped файлах (≥1 rule/event) топ-левел функции,
    недостижимые из roots транзитивно по графу вызовов (рекурсия без
    входа — тоже мертва), получают hint на `Decl.Range`; файл без
    rules/events — библиотека (корпус: `013__vectorStringDict.xs`, 18
    экспортируемых функций) — топ-левел не трогается; top-level
    переменные не помечаются вовсе (side-effect инициализация,
    static-семантика, prelude-глобалы `local int cAge* = …`)
  - **Локали** в телах function/rule/event: имя без единого упоминания
    в теле → hint на `Expr.Range`; декларатор сам себе использованием
    не считается; инициализатор с вызовом молчит (side effect);
    top-level `local`-глобалы прелюдии защищены ограничением проверки
    исполняемыми телами
- Приёмка: corpus — duplicate 0/100 (как было), unused **0 находок**
  (гейт `TestCorpus_NoDuplicateDecls` расширен: функция-level hint в
  библиотечном файле валит прогон, остальное — счётчики в лог)
- Тесты: `TestAnalyzeXs_UnusedDecls` — 9 кейсов (unused локаль,
  call-инициализатор, use во вложенном скоупе, script-shape
  reachability, транзитивность, мёртвая рекурсия, library-skip,
  rules/events, top-level переменные); фикстуры существующих тестов
  дополнены использованиями (честные true-positive срабатывания)
- Проверки: 15/15 пакетов ok, lint 0, fmt чисто, corpus harness
  100/100 ok panic 0
- Дока кода — `checks.md`; severityOverrides — общий механизм
