---
name: events-and-messaging
description: Outbox-события, in-domain NATS-subscribers и background jobs — единый набор инвариантов. Отдельного worker-процесса нет. Terse; «как и почему» — docs/reference/architecture.md § События и messaging. Грузится при правке events.go, delivery/nats/ или internal/jobs/.
paths:
  - "internal/domain/*/events.go"
  - "internal/domain/*/delivery/nats/**"
  - "internal/jobs/**"
---

# События и messaging

Полное описание — `docs/reference/architecture.md § События и messaging`. Здесь — инварианты.

## Outbox (публикация, per-domain)

- Событие пишется в БД в одной транзакции с изменением:
  `tx.WithTx(ctx, func(txCtx){ repo.X(txCtx); events.Publish(txCtx, subject, payload) })`.
  Никакой прямой публикации в JetStream из usecase в обход outbox (нет транзакционной гарантии).
- Каждый домен-публикатор — свой transactional-outbox: `internal/domain/<d>/infra/outbox/outbox.go`
  с `const Table = "outbox_<d>"` + `var Providers = events.OutboxFor("<d>", Table)`; `Providers`
  включается в `<d>/module/module.go` (fx.Private скоупит publisher доменом). Таблицу
  `outbox_<d>` заводит миграция этого репо (DDL зеркалит `outboxkit/pgstore.SchemaSQL`).
- Drain публикует в JetStream (`natskit.StreamBus`, PubAck + дедуп по `Nats-Msg-Id`). Нужны
  `NATS_JETSTREAM_ENABLED=true` + стрим, покрывающий subject (`NATS_JS_STREAM_NAMES`).
- Subject `<domain>.events.<event>` — **unexported const `subject<Event>`** в `events.go`
  источника, рядом с payload (не inline-литерал в usecase, не экспорт). Consumer заводит
  свою const в `delivery/nats/`. Breaking-change payload → новый subject (`….v2`).
- payload-типы — в `events.go` источника: `<Aggregate><PastEvent>`, поля **без json-тегов**
  (wire = Go-имена; смена формата = `.v2`), timestamp — `At time.Time`.

## Subscribers (in-domain)

- Durable JetStream-консьюмеры в `<consumer>/delivery/nats/` рядом с `delivery/http/` —
  равноправные входы в usecase: `natskit.SubscribeStreamJSON(bus, ctx, <stream>, <subject>,
  <durable>, handler)`, `bus` — `natskit.StreamBus`; `ErrJetStreamDisabled` → лог+skip
  (local-dev без `-js`).
- Объявляют **локальный** payload-DTO (`delivery/nats/dto.go`) — прямой импорт домена-
  источника запрещён depguard. Регистрируются в `module/module.go` потребителя.
- **Идемпотентность обязательна** (at-least-once), и она — свойство **схемы, не удачи**:
  unique-индекс под natural key + `ON CONFLICT DO NOTHING` (`:execrows`; 0 rows → skip
  side-effects: письмо/событие не повторяются), либо `UPDATE … WHERE status='pending'`,
  либо idempotency-key из payload. Канон — `verification.IssueEmailVerification` +
  партиал-индекс `email_verifications(user_id) WHERE used_at IS NULL`. Redelivery-кейс
  покрыт юнит-тестом (rule tests-pipeline). Poison consumer'а —
  `NATS_JS_MAX_DELIVER`→DLQ/Term (natskit).
- Subscriber — быстрый. Тяжёлое (PDF, batch на 1M) → отдельный job/usecase.

## Background jobs (`internal/jobs/`)

- Периодика без события-триггера. Поднимается той же fx.Lifecycle (`cmd/api`), отдельного
  worker-бинаря нет.
- Чистит таблицу kit'а → зовёт метод kit-интерфейса (`Guard.Cleanup`), не сырой SQL.
- `time.Now()` → инжектить `clockkit.Clock`. Ошибка job → лог, не ронять процесс.

## Kit-owned таблицы

`audit_log` / `bruteforce_*` / default `outbox` — схему держит golibinfra-kit (`MigrationsFS`),
не `migrations/` этого репо: не дублируй `CREATE TABLE`. Per-domain `outbox_<d>` — наоборот:
заводит миграция этого репо (зеркаля kit-DDL `outboxkit/pgstore.SchemaSQL`).
