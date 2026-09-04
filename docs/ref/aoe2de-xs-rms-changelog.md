# AoE2 DE — XS/RMS changelog из release notes
# Собрано: 2026-09-04, источник: ageofempires.com/news
# Период: 2019-11 (релиз AoE2 DE) — 2026-09. Просмотрено 77 постов обновлений AoE2 DE
# (Update/Minor Update/Hotfix/Update Preview). Посты без XS/RMS-изменений в файл не включены.

## Update 177723 — 2026-06-02
https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-177723/

### XS: добавлено
- `int xsGetLocale()` — Gets the current locale of this player. Note: See the Language Constants section of Constants.xs for all values
- `string xsGetString(int stringId, bool localized = false)` — Gets a string from the strings file
- `string xsGetPlayerAttributeName(int resourceId, bool localized = false)` — Gets the Editor resource name
- `string xsGetObjectAttributeName(int attributeId, bool localized = false)` — Gets the Object Attribute name
- `string xsGetDamageClassName(int damageClassId, bool localized = false)` — Gets the UI Damage Class name
- `bool xsIsObjectValid(int objectId, int playerId)` — Checks whether an object exists in data
- `int xsGetPlayerNumberOfObjects(int playerId)` — Gets the number of data objects of a player
- `int xsGetLocalPlayerId()` — local player number; Do not use this function for anything outside of things like display and chat — it will cause desyncs
- `string xsGetPlayerColorTag(int playerId)` — Gets the player color tag like `<RED>`; works when the color has been changed in a scenario from the Players menu. Note: See the Color Constants section of Constants.xs
- `int xsGetMapSeed()`
- `int xsGetTechAttribute(int playerId, int techId, int techAttribute, int indexOrCostType)` — Gets a specific attribute from a tech. Note: See the Tech Attribute Constants section of Constants.xs
- Функции, работающие как аналоги триггерных эффектов (passing -1 to playerId applies to all players):
  - `bool xsPlaySound(string eventOrSoundFileName, int playerId, vector position, float angle, int objectId, bool global = false)`
  - `bool xsDisplayInstructions(string msg, int time, int sourcePlayer, int iconObjectId, int panelPosition, bool useTagColorForIcon, bool playSound, string soundFilename, int playerId = -1)` — sourcePlayer corresponds with the editor "Source Player" option; playerId = which player to show this instruction to
  - `bool xsClearInstructions(int panelPosition, int playerId = -1)`
  - `bool xsDisplayTimer(int timerId, string msg, int time, int timeUnit, bool resetTimer, int playerId = -1)`
  - `bool xsClearTimer(int timerId, int playerId = -1)`
  - `float xsGetTimerTimeRemaining(int timerId, int timeUnit, int playerId = -1)`
  - `bool xsSendChat(string msg, int playerId = -1, bool silent = false)`
  - `void xsDeclareVictory(int playerId, bool victory = true)`
- Math-related functions: `float ln(float x)`, `float log2(float x)`, `float log10(float x)`, `float round(float x)`, `float radians(float x)`, `float degrees(float x)`, `float dist(vector v1, vector v2)`, `int bitAnd(int v1, int v2)`, `int bitOr(int v1, int v2)`, `int bitNot(int v1)`, `int bitXor(int v1, int v2)`

### XS: изменено/исправлено
- Fixed the XS main function not being invoked in replays.
- Fixed an XS error happening in custom lobbies upon transferring Random Maps or Custom Scenarios including XS files.
- Fixed file transfers for XS scripts failing on restored lobbies.

### Триггеры/константы
- Renamed the "Enable/Disable Attackable" effect to "Enable/Disable Unit Attackable State".
- Renamed the "Enable/Disable Unit Targeting" effect to "Enable/Disable Unit Targetable State".
- The "Display Instructions" effect no longer applies the `<TAG>` text color to the icon; the "Source Player" field is now used for this option as originally intended. New option "Use Tag Color For Icon" added to "Display Instructions" (on by default for scenarios affected by this bug to preserve old behavior).

### RMS
- нет изменений (только фикс передачи RMS/XS-файлов в лобби, см. XS выше)

## Update 169123 — 2026-02-17
https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-169123/

