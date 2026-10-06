package verification_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification/mocks"
	"git.stit.tech/stit-core/go-backend-template/internal/testkit/identitytest"
	"git.stit.tech/stit-core/golibinfra/auditkit"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/testkit/audittest"
	"git.stit.tech/stit-core/golibinfra/testkit/authntest"
	"git.stit.tech/stit-core/golibinfra/testkit/logtest"
	"git.stit.tech/stit-core/golibinfra/testkit/metrictest"
	"git.stit.tech/stit-core/golibinfra/testkit/outboxtest"
	"git.stit.tech/stit-core/golibinfra/testkit/pgtest"
)

var fixedNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

const (
	subjectEmailVerificationRequested = "verification.events.email_verification_requested.v2"
	subjectPasswordResetRequested     = "verification.events.password_reset_requested.v2"
)

type ucFixture struct {
	uc    *verification.Usecase
	bus   *outboxtest.Recorder
	audit *audittest.Recorder
	tx    *pgtest.Recorder
	log   *logtest.Recorder
}

type ucOpts struct {
	repo     *mocks.MockRepository
	users    *mocks.MockUserReader
	pwSet    *mocks.MockPasswordSetter
	emailSet *mocks.MockEmailVerificationSetter
	mailer   *mocks.MockMailer
	limiter  *mocks.MockResetLimiter
	hasher   authnkit.Hasher
	audit    auditkit.Logger
	metrics  *verification.Metrics
}

type ucOption func(*ucOpts)

func withRepo(r *mocks.MockRepository) ucOption  { return func(o *ucOpts) { o.repo = r } }
func withUsers(u *mocks.MockUserReader) ucOption { return func(o *ucOpts) { o.users = u } }

func withPwSet(p *mocks.MockPasswordSetter) ucOption { return func(o *ucOpts) { o.pwSet = p } }

func withEmailSet(e *mocks.MockEmailVerificationSetter) ucOption {
	return func(o *ucOpts) { o.emailSet = e }
}

func withMailer(m *mocks.MockMailer) ucOption { return func(o *ucOpts) { o.mailer = m } }

func withLimiter(l *mocks.MockResetLimiter) ucOption { return func(o *ucOpts) { o.limiter = l } }

func withHasher(h authnkit.Hasher) ucOption { return func(o *ucOpts) { o.hasher = h } }

func withAudit(a auditkit.Logger) ucOption { return func(o *ucOpts) { o.audit = a } }

func withMetrics(m *verification.Metrics) ucOption { return func(o *ucOpts) { o.metrics = m } }

// newUC — usecase на строгих моках портов без ожиданий: вызов, которого кейс не ждёт, — провал; порт с
// ожиданиями кейс передаёт опцией.
func newUC(t *testing.T, opts ...ucOption) ucFixture {
	t.Helper()
	o := &ucOpts{
		repo:     mocks.NewMockRepository(t),
		users:    mocks.NewMockUserReader(t),
		pwSet:    mocks.NewMockPasswordSetter(t),
		emailSet: mocks.NewMockEmailVerificationSetter(t),
		mailer:   mocks.NewMockMailer(t),
		limiter:  mocks.NewMockResetLimiter(t),
		hasher:   authntest.DeterministicHasher{},
	}
	for _, fn := range opts {
		fn(o)
	}

	bus := &outboxtest.Recorder{}
	aud := &audittest.Recorder{}
	tx := &pgtest.Recorder{}
	log := logtest.Capture()
	if o.audit == nil {
		o.audit = aud
	}
	uc := verification.NewUsecase(verification.UsecaseDeps{
		Repo:          o.repo,
		Users:         o.users,
		PasswordSet:   o.pwSet,
		EmailVerifier: o.emailSet,
		Hasher:        o.hasher,
		Tx:            tx,
		Events:        bus,
		Mailer:        o.mailer,
		Limiter:       o.limiter,
		Audit:         o.audit,
		Log:           log,
		Clock:         clockkit.NewFixed(fixedNow),
		Metrics:       o.metrics,
	})
	return ucFixture{uc: uc, bus: bus, audit: aud, tx: tx, log: log}
}

var (
	errHashing = errors.New("argon2: out of memory")
	errDown    = errors.New("db down")
	errAudit   = errors.New("audit_log: connection refused")
)

