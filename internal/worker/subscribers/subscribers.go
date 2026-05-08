package subscribers

import (
	"context"
	"time"

	"git.stit.tech/akadmin/golibinfra/logkit"

	"git.stit.tech/akadmin/go-backend-template/internal/domain/user"
	"git.stit.tech/akadmin/go-backend-template/internal/domain/verification"
	"git.stit.tech/akadmin/golibinfra/auditkit"
)

const (
	SubjectUserRegistered             = "user.events.registered"
	SubjectEmailVerificationRequested = "verification.events.email_verification_requested"
	SubjectPasswordResetRequested     = "verification.events.password_reset_requested"
)

func OnUserRegistered(log logkit.Logger, auditLog auditkit.Logger) func(context.Context, user.UserRegistered) error {
	return func(ctx context.Context, e user.UserRegistered) error {
		log.Info(ctx, "user registered: id="+e.ID, "user_id", e.ID, "email", e.Email)
		if auditLog != nil {
			_ = auditLog.Log(ctx, auditkit.Entry{
				ActorID:    e.ID,
				Action:     "user.registered.observed",
				Resource:   "user",
				ResourceID: e.ID,
				At:         time.Now(),
			})
		}
		return nil
	}
}

func OnEmailVerificationRequested(log logkit.Logger, auditLog auditkit.Logger) func(context.Context, verification.EmailVerificationRequested) error {
	return func(ctx context.Context, e verification.EmailVerificationRequested) error {
		log.Info(ctx, "would send verification email to "+e.Email+" (token_id="+e.TokenID+")",
			"user_id", e.UserID, "email", e.Email, "token_id", e.TokenID)
		if auditLog != nil {
			_ = auditLog.Log(ctx, auditkit.Entry{
				ActorID:    e.UserID,
				Action:     "email_verification.dispatch",
				Resource:   "email_verification",
				ResourceID: e.TokenID,
				At:         time.Now(),
			})
		}
		return nil
	}
}

func OnPasswordResetRequested(log logkit.Logger, auditLog auditkit.Logger) func(context.Context, verification.PasswordResetRequested) error {
	return func(ctx context.Context, e verification.PasswordResetRequested) error {
		log.Info(ctx, "would send password reset email to "+e.Email+" (token_id="+e.TokenID+")",
			"user_id", e.UserID, "email", e.Email, "token_id", e.TokenID)
		if auditLog != nil {
			_ = auditLog.Log(ctx, auditkit.Entry{
				ActorID:    e.UserID,
				Action:     "password_reset.dispatch",
				Resource:   "password_reset",
				ResourceID: e.TokenID,
				At:         time.Now(),
			})
		}
		return nil
	}
}
