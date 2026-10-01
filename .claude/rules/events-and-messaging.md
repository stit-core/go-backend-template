---
name: events-and-messaging
description: Outbox-события, in-domain NATS-subscribers и background jobs — единый набор инвариантов. Отдельного worker-процесса нет. Terse; «как и почему» — docs/reference/architecture.md § События и messaging. Грузится при правке events.go, delivery/nats/ или infra/outbox/.
paths:
  - "internal/domain/*/events.go"
  - "internal/domain/*/delivery/nats/**"
  - "internal/domain/*/infra/outbox/**"
---

# События и messaging

Полное описание — `docs/reference/architecture.md § События и messaging`. Здесь — инварианты.

## Outbox (публикация, per-domain)

- Событие пишется в БД в одной транзакции с изменением:
  `tx.WithTx(ctx, func(txCtx){ repo.X(txCtx); events.Publish(txCtx, subject, payload) })`.
  Никакой прямой публикации в JetStream из usecase в обход outbox (нет транзакционной гарантии).
- Каждый домен-публикатор — свой transactional-outbox: `internal/domain/<d>/infra/outbox/outbox.go`
  с `const Table = "outbox_<d>"`, `var Migrations = outboxpg.TableMigrations(Table)` и
  `var Providers = events.OutboxFor("<d>", Table)` (`outboxkit.Instance`: store, drain,
  партиции, retention, gauge'и); `Providers` включается в `<d>/module/module.go` (fx.Private
  скоупит publisher доменом). Схема таблицы — kit-owned: `Migrations` добавляется в список
  источников `cmd/migrate` (своя таблица версий `schema_migrations_outbox_<d>`), в
  `migrations/` не дублируется.
- Drain публикует в JetStream (`natskit.StreamBus`, PubAck + дедуп по `Nats-Msg-Id`). Нужны
  `NATS_JETSTREAM_ENABLED=true` + стрим, покрывающий subject (`NATS_JS_STREAM_NAMES`).
- Subject `<domain>.events.<event>` — **unexported const `subject<Event>`** в `events.go`
  источника, рядом с payload (не inline-литерал в usecase, не экспорт). Consumer заводит
  свою const в `delivery/nats/`. Breaking-change payload → новый subject (`….v2`).
- payload-типы — в `events.go` источника: `<Aggregate><PastEvent>`, поля **без json-тегов**
  (wire = Go-имена; смена формата = `.v2`), timestamp — `At time.Time`.

## Subscribers (in-domain)

- Durable JetStream-консьюмеры в `<consumer>/delivery/nats/` рядом с `delivery/http/` —
  равноправные входы в usecase: `natskit.SubscribeOnStart(lc, func(ctx) { return
  natskit.SubscribeStreamJSON(bus, ctx, <stream>, <subject>, <durable>, handler) })`, `bus` —
  `natskit.StreamBus`. Кит подписывает в OnStart и дренит подписку в OnStop раньше остановки
  её зависимостей; ошибка подписки, в том числе выключенный JetStream, валит старт.
- Объявляют **локальный** payload-DTO (`delivery/nats/dto.go`) — прямой импорт домена-
  источника запрещён archtest. Регистрируются в `module/module.go` потребителя.
- **Идемпотентность обязательна** (at-least-once), и она — свойство **схемы, не удачи**:
  unique-индекс под natural key (`INSERT … ON CONFLICT`), либо `UPDATE … WHERE
  status='pending'`, либо idempotency-key из payload. **Внешний эффект** (письмо, вебхук) схема
  помнит отдельно от записи: отметка «выполнен» (`sent_at`) — после эффекта; повтор доставки
  выполненный эффект пропускает, невыполненный повторяет, сбой эффекта — ошибка обработчика
  (redelivery). Пропуск по одной лишь записи теряет эффект: запись есть, письмо не ушло. Канон —
  `verification.IssueEmailVerification`: upsert по партиал-индексу `email_verifications(user_id)
  WHERE used_at IS NULL` перевыпускает токен неотправленного письма, отправленное не трогает.
  Redelivery (сбой эффекта, повтор после успеха) покрыт юнит-тестом (rule tests-pipeline).
  Poison consumer'а — `NATS_JS_MAX_DELIVER`→DLQ/Term (natskit).
- Subscriber — быстрый. Тяжёлое (PDF, batch на 1M) → отдельный job/usecase.

## Background jobs (timerkit)

- Периодика без события-триггера — `timerkit.Job`, регистрируется `timerkit.Provide(ctor)`
  в модуле-владельце (fx-группа `timerkit.Group`); гоняет `timerkit.Module` в `cmd/api`,
  отдельного worker-бинаря нет. Спан `timerkit.job <name>`, метрики `timer_job_*`.
- Обслуживание kit-таблиц пишут сами киты: cleanup `bruteforce_attempts` — `bruteforcepg.Module`,
  drain/партиции/retention outbox — `outboxkit.Instance`. Свой job поверх kit-таблицы не заводить.
- `time.Now()` → инжектить `clockkit.Clock`. Ошибка `Run` → лог кита, цикл не прерывается;
  процесс не ронять.

## Kit-owned таблицы

`audit_log` / `bruteforce_attempts` / `outbox_<d>` — схему держит golibinfra-kit
(`auditpg.Migrations`, `bruteforcepg.Migrations`, `outboxpg.TableMigrations(table)`), не
`migrations/` этого репо: не дублируй `CREATE TABLE`. Источники перечислены в `cmd/migrate`,
у каждого своя таблица версий `schema_migrations_<source>` (приложение — `schema_migrations_app`).
