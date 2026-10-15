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
		h := authRateLimit(appconfig.AuthRateLimit{RPS: rps, Burst: burst})(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
		)
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

// Ответы операций доменов — с Cache-Control: no-store, и отказ строгого лимита тоже; пробы httpkit кэш не
// трогает.
func TestMiddlewares_NoStore(t *testing.T) {
	t.Parallel()
	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mws := middlewares(&appconfig.App{AuthRateLimit: appconfig.AuthRateLimit{RPS: 1, Burst: 1}})
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	for _, tc := range []struct {
		path        string
		status      int
		cacheHeader string
	}{
		{"/api/users/me", http.StatusOK, "no-store"},
		{"/api/auth/login", http.StatusOK, "no-store"},
		{"/api/auth/login", http.StatusTooManyRequests, "no-store"},
		{"/health", http.StatusOK, ""},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.path, http.NoBody))
		require.Equal(t, tc.status, rec.Code, tc.path)
		require.Equal(t, tc.cacheHeader, rec.Header().Get("Cache-Control"), tc.path)
	}
}
