package auth_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth"
	authbruteforce "git.stit.tech/stit-core/go-backend-template/internal/domain/auth/infra/bruteforce"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/auth/mocks"
	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/identitytest"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/auditkit"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/authnkit/session"
	"git.stit.tech/stit-core/golibinfra/bruteforcekit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/testkit/audittest"
	"git.stit.tech/stit-core/golibinfra/testkit/authntest"
	"git.stit.tech/stit-core/golibinfra/testkit/bruteforcetest"
	"git.stit.tech/stit-core/golibinfra/testkit/logtest"
	"git.stit.tech/stit-core/golibinfra/testkit/metrictest"
	"git.stit.tech/stit-core/golibinfra/testkit/pgtest"
	"git.stit.tech/stit-core/golibinfra/testkit/sessiontest"
)

var fixedNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// tokens — выпуск access-токена: чистый порт без эффектов, поэтому фейк, а не мок — токен
// "access.<id>.<role>" выводится из учётной записи, err — сбой подписи.
type tokens struct{ err error }

func (tk tokens) IssueAccess(_ context.Context, u *auth.User) (string, error) {
	if tk.err != nil {
		return "", tk.err
	}
	return "access." + u.ID + "." + string(u.Role), nil
}

// loginGuard ждёт ровно одну попытку входа subject с IP запроса до проверки пароля и, если succeeded, — один
// успех.
func loginGuard(t *testing.T, subject string, succeeded bool) *mocks.MockLoginGuard {
	t.Helper()
	g := mocks.NewMockLoginGuard(t)
	g.EXPECT().Attempt(mock.Anything, subject, identitytest.IP).Return(nil, nil).Once()
	if succeeded {
		g.EXPECT().RecordSuccess(mock.Anything, subject, identitytest.IP).Return(nil).Once()
	}
	return g
}

// busyLoginGuard ждёт попытку входа subject с IP запроса и после неё ровно один возврат: пароль хэшер не проверил.
// Возврат идёт и после отмены запроса — его ctx не отменён.
func busyLoginGuard(t *testing.T, subject string) *mocks.MockLoginGuard {
	t.Helper()
	g := mocks.NewMockLoginGuard(t)
	attempt := g.EXPECT().Attempt(mock.Anything, subject, identitytest.IP).Return(nil, nil).Once()
	live := mock.MatchedBy(func(ctx context.Context) bool { return ctx.Err() == nil })
	g.EXPECT().Refund(live, subject, identitytest.IP).Return(nil).Once().NotBefore(attempt)
	return g
}

// canceled — ctx запроса ctx, который клиент уже оборвал.
func canceled(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	return ctx
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
	setpw    *mocks.MockPasswordSetter
	guard    auth.LoginGuard
	tokens   auth.Tokens
	hasher   authnkit.Hasher
	audit    auditkit.Logger
	sessions func(session.Store) session.Store
	metrics  *auth.Metrics
}

type ucOption func(*ucOpts)

func withUsers(u *mocks.MockUserReader) ucOption      { return func(o *ucOpts) { o.users = u } }
func withSetter(s *mocks.MockPasswordSetter) ucOption { return func(o *ucOpts) { o.setpw = s } }
func withGuard(g auth.LoginGuard) ucOption            { return func(o *ucOpts) { o.guard = g } }

func withTokens(tk auth.Tokens) ucOption    { return func(o *ucOpts) { o.tokens = tk } }
func withHasher(h authnkit.Hasher) ucOption { return func(o *ucOpts) { o.hasher = h } }
func withAudit(a auditkit.Logger) ucOption  { return func(o *ucOpts) { o.audit = a } }
func withMetrics(m *auth.Metrics) ucOption  { return func(o *ucOpts) { o.metrics = m } }

// withSessions оборачивает хранилище сессий фикстуры: сбой одного метода поверх фейка кита.
func withSessions(wrap func(session.Store) session.Store) ucOption {
	return func(o *ucOpts) { o.sessions = wrap }
}

