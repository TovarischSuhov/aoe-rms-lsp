# Design Document: `cross-file-navigation`

Кросс-файловая навигация и резолв include-ов в aoe2-lsp. Источник контрактов:
`docs/arch/cross-file-navigation.md` (материализованы в коммите `90d725e`).
Задача: `docs/tasks/cross-file-navigation.md`.

## Contract Changes

### Changed CODEMANIFEST Files

- `rms/CODEMANIFEST` — тип `Include` (Path+Range аргумента); `RmsFile.Includes`
  `[]string`→`[]Include`; +`XsIncludes []Include`; +`References(name)`;
  Parse-Algorithm шаг 5 (директивы с аргументом-путём)
- `xs/CODEMANIFEST` — +`XsFile.References(name)`
- `include/CODEMANIFEST` — **новая ячейка**: `Source`, `Resolver`, `Closure`,
  `RmsEntry`, `XsEntry`, `ResolvedInclude`, `MissingInclude`, `Target`
- `analysis/CODEMANIFEST` — `AnalyzeXs(file, externals []Decl)`
- `server/CODEMANIFEST` — Imports +include(5 типов + usage `closure`) +`Decl`
  (из xs); `DocStore.Text`/`DocStore.URIs`; `Definition`/`References` через
  `Resolver`; `DidOpen`/`DidChange` с missing-include и externals; Description

### New Entities

- `Include` — rms/ast.go — директива-подключение (путь + диапазон аргумента)
- `Source` — include/source.go — интерфейс editor-state (Text, URIs)
- `Resolver` — include/resolver.go — замыкание + навигация (Closure,
  Definition, References)
- `Closure` — include/closure.go — результат замыкания (+ExternalDecls)
- `RmsEntry`/`XsEntry` — include/closure.go — URI+AST единицы замыкания
- `ResolvedInclude`/`MissingInclude` — include/closure.go — резолв/пропуск
- `Target` — include/closure.go — точка навигации (URI+Range)

### Changed Entities

- `RmsFile` — Includes типизирован; +XsIncludes; +References(name)
- `XsFile` — +References(name)
- `Analyzer.AnalyzeXs` — +параметр externals
- `DocStore` — +Text, +URIs (удовлетворение `Source`)
- `Server` — Definition/References делегируют Resolver; конвейер диагностики
  расширен (missing-include, externals)

### Deleted Entities

Нет.

### Usages and Annotations Changes

- `rms` Annotations: абзац о директивах подключений
- `include` Annotations: новые (conventions, lsp-protocol, практики Imports)
- `server` Annotations: `closure` в списке Imports-практик; абзацы о Resolver
  и Decl-прокидывании
- usage-файлы: созданы `rms/.usages/includes.md`, `include/.usages/closure.md`;
  расширены `rms-parsing.md`, `xs-parsing.md`, `checks.md`, `lifecycle.md`
  (всё — в коммите `90d725e`)

## Applied Fixes

### Fixed CODEMANIFEST Defects

- `include/CODEMANIFEST` (Phase 4 трассировки, 2026-09-07):
  `Resolver.References` искал только по замыканию запрошенного файла —
  сценарий 3 задачи (References из `lib.xs` → вхождения в включающем
  `main.rms`) не покрывался: замыкание `lib.xs` = {lib.xs}, обратное
  направление отсутствовало.
  **Fix (утверждено пользователем):** `Source` получил метод
  `URIs() -> []string`; Algorithm `References` получил шаг «Корни поиска:
  запрошенный uri + открытые uri, чьё замыкание содержит запрошенный файл»
  и дедуп по (URI, Range); `DocStore` (server) получил симметричный `URIs`;
  `include/.usages/closure.md` и сценарий 3 задачи обновлены. Причина:
  противоречие контракт↔сценарий (interface↔interface consistency).

## Entity Interaction and Data Flow

### Interaction Diagram

```
                       editor (LSP client)
                              │ stdio (jsonrpc2)
                              ▼
┌──────────────────────── server ────────────────────────┐
│ Server                                                  │
│  ├─ DocStore ──── реализует ────┐                       │
│  │   Put/Get/Text/URIs/Remove   │                       │
│  └─ analyzer: analysis.Analyzer │                       │
│                                  ▼                       │
│            include.Resolver(source = DocStore)           │
│             ├─ Closure(uri)  → Closure                    │
│             │    ├─ RmsEntry{URI, RmsFile}               │
│             │    ├─ XsEntry{URI, XsFile}                 │
│             │    ├─ ResolvedInclude[] / MissingInclude[] │
│             │    └─ ExternalDecls(exclude) → []Decl      │
│             ├─ Definition(uri,pos) → (Target, bool)      │
│             └─ References(uri,pos) → []Target             │
│         ▲ читает                ▲ парсит                 │
│         │ Source.Text/URIs       │                        │
│    ┌────┴────┐            ┌──────┴─────┐           ┌─────┴─────┐
│    │  диск   │            │ rms.Parse  │           │ xs.XsParse│
│    └─────────┘            │ RmsFile.*  │           │ XsFile.*  │
│                           └────────────┘           └───────────┘
└───────────────────────────────────────────────────────────┘
```

