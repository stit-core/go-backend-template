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
  Парность+непрерывность enforce'ит `scripts/tools/migrations-check.sh` (шаг `make check`); он
  же не пускает пустой `down` и разрушительный `up` без маркера (ниже).
- Каждая миграция **обратима** через `down`: он возвращает схему к состоянию до `up` (данные —
  нет: drop колонки откатывается её пересозданием). Держит лесенка up → down → up по каждой
  версии в `make check` (`cmd/migrate/migrate_test.go`). Миграция только данных схему не
  меняет — её `down` может быть `-- noop` + WHY-комментарий.
- Разрушительный `up` (`DROP`, `TRUNCATE`, `ALTER … TYPE`, `RENAME`) — только со строкой
  `-- infra-handoff: <что сделать инфре>` в файле и INFRA-HANDOFF-блоком (rule `infra-impact`):
  выкатке нужен порядок или окно.
- **Не правь уже накатившуюся в боевой среде миграцию.** Только новая.
- `make migrate-up` / `make migrate-down` (нужна `PG_DSN`); `down` откатывает одну миграцию
  приложения, схему китов ведёт golibinfra. Integration-тесты (`make test-int`, шаг
  `make check`) накатывают миграции на чистую БД в testcontainers.

## sqlc

- Запросы пишешь сам в `internal/domain/<name>/infra/postgres/queries/<file>.sql`.
  Сгенерированный `infra/postgres/db/` — **никогда руками**, только `make gen`.
- Аннотация обязательна: `-- name: <Method> :<one|many|exec|execrows>`.
  - `:one` — одна строка; отсутствие → `pgx.ErrNoRows`, маппь на sentinel (`ErrNotFound`).
  - `:many` — срез; пусто → пустой срез.
  - `:exec` — без возврата; `:execrows` — число затронутых строк (optimistic-lock, idempotent insert).
- Создание — `Insert<Entity>` (repo-метод и sqlc-name), не `Create*`.
- Repository мапит ошибки: no-rows (`pgkit.IsNoRows`) → sentinel not-found домена; невалидный
  UUID в **lookup-ключе** → тот же sentinel not-found (не отличим снаружи); невалидный UUID в
  **write-параметре** → wrapped `fmt.Errorf` (программная ошибка). Row→entity-маппер при ≥2
  использованиях — функция `rowToEntity`, не копипаста литералов.
- После правки `queries/` или `migrations/` — `make gen`; `make gen-check` (в `make check`)
  падает, если сгенерированное разошлось (включая untracked-моки).

## PG-конвенции

- PK — `uuid`.
- **Delete-политика — решение домена, фиксируется явно.** user-домен — status-based
  soft-delete (`status='deleted'` + partial unique index `WHERE status != 'deleted'`):
  сохраняет audit-историю и освобождает email. Домен без такой нужды — hard delete.
  Теневые флаги `deleted_at` без политики — нет.
- Optimistic lock через `version`: `WHERE id=$1 AND version=$2 … RETURNING` (`:one`;
  no-rows → `ErrConcurrentUpdate`) либо `:execrows`=0 → `ErrConcurrentUpdate`.
- **Kit-таблицы** (`outbox_<domain>`/`audit_log`/`bruteforce_attempts`) схему держат
  `golibinfra/{outboxkit,auditkit,bruteforcekit}/pgstore` (`Migrations`,
  `TableMigrations(table)`), `cmd/migrate` накатывает рядом — таблица версий у каждого
  источника своя (`schema_migrations_<source>`, приложение — `schema_migrations_app`).
  **Не дублируй их `CREATE TABLE`** в `migrations/`.
- Имена: таблицы — множественное snake_case (`users`, `refresh_sessions`); колонки — snake_case.
