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

func TestTokens_AccessCarriesRole(t *testing.T) {
	t.Parallel()
	j, err := authnkit.NewHS256JWT(authnkit.JWTConfig{Secret: strings.Repeat("s", authnkit.MinSecretLen), AccessTTL: 15 * time.Minute})
	require.NoError(t, err)

	token, err := authjwt.NewTokens(j).IssueAccess(context.Background(), &auth.User{ID: "u1", Role: identity.RoleAdmin})
	require.NoError(t, err)

	claims, err := j.ParseAccess(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, "u1", claims["sub"])
	require.Equal(t, "admin", claims["role"])
}
