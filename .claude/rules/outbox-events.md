---
name: outbox-events
description: Outbox-паттерн и доменные события. Грузится при работе с events.go или outbox.
paths:
  - "internal/domain/*/events.go"
  - "internal/domain/*/events/**"
  - "internal/worker/subscribers/**"
  - "migrations/0003_outbox*"
---

# Outbox и доменные события

## Зачем outbox

Транзакционная гарантия: событие пишется в БД в той же транзакции, что и основное изменение. Воркер потом отправляет в шину сообщений (NATS). Если падение — событие в `outbox` таблице, retried.

Альтернатива (прямая публикация в NATS из usecase) — нет гарантии что данные применились + событие отправлено.

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

## Подписчики (`internal/worker/subscribers/`)

Worker (`cmd/worker/`) поднимает NATS-консьюмеров. Каждый подписчик — структура с зависимостями + handler:

```go
package subscribers

type UserRegistered struct {
    notifier Notifier  // порт из текущего пакета
}

func (h *UserRegistered) Handle(ctx context.Context, ev user.Registered) error {
    return h.notifier.SendWelcomeEmail(ctx, ev.Email)
}

func (h *UserRegistered) Subject() string { return user.RegisteredSubject }
```

Регистрируется в worker-е через `natskit.Subscribe[user.Registered](js, h)`.

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