// newUC — usecase на строгих моках портов без ожиданий (вызов, которого кейс не ждёт, — провал) и фейке хранилища
// сессий кита (sessiontest) с часами фикстуры: время сессий двигает кейс (f.clock.Advance).
func newUC(t *testing.T, opts ...ucOption) ucFixture {
	t.Helper()
	o := &ucOpts{
		users:  mocks.NewMockUserReader(t),
		setpw:  mocks.NewMockPasswordSetter(t),
		guard:  mocks.NewMockLoginGuard(t),
		tokens: tokens{},
		hasher: &authntest.DeterministicHasher{},
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
	if o.audit == nil {
		o.audit = aud
	}

	uc := auth.NewUsecase(auth.UsecaseDeps{
		Users:       o.users,
		PasswordSet: o.setpw,
		Hasher:      o.hasher,
		Tokens:      o.tokens,
		Sessions:    issueInTx{Store: sessions, t: t},
		Tx:          pgtest.Identity{},
		Guard:       o.guard,
		Audit:       o.audit,
		Log:         log,
		Clock:       clock,
		Metrics:     o.metrics,
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

// audited — запись аудита запроса identitytest: время — часы usecase, IP клиента и ID запроса auditkit
// берёт из ctx.
func audited(e auditkit.Entry) *auditkit.Entry {
	e.At, e.IP, e.RequestID = fixedNow, identitytest.IP, identitytest.RequestID
	return &e
}

// issueInTx — хранилище сессий, которое открывает сессию только в транзакции: под строкой пользователя,
// которую она держит.
type issueInTx struct {
	session.Store
	t *testing.T
}

func (s issueInTx) Issue(ctx context.Context, subject string) (session.Token, error) {
	require.True(s.t, pgkit.InTx(ctx), "the session is opened outside the transaction that locks the user")
	return s.Store.Issue(ctx, subject)
}

// signIn — учётная запись u, которую находит вход: поиск по адресу и чтение под блокировкой при открытии
// сессии (в транзакции) отдают её же.
func signIn(t *testing.T, u *auth.User) *mocks.MockUserReader {
	t.Helper()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByEmail(mock.Anything, u.Email).Return(u, nil).Once()
	users.EXPECT().LockByID(mock.MatchedBy(pgkit.InTx), u.ID).Return(u, nil).Once()
	return users
}

// alice — учётная запись входа тестов: пароль pass1234, пускают.
func alice() *auth.User {
	return &auth.User{ID: "u1", Email: "a@b.com", PasswordHash: "hash:pass1234", Name: "Alice", CanLogin: true, Role: identity.RoleClient}
}

// found — учётная запись u, которую находит поиск по адресу: дальше проверки пароля вход не идёт.
func found(t *testing.T, u *auth.User) *mocks.MockUserReader {
	t.Helper()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByEmail(mock.Anything, u.Email).Return(u, nil).Once()
	return users
}

var (
	errStore   = errors.New("authn_sessions: connection refused")
	errSign    = errors.New("jwt: signer unavailable")
	errHashing = errors.New("argon2: out of memory")
	errAudit   = errors.New("audit_log: connection refused")
)

// failingSessions — хранилище сессий, у которого сбоит каждый вызов.
type failingSessions struct{ session.Store }

func (failingSessions) Issue(context.Context, string) (session.Token, error) {
	return session.Token{}, errStore
}

func (failingSessions) Rotate(context.Context, string) (session.Token, error) {
	return session.Token{}, errStore
}

func (failingSessions) Revoke(context.Context, string) (string, error) { return "", errStore }

// revokeFails — хранилище сессий, у которого сбоит только закрытие сессии.
type revokeFails struct{ session.Store }

func (revokeFails) Revoke(context.Context, string) (string, error) { return "", errStore }

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
		f := newUC(t, withUsers(signIn(t, alice())), withGuard(loginGuard(t, "a@b.com", true)))

		out, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
		require.NoError(t, err)
		require.Equal(t, "access.u1.client", out.AccessToken)
		require.Equal(t, alice(), out.User, "the response names the account: address, name and role")
		require.Equal(t, 1, f.sessions.Active("u1"), "the login opens a session of the user")
		next, err := f.sessions.Rotate(context.Background(), out.RefreshToken)
		require.NoError(t, err, "the refresh token is the token of that session")
		require.Equal(t, "u1", next.Subject)
		require.Equal(t, audited(auditkit.Entry{ActorID: "u1", Action: "login", Resource: "user", ResourceID: "u1"}), f.audit.Last("login"))
	})

	// Сессия не открылась — входа нет: ни токенов, ни аудита входа.
	t.Run("session not opened", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withUsers(signIn(t, alice())), withGuard(loginGuard(t, "a@b.com", true)),
			withSessions(func(s session.Store) session.Store { return failingSessions{s} }))

		out, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
		require.ErrorIs(t, err, errStore)
		require.Nil(t, out)
		require.False(t, f.audit.Has("login"))
	})

	t.Run("wrong password", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withUsers(found(t, alice())), withGuard(loginGuard(t, "a@b.com", false)))

		out, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: "wrong"})
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
		require.Nil(t, out)
		require.Zero(t, f.sessions.Active("u1"))
		require.Equal(t, audited(auditkit.Entry{
			Action: "login_failed", Resource: "user", ResourceID: "u1", Metadata: map[string]any{"reason": "invalid_credentials"},
		}), f.audit.Last("login_failed"), "the audit names the account by ID, not by address")
	})

	// Заблокированного не пускают и с верным паролем: отказ тот же, что на неверный пароль, причина — в аудите.
	t.Run("blocked status", func(t *testing.T) {
		t.Parallel()
		blocked := alice()
		blocked.CanLogin = false
		f := newUC(t, withUsers(found(t, blocked)), withGuard(loginGuard(t, "a@b.com", false)))

		out, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
		require.Nil(t, out)
		require.Zero(t, f.sessions.Active("u1"))
		require.Equal(t, audited(auditkit.Entry{
			Action: "login_failed", Resource: "user", ResourceID: "u1", Metadata: map[string]any{"reason": "blocked"},
		}), f.audit.Last("login_failed"))
		require.False(t, f.audit.Has("login"))
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, "x@y.com").Return(nil, auth.ErrUserNotFound).Once()
		f := newUC(t, withUsers(users), withGuard(loginGuard(t, "x@y.com", false)))

		_, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "x@y.com", Password: "p"})
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
		require.Empty(t, f.audit.Entries(), "an unknown address does not reach the audit")
	})

	t.Run("lookup infra error is not invalid credentials", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, "x@y.com").Return(nil, errors.New("db down")).Once()
		f := newUC(t, withUsers(users), withGuard(loginGuard(t, "x@y.com", false)))

		_, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "x@y.com", Password: "p"})
		require.Error(t, err)
		require.NotErrorIs(t, err, auth.ErrInvalidCredentials)
		require.False(t, f.audit.Has("login_failed"))
	})

	t.Run("account locked", func(t *testing.T) {
		t.Parallel()
		g := mocks.NewMockLoginGuard(t)
		g.EXPECT().Attempt(mock.Anything, "a@b.com", identitytest.IP).
			Return(&auth.LockState{RetryAfter: 15 * time.Minute}, nil).Once()
		f := newUC(t, withGuard(g))

		_, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: "p"})
		require.ErrorIs(t, err, auth.ErrAccountLocked)
		require.EqualError(t, err, auth.ErrAccountLocked.Error())
		var locked *auth.LockedError
		require.ErrorAs(t, err, &locked)
		require.Equal(t, 15*time.Minute, locked.RetryAfter(), "retry when the lock ends")
		require.Empty(t, f.audit.Entries(), "a lock before the lookup knows no account: the outcome goes to the metric")
	})

	// Fail-closed: строгий мок пользователей — до поиска учётной записи и пароля вход не доходит.
	t.Run("guard failure refuses the login", func(t *testing.T) {
		t.Parallel()
		g := mocks.NewMockLoginGuard(t)
		g.EXPECT().Attempt(mock.Anything, "a@b.com", identitytest.IP).Return(nil, errors.New("guard store down")).Once()
		f := newUC(t, withGuard(g))

		_, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
		require.ErrorContains(t, err, "guard store down")
		require.NotErrorIs(t, err, auth.ErrInvalidCredentials, "a guard failure is not a wrong password")
		require.False(t, f.audit.Has("login"))
	})

	t.Run("attempt the guard cannot count is invalid credentials", func(t *testing.T) {
		t.Parallel()
		g := mocks.NewMockLoginGuard(t)
		g.EXPECT().Attempt(mock.Anything, "", identitytest.IP).Return(nil, auth.ErrInvalidCredentials).Once()
		f := newUC(t, withGuard(g))

		_, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "", Password: "pass1234"})
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
	})

	t.Run("success not recorded keeps the login", func(t *testing.T) {
		t.Parallel()
		g := mocks.NewMockLoginGuard(t)
		g.EXPECT().Attempt(mock.Anything, "a@b.com", identitytest.IP).Return(nil, nil).Once()
		g.EXPECT().RecordSuccess(mock.Anything, "a@b.com", identitytest.IP).Return(errors.New("guard store down")).Once()
		f := newUC(t, withUsers(signIn(t, alice())), withGuard(g))

		out, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
		require.NoError(t, err)
		require.Equal(t, "access.u1.client", out.AccessToken)
		require.True(t, f.audit.Has("login"))
		e, logged := f.log.Find(slog.LevelError, "guard record success")
		require.True(t, logged)
		require.Contains(t, e.Str("error"), "guard store down")
	})

	// Сбой записи аудита вход не отменяет: причина — в логе.
	t.Run("audit not written keeps the login", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withUsers(signIn(t, alice())), withGuard(loginGuard(t, "a@b.com", true)), withAudit(&audittest.Recorder{LogErr: errAudit}))

		out, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
		require.NoError(t, err)
		require.Equal(t, "access.u1.client", out.AccessToken)
		e, logged := f.log.Find(slog.LevelError, "audit login")
		require.True(t, logged)
		require.Contains(t, e.Str("error"), errAudit.Error())
	})
}