// brokenHasher — хэшер, у которого сбоит хэширование: у authntest такого фейка нет.
type brokenHasher struct{ authntest.DeterministicHasher }

func (brokenHasher) Hash(context.Context, string) (string, error) { return "", errHashing }

// failingAudit — журнал аудита, запись в который не проходит: у audittest такого фейка нет.
type failingAudit struct{}

func (failingAudit) Log(context.Context, auditkit.Entry) error { return errAudit }

// audited — запись аудита запроса identitytest: пользователь u1 сделал action над своей учётной записью;
// IP клиента и ID запроса auditkit берёт из ctx.
func audited(action string) *auditkit.Entry {
	return &auditkit.Entry{
		ActorID: "u1", Action: action, Resource: "user", ResourceID: "u1", At: fixedNow,
		IP: identitytest.IP, RequestID: identitytest.RequestID,
	}
}

// userByID отдаёт пользователя u1 по ID ровно times раз.
func userByID(t *testing.T, times int) *mocks.MockUserReader {
	t.Helper()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByID(mock.Anything, "u1").Return(&verification.User{ID: "u1", Email: "a@b.com", CanLogin: true, Version: 3}, nil).Times(times)
	return users
}

// userByEmail ждёт один поиск адреса a@b.com и отвечает u или err.
func userByEmail(t *testing.T, u *verification.User, err error) *mocks.MockUserReader {
	t.Helper()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(u, err).Once()
	return users
}

func activeUser(t *testing.T) *mocks.MockUserReader {
	t.Helper()
	return userByEmail(t, &verification.User{ID: "u1", Email: "a@b.com", CanLogin: true}, nil)
}

// limiter ждёт ровно одну проверку лимита сброса пользователя userID с IP запроса и отвечает allowed.
func limiter(t *testing.T, userID string, allowed bool, err error) *mocks.MockResetLimiter {
	t.Helper()
	l := mocks.NewMockResetLimiter(t)
	l.EXPECT().Allow(mock.Anything, userID, identitytest.IP).Return(allowed, err).Once()
	return l
}

func TestIssueEmailVerification_HappyPath(t *testing.T) {
	t.Parallel()
	var storedHash, sentToken string
	repo := mocks.NewMockRepository(t)
	upsert := repo.EXPECT().UpsertEmailVerification(mock.MatchedBy(pgkit.InTx), "u1", mock.Anything, fixedNow.Add(24*time.Hour)).
		RunAndReturn(func(_ context.Context, _, hash string, _ time.Time) (string, bool, error) {
			storedHash = hash
			return "vid", true, nil
		}).Once()
	mailer := mocks.NewMockMailer(t)
	send := mailer.EXPECT().SendEmailVerification(mock.Anything, "a@b.com", mock.Anything).
		RunAndReturn(func(_ context.Context, _, token string) error {
			sentToken = token
			return nil
		}).Once().NotBefore(upsert)
	repo.EXPECT().MarkEmailVerificationSent(mock.Anything, "vid").Return(nil).Once().NotBefore(send)
	f := newUC(t, withRepo(repo), withUsers(userByID(t, 1)), withMailer(mailer))

	require.NoError(t, f.uc.IssueEmailVerification(context.Background(), "u1"))
	require.Equal(t, authnkit.HashToken(sentToken), storedHash, "the mailed token is the one stored by hash")
	require.Equal(t, verification.EmailVerificationRequested{TokenID: "vid", UserID: "u1", At: fixedNow},
		f.bus.Last(subjectEmailVerificationRequested), "TokenID is the id of the stored token, not the token")
	require.Equal(t, 1, f.bus.Count())
}

