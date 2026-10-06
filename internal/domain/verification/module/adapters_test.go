// White-box (package module): адаптер — неэкспортируемый тип, проверяется его маппинг полей и ошибок
// поверх настоящего user.Trusted.
package module

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	usermocks "git.stit.tech/stit-core/go-backend-template/internal/domain/user/mocks"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/testkit/authntest"
)

func newAdapter(t *testing.T) (*userAdapter, *usermocks.MockRepository) {
	t.Helper()
	repo := usermocks.NewMockRepository(t)
	return newUserAdapter(user.NewTrusted(user.TrustedDeps{Repo: repo, Hasher: authntest.DeterministicHasher{}})), repo
}

func TestUserAdapter_User(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   user.Status
		canLogin bool
	}{
		{"active", user.StatusActive, true},
		{"blocked", user.StatusBlocked, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, repo := newAdapter(t)
			found := &user.User{ID: "u1", Email: "a@b.com", Status: tc.status, Version: 3}
			repo.EXPECT().GetByID(mock.Anything, "u1").Return(found, nil).Once()
			repo.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(found, nil).Once()
			want := &verification.User{ID: "u1", Email: "a@b.com", CanLogin: tc.canLogin, Version: 3}

			byID, err := a.GetByID(context.Background(), "u1")
			require.NoError(t, err)
			require.Equal(t, want, byID)
			byEmail, err := a.GetByEmail(context.Background(), "a@b.com")
			require.NoError(t, err)
			require.Equal(t, want, byEmail)
		})
	}
}

// Запись пароля и отметка email остаются в транзакции verification: адаптер передаёт её ctx в user.Trusted.
func TestUserAdapter_WritesInCallerTx(t *testing.T) {
	t.Parallel()
	a, repo := newAdapter(t)
	repo.EXPECT().UpdatePassword(mock.MatchedBy(pgkit.InTx), "u1", "hash", 3).Return(&user.User{ID: "u1", Version: 4}, nil).Once()
	repo.EXPECT().SetEmailVerified(mock.MatchedBy(pgkit.InTx), "u1").Return(nil).Once()
	ctx := pgkit.WithTestTx(context.Background())

	require.NoError(t, a.UpdatePassword(ctx, "u1", "hash", 3))
	require.NoError(t, a.MarkEmailVerified(ctx, "u1"))
}

// Каждая ошибка из user.TrustedErrors доходит до verification своим sentinel'ом, а не чужим: новая
// ошибка Trusted без перевода роняет этот тест.
func TestUserAdapter_MapsUserErrors(t *testing.T) {
	t.Parallel()
	translated := map[error]error{
		user.ErrNotFound:         verification.ErrUserNotFound,
		user.ErrConcurrentUpdate: verification.ErrConcurrentUpdate,
	}
	infra := errors.New("db down")
	calls := []struct {
		name string
		call func(a *userAdapter, repo *usermocks.MockRepository, err error) error
	}{
		{"GetByID", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().GetByID(mock.Anything, "u1").Return(nil, err).Once()
			_, got := a.GetByID(context.Background(), "u1")
			return got
		}},
		{"GetByEmail", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(nil, err).Once()
			_, got := a.GetByEmail(context.Background(), "a@b.com")
			return got
		}},
		{"UpdatePassword", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().UpdatePassword(mock.Anything, "u1", "hash", 3).Return(nil, err).Once()
			return a.UpdatePassword(context.Background(), "u1", "hash", 3)
		}},
		{"MarkEmailVerified", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().SetEmailVerified(mock.Anything, "u1").Return(err).Once()
			return a.MarkEmailVerified(context.Background(), "u1")
		}},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			for _, from := range user.TrustedErrors {
				want, ok := translated[from]
				require.True(t, ok, "%v from user.Trusted has no verification error", from)
				a, repo := newAdapter(t)
				got := c.call(a, repo, from)
				require.ErrorIs(t, got, want)
				require.NotErrorIs(t, got, from, "the user sentinel must not leak to verification")
			}
			a, repo := newAdapter(t)
			require.ErrorIs(t, c.call(a, repo, infra), infra, "an infrastructure error passes through")
		})
	}
}
