package outbox

import "git.stit.tech/stit-core/go-backend-template/internal/platform/events"

// Table — per-domain outbox-таблица домена verification (migration 0003_outbox_domains).
const Table = "outbox_verification"

// Providers — per-domain outbox-инстанс (см. events.OutboxFor). Включается в verification
// fx.Module, чтобы fx.Private заскоупился доменным модулем.
var Providers = events.OutboxFor("verification", Table)
