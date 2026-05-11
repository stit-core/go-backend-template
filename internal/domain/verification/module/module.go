package module

import (
	"go.uber.org/fx"

	"git.stit.tech/akadmin/go-backend-template/internal/domain/user"
	"git.stit.tech/akadmin/go-backend-template/internal/domain/verification"
	verifhttp "git.stit.tech/akadmin/go-backend-template/internal/domain/verification/delivery/http"
	verifpg "git.stit.tech/akadmin/go-backend-template/internal/domain/verification/infra/postgres"
)

var Module = fx.Module(
	"verification",
	fx.Provide(
		verifpg.NewRepository,
		newUserReaderAdapter,
		func(uc *user.Usecase) verification.PasswordSetter { return uc },
		func(uc *user.Usecase) verification.EmailVerificationSetter { return uc },
		func(uc *verification.Usecase) user.EmailVerificationIssuer { return uc },
		newLogMailerAdapter,
		verification.NewUsecase,
		verifhttp.NewHandler,
		func(u *verification.Usecase) verifhttp.Usecase { return u },
	),
	fx.Invoke(verifhttp.Register),
)

func newUserReaderAdapter(uc *user.Usecase) verification.UserReader {
	return &userReaderAdapter{uc: uc}
}
