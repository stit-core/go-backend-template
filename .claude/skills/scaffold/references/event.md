# scaffold › event — outbox-эвент + subscriber

Эвент-флоу: usecase домена-источника пишет событие в **per-domain outbox** (`outbox_<source>`) в той же транзакции, что и основное изменение → drain этого инстанса (поднимается fx.Lifecycle того же `cmd/api`) публикует в **JetStream** через `natskit.StreamBus` → durable-subscriber потребляющего домена (`internal/domain/<consumer>/delivery/nats/`) обрабатывает. Отдельного worker-процесса нет — всё в одном бинаре.

См. `.claude/rules/events-and-messaging.md` (бэкграунд — автогрузится при правке `events.go` / `delivery/nats/` / `infra/outbox/`) и `docs/reference/architecture.md § События и messaging`. Живой образец — `user.events.registered`: публикация в `internal/domain/user/usecase.go`, потребление в `internal/domain/verification/delivery/nats/`.

> `<module-path>` в импортах ниже — module path из `go.mod` (после `/de-template` — свой
> `git.stit.tech/<org>/backend`). Подставляй фактический, не копируй буквально.

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
- Какие поля в payload (минимум — ID агрегата + `At`)?
- Какой Subject? Формат: `<source>.events.<event>` (реф. — `user.events.registered`, `verification.events.password_reset_requested` в `internal/domain/*/events.go`).
- Какой домен **обрабатывает** (или несколько)?
- Idempotency: на каком natural key consumer отсечёт повторную доставку?

### 2. Тип события и subject в `internal/domain/<source>/events.go`

```go
package <source>

import "time"

// Subjects stay unexported: consumers declare their own literal (rule events-and-messaging).
const subject<Aggregate><PastEvent> = "<source>.events.<event>"

type <Aggregate><PastEvent> struct {
    <Aggregate>ID string
    // ...payload поля
    At            time.Time
}
```

Поля **без json-тегов**: wire = Go-имена, смена формата = новый subject `….v2`. `At` — `uc.clock.Now()` (или время из сохранённой сущности). Subject — unexported const рядом с payload: не inline-литерал в usecase и не экспорт; consumer заводит свою константу (шаг 4).

### 3. Публикация в usecase

В usecase-методе, **внутри той же tx что и основное изменение**, через `pgkit.Transactor.WithTx` (tx живёт в `context`, передаётся в repo и outbox через `txCtx`):

```go
err := uc.tx.WithTx(ctx, func(txCtx context.Context) error {
    if err := uc.repo.UpdateX(txCtx, ...); err != nil {
        return err
    }
    return uc.events.Publish(txCtx, subject<Aggregate><PastEvent>, <Aggregate><PastEvent>{
        <Aggregate>ID: ...,
        At:            uc.clock.Now(),
    })
})
```

`uc.events` — это `outboxkit.Publisher` (`Publish(ctx, subject, payload) error`), инжектится в `UsecaseDeps` (поле `Events`). Сериализация в JSON — внутри `outboxkit.Bus.Publish`.

Если домен-источник **ещё не публикует** события — заведи ему per-domain outbox: `internal/domain/<source>/infra/outbox/outbox.go` с `const Table = "outbox_<source>"`, `var Migrations = outboxpg.TableMigrations(Table)` и `var Providers = events.OutboxFor("<source>", Table)`; включи `Providers` в `<source>/module/module.go` (даёт приватный `outboxkit.Publisher`); добавь `<source>outbox.Migrations` в список источников `cmd/migrate/main.go` (схема kit-owned, в `migrations/` не дублируется — образец `internal/domain/user/infra/outbox/outbox.go`); если subject-префикс новый — добавь стрим в `NATS_JS_STREAM_NAMES` (инфра-handoff).

### 4. Subscriber in-domain (`internal/domain/<consumer>/delivery/nats/`)

Subscriber лежит **внутри** потребляющего домена, рядом с `delivery/http/`. archtest не пускает прямой импорт `<source>`-домена из `<consumer>` — поэтому payload-тип события объявляется **локально** в `delivery/nats/dto.go`, даже если он 1:1 совпадает с типом из источника. Subject — тоже локальная константа.

```go
// internal/domain/<consumer>/delivery/nats/dto.go
package nats

import "time"

// Локальная схема payload'а <source>.<Aggregate><PastEvent>: json-теги = Go-имена полей
// источника (у источника тегов нет — wire = Go-имена).
type <Event>Event struct {
    <Aggregate>ID string    `json:"<Aggregate>ID"`
    // …поля 1:1 с источником, но локальный тип
    At            time.Time `json:"At"`
}

const Subject<Event> = "<source>.events.<event>"
```

