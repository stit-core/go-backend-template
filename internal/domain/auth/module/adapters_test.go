// White-box (package module): адаптер — неэкспортируемый тип, проверяется его маппинг полей и ошибок
// поверх настоящего user.Trusted.
package module

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	usermocks "git.stit.tech/stit-core/go-backend-template/internal/domain/user/mocks"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/testkit/authntest"
	"git.stit.tech/stit-core/golibinfra/testkit/pgtest"
)

func newAdapter(t *testing.T) (*userAdapter, *usermocks.MockRepository) {
	t.Helper()
	repo := usermocks.NewMockRepository(t)
	return newUserAdapter(user.NewTrusted(user.TrustedDeps{
		Repo: repo, Hasher: &authntest.DeterministicHasher{}, Tx: pgtest.Identity{}, Sessions: usermocks.NewMockSessions(t),
	})), repo
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
			found := &user.User{ID: "u1", Email: "a@b.com", PasswordHash: "h", Name: "Alice", Status: tc.status, Role: identity.RoleAdmin, Version: 3}
			repo.EXPECT().GetByID(mock.Anything, "u1").Return(found, nil).Once()
			repo.EXPECT().GetByIDForShare(mock.Anything, "u1").Return(found, nil).Once()
			repo.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(found, nil).Once()
			want := &auth.User{ID: "u1", Email: "a@b.com", PasswordHash: "h", Name: "Alice", CanLogin: tc.canLogin, Role: identity.RoleAdmin, Version: 3}

			byID, err := a.GetByID(context.Background(), "u1")
			require.NoError(t, err)
			require.Equal(t, want, byID)
			locked, err := a.LockByID(context.Background(), "u1")
			require.NoError(t, err)
			require.Equal(t, want, locked)
			byEmail, err := a.GetByEmail(context.Background(), "a@b.com")
			require.NoError(t, err)
			require.Equal(t, want, byEmail)
		})
	}
}

func TestUserAdapter_VerifyPassword(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		plain string
		want  bool
	}{
		{"match", "pass1234", true},
		{"mismatch", "wrong-password", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, repo := newAdapter(t)
			repo.EXPECT().GetByID(mock.Anything, "u1").Return(&user.User{ID: "u1", PasswordHash: "hash:pass1234"}, nil).Once()

			ok, err := a.VerifyPassword(context.Background(), "u1", tc.plain)
			require.NoError(t, err)
			require.Equal(t, tc.want, ok)
		})
	}
}

// Чтение под блокировкой идёт в транзакции входа: адаптер передаёт её ctx в user.Trusted, и строка
// пользователя держится до открытия сессии.
func TestUserAdapter_LocksInCallerTx(t *testing.T) {
	t.Parallel()
	a, repo := newAdapter(t)
	repo.EXPECT().GetByIDForShare(mock.MatchedBy(pgkit.InTx), "u1").Return(&user.User{ID: "u1", Status: user.StatusActive}, nil).Once()

	u, err := a.LockByID(pgkit.WithTestTx(context.Background()), "u1")
	require.NoError(t, err)
	require.Equal(t, "u1", u.ID)
}

// Каждая ошибка из user.TrustedErrors доходит до auth своим sentinel'ом, а не чужим: новая
// ошибка Trusted без перевода роняет этот тест.
func TestUserAdapter_MapsUserErrors(t *testing.T) {
	t.Parallel()
	translated := map[error]error{
		user.ErrNotFound:         auth.ErrUserNotFound,
		user.ErrConcurrentUpdate: auth.ErrConcurrentUpdate,
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
		{"LockByID", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().GetByIDForShare(mock.Anything, "u1").Return(nil, err).Once()
			_, got := a.LockByID(context.Background(), "u1")
			return got
		}},
		{"GetByEmail", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(nil, err).Once()
			_, got := a.GetByEmail(context.Background(), "a@b.com")
			return got
		}},
		{"VerifyPassword", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().GetByID(mock.Anything, "u1").Return(nil, err).Once()
			_, got := a.VerifyPassword(context.Background(), "u1", "secret")
			return got
		}},
		{"UpdatePassword", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().UpdatePassword(mock.Anything, "u1", "hash", 3).Return(nil, err).Once()
			return a.UpdatePassword(context.Background(), "u1", "hash", 3)
		}},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			for _, from := range user.TrustedErrors {
				want, ok := translated[from]
				require.True(t, ok, "%v from user.Trusted has no auth error", from)
				a, repo := newAdapter(t)
				got := c.call(a, repo, from)
				require.ErrorIs(t, got, want)
				require.NotErrorIs(t, got, from, "the user sentinel must not leak to auth")
			}
			a, repo := newAdapter(t)
			require.ErrorIs(t, c.call(a, repo, infra), infra, "an infrastructure error passes through")
		})
	}
}
