package auth_test

import (
	"context"
	"testing"

	"git.stit.tech/akadmin/go-backend-template/internal/domain/auth"
	"git.stit.tech/akadmin/go-backend-template/internal/platform/identity"
)

func TestPolicy_CanChangePassword(t *testing.T) {
	p := auth.NewPolicy()
	cases := []struct {
		name     string
		caller   identity.Identity
		targetID string
		want     bool
	}{
		{"admin self", identity.Identity{UserID: "a1", Role: identity.RoleAdmin}, "a1", true},
		{"admin other", identity.Identity{UserID: "a1", Role: identity.RoleAdmin}, "u2", true},
		{"client self", identity.Identity{UserID: "u1", Role: identity.RoleClient}, "u1", true},
		{"client other", identity.Identity{UserID: "u1", Role: identity.RoleClient}, "u2", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := p.CanChangePassword(context.Background(), tc.caller, tc.targetID).IsAllowed
			if got != tc.want {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}
}
