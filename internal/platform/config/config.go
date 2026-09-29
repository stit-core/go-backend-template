package config

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
)

// App — конфиги самого приложения; конфиги китов читают их <kit>.NewConfig.
type App struct {
	Cookie authgate.CookieConfig
}

var Module = fx.Module(
	"internal/platform/config",
	fx.Provide(
		Load,
		func(a *App) authgate.CookieConfig { return a.Cookie },
	),
)

// Load читает App из ENV тем же envconfig, что и конфиги китов.
func Load() (*App, error) {
	var cfg App
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("app config: %w", err)
	}
	return &cfg, nil
}
