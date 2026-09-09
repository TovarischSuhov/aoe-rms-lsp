# Architecture Plan: doc-highlight

## Topic

**doc-highlight** — `textDocument/documentHighlight` в ячейке server
(волна 0 пачки editor-experience,
`.goga/history/2026/editor-experience/task.md`).

План: `.goga/history/2026/task-doc-highlight/arch.md`.

## Implementation Order

1. **`internal/server`** (modify) — единственная ячейка с изменениями.
   Порядок не важен (одна ячейка); провайдеры `internal/rms`,
   `internal/xs` — read-only, их контракты не меняются.

Артефакты project-level (вне ячеек): `.goga/usages/cooks/lsp-protocol.md`
(подсекция Navigation), применяются вместе с ячейкой.

## Artifacts

### Cell: `internal/server` — modify

#### CODEMANIFEST diff (`internal/server/CODEMANIFEST`)

**D1 — Header → Annotations** (строка о навигационных хендлерах):

```yaml
# БЫЛО:
#   Навигационные хендлеры (Definition/References/DocumentSymbol) следуют
#   секции Navigation `lsp-protocol`: пустой результат — пустой slice, не nil.
# СТАЛО:
#   Навигационные хендлеры (Definition/References/DocumentSymbol/
#   DocumentHighlight) следуют секции Navigation `lsp-protocol`: пустой
#   результат — пустой slice, не nil.
```

**D2 — Body → `Server.methods`**: новый метод после `References`
(перед `DocumentSymbol`):

```yaml
    "DocumentHighlight(ctx: Context, params: DocumentHighlightParams) -> highlights: []DocumentHighlight, err: error": |
      Вхождения имени под позицией в текущем файле (stateless, read-only).

      `params`: позиция и документ запроса; `highlights`: подсвечиваемые
      диапазоны текущего файла; `err`: только протокольные сбои

      Algorithm:
      1. Язык по расширению URI (общий шаблон Hover/SignatureHelp);
         неизвестный — пустой список
      2. .rms: `Parse`; позиция внутри `XsBlock` → сдвиг в координаты
         блока → `XsParse` → `XsFile.ReferencesAt` → сдвиг диапазонов
         назад в координаты файла; иначе `RmsFile.ReferencesAt`
      3. .xs: `XsParse` → `XsFile.ReferencesAt`
      4. Каждый Range → protocol DocumentHighlight: Range с конвертацией
         positionEncoding, Kind=Text

      Requirements:
      - пустой результат — пустой slice, не nil

      Constraints:
      - без `Resolver`/`Closure`: замыкание не вычисляется, вхождения
        только текущего файла
      - без Read/Write-дискриминации: Kind=Text всем вхождениям
```

**D3 — Body → `Server.methods.Initialize`**: в перечислении
Boolean-провайдеров добавить `DocumentHighlightProvider`:

```yaml
      # БЫЛО: DefinitionProvider, ReferencesProvider,
      #       DocumentSymbolProvider — Boolean(true);
      # СТАЛО: DefinitionProvider, ReferencesProvider,
      #        DocumentSymbolProvider, DocumentHighlightProvider —
      #        Boolean(true);
```

Footer, Imports, Usages, остальные методы — без изменений.

#### `.usages/` diff (`internal/server/.usages/lifecycle.md`)

**U1** — секция `Advertised capabilities`, дополнение списка:

```md
Initialize advertises: diagnostics (Full sync + OpenClose), hover,
completion, navigation — definition, references, documentSymbol,
documentHighlight — and signature help (TriggerCharacters "(" and ",").
```

**U2** — новая секция после `Cross-file navigation`:

```md
## In-file highlights

documentHighlight returns every occurrence of the word under the cursor
within the current file only — RMS word tokens (section/command/attribute
names, ident/const values) or XS names, including inline-XS regions of
.rms files. All highlights carry kind Text: occurrences are syntactic
name matches, the server does not distinguish reads from writes.
Inline regions report ranges in the outer .rms file's coordinates, so
the editor highlights the source text as written.
```

### Project-level usage (вне ячеек)

#### `.goga/usages/cooks/lsp-protocol.md` — modify

**C1** — новая подсекция в конце секции `Navigation (Definition /
References / DocumentSymbol)` (перед `## Cross-file Navigation Results`):

```md
### Document Highlight

`textDocument/documentHighlight` returns in-file occurrences of the
word under the cursor. Advertise `DocumentHighlightProvider:
protocol.Boolean(true)` in Initialize. Return
`[]protocol.DocumentHighlight` — each entry carries `Range` (converted
per the negotiated positionEncoding) and `Kind` (`protocol.Text`).
Empty result is an empty slice, not nil. Unlike Definition/References,
no include closure is computed: highlights never cross file
boundaries; inline-XS regions of .rms files shift block coordinates
back to the outer file before returning.
```

## Dependency Map

```
internal/common ─┐
internal/kb ─────┤
internal/rms ────┼──(Imports без изменений)──► internal/server
internal/xs ─────┤      [modify: D1–D3]
internal/analysis┤
internal/hints ──┤
internal/complete┤
internal/include ┘
```

Новых Imports нет: `Parse`, `RmsFile`, `XsBlock`, `XsParse`, `XsFile`
уже импортированы; `ReferencesAt` — методы импортированных типов.
Циклов нет (server — корень).

## Verification Checklist

- [ ] `internal/server/CODEMANIFEST` после D1–D3: `goga lint` — 0 ошибок
- [ ] `goga contract internal/server` зелёный **после** реализации метода
      в `server.go` (до реализации контракт честно красный — метод
      объявлен, но не экспортирован)
- [ ] `internal/server/.usages/lifecycle.md` (U1–U2): секция на месте,
      список capabilities актуален
- [ ] `.goga/usages/cooks/lsp-protocol.md` (C1): подсекция внутри
      Navigation, до `Cross-file Navigation Results`
- [ ] Итог: `make check` зелёный; stdio-интеграционный тест
      documentHighlight (позиция на вхождении → все вхождения файла,
      kind=Text; позиция на слове без пар → пустой список)

## Принятые решения (из brainstorm)

- kind=Text(1) всем вхождениям — ReferencesAt синтаксический, без
  ролей; Read/Write-дискриминация осознанно вне скоупа
- Resolver/Closure не участвуют: documentHighlight строго в пределах
  текущего файла
- Отдельная ячейка не создаётся: ответственность «навигационные
  хендлеры» уже у server