### XS: добавлено
- `bool xsSetUnitPosition(int unitId, vector position, bool checkCollision = true)`
- `int xsGetWorldPlayerId(int scenarioPlayerId)`
- `int xsGetGarrisonedInUnitId(int unitId)`
- `int xsGetGarrisonedUnitIds(int unitId)` — Returns an int array.
- `int xsGetPlayerType(int playerId)` — Refer to the Player Type Constants in Constants.xs.
- `bool xsRemoveUnit(int unitId)`
- `int xsGetDiplomacy(int sourcePlayerId, int targetPlayerId)` — Refer to the Diplomacy Constants in Constants.xs.
- `bool xsSetDiplomacy(int sourcePlayerId, int targetPlayerId, int diploState, bool mirror = false)`
- `int xsGetColorMood()` — Refer to the Color Mood Constants in Constants.xs.
- `bool xsSetColorMood(int colorMood, int interval)`
- `int xsGetDifficulty()` — Refer to the Difficulty Constants in Constants.xs.
- `int xsCreateUnit(int objectId, int playerId, vector location, bool foundation = false, bool playCreatedSound = true, bool checkCollision = true)` — Returns the ID of the created unit.
- `int xsGetObjectTaskCount(int objectId, int playerId)`
- `int xsGetUnitTaskCount(int unitId)`
- `bool xsObjectTaskAmount(int objectId, int playerId, int taskId)`
- `bool xsUnitTaskAmount(int unitId, int taskId)` — Both TaskAmount functions set the XS task struct from the given unit/data object.
- `float xsGetTaskAmount(int taskFieldId)` — Queries the XS task struct for the value of the given field.
- `bool xsModifyObjectTasks(int objectOrClassId, int playerId, int taskId, bool edit = false)`
- `bool xsModifyUnitTasks(int unitId, int taskId, bool edit = false)`
  - A positive index inserts new tasks into the task list. When edit = true, modify an existing task instead of inserting. A negative index removes tasks at that index − 1 (−1 removes task 0, −2 removes task 1, etc.).
  - Note: xsTaskAmount should now be used to set the task type, the object/class to target using: `cTaskAttrTaskType = 28`, `cTaskAttrObjectId = 29`, `cTaskAttrObjectClass = 30`.
  - These ModifyTask functions should be preferred over xsTask moving forward, as they do not have the same insertion/search-related limitations as xsTask.

### XS: изменено/исправлено
- (см. выше — ModifyTask предпочтительнее xsTask)

### Триггеры/константы
- Added additional options to the Task Object trigger: checkbox Mutual (Change Diplomacy); Task Types: Tranform, Ring Town Bell, Send Back To Work, Force Drop Off, Buy Resources, Sell Resources.
- New Trigger: Build Object.
- Extended attributes support for Modify Attribute.

### Modding (атрибуты/таски — влияют на константы для XS/Modify Attribute)
- New attributes: 160: Add Armor Type; 161: Add Attack Type; 162: Charge Target Field; 163: Size Class (garrison types 0–8: CIVILIAN, INFANTRY, CAVALRY, RELIGIOUS, LIVESTOCK, SIEGE, SHIPS, UNUSED).
- New unit tasks: Task 156: Additional Spawn; Task 159: Amphibious; Task 160: HP Damage Modifier; Task 161: Unit Refund.
- New Trail Mode 4; new Graphics Sequence Type 16 (reverse animation); new blast attack level 32 (with Flag 128 — cone damage); new Flag 8 for Disable Attribute (object not on UI if train limit reached); Train Limit attribute now functions on buildings.

### RMS
- Fixed an issue where incorrectly defined terrains in Random Map Scripts could result in inconsistent outcomes.
- Fixed certain Gaia buildings to reset to Western European architecture when applied the make_indestructible parameter on Random Maps Generation.
- Added a new objreplacement.json file to enable civilization-specific object replacements for RMS. Parameters: object_id, technology, technology_state, required_attribute, replacement_attribute, excluded_computer_players, replacement_object, chance, replacement_rate.
- Added new terrain: Forest, Dry South American (128).

## Update 158041 — 2025-10-14
https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-158041/

### XS: добавлено
- нет

### XS: изменено/исправлено
- нет

