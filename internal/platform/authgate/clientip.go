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
