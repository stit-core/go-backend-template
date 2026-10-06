package auth_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth/mocks"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/identitytest"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/auditkit"
	"git.stit.tech/stit-core/golibinfra/authnkit/session"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/testkit/audittest"
	"git.stit.tech/stit-core/golibinfra/testkit/authntest"
	"git.stit.tech/stit-core/golibinfra/testkit/logtest"
	"git.stit.tech/stit-core/golibinfra/testkit/sessiontest"
)

// accessTokens выпускает access-токен "access.<id>.<role>": выпуск без побочных эффектов.
func accessTokens(t *testing.T) *mocks.MockTokens {
	t.Helper()
	tk := mocks.NewMockTokens(t)
	tk.EXPECT().IssueAccess(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, u *auth.User) (string, error) {
			return "access." + u.ID + "." + string(u.Role), nil
		}).Maybe()
	return tk
}

var fixedNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func defaultPolicy(t *testing.T, allow bool) *mocks.MockAuthorizer {
	t.Helper()
	p := mocks.NewMockAuthorizer(t)
	p.EXPECT().CanChangePassword(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(context.Context, identity.Identity, string) policy.Decision {
			if allow {
				return policy.Allow()
			}
			return policy.Deny("test deny")
		}).Maybe()
	return p
}

// loginGuard ждёт ровно одну попытку входа subject до проверки пароля и, если succeeded, — один успех.
func loginGuard(t *testing.T, subject string, succeeded bool) *mocks.MockLoginGuard {
	t.Helper()
	g := mocks.NewMockLoginGuard(t)
	g.EXPECT().Attempt(mock.Anything, subject, "").Return(nil, nil).Once()
	if succeeded {
		g.EXPECT().RecordSuccess(mock.Anything, subject, "").Return(nil).Once()
	}
	return g
}

// sessionConfig — сроки сессий тестов: токен живёт час, сессия — три.
var sessionConfig = session.Config{IdleTTL: time.Hour, AbsoluteTTL: 3 * time.Hour, CleanupInterval: time.Hour}

type ucFixture struct {
	uc       *auth.Usecase
	sessions *sessiontest.Store
	clock    *clockkit.Tickable
	audit    *audittest.Recorder
	log      *logtest.Recorder
}

type ucOpts struct {
	users    *mocks.MockUserReader
	verify   *mocks.MockPasswordVerifier
	setpw    *mocks.MockPasswordSetter
	policy   *mocks.MockAuthorizer
	guard    *mocks.MockLoginGuard
	sessions func(session.Store) session.Store
	metrics  *auth.Metrics
}

type ucOption func(*ucOpts)

func withUsers(u *mocks.MockUserReader) ucOption          { return func(o *ucOpts) { o.users = u } }
func withVerifier(v *mocks.MockPasswordVerifier) ucOption { return func(o *ucOpts) { o.verify = v } }
func withSetter(s *mocks.MockPasswordSetter) ucOption     { return func(o *ucOpts) { o.setpw = s } }
func withPolicy(p *mocks.MockAuthorizer) ucOption         { return func(o *ucOpts) { o.policy = p } }
func withGuard(g *mocks.MockLoginGuard) ucOption          { return func(o *ucOpts) { o.guard = g } }
func withMetrics(m *auth.Metrics) ucOption                { return func(o *ucOpts) { o.metrics = m } }

// withSessions оборачивает хранилище сессий фикстуры: сбой одного метода поверх фейка кита.
func withSessions(wrap func(session.Store) session.Store) ucOption {
	return func(o *ucOpts) { o.sessions = wrap }
}