// Доставка at-least-once: сбой mailer не теряет письмо, повтор после успеха его не дублирует.
func TestIssueEmailVerification_Redelivery(t *testing.T) {
	t.Parallel()
	var hashes, mailed []string
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().UpsertEmailVerification(mock.MatchedBy(pgkit.InTx), "u1", mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _, hash string, _ time.Time) (string, bool, error) {
			hashes = append(hashes, hash)
			return "vid", true, nil
		}).Twice()
	repo.EXPECT().UpsertEmailVerification(mock.Anything, "u1", mock.Anything, mock.Anything).Return("", false, nil).Once()
	repo.EXPECT().MarkEmailVerificationSent(mock.Anything, "vid").Return(nil).Once()
	mailer := mocks.NewMockMailer(t)
	mailer.EXPECT().SendEmailVerification(mock.Anything, "a@b.com", mock.Anything).
		RunAndReturn(func(_ context.Context, _, token string) error {
			mailed = append(mailed, token)
			if len(mailed) == 1 {
				return errors.New("smtp unavailable")
			}
			return nil
		}).Twice()
	f := newUC(t, withRepo(repo), withUsers(userByID(t, 3)), withMailer(mailer))
	ctx := context.Background()

	require.Error(t, f.uc.IssueEmailVerification(ctx, "u1"), "a mailer failure goes back to the broker for redelivery")
	require.NoError(t, f.uc.IssueEmailVerification(ctx, "u1"), "redelivery sends the email")
	require.NoError(t, f.uc.IssueEmailVerification(ctx, "u1"), "redelivery after success is a no-op")

	require.Len(t, mailed, 2)
	require.NotEqual(t, mailed[0], mailed[1], "the resent email carries a new token")
	require.Equal(t, []string{authnkit.HashToken(mailed[0]), authnkit.HashToken(mailed[1])}, hashes)
	require.Len(t, f.bus.Published(subjectEmailVerificationRequested), 2, "one event per issued token")
}

// Пользователь удалён до доставки события: письма нет, повтора доставки тоже.
func TestIssueEmailVerification_UserGone(t *testing.T) {
	t.Parallel()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByID(mock.Anything, "u1").Return(nil, verification.ErrUserNotFound).Once()
	f := newUC(t, withUsers(users))

	require.NoError(t, f.uc.IssueEmailVerification(context.Background(), "u1"))
	require.Zero(t, f.bus.Count())
}

// Сбой до отметки «письмо ушло» — ошибка обработчика: брокер повторит доставку, а не потеряет письмо.
func TestIssueEmailVerification_Failure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		opts   func(t *testing.T) []ucOption
		events int
	}{
		{"user lookup", func(t *testing.T) []ucOption {
			users := mocks.NewMockUserReader(t)
			users.EXPECT().GetByID(mock.Anything, "u1").Return(nil, errDown).Once()
			return []ucOption{withUsers(users)}
		}, 0},
		{"token not stored", func(t *testing.T) []ucOption {
			repo := mocks.NewMockRepository(t)
			repo.EXPECT().UpsertEmailVerification(mock.Anything, "u1", mock.Anything, mock.Anything).Return("", false, errDown).Once()
			return []ucOption{withUsers(userByID(t, 1)), withRepo(repo)}
		}, 0},
		{"sent mark not stored", func(t *testing.T) []ucOption {
			repo := mocks.NewMockRepository(t)
			repo.EXPECT().UpsertEmailVerification(mock.Anything, "u1", mock.Anything, mock.Anything).Return("vid", true, nil).Once()
			repo.EXPECT().MarkEmailVerificationSent(mock.Anything, "vid").Return(errDown).Once()
			mailer := mocks.NewMockMailer(t)
			mailer.EXPECT().SendEmailVerification(mock.Anything, "a@b.com", mock.Anything).Return(nil).Once()
			return []ucOption{withUsers(userByID(t, 1)), withRepo(repo), withMailer(mailer)}
		}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newUC(t, tc.opts(t)...)

			require.ErrorIs(t, f.uc.IssueEmailVerification(context.Background(), "u1"), errDown)
			require.Equal(t, tc.events, f.bus.Count())
		})
	}
}

