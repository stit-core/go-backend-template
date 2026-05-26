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

## Outbox (публикация)

- Событие пишется в БД в одной транзакции с изменением:
  `tx.WithTx(ctx, func(txCtx){ repo.X(txCtx); events.Publish(txCtx, subject, payload) })`.
  Никакой прямой публикации в NATS из usecase в обход outbox (нет транзакционной гарантии).
- Subject — литерал `<domain>.events.<event>`. Источник его **не экспортит**; consumer
  заводит локальную константу. Breaking-change payload → новый subject (`…​.v2`).
- payload-типы — в `events.go` домена-источника.

## Subscribers (in-domain)

- Лежат в `<consumer>/delivery/nats/` рядом с `delivery/http/` — равноправные входы в usecase.
- Объявляют **локальный** payload-DTO (`delivery/nats/dto.go`) — прямой импорт домена-
  источника запрещён depguard. Регистрируются в `module/module.go` потребителя.
- **Идемпотентность обязательна** (at-least-once): `ON CONFLICT DO NOTHING`, `UPDATE … WHERE
  status='pending'`, idempotency-key. Не «вряд ли задвоится».
- Subscriber — быстрый. Тяжёлое (PDF, batch на 1M) → отдельный job/usecase.

## Background jobs (`internal/jobs/`)

- Периодика без события-триггера. Поднимается той же fx.Lifecycle (`cmd/api`), отдельного
  worker-бинаря нет.
- Чистит таблицу kit'а → зовёт метод kit-интерфейса (`Guard.Cleanup`), не сырой SQL.
- `time.Now()` → инжектить `clockkit.Clock`. Ошибка job → лог, не ронять процесс.

## Kit-owned таблицы

`outbox` / `audit_log` / `bruteforce_*` — схему держит golibinfra-kit (`MigrationsFS`),
не `migrations/` этого репо. Не дублируй `CREATE TABLE`.
