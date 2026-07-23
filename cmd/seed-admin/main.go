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
	"github.com/kelseyhightower/envconfig"

	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
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
	log.Info(ctx, "starting", "component", "seed-admin", "version", version, "commit", commit)

	os.Exit(run(ctx, log))
}

func run(ctx context.Context, log logkit.Logger) int {
	dsn := os.Getenv("PG_DSN")
	email := os.Getenv("SEED_ADMIN_EMAIL")
	password := os.Getenv("SEED_ADMIN_PASSWORD")
	name := os.Getenv("SEED_ADMIN_NAME")
	if name == "" {
		name = "Admin"
	}
	if dsn == "" || email == "" || password == "" {
		log.Error(ctx, "PG_DSN, SEED_ADMIN_EMAIL, SEED_ADMIN_PASSWORD are required")
		return 1
	}

	var p authnkit.Argon2idParams
	if err := envconfig.Process("", &p); err != nil {
		log.Error(ctx, "argon params", "err", err)
		return 1
	}
	hash, err := authnkit.NewArgon2idHasher(p).Hash(password)
	if err != nil {
		log.Error(ctx, "hash password", "err", err)
		return 1
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		log.Error(ctx, "pg connect", "err", err)
		return 1
	}
	defer func() { _ = conn.Close(ctx) }()

	if err := upsertAdmin(ctx, conn, email, hash, name); err != nil {
		log.Error(ctx, "seed admin failed", "err", err)
		return 1
	}
	log.Info(ctx, "seed admin: ok", "email", email)
	return 0
}

// upsertAdmin идемпотентен: повторный запуск обновляет role/hash, не плодит
// строк. Уникальность users — частичный индекс LOWER(email) WHERE
// status != 'deleted'; матчим по нему вручную в транзакции.
func upsertAdmin(ctx context.Context, conn *pgx.Conn, email, passwordHash, name string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	err = tx.QueryRow(
		ctx,
		"SELECT id FROM users WHERE LOWER(email) = LOWER($1) AND status <> 'deleted' FOR UPDATE",
		email,
	).Scan(&id)
	switch {
	case err == nil:
		if _, err := tx.Exec(
			ctx,
			"UPDATE users SET role = 'admin', password_hash = $1 WHERE id = $2",
			passwordHash, id,
		); err != nil {
			return fmt.Errorf("update: %w", err)
		}
	case errors.Is(err, pgx.ErrNoRows):
		if _, err := tx.Exec(
			ctx,
			"INSERT INTO users (email, password_hash, name, status, role, email_verified) "+
				"VALUES ($1, $2, $3, 'active', 'admin', true)",
			email, passwordHash, name,
		); err != nil {
			return fmt.Errorf("insert: %w", err)
		}
	default:
		return fmt.Errorf("lookup: %w", err)
	}
	return tx.Commit(ctx)
}