func TestRequestPasswordReset_HappyPath(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	var capturedHash string
	insert := repo.EXPECT().InsertPasswordReset(mock.MatchedBy(pgkit.InTx), "u1", mock.Anything, fixedNow.Add(time.Hour)).
		RunAndReturn(func(_ context.Context, _, h string, _ time.Time) (string, error) {
			capturedHash = h
			return "rid", nil
		}).Once()
	var sentToken string
	mailer := mocks.NewMockMailer(t)
	mailer.EXPECT().SendPasswordReset(mock.Anything, "a@b.com", mock.Anything).
		RunAndReturn(func(_ context.Context, _, token string) error {
			sentToken = token
			return nil
		}).Once().NotBefore(insert)
	f := newUC(t, withRepo(repo), withUsers(activeUser(t)), withMailer(mailer), withLimiter(limiter(t, "u1", true, nil)))

	require.NoError(t, f.uc.RequestPasswordReset(identitytest.Anon(), "a@b.com"))
	require.Equal(t, authnkit.HashToken(sentToken), capturedHash)
	require.Equal(t, verification.PasswordResetRequested{TokenID: "rid", UserID: "u1", At: fixedNow},
		f.bus.Last(subjectPasswordResetRequested))
	require.Equal(t, 1, f.bus.Count())
}

