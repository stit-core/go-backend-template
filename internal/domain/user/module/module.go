package module

import (
	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	userhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/user/delivery/http"
	useroutbox "git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/outbox"
	userpg "git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/postgres"
	"git.stit.tech/stit-core/golibinfra/authnkit/session"
)

var Module = fx.Module(
	"internal/domain/user/module",
	useroutbox.Providers,
	fx.Provide(
		userpg.NewRepository,
		func(s session.Store) user.Sessions { return s },
		user.NewPolicy,
		user.NewUsecase,
		user.NewTrusted,
		userhttp.NewHandler,
		func(u *user.Usecase) userhttp.Usecase { return u },
	),
)
