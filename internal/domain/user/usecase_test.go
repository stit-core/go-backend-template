package user_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	noopmetric "go.opentelemetry.io/otel/metric/noop"

	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/domain/user"
	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/domain/user/mocks"
	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/platform/identity"
	"git.sftbmb.ru/sftbmb-core/go-backend-template/internal/testkit/identitytest"
	"git.sftbmb.ru/sftbmb-core/go-backend-template/pkg/policy"
	"git.sftbmb.ru/sftbmb-core/golibinfra/clockkit"
	"git.sftbmb.ru/sftbmb-core/golibinfra/logkit"
	"git.sftbmb.ru/sftbmb-core/golibinfra/testkit/audittest"
	"git.sftbmb.ru/sftbmb-core/golibinfra/testkit/authntest"
	"git.sftbmb.ru/sftbmb-core/golibinfra/testkit/eventbus"
	"git.sftbmb.ru/sftbmb-core/golibinfra/testkit/txkit"
)

func defaultPolicy(t *testing.T, allow bool) *mocks.MockAuthorizer {
	t.Helper()
	p := mocks.NewMockAuthorizer(t)
	dec := func() policy.Decision {
		if allow {
			return policy.Allow()
		}
		return policy.Deny("test deny")
	}
	p.EXPECT().CanGetByID(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(context.Context, identity.Identity, string) policy.Decision { return dec() }).Maybe()
	p.EXPECT().CanList(mock.Anything, mock.Anything).
		RunAndReturn(func(context.Context, identity.Identity) policy.Decision { return dec() }).Maybe()
	p.EXPECT().CanUpdate(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(context.Context, identity.Identity, string) policy.Decision { return dec() }).Maybe()
	p.EXPECT().CanDelete(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(context.Context, identity.Identity, string) policy.Decision { return dec() }).Maybe()
	return p
}

type ucFixture struct {
	uc    *user.Usecase
	repo  *mocks.MockRepository
	bus   *eventbus.Recorder
	audit *audittest.Recorder
}

type ucOpts struct {
	repo *mocks.MockRepository
	pol  user.Authorizer
}

type ucOption func(*ucOpts)

func withRepo(r *mocks.MockRepository) ucOption { return func(o *ucOpts) { o.repo = r } }
func withPolicy(p user.Authorizer) ucOption     { return func(o *ucOpts) { o.pol = p } }

func newUC(t *testing.T, opts ...ucOption) ucFixture {
	t.Helper()
	o := &ucOpts{}
	for _, fn := range opts {
		fn(o)
	}
	if o.repo == nil {
		o.repo = mocks.NewMockRepository(t)
	}
	if o.pol == nil {
		o.pol = defaultPolicy(t, false)
	}

	bus := &eventbus.Recorder{}
	aud := &audittest.Recorder{}

	m, err := user.NewMetrics(noopmetric.NewMeterProvider().Meter("test"))
	require.NoError(t, err)

	uc := user.NewUsecase(user.UsecaseDeps{
		Repo:    o.repo,
		Hasher:  authntest.DeterministicHasher{},
		Policy:  o.pol,
		Tx:      txkit.Identity{},
		Events:  bus,
		Audit:   aud,
		Metrics: m,
		Log:     logkit.Nop(),
		Clock:   clockkit.NewFixed(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	})
	return ucFixture{uc: uc, repo: o.repo, bus: bus, audit: aud}
}

func TestRegister(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		var captured *user.User
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().Create(mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, u *user.User) error {
				captured = u
				u.ID = "id-1"
				return nil
			})
		f := newUC(t, withRepo(repo))

		got, err := f.uc.Register(context.Background(), &user.Create{
			Email: "a@b.com", Password: "longenough", Name: "Alice",
		})
		require.NoError(t, err)
		require.Equal(t, "hash:longenough", got.PasswordHash)
		require.NotNil(t, captured)
		require.True(t, f.audit.Has("register"))
	})

	t.Run("weak password", func(t *testing.T) {
		t.Parallel()
		f := newUC(t)
		_, err := f.uc.Register(context.Background(), &user.Create{
			Email: "a@b.com", Password: "short", Name: "Alice",
		})
		require.ErrorIs(t, err, user.ErrWeakPassword)
	})

	t.Run("repo conflict", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(user.ErrAlreadyExists)
		f := newUC(t, withRepo(repo))
		_, err := f.uc.Register(context.Background(), &user.Create{
			Email: "a@b.com", Password: "longenough", Name: "Alice",
		})
		require.ErrorIs(t, err, user.ErrAlreadyExists)
	})
}

func TestGetByID(t *testing.T) {
	t.Parallel()

	t.Run("forbidden", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withPolicy(defaultPolicy(t, false)))
		_, err := f.uc.GetByID(identitytest.Client("u1"), "u2")
		var fe *user.ForbiddenError
		require.ErrorAs(t, err, &fe)
	})

	t.Run("no identity", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withPolicy(defaultPolicy(t, true)))
		_, err := f.uc.GetByID(identitytest.Anon(), "u1")
		var fe *user.ForbiddenError
		require.ErrorAs(t, err, &fe)
	})

	t.Run("allowed", func(t *testing.T) {
		t.Parallel()
		want := &user.User{ID: "u1", Email: "a@b.com"}
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, mock.Anything).Return(want, nil)
		f := newUC(t, withRepo(repo), withPolicy(defaultPolicy(t, true)))

		got, err := f.uc.GetByID(identitytest.Client("u1"), "u1")
		require.NoError(t, err)
		require.Same(t, want, got)
	})
}

