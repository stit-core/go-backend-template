package authgate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

type stubParser struct {
	claims map[string]any
	err    error
}

func (s stubParser) Parse(context.Context, string) (map[string]any, error) { return s.claims, s.err }

func (s stubParser) ParseAccess(context.Context, string) (map[string]any, error) {
	return s.claims, s.err
}

func TestExtract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		cookie          *http.Cookie
		parser          stubParser
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
			cookie:          &http.Cookie{Name: authgate.CookieAccess, Value: ""},
			wantErrContains: "no auth cookie",
		},
		{
			name:      "parser failure is wrapped",
			cookie:    &http.Cookie{Name: authgate.CookieAccess, Value: "forged-token"},
			parser:    stubParser{err: authnkit.ErrTokenSignature},
			wantErrIs: authnkit.ErrTokenSignature,
		},
		{
			name:   "valid claims populate identity",
			cookie: &http.Cookie{Name: authgate.CookieAccess, Value: "good-token"},
			parser: stubParser{claims: map[string]any{"sub": "u1", "role": "admin"}},
			wantID: identity.Identity{UserID: "u1", Role: identity.RoleAdmin},
		},
		{
			name:   "non-string claims degrade to empty identity",
			cookie: &http.Cookie{Name: authgate.CookieAccess, Value: "good-token"},
			parser: stubParser{claims: map[string]any{"sub": 42, "role": true}},
			wantID: identity.Identity{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			if tc.cookie != nil {
				r.AddCookie(tc.cookie)
			}

			ctx, err := authgate.NewExtractor(tc.parser).Extract(r)

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
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: authgate.CookieAccess, Value: "expired-token"})

	_, err := authgate.NewExtractor(stubParser{err: authnkit.ErrTokenExpired}).Extract(r)
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
