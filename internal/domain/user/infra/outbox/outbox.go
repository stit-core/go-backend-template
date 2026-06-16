package outbox

import "git.stit.tech/stit-core/go-backend-template/internal/platform/events"

// Table — per-domain outbox-таблица домена user (migration 0003_outbox_domains).
const Table = "outbox_user"

// Providers — per-domain outbox-инстанс (см. events.OutboxFor). Включается в user
// fx.Module, чтобы fx.Private заскоупился доменным модулем.
var Providers = events.OutboxFor("user", Table)
