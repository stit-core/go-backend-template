package identitytest

import (
	"context"

	"git.stit.tech/stit-template/go-backend-template/internal/platform/identity"
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
