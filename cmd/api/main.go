package main

import (
	"context"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/fx"

	auditpg "git.stit.tech/stit-core/golibinfra/auditkit/pgstore"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/bruteforcekit"
	bruteforcepg "git.stit.tech/stit-core/golibinfra/bruteforcekit/pgstore"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/healthkit"
	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/natskit"
	"git.stit.tech/stit-core/golibinfra/outboxkit"
	outboxpg "git.stit.tech/stit-core/golibinfra/outboxkit/pgstore"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/telemetry"
	"git.stit.tech/stit-core/golibinfra/timerkit"

	authmod "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/module"
	usermod "git.stit.tech/stit-core/go-backend-template/internal/domain/user/module"
	verifmod "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/module"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
	appevents "git.stit.tech/stit-core/go-backend-template/internal/platform/events"
)

// kitConfigs — конструкторы конфигов китов из ENV: Module'ы golibinfra свои конфиги
// не читают, их провайдит приложение.
var kitConfigs = []any{
	telemetry.NewConfig,
	logkit.NewConfig,
	logkit.NewRedactConfig,
	pgkit.NewPoolConfig,
	pgkit.NewReplicaConfig,
	httpkit.NewConfig,
	natskit.NewConfig,
	authnkit.NewArgon2idParams,
	authnkit.NewJWTConfig,
	bruteforcekit.NewConfig,
	outboxkit.NewConfig,
	outboxpg.NewConfig,
}

var (
	infraModules = fx.Options(
		fx.Provide(kitConfigs...),
		// Первым: глобальные провайдеры OTel, в которые пишут киты и метрики доменов; при
		// OTEL_SDK_DISABLED=true сигналы не экспортируются, логи — только в stdout.
		telemetry.Module,
		fx.Provide(func() metric.Meter { return otel.Meter("app") }),
		logkit.Module,
		pgkit.Module,
		natskit.Module(""),
		httpkit.Module,
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

// service — граф сервиса целиком: его запускает main, его же собирают тест графа и boot-тест.
var service = fx.Options(
	logkit.FxLogger,
	fx.RecoverFromPanics(),
	fx.StartTimeout(15*time.Second),
	// Бюджет остановки: дефолты HTTP_SHUTDOWN_TIMEOUT (15s) + NATS_DRAIN_TIMEOUT (10s) + запас; env выше он режет.
	fx.StopTimeout(30*time.Second),
	infraModules,
	platformModules,
	domainModules,
	httpModule,
	fx.Invoke(logStart),
)

// logStart: контекста запроса при сборке графа нет — корень процесса.
func logStart(log logkit.Logger) {
	log.Info(context.Background(), "starting", "component", "api", "version", version, "commit", commit)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(context.Background()))
	}
	fx.New(service).Run()
}
