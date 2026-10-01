package user_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
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
	t.Parallel()
	p := user.NewPolicy()
	for _, tc := range selfOrAdminCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, p.CanGetByID(context.Background(), tc.caller, tc.targetID).IsAllowed)
		})
	}
}

func TestPolicy_CanUpdate(t *testing.T) {
	t.Parallel()
	p := user.NewPolicy()
	for _, tc := range selfOrAdminCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, p.CanUpdate(context.Background(), tc.caller, tc.targetID).IsAllowed)
		})
	}
}

func TestPolicy_CanDelete(t *testing.T) {
	t.Parallel()
	p := user.NewPolicy()
	for _, tc := range selfOrAdminCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, p.CanDelete(context.Background(), tc.caller, tc.targetID).IsAllowed)
		})
	}
}

var adminOnlyCases = []policyCase{
	{"admin self", identity.Identity{UserID: "a1", Role: identity.RoleAdmin}, "a1", true},
	{"admin other", identity.Identity{UserID: "a1", Role: identity.RoleAdmin}, "u2", true},
	{"client self", identity.Identity{UserID: "u1", Role: identity.RoleClient}, "u1", false},
	{"client other", identity.Identity{UserID: "u1", Role: identity.RoleClient}, "u2", false},
	{"unknown self", identity.Identity{UserID: "u1", Role: identity.RoleUnknown}, "u1", false},
}

func TestPolicy_CanList(t *testing.T) {
	t.Parallel()
	p := user.NewPolicy()
	for _, tc := range adminOnlyCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, p.CanList(context.Background(), tc.caller).IsAllowed)
		})
	}
}

func TestPolicy_CanChangeStatus(t *testing.T) {
	t.Parallel()
	p := user.NewPolicy()
	for _, tc := range adminOnlyCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, p.CanChangeStatus(context.Background(), tc.caller, tc.targetID).IsAllowed)
		})
	}
}
