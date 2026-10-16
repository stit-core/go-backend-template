# scaffold › event — outbox-эвент + subscriber

**Шаг 0** — прочитай инструментом Read `.claude/rules/events-and-messaging.md`,
`.claude/rules/domain-architecture.md` и `.claude/rules/tests-pipeline.md`.

Эвент-флоу: usecase домена-источника пишет событие в **per-domain outbox** (`outbox_<source>`) в той же транзакции, что и основное изменение → drain этого инстанса (поднимается fx.Lifecycle того же `cmd/api`) публикует в **JetStream** через `natskit.StreamBus` → durable-subscriber потребляющего домена (`internal/domain/<consumer>/delivery/nats/`) обрабатывает. Отдельного worker-процесса нет — всё в одном бинаре.

Как и почему — `docs/reference/architecture.md § События и messaging`. Живой образец потока — `user.events.registered.v2`: публикация в `internal/domain/user/usecase.go`, потребление в `internal/domain/verification/delivery/nats/` (адрес потребитель берёт портом `UserReader` по ID из события). Поля payload — по правилу «ID и факты» (rule `domain-architecture`).

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
- Время действия не должно попасть в ответ: письмо на адрес, который ввёл аноним, выдало бы учётную запись.
- Несколько consumer'ов одного события (email + push + analytics).

Если хотя бы один из ✓ — иди в outbox. Иначе — порт + адаптер, синхронно.

## Шаги

### 1. Уточнить контракт события

- Какой домен **публикует**?
- Какие поля в payload (ID и факты, минимум — ID агрегата + `At`; токенов и PII нет)?
- Какой Subject? Формат: `<source>.events.<event>`; первая версия — без суффикса (`.v1` — отказ archtest `TestSubjectVersions`), новая версия payload — следующий номер `.v2`, `.v3`… (реф. — `user.events.registered.v2`, `verification.events.password_reset_requested.v3` в `internal/domain/*/events.go`).
- Какой домен **обрабатывает** (или несколько)?
- Idempotency: на каком ключе — событие (idempotency-key) или состояние агрегата — consumer отсечёт повторную доставку?

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

Поля **без json-тегов**: wire = Go-имена, смена формата = новый subject `….v2`. `At` — `uc.d.Clock.Now()`. Subject — unexported const рядом с payload: не inline-литерал в usecase и не экспорт; consumer заводит свою константу (шаг 4).

### 3. Публикация в usecase

В usecase-методе, **внутри той же tx что и основное изменение**, через `pgkit.Transactor.WithTx` (tx живёт в `context`, передаётся в repo и outbox через `txCtx`):

```go
err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
    if err := uc.d.Repo.UpdateX(txCtx, ...); err != nil {
        return err
    }
    return uc.d.Events.Publish(txCtx, subject<Aggregate><PastEvent>, <Aggregate><PastEvent>{
        <Aggregate>ID: ...,
        At:            uc.d.Clock.Now(),
    })
})
```

`uc.d.Events` — `outboxkit.Publisher` (`Publish(ctx, subject, payload) error`), поле `Events` в `UsecaseDeps`. Сериализация в JSON — внутри `outboxkit.Bus.Publish`.

Если домен-источник **ещё не публикует** события — заведи ему per-domain outbox: `internal/domain/<source>/infra/outbox/outbox.go` с `const Table = "outbox_<source>"`, `var Migrations = outboxpg.TableMigrations(Table)` и `var Providers = events.OutboxFor("<source>", Table)`; включи `Providers` в `<source>/module/module.go` (даёт приватный `outboxkit.Publisher`); добавь `<source>outbox.Migrations` в `sources.Kits` (`cmd/migrate/sources/sources.go`; схема kit-owned, в `migrations/` не дублируется — образец `internal/domain/user/infra/outbox/outbox.go`); если subject-префикс новый — добавь стрим в `NATS_JS_STREAM_NAMES` (инфра-handoff).

### 4. Subscriber in-domain (`internal/domain/<consumer>/delivery/nats/`)