// newUC — usecase на строгих моках портов и фейке хранилища сессий кита (sessiontest) с часами
// фикстуры: время сессий двигает кейс (f.clock.Advance).
func newUC(t *testing.T, opts ...ucOption) ucFixture {
	t.Helper()
	o := &ucOpts{
		users:  mocks.NewMockUserReader(t),
		verify: mocks.NewMockPasswordVerifier(t),
		setpw:  mocks.NewMockPasswordSetter(t),
		policy: defaultPolicy(t, false),
		guard:  mocks.NewMockLoginGuard(t),
	}
	for _, fn := range opts {
		fn(o)
	}
	clock := clockkit.NewTickable(fixedNow)
	fake, err := sessiontest.New(sessionConfig, clock)
	require.NoError(t, err)
	var sessions session.Store = fake
	if o.sessions != nil {
		sessions = o.sessions(fake)
	}
	aud, log := &audittest.Recorder{}, logtest.Capture()

	uc := auth.NewUsecase(auth.UsecaseDeps{
		Users:          o.users,
		PasswordVerify: o.verify,
		PasswordSet:    o.setpw,
		Hasher:         authntest.DeterministicHasher{},
		Tokens:         accessTokens(t),
		Sessions:       sessions,
		Policy:         o.policy,
		Guard:          o.guard,
		Audit:          aud,
		Log:            log,
		Clock:          clock,
		Metrics:        o.metrics,
	})
	return ucFixture{uc: uc, sessions: fake, clock: clock, audit: aud, log: log}
}

// issue открывает сессию userID в хранилище фикстуры — как вход.
func (f ucFixture) issue(t *testing.T, userID string) session.Token {
	t.Helper()
	tok, err := f.sessions.Issue(context.Background(), userID)
	require.NoError(t, err)
	return tok
}

var errStore = errors.New("authn_sessions: connection refused")

// failingSessions — хранилище сессий, у которого сбоит каждый вызов.
type failingSessions struct{ session.Store }

func (failingSessions) Issue(context.Context, string) (session.Token, error) {
	return session.Token{}, errStore
}

func (failingSessions) Rotate(context.Context, string) (session.Token, error) {
	return session.Token{}, errStore
}

func (failingSessions) Revoke(context.Context, string) error { return errStore }

// revokeFails — хранилище сессий, у которого сбоит только закрытие сессии.
type revokeFails struct{ session.Store }

func (revokeFails) Revoke(context.Context, string) error { return errStore }

// canLogin — пользователь u1, которого пускают: его отдаёт поиск по ID ровно один раз.
func canLogin(t *testing.T) *mocks.MockUserReader {
	t.Helper()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByID(mock.Anything, "u1").Return(&auth.User{ID: "u1", Email: "a@b.com", CanLogin: true, Role: identity.RoleClient}, nil).Once()
	return users
}

