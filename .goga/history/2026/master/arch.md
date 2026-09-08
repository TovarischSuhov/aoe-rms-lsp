# Architecture Plan — claudemd-compliance-migration

Topic: `claudemd-compliance-migration` — миграция проекта на правила CLAUDE.md.
Path: `.goga/history/2026/master/arch.md` (this file).
Границы плана: только артефакты CODEMANIFEST / `.usages/`. Кодовые и инфраструктурные
изменения (CI, README, kb/gen.go, тесты) — в исполнительном плане (plan.md), не здесь.
Источник: task.md (та же директория), три аудита 2026-09-09.

## Implementation Order

1. **kb** (лист, deps: –) — единственная ячейка с изменением публичной поверхности
   (`GenKB` + сигнатура `ExtractRmsCommands`); идёт первой: от неё зависит cmd/kbgen.
2. **common** (лист) — дедупликация аннотаций; независимо, но по порядку leaves→root.
3. **rms**, **xs** (зависят от common) — дедупликация + чистка `Algorithm:`.
4. **analysis** (common/kb/rms/xs) — чистка `Algorithm:`.
5. **hints**, **complete** (common/kb/rms/xs) — дедупликация + `.usages`.
6. **include** (common/rms/xs) — только `.usages/closure.md`.
7. **server** (все) — чистка `Algorithm:` + `.usages/lifecycle.md`; последняя, корень.

Все изменения аннотаций не затрагивают импорт-граф; порядок внутри групп 2–6 свободен.

## Artifacts

### Cell: kb — MODIFY

**CODEMANIFEST diff:**

1. Глобальные `Annotations:` — добавить абзац в конец существующего блока:
   ```
   Утилиты сборки данных (`GenKB`, `ExtractRmsCommands`) — оффлайн-этап подготовки
   данных, вне рантайма: контракт Store («No IO after construction») их не покрывает.
   ```
2. Body — заменить сигнатуру и аннотацию `ExtractRmsCommands`:
   - сигнатура: `"ExtractRmsCommands(path: string, log: Logger) -> commands: []Command, err: error"`
   - добавить параметр-строку после `path`:
     `` `log`: инжектированный логгер сборки (в Go — *slog.Logger; nil → slog.Default())``
   - Constraints, заменить «…писать WARN в slog, не прерывать…» на
     «…писать WARN в `log`, не прерывать…» (остальной текст без изменений)
3. Body — добавить после `ExtractRmsCommands` новую Routine (полное содержание):
   ```yaml
   "GenKB(refDir: string, dataDir: string, log: Logger) -> err: error":
     location: gen.go
     annotations: |
       Генерация файлов данных kb из исходников документации (оффлайн-сборка базы).

       `refDir`: корень исходников документации (functions.json, constants.json,
       zetnus-гайд, changelog — layout по `kbdata`)
       `dataDir`: целевой каталог данных
       `log`: инжектированный логгер сборки (в Go — *slog.Logger; nil → slog.Default())
       `err`: ошибки чтения/записи; обёрнуты с путём файла

       Algorithm:
       1. Прочитать из `refDir` источники по `kbdata`: функции и константы —
          адаптацией JSON ugc-guide; команды — `ExtractRmsCommands`(гайд, `log`)
       2. Пополнить SinceUpdate по changelog из `refDir`
       3. Проверить уникальность имён внутри каждого набора; коллизия — ошибка
       4. Записать три файла данных в `dataDir` по формату `kbdata`

       Requirements:
       - сериализация детерминирована: повторный запуск на тех же исходниках
         даёт идентичные файлы (стабильный порядок ключей)

       Constraints:
       - утилита сборки, не вызывается в рантайме сервера
       - исходники в `refDir` не изменяются
   ```
4. Footer — Description: «База знаний AoE2 RMS+XS: встроенные JSON, lookup API,
   экстракция из Zetnus и генерация данных.»

**`.usages/` files:**

