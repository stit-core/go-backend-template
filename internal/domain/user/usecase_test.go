package user_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/user/mocks"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/identitytest"
	"git.stit.tech/stit-core/go-backend-template/pkg/paging"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/auditkit"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/ptrkit"
	"git.stit.tech/stit-core/golibinfra/testkit/audittest"
	"git.stit.tech/stit-core/golibinfra/testkit/authntest"
	"git.stit.tech/stit-core/golibinfra/testkit/logtest"
	"git.stit.tech/stit-core/golibinfra/testkit/outboxtest"
	"git.stit.tech/stit-core/golibinfra/testkit/pgtest"
)

var fixedNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

const (
	subjectRegistered = "user.events.registered.v2"
	subjectUpdated    = "user.events.updated"
	subjectDeleted    = "user.events.deleted"
)

type ucFixture struct {
	uc    *user.Usecase
	bus   *outboxtest.Recorder
	audit *audittest.Recorder
	log   *logtest.Recorder
}

type ucOpts struct {
	repo     *mocks.MockRepository
	sessions *mocks.MockSessions
	hasher   authnkit.Hasher
	audit    auditkit.Logger
}

type ucOption func(*ucOpts)

func withRepo(r *mocks.MockRepository) ucOption   { return func(o *ucOpts) { o.repo = r } }
func withSessions(s *mocks.MockSessions) ucOption { return func(o *ucOpts) { o.sessions = s } }
func withHasher(h authnkit.Hasher) ucOption       { return func(o *ucOpts) { o.hasher = h } }
func withAudit(a auditkit.Logger) ucOption        { return func(o *ucOpts) { o.audit = a } }

// newUC — usecase на строгих моках репозитория и сессий и настоящей policy: право решает identity
// кейса, вызов порта, которого кейс не ждёт, — провал; порт с ожиданиями кейс передаёт опцией.
func newUC(t *testing.T, opts ...ucOption) ucFixture {
	t.Helper()
	o := &ucOpts{repo: mocks.NewMockRepository(t), sessions: mocks.NewMockSessions(t), hasher: &authntest.DeterministicHasher{}}
	for _, fn := range opts {
		fn(o)
	}
	bus, aud, log := &outboxtest.Recorder{}, &audittest.Recorder{}, logtest.Capture()
	if o.audit == nil {
		o.audit = aud
	}
	uc := user.NewUsecase(user.UsecaseDeps{
		Repo:     o.repo,
		Hasher:   o.hasher,
		Policy:   user.NewPolicy(),
		Tx:       pgtest.Identity{},
		Events:   bus,
		Sessions: o.sessions,
		Audit:    o.audit,
		Log:      log,
		Clock:    clockkit.NewFixed(fixedNow),
	})
	return ucFixture{uc: uc, bus: bus, audit: aud, log: log}
}

// event — последнее событие subject в своём типе: outbox хранит JSON.
func event[T any](t *testing.T, f ucFixture, subject string) T {
	t.Helper()
	var e T
	require.NoError(t, json.Unmarshal(f.bus.Last(subject), &e))
	return e
}

// revoked ждёт ровно одно закрытие всех сессий пользователя id в транзакции записи и отвечает err.
func revoked(t *testing.T, id string, err error) *mocks.MockSessions {
	t.Helper()
	s := mocks.NewMockSessions(t)
	s.EXPECT().RevokeAll(mock.MatchedBy(pgkit.InTx), id).Return(err).Once()
	return s
}

var errRevoke = errors.New("authn_sessions: connection refused")

// audited — запись аудита запроса identitytest: actor сделал action над пользователем id; IP клиента и ID
// запроса auditkit берёт из ctx.
func audited(actor, action, id string) *auditkit.Entry {
	return &auditkit.Entry{
		ActorID: actor, Action: action, Resource: "user", ResourceID: id, At: fixedNow,
		IP: identitytest.IP, RequestID: identitytest.RequestID,
	}
}

