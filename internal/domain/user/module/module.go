package module

import (
	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	userhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/user/delivery/http"
	useroutbox "git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/outbox"
	userpg "git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/postgres"
)

var Module = fx.Module(
	"internal/domain/user/module",
	useroutbox.Providers,
	fx.Provide(
		userpg.NewRepository,
		user.NewPolicy,
		user.NewMetrics,
		user.NewUsecase,
		userhttp.NewHandler,
		func(u *user.Usecase) userhttp.Usecase { return u },
	),
)
