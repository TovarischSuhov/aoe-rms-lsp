# Plan: `semantic-tokens`

## Purpose

Реализовать `textDocument/semanticTokens/full`: классификация
идентификаторов (легенда known/unknown/deprecated/section/kind) в
analysis + delta-кодирование в server. Контракты применены (apply),
дизайн-трассировка зафиксировала источники спанов.

## Context

Ключевые факты (design.md, проверены по исходникам):
- Метод протокола — `SemanticTokensFull`; `SemanticTokens{Data []uint32}`.
- Спан имени команды = `stmt.Range.Start` + `len(Name)` (имя — первый
  токен statement, parse.go:282/381); атрибут — `attr.Range`; секция —
  первое вхождение `References(sec.Name)` на линии заголовка; XS
  декларации — `Symbols().Selection`; ident/вызовы — `Expr.Range`
  (callee от `Range.Start` длиной `len(Callee)`).
- Предикат known для XS идентичен checkIdent (analyzer.go:398):
  declared ∪ xsBuiltins ∪ Store.Function ∪ Store.Constant.
- Inline-XS сдвиг координат — шаблон DocumentHighlight.

## Tasks

### Task 1: `common.Token` (infrastructure)

Чистый тип данных `token.go`: `Token{Range common.Range, Type string}`;
doc-комментарий; тест тривиальной структуре не нужен (conventions).
- [ ] Создать `internal/common/token.go`
- [ ] `go build ./...` зелёный; `golangci-lint run ./internal/common/...`
- [ ] `goga contract internal/common` — Token implementation не null

### Task 2: `analysis.TokensRms/TokensXs` (TDD coding)

Контракты — CODEMANIFEST analysis (Algorithm обоих методов); трассировки
— design.md. Реализация в `internal/analysis/tokens.go` (новый файл,
walk-хелперы рядом; переиспользовать стили walkStmts/checkIdent).
- [ ] **Contract tests**: `tokens_test.go` — таблица (Range, Type) на
  rms-фикстуре (секция/known/unknown/effect_percent/атрибуты/const) и
  xs-фикстуре (декларации kind/ident known-unknown/callee); упадут
- [ ] **Code**: `TokensRms` (statement-walk + References для секций)
- [ ] **Code**: `TokensXs` (Symbols().Selection для kind; walk тел для
  ident/callee; предикат checkIdent)
- [ ] **Interface verification**: прогоны обоих тестов зелёные
- [ ] **Logic tests**: перекрытия (kind > known), сортировка,
  partial-AST, директивы `#` пропущены, externals подавляют unknown
- [ ] **Debugging**: `go test ./internal/analysis/ -count=1` (memory cap)
- [ ] **Contract re-verification**: `goga contract internal/analysis`
- [ ] **Lint**: `golangci-lint run ./internal/analysis/...`

### Task 3: `server.SemanticTokensFull` + легенда + capability (TDD coding)

- [ ] **Contract tests**: `TestInitialize_AdvertisesSemanticTokens`
  (легенда/модификаторы); `TestServerSemanticTokens_APIShape` (пустой
  Data не nil)
- [ ] **Code**: Initialize — `SemanticTokensProvider` с Legend
- [ ] **Code**: `SemanticTokensFull` в server.go: парс по расширению →
  Tokens* → inline-XS сдвиг → utf-16 конвертация позиций/длин →
  delta-кодировка → `&protocol.SemanticTokens{Data}`
- [ ] **Interface verification**: контракт-тесты зелёные
- [ ] **Logic tests**: golden Data на rms-фикстуре с inline-XS и
  xs-фикстуре; не-ASCII имя (utf-16 длина); сортировка/delta на одной
  строке
- [ ] **Debugging**: `go test ./internal/server/ -count=1` (memory cap)
- [ ] **Contract re-verification**: `goga contract internal/server`
- [ ] **Lint**: `golangci-lint run ./internal/server/...`

### Task 4: интеграционный stdio-тест (integration tests)

- [ ] harness: didOpen .rms (unknown-команда + inline-XS) →
  `h.disp.SemanticTokensFull` → Data непуста, индексы согласованы с
  легендой из Initialize; пустой док → пустая Data
- [ ] Прогон под memory cap

## Validation Commands

- `make check`
- `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p
  MemorySwapMax=0 bash -c 'go test ./... -count=1'`
- `goga contract internal/common internal/analysis internal/server`
- `golangci-lint run`

## Completion Criteria

- [ ] Token/TokensRms/TokensXs/SemanticTokensFull реализованы по контрактам
- [ ] Легенда фиксирована: [known, unknown, deprecated, section, kind]
- [ ] Golden-тест легенды/позиций зелёный (приёмка слота)
- [ ] stdio-интеграция зелёная; CODEMANIFEST не менялись при реализации
- [ ] rms/xs ячейки не тронуты; все Validation Commands зелёные
