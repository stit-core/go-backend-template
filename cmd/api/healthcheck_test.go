package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// healthServer отвечает на GET /health статусом status, на прочие пути — 404.
func healthServer(t *testing.T, status int) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String()
}

// closedAddr — адрес, который никто не слушает.
func closedAddr(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	return srv.Listener.Addr().String()
}

func port(t *testing.T, addr string) string {
	t.Helper()
	_, p, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	return p
}

func TestCheckHealth(t *testing.T) {
	t.Parallel()
	live := healthServer(t, http.StatusOK)
	tests := []struct {
		name    string
		addr    string
		wantErr string // "" — сервис здоров
	}{
		{"listener address", live, ""},
		{"address without host is loopback", ":" + port(t, live), ""},
		{"unspecified host is loopback", "0.0.0.0:" + port(t, live), ""},
		{"health is not ok", healthServer(t, http.StatusServiceUnavailable), "503"},
		{"nobody listens", closedAddr(t), "connection refused"},
		{"address without port", "8080", "HTTP_ADDR"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkHealth(context.Background(), tc.addr)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// Код выхода — по /health адреса из HTTP_ADDR, того же, что слушает сервис. Без t.Parallel: t.Setenv.
func TestHealthcheck(t *testing.T) {
	live := healthServer(t, http.StatusOK)
	unsetHTTPEnv(t)

	t.Setenv("HTTP_ADDR", ":"+port(t, live))
	require.Equal(t, 0, healthcheck(context.Background()), "the service answers on HTTP_ADDR")

	t.Setenv("HTTP_ADDR", closedAddr(t))
	require.Equal(t, 1, healthcheck(context.Background()), "nobody answers on HTTP_ADDR")
}
