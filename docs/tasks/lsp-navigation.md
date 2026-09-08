# Navigation in aoe2-lsp (definition / references / documentSymbol)

Status: Done — PR #3 (task/lsp-navigation)

## Current State

MVP `aoe2-lsp` реализует только diagnostics + hover + completion. В контракте
`server/CODEMANIFEST` методов Definition/References/DocumentSymbol нет — запросы
навигации уходят в `protocol.UnimplementedServer` и возвращают not-implemented.

Фундамент для навигации уже есть:

- `xs/ast.go`: парсер строит индекс вхождений идентификаторов с range
  (`symbols []symbol`, обслуживает `SymbolAt`/hover) — нет только связи
  «вхождение → декларирующий `Decl`»
- `Decl.Range`, `Stmt.Range`, `Section.Range`, `Statement.Range`,
  `XsBlock.Range` — позиции всех узлов AST сохранены
- В RMS нет пользовательских деклараций (`#define` в грамматике отсутствует,
  константы предопределены) — локальный go-to-definition осмысленен только
  для XS; в RMS навигация = outline + вхождения имён
- `RmsFile.Includes` (`#include`) парсится, но файлы с диска не резолвятся —
  кросс-файловая навигация сегодня невозможна

## Description

Добавить в `aoe2-lsp` однодокументную навигацию (three LSP features):

1. **`textDocument/definition`** — XS: вхождение идентификатора → range его
   декларации (функция/константа/переменная/rule/параметр) в том же файле,
   скоуп-aware (побеждает внутренняя декларация: параметр/локаль затеняют
   внешнюю). Для builtin-символов (prelude-интринсики) и RMS — пустой
   результат
2. **`textDocument/references`** — все вхождения символа в документе
   (XS: идентификаторы; RMS: имена констант/команд), с учётом
   `params.Context.IncludeDeclaration`
3. **`textDocument/documentSymbol`** — outline: RMS → дерево
   Section → Statement (+ `XsBlock`), XS → декларации; иерархический
   `DocumentSymbol` с корректными `Range`/`SelectionRange`

## Scope

**In scope:**

- Новые методы `Server`: Definition, References, DocumentSymbol + заявка
  провайдеров в capabilities `Initialize`
- Навигационный API ячейки `xs`: `DeclAt(pos)`, `Definition(pos)`
  (скоуп-aware), `References(name)`
- Outline-API ячейки `rms`: `Symbols()` (Section → Statement → XsBlock) +
  вхождения имён констант/команд
- Обновление контрактов: `xs/CODEMANIFEST`, `rms/CODEMANIFEST`,
  `server/CODEMANIFEST` (через `goga-brainstorm`/изменение ячеек)
- Юнит-тесты в ячейках + интеграционный LSP-тест по stdio

**Out of scope:**

- Кросс-файловая навигация (резолв `#include` с диска, workspace)
- `workspace/symbol`
- rename / prepareRename
- call hierarchy, type definition, implementation
- Семантическая фильтрация references по типам (поиск вхождений —
  синтаксический, по имени)

## Acceptance Criteria

- `go test ./...`, `goimports`, `golangci-lint run`, `goga lint`,
  `goga contract` для `xs`, `rms`, `server` — зелёные
- XS: definition на вызове функции возвращает range её декларации; на
  идентификаторе, затенённом параметром, — range параметра, не внешней
  переменной; на builtin — пустой результат
- XS: references возвращает все вхождения имени (декларация — по флагу
  `IncludeDeclaration`)
- RMS: documentSymbol возвращает дерево секция → команды, `SelectionRange`
  содержится в `Range` (требование спецификации)
- Интеграционный тест: LSP-клиент по stdio → initialize (capabilities
  заявлены) → definition в .xs → корректный Location; documentSymbol в .rms
  → дерево; references → список Location

## Stack

- **Language:** Go 1.23+ (существующий модуль, новых модулей нет)
- **LSP:** `go.lsp.dev/protocol` v1.0.1 (закреплён; типы
  Definition/References/DocumentSymbol входят в LSP 3.18)
- **Реализация:** поверх существующих AST `xs.XsFile` (индекс вхождений) и
  `rms.RmsFile`; без обращения к диску
