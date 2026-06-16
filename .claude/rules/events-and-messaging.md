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
- Subject — литерал `<domain>.events.<event>`. Источник его **не экспортит**; consumer
  заводит локальную константу. Breaking-change payload → новый subject (`….v2`).
- payload-типы — в `events.go` домена-источника.

## Subscribers (in-domain)

- Durable JetStream-консьюмеры в `<consumer>/delivery/nats/` рядом с `delivery/http/` —
  равноправные входы в usecase: `natskit.SubscribeStreamJSON(bus, ctx, <stream>, <subject>,
  <durable>, handler)`, `bus` — `natskit.StreamBus`; `ErrJetStreamDisabled` → лог+skip
  (local-dev без `-js`).
- Объявляют **локальный** payload-DTO (`delivery/nats/dto.go`) — прямой импорт домена-
  источника запрещён depguard. Регистрируются в `module/module.go` потребителя.
- **Идемпотентность обязательна** (at-least-once): `ON CONFLICT DO NOTHING`, `UPDATE … WHERE
  status='pending'`, idempotency-key. Не «вряд ли задвоится». Poison consumer'а —
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
