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

func MustFrom(ctx context.Context) Identity {
	id, ok := From(ctx)
	if !ok {
		panic("identity: not in context")
	}
	return id
}

type ipCtxKey struct{}

func WithIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, ipCtxKey{}, ip)
}

func IPFrom(ctx context.Context) string {
	ip, _ := ctx.Value(ipCtxKey{}).(string)
	return ip
}

// RequestIDFrom keeps domains transport-agnostic: usecases read request meta
// through platform/identity only, never httpkit directly (rule domain-architecture).
func RequestIDFrom(ctx context.Context) string {
	return httpkit.RequestIDFrom(ctx)
}
