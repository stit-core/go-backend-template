package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/kelseyhightower/envconfig"

	auditpg "git.stit.tech/stit/golibinfra/auditkit/pgstore"
	bruteforcepg "git.stit.tech/stit/golibinfra/bruteforcekit/pgstore"
	"git.stit.tech/stit/golibinfra/logkit"
	outboxpg "git.stit.tech/stit/golibinfra/outboxkit/pgstore"
	"git.stit.tech/stit/golibinfra/pgkit"
	"git.stit.tech/stit/golibinfra/telemetry"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	ctx := context.Background()

	var telCfg telemetry.Config
	if err := envconfig.Process("", &telCfg); err != nil {
		// До инициализации logger'а — единственное допустимое место для stderr.
		fmt.Fprintln(os.Stderr, "migrate: telemetry config:", err)
		os.Exit(1)
	}
	providers, err := telemetry.Init(ctx, telCfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate: telemetry init:", err)
		os.Exit(1)
	}

	log := logkit.New(logkit.Config{Level: "info"}, providers.LoggerProvider)
	log.Info(ctx, "starting", "component", "migrate", "version", version, "commit", commit)

	// run возвращает exit code. os.Exit обходит defer'ы — поэтому
	// shutdown зовём здесь, явно, до Exit. Без shutdown'а BatchProcessor
	// потеряет последние log records (в т.ч. итоговый успех/ошибку миграции).
	exitCode := run(ctx, log)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_ = providers.Shutdown(shutdownCtx)
	cancel()
	os.Exit(exitCode)
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
