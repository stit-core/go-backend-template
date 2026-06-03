package config

import (
	"context"

	"go.uber.org/fx"

	"git.stit.tech/stit-infra/golibinfra/config"
	"git.stit.tech/stit-infra/golibinfra/httpkit"
	"git.stit.tech/stit-infra/golibinfra/pgkit"
	"git.stit.tech/stit-infra/golibinfra/telemetry"
	"git.stit.tech/stit-template/go-backend-template/internal/platform/authgate"
)

type App struct {
	Postgres  pgkit.PoolConfig
	HTTP      httpkit.Config
	Telemetry telemetry.Config
	Cookie    authgate.CookieConfig
}

var Module = fx.Module(
	"appconfig",
	fx.Provide(
		Load,

		func(a *App) pgkit.PoolConfig { return a.Postgres },
		func(a *App) httpkit.Config { return a.HTTP },
		func(a *App) telemetry.Config { return a.Telemetry },
		func(a *App) authgate.CookieConfig { return a.Cookie },
	),
)

func Load(ctx context.Context, srcs []config.Source) (*App, error) {
	var cfg App
	if err := config.Load(ctx, &cfg, srcs...); err != nil {
		return nil, err
	}
	return &cfg, nil
}
