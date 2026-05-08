---
name: worker-subscribers
description: Конвенции для cmd/worker и internal/worker/subscribers/. Фоновые консьюмеры и jobs.
paths:
  - "cmd/worker/**"
  - "internal/worker/**"
---

# Worker и подписчики

## Структура

```
cmd/worker/
  main.go             fx.New(infraModules, platformModules, subscribersModule).Run()
internal/worker/
  bruteforce.go       Background job: cleanup brute-force counters
  subscribers/
    <event>_handler.go   Один подписчик на одно событие домена
    module.go            fx.Module: Provide хендлеров + Invoke регистрации
```

## Подписчик

Один файл — одна подписка. Имя `<source_domain>_<event>_handler.go` или `<event>_handler.go` если domain очевиден.

Структура:

```go
package subscribers

type UserRegisteredHandler struct {
    log      logkit.Logger
    notifier Notifier  // порт, реализуется в module/adapters.go
}

func NewUserRegisteredHandler(log logkit.Logger, n Notifier) *UserRegisteredHandler {
    return &UserRegisteredHandler{log: log, notifier: n}
}

func (h *UserRegisteredHandler) Handle(ctx context.Context, ev user.Registered) error {
    h.log.Info(ctx, "user registered", "user_id", ev.UserID)
    return h.notifier.SendWelcomeEmail(ctx, ev.Email)
}

func (h *UserRegisteredHandler) Subject() string { return user.RegisteredSubject }
```

## Регистрация в fx

`internal/worker/subscribers/module.go`:

```go
var Module = fx.Module(
    "subscribers",
    fx.Provide(NewUserRegisteredHandler),
    fx.Invoke(func(js natskit.JetStream, h *UserRegisteredHandler) error {
        return natskit.Subscribe[user.Registered](js, h)
    }),
)
```

`natskit.Subscribe` — обёртка над nats.Subscription, разбирает payload в типизированную структуру.

## Idempotency

- Все подписчики должны быть **идемпотентны**: повторная обработка того же события не должна давать побочных эффектов.
- Стратегии:
  - Дедупликация по идентификатору события (если событие несёт `event_id`).
  - Идемпотентная семантика: `INSERT ... ON CONFLICT DO NOTHING`, `UPDATE ... WHERE status = ...`.
  - Внешние API: idempotency key в заголовке.

## Background jobs (не подписчики)

Например `internal/worker/bruteforce.go` — периодический cleanup.

```go
type BruteforceJob struct {
    repo  bruteforcekit.Repository
    timer timerkit.Ticker
}

func (j *BruteforceJob) Start(ctx context.Context) error {
    return j.timer.Every(ctx, 5*time.Minute, func(ctx context.Context) error {
        return j.repo.CleanupExpired(ctx)
    })
}
```

Регистрируется аналогично — через fx.Module + fx.Invoke стартует через lifecycle hook.

## Логирование

- Все хендлеры логируют через `logkit.Logger`.
- На входе — `Info` с ключевыми полями события.
- На ошибке — `Error` с `err`. Возврат err — natskit ретраит сообщение.
- Не логируй `Debug` в проде (если не включён debug-mode).

## Anti-patterns

- ❌ Два подписчика на один Subject в одном консьюмере. Запросы дедуплицируются — будет один из них.
- ❌ Подписчик использует прямой импорт другого домена. Используй порт + адаптер в `module/adapters.go`.
- ❌ Тяжёлая работа синхронно (например, генерация PDF). Выноси в отдельный job-runner.
- ❌ Захват контекстных полей в горутины — utilizировать `ctx` явно через cancellation.
