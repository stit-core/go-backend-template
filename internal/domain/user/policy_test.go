package user_test

import (
	"context"
	"testing"

	"git.stit.tech/stit-infra/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-infra/go-backend-template/internal/platform/identity"
)

type policyCase struct {
	name     string
	caller   identity.Identity
	targetID string
	want     bool
}

var selfOrAdminCases = []policyCase{
	{"admin self", identity.Identity{UserID: "a1", Role: identity.RoleAdmin}, "a1", true},
	{"admin other", identity.Identity{UserID: "a1", Role: identity.RoleAdmin}, "u2", true},
	{"client self", identity.Identity{UserID: "u1", Role: identity.RoleClient}, "u1", true},
	{"client other", identity.Identity{UserID: "u1", Role: identity.RoleClient}, "u2", false},
	{"unknown self", identity.Identity{UserID: "u1", Role: identity.RoleUnknown}, "u1", true},
	{"unknown other", identity.Identity{UserID: "u1", Role: identity.RoleUnknown}, "u2", false},
}

func TestPolicy_CanGetByID(t *testing.T) {
	p := user.NewPolicy()
	for _, tc := range selfOrAdminCases {
		t.Run(tc.name, func(t *testing.T) {
			got := p.CanGetByID(context.Background(), tc.caller, tc.targetID).IsAllowed
			if got != tc.want {
				t.Fatalf("CanGetByID: want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestPolicy_CanUpdate(t *testing.T) {
	p := user.NewPolicy()
	for _, tc := range selfOrAdminCases {
		t.Run(tc.name, func(t *testing.T) {
			got := p.CanUpdate(context.Background(), tc.caller, tc.targetID).IsAllowed
			if got != tc.want {
				t.Fatalf("CanUpdate: want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestPolicy_CanDelete(t *testing.T) {
	p := user.NewPolicy()
	for _, tc := range selfOrAdminCases {
		t.Run(tc.name, func(t *testing.T) {
			got := p.CanDelete(context.Background(), tc.caller, tc.targetID).IsAllowed
			if got != tc.want {
				t.Fatalf("CanDelete: want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestPolicy_CanList(t *testing.T) {
	p := user.NewPolicy()
	cases := []struct {
		name   string
		caller identity.Identity
		want   bool
	}{
		{"admin", identity.Identity{Role: identity.RoleAdmin}, true},
		{"client", identity.Identity{Role: identity.RoleClient}, false},
		{"unknown", identity.Identity{Role: identity.RoleUnknown}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := p.CanList(context.Background(), tc.caller).IsAllowed
			if got != tc.want {
				t.Fatalf("CanList: want %v, got %v", tc.want, got)
			}
		})
	}
}
