# fix-include-within-root-windows: виндовая корректность путей в ячейке include

Задача вне бэклог-очереди: вскрылась при переводе разработки на машину
с Windows (тулинг Go 1.27 + golangci-lint 2.15). Зеркало — GitHub
issue #187, PR закрывает `Fixes #187`.

## Current State

`go test ./internal/include` на Windows (GOOS=windows, реальная ФС)
падает в 5 тестах; на Linux CI — зелёный, баги платформоспецифичные:

1. `TestResolver_EscapeBeyondRootMissing` — **продуктовый баг**:
   `withinRoot` (internal/include/resolver.go) проверял
   `strings.HasPrefix(rel, "../")`, а `filepath.Rel` на Windows
   возвращает `..\`-префикс — `#include ../outside.rms` резолвится за
   пределами корня, инвариант «включения не покидают workspace»
   нарушен. Затрагивает релизные windows/amd64-бинарники.
2. `TestResolver_TwoSpellingsOwnTargetURI` — тестовый артефакт:
   dotted-URI строился как `file://` + `filepath.ToSlash(dir)`, на
   Windows диск оказывается в позиции host (`file://C:/...` → FsPath
   даёт `\\C:\...`).
3. `TestResolver_SetRootsFallback` (2 места) и
   `TestResolver_SetRootsOwnerDirWins` — тестовый артефакт:
   `assert.Contains(Resolved.Target, rawPath)` сравнивает URI-строку
   (`file:///c%3A/...`) с сырым дисковым путём (`C:\...`) — на Linux
   совпадало случайно.
4. `TestResolver_Drop` — тестовый артефакт: ключи дискового кэша
   формируются из `FsPath` (диск в lowercase: `c:\...`), а Drop
   вызывался с сырым путём (`C:\...`) — на Windows-ФС регистр диска
   различается.

## Description

Починить виндовую корректность ячейки include:

1. **withinRoot** — проверку эскейпа через `filepath.IsLocal(rel)`
   (stdlib, platform-aware: `..`/`..\x`/`../x`/абсолютные — не local);
   коммент «почему» с упоминанием виндового сепаратора
2. **Тесты** — портабельные конструкции: dotted-URI сплайсом в
   каноническую форму; сравнение Targets с `uri.File(root).String()`;
   Drop-пути через `uri.File(...).FsPath()`
3. Юнит-тест `TestWithinRoot` на таблице (внутри корня, сам корень,
   прямые/вложенные эскейпы через `filepath.Join` — сепаратор
   платформенный, абсолют извне)

## Scope

**In scope:**

- internal/include: `withinRoot` (resolver.go), resolver_test.go
- ветка `task/fix-include-within-root-windows` → PR; зелёные проверки
  (fmt/build/test/lint локально; race, corpus, fuzz — в CI)

**Out of scope:**

- нормализация регистра путей в дисковом кэше (NTFS
  case-insensitivity как продуктовая тема) — отдельно, если всплывёт
- прочие ячейки: sweep `../`-паттернов по internal/ других совпадений
  не нашёл

## Result

- `withinRoot`: `filepath.IsLocal` вместо ручного `../`-префикса
  (resolver.go); юнит-тест `TestWithinRoot` (7 кейсов)
- resolver_test.go: портабельные dotted-URI, URI-сравнения, Drop через
  FsPath
- Локально на Windows: `go test ./...` — все пакеты ok (впервые на
  этой машине); fmt/build/lint зелёные
- goga lint/contract на этой машине не ставились (решение пользователя
  2026-11-18); goga-проверки выполняются в контейнерном окружении
