---
name: tests-pipeline
description: Конвенции тестов, моки, фикстуры, TDD. Грузится при работе с тестами или testkit.
paths:
  - "**/*_test.go"
  - "internal/testkit/**"
  - ".mockery.yml"
---

# Тесты

## Структура

- `<file>_test.go` рядом с тестируемым `<file>.go`.
- Package: `<pkg>_test` (внешний — для black-box) или `<pkg>` (для white-box тестов приватных функций).
- Юнит-тесты используют моки портов (mockery v3).
- Интеграционные — build-tag `integration`, поднимают реальный Postgres / NATS.

## TDD для багфиксов

Обязательно. Сначала падающий тест воспроизводящий баг, потом фикс.

```
1. Пишешь t.Run("...", func(t *testing.T) { ... })  с assertion на правильное поведение
2. go test → red (тест падает)
3. Фикс в коде
4. go test → green
5. Коммит обоих файлов одним коммитом
```

## Моки (mockery v3)

Конфиг — `.mockery.yml`. Файлы: `<interface_snake>.go` в подпакете `mocks/` рядом с интерфейсом (пакет `mocks`, тип `Mock<Interface>`). Защита от утечки в прод-код — depguard rule `mocks-isolation`: пакеты `*/mocks` импортируются только из `*_test.go`.

### Добавить моки для нового интерфейса

1. Объяви интерфейс в `usecase.go` (например, `OrderRepository`).
2. В `.mockery.yml` под нужным пакетом добавь:
   ```yaml
   git.stit.tech/stit/go-backend-template/internal/domain/order:
     interfaces:
       OrderRepository:
   ```
3. `make mocks` — генерирует `internal/domain/order/mocks/order_repository.go`.
4. `make mocks-check` в CI проверит что моки не устарели.

### Использование моков в тестах

```go
import (
    "git.stit.tech/stit/go-backend-template/internal/domain/order/mocks"
)

func TestUsecase_CreateOrder(t *testing.T) {
    repo := mocks.NewMockOrderRepository(t)
    repo.EXPECT().Insert(mock.Anything, mock.Anything).Return(nil).Once()

    uc := NewUsecase(repo, ...)
    err := uc.CreateOrder(context.Background(), Order{...})
    require.NoError(t, err)
}
```

`mockery v3` с template `testify` даёт `EXPECT()` API. `.Once()` / `.Times(N)` для проверки количества вызовов. `t` в конструкторе — автоматически вызовет `AssertExpectations` при cleanup.

## Интеграционные тесты

Build-tag сверху файла:
```go
//go:build integration

package mypkg_test
```

Запуск: `make test-int`. Локально нужен Docker.

### `internal/testkit/pgseed`

Хелпер поднимающий чистый Postgres, накатывающий миграции, возвращающий `*pgxpool.Pool`. Используй в test-setup:

```go
//go:build integration

package mypkg_test

import "git.stit.tech/stit/go-backend-template/internal/testkit/pgseed"

func TestRepo_E2E(t *testing.T) {
    pool := pgseed.New(t)  // testcontainers + миграции
    repo := postgres.NewRepository(pool)
    // ...
}
```

`pgseed.New(t)` — каждый тест получает свежую БД. Через `t.Cleanup` контейнер останавливается.

### `internal/testkit/identitytest`

Хелперы для генерации тестовых principal/identity. Для тестов где нужен авторизованный subject.

## Coverage

- `make test-cover` — coverage по `internal/domain/...` и `internal/platform/...`.
- В CI — `coverage` job с floor 60%. Падает если ниже.
- Поднимать floor по мере зрелости теста-сюита.

## Anti-patterns

- ❌ Не мокай `*sql.DB` или `*pgxpool.Pool` напрямую. Мокай интерфейсы домена (Repository).
- ❌ Не пиши тесты на сгенерированный sqlc-код. Тестируй методы Repository.
- ❌ Не используй `time.Now()` в коде — инжекть `clockkit.Clock` для детерминизма.
- ❌ Не используй `context.Background()` в проде. В тестах — ок (`context.Background()` или `t.Context()` в Go 1.24+).
- ❌ Не оставляй закомментированные тесты. Удаляй или фикси.
