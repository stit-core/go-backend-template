package events

import (
	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/outboxkit"
	outboxpg "git.stit.tech/stit-core/golibinfra/outboxkit/pgstore"
)

// OutboxFor — Publisher без fx-тега (UsecaseDeps имени инстанса не знает) на subject'ы `<name>.>`: их накрывает стрим
// JetStream, иначе старт падает; fx.Options, а не fx.Module: fx.Private скоупит publisher модулем домена.
func OutboxFor(name, table string) fx.Option {
	return fx.Options(
		outboxkit.Instance(name, table, outboxkit.WithSubjects(name+".>")),
		fx.Provide(
			fx.Private,
			fx.Annotate(
				func(p outboxkit.Publisher) outboxkit.Publisher { return p },
				fx.ParamTags(`name:"`+name+`"`),
			),
		),
	)
}

// Module — общее per-domain outbox'ов: фабрика хранилищ pgstore, tx-guard (OUTBOX_REQUIRE_TX) и retention.
var Module = fx.Module(
	"internal/platform/events",
	outboxpg.FactoryModule,
)
