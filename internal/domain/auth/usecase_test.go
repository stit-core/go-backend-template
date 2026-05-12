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

	"git.stit.tech/stit/go-backend-template/internal/domain/auth"
	"git.stit.tech/stit/go-backend-template/internal/domain/auth/mocks"
	"git.stit.tech/stit/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit/go-backend-template/pkg/policy"
	"git.stit.tech/stit/golibinfra/clockkit"
	"git.stit.tech/stit/golibinfra/logkit"
	"git.stit.tech/stit/golibinfra/testkit/audittest"
	"git.stit.tech/stit/golibinfra/testkit/authntest"
	"git.stit.tech/stit/golibinfra/testkit/txkit"
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
	s.sessions[jti] = &auth.RefreshSession{JTI: jti, UserID: userID, IssuedAt: time.Now(), ExpiresAt: expiresAt}
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
	now := time.Now()
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
	tk.EXPECT().IssueAccess(mock.Anything).
		RunAndReturn(func(u *auth.User) (string, error) {
			return "access." + u.ID + "." + string(u.Role), nil
		}).Maybe()
	var (
		mu    sync.Mutex
		count = map[string]int{}
	)
	tk.EXPECT().IssueRefresh(mock.Anything).
		RunAndReturn(func(userID string) (string, string, time.Time, error) {
			mu.Lock()
			count[userID]++
			n := count[userID]
			mu.Unlock()
			jti := "jti-" + userID
			if n > 1 {
				jti = fmt.Sprintf("jti-%s-%d", userID, n)
			}
			return "refresh." + userID + "." + jti, jti, time.Now().Add(time.Hour), nil
		}).Maybe()
	return tk
}

func defaultParser(t *testing.T) *mocks.MockTokenParser {
	t.Helper()
	p := mocks.NewMockTokenParser(t)
	p.EXPECT().ParseRefresh(mock.Anything).Return("u1", "jti-u1", nil).Maybe()
	return p
}

func parserReturning(t *testing.T, userID, jti string, err error) *mocks.MockTokenParser {
	t.Helper()
	p := mocks.NewMockTokenParser(t)
	p.EXPECT().ParseRefresh(mock.Anything).Return(userID, jti, err).Maybe()
	return p
}

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

func defaultGuard(t *testing.T) *mocks.MockLoginGuard {
	t.Helper()
	g := mocks.NewMockLoginGuard(t)
	g.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	g.EXPECT().RecordFailure(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	g.EXPECT().RecordSuccess(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
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
}

type ucOption func(*ucOpts)

func withUsers(u *mocks.MockUserReader) ucOption          { return func(o *ucOpts) { o.users = u } }
func withVerifier(v *mocks.MockPasswordVerifier) ucOption { return func(o *ucOpts) { o.verify = v } }
func withSetter(s *mocks.MockPasswordSetter) ucOption     { return func(o *ucOpts) { o.setpw = s } }
func withParser(p *mocks.MockTokenParser) ucOption        { return func(o *ucOpts) { o.parser = p } }

func withSessions(s *memSessions) ucOption { return func(o *ucOpts) { o.sessions = s } }

func withPolicy(p *mocks.MockAuthorizer) ucOption { return func(o *ucOpts) { o.policy = p } }
func withGuard(g *mocks.MockLoginGuard) ucOption  { return func(o *ucOpts) { o.guard = g } }

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
		o.policy = defaultPolicy(t, true)
	}
	if o.guard == nil {
		o.guard = defaultGuard(t)
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
		Tx:             txkit.Identity{},
		Policy:         o.policy,
		Guard:          o.guard,
		Audit:          aud,
		Log:            logkit.Nop(),
		Clock:          clockkit.NewFixed(time.Now()),
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
			&auth.User{ID: "u1", Email: "a@b.com", PasswordHash: "hash:pass1234", Status: "active", Role: identity.RoleClient}, nil,
		)
		f := newUC(t, withUsers(users))

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
			&auth.User{ID: "u1", PasswordHash: "hash:correct", Status: "active"}, nil,
		)
		f := newUC(t, withUsers(users))

		_, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "a@b.com", Password: "wrong"})
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
		require.True(t, f.audit.Has("login_failed"))
	})

	t.Run("blocked status", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, mock.Anything).Return(
			&auth.User{ID: "u1", PasswordHash: "hash:pass1234", Status: "blocked"}, nil,
		)
		f := newUC(t, withUsers(users))

		out, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
		require.Nil(t, out)
		require.True(t, f.audit.Has("login_failed"))
		require.False(t, f.audit.Has("login"))
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, mock.Anything).Return(nil, errors.New("not found"))
		f := newUC(t, withUsers(users))

		_, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "x@y.com", Password: "p"})
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
	})

	t.Run("account locked", func(t *testing.T) {
		t.Parallel()
		g := mocks.NewMockLoginGuard(t)
		g.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).
			Return(&auth.LockState{Until: time.Now().Add(15 * time.Minute)}, nil)
		f := newUC(t, withGuard(g))

		_, err := f.uc.Login(context.Background(), &auth.LoginInput{Email: "a@b.com", Password: "p"})
		require.ErrorIs(t, err, auth.ErrAccountLocked)
		require.True(t, f.audit.Has("login_locked"))
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
		require.NoError(t, sessions.Insert(context.Background(), "jti-old", "u1", time.Now().Add(time.Hour)))
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
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
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
		require.NoError(t, sessions.Insert(context.Background(), "jti-u1", "u1", time.Now().Add(-time.Minute)))
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByID(mock.Anything, mock.Anything).Return(&auth.User{ID: "u1"}, nil).Maybe()
		f := newUC(t, withUsers(users), withSessions(sessions))

		_, err := f.uc.Refresh(context.Background(), "tok")
		require.ErrorIs(t, err, auth.ErrRefreshExpired)
	})

	t.Run("reused jti", func(t *testing.T) {
		t.Parallel()
		sessions := newMemSessions()
		require.NoError(t, sessions.Insert(context.Background(), "jti-u1", "u1", time.Now().Add(time.Hour)))
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
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
	})
}

