# kb-refresh-185872: refresh kb под патч 185872 + ренеймы XS в LSP

## Current State

Процесс обновления kb «день патча» уже существует и документирован
(`docs/kb-refresh.md`, задача #57): чеклист источников, `cmd/kbgen -diff`
(`kb.DiffKB`) как агрегированный отчёт «что нового», регенерация данных,
проверки. Changelog-док `docs/ref/aoe2de-xs-rms-changelog.md` собран по 77
постам release notes; верхняя секция — Update 177723 (2026-06-02).

Монитор патчей (issue #61, слот A пачки `kb-freshness`) сработал: **issue
#132** — новый пост [Update 185872](https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-185872/)
от 2026-09-22. Refresh по нему не выполнен.

Патч — крупнейший по XS/RMS-поверхности за весь период наблюдений:

- ренеймы существующих функций: `xsGetLocalPlayerId` →
  `xsUnsyncGetLocalPlayerId`, `xsGetTimerTimeRemaining` →
  `xsUnsyncGetTimerTimeRemaining`;
- десятки новых функций (unsync/local-семейство, строковые `ord`/`chr`/
  `strLen`/`strSplit`/`fstr`, аналоги триггерных эффектов), новые
  escape-последовательности;
- новые object attributes 167–221, новые атрибуты задач, новые Projectile
  Smart Mode флаги;
- RMS: исправления `ATTR_*`, unit class constants, `random_map.def`
  расширен с 41 устаревшей декларации до 194 актуальных записей;
- изменения XS: инициализация globals, `runImmediately`, `Effects.xs`,
  `xsModifyObject`/`xsGetTaskAmount`/`xsCreateUnit`.

При этом upstream-источники отстают: в
`docs/ref/ugc-guide/xs/functions/functions.json` по-прежнему только старые
имена, Zetnus-гайд не переэкспортирован. Следствия сейчас:

1. release-notes-шаг даст почти пустой дифф — новые имена в данных ещё не
   существуют, и `since_update` их не за что зацепить;
2. LSP ложно сработает `undefined-symbol` (`internal/analysis/analyzer.go`)
   на каждое новое имя патча;
3. LSP промолчит про имена, которых в игре больше нет: данные продолжат
   отдавать `xsGetLocalPlayerId` и `xsGetTimerTimeRemaining` как валидные.

Прецедент обработки устаревшего имени в LSP ровно один и захардкожен под
одну команду: `effect_percent` → `effect_amount`
(`analysis.CodeDeprecatedEffectPercent` → code-action
`replaceEffectPercentAction` в `internal/server` → семантический токен
`deprecated`).

## Description

Один топик, два слота; каждый слот берётся отдельно, в своей ветке и своём
PR.

**Слот A — `kb-refresh-185872`: refresh данных по патчу.**

1. Дополнить `docs/ref/aoe2de-xs-rms-changelog.md` секцией
   «## Update 185872 — 2026-09-22» с URL поста: транскрипция XS/RMS-разделов
   release notes по формату существующих секций, включая новую секцию
   «### XS: переименовано» (её потребляет слот B). Транскрипция — ручная, как
   и раньше.
2. `go run ./cmd/kbgen -diff` → отчёт в описание PR; `go run ./cmd/kbgen` →
   регенерация `internal/kb/data/*.json`; смысл `git diff` по JSON обязан
   совпасть с отчётом.
3. Проверки: `make check`, `goga contract internal/kb`, корпус-прогон.
4. PR в `master`, `Fixes #132`; в описании явно зафиксирован lag источников
   (новые сущности из UGC Guide/Zetnus появятся отдельным прогоном, когда
   апстрим обновится).

**Слот B — `kb-renames`: changelog как источник ренеймов и новых имён.**

1. `GenKB` читает changelog-док не только ради `since_update`, но и как
   источник фактов об именах:
   - секция «### XS: переименовано» → пары «старое → новое»;
   - секции «добавлено» → факт существования имени: запись-заготовка с
     `since_update` из секции и пустым desc, если источника-гайда для имени
     ещё нет. Когда гайд догоняет, запись гайда вытесняет заготовку
     (fill-when-empty merge), `since_update` сохраняется.
2. Данные переименования отдаются наружу через `Store`; lookup старого имени
   отвечает новым. Целевой API (скетч, финал — на дизайне):

   ```go
   // Store: новое имя для переименованного; ok=false, если имя не переименовано.
   func (s *Store) RenamedTo(name string) (to string, ok bool)

   // model: поле переименования на записи (Function/Constant/Command).
   RenamedTo string // "" — не переименовано
   ```

3. `internal/analysis`: хардкод `effect_percent` обобщается до диагностики
   «имя переименовано» по данным (новый diagnostic code и его severity — на
   дизайне); `undefined-symbol` перестаёт срабатывать на имена, известные из
   changelog (заготовки и добавленные).
4. `internal/server`: code-action переименования обобщается с
   `replaceEffectPercentAction`; семантический токен `deprecated`
   проставляется по данным, не по хардкоду.
5. `internal/hints` и `internal/complete`: новое имя — основная подсказка,
   старое — с пометкой переименования.
6. Правки контрактов и практик: `CODEMANIFEST` ячейки `internal/kb`
   (модель/`Store`/`GenKB`/`kbdata`), `internal/analysis/.usages/checks.md`,
   usages ячейки `internal/server`, дополнение `docs/kb-refresh.md`
   (регламент секции «переименовано» и правила заготовок), уточнение
   контракта desc-покрытия (#59) — заготовки с пустым desc не должны
   читаться регрессом покрытия.
7. Знания: обновить `knowledge/runbooks/kb-data-pipeline.md` (changelog-док
   перестаёт быть источником только `since_update` — становится источником
   ренеймов и факта существования имён) и
   `knowledge/architecture/stability-invariants.md` (новый diagnostic code
   входит в набор стабильных кодов) тем же PR.

## Scope

**In scope:**

- слот A: секция 185872 в changelog-доке, дифф-отчёт, регенерация данных,
  проверки, PR (`Fixes #132`)
- слот B: чтение changelog как источника ренеймов и существования имён в
  `GenKB`; данные переименования в модели и `Store`; обобщённая диагностика
  переименования в `analysis`; подавление ложного `undefined-symbol` для
  имён из changelog; code-action и семантический токен по данным в `server`;
  подсказки нового имени в `hints`/`complete`
- правки контрактов/практик: `CODEMANIFEST` internal/kb и internal/analysis,
  `.usages` ячеек kb/analysis/server, `docs/kb-refresh.md`, контракт
  desc-покрытия (#59)
- знания: `knowledge/runbooks/kb-data-pipeline.md`,
  `knowledge/architecture/stability-invariants.md` (тем же PR, что и правка
  поведения)
- тесты: golden-фикстуры changelog-секций (ренеймы, заготовки), тест
  сохранения `since_update` при «guide-wins», тест идемпотентности
  регенерации, тесты диагностики и code-action (сохранение поведения
  `effect_percent`)

**Out of scope:**

- пересинхронизация upstream-источников (UGC Guide functions/constants,
  Zetnus-гайд) — ждём апстрим; отдельный прогон процесса по чеклисту
  (follow-up issue)
- `kb-live-update` — подхват обновлённых данных сервером без релиза
  бинарника (#62)
- заполнение недостающих desc для новых имён (`kb-attribute-desc`, #59)
- автопарсинг постов release notes и автогенерация kb — транскрипция
  changelog остаётся ручной
- изменения состава диагностик, не связанные с переименованиями

## Acceptance Criteria

Общее для слотов:

- `make check` зелёный; `goga contract` затронутых ячеек зелёный; CI ветки
  зелёный

Слот A:

- секция «## Update 185872 — 2026-09-22» присутствует в changelog-доке,
  формат совпадает с существующими секциями
- `kbgen -diff` на текущих источниках даёт отчёт, воспроизводимый повторным
  прогоном (в т.ч. осознанно пустой/минимальный, если апстрим ещё не
  обновился — причина зафиксирована в PR)
- смысл `git diff` по `internal/kb/data/*.json` совпадает с отчётом; данные
  после регенерации проходят `NewStore`
- корпус-прогон не хуже master: расхождения — только по существу диффа, не
  по числу падений/диагностик

Слот B:

- код, использующий переименованную в 185872 функцию, получает диагностику с
  указанием нового имени; code-action применяет переименование (проверяется
  тестом на фикстуре changelog-секции)
- новые имена из changelog не дают `undefined-symbol` до обновления UGC
  Guide; после обновления гайда регенерация сходится к записи гайда, а
  `since_update` сохраняется (идемпотентность, golden-данные)
- семантический токен `deprecated` и code-action работают от данных: прежнее
  поведение `effect_percent` сохранено тестом, хардкод под одну команду
  устранён
- повторная регенерация на тех же источниках даёт байт-идентичные файлы
  данных
- контракт desc-покрытия уточнён: заготовки с пустым desc не считаются
  регрессом покрытия, уточнение зафиксировано в `knowledge/`

## Stack

- **Frameworks:** Go 1.26+ (stdlib; разбор changelog-дока — существующий
  текстовый проход `GenKB`)
- **Libraries:** новых зависимостей нет
- **Infrastructure:** без изменений CI (корпус-прогон — `workflow_dispatch`
  на ветке, как обычно)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| — | — | новых внешних зависимостей нет |

Release notes ageofempires.com — источник, который мейнтейнер читает вручную
при транскрипции changelog; автоматизация детекта уже сделана монитором
(#61), usage-файл не требуется.

## Risks and Constraints

- **upstream-lag** — заготовки и ренеймы держат LSP честным относительно
  игры, но desc/типы новых имён пусты до обновления гайдов: сигнатура-хелп
  по ним молчит. Это осознанное ограничение, а не дефект
- **ручной источник ренеймов** — пары «старое → новое» вносит мейнтейнер;
  ошибка транскрипции бьёт по диагностике напрямую, поэтому нужен тест на
  фикстуре, а не только на боевом changelog
- **desc-контракт #59** — заготовки меняют метрику покрытия; контракт
  уточняется явно, иначе предупреждения о непустом desc начнут читаться
  регрессом
- **идемпотентность** — merge «guide-wins» обязан сохранять `since_update` и
  давать байт-идентичный результат при повторной регенерации
- **расширение публичной поверхности** — `Store`/модель kb и новый
  diagnostic code в analysis расширяются осознанно, через `CODEMANIFEST`;
  имена кодов и severity — решение этапа дизайна
- goga-гатча: бэктики в аннотациях `CODEMANIFEST` — только в
  imports/usages/entities; сигнатуру метода в аннотации писать без бэктиков

## Scope Estimate

Два слота в одном топике:

| Слот | Ветка | Размер | Зеркало |
|---|---|---|---|
| A — refresh данных 185872 | `task/kb-refresh-185872` | S | #132 (`Fixes #132`) |
| B — ренеймы и новые имена | `task/kb-renames` | M | новая issue |

Слот B не блокируется слотом A жёстко, но его проверки информативнее после
того, как секция 185872 появится в changelog-доке (ренеймы и новые имена —
реальные данные для фикстур).

## Existing Architecture

- `internal/kb` — модель (`model.go`), `Store` (`store.go`), `GenKB`
  (`gen.go`, обогащение `since_update` из changelog), `DiffKB` (`diff.go`),
  данные `internal/kb/data/*.json`; CODEMANIFEST описывает `kbdata`-схему и
  утилиты сборки
- `internal/analysis` — `Analyzer` (`analyzer.go`): diagnostic codes,
  `xsBuiltins`, `CodeDeprecatedEffectPercent` как захардкоженный прецедент;
  `tokens.go` — семантические токены, `TokenDeprecated`
- `internal/server` — публикация диагностик, code-actions
  (`replaceEffectPercentAction`), легенда токенов
- `internal/hints` (`computer.go`, `renderKb`) и `internal/complete`
  (`completer.go`) — потребители записей kb для сигнатура-хелпа и
  автодополнения
- `docs/ref/aoe2de-xs-rms-changelog.md` — источник `since_update`; его шапка
  содержит регламент пополнения (расширяется секцией «переименовано»)
- `docs/kb-refresh.md` — процесс «день патча»; `internal/kb/.usages/data-pipeline.md`
  — практика регенерации
- `internal/corpus` — корпус-прогон как приёмочная проверка неизменности
  поведения
- `knowledge/runbooks/kb-data-pipeline.md` — описание пайплайна (источник
  `since_update`, монитор, лицензии); `knowledge/architecture/stability-invariants.md`
  — инвариант «стабильные коды диагностик», в который войдёт код переименования

## Notes

- Формулировка собрана 2026-09-25 по /goga-propose; решения пользователя:
  (1) объём — refresh **и** ренеймы в LSP, не только данные; (2) changelog-док
  — единый источник и ренеймов, и факта существования новых имён;
  (3) декомпозиция — две подзадачи в одном топике
- Топик `task-kb-refresh` (2026) — предыдущая задача #57 (процесс и
  инструмент diff); эта задача его использует, но не переписывает
- Зеркало слота A — существующая issue #132 (новая не заводится); зеркало
  слота B — новая issue по схеме меток репозитория (`enhancement`, размер,
  при необходимости `P2`)
- Правило проекта: каждой задаче — своя ветка `task/<имя>` от актуального
  `master`; мерж PR закрывает своё issue
