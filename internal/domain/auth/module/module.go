package module

import (
	"go.uber.org/fx"

	"git.stit.tech/akadmin/go-backend-template/internal/domain/auth"
	authhttp "git.stit.tech/akadmin/go-backend-template/internal/domain/auth/delivery/http"
	authpg "git.stit.tech/akadmin/go-backend-template/internal/domain/auth/infra/postgres"
	"git.stit.tech/akadmin/go-backend-template/internal/domain/user"
	"git.stit.tech/stit/golibinfra/authnkit"
	"git.stit.tech/stit/golibinfra/bruteforcekit"
)

var Module = fx.Module(
	"auth",
	fx.Provide(
		newUserReaderAdapter,
		func(uc *user.Usecase) auth.PasswordVerifier { return uc },
		func(uc *user.Usecase) auth.PasswordSetter { return uc },
		authpg.NewRepository,
		fx.Annotate(auth.NewPolicy, fx.As(new(auth.Authorizer))),
		func(s authnkit.JWTSigner, cfg authnkit.JWTConfig) auth.Tokens {
			return &tokensAdapter{signer: s, accessTTL: cfg.AccessTTL, refreshTTL: cfg.RefreshTTL}
		},
		func(p authnkit.JWTParser) auth.TokenParser {
			return &tokenParserAdapter{parser: p}
		},
		func(g bruteforcekit.Guard) auth.LoginGuard { return g },
		auth.NewUsecase,
		authhttp.NewHandler,
		func(u *auth.Usecase) authhttp.Usecase { return u },
	),
	fx.Invoke(authhttp.Register),
)

func newUserReaderAdapter(uc *user.Usecase) auth.UserReader {
	return &userReaderAdapter{uc: uc}
}
