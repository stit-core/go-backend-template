// Package sources — источники миграций сервиса: один список на cmd/migrate и тесты, которые
// поднимают БД как прод.
package sources

import (
	auditpg "git.stit.tech/stit-core/golibinfra/auditkit/pgstore"
	bruteforcepg "git.stit.tech/stit-core/golibinfra/bruteforcekit/pgstore"
	"git.stit.tech/stit-core/golibinfra/pgkit"

	useroutbox "git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/outbox"
	verifoutbox "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/outbox"
)

// Kits — kit-owned таблицы: киты golibinfra и outbox-таблицы доменов, у каждого своя таблица
// версий schema_migrations_<source>. Накатываются раньше каталога приложения: его миграции
// могут опираться на их таблицы.
var Kits = []pgkit.Source{
	bruteforcepg.Migrations,
	auditpg.Migrations,
	useroutbox.Migrations,
	verifoutbox.Migrations,
}