var (
	errHashing = errors.New("argon2: out of memory")
	errAudit   = errors.New("audit_log: connection refused")
	errDB      = errors.New("users: connection refused")
)

func requireForbidden(t *testing.T, err error) {
	t.Helper()
	var fe *policy.ForbiddenError
	require.ErrorAs(t, err, &fe)
}

// stored — пользователь u1 в БД до правки.
func stored() *user.User {
	return &user.User{ID: "u1", Email: "a@b.com", Name: "Old", Status: user.StatusActive, Version: 3}
}

func TestRegister(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Insert(mock.MatchedBy(pgkit.InTx), &user.User{
		Email: "a@b.com", PasswordHash: "hash:longenough", Name: "Alice", Status: user.StatusActive, Role: identity.RoleClient, Version: 1,
	}).RunAndReturn(func(_ context.Context, u *user.User) error {
		u.ID = "id-1"
		return nil
	}).Once()
	f := newUC(t, withRepo(repo))

	got, err := f.uc.Register(identitytest.Anon(), &user.CreateInput{Email: "a@b.com", Password: "longenough", Name: "Alice"})
	require.NoError(t, err)
	require.Equal(t, "id-1", got.ID)
	require.Equal(t, user.UserRegistered{ID: "id-1", At: fixedNow}, event[user.UserRegistered](t, f, subjectRegistered),
		"the event carries the ID only: consumers read the address by ID")
	require.Equal(t, 1, f.bus.Count())
	require.Equal(t, audited("id-1", "register", "id-1"), f.audit.Last("register"))
}

// Сбой записи аудита регистрацию не отменяет: пользователь и событие записаны, причина — в логе.
func TestRegister_AuditNotWritten(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Insert(mock.MatchedBy(pgkit.InTx), mock.Anything).RunAndReturn(func(_ context.Context, u *user.User) error {
		u.ID = "id-1"
		return nil
	}).Once()
	f := newUC(t, withRepo(repo), withAudit(&audittest.Recorder{LogErr: errAudit}))

	got, err := f.uc.Register(identitytest.Anon(), &user.CreateInput{Email: "a@b.com", Password: "longenough", Name: "Alice"})
	require.NoError(t, err)
	require.Equal(t, "id-1", got.ID)
	require.Equal(t, 1, f.bus.Count())
	e, logged := f.log.Find(slog.LevelError, "audit register")
	require.True(t, logged)
	require.Contains(t, e.Str("error"), errAudit.Error())
}

// Отказ регистрации ничего не пишет: ни пользователя, ни события, ни аудита.
func TestRegister_Rejected(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		in     user.CreateInput
		hasher authnkit.Hasher // nil — исправный
		insert error           // nil — до репозитория не доходит
		want   error
	}{
		{name: "weak password", in: user.CreateInput{Email: "a@b.com", Password: "short", Name: "Alice"}, want: user.ErrWeakPassword},
		{
			// Предел хэшера — в байтах: 1024 символа не из ASCII схема пропускает, это ошибка ввода, а не сбой.
			name: "password over the hasher limit",
			in:   user.CreateInput{Email: "a@b.com", Password: strings.Repeat("я", authnkit.MaxPasswordLen), Name: "Alice"},
			want: user.ErrPasswordTooLong,
		},
		{
			name: "password not hashed", in: user.CreateInput{Email: "a@b.com", Password: "longenough", Name: "Alice"},
			hasher: &authntest.DeterministicHasher{HashErr: errHashing}, want: errHashing,
		},
		{name: "invalid email", in: user.CreateInput{Email: "a.b.com", Password: "longenough", Name: "Alice"}, want: user.ErrInvalidEmail},
		{name: "name with NUL", in: user.CreateInput{Email: "a@b.com", Password: "longenough", Name: "A\x00B"}, want: user.ErrInvalidName},
		{
			name: "email taken", in: user.CreateInput{Email: "a@b.com", Password: "longenough", Name: "Alice"},
			insert: user.ErrAlreadyExists, want: user.ErrAlreadyExists,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := mocks.NewMockRepository(t)
			if tc.insert != nil {
				repo.EXPECT().Insert(mock.Anything, mock.Anything).Return(tc.insert).Once()
			}
			opts := []ucOption{withRepo(repo)}
			if tc.hasher != nil {
				opts = append(opts, withHasher(tc.hasher))
			}
			f := newUC(t, opts...)

			_, err := f.uc.Register(identitytest.Anon(), &tc.in)
			require.ErrorIs(t, err, tc.want)
			require.Zero(t, f.bus.Count())
			require.Empty(t, f.audit.Entries())
		})
	}
}

