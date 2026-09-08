# Completion (textDocument/completion) для AoE2 RMS + XS

## Current State

Сервер aoe2-lsp реализует: `definition`, `references`, `documentSymbol`,
`signatureHelp`, диагностику (push), include-замыкание и кросс-файловую
навигацию. **Completion есть только в виде базового MVP прямо в `server`**
(handler `Server.Completion`, контракт-green): `.rms` — команды секции
(`SectionAt` → `Store.Commands`) + все константы; `.xs` — функции/константы
по префиксу слова; inline-XS-блоки, атрибуты, значения аргументов и локальные
символы XS не поддерживаются; вычислители зашиты в server, отдельной
протоколо-независимой ячейки нет. *(Уточнено на этапе brainstorm-context:
изначальная формулировка «отсутствует полностью» была неточна.)*

Вход для задачи уже подготовлен (в том числе работой над signature help):

- `kb.Store` — команды RMS (с атрибутами и аргументами), XS-функции и
  константы, `ValueRange` + kind words (намайнены из Desc);
- `rms.RmsFile.ArgAt` — позиционный контекст: команда-владелец, аргумент vs
  атрибут (band-owner, записанные спаны);
- `xs.XsFile` — `Decl` (символы пользователя: функции/константы/переменные)
  + parser-индексы.

## Description

Реализовать `textDocument/completion` для `.rms` и `.xs` (+ inline-XS-блоки
внутри RMS):

1. **Новая протоколо-независимая ячейка `complete`** (по образцу `hints` —
   одна зона ответственности, одна ячейка): вычислитель кандидатов
   - **RMS**: имена команд (по префиксу и секции), атрибуты команды
     (`<...>`), слова-значения аргументов (kind words из kb), константы;
   - **XS**: встроенные функции/константы из kb + символы пользователя из
     `XsFile` (видимые в точке).
2. **server**: handler `Completion`, capability `CompletionProvider`,
   роутинг `.rms` / `.xs` / inline-XS (`unshiftPos`) — по образцу
   signature help.
3. Рендер: кандидаты → `protocol.CompletionItem` (Kind-маппинг, `Detail`
   одной строкой, `SortText`), plain text без snippets, фильтрация по
   префиксу — на клиенте.

## Scope

**In scope:**
- Ячейка `complete`: CODEMANIFEST, bootstrap, Computer (RmsAt/XsAt),
  протоколо-независимая модель кандидата;
- RMS-контексты: позиция команды, позиция атрибута, позиция значения
  аргумента (kind words), константы;
- XS-контексты: встроенные функции/константы kb + пользовательские символы;
- inline-XS-блоки в RMS (координатная трансляция unshiftPos);
- server: handler `Completion`, capability, DI;
- Тесты: unit по контекстам + integration full stack (по образцу
  signature help).

**Out of scope:**
- Hover (отдельная задача);
- `completionItem/resolve` (lazy detail/documentation);
- auto-import, добавление отсутствующих include;
- snippets-инфраструктура (`InsertTextFormat` — только plain text);
- расширенная документация в `Documentation` у items (concise-items).

## Acceptance Criteria

- `Initialize` анонсирует `CompletionProvider`.
- В `.rms`: кандидаты на позицию имени команды (из kb, по секции/префиксу
  контекста), на позицию атрибута команды, на позицию значения аргумента
  (kind words), константы.
- В `.xs` и inline-XS: встроенные функции/константы из kb + символы
  пользователя, видимые в точке; inline-XS использует unshiftPos.
- Stateless-правило: результат зависит только от (документ, позиция), не от
  `params.Context` и предыдущих ответов.
- Нет кандидатов → `CompletionList` с пустыми items (не nil, не ошибка).
- Метрики items: `Label` = текст кандидата, `Kind` из маппинга, `Detail`
  одной строкой, `SortText` задан; `InsertText` опущен, когда совпадает с
  Label.
- Валидация: `go test ./...` (memory cap), `goimports`, `golangci-lint`,
  `goga lint`, `goga contract` по затронутым ячейкам — зелёные; существующие
  тесты не меняются (SC8-правило signature help).

## Stack

