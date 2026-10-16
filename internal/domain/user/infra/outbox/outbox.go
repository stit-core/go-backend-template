package outbox

import (
	outboxpg "git.stit.tech/stit-core/golibinfra/outboxkit/pgstore"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/events"
)

const Table = "outbox_user"

// Migrations — схема кита: её накатывает cmd/migrate своей таблицей версий schema_migrations_outbox_user.
var Migrations = outboxpg.TableMigrations(Table)

// Providers включает только fx.Module домена user: fx.Private скоупит publisher им.
var Providers = events.OutboxFor("user", Table)