// Читать пользователя вправе он сам и админ; policy спрашивают о цели запроса, а не о вызывающем.
func TestGetByID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		ctx     context.Context
		allowed bool
	}{
		{"self", identitytest.Client("u1"), true},
		{"admin", identitytest.Admin("a1"), true},
		{"another client", identitytest.Client("u2"), false},
		{"no identity", identitytest.Anon(), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := stored()
			repo := mocks.NewMockRepository(t)
			if tc.allowed {
				repo.EXPECT().GetByID(mock.Anything, "u1").Return(want, nil).Once()
			}

			got, err := newUC(t, withRepo(repo)).uc.GetByID(tc.ctx, "u1")
			if !tc.allowed {
				requireForbidden(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

func TestMe(t *testing.T) {
	t.Parallel()
	want := stored()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetByID(mock.Anything, "u1").Return(want, nil).Once()

	got, err := newUC(t, withRepo(repo)).uc.Me(identitytest.Client("u1"))
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestMe_NoIdentity(t *testing.T) {
	t.Parallel()
	_, err := newUC(t).uc.Me(identitytest.Anon())
	requireForbidden(t, err)
}

func TestList(t *testing.T) {
	t.Parallel()
	blocked := user.StatusBlocked
	tests := []struct {
		name      string
		filter    user.ListFilter
		page      paging.Page
		wantQuery paging.Page
	}{
		{"filter and page as asked", user.ListFilter{Status: &blocked}, paging.Page{Number: 2, Size: 5}, paging.Page{Number: 2, Size: 5}},
		{"page out of bounds is normalized", user.ListFilter{}, paging.Page{Size: paging.MaxSize + 1}, paging.Page{Number: 1, Size: paging.MaxSize}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := []*user.User{{ID: "u1"}, {ID: "u2"}}
			repo := mocks.NewMockRepository(t)
			repo.EXPECT().Count(mock.Anything, tc.filter).Return(7, nil).Once()
			repo.EXPECT().List(mock.Anything, tc.filter, tc.wantQuery).Return(want, nil).Once()

			got, total, err := newUC(t, withRepo(repo)).uc.List(identitytest.Admin("a1"), tc.filter, tc.page)
			require.NoError(t, err)
			require.Equal(t, want, got)
			require.Equal(t, 7, total)
		})
	}
}

// Список — только админу: отказ до репозитория.
func TestList_Forbidden(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ctx  context.Context
	}{
		{"client", identitytest.Client("u1")},
		{"no identity", identitytest.Anon()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := newUC(t).uc.List(tc.ctx, user.ListFilter{}, paging.Page{Number: 1, Size: 20})
			requireForbidden(t, err)
		})
	}
}

func TestList_RepositoryError(t *testing.T) {
	t.Parallel()
	down := errors.New("db down")
	tests := []struct {
		name   string
		expect func(repo *mocks.MockRepository)
	}{
		{"count", func(repo *mocks.MockRepository) {
			repo.EXPECT().Count(mock.Anything, user.ListFilter{}).Return(0, down).Once()
		}},
		{"list", func(repo *mocks.MockRepository) {
			repo.EXPECT().Count(mock.Anything, user.ListFilter{}).Return(1, nil).Once()
			repo.EXPECT().List(mock.Anything, user.ListFilter{}, paging.Page{Number: 1, Size: 20}).Return(nil, down).Once()
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := mocks.NewMockRepository(t)
			tc.expect(repo)

			_, _, err := newUC(t, withRepo(repo)).uc.List(identitytest.Admin("a1"), user.ListFilter{}, paging.Page{Number: 1, Size: 20})
			require.ErrorIs(t, err, down)
		})
	}
}

// Правка — чтение и запись одной транзакцией, событие — в ней же; статус меняет только админ, и свой тоже.
// Блокировка той же транзакцией закрывает все сессии пользователя: заблокированный не обновит токены.
func TestUpdate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		ctx    context.Context
		before *user.User // пользователь в БД до правки
		in     user.UpdateInput
		write  *user.User // что уходит в репозиторий
		actor  string
		revoke bool // закрываются все сессии
	}{
		{
			name: "admin applies name and status in one write", ctx: identitytest.Admin("a1"), before: stored(),
			in:    user.UpdateInput{Name: ptrkit.Of("New"), Status: ptrkit.Of(user.StatusBlocked), Version: 3},
			write: &user.User{ID: "u1", Email: "a@b.com", Name: "New", Status: user.StatusBlocked, Version: 3}, actor: "a1", revoke: true,
		},
		{
			name: "client renames self", ctx: identitytest.Client("u1"), before: stored(),
			in:    user.UpdateInput{Name: ptrkit.Of("New"), Version: 3},
			write: &user.User{ID: "u1", Email: "a@b.com", Name: "New", Status: user.StatusActive, Version: 3}, actor: "u1",
		},
		{
			name: "admin unblocks", ctx: identitytest.Admin("a1"),
			before: &user.User{ID: "u1", Email: "a@b.com", Name: "Old", Status: user.StatusBlocked, Version: 3},
			in:     user.UpdateInput{Status: ptrkit.Of(user.StatusActive), Version: 3},
			write:  &user.User{ID: "u1", Email: "a@b.com", Name: "Old", Status: user.StatusActive, Version: 3}, actor: "a1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			saved := &user.User{ID: "u1", Name: tc.write.Name, Status: tc.write.Status, Version: 4}
			repo := mocks.NewMockRepository(t)
			read := repo.EXPECT().GetByID(mock.MatchedBy(pgkit.InTx), "u1").Return(tc.before, nil).Once()
			write := repo.EXPECT().Update(mock.MatchedBy(pgkit.InTx), tc.write).Return(saved, nil).Once().NotBefore(read)
			sessions := mocks.NewMockSessions(t)
			if tc.revoke {
				sessions.EXPECT().RevokeAll(mock.MatchedBy(pgkit.InTx), "u1").Return(nil).Once().NotBefore(write)
			}
			f := newUC(t, withRepo(repo), withSessions(sessions))

			got, err := f.uc.Update(tc.ctx, "u1", tc.in)
			require.NoError(t, err)
			require.Equal(t, saved, got)
			require.Equal(t, user.UserUpdated{ID: "u1", At: fixedNow}, event[user.UserUpdated](t, f, subjectUpdated))
			require.Equal(t, 1, f.bus.Count())
			require.Equal(t, audited(tc.actor, "update", "u1"), f.audit.Last("update"))
		})
	}
}

