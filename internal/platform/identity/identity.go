package identity

import (
	"context"

	"git.stit.tech/stit-core/golibinfra/httpkit"
)

type Role string

const (
	RoleUnknown Role = ""
	RoleAdmin   Role = "admin"
	RoleClient  Role = "client"
)

type Identity struct {
	UserID string
	Role   Role
}

type ctxKey struct{}

func With(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func From(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// IPFrom и RequestIDFrom держат домены вне транспорта: значения запроса в ctx кладёт цепочка httpkit,
// а usecase читает их только здесь (rule domain-architecture).
func IPFrom(ctx context.Context) string {
	return httpkit.ClientIPFrom(ctx)
}

func RequestIDFrom(ctx context.Context) string {
	return httpkit.RequestIDFrom(ctx)
}
