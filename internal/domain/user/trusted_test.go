package user_test

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
	"git.stit.tech/stit-core/go-backend-template/internal/domain/user/mocks"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/bruteforcekit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/testkit/authntest"
	"git.stit.tech/stit-core/golibinfra/testkit/pgtest"
)

// newTrusted — Trusted на строгих моках репозитория и сессий (nil — без ожиданий: их вызов — провал).
func newTrusted(t *testing.T, repo *mocks.MockRepository, sessions *mocks.MockSessions) *user.Trusted {
	t.Helper()
	if sessions == nil {
		sessions = mocks.NewMockSessions(t)
	}
	return user.NewTrusted(user.TrustedDeps{Repo: repo, Hasher: &authntest.DeterministicHasher{}, Tx: pgtest.Identity{}, Sessions: sessions})
}

func TestTrusted_Lookup(t *testing.T) {
	t.Parallel()
	want := stored()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetByID(mock.Anything, "u1").Return(want, nil).Once()
	repo.EXPECT().GetByIDForShare(mock.Anything, "u1").Return(want, nil).Once()
	repo.EXPECT().GetByEmail(mock.Anything, "A@b.com").Return(nil, user.ErrNotFound).Once()
	tr := newTrusted(t, repo, nil)

	got, err := tr.LookupByID(context.Background(), "u1")
	require.NoError(t, err)
	require.Same(t, want, got)
	locked, err := tr.LockByID(context.Background(), "u1")
	require.NoError(t, err)
	require.Same(t, want, locked, "the locking read is the repository's FOR SHARE read")
	_, err = tr.LookupByEmail(context.Background(), " A@b.com\t")
	require.ErrorIs(t, err, user.ErrNotFound, "the address is looked up without the spaces around it, the letter case is the query's")
}

// Поиск по адресу и guard входа считают один адрес одним субъектом: ключ поиска — ключ guard'а
// (bruteforcekit.NormalizeKey) без нижнего регистра, его снимает LOWER(email) запроса.
func FuzzTrusted_LookupByEmail(f *testing.F) {
	for _, seed := range []string{"a@b.com", " a@b.com", "A@B.com\t", "\u00a0a@b.com\n", "a b@c.d", ""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, email string) {
		subject, _, err := bruteforcekit.Config{}.NormalizeKey(email, "192.0.2.1")
		if err != nil || strings.HasPrefix(subject, "SHA256:") {
			t.Skip("the guard keys no such subject or keys it by its hash")
		}
		var key string
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByEmail(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, k string) (*user.User, error) {
			key = k
			return nil, user.ErrNotFound
		}).Once()

		_, err = newTrusted(t, repo, nil).LookupByEmail(context.Background(), email)
		require.ErrorIs(t, err, user.ErrNotFound)
		require.Equal(t, subject, strings.ToLower(key), "email %q", email)
	})
}

// Хэш нового пароля — по правилу пароля продукта; пароль длиннее предела хэшера — ошибка поля, сбой хэшера — ошибка.
func TestTrusted_HashPassword(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		plain  string
		hasher *authntest.DeterministicHasher
		want   string
		err    error
	}{
		{name: "strong", plain: "secret-123", hasher: &authntest.DeterministicHasher{}, want: "hash:secret-123"},
		{name: "weak", plain: "short", hasher: &authntest.DeterministicHasher{}, err: user.ErrWeakPassword},
		{name: "over the hasher limit", plain: strings.Repeat("я", authnkit.MaxPasswordLen), hasher: &authntest.DeterministicHasher{}, err: user.ErrPasswordTooLong},
		{name: "hasher failure", plain: "secret-123", hasher: &authntest.DeterministicHasher{HashErr: errHashing}, err: errHashing},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := user.NewTrusted(user.TrustedDeps{Repo: mocks.NewMockRepository(t), Hasher: tc.hasher, Tx: pgtest.Identity{}, Sessions: mocks.NewMockSessions(t)})

			got, err := tr.HashPassword(context.Background(), tc.plain)
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, tc.want, got)
		})
	}
}