// Отказ правки ничего не пишет: ни пользователя, ни события, ни аудита.
func TestUpdate_Rejected(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		ctx    context.Context
		in     user.UpdateInput
		expect func(repo *mocks.MockRepository) // nil — до репозитория не доходит
		revoke error                            // ответ закрытия сессий; nil — до него не доходит
		want   error                            // nil — отказ policy
	}{
		{
			name: "client does not change own status", ctx: identitytest.Client("u1"),
			in: user.UpdateInput{Name: ptrkit.Of("New"), Status: ptrkit.Of(user.StatusActive), Version: 3},
		},
		{name: "client does not update another user", ctx: identitytest.Client("u2"), in: user.UpdateInput{Name: ptrkit.Of("New"), Version: 3}},
		{name: "no identity", ctx: identitytest.Anon(), in: user.UpdateInput{Name: ptrkit.Of("New"), Version: 3}},
		{name: "no fields", ctx: identitytest.Client("u1"), in: user.UpdateInput{Version: 3}, want: user.ErrNoUpdateFields},
		{
			name: "unknown or deleted user", ctx: identitytest.Admin("a1"), in: user.UpdateInput{Name: ptrkit.Of("New"), Version: 3},
			expect: func(repo *mocks.MockRepository) {
				repo.EXPECT().GetByID(mock.Anything, "u1").Return(nil, user.ErrNotFound).Once()
			},
			want: user.ErrNotFound,
		},
		{
			name: "stale version", ctx: identitytest.Admin("a1"), in: user.UpdateInput{Name: ptrkit.Of("New"), Version: 2},
			expect: func(repo *mocks.MockRepository) {
				repo.EXPECT().GetByID(mock.Anything, "u1").Return(stored(), nil).Once()
			},
			want: user.ErrConcurrentUpdate,
		},
		{
			name: "concurrent write between read and update", ctx: identitytest.Admin("a1"), in: user.UpdateInput{Name: ptrkit.Of("New"), Version: 3},
			expect: func(repo *mocks.MockRepository) {
				repo.EXPECT().GetByID(mock.Anything, "u1").Return(stored(), nil).Once()
				repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil, user.ErrConcurrentUpdate).Once()
			},
			want: user.ErrConcurrentUpdate,
		},
		{
			name: "name with NUL", ctx: identitytest.Admin("a1"), in: user.UpdateInput{Name: ptrkit.Of("A\x00B"), Version: 3},
			expect: func(repo *mocks.MockRepository) {
				repo.EXPECT().GetByID(mock.Anything, "u1").Return(stored(), nil).Once()
			},
			want: user.ErrInvalidName,
		},
		{
			name: "status through Delete only", ctx: identitytest.Admin("a1"), in: user.UpdateInput{Status: ptrkit.Of(user.StatusDeleted), Version: 3},
			expect: func(repo *mocks.MockRepository) {
				repo.EXPECT().GetByID(mock.Anything, "u1").Return(stored(), nil).Once()
			},
			want: user.ErrInvalidStatusTransition,
		},
		{
			name: "sessions of the blocked not closed", ctx: identitytest.Admin("a1"), in: user.UpdateInput{Status: ptrkit.Of(user.StatusBlocked), Version: 3},
			expect: func(repo *mocks.MockRepository) {
				repo.EXPECT().GetByID(mock.Anything, "u1").Return(stored(), nil).Once()
				repo.EXPECT().Update(mock.Anything, mock.Anything).Return(&user.User{ID: "u1", Status: user.StatusBlocked, Version: 4}, nil).Once()
			},
			revoke: errRevoke, want: errRevoke,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := mocks.NewMockRepository(t)
			if tc.expect != nil {
				tc.expect(repo)
			}
			sessions := mocks.NewMockSessions(t)
			if tc.revoke != nil {
				sessions = revoked(t, "u1", tc.revoke)
			}
			f := newUC(t, withRepo(repo), withSessions(sessions))

			_, err := f.uc.Update(tc.ctx, "u1", tc.in)
			if tc.want == nil {
				requireForbidden(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
			require.Zero(t, f.bus.Count())
			require.Empty(t, f.audit.Entries())
		})
	}
}

