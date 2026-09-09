# Architecture Plan: corpus-ci

## Topic

**corpus-ci** — корпус-прогон 100 реальных карт как CI-гейт (волна 2
пачки, S–M). Инфраструктурный слот: контракты ячеек НЕ меняются
(Runner/Report/cmd уже умеют всё нужное: exit 1 на HardFailures,
exit 2 на сбой харнесса; fetch-скрипт пиннится SHA).

## Дизайн-решения

- **Триггер — выборочный** (правка пользователя 2026-09-09, отменяет
  исходный критерий пачки «PR краснит CI»): PR пропускают корпус;
  гейт работает на push в master (пост-мердж верификация), ручном
  `workflow_dispatch` и может быть назначен nightly при надобности.
- **Кэш**: выборка fetch-скрипта детерминирована (stable sort +
  фикс. random-source → стабильные имена файлов), поэтому кэшируем
  весь `.corpus` с ключом `hashFiles(scripts/corpus-sources.txt)`;
  при попадании (≥ CORPUS_COUNT файлов) fetch пропускается — ноль
  сети. Промах/изменение пула — полная перекачка (~100 мелких файлов,
  секунды).
- **Джоба `corpus`** в ci.yml: build → restore-cache → (fetch) →
  `go run ./cmd/corpus -bin ./aoe2-lsp -dir .corpus`; таймаут джобы
  15 минут; воркер-раннер выделять нечего — сессия на файл уже
  коротка (2.1s на корпусные unit-тесты).
- **Makefile-цель `corpus`** для локального прогона с той же
  логикой кэш-пропуска (`.corpus` в проекте, в .gitignore — проверить).
- Пер-нощно (nightly) НЕ заводим: пул запиннен SHA, дрейфап-стрима нет.

## Artifacts (код вне ячеек)

1. `Makefile`: цель `corpus` (fetch с кэш-пропуском + раннинг).
2. `.github/workflows/ci.yml`: джоба `corpus` (checkout@v5,
   setup-go@v6 по go.mod, cache, build, fetch-или-пропуск, раннер).
3. `.gitignore`: `.corpus/` (если ещё нет).

## Verification Checklist

- [x] локально: CORPUS_COUNT=3 fetch работает (сеть), build +
      `go run ./cmd/corpus` проходит (exit 0), повтор — кэш-пропуск
- [x] повторный fetch-прогон с существующим набором НЕ качает сеть
- [x] ci.yml валиден по YAML; джоба появляется в CI-прогоне ветки
- [x] `make check` зелёный; контракты не тронуты (goga lint 0)