// Новый пароль и закрытие всех сессий пользователя — одна транзакция: сессии старого пароля не переживают
// ни смену, ни сброс (их пишут auth и verification через адаптеры).
func TestTrusted_UpdatePassword(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	write := repo.EXPECT().UpdatePassword(mock.MatchedBy(pgkit.InTx), "u1", "hash:new", 3).Return(&user.User{ID: "u1", Version: 4}, nil).Once()
	sessions := mocks.NewMockSessions(t)
	sessions.EXPECT().RevokeAll(mock.MatchedBy(pgkit.InTx), "u1").Return(nil).Once().NotBefore(write)
	tx := &pgtest.Recorder{}
	tr := user.NewTrusted(user.TrustedDeps{Repo: repo, Hasher: &authntest.DeterministicHasher{}, Tx: tx, Sessions: sessions})

	require.NoError(t, tr.UpdatePassword(context.Background(), "u1", "hash:new", 3))
	require.Equal(t, 1, tx.Calls(), "the password and the sessions change in one transaction")
}

// Отказ записи пароля сессий не трогает; несданные сессии — отказ всей смены (транзакция откатывается).
func TestTrusted_UpdatePassword_Rejected(t *testing.T) {
	t.Parallel()
	errRevoke := errors.New("authn_sessions: connection refused")
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().UpdatePassword(mock.Anything, "u1", "hash:new", 2).Return(nil, user.ErrConcurrentUpdate).Once()
	repo.EXPECT().UpdatePassword(mock.Anything, "u1", "hash:new", 3).Return(&user.User{ID: "u1", Version: 4}, nil).Once()
	sessions := mocks.NewMockSessions(t)
	sessions.EXPECT().RevokeAll(mock.Anything, "u1").Return(errRevoke).Once()
	tr := newTrusted(t, repo, sessions)

	require.ErrorIs(t, tr.UpdatePassword(context.Background(), "u1", "hash:new", 2), user.ErrConcurrentUpdate)
	require.ErrorIs(t, tr.UpdatePassword(context.Background(), "u1", "hash:new", 3), errRevoke)
}

func TestTrusted_MarkEmailVerified(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().SetEmailVerified(mock.Anything, "u1").Return(nil).Once()
	repo.EXPECT().SetEmailVerified(mock.Anything, "gone").Return(user.ErrNotFound).Once()
	tr := newTrusted(t, repo, nil)

	require.NoError(t, tr.MarkEmailVerified(context.Background(), "u1"))
	require.ErrorIs(t, tr.MarkEmailVerified(context.Background(), "gone"), user.ErrNotFound)
}