func TestLogin(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, mock.Anything).Return(
			&auth.User{ID: "u1", Email: "a@b.com", PasswordHash: "hash:pass1234", CanLogin: true, Role: identity.RoleClient}, nil,
		)
		f := newUC(t, withUsers(users), withGuard(loginGuard(t, "a@b.com", true)))

		out, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
		require.NoError(t, err)
		require.Equal(t, "access.u1.client", out.AccessToken)
		require.Equal(t, 1, f.sessions.Active("u1"), "the login opens a session of the user")
		next, err := f.sessions.Rotate(context.Background(), out.RefreshToken)
		require.NoError(t, err, "the refresh token is the token of that session")
		require.Equal(t, "u1", next.Subject)
		require.Equal(t, &auditkit.Entry{ActorID: "u1", Action: "login", Resource: "user", ResourceID: "u1", At: fixedNow}, f.audit.Last("login"))
	})

	// Сессия не открылась — входа нет: ни токенов, ни аудита входа.
	t.Run("session not opened", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(
			&auth.User{ID: "u1", Email: "a@b.com", PasswordHash: "hash:pass1234", CanLogin: true, Role: identity.RoleClient}, nil,
		).Once()
		f := newUC(t, withUsers(users), withGuard(loginGuard(t, "a@b.com", true)),
			withSessions(func(s session.Store) session.Store { return failingSessions{s} }))

		out, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
		require.ErrorIs(t, err, errStore)
		require.Nil(t, out)
		require.False(t, f.audit.Has("login"))
	})

	t.Run("wrong password", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, mock.Anything).Return(
			&auth.User{ID: "u1", PasswordHash: "hash:correct", CanLogin: true}, nil,
		)
		f := newUC(t, withUsers(users), withGuard(loginGuard(t, "a@b.com", false)))

		_, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "a@b.com", Password: "wrong"})
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
		require.Equal(t, "u1", f.audit.Last("login_failed").ResourceID, "the audit names the account by ID, not by address")
	})

	t.Run("blocked status", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, mock.Anything).Return(
			&auth.User{ID: "u1", PasswordHash: "hash:pass1234", CanLogin: false}, nil,
		)
		f := newUC(t, withUsers(users), withGuard(loginGuard(t, "a@b.com", false)))

		out, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
		require.Nil(t, out)
		require.True(t, f.audit.Has("login_failed"))
		require.False(t, f.audit.Has("login"))
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, mock.Anything).Return(nil, auth.ErrUserNotFound)
		f := newUC(t, withUsers(users), withGuard(loginGuard(t, "x@y.com", false)))

		_, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "x@y.com", Password: "p"})
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
		require.Empty(t, f.audit.Entries(), "an unknown address does not reach the audit")
	})

	t.Run("lookup infra error is not invalid credentials", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, mock.Anything).Return(nil, errors.New("db down"))
		f := newUC(t, withUsers(users), withGuard(loginGuard(t, "x@y.com", false)))

		_, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "x@y.com", Password: "p"})
		require.Error(t, err)
		require.NotErrorIs(t, err, auth.ErrInvalidCredentials)
		require.False(t, f.audit.Has("login_failed"))
	})

	t.Run("account locked", func(t *testing.T) {
		t.Parallel()
		g := mocks.NewMockLoginGuard(t)
		g.EXPECT().Attempt(mock.Anything, "a@b.com", "").
			Return(&auth.LockState{Until: fixedNow.Add(15 * time.Minute)}, nil).Once()
		f := newUC(t, withGuard(g))

		_, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "a@b.com", Password: "p"})
		require.ErrorIs(t, err, auth.ErrAccountLocked)
		var locked *auth.LockedError
		require.ErrorAs(t, err, &locked)
		require.Equal(t, 15*time.Minute, locked.RetryAfter(), "retry when the lock ends")
		require.Empty(t, f.audit.Entries(), "a lock before the lookup knows no account: the outcome goes to the metric")
	})

	// Fail-closed: строгий мок пользователей — до поиска учётной записи и пароля вход не доходит.
	t.Run("guard failure refuses the login", func(t *testing.T) {
		t.Parallel()
		g := mocks.NewMockLoginGuard(t)
		g.EXPECT().Attempt(mock.Anything, "a@b.com", "").Return(nil, errors.New("guard store down")).Once()
		f := newUC(t, withGuard(g))

		_, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
		require.ErrorContains(t, err, "guard store down")
		require.NotErrorIs(t, err, auth.ErrInvalidCredentials, "a guard failure is not a wrong password")
		require.False(t, f.audit.Has("login"))
	})

	t.Run("attempt the guard cannot count is invalid credentials", func(t *testing.T) {
		t.Parallel()
		g := mocks.NewMockLoginGuard(t)
		g.EXPECT().Attempt(mock.Anything, "", "").Return(nil, auth.ErrInvalidCredentials).Once()
		f := newUC(t, withGuard(g))

		_, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "", Password: "pass1234"})
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
	})

	t.Run("success not recorded keeps the login", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(
			&auth.User{ID: "u1", Email: "a@b.com", PasswordHash: "hash:pass1234", CanLogin: true, Role: identity.RoleClient}, nil,
		).Once()
		g := mocks.NewMockLoginGuard(t)
		g.EXPECT().Attempt(mock.Anything, "a@b.com", "").Return(nil, nil).Once()
		g.EXPECT().RecordSuccess(mock.Anything, "a@b.com", "").Return(errors.New("guard store down")).Once()
		f := newUC(t, withUsers(users), withGuard(g))

		out, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
		require.NoError(t, err)
		require.Equal(t, "access.u1.client", out.AccessToken)
		require.True(t, f.audit.Has("login"))
	})
}

