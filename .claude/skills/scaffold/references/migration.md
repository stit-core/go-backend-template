# scaffold › migration — миграция схемы БД

**Шаг 0** — прочитай инструментом Read `.claude/rules/sqlc-and-migrations.md` и
`.claude/rules/infra-impact.md`.

Создаёт новую миграцию по конвенции и регенерирует sqlc-код.

## Когда применять

- "Добавь FK от sessions к users c cascade delete"
- "Добавь индекс на email"
- "Добавь колонку `last_login_at` в users"
- "Создай таблицу `audit_log`"

## Когда НЕ применять

- Изменение sqlc-запросов без правки схемы → просто правь queries/*.sql + `make gen`.
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
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    status TEXT NOT NULL,
    total_cents BIGINT NOT NULL,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_orders_user_id ON orders(user_id);
```

Миграция с эффектом для выкатки (разрушительный оператор, backfill, долгий lock, расширение) —
добавь в up строку `-- infra-handoff: <что сделать инфре>` и оформи блок (skill
`infra-handoff`); разрушительный оператор без неё не пропустит migrations-check.

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
-- Данные колонки теряются вместе с ней: восстановление — только из backup.
ALTER TABLE users DROP COLUMN sensitive_data;
```

### 5. Правки запросов (если нужны)

Если миграция меняет схему таблицы которой пользуется sqlc — обнови запросы в `internal/domain/<name>/infra/postgres/queries/`.

Пример: добавили `last_login_at` → возможно нужно обновить `GetUserByID` чтобы возвращал колонку:
```sql
-- name: GetUserByID :one
SELECT id, email, ..., last_login_at, version, created_at, updated_at
FROM users
WHERE id = $1;
```

Или новый запрос:
```sql
-- name: UpdateUserLastLogin :exec
UPDATE users SET last_login_at = NOW() WHERE id = $1;
```

### 6. Регенерация sqlc

```bash
make gen
```

Файлы в `internal/domain/<name>/infra/postgres/db/` обновятся автоматически.

### 7. Проверка компиляции

```bash
make build
```

Если падает — обычно потому что `repository.go` использует старую сигнатуру sqlc-метода. Поправь.

### 8. Тесты

Если миграция семантически меняет поведение — добавь тест.

```bash
make test-int   # юнит + integration + лесенка up → down → up по всем миграциям (Docker)
```

Integration-тесты поднимают Postgres через `golibinfra/testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations()))`, который накатывает миграции приложения — твоя свежая попадёт автоматически (kit-таблица нужна тесту → ещё `testkit.WithMigrations(<kit>pg.Migrations)`; `testkit/pgseed` — только seed-хелперы строк поверх).

### 9. gen-check

`make gen-check` (шаг `make check`) проверяет что sqlc-код совпадает с тем что `make gen` производит. Если падает — `make gen` локально и закоммить дельту.

Готово и коммит — по CLAUDE.md § Готово.

### 10. Коммит

```
feat(<domain>): add NNNN_name migration
```

Kit-таблицы (`audit_log`, `bruteforce_attempts`, `outbox_<domain>`) мигрирует golibinfra
(источники в `cmd/migrate`) — файлов в `migrations/` для них не бывает; общая для нескольких
доменов миграция приложения:
```
feat: add NNNN_name migration
```

## Anti-patterns

- ❌ Править существующую миграцию которая уже накатилась в проде. Только новые.
- ❌ Без `down`-файла или с пустым `down` — migrations-check не пропустит. Миграция только данных —
  `down` из `-- noop` + WHY-комментарий.
- ❌ DDL и DML в одной миграции (например, `ALTER TABLE ... ADD COLUMN` + `UPDATE table SET ...`). Раздели: одна миграция — schema change, следующая — data backfill.
- ❌ Длинные блокирующие миграции на больших таблицах. Используй `CREATE INDEX CONCURRENTLY`, разбивай на батчи.
- ❌ `make gen` забыли — gen-check в `make check` поймает, но локальный flow ломает.
