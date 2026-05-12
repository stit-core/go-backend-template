---
name: jobs-and-subscribers
description: Конвенции для in-domain NATS subscribers и background jobs. Грузится при работе с internal/domain/*/delivery/nats/ или internal/jobs/.
paths:
  - "internal/domain/*/delivery/nats/**"
  - "internal/jobs/**"
---

# NATS subscribers и background jobs

В этом шаблоне **нет отдельного worker-процесса**. HTTP, outbox drain, NATS subscribers и background jobs — всё в одном бинаре `cmd/api/` через одну fx.Lifecycle.

## NATS subscribers (in-domain)

Subscriber потребляющего домена лежит **внутри** этого домена, в `delivery/nats/` — рядом с `delivery/http/`. Оба слоя на равных правах: это «входы» в usecase из разных транспортов.

### Структура

```
internal/domain/<consumer>/
├── usecase.go             уже существует
├── delivery/
│   ├── http/              REST-входы
│   └── nats/              NATS-входы (этот файл)
│       ├── dto.go         payload-тип события (локальный для consumer'а)
│       └── handler.go     Handler + Register (fx)
└── module/module.go       fx.Provide(nats.NewHandler) + fx.Invoke(nats.Register)
```

### Handler

```go
// internal/domain/<consumer>/delivery/nats/handler.go
package nats

type Usecase interface {
    DoSomething(ctx context.Context, ...) error
}

type Handler struct {
    uc  Usecase
    log logkit.Logger
}

func NewHandler(uc Usecase, log logkit.Logger) *Handler {
    return &Handler{uc: uc, log: log.With("delivery", "<consumer>.nats")}
}

func (h *Handler) onEventX(ctx context.Context, e EventXPayload) error {
    if err := h.uc.DoSomething(ctx, ...); err != nil {
        h.log.Error(ctx, "handle eventX", "err", err)
        return fmt.Errorf("handle eventX: %w", err)
    }
    return nil
}

func Register(lc fx.Lifecycle, bus natskit.Bus, h *Handler) {
    // natskit.Subscribe[EventXPayload](bus, h.onEventX) под lifecycle-hook'и
}
```

### Cross-domain payload

Subscriber объявляет **локальный** payload-тип в `delivery/nats/dto.go`, даже если он 1:1 повторяет тип из домена-источника. Прямой импорт другого бизнес-домена запрещён depguard'ом.

```go
// internal/domain/verification/delivery/nats/dto.go
package nats

type UserRegisteredEvent struct {
    ID    string `json:"id"`
    Email string `json:"email"`
}
```

Если потребитель и источник эволюционируют независимо — это даёт чистое versioning'овое окно: меняешь payload в источнике, добавляешь поля в локальный DTO потребителя только когда они нужны.

### Idempotency

NATS даёт **at-least-once**. Subscriber обязан быть идемпотентен:
- INSERT с `ON CONFLICT (event_id) DO NOTHING`,
- `UPDATE … WHERE status = 'pending'` для state-machine'ов,
- Idempotency-key для внешних API.

Сделай это явно, не "вряд ли задвоится".

## Background jobs (`internal/jobs/`)

Job — это периодическая задача без события-триггера (cleanup, aggregation, polling external system). Лежит в `internal/jobs/<name>.go`. Запускается fx.Lifecycle того же `cmd/api`.

```go
// internal/jobs/bruteforce.go
package jobs

func NewBruteforceCleanup(g bruteforcekit.Guard) timerkit.Job {
    return timerkit.Job{
        Name:     "bruteforce.cleanup",
        Interval: 5 * time.Minute,
        Run:      func(ctx context.Context) error { return g.Cleanup(ctx) },
    }
}
```

Регистрация — в `cmd/api/main.go` через `fx.Annotate(jobs.NewBruteforceCleanup, fx.ResultTags(\`group:"timer.job"\`))`.

**Правило:** если job чистит/обходит таблицу, которой владеет какой-то kit (`bruteforcekit`, в будущем `auditkit`/`outboxkit`), зови метод kit-интерфейса (`Guard.Cleanup`), а не сырой SQL по имени таблицы. Сырой SQL → app знает приватную схему kit'а → дырявая абстракция, ломается при апгрейде kit'а.

## Логирование

- `logkit.Logger` с лейблом транспорта: subscriber — `log.With("delivery", "<domain>.nats")`; job — `log.With("job", "<name>")`.
- Ошибка из subscriber → `return err` → natskit ретраит сообщение (с учётом backoff).
- В job — `Error` с контекстом, возврат err НЕ должен ронять процесс; job-runner логирует и ждёт следующий tick.

## Anti-patterns

- ❌ Создавать отдельный `cmd/worker/` бинарь. В этом шаблоне всё в одном процессе.
- ❌ Subscriber импортит домен-источник напрямую. Объяви локальный payload-DTO в `delivery/nats/dto.go`.
- ❌ Два subscriber'а на один Subject в одном NATS-consumer'е. Сообщение получит только один.
- ❌ Тяжёлая логика в subscriber (генерация PDF, batch на 1M строк). Subscriber должен быть быстрым; длинное — вынеси в job или в отдельный usecase, который subscriber только дёргает.
- ❌ Захват `ctx` в горутины без явного cancellation.
- ❌ `time.Now()` в job — инжекть `clockkit.Clock` для тестируемости.
