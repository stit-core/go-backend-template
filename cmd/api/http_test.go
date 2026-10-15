package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authlimit"
	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
)

// Ответы операций доменов — с Cache-Control: no-store, и отказ строгого лимита тоже; пробы httpkit кэш не
// трогает.
func TestMiddlewares_NoStore(t *testing.T) {
	t.Parallel()
	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mws := middlewares(&appconfig.App{AuthRateLimit: authlimit.Config{RPS: 1, Burst: 1}})
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
