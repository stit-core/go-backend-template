package verification_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	"git.stit.tech/stit-core/golibinfra/idkit"
	"git.stit.tech/stit-core/golibinfra/outboxkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
	"git.stit.tech/stit-core/golibinfra/testkit/audittest"
	"git.stit.tech/stit-core/golibinfra/testkit/logtest"
	"git.stit.tech/stit-core/golibinfra/testkit/metrictest"
	"git.stit.tech/stit-core/golibinfra/testkit/outboxtest"
	"git.stit.tech/stit-core/golibinfra/testkit/pgtest"
)

var fixedNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// mailPause — пауза писем сброса usecase тестов.
const mailPause = 5 * time.Minute

const (
	subjectEmailVerificationRequested = "verification.events.email_verification_requested.v2"
	subjectPasswordResetRequested     = "verification.events.password_reset_requested.v3"
)

type ucFixture struct {
	uc    *verification.Usecase
	bus   *outboxtest.Recorder
	audit *audittest.Recorder
	tx    *pgtest.Recorder
	log   *logtest.Recorder
}

type ucOpts struct {
	events   outboxkit.Publisher
	repo     *mocks.MockRepository
	users    *mocks.MockUserReader
	pwSet    *mocks.MockPasswordSetter
	emailSet *mocks.MockEmailVerificationSetter
	mailer   *mocks.MockMailer
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

func withAudit(a auditkit.Logger) ucOption { return func(o *ucOpts) { o.audit = a } }

func withMetrics(m *verification.Metrics) ucOption { return func(o *ucOpts) { o.metrics = m } }

func withEvents(e outboxkit.Publisher) ucOption { return func(o *ucOpts) { o.events = e } }

// failingEvents — outbox, запись в который сбоит.
type failingEvents struct{}

func (failingEvents) Publish(context.Context, string, any) error { return errDown }

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
	if o.events == nil {
		o.events = bus
	}
	uc := verification.NewUsecase(verification.UsecaseDeps{
		Repo:          o.repo,
		Users:         o.users,
		PasswordSet:   o.pwSet,
		EmailVerifier: o.emailSet,
		Tx:            tx,
		Events:        o.events,
		Mailer:        o.mailer,
		Config:        verification.Config{ResetMailPause: mailPause},
		Audit:         o.audit,
		Log:           log,
		Clock:         clockkit.NewFixed(fixedNow),
		Metrics:       o.metrics,
	})
	return ucFixture{uc: uc, bus: bus, audit: aud, tx: tx, log: log}
}

// event — последнее событие subject в своём типе: outbox хранит JSON.
func event[T any](t *testing.T, f ucFixture, subject string) T {
	t.Helper()
	var e T
	require.NoError(t, json.Unmarshal(f.bus.Last(subject), &e))
	return e
}

var (
	errHashing = errors.New("argon2: out of memory")
	errDown    = errors.New("db down")
	errAudit   = errors.New("audit_log: connection refused")
)

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
		event[verification.EmailVerificationRequested](t, f, subjectEmailVerificationRequested), "TokenID is the id of the stored token, not the token")
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

// Повтор события после погашения ссылки: ожидающей записи уже нет, ключ — состояние пользователя, и
// подтверждённому адресу второе письмо не уходит (outbox переопубликовал строку, durable пересоздан).
func TestIssueEmailVerification_ReplayAfterConfirmation(t *testing.T) {
	t.Parallel()
	users := mocks.NewMockUserReader(t)
	users.EXPECT().GetByID(mock.Anything, "u1").Return(&verification.User{ID: "u1", Email: "a@b.com", CanLogin: true, EmailVerified: true}, nil).Once()
	f := newUC(t, withUsers(users))

	require.NoError(t, f.uc.IssueEmailVerification(context.Background(), "u1"))
	require.Zero(t, f.bus.Count(), "no token, no event, no mail for a verified address")
	require.Zero(t, f.tx.Calls())
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

// requestedEvent — событие запроса сброса глазами потребителя: в payload только ID запроса, ID учётной
// записи и время.
type requestedEvent struct {
	RequestID string
	UserID    string
	At        time.Time
}

func requested(t *testing.T, f ucFixture) requestedEvent {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(f.bus.Last(subjectPasswordResetRequested)))
	dec.DisallowUnknownFields()
	var e requestedEvent
	require.NoError(t, dec.Decode(&e))
	return e
}

