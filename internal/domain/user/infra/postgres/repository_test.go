//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/akadmin/go-backend-template/internal/domain/user"
	userpg "git.stit.tech/akadmin/go-backend-template/internal/domain/user/infra/postgres"
	"git.stit.tech/akadmin/go-backend-template/internal/platform/identity"
	"git.stit.tech/akadmin/go-backend-template/internal/testkit/pgseed"
	"git.stit.tech/stit/golibinfra/testkit"
)

func setup(t *testing.T) *userpg.Repository {
	t.Helper()
	fx := testkit.StartPostgres(t, "../../../../migrations")
	return userpg.NewRepository(fx.DB(t))
}

func TestCreate(t *testing.T) {
	repo := setup(t)
	ctx := context.Background()

	u := pgseed.InsertUser(t, repo)
	require.NotEmpty(t, u.ID)
	require.Equal(t, identity.RoleClient, u.Role)

	dup := pgseed.NewUser(pgseed.WithEmail(u.Email))
	err := repo.Create(ctx, dup)
	require.ErrorIs(t, err, user.ErrAlreadyExists)
}

func TestGetByID(t *testing.T) {
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
