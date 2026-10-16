package verification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/pkg/outcome"
	"git.stit.tech/stit-core/golibinfra/auditkit"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/idkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/outboxkit"
	"git.stit.tech/stit-core/golibinfra/pgkit"
)

type UserReader interface {
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
}

// PasswordSetter: HashPassword — хэш нового пароля по правилу продукта (ErrWeakPassword, ErrPasswordTooLong),
// UpdatePassword пишет его при версии учётной записи и закрывает все её сессии.
type PasswordSetter interface {
	HashPassword(ctx context.Context, plain string) (string, error)
	UpdatePassword(ctx context.Context, userID, hash string, version int) error
}

type EmailVerificationSetter interface {
	MarkEmailVerified(ctx context.Context, userID string) error
}

// Repository: Mark*Used и Redeem* гасят токен, только если он не погашен, иначе — ErrTokenUsed.
type Repository interface {
	// TryLockPasswordResets занимает выпуски ссылок пользователя до конца транзакции из ctx; false — их занял сосед.
	TryLockPasswordResets(ctx context.Context, userID string) (bool, error)
	// UpsertPasswordReset выпускает ссылку: новую или новый токен своей неотправленной; отправленную, погашенную, чужую,
	// после погашения ссылки и при другой ссылке пользователя моложе pause — нет (issued=false).
	UpsertPasswordReset(ctx context.Context, r PasswordResetIssue, pause time.Duration) (issued bool, err error)
	MarkPasswordResetSent(ctx context.Context, id string) error
	GetPasswordReset(ctx context.Context, tokenHash string) (*PasswordResetToken, error)
	// RedeemPasswordReset гасит ссылку id вместе со всеми непогашенными ссылками её пользователя userID.
	RedeemPasswordReset(ctx context.Context, id, userID string) error
	// DeleteExpiredLinks удаляет батч ссылок сброса и подтверждений адреса, истёкших раньше before, и отдаёт их число.
	DeleteExpiredLinks(ctx context.Context, before time.Time) (int64, error)
	// UpsertEmailVerification выпускает ожидающее подтверждение пользователя: неотправленное —
	// с новым токеном и тем же id, отправленное не трогает (issued=false).
	UpsertEmailVerification(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (id string, issued bool, err error)
	MarkEmailVerificationSent(ctx context.Context, id string) error
	GetEmailVerification(ctx context.Context, tokenHash string) (*EmailVerificationToken, error)
	MarkEmailVerificationUsed(ctx context.Context, id string) error
}

type Mailer interface {
	SendPasswordReset(ctx context.Context, email, token string) error
	SendEmailVerification(ctx context.Context, email, token string) error
}

type UsecaseDeps struct {
	fx.In

	Repo          Repository
	Users         UserReader
	PasswordSet   PasswordSetter
	EmailVerifier EmailVerificationSetter
	Tx            pgkit.Transactor
	Events        outboxkit.Publisher
	Mailer        Mailer
	Config        Config
	Audit         auditkit.Logger
	Log           logkit.Logger
	Clock         clockkit.Clock
	Metrics       *Metrics
}

type Usecase struct {
	d UsecaseDeps
}

func NewUsecase(d UsecaseDeps) *Usecase {
	d.Log = d.Log.With("domain", "verification")
	return &Usecase{d: d}
}

// IssueEmailVerification — идемпотентный обработчик user.events.registered.v2: ключ — состояние пользователя
// (подтверждённому письмо не нужно); отправленное повтор пропускает, неотправленное шлёт с новым токеном.
func (uc *Usecase) IssueEmailVerification(ctx context.Context, userID string) error {
	u, err := uc.d.Users.GetByID(ctx, userID)
	switch {
	case errors.Is(err, ErrUserNotFound):
		uc.d.Log.Info(ctx, "user gone before email verification, skip", "user_id", userID)
		return nil
	case err != nil:
		return fmt.Errorf("load user: %w", err)
	case u.EmailVerified:
		uc.d.Log.Info(ctx, "email already verified, skip", "user_id", userID)
		return nil
	}
	t := authnkit.NewToken()

	var (
		tokenID string
		issued  bool
	)
	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		var err error
		tokenID, issued, err = uc.d.Repo.UpsertEmailVerification(txCtx, userID, authnkit.HashToken(t), uc.d.Clock.Now().Add(emailVerificationTTL))
		if err != nil || !issued {
			return err
		}
		return uc.d.Events.Publish(txCtx, subjectEmailVerificationRequested,
			EmailVerificationRequested{TokenID: tokenID, UserID: userID, At: uc.d.Clock.Now()})
	}); err != nil {
		return err
	}
	if !issued {
		uc.d.Log.Info(ctx, "email verification already sent, skip", "user_id", userID)
		return nil
	}

	if err := uc.d.Mailer.SendEmailVerification(ctx, u.Email, t); err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}
	if err := uc.d.Repo.MarkEmailVerificationSent(ctx, tokenID); err != nil {
		return fmt.Errorf("mark verification email sent: %w", err)
	}
	return nil
}

