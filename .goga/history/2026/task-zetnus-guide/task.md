# zetnus-guide — убрать вендоренный текст гайда, оставить производный rms-commands.json

Зеркало — GitHub issue #144, PR закрывает `Fixes #144`.

## Current State

`docs/ref/zetnus-rms-guide.txt` — 279 КБ полного текста «Definitive Random
Map Scripting Guide» Zetnus, закоммиченного как есть. У гайда нет явной
лицензии (README сам это фиксирует: «no explicit license; used factually
with attribution»), а вендорить целый документ — уже не «фактическое
использование», а воспроизведение. Аудит лицензий по репозиторию нашёл это
единственным риском.

Кодовая зависимость от гайда ровно одна: `GenKB` зовёт
`ExtractRmsCommands(filepath.Join(refDir, "zetnus-rms-guide.txt"))`
(`internal/kb/gen.go:135`) и из результата пишет
`internal/kb/data/rms-commands.json` (53 команды, закоммичен, встроен через
`go:embed`). Рантайм гайд не читает. Пять тестов читают реальный файл через
константу `zetnusGuide` (`extract_test.go`, `extract_glossary_test.go`).
`cmd/kbgen -diff` требует все три файла данных (`DiffKB`,
`internal/kb/diff.go:131`).

## Description

Вариант (a) разрешения лицензионного риска: в репозитории остаётся только
**производный** `rms-commands.json` (имена команд/аргументов и наши
извлечённые краткие описания) и ссылка на оригинал; полный текст гайда
перестаёт коммититься и уходит в `.gitignore` (локальная необязательная
копия — как `.corpus`). Пайплайн при отсутствии гайда не падает: `GenKB`
сохраняет уже лежащий производный файл и логирует WARN — тот же режим, что
`docs/kb-refresh.md` уже описывает словами («гайд может задержаться»).

## Scope

**In scope:**

- `internal/kb/gen.go` — гайд опционален: при `os.ErrNotExist` WARN и пропуск
  overlay → coverage → записи `rms-commands.json`; `xs-functions.json` /
  `xs-constants.json` пишутся как раньше
- `internal/kb/extract.go` — константа `guideFile`
- `cmd/kbgen/main.go` — `-diff` без гайда не читает отсутствующий файл как
  «все команды удалены» (подкладывает прежний `rms-commands.json` в temp)
- `internal/kb/*_test.go` — real-guide тесты через helper со skip-паттерном
  (как `.corpus` в `internal/format/invariants_test.go`); тест на путь
  «гайда нет»
- `git rm --cached docs/ref/zetnus-rms-guide.txt` (файл остаётся на диске),
  `.gitignore`
- README (Data sources + License), `docs/kb-refresh.md`,
  `knowledge/runbooks/kb-data-pipeline.md`,
  `internal/kb/.usages/data-pipeline.md`, аннотация `kbdata` в
  `internal/kb/CODEMANIFEST`

**Out of scope:**

- скрипт-фетчер гайда из Google Doc (сетевой шаг; решено не заводить)
- изменения публичного API ячейки `kb` (`Store`/`Command`/`Function`/… —
  контракт не трогаем; правится только оффлайн-этап `GenKB`, вынесенный в
  CODEMANIFEST «вне рантайма»)
- регенерация `rms-commands.json` (файл остаётся байт-в-байт)
- удаление вендоренной `docs/ref/ugc-guide/**` (под GPL-3.0 — чисто)
- исторические документы (`docs/arch`, `docs/plans`, `docs/tasks`,
  `.goga/history`) — записи о том, как было

## Acceptance Criteria

- `docs/ref/zetnus-rms-guide.txt` не отслеживается git и попадает под
  `.gitignore` (`git check-ignore`); на диске у разработчика остаётся
- `go run ./cmd/kbgen` без гайда: exit 0, WARN в логе, `xs-*.json`
  перезаписаны, `rms-commands.json` не создан/не перезаписан
- `go run ./cmd/kbgen -diff` без гайда: exit 0, секция RMS-команд пуста
- `go run ./cmd/kbgen` с гайдом: `rms-commands.json` регенерируется
  байт-в-байт (детерминизм, пустой `git diff`)
- тесты: с гайдом на диске real-guide тесты выполняются; без него — skip;
  тест «гайда нет» зелёный в обоих режимах
- `make check`, `goga contract internal/kb` — зелёные

## Stack

- **Frameworks:** нет (Go 1.26+, module `aoe2-lsp`)
- **Libraries:** только stdlib — `os`, `errors`, `log/slog`, `path/filepath`
- **Infrastructure:** нет; сервер данные читает из встроенного `go:embed`

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| —         | —          | новых внешних зависимостей нет; проектная практика — `.goga/usages/conventions.md` |

## Risks and Constraints

- **Потеря покрытия в CI:** real-guide тесты (пины реального документа)
  начинают skip'аться без локальной копии. Механика извлечения остаётся
  покрытой синтетическими фикстурами (`gen_test.go`, `gen_overlay_test.go`);
  мейнтейнер с локальным гайдом прогоняет real-guide тесты как раньше.
- **Расхождение аннотации CODEMANIFEST:** `kbdata` перестаёт описывать
  источник `rms-commands.json` как вендоренный файл. Правка — фактологическая
  (происхождение данных), публичный контракт не меняется; требуется
  подтвердить, что правка аннотации в кодовой задаче допустима.
- **Локальная копия на диске:** `git rm --cached` не удаляет файл; после
  правки он остаётся у разработчика (нужен для регенерации команд).
- **Обратимость:** если Zetnus даст явное разрешение (письмо отправляется
  параллельно), ограничение снимается отдельной задачей.

## Scope Estimate

Одна задача **S-M**, декомпозиция не нужна: правка `kb`-ячейки небольшая,
тесты, вывод файла из индекса, документация. Ветвь `task/zetnus-guide` от
master → PR.

## Existing Architecture

- `internal/kb` — ячейка KB: `Store` (встроенный JSON, read-only lookup) +
  оффлайн-пайплайн (`GenKB`, `ExtractRmsCommands`, `DiffKB`)
- `internal/kb/gen.go` — `GenKB(refDir, dataDir, log)`: changelog + UGC
  functions/constants + гайд Zetnus → три JSON
- `internal/kb/extract.go` — `ExtractRmsCommands(path, log)`, `const changelogFile`
- `internal/kb/diff.go` — `DiffKB(oldDir, newDir)` требует все три файла
- `cmd/kbgen` — тонкая обвязка `GenKB`/`runDiff`
- `docs/kb-refresh.md` — процесс обновления KB; уже декларирует «гайд может
  задержаться»

## Notes

- Решение о поведении без гайда: `GenKB` сохраняет закоммиченный
  `rms-commands.json` + WARN (не падать, не тянуть из сети) — подтверждено
  в ходе формулирования.
- Нарезка: одна задача/PR — код, удаление из трекинга и документация
  внутренне связаны (убрав файл без правки `GenKB`, ломаем пайплайн).
- Leave-локально семантика: файл остаётся на диске (как `.corpus`), чтобы
  мейнтейнер мог по-прежнему регенерировать команды.
