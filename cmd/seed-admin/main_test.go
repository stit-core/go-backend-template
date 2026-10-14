//go:build integration

package main

import (
	"strings"
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
	store, err := sessionpg.NewStore(session.Config{IdleTTL: time.Hour, AbsoluteTTL: time.Hour, CleanupInterval: time.Hour}, db)
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

// Повышение и закрытие сессий — одна транзакция: сбой закрытия откатывает и роль, и пароль.
func TestRun_RevokeFailureRollsBackPromotion(t *testing.T) {
	pg := seedEnv(t, "admin@example.com")
	db := pg.DB(t)
	var id string
	require.NoError(t, db.QueryRow(
		t.Context(),
		`INSERT INTO users (email, password_hash, name, status, role) VALUES ('admin@example.com', 'x', 'Early', 'active', 'client') RETURNING id`,
	).Scan(&id))
	_, err := db.Exec(t.Context(), `ALTER TABLE authn_sessions RENAME TO authn_sessions_offline`)
	require.NoError(t, err)

	require.Equal(t, 1, run(t.Context(), logkit.Nop()))

	var role, hash string
	require.NoError(t, db.QueryRow(t.Context(), `SELECT role, password_hash FROM users WHERE id = $1`, id).Scan(&role, &hash))
	require.Equal(t, "client", role, "the promotion rolls back with the failed revocation")
	require.Equal(t, "x", hash)
}

// Адрес и имя admin — по правилам сущности user: учётной записи, которую не завела бы регистрация, seed не
// заводит.
func TestRun_RejectsAdminOutsideUserRules(t *testing.T) {
	pg := seedEnv(t, "admin@example.com")
	for _, tc := range []struct{ name, email, adminName string }{
		{"space before the address", " admin@example.com", "Admin"},
		{"tab after the address", "admin@example.com\t", "Admin"},
		{"no-break space after the address", "admin@example.com ", "Admin"},
		{"address with a display name", "Admin <admin@example.com>", "Admin"},
		{"not an address", "not an address", "Admin"},
		{"name over 200 characters", "admin@example.com", strings.Repeat("я", 201)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SEED_ADMIN_EMAIL", tc.email)
			t.Setenv("SEED_ADMIN_NAME", tc.adminName)
			require.Equal(t, 1, run(t.Context(), logkit.Nop()))
		})
	}
	var users int
	require.NoError(t, pg.DB(t).QueryRow(t.Context(), `SELECT count(*) FROM users`).Scan(&users))
	require.Zero(t, users, "a refused seed writes nothing")
}

// Первый запуск заводит admin с подтверждённым адресом, повторный идемпотентен: admin с этим адресом один.
func TestRun_CreatesAdminOnce(t *testing.T) {
	pg := seedEnv(t, "admin@example.com")
	type account struct {
		role     string
		verified bool
	}
	accounts := func() []account {
		rows, err := pg.DB(t).Query(t.Context(), `SELECT role, email_verified FROM users WHERE LOWER(email) = 'admin@example.com' AND status = 'active'`)
		require.NoError(t, err)
		var got []account
		for rows.Next() {
			var a account
			require.NoError(t, rows.Scan(&a.role, &a.verified))
			got = append(got, a)
		}
		require.NoError(t, rows.Err())
		return got
	}

	require.Equal(t, 0, run(t.Context(), logkit.Nop()))
	require.Equal(t, []account{{role: "admin", verified: true}}, accounts(), "the first run creates an admin with a verified address: the operator gave it")

	require.Equal(t, 0, run(t.Context(), logkit.Nop()))
	require.Equal(t, []account{{role: "admin", verified: true}}, accounts(), "the second run creates nothing")
}

// Без адреса или пароля seed не стартует и ничего не пишет: admin без пароля не заводится.
func TestRun_RequiresAddressAndPassword(t *testing.T) {
	pg := seedEnv(t, "admin@example.com")
	for _, unset := range []string{"SEED_ADMIN_EMAIL", "SEED_ADMIN_PASSWORD"} {
		t.Run(unset, func(t *testing.T) {
			t.Setenv(unset, "")
			require.Equal(t, 1, run(t.Context(), logkit.Nop()))
		})
	}
	var users int
	require.NoError(t, pg.DB(t).QueryRow(t.Context(), `SELECT count(*) FROM users`).Scan(&users))
	require.Zero(t, users)
}

// Удалённая учётная запись с тем же адресом адрес не держит: seed заводит нового admin, а удалённую не
// повышает и не оживляет.
func TestRun_DeletedAccountKeepsNothing(t *testing.T) {
	pg := seedEnv(t, "admin@example.com")
	db := pg.DB(t)
	var deletedID string
	require.NoError(t, db.QueryRow(
		t.Context(),
		`INSERT INTO users (email, password_hash, name, status, role) VALUES ('admin@example.com', 'x', 'Gone', 'deleted', 'client') RETURNING id`,
	).Scan(&deletedID))

	require.Equal(t, 0, run(t.Context(), logkit.Nop()))

	var status, role string
	require.NoError(t, db.QueryRow(t.Context(), `SELECT status, role FROM users WHERE id = $1`, deletedID).Scan(&status, &role))
	require.Equal(t, []string{"deleted", "client"}, []string{status, role}, "the deleted account stays as it was")
	var admins int
	require.NoError(t, db.QueryRow(
		t.Context(),
		`SELECT count(*) FROM users WHERE LOWER(email) = 'admin@example.com' AND role = 'admin' AND status = 'active' AND id <> $1`, deletedID,
	).Scan(&admins))
	require.Equal(t, 1, admins, "a new admin is created")
}
