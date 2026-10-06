package module

import (
	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	authhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/delivery/http"
	authbruteforce "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/infra/bruteforce"
	authjwt "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/infra/jwt"
)

var Module = fx.Module(
	"internal/domain/auth/module",
	fx.Provide(
		newUserAdapter,
		func(a *userAdapter) auth.UserReader { return a },
		func(a *userAdapter) auth.PasswordVerifier { return a },
		func(a *userAdapter) auth.PasswordSetter { return a },
		authjwt.NewTokens,
		authbruteforce.NewLoginGuard,
		auth.NewPolicy,
		auth.NewMetrics,
		auth.NewUsecase,
		authhttp.NewHandler,
		func(u *auth.Usecase) authhttp.Usecase { return u },
	),
)
