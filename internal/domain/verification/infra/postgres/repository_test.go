//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	userpg "git.stit.tech/stit-core/go-backend-template/internal/domain/user/infra/postgres"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	verifpg "git.stit.tech/stit-core/go-backend-template/internal/domain/verification/infra/postgres"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/pgseed"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/repopath"
	"git.stit.tech/stit-core/golibinfra/testkit"
)

var tokenExpiry = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

// setup seeds a user because both token tables reference users(id).
func setup(t *testing.T) (verification.Repository, string) {
	t.Helper()
	fx := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations()))
	db := fx.DB(t)
	u := pgseed.InsertUser(t, userpg.NewRepository(db))
	return verifpg.NewRepository(db), u.ID
}

func TestInsertPasswordResetAndGet(t *testing.T) {
	t.Parallel()
	repo, userID := setup(t)
	ctx := context.Background()

	require.NoError(t, repo.InsertPasswordReset(ctx, userID, "hash:pr-1", tokenExpiry))

	got, err := repo.GetPasswordReset(ctx, "hash:pr-1")
	require.NoError(t, err)
	require.NotEmpty(t, got.ID)
	require.Equal(t, userID, got.UserID)
	require.WithinDuration(t, tokenExpiry, got.ExpiresAt, time.Second)
	require.Nil(t, got.UsedAt)
}

func TestGetPasswordReset_UnknownTokenHash(t *testing.T) {
	t.Parallel()
	repo, _ := setup(t)

	_, err := repo.GetPasswordReset(context.Background(), "hash:absent")
	require.ErrorIs(t, err, verification.ErrInvalidResetToken)
}

func TestMarkPasswordResetUsed(t *testing.T) {
	t.Parallel()
	repo, userID := setup(t)
	ctx := context.Background()

	require.NoError(t, repo.InsertPasswordReset(ctx, userID, "hash:pr-1", tokenExpiry))
	got, err := repo.GetPasswordReset(ctx, "hash:pr-1")
	require.NoError(t, err)

	require.NoError(t, repo.MarkPasswordResetUsed(ctx, got.ID))

	used, err := repo.GetPasswordReset(ctx, "hash:pr-1")
	require.NoError(t, err)
	require.NotNil(t, used.UsedAt)
}

func TestInsertEmailVerification_PendingIdempotency(t *testing.T) {
	t.Parallel()
	repo, userID := setup(t)
	ctx := context.Background()

	inserted, err := repo.InsertEmailVerification(ctx, userID, "hash:ev-1", tokenExpiry)
	require.NoError(t, err)
	require.True(t, inserted, "first insert must create a pending verification")

	// Redelivery: a pending row exists -> ON CONFLICT no-op (partial unique
	// index email_verifications_pending_user_idx, migration 0004).
	inserted, err = repo.InsertEmailVerification(ctx, userID, "hash:ev-2", tokenExpiry)
	require.NoError(t, err)
	require.False(t, inserted, "second insert while pending must report inserted=false")

	_, err = repo.GetEmailVerification(ctx, "hash:ev-2")
	require.ErrorIs(t, err, verification.ErrInvalidVerificationToken, "no-op insert must not create a row")

	got, err := repo.GetEmailVerification(ctx, "hash:ev-1")
	require.NoError(t, err)
	require.Equal(t, userID, got.UserID)
	require.Nil(t, got.UsedAt)

	require.NoError(t, repo.MarkEmailVerificationUsed(ctx, got.ID))

	used, err := repo.GetEmailVerification(ctx, "hash:ev-1")
	require.NoError(t, err)
	require.NotNil(t, used.UsedAt)

	inserted, err = repo.InsertEmailVerification(ctx, userID, "hash:ev-3", tokenExpiry)
	require.NoError(t, err)
	require.True(t, inserted, "insert after the pending token is used must succeed")
}

func TestGetEmailVerification_UnknownTokenHash(t *testing.T) {
	t.Parallel()
	repo, _ := setup(t)

	_, err := repo.GetEmailVerification(context.Background(), "hash:absent")
	require.ErrorIs(t, err, verification.ErrInvalidVerificationToken)
}
