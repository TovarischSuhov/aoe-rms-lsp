# Architecture Plan: news (release-notes-monitor, #61)

## Topic

**news** — детектор патч-постов AoE2 DE (ячейка internal/news).
План: `.goga/history/2026/release-notes-monitor/arch.md` (этот файл).
Задача: `.goga/history/2026/release-notes-monitor/task.md` (авторитет),
зеркало — issue #61.

## Implementation Order

1. **`internal/news`** (создать) — лист, Imports пусты: не зависит ни от
   одной ячейки. Реализуется первым и единственным в клеточном слое.
2. Вне клеточного слоя (по плану задачи, не CODEMANIFEST):
   `cmd/newscheck` (тонкий entrypoint), `.github/workflows/kb-monitor.yml`,
   `docs/ref/.newscheck-state.json` (runtime-данные), cook
   `.goga/usages/cooks/github-actions.md` (секция scheduled-monitor).

## Artifacts

### Cell: `internal/news` — СОЗДАЁТСЯ (не модификация)

#### `internal/news/CODEMANIFEST`

```yaml
Usages:
  conventions: .goga/usages/conventions.md
  sources: |
    Договорённости ячейки с внешними источниками:
    - RSS-фид ageofempires.com/news/feed/ (WordPress, RSS 2.0): 5 айтемов
      на страницу, пагинация ?paged=N (1-based), айтемы обратно-
      хронологически; pubDate — RFC 822, приводится к RFC 3339 UTC.
    - Патч-пост AoE2 DE: заголовок упоминает Age of Empires II И содержит
      Update / Minor Update / Hotfix / Update Preview (регистр и порядок
      варьируются); посты других игр и не-патч-посты — мимо.
    - Семяние холодного старта: новейшая секция
      docs/ref/aoe2de-xs-rms-changelog.md — строка "## Update <id> —
      YYYY-MM-DD", следующей строкой URL поста.

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `sources` для формы фида, фильтра заголовков и формата семяния.

  Ячейка — детектор патч-постов: утилита мониторинга, в рантайме сервера
  не участвует. Зависимостей от других ячеек нет. Все отказы громкие:
  не получить/не распарсить источник — ошибка, тихий пропуск запрещён.

---

"Post()":
  location: post.go
  annotations: |
    Пост фида новостей в терминах детектора.
  properties:
    "Title -> string": |
      Заголовок поста; источник для фильтра (see `sources`).
    "URL -> string": |
      Постоянный адрес поста; человекочитаемый ключ состояния.
    "Published -> string": |
      Дата публикации, RFC 3339 UTC; ключ сравнения якоря.

"Result()":
  location: post.go
  annotations: |
    Итог прогона детектора.
  properties:
    "NewPosts -> []Post": |
      Новые AoE2-патч-посты (строже новее якоря), порядок фида —
      свежие сверху. Пусто — issue не создаётся.
    "LastSeen -> Post": |
      Новейший просмотренный пост — якорь следующего прогона;
      передаётся в `SaveState` после успешного создания issue.

"SeedFromChangelog(changelogPath: string) -> seed: Post, err: error":
  location: check.go
  annotations: |
    Якорь холодного старта: последний проверенный релиз с изменениями.

    `changelogPath`: путь к docs/ref/aoe2de-xs-rms-changelog.md
    `seed`: пост-якорь из новейшей секции (заголовок, URL, дата)
    `err`: файл нечитаем или верхняя секция не соответствует `sources`

    Algorithm:
    1. Прочитать верхнюю секцию файла и URL следующей строкой (`sources`)
    2. Вернуть Post с этим URL, заголовком и датой секции

    Requirements:
    - используется только верхняя (самая свежая проверенная) секция

"Check(ctx: Context, feedURL: string, statePath: string, changelogPath: string, log: Logger) -> result: Result, err: error":
  location: check.go
  annotations: |
    Один прогон детектора: от текущего якоря к новым постам.

    `ctx`: контекст отмены (прерывает сетевые запросы)
    `feedURL`: адрес RSS-фида (`sources`)
    `statePath`: state-файл; отсутствие — холодный старт, не ошибка
    `changelogPath`: путь к changelog-доку для семяния
    `log`: инжектированный логгер (*slog.Logger; nil → slog.Default())
    `result`: новые патч-посты и якорь следующего прогона
    `err`: отказ источника — HTTP≠2xx, XML не парсится, state нечитаем,
    потолок страниц исчерпан и все посты новее якоря (сообщение содержит
    лекарство)

    Algorithm:
    1. Прочитать state (URL + дата последнего просмотренного поста);
    файла нет — получить якорь через `SeedFromChangelog`
    2. Запрашивать страницы фида (?paged=1,2,…, `sources`), пока на
    странице нет поста с датой не новее якорной
    3. Отфильтровать просмотренные посты по заголовку (`sources`)
    4. Собрать Result: NewPosts — патч-посты строго новее якорной даты;
    LastSeen — новейший просмотренный пост
    5. State не записывать — якорь фиксируется обвязкой после успешного
    issue (`SaveState`)

    Requirements:
    - потолок страниц — константа ячейки; исчерпание без поста старее
    якоря — ошибка с лекарством в сообщении
    - сетевые запросы уважают ctx и HTTP-timeout

    Constraints:
    - файлы не модифицирует
    - неузнаваемый фид — ошибка, не пустой Result

"SaveState(statePath: string, post: Post) -> err: error":
  location: state.go
  annotations: |
    Атомарная запись state-файла.

    `statePath`: state-файл
    `post`: сохраняемый якорь
    `err`: ошибка записи

    Algorithm:
    1. Сериализовать `post` в человекочитаемый JSON (URL + дата)
    2. Записать во временный файл и заменить им statePath

    Requirements:
    - замена атомарна: читатели не видят частичный файл
    - формат стабилен между версиями (тот же набор полей, что у `Post`)

---

Author: Goga
CreatedAt: 16/09/26
Description: |
  Детектор новых патч-постов AoE2 DE в RSS-фиде ageofempires.com/news:
  пагинация до якоря, фильтр по заголовку, state-файл и семяние холодного
  старта из changelog-дока. Утилита мониторинга (cmd/newscheck +
  kb-monitor workflow), в рантайме сервера не участвует.
```

