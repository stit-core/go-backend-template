package auth_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/testkit/audittest"
	"git.stit.tech/stit-core/golibinfra/testkit/authntest"
	"git.stit.tech/stit-core/golibinfra/testkit/pgtest"
)

// memSessions — stateful in-memory SessionRepository для проверки сайд-эффектов.
type memSessions struct {
	mu       sync.Mutex
	sessions map[string]*auth.RefreshSession
}

func newMemSessions() *memSessions {
	return &memSessions{sessions: map[string]*auth.RefreshSession{}}
}

func (s *memSessions) Insert(_ context.Context, jti, userID string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[jti] = &auth.RefreshSession{JTI: jti, UserID: userID, IssuedAt: fixedNow, ExpiresAt: expiresAt}
	return nil
}

func (s *memSessions) GetByJTI(_ context.Context, jti string) (*auth.RefreshSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	got, ok := s.sessions[jti]
	if !ok {
		return nil, auth.ErrSessionNotFound
	}
	clone := *got
	return &clone, nil
}

func (s *memSessions) MarkUsed(_ context.Context, jti, replacedByJTI string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	got, ok := s.sessions[jti]
	if !ok {
		return auth.ErrSessionNotFound
	}
	now := fixedNow
	got.UsedAt = &now
	got.ReplacedByJTI = replacedByJTI
	return nil
}

func (s *memSessions) get(jti string) *auth.RefreshSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.sessions[jti]; ok {
		clone := *v
		return &clone
	}
	return nil
}

// defaultTokens выпускает детерминированные access/refresh токены со счётчиком на userID,
// чтобы refresh-rotation давал свежие JTI.
func defaultTokens(t *testing.T) *mocks.MockTokens {
	t.Helper()
	tk := mocks.NewMockTokens(t)
	tk.EXPECT().IssueAccess(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, u *auth.User) (string, error) {
			return "access." + u.ID + "." + string(u.Role), nil
		}).Maybe()
	var (
		mu    sync.Mutex
		count = map[string]int{}
	)
	tk.EXPECT().IssueRefresh(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, userID string) (string, string, time.Time, error) {
			mu.Lock()
			count[userID]++
			n := count[userID]
			mu.Unlock()
			jti := "jti-" + userID
			if n > 1 {
				jti = fmt.Sprintf("jti-%s-%d", userID, n)
			}
			return "refresh." + userID + "." + jti, jti, fixedNow.Add(time.Hour), nil
		}).Maybe()
	return tk
}

func defaultParser(t *testing.T) *mocks.MockTokenParser {
	t.Helper()
	p := mocks.NewMockTokenParser(t)
	p.EXPECT().ParseRefresh(mock.Anything, mock.Anything).Return("u1", "jti-u1", nil).Maybe()
	return p
}