### Триггеры/константы
- Allowed modifying the following unit attributes by Effect Amount, Modify Attribute trigger effect and technologies in data with the IDs: Train Locations Entry Mod (158) — индекс структуры Train Location для модификации; Train Locations Total Num (159) — получение/изменение длины массива Train Locations.
- Added an additional 20 blank technologies to the Scenario Editor.
- Fixed an issue preventing "Task Object" trigger from activating when the last unit in the selected unit list died.
- Fixed an issue where the editor settings option "Run triggers and effects in display order" would become unintentionally disabled if the user cancelled the dialogue window.
- The Trigger Active condition now correctly remembers the selected trigger when deleting other triggers.
- Fixed an issue where using the Modify Resource effect would show blank entries instead of actual resources.
- Flag Achaemenids 1/2/3, Flag Athenians 1/2/3, Flag Spartans 1/2/3 переименованы в "Civ Flag ..." (рендер-объекты редактора).

### RMS
- Fixed an issue that was allowing the spawning of distant herdable animals in some Empire Wars maps (генерация, не синтаксис).

## Update 153015 — 2025-08-12
https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-153015/

### XS: добавлено
- Разрешён вызов XS-функций из technology effects и effect amount команд: resource 33, установленное в положительное целое, вызывает xs-функцию `EffectFunctionN` с аргументом playerId (N = значение ресурса 33; playerId = игрок, чей эффект сработал). Функции хранятся в новом файле **Effects.xs**.
- `void xsResetTaskAmount()` — Resets all task values set by xsTaskAmount to their default values
- `int xsGetTurn()` — Returns the current game turn (similar to tick; high-frequency rules run every turn)
- `int xsGetPlayerUnitIds(int objectId, int playerId, int xsArrayId)` — Returns an array of unit IDs; третий параметр опционален, позволяет переиспользовать существующий массив
- `float xsGetUnitAttribute(int unitId, int attribute, int damageClass)`
- `bool xsDoesUnitExist(int unitId)`
- `int xsGetUnitOwner(int unitId)`
- `string xsGetPlayerName(int playerId)`
- `vector xsGetUnitPosition(int unitId)`
- `string xsGetUnitName(int unitId, bool internalName = false)`
- `int xsGetUnitTargetUnitId(int unitId)`
- `vector xsGetUnitMoveTarget(int unitId)`
- `int xsGetUnitGroupId(int unitId)` — Group as in one selection (formation)
- `vector xsGetGroupMoveTarget(int groupId)`
- `string xsGetObjectName(int objectId, int playerId, bool internalName = false)`
- `bool xsIsObjectAvailable(int objectId, int playerId)`
- `string xsGetTechName(int techId, int playerId)`
- `int xsGetTechState(int techId, int playerId)` — See Constants.xs for relevant constants
- `float xsGetUnitHitpoints(int unitId)`
- `bool xsSetUnitHitpoints(int unitId, float value)`
- `float xsGetUnitBuildPoints(int unitId)`
- `bool xsSetUnitBuildPoints(int unitId, float value)`
- `int xsGetUnitObjectId(int unitId)`
- `int xsGetUnitCopyId(int unitId)`
- `int xsGetObjectCopyId(int playerId, int objectId)`
- `int xsGetUnitClass(int unitId)`
- `int xsGetObjectClass(int playerId, int objectId)` — See Constants.xs for relevant constants
- `int xsGetUnitType(int unitId)`
- `int xsGetObjectType(int playerId, int objectId)` — See Constants.xs for relevant constants
- `int xsGetUnitAttributeTypesHeld(int unitId)` — Returns an array with all resource types held by the current unit
- `float xsGetUnitAttributeHeld(int unitId, int attributeId = -1)`
- `bool xsSetUnitAttributeHeld(int unitId, float value, int attributeId = -1)`
- `float xsGetUnitCharge(int unitId)`
- `bool xsSetUnitCharge(int unitId, float value)`
- `float atan2v(vector v)` — equivalent to atan2(xsVectorGetY(v), xsVectorGetX(v))
- `float exp(float x)`
- `float ceil(float x)`
- `float floor(float x)`
- `float bitCastToFloat(int number)`
- `int bitCastToInt(float number)`

