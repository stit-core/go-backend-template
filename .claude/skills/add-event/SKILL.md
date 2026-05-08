---
name: add-event
description: Добавить новый outbox-эвент в домен + subscriber-handler в worker + регистрацию в worker module. Активируй когда юзер просит "добавь событие", "новый outbox-эвент", "новый NATS subscriber", "после X должно произойти Y асинхронно".
---

# Skill: add-event

Эвент-флоу: usecase пишет событие в outbox в той же транзакции, что и основное изменение → worker читает outbox → publish в NATS → subscriber обрабатывает.

См. `.claude/rules/outbox-events.md` и `.claude/rules/worker-subscribers.md` для бэкграунда — они автогрузятся, когда правишь `events.go` или `worker/subscribers/`.

## Когда применять

- "После регистрации юзера слать welcome-email"
- "Добавь эвент `payment.refunded`"
- "Когда сессия заканчивается — записать в audit"

## Когда НЕ применять

- Side-effect синхронный, в контексте запроса — это просто вызов в usecase, не эвент.
- Нужен ровно один consumer, который ничего другого не делает — иногда проще вызвать порт напрямую.

## Решение: outbox или прямой вызов

Outbox оправдан когда:
- Действие должно произойти **после** успешного коммита транзакции.
- Действие **может упасть** (внешний API, email-провайдер) — нужны retries.
- Несколько consumer'ов одного события (email + push + analytics).

Если хотя бы один из ✓ — иди в outbox. Иначе — порт + адаптер, синхронно.

## Шаги

### 1. Уточнить контракт события

- Какой домен **публикует**?
- Какие поля в payload (минимум — `EventID` или ID агрегата + timestamp)?
- Какой Subject? Формат: `<domain>.<event>` (kebab через точку): `user.registered`, `payment.refunded.v2`.
- Какой домен **обрабатывает** (или несколько)?
- Idempotency: как consumer защитится от дубликатов?

### 2. Тип события в `internal/domain/<source>/events.go`

```go
package <source>

import "time"

const <Event>Subject = "<source>.<event>"

type <Event> struct {
    EventID   string    `json:"event_id"`   // для идемпотентности
    <Aggregate>ID string `json:"<aggregate>_id"`
    // ...payload поля
    At        time.Time `json:"at"`
}
```

`EventID` — uuid, генерится в usecase при публикации. `At` — `clock.Now()`.

### 3. Публикация в usecase

В uсcase-методе, **внутри той же tx что и основное изменение**:

```go
err := uc.tx.WithTx(ctx, func(txCtx context.Context) error {
    if err := uc.repo.UpdateX(txCtx, ...); err != nil {
        return err
    }
    return uc.events.Publish(txCtx, <source>.<Event>Subject, <source>.<Event>{
        EventID: uuid.NewString(),
        // ...
        At: uc.clock.Now(),
    })
})
```

`uc.events` — это `outboxkit.Publisher`, уже инжектится в `UsecaseDeps`. Сериализация в JSON — внутри `outboxkit`.

### 4. Subscriber в `internal/worker/subscribers/`

Один файл = один subscriber. Имя: `<event>_handler.go` или `<source>_<event>_handler.go`.

```go
package subscribers

import (
    "context"

    "git.stit.tech/akadmin/go-backend-template/internal/domain/<source>"
    "git.stit.tech/akadmin/golibinfra/logkit"
)

type Notifier interface {
    SendWelcomeEmail(ctx context.Context, email string) error
}

type <Event>Handler struct {
    log      logkit.Logger
    notifier Notifier
}

func New<Event>Handler(log logkit.Logger, n Notifier) *<Event>Handler {
    return &<Event>Handler{log: log.With("subscriber", "<event>"), notifier: n}
}

func (h *<Event>Handler) Handle(ctx context.Context, ev <source>.<Event>) error {
    h.log.Info(ctx, "<event> received", "event_id", ev.EventID)
    return h.notifier.SendWelcomeEmail(ctx, ev.Email)
}

func (h *<Event>Handler) Subject() string { return <source>.<Event>Subject }
```

### 5. Cross-domain адаптер (если subscriber вызывает другой домен)

Если `Notifier` — это другой домен, **не импортируй его напрямую** из `subscribers/`. Объяви порт в `subscribers/` (как в шаге 4) и реализуй адаптер в `subscribers/adapters.go`.

### 6. Регистрация в `subscribers/module.go`

```go
var Module = fx.Module(
    "subscribers",
    fx.Provide(
        New<Event>Handler,
        // …порты-адаптеры…
    ),
    fx.Invoke(func(js natskit.JetStream, h *<Event>Handler) error {
        return natskit.Subscribe[<source>.<Event>](js, h)
    }),
)
```

Один `fx.Invoke` на subscriber. Если хочешь группировать — собери в одну функцию.

### 7. Idempotency

Subscriber должен переживать повторную доставку:
- INSERT с `ON CONFLICT (event_id) DO NOTHING` если пишет в свою БД.
- `UPDATE … WHERE status = 'pending'` для state-machine'ов.
- Idempotency-key для внешних API.

Сделай это явно, не "оно вряд ли задвоится".

### 8. Тесты

- В домене-публикаторе: `usecase_test.go` проверяет, что `eventbus.Recorder` зафиксировал нужный subject и payload (`testkit/eventbus` уже есть).
- Subscriber: unit-тест на `Handle()` с моком `Notifier`. Минимум — happy path + один error path (Notifier возвращает err → Handle возвращает err).
- Integration: если есть end-to-end — отдельный тест под `//go:build integration` с реальным NATS через testcontainers.

### 9. Верификация

```bash
scripts/agent-tools/test.sh
scripts/agent-tools/lint.sh
make mocks-check  # если добавил интерфейс из .mockery.yml
```

### 10. Коммит

`feat(<source>): emit <event> event` или `feat(worker): handle <event>` — в зависимости от того, что добавил. Если оба в одном коммите — `feat(<source>): emit <event> + handler`.

## Versioning событий

Если меняешь payload-структуру с breaking change:
1. Новый тип `<Event>V2`, новый `<Event>SubjectV2 = "<source>.<event>.v2"`.
2. Старый subscriber продолжает работать со старым subject.
3. Когда все consumer'ы перешли — удаляешь публикацию старого.

Не правь существующий тип события без breaking-version-bump.

## Anti-patterns

- ❌ Прямой `nats.Publish()` из usecase в обход outbox — нет транзакционной гарантии.
- ❌ Тяжёлая логика в Subscriber (генерация PDF, batch-обработка). Subscriber обязан быть быстрым.
- ❌ Subscriber дёргает usecase своего же домена → риск зацикливания через outbox. Лучше отдельный домен или job.
- ❌ Прямой импорт другого бизнес-домена в `subscribers/`. Используй порт + адаптер.
- ❌ Забыть `EventID` в payload — без него идемпотентность невозможна.
- ❌ Менять формат payload без версионирования.