func parserReturning(t *testing.T, userID, jti string, err error) *mocks.MockTokenParser {
	t.Helper()
	p := mocks.NewMockTokenParser(t)
	p.EXPECT().ParseRefresh(mock.Anything, mock.Anything).Return(userID, jti, err).Maybe()
	return p
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

type ucFixture struct {
	uc       *auth.Usecase
	users    *mocks.MockUserReader
	verify   *mocks.MockPasswordVerifier
	setpw    *mocks.MockPasswordSetter
	tokens   *mocks.MockTokens
	parser   *mocks.MockTokenParser
	sessions *memSessions
	guard    *mocks.MockLoginGuard
	audit    *audittest.Recorder
}

type ucOpts struct {
	users    *mocks.MockUserReader
	verify   *mocks.MockPasswordVerifier
	setpw    *mocks.MockPasswordSetter
	tokens   *mocks.MockTokens
	parser   *mocks.MockTokenParser
	sessions *memSessions
	policy   *mocks.MockAuthorizer
	guard    *mocks.MockLoginGuard
	metrics  *auth.Metrics
}

type ucOption func(*ucOpts)

func withUsers(u *mocks.MockUserReader) ucOption          { return func(o *ucOpts) { o.users = u } }
func withVerifier(v *mocks.MockPasswordVerifier) ucOption { return func(o *ucOpts) { o.verify = v } }
func withSetter(s *mocks.MockPasswordSetter) ucOption     { return func(o *ucOpts) { o.setpw = s } }
func withParser(p *mocks.MockTokenParser) ucOption        { return func(o *ucOpts) { o.parser = p } }

func withSessions(s *memSessions) ucOption { return func(o *ucOpts) { o.sessions = s } }

func withPolicy(p *mocks.MockAuthorizer) ucOption { return func(o *ucOpts) { o.policy = p } }
func withGuard(g *mocks.MockLoginGuard) ucOption  { return func(o *ucOpts) { o.guard = g } }
func withMetrics(m *auth.Metrics) ucOption        { return func(o *ucOpts) { o.metrics = m } }

func newUC(t *testing.T, opts ...ucOption) ucFixture {
	t.Helper()
	o := &ucOpts{}
	for _, fn := range opts {
		fn(o)
	}
	if o.users == nil {
		o.users = mocks.NewMockUserReader(t)
	}
	if o.verify == nil {
		o.verify = mocks.NewMockPasswordVerifier(t)
	}
	if o.setpw == nil {
		o.setpw = mocks.NewMockPasswordSetter(t)
	}
	if o.tokens == nil {
		o.tokens = defaultTokens(t)
	}
	if o.parser == nil {
		o.parser = defaultParser(t)
	}
	if o.sessions == nil {
		o.sessions = newMemSessions()
	}
	if o.policy == nil {
		o.policy = defaultPolicy(t, false)
	}
	if o.guard == nil {
		o.guard = mocks.NewMockLoginGuard(t)
	}
	aud := &audittest.Recorder{}

	uc := auth.NewUsecase(auth.UsecaseDeps{
		Users:          o.users,
		PasswordVerify: o.verify,
		PasswordSet:    o.setpw,
		Hasher:         authntest.DeterministicHasher{},
		Tokens:         o.tokens,
		TokenParser:    o.parser,
		Sessions:       o.sessions,
		Tx:             pgtest.Identity{},
		Policy:         o.policy,
		Guard:          o.guard,
		Audit:          aud,
		Log:            logkit.Nop(),
		Clock:          clockkit.NewFixed(fixedNow),
		Metrics:        o.metrics,
	})
	return ucFixture{
		uc: uc, users: o.users, verify: o.verify, setpw: o.setpw,
		tokens: o.tokens, parser: o.parser, sessions: o.sessions, guard: o.guard, audit: aud,
	}
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
		require.NotNil(t, f.sessions.get("jti-u1"))
		require.True(t, f.audit.Has("login"))
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

func TestRefresh(t *testing.T) {
	t.Parallel()

	t.Run("happy path rotates jti", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByID(mock.Anything, mock.Anything).
			Return(&auth.User{ID: "u1", Email: "a@b.com", Role: identity.RoleClient}, nil)
		sessions := newMemSessions()
		require.NoError(t, sessions.Insert(context.Background(), "jti-old", "u1", fixedNow.Add(time.Hour)))
		f := newUC(
			t,
			withUsers(users),
			withSessions(sessions),
			withParser(parserReturning(t, "u1", "jti-old", nil)),
		)

		out, err := f.uc.Refresh(context.Background(), "tok")
		require.NoError(t, err)
		require.NotEmpty(t, out.AccessToken)
		require.NotEmpty(t, out.RefreshToken)

		old := sessions.get("jti-old")
		require.NotNil(t, old)
		require.NotNil(t, old.UsedAt)
		require.NotEmpty(t, old.ReplacedByJTI)
		require.NotEqual(t, "jti-old", old.ReplacedByJTI)
		require.NotNil(t, sessions.get(old.ReplacedByJTI))
	})

	t.Run("access token rejected", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withParser(parserReturning(t, "", "", errors.New("not a refresh token"))))
		_, err := f.uc.Refresh(context.Background(), "tok")
		require.ErrorIs(t, err, auth.ErrRefreshInvalid)
	})

	t.Run("expired at token level", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withParser(parserReturning(t, "", "", auth.ErrTokenExpired)))
		_, err := f.uc.Refresh(context.Background(), "tok")
		require.ErrorIs(t, err, auth.ErrRefreshExpired)
	})

	t.Run("expired at session level", func(t *testing.T) {
		t.Parallel()
		sessions := newMemSessions()
		require.NoError(t, sessions.Insert(context.Background(), "jti-u1", "u1", fixedNow.Add(-time.Minute)))
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByID(mock.Anything, mock.Anything).Return(&auth.User{ID: "u1"}, nil).Maybe()
		f := newUC(t, withUsers(users), withSessions(sessions))

		_, err := f.uc.Refresh(context.Background(), "tok")
		require.ErrorIs(t, err, auth.ErrRefreshExpired)
	})

	t.Run("reused jti", func(t *testing.T) {
		t.Parallel()
		sessions := newMemSessions()
		require.NoError(t, sessions.Insert(context.Background(), "jti-u1", "u1", fixedNow.Add(time.Hour)))
		require.NoError(t, sessions.MarkUsed(context.Background(), "jti-u1", "jti-other"))
		f := newUC(t, withSessions(sessions))

		_, err := f.uc.Refresh(context.Background(), "tok")
		require.ErrorIs(t, err, auth.ErrRefreshReused)
		require.True(t, f.audit.Has("refresh_reused"))
	})

	t.Run("session not found", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withParser(parserReturning(t, "u1", "missing", nil)))
		_, err := f.uc.Refresh(context.Background(), "tok")
		require.ErrorIs(t, err, auth.ErrRefreshInvalid)
	})
}