// Каждая учтённая попытка входа стоит одной проверки пароля: у неизвестного адреса и заблокированного —
// холостой, поэтому время ответа не выдаёт, есть ли учётная запись.
func TestLogin_OnePasswordCheckPerAttempt(t *testing.T) {
	t.Parallel()
	blocked := alice()
	blocked.CanLogin = false
	unknown := func(t *testing.T) *mocks.MockUserReader {
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(nil, auth.ErrUserNotFound).Once()
		return users
	}
	tests := []struct {
		name              string
		users             func(t *testing.T) *mocks.MockUserReader
		password          string
		succeeded         bool
		verified, dummies int
	}{
		{"unknown address", unknown, "pass1234", false, 0, 1},
		{"blocked", func(t *testing.T) *mocks.MockUserReader { return found(t, blocked) }, "pass1234", false, 0, 1},
		{"wrong password", func(t *testing.T) *mocks.MockUserReader { return found(t, alice()) }, "wrong", false, 1, 0},
		{"right password", func(t *testing.T) *mocks.MockUserReader { return signIn(t, alice()) }, "pass1234", true, 1, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := &authntest.DeterministicHasher{}
			f := newUC(t, withUsers(tc.users(t)), withGuard(loginGuard(t, "a@b.com", tc.succeeded)), withHasher(h))

			_, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: tc.password})
			require.Equal(t, tc.succeeded, err == nil, "login error: %v", err)
			require.Equal(t, tc.verified, h.VerifyCalls(), "password checks")
			require.Equal(t, tc.dummies, h.VerifyDummyCalls(), "dummy password checks")
		})
	}
}

// verifyFails — хэшер, у которого проверка пароля сбоит, а не отвечает «не совпал».
type verifyFails struct {
	authntest.DeterministicHasher
	err error
}

func (h *verifyFails) Verify(context.Context, string, string) (bool, error) { return false, h.err }

// Сбой проверки пароля — ошибка сервиса с причиной, а не «неверный пароль»: ни 401, ни записи login_failed,
// исход — error; попытка уже учтена guard'ом, успеха нет.
func TestLogin_HasherFailure(t *testing.T) {
	t.Parallel()
	unreadable := alice()
	unreadable.PasswordHash = "not-a-phc-string"
	tests := []struct {
		name   string
		user   *auth.User
		hasher authnkit.Hasher
		want   error
	}{
		{"argon2id queue deadline", alice(), &verifyFails{err: fmt.Errorf("authnkit: argon2id: waiting for a slot: %w", context.DeadlineExceeded)}, context.DeadlineExceeded},
		{"unreadable stored hash", unreadable, &authntest.DeterministicHasher{}, authnkit.ErrInvalidHash},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := metrictest.NewProvider()
			f := newUC(t, withUsers(found(t, tc.user)), withGuard(loginGuard(t, "a@b.com", false)), withHasher(tc.hasher),
				withMetrics(auth.NewMetrics(p.Meter("test"))))

			out, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
			require.ErrorIs(t, err, tc.want)
			require.NotErrorIs(t, err, auth.ErrInvalidCredentials, "a hasher failure is not a wrong password")
			require.Nil(t, out)
			require.Zero(t, f.sessions.Active("u1"))
			require.Empty(t, f.audit.Entries(), "a failure of the service writes no login_failed")
			requireOutcome(t, p, "biz_auth_login_attempts_total", "error")
		})
	}
}

