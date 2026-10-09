// Command seed-admin — идемпотентный bootstrap первого admin.
//
// В шаблоне нет публичного пути сделать admin (register даёт role=client).
// One-off как cmd/migrate: SEED_ADMIN_EMAIL/PASSWORD → upsert юзера с
// role=admin, хеш через golibinfra/authnkit (Argon2id, encoded PHC
// self-describing — приложение верифицит на login без доп. конфига).
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"

	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/authnkit/session"
	sessionpg "git.stit.tech/stit-core/golibinfra/authnkit/session/pgstore"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	ctx := context.Background()

	log, err := logkit.New(logkit.Config{Level: "info"})
	if err != nil {
		_, _ = os.Stderr.WriteString("seed-admin: logger: " + err.Error() + "\n") //nolint:errcheck,forbidigo // логгера нет: stderr — последний канал
		os.Exit(1)
	}
	log.Info(ctx, "starting", "component", "seed-admin", "version", version, "commit", commit)

	os.Exit(run(ctx, log))
}

func run(ctx context.Context, log logkit.Logger) int {
	email := os.Getenv("SEED_ADMIN_EMAIL")
	password := os.Getenv("SEED_ADMIN_PASSWORD")
	name := os.Getenv("SEED_ADMIN_NAME")
	if name == "" {
		name = "Admin"
	}
	if email == "" || password == "" {
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

	p, err := authnkit.NewArgon2idConfig()
	if err != nil {
		log.Error(ctx, "argon params", "error", err)
		return 1
	}
	hasher, err := authnkit.NewArgon2idHasher(p)
	if err != nil {
		log.Error(ctx, "argon params", "error", err)
		return 1
	}
	hash, err := hasher.Hash(ctx, password)
	if err != nil {
		log.Error(ctx, "hash password", "error", err)
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

	var id string
	if err := pgkit.NewPoolTransactor(pool).WithTx(ctx, func(txCtx context.Context) error {
		var err error
		if id, err = upsertAdmin(txCtx, db, email, hash, name); err != nil {
			return err
		}
		// Права и пароль сменились: прежние сессии учётной записи не получают ни того, ни другого.
		return sessions.RevokeAll(txCtx, id)
	}); err != nil {
		log.Error(ctx, "seed admin failed", "error", err)
		return 1
	}
	log.Info(ctx, "seed admin: ok", "user_id", id)
	return 0
}

// upsertAdmin идемпотентен: повторный запуск обновляет role/hash, не плодит строк. Уникальность users —
// частичный индекс LOWER(email) WHERE status != 'deleted'; матчим по нему вручную в транзакции из ctx.
func upsertAdmin(ctx context.Context, db *pgkit.DB, email, passwordHash, name string) (string, error) {
	var id string
	err := db.QueryRow(
		ctx,
		"SELECT id FROM users WHERE LOWER(email) = LOWER($1) AND status <> 'deleted' FOR UPDATE",
		email,
	).Scan(&id)
	switch {
	case err == nil:
		if _, err := db.Exec(
			ctx,
			"UPDATE users SET role = 'admin', password_hash = $1 WHERE id = $2",
			passwordHash, id,
		); err != nil {
			return "", fmt.Errorf("update: %w", err)
		}
	case errors.Is(err, pgx.ErrNoRows):
		if err := db.QueryRow(
			ctx,
			"INSERT INTO users (email, password_hash, name, status, role, email_verified) "+
				"VALUES ($1, $2, $3, 'active', 'admin', true) RETURNING id",
			email, passwordHash, name,
		).Scan(&id); err != nil {
			return "", fmt.Errorf("insert: %w", err)
		}
	default:
		return "", fmt.Errorf("lookup: %w", err)
	}
	return id, nil
}
