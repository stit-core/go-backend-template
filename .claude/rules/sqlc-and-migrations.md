---
name: sqlc-and-migrations
description: Правила миграций и работы с sqlc. Грузится при работе с migrations/ или infra/postgres/.
paths:
  - "migrations/**"
  - "internal/domain/*/infra/postgres/**"
  - "sqlc.yaml"
---

# Миграции и sqlc

## Миграции

- Файлы: `migrations/NNNN_name.up.sql` + `migrations/NNNN_name.down.sql`. Парные.
- Нумерация — последовательная, четырёхзначная (`0001`, `0002`, …).
- Имя — snake_case, описывает что делает: `0006_bruteforce_partial_index`.
- Каждая миграция должна быть **обратимой** через `down`. Если откат не имеет смысла (например, удаление колонки с данными) — `down` всё равно есть, минимум `-- noop` с комментарием почему.
- Не правь существующие миграции, которые уже накатились в боевой среде. Только новые.

Команды:
- `make migrate-up` — накатить все недокатанные. Нужна `PG_DSN`.
- `make migrate-down` — откатить последнюю.
- В CI применяются автоматически перед integration-тестами (testcontainers с чистой БД).

## sqlc

Конфиг — `sqlc.yaml`. Каждый домен с PG-инфрой имеет свой блок:

```yaml
- engine: postgresql
  schema: migrations/
  queries: internal/domain/<name>/infra/postgres/queries/
  gen:
    go:
      package: db
      out: internal/domain/<name>/infra/postgres/db
      sql_package: pgx/v5
      ...
```

Все запросы в `internal/domain/<name>/infra/postgres/queries/<file>.sql` — пишешь сам. Сгенерированный код — в `infra/postgres/db/` — **никогда не правится руками**, только регенерация.

### Workflow при изменении схемы

1. Добавь новую миграцию `migrations/NNNN_X.up.sql` + `.down.sql`.
2. Если нужны новые запросы — добавь в `internal/domain/<name>/infra/postgres/queries/`.
3. Запусти `make sqlc` (регенерирует `infra/postgres/db/`).
4. Используй сгенерированный метод в `infra/postgres/repository.go`.
5. `make sqlc-diff` в CI проверит что генерация чиста.

### Формат запроса

```sql
-- name: GetUserByEmail :one
SELECT id, email, password_hash, status, role, version, created_at
FROM users
WHERE email = $1;
```

`-- name: <Method> :<one|many|exec>` — обязательная аннотация для sqlc.

### Когда `:one`, когда `:many`, когда `:exec`

- `:one` — возвращает одну строку. На отсутствие — `pgx.ErrNoRows`. Маппь на свою sentinel-ошибку (`ErrNotFound`).
- `:many` — возвращает срез строк. Пустой результат — пустой срез.
- `:exec` — без возврата (UPDATE/DELETE без RETURNING). Возвращает `pgconn.CommandTag`.
- `:execrows` — `:exec`, но возвращает количество затронутых строк (полезно для optimistic lock).

## PG-конвенции проекта

- Первичные ключи — `uuid` (через `pgx`-расширение или `uuid_generate_v4()`).
- Soft-delete — нет. Удалили — удалили.
- Optimistic lock через `version` колонку: `WHERE id = $1 AND version = $2`. Возврат `:execrows` = 0 → `ErrConflict`.
- Outbox / audit / bruteforce — таблицы **не лежат в `migrations/` этого репо**; схему держат `golibinfra/{outboxkit,auditkit,bruteforcekit}/pgstore` (`MigrationsFS`), `cmd/migrate/main.go` накатывает их рядом с миграциями приложения. Не дублируй CREATE TABLE — наследуй от kit'а.
- Имена таблиц — множественное число, snake_case (`users`, `refresh_sessions`).
- Имена колонок — snake_case (`created_at`, `password_hash`).

## CI-проверки

- `make sqlc-diff` падает если сгенерированный код не совпадает с тем что в репо.
- Integration-тесты используют `internal/testkit/pgseed` для сидинга чистой БД.