// Ответ один и тот же, есть ли учётная запись, пускают ли её, исчерпан ли лимит и что сломалось после
// поиска: сбой видно только в логе, исход каждого пути — в метрике (на «не ушло письмо» и «упёрлись в
// лимит» стоят алерты). Токен, событие и письмо — только у разрешённого запроса, сбой лимитера токен не
// выпускает (fail-closed).
func TestRequestPasswordReset_SameAnswer(t *testing.T) {
	t.Parallel()
	issued := func(t *testing.T, insertErr error) *mocks.MockRepository {
		t.Helper()
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().InsertPasswordReset(mock.MatchedBy(pgkit.InTx), "u1", mock.Anything, mock.Anything).Return("rid", insertErr).Once()
		return repo
	}
	mailed := func(t *testing.T, err error) *mocks.MockMailer {
		t.Helper()
		mailer := mocks.NewMockMailer(t)
		mailer.EXPECT().SendPasswordReset(mock.Anything, "a@b.com", mock.Anything).Return(err).Once()
		return mailer
	}
	cases := []struct {
		name   string
		opts   func(t *testing.T) []ucOption
		result string
		events int
		failed string // причина в логе вместо ответа
	}{
		{name: "reset mailed", result: "ok", events: 1, opts: func(t *testing.T) []ucOption {
			return []ucOption{withUsers(activeUser(t)), withLimiter(limiter(t, "u1", true, nil)), withRepo(issued(t, nil)), withMailer(mailed(t, nil))}
		}},
		{name: "unknown email", result: "user_not_found", opts: func(t *testing.T) []ucOption {
			return []ucOption{withUsers(userByEmail(t, nil, verification.ErrUserNotFound))}
		}},
		{name: "account that cannot log in", result: "inactive", opts: func(t *testing.T) []ucOption {
			return []ucOption{withUsers(userByEmail(t, &verification.User{ID: "u1", Email: "a@b.com", CanLogin: false}, nil))}
		}},
		{name: "limit exhausted", result: "throttled", opts: func(t *testing.T) []ucOption {
			return []ucOption{withUsers(activeUser(t)), withLimiter(limiter(t, "u1", false, nil))}
		}},
		{name: "limiter failure", result: "error", failed: "guard store down", opts: func(t *testing.T) []ucOption {
			return []ucOption{withUsers(activeUser(t)), withLimiter(limiter(t, "u1", false, errors.New("guard store down")))}
		}},
		{name: "reset not stored", result: "error", failed: "db down", opts: func(t *testing.T) []ucOption {
			return []ucOption{withUsers(activeUser(t)), withLimiter(limiter(t, "u1", true, nil)), withRepo(issued(t, errDown))}
		}},
		{name: "mail not delivered", result: "error", events: 1, failed: "smtp unavailable", opts: func(t *testing.T) []ucOption {
			return []ucOption{
				withUsers(activeUser(t)), withLimiter(limiter(t, "u1", true, nil)), withRepo(issued(t, nil)),
				withMailer(mailed(t, errors.New("smtp unavailable"))),
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := metrictest.NewProvider()
			f := newUC(t, append(tc.opts(t), withMetrics(verification.NewMetrics(p.Meter("test"))))...)

			require.NoError(t, f.uc.RequestPasswordReset(identitytest.Anon(), "a@b.com"))
			require.Len(t, f.bus.Published(subjectPasswordResetRequested), tc.events)
			require.Equal(t, tc.events, f.bus.Count())
			e, logged := f.log.Find(slog.LevelError, "password reset request")
			require.Equal(t, tc.failed != "", logged, "a failure after the lookup is logged, not answered")
			if logged {
				require.Equal(t, "u1", e.Str("user_id"))
				require.Contains(t, e.Str("err"), tc.failed)
			}
			requireOutcome(t, p, "biz_verification_password_reset_requested_total", tc.result)
		})
	}
}

// Сбой самого поиска учётной записи от адреса не зависит и ничего не выдаёт: он уходит ответом, исход —
// в метрику.
func TestRequestPasswordReset_LookupFailure(t *testing.T) {
	t.Parallel()
	p := metrictest.NewProvider()
	f := newUC(t, withUsers(userByEmail(t, nil, errDown)), withMetrics(verification.NewMetrics(p.Meter("test"))))

	require.ErrorIs(t, f.uc.RequestPasswordReset(identitytest.Anon(), "a@b.com"), errDown)
	require.Zero(t, f.bus.Count())
	requireOutcome(t, p, "biz_verification_password_reset_requested_total", "error")
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

// resetToken — репозиторий, который находит по хэшу токена tok ссылку сброса pr пользователя u1.
func resetToken(t *testing.T, pr *verification.PasswordResetToken, err error) *mocks.MockRepository {
	t.Helper()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPasswordReset(mock.Anything, authnkit.HashToken("tok")).Return(pr, err).Once()
	return repo
}

func pendingReset(expiresAt time.Time) *verification.PasswordResetToken {
	return &verification.PasswordResetToken{ID: "rid", UserID: "u1", ExpiresAt: expiresAt}
}

// Погашение, отзыв прочих ссылок сброса и запись пароля — одной транзакцией, погашение — первым: параллельный
// confirm ждёт его блокировку. Ссылка годна до своего срока включительно: здесь — в последний его миг.
func TestConfirmPasswordReset_HappyPath(t *testing.T) {
	t.Parallel()
	repo := resetToken(t, pendingReset(fixedNow), nil)
	claim := repo.EXPECT().MarkPasswordResetUsed(mock.MatchedBy(pgkit.InTx), "rid").Return(nil).Once()
	repo.EXPECT().RevokePasswordResets(mock.MatchedBy(pgkit.InTx), "u1").Return(nil).Once().NotBefore(claim)
	pwSet := mocks.NewMockPasswordSetter(t)
	pwSet.EXPECT().UpdatePassword(mock.MatchedBy(pgkit.InTx), "u1", "hash:newpassword123", 3).Return(nil).Once().NotBefore(claim)
	f := newUC(t, withRepo(repo), withUsers(userByID(t, 1)), withPwSet(pwSet))

	require.NoError(t, f.uc.ConfirmPasswordReset(identitytest.Anon(), "tok", "newpassword123"))
	require.Equal(t, 1, f.tx.Calls(), "the claim, the revocation of the other links and the password write share one transaction")
	require.Equal(t, audited("password_reset_confirmed"), f.audit.Last("password_reset_confirmed"))
}

// Отказ до транзакции: ни погашения, ни пароля, ни аудита.
func TestConfirmPasswordReset_Rejected(t *testing.T) {
	t.Parallel()
	usedAt := fixedNow.Add(-time.Minute)
	tests := []struct {
		name     string
		password string
		opts     func(t *testing.T) []ucOption
		want     error
	}{
		{"weak password", "short", func(*testing.T) []ucOption { return nil }, verification.ErrWeakPassword},
		{"unknown token", "newpassword123", func(t *testing.T) []ucOption {
			return []ucOption{withRepo(resetToken(t, nil, verification.ErrInvalidResetToken))}
		}, verification.ErrInvalidResetToken},
		{"already used", "newpassword123", func(t *testing.T) []ucOption {
			pr := pendingReset(fixedNow.Add(time.Hour))
			pr.UsedAt = &usedAt
			return []ucOption{withRepo(resetToken(t, pr, nil))}
		}, verification.ErrTokenUsed},
		{"expired", "newpassword123", func(t *testing.T) []ucOption {
			return []ucOption{withRepo(resetToken(t, pendingReset(fixedNow.Add(-time.Nanosecond)), nil))}
		}, verification.ErrInvalidResetToken},
		{"new password not hashed", "newpassword123", func(t *testing.T) []ucOption {
			return []ucOption{withRepo(resetToken(t, pendingReset(fixedNow.Add(time.Hour)), nil)), withHasher(brokenHasher{})}
		}, errHashing},
		{"user deleted after the request", "newpassword123", func(t *testing.T) []ucOption {
			users := mocks.NewMockUserReader(t)
			users.EXPECT().GetByID(mock.Anything, "u1").Return(nil, verification.ErrUserNotFound).Once()
			return []ucOption{withRepo(resetToken(t, pendingReset(fixedNow.Add(time.Hour)), nil)), withUsers(users)}
		}, verification.ErrInvalidResetToken},
		{"user lookup fails", "newpassword123", func(t *testing.T) []ucOption {
			users := mocks.NewMockUserReader(t)
			users.EXPECT().GetByID(mock.Anything, "u1").Return(nil, errDown).Once()
			return []ucOption{withRepo(resetToken(t, pendingReset(fixedNow.Add(time.Hour)), nil)), withUsers(users)}
		}, errDown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newUC(t, tc.opts(t)...)

			require.ErrorIs(t, f.uc.ConfirmPasswordReset(identitytest.Anon(), "tok", tc.password), tc.want)
			require.Zero(t, f.tx.Calls())
			require.Empty(t, f.audit.Entries())
		})
	}
}

// Сбой в транзакции откатывает её целиком: погашение не переживает несменённый пароль, аудита нет.
func TestConfirmPasswordReset_RolledBack(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		expect func(repo *mocks.MockRepository, pwSet *mocks.MockPasswordSetter)
		want   error
	}{
		{"claimed in parallel", func(repo *mocks.MockRepository, _ *mocks.MockPasswordSetter) {
			repo.EXPECT().MarkPasswordResetUsed(mock.Anything, "rid").Return(verification.ErrTokenUsed).Once()
		}, verification.ErrTokenUsed},
		{"other links not revoked", func(repo *mocks.MockRepository, _ *mocks.MockPasswordSetter) {
			repo.EXPECT().MarkPasswordResetUsed(mock.Anything, "rid").Return(nil).Once()
			repo.EXPECT().RevokePasswordResets(mock.Anything, "u1").Return(errDown).Once()
		}, errDown},
		{"password changed concurrently", func(repo *mocks.MockRepository, pwSet *mocks.MockPasswordSetter) {
			repo.EXPECT().MarkPasswordResetUsed(mock.Anything, "rid").Return(nil).Once()
			repo.EXPECT().RevokePasswordResets(mock.Anything, "u1").Return(nil).Once()
			pwSet.EXPECT().UpdatePassword(mock.Anything, "u1", mock.Anything, 3).Return(verification.ErrConcurrentUpdate).Once()
		}, verification.ErrConcurrentUpdate},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := resetToken(t, pendingReset(fixedNow.Add(time.Hour)), nil)
			pwSet := mocks.NewMockPasswordSetter(t)
			tc.expect(repo, pwSet)
			f := newUC(t, withRepo(repo), withUsers(userByID(t, 1)), withPwSet(pwSet))

			require.ErrorIs(t, f.uc.ConfirmPasswordReset(identitytest.Anon(), "tok", "newpassword123"), tc.want)
			require.Equal(t, 1, f.tx.Calls())
			require.Empty(t, f.audit.Entries())
		})
	}
}

