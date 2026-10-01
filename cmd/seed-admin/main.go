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

	log, err := logkit.New(logkit.Config{Level: "info"})
	if err != nil {
		_, _ = os.Stderr.WriteString("seed-admin: logger: " + err.Error() + "\n") //nolint:errcheck,forbidigo // логгера нет: stderr — последний канал
		os.Exit(1)
	}
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
	hasher, err := authnkit.NewArgon2idHasher(p)
	if err != nil {
		log.Error(ctx, "argon params", "err", err)
		return 1
	}
	hash, err := hasher.Hash(ctx, password)
	if err != nil {
		log.Error(ctx, "hash password", "err", err)
		return 1
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		log.Error(ctx, "pg connect", "err", err)
		return 1
	}
	defer func() { _ = conn.Close(ctx) }() //nolint:errcheck // процесс завершается, ошибку закрытия некуда вернуть

	id, err := upsertAdmin(ctx, conn, email, hash, name)
	if err != nil {
		log.Error(ctx, "seed admin failed", "err", err)
		return 1
	}
	log.Info(ctx, "seed admin: ok", "user_id", id)
	return 0
}

// upsertAdmin идемпотентен: повторный запуск обновляет role/hash, не плодит
// строк. Уникальность users — частичный индекс LOWER(email) WHERE
// status != 'deleted'; матчим по нему вручную в транзакции.
func upsertAdmin(ctx context.Context, conn *pgx.Conn, email, passwordHash, name string) (string, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // после Commit — no-op с ErrTxClosed

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
			return "", fmt.Errorf("update: %w", err)
		}
	case errors.Is(err, pgx.ErrNoRows):
		if err := tx.QueryRow(
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
	return id, tx.Commit(ctx)
}