### Data Flows

**DF1 — Definition (курсор на include-пути или XS-имени):**
`client → Server.Definition(params{uri,pos}) → Resolver.Definition(ctx,uri,pos)
→ {Include.Range hit → resolve → Target{targetURI,0:0}} | {inline-блок →
translate → XsFile.Definition → translate-back} | {XsFile.Definition} |
{closure Symbols() fallback} → Target → protocol.Location{URI:uri.URI(t.URI),
Range:toProtocolRange(t.Range)} → LocationSlice/Location`

**DF2 — References:** `client → Server.References → Resolver.References →
имя (SymbolAt | ReferencesAt+text) → корни {запрошенный ∪ открытые-включающие}
→ References(name) по замыканиям корней + inline-блоки (трансляция) →
дедуп → []Target → []protocol.Location; IncludeDeclaration=false → исключить
локальную декларацию (range Definition в запрошенном файле)`

**DF3 — Диагностика (didOpen/didChange):** `client → Server.DidOpen →
DocStore.Put → publishDiagnostics: analyze(uri,text,closure): closure :=
Resolver.Closure(uri); Missing(owner==uri) → missing-include; .rms →
rms.Parse + AnalyzeRms + inline-блоки (XsParse + AnalyzeXs(xsFile,
closure.ExternalDecls("")) + shiftDiags); .xs → XsParse + AnalyzeXs(file,
closure.ExternalDecls(uri)) → sortDiags → PublishDiagnostics`

### Entity Dependencies

Инициализация: `NewDocStore()` → `NewServer(store, analyzer)` создаёт
`include.NewResolver(docs)` внутри (DocStore структурно удовлетворяет
`Source`: Text + URIs). Порядок ячеек реализации: rms ∥ xs → include ∥
analysis → server.

## Code Stack Trace

### Trace: `rms.Parse` — путь директив (модификация)

#### Chain

1. **Input**: строка `#include "parts/econ.rms"` (или `#includeXS lib.xs`),
   индекс строки `idx`; `p.starts[idx]` — позиция начала строки (существующее
   поле парсера)
2. `directive()` (parse.go:422): лексер строки даёт токен директивы
   (`directive.at` — диапазон ключевого слова) → checkpoint: токен с
   абсолютной позицией существует ✓
3. Следующий `lex.next()` после директивы — токен аргумента (слово с кавычками
   или bare): `.text` = `"parts/econ.rms"`, `.at` = диапазон аргумента →
   checkpoint: Range покрывает аргумент включая кавычки (hit-test курсора на
   кавычках срабатывает) ✓
4. `Path = strings.Trim(argTok.text, "\"")`; пусто → существующая
   диагностика `"#include needs a file name"`, Include не создаётся →
   checkpoint: контракт «путь без аргумента — Diagnostic, Include не
   создаётся» ✓
5. `#include`: `p.file.Includes = append(..., Include{Path, argTok.at})`;
   `#includeXS`: то же в `XsIncludes` **и** прежнее поведение
   (`p.inXs = true; p.xsStart = idx+1`) — аргумент больше не отбрасывается,
   inline-режим сохраняется → checkpoint: contract Includes/XsIncludes ✓
6. **Output**: `RmsFile.Includes []Include` / `XsIncludes []Include` с
   диапазонами аргументов

#### Checkpoint Summary

- диапазон аргумента из токена лексера — passed (абсолютные позиции уже
  используются для всех токенов)
- сохранение inline-семантики `#includeXS` — passed (режим не зависит от
  наличия аргумента)

### Trace: `RmsFile.References(name)` (новый)

#### Chain

1. **Input**: `name string` на `RmsFile` с внутренним `words []wordOcc`
   (ast.go:52 — индекс всех слов-токенов, ведётся при парсинге)
2. Фильтр `w.name == name` → `out = append(out, w.at)` (зеркало цикла
   `ReferencesAt`, ast.go:261-265) → checkpoint: тип `common.Range` ✓
3. `slices.SortFunc` по `Start.Before` (копия сортировки ReferencesAt,
   ast.go:267) ✓
4. **Output**: `[]common.Range` (пустой `nil`-slice при отсутствии имени —
   как ReferencesAt)

Эквивалентность: `ReferencesAt(pos)` = имя по Contains (ast.go:247-253) +
этот же фильтр — рефакторинг: ReferencesAt может делегировать
`References(name)` после разрешения имени.

### Trace: `XsFile.References(name)` (новый)

#### Chain

