package main

import (
	"context"
	"os"

	auditpg "git.stit.tech/stit-core/golibinfra/auditkit/pgstore"
	bruteforcepg "git.stit.tech/stit-core/golibinfra/bruteforcekit/pgstore"
	"git.stit.tech/stit-core/golibinfra/logkit"
	outboxpg "git.stit.tech/stit-core/golibinfra/outboxkit/pgstore"
	"git.stit.tech/stit-core/golibinfra/pgkit"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	ctx := context.Background()

	// Stdout-логгер (JSON в docker/kubectl logs). Stdout в struct-литерале
	// задаём явно: zero-value был бы false.
	logCfg := logkit.Config{Level: "info", Stdout: true}
	log := logkit.NewWithWriter(logCfg, os.Stdout)
	log.Info(ctx, "starting", "component", "migrate", "version", version, "commit", commit)

	os.Exit(run(ctx, log))
}

func run(ctx context.Context, log logkit.Logger) int {
	if len(os.Args) < 2 {
		log.Error(ctx, "usage: migrate <up|down|version>")
		return 1
	}

	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		log.Error(ctx, "PG_DSN is required")
		return 1
	}

	path := os.Getenv("PG_MIGRATIONS_PATH")
	if path == "" {
		path = "/migrations"
	}

	// Порядок Sources append-only — новые kit'ы добавляются в КОНЕЦ после "app",
	// чтобы не сдвигать существующие version'ы в schema_migrations.
	cfg := pgkit.MultiSourceConfig{
		DSN: dsn,
		Sources: []pgkit.Source{
			{Name: "bruteforcekit", FS: bruteforcepg.MigrationsFS, Prefix: "migrations"},
			{Name: "auditkit", FS: auditpg.MigrationsFS, Prefix: "migrations"},
			{Name: "outboxkit", FS: outboxpg.MigrationsFS, Prefix: "migrations"},
			{Name: "app", FS: os.DirFS(path), Prefix: "."},
		},
	}

	switch os.Args[1] {
	case "up":
		if err := pgkit.MigrateUpMulti(cfg); err != nil {
			log.Error(ctx, "migrate up failed", "err", err)
			return 1
		}
		log.Info(ctx, "migrate up: ok")

	case "down":
		if err := pgkit.MigrateDownMulti(cfg); err != nil {
			log.Error(ctx, "migrate down failed", "err", err)
			return 1
		}
		log.Info(ctx, "migrate down: ok")

	case "version":
		v, dirty, err := pgkit.MigrateVersionMulti(cfg)
		if err != nil {
			log.Error(ctx, "migrate version failed", "err", err)
			return 1
		}
		if v == 0 && !dirty {
			log.Info(ctx, "no migrations applied")
		} else {
			log.Info(ctx, "migrate version", "version", v, "dirty", dirty)
		}

	default:
		log.Error(ctx, "unknown command", "command", os.Args[1])
		return 1
	}
	return 0
}
