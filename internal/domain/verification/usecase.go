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

type Repository interface {
	InsertPasswordReset(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	GetPasswordReset(ctx context.Context, tokenHash string) (*PasswordResetToken, error)
	MarkPasswordResetUsed(ctx context.Context, id string) error
	// InsertEmailVerification reports inserted=false when a pending verification
	// already exists for the user (ON CONFLICT no-op) — the redelivery case.
	InsertEmailVerification(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (inserted bool, err error)
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

func (uc *Usecase) IssueEmailVerification(ctx context.Context, userID, email string) error {
	t, err := tokens.Generate()
	if err != nil {
		return fmt.Errorf("generate verify token: %w", err)
	}

	tokenID := idkit.NewV7()

	var inserted bool
	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		var err error
		inserted, err = uc.d.Repo.InsertEmailVerification(txCtx, userID, tokens.Hash(t), uc.d.Clock.Now().Add(24*time.Hour))
		if err != nil {
			return err
		}
		if !inserted {
			return nil
		}
		return uc.d.Events.Publish(txCtx, subjectEmailVerificationRequested,
			EmailVerificationRequested{
				TokenID: tokenID,
				UserID:  userID,
				Email:   email,
				At:      uc.d.Clock.Now(),
			})
	}); err != nil {
		uc.d.Metrics.RecordEmailIssued(ctx, outcome.Error)
		return err
	}
	if !inserted {
		uc.d.Log.Info(ctx, "email verification already pending, skip", "user_id", userID)
		uc.d.Metrics.RecordEmailIssued(ctx, resultDuplicate)
		return nil
	}

	if err := uc.d.Mailer.SendEmailVerification(ctx, email, t); err != nil {
		uc.d.Metrics.RecordEmailIssued(ctx, outcome.Error)
		return fmt.Errorf("send verification email: %w", err)
	}
	uc.d.Metrics.RecordEmailIssued(ctx, outcome.OK)
	return nil
}

func (uc *Usecase) RequestPasswordReset(ctx context.Context, email string) error {
	u, err := uc.d.Users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			uc.d.Metrics.RecordPasswordResetRequested(ctx, resultUserNotFound)
			return nil // anti-enumeration: absence of an account must be indistinguishable
		}
		return fmt.Errorf("lookup user: %w", err)
	}
	if !u.Active {
		uc.d.Metrics.RecordPasswordResetRequested(ctx, resultInactive)
		return nil
	}

	token, err := tokens.Generate()
	if err != nil {
		return fmt.Errorf("generate reset token: %w", err)
	}

	tokenID := idkit.NewV7()

	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.d.Repo.InsertPasswordReset(txCtx, u.ID, tokens.Hash(token), uc.d.Clock.Now().Add(time.Hour)); err != nil {
			return err
		}
		return uc.d.Events.Publish(txCtx, subjectPasswordResetRequested,
			PasswordResetRequested{
				TokenID: tokenID,
				UserID:  u.ID,
				Email:   u.Email,
				At:      uc.d.Clock.Now(),
			})
	}); err != nil {
		uc.d.Metrics.RecordPasswordResetRequested(ctx, outcome.Error)
		return err
	}

	if err := uc.d.Mailer.SendPasswordReset(ctx, u.Email, token); err != nil {
		uc.d.Metrics.RecordPasswordResetRequested(ctx, outcome.Error)
		return fmt.Errorf("send password reset email: %w", err)
	}
	uc.d.Metrics.RecordPasswordResetRequested(ctx, outcome.OK)
	return nil
}

func (uc *Usecase) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	if !policy.PasswordStrong(newPassword) {
		uc.d.Metrics.RecordPasswordResetConfirmed(ctx, outcome.WeakPassword)
		return ErrWeakPassword
	}

	pr, err := uc.d.Repo.GetPasswordReset(ctx, tokens.Hash(token))
	if err != nil {
		uc.d.Metrics.RecordPasswordResetConfirmed(ctx, outcome.Invalid)
		return err
	}
	if pr.UsedAt != nil {
		uc.d.Metrics.RecordPasswordResetConfirmed(ctx, resultAlreadyUsed)
		return ErrInvalidResetToken
	}
	if uc.d.Clock.Now().After(pr.ExpiresAt) {
		uc.d.Metrics.RecordPasswordResetConfirmed(ctx, outcome.Expired)
		return ErrInvalidResetToken
	}

	hash, err := uc.d.Hasher.Hash(ctx, newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	current, err := uc.d.Users.GetByID(ctx, pr.UserID)
	if err != nil {
		return fmt.Errorf("load user: %w", err)
	}

	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.d.PasswordSet.UpdatePassword(txCtx, pr.UserID, hash, current.Version); err != nil {
			return err
		}
		return uc.d.Repo.MarkPasswordResetUsed(txCtx, pr.ID)
	}); err != nil {
		return err
	}

	uc.d.Log.Info(ctx, "password reset", "user_id", pr.UserID)
	uc.writeAudit(ctx, pr.UserID, "password_reset_confirmed", "user", pr.UserID)
	uc.d.Metrics.RecordPasswordResetConfirmed(ctx, outcome.OK)
	return nil
}

func (uc *Usecase) ConfirmEmail(ctx context.Context, token string) error {
	ev, err := uc.d.Repo.GetEmailVerification(ctx, tokens.Hash(token))
	if err != nil {
		uc.d.Metrics.RecordEmailConfirmed(ctx, outcome.Invalid)
		return err
	}
	if ev.UsedAt != nil {
		uc.d.Metrics.RecordEmailConfirmed(ctx, resultAlreadyUsed)
		return ErrInvalidVerificationToken
	}
	if uc.d.Clock.Now().After(ev.ExpiresAt) {
		uc.d.Metrics.RecordEmailConfirmed(ctx, outcome.Expired)
		return ErrInvalidVerificationToken
	}

	if err := uc.d.Tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := uc.d.EmailVerifier.MarkEmailVerified(txCtx, ev.UserID); err != nil {
			return err
		}
		return uc.d.Repo.MarkEmailVerificationUsed(txCtx, ev.ID)
	}); err != nil {
		return err
	}
	uc.writeAudit(ctx, ev.UserID, "email_confirmed", "user", ev.UserID)
	uc.d.Metrics.RecordEmailConfirmed(ctx, outcome.OK)
	return nil
}

func (uc *Usecase) writeAudit(ctx context.Context, actorID, action, resource, resourceID string) {
	if auditErr := uc.d.Audit.Log(ctx, auditkit.Entry{
		ActorID:    actorID,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		At:         uc.d.Clock.Now(),
		IP:         identity.IPFrom(ctx),
		RequestID:  identity.RequestIDFrom(ctx),
	}); auditErr != nil {
		uc.d.Log.Error(ctx, "audit "+action, "err", auditErr)
	}
}