// Обновление ротирует токен в той же сессии: прежний токен погашен, новый — следующий в цепочке.
func TestRefresh(t *testing.T) {
	t.Parallel()
	f := newUC(t, withUsers(canLogin(t)))
	first := f.issue(t, "u1")

	out, err := f.uc.Refresh(context.Background(), first.Value)
	require.NoError(t, err)
	require.Equal(t, "access.u1.client", out.AccessToken)
	require.NotEqual(t, first.Value, out.RefreshToken)
	next, err := f.sessions.Rotate(context.Background(), out.RefreshToken)
	require.NoError(t, err, "the new refresh token continues the session")
	require.Equal(t, first.SessionID, next.SessionID)
	require.Empty(t, f.audit.Entries())
}

// Повтор погашенного токена — его предъявил клиент или укравший: сессия закрыта целиком, вместе со
// следующим токеном цепочки, повтор — в аудите учётной записи.
func TestRefresh_Reused(t *testing.T) {
	t.Parallel()
	f := newUC(t, withUsers(canLogin(t)))
	first := f.issue(t, "u1")
	out, err := f.uc.Refresh(context.Background(), first.Value)
	require.NoError(t, err)

	_, err = f.uc.Refresh(context.Background(), first.Value)
	require.ErrorIs(t, err, auth.ErrRefreshReused)
	require.Equal(t, &auditkit.Entry{ActorID: "u1", Action: "refresh_reused", Resource: "session", ResourceID: first.SessionID, At: fixedNow},
		f.audit.Last("refresh_reused"))
	_, err = f.uc.Refresh(context.Background(), out.RefreshToken)
	require.ErrorIs(t, err, auth.ErrRefreshInvalid, "the reuse closed the session with its next token")
	require.Zero(t, f.sessions.Active("u1"))
}

// Срок токена и абсолютный срок сессии: ротация продлевает токен, но не сессию.
func TestRefresh_Expired(t *testing.T) {
	t.Parallel()
	t.Run("token idle past its lifetime", func(t *testing.T) {
		t.Parallel()
		f := newUC(t)
		tok := f.issue(t, "u1")
		f.clock.Advance(sessionConfig.IdleTTL)

		_, err := f.uc.Refresh(context.Background(), tok.Value)
		require.ErrorIs(t, err, auth.ErrRefreshExpired)
	})
	t.Run("session past its absolute lifetime", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByID(mock.Anything, "u1").Return(&auth.User{ID: "u1", CanLogin: true}, nil).Times(3)
		f := newUC(t, withUsers(users))
		refresh := f.issue(t, "u1").Value
		for range 3 {
			f.clock.Advance(50 * time.Minute)
			out, err := f.uc.Refresh(context.Background(), refresh)
			require.NoError(t, err)
			refresh = out.RefreshToken
		}
		f.clock.Advance(31 * time.Minute)

		_, err := f.uc.Refresh(context.Background(), refresh)
		require.ErrorIs(t, err, auth.ErrRefreshExpired, "three hours after the login the session is over")
	})
}

func TestRefresh_UnknownToken(t *testing.T) {
	t.Parallel()
	_, err := newUC(t).uc.Refresh(context.Background(), "not-a-session-token")
	require.ErrorIs(t, err, auth.ErrRefreshInvalid)
}

// Удалённый и заблокированный сессию не продлевают: токен ротации клиенту не уходит, сессия закрыта.
func TestRefresh_UserCannotLogIn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		user *auth.User
		err  error
	}{
		{name: "blocked", user: &auth.User{ID: "u1", CanLogin: false}},
		{name: "deleted", err: auth.ErrUserNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			users := mocks.NewMockUserReader(t)
			users.EXPECT().GetByID(mock.Anything, "u1").Return(tc.user, tc.err).Once()
			f := newUC(t, withUsers(users))
			tok := f.issue(t, "u1")

			out, err := f.uc.Refresh(context.Background(), tok.Value)
			require.ErrorIs(t, err, auth.ErrRefreshInvalid)
			require.Nil(t, out)
			require.Zero(t, f.sessions.Active("u1"), "the session is closed")
		})
	}
}

