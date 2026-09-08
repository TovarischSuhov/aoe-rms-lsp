# Corpus-прогон 100 опубликованных карт — 2026-09-09

Прогон `internal/corpus` (полный LSP-цикл в свежем subprocess на файл:
initialize → didOpen → диагностика → hover/signatureHelp в 8 сэмплированных
позициях → completion в 2 → documentSymbol → shutdown/exit).

## Выборка

Пул 236 скриптов из 8 публичных GitHub-репозиториев, пин по SHA в
`scripts/corpus-sources.txt`, детерминированный сэмпл 100
(`scripts/corpus-fetch.sh`, seed `aoe2-corpus-v1`). 7 ZIP-архивов формата
ZR@ (aoe2map.net «Zip Random») отсеяны по magic bytes; загруженные файлы в
репозиторий не коммитятся (лицензии upstream различаются).

Источники: HSZemi/rms, Naramsim/AoE2-random-map-scripts,
dundass/rms-playground, Gatsuca/AOE2_RMS, asuky/aoe2_rms, rohanport/aoe2rms,
Maboey/aoe2_rms, matsaraiva/AOE2-RMS.

## Итог прогона

| Метрика | До фиксов | После фиксов |
|---|---|---|
| Жёсткие отказы (panic/exit/timeout/transport) | 0 | **0** |
| Ошибки JSON-RPC среди запросов | 0 | **0** |
| Всего диагностик на 100 карт | 617 | **301** |
| Медиана диагностик на карту | 1 | **0** |
| Максимум диагностик на карту | 80 | 49 |
| Карт без единой диагностики | 39 | **73** |
| Время прогона | 3.2 с | 3.3 с |

Бинарных ZIP-файлов в выборке до фильтра парсер не падал: 6.6K
syntax-диагностик на бинарный мусор, ни одной паники (устойчивость
подтверждена).

## Найденные и исправленные дефекты парсера (rms)

Все фиксы покрыты регрессионными тестами (`internal/rms/parse_test.go`).

1. **if/elseif-цепочки через секции** (`KHR_WSVG_*`, 9 карт). WSVG-карты
   переключают целые варианты карты одной гигантской условной цепочкой,
   легально пересекающей заголовки секций. Парсер закрывал условные скоупы
   на каждом `<SECTION>` → каскад ложных «unterminated block» /
   «without a matching opening block» (~164 диагностик на 9 карт).
   Фикс: граница секции закрывает только `{}`-блок; условные скоупы живут
   до EOF. Материализация секций отложена до EOF (ranges условных
   финализируются по последнему ребёнку).
2. **`#include_drs <file> <id>`** (45% карт). Директиза классики/HD,
   отсутствовала в грамматике; точка в имени файла (`random_map.def`)
   давала «unexpected "."» и ломала поток токенов. Фикс: слова могут
   содержать точки; директива парсится как statement (уже была в
   directiveWords, ломался только лексер).
3. **Бессобачные attribute-блоки `create_object TUNA … }`** (паттерн
   WSVG/es@). Движок принимает атрибуты `create_*`-команд без `{`;
   сохранённый `}` давал «unexpected "}"», атрибуты — «unknown command».
   Фикс: неявный attribute-контекст для `create_*` (до следующей команды,
   смены ветки условия или секции; `{`-строка после endif привязывается к
   последней create_-команде — паттерн es@the_unknown).
4. **Атрибуты внутри if/else-веток** (Rooster). Значения атрибутов
   выбираются веткой: `create_object TUNA / if … / number_of_objects 22 /
   else … / endif`. Неявный контекст продолжает собирать атрибуты из
   вложенных веток.
5. **Кавычки внутри комментариев** (`016___Infinite_Forest`). `/* the
   "relic isle" */` — кавычки в блочном комментарии включали
   string-трекинг, комментарий не гасился, содержимое протекало в токены
   (`create_land` получал строковый аргумент). Фикс: порядок состояний в
   blankComments — блочный комментарий гасит кавычки.
6. **Не-ASCII байты** (`011__first_map`, французский текст). Байты ≥0x80 —
   теперь словообразующие: вместо потока «unexpected \xc3» — нормальные
   слова (мусор вне комментариев честно помечается unknown-command).

## Остаток (301 диагностика) — не дефекты парсера

- **KB-серые зоны** (145 диагностик, 18 карт): атрибуты, не
  задокументированные у Зетнуса для данной команды, но используемые в
  работающих картах — `border_fuzziness`/`other_zone_avoidance_distance`
  на create_terrain, `clumping_factor` на create_elevation,
  `set_scale_by_size/groups` на create_object (гайд прямо говорит: для
  объектов — `set_scaling_to_map_size`). Нужна кураторская правка kb/data
  по источникам — см. план задачи.
- **Опечатки авторов** (~20): `group_varience`, `clumbing_factor`,
  `min_length_cliff`/`min_distance_cliff` (правильно
  `min_length_of_cliff`/`min_distance_cliffs`), французский текст вне
  сломанного самореферентного комментария — флаги корректны.
- **`deprecated-effect-percent`** (36) — задуманное предупреждение.
- **`bad-argument`** (~10) — корректные флаги.

## Воспроизведение

```sh
scripts/corpus-fetch.sh .corpus        # детерминированный сэмпл 100
go build -o aoe2-lsp ./cmd/aoe2-lsp
go build -o corpus-run ./cmd/corpus
timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 \
	bash -c './corpus-run -bin ./aoe2-lsp -dir .corpus'
```

Обновление пула (перепин SHA): `scripts/corpus-fetch.sh --update`.