func TestLogout(t *testing.T) {
	t.Parallel()

	t.Run("marks session used", func(t *testing.T) {
		t.Parallel()
		sessions := newMemSessions()
		require.NoError(t, sessions.Insert(context.Background(), "jti-u1", "u1", fixedNow.Add(time.Hour)))
		f := newUC(t, withSessions(sessions))

		require.NoError(t, f.uc.Logout(context.Background(), "tok"))
		got := sessions.get("jti-u1")
		require.NotNil(t, got)
		require.NotNil(t, got.UsedAt)
		require.True(t, f.audit.Has("logout"))
	})

	t.Run("unknown session is no-op", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withParser(parserReturning(t, "u1", "missing", nil)))
		require.NoError(t, f.uc.Logout(context.Background(), "tok"))
	})
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
	tests := []struct {
		result, password string
		opts             []ucOption
	}{
		{"ok", "pass1234", []ucOption{found(true), withGuard(loginGuard(t, "a@b.com", true))}},
		{"invalid_credentials", "wrong", []ucOption{found(true), withGuard(loginGuard(t, "a@b.com", false))}},
		{"blocked", "pass1234", []ucOption{found(false), withGuard(loginGuard(t, "a@b.com", false))}},
		{"locked", "pass1234", []ucOption{withGuard(locked)}},
		{"error", "pass1234", []ucOption{withGuard(failing)}},
	}
	for _, tc := range tests {
		t.Run(tc.result, func(t *testing.T) {
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
	reused := newMemSessions()
	require.NoError(t, reused.Insert(context.Background(), "jti-u1", "u1", fixedNow.Add(time.Hour)))
	require.NoError(t, reused.MarkUsed(context.Background(), "jti-u1", "jti-next"))
	tests := []struct {
		result string
		opts   []ucOption
	}{
		{"reused", []ucOption{withSessions(reused)}},
		{"invalid", []ucOption{withParser(parserReturning(t, "", "", errors.New("not a refresh token")))}},
		{"expired", []ucOption{withParser(parserReturning(t, "", "", auth.ErrTokenExpired))}},
	}
	for _, tc := range tests {
		t.Run(tc.result, func(t *testing.T) {
			t.Parallel()
			reader := sdkmetric.NewManualReader()
			m := auth.NewMetrics(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"))
			f := newUC(t, append(tc.opts, withMetrics(m))...)

			_, err := f.uc.Refresh(context.Background(), "tok")
			require.Error(t, err)
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
