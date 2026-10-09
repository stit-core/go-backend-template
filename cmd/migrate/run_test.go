//go:build integration

package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/testkit"
	"git.stit.tech/stit-core/golibinfra/testkit/logtest"

	"git.stit.tech/stit-core/go-backend-template/cmd/migrate/sources"
)

// migrate запускает команду с аргументами командной строки: код выхода и её лог.
func migrate(t *testing.T, args ...string) (int, *logtest.Recorder) {
	t.Helper()
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = append([]string{"migrate"}, args...)
	log := logtest.Capture()
	return run(t.Context(), log), log
}

// Команда migrate, как её зовёт выкатка, на своём каталоге приложения. up накатывает источники китов и
// каталог; блокировку таблицы миграция ждёт не дольше PG_MIGRATE_LOCK_TIMEOUT, отказ оставляет источник
// dirty на версии, до которой шёл: version отвечает ошибкой с его именем, up дальше не идёт. Снять только
// флаг — up сочтёт миграцию применённой; ремонт — прежняя версия без флага. down откатывает одну миграцию
// приложения. Без t.Parallel: t.Setenv и os.Args.
func TestRun(t *testing.T) {
	pg := testkit.StartPostgres(t)
	db := pg.Pool(t)
	ctx := t.Context()
	dir := t.TempDir()
	write := func(name, sql string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(sql), 0o600))
	}
	write("0001_probe.up.sql", "CREATE TABLE migrate_probe (id INT);")
	write("0001_probe.down.sql", "DROP TABLE migrate_probe;")
	t.Setenv("PG_DSN", pg.DSN)
	t.Setenv("PG_MIGRATIONS_PATH", dir)
	t.Setenv("PG_MIGRATE_LOCK_TIMEOUT", "1s")
	app := func() (int, bool) {
		var (
			version int
			dirty   bool
		)
		require.NoError(t, db.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations_app`).Scan(&version, &dirty))
		return version, dirty
	}
	hasNote := func() bool {
		var n int
		require.NoError(t, db.QueryRow(ctx,
			`SELECT count(*) FROM information_schema.columns WHERE table_name = 'migrate_probe' AND column_name = 'note'`).Scan(&n))
		return n == 1
	}
	setApp := func(sql string) {
		_, err := db.Exec(ctx, sql)
		require.NoError(t, err)
	}

	code, _ := migrate(t, "up")
	require.Zero(t, code)
	for _, src := range sources.Kits {
		require.NoError(t, pgkit.CheckSchema(ctx, pg.DB(t), src), "up applies the kit source %s", src.Name)
	}
	version, dirty := app()
	require.Equal(t, 1, version)
	require.False(t, dirty)

	write("0002_probe_note.up.sql", "ALTER TABLE migrate_probe ADD COLUMN note TEXT;")
	write("0002_probe_note.down.sql", "ALTER TABLE migrate_probe DROP COLUMN note;")
	tx, err := db.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `LOCK TABLE migrate_probe IN ACCESS SHARE MODE`)
	require.NoError(t, err)
	started := time.Now()
	code, log := migrate(t, "up")
	waited := time.Since(started)
	require.NoError(t, tx.Rollback(ctx))
	require.Equal(t, 1, code, "the table is locked")
	failed, ok := log.Find(slog.LevelError, "migrate up failed")
	require.True(t, ok)
	require.Contains(t, failed.Str("error"), "lock timeout")
	require.Less(t, waited, 4*time.Second, "the migration waits for the lock no longer than PG_MIGRATE_LOCK_TIMEOUT (1s here, 5s by default)")
	version, dirty = app()
	require.Equal(t, 2, version, "the failed migration leaves the version it was heading to")
	require.True(t, dirty)
	require.False(t, hasNote())

	code, log = migrate(t, "version")
	require.Equal(t, 1, code)
	failed, ok = log.Find(slog.LevelError, "migrate version failed")
	require.True(t, ok)
	require.Equal(t, pgkit.AppSourceName, failed.Str("source"), "version names the dirty source")
	code, _ = migrate(t, "up")
	require.Equal(t, 1, code, "a dirty source does not migrate")

	setApp(`UPDATE schema_migrations_app SET dirty = false`)
	code, _ = migrate(t, "up")
	require.Zero(t, code)
	require.False(t, hasNote(), "with the flag cleared alone the version says the migration is applied: up skips it")

	setApp(`UPDATE schema_migrations_app SET version = 1, dirty = false`)
	code, _ = migrate(t, "up")
	require.Zero(t, code)
	require.True(t, hasNote(), "the repair is the version the migration started from, without the flag")
	code, _ = migrate(t, "version")
	require.Zero(t, code)

	code, _ = migrate(t, "down")
	require.Zero(t, code)
	version, dirty = app()
	require.Equal(t, 1, version, "down rolls back one migration of the app")
	require.False(t, dirty)
	require.False(t, hasNote())
	for _, src := range sources.Kits {
		require.NoError(t, pgkit.CheckSchema(ctx, pg.DB(t), src), "down leaves the kit source %s alone", src.Name)
	}
}

// Вызов без команды, с неизвестной командой и без PG_DSN — код 1 с причиной в логе.
func TestRun_Usage(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		dsn  string
		want string
	}{
		{name: "no command", dsn: "postgres://app@127.0.0.1:1/app", want: "usage: migrate <up|down|version>"},
		{name: "unknown command", args: []string{"force"}, dsn: "postgres://app@127.0.0.1:1/app", want: "unknown command"},
		{name: "no PG_DSN", args: []string{"up"}, want: "PG_DSN is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PG_DSN", tc.dsn)
			code, log := migrate(t, tc.args...)
			require.Equal(t, 1, code)
			require.True(t, log.Has(slog.LevelError, tc.want), "log: %v", log.Entries())
		})
	}
}