1. **Input**: `name string` на `XsFile` с `symbols []symbol`
   (индекс вхождений идентификаторов, ast.go:85)
2. Фильтр `s.name == name` → ranges (зеркало ReferencesAt, xs/ast.go:118-122);
   сортировка (xs/ast.go:124-133) → checkpoint ✓
3. **Output**: `[]common.Range`, декларация включена (декларационное имя —
   обычное вхождение в индексе)

### Trace: `include.NewResolver(source)` (новый)

#### Chain

1. **Input**: `source Source` (DI; от Server — `*DocStore`)
2. Конструктор хранит source + создаёт пустой кэш
   `map[string]cachedFile` (канонический путь → {entry, size, modtime}) и
   `sync.Mutex` → checkpoint: потокобезопасность по конвенциям ✓
3. **Output**: `*Resolver`

### Trace: `Resolver.Closure(ctx, uri)` (новый)

#### Chain

1. **Input**: `uri string` корневого документа
2. `text, ok := source.Text(uri)`; `!ok` → `os.ReadFile(uri.URI(uri).Filename())`;
   обе неудачи (в т.ч. non-file схема) → `Closure{Root: uri}` с пустыми
   списками → checkpoint: контракт «корень недоступен → пустое замыкание» ✓
3. Расширение: `.xs` → `xs.XsParse(text, uri)` → XsEntry; иначе (`.rms`,
  `.inc`, без расширения) → `rms.Parse(text, uri)` → RmsEntry. Текстовые
  включения в игре — RMS-сниппеты любого расширения → checkpoint ✓
4. DFS-очередь с глубиной: элемент {uri, depth}; visit-set по каноническому
   пути (`filepath.Clean(filepath.Abs(...))`); `depth+1 > 64` → узел входит,
  директивы не разворачиваются; `len(files) > 1024` → разворот останавливается
   → checkpoint: циклы A→B→A дают одиночное вхождение ✓
5. Для RmsEntry: `for inc in file.Includes + file.XsIncludes`:
   `target := filepath.Join(filepath.Dir(ownerPath), inc.Path)`; канонизация;
   `os.Stat` ok → `ResolvedInclude{owner, inc, uri.File(target)}` +
   enqueue; Stat/read неудача → `MissingInclude{owner, inc.Path, inc.Range}`
   → checkpoint: Range в координатах owner ✓
6. Диск-кэш: entry из кэша валидна, если stat (size, modtime) совпал;
   записи из `Source.Text` кэшем не накрываются (парсятся на каждый вызов —
   корень обычно мал) → checkpoint: editor-state всегда свежий ✓
7. **Output**: `Closure{Root, Rms, Xs, Resolved, Missing}` в DFS-порядке
   директив

#### Checkpoint Summary

- типы Parse/XsParse фактические сигнатуры `func(string,string)(RmsFile,[]Diagnostic)`
  — diags отбрасываются (замыканию синтаксис не нужен) — passed
- `uri.File`/`Filename` из go.lsp.dev/uri — в go.sum транзитивно;
  `go mod tidy` переведёт в direct — passed

### Trace: `Resolver.Definition(ctx, uri, pos)` (новый)

#### Chain

1. **Input**: `uri`, `pos common.Pos`
2. Загрузить/спарсить запрошенный файл (шаги 2-3 Closure-трассы)
3. `for inc in Includes+XsIncludes`: `inc.Range.Contains(pos)` → резолв пути
   (шаг 5 Closure) → найдено: `Target{targetURI, Range{Pos{}, Pos{}}}`
   (0:0, нулевая длина); не найдено → found=false → checkpoint: контракт ✓
4. `.rms` и `pos` внутри `block.Range` какого-то XsBlock: обратная трансляция
   `bpos = pos − block.Range.Start` (line=Δ, column=Δ при line 0, offset=Δ) →
   `XsParse(block.Code)` → `file.Definition(bpos)` → прямая трансляция
   результата (зеркало `shiftPos` сервера, server.go:587) → checkpoint:
   трансляция идентична серверной семантике ✓
5. `.xs`: `file.Definition(pos)` напрямую ✓
6. Fallback: имя = `.xs`→`SymbolAt(pos)`; `.rms` inline→translated
   `SymbolAt`; поиск `for e in closure.Xs: for sym in e.File.Symbols():
   sym.Name == name → Target{e.URI, sym.Selection}` — первый в DFS-порядке →
   checkpoint: `Symbols().Selection` = name-range декларации (контракт xs) ✓
7. Ничего → `found=false` (builtin/неизвестное — не эвристики)

### Trace: `Resolver.References(ctx, uri, pos)` (новый)

#### Chain

1. **Input**: `uri`, `pos`
2. `closure := Closure(uri)`
3. Имя: `.xs` → `SymbolAt(pos)`; `.rms` → `ranges := file.ReferencesAt(pos)`;
   пусто → пустой результат; иначе имя = `text[ranges[0].Start.Offset:
   ranges[0].End.Offset]` (текст файла у резолвера есть — шаг 2 Closure)
   → checkpoint: Offset-поля `common.Pos` существуют ✓
