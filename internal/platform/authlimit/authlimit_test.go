package authlimit_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authlimit"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

// Лимит клиента — у путей под Prefix: один bucket на все их, а остальные пути и соседний префикс его не делят;
// RPS = 0 выключает лимит.
func TestMiddleware(t *testing.T) {
	t.Parallel()
	serve := func(rps, burst int) func(method, path string) *httptest.ResponseRecorder {
		h := authlimit.Middleware(authlimit.Config{RPS: rps, Burst: burst})(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
		)
		return func(method, path string) *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, path, http.NoBody))
			return rec
		}
	}

	limited := serve(1, 1)
	require.Equal(t, http.StatusNoContent, limited(http.MethodPost, "/api/auth/login").Code)
	refused := limited(http.MethodPost, "/api/auth/register")
	require.Equal(t, http.StatusTooManyRequests, refused.Code, "one bucket for every path under the prefix")
	var body httpkit.APIError
	require.NoError(t, json.NewDecoder(refused.Body).Decode(&body))
	require.Equal(t, authlimit.Rule.Code, body.Code, "the refusal answers the code the operations declare")
	require.Equal(t, authlimit.Rule.Status, refused.Code)
	for _, path := range []string{"/api/users/me", "/api/authx", "/health"} {
		require.Equal(t, http.StatusNoContent, limited(http.MethodGet, path).Code, path)
		require.Equal(t, http.StatusNoContent, limited(http.MethodGet, path).Code, path)
	}

	off := serve(0, 0)
	for range 3 {
		require.Equal(t, http.StatusNoContent, off(http.MethodPost, "/api/auth/login").Code)
	}
}