// dummyFails — хэшер, у которого холостая проверка сбоит (отмена ctx в очереди), а не проходит.
type dummyFails struct {
	authntest.DeterministicHasher
	err error
}

func (h *dummyFails) VerifyDummy(context.Context) error { return h.err }

// Сбой холостой проверки — как сбой проверки пароля: ошибка сервиса, а не «неверный пароль»; попытка остаётся
// учтённой (возврат — только на ErrBusy), аудита нет, исход — error.
func TestLogin_DummyCheckFailure(t *testing.T) {
	t.Parallel()
	blocked := alice()
	blocked.CanLogin = false
	unknown := func(t *testing.T) *mocks.MockUserReader {
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(nil, auth.ErrUserNotFound).Once()
		return users
	}
	tests := []struct {
		name  string
		users func(t *testing.T) *mocks.MockUserReader
	}{
		{"unknown address", unknown},
		{"blocked account", func(t *testing.T) *mocks.MockUserReader { return found(t, blocked) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := metrictest.NewProvider()
			f := newUC(t, withUsers(tc.users(t)), withGuard(loginGuard(t, "a@b.com", false)),
				withHasher(&dummyFails{err: context.Canceled}), withMetrics(auth.NewMetrics(p.Meter("test"))))

			out, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
			require.ErrorIs(t, err, context.Canceled)
			require.NotErrorIs(t, err, auth.ErrInvalidCredentials, "a failed dummy check is not a wrong password")
			require.NotErrorIs(t, err, auth.ErrBusy)
			require.Nil(t, out)
			require.Empty(t, f.audit.Entries())
			requireOutcome(t, p, "biz_auth_login_attempts_total", "error")
		})
	}
}

// Полная очередь хэшера (ErrBusy) на любом пути проверки — один ответ: ErrBusy с ошибкой кита в цепочке (503 с её
// Retry-After), по коду и времени учётную запись не различить. Попытка, учтённая guard'ом, возвращается ровно раз — и
// у клиента, оборвавшего запрос; аудита нет, исход — busy.
func TestLogin_HasherBusy(t *testing.T) {
	t.Parallel()
	blocked := alice()
	blocked.CanLogin = false
	unknown := func(t *testing.T) *mocks.MockUserReader {
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(nil, auth.ErrUserNotFound).Once()
		return users
	}
	tests := []struct {
		name     string
		users    func(t *testing.T) *mocks.MockUserReader
		password string
	}{
		{"known address", func(t *testing.T) *mocks.MockUserReader { return found(t, alice()) }, "pass1234"},
		{"password over the hasher limit", func(t *testing.T) *mocks.MockUserReader { return found(t, alice()) }, strings.Repeat("я", authnkit.MaxPasswordLen)},
		{"unknown address", unknown, "pass1234"},
		{"blocked account", func(t *testing.T) *mocks.MockUserReader { return found(t, blocked) }, "pass1234"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := metrictest.NewProvider()
			f := newUC(t, withUsers(tc.users(t)), withGuard(busyLoginGuard(t, "a@b.com")),
				withHasher(&authntest.DeterministicHasher{Busy: true}), withMetrics(auth.NewMetrics(p.Meter("test"))))

			out, err := f.uc.Login(canceled(identitytest.Anon()), auth.LoginInput{Email: "a@b.com", Password: tc.password})
			require.ErrorIs(t, err, auth.ErrBusy)
			require.ErrorIs(t, err, authnkit.ErrBusy, "the kit's error carries the Retry-After of the 503")
			require.Nil(t, out)
			require.Zero(t, f.sessions.Active("u1"))
			require.Empty(t, f.audit.Entries(), "an unchecked password writes no login_failed")
			requireOutcome(t, p, "biz_auth_login_attempts_total", "busy")
		})
	}
}

// Сбой возврата ответ не меняет — ErrBusy, причина в логе: попытка осталась учтённой, как без возврата.
func TestLogin_RefundFailure(t *testing.T) {
	t.Parallel()
	g := mocks.NewMockLoginGuard(t)
	g.EXPECT().Attempt(mock.Anything, "a@b.com", identitytest.IP).Return(nil, nil).Once()
	g.EXPECT().Refund(mock.Anything, "a@b.com", identitytest.IP).Return(errors.New("guard store down")).Once()
	f := newUC(t, withUsers(found(t, alice())), withGuard(g), withHasher(&authntest.DeterministicHasher{Busy: true}))

	_, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
	require.ErrorIs(t, err, auth.ErrBusy)
	require.NotContains(t, err.Error(), "guard store down", "the answer is the 503 of the busy hasher")
	e, logged := f.log.Find(slog.LevelError, "guard refund")
	require.True(t, logged)
	require.Contains(t, e.Str("error"), "guard store down")
}

// Возврат на guard'е с политикой pgstore (bruteforcetest.Counting): отказы ErrBusy подряд, сколько бы их ни было, пару
// не блокируют — клиент, повторяющий 503, входит, как только хэшер освободился; неверные пароли блокируют её после
// MaxFailures, как прежде.
func TestLogin_BusyDoesNotLockThePair(t *testing.T) {
	t.Parallel()
	cfg := bruteforcekit.Config{
		MaxFailures: 5, LockDuration: 15 * time.Minute, SubjectMaxFailures: 20, SubjectLockDuration: time.Minute,
		CleanupRetention: 24 * time.Hour, CleanupInterval: 5 * time.Minute, IPv6Prefix: 64,
	}
	guard, err := bruteforcetest.NewCounting(cfg, clockkit.NewFixed(fixedNow))
	require.NoError(t, err)
	hasher := &authntest.DeterministicHasher{Busy: true}
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(alice(), nil).Times(3*cfg.MaxFailures + 1)
	users.EXPECT().LockByID(mock.MatchedBy(pgkit.InTx), "u1").Return(alice(), nil).Once()
	f := newUC(t, withUsers(users), withGuard(authbruteforce.NewLoginGuard(guard)), withHasher(hasher))
	login := func(password string) error {
		_, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: password})
		return err
	}

	for range 2 * cfg.MaxFailures {
		require.ErrorIs(t, login("pass1234"), auth.ErrBusy)
	}
	hasher.Busy = false
	require.NoError(t, login("pass1234"), "refusals of a busy hasher leave the pair open")

	for range cfg.MaxFailures {
		require.ErrorIs(t, login("wrong"), auth.ErrInvalidCredentials)
	}
	require.ErrorIs(t, login("pass1234"), auth.ErrAccountLocked, "checked wrong passwords lock the pair")
}

