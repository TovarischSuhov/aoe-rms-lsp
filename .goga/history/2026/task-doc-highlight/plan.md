# Plan: `doc-highlight`

Компиляция `.goga/history/2026/task-doc-highlight/design.md` (design
commit `8d3979f`; контракты материализованы в `e4fee35`).
ralphex-compatible: одна задача за итерацию.

## Purpose

`textDocument/documentHighlight` в aoe2-lsp: все вхождения слова под
курсором в пределах текущего файла, kind=Text. Меняется одна ячейка —
`server` (метод `DocumentHighlight` на `Server`, capability, хелпер
`shiftRange`); rms/xs — read-only через готовый `ReferencesAt`.

Главные пробелы между контрактом и кодом:

- метод `DocumentHighlight` на `Server` отсутствует (клиент получает
  not-implemented от заглушки `UnimplementedServer`);
- `Initialize` не заявляет `DocumentHighlightProvider`;
- нет хелпера сдвига range из координат inline-блока в координаты
  документа (есть `shiftPos`/`unshiftPos` для позиций, `shiftDiags`
  для диагностик — паттерн готов).

Стратегия: один TDD-цикл на хендлер (контракт-тест → реализация →
логика), затем интеграционные сценарии с точными координатами.

## Context

### Contract Surface

**Method on `Server`** (server; `location: server/server.go`)
- `DocumentHighlight(ctx: Context, params: DocumentHighlightParams) ->
  highlights: []DocumentHighlight, err: error` — вхождения имени под
  позицией в текущем файле (stateless, read-only)
- Algorithm (из контракта, дословно):
  1. Язык по расширению URI (общий шаблон Hover/SignatureHelp);
     неизвестный — пустой список
  2. .rms: `Parse`; позиция внутри `XsBlock` → сдвиг в координаты
     блока → `XsParse` → ReferencesAt по `XsFile` → сдвиг диапазонов
     назад в координаты файла; иначе ReferencesAt по `RmsFile`
  3. .xs: `XsParse` → ReferencesAt по `XsFile`
  4. Каждый Range → protocol DocumentHighlight: Range с конвертацией
     positionEncoding, Kind=Text
- Requirements: пустой результат — пустой slice, не nil
- Constraints: без `Resolver`/`Closure` (вхождения только текущего
  файла); без Read/Write-дискриминации (Kind=Text всем)

**`Initialize` (изменение)**: список Boolean-провайдеров дополнить
`DocumentHighlightProvider` (D3 материализован; проверить код).

### Code Stack Trace (из design, дословно)

```
1. s.openDocument(uri) → (text, name, ok); ok=false → text=""
2. pos := s.fromProtocolPos(text, params.Position)
3. switch strings.HasSuffix(name, …):
   ├─ ".rms": file, _ := rms.Parse(text, name)
   │    for block in file.XsBlocks:
   │      block.Range.Contains(pos)?
   │        ├─ да: xsFile := xs.XsParse(block.Code, "inline:"+name)
   │        │      ranges := xsFile.ReferencesAt(unshiftPos(pos, block.Range.Start))
   │        │      каждый r → shiftRange(r, block.Range.Start); выход из цикла
   │        └─ нет: продолжить
   │    (ни один блок) ranges := file.ReferencesAt(pos)
   ├─ ".xs": xsFile, _ := xs.XsParse(text, name); ranges := xsFile.ReferencesAt(pos)
   └─ прочее: ranges = nil
4. out := make([]protocol.DocumentHighlight, 0, len(ranges));
   каждый r → {Range: s.toProtocolRange(text, r), Kind: protocol.DocumentHighlightKindText}
5. slog.DebugContext(ctx, "document_highlight", uri, line, col, count)
6. return out, nil
```

Новый хелпер (внутренний, server.go): `shiftRange(r, base common.Range)
common.Range` = `{Start: shiftPos(r.Start, base.Start), End:
shiftPos(r.End, base.Start)}` — зеркален `shiftDiags`.

### Usages Context