```go
// internal/domain/<consumer>/delivery/nats/handler.go
package nats

import (
    "context"
    "errors"
    "fmt"

    "go.uber.org/fx"

    "git.stit.tech/stit-core/golibinfra/logkit"
    "git.stit.tech/stit-core/golibinfra/natskit"
)

const (
    stream<Event>  = "<source>"                  // стрим покрывает <source>.>
    durable<Event> = "<consumer>-<event>"        // уникальное имя durable-консьюмера
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

func (h *Handler) on<Event>(ctx context.Context, e <Event>Event) error {
    if err := h.uc.DoSomething(ctx, /* mapping из e */); err != nil {
        h.log.Error(ctx, "handle <event>", "<aggregate>_id", e.<Aggregate>ID, "err", err)
        return fmt.Errorf("handle <event>: %w", err)
    }
    h.log.Info(ctx, "<event> handled", "<aggregate>_id", e.<Aggregate>ID)
    return nil
}

func Register(lc fx.Lifecycle, bus natskit.StreamBus, h *Handler) {
    ctx, cancel := context.WithCancel(context.Background())
    var sub natskit.Subscription
    lc.Append(fx.Hook{
        OnStart: func(_ context.Context) error {
            s, err := natskit.SubscribeStreamJSON(bus, ctx, stream<Event>, Subject<Event>, durable<Event>, h.on<Event>)
            if errors.Is(err, natskit.ErrJetStreamDisabled) {
                h.log.Warn(ctx, "jetstream disabled: <event> intake off", "subject", Subject<Event>)
                return nil // local-dev без -js: сервис стартует, событие не потребляется
            }
            if err != nil {
                return fmt.Errorf("subscribe %s: %w", Subject<Event>, err)
            }
            sub = s
            return nil
        },
        OnStop: func(stopCtx context.Context) error {
            defer cancel() // Drain раньше отмены ctx: обработчики в работе успевают Ack
            if sub == nil {
                return nil
            }
            return sub.Drain(stopCtx)
        },
    })
}
```

### 5. Регистрация в `<consumer>/module/module.go`

```go
import (
    natsdelivery "<module-path>/internal/domain/<consumer>/delivery/nats"
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

JetStream доставляет at-least-once, поэтому идемпотентность — свойство **схемы**, а не удачи:
- unique-индекс под natural key + `INSERT … ON CONFLICT DO NOTHING` (`:execrows`; 0 rows → пропусти side-effects: письмо/событие не повторяются) — канон `verification.IssueEmailVerification` + партиал-индекс `email_verifications(user_id) WHERE used_at IS NULL`;
- `UPDATE … WHERE status = 'pending'` для state-machine'ов;
- idempotency-key из payload — для внешних API.

Redelivery-кейс покрой юнит-тестом (шаг 7).

### 7. Тесты

- В домене-публикаторе: `usecase_test.go` с фейком `golibinfra/testkit/outboxtest.Recorder` в `UsecaseDeps.Events` — проверяй subject и payload в `Recorder.All()`.
- Subscriber: unit-тест на handler-метод с моком `Usecase`-порта (добавь `delivery/nats` в `.mockery.yml` → `make gen`): happy path, error path (usecase вернул err → handler вернул err, сообщение уйдёт на redelivery) и дубль-доставка.
- Integration: если есть end-to-end — отдельный тест под `//go:build integration` с реальным NATS через testcontainers.

### 8. Верификация

```bash
make test
make lint
make gen-check  # если добавил интерфейс из .mockery.yml
```

### 9. Коммит

`feat(<source>): emit <event> event` или `feat(<consumer>): handle <event>` — в зависимости от того, что добавил. Если оба в одном коммите — `feat(<source>): emit <event> + <consumer> handler`.

## Versioning событий

Если меняешь payload-структуру с breaking change:
1. Новый тип `<Event>V2`, новая const-subject `"<source>.events.<event>.v2"`.
2. Старый subscriber продолжает работать со старым subject.
3. Когда все consumer'ы перешли — удаляешь публикацию старого.

Не правь существующий тип события без breaking-version-bump.

## Anti-patterns

- ❌ Прямой `nats.Publish()` из usecase в обход outbox — нет транзакционной гарантии.
- ❌ Тяжёлая логика в Subscriber (генерация PDF, batch-обработка). Subscriber обязан быть быстрым.
- ❌ Subscriber дёргает usecase своего же домена → риск зацикливания через outbox. Лучше отдельный домен или job.
- ❌ Прямой импорт другого бизнес-домена в `delivery/nats/`. Используй локальный DTO (и порт + адаптер, если нужны данные).
- ❌ Идемпотентность «на удачу» — без unique-индекса / статус-гварда повтор доставки задвоит side-effect.
- ❌ Менять формат payload без версионирования.
