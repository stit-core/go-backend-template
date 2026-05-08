package config

import (
	"context"

	"go.uber.org/fx"

	"git.stit.tech/akadmin/golibinfra/config"
	"git.stit.tech/akadmin/golibinfra/httpkit"
	"git.stit.tech/akadmin/golibinfra/pgkit"
	"git.stit.tech/akadmin/golibinfra/telemetry"
)

type App struct {
	Postgres  pgkit.PoolConfig
	HTTP      httpkit.Config
	Telemetry telemetry.Config
}

var Module = fx.Module(
	"appconfig",
	fx.Provide(
		Load,

		func(a *App) pgkit.PoolConfig { return a.Postgres },
		func(a *App) httpkit.Config { return a.HTTP },
		func(a *App) telemetry.Config { return a.Telemetry },
	),
)

func Load(ctx context.Context, srcs []config.Source) (*App, error) {
	var cfg App
	if err := config.Load(ctx, &cfg, srcs...); err != nil {
		return nil, err
	}
	return &cfg, nil
}
