# Обновление базы знаний под патч игры (kb-refresh)

Процесс мейнтейнера на «день патча» AoE2 DE: обновить источники в
`docs/ref/`, получить агрегированный дифф «что нового», перегенерировать
`internal/kb/data/*.json` и оформить PR. Инструмент регенерации —
`cmd/kbgen` (детали пайплайна — `internal/kb/.usages/data-pipeline.md`).

## Когда запускать

- вышел патч AoE2 DE с XS/RMS-изменениями (пост в ageofempires.com/news);
- обновился один из источников (UGC Guide, Zetnus-гайд) — можно без патча.

## Источники

| Источник | Что даёт | Где живёт | Берём |
|---|---|---|---|
| Release notes AoE2 DE | новые/изменённые функции, константы, команды; версионность `since_update` | `docs/ref/aoe2de-xs-rms-changelog.md` | ageofempires.com/news (посты Update/Hotfix) |
| UGC Guide | `functions.json`, `constants.json` | `docs/ref/ugc-guide/xs/{functions,constants}/` | ugc.aoe2.rocks |
| Zetnus-гайд (Definitive Random Map Scripting Guide) | RMS-команды (Syntax Skeleton) | `docs/ref/zetnus-rms-guide.txt` | гугл-док Zetnus, текстовый экспорт |
| attribute-descs overlay | ручные desc для пробелов гайда (сейчас пуст) | `docs/ref/attribute-descs.json` | проектный файл, не зеркало |

Шаги независимы — процесс переживает задержки внешних мейнтейнеров:

1. **Release notes — первичны.** Дополнить changelog-док (регламент — в его
   шапке). Уже этого достаточно, чтобы дифф показал новые `since_update`.
2. **UGC Guide** — если обновился, переложить оба JSON как есть.
3. **Zetnus-гайд** — если обновился, переэкспортировать текст. Гайд может
   задержаться: функции/константы и версионность обновятся без него;
   новые RMS-команды до обновления гайда в базу не попадут (осознанное
   ограничение: команды извлекаются только из него).

## Процесс

1. Обновить источники по чеклисту выше.
2. Посмотреть дифф, не трогая данных:

   ```sh
   go run ./cmd/kbgen -diff
   ```

   Отчёт агрегирован по сущностям: `+ имя` — добавлено, `- имя` — удалено,
   `~ имя: поля` — изменено (пути полей вида `args[terrain].kind`,
   `params[0]`, `since_update`). Отчёт детерминирован — повторный прогон на
   тех же исходниках даёт байт-в-байт тот же текст; на неизменённых
   исходниках — `No changes.`
3. Перегенерировать данные: `go run ./cmd/kbgen`. Смысл `git diff` по JSON
   должен совпадать с отчётом из шага 2.
4. Проверки: `make check`; корпус-прогон
   (`go run ./cmd/corpus -bin <aoe2-lsp> -dir <корпус-карт>` или
   workflow_dispatch CI на ветке) — расхождения с master только по
   существу диффа (новые сущности), не по числу падений.
5. PR: ветка `task/kb-refresh-<update-id>`, в описание — markdown-отчёт из
   шага 2.

## Дифф-отчёт как артефакт

`kb.DiffKB(oldDir, newDir)` сравнивает два каталога данных; контракт — в
CODEMANIFEST ячейки `internal/kb`. Поля `params`/`args`/`attributes`
сравниваются позиционно; расхождение длин схлопывается в путь без деталей
(`args`). Golden-пример рендера — `internal/kb/testdata/diff_report.md`.
