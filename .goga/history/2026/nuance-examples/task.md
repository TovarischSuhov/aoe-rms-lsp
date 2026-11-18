# nuance-examples: минимальные тест-фикстуры для 13 правил конвейера (#98–#111)

Подготовительный артефакт nuance-конвейера (#53/пачка #98–#111): для
каждого правила — минимальный положительный пример (должна срабатывать
диагностика) и отрицательный (тишина). Все примеры прогнаны через
парсеры 2026-11-18 — синтакс-ошибок нет, кроме отмеченных (XS-правила
#110/#111 опираются на вендорные bugged-кейсы, которые наш парсер
сейчас даёт как `syntax` — их nuance-вариант требует решения слота,
см. Notes). Источники фактов: issue, `docs/ref/real-map-nuances.md`,
vendored UGC Guide bugs.

Формат проверок — существующий nuance-механизм (`nuances_test.go`,
fixture-KB `testdata/nuance-kb`): data-driven правила задаются записями
nuances.json (aliases / deprecated / legacy / noop_attributes), кодовые
— условием в analyzer.

---

## RMS — директивы и атрибуты

### #98 `#include_drs` legacy → hint (`legacy-include_drs`, данные: legacy)

Положительный:
```
#include_drs random_map.def 54000
```
Отрицательный: `#include "std.inc"` — обычный include, тишина.
Факт: 78×/45 карт, официальные DE-карты (ref §1). Notes: механизм legacy
уже есть в nuances.json (map name→code/message), запись добавляет
`#include_drs`... проверка по RMS-директиве, не по имени — может
потребоваться кодовой ветки в parse-уровне директив, если маркер
вешается только на имена команд/атрибутов.

### #99 `set_scale_by_size/groups` на create_object → hint (данные: noop_attributes)

Положительный:
```
<OBJECTS_GENERATION>
create_object HUNTER {
	set_scale_by_size 3
}
```
Отрицательный: тот же атрибут на `create_terrain GRASS {…}` — норм
(документирован terrain/elevation, ref §3). Факт: 104×/4 карты.
Notes: noop_attributes-пары (attribute, owner-command) — механика
`probably-ignored` уже реализована.

### #100 create_land-атрибуты на create_terrain → hint

Положительный (репрезентативное подмножество, куратор per attribute):
```
<TERRAIN_GENERATION>
create_terrain GRASS {
	border_fuzziness 50
	other_zone_avoidance_distance 3
	left_border 2
	base_size 8
	number 3
}
```
Отрицательный: те же атрибуты на `create_land {…}` — норм (гайд:248–263).
Факт: border_fuzziness 48×/4, other_zone_avoidance 13×/5 и пр. (ref §3).

### #101 create_terrain-атрибуты на create_elevation → hint

Положительный:
```
<ELEVATION_GENERATION>
create_elevation 5 {
	clumping_factor 10
	spacing_to_other_terrain_types 3
}
```
Отрицательный: `clumping_factor` на `create_land {…}` — норм.
Факт: clumping_factor 16×/8, spacing 2×/2 (ref §3).

### #102 create_terrain-атрибуты на create_object → hint

Положительный:
```
<OBJECTS_GENERATION>
create_object HUNTER {
	terrain_to_build_on GRASS
	temp_min_distance_to_players 3
	base_terrain GRASS
	set_avoid_player_start_areas 3
}
```
Отрицательный: `set_avoid_player_start_areas` на `create_terrain {…}` —
норм (гайд:308). Факт: set_avoid 12×/2, terrain_to_build_on 3×/1 и пр.
(ref §3).

### #103 create_object-атрибуты на create_terrain → hint

Положительный:
```
<TERRAIN_GENERATION>
create_terrain GRASS {
	distance_to_other_terrain_types 3
	min_distance_to_players 5
	max_distance_to_players 10
}
```
Отрицательный: `spacing_to_other_terrain_types` на `create_terrain {…}` —
норм (документирован, гайд:301). Факт: distance 2×/2, min/max 2×/1
(ref §3).

### #104 вложенный create_terrain → hint (кодовое условие)

Положительный — **brace-less** (проверено: парсится чисто; вложенные
`{}`-блоки парсер не ест — `syntax: unexpected "{"`):
```
<TERRAIN_GENERATION>
create_terrain GRASS {
	set_scale_by_size 3
	create_terrain DIRT
}
```
Отрицательный: тот же файл без вложенной команды. Факт: 2×/1, Crater_Lake
(ref §3).

### #105 create_land с позиционным аргументом → hint (кодовое условие)

Положительный:
```
<LAND_GENERATION>
create_land 2 {
}
```
Отрицательный: `create_land {…}` — каноническая форма гайда. Факт: 8×
позиционной формы; скелет гайда — create_land {} без аргументов.

### #106 typo aliases → warning с точной правкой (данные: aliases)

Положительный (кураторский список целиком, минимально по одному):
```
<CLIFF_GENERATION>
create_terrain GRASS {
	group_varience 5
	clumbing_factor 10
	set_avoid_play_start_areas 3
	min_length_cliff 1
	max_length_cliff 2
	min_distance_cliff 3
	min_length_of_cliffs 1
	max_lenght_of_cliffs 2
}
```
Отрицательный: `group_variance`, `clumping_factor`,
`set_avoid_player_start_areas`, `min_length_of_cliff` — не warning
(значит должны резолвиться или падать обычным unknown, не typo-alias).
Факт: полный список и правильные имена — ref §4. Notes: механика
typo-alias реализована; записи alias→correct в nuances.json.

### #107 effect_percent deprecated → warning (данные: deprecated — уже есть)

Положительный:
```
<PLAYER_SETUP>
effect_percent SET_ATTRIBUTE HOUSE ATTR_STORAGE_VALUE 10
```
Отрицательный: `effect_amount …` — тишина. Факт: ref §4. Notes: правило
уже реализовано и покрыто (`TestAnalyzeRms_DeprecatedFromData`) —
пример уже в тестах, в конвейере переносить нечего.

### #108 terrain_type/base_terrain как команды вне create_-контекста → hint (кодовое условие)

Положительный:
```
<TERRAIN_GENERATION>
start_random
	percent_chance 50
		terrain_type GRASS
	percent_chance 50
		base_terrain WATER
end_random
```
Отрицательные: те же имена как **атрибуты** — `create_terrain GRASS {
terrain_type DIRT }` и `create_land { terrain_type GRASS }` — норм.
Факт: 50×, REDv1/v2 (ref §4) — мёртвый код, движок молча игнорирует.
Notes: проверка «имя-атрибута в позиции команды», за исключением
`start_random`-веток — команды-исключения по контексту, не по имени.

## XS

### #109 const int в теле функции → hint (кодовое условие)

Положительный:
```
void f() {
	const int x = 1;
}
```
Отрицательный: `const int x = 1;` на top-level — норм (гайд описывает
const-глобалы; прелюдия ими пользуется). Факт: task.xs из bugged-кейсов;
парсер принимает (проверено: syntax=0). Support движком не верифицирован.

### #110 long/double/char → warning (кодовое условие или парсер)

Положительный — вендорный bugged-кейс verbatim:
```
void main() {
    // none of these declarations work:
    long a = 1000;
    double b = 10.34;
    char c = '8';
}
```
Отрицательный: int/float/string/vector/bool декларации — тишина
(проверено: syntax=0).
**Notes (решение слота):** наш парсер сейчас даёт 3× `syntax: expected ;`
— декларации парсятся в [expr]-стейтменты (вендорный кейс тоже). Варианты:
(a) осмысленная syntax-диагностика в парсере при распознавании `long`/
`double`/`char` в позиции типа; (b) парсерная поддержка деклараций +
nuance-warning в analysis. Вариант (a) дешевле и не добавляет мёртвых
типов в AST.

### #111 goto → warning (кодовое условие или парсер)

Положительный — вендорный bugged-кейс verbatim:
```
void main() {
    goto non_existent_label;
}
```
Отрицательный: break/continue/for — тишина (проверено: syntax=0).
**Notes:** аналогично #110 — сейчас `syntax: expected ;`, statement
падает в [expr]. Тот же выбор варианта; (a) — распознавание `goto`
в позиции стейтмента.

---

## Верификация

Все примеры прогнаны через rms.Parse / xs.XsParse 2026-11-18
(claude-code/glm-5.3): RMS — 22 фрагмента, syntax=0 у всех, кроме
намеренного вложенного-braces случая (отвергнут как не-паттерн
корпуса); XS — 5 фрагментов, syntax=0 у #109 (обе стороны) и #110/#111
negative; positive #110/#111 — syntax-ошибки как у вендорных кейсов.
Файл — формулировка, не знание: при реализации каждого правила примеры
становятся table-driven кейсами в nuances_test.go / новых тестах, а
`verified` ставится фактическим прогоном правила.
