---
name: add-migration
description: Добавить миграцию схемы БД с парой up/down + регенерация sqlc. Активируй когда пользователь просит "добавь миграцию", "измени схему", "добавь колонку/индекс/таблицу".
---

# Skill: add-migration

Создаёт новую миграцию по конвенции (см. `.claude/rules/sqlc-and-migrations.md`) и регенерирует sqlc-код.

## Когда применять

- "Добавь FK от sessions к users c cascade delete"
- "Добавь индекс на email"
- "Добавь колонку `last_login_at` в users"
- "Создай таблицу `audit_log`"

## Когда НЕ применять

- Изменение sqlc-запросов без правки схемы → просто правь queries/*.sql + `make sqlc`.
- Откат уже применённой миграции в проде → опасно, обсуди с пользователем.

## Шаги

### 1. Понять номер

Найди максимальный номер в `migrations/`:
```bash
ls migrations/ | grep -oE '^[0-9]+' | sort -un | tail -1
```

Новая миграция — `NNNN+1`, четырёхзначная (`0007`, `0008`, …).

### 2. Имя

Snake_case описывающий что делает:
- `0007_users_last_login_at` — добавить колонку
- `0008_sessions_users_fk` — FK
- `0009_orders` — новая таблица
- `0010_email_index` — индекс

### 3. Up-миграция

`migrations/NNNN_name.up.sql`:

Примеры:
```sql
-- Добавить колонку
ALTER TABLE users ADD COLUMN last_login_at TIMESTAMPTZ;
CREATE INDEX idx_users_last_login_at ON users(last_login_at);

-- FK
ALTER TABLE sessions
    ADD CONSTRAINT sessions_user_id_fk
    FOREIGN KEY (user_id) REFERENCES users(id)
    ON DELETE CASCADE;

-- Новая таблица
CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    status TEXT NOT NULL,
    total_cents BIGINT NOT NULL,
    version INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_orders_user_id ON orders(user_id);
```

### 4. Down-миграция

`migrations/NNNN_name.down.sql` — обратимая.

```sql
-- Откат колонки
DROP INDEX IF EXISTS idx_users_last_login_at;
ALTER TABLE users DROP COLUMN IF EXISTS last_login_at;

-- Откат FK
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_user_id_fk;

-- Откат таблицы
DROP INDEX IF EXISTS idx_orders_user_id;
DROP TABLE IF EXISTS orders;
```

Если откат с потерей данных — добавь комментарий:
```sql
-- noop: данные удаляются вместе с колонкой; восстановление требует backup-restore
ALTER TABLE users DROP COLUMN sensitive_data;
```

### 5. Правки запросов (если нужны)

Если миграция меняет схему таблицы которой пользуется sqlc — обнови запросы в `internal/domain/<name>/infra/postgres/queries/`.

Пример: добавили `last_login_at` → возможно нужно обновить `GetByID` чтобы возвращал колонку:
```sql
-- name: GetByID :one
SELECT id, email, ..., last_login_at, version, created_at, updated_at
FROM users
WHERE id = $1;
```

Или новый запрос:
```sql
-- name: UpdateLastLogin :exec
UPDATE users SET last_login_at = NOW() WHERE id = $1;
```

### 6. Регенерация sqlc

```bash
make sqlc
```

Файлы в `internal/domain/<name>/infra/postgres/db/` обновятся автоматически.

### 7. Проверка компиляции

```bash
scripts/agent-tools/build.sh
```

Если падает — обычно потому что `repository.go` использует старую сигнатуру sqlc-метода. Поправь.

### 8. Тесты

Если миграция семантически меняет поведение — добавь тест.

```bash
scripts/agent-tools/test.sh
```

Integration-тесты используют `testkit/pgseed`, который накатывает все миграции — твоя свежая попадёт автоматически.

### 9. CI-проверка

`make sqlc-diff` проверяет что sqlc-код совпадает с тем что `make sqlc` производит. Если падает — `make sqlc` локально и закоммить дельту.

### 10. Коммит

```
feat(<domain>): add NNNN_name migration
```

Или для общих миграций (audit, outbox):
```
feat: add NNNN_name migration
```

## Anti-patterns

- ❌ Править существующую миграцию которая уже накатилась в проде. Только новые.
- ❌ Без `down`-файла — даже если откат бессмысленен, добавь stub.
- ❌ DDL и DML в одной миграции (например, `ALTER TABLE ... ADD COLUMN` + `UPDATE table SET ...`). Раздели: одна миграция — schema change, следующая — data backfill.
- ❌ Длинные блокирующие миграции на больших таблицах. Используй `CREATE INDEX CONCURRENTLY`, разбивай на батчи.
- ❌ `make sqlc` забыли — sqlc-diff в CI поймает, но локальный flow ломает.
