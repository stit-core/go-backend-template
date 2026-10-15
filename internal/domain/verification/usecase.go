package verification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/identity"
	"git.stit.tech/stit-core/go-backend-template/pkg/outcome"
	"git.stit.tech/stit-core/go-backend-template/pkg/policy"
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

type PasswordSetter interface {
	UpdatePassword(ctx context.Context, userID, hash string, version int) error
}

type EmailVerificationSetter interface {
	MarkEmailVerified(ctx context.Context, userID string) error
}

// Repository: Mark*Used и Redeem* гасят токен, только если он не погашен, иначе — ErrTokenUsed.
type Repository interface {
	// UpsertPasswordReset выпускает ссылку: новую или новый токен своей неотправленной (отправленную, погашенную, чужую
	// и ссылку запроса, после которого пользователь погасил ссылку сброса, — нет: issued=false).
	UpsertPasswordReset(ctx context.Context, r PasswordResetIssue) (issued bool, err error)
	MarkPasswordResetSent(ctx context.Context, id string) error
	GetPasswordReset(ctx context.Context, tokenHash string) (*PasswordResetToken, error)
	// RedeemPasswordReset гасит ссылку id вместе со всеми непогашенными ссылками её пользователя userID.
	RedeemPasswordReset(ctx context.Context, id, userID string) error
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

// ResetLimiter: Allow учитывает запрос сброса пароля на адрес email с IP ip; false — лимит запросов исчерпан,
// запрос ничего не делает.
type ResetLimiter interface {
	Allow(ctx context.Context, email, ip string) (bool, error)
}

type UsecaseDeps struct {
	fx.In

	Repo          Repository
	Users         UserReader
	PasswordSet   PasswordSetter
	EmailVerifier EmailVerificationSetter
	Hasher        authnkit.Hasher
	Tx            pgkit.Transactor
	Events        outboxkit.Publisher
	Mailer        Mailer
	Limiter       ResetLimiter
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

// IssueEmailVerification — идемпотентный обработчик доставки user.events.registered.v2. Ключ — состояние
// пользователя: подтверждённому адресу письмо не нужно, а погашенная ссылка ожидающей записи уже не держит.
// Отправленное письмо повтор пропускает, неотправленное (сбой mailer) отправляет заново с новым токеном.
// Адрес берётся у пользователя по ID: в событии его нет.
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

// RequestPasswordReset делает одну и ту же работу для любого адреса — поиск, лимит, событие запроса: ни ответ,
// ни его время не выдают, есть ли учётная запись и пускают ли её. Письмо шлёт подписчик события
// (IssuePasswordReset). Исход — в метрику.
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
	// Лимит считает адрес, как guard входа, и для учётной записи: поиск (LOWER БД) и ключ guard'а (strings.ToLower)
	// складывают регистр не ASCII по-разному, и ключ по учётной записи выдавал бы её работой запроса.
	allowed, err := uc.d.Limiter.Allow(ctx, email, identity.IPFrom(ctx))
	switch {
	case err != nil:
		return outcome.Error, fmt.Errorf("limiter: %w", err)
	case !allowed:
		return resultThrottled, nil
	}
	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		return uc.d.Events.Publish(txCtx, subjectPasswordResetRequested,
			PasswordResetRequested{RequestID: idkit.NewV7(), UserID: userID, At: uc.d.Clock.Now()})
	}); err != nil {
		return outcome.Error, err
	}
	return result, nil
}

// IssuePasswordReset — идемпотентный обработчик доставки verification.events.password_reset_requested.v3: ключ —
// запрос (RequestID — id ссылки сброса). Письмо получает учётная запись, которую пускают, пока ссылка запроса
// не истекла и пользователь не погасил ссылку сброса после запроса; отправленную ссылку повтор пропускает,
// неотправленную — отправляет заново с новым токеном.
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
	issued, err := uc.d.Repo.UpsertPasswordReset(ctx, PasswordResetIssue{
		RequestID: requestID, UserID: userID, UserVersion: u.Version, TokenHash: authnkit.HashToken(token),
		RequestedAt: requestedAt, ExpiresAt: expiresAt,
	})
	switch {
	case err != nil:
		return err
	case !issued:
		uc.d.Log.Info(ctx, "password reset not issued: sent or redeemed, skip", "user_id", userID)
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
	if !policy.PasswordStrong(newPassword) {
		return ErrWeakPassword
	}

	pr, err := uc.d.Repo.GetPasswordReset(ctx, authnkit.HashToken(token))
	switch {
	case err != nil:
		return err
	case pr.UsedAt != nil:
		return ErrTokenUsed
	case uc.d.Clock.Now().After(pr.ExpiresAt):
		return ErrInvalidResetToken
	}

	hash, err := uc.d.Hasher.Hash(ctx, newPassword)
	switch {
	case errors.Is(err, authnkit.ErrPasswordTooLong):
		return ErrPasswordTooLong
	case err != nil:
		return fmt.Errorf("hash new password: %w", err)
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
	uc.audit(ctx, auditkit.Entry{ActorID: ev.UserID, Action: "email_confirmed", Resource: "user", ResourceID: ev.UserID})
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
