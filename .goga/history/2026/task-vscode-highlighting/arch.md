# Architecture Plan — vscode-highlighting

## Topic

TextMate-грамматики для aoe2rms/aoe2xs (задача `task-vscode-highlighting`,
зеркало #51, эпик v1.0.0 — до публикации расширения #7).

План: `.goga/history/2026/task-vscode-highlighting/arch.md`
(писан по прямому пути: `goga history path -f arch.md` резолвится в топик
`master` — указатель текущего топика стоит там; переключать ветки ради
brainstorm-этапа не нужно).

## Implementation Order

| # | Артефакт | Тип | Обоснование порядка |
|---|---|---|---|
| 1 | `internal/highlight` | ячейка, NEW | лист: единственная зависимость — существующий `internal/kb` (импортируется, не модифицируется); ячейка не нужна никому выше |
| 2 | `cmd/tmgen` | CLI-обёртка | тонкий entrypoint над `highlight.GenTmLanguage`; после ячейки |
| 3 | `editors/vscode/syntaxes/aoe2rms.tmLanguage.json` | генерируемый артефакт | продукт `cmd/tmgen`; коммитится, ручные правки запрещены |
| 4 | `editors/vscode/syntaxes/aoe2xs.tmLanguage.json` | hand-written | не зависит от ячейки; пишется рядом с RMS-грамматикой |
| 5 | `editors/vscode/package.json` | modify | `contributes.grammars` — после появления обоих файлов |
| 6 | `.goga/usages/cooks/vscode-extension.md` | modify (usage-файл) | секция Static highlighting фиксирует итоговую структуру — в конце |

## Artifacts

### Cell: `internal/highlight` — NEW

#### CODEMANIFEST

Полное содержимое `internal/highlight/CODEMANIFEST`:

```yaml
Imports:
  - Types:
      - Store
      - Command
      - Constant
    Usages:
      - lookups
      - data-pipeline
    From: internal/kb

Usages:
  conventions: .goga/usages/conventions.md
  vscode-extension: .goga/usages/cooks/vscode-extension.md

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `vscode-extension` for the artifact home and tmLanguage structure
  (scope naming, contributes.grammars).
  Use `data-pipeline` from Imports for the regeneration order: kb data
  (kbgen) first, then grammar (tmgen) — artifact must never drift from
  committed kb data.

  Ячейка — оффлайн-генерация статической подсветки: читает загруженную
  базу знаний kb и эмитет RMS-грамматику; утилита сборки, в рантайме
  сервера не участвует. Модель грамматики (скелет, scope-константы,
  keyword-наборы) — внутренняя реализация, вне публичной поверхности.

---

"GenTmLanguage(store: Store, outPath: string, log: Logger) -> err: error":
  location: gen.go
  annotations: |
    Генерация TextMate-грамматики aoe2rms.tmLanguage.json: hand-written
    скелет + сгенерированные keyword-альтернации из базы знаний.

    `store`: загруженная база знаний (kb.NewStore); read-only
    `outPath`: путь к целевому файлу
    (editors/vscode/syntaxes/aoe2rms.tmLanguage.json)
    `log`: инжектированный логгер сборки (в Go — *slog.Logger;
    nil → slog.Default())
    `err`: ошибки записи; обёрнуты с путём файла

    Use `lookups` from Imports for keyword collection via Store.

    Algorithm:
    1. Собрать keyword-наборы: имена команд (`Command` из Commands("")),
       имена атрибутов из Attributes каждой команды, имена констант
       (`Constant` из Constants(""))
    2. Построить модель грамматики: скелет (секции, #-директивы,
       блок-комментарии, числа/проценты, строки) + keyword-альтернации
       трёх классов (команды/атрибуты/константы); имена экранированы,
       порядок стабилен
    3. Сериализовать в JSON детерминированно (стабильный порядок ключей)
    4. Записать файл в outPath

    Requirements:
    - повторный запуск на тех же данных → побайтово идентичный файл
    - RMS-имена матчатся case-insensitively; границы слов учитывают
      подчёркивания и цифры в именах
    - три keyword-класса с раздельными scope'ами; нейминг согласован
      с легендой semantic tokens сервера (по `vscode-extension`)
    - пустой keyword-набор → соответствующий паттерн опускается,
      не ошибка

    Constraints:
    - утилита сборки, не вызывается в рантайме сервера
    - Store не модифицируется
    - скелет грамматики определён в коде ячейки, не читается из
      внешних файлов

---

Author: Goga
CreatedAt: 10/09/26
Description: |
  Оффлайн-генерация TextMate-грамматики aoe2rms.tmLanguage.json из базы
  знаний kb (скелет + keyword-альтернации): статическая подсветка RMS.
```

#### .usages

**Файл:** `internal/highlight/.usages/grammar-pipeline.md`

Полное содержимое:

```md
# Grammar Pipeline — regenerating the RMS TextMate grammar

Domain: rebuilding editors/vscode/syntaxes/aoe2rms.tmLanguage.json from the
committed kb data. Target audience: maintainers updating the knowledge base
or the grammar skeleton.

## Regenerate

Single entry point — highlight.GenTmLanguage emits the grammar from a
loaded kb Store: the hand-written skeleton (sections, # directives, block
comments, numbers/percent, strings) plus keyword alternations generated
from Commands(""), command Attributes and Constants(""):

```go
store, err := kb.NewStore()
if err != nil {
    return fmt.Errorf("load knowledge base: %w", err)
}
err = highlight.GenTmLanguage(store,
    "editors/vscode/syntaxes/aoe2rms.tmLanguage.json", slog.Default())
```

cmd/tmgen wraps exactly this call (flags only, no logic of its own);
its default output is editors/vscode/syntaxes/aoe2rms.tmLanguage.json,
overridable with -out.

## Order: kbgen first, then tmgen

Keyword content comes from the committed kb data. When docs/ref changes,
regenerate in order: kb data first (cmd/kbgen), then the grammar
(cmd/tmgen). The grammar artifact is committed and must never drift from
the committed kb data.

## No-drift golden

Regeneration is deterministic — the same data yields a byte-identical
file. The cell's golden test regenerates into a temp dir and compares
against the committed artifact. Never edit the committed
aoe2rms.tmLanguage.json by hand: change the skeleton in internal/highlight
and regenerate.

## Hand-written companions

- aoe2xs.tmLanguage.json is hand-written (fixed XS language keywords,
  no generation). Constants in XS are colored by the server's semantic
  tokens, not by the grammar. JSON validity of both grammar files is
  guarded by tests.
- contributes.grammars in editors/vscode/package.json registers both
  grammars (scopeName source.aoe2rms / source.aoe2xs).
```

### Существующие артефакты — diffs

#### `editors/vscode/package.json` — modify

Добавить в `contributes` ключ `grammars` (языки и конфигурации уже
заявлены, сборка не меняется):

```jsonc
"grammars": [
  {
    "language": "aoe2rms",
    "scopeName": "source.aoe2rms",
    "path": "./syntaxes/aoe2rms.tmLanguage.json"
  },
  {
    "language": "aoe2xs",
    "scopeName": "source.aoe2xs",
    "path": "./syntaxes/aoe2xs.tmLanguage.json"
  }
]
```

#### `.goga/usages/cooks/vscode-extension.md` — modify

Добавить секцию `## Static highlighting` (после `## Language
configuration`): структура tmLanguage (scopeName source.aoe2rms /
source.aoe2xs, репозиторий паттернов: comments / sections / directives /
kb-keywords), регистрация через `contributes.grammars`, scope-нейминг
согласован с легендой semantic tokens (TextMate — базовый слой,
semantic уточняет идентификаторы), регенерация — `go run ./cmd/tmgen`,
порядок kbgen → tmgen, ручные правки сгенерированного файла запрещены.

## Dependency Map

```
                    ┌────────────────────────────────────┐
                    │          internal/kb (существует)  │
                    │  Store, Command, Constant          │
                    │  .usages: lookups, data-pipeline   │
                    └───────────────┬────────────────────┘
                                    │ Imports
                                    ▼
┌───────────────────────────────────────────────────────────┐
│                internal/highlight (NEW, leaf)              │
│  GenTmLanguage(store, outPath, log) -> err                 │
│  .usages: grammar-pipeline                                 │
└───────────────┬───────────────────────────────────────────┘
                │ вызывается только оффлайн (сборка)
                ▼
     cmd/tmgen → editors/vscode/syntaxes/aoe2rms.tmLanguage.json
                        (коммитится; ≡ golden)
     aoe2xs.tmLanguage.json (hand-written) ──┐
                                              ▼
                        editors/vscode/package.json
                              (contributes.grammars)
```

Циклов нет: `highlight → kb` однонаправленная; ячейка никем не
импортируется. Рантайм сервера не затронут.

## Verification Checklist

После реализации каждого артефакта:

- [ ] `internal/highlight` (код + тесты):
  - [ ] `goga contract internal/highlight` — сигнатура совпадает
  - [ ] golden: регенерация в temp dir побайтово ≡ закоммиченный
        `aoe2rms.tmLanguage.json`; JSON-валидность обоих grammar-файлов
        (включая hand-written XS)
  - [ ] тест «команда из kb ⇒ подсвечивается»: имя из Commands("")
        присутствует в keyword-альтернации
  - [ ] тесты — под memory cap:
        `timeout 300 systemd-run --user --scope -p MemoryMax=1500M
        -p MemorySwapMax=0 bash -c 'go test ./... -count=1'`
- [ ] `cmd/tmgen`: `go run ./cmd/tmgen` на чистом дереве → `git diff`
      пуст (идемпотентность); `-out` перенаправляет вывод
- [ ] `editors/vscode`:
  - [ ] `npm run check` зелёный; грамматики грузятся без ошибок
        (developer tools / `code --status`)
  - [ ] `npx vsce package --no-dependencies` — .vsix содержит
        `syntaxes/` обоих файлов
  - [ ] ручная приёмка: .rms/.xs подсвечиваются без сервера; с сервером
        semantic tokens уточняют идентификаторы, синтаксис — от
        грамматики; светлая и тёмная темы
- [ ] cook `vscode-extension.md`: секция Static highlighting отражает
      итог (структура, contributes, регенерация, no-drift)
- [ ] финал: `make check` зелёный; `goga lint` 0 errors
