# [ARCHITECTURE_PLAN]

## Topic

semantic-tokens — путь: `.goga/history/2026/task-semantic-tokens/arch.md`
(слот M волны 4 пачки editor-experience; формулировка — строка слота в
`.goga/history/2026/editor-experience/task.md`, утверждена 2026-09-09).

## Implementation Order

1. **`internal/common`** (modify) — leaf: новый `Token` без зависимостей
   кроме `Range`.
2. **`internal/analysis`** (modify) — зависит от common/kb/rms/xs;
   классифицирующий проход `TokensRms`/`TokensXs`.
3. **`internal/server`** (modify) — root: хендлер + capability + легенда.

## Artifacts

### Cell: `internal/common` — modify

#### CODEMANIFEST

Body — новый Entity (после `Symbol`):

```yaml
"Token(range: Range, typ: string)":
  location: token.go
  annotations: |
    Классифицированный диапазон семантического токена: общий словарь
    producer (analysis) и consumer (server) — прецедент `Symbol`.

    `range`: диапазон токена в координатах файла
    `typ`: класс токена; словарь фиксирован легендой сервера —
    known/unknown/deprecated/section/kind
  properties:
    "Range -> Range": |
      Диапазон токена в координатах файла.
    "Type -> string": |
      Класс токена из словаря легенды.
```

### Cell: `internal/analysis` — modify

#### CODEMANIFEST

Entity `Analyzer` — два новых метода (после `AnalyzeXs`):

```yaml
    "TokensRms(file: RmsFile) -> tokens: []Token": |
      Классификация идентификаторов RMS-файла для semantic tokens.

      `file`: AST из rms.Parse; `tokens`: классифицированные диапазоны

      Algorithm:
      1. Обойти все statement (вкл. Children блоков) в порядке документа
      2. Имя команды: lookup `Store` по `lookups` — есть → known, нет →
         unknown; effect_percent → deprecated (перекрывает known)
      3. Имя секции → section; имена атрибутов — known/unknown по
         `Store`; идент-значения аргументов (константы RMS) —
         known/unknown по `Store`
      4. Отсортировать по позиции; перекрытия диапазонов не допускаются

      Requirements:
      - детерминированный порядок (позиция)
      - входной AST не изменяется
      - пороги did-you-mean из AnalyzeRms НЕ применяются

    "TokensXs(file: XsFile, externals: []Decl) -> tokens: []Token": |
      Классификация идентификаторов XS-файла для semantic tokens.

      `file`: AST из XsParse; `externals`: декларации замыкания
      (как AnalyzeXs); `tokens`: классифицированные диапазоны

      Algorithm:
      1. Собрать объявленные имена (файл + externals) — шаблон
         AnalyzeXs шаги 1–2
      2. Каждое ident-вхождение: объявлен или в kb (`Store.Function`,
         `Store.Constant`) → known, иначе → unknown
      3. Имя каждой декларации (function/variable/rule/event) → kind
         (перекрывает known на этом диапазоне)
      4. Отсортировать по позиции; перекрытий нет

      Requirements:
      - детерминированный порядок (позиция)
      - входной AST не изменяется
```

Импорт `Token` добавить к существующему `From: internal/common`
(рядом с `Diagnostic`, `Range`).

### Cell: `internal/server` — modify

#### CODEMANIFEST

1. Глобальный `Annotations` — добавить строку:
```yaml
  SemanticTokens: легенда known/unknown/deprecated/section/kind держится
  на сервере целиком (не настраиваемая — клиентские легенды
  различаются); полный файл, range-запросы и инкремент не поддерживаются.
```

2. `Initialize` — в перечень capabilities:
```yaml
      SemanticTokensProvider — SemanticTokensOptions{Legend:
      TokenTypes=[known, unknown, deprecated, section, kind],
      TokenModifiers=[]};
```

3. Новый метод после `Symbols` (имя интерфейса go.lsp.dev/protocol
   уточняется на design-трассировке; рабочая гипотеза SemanticTokensFull):

```yaml
    "SemanticTokensFull(ctx: Context, params: SemanticTokensParams) -> tokens: SemanticTokens, err: error": |
      Семантические токены полного файла (stateless, read-only).

      `params`: документ запроса; `tokens`: delta-закодированные токены

      Algorithm:
      1. Язык по расширению URI (общий шаблон Hover); неизвестный —
         Data:[] (не nil, не ошибка)
      2. .rms: `Parse` → `Analyzer.TokensRms`; каждый inline `XsBlock`:
         XsParse → `Analyzer.TokensXs` (externals — пустые), сдвиг
         диапазонов в координаты файла (шаблон DocumentHighlight)
      3. .xs: `XsParse` → внешние декларации `Closure` uri →
         `Analyzer.TokensXs`
      4. Каждый Token → (line, start, length) с конвертацией
         positionEncoding; type → индекс в легенде
      5. Delta-кодировка [Δline, Δstart, length, typeIdx] в документном
         порядке; результат Data (пустой — пустой slice, не nil)

      Requirements:
      - детерминированность: одинаковый вход → одинаковая Data
      - токены отсортированы по (line, start), перекрытий нет

      Constraints:
      - без range-запросов и инкремента (Full only)
      - легенда фиксирована на сервере
```

#### .usages/ files

- `internal/server/.usages/lifecycle.md` — capabilities: + semantic
  tokens; новая секция «Semantic tokens» (легенда, full-only, пустые
  документы — пустой Data).
- `.goga/usages/cooks/lsp-protocol.md` — новая секция **Semantic
  Tokens** (интерфейсный метод — проверить по исходнику protocol;
  SemanticTokensOptions/Legend в Initialize; delta-формат; пустой
  результат — Data:[] не nil; регистрация провайдера без
  range/incremental). Вносится в этом слоте (заявлено External
  Dependencies пачки).

## Dependency Map

```
common [+Token] ──> analysis [+TokensRms/TokensXs] ──> server [+SemanticTokensFull]
   ↑ Range/Symbol уже импортированы; analysis получает Token в существующий
     импорт common; server — аналогично (Symbol уже импортирован)
kb ──> analysis (Store lookups, без изменений)
rms/xs ──> analysis, server (без изменений)
```

## Verification Checklist

- [ ] `goga lint` — 0 ошибок после всех диффов
- [ ] `goga contract internal/common internal/analysis internal/server` — зелёные
- [ ] Имя интерфейсного метода протокола сверено по исходнику
      go.lsp.dev/protocol (прецеденты FoldingRanges/Symbols)
- [ ] lifecycle.md и lsp-protocol.md расширены, не заменены
- [ ] Реализация: make check; golden-тест легенды/позиций; stdio-тест
