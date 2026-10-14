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
  Парность+непрерывность enforce'ит `scripts/tools/migrations-check.sh` (шаг `make check`): файл
  или каталог вне маски — отказ; он же не пускает пустой `down` и `up` с эффектом без маркера (ниже).
- Каждая миграция обратима через `down`: он возвращает схему к состоянию до `up` (данные —
  нет: drop колонки откатывается её пересозданием). Держит лесенка `testkit.MigrationLadder` по
  источникам `cmd/migrate` в `make check` (`cmd/migrate/migrate_test.go`): up → down → up по каждой
  версии и откат источника до нуля со сверкой схемы. Миграция только данных схему не
  меняет — её `down` может быть `-- noop` + WHY-комментарий.
- Миграция с эффектом для выкатки — разрушительная (`DROP`, `TRUNCATE`, `ALTER … TYPE`, `RENAME`),
  правка данных (`DELETE`, `UPDATE … SET`, backfill, код `DO`, `CALL`, `SELECT` верхнего уровня), долгий
  lock (`SET NOT NULL`, `CREATE INDEX` без `CONCURRENTLY` по существующей таблице), новое расширение
  (`CREATE EXTENSION`) — только со строкой `-- infra-handoff: <что сделать инфре>` в `up` и
  INFRA-HANDOFF-блоком (rule `infra-impact`): выкатке нужен порядок или окно. Эти операторы без строки не
  пропустит migrations-check (индекс по таблице того же файла и тело объявленной функции — не эффект;
  выпущенные `0001` и `0003` — списком в скрипте); прочее с эффектом (`INSERT … SELECT`, `ADD CONSTRAINT`
  без `NOT VALID`) держат правило и ревью. По строке миграцию видит `make status`.
- **Не правь уже накатившуюся в боевой среде миграцию.** Только новая.
- `make migrate-up` / `make migrate-down` (нужна `PG_DSN`); `down` откатывает одну миграцию
  приложения, схему китов ведёт golibinfra. Integration-тесты (`make test-int`, шаг
  `make check`) накатывают миграции на чистую БД в testcontainers.

## sqlc

- Запросы пишешь сам в `internal/domain/<name>/infra/postgres/queries/<file>.sql`.
  Сгенерированный `infra/postgres/db/` — никогда руками, только `make gen`.
- Аннотация обязательна: `-- name: <Method> :<one|many|exec|execrows>`.
  - `:one` — одна строка; отсутствие → `pgx.ErrNoRows`, маппь на sentinel (`ErrNotFound`).
  - `:many` — срез; пусто → пустой срез.
  - `:exec` — без возврата; `:execrows` — число затронутых строк (optimistic-lock, погашение).
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
  сохраняет audit-историю и освобождает email; удаление — `UPDATE … WHERE id = $1 AND status !=
  'deleted'` без версии (`:execrows`, 0 строк → `ErrNotFound`): повтор и гонка — не конфликт, а
  запросы на запись удалённого не трогают. Домен без такой нужды — hard delete.
  Теневые флаги `deleted_at` без политики — нет.
- Optimistic lock через `version`: `WHERE id=$1 AND version=$2 … RETURNING` (`:one`;
  no-rows → `ErrConcurrentUpdate`) либо `:execrows`=0 → `ErrConcurrentUpdate`.
- Одноразовое погашение (токен, инвайт, купон) — условный `UPDATE … SET used_at = NOW() WHERE
  id = $1 AND used_at IS NULL` (`:execrows`, 0 строк → sentinel «уже использован») первым в
  транзакции эффекта: чтение `used_at` и безусловный `UPDATE` пропускают параллельный повтор.
  Канон — `MarkPasswordResetUsed` домена `verification`.
- **Kit-таблицы** (`outbox_<domain>`/`audit_log`/`bruteforce_attempts`/`authn_sessions`) схему держат
  `golibinfra/{outboxkit,auditkit,bruteforcekit,authnkit/session}/pgstore` (`Migrations`,
  `TableMigrations(table)`), `cmd/migrate` накатывает рядом — таблица версий у каждого
  источника своя (`schema_migrations_<source>`, приложение — `schema_migrations_app`).
  **Не дублируй их `CREATE TABLE`** в `migrations/`.
- Имена: таблицы — множественное snake_case (`users`, `email_verifications`); колонки — snake_case.
