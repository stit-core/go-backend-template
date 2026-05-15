package user_test

import (
	"context"
	"testing"

	"git.stit.tech/stit-infra/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-infra/go-backend-template/internal/platform/identity"
)

func BenchmarkCanGetByID(b *testing.B) {
	p := user.NewPolicy()
	ctx := context.Background()
	cases := []struct {
		name   string
		caller identity.Identity
	}{
		{"admin", identity.Identity{UserID: "u1", Role: identity.RoleAdmin}},
		{"client", identity.Identity{UserID: "u1", Role: identity.RoleClient}},
		{"unknown", identity.Identity{UserID: "u1", Role: identity.RoleUnknown}},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = p.CanGetByID(ctx, c.caller, "u1")
			}
		})
	}
}

func BenchmarkCanList(b *testing.B) {
	p := user.NewPolicy()
	ctx := context.Background()
	cases := []struct {
		name   string
		caller identity.Identity
	}{
		{"admin", identity.Identity{UserID: "u1", Role: identity.RoleAdmin}},
		{"client", identity.Identity{UserID: "u1", Role: identity.RoleClient}},
		{"unknown", identity.Identity{UserID: "u1", Role: identity.RoleUnknown}},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = p.CanList(ctx, c.caller)
			}
		})
	}
}
