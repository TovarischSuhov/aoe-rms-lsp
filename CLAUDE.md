# aoe_maps / aoe2-lsp

Проект: LSP-сервер `aoe2-lsp` для AoE2 RMS + XS (Go 1.23+). Авторитетный
план — `docs/plans/build-lsp-rms-xs.md`, контракты ячеек — `CODEMANIFEST`
в каждой директории (.goga). Валидация после задач: `go test ./...`,
`goimports -w .`, `golangci-lint run`, `goga lint`, `goga contract <cell>`.

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

## Тесты — только под лимитом памяти

`go test` запускать в cgroup-песочнице (см. memory
`go-tests-under-memory-cap`): runaway-тест уже валил машину в OOM.

```bash
timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 \
	bash -c 'go test ./... -count=1'
```
