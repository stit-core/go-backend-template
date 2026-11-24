package config_test

import (
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authlimit"
	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
	"git.stit.tech/stit-core/golibinfra/authnkit"
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

// Лимит /api/auth/* по умолчанию — 5 запросов в секунду и запас 10 на клиента; отрицательное значение — отказ
// старта, а не молча выключенный лимит.
func TestLoad_AuthRateLimit(t *testing.T) {
	cases := []struct {
		name, rps, burst string
		want             authlimit.Config
		wantErr          bool
	}{
		{name: "defaults", want: authlimit.Config{RPS: 5, Burst: 10}},
		{name: "from env", rps: "2", burst: "4", want: authlimit.Config{RPS: 2, Burst: 4}},
		{name: "off", rps: "0", want: authlimit.Config{RPS: 0, Burst: 10}},
		{name: "negative rate", rps: "-1", wantErr: true},
		{name: "negative burst", burst: "-1", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "APP_ENV", "dev")
			setenv(t, "COOKIE_SECURE", "")
			setenv(t, "HTTP_AUTH_RATE_LIMIT_RPS", tc.rps)
			setenv(t, "HTTP_AUTH_RATE_LIMIT_BURST", tc.burst)

			got, err := appconfig.Load()
			if tc.wantErr {
				require.ErrorContains(t, err, "HTTP_AUTH_RATE_LIMIT_RPS and HTTP_AUTH_RATE_LIMIT_BURST must be >= 0")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got.AuthRateLimit)
		})
	}
}

// Пауза писем сброса домена verification — 5 минут по умолчанию, 0 — без паузы, отрицательная — отказ старта.
func TestLoad_PasswordResetMailPause(t *testing.T) {
	cases := []struct {
		name, pause string
		want        time.Duration
		wantErr     bool
	}{
		{name: "default", want: 5 * time.Minute},
		{name: "from env", pause: "90s", want: 90 * time.Second},
		{name: "off", pause: "0s"},
		{name: "negative", pause: "-1s", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "APP_ENV", "dev")
			setenv(t, "COOKIE_SECURE", "")
			setenv(t, "PASSWORD_RESET_MAIL_PAUSE", tc.pause)

			got, err := appconfig.Load()
			if tc.wantErr {
				require.ErrorContains(t, err, "PASSWORD_RESET_MAIL_PAUSE must be >= 0")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got.PasswordResetMailPause)
		})
	}
}

