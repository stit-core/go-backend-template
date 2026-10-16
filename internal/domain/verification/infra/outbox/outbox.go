package outbox

import (
	outboxpg "git.stit.tech/stit-core/golibinfra/outboxkit/pgstore"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/events"
)

const Table = "outbox_verification"

// Migrations — схема кита: её накатывает cmd/migrate своей таблицей версий schema_migrations_outbox_verification.
var Migrations = outboxpg.TableMigrations(Table)

// Providers включает только fx.Module домена verification: fx.Private скоупит publisher им.
var Providers = events.OutboxFor("verification", Table)