// На каждом исходе своего порта метод Trusted отдаёт ровно ошибки своей строки TrustedErrors и сбои
// инфраструктуры (свои return — archtest): их переводят адаптеры потребителей, ошибка вне строки ушла бы им 500, а
// строка про запас заставила бы переводить невозможное. Новый метод без строки в calls роняет этот тест.
func TestTrusted_ErrorsAreDeclared(t *testing.T) {
	t.Parallel()
	infra := errors.New("db down")
	found := func(repoErr error) *user.User {
		if repoErr != nil {
			return nil
		}
		return &user.User{ID: "u1", PasswordHash: "hash:secret-1", Version: 3}
	}
	type call func(tr *user.Trusted, repo *mocks.MockRepository, sessions *mocks.MockSessions, repoErr error) error
	// calls — метод → исходы его порта (контракт Repository) и вызов.
	calls := map[string]struct {
		outcomes []error
		call     call
	}{
		"LookupByID": {[]error{user.ErrNotFound}, func(tr *user.Trusted, repo *mocks.MockRepository, _ *mocks.MockSessions, repoErr error) error {
			repo.EXPECT().GetByID(mock.Anything, "u1").Return(found(repoErr), repoErr).Once()
			_, err := tr.LookupByID(context.Background(), "u1")
			return err
		}},
		"LockByID": {[]error{user.ErrNotFound}, func(tr *user.Trusted, repo *mocks.MockRepository, _ *mocks.MockSessions, repoErr error) error {
			repo.EXPECT().GetByIDForShare(mock.Anything, "u1").Return(found(repoErr), repoErr).Once()
			_, err := tr.LockByID(context.Background(), "u1")
			return err
		}},
		"LookupByEmail": {[]error{user.ErrNotFound}, func(tr *user.Trusted, repo *mocks.MockRepository, _ *mocks.MockSessions, repoErr error) error {
			repo.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(found(repoErr), repoErr).Once()
			_, err := tr.LookupByEmail(context.Background(), "a@b.com")
			return err
		}},
		// Исходы HashPassword — от пароля и хэшера, а не от репозитория.
		"HashPassword": {[]error{user.ErrWeakPassword, user.ErrPasswordTooLong}, func(tr *user.Trusted, _ *mocks.MockRepository, _ *mocks.MockSessions, outcome error) error {
			plain := map[error]string{nil: "secret-123", user.ErrWeakPassword: "short", user.ErrPasswordTooLong: strings.Repeat("я", authnkit.MaxPasswordLen)}
			if _, input := plain[outcome]; !input {
				tr = user.NewTrusted(user.TrustedDeps{Hasher: &authntest.DeterministicHasher{HashErr: outcome}})
			}
			_, err := tr.HashPassword(context.Background(), cmp.Or(plain[outcome], "secret-123"))
			return err
		}},
		"UpdatePassword": {[]error{user.ErrNotFound, user.ErrConcurrentUpdate}, func(tr *user.Trusted, repo *mocks.MockRepository, sessions *mocks.MockSessions, repoErr error) error {
			repo.EXPECT().UpdatePassword(mock.Anything, "u1", "hash:new", 3).Return(found(repoErr), repoErr).Once()
			if repoErr == nil {
				sessions.EXPECT().RevokeAll(mock.Anything, "u1").Return(nil).Once()
			}
			return tr.UpdatePassword(context.Background(), "u1", "hash:new", 3)
		}},
		"MarkEmailVerified": {[]error{user.ErrNotFound}, func(tr *user.Trusted, repo *mocks.MockRepository, _ *mocks.MockSessions, repoErr error) error {
			repo.EXPECT().SetEmailVerified(mock.Anything, "u1").Return(repoErr).Once()
			return tr.MarkEmailVerified(context.Background(), "u1")
		}},
	}
	var methods []string
	for m := range reflect.TypeFor[*user.Trusted]().Methods() {
		methods = append(methods, m.Name)
	}
	require.Equal(t, methods, slices.Sorted(maps.Keys(calls)), "every method of Trusted is checked here")
	require.Equal(t, methods, slices.Sorted(maps.Keys(user.TrustedErrors)), "every method of Trusted has its line in TrustedErrors")
	for name, c := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var produced []error
			for _, repoErr := range append([]error{nil, infra}, c.outcomes...) {
				repo, sessions := mocks.NewMockRepository(t), mocks.NewMockSessions(t)
				err := c.call(newTrusted(t, repo, sessions), repo, sessions, repoErr)
				switch {
				case err == nil, errors.Is(err, infra):
				default:
					i := slices.IndexFunc(user.TrustedErrors[name], func(e error) bool { return errors.Is(err, e) })
					require.NotEqual(t, -1, i, "%v is not in TrustedErrors[%q]", err, name)
					produced = append(produced, user.TrustedErrors[name][i])
				}
			}
			for _, listed := range user.TrustedErrors[name] {
				require.Contains(t, produced, listed, "TrustedErrors[%q] lists %v, which the method never returns", name, listed)
			}
		})
	}
}