// Сбой закрытия сессии ответа не меняет: токен ротации клиенту не ушёл, причина — в логе.
func TestRefresh_BlockedSessionNotClosed(t *testing.T) {
	t.Parallel()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByID(mock.Anything, "u1").Return(&auth.User{ID: "u1", CanLogin: false}, nil).Once()
	f := newUC(t, withUsers(users), withSessions(func(s session.Store) session.Store { return revokeFails{s} }))
	tok := f.issue(t, "u1")

	_, err := f.uc.Refresh(context.Background(), tok.Value)
	require.ErrorIs(t, err, auth.ErrRefreshInvalid)
	e, logged := f.log.Find(slog.LevelError, "revoke session")
	require.True(t, logged)
	require.Equal(t, "u1", e.Str("user_id"))
	require.Contains(t, e.Str("err"), errStore.Error())
}

// Сбой хранилища или поиска пользователя — не «войти заново», а ошибка сервиса.
func TestRefresh_Failure(t *testing.T) {
	t.Parallel()
	t.Run("session store", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withSessions(func(s session.Store) session.Store { return failingSessions{s} }))
		_, err := f.uc.Refresh(context.Background(), "tok")
		require.ErrorIs(t, err, errStore)
	})
	t.Run("user lookup", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByID(mock.Anything, "u1").Return(nil, errors.New("db down")).Once()
		f := newUC(t, withUsers(users))
		tok := f.issue(t, "u1")

		_, err := f.uc.Refresh(context.Background(), tok.Value)
		require.ErrorContains(t, err, "db down")
		require.NotErrorIs(t, err, auth.ErrRefreshInvalid)
	})
}

// Выход закрывает сессию токена; неизвестный токен — тоже выход.
func TestLogout(t *testing.T) {
	t.Parallel()
	f := newUC(t)
	tok := f.issue(t, "u1")

	require.NoError(t, f.uc.Logout(context.Background(), tok.Value))
	require.Zero(t, f.sessions.Active("u1"))
	_, err := f.sessions.Rotate(context.Background(), tok.Value)
	require.ErrorIs(t, err, session.ErrInvalid, "the token of a closed session does not refresh")
	require.NoError(t, f.uc.Logout(context.Background(), "unknown"))
	require.Empty(t, f.audit.Entries(), "the store does not say whose session it closed")
}

func TestLogout_StoreFailure(t *testing.T) {
	t.Parallel()
	f := newUC(t, withSessions(func(s session.Store) session.Store { return failingSessions{s} }))
	require.ErrorIs(t, f.uc.Logout(context.Background(), "tok"), errStore)
}

// pwUser отдаёт учётную запись u1 для смены пароля ровно один раз.
func pwUser(t *testing.T) *mocks.MockUserReader {
	t.Helper()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByID(mock.Anything, "u1").Return(&auth.User{ID: "u1", Email: "a@b.com", Version: 7}, nil).Once()
	return users
}

