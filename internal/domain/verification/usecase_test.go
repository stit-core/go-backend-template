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
	"git.stit.tech/stit-core/go-backend-template/pkg/tokens"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/testkit/audittest"
	"git.stit.tech/stit-core/golibinfra/testkit/authntest"
	"git.stit.tech/stit-core/golibinfra/testkit/logtest"
	"git.stit.tech/stit-core/golibinfra/testkit/outboxtest"
	"git.stit.tech/stit-core/golibinfra/testkit/pgtest"
)

var fixedNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

const (
	subjectEmailVerificationRequested = "verification.events.email_verification_requested"
	subjectPasswordResetRequested     = "verification.events.password_reset_requested"
)

type ucFixture struct {
	uc       *verification.Usecase
	repo     *mocks.MockRepository
	users    *mocks.MockUserReader
	pwSet    *mocks.MockPasswordSetter
	emailSet *mocks.MockEmailVerificationSetter
	mailer   *mocks.MockMailer
	limiter  *mocks.MockResetLimiter
	bus      *outboxtest.Recorder
	audit    *audittest.Recorder
	tx       *pgtest.Recorder
	log      *logtest.Recorder
}

type ucOpts struct {
	repo     *mocks.MockRepository
	users    *mocks.MockUserReader
	pwSet    *mocks.MockPasswordSetter
	emailSet *mocks.MockEmailVerificationSetter
	mailer   *mocks.MockMailer
	limiter  *mocks.MockResetLimiter
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

// limiter ждёт ровно одну проверку лимита сброса пользователя userID и отвечает allowed.
func limiter(t *testing.T, userID string, allowed bool, err error) *mocks.MockResetLimiter {
	t.Helper()
	l := mocks.NewMockResetLimiter(t)
	l.EXPECT().Allow(mock.Anything, userID, "").Return(allowed, err).Once()
	return l
}

func newUC(t *testing.T, opts ...ucOption) ucFixture {
	t.Helper()
	o := &ucOpts{}
	for _, fn := range opts {
		fn(o)
	}
	if o.repo == nil {
		r := mocks.NewMockRepository(t)
		r.EXPECT().InsertPasswordReset(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("rid", nil).Maybe()
		r.EXPECT().GetPasswordReset(mock.Anything, mock.Anything).Return(nil, verification.ErrInvalidResetToken).Maybe()
		r.EXPECT().MarkPasswordResetUsed(mock.Anything, mock.Anything).Return(nil).Maybe()
		r.EXPECT().UpsertEmailVerification(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("vid", true, nil).Maybe()
		r.EXPECT().MarkEmailVerificationSent(mock.Anything, mock.Anything).Return(nil).Maybe()
		r.EXPECT().GetEmailVerification(mock.Anything, mock.Anything).Return(nil, verification.ErrInvalidVerificationToken).Maybe()
		r.EXPECT().MarkEmailVerificationUsed(mock.Anything, mock.Anything).Return(nil).Maybe()
		o.repo = r
	}
	if o.users == nil {
		u := mocks.NewMockUserReader(t)
		u.EXPECT().GetByID(mock.Anything, mock.Anything).Return(nil, verification.ErrUserNotFound).Maybe()
		u.EXPECT().GetByEmail(mock.Anything, mock.Anything).Return(nil, verification.ErrUserNotFound).Maybe()
		o.users = u
	}
	if o.pwSet == nil {
		p := mocks.NewMockPasswordSetter(t)
		p.EXPECT().UpdatePassword(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
		o.pwSet = p
	}
	if o.emailSet == nil {
		e := mocks.NewMockEmailVerificationSetter(t)
		e.EXPECT().MarkEmailVerified(mock.Anything, mock.Anything).Return(nil).Maybe()
		o.emailSet = e
	}
	if o.mailer == nil {
		m := mocks.NewMockMailer(t)
		m.EXPECT().SendPasswordReset(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
		m.EXPECT().SendEmailVerification(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
		o.mailer = m
	}
	if o.limiter == nil {
		o.limiter = mocks.NewMockResetLimiter(t)
	}

	bus := &outboxtest.Recorder{}
	aud := &audittest.Recorder{}
	tx := &pgtest.Recorder{}
	log := logtest.Capture()
	uc := verification.NewUsecase(verification.UsecaseDeps{
		Repo:          o.repo,
		Users:         o.users,
		PasswordSet:   o.pwSet,
		EmailVerifier: o.emailSet,
		Hasher:        authntest.DeterministicHasher{},
		Tx:            tx,
		Events:        bus,
		Mailer:        o.mailer,
		Limiter:       o.limiter,
		Audit:         aud,
		Log:           log,
		Clock:         clockkit.NewFixed(fixedNow),
	})
	return ucFixture{
		uc: uc, repo: o.repo, users: o.users, pwSet: o.pwSet, emailSet: o.emailSet, mailer: o.mailer, limiter: o.limiter,
		bus: bus, audit: aud, tx: tx, log: log,
	}
}

func TestIssueEmailVerification_HappyPath(t *testing.T) {
	t.Parallel()
	var storedHash, sentToken string
	repo := mocks.NewMockRepository(t)
	upsert := repo.EXPECT().UpsertEmailVerification(mock.Anything, "u1", mock.Anything, fixedNow.Add(24*time.Hour)).
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
	f := newUC(t, withRepo(repo), withMailer(mailer))

	require.NoError(t, f.uc.IssueEmailVerification(context.Background(), "u1", "a@b.com"))
	require.Equal(t, tokens.Hash(sentToken), storedHash, "the mailed token is the one stored by hash")
	require.Equal(t, verification.EmailVerificationRequested{TokenID: "vid", UserID: "u1", Email: "a@b.com", At: fixedNow},
		f.bus.Last(subjectEmailVerificationRequested), "TokenID is the id of the stored token, not the token")
}

// Доставка at-least-once: сбой mailer не теряет письмо, повтор после успеха его не дублирует.
func TestIssueEmailVerification_Redelivery(t *testing.T) {
	t.Parallel()
	var hashes, mailed []string
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().UpsertEmailVerification(mock.Anything, "u1", mock.Anything, mock.Anything).
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
	f := newUC(t, withRepo(repo), withMailer(mailer))
	ctx := context.Background()

	require.Error(t, f.uc.IssueEmailVerification(ctx, "u1", "a@b.com"), "a mailer failure goes back to the broker for redelivery")
	require.NoError(t, f.uc.IssueEmailVerification(ctx, "u1", "a@b.com"), "redelivery sends the email")
	require.NoError(t, f.uc.IssueEmailVerification(ctx, "u1", "a@b.com"), "redelivery after success is a no-op")

	require.Len(t, mailed, 2)
	require.NotEqual(t, mailed[0], mailed[1], "the resent email carries a new token")
	require.Equal(t, []string{tokens.Hash(mailed[0]), tokens.Hash(mailed[1])}, hashes)
	require.Len(t, f.bus.Published(subjectEmailVerificationRequested), 2, "one event per issued token")
}

func activeUser(t *testing.T) *mocks.MockUserReader {
	t.Helper()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(
		&verification.User{ID: "u1", Email: "a@b.com", CanLogin: true}, nil,
	).Once()
	return users
}

func TestRequestPasswordReset_HappyPath(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	var capturedHash string
	repo.EXPECT().InsertPasswordReset(mock.MatchedBy(pgkit.InTx), "u1", mock.Anything, fixedNow.Add(time.Hour)).
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
		}).Once()
	f := newUC(t, withRepo(repo), withUsers(activeUser(t)), withMailer(mailer), withLimiter(limiter(t, "u1", true, nil)))

	require.NoError(t, f.uc.RequestPasswordReset(context.Background(), "a@b.com"))
	require.Equal(t, tokens.Hash(sentToken), capturedHash)
	require.Equal(t, verification.PasswordResetRequested{TokenID: "rid", UserID: "u1", Email: "a@b.com", At: fixedNow},
		f.bus.Last(subjectPasswordResetRequested))
}

// Ответ один и тот же, есть ли учётная запись, пускают ли её, исчерпан ли лимит и ушло ли письмо:
// строгие моки — токен, событие и письмо появляются только у разрешённого запроса.
func TestRequestPasswordReset_SameAnswer(t *testing.T) {
	t.Parallel()
	blocked := mocks.NewMockUserReader(t)
	blocked.EXPECT().GetByEmail(mock.Anything, "a@b.com").Return(&verification.User{ID: "u1", CanLogin: false}, nil).Once()
	failingMailer := mocks.NewMockMailer(t)
	failingMailer.EXPECT().SendPasswordReset(mock.Anything, "a@b.com", mock.Anything).Return(errors.New("smtp unavailable")).Once()
	issuing := mocks.NewMockRepository(t)
	issuing.EXPECT().InsertPasswordReset(mock.Anything, "u1", mock.Anything, mock.Anything).Return("rid", nil).Once()

	cases := []struct {
		name   string
		opts   []ucOption
		events int
		logged string // ошибка в логе вместо ответа
	}{
		{name: "unknown email", opts: []ucOption{withRepo(mocks.NewMockRepository(t)), withMailer(mocks.NewMockMailer(t))}},
		{name: "account that cannot log in", opts: []ucOption{withUsers(blocked), withRepo(mocks.NewMockRepository(t)), withMailer(mocks.NewMockMailer(t))}},
		{name: "limit exhausted", opts: []ucOption{
			withUsers(activeUser(t)), withLimiter(limiter(t, "u1", false, nil)),
			withRepo(mocks.NewMockRepository(t)), withMailer(mocks.NewMockMailer(t)),
		}},
		{name: "mail not delivered", events: 1, logged: "send password reset email", opts: []ucOption{
			withUsers(activeUser(t)), withLimiter(limiter(t, "u1", true, nil)), withRepo(issuing), withMailer(failingMailer),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newUC(t, tc.opts...)

			require.NoError(t, f.uc.RequestPasswordReset(context.Background(), "a@b.com"))
			require.Len(t, f.bus.Published(subjectPasswordResetRequested), tc.events)
			if tc.logged != "" {
				e, ok := f.log.Find(slog.LevelError, tc.logged)
				require.True(t, ok, "the delivery failure is logged")
				require.Equal(t, "u1", e.Str("user_id"))
			}
		})
	}
}

// Fail-closed: без учёта запроса токен не выпускается.
func TestRequestPasswordReset_LimiterFailure(t *testing.T) {
	t.Parallel()
	f := newUC(t, withUsers(activeUser(t)), withLimiter(limiter(t, "u1", false, errors.New("guard store down"))),
		withRepo(mocks.NewMockRepository(t)), withMailer(mocks.NewMockMailer(t)))

	require.ErrorContains(t, f.uc.RequestPasswordReset(context.Background(), "a@b.com"), "guard store down")
	require.Empty(t, f.bus.Published(subjectPasswordResetRequested))
}

func TestConfirmPasswordReset_HappyPath(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPasswordReset(mock.Anything, tokens.Hash("tok")).Return(
		&verification.PasswordResetToken{ID: "rid", UserID: "u1", ExpiresAt: fixedNow.Add(time.Hour)}, nil,
	)
	claim := repo.EXPECT().MarkPasswordResetUsed(mock.MatchedBy(pgkit.InTx), "rid").Return(nil).Once()
	revoke := repo.EXPECT().RevokePasswordResets(mock.MatchedBy(pgkit.InTx), "u1").Return(nil).Once().NotBefore(claim)
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByID(mock.Anything, "u1").Return(
		&verification.User{ID: "u1", Email: "a@b.com", CanLogin: true, Version: 3}, nil,
	)
	pwSet := mocks.NewMockPasswordSetter(t)
	pwSet.EXPECT().UpdatePassword(mock.MatchedBy(pgkit.InTx), "u1", "hash:newpassword123", 3).Return(nil).Once().NotBefore(revoke)
	f := newUC(t, withRepo(repo), withUsers(users), withPwSet(pwSet))

	require.NoError(t, f.uc.ConfirmPasswordReset(context.Background(), "tok", "newpassword123"))
	require.Equal(t, 1, f.tx.Calls(), "the claim, the revocation of the other links and the password write share one transaction")
	require.True(t, f.audit.Has("password_reset_confirmed"))
}

func TestConfirmPasswordReset_AlreadyUsed(t *testing.T) {
	t.Parallel()
	usedAt := fixedNow.Add(-time.Minute)
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPasswordReset(mock.Anything, mock.Anything).Return(
		&verification.PasswordResetToken{ID: "rid", UserID: "u1", ExpiresAt: fixedNow.Add(time.Hour), UsedAt: &usedAt}, nil,
	).Once()
	f := newUC(t, withRepo(repo), withPwSet(mocks.NewMockPasswordSetter(t)))

	require.ErrorIs(t, f.uc.ConfirmPasswordReset(context.Background(), "tok", "newpassword123"), verification.ErrTokenUsed)
}

// Параллельный confirm погасил токен первым: условное погашение — 0 строк, пароль не меняется.
func TestConfirmPasswordReset_ClaimedInParallel(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPasswordReset(mock.Anything, mock.Anything).Return(
		&verification.PasswordResetToken{ID: "rid", UserID: "u1", ExpiresAt: fixedNow.Add(time.Hour)}, nil,
	).Once()
	repo.EXPECT().MarkPasswordResetUsed(mock.Anything, "rid").Return(verification.ErrTokenUsed).Once()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByID(mock.Anything, "u1").Return(&verification.User{ID: "u1", Version: 3}, nil).Once()
	f := newUC(t, withRepo(repo), withUsers(users), withPwSet(mocks.NewMockPasswordSetter(t)))

	require.ErrorIs(t, f.uc.ConfirmPasswordReset(context.Background(), "tok", "newpassword123"), verification.ErrTokenUsed)
	require.False(t, f.audit.Has("password_reset_confirmed"))
}

// Токен пользователя, удалённого после запроса сброса, недействителен.
func TestConfirmPasswordReset_UserGone(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPasswordReset(mock.Anything, mock.Anything).Return(
		&verification.PasswordResetToken{ID: "rid", UserID: "u1", ExpiresAt: fixedNow.Add(time.Hour)}, nil,
	).Once()
	f := newUC(t, withRepo(repo), withPwSet(mocks.NewMockPasswordSetter(t)))

	require.ErrorIs(t, f.uc.ConfirmPasswordReset(context.Background(), "tok", "newpassword123"), verification.ErrInvalidResetToken)
}

// Пароль сменили между чтением пользователя и записью: погашение — в той же транзакции, что запись
// пароля, и откатывается вместе с ней; токен годен для повтора.
func TestConfirmPasswordReset_ConcurrentUpdate(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPasswordReset(mock.Anything, mock.Anything).Return(
		&verification.PasswordResetToken{ID: "rid", UserID: "u1", ExpiresAt: fixedNow.Add(time.Hour)}, nil,
	).Once()
	repo.EXPECT().MarkPasswordResetUsed(mock.MatchedBy(pgkit.InTx), "rid").Return(nil).Once()
	repo.EXPECT().RevokePasswordResets(mock.MatchedBy(pgkit.InTx), "u1").Return(nil).Once()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByID(mock.Anything, "u1").Return(&verification.User{ID: "u1", Version: 3}, nil).Once()
	pwSet := mocks.NewMockPasswordSetter(t)
	pwSet.EXPECT().UpdatePassword(mock.MatchedBy(pgkit.InTx), "u1", mock.Anything, 3).Return(verification.ErrConcurrentUpdate).Once()
	f := newUC(t, withRepo(repo), withUsers(users), withPwSet(pwSet))

	require.ErrorIs(t, f.uc.ConfirmPasswordReset(context.Background(), "tok", "newpassword123"), verification.ErrConcurrentUpdate)
	require.Equal(t, 1, f.tx.Calls(), "the claim and the password write share one transaction")
	require.False(t, f.audit.Has("password_reset_confirmed"))
}

func TestConfirmPasswordReset_WeakPassword(t *testing.T) {
	t.Parallel()
	f := newUC(t)
	err := f.uc.ConfirmPasswordReset(context.Background(), "tok", "short")
	require.ErrorIs(t, err, verification.ErrWeakPassword)
}

// Истёкший токен не доходит ни до пользователя, ни до пароля: строгие моки без ожиданий.
func TestConfirmPasswordReset_Expired(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPasswordReset(mock.Anything, mock.Anything).Return(
		&verification.PasswordResetToken{ID: "rid", UserID: "u1", ExpiresAt: fixedNow.Add(-time.Hour)}, nil,
	).Once()
	f := newUC(t, withRepo(repo), withUsers(mocks.NewMockUserReader(t)), withPwSet(mocks.NewMockPasswordSetter(t)))

	err := f.uc.ConfirmPasswordReset(context.Background(), "tok", "newpassword123")
	require.ErrorIs(t, err, verification.ErrInvalidResetToken)
}

func TestConfirmEmail_HappyPath(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetEmailVerification(mock.Anything, tokens.Hash("tok")).Return(
		&verification.EmailVerificationToken{ID: "vid", UserID: "u1", ExpiresAt: fixedNow.Add(time.Hour)}, nil,
	)
	claim := repo.EXPECT().MarkEmailVerificationUsed(mock.MatchedBy(pgkit.InTx), "vid").Return(nil).Once()
	emailSet := mocks.NewMockEmailVerificationSetter(t)
	emailSet.EXPECT().MarkEmailVerified(mock.MatchedBy(pgkit.InTx), "u1").Return(nil).Once().NotBefore(claim)
	f := newUC(t, withRepo(repo), withEmailSet(emailSet))

	require.NoError(t, f.uc.ConfirmEmail(context.Background(), "tok"))
	require.Equal(t, 1, f.tx.Calls(), "the claim and the email mark share one transaction")
	require.True(t, f.audit.Has("email_confirmed"))
}

func TestConfirmEmail_AlreadyUsed(t *testing.T) {
	t.Parallel()
	usedAt := fixedNow.Add(-time.Minute)
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetEmailVerification(mock.Anything, mock.Anything).Return(
		&verification.EmailVerificationToken{ID: "vid", UserID: "u1", ExpiresAt: fixedNow.Add(time.Hour), UsedAt: &usedAt}, nil,
	).Once()
	f := newUC(t, withRepo(repo), withEmailSet(mocks.NewMockEmailVerificationSetter(t)))

	require.ErrorIs(t, f.uc.ConfirmEmail(context.Background(), "tok"), verification.ErrTokenUsed)
}

// Параллельный confirm погасил токен первым: условное погашение — 0 строк, email не отмечается.
func TestConfirmEmail_ClaimedInParallel(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetEmailVerification(mock.Anything, mock.Anything).Return(
		&verification.EmailVerificationToken{ID: "vid", UserID: "u1", ExpiresAt: fixedNow.Add(time.Hour)}, nil,
	).Once()
	repo.EXPECT().MarkEmailVerificationUsed(mock.Anything, "vid").Return(verification.ErrTokenUsed).Once()
	f := newUC(t, withRepo(repo), withEmailSet(mocks.NewMockEmailVerificationSetter(t)))

	require.ErrorIs(t, f.uc.ConfirmEmail(context.Background(), "tok"), verification.ErrTokenUsed)
	require.False(t, f.audit.Has("email_confirmed"))
}

// Токен пользователя, удалённого после выпуска, недействителен.
func TestConfirmEmail_UserGone(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetEmailVerification(mock.Anything, mock.Anything).Return(
		&verification.EmailVerificationToken{ID: "vid", UserID: "u1", ExpiresAt: fixedNow.Add(time.Hour)}, nil,
	).Once()
	repo.EXPECT().MarkEmailVerificationUsed(mock.Anything, "vid").Return(nil).Once()
	emailSet := mocks.NewMockEmailVerificationSetter(t)
	emailSet.EXPECT().MarkEmailVerified(mock.Anything, "u1").Return(verification.ErrUserNotFound).Once()
	f := newUC(t, withRepo(repo), withEmailSet(emailSet))

	require.ErrorIs(t, f.uc.ConfirmEmail(context.Background(), "tok"), verification.ErrInvalidVerificationToken)
	require.False(t, f.audit.Has("email_confirmed"))
}

// Истёкший токен не доходит до отметки email: строгие моки без ожиданий.
func TestConfirmEmail_Expired(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetEmailVerification(mock.Anything, mock.Anything).Return(
		&verification.EmailVerificationToken{ID: "vid", UserID: "u1", ExpiresAt: fixedNow.Add(-time.Hour)}, nil,
	).Once()
	f := newUC(t, withRepo(repo), withEmailSet(mocks.NewMockEmailVerificationSetter(t)))

	err := f.uc.ConfirmEmail(context.Background(), "tok")
	require.ErrorIs(t, err, verification.ErrInvalidVerificationToken)
}