4. Корни: `{uri}` ∪ `{u ∈ source.URIs(), u ≠ uri : closure(u) содержит uri}`
   (обратное направление — фикс дефекта; замыкания открытых уже в дисковом
   кэше, стоимость низкая) ✓
5. Для каждого корня: по файлам его замыкания `References(name)` (.rms и
   .xs) → Target; inline-блоки корня: `XsParse(block.Code)`,
   `References(name)`, трансляция ranges ✓
6. Дедуп `map[struct{uri string; r common.Range}]struct{}{}`;
   сортировка (URI, затем Start) ✓
7. **Output**: `[]Target` — запрошенный файл входит; декларация включена

### Trace: `Closure.ExternalDecls(exclude)` (новый)

1. **Input**: `exclude string` (URI или "")
2. `for e in closure.Xs: if e.URI != exclude: out = append(out, e.File.Decls...)`
3. **Output**: `[]xs.Decl` в DFS-порядке → checkpoint: тип параметра
   `AnalyzeXs.externals` — `[]Decl` ✓

### Trace: `Analyzer.AnalyzeXs(file, externals)` (модификация)

#### Chain

1. **Input**: `file xs.XsFile`, `externals []xs.Decl`
2. `env := NewTypeEnv(file)` (types.go:38 — root map из Decls файла) —
   без изменений
3. **Новое**: `for d in externals: if _, busy := root[d.Name]; !busy &&
   d.Name != "": root[d.Name] = тип по Kind` (правила типов — как
   NewTypeEnv: DeclFunction/DeclExtern/DeclVariable → d.Type, прочие → "")
   → checkpoint: «только если имя ещё не объявлено файлом» = локальные
   приоритетнее ✓; затенение параметрами/локалями работает — они кладутся
   в более внутренние области при обходе (Declare во вложенные scopes)
4. Дальше без изменений: обход объявлений, undefined-symbol, bad-arity,
   bad-type, сортировка
5. **Output**: `[]common.Diagnostic`; nil-externals ≡ прежнее поведение

### Trace: `Server.Definition` / `Server.References` (модификация)

#### Chain

1. **Input**: params из протокола
2. `s.resolver.Definition(ctx, string(params.TextDocument.URI),
   fromProtocolPos(params.Position))` — **gate открытого документа снят**
   (прежде `openDocument` !ok → пусто): резолвер сам читает диск →
   checkpoint: контракт Algorithm не требует открытости ✓
3. `Target → protocol.Location{URI: uri.URI(t.URI), Range:
   toProtocolRange(t.Range)}`; found=false → `protocol.LocationSlice{}`
   (не nil) ✓
4. References: `[]Target → []protocol.Location`;
   `IncludeDeclaration=false` → исключить range локальной декларации:
   `Resolver.Definition` в запрошенном файле (обобщение существующего
   `excludeDeclaration`, server.go:389) ✓

### Trace: конвейер диагностики (модификация)

1. `publishDiagnostics(ctx, uri)` → `closure := s.resolver.Closure(ctx,
   string(uri))`
2. `for m in closure.Missing: m.Owner == uri → common.Diagnostic{Range:
   m.Range, Severity: SeverityError, Code: "missing-include", Message:
   "include not found: " + m.Path}` — только открытый корень (не спамить
   неоткрытые URI)
3. `.rms`: `analyzeRms` как прежде + inline-блоки:
   `AnalyzeXs(xsFile, closure.ExternalDecls(""))` + shiftDiags;
   `.xs`: `AnalyzeXs(file, closure.ExternalDecls(string(uri)))`
4. `sortDiags` → `toProtocolDiags` → publish одной пачкой
5. Сигнатура внутренних хелперов: `analyze(name,text)` →
   `analyze(ctx,uri,name,text)` (нужно ctx и uri для замыкания)

### Trace: `DocStore.Text` / `DocStore.URIs` (новые)

1. `Text(uri)` = `Get(uri)` без version (мьютекс уже есть, docstore.go:39)
2. `URIs()` = ключи `s.documents` под мьютексом (порядок — сортировка для
   детерминизма)

## Algorithm Design

### `Resolver`

**Responsibility**: include-замыкание документа и кросс-файловая навигация;
кэш парсов дисковых файлов.

