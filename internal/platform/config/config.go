package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	kitconfig "git.stit.tech/stit-core/golibinfra/config"
	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

// App — конфиги самого приложения; конфиги китов читают их <kit>.NewConfig.
type App struct {
	Env           Env `envconfig:"APP_ENV" required:"true"`
	Cookie        authgate.CookieConfig
	AuthRateLimit AuthRateLimit
	// PasswordResetMailPause — после ссылки сброса учётная запись столько новых писем сброса не получает; 0 — без паузы.
	PasswordResetMailPause time.Duration `default:"5m" envconfig:"PASSWORD_RESET_MAIL_PAUSE"`
}

// AuthRateLimit — token-bucket на клиента (IP, как у лимитера httpkit) для /api/auth/*: вход, регистрация,
// смена и сброс пароля считают Argon2id в общей очереди процесса, и поток одного клиента в пределах общего
// лимита занял бы её для всех. RPS = 0 выключает его: лимит держит край (прокси, WAF).
type AuthRateLimit struct {
	RPS   int `default:"5"  envconfig:"HTTP_AUTH_RATE_LIMIT_RPS"`
	Burst int `default:"10" envconfig:"HTTP_AUTH_RATE_LIMIT_BURST"`
}

// Env — окружение сервиса (APP_ENV): вне Dev старт не пускает небезопасную конфигурацию (Guard).
type Env string

// Dev — локальная разработка: cookie без Secure, лог-mailer, docs, логи без маскирования и токены без aud.
const Dev Env = "dev"

// ErrUnsafe — настройка, допустимая только в dev, вне dev.
var ErrUnsafe = errors.New("config: unsafe outside APP_ENV=dev")

var Module = fx.Module(
	"internal/platform/config",
	fx.Provide(
		Load,
		func(a *App) Env { return a.Env },
	),
	fx.Invoke(Guard),
)

// Load читает App из ENV тем же ридером, что и конфиги китов.
func Load() (*App, error) {
	var cfg App
	if err := kitconfig.Load(&cfg, kitconfig.Env()); err != nil {
		return nil, fmt.Errorf("app config: %w", err)
	}
	if cfg.AuthRateLimit.RPS < 0 || cfg.AuthRateLimit.Burst < 0 {
		return nil, errors.New("app config: HTTP_AUTH_RATE_LIMIT_RPS and HTTP_AUTH_RATE_LIMIT_BURST must be >= 0")
	}
	if cfg.PasswordResetMailPause < 0 {
		return nil, errors.New("app config: PASSWORD_RESET_MAIL_PAUSE must be >= 0")
	}
	return &cfg, nil
}

// exampleJWTSecrets — ключи подписи из примеров окружения репо (.env.example, test/stack): их знает любой читатель.
var exampleJWTSecrets = []string{
	"local-dev-only-jwt-secret-at-least-32-bytes",
	"smoke-only-secret-do-not-use-in-prod-min-32chars",
}

// Guard не даёт стартовать вне dev с настройками, которые в проде отдают сессию по HTTP, секреты — в лог, устройство
// API — анонимам, а подпись access-токенов — сервису на том же JWT_SECRET или читателю примеров окружения.
func Guard(app *App, redact logkit.RedactConfig, httpCfg httpkit.Config, jwt authnkit.JWTConfig) error {
	if app.Env == Dev {
		return nil
	}
	var unsafe []string
	if !app.Cookie.Secure {
		unsafe = append(unsafe, "COOKIE_SECURE=false")
	}
	if redact.Disabled {
		unsafe = append(unsafe, "LOG_REDACT_DISABLED=true")
	}
	if httpCfg.DocsEnabled {
		unsafe = append(unsafe, "HTTP_DOCS_ENABLED=true")
	}
	if len(jwt.Audience) == 0 {
		unsafe = append(unsafe, "JWT_AUDIENCE empty")
	}
	if slices.Contains(exampleJWTSecrets, jwt.Secret) {
		unsafe = append(unsafe, "JWT_SECRET from an example env")
	}
	if slices.Contains(exampleJWTSecrets, jwt.PreviousSecret) {
		unsafe = append(unsafe, "JWT_PREVIOUS_SECRET from an example env")
	}
	if len(unsafe) > 0 {
		return fmt.Errorf("%w: APP_ENV=%q with %s", ErrUnsafe, app.Env, strings.Join(unsafe, ", "))
	}
	return nil
}
