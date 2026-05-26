---
id: stack.dependencies
kind: dependencies
name: dependencies
status: draft
---

# Dependencies — зависимости и ёмкость

## golibinfra kits
<Какие kit'ы golibinfra подключены и зачем: telemetry, httpkit, pgkit, authnkit,
outboxkit, natskit, bruteforcekit, auditkit, cachekit, clockkit, logkit. Версия golibinfra.>

## External runtime deps
<Внешние runtime-зависимости: Postgres (обяз.), NATS, Redis, S3/MinIO — что реально
используется. Каждая — это инфра-поверхность (INFRA-HANDOFF).>

## Capacity
<Ожидания по ёмкости: CPU/RAM, replicas, connection-pool, rate-ожидания. Грубо, для
infra-стороны.>

## Generates
<Что de-template родит:
- kit-импорты + fx-провайдеры (только нужные kit'ы — prune ненужных)
- wiring внешних deps (Redis/NATS/…)
- golibinfra-first: generic util → golibinfra, не сюда (CLAUDE.md § Стиль кода)>