### XS: изменено/исправлено
- xsTaskAmount, xsTask, xsRemoveTask functions are no longer limited to scenario games.
- Fixed an issue where object group-targeting tasks added by xsTask function were overwriting the previously added tasks.
- Added support for all task attributes to be set in xsTaskAmount. Refer to Constants.xs.
- `xsGetObjectAttribute` получил опциональный параметр `int damageClass` (getting armor/attack from specific damage classes). Usage: `xsGetObjectAttribute(1, 4, cArmor, cDamageClassPierce);`
- Fixed `float atan2(float y, float x)` to take two parameters.
- Fixed an issue where gaia units would not be affected by effect_amount commands if set_gaia_civilization was used in the script.
- Fixed an issue where the Multiply Resource effect type did not perform the correct action when used in effect amount commands.
- Spawn Unit effect type used in effect amount commands is now able to spawn gaia-only objects.

### Триггеры/константы
- Added new "Modify Object Attribute" and "Modify Object Attribute By Variable" trigger effects (modify stats of specific units on the map with all functionality of "Modify Attribute"; targeted units no longer affected by technologies).
- Added new "Modify Attribute for Class" trigger effect (based on the specified unit class).
- Added new "Research Local Technology" trigger effect (applies tech effect within the marked map area and/or towards selected units only).
- Added new "Add Train Location" trigger effect.
- "Change Object Caption" trigger effect now supports entering custom strings, rather than just string IDs.
- Certain object attributes in the "Modify Attribute" family of effects now support floating-point values.
- Added an option to reset the starting view for the player.
- Added a previously missing technology to detect Wei civilization in "Research Technology" trigger condition.
- Changed the name of Penguin cheat unit to "War Penguin".
- Разрешена модификация большого списка атрибутов юнитов (IDs 81–99, 131–157) через Effect Amount / Modify Attribute / технологии: Special Ability (81), Idle Attack Graphic (82), Hero Glow Graphic (83), Garrison Graphic (84), Construction Graphic (85), Snow Graphic (86), Destruction Graphic (87), Destruction Rubble Graphic (88), Researching Graphic (89), Research Completed Graphic (90), Damage Graphic (91), Selection Sound (92–99 и Event-варианты), Attack Graphic 2 (131), Command/Move/Construction/Transform Sound (132–139), Run Pattern (140), Interface Kind (141), Combat Level (142), Interaction Mode (143), Minimap Mode (144), Trailing Unit (145), Trail Mode (146), Trail Density (147), Projectile Graphic Displacement X/Y/Z (148–150), Projectile Spawning Area Width/Length/Randomness (151–153), Damage Graphics Entry Mod (154), Damage Graphics Total Num (155), Damage Graphic Percent (156), Damage Graphic Apply Mode (157).
- New resources: 203 "Reveal Map" (1 = explored, 2 = fully visible), 204 "Reveal Unit on Map" (positive ID = explore at creation, negative = fully visible); resources 70 и 71 "Source Market or Dock X/Y Coordinate"; Resource 262 "Civilization Name Override" now works correctly.

