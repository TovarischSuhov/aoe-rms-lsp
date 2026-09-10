# Architecture Plan — rename-парсеры

## Topic

rename-парсеры: скоуп-aware rename-сайты в xs/rms (задача
`task-rename-parsers`, зеркало #43, подзадача 3 эпика v1.0.0; следующая —
#44 rename-сервер).

План: `.goga/history/2026/task-rename-parsers/arch.md` (прямой путь:
`goga history path -f arch.md` резолвится в топик `master` — указатель
текущего топика стоит там).

## Implementation Order

| # | Артефакт | Тип | Обоснование порядка |
|---|---|---|---|
| 1 | `internal/xs` | modify | лист (единственная зависимость — common, не меняется); поставляет RenameSite и семантику затенения |
| 2 | `internal/rms` | modify | независим от xs (свои сайты `#const`/`#define`); параллелен xs, но по порядку следования идёт вторым |
| 3 | usage-обновления | docs | фиксируют потребительские практики обеих ячеек |

Ячейки xs и rms независимы — порядок между ними свободен; новых ячеек
нет, `common` read-only.

## Artifacts

Обе ячейки — modify: план фиксирует **дифы** (уже применены на диск в
brainstorm-сессии, `goga lint` 0 errors).

### Cell: `internal/xs` — modify

#### CODEMANIFEST diff

Заголовок (Imports/Usages/Annotations) и футер — без изменений.

**ADD** тип данных после `CallSite` (перед `Decl`):

```yaml
"RenameSite()":
  location: ast.go
  annotations: |
    Сайт переименования: name-range одного вхождения биндинга (данные).

    Requirements:
    - Kind описывает биндинг, а не вхождение: одинаков у всех сайтов
      одного результата RenameSites; Decl=true несут только
      декларационные сайты
    - construct-and-use, без мутации
  properties:
    "Range -> Range": |
      Name-range вхождения (не всей декларации).
    "Kind -> string": |
      Вид биндинга — словарь как у VisibleAt: function/variable/rule/
      event/extern — топ-левел, param/local — скоупированные.
    "Decl -> bool": |
      Вхождение — декларация биндинга.
```

**ADD** метод в `XsFile.methods` (после `References`, перед `Symbols`):

```yaml
    "RenameSites(pos: Pos) -> sites: []RenameSite, found: bool": |
      Сайты переименования биндинга под позицией: резолв как в Definition,
      возвращаются вхождения только этого биндинга (LSP prepareRename/rename;
      серверная склейка — вне ячейки).

      `pos`: позиция в файле
      `sites`: сайты биндинга (декларация Decl=true + ссылки), по позиции
      `found`: false — не-идентификатор, builtin или неизвестное имя

      Algorithm:
      1. Вхождение идентификатора под `pos` (как SymbolAt); нет — found=false
      2. Резолв биндинга — семантика Definition: innermost объемлющая
         декларация (параметр/локаль затеняют топ-левел; init-декларация for
         скоупирована оператором); локальной декларации нет (builtin или
         неизвестное) — found=false
      3. Пройти индекс вхождений имени: каждое вхождение резолвить тем же
         правилом; собрать резолвящиеся к тому же биндингу — name-range,
         декларационные с Decl=true, Kind — вид биндинга
      4. Отсортировать по позиции

      Requirements:
      - согласованность с Definition: если Definition вхождения возвращает
        декларацию D, RenameSites того же вхождения возвращает сайты
        биндинга D
      - found=true ⇒ результат непуст (сайт под `pos` включён)
      - детерминированность

      Constraints:
      - ReferencesAt и References не меняются (синтаксическая семантика
        сохраняется)
      - один проход по индексу вхождений, повторный парс не выполняется
```

#### .usages diff

`internal/xs/.usages/xs-parsing.md` — ADD раздел «Rename sites (rename,
prepareRename)» после Navigation: скоуп-aware семантика (резолв как
Definition: param > local > top-level; for-init скоупирована), пример
вызова с полями sites[].Range/Kind/Decl, found=false случаи; preconditions:
парс первым, Kind описывает биндинг (топ-левел kinds мерджатся между
файлами на сервере, param/local — файл-локальны), одноимённое вхождение
другого (затеняющего) биндинга в результат не входит.

### Cell: `internal/rms` — modify

#### CODEMANIFEST diff

Заголовок и футер — без изменений.

**ADD** шаг в Algorithm `Parse` (после шага 6):

```yaml
    7. Построить индекс пользовательских деклараций: имена #const/#define
       с их name-range (по `rms_grammar`, Directives) — вход для RenameSites
```

**ADD** метод в `RmsFile.methods` (после `References`, перед `ArgAt`):

```yaml
    "RenameSites(pos: Pos) -> ranges: []Range, found: bool": |
      Сайты переименования пользовательского символа под позицией: имена
      #const/#define — декларация и ident-использования (LSP
      prepareRename/rename; склейка по замыканию — вне ячейки).

      `pos`: позиция в файле
      `ranges`: name-range декларации + ident-вхождения имени, по позиции
      `found`: false — словарь языка (команды/атрибуты/секции), прочие
      директивы, строки, комментарии, builtin-константы kb

      Algorithm:
      1. Слово-токен под `pos`; нет или не в ident-позиции — found=false
      2. Переименовываемо: декларация #const/#define или ident-вхождение,
         чьё имя есть в индексе пользовательских деклараций; иначе found=false
      3. Собрать ident-вхождения имени + name-range декларации
      4. Отсортировать по позиции

      Requirements:
      - вхождения в позициях команд/атрибутов не считаются сайтами константы:
         коллизия #const-имени со словарём языка решается позиционно
      - found=true ⇒ результат непуст
      - детерминированность

      Constraints:
      - ReferencesAt и References не меняются
      - индекс деклараций строится при парсинге (шаг 7 Parse), не при запросе
```

#### .usages diff

`internal/rms/.usages/rms-parsing.md`:
- в разделе Navigation уточнена строка про references: «RMS has no local
  declarations — all occurrences are equal (for rename, #const/#define
  declarations are discriminated — see Rename sites)»
- ADD раздел «Rename sites (rename, prepareRename)» после Navigation:
  пользовательские символы `#const`/`#define`, позиционная дискриминация
  (`#const players 5` не делает команду `players` переименовываемой),
  пример вызова, found=false случаи; preconditions: индекс при парсинге;
  inline-XS — делегация xs-парсеру над XsBlock.Code со сдвигом на
  Range.Start (прецедент — semantic tokens в server)

## Dependency Map

```
internal/common (Range, Pos — не меняется)
        ▲                                   ▲
        │ Imports (без изменений)           │ Imports (без изменений)
   ┌────┴────────────┐              ┌───────┴─────────┐
   │ internal/xs     │              │ internal/rms    │
   │ +RenameSite     │              │ +Parse шаг 7    │
   │ +RenameSites    │              │ +RenameSites    │
   └─────────────────┘              └─────────────────┘
   новых межъячеечных рёбер нет; циклов нет
   потребитель склейки — #44: server/include (References(name) → pos →
   RenameSites(pos); фильтр топ-левел биндингов по Kind)
```

## Verification Checklist

После реализации:

- [ ] xs: `RenameSites` согласован с `Definition` — общие фикстуры:
      для каждого вхождения `Definition(pos) → D` ⇔ `RenameSites(pos)`
      возвращает сайты биндинга D
- [ ] xs: затенение — топ-левел `x` + локаль `x`: сайты локали не
      включают вхождения топ-левела и наоборот; for-init скоуп
- [ ] xs: builtin/неизвестное/строка/комментарий/оператор → found=false
- [ ] xs: found=true ⇒ результат непуст, Decl=true у декларации,
      Kind одинаков у всех сайтов
- [ ] rms: `#const NAME value` → декларация + все ident-использования;
      команда/атрибут/секция/directive-не-константа → found=false
- [ ] rms: коллизия `#const players 5` — вхождение команды `players`
      сайтом не является
- [ ] регресс: ReferencesAt/References поведение не изменилось
      (существующие тесты зелёные без правок ожиданий)
- [ ] `goga contract internal/xs`, `goga contract internal/rms` —
      совпадение сигнатур
- [ ] тесты — под memory cap; финал: `make check` зелёный,
      `goga lint` 0 errors
