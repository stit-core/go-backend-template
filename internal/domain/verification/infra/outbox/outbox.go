package outbox

import (
	outboxpg "git.stit.tech/stit-core/golibinfra/outboxkit/pgstore"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/events"
)

// Table — per-domain outbox-таблица домена verification.
const Table = "outbox_verification"

// Migrations — схема таблицы (kit-owned DDL outboxkit/pgstore, своя таблица
// версий schema_migrations_outbox_verification); накатывает cmd/migrate.
var Migrations = outboxpg.TableMigrations(Table)

// Providers — per-domain outbox-инстанс (см. events.OutboxFor). Включается в verification
// fx.Module, чтобы fx.Private заскоупился доменным модулем.
var Providers = events.OutboxFor("verification", Table)