### RMS
- Constants no longer get rounded to integer values when used in math operations.
- The % (integer modulo) math operation will no longer crash the game when used with a floating-point value between -1 and 1 as the right operand; with 0 as the right operand returns the left operand truncated.
- Fixed an issue where the land and amphibious placeholder objects couldn't be placed on unbuildable land terrains.
- Allowed use of float-format values in land generation commands: land_position, land_percent, left_border, right_border, top_border, bottom_border, circle_radius.
- New land attribute `set_circular_base` — меняет форму land origin с квадратной на круговую.
- New object attribute `avoid_other_land_zones` — restricts objects to land zones different from that of their origin; принимает один целочисленный параметр (push objects away from the edge of the zone); работает только в сочетании с set_place_for_every_player или place_on_specific_land_id.
- New command `water_definition` — выбор конкретного water definition (3D-эффекты и вид воды). Переименования: Preset_Main → Default (0), Preset_China → Dimmed (6), Preset_Backup_WickedWitch → Choppy (4); Preset_Backup_FE1 и Preset_Backup_FE2 удалены. Новые определения: Afternoon (1), Arctic (2), Calm (3), Dark Night (5), Evening (7), Hurricane (8), Moonlit Night (9), Morning (10), Murky (11), Night (12), Noon (13), Ocean (14), Red Tide (15), Stormy (16), Sunrise (17), Sunset (18), Swamp (19), Tropical (20).
- New universal command `create_object_group` (+ декларация `add_object`) — определяет группы объектов, генерируемых случайно.
- New land attribute `land_conformity` — integer от -100 до 100; чем выше, тем больше дополнительные тайлы (land_percent / number_of_tiles) следуют base_size (не создаёт больших отклонений от исходной формы); отрицательные значения препятствуют. Note: currently doesn't function as intended, не полагаться.
- New connections command `create_connect_land_zones` — создаёт связи между двумя конкретными land zones.
- New land attribute `generate_mode` — контролирует позиционирование land, когда land_position не задан; при 1 земли больше не генерируются крестом, а могут появляться в любом месте карты.
- New terrain attribute `spacing_to_specific_terrain` — позволяет террейнам избегать одни террейны, но не другие; максимум 4 на декларацию terrain.
- Random map seed is now displayed in the Objectives screen in single player and finished multiplayer games.
- All original ES map scripts are now available in the official mod (Ensemble Studios Rework).
- Новые объекты для карт: Red Fox (1955), Arctic Fox (1958), Hare (2098, 2099), Arctic Hare (2100), Llama B (1963), Arctic Wolf (1965); новые террейны: Grass, Flowers 1 (122), Grass, Flowers 2 (123), Snow (Soft) (124), Snow (Soft), Light (125), Snow (Soft), Strong (126), Ice (Soft) (127).

## Update 141935 — 2025-04-10 (+ patch notes preview 2025-03-11)
https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-141935/
Полные RMS/XS-секции были опубликованы в preview-посте:
https://www.ageofempires.com/news/a-sneak-peek-at-new-content-coming-to-age-of-empires-ii-definitive-edition/

### XS: добавлено
- `float xsGetObjectAttribute(int32_t playerId, int32_t objectId, int32_t attribute)` — returns the unit attribute value of the specific player's object ID.

### XS: изменено/исправлено
- (в самом посте 141935 других XS-пунктов нет)

### Триггеры/константы
- Added new elevation height options up to 16 (from 7).
- Fixed a crash when using Modify Attribute trigger effect with Graphics ID attributes originally set to -1.
- Added Item ID field to Research Technology trigger effect.
- Script Filename field now accepts file names with up to 100 characters.
- Новые/обновлённые атрибуты юнитов (для Modify Attribute / xsGetObjectAttribute): Attribute 118: Damage Reflection; Attribute 119: Friendly Fire Multiplier; Attribute 120: HP based regeneration (% max HP в минуту); Attribute 126: Availability (limited training с LinkedUnits.json); Attribute 127: Disabled (flags 1/2/4); Attribute 128: Attack Priority; Attribute 129: Invulnerability Level; Attribute 68: Vanish Mode new Flag 2; Special Ability new mode 7 (building placed directly on top of the unit).
- Новые ресурсы: 288: Pasture Food Amount, 289: Pasture Animal Count, 290: Pasture Herder Count.
- Reload Time / Shown Reload Time теперь контролируют faith/конверсию (раньше resources 35 и 83).
- Modding: campaign json "Condition" block для SequenceItems (типы "CampaignVariable" — Persistent Variable из "Store Key Value" trigger effect, "PlayerSetting").

