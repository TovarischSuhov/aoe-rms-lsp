# [ARCHITECTURE_PLAN]

## Topic

workspace-symbol — путь: `.goga/history/2026/task-workspace-symbol/arch.md`
(слот M волны 4 пачки editor-experience, формулировка — task.md в этом
же топике).

## Implementation Order

1. **`internal/server`** (modify) — единственная ячейка. Новых ячеек нет,
   зависимые ячейки-провайдеры (include/rms/xs/common) не меняются, поэтому
   порядок вырождается: одна ячейка, изменений контрактов ниже по графу нет.

## Artifacts

### Cell: `internal/server` — modify

#### CODEMANIFEST (`internal/server/CODEMANIFEST`)

Дифф к существующему файлу (Header/Body/Footer структурно не меняются):

**1. Header / глобальный `Annotations`** — добавить в конец блока (после
абзаца про dynamicRegistration watchers):

```yaml
  WorkspaceSymbol следует секции Workspace Symbols `lsp-protocol`:
  вселенная — открытые доки + их замыкания, fuzzy score desc с
  тай-брейком (URI, позиция), пустой результат — пустой slice, не nil.
```

**2. Body / Entity `Server` / methods / `Initialize`** — в перечень
capabilities между `CodeActionProvider с CodeActionKinds=[QuickFix]` и
`согласовать positionEncoding` добавить:

```yaml
      WorkspaceSymbolProvider — Boolean(true);
```

**3. Body / Entity `Server` / methods** — новый метод после
`DocumentSymbol` (перед `Shutdown`):

```yaml
    "WorkspaceSymbol(ctx: Context, params: WorkspaceSymbolParams) -> result: WorkspaceSymbolResult, err: error": |
      Fuzzy-поиск символов по вселенной «открытые доки + их замыкания»
      (stateless, read-only).

      `params`: query-строка фильтра; `result`: плоский список
      SymbolInformation (возможно пустой — не ошибка)

      Algorithm:
      1. Вселенная: URIs() `DocStore`; для каждого ури — текст из DocStore
         (парс по расширению по `rms-parsing`/`xs-parsing`) плюс `Closure`
         ури по `closure`; дедуп по URI (editor-state выигрывает у диска)
      2. Сбор: RMS — рекурсивный обход Symbols() с отбором kind=section
         (команды и xs-узлы пропускаются); XS — все топ-декларации Symbols()
      3. Фильтр: пустой query — все символы; иначе fuzzy subsequence-скоринг
         (case-insensitive, бонусы за подряд/начало имени), несовпавшие
         пропускаются
      4. Сортировка: score desc, тай-брейк (URI, позиция Selection)
      5. Каждый Symbol → SymbolInformation по `lsp-protocol` (Workspace
         Symbols): Name, Kind по таблице DocumentSymbol, Location{URI,
         Selection}; конвертация позиций по positionEncoding

      Requirements:
      - детерминированность: одинаковый вход → одинаковый порядок
      - пустой результат — пустой SymbolInformationSlice, не nil и не ошибка
      - stateless/read-only: состояние сервера не меняется

      Constraints:
      - без скана ФС воркспейса (только DocStore + Closure)
      - RMS-команды и inline-XS в выдачу не входят;
        workspaceSymbol/resolve не заявляем
```

#### .usages/ files

**File:** `internal/server/.usages/lifecycle.md` — расширение:

1. Секция «Advertised capabilities» — в перечень после documentHighlight
   добавить workspace symbol search (формулировка согласуется с текстом
   секции при применении).

2. Новая секция после «Cross-file navigation» (перед «In-file highlights»):

```markdown
## Workspace symbol search

workspace/symbol answers a fuzzy query over the union of every open
document and its include closure (disk-backed files included — same
universe as cross-file navigation). RMS files contribute sections only
(commands are noise and stay out); XS files contribute all top-level
declarations. An empty query returns the whole universe.

Preconditions:
- Results are ordered by match score (contiguous and word-start matches
  score higher), ties broken by URI then position — the order is stable
  across identical requests.
- Locations may point into files that are not open in the editor.
```

## Dependency Map

```
common ─┐
kb ─────┤
rms ────┤   (без изменений)
xs ─────┼──> include ──┐
analysis┤              ├──> server  [modify: +WorkspaceSymbol, +capability]
hints ──┤              │        └── lifecycle.md [+workspace symbol search]
complete┘──────────────┘
```

Новых Imports-связей нет; `WorkspaceSymbol` потребляет уже импортированные
типы (`Resolver`, `Closure`, `RmsFile`, `XsFile`, `Symbol`) и `DocStore`
ячейки.

## Verification Checklist

После применения плана (`goga apply` / ручная материализация):

- [ ] `goga lint` — 0 ошибок (DSL-синтаксис диффа корректен)
- [ ] `goga contract internal/server` — контракт зелёный
- [ ] В `Initialize`-аннотации перечислены все capabilities, включая
      `WorkspaceSymbolProvider Boolean(true)`
- [ ] `lifecycle.md` не потерял существующие секции (расширение, не замена)
- [ ] `.goga/usages/cooks/lsp-protocol.md` содержит секцию Workspace
      Symbols (внесена при формулировке — уже на диске)
- [ ] Реализация (после apply): make check зелёный; интеграционный
      stdio-тест workspace/symbol зелёный
