package main

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/fx"

	auditpg "git.stit.tech/stit-core/golibinfra/auditkit/pgstore"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/bruteforcekit"
	bruteforcepg "git.stit.tech/stit-core/golibinfra/bruteforcekit/pgstore"
	"git.stit.tech/stit-core/golibinfra/cachekit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/config"
	"git.stit.tech/stit-core/golibinfra/flagkit"
	"git.stit.tech/stit-core/golibinfra/healthkit"
	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/natskit"
	"git.stit.tech/stit-core/golibinfra/outboxkit"
	outboxpg "git.stit.tech/stit-core/golibinfra/outboxkit/pgstore"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/timerkit"

	authmod "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/module"
	usermod "git.stit.tech/stit-core/go-backend-template/internal/domain/user/module"
	verifmod "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/module"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
	appevents "git.stit.tech/stit-core/go-backend-template/internal/platform/events"
)

// kitConfigs — конструкторы конфигов китов из ENV: Module'ы golibinfra свои конфиги
// не читают, их провайдит приложение. pgkit.PoolConfig и httpkit.Config приходят
// из App-структуры (appconfig).
var kitConfigs = []any{
	logkit.NewConfig,
	logkit.NewRedactConfig,
	flagkit.NewConfig,
	cachekit.NewConfig,
	natskit.NewConfig,
	authnkit.NewArgon2idParams,
	authnkit.NewJWTConfig,
	bruteforcekit.NewConfig,
	outboxkit.NewConfig,
	outboxpg.NewConfig,
	pgkit.NewReplicaConfig,
}

// beforeHTTP поднимает пул, NATS и Redis раньше сервера HTTP: fx останавливает хуки в обратном
// порядке, и HTTP гасится до них — запросам в полёте не достаются закрытые клиенты.
var beforeHTTP = fx.Module("before-http", fx.Invoke(func(*pgkit.ReadWriteDB, *natskit.Client, cachekit.Cache) {}))

var (
	infraModules = fx.Options(
		// Конфиг только из env: настройки/секреты приходят из окружения
		// (compose/.env проектной инфры).
		config.Module,
		fx.Provide(context.Background),
		fx.Provide(kitConfigs...),
		// metric.Meter — глобальный no-op: MeterProvider не сконфигурирован,
		// поэтому business-counter'ы доменов (biz_*) инертны и никуда не
		// экспортируются. Держится, чтобы DI доменных Metrics резолвился.
		fx.Provide(func() metric.Meter { return otel.Meter("app") }),
		// Логи — локальный sink (JSON в stdout → docker/kubectl logs); OTLP-sink
		// не поднимается: log.LoggerProvider в графе отсутствует.
		logkit.Module,
		pgkit.Module,
		natskit.Module(""),
		cachekit.Module,
		beforeHTTP,
		httpkit.Module,
		flagkit.Module,
		timerkit.Module,
	)

	platformModules = fx.Options(
		appconfig.Module,
		authgate.Module,
		bruteforcepg.Module,
		auditpg.Module,
		clockkit.Module,
		appevents.Module,
		// /ready отвечает 503, пока хоть одна зависимость не отвечает.
		healthkit.Provide(pgkit.Probe),
		healthkit.Provide(natskit.Probe),
		healthkit.Provide(cachekit.Probe),
	)

	domainModules = fx.Options(
		usermod.Module,
		authmod.Module,
		verifmod.Module,
		fx.Invoke(registerHTTP),
	)
)

var (
	version = "dev"
	commit  = "unknown"
)

// service — граф сервиса целиком: его запускает main, его же поднимает boot-тест.
var service = fx.Options(
	infraModules,
	platformModules,
	domainModules,
	fx.Invoke(func(ctx context.Context, log logkit.Logger) {
		log.Info(ctx, "starting", "component", "api", "version", version, "commit", commit)
	}),
)

func main() {
	fx.New(service).Run()
}
