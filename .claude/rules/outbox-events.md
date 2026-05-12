---
name: outbox-events
description: Outbox-паттерн и доменные события. Грузится при работе с events.go или delivery/nats/.
paths:
  - "internal/domain/*/events.go"
  - "internal/domain/*/delivery/nats/**"
  - "migrations/0004_outbox*"
---

# Outbox и доменные события

## Зачем outbox

Транзакционная гарантия: событие пишется в БД в той же транзакции, что и основное изменение. Outbox-drain (`golibinfra/outboxkit`, поднимается fx.Lifecycle внутри того же `cmd/api`) потом отправляет в шину сообщений (NATS). Если падение между commit'ом и publish'ем — событие остаётся в `outbox`-таблице и ретраится.

Альтернатива (прямая публикация в NATS из usecase) — нет гарантии что данные применились + событие отправлено.

**Отдельного worker-процесса нет.** API, outbox drain, NATS subscribers и background jobs живут в одном бинаре через одну fx-графу.

## Слой событий

В корне домена — `events.go` с типами:

```go
package user

type Registered struct {
    UserID    string    `json:"user_id"`
    Email     string    `json:"email"`
    Timestamp time.Time `json:"timestamp"`
}

const RegisteredSubject = "user.registered"
```

`Subject` — имя топика NATS (kebab-case через точку: `domain.event`).

## Запись в outbox из usecase

В транзакции репозитория:

```go
err := pgkit.InTx(ctx, r.pool, func(tx pgx.Tx) error {
    // 1. Основное изменение.
    if err := r.queries.WithTx(tx).InsertUser(ctx, params); err != nil {
        return err
    }
    // 2. Событие в outbox — той же tx.
    return r.outbox.Publish(ctx, tx, user.RegisteredSubject, event)
})
```

`outbox.Publish` — из `golibinfra/outboxkit`, принимает Subject + payload. Сериализует payload в JSON и пишет строку в `outbox` таблицу.

## Подписчики (in-domain — `internal/domain/<consumer>/delivery/nats/`)

Subscriber лежит **внутри** потребляющего домена, в `delivery/nats/` — рядом с `delivery/http/`, на равных правах: оба слоя — это «входы» в usecase из разных транспортов. Регистрируется fx.Lifecycle того же `cmd/api`.

```go
// internal/domain/verification/delivery/nats/handler.go
package nats

type Usecase interface {
    IssueEmailVerification(ctx context.Context, userID, email string) error
}

type Handler struct {
    uc  Usecase
    log logkit.Logger
}

func NewHandler(uc Usecase, log logkit.Logger) *Handler {
    return &Handler{uc: uc, log: log.With("delivery", "verification.nats")}
}

func (h *Handler) onUserRegistered(ctx context.Context, e UserRegisteredEvent) error {
    return h.uc.IssueEmailVerification(ctx, e.ID, e.Email)
}

func Register(lc fx.Lifecycle, bus natskit.Bus, h *Handler) {
    // подписка через natskit.Subscribe[UserRegisteredEvent](bus, h.onUserRegistered)
    // запуск/остановка через fx.Lifecycle
}
```

Регистрация — в `module/module.go` потребляющего домена: `fx.Provide(nats.NewHandler)` + `fx.Invoke(nats.Register)`.

**Cross-domain payload**: subscriber объявляет **локальный** payload-тип (`UserRegisteredEvent` в `verification/delivery/nats/dto.go`), даже если он 1:1 совпадает с типом из домена-источника. Прямой импорт `user`-домена из `verification` запрещён depguard'ом.

## Гарантии

- **At-least-once** доставка. Подписчики должны быть **idempotent**.
- Дедупликация — через primary key операции или явный `IF NOT EXISTS`.
- Не делай side-effects в usecase до коммита транзакции. Side-effects — в подписчиках.

## Порядок событий

NATS даёт ordered delivery в рамках одного subject и одного consumer. Если важен глобальный порядок — используй один consumer.

## Anti-patterns

- ❌ Прямая публикация в NATS из usecase в обход outbox — нет транзакционной гарантии.
- ❌ Тяжёлая логика в подписчике — она должна быть быстрой и идемпотентной. Длинная логика — отдельный job.
- ❌ Подписчик мутирует БД своего же домена → опасно (можно зациклить через outbox). Лучше — отдельный домен / отдельный воркер.
- ❌ Изменение payload-структуры события без version-poly. Если ломаешь — новый Subject (`user.registered.v2`).

## Миграция события (breaking change)

1. Новая структура `RegisteredV2`.
2. Новый Subject (`user.registered.v2`).
3. Подписчики на старый Subject продолжают работать.
4. Когда все подписчики мигрировали — снимаешь публикацию старого.
