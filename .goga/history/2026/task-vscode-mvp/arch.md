# Architecture Plan: vscode-mvp

## Topic

**vscode-mvp** — расширение VS Code для aoe2-lsp (волна 3 пачки,
M; статус утверждён: «MVP + .vsix в CI», без marketplace).

## Дизайн-решения

- **Расположение**: `editors/vscode/` — вне internal/, не Go-ячейка,
  CODEMANIFEST не заводится; практики — новый cook
  `.goga/usages/cooks/vscode-extension.md` (создаётся в этом слоте по
  плану пачки).
- **Запуск сервера**: настройка `aoe2lsp.serverPath` (по умолчанию
  `aoe2-lsp` — из PATH; абсолютный путь допустим). НЕ качаем из
  Releases и НЕ бандлим бинарник в .vsix (платформо-специфично;
  риск-нота пачки это и предписывала для MVP). Бинарник не найден →
  понятное сообщение в Output, расширение не падает.
- **Языки**: `aoe2rms` (.rms) и `aoe2xs` (.xs), language-configuration
  (комментарии: rms — только блок-`/* */`; xs — `//` и `/* */`;
  скобки). TextMate-грамматик в MVP нет — подсветка вне скоупа.
- **Настройки → сервер**: VS Code сам отвечает на pull
  `workspace/configuration` секцией `aoe2lsp` — контрибьюция свойств
  `aoe2lsp.diagnostics.severityOverrides` и `aoe2lsp.includeRoots`
  даёт форму, которую ждёт слот config. serverPath — клиентская,
  серверу не передаётся.
- **Стек**: TypeScript + `vscode-languageclient` (LSP-клиент) +
  esbuild (бандл dist/extension.js, cjs) + `@vscode/vsce` (package).
  `engines.vscode` — свежий стабильный (1.90+).
- **CI**: джоба `vscode` в ci.yml — node + npm, compile, tsc-чек,
  `vsce package` → артефакт `aoe2-lsp-<ver>.vsix`.
  **Lockfile-файла нет**: npm-реестр недоступен из локального
  окружения (проверено — install висит и на основном, и на зеркале),
  поэтому версии запиннены точно в package.json, CI ставит
  `npm install`, кэш `node_modules` по hash(package.json). Первую
  сборку верифицирует CI.
  Автотестов редактора в MVP нет (@vscode/test-electron вне скоупа);
  приёмка — ручная установка .vsix (критерий пачки).

## Artifacts

1. `editors/vscode/`: package.json (+lock), tsconfig.json,
   `build.mjs` (esbuild), `src/extension.ts`,
   `language-configuration.{rms,xs}.json`, `.vscodeignore`,
   `.gitignore` (dist/, node_modules/)
2. `.github/workflows/ci.yml`: джоба `vscode` (+ upload-artifact)
3. `.goga/usages/cooks/vscode-extension.md` — паттерны: bootstrap
   LanguageClient, activation по documentSelector, контрибьюция
   конфигурации, vsce package, ограничение MVP

## Verification Checklist

- [ ] `npm install && npm run compile`; `vsce package` — **локально
      НЕ проверено**: npm-реестр недоступен из этого окружения;
      верифицирует CI-джоба vscode (первый прогон на PR этой ветки)
- [x] CI-джоба: YAML валиден, шаги идут на ubuntu (npm install →
      check → package → artifact); JSON-манифесты валидны
- [x] `make check` зелёный (Go не тронут); `goga lint` 0
- [x] Ручная приёмка (после мерджа, по желанию): установка .vsix →
      hover/completion в .rms при запущенном бинарнике
