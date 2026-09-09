// Command migrate — прогон миграций отдельным шагом (lockstep с образом api).
// Источники: киты golibinfra (bruteforcekit, auditkit, per-domain outbox-таблицы)
// и каталог приложения PG_MIGRATIONS_PATH; у каждого — своя таблица версий
// schema_migrations_<source>, новая миграция кита не сдвигает версии приложения.
package main

import (
	"context"
	"os"

	auditpg "git.stit.tech/stit-core/golibinfra/auditkit/pgstore"
	bruteforcepg "git.stit.tech/stit-core/golibinfra/bruteforcekit/pgstore"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"

	useroutbox "git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/outbox"
	verifoutbox "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/outbox"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	ctx := context.Background()

	log, err := logkit.New(logkit.Config{Level: "info"})
	if err != nil {
		_, _ = os.Stderr.WriteString("migrate: logger: " + err.Error() + "\n")
		os.Exit(1)
	}
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

	// PG_MIGRATIONS_PATH (default /migrations) и PG_MIGRATE_TIMEOUT (2m) — тот же
	// конфиг, что у pgkit.MigrateModule: одна семантика на оба способа прогона.
	cfg, err := pgkit.NewMigrateRunnerConfig()
	if err != nil {
		log.Error(ctx, "migrate config", "err", err)
		return 1
	}
	app := pgkit.DirSource(pgkit.AppSourceName, cfg.Path)
	// Киты раньше приложения: миграции приложения могут опираться на их таблицы.
	sources := []pgkit.Source{
		bruteforcepg.Migrations,
		auditpg.Migrations,
		useroutbox.Migrations,
		verifoutbox.Migrations,
		app,
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	switch os.Args[1] {
	case "up":
		if err := pgkit.MigrateUpAll(ctx, dsn, sources...); err != nil {
			log.Error(ctx, "migrate up failed", "err", err)
			return 1
		}
		log.Info(ctx, "migrate up: ok")

	case "down":
		// Откат — одна миграция приложения: схему китов ведёт golibinfra.
		if err := pgkit.MigrateDown(ctx, dsn, app); err != nil {
			log.Error(ctx, "migrate down failed", "err", err)
			return 1
		}
		log.Info(ctx, "migrate down: ok")

	case "version":
		for _, src := range sources {
			v, dirty, err := pgkit.MigrateVersion(ctx, dsn, src)
			if err != nil {
				log.Error(ctx, "migrate version failed", "source", src.Name, "version", v, "dirty", dirty, "err", err)
				return 1
			}
			log.Info(ctx, "migrate version", "source", src.Name, "version", v)
		}

	default:
		log.Error(ctx, "unknown command", "command", os.Args[1])
		return 1
	}
	return 0
}
