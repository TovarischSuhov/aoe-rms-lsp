# Brainstorm state — vscode-highlighting (COMPLETE)

Пайплайн goga-brainstorm завершён 2026-09-10: все 10 фаз пройдены,
финальный план верифицирован (19/19 проверок PASS) и подтверждён
пользователем. Этот файл — больше не точка восстановления пайплайна,
а запись о его итогах.

Задача: `task.md` (этот каталог), зеркало #51, эпик v1.0.0 (выполнять
до публикации расширения #47/#7).

## Итоги

- Авторитетный план: `arch.md` (этот каталог) — CODEMANIFEST +
  `.usages/grammar-pipeline.md` ячейки `internal/highlight`, diffs для
  `editors/vscode/package.json` (contributes.grammars) и cook
  `vscode-extension.md` (секция Static highlighting), порядок
  реализации (6 шагов), dependency map, verification checklist
- Контракт ячейки собран и записан: `internal/highlight/CODEMANIFEST`
  (`GenTmLanguage`), `internal/highlight/.usages/grammar-pipeline.md`;
  `goga lint` — 0 errors
- Решение propose остаётся в силе: kb-генерация keyword-частей
  RMS-грамматики обязательна

## Дальше (порядок возобновления)

1. `goga-apply` / реализация по arch.md: ветка
   `task/vscode-highlighting` от master
2. Шаги: ячейка internal/highlight (+ golden/json тесты) → cmd/tmgen →
   регенерация aoe2rms.tmLanguage.json (коммит) → aoe2xs.tmLanguage.json
   (hand-written) → contributes.grammars → cook update
3. PR в master, `Fixes #51`; ручная приёмка в VS Code (светлая/тёмная,
   с сервером/без) по checklist из arch.md

## Готчи для реализации

- бэктики в аннотациях — только imports/usages/entities; имена методов
  без бэктиков
- тесты — под memory cap (`timeout 300 systemd-run --user --scope
  -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`)
- `make check` + `goga contract internal/highlight` перед завершением
- ручные правки сгенерированного aoe2rms.tmLanguage.json запрещены —
  править скелет в internal/highlight и регенерировать