func TestChangePassword(t *testing.T) {
	t.Parallel()

	t.Run("happy path propagates version", func(t *testing.T) {
		t.Parallel()
		verify := mocks.NewMockPasswordVerifier(t)
		verify.EXPECT().VerifyPassword(mock.Anything, "u1", "old").Return(true, nil).Once()
		setter := mocks.NewMockPasswordSetter(t)
		setter.EXPECT().UpdatePassword(mock.Anything, "u1", "hash:newpassword123", 7).Return(nil).Once()
		f := newUC(t, withUsers(pwUser(t)), withGuard(loginGuard(t, "a@b.com", true)), withVerifier(verify),
			withSetter(setter), withPolicy(defaultPolicy(t, true)))

		require.NoError(t, f.uc.ChangePassword(identitytest.Client("u1"), "u1", "old", "newpassword123"))
		require.True(t, f.audit.Has("change_password"))
	})

	t.Run("concurrent update", func(t *testing.T) {
		t.Parallel()
		verify := mocks.NewMockPasswordVerifier(t)
		verify.EXPECT().VerifyPassword(mock.Anything, "u1", "old").Return(true, nil).Once()
		setter := mocks.NewMockPasswordSetter(t)
		setter.EXPECT().UpdatePassword(mock.Anything, "u1", "hash:newpassword123", 7).Return(auth.ErrConcurrentUpdate).Once()
		f := newUC(t, withUsers(pwUser(t)), withGuard(loginGuard(t, "a@b.com", true)), withVerifier(verify),
			withSetter(setter), withPolicy(defaultPolicy(t, true)))

		err := f.uc.ChangePassword(identitytest.Client("u1"), "u1", "old", "newpassword123")
		require.ErrorIs(t, err, auth.ErrConcurrentUpdate)
		require.False(t, f.audit.Has("change_password"))
	})

	t.Run("unknown user", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByID(mock.Anything, "u2").Return(nil, auth.ErrUserNotFound).Once()
		f := newUC(t, withUsers(users), withPolicy(defaultPolicy(t, true)))

		err := f.uc.ChangePassword(identitytest.Admin("a1"), "u2", "old", "newpassword123")
		require.ErrorIs(t, err, auth.ErrUserNotFound)
	})

	// Неверный старый пароль — попытка в счётчике входа учётной записи, успеха guard не видит.
	t.Run("wrong old", func(t *testing.T) {
		t.Parallel()
		verify := mocks.NewMockPasswordVerifier(t)
		verify.EXPECT().VerifyPassword(mock.Anything, "u1", "old").Return(false, nil).Once()
		f := newUC(t, withUsers(pwUser(t)), withGuard(loginGuard(t, "a@b.com", false)), withVerifier(verify),
			withPolicy(defaultPolicy(t, true)))

		err := f.uc.ChangePassword(identitytest.Client("u1"), "u1", "old", "newpassword123")
		require.ErrorIs(t, err, auth.ErrWrongOldPassword)
	})

	t.Run("weak new", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withPolicy(defaultPolicy(t, true)))
		err := f.uc.ChangePassword(identitytest.Client("u1"), "u1", "old", "short")
		require.ErrorIs(t, err, auth.ErrWeakPassword)
	})

	t.Run("forbidden", func(t *testing.T) {
		t.Parallel()
		f := newUC(t)
		err := f.uc.ChangePassword(identitytest.Client("u1"), "u2", "old", "newpassword123")
		var fe *policy.ForbiddenError
		require.ErrorAs(t, err, &fe)
	})
}

