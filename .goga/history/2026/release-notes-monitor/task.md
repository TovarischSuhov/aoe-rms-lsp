# release-notes-monitor: ежедневный детект патч-постов AoE2 DE

## Current State

Процесс обновления kb под патч существует и смержен (#57, PR #77):
`docs/kb-refresh.md` («день патча»), `cmd/kbgen -diff`. Но триггер «вышел
патч» ручной: `docs/ref/aoe2de-xs-rms-changelog.md` собран 2026-09-04
просмотром 77 постов ageofempires.com/news руками; регламент пополнения —
в шапке этого файла. Автоматического мониторинга нет вовсе (#61 открыт).

Источник проверен 2026-09-16: `https://www.ageofempires.com/news/feed/`
— HTTP 200, валидный RSS 2.0 (WordPress-фид). Особенности: в фиде только
**5 последних постов на страницу** → детектору нужна пагинация
(`?paged=2…`) до последнего обработанного поста; фид общий для всех игр
 AoE (в сниппете виден пост AoE3) → фильтр по заголовку обязателен
(AoE2/II + Update/Minor Update/Hotfix/Update Preview — тот же набор, что
в регламенте changelog-дока).

## Description

Scheduled-воркфлоу раз в сутки проверяет фид новостей на новые
патч-посты AoE2 DE; новые посты → ровно один issue с label `kb-refresh`
(ссылки, даты, чеклист процесса), который триггерит ручной kb-refresh.
Детектор — Go-инструмент: новая ячейка `internal/news` + тонкий
`cmd/newscheck`; парсинг только RSS (фид подтверждён живым), при
недоступности/поломке источника — громкий отказ (non-zero), тихий
пропуск патчей запрещён.

## Scope

**In scope:**

- ячейка `internal/news` (brainstorm → CODEMANIFEST → apply, по образцу
  highlight/corpus): фетч RSS с пагинацией до last-seen (с потолком
  страниц), парсинг `encoding/xml`, детект патч-постов AoE2 по заголовку
  (regexp), чтение/запись state-файла
- `cmd/newscheck` — тонкий entrypoint (флаги: `-state`, при необходимости
  `-feed`); контракт вывода: новые посты в машиночитаемом виде (JSON в
  stdout) для шага issue-создания; коды выхода: 0 — прогон успешен
  (в т.ч. «новых нет»), ≠0 — источник не удалось получить/распарсить
- `.github/workflows/kb-monitor.yml`: `schedule` (cron раз в сутки,
  решение пользователя 2026-09-16) + `workflow_dispatch`; permissions
  `contents: write` + `issues: write`; шаги: checkout → setup-go →
  детектор → при новых постах `gh label create kb-refresh`
  (идемпотентно) + **один** `gh issue create` на батч постов → коммит
  state и пуш в master (при гонке — pull --rebase и ретрай один раз,
  дальше красный)
- state-файл рядом с changelog: `docs/ref/.newscheck-state.json`
  (последний обработанный пост: URL + pubDate), воркфлоу коммитит обратно
- дедуп: state + дополнительно пропуск постов, чей URL уже есть в
  открытых issues с label `kb-refresh` (`gh issue list --label`)
- тесты: фикстуры фида в `internal/news/testdata` (страница 1 без
  last-seen → пагинация; AoE3/non-patch посты отфильтрованы; битый XML),
  table-driven, memory cap

**Out of scope:**

- HTML/regexp-фолбэк парсинга сайта — осознанно выкинут при
  формулировке (2026-09-16, решение пользователя): RSS подтверждён,
  при смерти фида — громкий отказ
- автопарсинг XS/RMS-изменений из поста и автогенерация KB — ручной
  процесс #57 (issue только триггерит)
- обновление внешних источников (UGC Guide, Zetnus) — #57
- доставка бинарника (#52/#49), kb-live-update (#62)

## Acceptance Criteria

- `workflow_dispatch` на актуальном состоянии: issue не создаётся,
  state не меняется, воркфлоу зелёный
- симуляция нового поста (state временно откатан на более старый):
  создаётся ровно один issue по шаблону со всеми более новыми постами,
  label `kb-refresh`, state обновлён и закоммичен
- детектор: недоступность/битый фид → non-zero, воркфлоу красный
  (тихая деградация запрещена)
- пагинация: фикстура, где last-seen на странице 2 → посты страницы 1
  детектированы
- фильтр: AoE3-посты и не-патч-посты не попадают в issue (фикстуры)
- `make check` зелёный; `goga contract news` зелёный

## Stack

- **Frameworks:** Go 1.26+ (stdlib: `net/http`, `encoding/xml`,
  `regexp`, `flag`)
- **Libraries:** без новых зависимостей
- **Infrastructure:** GitHub Actions (schedule + workflow_dispatch,
  `gh` CLI, пуш в master с rebase-guard)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| GitHub Actions scheduled-monitor | `.goga/usages/cooks/github-actions.md` | update (секция scheduled-monitor — в реализации, как расписано в пачке kb-freshness) |
| ageofempires.com/news/feed/ (RSS 2.0) | — | внешний источник; проверен 2026-09-16: HTTP 200, 5 айтемов/страница |

## Risks and Constraints

- вёрстка/фид могут измениться или умереть — падение loudly по дизайну;
  HTML-фолбэк осознанно не строим (вернуться осознанно, если случится)
- серия hotfix'ов за один прогон — один issue на батч (заложено)
- гонка пуша state в master — pull --rebase + один ретрай, иначе красный
- cron-триггеры не работают в форках — норм для upstream
- фид общий на все игры AoE и короткий (5 постов) — фильтр по заголовку
  и пагинация обязательны; «не нашли last-seen даже после потолка
  страниц» — не «всё новое», а отдельный случай: источник мог потерять
  историю → громкий отказ (семантику зафиксировать на design-этапе)

## Scope Estimate

Одна задача: ветка `task/release-notes-monitor` → один PR (Fixes #61).
Объём S–M: ячейка с 2–3 контрактами + тонкий cmd + workflow + тесты.

## Existing Architecture

- новая ячейка `internal/news` не зависит от других ячеек (чистый
  листок, как kb); по образцу kbgen: логика в ячейке, `cmd/newscheck`
  тонкий
- workflow по паттернам cook `github-actions.md` (least privilege,
  timeout-minutes, first-party actions)
- issue-шаблон и чеклист — из `docs/kb-refresh.md`; регламент заголовков
  патч-постов — шапка `docs/ref/aoe2de-xs-rms-changelog.md`

## Notes

- Решения сессии (2026-09-16): каденция — раз в сутки (пользователь);
  парсер — только RSS, HTML-фолбэк из формулировки пачки выкинут
  (пользователь); каденция/имена (`internal/news`, `cmd/newscheck`,
  `kb-monitor.yml`) предложены и не оспорены
- Слот пачки kb-freshness (`2026/task-kb-freshness/task.md`), зеркало —
  issue #61 (существует, новое не нужно)
- Реальная проверка фида при формулировке: curl → 200, RSS 2.0,
  5 айтемов, заголовок «Age of Empires III: Definitive Edition – Update
  19.18309» подтверждает формат патч-постов
- Первый прогон на живом фиде может создать issue по последнему
  непатчнутому посту — начальный state заполнить актуальным последним
  патч-постом из changelog-дока при реализации
