# Архитектурный план: operational-readiness

Topic: **operational-readiness** — debug-логи, corpus-прогон 100 реальных
карт, бенчмарки горячих путей.
План: `.goga/history/2026/task-debug-bench-corpus/arch.md`
Задача: `.goga/history/2026/task-debug-bench-corpus/task.md`

## Implementation Order

1. **`internal/corpus`** (create) — лист без Imports из ячеек; связь с
   сервером только subprocess-LSP. Создаётся первым: не зависит ни от
   кого и не создаёт зависимостей.
2. **`cmd/corpus`** (create, вне ячеек) — тонкий entrypoint над ячейкой;
   после ячейки.
3. Внеячеечные треки (контрактами не описываются, порядок свободный):
   - PR A: `-debug` в `cmd/aoe2-lsp` + debug-события в реализации
     `internal/server` (контракт не меняется);
   - PR C: `Benchmark*`-файлы внутри существующих ячеек;
   - PR B: `scripts/corpus-fetch.sh` + прогон + фиксы.

Рекомендуемый порядок PR: **A → B → C** (corpus даёт реалистичные входы
и приоритизацию для бенчмарков; debug-логи помогают разборкам корпуса).

## Artifacts

### Cell: `internal/corpus` — CREATED

#### CODEMANIFEST (`internal/corpus/CODEMANIFEST`)

```yaml
Usages:
  conventions: .goga/usages/conventions.md
  lsp-protocol: .goga/usages/cooks/lsp-protocol.md

Annotations: |
  Use `conventions` for code writing rules and testing.
  Use `lsp-protocol` for client-side go.lsp.dev patterns (union/Optional
  types, positions, Full sync — со стороны клиента).

  Чёрный ящик относительно ячеек: связь с aoe2-lsp только через
  LSP-протокол поверх subprocess; Imports из internal/ запрещены.
  Каждый файл корпуса — свежий subprocess: падение одной карты не влияет
  на остальные.

---

"Runner(bin: string)":
  location: runner.go
  annotations: |
    Оркестратор corpus-прогона: каталог скриптов через полный LSP-цикл
    бинарника aoe2-lsp, агрегированный отчёт.

    `bin`: путь к бинарнику aoe2-lsp (спавнится на каждый файл)

    Requirements:
    - детерминированный порядок файлов (сортировка путей)
    - фиксированный бюджет времени на файл (таймаут сессии)
  methods:
    "Run(ctx: Context, dir: string) -> report: Report, err: error": |
      Прогон всех .rms/.xs файлов каталога.

      `ctx`: контекст прогона (отмена прерывает весь прогон)
      `dir`: корень каталога корпуса
      `report`: результаты по каждому файлу; `err`: только
      инфраструктурные сбои харнесса (bin не запускается, dir нечитаем)

      Algorithm:
      1. Рекурсивно обойти `dir`, собрать .rms/.xs, отсортировать пути
      2. На каждый файл — LSP-сессия: спавн процесса сервера (stderr —
         в ограниченный буфер), initialize (utf-8 positionEncoding,
         rootUri=`dir`), didOpen, дождаться publishDiagnostics,
         hover/completion/signatureHelp/documentSymbol в
         детерминированно сэмплированных позициях текста, shutdown+exit
      3. Жёсткая классификация файла: stderr содержит panic → panic;
         ненулевой exit до конца сессии → exit; истёк бюджет → timeout;
         обрыв JSON-RPC → transport; иначе ok
      4. Свести результаты в `Report` (HardFailures — число не-ok)

      Requirements:
      - позиции сэмплируются детерминированно из текста (без парсинга)
      - диагностика не интерпретируется — только счётчики

      Constraints:
      - файлы корпуса не модифицируются
      - содержимое документов не попадает в логи харнесса

"Report()":
  location: report.go
  annotations: |
    Агрегат прогона по каталогу.

    Requirements:
    - Files в порядке прогона; HardFailures — число FileResult
      со Status != ok
  properties:
    "Files -> []FileResult": |
      Результаты по файлам в порядке прогона.
    "HardFailures -> int": |
      Число файлов с жёстким отказом (критерий exit code обвязки).
    "DurationMS -> int64": |
      Длительность всего прогона.
  methods:
    "Summary() -> text: string": |
      Человекочитаемая сводка: строка на каждый не-ok файл + итоговые
      счётчики (stdout обвязки cmd).

"FileResult()":
  location: report.go
  annotations: |
    Результат прогона одного файла корпуса.
  properties:
    "Path -> string": |
      Путь файла относительно корня прогона.
    "Status -> string": |
      Жёсткая классификация: "ok" | "panic" | "exit" | "timeout" |
      "transport".
    "Detail -> string": |
      Краткая причина отказа (фрагмент panic-трейса, код выхода,
      ошибка транспорта); пусто для ok.
    "Diagnostics -> int": |
      Число опубликованных диагностик (счётчик без семантики).
    "Requests -> int": |
      Число LSP-запросов харнесса в сессии файла.
    "RequestErrors -> int": |
      Число ошибок JSON-RPC среди запросов.
    "DurationMS -> int64": |
      Длительность сессии файла.

---

Author: Goga
CreatedAt: 09/09/26
Description: |
  Корпус-харнесс: прогон реальных RMS/XS скриптов через aoe2-lsp
  subprocess'ом (полный LSP-цикл), детект паник/падений/таймаутов,
  агрегированный отчёт. Связь с сервером — только LSP-протокол.
```

