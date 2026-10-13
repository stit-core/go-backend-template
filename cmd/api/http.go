package main

import (
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/httpkit"

	authhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/delivery/http"
	userhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/user/delivery/http"
	verifhttp "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/delivery/http"
	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
)

// httpModule — HTTP-вход сервиса. fx выполняет Invoke модулей раньше корневого хука сервера
// httpkit: зависимости хендлеров стартуют до HTTP и гасятся после него, где бы они ни были.
var httpModule = fx.Module(
	"cmd/api",
	fx.Provide(authRateLimit),
	fx.Invoke(registerHTTP),
)

// authPathPrefix — пути входа, регистрации, сессии и паролей: под ними — строгий лимит клиента.
const authPathPrefix = "/api/auth/"

// authRateLimit — лимит клиента на /api/auth/* поверх общего лимита httpkit (HTTP_AUTH_RATE_LIMIT_*): общая
// очередь Argon2id процесса не ждёт поток одного клиента.
func authRateLimit(app *appconfig.App) httpkit.Middlewares {
	limit := httpkit.RateLimit(httpkit.RateLimitConfig{RPS: app.AuthRateLimit.RPS, Burst: app.AuthRateLimit.Burst})
	return httpkit.Middlewares{func(next http.Handler) http.Handler {
		limited := limit(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, authPathPrefix) {
				limited.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}}
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