// Удалить пользователя вправе он сам и админ; удаление, закрытие всех его сессий и событие — одной
// транзакцией.
func TestDelete(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		ctx   context.Context
		actor string // "" — отказ policy
	}{
		{"self", identitytest.Client("u1"), "u1"},
		{"admin", identitytest.Admin("a1"), "a1"},
		{"another client", identitytest.Client("u2"), ""},
		{"no identity", identitytest.Anon(), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := mocks.NewMockRepository(t)
			sessions := mocks.NewMockSessions(t)
			if tc.actor != "" {
				del := repo.EXPECT().Delete(mock.MatchedBy(pgkit.InTx), "u1").Return(nil).Once()
				sessions.EXPECT().RevokeAll(mock.MatchedBy(pgkit.InTx), "u1").Return(nil).Once().NotBefore(del)
			}
			f := newUC(t, withRepo(repo), withSessions(sessions))

			err := f.uc.Delete(tc.ctx, "u1")
			if tc.actor == "" {
				requireForbidden(t, err)
				require.Zero(t, f.bus.Count())
				return
			}
			require.NoError(t, err)
			require.Equal(t, user.UserDeleted{ID: "u1", At: fixedNow}, event[user.UserDeleted](t, f, subjectDeleted))
			require.Equal(t, 1, f.bus.Count())
			require.Equal(t, audited(tc.actor, "delete", "u1"), f.audit.Last("delete"))
		})
	}
}