// RequestPasswordReset делает одну работу для любого адреса — поиск и событие запроса: ни ответ, ни его время не
// выдают учётную запись. Письмо шлёт подписчик события (IssuePasswordReset), исход — в метрику.
func (uc *Usecase) RequestPasswordReset(ctx context.Context, email string) error {
	result, err := uc.requestPasswordReset(ctx, email)
	uc.d.Metrics.RecordPasswordResetRequested(ctx, result)
	return err
}

func (uc *Usecase) requestPasswordReset(ctx context.Context, email string) (outcome.Result, error) {
	var userID string
	result := resultUserNotFound
	u, err := uc.d.Users.GetByEmail(ctx, email)
	switch {
	case err == nil && u.CanLogin:
		userID, result = u.ID, outcome.OK
	case err == nil:
		userID, result = u.ID, resultInactive
	case !errors.Is(err, ErrUserNotFound):
		return outcome.Error, fmt.Errorf("lookup user: %w", err)
	}
	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		return uc.d.Events.Publish(txCtx, subjectPasswordResetRequested,
			PasswordResetRequested{RequestID: idkit.NewV7(), UserID: userID, At: uc.d.Clock.Now()})
	}); err != nil {
		return outcome.Error, err
	}
	return result, nil
}

// IssuePasswordReset — идемпотентный обработчик verification.events.password_reset_requested.v3 (ключ — RequestID):
// письмо — учётной записи, которую пускают, раз в паузу Config.ResetMailPause, с каких бы адресов ни шли запросы.
func (uc *Usecase) IssuePasswordReset(ctx context.Context, requestID, userID string, requestedAt time.Time) error {
	now := uc.d.Clock.Now()
	if requestedAt.After(now) {
		requestedAt = now // часы публикатора спешат или событие записано мимо outbox: срок не дольше TTL от обработки
	}
	expiresAt := requestedAt.Add(passwordResetTTL)
	if userID == "" || now.After(expiresAt) {
		return nil
	}
	u, err := uc.d.Users.GetByID(ctx, userID)
	switch {
	case errors.Is(err, ErrUserNotFound):
		uc.d.Log.Info(ctx, "user gone before the password reset, skip", "user_id", userID)
		return nil
	case err != nil:
		return fmt.Errorf("load user: %w", err)
	case !u.CanLogin:
		uc.d.Log.Info(ctx, "user blocked before the password reset, skip", "user_id", userID)
		return nil
	}

	token := authnkit.NewToken()
	var issued bool
	// Доставки подписчик обрабатывает параллельно: без блокировки пауза не увидела бы незакоммиченную ссылку соседнего
	// выпуска, а занятые соседом выпуски — та же пауза, не очередь: поток в один ящик не держит соединения пула.
	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		if uc.d.Config.ResetMailPause > 0 {
			locked, err := uc.d.Repo.TryLockPasswordResets(txCtx, userID)
			if err != nil || !locked {
				return err
			}
		}
		var err error
		issued, err = uc.d.Repo.UpsertPasswordReset(txCtx, PasswordResetIssue{
			RequestID: requestID, UserID: userID, UserVersion: u.Version, TokenHash: authnkit.HashToken(token),
			RequestedAt: requestedAt, ExpiresAt: expiresAt,
		}, uc.d.Config.ResetMailPause)
		return err
	}); err != nil {
		return err
	}
	if !issued {
		uc.d.Log.Info(ctx, "password reset not issued: sent, redeemed or paused, skip", "user_id", userID)
		return nil
	}
	if err := uc.d.Mailer.SendPasswordReset(ctx, u.Email, token); err != nil {
		return fmt.Errorf("send password reset email: %w", err)
	}
	if err := uc.d.Repo.MarkPasswordResetSent(ctx, requestID); err != nil {
		return fmt.Errorf("mark password reset sent: %w", err)
	}
	return nil
}

