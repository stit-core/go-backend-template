package verification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/fx"

	"git.stit.tech/stit/go-backend-template/pkg/tokens"
	"git.stit.tech/stit/golibinfra/authnkit"
	"git.stit.tech/stit/golibinfra/clockkit"
	"git.stit.tech/stit/golibinfra/logkit"
	"git.stit.tech/stit/golibinfra/outboxkit"
	"git.stit.tech/stit/golibinfra/pgkit"
	"git.stit.tech/stit/golibinfra/telemetry"
)

const minPasswordLen = 8

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

type Repository interface {
	InsertPasswordReset(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	GetPasswordReset(ctx context.Context, tokenHash string) (*PasswordResetToken, error)
	MarkPasswordResetUsed(ctx context.Context, id string) error
	InsertEmailVerification(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
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
	Hasher        authnkit.Hasher
	Tx            pgkit.Transactor
	Events        outboxkit.Publisher
	Mailer        Mailer
	Log           logkit.Logger
	Clock         clockkit.Clock
}

type Usecase struct {
	repo     Repository
	users    UserReader
	pwSet    PasswordSetter
	emailSet EmailVerificationSetter
	hasher   authnkit.Hasher
	tx       pgkit.Transactor
	events   outboxkit.Publisher
	mailer   Mailer
	log      logkit.Logger
	clock    clockkit.Clock
}

func NewUsecase(d UsecaseDeps) *Usecase {
	return &Usecase{
		repo:     d.Repo,
		users:    d.Users,
		pwSet:    d.PasswordSet,
		emailSet: d.EmailVerifier,
		hasher:   d.Hasher,
		tx:       d.Tx,
		events:   d.Events,
		mailer:   d.Mailer,
		log:      d.Log.With("domain", "verification"),
		clock:    d.Clock,
	}
}

func (uc *Usecase) IssueEmailVerification(ctx context.Context, userID, email string) (err error) {
	ctx, end := telemetry.Span(ctx, "verification.usecase.issue_email_verification")
	defer end(&err)

	t, err := tokens.Generate()
	if err != nil {
		return fmt.Errorf("generate verify token: %w", err)
	}

	tokenID := uuid.NewString()

	if err := uc.tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.repo.InsertEmailVerification(txCtx, userID, tokens.Hash(t), uc.clock.Now().Add(24*time.Hour)); err != nil {
			return err
		}
		return uc.events.Publish(txCtx, "verification.events.email_verification_requested",
			EmailVerificationRequested{
				TokenID: tokenID,
				UserID:  userID,
				Email:   email,
				At:      uc.clock.Now(),
			})
	}); err != nil {
		return err
	}

	if err := uc.mailer.SendEmailVerification(ctx, email, t); err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}
	return nil
}

func (uc *Usecase) RequestPasswordReset(ctx context.Context, email string) (err error) {
	ctx, end := telemetry.Span(ctx, "verification.usecase.request_password_reset")
	defer end(&err)

	u, err := uc.users.GetByEmail(ctx, email)
	if err != nil {
		return nil
	}
	if !u.Active {
		return nil
	}

	token, err := tokens.Generate()
	if err != nil {
		return fmt.Errorf("generate reset token: %w", err)
	}

	tokenID := uuid.NewString()

	if err := uc.tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.repo.InsertPasswordReset(txCtx, u.ID, tokens.Hash(token), uc.clock.Now().Add(time.Hour)); err != nil {
			return err
		}
		return uc.events.Publish(txCtx, "verification.events.password_reset_requested",
			PasswordResetRequested{
				TokenID: tokenID,
				UserID:  u.ID,
				Email:   u.Email,
				At:      uc.clock.Now(),
			})
	}); err != nil {
		return err
	}

	if err := uc.mailer.SendPasswordReset(ctx, u.Email, token); err != nil {
		return fmt.Errorf("send password reset email: %w", err)
	}
	return nil
}

func (uc *Usecase) ConfirmPasswordReset(ctx context.Context, token, newPassword string) (err error) {
	ctx, end := telemetry.Span(ctx, "verification.usecase.confirm_password_reset")
	defer end(&err)

	if len(newPassword) < minPasswordLen {
		return ErrWeakPassword
	}

	pr, err := uc.repo.GetPasswordReset(ctx, tokens.Hash(token))
	if err != nil {
		return err
	}
	if pr.UsedAt != nil || uc.clock.Now().After(pr.ExpiresAt) {
		return ErrInvalidResetToken
	}

	hash, err := uc.hasher.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	current, err := uc.users.GetByID(ctx, pr.UserID)
	if err != nil {
		return fmt.Errorf("load user: %w", err)
	}

	if err := uc.tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.pwSet.UpdatePassword(txCtx, pr.UserID, hash, current.Version); err != nil {
			return err
		}
		return uc.repo.MarkPasswordResetUsed(txCtx, pr.ID)
	}); err != nil {
		return err
	}

	uc.log.Info(ctx, "password reset", "user_id", pr.UserID)
	return nil
}

func (uc *Usecase) ConfirmEmail(ctx context.Context, token string) (err error) {
	ctx, end := telemetry.Span(ctx, "verification.usecase.confirm_email")
	defer end(&err)

	ev, err := uc.repo.GetEmailVerification(ctx, tokens.Hash(token))
	if err != nil {
		return err
	}
	if ev.UsedAt != nil || uc.clock.Now().After(ev.ExpiresAt) {
		return ErrInvalidVerificationToken
	}

	return uc.tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.emailSet.MarkEmailVerified(txCtx, ev.UserID); err != nil {
			return err
		}
		return uc.repo.MarkEmailVerificationUsed(txCtx, ev.ID)
	})
}