// Удалённого нет: повтор и гонка двух удалений — 404, а не конфликт; события и аудита нет.
func TestDelete_UnknownOrAlreadyDeleted(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Delete(mock.Anything, "u1").Return(user.ErrNotFound).Once()
	f := newUC(t, withRepo(repo))

	require.ErrorIs(t, f.uc.Delete(identitytest.Client("u1"), "u1"), user.ErrNotFound)
	require.Zero(t, f.bus.Count())
	require.Empty(t, f.audit.Entries())
}

// Сессии удалённого не закрылись — удаления нет: транзакция откатывается, ни события, ни аудита.
func TestDelete_SessionsNotClosed(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Delete(mock.Anything, "u1").Return(nil).Once()
	f := newUC(t, withRepo(repo), withSessions(revoked(t, "u1", errRevoke)))

	require.ErrorIs(t, f.uc.Delete(identitytest.Client("u1"), "u1"), errRevoke)
	require.Zero(t, f.bus.Count())
	require.Empty(t, f.audit.Entries())
}

// seedInput — вход seed-admin с паролем по правилу продукта.
var seedInput = user.CreateInput{Email: "admin@b.com", Password: "longenough", Name: "Admin"}

// seededEntry — запись аудита seed-admin над учётной записью id: оператора в ней нет, его решение — доступ к БД.
func seededEntry(id string) *auditkit.Entry {
	return &auditkit.Entry{Action: "seed_admin", Resource: "user", ResourceID: id, At: fixedNow}
}

// Нет учётной записи с адресом — seed заводит admin с подтверждённым адресом; есть — повышает её при её версии
// (новый пароль, версия растёт). Прежние сессии закрываются той же транзакцией, запись — в аудите.
func TestEnsureAdmin(t *testing.T) {
	t.Parallel()
	t.Run("new account", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByEmailForUpdate(mock.MatchedBy(pgkit.InTx), "admin@b.com").Return(nil, user.ErrNotFound).Once()
		repo.EXPECT().Insert(mock.MatchedBy(pgkit.InTx), &user.User{
			Email: "admin@b.com", PasswordHash: "hash:longenough", Name: "Admin", Status: user.StatusActive,
			Role: identity.RoleAdmin, Version: 1, EmailVerified: true,
		}).RunAndReturn(func(_ context.Context, u *user.User) error { u.ID = "a1"; return nil }).Once()
		f := newUC(t, withRepo(repo), withSessions(revoked(t, "a1", nil)))

		got, err := f.uc.EnsureAdmin(context.Background(), seedInput)
		require.NoError(t, err)
		require.Equal(t, "a1", got.ID)
		require.Equal(t, seededEntry("a1"), f.audit.Last("seed_admin"))
		require.Zero(t, f.bus.Count(), "an operator input publishes no events")
	})
	t.Run("existing account", func(t *testing.T) {
		t.Parallel()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().GetByEmailForUpdate(mock.MatchedBy(pgkit.InTx), "admin@b.com").Return(stored(), nil).Once()
		promoted := &user.User{ID: "u1", Email: "a@b.com", Role: identity.RoleAdmin, Version: 4}
		repo.EXPECT().Promote(mock.MatchedBy(pgkit.InTx), "u1", "hash:longenough", 3).Return(promoted, nil).Once()
		f := newUC(t, withRepo(repo), withSessions(revoked(t, "u1", nil)))

		got, err := f.uc.EnsureAdmin(context.Background(), seedInput)
		require.NoError(t, err)
		require.Same(t, promoted, got)
		require.Equal(t, seededEntry("u1"), f.audit.Last("seed_admin"))
	})
}

