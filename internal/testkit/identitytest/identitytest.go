package identitytest

import (
	"context"
	"errors"
	"net/http"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
)

func Anon() context.Context { return context.Background() }

func With(id identity.Identity) context.Context {
	return identity.With(context.Background(), id)
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
