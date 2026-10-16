package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.uber.org/fx"

	auditpg "git.stit.tech/stit-core/golibinfra/auditkit/pgstore"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/authnkit/session"
	sessionpg "git.stit.tech/stit-core/golibinfra/authnkit/session/pgstore"
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
	authnkit.NewArgon2idConfig,
	authnkit.NewJWTConfig,
	session.NewConfig,
	bruteforcekit.NewConfig,
	auditpg.NewRetentionConfig,
	outboxkit.NewConfig,
	outboxpg.NewConfig,
}

var (
	infraModules = fx.Options(
		fx.Provide(kitConfigs...),
		// Первым: глобальные провайдеры OTel, в которые пишут киты; при OTEL_SDK_DISABLED=true сигналы
		// не экспортируются, логи — только в stdout.
		telemetry.Module,
		// Meter доменов — провайдер telemetry из графа, а не глобал: порядок модулей ему не важен.
		fx.Provide(func(p *telemetry.Providers) metric.Meter { return p.MeterProvider.Meter("app") }),
		logkit.Module,
		pgkit.Module,
		natskit.Module(""),
		httpkit.Module,
		timerkit.Module,
	)

	platformModules = fx.Options(
		appconfig.Module,
		authgate.Module,
		sessionpg.Module,
		bruteforcepg.Module,
		auditpg.Module,
		auditpg.RetentionModule,
		clockkit.Module,
		appevents.Module,
		// /ready отвечает 503, пока хоть одна зависимость не отвечает.
		healthkit.ProvideProbe(pgkit.Probe),
		natskit.ProvideProbe(""),
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

// stopTimeout — бюджет хуков остановки: HTTP берёт из него не больше ⅔ (больше httpkit не даст), остаток —
// хукам после него.
const stopTimeout = 30 * time.Second

// service — граф сервиса целиком: его запускает main, его же собирают тест графа и boot-тест.
var service = fx.Options(
	logkit.FxLogger,
	fx.RecoverFromPanics(),
	fx.StartTimeout(15*time.Second),
	fx.StopTimeout(stopTimeout),
	infraModules,
	platformModules,
	domainModules,
	httpModule,
	fx.Invoke(checkStopBudget, logStart),
)

// errStopBudget — остановка, заданная env, не помещается в stopTimeout.
var errStopBudget = errors.New("shutdown env exceeds the stop budget")

// checkStopBudget: HTTP останавливается в ⅔ бюджета, больше трети NATS_DRAIN_TIMEOUT шина не получит никогда, а
// дренажу подписки и это не гарантия (docs/reference/architecture.md § fx).
func checkStopBudget(httpCfg httpkit.Config, natsCfg natskit.Config) error {
	if d := httpCfg.ShutdownDrainDelay + httpCfg.ShutdownTimeout; d > stopTimeout*2/3 {
		return fmt.Errorf("%w: HTTP_SHUTDOWN_DRAIN_DELAY + HTTP_SHUTDOWN_TIMEOUT = %s, at most %s", errStopBudget, d, stopTimeout*2/3)
	}
	if natsCfg.DrainTimeout > stopTimeout/3 {
		return fmt.Errorf("%w: NATS_DRAIN_TIMEOUT = %s, at most %s", errStopBudget, natsCfg.DrainTimeout, stopTimeout/3)
	}
	return nil
}

// logStart: контекста запроса при сборке графа нет — корень процесса.
func logStart(log logkit.Logger) {
	log.Info(context.Background(), "starting", "component", "api", "version", version, "commit", commit)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(httpkit.Healthcheck(context.Background()))
	}
	fx.New(service).Run()
}