// verifyToken — репозиторий, который находит по хэшу токена tok подтверждение email ev пользователя u1.
func verifyToken(t *testing.T, ev *verification.EmailVerificationToken, err error) *mocks.MockRepository {
	t.Helper()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetEmailVerification(mock.Anything, authnkit.HashToken("tok")).Return(ev, err).Once()
	return repo
}

func pendingVerification(expiresAt time.Time) *verification.EmailVerificationToken {
	return &verification.EmailVerificationToken{ID: "vid", UserID: "u1", ExpiresAt: expiresAt}
}

// Погашение и отметка email — одной транзакцией и в этом порядке; ссылка годна до своего срока включительно.
func TestConfirmEmail_HappyPath(t *testing.T) {
	t.Parallel()
	repo := verifyToken(t, pendingVerification(fixedNow), nil)
	claim := repo.EXPECT().MarkEmailVerificationUsed(mock.MatchedBy(pgkit.InTx), "vid").Return(nil).Once()
	emailSet := mocks.NewMockEmailVerificationSetter(t)
	emailSet.EXPECT().MarkEmailVerified(mock.MatchedBy(pgkit.InTx), "u1").Return(nil).Once().NotBefore(claim)
	f := newUC(t, withRepo(repo), withEmailSet(emailSet))

	require.NoError(t, f.uc.ConfirmEmail(identitytest.Anon(), "tok"))
	require.Equal(t, 1, f.tx.Calls(), "the claim and the email mark share one transaction")
	require.Equal(t, audited("email_confirmed"), f.audit.Last("email_confirmed"))
}

