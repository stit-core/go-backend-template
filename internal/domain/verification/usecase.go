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
	"git.stit.tech/stit-core/go-backend-template/pkg/tokens"
	"git.stit.tech/stit-core/golibinfra/auditkit"
	"git.stit.tech/stit-core/golibinfra/authnkit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
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

// Repository: Mark*Used гасит токен, только если он не погашен, иначе — ErrTokenUsed.
type Repository interface {
	InsertPasswordReset(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (id string, err error)
	GetPasswordReset(ctx context.Context, tokenHash string) (*PasswordResetToken, error)
	MarkPasswordResetUsed(ctx context.Context, id string) error
	// RevokePasswordResets гасит все непогашенные токены сброса пользователя.
	RevokePasswordResets(ctx context.Context, userID string) error
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

// ResetLimiter: Allow учитывает запрос сброса пароля пользователя с адреса ip; false — лимит
// запросов исчерпан, письмо не уходит.
type ResetLimiter interface {
	Allow(ctx context.Context, userID, ip string) (bool, error)
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

// IssueEmailVerification — идемпотентный обработчик доставки user.events.registered.v2: отправленное
// письмо повтор пропускает, неотправленное (сбой mailer) отправляет заново с новым токеном. Адрес
// берётся у пользователя по ID: в событии его нет.
func (uc *Usecase) IssueEmailVerification(ctx context.Context, userID string) error {
	u, err := uc.d.Users.GetByID(ctx, userID)
	if errors.Is(err, ErrUserNotFound) {
		uc.d.Log.Info(ctx, "user gone before email verification, skip", "user_id", userID)
		return nil
	}
	if err != nil {
		return fmt.Errorf("load user: %w", err)
	}
	t, err := tokens.Generate()
	if err != nil {
		return fmt.Errorf("generate verify token: %w", err)
	}

	var (
		tokenID string
		issued  bool
	)
	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		var err error
		tokenID, issued, err = uc.d.Repo.UpsertEmailVerification(txCtx, userID, tokens.Hash(t), uc.d.Clock.Now().Add(emailVerificationTTL))
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

// RequestPasswordReset отвечает одинаково, есть ли учётная запись, пускают ли её, исчерпан ли лимит
// запросов и что сломалось после поиска: по ответу не узнать, чей email зарегистрирован. Исход — в метрику.
func (uc *Usecase) RequestPasswordReset(ctx context.Context, email string) error {
	result, err := uc.requestPasswordReset(ctx, email)
	uc.d.Metrics.RecordPasswordResetRequested(ctx, result)
	return err
}

func (uc *Usecase) requestPasswordReset(ctx context.Context, email string) (outcome.Result, error) {
	u, err := uc.d.Users.GetByEmail(ctx, email)
	switch {
	case errors.Is(err, ErrUserNotFound):
		return resultUserNotFound, nil
	case err != nil:
		return outcome.Error, fmt.Errorf("lookup user: %w", err)
	case !u.CanLogin:
		return resultInactive, nil
	}
	// Дальше путь есть только у учётной записи: 500 на её сбое (лимитер, запись, письмо) выдал бы её —
	// сбой уходит в лог и метрику, ответ тот же; повтор — новым запросом.
	result, err := uc.issuePasswordReset(ctx, u)
	if err != nil {
		uc.d.Log.Error(ctx, "password reset request", "user_id", u.ID, "err", err)
	}
	return result, nil
}

func (uc *Usecase) issuePasswordReset(ctx context.Context, u *User) (outcome.Result, error) {
	allowed, err := uc.d.Limiter.Allow(ctx, u.ID, identity.IPFrom(ctx))
	switch {
	case err != nil:
		return outcome.Error, fmt.Errorf("limiter: %w", err)
	case !allowed:
		return resultThrottled, nil
	}

	token, err := tokens.Generate()
	if err != nil {
		return outcome.Error, fmt.Errorf("generate reset token: %w", err)
	}
	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		tokenID, err := uc.d.Repo.InsertPasswordReset(txCtx, u.ID, tokens.Hash(token), uc.d.Clock.Now().Add(passwordResetTTL))
		if err != nil {
			return err
		}
		return uc.d.Events.Publish(txCtx, subjectPasswordResetRequested,
			PasswordResetRequested{TokenID: tokenID, UserID: u.ID, At: uc.d.Clock.Now()})
	}); err != nil {
		return outcome.Error, err
	}

	if err := uc.d.Mailer.SendPasswordReset(ctx, u.Email, token); err != nil {
		return outcome.Error, fmt.Errorf("send password reset email: %w", err)
	}
	return outcome.OK, nil
}

func (uc *Usecase) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	if !policy.PasswordStrong(newPassword) {
		return ErrWeakPassword
	}

	pr, err := uc.d.Repo.GetPasswordReset(ctx, tokens.Hash(token))
	switch {
	case err != nil:
		return err
	case pr.UsedAt != nil:
		return ErrTokenUsed
	case uc.d.Clock.Now().After(pr.ExpiresAt):
		return ErrInvalidResetToken
	}

	hash, err := uc.d.Hasher.Hash(ctx, newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	current, err := uc.d.Users.GetByID(ctx, pr.UserID)
	if errors.Is(err, ErrUserNotFound) {
		return ErrInvalidResetToken // токен удалённого пользователя
	}
	if err != nil {
		return fmt.Errorf("load user: %w", err)
	}

	// Погашение — первым в транзакции: параллельный confirm ждёт блокировку строки и получает
	// ErrTokenUsed, не меняя пароль. Остальные ссылки сброса гаснут с ним: старое письмо пароль не сменит.
	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.d.Repo.MarkPasswordResetUsed(txCtx, pr.ID); err != nil {
			return err
		}
		if err := uc.d.Repo.RevokePasswordResets(txCtx, pr.UserID); err != nil {
			return err
		}
		return uc.d.PasswordSet.UpdatePassword(txCtx, pr.UserID, hash, current.Version)
	}); err != nil {
		return err
	}

	uc.d.Log.Info(ctx, "password reset", "user_id", pr.UserID)
	uc.audit(ctx, auditkit.Entry{ActorID: pr.UserID, Action: "password_reset_confirmed", Resource: "user", ResourceID: pr.UserID})
	return nil
}

func (uc *Usecase) ConfirmEmail(ctx context.Context, token string) error {
	ev, err := uc.d.Repo.GetEmailVerification(ctx, tokens.Hash(token))
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

// audit дописывает время, IP и запрос; сбой записи аудита операцию не отменяет — он уходит в лог.
func (uc *Usecase) audit(ctx context.Context, e auditkit.Entry) {
	e.At, e.IP, e.RequestID = uc.d.Clock.Now(), identity.IPFrom(ctx), identity.RequestIDFrom(ctx)
	if err := uc.d.Audit.Log(ctx, e); err != nil {
		uc.d.Log.Error(ctx, "audit "+e.Action, "err", err)
	}
}
