package user_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	noopmetric "go.opentelemetry.io/otel/metric/noop"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/user/mocks"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/identitytest"
	"git.stit.tech/stit-core/go-backend-template/pkg/paging"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/testkit/audittest"
	"git.stit.tech/stit-core/golibinfra/testkit/authntest"
	"git.stit.tech/stit-core/golibinfra/testkit/outboxtest"
	"git.stit.tech/stit-core/golibinfra/testkit/pgtest"
)

var fixedNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

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
	p.EXPECT().CanChangeStatus(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(context.Context, identity.Identity, string) policy.Decision { return dec() }).Maybe()
	p.EXPECT().CanDelete(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(context.Context, identity.Identity, string) policy.Decision { return dec() }).Maybe()
	return p
}

type ucFixture struct {
	uc    *user.Usecase
	repo  *mocks.MockRepository
	bus   *outboxtest.Recorder
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

	bus := &outboxtest.Recorder{}
	aud := &audittest.Recorder{}

	m, err := user.NewMetrics(noopmetric.NewMeterProvider().Meter("test"))
	require.NoError(t, err)

	uc := user.NewUsecase(user.UsecaseDeps{
		Repo:    o.repo,
		Hasher:  authntest.DeterministicHasher{},
		Policy:  o.pol,
		Tx:      pgtest.Identity{},
		Events:  bus,
		Audit:   aud,
		Metrics: m,
		Log:     logkit.Nop(),
		Clock:   clockkit.NewFixed(fixedNow),
	})
	return ucFixture{uc: uc, repo: o.repo, bus: bus, audit: aud}
}

func TestRegister(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		var captured *user.User
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().Insert(mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, u *user.User) error {
				captured = u
				u.ID = "id-1"
				return nil
			})
		f := newUC(t, withRepo(repo))

		got, err := f.uc.Register(context.Background(), &user.CreateInput{
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
		_, err := f.uc.Register(context.Background(), &user.CreateInput{
			Email: "a@b.com", Password: "short", Name: "Alice",
		})
		require.ErrorIs(t, err, user.ErrWeakPassword)
	})

	t.Run("repo conflict", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().Insert(mock.Anything, mock.Anything).Return(user.ErrAlreadyExists)
		f := newUC(t, withRepo(repo))
		_, err := f.uc.Register(context.Background(), &user.CreateInput{
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
		var fe *policy.ForbiddenError
		require.ErrorAs(t, err, &fe)
	})

	t.Run("no identity", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withPolicy(defaultPolicy(t, true)))
		_, err := f.uc.GetByID(identitytest.Anon(), "u1")
		var fe *policy.ForbiddenError
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
		var fe *policy.ForbiddenError
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
		_, _, err := f.uc.List(identitytest.Client("u1"), user.ListFilter{}, paging.Page{Number: 1, Size: 20})
		var fe *policy.ForbiddenError
		require.ErrorAs(t, err, &fe)
	})

	t.Run("admin lists all", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).
			Return([]*user.User{{ID: "u1"}, {ID: "u2"}}, nil)
		repo.EXPECT().Count(mock.Anything, mock.Anything).Return(2, nil)
		f := newUC(t, withRepo(repo), withPolicy(defaultPolicy(t, true)))

		got, total, err := f.uc.List(identitytest.Admin("admin"), user.ListFilter{}, paging.Page{Number: 1, Size: 20})
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, 2, total)
	})

	t.Run("pagination defaults", func(t *testing.T) {
		t.Parallel()
		var seen paging.Page
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, _ user.ListFilter, page paging.Page) ([]*user.User, error) {
				seen = page
				return nil, nil
			})
		repo.EXPECT().Count(mock.Anything, mock.Anything).Return(0, nil)
		f := newUC(t, withRepo(repo), withPolicy(defaultPolicy(t, true)))

		_, _, err := f.uc.List(identitytest.Admin("admin"), user.ListFilter{}, paging.Page{})
		require.NoError(t, err)
		require.Equal(t, paging.Page{Number: 1, Size: paging.DefaultSize}, seen)
	})
}

// stored — пользователь u1 в БД до правки.
func stored() *user.User {
	return &user.User{ID: "u1", Email: "a@b.com", Name: "Old", Status: user.StatusActive, Version: 3}
}

func ptr[T any](v T) *T { return &v }