### RMS
- Added the ability to use simple math operators (addition, subtraction, multiplication and division). Handles floating-point input values, but the final result will be rounded to the nearest whole number.
- All numeric constants in RMS files are now in floating-point format. (This makes the effect_percent command obsolete.)
- New object attribute `require_path` — только если у объекта есть проходимый путь к spawn origin; принимает один целочисленный параметр (требуется и прямая видимость).
- The attribute `find_closest` now works more appropriately in conjunction with set_circular_placement and enable_tile_shuffling.
- min_distance_to_players and max_distance_to_players no longer make objects avoid neutral lands.
- Added several new colour moods (Arctic, Brumous, Darkness, Evening, Misty, Murky, Rainforest, Savannah, Spring, Steppes, Summer, Swamp, Twilight).
- Новые невидимые placeholder-объекты для карт (on- и off-tile варианты): Generic Placeholder (1543, 1902), Land Placeholder (1544, 1905), Amphibious Placeholder (1545, 1900), Hybrid Placeholder (1546, 1912), Water Placeholder (1547, 1921).
- Новые деревья: Lush Bamboo (1984), Asian Pine (2016), Peach Blossom (2017), Willow (2025), Asian Maple Green (2027), Asian Maple Autumn (2028); пни: 1536–1538, 1542; срубленные: Felled Tree (1539), Felled Bamboo (1540), Felled Baobab (1541), Felled Lush Bamboo (1542).
- Новые камни: Pillar Rock (2008), Limestone Rock (2009), Panda Rock (2082); хищники: Black Bear (2089), Polar Bear (2090), Arabian Wolf (2091) (Bear→Brown Bear, Wolf→Grey Wolf переименованы); Chicken (2083, 2085, 2087); Argali (1896); Wild Chicken (2084, 2086, 2088); Red Crowned Crane (2026); Wild Horse B/C/D/E (2092–2095), Monkey (2096), Wild Penguin (2097); Villager male (1875) / female (1876) без автогатеринга в Empire Wars; временные revealer'ы: Small 4 LOS (1872), Medium 10 LOS (1873), Large 20 LOS (1874).
- Новые террейны: Lush Bamboo Forest (113), Shallow Water Yellow (114), Shallows Yellow (115), Deep Water Yellow (116), Pasture (117), Pasture Dead (118), Pasture 0% (119), Pasture 33% (120), Pasture 66% (121); новый тип скал: Limestone Cliff (4).

## Update 128442 — 2024-11-14 (Chronicles: Battle for Greece)
https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-128442/

### XS: добавлено
Note: These features are currently only supported in Custom Scenarios.
- `xsTaskAmount(taskFieldId, float value)` — Specifies values to use for the xsTask or xsRemoveTask functions
- `xsTask(masterUnitID, actionType, targetMasterUnitID, playerID)` — Inserts a new task to a unit
- `xsRemoveTask(masterUnitID, actionType, targetMasterUnitID, playerID)` — Removes a task from a unit
- Пример (аура для героя Artaphernes): xsEffectAmount(cSetAttribute, 2308, 63, 32, -1); xsTaskAmount(0,2)/xsTaskAmount(1,1)/xsTaskAmount(2,10.0)/xsTaskAmount(4,9)/xsTaskAmount(5,38 — битовые флаги ауры: 2 circular + 4 visible + 32 translucent)/xsTaskAmount(6,4 — Target Diplomacy); xsTask(2308, 155, <unitId>, 1) — task 155 = aura/power up.

### Триггеры/константы
- Added 20 blank technologies with no effect for use as custom techs.
- Persistent Variables — новый тип переменных между сценариями кампании, три новых триггера (только внутри кампании): Store Key, Load Key, Delete Key.
- Set Object Cost — sets an attribute cost for a specified object.
- Change Technology Icon / Change Technology Hotkey.
- Modify Variable By Resource / Modify Variable By Attribute.
- Change Player Color.
- Task Object trigger now allows unit formations to be set.
- Data: Full Tech Mode technology attribute flag 26 (только при Antiquity Mode); Type technology attribute flags 32 (building-specific upgrade) и 33 (repeatable technology); Technology cost deduction type flag 2; Store Mode type 16; Technology effect types 200 (Set attribute for local building) и 201 (Add/subtract attribute for local building); Task 133 speed charge; Linked Techs (linkedTechs.json); Eras (eras.json).

### RMS
- ANTIQUITY_MODE flag added to RMS scripts, based on the status of the in lobby toggle.

## Update 107882 — 2024-03-13 (Victors and Vanquished)
https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-107882/

### XS: изменено/исправлено
- The XS error checker now checks for a greater variety of syntax errors, and the level of detail in error messages has been increased.
- The x256tech script has been updated to its final version, and a variant called x9tech is now available.

### Триггеры/константы
- Resources tracking player stats (such as P1 Kills, Razed by P3, etc.) now support Gaia and have been relocated to the 300-499 range. Any existing references in older Scenario Triggers will update automatically, and the XS Constants for such resources will use new values.
- The game can now boot directly into the Scenario Editor if the EDITOR launch parameter is specified (can be combined with e.g. LAUNCH_GAME_VARIANT=AOE1).

