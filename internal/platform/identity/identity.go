package identity

import "context"

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
