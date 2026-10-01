//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	userpg "git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/postgres"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/pgseed"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/repopath"
	"git.stit.tech/stit-core/go-backend-template/pkg/paging"
	"git.stit.tech/stit-core/golibinfra/testkit"
)

func setup(t *testing.T) user.Repository {
	t.Helper()
	fx := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations()))
	return userpg.NewRepository(fx.DB(t))
}

func TestInsert(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo)
	require.NotEmpty(t, u.ID)
	require.Equal(t, identity.RoleClient, u.Role)

	dup := pgseed.NewUser(pgseed.WithEmail(u.Email))
	err := repo.Insert(ctx, dup)
	require.ErrorIs(t, err, user.ErrAlreadyExists)
}

func TestGetByID(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo, pgseed.WithRole(identity.RoleAdmin))

	got, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, u.Email, got.Email)
	require.Equal(t, identity.RoleAdmin, got.Role)

	_, err = repo.GetByID(ctx, "00000000-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, user.ErrNotFound)
}

func TestList_WithRole(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	u1 := pgseed.InsertUser(t, repo, pgseed.WithRole(identity.RoleClient))
	u2 := pgseed.InsertUser(t, repo, pgseed.WithRole(identity.RoleAdmin))

	list, err := repo.List(ctx, user.ListFilter{}, paging.Page{Number: 1, Size: 100})
	require.NoError(t, err)

	roleByEmail := make(map[string]identity.Role)
	for _, u := range list {
		roleByEmail[u.Email] = u.Role
	}
	require.Equal(t, identity.RoleClient, roleByEmail[u1.Email])
	require.Equal(t, identity.RoleAdmin, roleByEmail[u2.Email])
}

func TestUpdate(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo)
	u.Name, u.Status = "Renamed", user.StatusBlocked
	got, err := repo.Update(ctx, u)
	require.NoError(t, err)
	require.Equal(t, u.Version+1, got.Version)

	stored, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "Renamed", stored.Name, "name and status are written together")
	require.Equal(t, user.StatusBlocked, stored.Status, "name and status are written together")
	require.Equal(t, got.Version, stored.Version)
}

func TestUpdate_StaleVersion(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	updates := []struct {
		name   string
		update func(u *user.User) (*user.User, error)
	}{
		{"name and status", func(u *user.User) (*user.User, error) {
			return repo.Update(ctx, &user.User{ID: u.ID, Name: "renamed", Status: user.StatusBlocked, Version: u.Version})
		}},
		{"password", func(u *user.User) (*user.User, error) {
			return repo.UpdatePassword(ctx, u.ID, "new-hash", u.Version)
		}},
	}
	for _, tc := range updates {
		t.Run(tc.name, func(t *testing.T) {
			u := pgseed.InsertUser(t, repo)

			got, err := tc.update(u)
			require.NoError(t, err)
			require.Equal(t, u.Version+1, got.Version)

			_, err = tc.update(u)
			require.ErrorIs(t, err, user.ErrConcurrentUpdate)
		})
	}
}

// Удаление без версии: повтор — ErrNotFound, из одновременных удалений проходит ровно одно.
func TestDelete(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo)
	require.NoError(t, repo.Delete(ctx, u.ID))
	_, err := repo.GetByID(ctx, u.ID)
	require.ErrorIs(t, err, user.ErrNotFound)
	require.ErrorIs(t, repo.Delete(ctx, u.ID), user.ErrNotFound)
	require.ErrorIs(t, repo.Delete(ctx, "00000000-0000-0000-0000-000000000000"), user.ErrNotFound)

	raced := pgseed.InsertUser(t, repo)
	const n = 8
	errs := make(chan error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			<-start
			errs <- repo.Delete(ctx, raced.ID)
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	deleted := 0
	for err := range errs {
		if err == nil {
			deleted++
			continue
		}
		require.ErrorIs(t, err, user.ErrNotFound)
	}
	require.Equal(t, 1, deleted, "exactly one of the concurrent deletes deletes")
}

func TestSetEmailVerified(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo)
	require.NoError(t, repo.SetEmailVerified(ctx, u.ID))
	got, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.True(t, got.EmailVerified)

	require.ErrorIs(t, repo.SetEmailVerified(ctx, "00000000-0000-0000-0000-000000000000"), user.ErrNotFound)
}

// Удалённый пользователь снаружи не существует: чтение его не находит, запись не трогает.
func TestDeletedUserIsUntouched(t *testing.T) {
	t.Parallel()
	pg := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations()))
	repo := userpg.NewRepository(pg.DB(t))
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo)
	require.NoError(t, repo.Delete(ctx, u.ID))
	deletedVersion := u.Version + 1

	_, err := repo.GetByID(ctx, u.ID)
	require.ErrorIs(t, err, user.ErrNotFound)
	_, err = repo.GetByEmail(ctx, u.Email)
	require.ErrorIs(t, err, user.ErrNotFound)
	list, err := repo.List(ctx, user.ListFilter{}, paging.Page{Number: 1, Size: paging.MaxSize})
	require.NoError(t, err)
	require.Empty(t, list)

	_, err = repo.Update(ctx, &user.User{ID: u.ID, Name: "revived", Status: user.StatusActive, Version: deletedVersion})
	require.ErrorIs(t, err, user.ErrConcurrentUpdate)
	_, err = repo.UpdatePassword(ctx, u.ID, "new-hash", deletedVersion)
	require.ErrorIs(t, err, user.ErrConcurrentUpdate)
	require.ErrorIs(t, repo.SetEmailVerified(ctx, u.ID), user.ErrNotFound)

	var (
		name, status, hash string
		version            int
		verified           bool
	)
	require.NoError(t, pg.Pool(t).QueryRow(ctx,
		`SELECT name, status, password_hash, version, email_verified FROM users WHERE id = $1`, u.ID).
		Scan(&name, &status, &hash, &version, &verified))
	require.Equal(t, []any{u.Name, "deleted", u.PasswordHash, deletedVersion, false},
		[]any{name, status, hash, version, verified})
}