### RMS
- Added MORE_MAP_SIZES launch parameter to unlock new map sizes in Editor and Lobbies: Miniature [80], Giant [252], Massive [276], Enormous [300], Colossal [320], Incredible [360], Monstrous [400]. Note that some standard random maps may not correctly support the new map sizes yet.
- Changed the name of the map size with 240 tiles dimension from Giant to Huge.

## Update 99311 — 2023-12-11
https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-99311/

### XS: изменено/исправлено
- нет

### Триггеры/константы
- Added new trigger effects: "Create Object Armor", "Create Object Attack", "Modify Attribute By Variable".
- "Change Object Civilization Name", "Change Object Player Color" and "Change Object Player Name" trigger effects now reset on the object ownership change.
- "Replace Object" trigger effect now correctly transfers the HP percentage of the original object instead of the absolute HP value.

### RMS
- нет изменений

## Update 95810 (preview) — 2023-10-30
https://www.ageofempires.com/news/preview-age-of-empires-ii-definitive-edition-update-95810/

### XS: изменено/исправлено
- нет

### Триггеры/константы
- The maximum length of text that can be entered in a Script Call effect box has been increased from 256 to 500,000.
- Added 10 placeholder technologies without effects (need to be enabled by a trigger effect and assigned location/button ID).
- Modding: Resource 270 — repair cost of siege weapons and ships; Resource 271 — repair cost of buildings; Resource 272/273 — damage received from higher/lower elevation; Resources 279–283 — военная конверсия и spawn garrisoned.

### RMS
- Added the new `cliff_type` parameter for cliff generation — выбирает тип скал: CT_GRANITE (default), CT_DESERT, CT_SNOW.

## Update 93001 — 2023-09-06
https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-93001/

### XS: изменено/исправлено
- (раздел Scripting/AI) Fixed an issue where a constant may not actually be redefined.

### Триггеры/константы
- Fixed an issue when applying Modify Attribute to Secondary Projectile no longer causes a crash.
- Game no longer crashes upon resetting trigger effect location in Scenario Editor.
- Fixed an issue where unit upgrades in a unit line would not benefit from modify attribute correctly.

### RMS
- нет изменений

## Update 87863 — 2023-06-27
https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-87863/

### XS: добавлено (XS support for AI)
- Added XS support for AI, allowing AIs to store more information (including between games) and perform more advanced mathematical calculations:
  - `use (include "script.xs")` — load an XS script from AI
  - `xs-script-call "function name"` — call an XS function (fact or action; no need to defconst the function name)
  - `xsGetGoal(integer)` / `xsGetStrategicNumber(integer)` — read a goal / SN
  - `xsSetGoal(goal ID, value)` / `xsSetStrategicNumber(sn ID, value)` — set a goal / SN (value must be an integer)
  - AIs can store information using commands such as `xsWriteInt()`. Важно: перед открытием файла `xsSetContextPlayer()` должен быть выставлен в номер игрока и сброшен обратно в -1 перед закрытием файла.

### Триггеры/константы
- Enable Technology trigger no longer disables technology for other players.
- Modifying Idle Graphic IDs of a unit by trigger or technology effects now immediately updates the current graphic/animation.

### RMS
- нет изменений

## Update 83607 — 2023-05-15
https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-83607/

### XS: изменено/исправлено
- Fixed an issue where units were not getting upgraded correctly by xsEffectAmount command.

### Триггеры/константы
- Enable Tech command type used by Effect Amount (ID 7, -8 for gaia) has been replaced by Spawn Unit, which functions similarly to the data version of this effect type.
- Modify Technology effect is now able to set the state of technology (disable, enable, force enable or research the specified technology ID).
- Expanded the functionality of Full Tech Mode technology attribute to restrict the technology to the specific game setting (отрицательное значение — отключить): 2: Random Map, 3: Death Match, 4: Empire Wars, 5: Wonder Race, 6: Defend the Wonder, 7: Sudden Death, 8: King of the Hill, 9: Capture the Relic, 10: Battle Royale, 11: Regicide, 12: Turbo Mode, 13: Scenario/Campaign, 14: Regicide Mode, 15: Scenario Editor, 16: D3, 17: Solid Farms, 18: Shared Exploration, 19–25: Starting Resources.
- Added task 155 (power up/aura): Unit or Class, Search wait time (attribute ID: attack 9, reload 10, work rate 13, regen 109, speed 5), Work value 1/2, Work range, Target diplomacy, Unused flags (1 multiplier, 2 round area, 4 range indicator).
- Разрешена модификация дополнительных атрибутов юнитов через Effect Amount / Modify Attribute (Can be Built on, Foundation Terrain, Graphic IDs и др.); новые атрибуты: Minimum/Maximum Conversion Time Modifier, Conversion Chance Modifier, Formation Category/Spacing, Blast Damage; removed Blast Attack Level attribute flags 4, 8, 16, 32 (заменены Blast Damage).