- **Frameworks:** —
- **Libraries:** `go.lsp.dev/protocol` + `go.lsp.dev/jsonrpc2` +
  `go.lsp.dev/uri` (в go.mod), `testify` (тесты)
- **Infrastructure:** — (stdio LSP-сервер, без новых компонентов)

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| go.lsp.dev/protocol | `.goga/usages/cooks/lsp-protocol.md` | updated (секция «Completion Items») |

Новых внешних зависимостей нет.

## Risks and Constraints

- Дискриминация RMS-контекстов (команда vs атрибут vs значение) опирается на
  `ArgAt` band-owner — пограничные случаи на стыках band'ов требуют
  отдельного внимания в дизайне.
- Scope-резолюция XS (какие символы видимы в точке): `XsFile` может не
  хранить блочных scope'ов — на design-этапе решить (худший случай —
  файловый уровень видимости).
- Объём кандидатов: сервер отдаёт полный контекстный набор, фильтрация на
  клиенте — проверить поведение на больших списках kb.
- Kind-маппинг (какой CompletionItemKind какому типу кандидата соответствует)
  — зафиксировать в дизайне, не в рантайме.
- Go 1.23+ совместимость; conventions.md обязателен.

## Scope Estimate

Одна задача-топик `completion` (прецедент — signature help: один топик →
brainstorm контрактов → design → план). Ожидаемая декомпозиция в плане:
~5–8 задач (bootstrap ячейки + CODEMANIFEST, RMS-компьютер, XS-компьютер,
модель кандидата/сортировка, server handler + capability, integration-тесты).

## Existing Architecture

- Новая ячейка `complete` (deps: `kb`, `rms`, `xs`, `common` — read-only
  Imports, как у `hints`).
- `server` импортирует `complete` (как импортирует `hints`); обратные
  импорты запрещены DSL.
- Контракт `hints` не меняется; `complete` не встраивает `Hint` — у
  completion своя модель кандидата.
- `.goga/usages/cooks/lsp-protocol.md` уже дополнен секцией «Completion
  Items» (kinds, SortText, stateless, фильтрация на клиенте).

## Notes

Решения, принятые при формулировке (2026-09-08):

- Одна задача, не две (RMS+XS делят ячейку, рендер и роутинг).
- Новая ячейка `complete`, не расширение `hints` (разные зоны
  ответственности).
- Фильтрация по префиксу — на клиенте; сервер отдаёт контекстный набор.
- Snippets, resolve, auto-import — вне области.

Целевое API (sketch, Go; контракты зафиксирует brainstorm):

```go
// Пакет complete — протоколо-независимый вычислитель кандидатов.
// Образец — hints.Computer.

// Candidate — один протоколо-независимый кандидат completion.
type Candidate struct {
	Label  string // текст кандидата; InsertText опускается, когда равен Label
	Kind   Kind   // протоколо-независимый вид: Command, Attribute, Value,
	//        Constant, Function, Variable (маппинг в CompletionItemKind — в server)
	Detail string // одна строка: диапазон значений, сводка параметров
	Sort   string // основа SortText: стабильный порядок контекстных групп
}

// Computer вычисляет кандидатов для RMS- и XS-контекстов.
type Computer struct { /* kb.Store — constructor DI */ }

func NewComputer(store *kb.Store) *Computer

// RmsAt — кандидаты в точке RMS-документа: имена команд (контекст секции),
// атрибуты команды-владельца, слова-значения аргумента (kind words), константы.
func (c *Computer) RmsAt(f *rms.RmsFile, pos common.Pos) []Candidate

// XsAt — кандидаты в точке XS-документа: встроенные функции/константы kb +
// символы пользователя, видимые в точке.
func (c *Computer) XsAt(f *xs.XsFile, pos common.Pos) []Candidate
```

```go
// server — handler и роутинг по образцу SignatureHelp.
func (s *Server) Completion(ctx context.Context, params *protocol.CompletionParams) (protocol.CompletionResult, error) {
	// роутинг: .xs | inline-XS блок (unshiftPos) | .rms
	// Candidate → protocol.CompletionItem (Kind-маппинг, Detail, SortText)
	return &protocol.CompletionList{IsIncomplete: false, Items: items}, nil
}
```