// Пароль длиннее предела хэшера ни с одним хэшем не совпадает: это неверный пароль — с записью аудита, а
// холостой хэш вместо него считает сам Verify хэшера.
func TestLogin_PasswordOverHasherLimit(t *testing.T) {
	t.Parallel()
	p := metrictest.NewProvider()
	f := newUC(t, withUsers(found(t, alice())), withGuard(loginGuard(t, "a@b.com", false)), withMetrics(auth.NewMetrics(p.Meter("test"))))

	_, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: strings.Repeat("я", authnkit.MaxPasswordLen)})
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)
	require.Equal(t, audited(auditkit.Entry{
		Action: "login_failed", Resource: "user", ResourceID: "u1", Metadata: map[string]any{"reason": "invalid_credentials"},
	}), f.audit.Last("login_failed"))
	requireOutcome(t, p, "biz_auth_login_attempts_total", "invalid_credentials")
}

// Access-токен не подписан — входа нет: ошибка сервиса, а не «неверный пароль», сессии и аудита входа нет.
func TestLogin_AccessNotIssued(t *testing.T) {
	t.Parallel()
	p := metrictest.NewProvider()
	f := newUC(t, withUsers(found(t, alice())), withGuard(loginGuard(t, "a@b.com", true)), withTokens(tokens{err: errSign}),
		withMetrics(auth.NewMetrics(p.Meter("test"))))

	out, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
	require.ErrorIs(t, err, errSign)
	require.NotErrorIs(t, err, auth.ErrInvalidCredentials)
	require.Nil(t, out)
	require.Zero(t, f.sessions.Active("u1"))
	require.False(t, f.audit.Has("login"))
	requireOutcome(t, p, "biz_auth_login_attempts_total", "error")
}

// Смена пароля, роли или статуса, которая успела между проверкой пароля и открытием сессии, отказывает во
// входе: учётную запись при открытии сессии перечитывают под блокировкой, и сессии старых учётных данных нет.
func TestLogin_CredentialsChangedDuringLogin(t *testing.T) {
	t.Parallel()
	changed := func(edit func(u *auth.User)) *auth.User {
		u := alice()
		edit(u)
		return u
	}
	tests := []struct {
		name    string
		current *auth.User // учётная запись под блокировкой; nil — её нет
		err     error
	}{
		{name: "password changed", current: changed(func(u *auth.User) { u.PasswordHash = "hash:new-password" })},
		{name: "role changed", current: changed(func(u *auth.User) { u.Role = identity.RoleAdmin })},
		{name: "blocked", current: changed(func(u *auth.User) { u.CanLogin = false })},
		{name: "deleted", err: auth.ErrUserNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			users := mocks.NewMockUserReader(t)
			users.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(alice(), nil).Once()
			users.EXPECT().LockByID(mock.MatchedBy(pgkit.InTx), "u1").Return(tc.current, tc.err).Once()
			p := metrictest.NewProvider()
			f := newUC(t, withUsers(users), withGuard(loginGuard(t, "a@b.com", true)), withMetrics(auth.NewMetrics(p.Meter("test"))))

			out, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
			require.ErrorIs(t, err, auth.ErrInvalidCredentials)
			require.Nil(t, out)
			require.Zero(t, f.sessions.Active("u1"), "no session of the outdated credentials")
			require.Equal(t, audited(auditkit.Entry{
				Action: "login_failed", Resource: "user", ResourceID: "u1", Metadata: map[string]any{"reason": "invalid_credentials"},
			}), f.audit.Last("login_failed"))
			require.False(t, f.audit.Has("login"))
			requireOutcome(t, p, "biz_auth_login_attempts_total", "invalid_credentials")
		})
	}
}

// Сбой чтения учётной записи под блокировкой — не «неверный пароль», а ошибка сервиса; сессии нет.
func TestLogin_LockFailure(t *testing.T) {
	t.Parallel()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(alice(), nil).Once()
	users.EXPECT().LockByID(mock.Anything, "u1").Return(nil, errors.New("db down")).Once()
	f := newUC(t, withUsers(users), withGuard(loginGuard(t, "a@b.com", true)))

	_, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: "pass1234"})
	require.ErrorContains(t, err, "db down")
	require.NotErrorIs(t, err, auth.ErrInvalidCredentials)
	require.Zero(t, f.sessions.Active("u1"))
}

// Обновление ротирует токен в той же сессии: прежний токен погашен, новый — следующий в цепочке; access-токен
// несёт роль из учётной записи, а не роль по умолчанию.
func TestRefresh(t *testing.T) {
	t.Parallel()
	for _, role := range []identity.Role{identity.RoleClient, identity.RoleAdmin} {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			users := mocks.NewMockUserReader(t)
			users.EXPECT().GetByID(mock.Anything, "u1").Return(&auth.User{ID: "u1", Email: "a@b.com", CanLogin: true, Role: role}, nil).Once()
			f := newUC(t, withUsers(users))
			first := f.issue(t, "u1")

			out, err := f.uc.Refresh(identitytest.Anon(), first.Value)
			require.NoError(t, err)
			require.Equal(t, "access.u1."+string(role), out.AccessToken)
			require.NotEqual(t, first.Value, out.RefreshToken)
			next, err := f.sessions.Rotate(context.Background(), out.RefreshToken)
			require.NoError(t, err, "the new refresh token continues the session")
			require.Equal(t, first.SessionID, next.SessionID)
			require.Empty(t, f.audit.Entries())
		})
	}
}

