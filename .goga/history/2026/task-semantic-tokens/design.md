# Design Document: `semantic-tokens`

## Contract Changes

### Changed CODEMANIFEST Files
- `internal/common/CODEMANIFEST`: новый Entity `Token` (token.go).
- `internal/analysis/CODEMANIFEST`: импорт `Token`; методы `TokensRms`,
  `TokensXs` у `Analyzer`.
- `internal/server/CODEMANIFEST`: импорт `Token`; метод
  `SemanticTokensFull`; capability в `Initialize`; строка в глобальных
  Annotations.

### New Entities
- `Token{Range, Type}` — common, чистые данные (прецедент `Symbol`).

### Changed Entities
- `Analyzer` +2 метода (зеркально AnalyzeRms/AnalyzeXs); `Server`
  +метод +capability.

### Usages and Annotations Changes
- `.goga/usages/cooks/lsp-protocol.md` — секция Semantic Tokens
  (метод `SemanticTokensFull`, delta-формат, Data:[] не nil).
- `internal/server/.usages/lifecycle.md` — capabilities + секция
  Semantic tokens.

## Applied Fixes

### Fixed CODEMANIFEST Defects
- Ссылки аннотаций: точечная нотация (`Analyzer.TokensRms`,
  `Store.Function`) не разрешима линтером → «TokensRms у `Analyzer`»,
  bare-упоминания (стиль соседних аннотаций); `Token` в backticks в
  аннотации сервера (import_is_used). Коммиты e354d33, за ним.

## Entity Interaction and Data Flow

```
SemanticTokensFull(params)
  ├─ .rms: rms.Parse → Analyzer.TokensRms(file)
  │        + inline XsBlocks: XsParse → Analyzer.TokensXs(block, nil)
  │          → сдвиг Range на block.Offset (шаблон DocumentHighlight)
  ├─ .xs:  XsParse → Closure(uri).ExternalDecls(uri) → Analyzer.TokensXs
  └─ []Token → sort(line,start) → (line,col,len)×encoding → delta []uint32
```

## Code Stack Trace

### Trace: `Analyzer.TokensRms`
1. Вход: `rms.RmsFile` (Sections[].Statements[] рекурсивно, Children).
2. Секции: имя секции → token `section`; диапазон имени — первое
   вхождение из `file.References(sec.Name)` на линии заголовка
   (`sec.Range.Start.Line`): парсер пишет слово секции в word-индекс
   (recordSectionWord: col после `<`, длина = len(name); case не влияет
   на длину) → checkpoint: точный спан имени без расширения rms ✓
3. Каждый statement: `Name[0]=='#'` → пропуск (директивы, шаблон
   checkCommand); иначе имя — токен длиной `len(Name)` от
   `stmt.Range.Start` (парсер: имя — первый токен statement,
   parse.go:282/381) → known/unknown по `store.Command(name)`; блоки
   (KindRandom/KindConditional) — тот же lookup (шаблон checkBlockStmt);
   `effect_percent` → `deprecated` (перекрывает known) ✓
4. Атрибуты: `attr.Range` — точный спан имени атрибута (buildAttribute:
   Range = first.at); known/unknown по `store.Attribute(owner, name)`,
   owner = команда для in-brace, lastKnown для bare-строк (шаблон
   walkStmts lastKnown) ✓
5. Значения аргументов/атрибутов: рекурсивный обход `Expr.Children`;
   листья Kind ∈ {ident, const} → known/unknown по
   `store.Constant(value)` (const-имена уровня RMS kb не моделирует —
   почти все unknown; правило простое и детерминированное) ✓
6. Сортировка по (line, column); перекрытий нет (имя/атрибуты/значения
   — непересекающиеся токены парсера).

### Trace: `Analyzer.TokensXs`
1. Вход: `xs.XsFile`, `externals []Decl`.
2. Декларации: kind-токены по `file.Symbols()` — Selection каждого
   топ-символа = точный спан имени декларации (семантика xs-parsing) ✓
3. Ident-вхождения: обход деклараций (тела функций/правил, выражения
   Stmt) — Expr Kind=ident → Range = спан; предикат known — тот же,
   что у checkIdent (analyzer.go:398): declared (файл: functions/
   variables/rules + params/локали по шаблону walkStmtsXs; externals)
   ∪ xsBuiltins ∪ `store.Function` ∪ `store.Constant` ✓
4. Вызовы: Expr Kind=call — callee-токен от `Range.Start` длиной
   `len(Callee)` (callee — первый токен вызова), предикат known тот же ✓
5. Сортировка; kind-токен декларации перекрывает known на том же спане
   (приоритет: kind > known/unknown).

### Trace: `Server.SemanticTokensFull`
1. `openDocument(uri)` → (text, name); не открыт → `Data: []uint32{}`
   (не nil) — но docstore-документы в этом хендлере всегда открыты
   (запрос по TextDocument.URI); чужое расширение → Data:[] ✓
