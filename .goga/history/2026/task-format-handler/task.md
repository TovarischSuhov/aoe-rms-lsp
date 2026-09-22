# task-format-handler: server — хендлер textDocument/formatting (RMS)

Слот 1 пачки v1-final-mile (`.goga/history/2026/v1-final-mile/task.md`),
закрывает #46 частично; зеркало — issue #118 (PR закрывает `Fixes #118`).

## Current State

- `internal/format` в master (мерж PR #125, bf7a8bd): точка входа
  `RMS(source string, opts Options) (string, error)`; входы с
  error-диагностиками отказываются — `ErrParseErrors`, warnings
  форматируются; `Options{TabSize int, IndentTabs bool}`, zero value
  валидна (`TabSize` 0 → ширина 4)
- `internal/server`: все LSP-хендлеры, кроме `textDocument/formatting`;
  capability `DocumentFormattingProvider` в `Initialize` не объявлена
  (`server.go`, блок `ServerCapabilities`); текст открытых документов —
  `DocStore.Get/Text`; диспетчеризация .rms/.xs — идиома
  `case strings.HasSuffix(name, ".rms")` / `".xs"`; конвертация позиций с
  учётом negotiated encoding — `toProtocolPos`/`toProtocolRange`
- Канон `.goga/usages/cooks/lsp-protocol.md` есть, про formatting не пишет
- Типы протокола (go.lsp.dev/protocol, уже в go.mod):
  `DocumentFormattingParams{TextDocument, Options FormattingOptions}`,
  `FormattingOptions{TabSize uint32, InsertSpaces bool, …}`

## Description

Хендлер `textDocument/formatting` для .rms-документов поверх готового
`format.RMS`:

- capability `DocumentFormattingProvider` в `Initialize` (boolean-форма,
  как `FoldingRangeProvider`)
- метод `Formatting` на `Server`: текст из `DocStore` → `format.RMS` →
  ровно один full-document TextEdit (диапазон `{0,0}`–конец документа,
  конец через `toProtocolPos`)
- маппинг `FormattingOptions` → `format.Options`:
  `TabSize` — как есть (0 → дефолт внутри format), `InsertSpaces=false`
  ⇒ `IndentTabs=true`
- отказ: `errors.Is(err, format.ErrParseErrors)` → пустой срез
  `[]protocol.TextEdit{}` — не nil и не JSON-RPC-ошибка: клиент оставляет
  текст как есть, причина уже видна в diagnostics
- .xs-документы → пустой срез edits без вызова format (расширит слот 3)

Форма метода (имена параметров — по коду сервера):

```go
func (s *Server) Formatting(ctx context.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error)
```

## Scope

**In scope:**

- `internal/server`: метод `Formatting`, capability в `Initialize`,
  table-driven тесты через существующий pipe-харнесс (`serve_test.go`):
  чистый .rms → один TextEdit, байт-идентичный `format.RMS` с теми же
  опциями; error-диагностики → пустой не-nil срез; .xs → пустой срез;
  TabSize/InsertSpaces маппинг (табы при `InsertSpaces=false`)
- `internal/server/CODEMANIFEST`: новый метод контракта (обратное
  расширение, не read-only правка — через goga-change flow)
- `.goga/usages/cooks/lsp-protocol.md`: секция formatting (capability,
  семантика отказа пустым срезом, маппинг опций)

**Out of scope:**

- форматирование .xs (слот 3, #120) и `format.XS`
- `textDocument/rangeFormatting` — эпиком не заявлен
- настройки форматирования в секции `aoe2lsp` (конфигурация клиента)
- несколько/инкрементальные edits, EOL-переговоры (формат сохраняет
  доминирующий EOL входа — уже решено в format)

## Acceptance Criteria

- capability `DocumentFormattingProvider` объявлена в `Initialize`
- formatting .rms без error-диагностик → ровно один full-document
  TextEdit; `NewText` байт-идентичен `format.RMS(text, opts)`
- error-диагностики → пустой срез edits (не nil), без JSON-RPC-ошибки
- .xs-документ → пустой срез edits
- `InsertSpaces=false` → в выводе табы; `TabSize` пробрасывается
- `make check` зелёный, `goga contract server` зелёный, CI ветки зелёный

## Stack

- **Frameworks:** —
- **Libraries:** stdlib Go 1.26+; go.lsp.dev/protocol (уже в go.mod);
  internal/format (готов, без изменений)
- **Infrastructure:** —

## External Dependencies

| Component | Usage file | Status |
|-----------|------------|--------|
| go.lsp.dev/protocol (FormattingOptions, TextEdit) | `.goga/usages/cooks/lsp-protocol.md` | existing → updated (секция formatting) |

Новых внешних зависимостей нет.

## Risks and Constraints

- конец full-document TextEdit зависит от negotiated encoding
  (utf-16/utf-8) — считать через `toProtocolPos`, не вручную
- `FormattingOptions` несёт и опции вне маппинга
  (`TrimTrailingWhitespace` и др.) — игнорировать, не расширять
  `format.Options` в этой задаче
- CODEMANIFEST `server` — модификация публичной поверхности: добавить
  метод контракта той же правкой, `goga contract server` должен остаться
  зелёным

## Scope Estimate

Единая задача (size small по issue #118): одна ячейка + канон + тесты.
Разбивка не нужна.

## Existing Architecture

- `internal/server`: `Server` (protocol.UnimplementedServer, DI в
  `NewServer`), `Initialize` — блок `ServerCapabilities`, `DocStore`,
  pipe-тесты `serve_test.go`; конвенции — `.goga/usages/conventions.md`
- `internal/format`: `RMS`, `Options`, `ErrParseErrors` — потребитель,
  изменений нет

## Notes

- Отказ по error-диагностикам уже реализован внутри `format.RMS`
  (`refuse`); хендлер только транслирует `ErrParseErrors` → пустой срез
- Решение «пустой срез, не nil» зафиксировано в AC пачки v1-final-mile:
  клиент различает nil (нет хендлера) и `[]` (ничего не делать)
- Прочие опции форматирования LSP 3.15+ остаются без эффекта до
  отдельного решения (кандидат 1.x)
