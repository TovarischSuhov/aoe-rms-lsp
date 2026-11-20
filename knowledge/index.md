---
okf_version: "0.2"
---

# База знаний aoe2-lsp

## Архитектура

* [Архитектура aoe2-lsp](/architecture/overview.md) — слои и порядок зависимостей ячеек, entrypoints в cmd/, путь данных от LSP-запроса до парсеров и базы знаний
* [Инварианты стабильности](/architecture/stability-invariants.md) — Правила, которые проект сознательно поддерживает и которые легко сломать незаметно — recovery-парсеры, асимметрия EOL между RMS и XS, сходимость форматтеров (RMS и XS), покрытие потреблённых токенов диагностиками, отказ форматтера на error-диагностиках, стабильные коды диагностик, stdout только для протокола, неизвестные флаги не роняют сервер, пересортировка merged-списков semantic tokens на месте склейки, мемоизация парсов editor-документов по canonical path с валидацией полным сравнением текста, паника в request-хендлере не рвёт сессию (defer recoverRequest), serve логирует conn.Err(), lifecycle: exit без shutdown — ошибка serve и код выхода 1, после shutdown все запросы кроме exit отвечают InvalidRequest (−32600, per-handler shutdownGuard), shutdown без exit — процесс жив, данные без сети, disk-кэш резолвера ограничен LRU (maxCacheEntries = 512, вытеснение по количеству).
* [Резолюция #include_drs в AoE2 DE](/architecture/include-drs-resolution.md) — движок резолвит по имени из resources/_common/drs/gamedata_x2/ и по относительным путям; id — легаси AoC/HD; опора include-замыкания и game-detect includeRoots

## Контракты

* [Ячеечные контракты CODEMANIFEST](/contracts/cell-contracts.md) — goga-устройство репозитория: CODEMANIFEST как публичная поверхность ячейки, .usages/, приоритет CLAUDE.md над conventions.md, интерфейсы объявляет консюмер

## Процедуры

* [Проверка изменений](/runbooks/verification.md) — предзадачная последовательность make check, тесты под memory cap, corpus gate, fuzz-цели, состав CI, рабочий процесс задача → ветка → PR
* [Пайплайн данных базы знаний](/runbooks/kb-data-pipeline.md) — регенерация embedded JSON из docs/ref/ через kbgen (гайд Zetnus — необязательный источник, не вендорится), обновление на патчах игры, ночной kb-monitor, лицензионные следствия GPL-3.0