Subscriber лежит **внутри** потребляющего домена, рядом с `delivery/http/`: подписку функциями natskit заводит только `delivery/nats` (не `module/` и не подпакет), обработчик зовёт домен портом `Usecase`, его `module/` привязывает к `*<consumer>.Usecase` (archtest `TestInputsRegisterInDelivery`, `TestDeliveryPortsBoundToUsecase`). archtest не пускает прямой импорт `<source>`-домена из `<consumer>` — поэтому payload-тип события объявляется **локально** в `delivery/nats/dto.go`, даже если он 1:1 совпадает с типом из источника. Subject — тоже локальная константа.

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

// Ошибку логирует natskit вместе с повтором доставки — обработчик её только возвращает.
func (h *Handler) on<Event>(ctx context.Context, e <Event>Event) error {
    if err := h.uc.DoSomething(ctx, /* mapping из e */); err != nil {
        return fmt.Errorf("handle <event> for <aggregate> %s: %w", e.<Aggregate>ID, err)
    }
    h.log.Info(ctx, "<event> handled", "<aggregate>_id", e.<Aggregate>ID)
    return nil
}

func Register(lc fx.Lifecycle, bus natskit.StreamBus, h *Handler) {
    natskit.SubscribeOnStart(lc, func(ctx context.Context) (natskit.Subscription, error) {
        return natskit.SubscribeStreamJSON(bus, ctx, stream<Event>, Subject<Event>, durable<Event>, h.on<Event>)
    })
}
```

`natskit.SubscribeOnStart` подписывает в OnStart и дренит подписку в OnStop, раньше остановки её
зависимостей (обработчики в работе успевают Ack). Ошибка подписки валит старт, в том числе
выключенный JetStream: нужны `NATS_JETSTREAM_ENABLED=true` и стрим в `NATS_JS_STREAM_NAMES`.

### 5. Регистрация в `<consumer>/module/module.go`

```go
import (
    natsdelivery "<module-path>/internal/domain/<consumer>/delivery/nats"
)

