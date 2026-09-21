# Архитектурный план: format-rms

## Topic

`format-rms` — план по task.md этого топика (issue #45).
Артефакт плана: `.goga/history/2026/task-format-rms/arch.md`.

## Implementation Order

1. **`internal/rms` [modify]** — правка не зависит от новых артефактов
   (свойство открывает уже собираемые парсером данные); должна быть
   готова до контракта `format`, который её импортирует.
2. **`internal/format` [create]** — зависит от `rms` (Types `RmsFile`,
   `Parse`, Usages `rms-parsing`) и `common` (`Range`, `Diagnostic`).

Лист→корень: `common` (read-only, не трогается) → `rms` → `format`.
Будущий потребитель `server` (#46) в этот план не входит.

## Artifacts

### Cell: `internal/rms` — modify

**CODEMANIFEST** (`internal/rms/CODEMANIFEST`): заголовок, footer и все
существующие типы — без изменений. Единственная правка — в свойствах
`RmsFile`, после `XsBlocks -> []XsBlock` добавить:

```yaml
    "Comments -> []Range": |
      Диапазоны всех комментариев файла: /* … */ (включая
      многострочные), //… и #-строки вне словаря директив —
      по `rms_grammar`.

      Тексты комментариев не хранятся: потребитель читает их из
      исходника по диапазонам.

      Requirements:
      - точные экстенты: блок-комментарий от «/*» до «*/»
        включительно, строчный — до конца строки
      - отсортированы по позиции; диапазоны не пересекаются
```

**`.usages/`** — расширить `internal/rms/.usages/rms-parsing.md` новой
секцией (в конец файла):

```md
## Comment extents (formatting, comment-aware checks)

RmsFile.Comments carries every comment extent of the file — /* … */
(including multi-line), //… and #-lines that are not directives —
sorted by position. The parser does not keep comment texts: extract
them from the source by range.

```go
file, _ := rms.Parse(text, uri)
for _, r := range file.Comments {
	// r is a common.Range in absolute file coordinates;
	// text[r.Start.Offset:r.End.Offset] is the exact comment bytes
	// (from "/*" through "*/", or to the end of line)
}
```

Preconditions:
- Extents are byte-exact and non-overlapping.
- Positions inside them answer found=false in ArgAt — the same
  extents gate argument lookup.
```

### Cell: `internal/format` — create

**CODEMANIFEST** (`internal/format/CODEMANIFEST`), полный файл:

```yaml
Imports:
  - Types:
      - Range
      - Diagnostic
    From: internal/common
  - Types:
      - RmsFile
      - Parse
    Usages:
      - rms-parsing
    From: internal/rms

Usages:
  conventions: .goga/usages/conventions.md
  rms_grammar: .goga/usages/rms-grammar.md

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `rms_grammar` for the RMS language structure being printed.
  Use `rms-parsing` from Imports для потребления Parse и AST.

  Ячейка печатает отформатированный текст из готовых AST:
  stateless, протоколо-независима (без LSP-типов); результат
  зависит только от аргументов вызова. Печать перестраивает
  пробелы, отступы и пустые строки; тексты токенов — из AST;
  достоверность выхода относительно входа держится инвариантами
  контракта RMS.

---

"Options(tabSize: int, indentTabs: bool)":
  location: options.go
  annotations: |
    Параметры отступа печати — общие для точек входа ячейки.

    Requirements:
    - zero-value валиден: TabSize=0 означает дефолт 4
    - чистые данные: construct-and-use, без методов
  properties:
    "TabSize -> int": |
      Ширина одного уровня отступа в пробелах; 0 — дефолт 4.
    "IndentTabs -> bool": |
      true — один таб на уровень вместо TabSize пробелов.

"RMS(source: string, opts: Options) -> formatted: string, err: error":
  location: rms.go
  annotations: |
    Форматирование RMS-исходника в канонический стиль:
    отступы и пустые строки перестраиваются, семантика неизменна.

    `source`: текст .rms-файла; `opts`: параметры отступа;
    `formatted`: результат; `err`: отказ — вход содержит
    error-диагностики (sentinel, классификация errors.Is
    по `conventions`; `formatted` пуст)

    Algorithm:
    1. Разобрать вход: `Parse` по `rms-parsing` → `RmsFile` и
       diags; error-severity в `Diagnostic` → отказ
    2. Определить EOL входа: доминирующий из "\r\n"/"\n";
       нет переводов или равенство — "\n"
    3. Упорядочить элементы: слить Sections, Includes, XsIncludes,
       XsBlocks по Start их `Range` (исходный порядок); глобальные
       statements — из синтетической секции "global"
    4. Напечатать по `rms_grammar`: секция — <name>, содержимое
       с отступом уровня+1, </name>; команда — имя и позиционные
       аргументы одной строкой; каждый атрибут — отдельной строкой
       с отступом уровня+1; блоки random/conditional — рекурсивно
       (строка percent_chance/if, дети с отступом); директивы
       #include/#includeXS — как записаны; XsBlock — директива
       и Code вербатим
    5. Якорение комментариев: тексты из source по диапазонам
       свойства Comments у `RmsFile`; сравнением с диапазонами
       узлов поместить каждый в ближайшую меж-узловую позицию
       потока; порядок комментариев сохраняется; каждый — на
       отдельной строке
    6. Пустые строки: одна между элементами верхнего уровня,
       внутри секций/блоков — без пустых строк; один финальный
       перевод строки
    7. Склеить строки EOL-ом входа

    Requirements:
    - инварианты: parse(RMS(x)) структурно эквивалентен parse(x)
      (сравнение без Range); все тексты комментариев входа
      присутствуют в выходе байт-в-байт; RMS(RMS(x)) байт-идентичен
      RMS(x)
    - порядок позиционных аргументов и атрибутов команды неизменен
    - детерминизм: одинаковые вход и `Options` → одинаковый выход

    Constraints:
    - XsBlock.Code не переформатируется
    - не печатать содержимое, отсутствующее в AST (потеря ловится
      инвариантом на корпусе)

---

Author: Goga
CreatedAt: 21/09/26
Description: |
  Форматирование RMS: печать канонического стиля из AST с якорением
  комментариев; протоколо-независимая ячейка над rms/common.
```

**`.usages/`** — создать `internal/format/.usages/formatting.md`:

```md
# Formatting — consuming the format cell

Domain: reformatting RMS sources into the cell's canonical style.
Target audience: implementers of the server cell (the
textDocument/formatting handler) and tooling that rewrites map
scripts.

## Format a document

```go
out, err := format.RMS(text, format.Options{TabSize: 4})
if err != nil {
	// source has error-severity diagnostics — leave the text
	// unchanged and report no edits (classify with errors.Is)
}
// out is the whole formatted document: emit one full-document
// TextEdit, not per-line diffs
```

## Mapping LSP FormattingOptions (server)

```go
opts := format.Options{
	TabSize:    int(params.Options.TabSize),
	IndentTabs: !params.Options.InsertSpaces,
}
// zero value is valid: TabSize 0 formats with the default
// width of 4; InsertSpaces=false maps to tab indentation
```

Preconditions:
- Refusal is a sentinel error: classify with errors.Is — never a
  panic, never partial output (formatted is empty on refusal).
- The cell is stateless: call per request, no caching obligations.

## Guarantees to rely on

- parse(format(x)) is structurally equal to parse(x): replacing the
  document with the output never introduces new syntax errors on
  input the parser accepted.
- format(format(x)) is byte-identical to format(x): formatting an
  already formatted document produces no diff.
- Every input comment is present in the output (texts byte-exact);
  inline XS blocks pass through verbatim.
- The output keeps the input's dominant EOL (CRLF stays CRLF).
```

## Dependency Map

```
internal/common (read-only)
   │  Range, Diagnostic
   ├──> internal/rms [modify: RmsFile.Comments]
   │        │  RmsFile, Parse + usages rms-parsing
   │        └──> internal/format [create: Options, RMS]
   │                 ↑
   └─────────────────┘ Range, Diagnostic

(будущее #46: internal/server ──> internal/format)
```

Циклов нет (rms не импортирует format; обратных рёбер нет).

## Verification Checklist

После применения плана (`goga-apply` — отдельный шаг):

- [ ] `internal/rms/CODEMANIFEST`: `goga contract rms` зелёный
      (свойство `Comments -> []Range` реализовано экспортированным
      полем; существующие поверхности не тронуты)
- [ ] `internal/rms/.usages/rms-parsing.md`: секция Comment extents
      добавлена, `goga lint` зелёный
- [ ] `internal/format/CODEMANIFEST`: `goga lint` зелёный;
      `goga contract format` зелёный после реализации ячейки
- [ ] `internal/format/.usages/formatting.md`: создан, линтуется
- [ ] `goga schema` отражает format → {rms, common} без циклов
- [ ] Инварианты (реализация): golden-тесты `-update` +
      `parse(format(x)) ≡ parse(x)`, идемпотентность, полнота
      комментариев; прогон на `.corpus` (пропуск, если не скачан)
- [ ] `make check` зелёный

## Notes

- Решения brainstorm-сессии 2026-09-21 утверждены пользователем
  пофазно (первичный анализ, карта типов, построчная детализация,
  распределение, контракты обеих ячеек, сборка, финал).
- Внутренние типы принтера (якорение комментариев, буфер) —
  реализация, вне контракта; план содержит только артефакты
  CODEMANIFEST и `.usages/`.
