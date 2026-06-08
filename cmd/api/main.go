package main

import (
	"context"

	"go.uber.org/fx"

	auditpg "git.stit.tech/stit-core/golibinfra/auditkit/pgstore"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/bruteforcekit"
	bruteforcepg "git.stit.tech/stit-core/golibinfra/bruteforcekit/pgstore"
	"git.stit.tech/stit-core/golibinfra/cachekit"
	"git.stit.tech/stit-core/golibinfra/config"
	"git.stit.tech/stit-core/golibinfra/flagkit"
	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/natskit"
	"git.stit.tech/stit-core/golibinfra/outboxkit"
	outboxpg "git.stit.tech/stit-core/golibinfra/outboxkit/pgstore"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/telemetry"
	"git.stit.tech/stit-core/golibinfra/timerkit"

	"git.stit.tech/stit-core/golibinfra/clockkit"

	authmod "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/module"
	usermod "git.stit.tech/stit-core/go-backend-template/internal/domain/user/module"
	verifmod "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/module"
	"git.stit.tech/stit-core/go-backend-template/internal/jobs"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
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
