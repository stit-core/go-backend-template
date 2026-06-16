---
id: stack.messaging
kind: messaging
name: messaging
status: draft
---

# Messaging — события и шина

## Published subjects
<Subjects, которые сервис публикует (через outbox): `<domain>.events.<event>`. Кто
потенциальные consumer'ы (свои домены / другие сервисы). Cross-service — это контракт.>

## Consumed subjects
<Subjects, которые сервис слушает (in-domain `delivery/nats/`): subject, домен-обработчик,
требование идемпотентности.>

## Delivery semantics
<at-least-once (JetStream PubAck + дедуп по `Nats-Msg-Id`), idempotency-стратегия, ordering
(per-domain outbox-drain; `OUTBOX_PUBLISH_MODE` batch|ordered), poison-handling
(`OUTBOX_MAX_ATTEMPTS`→dead; consumer `NATS_JS_MAX_DELIVER`→DLQ), versioning subject'ов при
breaking-changes.>

## Generates
<Что de-template родит:
- subject-константы и stream/durable у consumer'ов (`delivery/nats/dto.go`)
- регистрация durable JetStream-подписчиков в fx
- per-domain outbox wiring: `infra/outbox/outbox.go` (`OutboxFor`) + таблица `outbox_<domain>`
  в миграции + `NATS_JS_STREAM_NAMES` под subject-префиксы>
