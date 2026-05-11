package main

import (
	"context"

	"go.uber.org/fx"

	"git.stit.tech/stit/golibinfra/auditkit"
	"git.stit.tech/stit/golibinfra/authnkit"
	"git.stit.tech/stit/golibinfra/bruteforcekit"
	"git.stit.tech/stit/golibinfra/cachekit"
	"git.stit.tech/stit/golibinfra/config"
	"git.stit.tech/stit/golibinfra/flagkit"
	"git.stit.tech/stit/golibinfra/httpkit"
	"git.stit.tech/stit/golibinfra/logkit"
	"git.stit.tech/stit/golibinfra/natskit"
	"git.stit.tech/stit/golibinfra/outboxkit"
	"git.stit.tech/stit/golibinfra/pgkit"
	"git.stit.tech/stit/golibinfra/telemetry"
	"git.stit.tech/stit/golibinfra/timerkit"

	"git.stit.tech/stit/golibinfra/clockkit"

	authmod "git.stit.tech/akadmin/go-backend-template/internal/domain/auth/module"
	usermod "git.stit.tech/akadmin/go-backend-template/internal/domain/user/module"
	verifmod "git.stit.tech/akadmin/go-backend-template/internal/domain/verification/module"
	"git.stit.tech/akadmin/go-backend-template/internal/platform/authgate"
	appconfig "git.stit.tech/akadmin/go-backend-template/internal/platform/config"
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
		bruteforcekit.Module,
		auditkit.Module,
		clockkit.Module,
		outboxkit.Module,
		fx.Provide(
			fx.Annotate(pgkit.DBProbe, fx.ResultTags(`group:"health"`)),
			fx.Annotate(natskit.Probe, fx.ResultTags(`group:"health"`)),
			fx.Annotate(cachekit.Probe, fx.ResultTags(`group:"health"`)),
		),
	)

	domainModules = fx.Options(
		usermod.Module,
		authmod.Module,
		verifmod.Module,
	)
)

func main() {
	fx.New(
		infraModules,
		platformModules,
		domainModules,
	).Run()
}
