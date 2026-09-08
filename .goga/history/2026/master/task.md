# Fix review defects 2: findings №6–№11 (encoding, phantom block, resolver)

## Current State

Продолжение `docs/reviews/2026-09-08-full-review.md`: блокирующие №1–№5
исправлены (PR #7–#12). Остались №6–№11:

- №6 (WARNING): клиент без utf-8 → сервер на utf-16, но колонки
  проксируются байтами — сдвиг колонок правее не-ASCII (кириллические
  комментарии) во всех диапазонах. Контракт server (Definition
  Requirements: «конвертация позиций — с учётом согласованного
  positionEncoding») уже требует правильного поведения — разрыв
  реализации.
- №7 (INFO): `#includeXS file.xs` с файловым аргументом создаёт пустой
  фантомный XsBlock нулевой ширины в outline.
- №8 (INFO): кэш диска resolver'а хранит uri первого запросившего:
  два написания одного файла (symlink/регистр) → Definition возвращает
  «чужое» написание URI.
- №9 (INFO): include-пути не ограничены: `#include ../../etc/passwd`
  читается; директория как цель проходит Stat, но молча теряется
  (Resolved без файла, не Missing).
- №10 (INFO): References вычисляет Closure каждого includer'а дважды
  (closureContains + повторный сбор) — производительность.
- №11 (INFO): blankComments гасит `//`/`/*` внутри строковых литералов
  → искажение Path/Range у `#include "a//b.rms"`.

## Description

Исправить №6–№11 с тестами. Контрактные дельты — минимальные
уточнения аннотаций (паттерн fix-review-defects №1–№5):

- **№6** `server`: byte↔UTF-16 конвертация колонок в
  toProtocolPos/fromProtocolPos при positionEncoding=utf-16; utf-8 —
  без конвертации. Внутренние хелперы сервера; CODEMANIFEST не меняется
  (требование уже в контракте).
- **№7** `rms`: пустой inline-регион после #includeXS **с аргументом**
  не создаёт XsBlock; bare #includeXS создаёт блок всегда (включая
  пустой в EOF — регрессия Task 1 сохраняется). Контракт: Parse шаг 5.
- **№8** `include`: Target.URI — написание цели из резолва данного
  запроса; канонический путь — только ключ кэша/visit-set. Контракт:
  Requirements в Definition.
- **№9** `include`: резолв ограничен директорией корневого документа
  замыкания (побег `../` наружу → MissingInclude); цель-директория →
  MissingInclude. Контракт: Closure шаг 3 + Constraints.
- **№10** `include`: References — Closure includer'а вычисляется один
  раз (closureContains и сбор из одного замыкания). Контракта не
  касается.
- **№11** `rms`: blankComments учитывает кавычки — содержимое строковых
  литералов не гасится. Контракт: Parse шаг 1 (строки — токены).

## Scope

**In scope:** №6–№11 с регрессионными тестами; контрактные дельты
аннотаций rms/include; материализация контрактов через конвейер.

**Out of scope:** новые фичи; изменения сигнатур; позиции вне
utf-8/utf-16 (utf-32-клиенты отсутствуют в практике); глобальный
workspace-root (его нет в Resolver — граница по корню замыкания).

## Acceptance Criteria

- Репро №6–№11 из ревью дают корректный результат; каждое —
  регрессионный тест
- №6: колонки в utf-16-режиме совпадают с ожиданием редактора на
  строках с кириллицей (диагностики/hover/completion/definition)
- №7: outline без фантомных узлов; bare-блок в EOF остаётся
- №8: два написания одного файла → Target.URI каждого запроса — своё
- №9: `../`-побег и директория-цель → MissingInclude (не чтение)
- №11: `#include "a//b.rms"` → Path="a//b.rms", Range без растяжения
- Существующие тесты не ломаются; memory-cap `go test ./...`,
  goimports, golangci-lint, `goga lint`, `goga contract` — зелёные

## Stack

Go 1.23+, testify, go.lsp.dev/protocol (existing). Новых зависимостей 0.

## External Dependencies

Нет. `lsp-protocol` (cooks) уже описывает positionEncoding-механику.

## Risks and Constraints

- №6: конвертация обязана быть симметричной (to/from) и дешёвой
  (каждый хендлер); не-ASCII тесты обязательны
- №9: граница по директории корня может отсечь легитимные
  `../`-включения общих библиотек карт — принято (локальный
  инструмент, защита по умолчанию); документируется в контракте
- №8: кэш по каноническому пути сохраняется (производительность),
  меняется только URI в ответе
- Тесты — только под memory cap

## Scope Estimate

3 подзадачи по ячейкам: (1) rms №7+№11; (2) include №8+№9+№10;
(3) server №6. Порядок: rms → include → server (от листьев).

## Existing Architecture

Ячейки rms (Parse: blankComments, endXsBlock/directive), include
(Resolver: Closure/Definition/References, loadDisk-кэш), server
(позиционная конвертация). Зависимости не меняются.

## Notes

- Решения приняты автономно (полномочие пользователя, 2026-09-08):
  №6 — внутренние хелперы server, без контракта; №9 — граница =
  директория корневого документа замыкания; №7 — skip только для
  директивы с аргументом.
- Источник: разделы №6–№11 `docs/reviews/2026-09-08-full-review.md`.
