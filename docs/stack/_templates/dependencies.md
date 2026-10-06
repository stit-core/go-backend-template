---
id: stack.dependencies
kind: dependencies
name: dependencies
status: draft
---

# Dependencies — зависимости и ёмкость

## golibinfra kits
<Какие kit'ы golibinfra подключены и зачем (таблица `| Kit | Зачем |`). В графе `cmd/api` шаблона: telemetry, logkit, pgkit, natskit, httpkit, timerkit, authnkit (с session), bruteforcekit, auditkit, clockkit, outboxkit, healthkit; в коде доменов — ещё bizmetric; полный набор — README golibinfra § Пакеты нужной версии (storagekit, httpclient, …). Версия golibinfra.>

## Что НЕ подключено
<Kit'ы golibinfra вне графа `cmd/api` — их env сервис не читает. Перечисли (напр. cachekit/flagkit/storagekit/httpclient), чтобы не было ложных ожиданий «эта env читается»; кит шаблона, который здесь не нужен, de-template убирает из графа.>

## External runtime deps
<Внешние runtime-зависимости, каждая — инфра-поверхность (INFRA-HANDOFF). Инфраструктурные: Postgres (обяз.), NATS, Redis, S3/MinIO — что реально используется. Внешние backend'ы/интеграции (геокодеры, платёжки, провайдеры): опиши как интеграцию — что/зачем, env-ключи, режим отказа (какой HTTP-код отдаём), retry/timeout, quota/лимиты, как менять провайдера.>

## Capacity
<Ожидания по ёмкости: CPU/RAM, replicas, connection-pool, rate-ожидания. Грубо, для infra-стороны.>

## Generates
<Что de-template родит:
- kit-импорты + fx-провайдеры (только нужные kit'ы — prune ненужных)
- wiring внешних deps (Postgres/S3/NATS/…)
- golibinfra-first: generic util → golibinfra, не сюда (CLAUDE.md § Стиль кода)>