- **Testing:** `testify` + `cmp` по `conventions`
- **Infrastructure:** нет

## External Dependencies

| Component                     | Usage file                            | Status                          |
|-------------------------------|---------------------------------------|---------------------------------|
| `go.lsp.dev/protocol`         | `.goga/usages/cooks/lsp-protocol.md`  | updated (2026-09-07, секция Navigation) |
| `testify`, `cmp`              | `.goga/usages/conventions.md`         | existing (covered by conventions) |

## Risks and Constraints

- **Скоуп-правила XS** — разрешение деклараций при затенении
  (параметр/локаль vs топ-левел) самая тонкая часть; в `analysis` уже есть
  `collectLocals`/`TypeEnv` — можно перенести/переиспользовать прилив
  логики, но навигация живёт в ячейке `xs` (синтаксический уровень)
- **`SelectionRange` ⊆ `Range`** — обязательное требование LSP для
  иерархических DocumentSymbol; нарушение клиенты отбрасывают
- **Union/Optional формы результатов** — DefinitionResult/
  DocumentSymbolResult — sealed interfaces; пустой результат =
  пустой slice, не nil (см. cook)
- **RMS statements без имени-токена** — у части команд «имя» — это сам
  токен команды; `SelectionRange` = range имени команды
- Контракты трёх ячеек меняются — порядок согласования:
  leaves (`xs`, `rms`) → `server` (см. `goga-cookbook`, design order)

## Scope Estimate

Одна задача, 3 подзадачи (порядок: 1 ∥ 2 → 3):

1. **xs-navigation** — `DeclAt`/`Definition` (скоуп-aware)/`References` на
   `XsFile` + контракт `xs` + тесты. Самостоятельная ценность: навигация на
   уровне библиотеки
2. **rms-outline** — `Symbols()` + вхождения имён + контракт `rms` + тесты.
   Самостоятельная ценность: outline-билдер
3. **server-wiring** — хендлеры Definition/References/DocumentSymbol,
   capabilities, интеграционный тест по stdio

## Existing Architecture

Затрагиваемые ячейки (см. `goga schema`):

- `xs` (leaf) — новый навигационный API над индексом вхождений
- `rms` (leaf) — outline-API; `Imports` не меняются
- `server` (root) — 3 новых метода; `Imports` не меняются (использует уже
  импортированные `XsFile`/`RmsFile`)

`analysis` и `kb` не затрагиваются: навигация синтаксическая (позиции и
вхождения), семантика и KB не нужны. Интеграционные требования: изменения контрактов
оформляются по процессу `goga` (brainstorm/изменение ячеек) до реализации.

## Notes

- Скоуп-решение принято 2026-09-07: **только один документ** (без резолва
  `#include` с диска); rename и workspace/symbol — исключены
- Примеры целевого Go-API (уточнятся на brainstorm):

```go
// xs: индекс вхождений уже есть (symbols []symbol), добавляем связь с декларацией
func (f XsFile) DeclAt(pos Pos) (Decl, bool)        // курсор на имени декларации
func (f XsFile) Definition(pos Pos) (Range, bool)   // вхождение → range декларации
func (f XsFile) References(name string) []Range     // все вхождения имени (вкл. декларацию)

// rms: outline-дерево
func (f RmsFile) Symbols() []Symbol                 // Section/Statement/XsBlock с range
func (f XsFile) Symbols() []Symbol                  // Decl-ы с range

// server: три новых LSP-метода
func (s *Server) Definition(ctx, *DefinitionParams) (DefinitionResult, error)
func (s *Server) References(ctx, *ReferenceParams) ([]Location, error)
func (s *Server) DocumentSymbol(ctx, *DocumentSymbolParams) (DocumentSymbolResult, error)
```

- `protocol.DefinitionResult` = `*Location | LocationSlice | DefinitionLinkSlice`;
  `protocol.DocumentSymbolResult` = `DocumentSymbolSlice |
  SymbolInformationSlice` (проверено по исходникам go.lsp.dev/protocol@v1.0.1)
- Cook `.goga/usages/cooks/lsp-protocol.md` дополнен секцией Navigation
  (2026-09-07): формы результатов, capability-флаги, правило
  SelectionRange ⊆ Range, IncludeDeclaration