#### .usages (`internal/corpus/.usages/running.md`)

```markdown
# Прогон корпуса и чтение отчёта

Область: запуск corpus-харнесса над директорией скриптов и интерпретация
результата. Аудитория: авторы CLI-обвязки (cmd/corpus) и разработчики,
разбирающие инциденты на реальных картах.

## Минимальный прогон

Runner получает путь к бинарнику aoe2-lsp и прогоняет директорию: каждый
файл .rms/.xs открывается в свежем subprocess'е сервера через полный
LSP-цикл (initialize → didOpen → диагностика → hover/completion/
signatureHelp/documentSymbol → shutdown).

    runner := corpus.NewRunner(binPath)
    report, err := runner.Run(ctx, corpusDir)
    if err != nil {
        // инфраструктурный сбой харнесса: бинарник не запускается,
        // директория нечитаема — это НЕ результаты карт
        return err
    }
    fmt.Println(report.Summary())
    if report.HardFailures > 0 {
        os.Exit(1)
    }

## Чтение FileResult

Status — жёсткая классификация сессии одного файла:

| Status      | Значение                                        |
|-------------|-------------------------------------------------|
| ok          | сессия завершилась штатно                       |
| panic       | сервер запаниковал (stderr содержит panic)      |
| exit        | ненулевой код возврата до конца сессии          |
| timeout     | истёк бюджет времени на файл                    |
| transport   | обрыв JSON-RPC соединения                       |

Detail — краткая причина (фрагмент panic-трейса, код выхода, ошибка
транспорта). Diagnostics — число опубликованных диагностик (счётчик без
семантики: аномально большие значения — повод смотреть отчёт глазами,
не критерий отказа). Requests/RequestErrors — число LSP-запросов
харнесса и ошибок JSON-RPC среди них.

## Предусловия и побочные эффекты

- Бинарник должен быть собран под текущую платформу; директория —
  читаемой. Include-цепочки резолвятся относительно прогоняемой
  директории (rootUri сессии).
- Харнесс только читает файлы корпуса; содержимое документов не
  попадает в логи.
- Прогон детерминирован: порядок файлов и сэмплируемые позиции не
  зависят от времени запуска — повторный прогон даёт тот же отчёт
  при неизменных входах и бинарнике.
```

### Cell: `internal/server` — MODIFIED (реализация, контракт без изменений)

Diff CODEMANIFEST: **нет**. Debug-события (`slog.DebugContext` по
категориям DEBUG из `conventions`: lifecycle, ветвления, объёмы) —
детали реализации хендлеров; сигнатуры и аннотации стабильны.

## Dependency Map

```
common   kb   corpus*            (* новая; без Imports из ячеек)
  │       │    │ subprocess LSP (не Imports)
  ▼       ▼    ▼
  rms    xs  [aoe2-lsp bin]
   └──┬───┘
      ▼
 analysis  include  hints  complete
      └────────┬─────────┘
               ▼
            server ← правки реализации (debug-события), контракт стабилен
```

Циклов нет: `corpus` не импортирует ячейки.

## Verification Checklist

После реализации:

- [ ] `goga lint` и `goga contract internal/corpus` зелёные (новый
      манифест соответствует коду: Runner/Report/FileResult
      экспортированы дословно, location runner.go/report.go);
- [ ] `internal/corpus/.usages/running.md` существует, ключ `running`
      не конфликтует;
- [ ] `make check` зелёный (fmt → build → test под memory cap → lint →
      goga lint);
- [ ] PR A: тест — с `-debug` идут debug-события (stderr), без флага
      уровень Info, stdout чист;
- [ ] PR B: прогон 100 карт под cgroup-песочницей; каждый не-ok статус
      либо закрыт фиксом с тестом, либо записан в план; список
      источников (URL + SHA/seed) закоммичен;
- [ ] PR C: `go test -bench` гоняется в ячейках rms/xs/analysis/hints/
      complete/include/kb; отчёт в `docs/reviews/` (benchstat-совместимые
      числа, слабые места, выводы);
- [ ] CODEMANIFEST существующих ячеек не изменены (git diff пуст для
      всех манифестов, кроме нового corpus).
