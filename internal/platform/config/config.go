package config

import (
	"context"

	"go.uber.org/fx"

	"git.stit.tech/stit/golibinfra/config"
	"git.stit.tech/stit/golibinfra/httpkit"
	"git.stit.tech/stit/golibinfra/pgkit"
	"git.stit.tech/stit/golibinfra/telemetry"
)

type App struct {
	Postgres  pgkit.PoolConfig
	HTTP      httpkit.Config
	Telemetry telemetry.Config
}

// С golibinfra ≥ v0.17 ни один kit не provide'ит свой Config внутренне —
// caller обязан зарегистрировать. Здесь главные kit'ы (pgkit/httpkit/
// telemetry) идут через App-структуру — это даёт единое envconfig-чтение
// и удобное переопределение per-проект. Если используешь дополнительные
// kit'ы (logkit/authnkit/bruteforcekit/flagkit/cachekit/etc) — добавь их
// либо в App ниже, либо `fx.Provide(<kit>.NewConfig)` рядом с <kit>.Module
// в cmd/api/main.go.
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
