// White-box (package module): адаптер — неэкспортируемый тип, проверяется его маппинг полей и ошибок
// поверх настоящего user.Trusted.
package module

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	usermocks "git.stit.tech/stit-core/go-backend-template/internal/domain/user/mocks"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/golibinfra/authnkit"
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

// Ошибки метода user.Trusted, который зовёт метод адаптера (TrustedErrors[метод]), доходят до auth своими
// sentinel'ами, а не чужими: новая ошибка метода без перевода и новый метод без строки в calls роняют этот тест.
func TestUserAdapter_MapsUserErrors(t *testing.T) {
	t.Parallel()
	translated := map[error]error{
		user.ErrNotFound:         auth.ErrUserNotFound,
		user.ErrConcurrentUpdate: auth.ErrConcurrentUpdate,
		user.ErrWeakPassword:     auth.ErrWeakPassword,
		user.ErrPasswordTooLong:  auth.ErrPasswordTooLong,
		user.ErrBusy:             auth.ErrBusy,
	}
	infra := errors.New("db down")
	type call func(a *userAdapter, repo *usermocks.MockRepository, err error) error
	// calls — метод адаптера → метод user.Trusted, который он зовёт, и вызов.
	calls := map[string]struct {
		trusted string
		call    call
	}{
		"GetByID": {"LookupByID", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().GetByID(mock.Anything, "u1").Return(nil, err).Once()
			_, got := a.GetByID(context.Background(), "u1")
			return got
		}},
		"LockByID": {"LockByID", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().GetByIDForShare(mock.Anything, "u1").Return(nil, err).Once()
			_, got := a.LockByID(context.Background(), "u1")
			return got
		}},
		"GetByEmail": {"LookupByEmail", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(nil, err).Once()
			_, got := a.GetByEmail(context.Background(), "a@b.com")
			return got
		}},
		// Исходы HashPassword — от пароля и хэшера: перегрузка — у адаптера с полной очередью хэшера, сбой (infra) — с
		// отказом хэшера.
		"HashPassword": {"HashPassword", func(a *userAdapter, _ *usermocks.MockRepository, err error) error {
			plain := map[error]string{user.ErrWeakPassword: "short", user.ErrPasswordTooLong: strings.Repeat("я", authnkit.MaxPasswordLen)}
			switch _, input := plain[err]; {
			case errors.Is(err, user.ErrBusy):
				a = newUserAdapter(user.NewTrusted(user.TrustedDeps{Hasher: &authntest.DeterministicHasher{Busy: true}}))
			case !input:
				a = newUserAdapter(user.NewTrusted(user.TrustedDeps{Hasher: &authntest.DeterministicHasher{HashErr: err}}))
			}
			_, got := a.HashPassword(context.Background(), cmp.Or(plain[err], "longenough"))
			return got
		}},
		"UpdatePassword": {"UpdatePassword", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().UpdatePassword(mock.Anything, "u1", "hash", 3).Return(nil, err).Once()
			return a.UpdatePassword(context.Background(), "u1", "hash", 3)
		}},
	}
	var methods []string
	for m := range reflect.TypeFor[*userAdapter]().Methods() {
		methods = append(methods, m.Name)
	}
	require.Equal(t, methods, slices.Sorted(maps.Keys(calls)), "every method of the adapter is checked here")
	for name, c := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Contains(t, user.TrustedErrors, c.trusted)
			for _, from := range user.TrustedErrors[c.trusted] {
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

// Хэш нового пароля адаптер отдаёт как есть: правило и хэш — у владельца учётной записи.
func TestUserAdapter_HashPassword(t *testing.T) {
	t.Parallel()
	a := newUserAdapter(user.NewTrusted(user.TrustedDeps{Hasher: &authntest.DeterministicHasher{}}))

	hash, err := a.HashPassword(context.Background(), "longenough")
	require.NoError(t, err)
	require.Equal(t, "hash:longenough", hash)
}

// Перегрузка хэшера доходит до auth его ErrBusy, а ошибка кита остаётся в цепочке: из неё Retry-After ответа 503.
func TestUserAdapter_HashPasswordBusy(t *testing.T) {
	t.Parallel()
	a := newUserAdapter(user.NewTrusted(user.TrustedDeps{Hasher: &authntest.DeterministicHasher{Busy: true}}))

	_, err := a.HashPassword(context.Background(), "longenough")
	require.ErrorIs(t, err, auth.ErrBusy)
	require.ErrorIs(t, err, authnkit.ErrBusy, "the kit's error carries the Retry-After of the answer")
}