**Algorithm:**
```
Closure(ctx, uri):
1. text := load(uri)                       # Source.Text → диск; оба промах → пустое замыкание
2. entry := parse(uri, text)               # .xs→XsParse; иначе→rms.Parse; diags отбрасываются
3. queue := [{uri, entry, depth 0}]; visited := {canon(uri)}
4. WHILE queue:
   e := pop-front
   IF e — RmsEntry:
     FOR inc IN e.File.Includes + e.File.XsIncludes:
       target := canon(join(dir(path(e.URI)), inc.Path))
       IF stat(target) ok:
         Resolved += {e.URI, inc, uriFile(target)}
         IF target ∉ visited AND depth(e)+1 ≤ 64 AND len(files) < 1024:
           tEntry := cached-or-load(target)   # stat size+modtime
           visited += target; queue += {tEntry, depth+1}
       ELSE:
         Missing += {e.URI, inc.Path, inc.Range}
5. RETURN Closure{Root: uri, Rms, Xs, Resolved, Missing}   # DFS-порядок

Definition(ctx, uri, pos):
1. entry := load-and-parse(uri)
2. FOR inc IN entry.Includes+XsIncludes: inc.Range.Contains(pos)
   → resolve(inc) → RETURN Target{targetURI, Range(0:0)}, true
3. IF uri — .rms AND pos ∈ block.Range: RETURN translated(block.Definition(pos − block.Start))
4. IF uri — .xs: entry.Definition(pos) → hit → Target{uri, r}
5. name := symbolNameAt(uri, pos)   # SymbolAt (.xs / inline-block)
   FOR e IN closure.Xs (DFS): FOR sym IN e.File.Symbols():
     sym.Name == name → RETURN Target{e.URI, sym.Selection}
6. RETURN zero, false

References(ctx, uri, pos):
1. closure := Closure(uri); name := nameAt(uri, pos)   # SymbolAt | ReferencesAt+text
2. roots := {uri} ∪ {u ∈ Source.URIs(): u≠uri, Closure(u) ∋ uri}
3. hits := []; FOR root IN roots: FOR f IN Closure(root).files:
     hits += {f.URI, r : r ∈ f.References(name)}
   FOR root IN roots: FOR block IN root.blocks:
     hits += {root.URI, shift(r, block.Start) : r ∈ XsParse(block.Code).References(name)}
4. RETURN dedup(hits) sorted by (URI, Start)
```

**Errors:**
- корень недоступен (не открыт + не читается / non-file URI) → пустое
  `Closure`, Definition found=false, References пусто — деградация без ошибок
- stat/read включаемого файла → `MissingInclude` (не ошибка вызова)
- ctx отменён → вернуть посчитанное (частичное) — запрос повторится клиентом

**Edge Cases:**
- цикл A→B→A: visit-set, каждый файл в замыкании один раз
- глубина >64 / файлов >1024: разворот останавливается, ошибки нет
- `#include` на директорию: stat ok, но read Fail → Missing
- один и тот же путь включён дважды: одна запись в файлах, директива
  резолвится каждая (Resolved по директивам)
- пустой `XsIncludes`-аргумент (`#includeXS` bare): Include нет, блок есть
- Target.URI string → `uri.URI(t.URI)` в server: валидная file-схема
  (произведена `uri.File`)

### `Analyzer.AnalyzeXs` (externals)

**Responsibility**: XS-проверки с внешними декларациями.

**Algorithm:** см. трассу — сидинг в root-область TypeEnv после локальных,
только незанятые имена; nil ≡ прежнее поведение.

**Errors:** нет (stateless, без IO).

**Edge Cases:** external с пустым Name — пропуск; external дублирует имя
другого external — первый в DFS-порядке выигрывает (детерминизм).

## Cross-cutting Concerns

- **Error handling**: резолвер не возвращает error — всё деградирует
  (Missing/пустые результаты); парсеры уже never-fail; сервер мапит в
  диагностики. `%w` не требуется (нет пробрасываемых ошибок наружу)
- **Logging**: паритет с существующим сервером — логирования нет; при
  отладке — `protocol.LoggingStream` (см. `lsp-protocol`). Новых логгеров
  не вводим (в контрактах не заявлено)
- **Validation**: hit-test `Range.Contains`; пути — только `filepath`
  (кроссплатформенность, CI win/mac/linux)
- **Caching**: дисковый парс-кэш `Resolver` по каноническому пути;
  инвалидация stat-ом (size+mtime); открытые документы — всегда свежие из
  `Source` (без кэша). Изменения вне редактора без смены stat не видны —
  документированное ограничение
- **Concurrency**: `Resolver.mu sync.Mutex` на кэш (jsonrpc2 interleaves
  запросы); `DocStore` уже под мьютексом; `Closure` — value-тип, общему
  изменению не подлежит

## Usages Analysis

### `conventions`
- **What**: Go-правила проекта (DI, ctx-first, тесты, goimports)
- **Where**: все пять ячеек
- **Why**: базовая практика проекта
- **How**: DI-конструкторы NewResolver/NewServer; ctx первым параметром;
  table-driven тесты testify+cmp

### `lsp-protocol`
- **What**: паттерны go.lsp.dev/protocol + Disk-backed Documents +
  Cross-file Navigation Results