var Module = fx.Module(
    "internal/domain/<consumer>/module",
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
- unique-индекс под natural key + `INSERT … ON CONFLICT` — одна запись на ключ;
- `UPDATE … WHERE status = 'pending'` для state-machine'ов;
- idempotency-key из payload — для внешних API.

Внешний эффект (письмо, вебхук) при сбое не теряется: схема помнит, выполнен ли он. Канон —
`verification.IssueEmailVerification`:
1. состояние агрегата: подтверждённому адресу письмо не нужно — обработчик пропускает его до записи
   (порт `UserReader` отдаёт `EmailVerified`); партиал-индекс ключует только ожидающую запись, и после
   погашения ссылки повтор события выпустил бы второе письмо;
2. upsert по партиал-индексу `email_verifications(user_id) WHERE used_at IS NULL`: новое ожидающее
   подтверждение или новый токен неотправленного; отправленное (`sent_at`) не трогается — `RETURNING`
   пуст, обработчик пропускает эффект;
3. эффект — после коммита; сбой — ошибка обработчика, сообщение придёт снова;
4. отметка `sent_at` — после эффекта.

Пропуск эффекта по одной лишь записи («0 строк → пропусти») теряет его навсегда: запись есть, а
письмо не ушло. Запись с завершённым жизненным циклом повтор эффекта не открывает: ключ — событие
или состояние агрегата. Ключ-событие — `verification.IssuePasswordReset`: свой эффект запроса (письмо
сброса) шлёт подписчик своего события, ссылка — под id запроса из payload (`ON CONFLICT (id)`), а сам
обработчик событий не публикует. Redelivery-кейсы — сбой эффекта, повтор после успеха и повтор после
погашения — покрой юнит-тестом (шаг 7).

### 7. Тесты

- В домене-публикаторе: `usecase_test.go` с фейком `golibinfra/testkit/outboxtest.Recorder` в `UsecaseDeps.Events` и `pgtest.Identity{}` (`golibinfra/testkit/pgtest`) в `Tx` — payload по subject — `Recorder.Last(subject)`, лишних событий нет — `Count()`. Публикация мимо транзакции даёт `outboxkit.ErrNoTx`, как в проде.
- Subscriber: unit-тест на handler-метод с моком `Usecase`-порта (добавь `delivery/nats` в `.mockery.yml` → `make gen`): happy path, error path (usecase вернул err → handler вернул err, сообщение уйдёт на redelivery) и дубль-доставка.
- Контракт (у потребителя, `delivery/nats/contract_test.go`, один тест `TestContracts`): сценарий поставщика — его настоящий usecase публикует событие в `outboxtest.Recorder` (JSON, как его отдаёт outbox) и ставит ожидание usecase-порта потребителя с ID из события; события всех сценариев доставь подпискам `Register` через `natstest`, затем каждая подписка (`bus.Subscriptions()`) должна получить событие поставщика — подписка без него роняет тест. Новая подписка — новый сценарий. Переименование поля или subject на одной стороне роняет его, даже если каждая сторона поправила свои тесты. `delivery/nats` с подпиской без такого теста — отказ archtest `TestSubscriptionsHaveContractTest`. Образец — `internal/domain/verification/delivery/nats/contract_test.go`.
- Источник события в сценарии: поставщик из этого репо — только его usecase; payload, записанный рукой (`outboxtest.Event{…}`) с его subject, archtest пускает лишь в функции с «снять в релизе …» в комментарии (прежняя версия, § Versioning). Поставщик — другой сервис: его usecase в репо нет, сценарий — payload из его контракта (артефакт strategy или файл `testdata/`), источник — в комментарии сценария.
- Поставщик события — домен идентичности (`user`, `auth`, `verification`): контракт-тест потребителя берёт его
  usecase, и в шаблоне и у форка без решения `TestPruneNamesIdentityDependents` требует назвать
  `delivery/nats/contract_test.go` потребителя в `.claude/skills/init-service/references/prune.md` — prune его правит
  (как адаптер — `port.md`); у проекта с `identity: own` prune не нужен, при `identity: external` доменов
  идентичности и их событий нет.
- Integration: если есть end-to-end — отдельный тест под `//go:build integration` с реальным NATS через testcontainers.

### 8. Верификация

```bash
make test
make lint
make gen-check  # если добавил интерфейс из .mockery.yml
```

Готово и коммит — по CLAUDE.md § Готово.

### 9. Коммит

`feat(<source>): emit <event> event` или `feat(<consumer>): handle <event>` — в зависимости от того, что добавил. Если оба в одном коммите — `feat(<source>): emit <event> + <consumer> handler`. В теле — строка `Review:` (CLAUDE.md § Готово).

## Versioning событий

Если меняешь payload-структуру с breaking change:
1. Новая const-subject `"<source>.events.<event>.v2"`; пока публикуются обе версии — второй тип `<Event>V2`.
2. Старый subscriber продолжает работать со старым subject.
3. Когда все consumer'ы перешли — удаляешь публикацию старого.

Поле, которому в payload не место (токен, PII), со старым subject не остаётся: публикация уходит на
`.v2` сразу, внешним потребителям — INFRA-HANDOFF. Свой потребитель тем же изменением читает `.v2`
новым durable, а старый subject ещё один релиз — прежним durable тем же обработчиком (локальный DTO
без убранного поля читает обе версии): так доходят события, записанные в outbox до выкатки, и
недоставленные прежнему консьюмеру. Следующий релиз снимает эту подписку, durable удаляет инфра
(INFRA-HANDOFF). Прежнюю версию поставщик уже не публикует: её подписку в `TestContracts` сверяет сценарий —
функция с записанным payload прежней схемы и «снять в релизе <версия>» в комментарии; его снимают вместе с
подпиской. Не правь существующий тип события без breaking-version-bump.

## Anti-patterns

- ❌ Прямой `nats.Publish()` из usecase в обход outbox — нет транзакционной гарантии.
- ❌ Тяжёлая логика в Subscriber (генерация PDF, batch-обработка). Subscriber обязан быть быстрым.
- ❌ Обработчик события публикует то же событие → зацикливание через outbox. Подписчик своего события (письмо сброса) — usecase, который событий не публикует.
- ❌ Прямой импорт другого бизнес-домена в `delivery/nats/`. Используй локальный DTO (и порт + адаптер, если нужны данные).
- ❌ Идемпотентность «на удачу» — без unique-индекса / статус-гварда повтор доставки задвоит side-effect.
- ❌ Менять формат payload без версионирования.
