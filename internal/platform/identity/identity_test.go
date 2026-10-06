package identity_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
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

// IP клиента кладёт в ctx middleware ClientIP цепочки httpkit: identity читает его, а не свой ключ.
func TestIPFrom(t *testing.T) {
	t.Parallel()
	var got string
	h := httpkit.ClientIP(logkit.Nop(), 0)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = identity.IPFrom(r.Context())
	}))
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.7:51234"
	h.ServeHTTP(httptest.NewRecorder(), r)

	require.Equal(t, "203.0.113.7", got)
	require.Empty(t, identity.IPFrom(context.Background()), "a context outside a request carries no IP")
}

func TestRequestIDFrom_EmptyContext(t *testing.T) {
	t.Parallel()

	require.Empty(t, identity.RequestIDFrom(context.Background()))
}
