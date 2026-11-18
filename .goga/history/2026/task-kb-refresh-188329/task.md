# kb-refresh-188329: refresh kb под патчи 188329 (+ отставший апстрим 185872)

Слот по авто-issue **#186** (Update 188329, 2026-10-27) с догрузкой
апстрима по **#132** (Update 185872, 2026-09-22): в апстриме UGC Guide
28.10.2026 приземлился коммит «Update XS docs for the 2026 October
update» + «Add missed constants» — оба патча должны войти в данные за
один refresh. Зеркало прогресса — комментарии в issues; закрывает
только полностью покрытый слот (урок #132: premature `Fixes` по
первому шагу → reopen).

## Current State

- Changelog: секция 185872 внесена (PR #141), секции 188329 нет.
  Патч 188329 по release notes: 15 новых XS-функций net-data семейства
  (`xsResetNetData`, `xsWriteNetInt/Float/Vector/String`,
  `xsSendNetData`, `xsReadNet{Int,Float,Vector,String}`,
  `xsSet/Get{Write,Read}NetDataPosition`, `xsUnsyncIsSandboxMode`),
  фикс `xsSetUnitProperty` (краш на garrisoned-in-unit id); RMS — без
  изменений.
- Апстрим UGC Guide (Divy1211/AoE2DE_UGC_Guide): локальный снапшот
  2026-07-24 (177723); свежие `functions.json`/`constants.json` —
  `docs/general/xs/{functions,constants}/`.
- Zetnus-гайд: на машине разработки отсутствует (gitignored, локальная
  копия мейнтейнера) — `rms-commands.json` остаётся как есть
  (осознанное ограничение `docs/kb-refresh.md`). RMS-фиксы 185872
  (`ATTR_*`, unit classes, `random_map.def`) ждут обновления гайда —
  вне этого слота.

## Description

1. Переложить `functions.json`/`constants.json` (+ `prelude.xs`/
   `prelude.py` для честности зеркала) из апстрима
2. Changelog: секция `## Update 188329 — 2026-10-27` по регламенту
   шапки; период/счётчик постов в шапке актуализировать
3. `go run ./cmd/kbgen -diff` → отчёт в PR; `go run ./cmd/kbgen` →
   регенерация; `since_update` новых функций — из changelog
4. Проверки: make check; корпус — локально (`make corpus`) либо
   workflow_dispatch corpus-джобы на ветке
5. Issue: закрывать только если данные покрывают патч целиком
   (#186 — кандидата на `Fixes`, если апстрим-документация 188329
   полна; #132 — Refs, ждёт Zetnus)

## Result

- Источники: апстрим UGC Guide переложен (functions/constants JSON +
  prelude.xs/py, коммит апстрима «Update XS docs for the 2026 October
  update» 2026-10-28 + «Add missed constants»); changelog — секция
  Update 188329 (15 net-data функций + фикс xsSetUnitProperty),
  шапка — период 2026-11, 79 постов
- Данные: xs-functions 204→289 (+87, −2 — ренеймы 185872), константы
  882→1170 (+288: ActionType/Annex/Attribute/ColorId/TechState/Capture
  и пр. — TVS DLC + октябрьский патч), `since_update` 188329/185872 из
  changelog; rms-commands не тронут (Zetnus отсутствует локально)
- Вынужденные спутники: пины счётчиков (kb store 204→289/27→45,
  server completion 204→289, xs prelude 884→1172 экстернов),
  регенерация TextMate-грамматики (tmgen, 1170 констант)
- Проверки: fmt/build/lint зелёные; тесты 14/15 пакетов ok (include —
  красный только на Windows, фиксялся в PR #188, не этой веткой);
  corpus gate локально: 100/100 ok, panic 0; CI-dispatch на ветке —
  corpus-джоба
- Issues: `Fixes #186` (188329 покрыт полностью: апстрим-документация
  патча вошла в данные); `Refs #132` — UGC-часть 185872 вошла, слот
  остаётся открытым до переэкспорта Zetnus-гайда (локальная копия
  мейнтейнера; RMS-фиксы 185872 — ATTR_*/unit classes/random_map.def —
  ждут его)
- Отчёт диффа — в описании PR
