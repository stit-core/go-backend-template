package authgate_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
)

func TestClientIP(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		trustProxy bool
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{
			name:       "remote addr without port",
			remoteAddr: "203.0.113.7:51234",
			want:       "203.0.113.7",
		},
		{
			name:       "forwarded header ignored without trusted proxy",
			remoteAddr: "10.0.0.2:443",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.9"},
			want:       "10.0.0.2",
		},
		{
			name:       "forwarded header honoured behind trusted proxy",
			trustProxy: true,
			remoteAddr: "10.0.0.2:443",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.9"},
			want:       "198.51.100.9",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got string
			h := authgate.ClientIP(tc.trustProxy)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = identity.IPFrom(r.Context())
			}))

			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			h.ServeHTTP(httptest.NewRecorder(), r)

			require.Equal(t, tc.want, got)
		})
	}
}
