# scaffold › event — outbox-эвент + subscriber

Эвент-флоу: usecase домена-источника пишет событие в outbox в той же транзакции, что и основное изменение → `outboxkit.Publisher` (поднимается fx.Lifecycle того же `cmd/api`) публикует в NATS → subscriber потребляющего домена (`internal/domain/<consumer>/delivery/nats/`) обрабатывает. Отдельного worker-процесса нет — всё в одном бинаре.

См. `.claude/rules/events-and-messaging.md` (бэкграунд — автогрузится при правке `events.go` / `delivery/nats/` / `internal/jobs/`) и `docs/reference/architecture.md § События и messaging`.

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
- Какой Subject? Формат: `<source>.events.<event>` (реф. — `user.events.registered`, `verification.events.password_reset_requested` из `internal/domain/*/usecase.go`).
- Какой домен **обрабатывает** (или несколько)?
- Idempotency: как consumer защитится от дубликатов?

### 2. Тип события в `internal/domain/<source>/events.go`

```go
package <source>

import "time"

type <Event> struct {
    EventID       string    `json:"event_id"`   // для идемпотентности
    <Aggregate>ID string    `json:"<aggregate>_id"`
    // ...payload поля
    At            time.Time `json:"at"`
}
```

`EventID` — uuid, генерится в usecase при публикации. `At` — `uc.clock.Now()`. Subject как именованную константу публикатор обычно **не экспортит** — он литералом передаётся в `Publish(...)`, а consumer заводит свою локальную константу (см. шаг 4).

### 3. Публикация в usecase

В usecase-методе, **внутри той же tx что и основное изменение**, через `pgkit.Transactor.WithTx` (tx живёт в `context`, передаётся в repo и outbox через `txCtx`):

```go
err := uc.tx.WithTx(ctx, func(txCtx context.Context) error {
    if err := uc.repo.UpdateX(txCtx, ...); err != nil {
        return err
    }
    return uc.events.Publish(txCtx, "<source>.events.<event>", <source>.<Event>{
        EventID:       uuid.NewString(),
        <Aggregate>ID: ...,
        At:            uc.clock.Now(),
    })
})
```

`uc.events` — это `outboxkit.Publisher` (`Publish(ctx, subject, payload) error`), уже инжектится в `UsecaseDeps`. Сериализация в JSON — внутри `outboxkit/pgstore.Insert`.

### 4. Subscriber in-domain (`internal/domain/<consumer>/delivery/nats/`)

Subscriber лежит **внутри** потребляющего домена, рядом с `delivery/http/`. depguard не пускает прямой импорт `<source>`-домена из `<consumer>` — поэтому payload-тип события объявляется **локально** в `delivery/nats/dto.go`, даже если он 1:1 совпадает с типом из источника. Subject — тоже локальная константа. Если `<consumer>` — новый домен, заведи ему правило `<consumer>-domain-isolation` в `.golangci.yml` (`scaffold/references/domain.md`).

```go
// internal/domain/<consumer>/delivery/nats/dto.go
package nats

type <Event>Payload struct {
    EventID string `json:"event_id"`
    // …поля 1:1 с <source>.<Event>, но локальный тип
}

const Subject<Event> = "<source>.events.<event>"
```

```go
// internal/domain/<consumer>/delivery/nats/handler.go
package nats

import (
    "context"
    "fmt"

    "go.uber.org/fx"

    "git.stit.tech/stit-infra/golibinfra/logkit"
    "git.stit.tech/stit-infra/golibinfra/natskit"
)

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

func (h *Handler) on<Event>(ctx context.Context, e <Event>Payload) error {
    if err := h.uc.DoSomething(ctx, /* mapping из e */); err != nil {
        h.log.Error(ctx, "handle <event>", "event_id", e.EventID, "err", err)
        return fmt.Errorf("handle <event>: %w", err)
    }
    h.log.Info(ctx, "<event> handled", "event_id", e.EventID)
    return nil
}

func Register(lc fx.Lifecycle, bus natskit.Bus, h *Handler) {
    ctx, cancel := context.WithCancel(context.Background())
    lc.Append(fx.Hook{
        OnStart: func(_ context.Context) error {
            return natskit.SubscribeJSON(bus, ctx, Subject<Event>, h.on<Event>)
        },
        OnStop: func(_ context.Context) error { cancel(); return nil },
    })
}
```

### 5. Регистрация в `<consumer>/module/module.go`

```go
import (
    natsdelivery "git.stit.tech/stit-template/go-backend-template/internal/domain/<consumer>/delivery/nats"
)

var Module = fx.Module(
    "<consumer>",
    fx.Provide(
        // …существующие провайдеры…
        natsdelivery.NewHandler,
        func(uc *<consumer>.Usecase) natsdelivery.Usecase { return uc },
    ),
    fx.Invoke(natsdelivery.Register),
)
```

Subscriber фактически — это просто ещё один транспорт в usecase. Никаких отдельных fx-модулей "subscribers" не заводи.

### 6. Idempotency

Subscriber должен переживать повторную доставку:
- INSERT с `ON CONFLICT (event_id) DO NOTHING` если пишет в свою БД.
- `UPDATE … WHERE status = 'pending'` для state-machine'ов.
- Idempotency-key для внешних API.

Сделай это явно, не "оно вряд ли задвоится".

### 7. Тесты

- В домене-публикаторе: `usecase_test.go` через мок `outboxkit.Publisher` (`Mock<...>` — `make gen` поднимает по `.mockery.yml`) проверяет, что вызов `Publish` пришёл с ожидаемым subject и payload.
- Subscriber: unit-тест на handler-метод с моком `Usecase`-порта (мок генерируется автоматически — `.mockery.yml` уже включает `delivery/nats` если ты его добавил). Минимум — happy path + один error path (usecase возвращает err → handler возвращает err).
- Integration: если есть end-to-end — отдельный тест под `//go:build integration` с реальным NATS через testcontainers.

### 8. Верификация

```bash
scripts/tools/test.sh
scripts/tools/lint.sh
make gen-check  # если добавил интерфейс из .mockery.yml
```

### 9. Коммит

`feat(<source>): emit <event> event` или `feat(<consumer>): handle <event>` — в зависимости от того, что добавил. Если оба в одном коммите — `feat(<source>): emit <event> + <consumer> handler`.

## Versioning событий

Если меняешь payload-структуру с breaking change:
1. Новый тип `<Event>V2`, новый литерал-subject `"<source>.events.<event>.v2"`.
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
