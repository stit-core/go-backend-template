package events

import (
	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/outboxkit"
	outboxpg "git.stit.tech/stit-core/golibinfra/outboxkit/pgstore"
)

// OutboxFor разводит per-domain transactional-outbox: outboxkit.Instance поверх
// таблицы table (store из фабрики pgstore, транспорт с namespace'ным Nats-Msg-Id,
// drain- и maintenance-job'ы в timerkit.Group, backlog-gauge'и) плюс приватный
// алиас его Publisher'а без fx-тега — UsecaseDeps домена имени инстанса не
// знает. Возвращает fx.Options (НЕ fx.Module): fx.Private обязан заскоупиться
// доменным модулем, в который это включают. Схему таблицы даёт
// pgstore.TableMigrations(table), накатывает её cmd/migrate.
func OutboxFor(name, table string) fx.Option {
	return fx.Options(
		outboxkit.Instance(name, table),
		fx.Provide(
			fx.Private,
			fx.Annotate(
				func(p outboxkit.Publisher) outboxkit.Publisher { return p },
				fx.ParamTags(`name:"`+name+`"`),
			),
		),
	)
}

// Module — общий слой per-domain outbox'ов: фабрика pgstore-хранилищ на
// произвольные таблицы; tx-guard (OUTBOX_REQUIRE_TX) и retention у всех
// инстансов общие.
var Module = fx.Module(
	"platform.events",
	outboxpg.FactoryModule,
)
