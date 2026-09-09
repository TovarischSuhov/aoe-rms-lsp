# Architecture Plan: folding

## Topic

**folding** — `textDocument/foldingRange` в ячейке server (волна 0 пачки
editor-experience). Слот S: контракты — минимальный дифф, без полного
brainstorm-цикла (правило пачки: S-слоты реализуются сразу).

## Implementation Order

Только `internal/server` (modify); rms/xs — read-only (готовые `Symbols()`).

## Artifacts

### Cell: `internal/server` — modify

#### CODEMANIFEST diff

**D1 — Header → Annotations**, новая строка после навигационной:

```yaml
#   FoldingRanges строится из Symbols() по языку: регионы — узлы дерева
#   диапазоном больше одной строки; пустой результат — пустой slice, не nil.
```

**D2 — Body → `Server.methods`**: новый метод после `DocumentHighlight`
(перед `DocumentSymbol`). Имя — как в interface protocol.Server
(`FoldingRanges`, множественное число; диспетчер MethodTextDocumentFoldingRange):

```yaml
    "FoldingRanges(ctx: Context, params: FoldingRangeParams) -> ranges: []FoldingRange, err: error": |
      Регионы сворачивания текущего файла (stateless, read-only).

      `params`: документ запроса; `ranges`: построчные регионы; `err`:
      только протокольные сбои

      Algorithm:
      1. Язык по расширению URI (общий шаблон Hover/SignatureHelp);
         неизвестный — пустой список
      2. Symbols() по языку (`xs-parsing` / `rms-parsing`)
      3. Рекурсивный обход дерева (xs — плоский список): узел с
         End.Line > Start.Line → FoldingRange (StartLine, EndLine;
         символы и kind опускаются)
      4. Порядок — порядок обхода (документный)

      Requirements:
      - пустой результат — пустой slice, не nil
      - однолинейные узлы не отдаются

      Constraints:
      - xs — только топ-декларации (тела не раскрываются, семантика
        Symbols); вложенные области не сворачиваются
```

**D3 — `Initialize`**: `FoldingRangeProvider — Boolean(true)` в список
провайдеров.

#### `.usages/` diff (`internal/server/.usages/lifecycle.md`)

- `Advertised capabilities`: список + `folding ranges`

#### Cook (`.goga/usages/cooks/lsp-protocol.md`)

Подсекция после Document Highlight:

```md
### Folding Ranges

`textDocument/foldingRange` returns line regions from the symbol tree.
Advertise `FoldingRangeProvider: protocol.Boolean(true)`. The protocol
server-interface method is `FoldingRanges` (plural). Return
`[]protocol.FoldingRange` with only `StartLine`/`EndLine` set —
characters and kind stay unset (the client applies its defaults).
Emit every outline node whose range spans more than one line, in
document order; empty result is an empty slice, not nil.
```

## Dependency Map

Без изменений: FoldingRanges потребляет `Symbols()` уже импортированных
RmsFile/XsFile. Новых Imports нет.

## Verification Checklist

- [x] `goga lint` — 0 ошибок после диффов
- [x] `goga contract internal/server` зелёный после реализации
- [x] Тесты: capability; закрытый/неизвестный док → пустой slice ≠ nil;
      .rms: секция + многострочная команда + inline-XS, однолинейная
      команда не отдаётся; .xs: функция и rule сворачиваются,
      переменная нет; порядок документный
- [x] `make check` зелёный

## Принятые решения

- Источник регионов — `Symbols()` (без новых методов у rms/xs):
  rms-дерево даёт секции/команды/random-if/XsBlock; xs — топ-декларации
- kind и символы опускаются (клиентские дефолты); фильтр End > Start
- Имя метода — `FoldingRanges` (требование interface protocol.Server)
