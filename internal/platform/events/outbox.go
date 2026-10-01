package events

import (
	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/outboxkit"
	outboxpg "git.stit.tech/stit-core/golibinfra/outboxkit/pgstore"
)

// OutboxFor — per-domain outbox на таблице table (outboxkit.Instance) и его Publisher без fx-тега:
// UsecaseDeps имени инстанса не знает. fx.Options, а не fx.Module — fx.Private скоупит publisher
// модулем домена, в который его включают.
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
	"internal/platform/events",
	outboxpg.FactoryModule,
)
