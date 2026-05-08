---
name: domain-architecture
description: Архитектура и слои бизнес-доменов. Грузится при работе с internal/domain/.
paths:
  - "internal/domain/**"
---

# Архитектура домена

Каждый домен в `internal/domain/<name>/` — изолированный bounded context.

## Слои

```
<domain>/
├── entity.go        Доменные типы: User, Session, и т.п. Без зависимостей от инфраструктуры.
├── dto.go           Input/Output типы публичного API (для usecase, не HTTP-DTO).
├── errors.go        Sentinel-ошибки: var ErrNotFound = errors.New(...)
├── events.go        Outbox-эвенты домена. Типы и payload-структуры.
├── policy.go        Authorization policy. Decisions без сторонних эффектов.
├── policy_test.go   Тесты политики.
├── usecase.go       Бизнес-логика. Структура Usecase с зависимостями через интерфейсы (порты).
├── usecase_test.go  Тесты usecase с моками портов.
├── delivery/http/   HTTP-слой: handler.go (huma/v2 + chi), dto.go, view.go, handler_test.go
├── infra/postgres/  Persistence:
│   ├── repository.go            Реализация *Repository, использует sqlc-сгенерированные методы
│   └── queries/<name>.sql        Запросы. Имена методов идут в sqlc.
└── module/
    ├── module.go    fx.Module — Provide всех конструкторов
    └── adapters.go  Кросс-доменные адаптеры (см. ниже)
```

## Порты и адаптеры (cross-domain)

depguard в `.golangci.yml` блокирует прямые импорты между бизнес-доменами:
- `auth` ↛ `user`, `auth` ↛ `verification`
- `user` ↛ `auth`, `user` ↛ `verification`
- `verification` ↛ `user`, `verification` ↛ `auth`

Когда домену A нужны данные из домена B, объявляешь **порт** в A:

```go
// internal/domain/auth/usecase.go
type UserReader interface {
    GetByID(ctx context.Context, id string) (*User, error)
    GetByEmail(ctx context.Context, email string) (*User, error)
}
```

И **адаптер** в `auth/module/adapters.go` оборачивает домен B в этот порт:

```go
type userReaderAdapter struct {
    uc *user.Usecase
}

func (a *userReaderAdapter) GetByID(ctx context.Context, id string) (*auth.User, error) {
    u, err := a.uc.LookupByID(ctx, id)
    if err != nil { return nil, err }
    return userToAuth(u), nil
}
```

Адаптер в `auth/module/` имеет право импортить `user` (он не в `auth/`-дереве с точки зрения depguard `files:` правила исключены через `*_test.go` исключение и path-match).

## Что куда

| Тип кода | Где |
|---|---|
| Доменные типы (User, Session) | `entity.go` |
| Ошибки домена | `errors.go` (sentinel: `var ErrX = errors.New(...)`) |
| Логика usecase | `usecase.go` |
| Authorization | `policy.go` |
| Outbox-события | `events.go` |
| HTTP-handler | `delivery/http/handler.go` |
| HTTP-input/output | `delivery/http/dto.go` (huma-теги) |
| HTTP→domain mapping | `delivery/http/view.go` |
| sqlc-запросы | `infra/postgres/queries/*.sql` |
| Repository (имплементит порт) | `infra/postgres/repository.go` |
| fx-wiring | `module/module.go` |
| Cross-domain адаптеры | `module/adapters.go` |

## Test-стратегия

- Юнит: моки портов, реальные структуры домена. Запускаются `make test`.
- Integration: build-tag `integration`, поднимают Postgres через testcontainers. `make test-int`.
- Все exported интерфейсы из `usecase.go` идут в `.mockery.yml`. После добавления — `make mocks`.

## Outbox-события

Если usecase эмитит событие (например, `user.registered`), оно идёт в `events.go` как тип:

```go
type UserRegistered struct {
    UserID    string
    Email     string
    Timestamp time.Time
}
```

Repository пишет событие в outbox-таблицу в той же транзакции что и основное изменение (`outboxkit.Publisher`). Воркер в `internal/worker/subscribers/` подписан на events bus и обрабатывает.

См. `auth.UserRegisteredHandler` в `internal/worker/subscribers/` как референс.
