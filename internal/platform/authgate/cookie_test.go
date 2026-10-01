package authgate_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
)

func TestBuildAuthCookies(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  authgate.CookieConfig
	}{
		{name: "insecure config", cfg: authgate.CookieConfig{Secure: false}},
		{name: "secure config", cfg: authgate.CookieConfig{Secure: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := authgate.BuildAuthCookies("access-value", "refresh-value", tc.cfg)

			require.Len(t, got, 2)
			require.Equal(t, authgate.CookieAccess, got[0].Name)
			require.Equal(t, "access-value", got[0].Value)
			require.Equal(t, authgate.CookieRefresh, got[1].Name)
			require.Equal(t, "refresh-value", got[1].Value)

			for _, c := range got {
				require.Equal(t, "/", c.Path)
				require.True(t, c.HttpOnly)
				require.Equal(t, http.SameSiteStrictMode, c.SameSite)
				require.Equal(t, tc.cfg.Secure, c.Secure)
				require.Zero(t, c.MaxAge, "auth cookies must be session cookies without Max-Age")
				require.True(t, c.Expires.IsZero(), "auth cookies must not set Expires")
			}
		})
	}
}

func TestBuildExpiredAuthCookies(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  authgate.CookieConfig
	}{
		{name: "insecure config", cfg: authgate.CookieConfig{Secure: false}},
		{name: "secure config", cfg: authgate.CookieConfig{Secure: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := authgate.BuildExpiredAuthCookies(tc.cfg)

			require.Len(t, got, 2)
			require.Equal(t, authgate.CookieAccess, got[0].Name)
			require.Equal(t, authgate.CookieRefresh, got[1].Name)

			for _, c := range got {
				require.Empty(t, c.Value)
				require.Equal(t, "/", c.Path)
				require.True(t, c.HttpOnly)
				require.Equal(t, http.SameSiteStrictMode, c.SameSite)
				require.Equal(t, tc.cfg.Secure, c.Secure)
				require.Equal(t, -1, c.MaxAge, "expired cookies must instruct the browser to drop them")
			}
		})
	}
}
