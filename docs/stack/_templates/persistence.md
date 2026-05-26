---
id: stack.persistence
kind: persistence
name: persistence
status: draft
---

# Persistence — хранилище

## Engine
<Движок и драйвер (Postgres + pgx/v5), sqlc для типизированных запросов. PK-конвенции
(uuid), naming (snake_case, мн. число), optimistic-lock через `version`.>

## Migration strategy
<golang-migrate, парные up/down, последовательная нумерация. `cmd/migrate` как
init-контейнер. Где живут SQL.>

## Kit-owned tables
<Таблицы, схему которых держит golibinfra-kit, а не `migrations/` этого репо:
`outbox` (outboxkit), `audit_log` (auditkit), `bruteforce_*` (bruteforcekit). Не
дублировать CREATE TABLE — наследовать от kit'а через MigrationsFS.>

## Connection pool
<Ожидания по пулу: размер, таймауты acquire, метрики `db_pool_*`.>

## Generates
<Что de-template родит:
- pgkit-wiring (pool, transactor) в fx
- база `sqlc.yaml`
- `cmd/migrate` + порядок накатки app+kit миграций>
