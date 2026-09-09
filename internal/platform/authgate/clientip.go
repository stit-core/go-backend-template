package authgate

import (
	"net/http"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

// ClientIP — middleware: кладёт IP клиента в ctx (identity.WithIP) для
// bruteforce-guard'а и аудита. trustProxy — брать адрес из X-Forwarded-For /
// X-Real-IP; включать только за собственным reverse-proxy, иначе заголовок
// подделывается и ключ guard'а выбирает сам клиент.
func ClientIP(trustProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := httpkit.ClientIP(r, trustProxy)
			next.ServeHTTP(w, r.WithContext(identity.WithIP(r.Context(), ip)))
		})
	}
}

// newClientIPMiddleware: trustProxy — HTTP_RATE_LIMIT_TRUST_PROXY, тот же флаг,
// что у rate-limiter'а httpkit: один ответ на вопрос «стоит ли перед сервисом
// доверенный прокси».
func newClientIPMiddleware(cfg httpkit.Config) func(http.Handler) http.Handler {
	return ClientIP(cfg.RateLimit.TrustProxy)
}