// Сбой записи аудита подтверждение не отменяет: email отмечен, причина — в логе.
func TestConfirmEmail_AuditNotWritten(t *testing.T) {
	t.Parallel()
	repo := verifyToken(t, pendingVerification(fixedNow), nil)
	repo.EXPECT().MarkEmailVerificationUsed(mock.MatchedBy(pgkit.InTx), "vid").Return(nil).Once()
	emailSet := mocks.NewMockEmailVerificationSetter(t)
	emailSet.EXPECT().MarkEmailVerified(mock.MatchedBy(pgkit.InTx), "u1").Return(nil).Once()
	f := newUC(t, withRepo(repo), withEmailSet(emailSet), withAudit(failingAudit{}))

	require.NoError(t, f.uc.ConfirmEmail(identitytest.Anon(), "tok"))
	e, logged := f.log.Find(slog.LevelError, "audit email_confirmed")
	require.True(t, logged)
	require.Contains(t, e.Str("err"), errAudit.Error())
}

// Отказ подтверждения: email не отмечен, аудита нет; до транзакции доходит только годный токен.
func TestConfirmEmail_Rejected(t *testing.T) {
	t.Parallel()
	usedAt := fixedNow.Add(-time.Minute)
	tests := []struct {
		name    string
		repo    func(t *testing.T) *mocks.MockRepository
		tx      int
		markErr error // ответ отметки email; nil — до неё не доходит
		want    error
	}{
		{name: "unknown token", want: verification.ErrInvalidVerificationToken, repo: func(t *testing.T) *mocks.MockRepository {
			return verifyToken(t, nil, verification.ErrInvalidVerificationToken)
		}},
		{name: "already used", want: verification.ErrTokenUsed, repo: func(t *testing.T) *mocks.MockRepository {
			ev := pendingVerification(fixedNow.Add(time.Hour))
			ev.UsedAt = &usedAt
			return verifyToken(t, ev, nil)
		}},
		{name: "expired", want: verification.ErrInvalidVerificationToken, repo: func(t *testing.T) *mocks.MockRepository {
			return verifyToken(t, pendingVerification(fixedNow.Add(-time.Nanosecond)), nil)
		}},
		{name: "claimed in parallel", tx: 1, want: verification.ErrTokenUsed, repo: func(t *testing.T) *mocks.MockRepository {
			repo := verifyToken(t, pendingVerification(fixedNow.Add(time.Hour)), nil)
			repo.EXPECT().MarkEmailVerificationUsed(mock.Anything, "vid").Return(verification.ErrTokenUsed).Once()
			return repo
		}},
		{
			name: "user deleted after the issue", tx: 1, markErr: verification.ErrUserNotFound, want: verification.ErrInvalidVerificationToken,
			repo: func(t *testing.T) *mocks.MockRepository {
				repo := verifyToken(t, pendingVerification(fixedNow.Add(time.Hour)), nil)
				repo.EXPECT().MarkEmailVerificationUsed(mock.Anything, "vid").Return(nil).Once()
				return repo
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			emailSet := mocks.NewMockEmailVerificationSetter(t)
			if tc.markErr != nil {
				emailSet.EXPECT().MarkEmailVerified(mock.Anything, "u1").Return(tc.markErr).Once()
			}
			f := newUC(t, withRepo(tc.repo(t)), withEmailSet(emailSet))

			require.ErrorIs(t, f.uc.ConfirmEmail(identitytest.Anon(), "tok"), tc.want)
			require.Equal(t, tc.tx, f.tx.Calls())
			require.Empty(t, f.audit.Entries())
		})
	}
}
