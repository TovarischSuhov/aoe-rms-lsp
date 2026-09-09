# Автозагрузка бинарника aoe2-lsp расширением VS Code

## Current State

Расширение `editors/vscode` (TypeScript, `vscode-languageclient` 9.0.1,
esbuild-бандл) запускает сервер из настройки `aoe2lsp.serverPath` (дефолт
`aoe2-lsp` → резолв по PATH) и принципиально не доставляет бинарник —
MVP-граница зафиксирована в `.goga/usages/cooks/vscode-extension.md`:
пользователь сам собирает `go build ./cmd/aoe2-lsp` или ставит релизный
бинарник руками. Отсутствие бинарника — `CloseAction.DoNotRestart` + одна
`showErrorMessage`.

Релизная инфраструктура при этом готова: `release.yml` по тегу `v*`
выкладывает в GitHub Release (`TovarischSuhov/aoe-rms-lsp`) односайловые
платформенные архивы `aoe2-lsp-<os>-<arch>` (windows/amd64 `.zip`;
linux/amd64, darwin/amd64, darwin/arm64 `.tar.gz`; бинарник лежит в корне
архива) + сводный `SHA256SUMS`. Ручная установка остаётся барьером входа
(issue #37 — живой пример страданий Windows-пользователя).

## Description

Нулевая конфигурация после установки расширения: при активации расширение
молчаливо резолвит сервер по приоритету «явный `serverPath` → кэш в
globalStorage → PATH → скачивание latest-релиза с GitHub Releases» и при
необходимости скачивает, проверяет SHA256, распаковывает системным `tar`,
кэширует версионированно и стартует клиента. Скачивание сопровождается
индикатором прогресса; любые сетевые неудачи деградируют до PATH-fallback
с понятным сообщением, активация никогда не блокируется.

## Scope

**In scope:**
- порядок резолва: явно заданный `serverPath` (не дефолт) → кэш
  globalStorage (`<storage>/servers/<tag>/aoe2-lsp[.exe]`) → бинарник в
  PATH → скачивание
- скачивание: GitHub Releases API (`releases/latest`) репо
  `TovarischSuhov/aoe-rms-lsp`, выбор asset по os/arch, сверка SHA256 со
  `SHA256SUMS`, распаковка системным `tar` (child_process; bsdtar в
  Windows 10+ читает и `.zip`, и `.tar.gz`), `chmod +x` на unix
- режимы: `aoe2lsp.download.mode = "auto" | "off"` (off — никаких сетевых
  вызовов); одна сетевая проверка на активацию, при смене tag — обновление
  и чистка устаревших версий в кэше
- атомарность: download во временную директорию → rename в
  `<storage>/servers/<tag>/`; гонка нескольких окон активации не ломает
  состояние
- асинхронная активация: клиент стартует после резолва;
  `window.withProgress` на время скачивания
- целевой API нового модуля `src/install.ts` (TypeScript):

```ts
export type DownloadMode = "auto" | "off";

export interface ServerResolution {
  command: string; // команда для LanguageClient
  origin: "setting" | "cache" | "path" | "download";
}

// Резолв по приоритету setting → cache → path → (mode=auto) download.
export async function resolveServer(
  context: vscode.ExtensionContext,
  mode: DownloadMode,
): Promise<ServerResolution>;

export interface ReleaseAsset {
  tagName: string; // напр. "v0.3.0"
  archiveUrl: string; // browser_download_url для os/arch
}

// Latest-релиз → asset под платформу; ошибка на неподдерживаемой платформе.
export async function latestReleaseAsset(
  platform: NodeJS.Platform,
  arch: string,
): Promise<ReleaseAsset>;

// Скачивание + SHA256 + распаковка в globalStorage/servers/<tag>/;
// возвращает путь к бинарнику; rename-атомарно, при неудаче — чистый откат.
export async function installFromRelease(
  context: vscode.ExtensionContext,
  asset: ReleaseAsset,
  progress?: (fraction: number) => void,
): Promise<string>;
```

- `package.json`: настройка `aoe2lsp.download.mode`, обновление описания
  `aoe2lsp.serverPath` (теперь опционален)
- README, секция Editor setup: сервер ставится автоматически, `serverPath`
  нужен только для override
- cook `.goga/usages/cooks/vscode-extension.md`: убрать правило «the
  extension does NOT bundle or download it (MVP scope)», добавить секцию
  Server auto-download (резолв-порядок, режимы, layout кэша, отказоустойчивость,
  прокси-заметка `http.proxySupport`), дополнить Manual acceptance сценарием
  «чистая машина без бинарника»

**Out of scope:**
- бандлинг бинарника внутрь `.vsix` (размер, платформенная матрица)
- Go-код и ячейки `internal/*` — не затрагиваются вообще
- issue #37 (no-op `-stdio` флаг в `cmd/aoe2-lsp/main.go` для чужих
  клиентов) — отдельная задача `task/fix-stdio-flag`
