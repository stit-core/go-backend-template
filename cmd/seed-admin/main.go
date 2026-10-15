// Command seed-admin — первый admin сервиса: регистрация даёт только роль client. One-off, как cmd/migrate:
// SEED_ADMIN_EMAIL и SEED_ADMIN_PASSWORD (SEED_ADMIN_NAME) → user.Usecase.EnsureAdmin.
package main

import (
	"cmp"
	"context"
	"os"

	auditpg "git.stit.tech/stit-core/golibinfra/auditkit/pgstore"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/authnkit/session"
	sessionpg "git.stit.tech/stit-core/golibinfra/authnkit/session/pgstore"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	userpg "git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/postgres"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	ctx := context.Background()

	cfg, err := logkit.NewConfig()
	var log logkit.Logger
	if err == nil {
		log, err = logkit.New(cfg)
	}
	if err != nil {
		_, _ = os.Stderr.WriteString("seed-admin: logger: " + err.Error() + "\n") //nolint:errcheck,forbidigo // логгера нет: stderr — последний канал
		os.Exit(1)
	}
	log.Info(ctx, "starting", "component", "seed-admin", "version", version, "commit", commit)

	os.Exit(run(ctx, log))
}

func run(ctx context.Context, log logkit.Logger) int {
	in := user.EnsureAdminInput{
		Email:    os.Getenv("SEED_ADMIN_EMAIL"),
		Password: os.Getenv("SEED_ADMIN_PASSWORD"),
		Name:     cmp.Or(os.Getenv("SEED_ADMIN_NAME"), "Admin"),
	}
	if in.Email == "" || in.Password == "" {
		log.Error(ctx, "SEED_ADMIN_EMAIL, SEED_ADMIN_PASSWORD are required")
		return 1
	}
	poolCfg, err := pgkit.NewPoolConfig()
	if err != nil {
		log.Error(ctx, "pg config", "error", err)
		return 1
	}
	sessionCfg, err := session.NewConfig()
	if err != nil {
		log.Error(ctx, "session config", "error", err)
		return 1
	}
	argon, err := authnkit.NewArgon2idConfig()
	if err != nil {
		log.Error(ctx, "argon params", "error", err)
		return 1
	}
	hasher, err := authnkit.NewArgon2idHasher(argon)
	if err != nil {
		log.Error(ctx, "argon params", "error", err)
		return 1
	}
	pool, err := pgkit.NewPool(ctx, poolCfg)
	if err != nil {
		log.Error(ctx, "pg connect", "error", err)
		return 1
	}
	defer pool.Close()
	db := pgkit.NewDB(pool)
	sessions, err := sessionpg.NewStore(sessionCfg, db)
	if err != nil {
		log.Error(ctx, "session store", "error", err)
		return 1
	}

	users := user.NewUsecase(user.UsecaseDeps{
		Repo: userpg.NewRepository(db), Hasher: hasher, Tx: pgkit.NewPoolTransactor(pool), Sessions: sessions,
		Audit: auditpg.NewStore(db), Log: log, Clock: clockkit.Real{},
	})
	admin, err := users.EnsureAdmin(ctx, in)
	if err != nil {
		log.Error(ctx, "seed admin failed", "error", err)
		return 1
	}
	log.Info(ctx, "seed admin: ok", "user_id", admin.ID)
	return 0
}
