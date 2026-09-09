# Пачка UX и data quality: 7 слотов + 2 кандидата 1.x

## Current State

aoe2-lsp v0.4.0: LSP-поверхность ядра готова (диагностика, hover,
completion, signatureHelp, навигация, semantic tokens, folding,
quickfix), эпик v1.0.0 (documentLink, selectionRange, rename, format,
публикация) сформулирован отдельно. При этом: корпусная мудрость
(`docs/ref/real-map-nuances.md`, ✅/❓/✍️/💬 по 100 картам) так и не
превращена в правила анализа — слот nuance-rules из пакета
editor-experience выпал между волнами; расширение не даёт сниппетов и
статической подсветки (подсветка — отдельная задача vscode-highlighting);
структурные ошибки карты (нет обязательных секций, дубликаты) не
диагностируются; kb не имеет процесса обновления под патчи игры.

## Description

Roadmap-пачка самостоятельных задач повседневной ценности для
скриптеров + гигиена данных. Каждый слот берётся отдельно, в ветке
`task/<имя-слота>` → PR; S-слоты реализуются сразу, M — с коротким
дизайн-проходом. Не зависит от эпика v1.0.0, может чередоваться с ним.

## Scope

**In scope (слоты):**

| Слот | Что делаем | Ячейки/области | Размер |
|---|---|---|---|
| `nuance-rules` | Вердикты real-map-nuances → данные в kb (алиасы/опечатки/no-op списки) + правила: «вероятно игнорируется»=hint, ✍️=warning. Перенос потерянного слота editor-experience (волна 1) | analysis, kb | M |
| `snippets` | VS Code snippets: скелет новой карты (Zetnus skeleton), частые блоки из map-scripting-practices (create_object с полями, start_random, base_terrain…) | editors/vscode | S |
| `map-structure` | Структурные линты карты: отсутствующие обязательные секции, дубликаты секций, пустые секции; коды диагностик + severity-override из конфига работают как обычно | analysis (+rms Sections), server | S–M |
| `fuzz` | Fuzz-цели парсеров rms/xs (native Go fuzzing), сиды из фикстур и корпуса; CI-джоба с коротким бюджетом | rms, xs, CI | S |
| `kb-refresh` | Документированный процесс обновления kb под патч игры: чеклист источников (UGC Guide, release notes), полуавтоматический diff «что нового», версионирование "since update N" | kb, docs/ref | S |
| `duplicate-include` | Warning на повторный `#include`/`#includeXS` одного файла в замыкании одного корня (дубли эффектов — реальная боль RMS) | include, analysis/server | S |
| `kb-attribute-desc` | Полнота текстов атрибутов CommandArg в kb — предусловие для hover-attribute (показывать справку поля будет нечего без desc) | kb (данные) | S–M |

**Out of scope (кандидаты 1.x):**

- command-reference webview в VS Code (browseable kb, поиск по секциям) —
  продуктовая фича, после 1.0.0
- контрибуция в nvim-lspconfig upstream — после публикации и 1.0.0

## Acceptance Criteria

Общие для каждого слота:

- `make check` зелёный; `goga contract` затронутых ячеек зелёный
- новые диагностики: коды задокументированы, работают severityOverrides
  (`none` подавляет), не шумят на ✅-конструкциях корпуса (прогон корпуса
  как приёмка для analysis-слотов)

Слотовые критерии:

- `nuance-rules`: находки справочника дают hint/warning на картах
  корпуса; ✅-конструкции не шумят
- `snippets`: скелет карты разворачивается в валидный минимальный RMS;
  сниппеты в .vsix
- `map-structure`: карта без обязательных секций получает диагностику с
  кодом и quickfix-кандидатом (вставить секцию) — quickfix опционален
- `fuzz`: fuzz-прогон (короткий, seeded) без паник; джоба в CI не длиннее
  пары минут
- `kb-refresh`: README/docs описывают процесс; прогон на актуальном
  патче показывает воспроизводимый diff
- `duplicate-include`: двойной include в фикстуре → warning; легитимные
  паттерны (если есть в корпусе) не шумят
- `kb-attribute-desc`: покрытие desc по атрибутам, используемым в
  completion/hover; отчёт по пробелам

## Stack

- **Frameworks:** Go 1.26+ (stdlib, native fuzzing), TypeScript для
  snippets (только JSON-контрибуция VS Code)
- **Libraries:** без новых зависимостей
- **Infrastructure:** GitHub Actions (fuzz-джоба опционально nightly)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| VS Code snippets contribution | `.goga/usages/cooks/vscode-extension.md` | update в слоте snippets (секция) |
| Go native fuzzing | — | stdlib, usage не требуется |

## Risks and Constraints

- `nuance-rules`/`map-structure`: главный риск — шум на реальных картах;
  приёмка прогоном корпуса обязательна, пороги severity — на design-этапе
- `kb-refresh`: источники обновляются внешними мейнтейнерами (UGC Guide) —
  процесс должен переживать их задержки
- `duplicate-include`: сверить семантику игры (двойной include
  действительно вреден?) с корпусом/справочником до реализации
- Пачка не блокирует и не блокируется эпиком v1.0.0

## Scope Estimate

7 слотов (3×S, 2×S–M, 1×M, 1×S для kb-refresh — итого ~2–3 «волны»
по настроению); каждый — независимая задача со своей веткой и PR.

## Existing Architecture

- `nuance-rules`/`map-structure` опираются на `analysis.Analyzer`
  (паттерн добавления проверок) и `kb.Store` (данные)
- `snippets` — editors/vscode/package.json contributes.snippets
- `fuzz` — фикстуры internal/rms/testdata, internal/xs/testdata, корпус
- `kb-refresh`/`kb-attribute-desc` — пайплайн internal/kb (GenKB,
  ExtractRmsCommands), docs/ref-источники
- `duplicate-include` — include.Closure.Resolved (владельцы/цели уже там)

## Notes

- Пачка собрана 2026-09-10 по итогам разбора «какие ещё задачи полезны»;
  пользователь утвердил фиксацию всех предложенных слотов
- nuance-rules — перенос потерянного слота editor-experience (волна 1,
  никогда не подбирался: топика в истории нет)
- webview/nvim-lspconfig осознанно в 1.x-кандидаты
