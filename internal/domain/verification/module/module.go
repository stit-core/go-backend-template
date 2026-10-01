package module

import (
	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	verifhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/delivery/http"
	verifnats "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/delivery/nats"
	verifbruteforce "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/bruteforce"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/logmail"
	verifoutbox "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/outbox"
	verifpg "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/postgres"
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
		verifbruteforce.NewResetLimiter,
		verification.NewMetrics,
		verification.NewUsecase,
		verifhttp.NewHandler,
		func(u *verification.Usecase) verifhttp.Usecase { return u },
		verifnats.NewHandler,
		func(u *verification.Usecase) verifnats.Usecase { return u },
	),
	fx.Invoke(verifnats.Register),
)