// Старый пароль угадывают так же, как при входе: под блокировкой и при сбое guard'а он не проверяется —
// строгий мок проверки пароля.
func TestChangePassword_Guarded(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		lock    *auth.LockState
		err     error
		wantErr error
	}{
		{name: "locked", lock: &auth.LockState{Until: fixedNow.Add(time.Minute)}, wantErr: auth.ErrAccountLocked},
		{name: "guard failure", err: errors.New("guard store down")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := mocks.NewMockLoginGuard(t)
			g.EXPECT().Attempt(mock.Anything, "a@b.com", "").Return(tc.lock, tc.err).Once()
			f := newUC(t, withUsers(pwUser(t)), withGuard(g), withPolicy(defaultPolicy(t, true)))

			err := f.uc.ChangePassword(identitytest.Client("u1"), "u1", "old", "newpassword123")
			require.Error(t, err)
			require.NotErrorIs(t, err, auth.ErrWrongOldPassword)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

// Исходы входа и обновления сессии — в метрике: на перебор паролей и повтор refresh стоят алерты.
func TestLogin_RecordsOutcome(t *testing.T) {
	t.Parallel()
	found := func(canLogin bool) ucOption {
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(
			&auth.User{ID: "u1", Email: "a@b.com", PasswordHash: "hash:pass1234", CanLogin: canLogin}, nil,
		).Once()
		return withUsers(users)
	}
	locked := mocks.NewMockLoginGuard(t)
	locked.EXPECT().Attempt(mock.Anything, "a@b.com", "").Return(&auth.LockState{Until: fixedNow.Add(time.Minute)}, nil).Once()
	failing := mocks.NewMockLoginGuard(t)
	failing.EXPECT().Attempt(mock.Anything, "a@b.com", "").Return(nil, errors.New("guard store down")).Once()
	unkeyable := mocks.NewMockLoginGuard(t)
	unkeyable.EXPECT().Attempt(mock.Anything, "a@b.com", "").Return(nil, auth.ErrInvalidCredentials).Once()
	tests := []struct {
		name, result, password string
		opts                   []ucOption
	}{
		{"ok", "ok", "pass1234", []ucOption{found(true), withGuard(loginGuard(t, "a@b.com", true))}},
		{"wrong password", "invalid_credentials", "wrong", []ucOption{found(true), withGuard(loginGuard(t, "a@b.com", false))}},
		{"attempt the guard cannot count", "invalid_credentials", "pass1234", []ucOption{withGuard(unkeyable)}},
		{"blocked", "blocked", "pass1234", []ucOption{found(false), withGuard(loginGuard(t, "a@b.com", false))}},
		{"locked", "locked", "pass1234", []ucOption{withGuard(locked)}},
		{"guard failure", "error", "pass1234", []ucOption{withGuard(failing)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reader := sdkmetric.NewManualReader()
			m := auth.NewMetrics(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"))
			f := newUC(t, append(tc.opts, withMetrics(m))...)

			_, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "a@b.com", Password: tc.password})
			require.Equal(t, tc.result == "ok", err == nil, "login error: %v", err)
			require.Equal(t, map[string]int64{tc.result: 1}, counted(t, reader, "biz_auth_login_attempts_total"))
		})
	}
}

func TestRefresh_RecordsOutcome(t *testing.T) {
	t.Parallel()
	none := func(*testing.T) []ucOption { return nil }
	issued := func(t *testing.T, f ucFixture) string { return f.issue(t, "u1").Value }
	tests := []struct {
		name, result string
		opts         func(t *testing.T) []ucOption
		token        func(t *testing.T, f ucFixture) string
	}{
		{"rotated", "ok", func(t *testing.T) []ucOption { return []ucOption{withUsers(canLogin(t))} }, issued},
		{"reused", "reused", none, func(t *testing.T, f ucFixture) string {
			first := f.issue(t, "u1")
			_, err := f.sessions.Rotate(context.Background(), first.Value)
			require.NoError(t, err)
			return first.Value
		}},
		{"unknown token", "invalid", none, func(*testing.T, ucFixture) string { return "tok" }},
		{"blocked user", "invalid", func(t *testing.T) []ucOption {
			users := mocks.NewMockUserReader(t)
			users.EXPECT().GetByID(mock.Anything, "u1").Return(&auth.User{ID: "u1"}, nil).Once()
			return []ucOption{withUsers(users)}
		}, issued},
		{"expired", "expired", none, func(t *testing.T, f ucFixture) string {
			tok := f.issue(t, "u1")
			f.clock.Advance(sessionConfig.IdleTTL)
			return tok.Value
		}},
		{"store failure", "error", func(*testing.T) []ucOption {
			return []ucOption{withSessions(func(s session.Store) session.Store { return failingSessions{s} })}
		}, func(*testing.T, ucFixture) string { return "tok" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reader := sdkmetric.NewManualReader()
			m := auth.NewMetrics(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"))
			f := newUC(t, append(tc.opts(t), withMetrics(m))...)

			_, err := f.uc.Refresh(context.Background(), tc.token(t, f))
			require.Equal(t, tc.result == "ok", err == nil, "refresh error: %v", err)
			require.Equal(t, map[string]int64{tc.result: 1}, counted(t, reader, "biz_auth_refresh_total"))
		})
	}
}

// counted — значения счётчика name по метке result.
func counted(t *testing.T, reader *sdkmetric.ManualReader, name string) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	got := map[string]int64{}
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "%s is an int64 counter", name)
			for _, dp := range sum.DataPoints {
				result, _ := dp.Attributes.Value("result")
				got[result.AsString()] += dp.Value
			}
		}
	}
	return got
}
