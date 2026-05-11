package main

import (
	"context"
	"time"

	"go.uber.org/fx"

	"git.stit.tech/stit/golibinfra/cachekit"
	"git.stit.tech/stit/golibinfra/config"
	"git.stit.tech/stit/golibinfra/flagkit"
	"git.stit.tech/stit/golibinfra/logkit"
	"git.stit.tech/stit/golibinfra/natskit"
	"git.stit.tech/stit/golibinfra/outboxkit"
	"git.stit.tech/stit/golibinfra/pgkit"
	"git.stit.tech/stit/golibinfra/telemetry"
	"git.stit.tech/stit/golibinfra/timerkit"

	appconfig "git.stit.tech/akadmin/go-backend-template/internal/platform/config"
	"git.stit.tech/akadmin/go-backend-template/internal/worker"
	"git.stit.tech/akadmin/go-backend-template/internal/worker/subscribers"
	platformaudit "git.stit.tech/stit/golibinfra/auditkit"
)

var kitConfigs = fx.Options(
	fx.Provide(
		context.Background,

		logkit.NewConfig,
		logkit.NewServiceConfig,
		flagkit.NewConfig,
		cachekit.NewConfig,
		natskit.NewConfig,
		outboxkit.NewConfig,
		pgkit.NewReplicaConfig,
	),
)

var (
	infraModules = fx.Options(
		config.Module,
		kitConfigs,
		logkit.Module,
		telemetry.Module,
		pgkit.Module,
		natskit.Module,
		cachekit.Module,
		flagkit.Module,
		timerkit.Module,
	)

	platformModules = fx.Options(
		appconfig.Module,

		outboxkit.Module,
		platformaudit.Module,
	)

	workerModules = fx.Options(
		fx.Provide(
			fx.Annotate(
				newBruteforceCleanupJob,
				fx.ResultTags(`group:"timer.job"`),
			),
		),
		subscribers.Module,
	)
)

func newBruteforceCleanupJob(db *pgkit.DB) timerkit.Job {
	return timerkit.Job{
		Name:     "bruteforce.cleanup",
		Interval: 5 * time.Minute,
		Run: func(ctx context.Context) error {
			return worker.Cleanup(ctx, db)
		},
	}
}

func main() {
	fx.New(
		infraModules,
		platformModules,
		workerModules,
	).Run()
}
