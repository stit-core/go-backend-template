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
<at-least-once, idempotency-стратегия, ordering (single-instance outbox-drain), versioning
subject'ов при breaking-changes.>

## Generates
<Что de-template родит:
- subject-константы у consumer'ов (`delivery/nats/dto.go`)
- регистрация подписчиков в fx
- outbox-publisher wiring в usecase-deps>