// Отказ seed-admin ничего не оставляет: правило пароля и сущности — до транзакции, сбой в ней — откат, аудита нет.
func TestEnsureAdmin_Rejected(t *testing.T) {
	t.Parallel()
	lookup := func(found *user.User, err error) func(*mocks.MockRepository) {
		return func(repo *mocks.MockRepository) {
			repo.EXPECT().GetByEmailForUpdate(mock.Anything, "admin@b.com").Return(found, err).Once()
		}
	}
	tests := []struct {
		name     string
		in       user.CreateInput
		hasher   authnkit.Hasher // nil — исправный
		repo     func(*mocks.MockRepository)
		revokeOn string // "" — до закрытия сессий не доходит
		want     error
	}{
		{name: "weak password", in: user.CreateInput{Email: "admin@b.com", Password: "1", Name: "Admin"}, want: user.ErrWeakPassword},
		{name: "password over the hasher limit", in: user.CreateInput{
			Email: "admin@b.com", Password: strings.Repeat("я", authnkit.MaxPasswordLen), Name: "Admin",
		}, want: user.ErrPasswordTooLong},
		{name: "password not hashed", in: seedInput, hasher: &authntest.DeterministicHasher{HashErr: errHashing}, want: errHashing},
		{name: "not an address", in: user.CreateInput{Email: "not an address", Password: "longenough", Name: "Admin"}, want: user.ErrInvalidEmail},
		{name: "blank name", in: user.CreateInput{Email: "admin@b.com", Password: "longenough", Name: " "}, want: user.ErrInvalidName},
		{name: "lookup fails", in: seedInput, repo: lookup(nil, errDB), want: errDB},
		{name: "address taken concurrently", in: seedInput, repo: func(repo *mocks.MockRepository) {
			lookup(nil, user.ErrNotFound)(repo)
			repo.EXPECT().Insert(mock.Anything, mock.Anything).Return(user.ErrAlreadyExists).Once()
		}, want: user.ErrAlreadyExists},
		{name: "account changed concurrently", in: seedInput, repo: func(repo *mocks.MockRepository) {
			lookup(stored(), nil)(repo)
			repo.EXPECT().Promote(mock.Anything, "u1", "hash:longenough", 3).Return(nil, user.ErrConcurrentUpdate).Once()
		}, want: user.ErrConcurrentUpdate},
		{name: "sessions not closed", in: seedInput, revokeOn: "u1", repo: func(repo *mocks.MockRepository) {
			lookup(stored(), nil)(repo)
			repo.EXPECT().Promote(mock.Anything, "u1", "hash:longenough", 3).Return(stored(), nil).Once()
		}, want: errRevoke},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := mocks.NewMockRepository(t)
			if tc.repo != nil {
				tc.repo(repo)
			}
			opts := []ucOption{withRepo(repo)}
			if tc.hasher != nil {
				opts = append(opts, withHasher(tc.hasher))
			}
			if tc.revokeOn != "" {
				opts = append(opts, withSessions(revoked(t, tc.revokeOn, errRevoke)))
			}
			f := newUC(t, opts...)

			_, err := f.uc.EnsureAdmin(context.Background(), tc.in)
			require.ErrorIs(t, err, tc.want)
			require.Empty(t, f.audit.Entries())
		})
	}
}