func (uc *Usecase) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	pr, err := uc.d.Repo.GetPasswordReset(ctx, authnkit.HashToken(token))
	switch {
	case err != nil:
		return err
	case pr.UsedAt != nil:
		return ErrTokenUsed
	case uc.d.Clock.Now().After(pr.ExpiresAt):
		return ErrInvalidResetToken
	}

	hash, err := uc.d.PasswordSet.HashPassword(ctx, newPassword)
	if err != nil {
		return err
	}

	// Погашение — первым в транзакции: параллельный confirm ждёт блокировку строк и получает ErrTokenUsed. Пароль
	// пишется при версии учётной записи на момент письма: изменённой или удалённой после него ссылка его не меняет.
	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.d.Repo.RedeemPasswordReset(txCtx, pr.ID, pr.UserID); err != nil {
			return err
		}
		err := uc.d.PasswordSet.UpdatePassword(txCtx, pr.UserID, hash, pr.UserVersion)
		if errors.Is(err, ErrConcurrentUpdate) || errors.Is(err, ErrUserNotFound) {
			return ErrInvalidResetToken
		}
		return err
	}); err != nil {
		return err
	}

	uc.d.Log.Info(ctx, "password reset", "user_id", pr.UserID)
	uc.audit(ctx, auditkit.Entry{ActorID: pr.UserID, Action: "password_reset_confirmed", Resource: "user", ResourceID: pr.UserID})
	return nil
}

func (uc *Usecase) ConfirmEmail(ctx context.Context, token string) error {
	ev, err := uc.d.Repo.GetEmailVerification(ctx, authnkit.HashToken(token))
	switch {
	case err != nil:
		return err
	case ev.UsedAt != nil:
		return ErrTokenUsed
	case uc.d.Clock.Now().After(ev.ExpiresAt):
		return ErrInvalidVerificationToken
	}

	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.d.Repo.MarkEmailVerificationUsed(txCtx, ev.ID); err != nil {
			return err
		}
		err := uc.d.EmailVerifier.MarkEmailVerified(txCtx, ev.UserID)
		if errors.Is(err, ErrUserNotFound) {
			return ErrInvalidVerificationToken // токен удалённого пользователя
		}
		return err
	}); err != nil {
		return err
	}
	uc.d.Log.Info(ctx, "email confirmed", "user_id", ev.UserID)
	uc.audit(ctx, auditkit.Entry{ActorID: ev.UserID, Action: "email_confirmed", Resource: "user", ResourceID: ev.UserID})
	return nil
}

// PurgeLinks — тело job'а verification.purge_links: ссылки, истёкшие раньше linkRetention, ничего не решают, а их
// хэши и время запросов хранить незачем. Батчами до пустого в пределах Timeout job'а: хвост выше батча не копится.
func (uc *Usecase) PurgeLinks(ctx context.Context) error {
	before := uc.d.Clock.Now().Add(-linkRetention)
	var total int64
	for {
		n, err := uc.d.Repo.DeleteExpiredLinks(ctx, before)
		if err != nil {
			return fmt.Errorf("purge expired links (%d purged): %w", total, err)
		}
		if n == 0 {
			break
		}
		total += n
	}
	uc.d.Log.Info(ctx, "expired links purged", "count", total)
	return nil
}

// audit дописывает время (IP клиента и ID запроса auditkit берёт из ctx); сбой записи аудита операцию не
// отменяет — он уходит в лог.
func (uc *Usecase) audit(ctx context.Context, e auditkit.Entry) {
	e.At = uc.d.Clock.Now()
	if err := uc.d.Audit.Log(ctx, e); err != nil {
		uc.d.Log.Error(ctx, "audit "+e.Action, "error", err)
	}
}
