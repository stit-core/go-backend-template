package main

import (
	"github.com/danielgtaylor/huma/v2"
	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/httpkit"

	authhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/delivery/http"
	userhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/user/delivery/http"
	verifhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/delivery/http"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
)

// httpModule — HTTP-вход сервиса. fx выполняет Invoke модулей раньше корневого хука сервера
// httpkit: зависимости хендлеров стартуют до HTTP и гасятся после него, где бы они ни были.
var httpModule = fx.Module(
	"cmd/api",
	fx.Provide(newMiddlewares),
	fx.Invoke(registerHTTP),
)

// newMiddlewares: доверие прокси — HTTP_RATE_LIMIT_TRUST_PROXY, тот же флаг, что у rate-limiter'а
// httpkit: один ответ на вопрос «стоит ли перед сервисом доверенный прокси».
func newMiddlewares(cfg httpkit.Config) httpkit.Middlewares {
	return httpkit.Middlewares{authgate.ClientIP(cfg.RateLimit.TrustProxy)}
}

type httpHandlers struct {
	fx.In

	User         *userhttp.Handler
	Auth         *authhttp.Handler
	Verification *verifhttp.Handler
}

// registerHTTP — единственный набор операций API: его регистрирует сервис, а тесты собирают
// по нему API без БД и брокера (хендлеры пустые — huma нужны только их типы).
func registerHTTP(api huma.API, h httpHandlers) {
	userhttp.Register(api, h.User)
	authhttp.Register(api, h.Auth)
	verifhttp.Register(api, h.Verification)
}
