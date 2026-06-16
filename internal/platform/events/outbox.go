package events

import (
	"context"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/natskit"
	"git.stit.tech/stit-core/golibinfra/outboxkit"
	outboxpg "git.stit.tech/stit-core/golibinfra/outboxkit/pgstore"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/timerkit"
)

// OutboxFor разводит per-domain transactional-outbox: приватные store+Outbox+Publisher
// и экспорт в общие группы (drain-Job, partition-keeper-Job, BacklogReporter). Drain
// публикует в JetStream через общий natskit.StreamBus; namespace = имя инстанса разводит
// Nats-Msg-Id между таблицами. Возвращает fx.Options (НЕ fx.Module): fx.Private обязан
// заскоупиться доменным модулем, в который это включают.
func OutboxFor(name, table string) fx.Option {
	return fx.Options(
		fx.Provide(
			fx.Private,
			func(db *pgkit.DB) (*outboxpg.Store, error) { return outboxpg.NewStoreFor(db, table) },
			func(s *outboxpg.Store, sb natskit.StreamBus, cfg outboxkit.Config) *outboxkit.Outbox {
				return outboxkit.New(name, s, outboxkit.NewStreamTransportFor(sb, name), cfg)
			},
			func(o *outboxkit.Outbox) outboxkit.Publisher { return o.Publisher() },
		),
		fx.Provide(
			fx.Annotate(func(o *outboxkit.Outbox) timerkit.Job { return o.Job() }, fx.ResultTags(`group:"timer.job"`)),
			fx.Annotate(outboxpg.PartitionKeeperJob, fx.ResultTags(`group:"timer.job"`)),
			fx.Annotate(func(s *outboxpg.Store) outboxkit.BacklogReporter { return s }, fx.ResultTags(`group:"outbox.backlog"`)),
		),
	)
}

type outboxBacklogParams struct {
	fx.In

	LC        fx.Lifecycle
	Reporters []outboxkit.BacklogReporter `group:"outbox.backlog"`
}

func registerOutboxBacklog(p outboxBacklogParams) error {
	for _, r := range p.Reporters {
		reg, err := outboxkit.RegisterBacklogGauges(r)
		if err != nil {
			return err
		}
		p.LC.Append(fx.Hook{OnStop: func(context.Context) error { return reg.Unregister() }})
	}
	return nil
}

var Module = fx.Module(
	"platform.events",
	fx.Invoke(registerOutboxBacklog),
)
