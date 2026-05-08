// Package module склеивает реализации user-домена в fx.Module.
//
// Принцип: golibinfra (authnkit, outboxkit, pgkit, ...) — наш собственный
// port-слой, поэтому usecase зависит от его типов напрямую без адаптеров.
// Доменные интерфейсы заводим только там, где это даёт реальную пользу
// сужения широких примитивов (Repository поверх *pgxpool, Authorizer как
// бизнес-политика). Для outbox-публикации используется outboxkit.Publisher.
package module

import (
	"go.uber.org/fx"

	"git.stit.tech/akadmin/go-backend-template/internal/domain/user"
	userhttp "git.stit.tech/akadmin/go-backend-template/internal/domain/user/delivery/http"
	userpg "git.stit.tech/akadmin/go-backend-template/internal/domain/user/infra/postgres"
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
