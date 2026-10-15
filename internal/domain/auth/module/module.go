package module

import (
	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	authhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/delivery/http"
	authbruteforce "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/infra/bruteforce"
	authjwt "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/infra/jwt"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
	"git.stit.tech/stit-core/golibinfra/authnkit"
)

var Module = fx.Module(
	"internal/domain/auth/module",
	fx.Provide(
		newUserAdapter,
		func(a *userAdapter) auth.UserReader { return a },
		func(a *userAdapter) auth.PasswordSetter { return a },
		// Токены и cookie сессии выдаёт вход: подпись и настройки cookie — у этого домена.
		func(j *authnkit.HS256JWT) authnkit.JWTSigner { return j },
		func(a *appconfig.App) authgate.CookieConfig { return a.Cookie },
		authjwt.NewTokens,
		authbruteforce.NewLoginGuard,
		auth.NewMetrics,
		auth.NewUsecase,
		authhttp.NewHandler,
		func(u *auth.Usecase) authhttp.Usecase { return u },
	),
)
