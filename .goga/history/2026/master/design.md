# Design — claudemd-compliance-migration (кодовая и инфраструктурная фаза)

Производный от: task.md + arch.md (контракты уже материализованы, коммит 0e45c4c).
Этот документ — дизайн имплементации оставшейся части: Go-код, тесты, CI, доки.

## 1. kb: генерация данных (GenKB) + инжекция логгера

**kb/gen.go (новый):**
- `func GenKB(refDir, dataDir string, log *slog.Logger) error` — перенос
  `run/genFunctions/genConstants/parseUpdates/sinceByFunction/sinceByConstant/
  rawValue/writeJSON/sortedKeys` из `cmd/kbgen/main.go:118-424`.
- Пути источников выводятся из `refDir` (layout — `kbdata`):
  `ugc-guide/xs/functions/functions.json`, `ugc-guide/xs/constants/constants.json`,
  `zetnus-rms-guide.txt`, `aoe2de-xs-rms-changelog.md`; выходы — три JSON в `dataDir`.
- Regexp-ы не дублируются: переиспользуются пакеты `updateHeadingRe/backtickRe/wordRe`
  из `kb/extract.go` (unexported, тот же пакет).
- Детерминированная сериализация: `encoding/json` MarshalIndent + стабильный
  порядок (сортировка ключей/списков как в текущем kbgen — сохранить поведение
  байт-в-байт для тех же входов; если текущий kbgen не сортировал — сортировать
  заново допустимо, JSON семантически эквивалентен).
- Ошибки: `fmt.Errorf("...: %w", err)` с путём файла.
- Doc-комментарий на экспорт.

**kb/extract.go:**
- `ExtractRmsCommands(path string, log *slog.Logger) ([]Command, error)`;
  в начале: `if log == nil { log = slog.Default() }`; WARN-вызовы (`parseSkeleton`,
  `placeholderKind`, `readChangelog`) получают `log` параметром (unexported).
- kb/extract.go:99: `fmt.Errorf("syntax skeleton heading not found in %s", path)` —
  контекст добавляется.

**cmd/kbgen/main.go:**
- Остаётся: разбор флагов (существующий CLI-контракт сохраняется), `slog`
  настройка, вызов `kb.GenKB(...)`; цель ≤ ~50 строк.

**Тесты:** kbgen не имеет тестов (логика была в cmd); после переноса —
table-driven тест `TestGenKB` (tmp-каталоги с мини-источниками → проверка трёх
JSON) — `t.TempDir()`, `t.Parallel()`.

## 2. server: точечные дефекты

- `server/server.go:350`: удалить задвоенную первую строку doc-комментария
  `Completion` («Completion answers with the kb entries in scope: for .rms the
  commands of»).
- `server/server.go:661`: `if err := client.PublishDiagnostics(ctx, ...); err != nil {
  slog.WarnContext(ctx, "publish diagnostics", "uri", ..., "err", err) }` —
  вместо `_ =`. Логгер — slog (дефолтный сконфигурирован entrypoint'ом;
  WarnContext с ctx — консистентно с wiring в serve.go).

## 3. Вложенность: 5 экстракций хелперов

Behavior-preserving, по предложениям аудита:
1. `analysis/analyzer.go:249` `collectLocals` → извлечь `declareItem(item Expr)`
2. `xs/ast.go:427` `collectLocals` → извлечь внутренний пер-элементный цикл
3. `xs/parse.go:275` `parseEvent` → извлечь `eventArgsName(args)`
4. `analysis/analyzer.go:84` `walkStmts` → извлечь guard-хелпер `checkArgValues`
5. `xs/ast.go:549` `appendLocals` → выровнять до ≤3 уровней

Имена хелперов — по домену; unexported; doc не нужен (unexported), комментарий —
«почему» при неочевидности.

## 4. Тесты: t.Parallel + нейминг

- Func-level `t.Parallel()` во всех тестах чистых пакетов: common, rms, xs,
  analysis, kb, complete, hints (включая table-driven — параллелим func;
  в устойчивых таблицах допускается и в `t.Run` — семантика переменных цикла
  Go 1.22+ безопасна).
- НЕ трогаем: `server/serve_test.go` (pipe-harness с горутиной Serve),
  `server/integration_crossfile_test.go` (оставляем sequential; отдельные
  `server/docstore_test.go`/`include/resolver_test.go` — добавляем, там
  `t.TempDir()` и свежие сторы: безопасно).
- `analysis/types_test.go:23` `TestCoerce_Table` → `TestCoerce_ValueShapes`
  (сценарий-имя; поведение теста не меняется).

## 5. CI: fmt-гейт

`.github/workflows/ci.yml` — шаг после goimports:
```yaml
- name: Check gofumpt formatting
  run: test -z "$(golangci-lint fmt --diff .)"
```
Установка golangci-lint уже есть (v2.13.2 action — шаг использует CLI из action?
Нет: action запускает lint сам). Вариант: отдельный step с установкой
`golangci/golangci-lint-action@v8` в `fmt` mode невозможен — вместо этого
`go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 fmt --diff .`
либо переиспользовать установку из action (`installation` не переиспользуется
между шагами). Рекомендовано: шаг `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2`
+ `golangci-lint fmt --diff .` (exit≠0 при диффе). Пин версии идентичен action.

## 6. Доки: реальность

- `CLAUDE.md`: «Go 1.23+» → «Go 1.26+»; `testdata/` → «`<cell>/testdata/` —
  фикстуры ячеек (rms/testdata, xs/testdata)». Остальное не трогать.
- `.goga/usages/conventions.md`: версия → 1.26+; добавить строку приоритета:
  «При противоречии с CLAUDE.md приоритет у CLAUDE.md.»
- `.goga/usages/cooks/lsp-protocol.md`: «Go 1.23+» → «Go 1.26+» (:266).
- `.goga/usages/cooks/github-actions.md`: в секцию Matrix cross-compile —
  примечание, благословляющее single-job sequential вариант для малых модулей
  (как в release.yml).
- `README.md`: фичи (добавить definition, references, document symbols,
  signature help); layout-блок (cmd/, hints/, complete/, include/, docs-поддерево);
  «each release tag is pushed by CI» → «releases are built and published by CI
  on tag push»; «requires Go 1.23+» → «Go 1.26+».
- `docs/plans/build-lsp-rms-xs.md`: отметить выполненные чекбоксы; шапка —
  статус «реализовано; ячейки hints/complete/include добавлены задачами
  docs/tasks/{lsp-navigation,cross-file-navigation,analysis-deepening}.md».
- `docs/tasks/*.md` (5 файлов): первая строка после заголовка —
  `Status: Done (PR #N)` по данным git-истории; ci-build.md — Done.

## 7. Порядок исполнения и коммиты

1. kb (gen.go + extract + kbgen slim + тест GenKB) — коммит «feat: kb cell — GenKB...»
2. server fixes — коммит «fix: server cell — ...»
3. вложенность refactors (analysis, xs) — коммит «refactor: ...»
4. тесты t.Parallel + rename — коммит «test: ...»
5. CI fmt-гейт — коммит «ci: ...»
6. доки (CLAUDE.md, conventions, cooks, README, план, tasks) — коммит «docs: ...»

Проверки после каждого шага: memory-cap `go test ./... -count=1`;
финал: `golangci-lint fmt` → `golangci-lint run` → `goga lint` →
`goga contract kb` (GenKB implementation должна появиться).
