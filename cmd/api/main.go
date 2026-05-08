package main

import (
	"go.uber.org/fx"

	"git.stit.tech/akadmin/golibinfra/auditkit"
	"git.stit.tech/akadmin/golibinfra/authnkit"
	"git.stit.tech/akadmin/golibinfra/bruteforcekit"
	"git.stit.tech/akadmin/golibinfra/cachekit"
	"git.stit.tech/akadmin/golibinfra/config"
	"git.stit.tech/akadmin/golibinfra/flagkit"
	"git.stit.tech/akadmin/golibinfra/httpkit"
	"git.stit.tech/akadmin/golibinfra/logkit"
	"git.stit.tech/akadmin/golibinfra/natskit"
	"git.stit.tech/akadmin/golibinfra/outboxkit"
	"git.stit.tech/akadmin/golibinfra/pgkit"
	"git.stit.tech/akadmin/golibinfra/telemetry"
	"git.stit.tech/akadmin/golibinfra/timerkit"

	"git.stit.tech/akadmin/golibinfra/clockkit"

	authmod "git.stit.tech/akadmin/go-backend-template/internal/domain/auth/module"
	usermod "git.stit.tech/akadmin/go-backend-template/internal/domain/user/module"
	verifmod "git.stit.tech/akadmin/go-backend-template/internal/domain/verification/module"
	"git.stit.tech/akadmin/go-backend-template/internal/platform/authgate"
	appconfig "git.stit.tech/akadmin/go-backend-template/internal/platform/config"
)

var (
	infraModules = fx.Options(
		config.Module,
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