// Запрос сброса делает одну и ту же работу для любого адреса — поиск и событие в транзакции — и письма не шлёт: его
// шлёт подписчик события. Ни ответ, ни его время не выдают учётную запись; исход — в метрику.
func TestRequestPasswordReset_SameWorkForAnyAddress(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		found   *verification.User
		lookErr error
		userID  string
		result  string
	}{
		{"account that can log in", &verification.User{ID: "u1", Email: "a@b.com", CanLogin: true}, nil, "u1", "ok"},
		{"account that cannot log in", &verification.User{ID: "u1", Email: "a@b.com"}, nil, "u1", "inactive"},
		{"unknown address", nil, verification.ErrUserNotFound, "", "user_not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := metrictest.NewProvider()
			f := newUC(t, withUsers(userByEmail(t, tc.found, tc.lookErr)), withMetrics(verification.NewMetrics(p.Meter("test"))))

			require.NoError(t, f.uc.RequestPasswordReset(identitytest.Anon(), "a@b.com"))
			require.Equal(t, 1, f.tx.Calls())
			require.Equal(t, 1, f.bus.Count())
			e := requested(t, f)
			require.True(t, idkit.IsV7(e.RequestID), "the request id %q is a new UUIDv7", e.RequestID)
			require.Equal(t, requestedEvent{RequestID: e.RequestID, UserID: tc.userID, At: fixedNow}, e)
			requireOutcome(t, p, "biz_verification_password_reset_requested_total", tc.result)
		})
	}
}

// Сбой поиска или записи события уходит ответом: работа запроса одна для любого адреса, и сбой учётную запись не
// выдаёт.
func TestRequestPasswordReset_Failure(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opts func(t *testing.T) []ucOption
		want error
	}{
		{"lookup", func(t *testing.T) []ucOption { return []ucOption{withUsers(userByEmail(t, nil, errDown))} }, errDown},
		{"event not stored", func(t *testing.T) []ucOption {
			return []ucOption{withUsers(activeUser(t)), withEvents(failingEvents{})}
		}, errDown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := metrictest.NewProvider()
			f := newUC(t, append(tc.opts(t), withMetrics(verification.NewMetrics(p.Meter("test"))))...)

			require.ErrorIs(t, f.uc.RequestPasswordReset(identitytest.Anon(), "a@b.com"), tc.want)
			require.Zero(t, f.bus.Count())
			requireOutcome(t, p, "biz_verification_password_reset_requested_total", "error")
		})
	}
}

// resetIssued ждёт выпуск ссылки сброса запроса rid пользователя u1 версии 3, сделанного в requestedAt, сроком час
// от него, с паузой писем конфига — в транзакции после очереди выпусков пользователя, — и копит хэши выпущенных
// токенов; issued — ответы по порядку вызовов.
func resetIssued(t *testing.T, requestedAt time.Time, hashes *[]string, issued ...bool) *mocks.MockRepository {
	t.Helper()
	repo := mocks.NewMockRepository(t)
	for _, ok := range issued {
		lock := repo.EXPECT().LockPasswordResets(mock.MatchedBy(pgkit.InTx), "u1").Return(nil).Once()
		repo.EXPECT().UpsertPasswordReset(mock.MatchedBy(pgkit.InTx), mock.MatchedBy(func(r verification.PasswordResetIssue) bool {
			return r.RequestID == "rid" && r.UserID == "u1" && r.UserVersion == 3 &&
				r.RequestedAt.Equal(requestedAt) && r.ExpiresAt.Equal(requestedAt.Add(time.Hour))
		}), mailPause).RunAndReturn(func(_ context.Context, r verification.PasswordResetIssue, _ time.Duration) (bool, error) {
			*hashes = append(*hashes, r.TokenHash)
			return ok, nil
		}).Once().NotBefore(lock)
	}
	return repo
}

// Письмо сброса — подписчиком события запроса: токен выпускает он, в письме — тот, чей хэш сохранён, отметка
// sent_at — после письма. Ссылка годна до своего срока включительно: здесь запрос ровно час назад.
func TestIssuePasswordReset_HappyPath(t *testing.T) {
	t.Parallel()
	var hashes []string
	requestedAt := fixedNow.Add(-time.Hour)
	repo := resetIssued(t, requestedAt, &hashes, true)
	var sentToken string
	mailer := mocks.NewMockMailer(t)
	send := mailer.EXPECT().SendPasswordReset(mock.Anything, "a@b.com", mock.Anything).
		RunAndReturn(func(_ context.Context, _, token string) error {
			sentToken = token
			return nil
		}).Once()
	repo.EXPECT().MarkPasswordResetSent(mock.Anything, "rid").Return(nil).Once().NotBefore(send)
	f := newUC(t, withRepo(repo), withUsers(userByID(t, 1)), withMailer(mailer))

	require.NoError(t, f.uc.IssuePasswordReset(context.Background(), "rid", "u1", requestedAt))
	require.Equal(t, []string{authnkit.HashToken(sentToken)}, hashes, "the mailed token is the one stored by hash")
	require.Zero(t, f.bus.Count(), "the handler of its own event publishes nothing: no loop through the outbox")
}

