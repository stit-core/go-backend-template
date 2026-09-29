//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	userpg "git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/postgres"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/pgseed"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/repopath"
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

	list, err := repo.List(ctx, user.ListFilter{}, user.ListPagination{Page: 1, Size: 100})
	require.NoError(t, err)

	roleByEmail := make(map[string]identity.Role)
	for _, u := range list {
		roleByEmail[u.Email] = u.Role
	}
	require.Equal(t, identity.RoleClient, roleByEmail[u1.Email])
	require.Equal(t, identity.RoleAdmin, roleByEmail[u2.Email])
}

func TestUpdate_StaleVersion(t *testing.T) {
	t.Parallel()
	repo := setup(t)
	ctx := context.Background()

	updates := []struct {
		name   string
		update func(id string, version int) (*user.User, error)
	}{
		{"profile", func(id string, version int) (*user.User, error) {
			return repo.UpdateProfile(ctx, id, "renamed", version)
		}},
		{"status", func(id string, version int) (*user.User, error) {
			return repo.UpdateStatus(ctx, id, user.StatusBlocked, version)
		}},
		{"password", func(id string, version int) (*user.User, error) {
			return repo.UpdatePassword(ctx, id, "new-hash", version)
		}},
	}
	for _, tc := range updates {
		t.Run(tc.name, func(t *testing.T) {
			u := pgseed.InsertUser(t, repo)

			got, err := tc.update(u.ID, u.Version)
			require.NoError(t, err)
			require.Equal(t, u.Version+1, got.Version)

			_, err = tc.update(u.ID, u.Version)
			require.ErrorIs(t, err, user.ErrConcurrentUpdate)
		})
	}
}