// Повтор погашенного токена — его предъявил клиент или укравший: сессия закрыта целиком, вместе со
// следующим токеном цепочки, повтор — в аудите учётной записи.
func TestRefresh_Reused(t *testing.T) {
	t.Parallel()
	f := newUC(t, withUsers(canLogin(t)))
	first := f.issue(t, "u1")
	out, err := f.uc.Refresh(identitytest.Anon(), first.Value)
	require.NoError(t, err)

	_, err = f.uc.Refresh(identitytest.Anon(), first.Value)
	require.ErrorIs(t, err, auth.ErrRefreshReused)
	require.Equal(t, audited(auditkit.Entry{ActorID: "u1", Action: "refresh_reused", Resource: "session", ResourceID: first.SessionID}),
		f.audit.Last("refresh_reused"))
	_, err = f.uc.Refresh(identitytest.Anon(), out.RefreshToken)
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

		_, err := f.uc.Refresh(identitytest.Anon(), tok.Value)
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
			out, err := f.uc.Refresh(identitytest.Anon(), refresh)
			require.NoError(t, err)
			refresh = out.RefreshToken
		}
		f.clock.Advance(31 * time.Minute)

		_, err := f.uc.Refresh(identitytest.Anon(), refresh)
		require.ErrorIs(t, err, auth.ErrRefreshExpired, "three hours after the login the session is over")
	})
}

func TestRefresh_UnknownToken(t *testing.T) {
	t.Parallel()
	_, err := newUC(t).uc.Refresh(identitytest.Anon(), "not-a-session-token")
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

			out, err := f.uc.Refresh(identitytest.Anon(), tok.Value)
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

	_, err := f.uc.Refresh(identitytest.Anon(), tok.Value)
	require.ErrorIs(t, err, auth.ErrRefreshInvalid)
	e, logged := f.log.Find(slog.LevelError, "revoke session")
	require.True(t, logged)
	require.Equal(t, "u1", e.Str("user_id"))
	require.Contains(t, e.Str("error"), errStore.Error())
}

// Сбой хранилища или поиска пользователя — не «войти заново», а ошибка сервиса.
func TestRefresh_Failure(t *testing.T) {
	t.Parallel()
	t.Run("session store", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withSessions(func(s session.Store) session.Store { return failingSessions{s} }))
		_, err := f.uc.Refresh(identitytest.Anon(), "tok")
		require.ErrorIs(t, err, errStore)
	})
	t.Run("user lookup", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByID(mock.Anything, "u1").Return(nil, errors.New("db down")).Once()
		f := newUC(t, withUsers(users))
		tok := f.issue(t, "u1")

		_, err := f.uc.Refresh(identitytest.Anon(), tok.Value)
		require.ErrorContains(t, err, "db down")
		require.NotErrorIs(t, err, auth.ErrRefreshInvalid)
	})
	t.Run("access token", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withUsers(canLogin(t)), withTokens(tokens{err: errSign}))
		tok := f.issue(t, "u1")

		out, err := f.uc.Refresh(identitytest.Anon(), tok.Value)
		require.ErrorIs(t, err, errSign)
		require.NotErrorIs(t, err, auth.ErrRefreshInvalid)
		require.Nil(t, out)
	})
}

// Выход закрывает сессию токена и пишет аудит с её владельцем; неизвестный токен и закрытая раньше
// сессия — тоже выход, без записи.
func TestLogout(t *testing.T) {
	t.Parallel()
	f := newUC(t)
	tok := f.issue(t, "u1")

	require.NoError(t, f.uc.Logout(identitytest.Anon(), tok.Value))
	require.Zero(t, f.sessions.Active("u1"))
	_, err := f.sessions.Rotate(context.Background(), tok.Value)
	require.ErrorIs(t, err, session.ErrInvalid, "the token of a closed session does not refresh")
	require.NoError(t, f.uc.Logout(identitytest.Anon(), "unknown"))
	require.NoError(t, f.uc.Logout(identitytest.Anon(), tok.Value))
	require.Equal(t, []auditkit.Entry{*audited(auditkit.Entry{ActorID: "u1", Action: "logout", Resource: "user", ResourceID: "u1"})},
		f.audit.Entries(), "one record, with the owner of the session this call closed")
}

func TestLogout_StoreFailure(t *testing.T) {
	t.Parallel()
	f := newUC(t, withSessions(func(s session.Store) session.Store { return failingSessions{s} }))
	require.ErrorIs(t, f.uc.Logout(identitytest.Anon(), "tok"), errStore)
	require.Empty(t, f.audit.Entries(), "a logout that failed closed no session: nothing to audit")
	require.Empty(t, f.log.Entries(), "the failure goes to the caller: neither a log record nor a refused audit entry")
}

// pwUser отдаёт учётную запись u1 с паролем «old» для смены пароля ровно один раз.
func pwUser(t *testing.T) *mocks.MockUserReader {
	t.Helper()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByID(mock.Anything, "u1").Return(&auth.User{ID: "u1", Email: "a@b.com", PasswordHash: "hash:old", Version: 7}, nil).Once()
	return users
}

// newPassword — хэш нового пароля у владельца учётной записи (hashErr — его отказ) и, при хэше, запись его при версии
// прочитанной учётной записи u1 с ответом writeErr.
func newPassword(t *testing.T, plain string, hashErr, writeErr error) *mocks.MockPasswordSetter {
	t.Helper()
	setter := mocks.NewMockPasswordSetter(t)
	if hashErr != nil {
		setter.EXPECT().HashPassword(mock.Anything, plain).Return("", hashErr).Once()
		return setter
	}
	setter.EXPECT().HashPassword(mock.Anything, plain).Return("hash:"+plain, nil).Once()
	setter.EXPECT().UpdatePassword(mock.Anything, "u1", "hash:"+plain, 7).Return(writeErr).Once()
	return setter
}

