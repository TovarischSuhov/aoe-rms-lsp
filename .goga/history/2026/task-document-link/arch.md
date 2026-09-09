# Architecture Plan — document-link

## Topic

document-link — `textDocument/documentLink` для директив `#include`/`#includeXS`
(подзадача 1 эпика v1-lsp-completeness).

План: `.goga/history/2026/task-document-link/arch.md`

## Implementation Order

1. `internal/server` — единственная изменяемая ячейка (корень). Все
   зависимости (`include` с `Closure.Resolved`, `rms` с `Include`,
   `common` с `Range`) уже существуют и не меняются; сборка снизу вверх
   вырождается в один шаг: server после без изменений существующих
   листьев.

Обоснование порядка: новых ячеек нет; ребро include→server уже
существует, добавляется только тип в Imports.

## Artifacts

### Cell: `internal/server` — MODIFY

#### CODEMANIFEST (`internal/server/CODEMANIFEST`) — diff, 4 изменения

**① Header → Imports (блок internal/include) — добавить тип:**

```yaml
  - Types:
      - Source
      - Resolver
      - Closure
      - Target
      - MissingInclude
      - ResolvedInclude          # NEW — итерация Closure.Resolved
    Usages:
      - closure
    From: internal/include
```

**② Header → Annotations — абзац в конец глобального блока (после
абзаца SemanticTokens):**

```yaml
  DocumentLinks следует секции Document Links `lsp-protocol`: ссылки только
  резолвленных include-директив запрошенного .rms-документа по `closure`
  (Owner == uri); missing-директивы пропускаются; пустой результат —
  пустой slice, не nil.
```

**③ `Server.methods` → `Initialize` — вставка в перечень capabilities:**

```yaml
      WorkspaceSymbolProvider — Boolean(true);
      DocumentLinkProvider — Boolean(true);              # NEW
      SemanticTokensProvider — ...
```

**④ `Server.methods` → новый метод (после `SemanticTokensFull`,
перед `Shutdown`):**

```yaml
    "DocumentLinks(ctx: Context, params: DocumentLinkParams) -> links: []DocumentLink, err: error": |
      Ссылки include-директив текущего документа (stateless, read-only);
      протокольное имя метода — DocumentLinks (как FoldingRanges/Symbols).

      `params`: документ запроса; `links`: ссылки на файлы-цели;
      `err`: только протокольные сбои

      Algorithm:
      1. Язык по расширению URI (общий шаблон Hover/SignatureHelp);
         не .rms — пустой список (include-директив в .xs нет)
      2. `Resolver`.Closure(ctx, uri) по `closure`
      3. Отобрать Resolved с Owner == uri; порядок — порядок директив
         в запрошенном файле
      4. Каждый → DocumentLink: Range из Inc.Range с конвертацией
         positionEncoding, Target из ResolvedInclude.Target (файл-цель
         может быть не открыт — Cross-file Navigation Results в
         `lsp-protocol`); Tooltip опускается
      5. Пусто → пустой slice (не nil, не ошибка)

      Requirements:
      - stateless/read-only: состояние сервера не меняется
      - детерминированность: одинаковый вход → одинаковый порядок

      Constraints:
      - Missing-директивы пропускаются (сигнал остаётся за диагностикой
        missing-include)
      - собственный резолв путей не строится — только Closure (includeRoots
        и watch-инвалидация уже в Resolver)
      - documentLink/resolve не заявляем (Target известен сразу)
```

#### .usages файлы

**Файл:** `internal/server/.usages/lifecycle.md` — EXTEND

- В секцию «Advertised capabilities» добавить document links в перечень
  (после workspace symbol search, перед semantic tokens — по порядку
  handler'ов).

- Новая секция (после «Workspace symbol search», перед «In-file
  highlights»):

```md
## Document links

textDocument/documentLink returns one clickable link per resolved
#include / #includeXS directive of the queried .rms document: the link
range covers the path argument, the target is the resolved file's URI
(opened on demand — the target need not be an open document).

Preconditions:
- Resolution follows the include closure — includeRoots and file
  watchers apply automatically; links stay consistent with
  missing-include diagnostics.
- Unresolved directives are skipped (the missing-include diagnostic is
  the signal); .xs documents return an empty list.
```

**Файл:** `.goga/usages/cooks/lsp-protocol.md` — EXTEND

Новая секция (после «### Code Actions», перед «### Workspace Symbols»):

```md
### Document Links

`textDocument/documentLink` returns `[]protocol.DocumentLink` — one
link per resolved include directive of the queried document. Advertise
`DocumentLinkProvider: protocol.Boolean(true)` in Initialize; the
server-interface method follows the result name (`DocumentLinks`, like
`FoldingRanges`/`Symbols`). Link `Range` covers the directive's path
argument; `Target` is the resolved file URI — it may point at a file
that is not open. Skip unresolved directives (the missing-include
diagnostic is the signal, not a targetless link); empty result is an
empty slice, not nil.
```

## Dependency Map

```
internal/common ─┬─(Range)─▶ internal/rms ─(Include)─▶ internal/include ─┐
                 │                                                     │
                 └─(Symbol, Token)─▶ internal/server ◀──────────────────┘
                                     ▲  Source, Resolver, Closure, Target,
                                     │  MissingInclude, ResolvedInclude  ← NEW тип
                                     │
                  kb/rms/xs/analysis/hints/complete ──── (существующие ребра)
```

Порядок листья→корень: `common → rms → include → server`. Циклов нет.

## Verification Checklist

После применения каждого артефакта:

- [ ] `goga contract internal/server` — контракт синтаксически валиден,
      метод `DocumentLinks` извлекается (имя совпадает с будущей
      Go-функцией), `ResolvedInclude` в Imports резолвится в include
- [ ] `goga lint` — 0 ошибок по всем ячейкам
- [ ] `internal/server/.usages/lifecycle.md`: секция Document links
      самодостаточна (без ссылок на другие практики), capabilities-строка
      соответствует реальному перечню
- [ ] `.goga/usages/cooks/lsp-protocol.md`: секция Document Links не
      дублирует аннотации CODEMANIFEST (описывает потребление, не контракт)
- [ ] Аннотация `DocumentLinks` упоминается глобальным абзацем (каждая
      подключённая практика использована в аннотации)