// Доставка at-least-once: сбой mailer не теряет письмо — та же ссылка с новым токеном, — а повтор после
// успеха его не дублирует.
func TestIssuePasswordReset_Redelivery(t *testing.T) {
	t.Parallel()
	var hashes, mailed []string
	repo := resetIssued(t, fixedNow, &hashes, true, true, false)
	repo.EXPECT().MarkPasswordResetSent(mock.Anything, "rid").Return(nil).Once()
	mailer := mocks.NewMockMailer(t)
	mailer.EXPECT().SendPasswordReset(mock.Anything, "a@b.com", mock.Anything).
		RunAndReturn(func(_ context.Context, _, token string) error {
			mailed = append(mailed, token)
			if len(mailed) == 1 {
				return errors.New("smtp unavailable")
			}
			return nil
		}).Twice()
	f := newUC(t, withRepo(repo), withUsers(userByID(t, 3)), withMailer(mailer))
	ctx := context.Background()

	require.Error(t, f.uc.IssuePasswordReset(ctx, "rid", "u1", fixedNow), "a mailer failure goes back to the broker for redelivery")
	require.NoError(t, f.uc.IssuePasswordReset(ctx, "rid", "u1", fixedNow), "redelivery sends the email")
	require.NoError(t, f.uc.IssuePasswordReset(ctx, "rid", "u1", fixedNow), "redelivery after success is a no-op")

	require.Len(t, mailed, 2)
	require.NotEqual(t, mailed[0], mailed[1], "the resent email carries a new token")
	require.Equal(t, []string{authnkit.HashToken(mailed[0]), authnkit.HashToken(mailed[1])}, hashes[:2])
}

// Срок ссылки — от времени запроса, но не позже обработки: событие с At из будущего (часы публикатора спешат или
// событие записано мимо outbox) ссылку не продлевает.
func TestIssuePasswordReset_RequestFromTheFuture(t *testing.T) {
	t.Parallel()
	var hashes []string
	repo := resetIssued(t, fixedNow, &hashes, true)
	repo.EXPECT().MarkPasswordResetSent(mock.Anything, "rid").Return(nil).Once()
	mailer := mocks.NewMockMailer(t)
	mailer.EXPECT().SendPasswordReset(mock.Anything, "a@b.com", mock.Anything).Return(nil).Once()
	f := newUC(t, withRepo(repo), withUsers(userByID(t, 1)), withMailer(mailer))

	require.NoError(t, f.uc.IssuePasswordReset(context.Background(), "rid", "u1", fixedNow.Add(30*24*time.Hour)))
}

// Письма нет, повтора доставки тоже: у адреса нет учётной записи, ссылка запроса истекла, учётную запись
// удалили или заблокировали после запроса.
func TestIssuePasswordReset_Skipped(t *testing.T) {
	t.Parallel()
	byID := func(u *verification.User, err error) func(t *testing.T) *mocks.MockUserReader {
		return func(t *testing.T) *mocks.MockUserReader {
			users := mocks.NewMockUserReader(t)
			users.EXPECT().GetByID(mock.Anything, "u1").Return(u, err).Once()
			return users
		}
	}
	cases := []struct {
		name        string
		userID      string
		requestedAt time.Time
		users       func(t *testing.T) *mocks.MockUserReader
	}{
		{name: "address without an account", requestedAt: fixedNow},
		{name: "link of the request expired", userID: "u1", requestedAt: fixedNow.Add(-time.Hour - time.Nanosecond)},
		{name: "user deleted after the request", userID: "u1", requestedAt: fixedNow, users: byID(nil, verification.ErrUserNotFound)},
		{name: "user blocked after the request", userID: "u1", requestedAt: fixedNow, users: byID(&verification.User{ID: "u1", Email: "a@b.com"}, nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var opts []ucOption
			if tc.users != nil {
				opts = append(opts, withUsers(tc.users(t)))
			}
			f := newUC(t, opts...)

			require.NoError(t, f.uc.IssuePasswordReset(context.Background(), "rid", tc.userID, tc.requestedAt))
		})
	}
}

