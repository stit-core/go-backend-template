//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	authpg "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/infra/postgres"
	userpg "git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/postgres"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/pgseed"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/repopath"
	"git.stit.tech/stit-core/golibinfra/testkit"
)

var sessionExpiry = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

// setup seeds a user because refresh_sessions.user_id references users(id).
func setup(t *testing.T) (auth.SessionRepository, string) {
	t.Helper()
	fx := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations()))
	db := fx.DB(t)
	u := pgseed.InsertUser(t, userpg.NewRepository(db))
	return authpg.NewRepository(db), u.ID
}

func TestInsertAndGetByJTI(t *testing.T) {
	repo, userID := setup(t)
	ctx := context.Background()

	jti := uuid.NewString()
	require.NoError(t, repo.Insert(ctx, jti, userID, sessionExpiry))

	got, err := repo.GetByJTI(ctx, jti)
	require.NoError(t, err)
	require.Equal(t, jti, got.JTI)
	require.Equal(t, userID, got.UserID)
	require.WithinDuration(t, sessionExpiry, got.ExpiresAt, time.Second)
	require.False(t, got.IssuedAt.IsZero(), "issued_at must come from the DB default")
	require.Nil(t, got.UsedAt)
	require.Empty(t, got.ReplacedByJTI)
}

func TestGetByJTI_NotFound(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()

	tests := []struct {
		name string
		jti  string
	}{
		{name: "unknown jti", jti: uuid.NewString()},
		{name: "malformed jti", jti: "not-a-uuid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.GetByJTI(ctx, tc.jti)
			require.ErrorIs(t, err, auth.ErrSessionNotFound)
		})
	}
}

func TestMarkUsed(t *testing.T) {
	repo, userID := setup(t)
	ctx := context.Background()

	jti := uuid.NewString()
	replacedBy := uuid.NewString()
	require.NoError(t, repo.Insert(ctx, jti, userID, sessionExpiry))

	require.NoError(t, repo.MarkUsed(ctx, jti, replacedBy))

	got, err := repo.GetByJTI(ctx, jti)
	require.NoError(t, err)
	require.NotNil(t, got.UsedAt)
	require.Equal(t, replacedBy, got.ReplacedByJTI)
}

func TestMarkUsed_MissingJTI(t *testing.T) {
	repo, _ := setup(t)

	// The UPDATE is :exec without rows-affected check: a jti matching no row
	// is a silent no-op, not an error.
	require.NoError(t, repo.MarkUsed(context.Background(), uuid.NewString(), ""))
}
