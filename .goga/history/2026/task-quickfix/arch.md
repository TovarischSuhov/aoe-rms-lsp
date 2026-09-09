# Architecture Plan: quickfix

## Topic

**quickfix** — `textDocument/codeAction` в ячейке server (волна 1 пачки
editor-experience, M-слот; компактный arch → apply → TDD). Состав
фиксов утверждён пользователем: все три.

## Implementation Order

Только `internal/server` (modify). analysis/kb не меняются: источник
всех данных — диагностики, присланные клиентом в
`params.Context.Diagnostics` (клиент эхом возвращает показанное), и
сообщения с задокументированными форматами (checks.md, тесты пинкуют).

## Дизайн-решения

- **Stateless от контекста клиента**: правки вычисляются только из
  `params` — никаких сохранённых диагностик; в файлы не пишем.
- **Диапазон слова для замены**: диагностики не несут точный
  name-токен (`Attribute.Range` — вся строка; unknown-command —
  нулевая ширина). Слово берётся через ReferencesAt-роутинг
  DocumentHighlight: диапазон, содержащий начало диагностики. Из
  DocumentHighlight выделяется общий хелпер `occurrenceRanges`.
- **Три фикса**:
  1. unknown-command/unknown-attribute/undefined-symbol с суффиксом
     `; did you mean "X"?` → `Change to 'X'` (TextEdit слова → X)
  2. deprecated-effect-percent → `Replace with effect_amount`
     (замена ТОЛЬКО токена команды; семантика последнего аргумента
     %/абсолют остаётся автору — осознанно, зафиксировано в title)
  3. missing-include (`include not found: <path>`) → `Create '<path>'`
     (CreateFile с IgnoreIfExists по пути относительно директории
     документа; клиенты без resourceOperations молча игнорируют)
- **Only**: непустой `params.Context.Only` без QuickFix → пустой ответ.
- Kind всех действий — QuickFix; пустой результат — пустой slice ≠ nil.

## Artifacts

### Cell: `internal/server` — modify

**CODEMANIFEST**:

- новый метод `CodeAction` (после `FoldingRanges`, перед
  `DocumentSymbol`):

```yaml
    "CodeAction(ctx: Context, params: CodeActionParams) -> actions: []CodeAction, err: error": |
      Квикфиксы по диагностикам контекста запроса (stateless).

      `params`: диапазон запроса, диагностики клиента и фильтр Only;
      `actions`: QuickFix-действия; `err`: только протокольные сбои

      Algorithm:
      1. Отобрать диагностики контекста с кодами unknown-command/
         unknown-attribute/undefined-symbol/
         deprecated-effect-percent/missing-include, пересекающиеся
         с params.Range
      2. unknown-* с суффиксом '; did you mean "X"?' в сообщении →
         Change to 'X': замена слова под началом диагностики
         (ReferencesAt-роутинг как DocumentHighlight)
      3. deprecated-effect-percent → Replace with effect_amount:
         замена токена команды (аргументы не трогаются)
      4. missing-include → Create '<path>': CreateFile по пути из
         сообщения, разрешённому относительно директории документа
         (IgnoreIfExists)
      5. Only непусто и без QuickFix → пустой ответ

      Requirements:
      - пустой результат — пустой slice, не nil
      - stateless: только params, без сохранённых состояний

      Constraints:
      - файловая система не читается и не меняется (только Edit)
```

- `Initialize`: `CodeActionProvider — CodeActionOptions с
  CodeActionKinds=[QuickFix]`.

**`.usages/lifecycle.md`**: capabilities + строка про квикфиксы.

**Cook `lsp-protocol.md`**: подсекция Code Actions после Folding Ranges
(union `[]CommandOrCodeAction`, `&CodeAction`, Changes vs
DocumentChanges/CreateFile, Only).

## Verification Checklist

- [ ] `goga lint` 0; `goga contract internal/server` зелёный после
      реализации
- [ ] capability: codeActionProvider с kinds=[quickfix]
- [ ] did-you-mean rename: TextEdit покрывает ровно слово, NewText —
      подсказка; для .rms с inline-XS — координаты документа
- [ ] effect_percent: замена токена, аргументы нетронуты
- [ ] missing-include: CreateFile с IgnoreIfExists и путём от
      директории документа
- [ ] Only без QuickFix → пусто; без диагностик → пустой slice ≠ nil;
      диагностики без суффикса/чужие коды → действий нет
- [ ] `make check` зелёный
