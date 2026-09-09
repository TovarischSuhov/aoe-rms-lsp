# Architecture Plan: did-you-mean

## Topic

**did-you-mean** — подсказка ближайшего имени в unknown-диагностиках
(волна 1 пачки editor-experience). Размер S–M: лёгкий путь (компактный
arch → apply → TDD), как для folding.

## Implementation Order

Только `internal/analysis` (modify) — алгоритм подсказки + формат
сообщений. **kb и server не меняются**: кандидатные наборы собираются
из существующих lookup-ов.

## Дизайн-решения

- **Формат**: суффикс в сообщение — `unknown command "creat_object";
  did you mean "create_object"?` (английский, как все сообщения).
- **Алгоритм**: Левенштейн без учёта регистра; порог
  `dist <= max(1, len(typo)/4)`; при равенстве дистанций —
  лексикографически меньшее имя. Детерминированность обязательна.
- **Кандидаты** (всё из существующих API, найдено при разведке):
  - `unknown-command` → имена из `Commands("")` (контракт: "" — все)
  - `unknown-attribute` (у известной команды) → `Command(name)`.
    Attributes (модель kb несёт список атрибутов команды)
  - `undefined-symbol` → имена из `Functions()` + `Constants("")` +
    объявленные имена файла (declared — analysis их уже собирает;
    опечатка в локали — самый частый случай)
  - `unknown-section` — БЕЗ подсказки (вне скоупа слота)
- **Владение**: kb — чистые данные (без изменений); алгоритм близости
  и форматирование — analysis (он владеет диагностиками); суффикс
  доезжает до редактора в Diagnostic.Message без участия server.

## Artifacts

### Cell: `internal/kb`

Без изменений (см. Дизайн-решения).

### Cell: `internal/analysis` — modify

`AnalyzeRms` шаг 2/3 и `AnalyzeXs` шаг 3 дополняются (аннотации):

```yaml
# AnalyzeRms, шаг 2, после "code=unknown-command/unknown-section":
#         к unknown-command дописывается подсказка ближайшего имени
#         из Commands("") (порог max(1, len/4), регистр не важен):
#         '; did you mean "X"?'; unknown-section — без подсказки
# AnalyzeRms, шаг 3: то же для unknown-attribute — кандидаты из
#         Attributes известной команды
# AnalyzeXs, шаг 3: undefined-symbol получает ту же подсказку;
#         кандидаты — имена Functions(), Constants("") и объявленные
#         имена файла
```

Requirements (в оба метода): `- подсказка детерминирована: порог
max(1, len/4), при равных дистанциях — меньшее имя`.

### `.usages/` (`internal/analysis/.usages/checks.md`)

Таблица кодов: колонка/примеры сообщений unknown-command /
unknown-attribute / undefined-symbol с суффиксом подсказки и правило
его отсутствия (нет кандидата в пороге — суффикса нет).

## Dependency Map

analysis уже импортирует Store (Types). Новых Imports нет.

## Verification Checklist

- [x] `goga lint` 0 ошибок; `goga contract internal/kb internal/analysis`
      зелёные после реализации
- [x] kb: CommandNames/ConstantNames — отсортированы, без повторов,
      длины согласуются с данными (204 функции → имена констант/команд
      из фикстур)
- [x] analysis: `creat_object` → did you mean "create_object";
      garbage-имя → суффикса нет; опечатка в локальной XS-переменной →
      подсказка из declared; unknown-attribute → подсказка из атрибутов
      команды; unknown-section без подсказки; сообщения без подсказки
      не изменились
- [x] server: существующие диагностики едут с суффиксом без изменений
      кодов/range (передаётся в Message)
- [x] `make check` зелёный
