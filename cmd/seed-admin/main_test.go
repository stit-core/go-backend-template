//go:build integration

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/golibinfra/authnkit/session"
	sessionpg "git.stit.tech/stit-core/golibinfra/authnkit/session/pgstore"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/testkit"

	"git.stit.tech/stit-core/go-backend-template/cmd/migrate/sources"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/repopath"
)

// seedEnv — БД со схемой прода и env seed-admin для адреса email. Без t.Parallel: t.Setenv.
func seedEnv(t *testing.T, email string) *testkit.PostgresFixture {
	t.Helper()
	pg := testkit.StartPostgres(t, testkit.WithMigrations(sources.Kits...), testkit.WithMigrationsDir(repopath.Migrations()))
	t.Setenv("PG_DSN", pg.DSN)
	t.Setenv("SEED_ADMIN_EMAIL", email)
	t.Setenv("SEED_ADMIN_PASSWORD", "admin-password-1")
	return pg
}

// Повышение до admin закрывает прежние сессии учётной записи: токен, выданный до него, не получит прав
// admin при обновлении сессии (SEC-07).
func TestRun_PromotionClosesSessions(t *testing.T) {
	pg := seedEnv(t, "Admin@Example.com")
	db := pg.DB(t)
	var id string
	require.NoError(t, db.QueryRow(
		t.Context(),
		`INSERT INTO users (email, password_hash, name, status, role) VALUES ('admin@example.com', 'x', 'Early', 'active', 'client') RETURNING id`,
	).Scan(&id))
	store, err := sessionpg.NewStore(db, session.Config{IdleTTL: time.Hour, AbsoluteTTL: time.Hour, CleanupInterval: time.Hour})
	require.NoError(t, err)
	tok, err := store.Issue(t.Context(), id)
	require.NoError(t, err)

	require.Equal(t, 0, run(t.Context(), logkit.Nop()))

	var role string
	require.NoError(t, db.QueryRow(t.Context(), `SELECT role FROM users WHERE id = $1`, id).Scan(&role))
	require.Equal(t, "admin", role, "the account with this address is promoted")
	_, err = store.Rotate(t.Context(), tok.Value)
	require.ErrorIs(t, err, session.ErrInvalid, "the session opened before the promotion is closed")
}

// Повторный запуск идемпотентен: admin с этим адресом один.
func TestRun_CreatesAdminOnce(t *testing.T) {
	pg := seedEnv(t, "admin@example.com")

	require.Equal(t, 0, run(t.Context(), logkit.Nop()))
	require.Equal(t, 0, run(t.Context(), logkit.Nop()))

	var admins int
	require.NoError(t, pg.DB(t).QueryRow(
		t.Context(),
		`SELECT count(*) FROM users WHERE LOWER(email) = 'admin@example.com' AND role = 'admin' AND status = 'active'`,
	).Scan(&admins))
	require.Equal(t, 1, admins)
}
