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

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	usermocks "git.stit.tech/stit-core/go-backend-template/internal/domain/user/mocks"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/testkit/authntest"
)

// callerTx — транзакции Trusted внутри транзакции вызывающего, как у pgkit: своей не открывает, поэтому
// запись видит транзакцию, только если её принёс ctx адаптера.
type callerTx struct{ pgkit.Transactor }

func (callerTx) WithTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func newAdapter(t *testing.T) (*userAdapter, *usermocks.MockRepository, *usermocks.MockSessions) {
	t.Helper()
	repo, sessions := usermocks.NewMockRepository(t), usermocks.NewMockSessions(t)
	a := newUserAdapter(user.NewTrusted(user.TrustedDeps{Repo: repo, Hasher: &authntest.DeterministicHasher{}, Tx: callerTx{}, Sessions: sessions}))
	return a, repo, sessions
}

func TestUserAdapter_User(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   user.Status
		verified bool
		canLogin bool
	}{
		{"active", user.StatusActive, false, true},
		{"blocked", user.StatusBlocked, false, false},
		{"verified address", user.StatusActive, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, repo, _ := newAdapter(t)
			found := &user.User{ID: "u1", Email: "a@b.com", Status: tc.status, Version: 3, EmailVerified: tc.verified}
			repo.EXPECT().GetByID(mock.Anything, "u1").Return(found, nil).Once()
			repo.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(found, nil).Once()
			want := &verification.User{ID: "u1", Email: "a@b.com", CanLogin: tc.canLogin, EmailVerified: tc.verified, Version: 3}

			byID, err := a.GetByID(context.Background(), "u1")
			require.NoError(t, err)
			require.Equal(t, want, byID)
			byEmail, err := a.GetByEmail(context.Background(), "a@b.com")
			require.NoError(t, err)
			require.Equal(t, want, byEmail)
		})
	}
}

// Запись пароля с закрытием сессий и отметка email остаются в транзакции verification: адаптер передаёт её
// ctx в user.Trusted.
func TestUserAdapter_WritesInCallerTx(t *testing.T) {
	t.Parallel()
	a, repo, sessions := newAdapter(t)
	repo.EXPECT().UpdatePassword(mock.MatchedBy(pgkit.InTx), "u1", "hash", 3).Return(&user.User{ID: "u1", Version: 4}, nil).Once()
	sessions.EXPECT().RevokeAll(mock.MatchedBy(pgkit.InTx), "u1").Return(nil).Once()
	repo.EXPECT().SetEmailVerified(mock.MatchedBy(pgkit.InTx), "u1").Return(nil).Once()
	ctx := pgkit.WithTestTx(context.Background())

	require.NoError(t, a.UpdatePassword(ctx, "u1", "hash", 3))
	require.NoError(t, a.MarkEmailVerified(ctx, "u1"))
}

// Ошибки метода user.Trusted, который зовёт метод адаптера (TrustedErrors[метод]), доходят до verification своими
// sentinel'ами, а не чужими: новая ошибка метода без перевода и новый метод без строки в calls роняют этот тест.
func TestUserAdapter_MapsUserErrors(t *testing.T) {
	t.Parallel()
	translated := map[error]error{
		user.ErrNotFound:         verification.ErrUserNotFound,
		user.ErrConcurrentUpdate: verification.ErrConcurrentUpdate,
		user.ErrWeakPassword:     verification.ErrWeakPassword,
		user.ErrPasswordTooLong:  verification.ErrPasswordTooLong,
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
		"GetByEmail": {"LookupByEmail", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(nil, err).Once()
			_, got := a.GetByEmail(context.Background(), "a@b.com")
			return got
		}},
		// Исходы HashPassword — от пароля и хэшера: сбой (infra) — у адаптера с отказом хэшера.
		"HashPassword": {"HashPassword", func(a *userAdapter, _ *usermocks.MockRepository, err error) error {
			plain := map[error]string{user.ErrWeakPassword: "short", user.ErrPasswordTooLong: strings.Repeat("я", authnkit.MaxPasswordLen)}
			if _, input := plain[err]; !input {
				a = newUserAdapter(user.NewTrusted(user.TrustedDeps{Hasher: &authntest.DeterministicHasher{HashErr: err}}))
			}
			_, got := a.HashPassword(context.Background(), cmp.Or(plain[err], "longenough"))
			return got
		}},
		"UpdatePassword": {"UpdatePassword", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().UpdatePassword(mock.Anything, "u1", "hash", 3).Return(nil, err).Once()
			return a.UpdatePassword(context.Background(), "u1", "hash", 3)
		}},
		"MarkEmailVerified": {"MarkEmailVerified", func(a *userAdapter, repo *usermocks.MockRepository, err error) error {
			repo.EXPECT().SetEmailVerified(mock.Anything, "u1").Return(err).Once()
			return a.MarkEmailVerified(context.Background(), "u1")
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
				require.True(t, ok, "%v from user.Trusted has no verification error", from)
				a, repo, _ := newAdapter(t)
				got := c.call(a, repo, from)
				require.ErrorIs(t, got, want)
				require.NotErrorIs(t, got, from, "the user sentinel must not leak to verification")
			}
			a, repo, _ := newAdapter(t)
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
