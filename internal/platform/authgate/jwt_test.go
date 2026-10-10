package authgate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

var signedAt = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// accessTTL и leeway — срок access-токена и допуск часов у signer.
const (
	accessTTL = 15 * time.Minute
	leeway    = time.Minute
)

// signer — HS256-JWT кита на секрете secret с часами теста: подписывает и разбирает токены, как сервис.
func signer(t *testing.T, secret string, clock clockkit.Clock) *authnkit.HS256JWT {
	t.Helper()
	j, err := authnkit.NewHS256JWT(authnkit.JWTConfig{Secret: secret, AccessTTL: accessTTL, Leeway: leeway}, authnkit.WithClock(clock))
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
			require.Equal(t, tc.wantID.UserID, httpkit.SubjectFrom(ctx), "the httpkit subject scopes the idempotency key")
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
	clock.Advance(accessTTL + leeway + time.Second)

	_, err = extract(t, j, &http.Cookie{Name: authgate.CookieAccess, Value: token})
	var apiErr *httpkit.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusUnauthorized, apiErr.Status)
	require.Equal(t, "AUTH_ACCESS_EXPIRED", apiErr.Code)
}

// Extractor кладёт субъекта httpkit — личность запроса для идемпотентности: с подключённым
// IdempotencyStore повтор вошедшего пользователя с тем же ключом получает сохранённый ответ, тот же
// ключ другого пользователя и открытая операция (субъекта нет) исполняются.
func TestExtract_SubjectScopesIdempotency(t *testing.T) {
	t.Parallel()
	j := signer(t, secret, clockkit.NewFixed(signedAt))
	cfg, err := httpkit.NewConfig()
	require.NoError(t, err)
	srv, err := httpkit.NewServer(cfg, logkit.Nop(),
		httpkit.WithSecuritySchemes(authgate.CookieScheme(authgate.NewExtractor(j).Extract)),
		httpkit.WithIdempotencyStore(httpkit.NewMemoryStore()))
	require.NoError(t, err)
	type thingInput struct {
		Body struct {
			Name string `json:"name"`
		}
	}
	var secured, open atomic.Int32
	count := func(calls *atomic.Int32) func(context.Context, *thingInput) (*struct{}, error) {
		return func(context.Context, *thingInput) (*struct{}, error) {
			calls.Add(1)
			return nil, nil
		}
	}
	huma.Register(srv.API(), authgate.Secured(huma.Operation{OperationID: "createThing", Method: http.MethodPost, Path: "/api/things"}), count(&secured))
	huma.Register(srv.API(), httpkit.Public(huma.Operation{OperationID: "createOpenThing", Method: http.MethodPost, Path: "/api/open-things"}), count(&open))
	require.NoError(t, srv.CheckRoutes())
	api := humatest.Wrap(t, srv.API())
	post := func(path, userID string) *httptest.ResponseRecorder {
		t.Helper()
		token, err := j.SignAccess(context.Background(), map[string]any{"sub": userID, "role": "client"})
		require.NoError(t, err)
		resp := api.Post(path, cfg.Idempotency.Header+": k1", "Cookie: "+authgate.CookieAccess+"="+token, map[string]any{"name": "thing"})
		require.Equal(t, http.StatusNoContent, resp.Code, resp.Body.String())
		return resp
	}

	require.Empty(t, post("/api/things", "u1").Header().Get(httpkit.HeaderIdempotentReplayed))
	require.Equal(t, "true", post("/api/things", "u1").Header().Get(httpkit.HeaderIdempotentReplayed), "the repeat of a signed-in user is replayed")
	require.EqualValues(t, 1, secured.Load(), "the repeat does not run the operation again")
	require.Empty(t, post("/api/things", "u2").Header().Get(httpkit.HeaderIdempotentReplayed), "the key of another user is another key")
	require.EqualValues(t, 2, secured.Load())

	post("/api/open-things", "u1")
	require.Empty(t, post("/api/open-things", "u1").Header().Get(httpkit.HeaderIdempotentReplayed), "a request without a subject is not deduplicated")
	require.EqualValues(t, 2, open.Load())
}

func TestSecured(t *testing.T) {
	t.Parallel()
	op := authgate.Secured(huma.Operation{OperationID: "getThing"})
	require.Equal(t, []map[string][]string{{authgate.SecuritySchemeName: {}}}, op.Security)
	require.Contains(t, op.Errors, http.StatusUnauthorized, "the session codes are declared in OpenAPI")
}
