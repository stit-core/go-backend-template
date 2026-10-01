//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
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

	id, err := repo.InsertPasswordReset(ctx, userID, "hash:pr-1", tokenExpiry)
	require.NoError(t, err)

	got, err := repo.GetPasswordReset(ctx, "hash:pr-1")
	require.NoError(t, err)
	require.Equal(t, id, got.ID, "the event carries the id of the stored token")
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

	id, err := repo.InsertPasswordReset(ctx, userID, "hash:pr-1", tokenExpiry)
	require.NoError(t, err)

	require.NoError(t, repo.MarkPasswordResetUsed(ctx, id))
	require.ErrorIs(t, repo.MarkPasswordResetUsed(ctx, id), verification.ErrTokenUsed)

	used, err := repo.GetPasswordReset(ctx, "hash:pr-1")
	require.NoError(t, err)
	require.NotNil(t, used.UsedAt)
}

// Ожидающее подтверждение пользователя одно: повтор доставки до отправки письма перевыпускает
// токен, после отправки — не трогает.
func TestUpsertEmailVerification_Redelivery(t *testing.T) {
	t.Parallel()
	repo, userID := setup(t)
	ctx := context.Background()

	id, issued, err := repo.UpsertEmailVerification(ctx, userID, "hash:ev-1", tokenExpiry)
	require.NoError(t, err)
	require.True(t, issued)

	reissued, issued, err := repo.UpsertEmailVerification(ctx, userID, "hash:ev-2", tokenExpiry.Add(time.Hour))
	require.NoError(t, err)
	require.True(t, issued, "an unsent verification is issued again")
	require.Equal(t, id, reissued, "the same pending verification")
	_, err = repo.GetEmailVerification(ctx, "hash:ev-1")
	require.ErrorIs(t, err, verification.ErrInvalidVerificationToken, "the unsent token is replaced")
	got, err := repo.GetEmailVerification(ctx, "hash:ev-2")
	require.NoError(t, err)
	require.Equal(t, id, got.ID)
	require.WithinDuration(t, tokenExpiry.Add(time.Hour), got.ExpiresAt, time.Second)

	require.NoError(t, repo.MarkEmailVerificationSent(ctx, id))
	_, issued, err = repo.UpsertEmailVerification(ctx, userID, "hash:ev-3", tokenExpiry)
	require.NoError(t, err)
	require.False(t, issued, "a sent verification is not issued again")
	_, err = repo.GetEmailVerification(ctx, "hash:ev-3")
	require.ErrorIs(t, err, verification.ErrInvalidVerificationToken)
	_, err = repo.GetEmailVerification(ctx, "hash:ev-2")
	require.NoError(t, err, "the sent token stays valid")

	require.NoError(t, repo.MarkEmailVerificationUsed(ctx, id))
	require.ErrorIs(t, repo.MarkEmailVerificationUsed(ctx, id), verification.ErrTokenUsed)
	next, issued, err := repo.UpsertEmailVerification(ctx, userID, "hash:ev-4", tokenExpiry)
	require.NoError(t, err)
	require.True(t, issued, "a used verification no longer blocks a new one")
	require.NotEqual(t, id, next)
}

// Одновременные погашения одного токена: ровно одно успешное, остальные — ErrTokenUsed. Каждое — в
// своей транзакции, как в usecase; первая держит блокировку строки, остальные ждут её.
func TestMarkUsed_Concurrent(t *testing.T) {
	t.Parallel()
	pg := testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations()))
	repo := verifpg.NewRepository(pg.DB(t))
	tx := pg.Transactor(t)
	ctx := context.Background()
	userID := pgseed.InsertUser(t, userpg.NewRepository(pg.DB(t))).ID

	resetID, err := repo.InsertPasswordReset(ctx, userID, "hash:pr", tokenExpiry)
	require.NoError(t, err)
	verifyID, _, err := repo.UpsertEmailVerification(ctx, userID, "hash:ev", tokenExpiry)
	require.NoError(t, err)

	tests := []struct {
		name  string
		claim func(ctx context.Context) error
	}{
		{"password reset", func(ctx context.Context) error { return repo.MarkPasswordResetUsed(ctx, resetID) }},
		{"email verification", func(ctx context.Context) error { return repo.MarkEmailVerificationUsed(ctx, verifyID) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const n = 8
			errs := make(chan error, n)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for range n {
				wg.Go(func() {
					<-start
					errs <- tx.WithTx(ctx, func(txCtx context.Context) error {
						if err := tc.claim(txCtx); err != nil {
							return err
						}
						time.Sleep(50 * time.Millisecond)
						return nil
					})
				})
			}
			close(start)
			wg.Wait()
			close(errs)

			var claimed, used int
			for err := range errs {
				switch {
				case err == nil:
					claimed++
				case errors.Is(err, verification.ErrTokenUsed):
					used++
				default:
					require.NoError(t, err)
				}
			}
			require.Equal(t, 1, claimed, "exactly one claim succeeds")
			require.Equal(t, n-1, used)
		})
	}
}

func TestGetEmailVerification_UnknownTokenHash(t *testing.T) {
	t.Parallel()
	repo, _ := setup(t)

	_, err := repo.GetEmailVerification(context.Background(), "hash:absent")
	require.ErrorIs(t, err, verification.ErrInvalidVerificationToken)
}