// Смена пароля — у вызывающего: ключ — его UserID, старый пароль сверяется с хэшем учётной записи, новый хэширует
// владелец учётной записи и пишет при её версии.
func TestChangePassword(t *testing.T) {
	t.Parallel()

	t.Run("happy path propagates version", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withUsers(pwUser(t)), withGuard(loginGuard(t, "a@b.com", true)), withSetter(newPassword(t, "newpassword123", nil, nil)))

		require.NoError(t, f.uc.ChangePassword(identitytest.Client("u1"), "old", "newpassword123"))
		require.Equal(t, audited(auditkit.Entry{ActorID: "u1", Action: "change_password", Resource: "user", ResourceID: "u1"}),
			f.audit.Last("change_password"))
		require.True(t, f.log.Has(slog.LevelInfo, "password changed"))
	})

	t.Run("concurrent update", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withUsers(pwUser(t)), withGuard(loginGuard(t, "a@b.com", true)),
			withSetter(newPassword(t, "newpassword123", nil, auth.ErrConcurrentUpdate)))

		err := f.uc.ChangePassword(identitytest.Client("u1"), "old", "newpassword123")
		require.ErrorIs(t, err, auth.ErrConcurrentUpdate)
		require.False(t, f.audit.Has("change_password"))
	})

	t.Run("account gone", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByID(mock.Anything, "u1").Return(nil, auth.ErrUserNotFound).Once()
		f := newUC(t, withUsers(users))

		err := f.uc.ChangePassword(identitytest.Client("u1"), "old", "newpassword123")
		require.ErrorIs(t, err, auth.ErrUserNotFound)
	})

	// Неверный старый пароль — попытка в счётчике входа учётной записи, успеха guard не видит; пароль длиннее
	// предела хэшера ни с одним хэшем не совпадает — тоже неверный.
	for _, old := range []string{"wrong", strings.Repeat("я", authnkit.MaxPasswordLen)} {
		t.Run("wrong old", func(t *testing.T) {
			t.Parallel()
			f := newUC(t, withUsers(pwUser(t)), withGuard(loginGuard(t, "a@b.com", false)))

			err := f.uc.ChangePassword(identitytest.Client("u1"), old, "newpassword123")
			require.ErrorIs(t, err, auth.ErrWrongOldPassword)
			require.False(t, f.audit.Has("change_password"))
		})
	}

	// Правило нового пароля — у владельца учётной записи: его отказ доходит ошибкой auth, пароль не пишется.
	for _, refused := range []error{auth.ErrWeakPassword, auth.ErrPasswordTooLong} {
		t.Run(refused.Error(), func(t *testing.T) {
			t.Parallel()
			f := newUC(t, withUsers(pwUser(t)), withGuard(loginGuard(t, "a@b.com", true)), withSetter(newPassword(t, "short", refused, nil)))

			err := f.uc.ChangePassword(identitytest.Client("u1"), "old", "short")
			require.ErrorIs(t, err, refused)
			require.False(t, f.audit.Has("change_password"))
		})
	}

	t.Run("no identity", func(t *testing.T) {
		t.Parallel()
		f := newUC(t)
		err := f.uc.ChangePassword(identitytest.Anon(), "old", "newpassword123")
		var fe *policy.ForbiddenError
		require.ErrorAs(t, err, &fe)
	})
}

// Сбой проверки старого пароля, хэширования нового или поиска учётной записи — ошибка сервиса: пароль не записан,
// аудита нет.
func TestChangePassword_Failure(t *testing.T) {
	t.Parallel()
	t.Run("old password check", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withUsers(pwUser(t)), withGuard(loginGuard(t, "a@b.com", false)), withHasher(&verifyFails{err: errors.New("argon2: queue deadline")}))

		err := f.uc.ChangePassword(identitytest.Client("u1"), "old", "newpassword123")
		require.ErrorContains(t, err, "queue deadline")
		require.NotErrorIs(t, err, auth.ErrWrongOldPassword)
		require.False(t, f.audit.Has("change_password"))
	})
	t.Run("hashing", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withUsers(pwUser(t)), withGuard(loginGuard(t, "a@b.com", true)), withSetter(newPassword(t, "newpassword123", errHashing, nil)))

		err := f.uc.ChangePassword(identitytest.Client("u1"), "old", "newpassword123")
		require.ErrorIs(t, err, errHashing)
		require.False(t, f.audit.Has("change_password"))
	})
	t.Run("user lookup", func(t *testing.T) {
		t.Parallel()
		users := mocks.NewMockUserReader(t)
		users.EXPECT().GetByID(mock.Anything, "u1").Return(nil, errors.New("db down")).Once()
		f := newUC(t, withUsers(users))

		err := f.uc.ChangePassword(identitytest.Client("u1"), "old", "newpassword123")
		require.ErrorContains(t, err, "db down")
		require.NotErrorIs(t, err, auth.ErrUserNotFound)
	})
}

// Полная очередь хэшера на старом пароле — ErrBusy (503 с Retry-After ошибки кита), попытка guard'а возвращается ровно
// раз — и у клиента, оборвавшего запрос; на новом пароле — ErrBusy после проверенного старого, возврата нет.
func TestChangePassword_HasherBusy(t *testing.T) {
	t.Parallel()
	t.Run("old password check", func(t *testing.T) {
		t.Parallel()
		f := newUC(t, withUsers(pwUser(t)), withGuard(busyLoginGuard(t, "a@b.com")), withHasher(&authntest.DeterministicHasher{Busy: true}))

		err := f.uc.ChangePassword(canceled(identitytest.Client("u1")), "old", "newpassword123")
		require.ErrorIs(t, err, auth.ErrBusy)
		require.ErrorIs(t, err, authnkit.ErrBusy, "the kit's error carries the Retry-After of the 503")
		require.False(t, f.audit.Has("change_password"))
	})
	t.Run("new password hashing", func(t *testing.T) {
		t.Parallel()
		busy := fmt.Errorf("%w: %w", auth.ErrBusy, authnkit.ErrBusy)
		f := newUC(t, withUsers(pwUser(t)), withGuard(loginGuard(t, "a@b.com", true)), withSetter(newPassword(t, "newpassword123", busy, nil)))

		err := f.uc.ChangePassword(identitytest.Client("u1"), "old", "newpassword123")
		require.ErrorIs(t, err, auth.ErrBusy)
		require.False(t, f.audit.Has("change_password"))
	})
}