- linux/arm64 в релизной матрице (если понадобится — отдельно)
- публикация расширения (подзадача #7 эпика v1.0.0)
- пин конкретной версии сервера (кроме PATH/serverPath override)
- автодетект установленной игры (AoE2 DE) и проброс её include-корня
  (`ai-rms`) в `aoe2lsp.includeRoots` — отдельный кандидат 1.x,
  см. «Out of scope» в `v1-lsp-completeness/task.md`

## Acceptance Criteria

- чистая машина (нет `serverPath`, нет бинарника в PATH, mode `auto`):
  активация скачивает latest, клиент стартует, hover/completion работают
- явно заданный `serverPath` → запускается он, сетевых вызовов нет
- mode `off` → ноль сетевых вызовов при любой конфигурации
- повторная активация с кэшем → кэш используется без скачивания; вышедший
  новый релиз → обновление + чистка старых `<tag>`-директорий
- SHA256-несовпадение / битый архив / недоступен API / rate-limit →
  частичных состояний не остаётся, сообщение объясняет, что произошло,
  предпринимается PATH-fallback
- неподдерживаемая платформа (например linux/arm64) → внятное сообщение,
  PATH-fallback
- `npm run check` (tsc) зелёный; `make check` зелёный (Go не менялся —
  регрессий быть не должно)
- cook `vscode-extension.md` и README обновлены

## Stack

- **Frameworks:** TypeScript 5.6.3 + esbuild (существующие,
  `editors/vscode`); VS Code Extension API (`globalStorageUri`,
  `window.withProgress`)
- **Libraries:** Node stdlib — `fetch`, `node:fs/promises`, `node:crypto`
  (SHA256), `node:child_process` (системный `tar`); новых npm-зависимостей
  ноль
- **Infrastructure:** GitHub Releases API репо `TovarischSuhov/aoe-rms-lsp`
  (существующие релизы, ничего на серверной стороне не меняется)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| GitHub Releases API (latest → asset, скачивание) | `.goga/usages/cooks/vscode-extension.md` | update (новая секция Server auto-download) |
| Системный tar (распаковка .zip/.tar.gz) | `.goga/usages/cooks/vscode-extension.md` | update (заметка в секции) |
| Прокси VS Code (`http.proxySupport`) | `.goga/usages/cooks/vscode-extension.md` | update (заметка в секции) |

## Risks and Constraints

- **Rate limit GitHub API** (60 req/h на IP без авторизации): одна
  проверка на активацию приемлема; 403/rate-limit → работать из кэша без
  сообщений-ошибок
- **Прокси-среда:** `http.proxySupport` патчит fetch extension host'ом —
  поведение проверить на design-этапе; worst case задокументировать
  ограничение в cook
- **Гонка окон:** несколько окон VS Code могут активировать расширение
  одновременно — rename-атомарность установки, существующая `<tag>`-директория
  не считается ошибкой
- **Windows tar:** bsdtar входит в Windows 10 1803+; движок `^1.90`
  гарантирует совместимое окружение, но отказ spawn обрабатывать сообщением
- **linux/arm64 отсутствует в матрице релиза** → отдельная ветка
  «неподдерживаемая платформа» с fallback
- **Расширение не опубликовано** (Marketplace — подзадача #7 эпика):
  тестируется локальной установкой `.vsix`
- Скачивание меняет UX «внешнего сервера»: `vscode-extension.md` —
  единственный источник правды про client bootstrap, cook обязан идти в
  том же PR

## Scope Estimate

Одна задача (~300–450 строк TS, ноль Go, ноль новых зависимостей), одна
функциональная область (доставка бинарника). Ветка
`task/vscode-autodownload` → PR. Декомпозиция не требуется.

## Existing Architecture

- `editors/vscode/src/extension.ts` — точка интеграции: активация
  становится асинхронной (резолв → старт клиента); новый модуль
  `src/install.ts` несёт всю логику доставки
- `editors/vscode/package.json` — contributes.configuration (новая
  настройка + описания)
- CODEMANIFEST-ячейки не затрагиваются: у `editors/vscode` нет контракта
  (editor-side потребитель, см. cook)
- Потребляет существующие артефакты `release.yml` (архивы, SHA256SUMS) —
  без изменений CI

## Notes

Решения сессии (2026-09-09):

- Режим загрузки — молчаливый (`auto`), с прогрессом; `off` отключает
  сеть полностью. Спрашивать разрешения у пользователя не будем
- Распаковка — системный `tar` через child_process: ноль npm-зависимостей,
  философия stdlib-first; bsdtar на Windows 10+ покрывает оба формата
- Целевой API зафиксирован TS-сигнатурами в Scope (выше)
- Issue #37 сознательно вынесен в отдельную задачу: это совместимость
  сервера с чужими клиентами (`-stdio`), к автозагрузке отношения не
  имеет — наш клиент флаг не передаёт
- Cook-обновление `vscode-extension.md` утверждено по содержанию
  (см. Scope, последний пункт in-scope)
- Приоритет: кандидат 1.x, вне эпика v1.0.0; хорошо сочетается с
  подзадачей #7 (публикация) — zero-config первое впечатление
  установленного расширения
