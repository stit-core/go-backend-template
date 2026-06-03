package module

import (
	"go.uber.org/fx"

	"git.stit.tech/stit-template/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-template/go-backend-template/internal/domain/verification"
	verifhttp "git.stit.tech/stit-template/go-backend-template/internal/domain/verification/delivery/http"
	verifnats "git.stit.tech/stit-template/go-backend-template/internal/domain/verification/delivery/nats"
	verifpg "git.stit.tech/stit-template/go-backend-template/internal/domain/verification/infra/postgres"
)

var Module = fx.Module(
	"verification",
	fx.Provide(
		verifpg.NewRepository,
		newUserReaderAdapter,
		func(uc *user.Usecase) verification.PasswordSetter { return uc },
		func(uc *user.Usecase) verification.EmailVerificationSetter { return uc },
		newLogMailerAdapter,
		verification.NewMetrics,
		verification.NewUsecase,
		verifhttp.NewHandler,
		func(u *verification.Usecase) verifhttp.Usecase { return u },
		verifnats.NewHandler,
		func(u *verification.Usecase) verifnats.Usecase { return u },
	),
	fx.Invoke(verifhttp.Register),
	fx.Invoke(verifnats.Register),
)

func newUserReaderAdapter(uc *user.Usecase) verification.UserReader {
	return &userReaderAdapter{uc: uc}
}
