# Сборка vsix в релизном CI + харденинг (vsix-release-ci)

## Current State

- CI (`ci.yml`, джоба `vscode`) уже собирает `.vsix`: node 22 → `npm install`
  → `tsc --noEmit` → `vsce package` → upload артефакта `aoe2-lsp-vsix`.
  Проверок содержимого пакета нет.
- `release.yml` (тег `v*`) собирает только Go-бинарники (4 платформы +
  `SHA256SUMS`) — `.vsix` в GitHub Release не попадает. Установка
  расширения сегодня = «скачай VSIX из CI-артефакта master» (#68.7:
  в Marketplace расширения нет, 404).
- Версии рассинхронны: extension `0.1.0` в `editors/vscode/package.json`
  против сервера v0.6.0; `scripts/release.sh` про extension не знает.
- Диагностика #68 мотивирует харденинг: VSIX из master до PR #66 содержал
  `contributes.grammars` → несуществующие файлы, VS Code молча игнорирует
  битые пути (п.3); окно недоступности релиза — ассеты 404 при живом
  atom-фиде (п.1); пробелы метаданных пакета (п.4).
- В эпике v1.0.0 это «сборочная» половина подзадачи 7 «publish extension»
  (issue #47); публикация в Marketplace — отдельная часть.

## Description

Расширить релизный конвейер сборкой расширения и закрыть дыры, найденные
в #68:

1. **`release.yml` — vsix в релизе**: в релизной джобе собрать `.vsix`
   (node 22, `npm install` → `npm run check` → `npm run package`) и
   положить в `dist/` — файл попадает в `SHA256SUMS` и крепится к
   GitHub Release рядом с бинарниками.
2. **Синхронизация версий**: `scripts/release.sh` при релизе проставляет
   `version` в `editors/vscode/package.json` = версии релиза и включает
   правку в релизный коммит (как `CHANGELOG.md`). Версия в git всегда =
   последнему тегу; CI собирает без трюков.
3. **Пост-проверка доступности ассетов** (#68.1): после
   `gh release create` — анонимный HEAD-запрос каждого ассета с ретраями
   до HTTP 200; таймаут = fail джобы. Atom-фид источником истины не
   считается.
4. **CI-проверка contributes-путей** (#68.3): в джобе `vscode` после
   `vsce package` — сверить каждый путь из `contributes`
   (`grammars[].path`, `languages[].configuration`) с содержимым
   собранного архива; отсутствие файла = fail.
5. **Точечные метаданные** (#68.4): копия `LICENSE` в `editors/vscode/`
   и `license: "SEE LICENSE IN LICENSE"` (убирает warning vsce; корень
   пакета самодостаточен); `capabilities`: `virtualWorkspaces: false`
   (бинарник не запустить в vscode.dev), `untrustedWorkspaces`.

## Scope

**In scope:**
- `.github/workflows/release.yml` — сборка `.vsix`, попадание в
  `SHA256SUMS` и Release; пост-проверка анонимной доступности ассетов
- `.github/workflows/ci.yml` — джоба `vscode`: шаг проверки
  contributes-путей в собранном `.vsix`
- `scripts/release.sh` — bump `version` в `editors/vscode/package.json`
  в релизном коммите
- `editors/vscode/package.json` — `license`, `capabilities`
- `editors/vscode/LICENSE` — копия корневого LICENSE
- `.goga/usages/cooks/github-actions.md`, `.goga/usages/cooks/vscode-extension.md`
  — обновлены на этапе формулировки

**Out of scope:**
- `vsce publish` / `ovsx` — Marketplace-публикация (остаётся в #47,
  нужны `VSCE_PAT`/`OVSX_PAT`, аккаунт издателя)
- `icon`, `keywords`, `bugs`, `homepage` — листинг Marketplace (к #47)
- `allowScripts` в package.json (#68.6 — локальная сборка, не CI)
- правка текста `aoe2lsp.includeRoots` и автодетект игры (#68.5 —
  кандидат в vscode-autodetect / дока)
- автозагрузка бинарника расширением (#52)
- бандлинг сервера внутрь vsix

## Acceptance Criteria

- Пуш тега `vX.Y.Z` → GitHub Release содержит 4 архива + `.vsix` +
  `SHA256SUMS`, где сумма `.vsix` присутствует
- Релизный коммит `release.sh` содержит bump `version` в
  `editors/vscode/package.json`; имя файла = `aoe2-lsp-X.Y.Z.vsix`
- Пост-проверка: джоба падает, если ассет не отдаётся анонимно за
  отведённые ретраи (проверено хотя бы негативным сценарием — битый URL)
- Джоба `vscode` в PR падает на vsix, где путь из `contributes`
  отсутствует в архиве (проверено негативным сценарием локально или
  временной правкой package.json)
- `vsce package` не даёт warning про LICENSE
- `make check` зелёный; workflows без config-ошибок в Actions

## Stack

- **Frameworks:** GitHub Actions (`release.yml`, `ci.yml`)
- **Libraries:** без новых — node 22 + npm (`vsce`, `esbuild`, `tsc`
  уже в `editors/vscode/package.json` с точными пинами)
- **Infrastructure:** GitHub Actions runners (ubuntu-latest), GitHub
  Releases; bash (`scripts/release.sh`)

## External Dependencies

| Component    | Usage file                          | Status  |
|--------------|-------------------------------------|---------|
| GitHub Actions | `.goga/usages/cooks/github-actions.md` | updated |
| vsce / npm   | `.goga/usages/cooks/vscode-extension.md` | updated |

## Risks and Constraints

- npm-реестр недоступен локально — `.vsix` верифицирует только CI
  (джобы `vscode`/`release`); локально проверяем только YAML-валидность
- `release.yml` должен быть на `master` до тегирования — tag-событие
  запускает workflow на коммите тега
- Пост-проверка ассетов: доступность на CDN может запаздывать — ретраи
  с разумным таймаутом, чтобы не получить flaky-релизы; total budget
  уложить в `timeout-minutes` джобы
- `untrustedWorkspaces`: сервер только парсит файлы (не исполняет) —
  семантику выбрать соответственно при реализации
- Проверка contributes-путей должна читать пути из package.json, а не
  дублировать их списком в шаге — иначе проверка сама разъедется с
  фактом
- Версия в `package.json` после merge PR останется «прошторелизной»
  до следующего `release.sh` — это ожидаемо (bump живёт в релизном
  коммите)

## Scope Estimate

Одна задача: ветка `task/vsix-release-ci` → один PR (Fixes #69, зеркало
задачи).
Объём малый-средний, всё в одном инфраструктурном слое; части по
отдельности ценности почти не имеют (vsix без версии в релизе
не атрибутировать, проверки без релиза не поймают #68.1).

## Existing Architecture

Ячеек не касается (нет CODEMANIFEST-изменений). Затрагиваемые слои:
`.github/workflows/{ci,release}.yml`, `scripts/release.sh`,
`editors/vscode/{package.json,LICENSE,.vscodeignore}`. Связь с эпиком:
«сборочная» половина подзадачи 7 (#47) — после неё #47 сводится к
Marketplace-публикации (`vsce`/`ovsx`, секреты).

## Notes

- Решение (2026-09-10): версия синхронизируется через `release.sh`
  (коммитит bump), не через sed в CI — версия в git = тегу.
- Решение (2026-09-10): скоуп «релиз + харденинг», Marketplace — вне.
- `npm run package` уже прогоняет compile перед `vsce package` —
  отдельный шаг компиляции в release-джобе не нужен.
- `.vscodeignore` не исключает LICENSE — копия попадёт в пакет;
  при реализации убедиться, что это остаётся верным.