// Сбой до отметки «письмо ушло» — ошибка обработчика: брокер повторит доставку, а не потеряет письмо.
func TestIssuePasswordReset_Failure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts func(t *testing.T) []ucOption
	}{
		{"user lookup", func(t *testing.T) []ucOption {
			users := mocks.NewMockUserReader(t)
			users.EXPECT().GetByID(mock.Anything, "u1").Return(nil, errDown).Once()
			return []ucOption{withUsers(users)}
		}},
		{"issues not queued", func(t *testing.T) []ucOption {
			repo := mocks.NewMockRepository(t)
			repo.EXPECT().LockPasswordResets(mock.Anything, "u1").Return(errDown).Once()
			return []ucOption{withUsers(userByID(t, 1)), withRepo(repo)}
		}},
		{"link not stored", func(t *testing.T) []ucOption {
			repo := mocks.NewMockRepository(t)
			repo.EXPECT().LockPasswordResets(mock.Anything, "u1").Return(nil).Once()
			repo.EXPECT().UpsertPasswordReset(mock.Anything, mock.Anything, mailPause).Return(false, errDown).Once()
			return []ucOption{withUsers(userByID(t, 1)), withRepo(repo)}
		}},
		{"sent mark not stored", func(t *testing.T) []ucOption {
			var hashes []string
			repo := resetIssued(t, fixedNow, &hashes, true)
			repo.EXPECT().MarkPasswordResetSent(mock.Anything, "rid").Return(errDown).Once()
			mailer := mocks.NewMockMailer(t)
			mailer.EXPECT().SendPasswordReset(mock.Anything, "a@b.com", mock.Anything).Return(nil).Once()
			return []ucOption{withUsers(userByID(t, 1)), withRepo(repo), withMailer(mailer)}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newUC(t, tc.opts(t)...)

			require.ErrorIs(t, f.uc.IssuePasswordReset(context.Background(), "rid", "u1", fixedNow), errDown)
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

// resetToken — репозиторий, который находит по хэшу токена tok ссылку сброса pr пользователя u1.
func resetToken(t *testing.T, pr *verification.PasswordResetToken, err error) *mocks.MockRepository {
	t.Helper()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPasswordReset(mock.Anything, authnkit.HashToken("tok")).Return(pr, err).Once()
	return repo
}

// newPassword — хэш нового пароля у владельца учётной записи: plain → "hash:"+plain или отказ err.
func newPassword(t *testing.T, plain string, err error) *mocks.MockPasswordSetter {
	t.Helper()
	pwSet := mocks.NewMockPasswordSetter(t)
	hash := ""
	if err == nil {
		hash = "hash:" + plain
	}
	pwSet.EXPECT().HashPassword(mock.Anything, plain).Return(hash, err).Once()
	return pwSet
}

// pendingReset — непогашенная ссылка rid пользователя u1, выпущенная при его версии 3.
func pendingReset(expiresAt time.Time) *verification.PasswordResetToken {
	return &verification.PasswordResetToken{ID: "rid", UserID: "u1", UserVersion: 3, ExpiresAt: expiresAt}
}

// Погашение ссылки с соседними и запись пароля при версии учётной записи на момент письма — одной транзакцией,
// погашение — первым: параллельный confirm ждёт его блокировку. Ссылка годна до своего срока включительно.
func TestConfirmPasswordReset_HappyPath(t *testing.T) {
	t.Parallel()
	repo := resetToken(t, pendingReset(fixedNow), nil)
	claim := repo.EXPECT().RedeemPasswordReset(mock.MatchedBy(pgkit.InTx), "rid", "u1").Return(nil).Once()
	pwSet := newPassword(t, "newpassword123", nil)
	pwSet.EXPECT().UpdatePassword(mock.MatchedBy(pgkit.InTx), "u1", "hash:newpassword123", 3).Return(nil).Once().NotBefore(claim)
	f := newUC(t, withRepo(repo), withPwSet(pwSet))

	require.NoError(t, f.uc.ConfirmPasswordReset(identitytest.Anon(), "tok", "newpassword123"))
	require.Equal(t, 1, f.tx.Calls(), "the redemption of the links and the password write share one transaction")
	require.Equal(t, audited("password_reset_confirmed"), f.audit.Last("password_reset_confirmed"))
}

// Отказ до транзакции: ни погашения, ни пароля, ни аудита. Правило нового пароля — у владельца учётной записи:
// его отказ доходит ошибкой verification после проверки ссылки.
func TestConfirmPasswordReset_Rejected(t *testing.T) {
	t.Parallel()
	usedAt := fixedNow.Add(-time.Minute)
	refused := func(plain string, err error) func(t *testing.T) []ucOption {
		return func(t *testing.T) []ucOption {
			return []ucOption{withRepo(resetToken(t, pendingReset(fixedNow.Add(time.Hour)), nil)), withPwSet(newPassword(t, plain, err))}
		}
	}
	tests := []struct {
		name     string
		password string
		opts     func(t *testing.T) []ucOption
		want     error
	}{
		{"weak password", "short", refused("short", verification.ErrWeakPassword), verification.ErrWeakPassword},
		{"new password over the hasher limit", "long", refused("long", verification.ErrPasswordTooLong), verification.ErrPasswordTooLong},
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
		{"new password not hashed", "newpassword123", refused("newpassword123", errHashing), errHashing},
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

// Отказ в транзакции откатывает её целиком: погашение не переживает несменённый пароль, аудита нет. Учётная
// запись, изменённая или удалённая после письма, — негодная ссылка.
func TestConfirmPasswordReset_RolledBack(t *testing.T) {
	t.Parallel()
	redeemed := func(repo *mocks.MockRepository) {
		repo.EXPECT().RedeemPasswordReset(mock.Anything, "rid", "u1").Return(nil).Once()
	}
	tests := []struct {
		name   string
		expect func(repo *mocks.MockRepository, pwSet *mocks.MockPasswordSetter)
		want   error
	}{
		{"redeemed in parallel", func(repo *mocks.MockRepository, _ *mocks.MockPasswordSetter) {
			repo.EXPECT().RedeemPasswordReset(mock.Anything, "rid", "u1").Return(verification.ErrTokenUsed).Once()
		}, verification.ErrTokenUsed},
		{"links not redeemed", func(repo *mocks.MockRepository, _ *mocks.MockPasswordSetter) {
			repo.EXPECT().RedeemPasswordReset(mock.Anything, "rid", "u1").Return(errDown).Once()
		}, errDown},
		{"account changed after the mail", func(repo *mocks.MockRepository, pwSet *mocks.MockPasswordSetter) {
			redeemed(repo)
			pwSet.EXPECT().UpdatePassword(mock.Anything, "u1", mock.Anything, 3).Return(verification.ErrConcurrentUpdate).Once()
		}, verification.ErrInvalidResetToken},
		{"account gone", func(repo *mocks.MockRepository, pwSet *mocks.MockPasswordSetter) {
			redeemed(repo)
			pwSet.EXPECT().UpdatePassword(mock.Anything, "u1", mock.Anything, 3).Return(verification.ErrUserNotFound).Once()
		}, verification.ErrInvalidResetToken},
		{"password not written", func(repo *mocks.MockRepository, pwSet *mocks.MockPasswordSetter) {
			redeemed(repo)
			pwSet.EXPECT().UpdatePassword(mock.Anything, "u1", mock.Anything, 3).Return(errDown).Once()
		}, errDown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := resetToken(t, pendingReset(fixedNow.Add(time.Hour)), nil)
			pwSet := newPassword(t, "newpassword123", nil)
			tc.expect(repo, pwSet)
			f := newUC(t, withRepo(repo), withPwSet(pwSet))

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
	f := newUC(t, withRepo(repo), withEmailSet(emailSet), withAudit(&audittest.Recorder{LogErr: errAudit}))

	require.NoError(t, f.uc.ConfirmEmail(identitytest.Anon(), "tok"))
	e, logged := f.log.Find(slog.LevelError, "audit email_confirmed")
	require.True(t, logged)
	require.Contains(t, e.Str("error"), errAudit.Error())
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

// Job удаляет ссылки, истёкшие раньше суток назад; сбой удаления — ошибка прогона (её логирует timerkit).
func TestPurgeLinks(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().DeleteExpiredLinks(mock.Anything, fixedNow.Add(-24*time.Hour)).Return(3, nil).Once()
	repo.EXPECT().DeleteExpiredLinks(mock.Anything, fixedNow.Add(-24*time.Hour)).Return(0, errDown).Once()
	f := newUC(t, withRepo(repo))

	require.NoError(t, f.uc.PurgeLinks(context.Background()))
	e, logged := f.log.Find(slog.LevelInfo, "expired links purged")
	require.True(t, logged)
	require.Equal(t, "3", e.Str("count"))
	require.ErrorIs(t, f.uc.PurgeLinks(context.Background()), errDown)
}
