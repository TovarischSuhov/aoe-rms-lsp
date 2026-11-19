# Полный аудит репозитория — 2026-11-19

- **Репозиторий**: aoe-rms-lsp (origin `github.com:TovarischSuhov/aoe-rms-lsp`)
- **SHA**: `f3762010822015326a6b85c493561b761a04c131` (master, дерево чистое, fresh-check пройден)
- **Режим**: полный аудит всего дерева (не дельта ветки), 4 параллельных ревьюера по группам модулей
- **Трекер находок**: issues GitHub (пользователь подтвердил: репо хостится на GitHub, не GitLab)
- **Вердикт**: **CHANGES** — 4 CRITICAL, 13 WARNING, блокирующих «стоп-сборку» нет; аудит read-only, сборка/тесты не запускались

## Сводка находок

| # | Серьёзность | Где | Суть |
|---|---|---|---|
| 1 | CRITICAL | internal/include/resolver.go:832 | `nameAt` режет сырой текст по CRLF-нормализованным офсетам + теряет регистр секционных слов — References/Definition молча пусты (подтверждено 3 ревьюерами независимо) |
| 2 | CRITICAL | internal/server/server.go:1323 | `renameToAction` — тот же класс: срез сырого текста нормализованными офсетами; на CRLF-картах квикфикс «Rename to …» мёртв или предлагает чужое имя |
| 3 | CRITICAL | internal/server/server.go:1763 | `SemanticTokensFull` для .rms склеивает несортированные токены inline-XS-блоков → uint32 delta-underflow, битая подсветка |
| 4 | CRITICAL | knowledge/index.md:10-15 | В master закоммичены неразрешённые merge-конфликт-маркеры (внесены merge f0bc631) |
| 5 | WARNING | internal/format/xs.go:2435 | `stmtDo` печатает ` while` с ведущим пробелом при безскобочном теле — битый канонический вывод |
| 6 | WARNING | internal/rms/parse.go:1019 | `blankComments` — O(L²) аллокаций на строку блок-комментария; Parse на каждый keystroke |
| 7 | WARNING | internal/include/resolver.go:365 | `definitionInRms` резолвит include только по каталогу владельца — go-to-definition молча не работает для root/..-целей |
| 8 | WARNING | internal/include/resolver.go:233 | Открытые файлы перезапарсиваются на каждый keystroke, замыкание без мемоизации |
| 9 | WARNING | internal/server/server.go:94 | `applySettings` не разворачивает обёртку `{"aoe2lsp": …}` — neovim/emacs клиенты молча сбрасывают настройки |
| 10 | WARNING | internal/server/server.go:461 | Паника в request-хендлере закрывает соединение (jsonrpc2 `fail`), `serve` глушит ошибку — процесс умирает с кодом 0 |
| 11 | WARNING | internal/include/resolver.go:269 | Кэш дисковых файлов растёт без вытеснения (RSS растёт монотонно в длинной сессии) |
| 12 | WARNING | internal/kb/diff.go:224 | `DiffKB` не сравнивает `renamed_to` — ренеймы проходят kb-refresh без ревью |
| 13 | WARNING | cmd/aoe2-lsp/main.go:50 | exit без shutdown даёт код 0; после Shutdown запросы выполняются вместо InvalidRequest |
| 14 | WARNING | Makefile:17-22 | `make corpus` локально сломан: `go build ./...` не пишет бинарник, `filepath.Abs` убивает PATH-lookup |
| 15 | WARNING | .github/workflows/ci.yml:64-71 | Lockfile закоммичен, но CI на «No lockfile»-контракте: `npm install` вместо `npm ci`, кэш по package.json |
| 16 | WARNING | README.md:102 | Устаревшие числа KB: 204 функции/27 секций vs фактические 291/1170 |
| 17 | WARNING | scripts/check-vsix.sh:20-27 | Гейт не проверяет walkthrough media paths (добавлены позже скрипта) |

## Non-blocking (наблюдения, не заводились отдельными issues)

