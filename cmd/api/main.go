package main

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
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
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/timerkit"

	"git.stit.tech/stit-core/golibinfra/clockkit"

	authmod "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/module"
	usermod "git.stit.tech/stit-core/go-backend-template/internal/domain/user/module"
	verifmod "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/module"
	"git.stit.tech/stit-core/go-backend-template/internal/jobs"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
	appevents "git.stit.tech/stit-core/go-backend-template/internal/platform/events"
)

var kitConfigs = fx.Options(
	fx.Provide(
		context.Background,

		logkit.NewConfig,
		flagkit.NewConfig,
		cachekit.NewConfig,
		authnkit.NewArgon2idParams,
		authnkit.NewJWTConfig,
		bruteforcekit.NewConfig,
		outboxkit.NewConfig,
		pgkit.NewReplicaConfig,
	),
)

// logging — stdout-логгер (JSON в docker/kubectl logs). Без OTLP-экспорта:
// корп observability-стек отсутствует, поэтому logkit собирается вокруг
// os.Stdout, а не OTel LoggerProvider.
var logging = fx.Options(
	fx.Provide(
		func(cfg logkit.Config) logkit.Logger { return logkit.NewWithWriter(cfg, os.Stdout) },
		// timerkit resolves *slog.Logger from the fx graph; built via logkit, not for direct logging.
		func(cfg logkit.Config) *slog.Logger { return logkit.NewSlogWithWriter(cfg, os.Stdout) }, //nolint:forbidigo
		// metric.Meter — глобальный no-op: MeterProvider не сконфигурирован,
		// поэтому domain business-counter'ы (biz_*) инертны и никуда не
		// экспортируются. Держится, чтобы DI доменных Metrics резолвился.
		func() metric.Meter { return otel.Meter("app") },
	),
)

var (
	infraModules = fx.Options(
		// Конфиг только из env: приложение читает настройки/секреты из
		// окружения (compose/.env проектной инфры). Vault-источник не
		// подключается — секрет-стор в экосистеме отсутствует.
		fx.Provide(func() []config.Source { return []config.Source{config.Env()} }),
		kitConfigs,
		logging,
		pgkit.Module,
		httpkit.Module,
		natskit.Module(""),
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
		appevents.Module,
		fx.Provide(
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
