package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

// setenv выставляет переменную на время теста; "" — переменной нет.
func setenv(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value)
	if value == "" {
		require.NoError(t, os.Unsetenv(key))
	}
}

// Без t.Parallel: t.Setenv меняет окружение процесса.
func TestLoad(t *testing.T) {
	cases := []struct {
		name       string
		env        string
		secure     string
		wantErr    bool
		wantSecure bool
	}{
		{name: "secure cookie by default", env: "prod", wantSecure: true},
		{name: "insecure cookie from env", env: "dev", secure: "false"},
		{name: "APP_ENV is required", wantErr: true},
		{name: "invalid cookie flag fails", env: "dev", secure: "maybe", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "APP_ENV", tc.env)
			setenv(t, "COOKIE_SECURE", tc.secure)

			got, err := appconfig.Load()

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, appconfig.Env(tc.env), got.Env)
			require.Equal(t, tc.wantSecure, got.Cookie.Secure)
		})
	}
}

func TestGuard(t *testing.T) {
	t.Parallel()
	app := func(env appconfig.Env, secure bool) *appconfig.App {
		a := &appconfig.App{Env: env}
		a.Cookie.Secure = secure
		return a
	}

	cases := []struct {
		name    string
		app     *appconfig.App
		redact  logkit.RedactConfig
		http    httpkit.Config
		wantErr string // "" — старт разрешён
	}{
		{name: "prod with safe settings", app: app("prod", true)},
		{name: "dev allows everything unsafe", app: app(appconfig.Dev, false), redact: logkit.RedactConfig{Disabled: true}, http: httpkit.Config{DocsEnabled: true}},
		{name: "insecure cookie outside dev", app: app("prod", false), wantErr: "COOKIE_SECURE=false"},
		{name: "log masking off outside dev", app: app("staging", true), redact: logkit.RedactConfig{Disabled: true}, wantErr: "LOG_REDACT_DISABLED=true"},
		{name: "docs outside dev", app: app("prod", true), http: httpkit.Config{DocsEnabled: true}, wantErr: "HTTP_DOCS_ENABLED=true"},
		{name: "any env but dev is strict", app: app("Dev", false), wantErr: `APP_ENV="Dev"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := appconfig.Guard(tc.app, tc.redact, tc.http)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, appconfig.ErrUnsafe)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// Модуль сам вызывает Guard: вне dev сервис с небезопасной конфигурацией не собирается.
func TestModule(t *testing.T) {
	cases := []struct {
		name, env string
		wantErr   error
	}{
		{name: "docs on outside dev", env: "prod", wantErr: appconfig.ErrUnsafe},
		{name: "docs on in dev", env: "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "APP_ENV", tc.env)
			setenv(t, "COOKIE_SECURE", "")

			app := fx.New(fx.NopLogger, appconfig.Module,
				fx.Supply(logkit.RedactConfig{}, httpkit.Config{DocsEnabled: true}))

			require.ErrorIs(t, app.Err(), tc.wantErr)
		})
	}
}
