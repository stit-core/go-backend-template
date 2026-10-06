//go:build integration

package main

import (
	"testing"

	"git.stit.tech/stit-core/golibinfra/testkit"

	"git.stit.tech/stit-core/go-backend-template/cmd/migrate/sources"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/repopath"
)

// Лесенка источников cmd/migrate в порядке прогона: down каждой версии возвращает схему ровно к
// состоянию до её up, повторный up воспроизводит схему версии, весь источник откатывается до нуля.
func TestMigrations_Ladder(t *testing.T) {
	t.Parallel()
	testkit.MigrationLadder(t, testkit.WithMigrations(sources.Kits...), testkit.WithMigrationsDir(repopath.Migrations()))
}