func TestLogout(t *testing.T) {
	t.Parallel()

	t.Run("marks session used", func(t *testing.T) {
		t.Parallel()
		sessions := newMemSessions()
		require.NoError(t, sessions.Insert(context.Background(), "jti-u1", "u1", time.Now().Add(time.Hour)))
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

func TestChangePassword(t *testing.T) {
	t.Parallel()

	t.Run("happy path propagates version", func(t *testing.T) {
		t.Parallel()
		verify := mocks.NewMockPasswordVerifier(t)
		verify.EXPECT().VerifyPassword(mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
		setter := mocks.NewMockPasswordSetter(t)
		var lastHash string
		var lastVersion int
		setter.EXPECT().UpdatePassword(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, _, hash string, version int) error {
				lastHash = hash
				lastVersion = version
				return nil
			})
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByID(mock.Anything, mock.Anything).Return(&auth.User{ID: "u1", Version: 7}, nil)
		f := newUC(t, withUsers(users), withVerifier(verify), withSetter(setter))

		ctx := identity.With(context.Background(), identity.Identity{UserID: "u1"})
		require.NoError(t, f.uc.ChangePassword(ctx, "u1", "old", "newpassword123"))
		require.Equal(t, "hash:newpassword123", lastHash)
		require.Equal(t, 7, lastVersion)
		require.True(t, f.audit.Has("change_password"))
	})

	t.Run("wrong old", func(t *testing.T) {
		t.Parallel()
		verify := mocks.NewMockPasswordVerifier(t)
		verify.EXPECT().VerifyPassword(mock.Anything, mock.Anything, mock.Anything).Return(false, nil)
		f := newUC(t, withVerifier(verify))

		ctx := identity.With(context.Background(), identity.Identity{UserID: "u1"})
		err := f.uc.ChangePassword(ctx, "u1", "old", "newpassword123")
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
	})

	t.Run("weak new", func(t *testing.T) {
		t.Parallel()
		f := newUC(t)
		ctx := identity.With(context.Background(), identity.Identity{UserID: "u1"})
		err := f.uc.ChangePassword(ctx, "u1", "old", "short")
		require.ErrorIs(t, err, auth.ErrWeakPassword)
	})

	t.Run("forbidden", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withPolicy(defaultPolicy(t, false)))
		ctx := identity.With(context.Background(), identity.Identity{UserID: "u1"})
		err := f.uc.ChangePassword(ctx, "u2", "old", "newpassword123")
		var fe *auth.ForbiddenError
		require.ErrorAs(t, err, &fe)
	})
}
