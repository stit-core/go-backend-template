package identity_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
)

func TestWithFrom(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		id   identity.Identity
	}{
		{name: "admin role", id: identity.Identity{UserID: "u1", Role: identity.RoleAdmin}},
		{name: "client role", id: identity.Identity{UserID: "u2", Role: identity.RoleClient}},
		{name: "unknown role", id: identity.Identity{UserID: "u3", Role: identity.RoleUnknown}},
		{name: "zero identity", id: identity.Identity{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := identity.With(context.Background(), tc.id)

			got, ok := identity.From(ctx)
			require.True(t, ok, "identity must be present after With")
			require.Equal(t, tc.id, got)
		})
	}
}

func TestFrom_EmptyContext(t *testing.T) {
	t.Parallel()

	_, ok := identity.From(context.Background())
	require.False(t, ok, "bare context must not carry identity")
}

func TestIPFrom(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{name: "returns stored ip", ctx: identity.WithIP(context.Background(), "10.0.0.1"), want: "10.0.0.1"},
		{name: "empty on bare context", ctx: context.Background(), want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, identity.IPFrom(tc.ctx))
		})
	}
}

func TestRequestIDFrom_EmptyContext(t *testing.T) {
	t.Parallel()

	require.Empty(t, identity.RequestIDFrom(context.Background()))
}