// Авторизация — настоящая policy: статус меняет только админ, и свой тоже.
func TestUpdate(t *testing.T) {
	t.Parallel()

	t.Run("admin applies name and status in one write", func(t *testing.T) {
		t.Parallel()
		saved := &user.User{ID: "u1", Email: "a@b.com", Name: "New", Status: user.StatusBlocked, Version: 4, UpdatedAt: fixedNow}
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, "u1").Return(stored(), nil).Once()
		repo.EXPECT().Update(mock.Anything, &user.User{ID: "u1", Email: "a@b.com", Name: "New", Status: user.StatusBlocked, Version: 3}).
			Return(saved, nil).Once()
		f := newUC(t, withRepo(repo), withPolicy(user.NewPolicy()))

		got, err := f.uc.Update(identitytest.Admin("a1"), "u1",
			user.UpdateInput{Name: ptr("New"), Status: ptr(user.StatusBlocked), Version: 3})
		require.NoError(t, err)
		require.Same(t, saved, got)
		require.Equal(t, user.UserUpdated{ID: "u1", At: fixedNow}, f.bus.Last("user.events.updated"))
		require.Equal(t, "a1", f.audit.Last("update").ActorID)
	})

	t.Run("client renames self", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, "u1").Return(stored(), nil).Once()
		repo.EXPECT().Update(mock.Anything, &user.User{ID: "u1", Email: "a@b.com", Name: "New", Status: user.StatusActive, Version: 3}).
			Return(&user.User{ID: "u1", Name: "New", Status: user.StatusActive, Version: 4}, nil).Once()
		f := newUC(t, withRepo(repo), withPolicy(user.NewPolicy()))

		got, err := f.uc.Update(identitytest.Client("u1"), "u1", user.UpdateInput{Name: ptr("New"), Version: 3})
		require.NoError(t, err)
		require.Equal(t, 4, got.Version)
	})

	t.Run("client does not change own status", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withPolicy(user.NewPolicy()))

		_, err := f.uc.Update(identitytest.Client("u1"), "u1",
			user.UpdateInput{Name: ptr("New"), Status: ptr(user.StatusActive), Version: 3})
		var fe *policy.ForbiddenError
		require.ErrorAs(t, err, &fe)
		require.Zero(t, f.bus.Count())
	})

	t.Run("client does not update another user", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withPolicy(user.NewPolicy()))

		_, err := f.uc.Update(identitytest.Client("u1"), "u2", user.UpdateInput{Name: ptr("New"), Version: 3})
		var fe *policy.ForbiddenError
		require.ErrorAs(t, err, &fe)
	})

	t.Run("no fields", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withPolicy(user.NewPolicy()))
		_, err := f.uc.Update(identitytest.Client("u1"), "u1", user.UpdateInput{Version: 3})
		require.ErrorIs(t, err, user.ErrNoUpdateFields)
	})

	t.Run("unknown or deleted user", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, "u1").Return(nil, user.ErrNotFound).Once()
		f := newUC(t, withRepo(repo), withPolicy(user.NewPolicy()))

		_, err := f.uc.Update(identitytest.Admin("a1"), "u1", user.UpdateInput{Name: ptr("New"), Version: 3})
		require.ErrorIs(t, err, user.ErrNotFound)
		require.Zero(t, f.bus.Count())
	})

	t.Run("stale version is not written", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, "u1").Return(stored(), nil).Once()
		f := newUC(t, withRepo(repo), withPolicy(user.NewPolicy()))

		_, err := f.uc.Update(identitytest.Admin("a1"), "u1", user.UpdateInput{Name: ptr("New"), Version: 2})
		require.ErrorIs(t, err, user.ErrConcurrentUpdate)
	})

	t.Run("concurrent write between read and update", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, "u1").Return(stored(), nil).Once()
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil, user.ErrConcurrentUpdate).Once()
		f := newUC(t, withRepo(repo), withPolicy(user.NewPolicy()))

		_, err := f.uc.Update(identitytest.Admin("a1"), "u1", user.UpdateInput{Name: ptr("New"), Version: 3})
		require.ErrorIs(t, err, user.ErrConcurrentUpdate)
		require.Zero(t, f.bus.Count())
	})

	t.Run("status through Delete only", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, "u1").Return(stored(), nil).Once()
		f := newUC(t, withRepo(repo), withPolicy(user.NewPolicy()))

		_, err := f.uc.Update(identitytest.Admin("a1"), "u1", user.UpdateInput{Status: ptr(user.StatusDeleted), Version: 3})
		require.ErrorIs(t, err, user.ErrInvalidStatusTransition)
	})
}

func TestDelete(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, "u1").Return(stored(), nil).Once()
		repo.EXPECT().Update(mock.Anything, &user.User{ID: "u1", Email: "a@b.com", Name: "Old", Status: user.StatusDeleted, Version: 3}).
			Return(&user.User{ID: "u1", Status: user.StatusDeleted, Version: 4}, nil).Once()
		f := newUC(t, withRepo(repo), withPolicy(defaultPolicy(t, true)))

		require.NoError(t, f.uc.Delete(identitytest.Client("u1"), "u1"))
		require.Equal(t, user.UserDeleted{ID: "u1", At: fixedNow}, f.bus.Last("user.events.deleted"))
		require.True(t, f.audit.Has("delete"))
	})

	t.Run("unknown or deleted user", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, "u1").Return(nil, user.ErrNotFound).Once()
		f := newUC(t, withRepo(repo), withPolicy(defaultPolicy(t, true)))

		err := f.uc.Delete(identitytest.Client("u1"), "u1")
		require.ErrorIs(t, err, user.ErrNotFound)
		require.Zero(t, f.bus.Count())
	})

	t.Run("forbidden", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withPolicy(defaultPolicy(t, false)))
		err := f.uc.Delete(identitytest.Client("u1"), "u2")
		var fe *policy.ForbiddenError
		require.ErrorAs(t, err, &fe)
	})
}
