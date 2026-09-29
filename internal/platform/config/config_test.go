package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
)

// Без t.Parallel: t.Setenv меняет окружение процесса.
func TestLoad(t *testing.T) {
	cases := []struct {
		name       string
		secure     string // "" — переменной нет
		wantErr    bool
		wantSecure bool
	}{
		{name: "defaults without env"},
		{name: "cookie secure from env", secure: "true", wantSecure: true},
		{name: "invalid value fails", secure: "maybe", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("COOKIE_SECURE", tc.secure)
			if tc.secure == "" {
				require.NoError(t, os.Unsetenv("COOKIE_SECURE"))
			}

			got, err := appconfig.Load()

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantSecure, got.Cookie.Secure)
		})
	}
}
