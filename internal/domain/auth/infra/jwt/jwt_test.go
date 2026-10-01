package jwt_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	authjwt "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/infra/jwt"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
)

var fixedNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func newJWT(t *testing.T, cfg authnkit.JWTConfig, now time.Time) *authnkit.HS256JWT {
	t.Helper()
	cfg.Secret = strings.Repeat("s", authnkit.MinSecretLen)
	j, err := authnkit.NewHS256JWT(cfg, authnkit.WithClock(clockkit.NewFixed(now)))
	require.NoError(t, err)
	return j
}

func TestTokens_RefreshRoundTrip(t *testing.T) {
	t.Parallel()
	cfg := authnkit.JWTConfig{RefreshTTL: time.Hour}
	j := newJWT(t, cfg, fixedNow)
	tokens := authjwt.NewTokens(j, cfg, clockkit.NewFixed(fixedNow))
	parser := authjwt.NewTokenParser(j)

	token, jti, expiresAt, err := tokens.IssueRefresh(context.Background(), "u1")
	require.NoError(t, err)
	require.NotEmpty(t, jti)
	require.Equal(t, fixedNow.Add(time.Hour), expiresAt)

	userID, gotJTI, err := parser.ParseRefresh(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, "u1", userID)
	require.Equal(t, jti, gotJTI)
}

// Нулевой JWT_REFRESH_TTL — дефолт кита: expires_at строки сессии совпадает со сроком токена.
func TestTokens_DefaultRefreshTTL(t *testing.T) {
	t.Parallel()
	j := newJWT(t, authnkit.JWTConfig{}, fixedNow)
	_, _, expiresAt, err := authjwt.NewTokens(j, authnkit.JWTConfig{}, clockkit.NewFixed(fixedNow)).
		IssueRefresh(context.Background(), "u1")
	require.NoError(t, err)
	require.Equal(t, fixedNow.Add(authnkit.DefaultRefreshTTL), expiresAt)
}

func TestTokens_AccessCarriesRole(t *testing.T) {
	t.Parallel()
	j := newJWT(t, authnkit.JWTConfig{}, fixedNow)
	token, err := authjwt.NewTokens(j, authnkit.JWTConfig{}, clockkit.NewFixed(fixedNow)).
		IssueAccess(context.Background(), &auth.User{ID: "u1", Role: identity.RoleAdmin})
	require.NoError(t, err)

	claims, err := j.ParseAccess(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, "u1", claims["sub"])
	require.Equal(t, "admin", claims["role"])
}

func TestTokenParser_Rejects(t *testing.T) {
	t.Parallel()
	j := newJWT(t, authnkit.JWTConfig{}, fixedNow)
	access, err := j.SignAccess(context.Background(), map[string]any{"sub": "u1", "jti": "j1"})
	require.NoError(t, err)
	noJTI, err := j.SignRefresh(context.Background(), map[string]any{"sub": "u1"})
	require.NoError(t, err)
	noSub, err := j.SignRefresh(context.Background(), map[string]any{"jti": "j1"})
	require.NoError(t, err)
	expired, err := newJWT(t, authnkit.JWTConfig{RefreshTTL: time.Minute}, fixedNow.Add(-time.Hour)).
		SignRefresh(context.Background(), map[string]any{"sub": "u1", "jti": "j1"})
	require.NoError(t, err)

	tests := []struct {
		name    string
		token   string
		expired bool
	}{
		{"access token", access, false},
		{"no jti", noJTI, false},
		{"no sub", noSub, false},
		{"garbage", "not-a-jwt", false},
		{"expired", expired, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := authjwt.NewTokenParser(j).ParseRefresh(context.Background(), tc.token)
			require.Error(t, err)
			require.Equal(t, tc.expired, errors.Is(err, auth.ErrTokenExpired), "only an expired token is ErrTokenExpired: %v", err)
		})
	}
}
