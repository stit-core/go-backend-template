---
name: sqlc-and-migrations
description: Инварианты миграций и sqlc. Грузится при работе с migrations/ или infra/postgres/. Процедура добавления — skill scaffold (вариант migration/domain).
paths:
  - "migrations/**"
  - "internal/domain/*/infra/postgres/**"
  - "sqlc.yaml"
---

# Миграции и sqlc

Процедура (создать миграцию, обновить sqlc.yaml, регенерация) — skill `scaffold`
(`references/migration.md`, `references/domain.md`). Здесь — инварианты, которые обязан
соблюдать при правке.

## Миграции

- Файлы парные: `migrations/NNNN_name.up.sql` + `NNNN_name.down.sql`. Нумерация —
  последовательная четырёхзначная (`0001`…), имя snake_case (`0006_bruteforce_partial_index`).
- Каждая миграция **обратима** через `down`. Откат бессмыслен (drop колонки с данными) —
  `down` всё равно есть: минимум `-- noop` + WHY-комментарий.
- **Не правь уже накатившуюся в боевой среде миграцию.** Только новая.
- `make migrate-up` / `make migrate-down` (нужна `PG_DSN`). В CI — автоматически перед
  integration-тестами (testcontainers с чистой БД).

## sqlc

- Запросы пишешь сам в `internal/domain/<name>/infra/postgres/queries/<file>.sql`.
  Сгенерированный `infra/postgres/db/` — **никогда руками**, только `make gen`.
- Аннотация обязательна: `-- name: <Method> :<one|many|exec|execrows>`.
  - `:one` — одна строка; отсутствие → `pgx.ErrNoRows`, маппь на sentinel (`ErrNotFound`).
  - `:many` — срез; пусто → пустой срез.
  - `:exec` — без возврата; `:execrows` — число затронутых строк (optimistic-lock).
- После правки `queries/` или `migrations/` — `make gen`; `make gen-check` (в `make check`)
  падает, если сгенерированное разошлось.

## PG-конвенции

- PK — `uuid`. Soft-delete нет: удалили — удалили.
- Optimistic lock через `version`: `WHERE id=$1 AND version=$2`; `:execrows`=0 → `ErrConflict`.
- **Kit-таблицы** (`outbox`/`audit_log`/`bruteforce_*`) схему держат
  `golibinfra/{outboxkit,auditkit,bruteforcekit}/pgstore` (`MigrationsFS`), `cmd/migrate`
  накатывает рядом. **Не дублируй их `CREATE TABLE`** в `migrations/`.
- Имена: таблицы — множественное snake_case (`users`, `refresh_sessions`); колонки — snake_case.
