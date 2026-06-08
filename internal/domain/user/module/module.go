package module

import (
	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	userhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/user/delivery/http"
	userpg "git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/postgres"
)

var Module = fx.Module(
	"user",
	fx.Provide(
		userpg.NewRepository,
		fx.Annotate(user.NewPolicy, fx.As(new(user.Authorizer))),
		user.NewMetrics,
		user.NewUsecase,
		userhttp.NewHandler,
		func(u *user.Usecase) userhttp.Usecase { return u },
	),
	fx.Invoke(userhttp.Register),
)
