# Незадокументированные нюансы реальных RMS/XS карт

Справочник синтаксических конструкций и атрибутов, встреченных в корпусе
100 опубликованных карт (прогон 09.09.2026, см.
[corpus-run-2026-09-09.md](../reviews/corpus-run-2026-09-09.md)), которые
**не описаны в референсе** — гайде Зетнуса
([Google Doc](https://docs.google.com/document/d/1jnhZXoeL9mkRUJxcGlKnO98fIwFKStP_OBozpr0CHXo/edit),
локальная копия: `docs/ref/zetnus-rms-guide.txt`; строки указаны по ней).

Рабочая гипотеза (подтверждается структурой находок): движок RMS
прощающий — неизвестные команды и атрибуты **молча игнорируются**, поэтому
карты с «мёртвыми» директивами работают, а авторы не замечают. Движок XS,
наоборот, строже (см. раздел XS).

Вердикты:

- ✅ **работает** — конструкция в картах, которые реально играются
  (турнирные серии WSVG/T90, официальные карты DE);
- ❓ **вероятно игнорируется** — используется в работающих картах, но не
  документировано для этой команды; скорее всего no-op;
- ✍️ **опечатка автора** — близкое написание существующего имени;
- 💬 **мусор** — текст вне сломанного комментария.

Ссылки на карты — GitHub blob с запинненным SHA из
`scripts/corpus-sources.txt`.

---

## 1. Директивы

### `#include_drs <файл> <id>` — ❓ легаси, терпится движком DE

78 использований в 45 из 100 карт — включая **официальные карты DE**
(`real_world_*`, порты `es@*` из репо
[Naramsim/AoE2-random-map-scripts](https://github.com/Naramsim/AoE2-random-map-scripts)).
DRS-архивов в DE нет — директива фактически no-op, но официальный контент
её массово содержит (наследие AoC/HD).

- Пример: [`real_world_madagascar.rms`](https://github.com/Naramsim/AoE2-random-map-scripts/blob/018d0bfc05d2b5851fdd06ef27e503c026a168b5/The%20forgotten/real_world_madagascar.rms)
- Пример: [`KHR_WSVG_Chaos Pit.rms`](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/STANDARD_MAPS_WITH_USER_PATCH_FEATURES/KOTH_REGICIDE/WSVG%20Single%20Maps%20v2/KHR_WSVG_Chaos%20Pit.rms)
  (`#include_drs random_map.def 54000` + `std_resources.inc 54101`)
- Гайд Зетнуса: не упоминается.

## 2. Структурный синтаксис (✅ работает — подтверждено играбельными картами)

### if/elseif/else-цепочки через заголовки секций

Условная цепочка открывается до первой секции и тянется через все
`<LAND_GENERATION>…<OBJECTS_GENERATION>` до `endif` в конце файла — так
WSVG-карты переключают целые варианты карты. До фикса парсера давала
~164 ложных диагностик на 9 карт.

- [`KHR_WSVG_Chaos Pit.rms`](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/STANDARD_MAPS_WITH_USER_PATCH_FEATURES/KOTH_REGICIDE/WSVG%20Single%20Maps%20v2/KHR_WSVG_Chaos%20Pit.rms):
  `if WSVG_ACROPOLIS` (строка 49) … 4 секции … `endif` (конец файла);
- [`T90CG_BoA_MapPack_v4.rms`](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/NON_FREE_FOR_ALL_MAPS/T90CG_BoA_MapPack_v4.rms),
  [`es@the_unknown_v2`](https://github.com/Naramsim/AoE2-random-map-scripts/blob/018d0bfc05d2b5851fdd06ef27e503c026a168b5/Others/es%40the_unknown_v2.rms).
- Гайд: секции и условия описаны раздельно, пересечение не оговорено.

### Attribute-блок `create_*` без открывающей `{` (с закрывающей `}`)

13 блоков в 5 картах: `create_object TUNA` / атрибуты / `}`.

- [`KHR_WSVG_Chaos Pit.rms`](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/STANDARD_MAPS_WITH_USER_PATCH_FEATURES/KOTH_REGICIDE/WSVG%20Single%20Maps%20v2/KHR_WSVG_Chaos%20Pit.rms),
  [`Tiger_Woods_1.0.rms`](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/FREE_FOR_ALL_MAPS/Tiger_Woods_1.0.rms),
  [`KHR_WSVG_Rooster.rms`](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/STANDARD_MAPS_WITH_USER_PATCH_FEATURES/KOTH_REGICIDE/WSVG%20Single%20Maps%20v2/KHR_WSVG_Rooster.rms)
- Гайд: скелет всегда показывает `{ … }`.

### `{`-блок, открытый после закрытой условной цепочки

17 карт: `endif`, затем `{ атрибуты }` привязывается к последней
`create_*`-команде (атрибуты разделяются между ветками, выбравшими
разные имена террейнов).

- [`es@the_unknown_v2.rms`](https://github.com/Naramsim/AoE2-random-map-scripts/blob/018d0bfc05d2b5851fdd06ef27e503c026a168b5/Others/es%40the_unknown_v2.rms)
  (строка ~1933), [`KHR_WSVG_Pond Arena.rms`](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/STANDARD_MAPS_WITH_USER_PATCH_FEATURES/KOTH_REGICIDE/WSVG%20Single%20Maps%20v2/KHR_WSVG_Pond_Arena.rms)
- Гайд: не описан.

### Атрибуты внутри if/else-веток при brace-less контексте

`create_object TUNA / if … / number_of_objects 22 / else / … / endif` —
атрибуты собираются из веток. [`KHR_WSVG_Rooster.rms`](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/STANDARD_MAPS_WITH_USER_PATCH_FEATURES/KOTH_REGICIDE/WSVG%20Single%20Maps%20v2/KHR_WSVG_Rooster.rms)
(строка ~682).

### Кавычки и не-ASCII внутри комментариев

`/* the "relic isle" :) */` — 4 карты; не-ASCII (французский текст,
CP1251/UTF-8) — 4 карты. Комментарий не должен терминироваться кавычкой,
байты ≥0x80 — часть слов.

- [`first_map.rms`](https://github.com/Maboey/aoe2_rms/blob/3f064dbeedbdd4a490d974ae1c0cf7b5bc44353b/first_map.rms),
  [`!Infinite Forest! 0.4`](https://github.com/dundass/rms-playground/blob/74471f709f52338918d2cae5a7a7ef1694fd0e5a/%21Infinite%20Forest%21%200.4%20AI%20Friendly%20by%20vjaz.rms)

## 3. Атрибуты вне документированной команды (❓ вероятно игнорируются)

Ключевая улика гипотезы: [`KHR_WSVG`-серия](https://github.com/HSZemi/rms/tree/c422ed5e4eda735bd5ef705bd409ecb446c1502d/STANDARD_MAPS_WITH_USER_PATCH_FEATURES/KOTH_REGICIDE/WSVG%20Single%20Maps%20v2)
играется в турнирах годами с этими атрибутами — если бы движок считал их
ошибкой, карты бы не генерировались. Гайд Зетнуса прямо указывает
(`zetnus-rms-guide.txt:499`), что масштабирование объектов — это
`set_scaling_to_map_size`, а `set_scale_by_size/groups` — для
elevation/terrain: значит на `create_object` они, скорее всего, no-op.

| Команда | Атрибут | В сэмпле | Документирован для | Пример |
|---|---|---|---|---|
| `create_object` | `set_scale_by_size` | 52× / 4 карты | terrain, elevation (гайд:276, 307) | [Chaos Pit](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/STANDARD_MAPS_WITH_USER_PATCH_FEATURES/KOTH_REGICIDE/WSVG%20Single%20Maps%20v2/KHR_WSVG_Chaos%20Pit.rms) |
| `create_object` | `set_scale_by_groups` | 52× / 4 | terrain, elevation | там же |
| `create_terrain` | `border_fuzziness` | 48× / 4 | **create_land** (гайд:257) | [Chaos Pit](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/STANDARD_MAPS_WITH_USER_PATCH_FEATURES/KOTH_REGICIDE/WSVG%20Single%20Maps%20v2/KHR_WSVG_Chaos%20Pit.rms) |
| `create_elevation` | `clumping_factor` | 16× / 8 | **create_land, create_terrain** (гайд:258, 306) | [Acropolis](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/STANDARD_MAPS_WITH_USER_PATCH_FEATURES/KOTH_REGICIDE/WSVG%20Single%20Maps%20v2/KHR_WSVG_Acropolis.rms) |
| `create_terrain` | `other_zone_avoidance_distance` | 13× / 5 | **create_land** (гайд:263) | [Tiger Woods](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/FREE_FOR_ALL_MAPS/Tiger_Woods_1.0.rms) |
| `create_object` | `set_avoid_player_start_areas` | 12× / 2 | **create_terrain** (гайд:308) | [es@pilgrims_v2](https://github.com/Naramsim/AoE2-random-map-scripts/blob/018d0bfc05d2b5851fdd06ef27e503c026a168b5/Others/es%40pilgrims_v2.rms) |
| `create_terrain` | `left/right/top/bottom_border`, `base_size` | 6× / 2 | **create_land** (гайд:248, 253–256) | [es@paradiseisland_v2](https://github.com/Naramsim/AoE2-random-map-scripts/blob/018d0bfc05d2b5851fdd06ef27e503c026a168b5/Others/es%40paradiseisland_v2.rms) |
| `create_terrain` | `distance_to_other_terrain_types` | 2× / 2 | не найден (похож: `spacing_to_other_terrain_types`, гайд:301) | [T90CG_BoA_MapPack_v4](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/NON_FREE_FOR_ALL_MAPS/T90CG_BoA_MapPack_v4.rms) |
| `create_elevation` | `spacing_to_other_terrain_types` | 2× / 2 | **create_terrain** (гайд:301) | [Mountain Pass](https://github.com/Naramsim/AoE2-random-map-scripts/blob/018d0bfc05d2b5851fdd06ef27e503c026a168b5/The%20forgotten/Mountain%20Pass.rms) |
| `create_terrain` | `min/max_distance_to_players` | 2× / 1 | **create_object** (`set_avoid_player_start_areas`-семейство) | [es@prairie_v2](https://github.com/Naramsim/AoE2-random-map-scripts/blob/018d0bfc05d2b5851fdd06ef27e503c026a168b5/Others/es%40prairie_v2.rms) |
| `create_object` | `terrain_to_build_on` | 3× / 1 | не найден | [Nile Delta](https://github.com/Naramsim/AoE2-random-map-scripts/blob/018d0bfc05d2b5851fdd06ef27e503c026a168b5/The%20forgotten/Nile%20Delta.rms) |
| `create_object` | `temp_min_distance_to_players` | 1× / 1 | не найден | [real_world_world](https://github.com/Naramsim/AoE2-random-map-scripts/blob/018d0bfc05d2b5851fdd06ef27e503c026a168b5/The%20forgotten/real_world_world.rms) |
| `create_object` | `base_terrain` | 1× / 1 | **create_terrain, create_elevation** (гайд:272, 297) | [Tiger Woods](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/FREE_FOR_ALL_MAPS/Tiger_Woods_1.0.rms) |
| `create_terrain` | `number` | 1× / 1 | не найден | [KHR_WSVG_Atacama](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/STANDARD_MAPS_WITH_USER_PATCH_FEATURES/KOTH_REGICIDE/WSVG%20Single%20Maps%20v2/KHR_WSVG_Atacama.rms) |
| `create_terrain` | `create_terrain` (вложенный!) | 2× / 1 | — | [Crater_Lake](https://github.com/Naramsim/AoE2-random-map-scripts) |

Паттерн очевиден: авторы копируют атрибуты между `create_land` /
`create_terrain` / `create_elevation` / `create_object` — движок глотает.

`create_land N` с позиционным аргументом (8×,
[!Infinite Forest!](https://github.com/dundass/rms-playground/blob/74471f709f52338918d2cae5a7a7ef1694fd0e5a/%21Infinite%20Forest%21%200.4%20AI%20Friendly%20by%20vjaz.rms))
— гайд-скелет показывает `create_land {}` без аргументов (гайд:244) —
❓ вероятно игнорируется.

## 4. Опечатки и мусор (✍️ / 💬 — точно не работают)

| Что | В сэмпле | Правильно | Где |
|---|---|---|---|
| `min_length_cliff`, `max_length_cliff`, `min_distance_cliff` | 12× / 4 | `min_length_of_cliff`, `max_length_of_cliff`, `min_distance_cliffs` (гайд:284–290) | [Chaos Pit](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/STANDARD_MAPS_WITH_USER_PATCH_FEATURES/KOTH_REGICIDE/WSVG%20Single%20Maps%20v2/KHR_WSVG_Chaos%20Pit.rms) |
| `min_length_of_cliffs`, `max_lenght_of_cliffs` | 2× / 1 | `…_of_cliff` (без s, без опечатки lenght) | [!Infinite Forest!](https://github.com/dundass/rms-playground/blob/74471f709f52338918d2cae5a7a7ef1694fd0e5a/%21Infinite%20Forest%21%200.4%20AI%20Friendly%20by%20vjaz.rms) |
| `group_varience` | 8× / 4 | `group_variance`? (гайд знает `group_placement`-семейство) | [Chaos Pit](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/STANDARD_MAPS_WITH_USER_PATCH_FEATURES/KOTH_REGICIDE/WSVG%20Single%20Maps%20v2/KHR_WSVG_Chaos%20Pit.rms) |
| `clumbing_factor` | 1× | `clumping_factor` | [Tiger Woods](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/FREE_FOR_ALL_MAPS/Tiger_Woods_1.0.rms) |
| `set_avoid_play_start_areas` | 1× | `set_avoid_player_start_areas` | [es@paradiseisland_v2](https://github.com/Naramsim/AoE2-random-map-scripts/blob/018d0bfc05d2b5851fdd06ef27e503c026a168b5/Others/es%40paradiseisland_v2.rms) |
| `WORK`, `Dans`, `il`, `Normal`, `attention` | 5× / 1 | 💬 французский текст вне самореферентного комментария (внутри `/* … */` автор объяснял синтаксис комментариев — текст `*/` закрыл комментарий досрочно) | [first_map.rms](https://github.com/Maboey/aoe2_rms/blob/3f064dbeedbdd4a490d974ae1c0cf7b5bc44353b/first_map.rms) |

`effect_percent` (36× / 3) — документирован, но **deprecated** с DE
(гайд: «use effect_amount instead») — работает, предупреждение уместно.

**`terrain_type`/`base_terrain` как команды вне create_-контекста** —
[REDv1](https://github.com/HSZemi/rms/blob/c422ed5e4eda735bd5ef705bd409ecb446c1502d/FREE_FOR_ALL_MAPS/REDv1.rms)/REDv2:
внутри `start_random/percent_chance`-веток на верхнем уровне секции, без
create_terrain-обёртки — 50×. Это атрибуты, а не команды: движок молча
игнорирует, выбор террейна не работает (❓/💬 мёртвый код — находка
расширенного корпуса).

## 5. XS (по vendored UGC Guide, `docs/ref/ugc-guide/xs`)

- `const int x = …` внутри тела функции — встречено в
  [bugged-кейсах UGC Guide](https://github.com/Divy1211/AoE2DE_UGC_Guide)
  (`task.xs`); гайд XS-языка ([ugc.aoe2.rocks](https://ugc.aoe2.rocks/general/xs/))
  const для локалов не описывает. ❓ поддержка движком не проверена;
  наш парсер принимает.
- `long`, `double`, `char` — 💬 **не работают** (сам кейс
  `bugs/bugged/types1.xs`: «none of these declarations work»).
- `goto` — 💬 отсутствует в XS (`bugs/bugged/goto.xs` — кейс про его
  отсутствие).
- `prelude.xs` (дамп встроенной библиотеки, 4998 строк) — парсится нашим
  парсером с 0 диагностик; содержательная база для сверки грамматики.

---

## Что с этим делать (следующие шаги)

1. KB-кураторство (раздел 3): подтвердить/опровергнуть каждый ❓ по
   источникам вне Зетнуса (форум World's Edge, тесты в игре) и либо
   добавить атрибут в `kb/data` для нужной команды, либо оставить
   предупреждение с формулировкой «не документировано для этой команды,
   вероятно игнорируется».
2. Парсер (раздел 2) — уже поддерживает всё перечисленное (фиксы PR #22,
   #24).
3. При появлении публичных `.xs`-коллекций — расширить корпус.
