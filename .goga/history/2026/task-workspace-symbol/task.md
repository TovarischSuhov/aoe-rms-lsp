# workspace-symbol: fuzzy-поиск символов открытых доков и замыканий

## Current State

server отдаёт per-file outline (`DocumentSymbol`, дерево из `Symbols()`),
Definition/References — кросс-файловые по include-замыканию. `DocStore`
хранит открытые документы (`URIs()`), `Resolver.Closure(uri)` — файлы
замыкания с готовыми AST (editor-state приоритет, диск как fallback).
`workspace/symbol` хендлера нет; kind-таблица Symbol → protocol.SymbolKind
уже существует (DocumentSymbol).

## Description

Новый хендлер `WorkspaceSymbol` в ячейке server: fuzzy-поиск символов по
вселенной «открытые документы + их include-замыкания». RMS-файлы отдают
только секции (команды — шум), XS — все топ-декларации. Результат — плоский
`[]SymbolInformation`; контракт CODEMANIFEST server меняется до кода.

## Scope

**In scope:**

- `WorkspaceSymbol(ctx, params)` в server: вселенная = `DocStore.URIs()` +
  файлы `Closure` каждого открытого дока (дедуп по URI; открытые доки
  парсим по тексту из DocStore, stateless)
- RMS-состав: узлы kind=section любой вложенности; XS-состав: все
  топ-декларации `Symbols()`
- Fuzzy-фильтр: свой компактный subsequence-скоринг без зависимостей
  (прецедент — Левенштейн в did-you-mean): case-insensitive, бонусы за
  подряд/начало имени; score desc, тай-брейк (URI, позиция); пустой
  query — все символы в порядке URI → позиция
- Конвертация: Symbol → `SymbolInformation{Name, Kind,
  Location{URI, Selection}}`; kind-таблица паритетна DocumentSymbol;
  позиции — с учётом positionEncoding
- `WorkspaceSymbolProvider: Boolean(true)` в Initialize
- Интеграционный stdio-тест в server (как существующие
  navigation-сценарии)
- CODEMANIFEST server — до кода
- Секция Workspace Symbols в `.goga/usages/cooks/lsp-protocol.md`
  (добавлена при формулировке)

**Out of scope:**

- скан файловой системы воркспейса (только открытые + замыкания)
- `workspaceSymbol/resolve` (WorkspaceSymbolResolveSupport не заявляем)
- индекс-кэш/инкрементальность
- `#define`/`#const` как символы RMS (территория слота rename)
- изменения ячеек rms/xs/include (обходится готовыми API)

## Acceptance Criteria

- `make check` зелёный; `goga contract internal/server` зелёный
- query по символу из невключённого открытого дока находит его
- секция из include-замыкания находится по fuzzy-подстроке имени
- команды RMS не попадают в выдачу (секции и XS-декларации — да)
- пустой query — все символы вселенной, детерминированный порядок
- пустой результат — пустой slice, не nil; интеграционный stdio-тест
  зелёный

## Stack

- **Frameworks:** Go 1.26+ (stdlib first), go.lsp.dev/protocol v1.0.1
  (закреплён)
- **Libraries:** новых зависимостей нет; fuzzy-скоринг — свой компактный
  (~50 строк + тесты)
- **Infrastructure:** без изменений (CI не трогаем)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| go.lsp.dev/protocol | `.goga/usages/cooks/lsp-protocol.md` | дополнен секцией Workspace Symbols при формулировке |

## Risks and Constraints

- Fuzzy-скоринг «свой» — держать компактным, порядок и тай-брейки
  покрыть тестами (детерминированность)
- Замыкание считается на каждый запрос (stateless): дисковые парсы
  кэшированы резолвером, открытые доки парсим заново — приемлемо;
  индекс-кэш не заводим (out of scope)
- Открытые доки с незнакомым расширением — молча пропускаются

## Scope Estimate

Одиночная задача (слот M, волна 4 пачки editor-experience), декомпозиция
не нужна. Ветка `task/workspace-symbol` → PR.

## Existing Architecture

- internal/server: новый метод хендлера + capability в Initialize;
  вселенная через `DocStore.URIs()` и `Resolver.Closure` (обе зависимости
  уже в контракте ячейки)
- internal/include: `Closure.Rms`/`Closure.Xs` (AST замыканий), контракт
  без изменений
- internal/rms, internal/xs: `Symbols()` как есть, без изменений
- kind-таблица DocumentSymbol переиспользуется

## Notes

- Формулировка утверждена пользователем 2026-09-09: состав RMS — только
  секции (команды — шум); query — fuzzy (score desc, тай-брейк URI →
  позиция, пустой query = все); вселенная — открытые доки + замыкания,
  без скана ФС.
- Секция lsp-protocol.md «Workspace Symbols» внесена сразу (как Code
  Actions в quickfix-слоте).
- Следующий шаг по процессу: S-слот реализуется сразу; этот слот M —
  при желании свой цикл brainstorm → design → plan в топике слота.
