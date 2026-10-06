package user_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/user/mocks"
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
	return user.NewTrusted(user.TrustedDeps{Repo: repo, Hasher: authntest.DeterministicHasher{}, Tx: pgtest.Identity{}, Sessions: sessions})
}

func TestTrusted_Lookup(t *testing.T) {
	t.Parallel()
	want := stored()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetByID(mock.Anything, "u1").Return(want, nil).Once()
	repo.EXPECT().GetByIDForShare(mock.Anything, "u1").Return(want, nil).Once()
	repo.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(nil, user.ErrNotFound).Once()
	tr := newTrusted(t, repo, nil)

	got, err := tr.LookupByID(context.Background(), "u1")
	require.NoError(t, err)
	require.Same(t, want, got)
	locked, err := tr.LockByID(context.Background(), "u1")
	require.NoError(t, err)
	require.Same(t, want, locked, "the locking read is the repository's FOR SHARE read")
	_, err = tr.LookupByEmail(context.Background(), "a@b.com")
	require.ErrorIs(t, err, user.ErrNotFound)
}

func TestTrusted_VerifyPassword(t *testing.T) {
	t.Parallel()
	withHash := &user.User{ID: "u1", PasswordHash: "hash:secret-1"}
	tests := []struct {
		name    string
		plain   string
		found   *user.User
		repoErr error
		want    bool
	}{
		{"match", "secret-1", withHash, nil, true},
		{"mismatch", "other", withHash, nil, false},
		{"unknown user", "secret-1", nil, user.ErrNotFound, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := mocks.NewMockRepository(t)
			repo.EXPECT().GetByID(mock.Anything, "u1").Return(tc.found, tc.repoErr).Once()

			got, err := newTrusted(t, repo, nil).VerifyPassword(context.Background(), "u1", tc.plain)
			require.ErrorIs(t, err, tc.repoErr)
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
	tr := user.NewTrusted(user.TrustedDeps{Repo: repo, Hasher: authntest.DeterministicHasher{}, Tx: tx, Sessions: sessions})

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

// На каждом исходе репозитория методы Trusted отдают только ошибки TrustedErrors и сбои инфраструктуры
// (свои return — archtest): их переводят адаптеры потребителей, новая ошибка без перевода ушла бы им 500.
func TestTrusted_ErrorsAreDeclared(t *testing.T) {
	t.Parallel()
	infra := errors.New("db down")
	found := func(repoErr error) *user.User {
		if repoErr != nil {
			return nil
		}
		return &user.User{ID: "u1", PasswordHash: "hash:secret-1", Version: 3}
	}
	calls := []struct {
		name string
		call func(tr *user.Trusted, repo *mocks.MockRepository, sessions *mocks.MockSessions, repoErr error) error
	}{
		{"LookupByID", func(tr *user.Trusted, repo *mocks.MockRepository, _ *mocks.MockSessions, repoErr error) error {
			repo.EXPECT().GetByID(mock.Anything, "u1").Return(found(repoErr), repoErr).Once()
			_, err := tr.LookupByID(context.Background(), "u1")
			return err
		}},
		{"LockByID", func(tr *user.Trusted, repo *mocks.MockRepository, _ *mocks.MockSessions, repoErr error) error {
			repo.EXPECT().GetByIDForShare(mock.Anything, "u1").Return(found(repoErr), repoErr).Once()
			_, err := tr.LockByID(context.Background(), "u1")
			return err
		}},
		{"LookupByEmail", func(tr *user.Trusted, repo *mocks.MockRepository, _ *mocks.MockSessions, repoErr error) error {
			repo.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(found(repoErr), repoErr).Once()
			_, err := tr.LookupByEmail(context.Background(), "a@b.com")
			return err
		}},
		{"VerifyPassword", func(tr *user.Trusted, repo *mocks.MockRepository, _ *mocks.MockSessions, repoErr error) error {
			repo.EXPECT().GetByID(mock.Anything, "u1").Return(found(repoErr), repoErr).Once()
			_, err := tr.VerifyPassword(context.Background(), "u1", "secret-1")
			return err
		}},
		{"UpdatePassword", func(tr *user.Trusted, repo *mocks.MockRepository, sessions *mocks.MockSessions, repoErr error) error {
			repo.EXPECT().UpdatePassword(mock.Anything, "u1", "hash:new", 3).Return(found(repoErr), repoErr).Once()
			if repoErr == nil {
				sessions.EXPECT().RevokeAll(mock.Anything, "u1").Return(nil).Once()
			}
			return tr.UpdatePassword(context.Background(), "u1", "hash:new", 3)
		}},
		{"MarkEmailVerified", func(tr *user.Trusted, repo *mocks.MockRepository, _ *mocks.MockSessions, repoErr error) error {
			repo.EXPECT().SetEmailVerified(mock.Anything, "u1").Return(repoErr).Once()
			return tr.MarkEmailVerified(context.Background(), "u1")
		}},
	}
	outcomes := append([]error{nil, infra}, user.TrustedErrors...)
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			for _, repoErr := range outcomes {
				repo, sessions := mocks.NewMockRepository(t), mocks.NewMockSessions(t)
				err := c.call(newTrusted(t, repo, sessions), repo, sessions, repoErr)
				declared := err == nil || errors.Is(err, infra) ||
					slices.ContainsFunc(user.TrustedErrors, func(e error) bool { return errors.Is(err, e) })
				require.True(t, declared, "%v is not in user.TrustedErrors", err)
			}
		})
	}
}
