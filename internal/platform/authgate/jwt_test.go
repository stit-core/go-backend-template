package authgate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

var signedAt = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// signer — HS256-JWT кита на секрете secret с часами теста: подписывает и разбирает токены, как сервис.
func signer(t *testing.T, secret string, clock clockkit.Clock) *authnkit.HS256JWT {
	t.Helper()
	j, err := authnkit.NewHS256JWT(authnkit.JWTConfig{Secret: secret}, authnkit.WithClock(clock))
	require.NoError(t, err)
	return j
}

var (
	secret      = strings.Repeat("s", authnkit.MinSecretLen)
	otherSecret = strings.Repeat("o", authnkit.MinSecretLen)
)

// extract разбирает запрос с cookie (nil — без неё) экстрактором на parser.
func extract(t *testing.T, parser authnkit.JWTParser, cookie *http.Cookie) (context.Context, error) {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	return authgate.NewExtractor(parser).Extract(r)
}

func TestExtract(t *testing.T) {
	t.Parallel()
	clock := clockkit.NewFixed(signedAt)
	j := signer(t, secret, clock)
	sign := func(t *testing.T, claims map[string]any) string {
		t.Helper()
		token, err := j.SignAccess(context.Background(), claims)
		require.NoError(t, err)
		return token
	}

	cases := []struct {
		name            string
		token           func(t *testing.T) string // nil — запрос без cookie
		wantErrContains string
		wantErrIs       error
		wantID          identity.Identity
	}{
		{
			name:            "no auth cookie",
			wantErrContains: "no auth cookie",
		},
		{
			name:            "empty cookie value",
			token:           func(*testing.T) string { return "" },
			wantErrContains: "no auth cookie",
		},
		{
			name: "token of another secret",
			token: func(t *testing.T) string {
				token, err := signer(t, otherSecret, clock).SignAccess(context.Background(), map[string]any{"sub": "u1", "role": "admin"})
				require.NoError(t, err)
				return token
			},
			wantErrIs: authnkit.ErrTokenSignature,
		},
		{
			// Токен на том же секрете, но не access (нет typ=access): сессию по нему не открывают.
			name: "signed token that is not an access token",
			token: func(t *testing.T) string {
				token, err := j.Sign(context.Background(), map[string]any{"sub": "u1", "role": "admin"}, time.Hour)
				require.NoError(t, err)
				return token
			},
			wantErrIs: authnkit.ErrWrongTokenType,
		},
		{
			name:   "valid access token populates identity",
			token:  func(t *testing.T) string { return sign(t, map[string]any{"sub": "u1", "role": "admin"}) },
			wantID: identity.Identity{UserID: "u1", Role: identity.RoleAdmin},
		},
		{
			name:      "non-string subject is malformed",
			token:     func(t *testing.T) string { return sign(t, map[string]any{"sub": 42, "role": "admin"}) },
			wantErrIs: authnkit.ErrTokenMalformed,
		},
		{
			name:   "non-string role degrades to unknown",
			token:  func(t *testing.T) string { return sign(t, map[string]any{"sub": "u1", "role": true}) },
			wantID: identity.Identity{UserID: "u1", Role: identity.RoleUnknown},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var cookie *http.Cookie
			if tc.token != nil {
				cookie = &http.Cookie{Name: authgate.CookieAccess, Value: tc.token(t)}
			}

			ctx, err := extract(t, j, cookie)

			if tc.wantErrContains != "" || tc.wantErrIs != nil {
				require.Error(t, err)
				if tc.wantErrContains != "" {
					require.ErrorContains(t, err, tc.wantErrContains)
				}
				if tc.wantErrIs != nil {
					require.ErrorIs(t, err, tc.wantErrIs)
				}
				_, ok := identity.From(ctx)
				require.False(t, ok, "identity must not be set when extraction fails")
				return
			}

			require.NoError(t, err)
			got, ok := identity.From(ctx)
			require.True(t, ok, "identity must be set on the returned context")
			require.Equal(t, tc.wantID, got)
		})
	}
}

// Истёкший access — свой код 401: клиент обновляет сессию, а не входит заново.
func TestExtract_AccessExpired(t *testing.T) {
	t.Parallel()
	clock := clockkit.NewTickable(signedAt)
	j := signer(t, secret, clock)
	token, err := j.SignAccess(context.Background(), map[string]any{"sub": "u1", "role": "client"})
	require.NoError(t, err)
	clock.Advance(authnkit.DefaultAccessTTL + authnkit.DefaultLeeway + time.Second)

	_, err = extract(t, j, &http.Cookie{Name: authgate.CookieAccess, Value: token})
	var apiErr *httpkit.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusUnauthorized, apiErr.Status)
	require.Equal(t, "AUTH_ACCESS_EXPIRED", apiErr.Code)
}

func TestSecured(t *testing.T) {
	t.Parallel()
	op := authgate.Secured(huma.Operation{OperationID: "getThing"})
	require.Equal(t, []map[string][]string{{authgate.SecuritySchemeName: {}}}, op.Security)
	require.Contains(t, op.Errors, http.StatusUnauthorized, "the session codes are declared in OpenAPI")
}