// Несброшенный счётчик guard'а и незаписанный аудит смену пароля не отменяют: причины — в логе.
func TestChangePassword_SideRecordsFail(t *testing.T) {
	t.Parallel()
	g := mocks.NewMockLoginGuard(t)
	g.EXPECT().Attempt(mock.Anything, "a@b.com", identitytest.IP).Return(nil, nil).Once()
	g.EXPECT().RecordSuccess(mock.Anything, "a@b.com", identitytest.IP).Return(errors.New("guard store down")).Once()
	f := newUC(t, withUsers(pwUser(t)), withGuard(g), withSetter(newPassword(t, "newpassword123", nil, nil)),
		withAudit(&audittest.Recorder{LogErr: errAudit}))

	require.NoError(t, f.uc.ChangePassword(identitytest.Client("u1"), "old", "newpassword123"))
	require.True(t, f.log.Has(slog.LevelError, "guard record success"))
	e, logged := f.log.Find(slog.LevelError, "audit change_password")
	require.True(t, logged)
	require.Contains(t, e.Str("error"), errAudit.Error())
}

// Старый пароль угадывают так же, как при входе: под блокировкой и при сбое guard'а он не проверяется — хэшер
// фикстуры с отказом Verify не зовётся.
func TestChangePassword_Guarded(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		lock    *auth.LockState
		err     error
		wantErr error
	}{
		{name: "locked", lock: &auth.LockState{RetryAfter: time.Minute}, wantErr: auth.ErrAccountLocked},
		{name: "guard failure", err: errors.New("guard store down")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := mocks.NewMockLoginGuard(t)
			g.EXPECT().Attempt(mock.Anything, "a@b.com", identitytest.IP).Return(tc.lock, tc.err).Once()
			f := newUC(t, withUsers(pwUser(t)), withGuard(g), withHasher(&verifyFails{err: errors.New("verified under the lock")}))

			err := f.uc.ChangePassword(identitytest.Client("u1"), "old", "newpassword123")
			require.Error(t, err)
			require.NotErrorIs(t, err, auth.ErrWrongOldPassword)
			require.NotContains(t, err.Error(), "verified under the lock")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
			if tc.lock != nil {
				var locked *auth.LockedError
				require.ErrorAs(t, err, &locked)
				require.Equal(t, tc.lock.RetryAfter, locked.RetryAfter(), "retry when the lock ends")
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
	locked.EXPECT().Attempt(mock.Anything, "a@b.com", identitytest.IP).Return(&auth.LockState{RetryAfter: time.Minute}, nil).Once()
	failing := mocks.NewMockLoginGuard(t)
	failing.EXPECT().Attempt(mock.Anything, "a@b.com", identitytest.IP).Return(nil, errors.New("guard store down")).Once()
	unkeyable := mocks.NewMockLoginGuard(t)
	unkeyable.EXPECT().Attempt(mock.Anything, "a@b.com", identitytest.IP).Return(nil, auth.ErrInvalidCredentials).Once()
	tests := []struct {
		name, result, password string
		opts                   []ucOption
	}{
		{"ok", "ok", "pass1234", []ucOption{withUsers(signIn(t, alice())), withGuard(loginGuard(t, "a@b.com", true))}},
		{"wrong password", "invalid_credentials", "wrong", []ucOption{found(true), withGuard(loginGuard(t, "a@b.com", false))}},
		{"attempt the guard cannot count", "invalid_credentials", "pass1234", []ucOption{withGuard(unkeyable)}},
		{"blocked", "blocked", "pass1234", []ucOption{found(false), withGuard(loginGuard(t, "a@b.com", false))}},
		{"locked", "locked", "pass1234", []ucOption{withGuard(locked)}},
		{"guard failure", "error", "pass1234", []ucOption{withGuard(failing)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := metrictest.NewProvider()
			f := newUC(t, append(tc.opts, withMetrics(auth.NewMetrics(p.Meter("test"))))...)

			_, err := f.uc.Login(identitytest.Anon(), auth.LoginInput{Email: "a@b.com", Password: tc.password})
			require.Equal(t, tc.result == "ok", err == nil, "login error: %v", err)
			requireOutcome(t, p, "biz_auth_login_attempts_total", tc.result)
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
			p := metrictest.NewProvider()
			f := newUC(t, append(tc.opts(t), withMetrics(auth.NewMetrics(p.Meter("test"))))...)

			_, err := f.uc.Refresh(identitytest.Anon(), tc.token(t, f))
			require.Equal(t, tc.result == "ok", err == nil, "refresh error: %v", err)
			requireOutcome(t, p, "biz_auth_refresh_total", tc.result)
		})
	}
}

// requireOutcome: вызов записал в счётчик name исход result ровно один раз и других исходов не писал.
func requireOutcome(t *testing.T, p *metrictest.Provider, name, result string) {
	t.Helper()
	snap := p.Collect(t)
	got := snap.Get(t, name, "result", result)
	require.Equal(t, metrictest.Counter, got.Kind)
	require.InDelta(t, 1, got.Value, 0)
	require.InDelta(t, 1, snap.Value(name), 0, "one outcome per call: %v", snap.Find(name))
}
