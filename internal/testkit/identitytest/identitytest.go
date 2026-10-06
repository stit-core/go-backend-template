package identitytest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

// IP и RequestID — запрос, из которого приходят контексты пакета: тесты сверяют с ними аудит, guard
// входа и лимитер сброса.
const (
	IP        = "203.0.113.7"
	RequestID = "req-identitytest"
)

// Anon — ctx запроса без личности: IP клиента и request id в нём кладёт цепочка httpkit, как у сервиса.
func Anon() context.Context {
	var ctx context.Context
	h := httpkit.ClientIP(logkit.Nop(), 0)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { ctx = r.Context() }))
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	r.RemoteAddr = IP + ":51234"
	h.ServeHTTP(httptest.NewRecorder(), r)
	return httpkit.ContextWithRequestID(ctx, RequestID)
}

func With(id identity.Identity) context.Context {
	return identity.With(Anon(), id)
}

func Admin(userID string) context.Context {
	return With(identity.Identity{UserID: userID, Role: identity.RoleAdmin})
}

func Client(userID string) context.Context {
	return With(identity.Identity{UserID: userID, Role: identity.RoleClient})
}

func Unknown(userID string) context.Context {
	return With(identity.Identity{UserID: userID, Role: identity.RoleUnknown})
}

// Extract — экстрактор handler-тестов (handlertest.WithAuth): личность — из ctx запроса (PostCtx,
// GetCtx с контекстом пакета); запрос без неё — 401, как без cookie.
func Extract(r *http.Request) (context.Context, error) {
	if _, ok := identity.From(r.Context()); !ok {
		return r.Context(), errors.New("identitytest: no identity in the request context")
	}
	return r.Context(), nil
}