#### `internal/news/.usages/detecting.md`

```markdown
# Detecting: прогон детектора патч-постов

Домен: запуск цикла «проверка → issue → фиксация якоря». Аудитория —
обвязка `cmd/newscheck` и monitoring-workflow; сервер ячейку не зовёт.

## Обычный прогон

    res, err := news.Check(ctx, feedURL, statePath, changelogPath, slog.Default())
    if err != nil {
        // Громкий отказ: источник сломан. Обвязка завершается с exit ≠ 0,
        // состояние не трогаем — следующий прогон повторит попытку.
        return err
    }
    if len(res.NewPosts) > 0 {
        // 1) создать ОДИН issue на батч (label kb-refresh) по res.NewPosts
        // 2) только после успеха — зафиксировать якорь
    }
    if err := news.SaveState(statePath, res.LastSeen); err != nil {
        return err
    }

Порядок обязателен: якорь пишется после успешного issue, иначе посты
теряются при сбое создания issue.

## Холодный старт

State-файла нет — Check сеет якорь из changelog-дока сам (ошибки нет).
Типичный первый прогон: NewPosts пуст (changelog актуален) или содержит
пропущенные патчи (changelog отстал — issue создастся, это желаемое
самовосстановление). Якорь фиксируется как обычно.

## Симуляция для приёмки

Установите в state-файле дату постарше → Check вернёт накопившиеся
патч-посты как новые; ровно один issue, якорь обновится.

## Побочные эффекты и предусловия

- Check: только сеть и чтение файлов; состояние не меняет
- SaveState: атомарная запись statePath (temp+rename)
- Потолок страниц исчерпан и все посты новее якоря — err с лекарством
  в сообщении; обвязка обязана падать, не «переинициализироваться»
```

## Dependency Map

```
internal/news ──(без Imports, лист)──> cmd/newscheck ──> kb-monitor.yml
      │
      ├── читает: docs/ref/aoe2de-xs-rms-changelog.md (семяние), RSS-фид (сеть)
      └── пишет (SaveState): docs/ref/.newscheck-state.json
```

Межклеточных Imports нет; связь с kb — только процессов (issue →
kb-refresh), не кода.

## Verification Checklist

- [ ] `internal/news/CODEMANIFEST` синтаксически корректен: `goga lint`
- [ ] после реализации: `goga contract news` зелёный (имена экспортов
      совпадают с контрактом: Post/Result/SeedFromChangelog/Check/SaveState)
- [ ] фикстуры: страницы фида (пагинация до якоря на стр. 2), AoE3 и
      не-патч-посты отфильтрованы, битый XML → err, холодный старт
      (нет state → семя из changelog-фикстуры), потолок страниц → err
- [ ] `make check` зелёный (тесты — под memory cap)
- [ ] приёмка задачи #61: dispatch на актуальном state — без issue и
      git-диффа state; симуляция отката — ровно один issue + state
- [ ] cook `github-actions.md` дополнен секцией scheduled-monitor
