# kb-refresh-zetnus: переэкспорт гайда Zetnus, завершение #132

Финальный шаг чеклиста **#132** (Update 185872): после перекладки
апстрима UGC Guide (PR #189, смержен) остался только Zetnus-гайд.
Гайд качается свежим экспортом из гугл-дока (README, ссылка с
атрибуцией); файл gitignored — в репозиторий не вендорится, коммитится
только производный `rms-commands.json`.

## Current State

- Локальная копия `docs/ref/zetnus-rms-guide.txt` на машине разработки
  отсутствовала (gitignored); свежий экспорт 2026-11-18 — 6358 строк
  (старый: 5909), скачан curl'ом напрямую из
  `docs.google.com/document/d/1jnhZXoeL9mkRUJxcGlKnO98fIwFKStP_OBozpr0CHXo/export?format=txt`
  (блокировка docs.google.com из старого task-дока на эту машину не
  переносится)
- `kbgen -diff`: RMS commands — 0 added, 0 removed, 3 changed (desc:
  `ai_info_map_type`, `max_number_of_cliffs`, `min_number_of_cliffs` —
  правка опечатки и уточнение «If min=max, that number of cliffs will
  be placed»). Новых RMS-команд патчи 185872/188329 не добавляли —
  сходится с changelog; RMS-изменения 185872 — константы
  `random_map.def` (файл игры, kb его не вендорит)

## Description

1. Перегенерировать данные (`go run ./cmd/kbgen`) — 3 desc в
   `rms-commands.json`
2. Проверки: тесты kb-семейства, corpus gate, lint
3. PR с отчётом диффа, `Fixes #132` — чеклист обновления апстрима
   закрыт полностью (UGC — #189, Zetnus — здесь)

## Result

(заполняется по завершении)
