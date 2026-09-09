# Architecture Plan: watched-files

## Topic

**watched-files** — реакция на правки файлов на диске вне редактора:
`workspace/didChangeWatchedFiles` → форс-сброс дискового кэша
резолвера + републикация диагностик открытых документов (волна 2
пачки, M; компактный arch → apply → TDD).

## Контекст (что уже есть)

- Дисковый кэш резолвера перечитывает файл при изменении stat
  (size+mtime) — БЕЗ сигнала. Пробелы: (а) событие не триггерит
  пересчёт диагностик открытых includer'ов; (б) правка с сохранившимся
  stat-отпечатком не подхватывается вовсе.
- Клиент шлёт didChangeWatchedFiles только после регистрации
  watchers (dynamic registration).

## Дизайн-решения

- **include.Drop(paths)**: точечный форс-сброс записей дискового кэша
  (по путям из событий). После Drop следующий Closure гарантированно
  перечитывает файлы с диска независимо от stat-отпечатка. Обратный
  поиск «кто включает изменённый файл» НЕ строится: републикация всех
  открытых документов (события редки, N открытых доков мал).
- **DidChangeWatchedFiles**: события → FsPath → Drop → republishAll.
  Удалённые файлы (Deleted) покрываются тем же Drop: следующий
  Closure честно даст MissingInclude.
- **Регистрация**: в Initialized (дополняя pull настроек) при
  клиентской capability
  `workspace.didChangeWatchedFiles.dynamicRegistration` —
  `client.RegisterCapability`: id "aoe2lsp/watched-files", method
  "workspace/didChangeWatchedFiles", watchers `**/*.rms`, `**/*.xs`
  (kind Create|Change|Delete). Без capability — нет регистрации,
  хендлер всё равно работает (клиент может слать сам).
- **Initialize** дополнение: запоминать обе capability (configuration
  уже есть; + didChangeWatchedFiles.dynamicRegistration).

## Artifacts

### Cell: `internal/include` — modify

```yaml
    "Drop(paths: []string)": |
      Форс-сброс записей дискового кэша по путям.

      `paths`: файловые пути (как в Resolved.Target)

      Requirements:
      - потокобезопасность (общий мьютекс с кэшем)
      - последующие Closure перечитывают эти файлы с диска независимо
        от stat-отпечатка; неизвестный путь — no-op
```

### Cell: `internal/server` — modify

- Global Annotations: строка про реакцию на didChangeWatchedFiles.
- `Initialize`: запомнить capability dynamicRegistration.
- `Initialized`: дополнение — регистрация watchers.
- Новый метод:

```yaml
    "DidChangeWatchedFiles(ctx: Context, params: DidChangeWatchedFilesParams) -> err: error": |
      Algorithm:
      1. События params.Changes → пути (FsPath)
      2. Drop путей у резолвера
      3. Переиздать диагностику всех открытых документов
```

### `.usages/lifecycle.md`

Правка устаревшего предусловия Cross-file navigation («files changed
outside the editor are not tracked»): теперь отслеживаются при
поддержке клиентом dynamic registration; события форсируют перечитывание.

## Verification Checklist

- [x] include: Drop форсит перечитывание при неизменном stat-отпечатке
      (same size + os.Chtimes назад); неизвестный путь — no-op
- [x] server: правка включённого lib.xs на диске + событие →
      републикация main.rms с обновлённой диагностикой
      (undefined-symbol исчезает после появления декларации)
- [x] Initialized при capability регистрирует watchers (мок-клиент);
      без capability — регистрации нет
- [x] `goga lint` 0; `goga contract` include+server зелёные;
      `make check` зелёный
