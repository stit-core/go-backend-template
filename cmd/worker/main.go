package main

import (
	"context"
	"time"

	"go.uber.org/fx"

	"git.stit.tech/akadmin/golibinfra/cachekit"
	"git.stit.tech/akadmin/golibinfra/config"
	"git.stit.tech/akadmin/golibinfra/flagkit"
	"git.stit.tech/akadmin/golibinfra/logkit"
	"git.stit.tech/akadmin/golibinfra/natskit"
	"git.stit.tech/akadmin/golibinfra/outboxkit"
	"git.stit.tech/akadmin/golibinfra/pgkit"
	"git.stit.tech/akadmin/golibinfra/telemetry"
	"git.stit.tech/akadmin/golibinfra/timerkit"

	appconfig "git.stit.tech/akadmin/go-backend-template/internal/platform/config"
	"git.stit.tech/akadmin/go-backend-template/internal/worker"
	"git.stit.tech/akadmin/go-backend-template/internal/worker/subscribers"
	platformaudit "git.stit.tech/akadmin/golibinfra/auditkit"
)

var (
	infraModules = fx.Options(
		config.Module,
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
