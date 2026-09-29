package module

import (
	"context"
	"errors"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/user"
	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/golibinfra/auditkit"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

type userReaderAdapter struct {
	uc *user.Usecase
}

func (a *userReaderAdapter) GetByID(ctx context.Context, id string) (*verification.User, error) {
	u, err := a.uc.LookupByID(ctx, id)
	if err != nil {
		return nil, mapUserErr(err)
	}
	return userToVerification(u), nil
}

func (a *userReaderAdapter) GetByEmail(ctx context.Context, email string) (*verification.User, error) {
	u, err := a.uc.LookupByEmail(ctx, email)
	if err != nil {
		return nil, mapUserErr(err)
	}
	return userToVerification(u), nil
}

// Ports carry consumer-domain sentinels: callers can't errors.Is against user's (archtest).
func mapUserErr(err error) error {
	if errors.Is(err, user.ErrNotFound) {
		return verification.ErrUserNotFound
	}
	return err
}

func userToVerification(u *user.User) *verification.User {
	return &verification.User{
		ID:      u.ID,
		Email:   u.Email,
		Active:  u.Status == user.StatusActive,
		Version: u.Version,
	}
}

type logMailerAdapter struct {
	log   logkit.Logger
	audit auditkit.Logger
	clock clockkit.Clock
}

func newLogMailerAdapter(log logkit.Logger, auditLog auditkit.Logger, clock clockkit.Clock) verification.Mailer {
	return &logMailerAdapter{log: log.With("component", "mailer"), audit: auditLog, clock: clock}
}

func (m *logMailerAdapter) SendPasswordReset(ctx context.Context, email, token string) error {
	m.log.Info(ctx, "would send password reset link", "email", email, "token", token)
	return m.audit.Log(ctx, auditkit.Entry{
		Action:   "mail_sent",
		Resource: "password_reset",
		At:       m.clock.Now(),
		Metadata: map[string]any{"email": email},
	})
}

func (m *logMailerAdapter) SendEmailVerification(ctx context.Context, email, token string) error {
	m.log.Info(ctx, "would send email verification link", "email", email, "token", token)
	return m.audit.Log(ctx, auditkit.Entry{
		Action:   "mail_sent",
		Resource: "email_verification",
		At:       m.clock.Now(),
		Metadata: map[string]any{"email": email},
	})
}