2. .rms: `rms.Parse(text)` → TokensRms; далее каждый `XsBlock`:
   `xs.XsParse(block.Code)` → TokensXs(block, nil) → сдвиг
   `Range += block.Offset` (готовый шаблон document_highlight) ✓
3. .xs: `xs.XsParse(text)`; `ExternalDecls` из `Closure(uri)`
   (exclude=uri) → TokensXs ✓
4. Каждый Token: start = `toProtocolPos(text, r.Start)` (utf-16
   конвертация), length = col(r.End) − col(r.Start) в тех же юнитах
   (byteToUTF16 колонок через lineOf); typeIdx = индекс в легенде
   [known, unknown, deprecated, section, kind] ✓
5. Delta: сортировка (line, startChar) стабильная; [Δline, Δstart,
   len, typeIdx, 0]; первый токен абсолютный. ResultID не ставим
   (инкремент не поддерживаем) ✓
6. Debug-лог «semantic_tokens», return `&protocol.SemanticTokens{Data}`.

## Algorithm Design

### `Analyzer.TokensRms / TokensXs` — см. трассировки; вычисление
детерминировано, входные AST не меняются, пороги did-you-mean не
применяются (токенизация без подсказок).

### Delta-кодирование (server)
```
prev := (0, 0)
for tok in sorted tokens:
    dLine = tok.line - prev.line
    dStart = dLine == 0 ? tok.start - prev.start : tok.start
    data += [dLine, dStart, tok.length, typeIdx(tok.type), 0]
    prev = (tok.line, tok.start)
```

## Cross-cutting Concerns
- Errors: хендлер не ошибается; парсеры толерантны.
- Logging: один DebugContext «semantic_tokens» (count).
- Caching: none (stateless); Closure кэшируется резолвером.
- Concurrency: без новой (готовые синхронизированные зависимости).

## Usages Analysis
- `lsp-protocol` (Semantic Tokens): `SemanticTokensFull`,
  `SemanticTokensOptions{Legend}`, Data-формат — применены в хендлере и
  Initialize.
- `lookups` (kb): `Command`/`Attribute`/`Function`/`Constant` —
  предикаты known.
- `rms-parsing`/`xs-parsing`: AST-формы, `References(name)`, `Symbols()`
  (Selection = спан имени декларации), XsBlock-сдвиг.
- `symbols` (common): `Token` добавлен в тот же импорт.

## `.usages/` Update
- `internal/analysis/.usages/` — есть ли файлы? (проверить на
  реализации: если есть checks.md — дополнить секцией Tokens; если
  практика не покрывает — не создавать принудительно).
- `internal/server/.usages/lifecycle.md` — уже расширен на apply.

## Test Stack Trace

### General Setup
`newNavigationServer(t)`; для analysis — прямые юнит-тесты
(`analyzer_test.go` стиль, table-driven). Golden: фикстура → полный
Data-массив.

### Positive: `TestInitialize_AdvertisesSemanticTokens`
capability: `SemanticTokensProvider` c легендой
`[known, unknown, deprecated, section, kind]`, modifiers пуст.

### Positive: `TestAnalyzerTokens_Rms` (analysis, table-driven)
Фикстура: секция + известная команда + неизвестная команда +
effect_percent + атрибуты (known/unknown) + const-значение.
Ожидание: точные (Range, Type) пары; deprecated перекрывает known;
имя секции — спан внутри `<>`.

### Positive: `TestAnalyzerTokens_Xs` (analysis)
Фикстура: функция + правило + переменная; вызов известной/неизвестной
функции; ident known (параметр/локаль/external) / unknown; kind на
декларациях перекрывает known.

### Golden: `TestServerSemanticTokens_GoldenRms/Xs` (server)
Полный Data на фикстуре (рассчитанные квинтупли), включая inline-XS в
.rms (сдвиг) и пустой файл → `Data: []uint32{}` NotNil.

### Negative: `TestServerSemanticTokens_APIShape`
Закрытый/неизвестный документ → `Data` пустой не-nil, no error.

### Edge: сортировка и delta
Токены на одной строке (команда + атрибут) — Δline=0, Δstart
убывает корректно; utf-16 длина на не-ASCII имени (юникод-фикстура).

### Integration: `TestServerSemanticTokens_IntegrationStdio`
harness: didOpen .rms с inline-XS → `h.disp.SemanticTokensFull` →
непустая Data, легенда из Initialize согласована с индексами типов;
unknown-команда получает индекс unknown.

## Additional Instructions for the Implementation Agent
- Метод протокола — `SemanticTokensFull` (подтверждено по исходнику).
- Не расширять rms/xs: только экспортированные API (`References`,
  `Symbols`, AST-поля). Спан имени команды = от `stmt.Range.Start`
  длиной `len(Name)` — гарантия парсера (имя — первый токен).
- Легенда — пакетная переменная server (порядок = индексы).
- Модификаторы всегда 0 (легенда modifiers пуста).
- `make check` + `goga contract` трёх ячеек.
