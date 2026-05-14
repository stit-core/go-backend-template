---
name: outbox-events
description: Outbox-паттерн и доменные события. Грузится при работе с events.go или delivery/nats/.
paths:
  - "internal/domain/*/events.go"
  - "internal/domain/*/delivery/nats/**"
---

# Outbox и доменные события

## Зачем outbox

Транзакционная гарантия: событие пишется в БД в той же транзакции, что и основное изменение. Outbox-drain (`golibinfra/outboxkit`, поднимается fx.Lifecycle внутри того же `cmd/api`) потом отправляет в шину сообщений (NATS). Если падение между commit'ом и publish'ем — событие остаётся в `outbox`-таблице и ретраится.

Альтернатива (прямая публикация в NATS из usecase) — нет гарантии что данные применились + событие отправлено.

**Отдельного worker-процесса нет.** API, outbox drain, NATS subscribers и background jobs живут в одном бинаре через одну fx-графу.

Сама `outbox`-таблица — **не миграция в этом репо**: схему держит `golibinfra/outboxkit/pgstore` (`MigrationsFS`), `cmd/migrate` накатывает её рядом с миграциями приложения и kit'ов (`auditkit`, `bruteforcekit`). См. `cmd/migrate/main.go`.

## Слой событий

В корне домена — `events.go` с payload-типами. Реф. `internal/domain/user/events.go`:

```go
package user

import "time"

type UserRegistered struct {
    ID    string
    Email string
    At    time.Time
}
```

**Subject — это литерал в формате `<domain>.events.<event>`**, передаётся прямо в `events.Publish(...)`. Константу субджекта домен-источник **не экспортит** — публикатор знает свой subject, а consumer заводит **локальную** константу в `delivery/nats/dto.go` (`const SubjectUserRegistered = "user.events.registered"` — реф. `internal/domain/verification/delivery/nats/dto.go`). Это даёт version-coexistence: меняешь subject у источника → старый consumer тихо отвалится от старого, не сломав компиляцию.

## Запись в outbox из usecase

`outboxkit.Publisher` пишет в outbox-таблицу **через context-bound транзакцию**: `pgkit.Transactor.WithTx(ctx, fn)` кладёт `pgx.Tx` в context, репозиторий и `outboxkit/pgstore` оба достают его через `pgkit.Conn(ctx, pool)`. Никаких явных `tx` в сигнатуре `Publish`. Реф. — `internal/domain/user/usecase.go:103`:

```go
if err := uc.tx.WithTx(ctx, func(txCtx context.Context) error {
    if err := uc.repo.Create(txCtx, newU); err != nil {
        return err
    }
    return uc.events.Publish(txCtx, "user.events.registered",
        UserRegistered{ID: newU.ID, Email: newU.Email, At: newU.CreatedAt})
}); err != nil { ... }
```

`uc.events` имеет тип `outboxkit.Publisher` (узкий interface — `Publish(ctx, subject, payload) error`). Инжектится в `UsecaseDeps` через fx. Сериализация payload в JSON — внутри `outboxkit/pgstore.Insert`.

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
    ctx, cancel := context.WithCancel(context.Background())
    lc.Append(fx.Hook{
        OnStart: func(_ context.Context) error {
            return natskit.SubscribeJSON(bus, ctx, SubjectUserRegistered, h.onUserRegistered)
        },
        OnStop: func(_ context.Context) error { cancel(); return nil },
    })
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
- ❌ Изменение payload-структуры события без version-poly. Если ломаешь — новый Subject (`user.events.registered.v2`).

## Миграция события (breaking change)

1. Новая структура `UserRegisteredV2`.
2. Новый Subject (`user.events.registered.v2`).
3. Подписчики на старый Subject продолжают работать.
4. Когда все подписчики мигрировали — снимаешь публикацию старого.