- internal/rms/parse.go:328-345 — `cmd { attr 1` (незакрытый однострочный блок) поглощает `{` без диагностики — асимметрия с духом #96
- hints/computer.go:89 — `activeParameter` без clamp уходит за конец Params
- server.go:1768 — токены inline-блоков считаются с `externals=nil`, имена из include-файлов подсвечиваются «unknown»
- server.go:249 / resolver.go:302 — `Drop` передаёт неканонический путь, ключи кэша — `filepath.Clean`
- `lineOf` O(n) на вызов — References/Rename квадратичны по позициям
- complete/completer.go:251 — дубликаты кандидатов completion
- corpus/runner.go:731 — `truncate` может резать посреди руны
- `.vscodeignore` не исключает package-lock.json (~110 КБ в .vsix)
- `aoe2-lsp -version` (однодефисный) трактуется как unknown flag — сервер стартует и «подвешивает» ожидавшего
- README «Project layout» не перечисляет `internal/highlight` (13-я ячейка)
- docs/plans/build-lsp-rms-xs.md хранит устаревшие «204» — исторический документ, править не требуется

## Вердикты ревьюеров (дословно)

### Группа 1: internal/rms, internal/xs, internal/format

**VERDICT: CHANGES**

1. **[internal/server/server.go:1323] CRITICAL** — `renameToAction` (quickfix «Rename to …», коммит baa40b0 #134 от 2026-11-19) режет слово из **сырого** текста редактора по `r.Start.Offset:r.End.Offset`, хотя для `.rms` эти офсеты посчитаны `rms.Parse` после CRLF-нормализации. Файл сам содержит хелпер ровно для этого — `siteText` (server.go:1108, «counts offsets after CRLF normalization… never against the raw editor state alone»), который здесь не использован. Сценарий отказа: CRLF-карта (норма для AoE2 RMS с Windows), курсор на диагностике устаревшего символа в строке N → срез уезжает на N−1 байт влево, в текст предыдущей строки → `store.RenamedTo(word)` в большинстве случаев молча не находит nothing (фича мертва на CRLF), а при совпадении сдвинувшихся байтов с другим переименованным токеном — предлагается и применяется замена **чужого** имени к правильному диапазону настоящего символа: «Rename to <не то>» молча переименовывает символ пользователя. Фикс: заменить срез на `siteText(text, name, r)`.
2. **[internal/include/resolver.go:832] CRITICAL** — тот же класс в `nameAt` (используется `Resolver.References`, строка 413): диапазоны из `f.rms.ReferencesAt(pos)` (нормализованные офсеты) режутся из `f.text` — сырых байтов дискового файла; собственный хелпер `siteText(f, rr)` (resolver.go:637, с тем же докомментом про нормализацию) не использован. Сценарий отказа: CRLF `.rms`-файл (текстовый include) в замыкании → имя под курсором вырезается со сдвигом на число CRLF до позиции → cross-file References/переименование ищет не тот символ: пустой результат (фича молча мертва) или вхождения чужого имени. Фикс: `return siteText(f, ranges[0])`.
3. **[internal/format/xs.go:2435] WARNING** — `stmtDo` печатает закрывающую строку как `closer+" while ("+cond+");"`: при безскобочном теле `branch` возвращает false, `closer == ""`, и строка выходит с ведущим пробелом (`p.at(level)+" while…"`). Сценарий отказа: `void A(){do x(); while(c);}` форматируется в канонический вывод с битым отступом (` while` — 5 пробелов вместо 4); идемпотентность сохраняется, поэтому инвариант-тесты и goldens дефект не ловят — раскладка не закреплена. История: c0bca03 (2026-10-14, исходный XS-принтер). Фикс: собирать строку закрытия условно, добавить golden на безскобочный `do x(); while(c);`.
4. **[internal/rms/parse.go:1019] WARNING** — `blankComments` внутри блок-комментария заменяет байты по одному: `line = line[:j] + " " + line[j+1:]` — новая строка-аллокация на каждый байт, O(L²) на строку. Сценарий отказа: `.rms` с длинной строкой внутри `/* … */` (L ≈ 100 КБ) → ~5·10⁹ байт копирования на один вызов `Parse`, а `Parse` выполняется на каждый keystroke — LSP-фриз на секунды, при L ≈ 1 МБ — минуты. Тот же код в `internal/format/rms.go:202` (`blankCode`) делает это за O(L) по `[]byte`. История: c3deffa (2026-09-04). Фикс: переписать скан на `[]byte` по месту, как в `blankCode`.

Проверено: recovery-пути парсеров rms/xs полностью, сканер XS, принтеры форматтеров, verbatim/warn-вырезы, anchoring комментариев, потребители контрактов (`server.go`, `include/resolver.go` — все срезы по Offset), конвенции (`_`-сбросов нет, файлового I/O в ячейках нет). Вне охвата: запуск тестов/фаззера (запрещён брифом), прочие группы.

### Группа 2: internal/analysis, complete, highlight, hints, include

**VERDICT: CHANGES**

1. [internal/include/resolver.go:832] CRITICAL — `nameAt` режет raw-текст по CRLF-нормализованным офсетам и теряет регистр секционных слов (word-индекс хранит lowercase, rms/parse.go:941, а слайс даёт исходный регистр) — References на обычных словах RMS-файла и на именах секций молча возвращают пусто/мусор. 7 из 100 карт корпуса — CRLF. Фикс: нормализовать текст перед слайсом (зеркало siteText) и/или брать имя из word-индекса; добавить CRLF-тест (в resolver_test.go `\r` нет вообще).
2. [internal/include/resolver.go:365-372] WARNING — `definitionInRms` строит только `filepath.Join(filepath.Dir(path), inc.Path)` и отбрасывает по `withinRoot(filepath.Dir(path), …)`, тогда как `expandDirectives` использует `resolveTarget` (владелец → все roots). Сценарий: диагностика показывает include как Resolved, но Ctrl+Click молча даёт found=false. Фикс: использовать `r.resolveTarget`.
3. [internal/include/resolver.go:233-239] WARNING (перф, горячий путь) — editor-state файлы перезапарсиваются при каждом `load`, замыкание пересчитывается на каждое нажатие клавиши (`publishDiagnostics` строит Closure на каждый DidChange, References/RenameSites — по одному замыканию на каждый открытый документ). Фикс: мемоизация AST по (uri, version) — DocStore уже носит version, либо кэш замыкания.

Проверено: все нетестовые файлы группы полностью, отказные сценарии (циклы include — задокументированное решение, битые файлы, гонки, инвалидация, panic-кандидаты, error-handling), анализ на ложные/пропущенные диагностики, UTF-16 конверсии. Вне охвата: rms/xs/server/kb/format (группы других ревьюеров), запуск тестов.

### Группа 3: internal/server, kb, news, corpus, common

**VERDICT: CHANGES**

1. [internal/server/server.go:1763] CRITICAL — `SemanticTokensFull` для .rms склеивает `TokensRms` (отсортирован) с токенами inline-XS-блоков, дописанными в конец, и передаёт в `encodeSemanticTokens`, который не сортирует и считает вход «position-sorted»; после первого токена блока `dLine := start.Line - prevLine` в uint32 уходит в underflow → битый delta-энкодинг на каждом запросе к такому документу (каноническая фикстура `internal/rms/testdata/includes.rms` воспроизводит). Фикс: слить и отсортировать токены по позиции перед `encodeSemanticTokens`, плюс пин-тест.
2. [internal/include/resolver.go:832] WARNING — `nameAt` нарезает сырой `f.text` офсетами CRLF-нормализованного текста (см. группу 2).
3. [internal/server/server.go:1323] WARNING — `renameToAction` повторяет тот же класс на сыром тексте редактора (см. группу 1).
4. [internal/server/server.go:94] WARNING — `applySettings` трактует payload `didChangeConfiguration` как само содержимое секции `aoe2lsp`; клиент со стандартной обёрткой `{"aoe2lsp": {...}}` (neovim/lspconfig, emacs-lsp) даёт пустой `settingsJSON` → severityOverrides сброшены, include-roots очищены, у файлов внезапно missing-include — до перезапуска сервера. Фикс: разворачивать обёртку секции, тест на обёрнутый payload.
5. [internal/server/server.go:461] WARNING — комментарий `recoverNotification` утверждает, что паника в request-хендлерах «already answers with an error response»; в go.lsp.dev/jsonrpc2@v1.0.1 recover отправляет InternalError, но затем вызывает `c.fail`, который закрывает весь stream; `serve.go:47-50` читает только `conn.Done()` и игнорирует ошибку → процесс молча выходит с кодом 0. Фикс: обернуть request-хендлеры тем же recover-паттерном либо исправить комментарий и логировать fail-ошибку.
6. [internal/include/resolver.go:269] WARNING — кэш дисковых файлов `r.cache` растёт без вытеснения (чистка только по watcher-событиям), `Available` кэширует все .xs каталога и всех roots при каждом completion → RSS растёт монотонно в длинной сессии. Фикс: LRU/лимит.
7. [internal/kb/diff.go:224] WARNING — `DiffKB` не сравнивает поле `renamed_to` ни в одной из трёх сущностей (поле появилось в #134, дифф-утилита из #57 не обновлена) → регламент kb-refresh рапортует «No changes» при переименовании функции, ренейм-маркер сливается без ревью. Фикс: добавить сравнение `RenamedTo` во все три diff-функции + тест.
8. [cmd/aoe2-lsp/main.go:50] WARNING — «exit» без предварительного «shutdown» даёт процессу код 0 (LSP-спека требует ненулевой); после Shutdown сервер продолжает полноценно обрабатывать запросы вместо InvalidRequest. Фикс: atomic.Bool факт Shutdown, ошибка/код при exit-без-shutdown, InvalidRequest после Shutdown.

Проверено: server.go целиком, serve.go, docstore.go, include (замыкание, кэш, escape-guard), kb (store/model/mine/diff/gen), news, corpus, common, cmd/aoe2-lsp, библиотеки jsonrpc2/protocol/uri, VS Code-клиент. Инварианты stdout-протокола/без-сети/стабильных-кодов — соблюдены. Вне охвата: глубокий разбор rms/xs/format/analysis/hints/complete, хвост kb/extract.go, cmd/{corpus,kbgen,tmgen,newscheck}, запуск тестов.

### Группа 4: cmd/, editors/, scripts/, Makefile/CI, README/CHANGELOG/knowledge

**VERDICT: CHANGES**

1. **[knowledge/index.md:10-15] CRITICAL** — в master закоммичены неразрешённые merge-конфликт-маркеры (`<<<<<<< HEAD` / `=======` / `>>>>>>> origin/master`), внесены merge-коммитом f0bc631 от 2026-11-19 (подтверждено git blame); файл содержит два конфликтующих варианта строки «Инварианты стабильности» плюс строку include-drs внутри конфликтного блока. Сценарий отказа: AGENTS.md требует начинать каждую задачу с чтения knowledge/index.md — агент или читатель видит «суп» из маркеров и дублей; в CI нет ни одной проверки knowledge/, дефект проходит на каждом пуше. Фикс: разрешить конфликт (вариант origin/master: «сходимость форматтеров (RMS и XS)» + сохранить строку include-drs-resolution); добавить в CI grep-guard на конфликт-маркеры по knowledge/ и docs/.
2. **[Makefile:17-22, internal/corpus/runner.go:197] WARNING** — цель `corpus` зависит от `build` (`go build ./...`, бинарники не пишет), затем `go run ./cmd/corpus -bin aoe2-lsp` через `filepath.Abs` резолвит голое имя в `<repo>/aoe2-lsp` — PATH-lookup убит. Сценарий: свежий клон, `make corpus` — скачиваются все 100 файлов, затем `corpus run failed: executable file not found`; CI-джоба обходит дефект (строит `-o` явно), гейт локально сломан. Фикс: собрать бинарь `go build -o aoe2-lsp ./cmd/aoe2-lsp` и передать `-bin ./aoe2-lsp`.
3. **[.github/workflows/ci.yml:64-71, release.yml] WARNING** — package-lock.json закоммичен (c9e43db), но CI живёт по контракту «No lockfile» (65c7a43): `npm install` вместо `npm ci`, кэш по хэшу package.json → невоспроизводимые .vsix, lockfile-only изменения не инвалидируют кэш. Фикс: `npm ci` в обоих workflow, ключ кэша — package-lock.json, комментарий переписать.
4. **[README.md:102] WARNING** — таблица «Data sources» утверждает `204 XS functions / 27 sections`; в закоммиченных данных 291 функция и 1170 констант (тесты пинят 291: internal/kb/store_test.go:19, internal/server/serve_test.go:536). Фикс: обновить числа README.
5. **[scripts/check-vsix.sh:20-27] WARNING** — контракт скрипта «every contributes.* path exists in the packaged .vsix», но jq-фильтр покрывает только grammars/languages/snippets; walkthrough media (`contributes.walkthroughs[].steps[].media.markdown`, добавлены 2f2e6ec позже скрипта) не проверяются → битый путь media спокойно проходит гейт. Фикс: дополнить jq-фильтр.

Проверено: все 5 entrypoints cmd/, vscode-расширение (manifest, extension.ts, install.ts, gamedetect, deploy, statusbar, snippets, tmLanguage, walkthroughs), claude-code-манифесты, все 4 скрипта, Makefile, .golangci.yml, go.mod, CI-workflows, README/CHANGELOG/plans, knowledge/ (repo-wide grep на конфликтные маркеры — только index.md; все ссылки индекса существуют). Вне охвата: internal/** (кроме точки касания corpus-runner), runtime workflows на GitHub-стороне, публикация Marketplace end-to-end.

## Пост

Ревью выполнялось по запросу владельца как полный аудит репозитория (не MR — ветка master совпадает с origin/master, постить в MR нечего). По итогам заведены issues на GitHub (пользователь выбрал GitHub вместо GitLab — репо хостится там). Round: 1. Tier: CRITICAL.

| Находка | Issue |
|---|---|
| 1. nameAt CRLF (CRITICAL) | [#204](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/204) |
| 2. renameToAction CRLF (CRITICAL) | [#205](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/205) |
| 3. SemanticTokens underflow (CRITICAL) | [#206](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/206) |
| 4. knowledge/index.md конфликт-маркеры (CRITICAL) | [#207](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/207) |
| 5. xs do-while отступ | [#208](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/208) |
| 6. blankComments O(L²) | [#209](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/209) |
| 7. definitionInRms узкая резолюция | [#210](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/210) |
| 8. перезапарсивание на keystroke | [#211](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/211) |
| 9. applySettings обёртка секции | [#212](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/212) |
| 10. паника рвёт соединение | [#213](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/213) |
| 11. безлимитный кэш | [#214](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/214) |
| 12. DiffKB renamed_to | [#215](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/215) |
| 13. exit-код lifecycle | [#216](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/216) |
| 14. make corpus сломан | [#217](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/217) |
| 15. npm ci vs lockfile | [#218](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/218) |
| 16. README устаревшие числа | [#219](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/219) |
| 17. check-vsix walkthrough media | [#220](https://github.com/TovarischSuhov/aoe-rms-lsp/issues/220) |

Non-blocking наблюдения в issues не заводились — перечислены выше в одноимённой секции.

Дальше по решению владельца: исправления по одному замечанию за раз, каждое — через implement-mini в отдельном агенте, начиная с CRITICAL (#204).

## Обновление базы знаний

Изменений поведения не было (аудит read-only), обновление knowledge/ не требуется. Отчёт — docs/reviews/2026-11-19-full-review.md (этот файл).
