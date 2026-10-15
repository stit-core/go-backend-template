package module

import (
	"time"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	verifhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/delivery/http"
	verifnats "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/delivery/nats"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/logmail"
	verifoutbox "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/outbox"
	verifpg "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/postgres"
	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
	"git.stit.tech/stit-core/golibinfra/timerkit"
)

var Module = fx.Module(
	"internal/domain/verification/module",
	verifoutbox.Providers,
	fx.Provide(
		verifpg.NewRepository,
		newUserAdapter,
		func(a *userAdapter) verification.UserReader { return a },
		func(a *userAdapter) verification.PasswordSetter { return a },
		func(a *userAdapter) verification.EmailVerificationSetter { return a },
		logmail.New,
		func(a *appconfig.App) verification.Config {
			return verification.Config{ResetMailPause: a.PasswordResetMailPause}
		},
		verification.NewMetrics,
		verification.NewUsecase,
		verifhttp.NewHandler,
		func(u *verification.Usecase) verifhttp.Usecase { return u },
		verifnats.NewHandler,
		func(u *verification.Usecase) verifnats.Usecase { return u },
	),
	timerkit.ProvideJob(func(uc *verification.Usecase) timerkit.Job {
		return timerkit.Job{Name: "verification.purge_links", Interval: time.Hour, Timeout: time.Minute, Run: uc.PurgeLinks}
	}),
	fx.Invoke(verifnats.Register),
)
