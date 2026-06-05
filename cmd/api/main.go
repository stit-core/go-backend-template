package main

import (
	"context"

	"go.uber.org/fx"

	auditpg "git.sftbmb.ru/sftbmb-core/golibinfra/auditkit/pgstore"
	"git.sftbmb.ru/sftbmb-core/golibinfra/authnkit"
	"git.sftbmb.ru/sftbmb-core/golibinfra/bruteforcekit"
	bruteforcepg "git.sftbmb.ru/sftbmb-core/golibinfra/bruteforcekit/pgstore"
	"git.sftbmb.ru/sftbmb-core/golibinfra/cachekit"
	"git.sftbmb.ru/sftbmb-core/golibinfra/config"
	"git.sftbmb.ru/sftbmb-core/golibinfra/flagkit"
	"git.sftbmb.ru/sftbmb-core/golibinfra/httpkit"
	"git.sftbmb.ru/sftbmb-core/golibinfra/logkit"
	"git.sftbmb.ru/sftbmb-core/golibinfra/natskit"
	"git.sftbmb.ru/sftbmb-core/golibinfra/outboxkit"
	outboxpg "git.sftbmb.ru/sftbmb-core/golibinfra/outboxkit/pgstore"
	"git.sftbmb.ru/sftbmb-core/golibinfra/pgkit"
	"git.sftbmb.ru/sftbmb-core/golibinfra/telemetry"
	"git.sftbmb.ru/sftbmb-core/golibinfra/timerkit"

	"git.sftbmb.ru/sftbmb-core/golibinfra/clockkit"

	authmod "git.sftbmb.ru/sftbmb-core/go-backend-template/internal/domain/auth/module"
	usermod "git.sftbmb.ru/sftbmb-core/go-backend-template/internal/domain/user/module"
	verifmod "git.sftbmb.ru/sftbmb-core/go-backend-template/internal/domain/verification/module"
	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/jobs"
	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/platform/authgate"
	appconfig "git.sftbmb.ru/sftbmb-core/go-backend-template/internal/platform/config"
)

var kitConfigs = fx.Options(
	fx.Provide(
		context.Background,

		logkit.NewConfig,
		logkit.NewServiceConfig,
		flagkit.NewConfig,
		cachekit.NewConfig,
		natskit.NewConfig,
		authnkit.NewArgon2idParams,
		authnkit.NewJWTConfig,
		bruteforcekit.NewConfig,
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
		httpkit.Module,
		natskit.Module,
		cachekit.Module,
		flagkit.Module,
		timerkit.Module,
	)

	platformModules = fx.Options(
		appconfig.Module,
		authgate.Module,
		authnkit.Module,
		bruteforcepg.Module,
		auditpg.Module,
		clockkit.Module,
		outboxkit.Module,
		outboxpg.Module,
		fx.Provide(
			func(s *outboxpg.Store) outboxkit.LagReporter { return s },
			fx.Annotate(pgkit.DBProbe, fx.ResultTags(`group:"health"`)),
			fx.Annotate(natskit.Probe, fx.ResultTags(`group:"health"`)),
			fx.Annotate(cachekit.Probe, fx.ResultTags(`group:"health"`)),
			fx.Annotate(jobs.NewBruteforceCleanup, fx.ResultTags(`group:"timer.job"`)),
		),
	)

	domainModules = fx.Options(
		usermod.Module,
		authmod.Module,
		verifmod.Module,
	)
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	fx.New(
		infraModules,
		platformModules,
		domainModules,
		fx.Invoke(func(ctx context.Context, log logkit.Logger) {
			log.Info(ctx, "starting", "component", "api", "version", version, "commit", commit)
		}),
	).Run()
}
