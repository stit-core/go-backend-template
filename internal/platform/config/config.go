package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/authlimit"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	kitconfig "git.stit.tech/stit-core/golibinfra/config"
	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

// App — конфиги самого приложения; конфиги китов читают их <kit>.NewConfig.
type App struct {
	Env           Env `envconfig:"APP_ENV" required:"true"`
	Cookie        authgate.CookieConfig
	AuthRateLimit authlimit.Config
	// PasswordResetMailPause — после ссылки сброса учётная запись столько новых писем сброса не получает; 0 — без паузы.
	PasswordResetMailPause time.Duration `default:"5m" envconfig:"PASSWORD_RESET_MAIL_PAUSE"`
}

// Env — окружение сервиса (APP_ENV): вне Dev старт не пускает небезопасную конфигурацию (Guard).
type Env string

// Dev — локальная разработка: cookie без Secure, лог-mailer, docs, логи без маскирования и токены без aud.
const Dev Env = "dev"

// ErrUnsafe — настройка, допустимая только в dev, вне dev.
var ErrUnsafe = errors.New("config: unsafe outside APP_ENV=dev")

// Unsafe — включённые настройки, допустимые только в dev, которые нашла одна проверка: «ENV=значение» на каждую;
// пусто — безопасно.
type Unsafe []string

// UnsafeGroup — группа fx проверок Unsafe: Guard собирает их все.
const UnsafeGroup = "config.unsafe"

// ProvideUnsafe кладёт в группу UnsafeGroup проверку настроек, небезопасных вне dev: check берёт конфиги из графа и
// отдаёт Unsafe. Проверку кладёт тот, кто знает настройку, — кит рядом с его конфигом в cmd/api, домен в своём
// module/: сигнатура Guard от них не растёт.
func ProvideUnsafe(check any) fx.Option {
	return fx.Provide(fx.Annotate(check, fx.ResultTags(`group:"`+UnsafeGroup+`"`)))
}

// Module — App из окружения, его Env и проверки шаблона: все здесь, а не в cmd/api, — перенос кода форка без main.go
// не снимет ни одну молча.
var Module = fx.Module(
	"internal/platform/config",
	fx.Provide(
		Load,
		func(a *App) Env { return a.Env },
	),
	ProvideUnsafe(CookieUnsafe),
	ProvideUnsafe(LogUnsafe),
	ProvideUnsafe(DocsUnsafe),
	ProvideUnsafe(JWTUnsafe),
	fx.Invoke(fx.Annotate(Guard, fx.ParamTags(``, `group:"`+UnsafeGroup+`"`))),
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

// CookieUnsafe — cookie сессии без Secure: сессия уходит по HTTP.
func CookieUnsafe(app *App) Unsafe {
	if !app.Cookie.Secure {
		return Unsafe{"COOKIE_SECURE=false"}
	}
	return nil
}

// LogUnsafe — логи без маскирования: секреты уходят в лог.
func LogUnsafe(redact logkit.RedactConfig) Unsafe {
	if redact.Disabled {
		return Unsafe{"LOG_REDACT_DISABLED=true"}
	}
	return nil
}

// DocsUnsafe — документация API: устройство API — анонимам.
func DocsUnsafe(httpCfg httpkit.Config) Unsafe {
	if httpCfg.DocsEnabled {
		return Unsafe{"HTTP_DOCS_ENABLED=true"}
	}
	return nil
}

// JWTUnsafe — подпись access-токенов годится сервису на том же JWT_SECRET (нет aud) или читателю примеров окружения.
func JWTUnsafe(jwt authnkit.JWTConfig) Unsafe {
	var unsafe Unsafe
	if len(jwt.Audience) == 0 {
		unsafe = append(unsafe, "JWT_AUDIENCE empty")
	}
	if slices.Contains(exampleJWTSecrets, jwt.Secret) {
		unsafe = append(unsafe, "JWT_SECRET from an example env")
	}
	if slices.Contains(exampleJWTSecrets, jwt.PreviousSecret) {
		unsafe = append(unsafe, "JWT_PREVIOUS_SECRET from an example env")
	}
	return unsafe
}

// Guard не даёт стартовать вне dev, пока хоть одна проверка группы UnsafeGroup нашла настройку, допустимую только в
// dev: проверки шаблона и то, что положили в группу киты и домены сервиса.
func Guard(app *App, checks []Unsafe) error {
	if app.Env == Dev {
		return nil
	}
	var unsafe []string
	for _, c := range checks {
		unsafe = append(unsafe, c...)
	}
	if len(unsafe) == 0 {
		return nil
	}
	// Порядок группы fx не задан: отказ называет настройки одним порядком.
	slices.Sort(unsafe)
	return fmt.Errorf("%w: APP_ENV=%q with %s", ErrUnsafe, app.Env, strings.Join(unsafe, ", "))
}
