# aoe_maps / aoe2-lsp

LSP-сервер `aoe2-lsp` для AoE2 RMS + XS (Go 1.23+, module `aoe2-lsp`).
Авторитетный план — `docs/plans/build-lsp-rms-xs.md`, контракты ячеек —
`CODEMANIFEST` в каждой директории (.goga).

## Commands

- `go build ./...` — сборка
- `go test ./... -count=1` — тесты (только под memory cap, см. ниже)
- `golangci-lint run` — линтер (v2-конфиг `.golangci.yml`)
- `golangci-lint fmt` — форматирование (gofumpt)
- `goga lint` / `goga contract <cell>` — DSL-контракты ячеек
- **Проверка перед завершением задачи** (доводить до зелёного):
  memory-cap `go test ./... -count=1` → `golangci-lint fmt` →
  `golangci-lint run` → `goga lint` → `goga contract` затронутых ячеек

## Тесты — только под лимитом памяти

`go test` запускать в cgroup-песочнице (см. memory
`go-tests-under-memory-cap`): runaway-тест уже валил машину в OOM.

```bash
timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 \
	bash -c 'go test ./... -count=1'
```

Race-прогон — в CI (`go test -race ./...`).

## Conventions

- stdlib first; новые зависимости (`go get`) — только после явного вопроса.
- Ошибки оборачивай через `fmt.Errorf("...: %w", err)`, не отбрасывай
  через `_`.
- `context.Context` — первый аргумент, пробрасывай дальше, не храни в
  структурах.
- Публичная поверхность ячейки — её `CODEMANIFEST` (read-only в кодовых
  задачах). Принимай интерфейсы (`Source`), возвращай структуры.
- Тесты table-driven, same package; `t.Parallel()` для новых тестов без
  общего состояния.
- `any` вместо `interface{}`; без `github.com/pkg/errors` и `io/ioutil`.
- Комментарий — «почему», не пересказ кода; doc-комментарий обязателен
  на экспортах (`.goga/usages/conventions.md`).
- Формат — `golangci-lint fmt` (gofumpt); линт при правках гоняет хук.

## Structure

- `cmd/` — тонкие entrypoints (`aoe2-lsp`, `kbgen`)
- ячейки-пакеты: `common`, `kb`, `rms`, `xs`, `analysis`, `hints`,
  `complete`, `include`, `server` — контракт каждой в её `CODEMANIFEST`,
  потребительские практики в `<cell>/.usages/`
- `docs/` — планы, дизайн, задачи, ревью, справочники
- `testdata/` — фикстуры ячеек

## Git-политика (переопределение глобальной)

Для ЭТОГО проекта коммиты и другие git-команды **разрешены** без
дополнительного разрешения. Это явное переопределение глобального правила
«никогда не коммитить и не пушить самому»: здесь можно самостоятельно
выполнять `git add`, `git commit`, `git push`, `git branch` и прочие
git-команды, не спрашивая каждый раз.

Правила оформления:

- Коммитить атомарно, по одной ячейке/задаче на коммит, в стиле истории
  репозитория (`feat: rms cell — …`, `fix: …`, `docs: …`).
- Перед коммитом — зелёный `go test ./...`; коммитить только проверенное.
- Push — после коммитов, в текущую ветку (`master`).

## Рабочий процесс: задача → ветка → PR

- Каждая задача/подзадача (из `docs/tasks/` или плана `.goga/history/`)
  выполняется в отдельной ветке `task/<имя>` от актуального `master`.
- Агент делает всю работу в ветке: атомарные коммиты, зелёные проверки,
  пуш ветки, открытие PR в `master`.
- Пользователь отвечает только за итоговое ревью и мердж PR.
  Самостоятельно мерджить PR и закрывать их нельзя.
- Прямые коммиты в `master` — только для docs/задач и правок
  инфраструктуры вне кодовых задач.
