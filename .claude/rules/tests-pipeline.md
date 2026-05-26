---
name: tests-pipeline
description: Инварианты тестов, моков, фикстур, TDD. Грузится при работе с тестами или testkit. Процедура добавления моков/доменных тестов — skill scaffold; e2e-харнес — test/README.md.
paths:
  - "**/*_test.go"
  - "internal/testkit/**"
  - ".mockery.yml"
---

# Тесты

Процедура (добавить мок для интерфейса, тесты при скаффолде блока) — skill `scaffold`.
E2E/smoke-харнес (pytest, side-effect-ассерты) — `test/README.md`. Здесь — инварианты.

## Структура

- `<file>_test.go` рядом с `<file>.go`. Package `<pkg>_test` (black-box) или `<pkg>`
  (white-box приватных функций).
- Юнит-тесты — через моки портов (mockery v3). Интеграционные — build-tag `integration`,
  поднимают реальный Postgres/NATS (`make test-int`, нужен Docker).

## TDD для багфиксов — обязательно

RED→GREEN: падающий тест, воспроизводящий баг → фикс → зелёный → оба файла одним коммитом.
Тест должен падать **по той причине**, что и баг. Ведёт это skill `triage-failure`.

## Моки (mockery v3)

- Конфиг — `.mockery.yml`. Файлы: `mocks/<interface_snake>.go` рядом с интерфейсом
  (пакет `mocks`, тип `Mock<Interface>`). Регенерация — `make gen`; `make gen-check` ловит дрейф.
- **Изоляция (depguard `mocks-isolation`)**: `*/mocks` импортируется только из `*_test.go`.
- Template `testify` даёт `EXPECT()`-API: `repo.EXPECT().Insert(...).Return(nil).Once()`.
  `t` в конструкторе (`mocks.NewMockX(t)`) — авто-`AssertExpectations` на cleanup.

## Интеграционные

- Build-tag `//go:build integration` сверху файла.
- `internal/testkit/pgseed.New(t)` — свежий Postgres + миграции + `*pgxpool.Pool` на каждый
  тест (`t.Cleanup` гасит контейнер). `internal/testkit/identitytest` — тестовые principal/identity.

## Coverage

`make test-cover` по `internal/{domain,platform}/...`. CI-floor 60%, падает ниже. Поднимать по мере зрелости.

## Anti-patterns

- ❌ Мокать `*sql.DB`/`*pgxpool.Pool` напрямую — мокай интерфейсы домена (Repository).
- ❌ Тесты на сгенерированный sqlc-код — тестируй методы Repository.
- ❌ `time.Now()` в коде — инжекть `clockkit.Clock` (детерминизм). `context.Background()` в проде — нет.
- ❌ Закомментированные тесты — удаляй или фикси.
