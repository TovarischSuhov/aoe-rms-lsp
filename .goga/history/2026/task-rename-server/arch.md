# Architecture Plan — rename-сервер

## Topic

rename-сервер: PrepareRename/Rename с кросс-файловой склейкой rename-сайтов
по include-замыканию (задача `task-rename-server`, зеркало #44, подзадача 4
эпика v1.0.0; предыдущая — #43 rename-парсеры, смержена).

Формулировка: `.goga/history/2026/task-rename-server/task.md`.
План: `.goga/history/2026/task-rename-server/arch.md`.

## Implementation Order

| # | Артефакт | Тип | Обоснование порядка |
|---|----------|-----|---------------------|
| 1 | `internal/rms` | modify | лист (зависимость — только common, не меняется); поставляет by-name ident-сайты для склейки |
| 2 | `internal/include` | modify | зависит от rms (новый метод уже импортированного типа) и xs (новый тип в Imports); склейка — ядро задачи |
| 3 | `internal/server` | modify | корень; хендлеры над `Resolver.RenameSites` |
| 4 | cook + usage-обновления | docs | `lsp-protocol.md` секция Rename; `.usages` ячеек зафиксированы в шагах 1–3 |

Порядок rms → include → server строго по графу зависимостей; новых ячеек
нет; `common`/`xs` — read-only (в xs только тип `RenameSite` добавляется в
Imports include — код xs не меняется).

## Artifacts

Все три ячейки — modify: план фиксирует **дифы** (уже применены на диск в
brainstorm-сессии, `goga lint` 0 errors).

### Cell: `internal/rms` — modify

#### CODEMANIFEST diff

Заголовок (Imports/Usages/Annotations) и футер — без изменений.

**ADD** метод в `RmsFile.methods` (после `RenameSites`, перед `ArgAt`):

```yaml
    "RenameRefs(name: string) -> ranges: []Range": |
      By-name ident-сайты пользовательских символов без требования локальной
      декларации (кросс-файловая склейка rename: у вызывающей стороны нет
      позиции в чужом файле, а константа может быть объявлена в другом файле
      замыкания).

      `name`: имя символа
      `ranges`: name-range локальной декларации (если есть) + ident-вхождения
      имени, по позиции

      Algorithm:
      1. Собрать ident-вхождения `name` из индекса токенов (позиции
         ident/const-значений по `rms_grammar`)
      2. `name` в индексе пользовательских деклараций (шаг 7 Parse) →
         добавить name-range декларации
      3. Отсортировать по позиции

      Requirements:
      - имя объявлено в файле ⇒ результат эквивалентен RenameSites на любом
        сайте этого имени в файле
      - отсутствие имени — пустой список

      Constraints:
      - вхождения в позициях команд/атрибутов/секций не возвращаются
        (позиционная дискриминация RenameSites сохранена)
      - ReferencesAt и References не меняются
```

#### .usages diff

`internal/rms/.usages/rms-parsing.md` — в разделе «Rename sites (rename,
prepareRename)» после существующих Preconditions ADD: by-name абзац
(RenameRefs — ident-вхождения независимо от локальной декларации, для
кросс-файловой склейки), пример `for _, r := range file.RenameRefs(name)`
(«one TextEdit per range; the local #const/#define declaration range is
included when this file declares the name itself»), precondition
эквивалентности RenameSites при локальной декларации.

### Cell: `internal/include` — modify

#### CODEMANIFEST diff

Заголовок: **ADD** тип `RenameSite` в `Imports.Types` из `internal/xs`
(после `Decl`) — для Kind-классификации в аннотациях. Usages и глобальные
Annotations — без изменений.

**ADD** метод в `Resolver.methods` (после `References`):

```yaml
    "RenameSites(ctx: Context, uriArg: string, pos: Pos) -> sites: []Target, found: bool": |
      Сайты переименования биндинга под позицией по всему замыканию
      (LSP prepareRename/rename: сервер строит WorkspaceEdit по сайтам).

      `uriArg`: документ; `pos`: позиция; `sites`: сайты биндинга
      (декларации и ссылки, включая чужие файлы); `found`: false —
      позиция не переименовываема (не-идентификатор, builtin, словарь
      языка)

      Algorithm:
      1. Локальные сайты запрошенного файла: .rms — Parse по
         `rms-parsing`, позиция в inline XsBlock — координаты блока,
         XsParse + RenameSites по `xs-parsing`, трансляция назад;
         иначе RenameSites по `rms-parsing`; .xs — XsParse +
         RenameSites по `xs-parsing`. found=false → found=false
      2. Имя биндинга — текст сайта под `pos` (editor-state); Kind — из
         xs-результата (`RenameSite`.Kind; rms-символы — топ-левел)
      3. Kind=param/local — вернуть только локальные сайты (замыкание
         не строится: скоупированные биндинги файл-локальны)
      4. Топ-левел — корни как References: Closure(uriArg) плюс каждый
         открытый uri из URIs() Source, чьё замыкание содержит
         запрошенный файл
      5. Чужие .rms-файлы корней: RenameRefs(name) по `rms-parsing` →
         Target; inline XsBlock каждого — как шаг 6 с трансляцией
         координат блока
      6. Чужие .xs-файлы корней: References(name), на каждое вхождение
         RenameSites(Start): found=false — внешняя ссылка, включить
         вхождение; Kind топ-левел — включить все сайты (name-based
         merge одноимённых топ-левел биндингов); Kind param/local —
         пропустить (затенение)
      7. Дедуп (URI, Range); сортировка: URI, затем позиция

      Requirements:
      - запрошенный файл и сайт под `pos` входят в результат
      - согласованность с Definition: сайты вхождения — сайты биндинга
        декларации Definition того же вхождения
      - Target.URI — написание цели из резолва данного запроса
      - детерминированность

      Constraints:
      - inline XsBlocks в Closure не входят (обрабатываются через
        owner-файл, как References)
      - builtin-тёзка объявленного имени: вхождения без локального
        биндинга включаются (name-based merge)
```

#### .usages diff

`internal/include/.usages/closure.md` — ADD раздел «Rename sites (rename,
prepareRename)» после «Navigation (definition / references)»: пример
`resolver.RenameSites(ctx, uri, pos)` (found=false → prepareRename молчит
nil,nil; один TextEdit на Target; сайт под pos — диапазон placeholder);
preconditions: param/local файл-локальны, топ-левел name-based merge по
корням References (затенённые вхождения чужого файла исключены), ключи
Changes могут быть неоткрытыми файлами, Target.URI — написание из
резолва запроса, дедуп (URI, Range) + сортировка (URI, позиция).

### Cell: `internal/server` — modify

#### CODEMANIFEST diff

Заголовок (Imports/Usages) — без изменений. Глобальные Annotations —
**ADD** после абзаца SelectionRange:

```
  Rename/PrepareRename следует секции Rename `lsp-protocol`: prepareProvider
  заявлен; непереименовываемая позиция — nil, nil (молчание); Rename —
  plain WorkspaceEdit{Changes} по rename-сайтам замыкания (склейка —
  RenameSites `Resolver` по `closure`), включая неоткрытые файлы;
  невалидный NewName — ResponseError.
```

`Initialize` — **ADD** в перечень возможностей (после SelectionRangeProvider):

```
      RenameProvider — RenameOptions с PrepareProvider;
```

**ADD** методы в `Server.methods` (после `SelectionRange`, перед
`Shutdown`):

```yaml
    "PrepareRename(ctx: Context, params: PrepareRenameParams) -> result: PrepareRenameResult, err: error": |
      Диапазон и placeholder переименования для позиции (stateless,
      read-only); имя метода — по интерфейсу go.lsp.dev (PrepareRename).

      `params`: документ и позиция; `result`: arm PrepareRenamePlaceholder
      или nil; `err`: только протокольные сбои

      Algorithm:
      1. RenameSites у `Resolver` по `closure` для (uri, pos);
         found=false → nil, nil (молчание — конвенция Hover; клиент не
         открывает rename-box)
      2. Сайт под pos: Target с URI запрошенного документа и Range,
         содержащим pos; имя — текст сайта из editor-state (DocStore)
      3. → PrepareRenamePlaceholder{Range, Placeholder: имя} с
         конвертацией positionEncoding

      Requirements:
      - stateless/read-only: состояние сервера не меняется
      - placeholder непуст (имя сайта)
      - детерминированность

      Constraints:
      - arm PrepareRenameDefaultBehavior не используется
    "Rename(ctx: Context, params: RenameParams) -> edit: WorkspaceEdit, err: error": |
      Мультифайловое переименование по замыканию (stateless, read-only);
      имя метода — по интерфейсу go.lsp.dev (Rename).

      `params`: документ, позиция и NewName; `edit`: правки по всем
      сайтам биндинга; `err`: ResponseError при невалидном NewName или
      непереименовываемой позиции

      Algorithm:
      1. Валидация NewName: непустой, [A-Za-z_][A-Za-z0-9_]* — иначе
         ResponseError с сообщением (требование спецификации)
      2. RenameSites у `Resolver` по `closure`; found=false →
         ResponseError «позиция не переименовываема» (при заявленном
         prepareProvider нормальный клиент сюда не попадает)
      3. Каждый Target → TextEdit{Range (positionEncoding), NewText};
         группировка Changes[URI] — включая неоткрытые файлы
      4. → WorkspaceEdit{Changes} (plain-правки, прецедент CodeAction)

      Requirements:
      - stateless/read-only: сервер правки не применяет
      - все сайты замыкания входят в edit
      - детерминированность

      Constraints:
      - без DocumentChanges/resource-операций и ChangeAnnotations
      - словарные конфликты NewName (keyword/builtin-тёзка) не
        проверяются — лексическая валидация только
```

#### .usages diff

`internal/server/.usages/lifecycle.md`:
- в перечислении Advertised capabilities после «selection ranges» —
  «, rename (prepareProvider)»
- ADD раздел «## Rename» после «## Selection ranges»: placeholder из
  сайта под курсором; edit правит все сайты по замыканию включая
  неоткрытые файлы; XS params/locals файл-локальны; топ-левел
  name-based merge, затенённые вхождения не трогаются; непереименовываемое
  → null; невалидное имя — ошибка запроса; словарные конфликты не
  проверяются.

### Cook: `.goga/usages/cooks/lsp-protocol.md` — EXTEND

ADD секция «### Rename and Prepare Rename» после «### Selection Ranges»:
capability `RenameProvider: &protocol.RenameOptions{PrepareProvider:
&[]bool{true}[0]}}`; хендлеры по интерфейсу; `PrepareRenameResult` —
sealed, arm `PrepareRenamePlaceholder{Range, Placeholder}` (3.18),
`nil, nil` = непереименовываемо; `Rename` → `WorkspaceEdit{Changes}`
plain-правки, ключи — в т.ч. неоткрытые файлы; невалидный `NewName` и
непереименовываемая позиция → `ResponseError`. (Применено в этой ветке.)

## Dependency Map

```
internal/common (Range, Pos — не меняется)
        ▲                                   ▲
        │ Imports (без изменений)           │ Imports (без изменений)
   ┌────┴────────────┐              ┌───────┴─────────┐
   │ internal/rms    │              │ internal/xs     │
   │ +RenameRefs     │              │ read-only;      │
   └────────┬────────┘              │ RenameSite ─────┼──► тип в Imports
            │ Imports (существующее)│ include         │
            ▼                       └─────────────────┘
   ┌─────────────────────────────┐
   │ internal/include            │ ◄── новое ребро типов нет:
   │ +Resolver.RenameSites       │     RenameSite добавлен в
   │ +Imports: RenameSite (xs)   │     существующее ребро xs→include
   └────────┬────────────────────┘
            │ Imports (существующее: Resolver, Target)
            ▼
   ┌─────────────────────────────┐
   │ internal/server             │
   │ +PrepareRename, +Rename     │
   │ +Initialize: RenameProvider │
   └─────────────────────────────┘
```

Циклов нет; порядок leaves→root: rms → include → server.

## Verification Checklist

После реализации:

- [ ] rms: `RenameRefs(name)` при локальной декларации ≡ `RenameSites`
      на сайте имени; без декларации — только ident-вхождения;
      команда/атрибут/секция-тёзка не возвращаются
- [ ] rms: регресс — `ReferencesAt`/`References` не изменились
- [ ] include: локальный резолв — found=false на builtin/keyword/строке/
      комментарии/команде/атрибуте/секции сквозно из per-file
      RenameSites парсеров
- [ ] include: param/local — сайты только запрошенного файла; одноимённый
      топ-левел не затронут
- [ ] include: топ-левел xs — вхождения чужого файла без локального
      биндинга включены (found=false), с чужим param/local-затенением
      исключены, с одноимённой топ-левел декларацией — смержены
- [ ] include: rms `#const`/`#define` — декларация + ident-использования
      по всем корням (прямое + обратное направление), inline-XS в
      координатах .rms
- [ ] include: согласованность с Definition на общих фикстурах;
      дедуп (URI, Range), сортировка (URI, позиция)
- [ ] server: Initialize заявляет RenameProvider с prepareProvider
- [ ] server: prepareRename — placeholder = имя сайта под pos;
      непереименовываемое → nil, nil
- [ ] server: rename — WorkspaceEdit Changes по всем сайтам включая
      неоткрытые файлы; невалидный NewName и found=false → ResponseError
- [ ] `goga contract internal/rms`, `goga contract internal/include`,
      `goga contract internal/server` — совпадение сигнатур
- [ ] тесты — под memory cap; финал: `make check` зелёный,
      `goga lint` 0 errors
