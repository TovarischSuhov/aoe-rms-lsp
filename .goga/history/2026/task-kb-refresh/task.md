# kb-refresh: процесс обновления базы знаний под патчи игры

## Current State

Пайплайн данных kb существует и детерминирован: `GenKB(refDir, dataDir, log)`
собирает `internal/kb/data/*.json` из локальных источников `docs/ref/`
(functions/constants JSON ugc-guide, Zetnus-гайд через `ExtractRmsCommands`,
changelog для `since_update`), обёртка — `cmd/kbgen`. Changelog
(`docs/ref/aoe2de-xs-rms-changelog.md`) собран вручную 2026-09-04 по 77 постам
release notes. Чего нет — **процесса обновления под патч игры**: ни чеклиста
источников, ни агрегированного диффа «что изменилось в kb» (сегодня только
шумный `git diff` по JSON-файлам), ни регламента ведения `since_update`.
Без этого база устареет к 1.x (мотивировка issue #57).

## Description

Документированный воспроизводимый пайплайн обновления kb на «день патча»:

1. **Док-процесс для мейнтейнера** — `docs/kb-refresh.md`: чеклист источников
   (UGC Guide functions/constants, Zetnus-гайд, официальные release notes
   ageofempires.com) с адресами и порядком обновления; шаги проверки
   (`make check`, корпус-прогон). Процесс обязан переживать задержки внешних
   мейнтейнеров: sources обновляются независимо (release notes → since_update
   не ждут обновления гайда).
2. **Полуавтоматический дифф** — агрегированный отчёт old→new по сущностям
   (новые/удалённые/изменённые команды, функции, константы; для изменённых —
   какие поля). Точка входа — режим diff в `cmd/kbgen`: регенерация в
   временный каталог, сравнение с текущим `internal/kb/data`, детерминированный
   markdown-отчёт (годится в описание PR). Целевой API (скетч, финал — на
   дизайне):

   ```go
   // DiffKB сравнивает два каталога данных kb (текущий и свежесгенерированный).
   func DiffKB(oldDir, newDir string) (report: DiffReport, err: error)

   type DiffReport struct {
       Functions, Constants, Commands EntityDiff
   }

   // EntityDiff: Added/Removed — имена; Changed — имя + список изменённых полей.
   type EntityDiff struct {
       Added   []string
       Removed []string
       Changed []ChangedEntity
   }

   // Render — детерминированный markdown-отчёт (стабильный порядок: сортировка).
   func (r DiffReport) Render() (md: string)
   ```

3. **Регламент `since_update`** — шапка changelog-дока описывает, как дополнять
   его новыми апдейтами (формат секций по существующим записям) и что после
   этого базе требуется перегенерация.

## Scope

**In scope:**

- `docs/kb-refresh.md` — чеклист «день патча»: источники, порядок, проверка
- режим diff в `cmd/kbgen` (флаг/подкоманда) + рутина `DiffKB` в `internal/kb`
  с детерминированным рендером отчёта
- правка шапки `docs/ref/aoe2de-xs-rms-changelog.md` (регламент дополнения)
- дополнение `internal/kb/.usages/data-pipeline.md` указателем на процесс
  обновления
- правка `CODEMANIFEST` internal/kb: новая утилита сборки (`DiffKB`) —
  оффлайн-этап, как `GenKB`/`ExtractRmsCommands`
- тесты `DiffKB` (golden-файл отчёта в `internal/kb/testdata`)

**Out of scope:**

- автодетект постов патчей (release-notes-monitor, #61)
- подхват обновлённого KB сервером без релиза бинарника (kb-live-update, #62)
- заполнение недостающих desc атрибутов (kb-attribute-desc, #59)
- автоскачивание источников (sources в `docs/ref/` остаются ручными)

## Acceptance Criteria

- `make check` зелёный; `goga contract internal/kb` зелёный
- `docs/kb-refresh.md` описывает процесс: источники с адресами, порядок,
  проверка; процесс переживает задержку любого из внешних источников
- kbgen-режим diff: на неизменных исходниках — воспроизводимый пустой отчёт;
  на тестовых данных — стабильный отчёт (golden)
- повторный прогон процесса на текущих исходниках воспроизводит идентичные
  файлы данных (идемпотентность) и идентичный дифф-отчёт
- корпус-прогон до/после идентичен (данные не меняются; число диагностик то же)
- регламент `since_update` задокументирован в шапке changelog-дока

## Stack

- **Frameworks:** Go 1.26+ (stdlib; сравнение сущностей — map-lookup, без
  сторонних diff-библиотек)
- **Libraries:** нет новых зависимостей
- **Infrastructure:** без изменений CI (корпус-прогон — workflow_dispatch на
  ветке, как обычно)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| — | — | новых внешних зависимостей нет |

## Risks and Constraints

- Источники обновляются внешними мейнтейнерами (UGC Guide, Zetnus) — процесс
  не должен блокироваться их задержками: шаги независимы, release notes
  достаточны для `since_update` без гайда
- рендер отчёта обязан быть детерминированным (сортировка, не порядок map'ы) —
  отчёт сравнивается повторными прогонами и идёт в PR
- расширение публичной поверхности kb (`DiffKB`) — только как утилита сборки
  (оффлайн), контракт Store («No IO after construction») не трогается
- goga-гатча: бэктики в аннотациях CODEMANIFEST — только imports/usages/
  entities; сигнатуру метода в аннотации без бэктиков

## Scope Estimate

Одна задача (S-слот пачки ux-and-data-quality), без декомпозиции. Ветка
`task/kb-refresh` → PR, `Fixes #57`.

## Existing Architecture

- `internal/kb`: `GenKB` (gen.go), `ExtractRmsCommands` (extract.go),
  `Store`/`NewStore` (store.go), модель `SinceUpdate` (model.go);
  CODEMANIFEST ячейки — контракт, расширяется утилитой сборки
- `cmd/kbgen` — тонкая обёртка `GenKB` (флаги, без собственной логики) —
  получает режим diff
- источники: `docs/ref/ugc-guide/xs/{functions,constants}/*.json`,
  `docs/ref/zetnus-rms-guide.txt`, `docs/ref/aoe2de-xs-rms-changelog.md`
- `internal/kb/.usages/data-pipeline.md` — существующая практика
  регенерации, дополняется указателем на docs/kb-refresh.md
- `internal/corpus` — прогон как приёмочная проверка неизменности данных

## Notes

- Слот `kb-refresh` пачки ux-and-data-quality (формулировка пачки:
  `.goga/history/2026/task-ux-and-data-quality/task.md`); issue #57 уже
  существует как зеркало — новая issue не нужна, мердж закрывает её
- решение (2026-09-15, пользователь): дифф — рутина в `internal/kb` с
  обёрткой в kbgen (публичная поверхность kb расширяется осознанно, через
  CODEMANIFEST); пример целевого API включён в Description
- kb-refresh — ручной процесс; автоматизация детекта постов — отдельная
  задача #61, подхват KB сервером — #62 (1.x)
