package module

import (
	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	authhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/delivery/http"
	authpg "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/infra/postgres"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
)

var Module = fx.Module(
	"auth",
	fx.Provide(
		newUserReaderAdapter,
		func(uc *user.Usecase) auth.PasswordVerifier { return uc },
		func(uc *user.Usecase) auth.PasswordSetter { return uc },
		authpg.NewRepository,
		fx.Annotate(auth.NewPolicy, fx.As(new(auth.Authorizer))),
		newTokensAdapter,
		newTokenParserAdapter,
		newLoginGuardAdapter,
		auth.NewMetrics,
		auth.NewUsecase,
		authhttp.NewHandler,
		func(u *auth.Usecase) authhttp.Usecase { return u },
	),
)