### RMS
- `nomad_resources` parameter now reimburses the Town Center's cost (with the cost bonuses taken into account) instead of always adding 275 wood and 100 stone to the starting resources.
- (Modding/Data) It is now possible to define custom random map pools through maps.json in data mods; mod-specific loading symbols for random maps in the string 8739.

## Update 81058 — 2023-04-11
https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-81058/

### XS: изменено/исправлено
- `xsResearchTechnology`, `xsGetObjectCount`, `xsGetObjectCountTotal` and `xsGetTechCount` functions can now be used in Random Map game modes.

### Триггеры/константы
- Fixed an issue where Change Color Mood trigger effect was affecting the following scenario editor sessions.
- Value of resource 269 now determines the ID of an additional effect which fires when any technology gets researched.
- Added the new Loot Object task (ID 154).

### RMS
- Added the new `generate_for_first_land_only` parameter for object generation. Objects with this parameter will generate only for the first player land.
- Added the new `set_facet X` parameter for object generation, where X is the facet ID the created object will have.
- Added the new `override_map_size X` parameter, where X is the map dimension which will be used instead of the default dimension for the selected map size. The minimum allowed dimension is 36, the maximum is 480.

## Update 78174 — 2023-03-07
https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-78174/

### XS: изменено/исправлено
- нет

### Триггеры/константы
- Fixed armor and attack attributes in custom scenarios, retroactively fixing custom-made maps.
- Fixed an issue where Set Object did not always work properly.

### RMS
- нет изменений

## Update 42848 — 2020-11-17
https://www.ageofempires.com/news/aoe2de-update-42848/

### XS: добавлено
- Первое появление XS: "New and improved functionality for the Scenario Editor: including new triggers and .xs scripting support!" (детального списка функций в новостном посте нет — детали публиковались на форуме).

### RMS
- нет изменений (в посте)

## Update 40220 — 2020-08-24
https://www.ageofempires.com/news/aoe2de-update-40220/

### XS
- нет

### RMS
- Placement of resources in random maps is no longer constrained to cardinal directions.

---
# Посты обновлений, просмотренных за период, БЕЗ XS/RMS-изменений (не включены выше)

2026: 174992 (minor), 170934
2025: 162286 (только новые карты/Stonehenge eye candy), 160062 (minor, RoR mods path), 155976 (minor, data mods strings), 153638 (minor), 147949, 145651, 144358, 143421
2024: 133431 (minor), 130746 (minor), 125283 (preview к 128442; только AI goals 512→16000 и AGE), 118476 (wololo map update — только фиксы конкретных карт), 117204, 114480 (hotfix), 113358 (hotfix — только фиксы генерации Empire Wars), 111772 (новые карты RBW + AI scripting), 108769, 104954
2023: 90260 (только фикс Erase в редакторе), 85208 (hotfix), 85614 (hotfix), 82587 (hotfix)
2022: 73855, 66692, 63482, 62085, 61321, 59165, 58259
2021: 56005, 54480, 53347 (hotfix), 51737, 50292, 47820, 46295 (фикс чёрных тайлов в редакторе), 45185 (hotfix), 44725
2020: 43210 (hotfix), 41855, 40874, 39515 (hotfix), 39284, 37906 (hotfix), 37650, 36906, 36202, 35584, 35209 (hotfix), 34793 (hotfix), 34699, 34397 (hotfix)
2019: 34223 (hotfix), 34055, age2-december-update, 33315, 33164, 33059, 32911

Примечание: посты 2019–2022 годов — короткие анонсы; детальные патчноуты того периода публиковались на forums.ageofempires.com, поэтому XS/RMS-детали (кроме упомянутых в самих постах) там не отражены.
