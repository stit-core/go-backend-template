//go:build integration

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/testkit"

	"git.stit.tech/stit-core/go-backend-template/internal/testkit/repopath"
)

// Integration-тесты — шаг make check, а testkit без Docker их пропускает: здесь
// пропуск становится провалом, иначе гейт позеленеет, не открыв БД.
func TestDockerRequired(t *testing.T) {
	t.Parallel()
	available := false
	t.Run("docker", func(t *testing.T) {
		testkit.RequireDocker(t)
		available = true
	})
	require.True(t, available, "Docker is unavailable: make check requires integration tests (see make doctor)")
}

// Лесенка по миграциям приложения: down каждой версии возвращает схему ровно к
// состоянию до её up, повторный up воспроизводит схему версии.
func TestAppMigrationsRoundtrip(t *testing.T) {
	t.Parallel()
	dir := repopath.Migrations()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	pg := testkit.StartPostgres(t)
	pool := pg.Pool(t)
	ctx := t.Context()

	// Источник растёт по версии: MigrateUp доходит ровно до последней добавленной.
	files := fstest.MapFS{}
	src := pgkit.Source{Name: pgkit.AppSourceName, FS: files, Dir: "."}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		version := strings.TrimSuffix(name, ".up.sql")
		for _, f := range []string{name, version + ".down.sql"} {
			data, err := os.ReadFile(filepath.Join(dir, f))
			require.NoError(t, err)
			files[f] = &fstest.MapFile{Data: data}
		}

		before := schema(t, pool)
		require.NoError(t, pgkit.MigrateUp(ctx, pg.DSN, src), "up %s", version)
		after := schema(t, pool)
		require.NoError(t, pgkit.MigrateDown(ctx, pg.DSN, src), "down %s", version)
		require.Equal(t, before, schema(t, pool), "down %s must restore the schema it started from", version)
		require.NoError(t, pgkit.MigrateUp(ctx, pg.DSN, src), "up %s after down", version)
		require.Equal(t, after, schema(t, pool), "up %s after down must rebuild the same schema", version)
	}
}

// Отпечаток схемы public без таблиц версий и объектов расширений: down не удаляет
// общие расширения (pgcrypto).
const schemaQuery = `
SELECT format('column %s.%s %s null=%s default=%s', table_name, column_name, data_type, is_nullable, column_default)
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name NOT LIKE 'schema\_migrations\_%'
UNION ALL
SELECT 'index ' || indexdef
FROM pg_indexes
WHERE schemaname = 'public' AND tablename NOT LIKE 'schema\_migrations\_%'
UNION ALL
SELECT format('constraint %s.%s %s', conrelid::regclass, conname, pg_get_constraintdef(oid))
FROM pg_constraint
WHERE connamespace = 'public'::regnamespace AND conrelid::regclass::text NOT LIKE 'schema\_migrations\_%'
UNION ALL
SELECT 'trigger ' || pg_get_triggerdef(oid)
FROM pg_trigger
WHERE NOT tgisinternal
UNION ALL
SELECT 'function ' || p.oid::regprocedure
FROM pg_proc p
WHERE p.pronamespace = 'public'::regnamespace
  AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = p.oid AND d.deptype = 'e')
ORDER BY 1`

func schema(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), schemaQuery)
	require.NoError(t, err)
	objects, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	return objects
}