- **Where**: include (Resolver), server (хендлеры)
- **Why**: единственный источник паттернов протокола и дисковой загрузки
- **How**: `uri.File`/`URI.Filename()`; Location с чужим URI; пустой slice
  не nil; editor-state приоритет

### Imported Usages
- `positions-and-diagnostics` из `common` — Pos/Range/Diagnostic конструирование
  (Path: `common/.usages/positions-and-diagnostics.md`)
- `rms-parsing` из `rms` — Parse/ReferencesAt/Symbols (Path: `rms/.usages/rms-parsing.md`)
- `includes` из `rms` — Includes/XsIncludes/Range hit-test/by-name
  (Path: `rms/.usages/includes.md`)
- `xs-parsing` из `xs` — XsParse/SymbolAt/Definition/References/Symbols
  (Path: `xs/.usages/xs-parsing.md`)
- `lookups` из `kb`, `checks` из `analysis`, `symbols` из `common` — server
  (существующие связи, без изменений)

## `.usages/` Update

### Cell: `rms`
- **`includes.md`** → создан при apply; Status: current
- **`rms-parsing.md`** → расширен (References(name)); current

### Cell: `xs`
- **`xs-parsing.md`** → расширен (by-name + Symbols-fallback); current

### Cell: `include`
- **`closure.md`** → создан при apply, **обновлён при фикс дефекта**
  (корни поиска References по Source.URIs); current

### Cell: `analysis`
- **`checks.md`** → расширен (externals); current

### Cell: `server`
- **`lifecycle.md`** → расширен (Cross-file navigation + preconditions); current

Новых файлов не требуется; своих `.usages/` в `Usages`-директивы не добавляем
(потребительская документация, не контракт).

## Test Stack Trace

### General Setup

- tempdir-деревья `t.TempDir()` + `os.WriteFile`; URI — `uri.File(path).String()`
- fake Source в тестах include: `map[string]string` (+URIs) — hand-written
  fake по конвенциям (без gomock)
- серверные интеграции — существующий stdio-харнесс server-тестов
  (LSP-клиент из теста), документы — tempdir
- все прогоны под memory-cap песочницей (CLAUDE.md)

### Source File Registry

`rms/parse.go`, `rms/ast.go`, `xs/ast.go`, `include/source.go`,
`include/resolver.go`, `include/closure.go`, `analysis/analyzer.go`,
`analysis/types.go`, `server/server.go`, `server/docstore.go`

---

### Positive Tests

#### `TestParse_IncludeRecordsPathArgumentRange`

**Setup**: исходник
```
#include "parts/econ.rms"
#include parts/bare.inc
```
**Input**: `rms.Parse(src, "main.rms")`
**Trace**:
```
Parse(src, "main.rms")
  → directive("#include \"parts/econ.rms\"", 0)      # lex.next() → argTok
    argTok.text = "\"parts/econ.rms\"", argTok.at = [0:9..0:25]
    → Includes += Include{Path:"parts/econ.rms", Range:[0:9..0:25]}
  → directive("#include parts/bare.inc", 1)
    → Includes += Include{Path:"parts/bare.inc", Range:[1:9..1:22]}
```
**Assertions**:
```
len(file.Includes) == 2
file.Includes[0].Path == "parts/econ.rms"
file.Includes[0].Range.Start == Pos{Line:0, Column:9}   # включая кавычку
file.Includes[1].Path == "parts/bare.inc"               # bare-форма
```
**Sufficiency**: Range аргумента — основа include-hit-test Definition и
missing-диагностики; bare vs quoted — обе реальные формы грамматики.

#### `TestParse_IncludeXSArgumentAndInlineBlock`

**Setup**:
```
#includeXS lib/helpers.xs
void sharedFn(int n) { }
```
**Input**: `rms.Parse`
**Trace**:
```
directive("#includeXS lib/helpers.xs", 0)
  → XsIncludes += Include{Path:"lib/helpers.xs", Range:[0:11..0:26]}
  → p.inXs = true; p.xsStart = 1                      # прежнее поведение
endXsBlock(EOF) → XsBlocks += {Code:"void sharedFn(int n) { }", Range:[1:0..2:0]}
```
**Assertions**:
```
len(file.XsIncludes) == 1 && XsIncludes[0].Path == "lib/helpers.xs"
len(file.XsBlocks) == 1                                # inline сохранился
```
**Sufficiency**: фиксирует dual-режим #includeXS (файл + inline), который
прежде молча терял имя файла.

#### `TestReferences_ByName` (rms и xs)

**Setup**: rms: `create_elevator 7\ncreate_elevator 3`; xs:
`void f(){}\nvoid g(){ f(); }`
**Input**: `file.References("create_elevator")` / `xsFile.References("f")`
**Trace**: фильтр индекса words/symbols → сортировка
**Assertions**: rms — 2 range на строках 0,1; xs — 3 range (декларация f,
call f) — `ReferencesAt(pos на любом вхождении)` даёт те же множества.
**Sufficiency**: контракт эквивалентности ReferencesAt ≡ References(word).

