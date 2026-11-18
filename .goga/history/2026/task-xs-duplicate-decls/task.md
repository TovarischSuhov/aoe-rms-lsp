# xs-duplicate-decls: диагностика повторных деклараций в XS (#114)

Слот пачки ux-wave-2 (#79, разбит 2026-09-22). Warning на повторные
декларации в одном скоупе; unused + entry-point модель — отдельный
слот #115.

## Current State

`AnalyzeXs` (internal/analysis) проверяет имена и типы, но не
повторные декларации:

- top-level: `int x;` и `void x()` во втором экземпляре — молча
- локальные: `int x = 1; int x = 2;` в одном блоке — молча (undefined-
  symbol не срабатывает — имя «объявлено» плоским pre-pass
  `collectLocals` в map[string]bool без скоупов и ranges)

AST готов: `Decl.Range`, `Stmt.Range`, `Expr.Range` есть; `Param`
ranges не имеет (параметры — вне спеги слота, не трогаем). Скоуп-семантика
парсера: `if` — Body[0]=then, Body[1]=else (каждый свой скоуп);
`for` — Exprs=init/cond/step в скоупе цикла, Body — вложенный;
`switch` — Body держит StmtCase; rule-формы — StmtCondition/StmtAction.

## Description

Проверка `duplicate-declaration` (warning):

1. **Top-level**: повтор имени среди именованных деклараций файла
   (function/rule/event/variable; `include` — путь, не символ, скип;
   безымянные rules не коллидят). Range — `Decl.Range`
2. **Локальные**: повтор декларатора в одном блоке скоупа. Вложенные
   блоки/ветки/циклы — легальное затенение, молчание (консервативно
   против ложных срабатываний); `for`-init декларирует в скоупе цикла
   (сиблинг-циклы с `int i` — норма). Range — `Expr.Range` декларатора
3. severityOverrides — общий механизм, новый код работает из коробки;
   документация кода — `internal/analysis/.usages/checks.md`

## Acceptance

- make check зелёный; corpus gate — ноль срабатываний duplicate-
  declaration на 100 реальных картах (приёмка analysis-слотов)
- тесты table-driven: top-level dup, local same-block dup, sibling
  for-циклы, вложенное затенение, if/else ветки, rule-повторы,
  include/безымянные скипаются

## Result

- Код `duplicate-declaration` (warning) в internal/analysis:
  `checkDuplicateDecls` — top-level (именованные декларации; include и
  безымянные rules скипаются) + скоуп-ход локалей с семантикой веток
  парсера (if Body[0]/[1] независимы, for-init в скоупе цикла,
  switch-кейсы независимы, condition/action-блоки независимы);
  вложенные скоупы затеняют легально
- Range: top-level — Decl.Range, локалы — Expr.Range декларатора
- Приёмка: новый `TestCorpus_NoDuplicateDecls` — корпусный гейт
  analysis-слотов (skip без .corpus, пригоден следующим слотам):
  100/100 карт, ноль срабатываний
- Тесты: TestAnalyzeXs_DuplicateDecls, 12 кейсов (dup top-level,
  cross-kind, rules, same-block, bare declarator, затенение, if/else,
  sibling for в обеих формах, switch, безымянные rules, include-skip)
- Проверки: go test 15/15 пакетов ok, lint 0, fmt чисто
- Дока кода — internal/analysis/.usages/checks.md; severityOverrides
  работают через общий механизм без правок
