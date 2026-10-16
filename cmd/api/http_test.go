package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// Ответы операций доменов — с Cache-Control: no-store, пробы httpkit кэш не трогает.
func TestNoStore(t *testing.T) {
	t.Parallel()
	h := noStore(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	for path, want := range map[string]string{"/api/orders/1": "no-store", "/health": ""} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody))
		require.Equal(t, http.StatusOK, rec.Code, path)
		require.Equal(t, want, rec.Header().Get("Cache-Control"), path)
	}
}
