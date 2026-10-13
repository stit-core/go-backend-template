package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
)

// Лимит клиента — у путей /api/auth/: один bucket на все их, а остальные пути и соседний префикс его не
// делят; RPS = 0 выключает лимит.
func TestAuthRateLimit(t *testing.T) {
	t.Parallel()
	serve := func(rps, burst int) func(method, path string) int {
		mws := authRateLimit(&appconfig.App{AuthRateLimit: appconfig.AuthRateLimit{RPS: rps, Burst: burst}})
		require.Len(t, mws, 1)
		h := mws[0](http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
		return func(method, path string) int {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, path, http.NoBody))
			return rec.Code
		}
	}

	limited := serve(1, 1)
	require.Equal(t, http.StatusNoContent, limited(http.MethodPost, "/api/auth/login"))
	require.Equal(t, http.StatusTooManyRequests, limited(http.MethodPost, "/api/auth/register"), "one bucket for every path under /api/auth/")
	for _, path := range []string{"/api/users/me", "/api/authx", "/health"} {
		require.Equal(t, http.StatusNoContent, limited(http.MethodGet, path), path)
		require.Equal(t, http.StatusNoContent, limited(http.MethodGet, path), path)
	}

	off := serve(0, 0)
	for range 3 {
		require.Equal(t, http.StatusNoContent, off(http.MethodPost, "/api/auth/login"))
	}
}