func TestMe(t *testing.T) {
	t.Parallel()

	t.Run("no identity", func(t *testing.T) {
		t.Parallel()
		f := newUC(t)
		_, err := f.uc.Me(identitytest.Anon())
		var fe *user.ForbiddenError
		require.ErrorAs(t, err, &fe)
	})

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		want := &user.User{ID: "u1", Email: "a@b.com"}
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, mock.Anything).Return(want, nil)
		f := newUC(t, withRepo(repo))

		got, err := f.uc.Me(identitytest.Client("u1"))
		require.NoError(t, err)
		require.Same(t, want, got)
	})
}

func TestList(t *testing.T) {
	t.Parallel()

	t.Run("forbidden", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withPolicy(defaultPolicy(t, false)))
		_, _, err := f.uc.List(identitytest.Client("u1"), user.ListFilter{}, user.ListPagination{Page: 1, Size: 20})
		var fe *user.ForbiddenError
		require.ErrorAs(t, err, &fe)
	})

	t.Run("admin lists all", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).
			Return([]*user.User{{ID: "u1"}, {ID: "u2"}}, nil)
		repo.EXPECT().Count(mock.Anything, mock.Anything).Return(2, nil)
		f := newUC(t, withRepo(repo), withPolicy(defaultPolicy(t, true)))

		got, total, err := f.uc.List(identitytest.Admin("admin"), user.ListFilter{}, user.ListPagination{Page: 1, Size: 20})
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, 2, total)
	})

	t.Run("pagination defaults", func(t *testing.T) {
		t.Parallel()
		var seen user.ListPagination
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, _ user.ListFilter, pg user.ListPagination) ([]*user.User, error) {
				seen = pg
				return nil, nil
			})
		repo.EXPECT().Count(mock.Anything, mock.Anything).Return(0, nil)
		f := newUC(t, withRepo(repo), withPolicy(defaultPolicy(t, true)))

		_, _, err := f.uc.List(identitytest.Admin("admin"), user.ListFilter{}, user.ListPagination{})
		require.NoError(t, err)
		require.Equal(t, 1, seen.Page)
		require.Equal(t, 20, seen.Size)
	})
}

func TestUpdate(t *testing.T) {
	t.Parallel()

	t.Run("happy path propagates version", func(t *testing.T) {
		t.Parallel()
		var seenVersion int
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().UpdateProfile(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, id, name string, version int) (*user.User, error) {
				seenVersion = version
				return &user.User{ID: id, Name: name, Status: user.StatusActive, Version: version + 1, UpdatedAt: time.Now()}, nil
			})
		f := newUC(t, withRepo(repo), withPolicy(defaultPolicy(t, true)))

		newName := "New"
		got, err := f.uc.Update(identitytest.Client("u1"), "u1", user.Update{Name: &newName, Version: 1})
		require.NoError(t, err)
		require.Equal(t, "New", got.Name)
		require.Equal(t, 1, seenVersion)
		require.True(t, f.audit.Has("update"))
	})

	t.Run("stale version", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().UpdateProfile(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, user.ErrConcurrentUpdate)
		f := newUC(t, withRepo(repo), withPolicy(defaultPolicy(t, true)))

		newName := "New"
		_, err := f.uc.Update(identitytest.Client("u1"), "u1", user.Update{Name: &newName, Version: 1})
		require.ErrorIs(t, err, user.ErrConcurrentUpdate)
	})

	t.Run("no fields", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withPolicy(defaultPolicy(t, true)))
		_, err := f.uc.Update(identitytest.Client("u1"), "u1", user.Update{Version: 1})
		require.ErrorIs(t, err, user.ErrNoUpdateFields)
	})

	t.Run("forbidden", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withPolicy(defaultPolicy(t, false)))
		_, err := f.uc.Update(identitytest.Client("u1"), "u2", user.Update{})
		var fe *user.ForbiddenError
		require.ErrorAs(t, err, &fe)
	})
}

func TestDelete(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		var seenStatus user.Status
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, mock.Anything).
			Return(&user.User{ID: "u1", Status: user.StatusActive, Version: 1}, nil)
		repo.EXPECT().UpdateStatus(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, id string, status user.Status, version int) (*user.User, error) {
				seenStatus = status
				return &user.User{ID: id, Status: status, Version: version + 1}, nil
			})
		f := newUC(t, withRepo(repo), withPolicy(defaultPolicy(t, true)))

		require.NoError(t, f.uc.Delete(identitytest.Client("u1"), "u1"))
		require.Equal(t, user.StatusDeleted, seenStatus)
		require.True(t, f.audit.Has("delete"))
	})

	t.Run("already deleted", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, mock.Anything).
			Return(&user.User{ID: "u1", Status: user.StatusDeleted}, nil)
		f := newUC(t, withRepo(repo), withPolicy(defaultPolicy(t, true)))

		err := f.uc.Delete(identitytest.Client("u1"), "u1")
		require.ErrorIs(t, err, user.ErrNotFound)
	})

	t.Run("forbidden", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withPolicy(defaultPolicy(t, false)))
		err := f.uc.Delete(identitytest.Client("u1"), "u2")
		var fe *user.ForbiddenError
		require.ErrorAs(t, err, &fe)
	})
}