- `conventions` (.goga/usages/conventions.md): doc-комментарии,
  testify/require, table-driven, `Test<Component>_<Scenario>`,
  blank line между блоками, короткие функции
- `lsp-protocol` (.goga/usages/cooks/lsp-protocol.md, подсекция
  Document Highlight): `DocumentHighlightProvider: Boolean(true)`;
  `[]protocol.DocumentHighlight`; пустой slice ≠ nil; конвертация
  позиций по согласованному positionEncoding; без include-closure

### Imported Usages

- `rms-parsing` from `rms` — `rms/.usages/rms-parsing.md` (секция
  Navigation): `Parse`, `RmsFile.ReferencesAt`, `XsBlocks`/`Range`,
  reparse before query
- `xs-parsing` from `xs` — `xs/.usages/xs-parsing.md` (секция
  Navigation): `XsParse`, `XsFile.ReferencesAt` (включая декларацию)

### Local Usages

`internal/server/.usages/lifecycle.md` — обновлён при apply
(`e4fee35`), новых файлов план не создаёт.

### External Dependencies

- `go.lsp.dev/protocol` (в go.mod): `DocumentHighlightParams`,
  `DocumentHighlight`, константа `DocumentHighlightKindText` (=1,
  document_highlight.gen.go:14)
- testify (в go.mod): require
- stdlib: `strings`, `log/slog`

## Facts

- `openDocument` (server.go:684) возвращает `text, name, ok`;
  References игнорирует `ok` (закрытый док → text="" → пустой ответ) —
  тот же выбор здесь
- `fromProtocolPos` (server.go:899) — UTF-16→байт; `toProtocolRange`
  (server.go:910) — байт→протокол с текстом документа
- `shiftPos`/`unshiftPos` (server.go:813/831): первая строка блока
  получает/теряет также и колонку; `shiftDiags` (server.go:799) —
  образец сдвига диапазонов
- Inline-блок: `signatureHelpRms` (server.go:339) — `XsParse(block.Code,
  "inline:"+name)`, `unshiftPos(pos, block.Range.Start)`; блоки не
  пересекаются → `return` из цикла
- `ReferencesAt` xs **включает вхождение под позицией и декларацию**;
  rms — все одноимённые слова-токены (секции/команды/атрибуты/ident/
  const); уникальное слово → ровно 1 хайлайт
- Образец хендлера: `References` (server.go:545) — make(…, 0, len),
  debug-строка с count, `return out, nil`
- Фикстура inline-XS: `internal/rms/testdata/includes.rms`
  (`#includeXS` + `int seed = 0;` …)
- Тесты — под memory cap (systemd-run, см. CLAUDE.md)

## Gap Analysis

- Отсутствующая контрактная сущность: `Server.DocumentHighlight`
- Отсутствующее поведение: `DocumentHighlightProvider` не заявлен
- Отсутствующий внутренний хелпер: `shiftRange`
- Код для переиспользования: маршрутизация Hover/SignatureHelp,
  `fromProtocolPos`/`toProtocolRange`, `shiftPos`/`unshiftPos`,
  stdio-харнесс navigation_test.go
- Пробелы покрытия: 6 сценариев T1–T6 из design

---

## Tasks

> **Ветка**: вся работа в `task/doc-highlight`; атомарный коммит на
> задачу; PR в `master` после Task 2 (мерджит пользователь).
> **CODEMANIFEST — read-only** для исполнителя.

### Task 1: `Server.DocumentHighlight` — хендлер + capability (TDD coding)

Ячейка `server`. Контрактная сущность: метод `DocumentHighlight` на
entity `Server`, `location: server/server.go`; изменение `Initialize`
(capability). Реализация следует Code Stack Trace дословно; хелпер
`shiftRange` — приватный в server.go рядом с `shiftPos`. Usages:
`conventions`, `lsp-protocol` (подсекция Document Highlight),
`rms-parsing`/`xs-parsing` (ReferencesAt-фасады).

**Usages relevant to this task:**
- `conventions`: doc-комментарий на метод (почему, не пересказ),
  testify/require, `Test<Component>_<Scenario>`
