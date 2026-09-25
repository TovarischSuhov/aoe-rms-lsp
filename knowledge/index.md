---
okf_version: "0.2"
---

# База знаний aoe2-lsp

## Архитектура

* [Архитектура aoe2-lsp](/architecture/overview.md) — слои и порядок зависимостей ячеек, entrypoints в cmd/, путь данных от LSP-запроса до парсеров и базы знаний
* [Инварианты стабильности](/architecture/stability-invariants.md) — правила, которые проект сознательно поддерживает: recovery-парсеры, асимметрия EOL между RMS и XS, стабильные коды диагностик, stdout только для протокола, данные без сети

## Контракты

* [Ячеечные контракты CODEMANIFEST](/contracts/cell-contracts.md) — goga-устройство репозитория: CODEMANIFEST как публичная поверхность ячейки, .usages/, приоритет CLAUDE.md над conventions.md, интерфейсы объявляет консюмер

## Процедуры

* [Проверка изменений](/runbooks/verification.md) — предзадачная последовательность make check, тесты под memory cap, corpus gate, fuzz-цели, состав CI, рабочий процесс задача → ветка → PR
* [Пайплайн данных базы знаний](/runbooks/kb-data-pipeline.md) — регенерация embedded JSON из docs/ref/ через kbgen, обновление на патчах игры, ночной kb-monitor, лицензионные следствия GPL-3.0
