package jwt_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	authjwt "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/infra/jwt"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/golibinfra/authnkit"
)

// Роль claim'а — роль учётной записи: по ней authgate собирает личность, а policy решает права.
func TestTokens_AccessCarriesRole(t *testing.T) {
	t.Parallel()
	j, err := authnkit.NewHS256JWT(authnkit.JWTConfig{Secret: strings.Repeat("s", authnkit.MinSecretLen), AccessTTL: 15 * time.Minute})
	require.NoError(t, err)
	for _, tc := range []struct {
		role identity.Role
		want string
	}{
		{identity.RoleClient, "client"},
		{identity.RoleAdmin, "admin"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			token, err := authjwt.NewTokens(j).IssueAccess(context.Background(), &auth.User{ID: "u1", Role: tc.role})
			require.NoError(t, err)

			claims, err := j.ParseAccess(context.Background(), token)
			require.NoError(t, err)
			require.Equal(t, "u1", claims["sub"])
			require.Equal(t, tc.want, claims["role"])
		})
	}
}