#### `TestResolver_ClosureDFSAndMissing`

**Setup** (tempdir):
```
main.rms:  #include "a.rms"; #includeXS lib.xs; #include "missing.rms"
a.rms:     #include "b.rms"
b.rms:     (пусто)
lib.xs:    void sharedFn(int n) { }
```
fake Source: только main.rms открыт.
**Input**: `resolver.Closure(ctx, uriFile(main.rms))`
**Trace**:
```
Closure(main)
  → load main (Source) → rms.Parse → RmsEntry(main)
  → a.rms: stat ok → Resolved{main, ..., a}; load disk → RmsEntry(a)
  → b.rms: stat ok → Resolved{a, ..., b}; RmsEntry(b)
  → lib.xs: stat ok → Resolved{main,...,lib}; XsEntry(lib)
  → missing.rms: stat fail → Missing{main, "missing.rms", Range[2:9..2:20]}
```
**Assertions**:
```
len(closure.Rms) == 3 (main,a,b — DFS-порядок), len(closure.Xs) == 1
len(closure.Resolved) == 3, len(closure.Missing) == 1
closure.Missing[0].Owner == uri(main), .Range covers "missing.rms"
closure.ExternalDecls("") == lib.Decls (1 элемент)
```
**Sufficiency**: ядро замыкания — DFS-порядок, резолв, missing с range.

#### `TestResolver_EditorStateWinsOverDisk`

**Setup**: `a.rms` на диске содержит `#include "old.rms"`; fake Source
отдаёт `#include "new.rms"` для uri(a).
**Input**: `Closure(uri(a))`
**Trace**: `Source.Text` hit → парс editor-версии → Includes=[new.rms]
**Assertions**: `closure.Resolved[0].Target == uri(new)`;
disk-копия не читалась (fake считает обращения).
**Sufficiency**: контракт editor-state приоритета (кука Disk-backed).

#### `TestResolver_DefinitionIncludeDirective`

**Setup**: дерево из Closure-теста.
**Input**: `Definition(uri(main), Pos{Line:0, Column:12})` (курсор на
`a.rms` в include-строке)
**Trace**: `Includes[0].Range.Contains(pos)` → резолв → Target
**Assertions**: `found == true; target.URI == uri(a.rms);
target.Range == Range{Pos{0,0}, Pos{0,0}}` (нулевая длина на 0:0)
**Sufficiency**: LSP-сценарий 1 задачи.

#### `TestResolver_DefinitionExternalDecl` 

**Setup**: main.rms c `#includeXS lib.xs` + inline `sharedFn(1);`.
**Input**: `Definition(uri(main), pos на sharedFn в inline-блоке)`
**Trace**: inline-трансляция → XsParse(block).Definition(bpos) → промах →
fallback: closure.Xs[lib].Symbols() Name=="sharedFn" → Selection
**Assertions**: `target.URI == uri(lib.xs); target.Range == name-range
sharedFn в lib.xs`
**Sufficiency**: LSP-сценарий 2 (главная фича кросс-файлового Definition).

#### `TestResolver_ReferencesReverseOverOpenDocs`

**Setup**: то же дерево; fake Source: открыты main.rms и lib.xs.
**Input**: `References(uri(lib.xs), pos на sharedFn в её декларации)`
**Trace**: closure(lib)={lib}; имя=sharedFn; корни: lib + main (closure(main)
∋ lib) → References("sharedFn") по {lib, main} + inline-блок main → дедуп
**Assertions**: вхождения: декларация+call в lib.xs, inline-call в main.rms
(range в координатах main.rms — трансляция)
**Sufficiency**: LSP-сценарий 3 + фикс дефекта обратного направления.

#### `TestAnalyzeXs_ExternalsSuppressUndefinedAndLocalsWin`

**Setup**: file: `void h(){ extFn(1); }`; externals: `[]xs.Decl{{Kind:
DeclFunction, Name:"extFn", Type:"void"}}` и второй кейс — file сам
декларирует `extFn`.
**Input**: `AnalyzeXs(file, externals)` / `AnalyzeXs(file, nil)`
**Trace**: NewTypeEnv(file) → сидинг незанятых → undefined-symbol не
срабатывает для extFn; nil → срабатывает
**Assertions**: с externals — 0 диагностик undefined-symbol; nil — 1;
локальная декларация приоритетна (тип из file, не из external).
**Sufficiency**: контракт externals + регрессия ложных срабатываний.

---

### Negative Tests

#### `TestParse_IncludeWithoutPath`