- `lsp-protocol`: `[]protocol.DocumentHighlight` c `Kind:
  protocol.DocumentHighlightKindText`; пустой slice ≠ nil;
  `DocumentHighlightProvider: protocol.Boolean(true)`

**Covered contract entities:** `Server.DocumentHighlight`,
`Server.Initialize` (capability).

- [x] STEP 0: объявить задачу Task 1
- [x] STEP 1 (CONTRACT TESTS): stdio-сценарий: initialize отвечает
      `documentHighlightProvider=true`; запрос documentHighlight к
      .xs-документу НЕ возвращает MethodNotFound (заглушка отвечала
      бы ошибкой) — до реализации падает
- [x] STEP 2 (IMPLEMENTATION): метод `DocumentHighlight` по Code Stack
      Trace (маршрутизация по расширению; inline-блок через
      `unshiftPos`/`shiftRange`; `make(…, 0, len)`; debug-строка
      `document_highlight`); `DocumentHighlightProvider:
      protocol.Boolean(true)` в capabilities Initialize; хелпер
      `shiftRange` с doc-комментарием
- [x] STEP 3 (INTERFACE VERIFICATION): контракт-тесты STEP 1 зелёные;
      `goga contract internal/server` зелёный
- [x] STEP 4 (LOGIC TESTS): сценарии T4 (позиция вне слова-токена →
      len==0, err==nil), T5 (неизвестное расширение → len==0),
      T6 (уникальное слово → len==1, Kind==1) — table-driven где
      уместно, stdio-харнесс navigation_test.go
- [x] STEP 5 (DEBUGGING): все тесты ячейки зелёные под memory cap;
      фиксить реализацию, не тесты
- [x] STEP 6 (CONTRACT RE-VERIFICATION): `goga contract internal/server`
      зелёный; пустой slice ≠ nil на всех ветках
- [x] STEP 7 (LINT): `golangci-lint run`, `golangci-lint fmt`; при
      необходимости декомпозировать (выделить documentHighlightRms)
- [x] STEP 8 (COMPLETION): отметить чекбоксы; коммит
      `feat: server cell — DocumentHighlight хендлер + capability (doc-highlight, task 1)`

### Task 2: Интеграционные сценарии T1–T3 (integration tests)

Ячейка `server`. Тесты поведения из design (T1–T3), stdio-харнесс
navigation_test.go; assertion'ы с точными координатами. Кода помимо
тестов нет (если T3 не вскрыл дефект сдвига — тогда фикс в хелпере,
не в контракте).

**Covered contract entities:** `Server.DocumentHighlight` (поведение).

- [ ] T1 `TestDocumentHighlight_XsOccurrences`: map.xs с
      `int towerCount = 2;` и двумя использованиеми; highlight на
      использовании → len==3 (декларация+2), все Kind==1, координаты
      по фикстуре
- [ ] T2 `TestDocumentHighlight_RmsAttributeName`: map.rms с
      `land_percent` в двух секциях; highlight на имени атрибута →
      len==2, Kind==1
- [ ] T3 `TestDocumentHighlight_InlineXsShift`: map.rms с
      `#includeXS`-блоком (шаблон includes.rms: `seed` ×3); highlight
      на `seed` вне первой строки блока → len==3, все range в
      координатах внешнего .rms (первая строка блока — со сдвигом
      колонки)
- [ ] Прогон всех тестов ячейки под memory cap; `make check` зелёный
- [ ] `goga lint` + `goga contract internal/server` зелёные
- [ ] STEP 8 (COMPLETION): коммит
      `test: server cell — интеграционные сценарии documentHighlight T1–T3 (doc-highlight, task 2)`
- [ ] Открыть PR `task/doc-highlight` → `master` (мерджит пользователь)

## Validation Commands

- Тесты (memory cap):
  `timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 bash -c 'go test ./internal/server/... -count=1'`
- Все тесты: та же команда с `./...`
- Линт: `golangci-lint run`; формат: `golangci-lint fmt`
- Контракт: `goga contract internal/server`; DSL: `goga lint`
- Полный прогон: `make check`