- `kb/.usages/data-pipeline.md` — REPLACE (полное новое содержание):
  ```markdown
  # KB Data Pipeline — regenerating embedded JSON

  Domain: rebuilding kb/data/*.json from local sources in docs/ref/.
  Target audience: maintainers updating the knowledge base.

  ## Sources (all local, no network)

  - docs/ref/ugc-guide/xs/functions/functions.json → xs-functions.json
  - docs/ref/ugc-guide/xs/constants/constants.json → xs-constants.json
  - docs/ref/zetnus-rms-guide.txt → rms-commands.json (via ExtractRmsCommands)
  - docs/ref/aoe2de-xs-rms-changelog.md → since_update enrichment

  ## Regenerate

  Single entry point — kb.GenKB adapts functions/constants JSON, extracts RMS
  commands, enriches since_update, validates name uniqueness and writes all
  three files deterministically (sorted keys):

  ```go
  err := kb.GenKB("docs/ref", "kb/data", slog.Default())
  if err != nil {
      return fmt.Errorf("regenerate knowledge base: %w", err)
  }
  ```

  cmd/kbgen wraps exactly this call (flags only, no logic of its own).

  Preconditions:
  - Duplicate names within one file are a build error — resolve, do not skip.
  - NewStore() must pass after regeneration (run kb tests).

  ## Mining on regeneration (signature help)

  Extraction mines structured kind/range from Desc prose via kb.MineKindRange:
  bounded entries get Range ("number (0-99)" → 0..99); empty-Kind entries get
  the mined kind when prose has one. Flag attributes (empty Desc, e.g.
  set_circular_base) stay name-only — nothing to mine. Ambiguous fragments are
  skipped with a WARN to the injected logger — check warnings after a run that
  changes the guide. After regeneration run kb tests: NewStore re-mines
  fill-when-empty, so regenerated JSON and load-time mining must agree
  (idempotent rule).
  ```
  (внешний блок ```markdown выше — только для этого плана; файл начинается с `# KB Data Pipeline`)

- `kb/.usages/lookups.md` — две правки:
  1. «…inject it (constructor DI per `conventions`).» → «…inject it as an explicit
     constructor parameter.»
  2. Секция «## Mined kind/range on CommandArg (signature help rendering)» →
     заголовок «## Mined kind/range on CommandArg» и текст:
     «CommandArg carries structured Range (min/max strings, "" when unmined) and
     Kind filled at load when the extractor left it empty. Kind/Range are raw
     strings from the guide prose — consumers render them per their own contract
     (rendering rules live in the hints cell).»

### Cell: common — MODIFY (только аннотации)

CODEMANIFEST diff — дедупликация: из type-аннотаций `Pos`, `Range`, `Diagnostic`,
`Symbol` удалить параметровые строки (`line/column/offset`, `start/end`,
`r/severity/message/code`, `kind/name/r/selection`); уникальное сливается в
property-аннотации:
- `Diagnostic.Severity`: «Важность: 1 error / 2 warning / 3 info / 4 hint (по LSP).»
- `Symbol.Kind`: добавить словарь «(xs: function/variable/rule/event/extern;
  rms: section/command/xs)»
- `Symbol.Selection`: «Диапазон имени (⊆ Range).» (уже есть — без изменений)

**`.usages/positions-and-diagnostics.md`** — правки:
1. `(rms.Parse, xs.Parse, Analyzer)` → `(rms.Parse, xs.XsParse, Analyzer)`
2. Закрыть незакрытый код-фенс блока Positions (после строки `if r.Contains(...)`)
3. Закрыть незакрытый код-фенс блока Diagnostics (после `}` литерала)

### Cell: rms — MODIFY (аннотации + практика)

CODEMANIFEST diff — дедупликация `ArgSite`, `Include` (как в common):
- `ArgSite.Kind` property: «arg (курсор на позиционном аргументе) / attr (на
  атрибуте — имени или значении) / none (на имени команды или неоднозначно).»
- `ArgSite.Index`, `ArgSite.Name`, `ArgSite.Stmt` — без изменений
- `Include.Range` property: «Диапазон аргумента-пути (не всей директивы — hit-test
  курсора и диагностика точнее).»

**`.usages/includes.md`** — удалить секцию «## By-name references» целиком
(дублирует rms-parsing.md, чужой домен).

### Cell: xs — MODIFY (аннотации)

CODEMANIFEST diff:
1. `CallAt` Algorithm шаг 4: «`pos` на имени callee или между именем и «(» →
   CallSite{callee, argIndex=0, onArg=false}» → «`pos` на имени callee или между
   именем и «(» → вернуть `CallSite` с argIndex=0 и onArg=false»
2. `CallSite` — дедупликация: параметровые строки удалить; полноту слить в
   property-аннотации:
   - `ArgIndex`: «0-based ординал аргумента под курсором (валиден при onArg=true:
     сразу после «(» — 0; после k запятых верхнего уровня — k).»
   - `Callee`, `OnArg` — без изменений

### Cell: analysis — MODIFY (аннотации)

CODEMANIFEST diff — `AnalyzeRms` Algorithm шаги 2–3: «lookup через `lookups`
(Store.Command)» → «lookup команды через `Store` по `lookups`»;
«неизвестные атрибуты (Store.Attribute)» → «неизвестные атрибуты через `Store`».
Semantics не меняются.

### Cell: hints — MODIFY (аннотации + практика)

CODEMANIFEST diff — `Hint`: дедупликация параметров → property-аннотации
(как в common; уникальные формулировки из параметров сливаются в свойства).

**`.usages/computing.md`** — Preconditions: «…map it to nil, nil in the protocol
handler, never to a guessed hint.» → «…never a guessed hint.»

### Cell: complete — MODIFY (аннотации + практика)

CODEMANIFEST diff — `Candidate`: дедупликация параметров → property-аннотации.

**`.usages/completing.md`** — Preconditions: «Empty slice is the designed silence —
map it to an empty CompletionList, never to an error.» → «Empty slice is the
designed silence — never an error.»

### Cell: include — MODIFY (практика)

**`.usages/closure.md`** — `docs := server.NewStore()` → `docs := server.NewDocStore()`
(комментарий про Text сохранить).

### Cell: server — MODIFY (аннотации + практика)

CODEMANIFEST diff — переписывание код-шагов `Algorithm:` в язык действий
(semantics неизменны; ссылки — только на типы из Imports/Usages):
- `DidOpen` шаг 1: «DocStore.Put(uri, text, version)» → «сохранить текст и версию
  в `DocStore`»; шаг 2: цепочки «XsParse + AnalyzeXs(xsFile, ExternalDecls(""))» →
  «пропустить блок через `XsParse` и передать в `Analyzer` с внешними декларациями
  `Closure`»
- `DidChange` шаг 2: «DocStore.Put; пересчитать (как в DidOpen)» → «сохранить в
  `DocStore`; пересчитать (как в DidOpen)»
- `Completion` шаги 2–3: «completer.XsAt(file, pos, decls)» → «получить кандидатов
  у `Completer` для позиции и внешних деклараций»; «Parse(text); позиция внутри
  XsBlock (Range.Contains) → трансляция … + XsParse(block.Code) + ExternalDecls("")
  → XsAt» → «разобрать через `Parse`; для позиции внутри `XsBlock` перевести в
  координаты блока, пропустить код через `XsParse` и получить кандидатов у
  `Completer`»
- `SignatureHelp` шаги 2–3: аналогично (`computer.XsAt(...)` → «получить хинт у
  `Computer`»)
- `Serve` шаг 2: «protocol.NewServer(ctx, srv, stream); ждать conn.Done()» →
  «поднять сервер протокола по `lsp-protocol` и дождаться завершения соединения»

**`.usages/lifecycle.md`** — правки:
1. «## Editor configs (see task README)» → «## Editor configs»
2. Закрыть код-фенс блока Entrypoint
3. «(prefer utf-8 when offered, per `lsp-protocol`)» → «(prefer utf-8 when offered)»

## Dependency Map

```
common ◄── rms ◄──┬── analysis ◄──┬── server
       ◄── xs ◄───┤               │        ▲
kb (лист) ───────►├── hints ──────┤        │
          └──────►┴── complete ───┘        │
                 include (common,rms,xs) ──┘
```
Изменений графа нет. `GenKB`/`ExtractRmsCommands` потребляются только cmd/kbgen
(не ячейка, вне графа).

## Verification Checklist

- [ ] `goga lint` — 0 ошибок после каждой правки манифеста
- [ ] `goga contract kb` — `GenKB` появилась с парой сигнатур; `ExtractRmsCommands`
      показывает `(path: string, log: Logger)`; остальные ячейки без новых расхождений
- [ ] kb: `gen.go` экспортирует `GenKB(refDir, dataDir string, log *slog.Logger) error`;
      WARN-ы идут в инжектированный логгер (nil → slog.Default())
- [ ] Дедупликация: ни в одном манифесте параметр конструктора не описан дважды
      (type-аннотация + property-аннотация)
- [ ] `Algorithm:` шаги не содержат вызовного синтаксиса/композитных литералов
- [ ] `.usages`: нет ссылок на другие практики (`conventions`, `lsp-protocol`),
      нет битых имён (`NewDocStore`, `XsParse`), фенсы код-блоков закрыты,
      includes.md без секции by-name
- [ ] `goga schema` — граф импортов неизменен
