# Architecture Plan: config

## Topic

**config** — настройки сервера: severity-оверрайды и include-корни
(волна 2 пачки editor-experience, M; полный скоут утверждён
пользователем). Компактный arch → apply → TDD.

## Implementation Order

1. `internal/include` (modify) — `SetRoots` + порядок резолва
2. `internal/server` (modify) — настройки (pull/push), применение

## Дизайн-решения

- **Секция** `"aoe2lsp"`; форма:
  `{"diagnostics":{"severityOverrides":{"<code>":"error|warning|info|hint|none"}},"includeRoots":["/abs"]}`
- **Каналы**: pull — `Initialized` при клиентской capability
  `workspace.configuration` (клиент из ctx; секция "aoe2lsp", первый
  item); push — `DidChangeConfiguration` (params.Settings). Оба →
  одна функция применения.
- **severity**: применяется при конвертации диагностик
  (toProtocolDiags) по коду; `none` — подавить. Неизвестные коды и
  имена severity — WARN-лог, игнор.
- **includeRoots**: абсолютные пути → `Resolver.SetRoots` (полная
  замена). Резолв: директория includer'а → корни по порядку; гард
  «в пределах директории корня замыкания ИЛИ одного из корней».
  Кэш дисковых файлов (stat-keyed) остаётся валиден; замыкания не
  кэшируются — пере-резолв бесплатен.
- **После применения** настроек — переиздать диагностику всех
  открытых документов (severity мог измениться).

## Artifacts

### Cell: `internal/include` — modify

`Resolver` (resolver.go): новый метод + правка аннотации Closure:

```yaml
    "SetRoots(roots: []string)": |
      Дополнительные корни поиска include-целей (полная замена набора).

      `roots`: абсолютные каталоги в порядке приоритета после
      директории includer'а

      Requirements:
      - потокобезопасность (общий мьютекс с кэшем); действует на
        последующие замыкания, кэш дисковых файлов валиден
```

Closure-аннотация, шаг резолва директив: цель — директория
includer'а, затем корни по порядку; цель вне директории корня
замыкания и вне корней — MissingInclude.

### Cell: `internal/server` — modify

Global Annotations, абзац о настройках (форма, каналы, severity при
конвертации, includeRoots → SetRoots, републикация). Методы:

```yaml
    "Initialized(ctx: Context, params: InitializedParams) -> err: error": |
      Начальный pull настроек: при запомненной из Initialize
      клиентской capability workspace.configuration запросить у
      клиента (из ctx) секцию "aoe2lsp" и применить (общая функция с
      DidChangeConfiguration); без capability — no-op.
    "DidChangeConfiguration(ctx: Context, params: DidChangeConfigurationParams) -> err: error": |
      Algorithm:
      1. Разобрать params.Settings: diagnostics.severityOverrides
         (код → error/warning/info/hint/none), includeRoots
      2. Неизвестные коды/имена severity — WARN, игнорировать
      3. Сохранить настройки; includeRoots → SetRoots резолвера
      4. Переиздать диагностику всех открытых документов
```

`Initialize` — дополнение: запомнить клиентскую capability
workspace.configuration.

### `.usages/lifecycle.md`

Секция Settings: JSON-форма, дефолты (пусто), каналы
(pull/push), Neovim (settings → didChangeConfiguration) и VS Code
(pull по секции) примечания.

## Verification Checklist

- [ ] include: SetRoots — fallback в корень, порядок приоритета,
      гард (цель вне корней — Missing), замена набора, конкурентный
      доступ не гоняется
- [ ] server: severity override (hint/none/повышение) в
      диагностики; неизвестный код/severity — WARN-игнор; includeRoots
      доходят до резолвера (замыкание находит файл в корне)
- [ ] DidChangeConfiguration переиздаёт диагностику открытых доков
      (stdio: didOpen с unknown-command → override none → батч пуст)
- [ ] Initialized: с capability делает pull (мок-клиент), без — no-op
- [ ] `goga lint` 0; `goga contract` include+server зелёные;
      `make check` зелёный
