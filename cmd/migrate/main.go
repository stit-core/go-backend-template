package main

import (
	"context"
	"os"

	"git.stit.tech/stit/golibinfra/logkit"
	"git.stit.tech/stit/golibinfra/pgkit"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	log := logkit.New(logkit.Config{Level: "info"})
	ctx := context.Background()
	log.Info(ctx, "starting", "service", "migrate", "version", version, "commit", commit)

	if len(os.Args) < 2 {
		log.Error(ctx, "usage: migrate <up|down|version>")
		os.Exit(1)
	}

	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		log.Error(ctx, "PG_DSN is required")
		os.Exit(1)
	}

	path := os.Getenv("PG_MIGRATIONS_PATH")
	if path == "" {
		path = "/migrations"
	}

	cfg := pgkit.MigrateConfig{DSN: dsn, Path: path}

	switch os.Args[1] {
	case "up":
		if err := pgkit.MigrateUp(cfg); err != nil {
			log.Error(ctx, "migrate up failed", "err", err)
			os.Exit(1)
		}
		log.Info(ctx, "migrate up: ok")

	case "down":
		if err := pgkit.MigrateDown(cfg); err != nil {
			log.Error(ctx, "migrate down failed", "err", err)
			os.Exit(1)
		}
		log.Info(ctx, "migrate down: ok")

	case "version":
		v, dirty, err := pgkit.MigrateVersion(cfg)
		if err != nil {
			log.Error(ctx, "migrate version failed", "err", err)
			os.Exit(1)
		}
		if v == 0 && !dirty {
			log.Info(ctx, "no migrations applied")
		} else {
			log.Info(ctx, "migrate version", "version", v, "dirty", dirty)
		}

	default:
		log.Error(ctx, "unknown command", "command", os.Args[1])
		os.Exit(1)
	}
}