func TestGuard(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		env     appconfig.Env
		checks  []appconfig.Unsafe
		wantErr string // "" — старт разрешён
	}{
		{name: "prod without checks", env: "prod"},
		{name: "prod with safe settings", env: "prod", checks: []appconfig.Unsafe{nil, {}}},
		{name: "dev allows everything unsafe", env: appconfig.Dev, checks: []appconfig.Unsafe{{"COOKIE_SECURE=false"}, {"LOG_REDACT_DISABLED=true"}}},
		{name: "unsafe outside dev", env: "staging", checks: []appconfig.Unsafe{nil, {"HTTP_DOCS_ENABLED=true"}}, wantErr: `APP_ENV="staging" with HTTP_DOCS_ENABLED=true`},
		{
			name: "every check in one order", env: "prod",
			checks:  []appconfig.Unsafe{{"Z_INSECURE=true"}, {"LOG_REDACT_DISABLED=true", "A_INSECURE=true"}},
			wantErr: "with A_INSECURE=true, LOG_REDACT_DISABLED=true, Z_INSECURE=true",
		},
		{name: "any env but dev is strict", env: "Dev", checks: []appconfig.Unsafe{{"HTTP_DOCS_ENABLED=true"}}, wantErr: `APP_ENV="Dev"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := appconfig.Guard(&appconfig.App{Env: tc.env}, tc.checks)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, appconfig.ErrUnsafe)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// Проверки config.Module: cookie без Secure, логи без маскирования, документация API, access-токены без aud.
func TestUnsafeChecks(t *testing.T) {
	t.Parallel()
	require.Equal(t, appconfig.Unsafe{"COOKIE_SECURE=false"}, appconfig.CookieUnsafe(&appconfig.App{}))
	require.Empty(t, appconfig.CookieUnsafe(&appconfig.App{Cookie: authgate.CookieConfig{Secure: true}}))
	require.Equal(t, appconfig.Unsafe{"LOG_REDACT_DISABLED=true"}, appconfig.LogUnsafe(logkit.RedactConfig{Disabled: true}))
	require.Empty(t, appconfig.LogUnsafe(logkit.RedactConfig{}))
	require.Equal(t, appconfig.Unsafe{"HTTP_DOCS_ENABLED=true"}, appconfig.DocsUnsafe(httpkit.Config{DocsEnabled: true}))
	require.Empty(t, appconfig.DocsUnsafe(httpkit.Config{}))
	require.Equal(t, appconfig.Unsafe{"JWT_AUDIENCE empty"}, appconfig.JWTUnsafe(authnkit.JWTConfig{}))
	require.Empty(t, appconfig.JWTUnsafe(authnkit.JWTConfig{Audience: []string{"backend"}}))
}

// Ключ подписи из примеров окружения репо знает каждый читатель репо: вне dev он не пускает старт ни текущим, ни
// прежним ключом. Значения берутся из самих файлов — пример с новым ключом без строки в JWTUnsafe роняет тест.
func TestJWTUnsafe_ExampleSecrets(t *testing.T) {
	t.Parallel()
	secrets := exampleSecrets(t)
	require.Len(t, secrets, 2, "JWT_SECRET of .env.example and of test/stack/compose.yml")
	aud := []string{"backend"}
	for _, secret := range secrets {
		require.Equal(t, appconfig.Unsafe{"JWT_SECRET from an example env"},
			appconfig.JWTUnsafe(authnkit.JWTConfig{Secret: secret, Audience: aud}), "secret %q", secret)
		require.Equal(t, appconfig.Unsafe{"JWT_PREVIOUS_SECRET from an example env"},
			appconfig.JWTUnsafe(authnkit.JWTConfig{Secret: "a-production-secret-of-at-least-32-bytes", PreviousSecret: secret, Audience: aud}),
			"previous secret %q", secret)
	}
}

// exampleSecrets — значения JWT_SECRET в .env.example и в env стенда e2e.
func exampleSecrets(t *testing.T) []string {
	t.Helper()
	var secrets []string
	for file, line := range map[string]*regexp.Regexp{
		"../../../.env.example":           regexp.MustCompile(`(?m)^JWT_SECRET=(\S+)\s*$`),
		"../../../test/stack/compose.yml": regexp.MustCompile(`(?m)^\s+JWT_SECRET:\s*(\S+)\s*$`),
	} {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		m := line.FindSubmatch(data)
		require.NotNil(t, m, "%s sets JWT_SECRET", file)
		secrets = append(secrets, string(m[1]))
	}
	return secrets
}

// Модуль сам вызывает Guard со своими проверками и проверками группы извне (кит, домен): вне dev сервис с небезопасной
// конфигурацией не собирается.
func TestModule(t *testing.T) {
	aud := authnkit.JWTConfig{Audience: []string{"backend"}}
	insecure := appconfig.ProvideUnsafe(func() appconfig.Unsafe { return appconfig.Unsafe{"PROBE_INSECURE=true"} })
	cases := []struct {
		name, env, secure string
		opts              []fx.Option
		wantErr           string // "" — сервис собирается
	}{
		{name: "safe outside dev", env: "prod", opts: []fx.Option{fx.Supply(logkit.RedactConfig{}, httpkit.Config{}, aud)}},
		{name: "insecure cookie outside dev", env: "prod", secure: "false", opts: []fx.Option{fx.Supply(logkit.RedactConfig{}, httpkit.Config{}, aud)}, wantErr: "with COOKIE_SECURE=false"},
		{name: "log masking off outside dev", env: "prod", opts: []fx.Option{fx.Supply(logkit.RedactConfig{Disabled: true}, httpkit.Config{}, aud)}, wantErr: "with LOG_REDACT_DISABLED=true"},
		{name: "docs on outside dev", env: "prod", opts: []fx.Option{fx.Supply(logkit.RedactConfig{}, httpkit.Config{DocsEnabled: true}, aud)}, wantErr: "with HTTP_DOCS_ENABLED=true"},
		{name: "tokens without audience outside dev", env: "prod", opts: []fx.Option{fx.Supply(logkit.RedactConfig{}, httpkit.Config{}, authnkit.JWTConfig{})}, wantErr: "with JWT_AUDIENCE empty"},
		{name: "a check from outside the module", env: "prod", opts: []fx.Option{fx.Supply(logkit.RedactConfig{}, httpkit.Config{}, aud), insecure}, wantErr: "with PROBE_INSECURE=true"},
		{name: "everything in dev", env: "dev", secure: "false", opts: []fx.Option{fx.Supply(logkit.RedactConfig{Disabled: true}, httpkit.Config{DocsEnabled: true}, authnkit.JWTConfig{}), insecure}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "APP_ENV", tc.env)
			setenv(t, "COOKIE_SECURE", tc.secure)

			app := fx.New(append([]fx.Option{fx.NopLogger, appconfig.Module}, tc.opts...)...)

			if tc.wantErr == "" {
				require.NoError(t, app.Err())
				return
			}
			require.ErrorIs(t, app.Err(), appconfig.ErrUnsafe)
			require.ErrorContains(t, app.Err(), tc.wantErr)
		})
	}
}

// Проверка не того вида — ошибка графа, а не проверка мимо Guard: результат []string fx положил бы в другую группу.
func TestProvideUnsafe_RejectsNotACheck(t *testing.T) {
	t.Parallel()
	for name, check := range map[string]any{
		"another result type": func() []string { return []string{"PROBE_INSECURE=true"} },
		"no result":           func() {},
		"not a function":      appconfig.Unsafe{"PROBE_INSECURE=true"},
		"nil":                 nil,
	} {
		err := fx.New(fx.NopLogger, appconfig.ProvideUnsafe(check)).Err()
		require.ErrorContains(t, err, "the check must be a function returning config.Unsafe", name)
	}
}

// APP_ENV= (задан, но пуст) — не dev, а незаданное окружение: сервис не собирается.
func TestModule_EmptyEnvIsNotDev(t *testing.T) {
	t.Setenv("APP_ENV", "")
	setenv(t, "COOKIE_SECURE", "")

	app := fx.New(fx.NopLogger, appconfig.Module,
		fx.Supply(logkit.RedactConfig{}, httpkit.Config{DocsEnabled: true}, authnkit.JWTConfig{Audience: []string{"backend"}}))

	require.ErrorContains(t, app.Err(), "APP_ENV is required")
}

// Модуль отдаёт Env окружения — его берут платформа и модули доменов.
func TestModule_ProvidesEnv(t *testing.T) {
	setenv(t, "APP_ENV", "dev")
	setenv(t, "COOKIE_SECURE", "")
	var env appconfig.Env

	app := fx.New(fx.NopLogger, appconfig.Module,
		fx.Supply(logkit.RedactConfig{}, httpkit.Config{}, authnkit.JWTConfig{}), fx.Populate(&env))

	require.NoError(t, app.Err())
	require.Equal(t, appconfig.Dev, env)
}