**Setup**: `#include\n` (без аргумента).
**Input**: `rms.Parse`
**Trace**: argTok пуст → `reportf(directive.at, error, "syntax",
"#include needs a file name")` → Include не создан
**Assertions**: `len(file.Includes) == 0`; в diags есть syntax-диагностика
на строке 0.
**Sufficiency**: контракт «путь без аргумента → Diagnostic, Include нет».

#### `TestResolver_RootUnavailable`

**Setup**: uri указывает на несуществующий файл; Source: not found.
**Input**: `Closure(uri)`, `Definition(uri, 0:0)`
**Trace**: Source.Text miss → ReadFile error → пустое замыкание
**Assertions**: `closure.Root == uri; Rms/Xs/Resolved/Missing пусты;
Definition found == false` — без паники и ошибок.
**Sufficiency**: деградация закрытых/битых документов (кука Disk-backed).

---

### Edge Case Tests

#### `TestResolver_CycleTerminates`

**Setup**: a.rms: `#include "b.rms"`; b.rms: `#include "a.rms"`.
**Input**: `Closure(uri(a))` (таймаут-защита теста: `context`/`t.Deadline`)
**Trace**: visit-set: a→b→(a уже посещён, не разворачивается)
**Assertions**: `len(closure.Rms) == 2`; Resolved содержит обе директивы;
тест завершается (нет зацикливания).
**Sufficiency**: риск №1 задачи (runaway-цикл).

#### `TestResolver_DepthAndCountLimits`

**Setup**: цепочка f0→f1→…→f70 (по файлу на уровень).
**Input**: `Closure(uri(f0))`
**Trace**: глубина 64 — f64 входит, его директива не разворачивается
**Assertions**: `len(closure.Rms) == 65`; Missing пуст; нет ошибки.
**Sufficiency**: защита от include-бомбы (лимиты 64/1024).

#### `TestResolver_ReferencesDedupAndSort`

**Setup**: main включает lib; lib включён main-ом дважды (две директивы);
обе точки search roots покрывают одни файлы.
**Input**: `References(uri(main), pos на sharedFn)`
**Assertions**: нет дублей (URI,Range); результат отсортирован (URI, затем
Start).
**Sufficiency**: детерминированность контракта.

#### `TestDocStore_TextAndURIs`

**Setup**: Put("file:///a", "text-a", 1); Put("file:///b", "text-b", 2).
**Input**: `Text("file:///a")`, `URIs()`, `Text("file:///c")`
**Assertions**: `("text-a", true)`; `["file:///a","file:///b"]` (сортировка);
`("", false)`.
**Sufficiency**: структурное удовлетворение Source (Text+URIs).

#### `TestServer_CrossFileNavigation` (интеграционный, stdio)

**Setup**: tempdir-фикстура из задачи (main.rms / parts/econ.rms /
parts/lib.xs / broken.rms); LSP-клиент из теста (существующий харнесс).
**Input/Assertions** — сценарии 1–5 задачи:
```
1. Definition на include-строке main.rms     → Location econ.rms
2. Definition на sharedFn (inline)           → Location lib.xs name-range
3. References на sharedFn (lib.xs, main открыт) → main+lib вхождения
4. didOpen broken.rms                        → publishDiagnostics содержит
                                                code=missing-include с range
5. didOpen main при открытой вкладке econ (изменённой) → навигация видит
                                                редакторную версию econ
```
**Trace**: полный конвейер DF1/DF2/DF3 по stdio.
**Sufficiency**: приёмочные критерии задачи целиком.

## Additional Instructions for the Implementation Agent

- `go.mod`: `go mod tidy` переведёт `go.lsp.dev/uri` в direct (тот же
  модуль, новой версии нет); новых зависимостей не добавлять
- Порядок реализации: rms ∥ xs → include ∥ analysis → server; после каждой
  ячейки — `go test ./<cell>/...` (песочница memory-cap), `goimports -w .`,
  `golangci-lint run`, `goga lint`, `goga contract <cell>`
- Трансляция координат inline-блоков: включить локальные хелперы в
  `include` (зеркало `shiftPos`/`shiftDiags` сервера — серверовские
  unexported, переиспользовать нельзя; дублирование ~15 строк допустимо)
- `Resolver.References` для `.rms`: имя извлекать из текста по Offset-ам
  `ReferencesAt`-range (Offset-поля уже заполняются парсерами)
- `analyze*`-хелперы сервера получают `ctx` и `uri` (Closure требует);
  gate открытого документа в Definition/References снимается — резолвер
  сам fallback-ится на диск
- Диагностика `.xs`-корня: externals = `ExternalDecls(uri)` — при открытии
  lib.xs без main.rms вызовы соседних .xs остаются undefined (задокументированное
  ограничение, вне сценариев задачи)
- Definition-цель include-перехода: `Range{Pos{}, Pos{}}` (0:0, нулевая
  длина) — редактор открывает файл в начале
