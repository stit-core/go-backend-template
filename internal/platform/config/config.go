package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

// App — конфиги самого приложения; конфиги китов читают их <kit>.NewConfig.
type App struct {
	Env    Env `envconfig:"APP_ENV" required:"true"`
	Cookie authgate.CookieConfig
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
		func(a *App) authgate.CookieConfig { return a.Cookie },
		func(a *App) Env { return a.Env },
	),
	fx.Invoke(Guard),
)

// Load читает App из ENV тем же envconfig, что и конфиги китов.
func Load() (*App, error) {
	var cfg App
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("app config: %w", err)
	}
	return &cfg, nil
}

// Guard не даёт стартовать вне dev с настройками, которые в проде отдают сессию по HTTP, секреты —
// в лог, устройство API — анонимам, а access-токены — любому сервису на том же JWT_SECRET.
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
	if len(unsafe) > 0 {
		return fmt.Errorf("%w: APP_ENV=%q with %s", ErrUnsafe, app.Env, strings.Join(unsafe, ", "))
	}
	return nil
}
